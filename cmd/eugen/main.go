package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"eugen/internal/cmdvalidator"
	"eugen/internal/config"
	"eugen/internal/context"
	"eugen/internal/diagnostic"
	"eugen/internal/executor"
	"eugen/internal/inference"
	"eugen/internal/loganalyzer"
	"eugen/internal/ollama"
	"eugen/internal/openai"
	"eugen/internal/planner"
	"eugen/internal/plugin"
	"eugen/internal/sysdb"
)

const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
)

func main() {
	verbose := flag.Bool("v", false, "Aktiviere Verbose-Modus (zeigt Prompts und API Payload an)")
	logFile := flag.String("f", "", "Pfad zu einer Logdatei, die in den Kontext geladen werden soll")
	populateDB := flag.Bool("p", false, "Populiert die lokales Systemdatenbank mit Hardware/Paket-Wissen (Beendet Eugen danach)")
	resetDB := flag.Bool("r", false, "Leert/löscht die Systemdatenbank (Beendet Eugen danach)")
	flag.Parse()

	// Load configuration from /etc/eugen/eugen.conf
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("%s\u26A0\uFE0F Hinweis: Systemweite Konfiguration (/etc/eugen) nicht schreibbar (Root-Rechte benötigt?). Verwende Standardkonfiguration.%s\n", ColorYellow, ColorReset)
		cfg = config.DefaultConfig()
	}

	if *resetDB {
		if err := sysdb.ResetDB(); err != nil {
			fmt.Printf("%s\u274C Fehler beim Löschen der Datenbank: %v%s\n", ColorRed, err, ColorReset)
			os.Exit(1)
		}
		fmt.Printf("%s\u2714\uFE0F Systemdatenbank erfolgreich geleert.%s\n", ColorGreen, ColorReset)
		os.Exit(0)
	}

	if *populateDB {
		fmt.Printf("%s\u2139 Starte System-Indexing (Populate)...\nDies kann einen kleinen Moment dauern...%s\n\n", ColorBlue, ColorReset)
		data, err := sysdb.GatherSystemPopulate()
		if err != nil {
			fmt.Printf("\n%s\u274C Fehler beim Sammeln der Systemdaten: %v%s\n", ColorRed, err, ColorReset)
			os.Exit(1)
		}
		
		if err := sysdb.SaveDB(data); err != nil {
			fmt.Printf("\n%s\u274C Fehler beim Speichern der Datenbank: %v%s\n", ColorRed, err, ColorReset)
			os.Exit(1)
		}
		
		fmt.Printf("\n%s\u2714\uFE0F Systemdaten erfolgreich indexiert und als '%s' gespeichert.%s\n", ColorGreen, sysdb.DBPath(), ColorReset)
		fmt.Printf("Du kannst Eugen nun normal aufrufen (ohne '-p'). Er wird das neue Wissen automatisch nutzen.\n")
		os.Exit(0)
	}

	fmt.Printf("%s%s - Lokaler SLES/openSUSE Assistent%s\n", ColorBlue, cfg.AssistantName, ColorReset)
	fmt.Printf("Tippe 'help' oder '?' für eine Übersicht der Funktionen.\n")

	// Create inference backend from config
	backend, err := createBackend(cfg)
	if err != nil {
		fmt.Printf("%s\u274C Fehler beim Erstellen des Backends: %v%s\n", ColorRed, err, ColorReset)
		fmt.Printf("Prüfe deine Konfiguration in: %s\n", config.ConfigPath())
		os.Exit(1)
	}

	fmt.Printf("Backend: %s%s%s | ", ColorCyan, backend.Name(), ColorReset)
	if cfg.Backend == config.BackendOllama {
		fmt.Printf("URL: %s | Modell: %s\n", cfg.OllamaURL, cfg.OllamaModel)
	}
	fmt.Printf("Konfiguration: %s%s%s\n", ColorCyan, config.ConfigPath(), ColorReset)

	// Create modules
	execng := executor.NewExecutor()
	analyzer := loganalyzer.NewAnalyzer()
	plng := planner.NewPlanner(backend, cfg)
	cmdval := cmdvalidator.NewValidator(backend, cfg)

	// Initial system context for LLM
	sysContext := context.GatherSystemInfo()
	
	if sysdb.CheckDBExists() {
		fmt.Printf("%s\u2714\uFE0F System-Datenbank gefunden! Lade lokales Systemwissen...%s\n", ColorGreen, ColorReset)
		dbData, err := sysdb.LoadDB()
		if err == nil {
			sysContext += fmt.Sprintf("\nLokale %s Systemdatenbank (Eingelesen via Offline-Statistik):\nHardware: %s\nNetwork: %s\nActive Services: %s\n", 
				cfg.AssistantName, dbData.HardwareInfo, dbData.NetworkInfo, dbData.Services)
			
			if len(dbData.CustomNotes) > 0 {
				sysContext += "\nZusätzliche Benutzer-Notizen zur Infrastruktur:\n- " + strings.Join(dbData.CustomNotes, "\n- ") + "\n"
			}
		} else {
			fmt.Printf("%s\u26A0 Fehler beim Lesen der Systemdatenbank: %v%s\n", ColorYellow, err, ColorReset)
		}
	} else {
		fmt.Printf("%s\u2139 Tipp: Rufe %s einmalig mit '-p' auf, um sein Offline-Systemwissen drastisch auszubauen.%s\n", ColorYellow, cfg.AssistantName, ColorReset)
	}

	// Load local plugins
	plugins, pErr := plugin.LoadPlugins(filepath.Join(config.GetDataDir(), "plugins"))
	if pErr == nil && len(plugins) > 0 {
		sysContext += plugin.FormatPluginContext(plugins)
		fmt.Printf("%s\u2139 %d Admin-Plugins erfolgreich in den Kontext geladen.%s\n", ColorGreen, len(plugins), ColorReset)
	}
	
	if *logFile != "" {
		filteredContent, bytesScanned, err := filterLogFile(*logFile)
		if err != nil {
			fmt.Printf("%s\u26A0 Konnte Logdatei '%s' nicht lesen: %v%s\n", ColorRed, *logFile, err, ColorReset)
		} else {
			if filteredContent == "" {
				filteredContent = "Regulärer Scan: Keine auffälligen ERROR/WARN/CRITICAL Schüsselwörter gefunden."
			}
			sysContext += fmt.Sprintf("\n[Gefilterte Fehler-Extraktion aus '%s' (Originalgröße: %d Bytes)]\n%s\n", *logFile, bytesScanned, filteredContent)
			fmt.Printf("%s\u2139 Logdatei '%s' (ca. %d Bytes analysiert) smart gefiltert und in den System-Kontext geladen.%s\n", ColorCyan, *logFile, bytesScanned, ColorReset)
		}
	}
	
	// Load and build dynamic Vector RAG DB
	fmt.Printf("%s\u23F3 Initialisiere Vektor-Datenbank für lokales RAG...%s\n", ColorCyan, ColorReset)
	vecStore := context.BuildVectorDatabase(backend, *verbose)
	if len(vecStore.Chunks) > 0 {
		fmt.Printf("%s\u2139 RAG: Vektor-Datenbank mit %d Wissens-Chunks erfolgreich geladen.%s\n", ColorGreen, len(vecStore.Chunks), ColorReset)
	} else {
		fmt.Printf("%s\u2139 RAG: Vektor-Datenbank ist leer (0 Chunks). Falls Dateien existieren, gab es womöglich einen Embedding-Fehler.%s\n", ColorYellow, ColorReset)
	}
	
	// Prepare base system prompt from config template
	systemPrompt := cfg.RenderPrompt(cfg.PromptSystem, map[string]string{
		"context": sysContext,
	})

	// Konversations-Gedächtnis: Ringpuffer der letzten maxHistory Austausche
	const maxHistory = 10 // 10 Austuasche = 20 Messages (user+assistant)
	var chatHistory []inference.Message

	// REPL Loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("\n%s\u279C %s> %s", ColorGreen, strings.ToLower(cfg.AssistantName), ColorReset)
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "exit" || input == "quit" {
			fmt.Println("Bis bald!")
			break
		}
		if input == "help" || input == "?" {
			printHelp(cfg)
			continue
		}
		if input == "validation on" || input == "validate on" {
			cfg.ValidationEnabled = true
			fmt.Printf("%s\u2714\uFE0F Befehls-Validierung gegen Hilfe/man-Pages wurde AKTIVIERT.%s\n", ColorGreen, ColorReset)
			continue
		}
		if input == "validation off" || input == "validate off" {
			cfg.ValidationEnabled = false
			fmt.Printf("%s\u26A0 Befehls-Validierung wurde DEAKTIVIERT! Vorgeschlagene Befehle werden ungeschützt ausgeführt.%s\n", ColorYellow, ColorReset)
			continue
		}
		if input == "rag on" {
			cfg.RagEnabled = true
			fmt.Printf("%s\u2714\uFE0F Dynamische RAG Vektor-Suche wurde AKTIVIERT.%s\n", ColorGreen, ColorReset)
			continue
		}
		if input == "rag off" {
			cfg.RagEnabled = false
			fmt.Printf("%s\u26A0 Dynamische RAG Vektor-Suche wurde DEAKTIVIERT.%s\n", ColorYellow, ColorReset)
			continue
		}
		if input == "" {
			continue
		}

		var response string

		if strings.HasPrefix(input, "plan ") {
			taskStr := strings.TrimSpace(strings.TrimPrefix(input, "plan "))
			fmt.Printf("%s\u23F3 %s erstellt einen Ausführungsplan...%s\n", ColorCyan, cfg.AssistantName, ColorReset)
			
			zypperCtx := context.SearchPackageKeywords(taskStr)
			rpmCtx := context.CheckInstalledPackages(taskStr)
			activeContext := sysContext + zypperCtx + rpmCtx

			// Vector RAG Search
			if cfg.RagEnabled {
				ragCtx := vecStore.Search(backend, taskStr, 3, *verbose)
				activeContext += "\n" + ragCtx
			}
			
			if *verbose {
				fmt.Printf("\n%s[VERBOSE] Plan-Context:%s\n%s\n", ColorYellow, ColorReset, activeContext)
				fmt.Printf("%s[VERBOSE] Task:%s\n%s\n\n", ColorYellow, ColorReset, taskStr)
			}
			
			commands, planTxt, pErr := plng.CreatePlan(activeContext, taskStr)
			if pErr != nil {
				fmt.Printf("%s\u274C Fehler bei der Planung: %v%s\n", ColorRed, pErr, ColorReset)
				continue
			}

			fmt.Printf("\n%s\n", planTxt)
			if len(commands) == 0 {
				fmt.Printf("%sKeine ausführbaren Befehle im Plan gefunden.%s\n", ColorYellow, ColorReset)
				continue
			}

			fmt.Printf("\n%sErkannte Befehlssequenz:%s\n", ColorYellow, ColorReset)
			for i, c := range commands {
				fmt.Printf(" [%d] %s\n", i+1, c)
			}
			
			fmt.Printf("\nWas möchtest du tun? [A]usführen / [E]xportieren / [X]Abbrechen : ")
			scanner.Scan()
			ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if ans == "a" || ans == "ausführen" {
				for _, c := range commands {
					risk := execng.AnalyzeCommand(c)
					if risk == executor.RiskHigh {
						fmt.Printf("%s\u26A0 Überspringe hoch riskanten Befehl ohne EXECUTE-Bestätigung: %s%s\n", ColorRed, c, ColorReset)
						continue
					}
					runCommand(execng, c)
				}
			} else if ans == "e" || ans == "exportieren" {
				scriptName := strings.ToLower(cfg.AssistantName) + "_plan.sh"
				scriptContent := "#!/bin/bash\n\n# " + cfg.AssistantName + " Plan: " + taskStr + "\n\n" + strings.Join(commands, "\n") + "\n"
				os.WriteFile(scriptName, []byte(scriptContent), 0755)
				fmt.Printf("%sPlan exportiert nach %s%s\n", ColorGreen, scriptName, ColorReset)
			} else {
				fmt.Println("Plan verworfen.")
			}
			continue
		} else if input == "save" || input == "export" || input == "save chat" {
			if len(chatHistory) == 0 {
				fmt.Printf("%s\u2139 Die Chat-Historie ist leer.%s\n", ColorYellow, ColorReset)
				continue
			}
			
			timestamp := time.Now().Format("2006-01-02_15-04-05")
			filename := filepath.Join(config.GetDataDir(), fmt.Sprintf("chat_%s.md", timestamp))
			
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("# %s Chat Export - %s\n\n", cfg.AssistantName, time.Now().Format("02.01.2006 15:04")))
			
			for _, msg := range chatHistory {
				if msg.Role == "user" {
					sb.WriteString(fmt.Sprintf("## You\n\n%s\n\n", msg.Content))
				} else if msg.Role == "assistant" {
					sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n---\n\n", cfg.AssistantName, msg.Content))
				}
			}
			
			err := os.WriteFile(filename, []byte(sb.String()), 0644)
			if err != nil {
				fmt.Printf("%s\u274C Fehler beim Speichern: %v%s\n", ColorRed, err, ColorReset)
			} else {
				fmt.Printf("%s\u2714\uFE0F Chat als Markdown gespeichert unter: %s%s\n", ColorGreen, filename, ColorReset)
			}
			continue
		} else if strings.HasPrefix(input, "db add ") {
			note := strings.TrimSpace(strings.TrimPrefix(input, "db add "))
			err := sysdb.AddCustomNote(note)
			if err != nil {
				fmt.Printf("%s\u274C Fehler beim Speichern der Notiz in die DB: %v%s\n", ColorRed, err, ColorReset)
			} else {
				fmt.Printf("%s\u2714\uFE0F Notiz erfolgreich zur Datenbank hinzugefügt.%s\n", ColorGreen, ColorReset)
				sysContext += "\nZusätzliche Benutzer-Notiz zur Infrastruktur: " + note + "\n"
			}
			continue
		} else if input == "db show" || input == "db list" {
			if !sysdb.CheckDBExists() {
				fmt.Printf("%s\u2139 Die Datenbank ist leer. Nutze 'eugen -p' oder schreibe etwas mit 'db add <info>'.%s\n", ColorYellow, ColorReset)
				continue
			}
			dbData, err := sysdb.LoadDB()
			if err != nil {
				fmt.Printf("%s\u274C Fehler beim Lesen: %v%s\n", ColorRed, err, ColorReset)
				continue
			}
			fmt.Printf("\n%s--- %s System-Datenbank ---%s\n", ColorBlue, cfg.AssistantName, ColorReset)
			fmt.Printf("%sHardware:%s\n%s\n", ColorYellow, ColorReset, dbData.HardwareInfo)
			fmt.Printf("%sNetzwerk:%s\n%s\n", ColorYellow, ColorReset, dbData.NetworkInfo)
			fmt.Printf("%sService-Count:%s %d\n", ColorYellow, ColorReset, len(strings.Split(dbData.Services, "\n")))
			if len(dbData.CustomNotes) > 0 {
				fmt.Printf("\n%sEigene Notizen:%s\n", ColorGreen, ColorReset)
				for i, n := range dbData.CustomNotes {
					fmt.Printf(" [%d] %s\n", i+1, n)
				}
			} else {
				fmt.Printf("\nKeine manuellen Notizen hinterlegt.\n")
			}
			continue
		} else if input == "diagnose" || input == "supportconfig" {
			if err := diagnostic.EnsureSupportutils(); err != nil {
				fmt.Printf("%s\u274C supportutils Überprüfung/Installation fehlgeschlagen: %v%s\n", ColorRed, err, ColorReset)
				continue
			}

			fmt.Printf("\n%s\u2139 Wähle das Diagnose-Level für supportconfig:%s\n", ColorBlue, ColorReset)
			fmt.Printf(" [1] Minimal (Dauert ca. 1 Minute, stark aggregiert)\n")
			fmt.Printf(" [2] Komplett (Dauert oft mehrere Minuten, voller System-Dump)\n")
			fmt.Printf(" [X] Abbrechen\n")
			fmt.Print("Auswahl: ")
			
			scanner.Scan()
			lvl := strings.TrimSpace(scanner.Text())
			if strings.ToLower(lvl) == "x" || strings.ToLower(lvl) == "abbrechen" {
				fmt.Println("Diagnose abgebrochen.")
				continue
			}

			minimal := true
			if lvl == "2" {
				minimal = false
			} else if lvl != "1" {
				fmt.Println("Ungültige Eingabe, Abbruch.")
				continue
			}

			tarPath, sErr := diagnostic.RunSupportconfig(minimal)
			if sErr != nil {
				fmt.Printf("\n%s\u274C %v%s\n", ColorRed, sErr, ColorReset)
				continue
			}
			fmt.Printf("[\u2714\uFE0F] Tarball gesichert in: %s\n", tarPath)

			extText, eErr := diagnostic.ExtractAndFilter(tarPath)
			if eErr != nil {
				fmt.Printf("\n%s\u274C Fehler beim Auslesen des Dumps: %v%s\n", ColorRed, eErr, ColorReset)
				continue
			}

			analyzePrompt := cfg.RenderPrompt(cfg.PromptDiagnose, map[string]string{
				"context": extText,
			})

			if *verbose {
				printLen := 500
				if len(analyzePrompt) < printLen {
					printLen = len(analyzePrompt)
				}
				fmt.Printf("\n%s[VERBOSE] Analyze-Prompt (gekürzt auf 500 von %d Zeichen):%s\n%s...\n\n", 
					ColorYellow, len(analyzePrompt), ColorReset, analyzePrompt[:printLen])
			}
			
			fmt.Printf("%s\U0001f916 %s wertet das Archiv aus (%d Zeichen an Kontext) und antwortet live:%s\n", ColorCyan, cfg.AssistantName, len(extText), ColorReset)
			response, err = backend.Generate(analyzePrompt, "Bitte werte den Supportconfig-Dump objektiv aus und nenne die kritischsten Fehler.", func(t string) { fmt.Print(t) })
			fmt.Println()
		} else if input == "analyze" || input == "logs" {
			fmt.Printf("%s\u23F3 Lese System-Logs (journalctl & dmesg)...%s\n", ColorCyan, ColorReset)
			
			journalLogs, jErr := analyzer.GetRecentErrors(40)
			if jErr != nil {
				fmt.Printf("%s\u26A0 journalctl info: %v%s\n", ColorYellow, jErr, ColorReset)
			}
			dmesgLogs, dErr := analyzer.GetDmesgErrors(20)
			if dErr != nil {
				fmt.Printf("%s\u26A0 dmesg info: %v%s\n", ColorYellow, dErr, ColorReset)
			}
			
			analyzePrompt := cfg.RenderPrompt(cfg.PromptLogAnalysis, map[string]string{
				"journalctl": journalLogs,
				"dmesg":      dmesgLogs,
			})
			if *verbose {
				fmt.Printf("\n%s[VERBOSE] Analyze-Prompt:%s\n%s\n\n", ColorYellow, ColorReset, analyzePrompt)
			}
			
			fmt.Printf("%s\U0001f916 %s analysiert die Logs und antwortet live:%s\n", ColorCyan, cfg.AssistantName, ColorReset)
			response, err = backend.Generate(analyzePrompt, "Bitte analysiere diese Fehlermeldungen und gib mir Lösungsansätze.", func(t string) { fmt.Print(t) })
			fmt.Println()
		} else if input == "health" || input == "status" || input == "check" {
			fmt.Printf("%s\u23F3 Führe schnellen System-Health-Check durch...%s\n", ColorCyan, ColorReset)
			
			healthCtx := diagnostic.QuickHealthCheck()
			
			analyzePrompt := cfg.RenderPrompt(cfg.PromptHealthCheck, map[string]string{
				"context": healthCtx,
			})
			
			if *verbose {
				fmt.Printf("\n%s[VERBOSE] Health-Prompt:%s\n%s\n\n", ColorYellow, ColorReset, analyzePrompt)
			}
			
			fmt.Printf("%s\U0001f916 %s wertet den Systemzustand live aus:%s\n", ColorCyan, cfg.AssistantName, ColorReset)
			response, err = backend.Generate(analyzePrompt, "Hier ist der aktuelle Systemzustand. Bitte gib mir eine schnelle, präzise und kreative Zusammenfassung.", func(t string) { fmt.Print(t) })
			fmt.Println()
		} else {
			zypperCtx := context.SearchPackageKeywords(input)
			rpmCtx := context.CheckInstalledPackages(input)
			activePrompt := systemPrompt
			if zypperCtx != "" {
				fmt.Printf("%s\u2139 Lokale Suche in zypper abgeschlossen.%s\n", ColorBlue, ColorReset)
				activePrompt += "\n" + zypperCtx
			}
			if rpmCtx != "" {
				fmt.Printf("%s\u2139 Lokale Suche in rpm abgeschlossen.%s\n", ColorBlue, ColorReset)
				activePrompt += "\n" + rpmCtx
			}

			// Vector RAG Search
			if cfg.RagEnabled {
				ragCtx := vecStore.Search(backend, input, 3, *verbose)
				if ragCtx != "" {
					activePrompt += "\n" + ragCtx
				}
			}
			
			// Baue Nachrichten-Array mit System-Prompt, Historie und aktuellem Input
			var messages []inference.Message
			messages = append(messages, inference.Message{Role: "system", Content: activePrompt})
			messages = append(messages, chatHistory...)
			messages = append(messages, inference.Message{Role: "user", Content: input})
			
			if *verbose {
				fmt.Printf("\n%s[VERBOSE] System Prompt:%s\n%s\n", ColorYellow, ColorReset, activePrompt)
				fmt.Printf("%s[VERBOSE] Chat-Historie: %d Nachrichten im Gedächtnis%s\n", ColorYellow, len(chatHistory), ColorReset)
				fmt.Printf("%s[VERBOSE] User Prompt:%s\n%s\n\n", ColorYellow, ColorReset, input)
			}
			
			fmt.Printf("%s\U0001f916 %s antwortet:%s\n", ColorCyan, cfg.AssistantName, ColorReset)
			response, err = backend.Chat(messages, func(t string) { fmt.Print(t) })
			fmt.Println()
			
			// Bei Erfolg: Austausch ins Gedächtnis schreiben
			if err == nil {
				chatHistory = append(chatHistory,
					inference.Message{Role: "user", Content: input},
					inference.Message{Role: "assistant", Content: response},
				)
				// Ringpuffer: Älteste Austausche kappen wenn > maxHistory Paare
				if len(chatHistory) > maxHistory*2 {
					chatHistory = chatHistory[len(chatHistory)-maxHistory*2:]
				}
			}
		}

		if err != nil {
			fmt.Printf("%s\u274C Fehler bei der Kommunikation mit dem Backend: %v%s\n", ColorRed, err, ColorReset)
			continue
		}

		// Befehle aus der Antwort extrahieren
		cmdsToExecute := extractCommands(response)
		
		// Da die Antwort bereits gestreamt wurde, müssen wir den Text nicht nochmal printen.
		// Wir parsen nur die Befehle und validieren sie für die Ausführungs-Abfrage.
		if len(cmdsToExecute) > 0 {
			if cfg.ValidationEnabled {
				cmdsToExecute = cmdval.ValidateCommands(cmdsToExecute, *verbose)
			} else {
				fmt.Printf("\n%s\u26A0 Befehls-Validierung ist DEAKTIVIERT! Die Befehle werden ungeprüft vorgeschlagen.%s\n", ColorYellow, ColorReset)
			}
		}
		
		if len(cmdsToExecute) > 0 {
			if len(cmdsToExecute) == 1 {
				handleExecution(execng, backend, cfg, cmdsToExecute[0], scanner)
			} else {
				fmt.Printf("\n%s\u2139 %s hat mehrere Befehle vorgeschlagen:%s\n", ColorBlue, cfg.AssistantName, ColorReset)
				for i, c := range cmdsToExecute {
					relRisk := execng.AnalyzeCommand(c)
					riskIndicator := ""
					if relRisk == executor.RiskHigh {
						riskIndicator = ColorRed + " [! HOCHRISIKO]" + ColorReset
					} else if relRisk == executor.RiskMedium {
						riskIndicator = ColorYellow + " [Achtung]" + ColorReset
					}
					fmt.Printf(" [%d] %s%s\n", i+1, c, riskIndicator)
				}
				fmt.Printf("\nWelchen Befehl ausführen? (Nummer) oder [A]lle / [X]Abbrechen: ")
				scanner.Scan()
				ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
				
				if ans == "a" || ans == "alle" {
					for _, c := range cmdsToExecute {
						handleExecution(execng, backend, cfg, c, scanner)
					}
				} else if ans == "x" || ans == "abbrechen" || ans == "" {
					fmt.Println("Abbruch.")
				} else {
					idx, err := strconv.Atoi(ans)
					if err == nil && idx > 0 && idx <= len(cmdsToExecute) {
						handleExecution(execng, backend, cfg, cmdsToExecute[idx-1], scanner)
					} else {
						fmt.Println("Ungültige Auswahl, Abbruch.")
					}
				}
			}
		}
	}
}

func handleExecution(execng *executor.Executor, backend inference.Backend, cfg *config.EugenConfig, cmdToExecute string, scanner *bufio.Scanner) {
	risk := execng.AnalyzeCommand(cmdToExecute)
			
	if risk == executor.RiskHigh {
		fmt.Printf("\n%s\u26A0 WARNUNG: Dieser Befehl ist potenziell DESTRUKTIV!%s\n", ColorRed, ColorReset)
		fmt.Printf("Befehl: %s%s%s\n", ColorRed, cmdToExecute, ColorReset)
		fmt.Printf("Zum Ausführen bitte %sEXECUTE%s eintippen (oder leer lassen zum Abbruch): ", ColorRed, ColorReset)
		
		scanner.Scan()
		confirm := strings.TrimSpace(scanner.Text())
		if confirm == "EXECUTE" {
			promptForSnapshotAndRun(execng, cmdToExecute, scanner)
		} else {
			fmt.Println("Abbruch.")
		}

	} else if risk == executor.RiskMedium {
		fmt.Printf("\n%s\u26A0 Systemveränderung detektiert.%s\n", ColorYellow, ColorReset)
		fmt.Printf("Befehl: %s%s%s\n", ColorYellow, cmdToExecute, ColorReset)
		fmt.Printf("Befehl ausführen? [J]a / [N]ein / [A]npassen / [E]rklären : ")
		
		scanner.Scan()
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans == "j" || ans == "ja" {
			promptForSnapshotAndRun(execng, cmdToExecute, scanner)
		} else if ans == "a" || ans == "anpassen" {
			fmt.Print("Neuer Befehl: ")
			scanner.Scan()
			modCmd := strings.TrimSpace(scanner.Text())
			if modCmd != "" {
				promptForSnapshotAndRun(execng, modCmd, scanner)
			}
		} else if ans == "e" || ans == "erklären" {
			explainPrompt := fmt.Sprintf("Erkläre mir ganz kurz und prägnant, was dieser Befehl macht und welche Flags genutzt werden:\n`%s`", cmdToExecute)
			fmt.Printf("\n%s\U0001f916 Erklärung:%s\n", ColorCyan, ColorReset)
			_, _ = backend.Generate("Du erklärst Bash-Befehle präzise.", explainPrompt, func(t string) { fmt.Print(t) })
			fmt.Println()
			// Retry execution after explanation
			handleExecution(execng, backend, cfg, cmdToExecute, scanner)
		} else {
			fmt.Println("Abbruch.")
		}
	} else {
		// Low risk
		fmt.Printf("\nErkannter Befehl: %s\n", cmdToExecute)
		fmt.Printf("Ausführen? [J/n]: ")
		scanner.Scan()
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans == "" || ans == "j" || ans == "ja" {
			runCommand(execng, cmdToExecute)
		} else {
			fmt.Println("Abbruch.")
		}
	}
}

func promptForSnapshotAndRun(execng *executor.Executor, cmdToExecute string, scanner *bufio.Scanner) {
	if execng.CheckIfBtrfs() && execng.IsSnapperInstalled() {
		fmt.Printf("\n%s\u2139 BTRFS System detektiert.%s Vor der Ausführung einen Snapper-Snapshot anlegen? [J/n]: ", ColorBlue, ColorReset)
		scanner.Scan()
		ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if ans == "" || ans == "j" || ans == "ja" {
			fmt.Printf("%s\u23F3 Erstelle Snapshot...%s", ColorCyan, ColorReset)
			err := execng.CreateSnapshot(cmdToExecute)
			if err != nil {
				fmt.Printf("\n%s\u26A0 Snapshot fehlgeschlagen: %v%s\n", ColorYellow, err, ColorReset)
			} else {
				fmt.Printf("\n%s\u2714\uFE0F Snapshot erfolgreich erstellt.%s\n", ColorGreen, ColorReset)
			}
		}
	}
	runCommand(execng, cmdToExecute)
}

// stripCodeBlocks entfernt Code-Blöcke und Standalone-Backtick-Zeilen aus der Antwort,
// sodass nur der Erklärungstext übrig bleibt.
func stripCodeBlocks(resp string) string {
	lines := strings.Split(resp, "\n")
	var result []string
	inBlock := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "```") {
			inBlock = !inBlock
			continue
		}
		if inBlock {
			continue
		}
		// Standalone backtick commands überspringen
		if strings.HasPrefix(trimmed, "`") && strings.HasSuffix(trimmed, "`") && len(trimmed) > 2 {
			continue
		}
		result = append(result, l)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// filterLogFile reads a log file line by line and filters for critical keywords
// to avoid overflowing the AI context with megabytes of debug logs.
func filterLogFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Some log lines can be huge
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var matched []string
	keywords := []string{"error", "fail", "warn", "panic", "critical", "crit", "emerg", "alert", "err"}
	
	var bytesScanned int64
	for scanner.Scan() {
		line := scanner.Text()
		bytesScanned += int64(len(line))
		
		lowerL := strings.ToLower(line)
		for _, k := range keywords {
			if strings.Contains(lowerL, k) {
				matched = append(matched, strings.TrimSpace(line))
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", bytesScanned, err
	}
	
	// Keep the last 150 matched lines at most to ensure context safety
	maxLines := 150
	if len(matched) > maxLines {
		matched = matched[len(matched)-maxLines:]
	}
	
	res := strings.Join(matched, "\n")
	return strings.ToValidUTF8(res, ""), bytesScanned, nil
}

// extractCommands greift ausführbare Befehle aus der Antwort ab.
// Jeder Code-Block (```...```) wird als EIN zusammenhängender Befehl behandelt,
// damit mehrzeilige Konstrukte (for/done, if/fi) nicht zerstückelt werden.
// Inline-Backtick-Befehle werden nur akzeptiert, wenn sie mit einem bekannten
// System-Kommando starten, um reine Beispiele/Illustrationen auszufiltern.
func extractCommands(resp string) []string {
	// Strip chain-of-thought <think>...</think> blocks
	resp = stripThinkBlocks(resp)

	var cmds []string
	lines := strings.Split(resp, "\n")
	
	inBlock := false
	var blockLines []string
	
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "```") {
			if inBlock {
				// Block endet: Zeilen einzeln als Befehle extrahieren
				for _, bl := range blockLines {
					bl = strings.TrimSpace(bl)
					if bl == "" {
						continue
					}
					// Skip pure comment lines
					if strings.HasPrefix(bl, "#") {
						continue
					}
					// Skip lines that are config file content (e.g. [Service], CPUQuota=)
					if strings.HasPrefix(bl, "[") || (strings.Contains(bl, "=") && !strings.Contains(bl, " ") && !strings.HasPrefix(bl, "export")) {
						continue
					}
					// Skip lines that look like a prompt or description
					if strings.HasPrefix(bl, "# ") {
						continue
					}
					// Remove inline trailing comments (e.g. "sudo foo  # explanation")
					if idx := strings.Index(bl, " # "); idx > 0 {
						bl = strings.TrimSpace(bl[:idx])
					}
					if bl != "" && looksLikeExecutableCommand(bl) {
						cmds = append(cmds, bl)
					}
				}
				blockLines = nil
			}
			inBlock = !inBlock
			continue 
		}
		
		if inBlock {
			if trimmed != "" {
				blockLines = append(blockLines, trimmed)
			}
			continue
		}
		
		// Inline-Backticks: Nur echte ausführbare Kommandos akzeptieren
		if strings.HasPrefix(trimmed, "`") && strings.HasSuffix(trimmed, "`") && len(trimmed) > 2 {
			candidate := strings.Trim(trimmed, "`")
			if looksLikeExecutableCommand(candidate) {
				cmds = append(cmds, candidate)
			}
		}
	}
	return cmds
}

// stripThinkBlocks removes chain-of-thought <think>...</think> blocks.
func stripThinkBlocks(s string) string {
	for {
		start := strings.Index(s, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(s, "</think>")
		if end == -1 {
			s = s[:start]
			break
		}
		s = s[:start] + s[end+len("</think>"):]
	}
	s = strings.ReplaceAll(s, "</think>", "")
	return s
}

// looksLikeExecutableCommand filtert reine Tool-Namen und Beispiele heraus.
// Gibt nur true zurück wenn der Befehl wie ein echtes, ausführbares Kommando aussieht.
func looksLikeExecutableCommand(cmd string) bool {
	// Zu kurz oder nur ein einzelnes Wort → eher ein Inline-Verweis, kein Befehl
	parts := strings.Fields(cmd)
	if len(parts) < 2 {
		return false
	}
	
	// Muss mit einem bekannten Kommando oder im PATH existierenden Binary starten
	base := parts[0]
	if base == "sudo" && len(parts) > 1 {
		base = parts[1]
	}
	
	// Shell Builtins (da diese von LookPath nicht gefunden werden)
	builtins := []string{"export", "source", "echo", "cd", "alias", "bg", "fg", "jobs", "set", "history", "exit", "logout"}
	for _, b := range builtins {
		if base == b {
			return true
		}
	}
	
	// Dynamische Prüfung via $PATH
	if _, err := exec.LookPath(base); err == nil {
		return true
	}
	
	return false
}

func runCommand(e *executor.Executor, cmd string) {
	fmt.Printf("\n%sFühre aus: %s%s\n", ColorCyan, cmd, ColorReset)
	_, err := e.Execute(cmd)
	if err != nil {
		fmt.Printf("\n%s\u274C BEFEHL FEHLGESCHLAGEN:%s %v\n", ColorRed, ColorReset, err)
	} else {
		fmt.Printf("\n%s\u2714\uFE0F Befehl abgeschlossen.%s\n", ColorGreen, ColorReset)
	}
}

func printHelp(cfg *config.EugenConfig) {
	name := cfg.AssistantName
	fmt.Printf("\n%s--- %s Hilfe ---%s\n", ColorCyan, name, ColorReset)
	fmt.Printf("Hallo! Ich bin %s, dein lokaler Systemassistent für SLES/openSUSE.\n", name)
	fmt.Printf("Du kannst mir Fragen zur Systemadministration stellen oder mich bitten, Aufgaben zu erledigen.\n\n")
	fmt.Printf("%sFunktionen:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  - %sBefehlsgenerierung%s: Ich wandle natürliche Sprache in Bash-Befehle um.\n", ColorGreen, ColorReset)
	fmt.Printf("  - %sInteraktive Ausführung%s: Ich frage vor dem Ausführen nach und fange Fehler ab.\n", ColorGreen, ColorReset)
	fmt.Printf("  - %sSicherheitsprüfung%s: Gefährliche Befehle werden erkannt und blockiert.\n", ColorGreen, ColorReset)
	fmt.Printf("  - %sOffline%s: Deine Daten bleiben auf dem Rechner. Ich arbeite komplett lokal.\n", ColorGreen, ColorReset)
	fmt.Printf("\n%sBefehle:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  %svalidation off/on%s - Deaktiviert/Aktiviert die man-Page Validierung von KI-Befehlen.\n", ColorGreen, ColorReset)
	fmt.Printf("  %srag off/on%s        - Deaktiviert/Aktiviert die dynamische RAG Vektordatenbank-Suche.\n", ColorGreen, ColorReset)
	fmt.Printf("  %ssave, export%s  - Speichert die aktuelle Chat-Sitzung als Markdown Datei im config-Ordner.\n", ColorGreen, ColorReset)
	fmt.Printf("  %sdb show%s       - Zeigt den Inhalt der lokalen Systemdatenbank an.\n", ColorGreen, ColorReset)
	fmt.Printf("  %sdb add <text>%s - Fügt eigenes Wissen dauerhaft in die DB und den Prompt hinzu.\n", ColorGreen, ColorReset)
	fmt.Printf("  %shealth, check%s - Führt einen sekundenschnellen Basis-Systemcheck (Load, RAM, Disk) aus.\n", ColorGreen, ColorReset)
	fmt.Printf("  %sdiagnose%s      - Lädt oder installiert supportconfig und führt eine SLES Tiefendiagnose durch.\n", ColorGreen, ColorReset)
	fmt.Printf("  %splan <aufgabe>%s- Erstellt ein schrittweises Ausführungsskript für komplexe Tasks.\n", ColorGreen, ColorReset)
	fmt.Printf("  %sanalyze, logs%s - Liest aktuelle kritische Systemlogs aus und bittet die KI um Analyse.\n", ColorGreen, ColorReset)
	fmt.Printf("  %shelp, ?%s       - Zeigt diese Hilfeseite an.\n", ColorGreen, ColorReset)
	fmt.Printf("  %sexit, quit%s    - Beendet den Assistenten.\n\n", ColorGreen, ColorReset)
	fmt.Printf("%sKonfiguration:%s\n", ColorYellow, ColorReset)
	fmt.Printf("  Backend, Modell, Prompts und Name werden über '%s%s%s' konfiguriert.\n", ColorCyan, config.ConfigPath(), ColorReset)
	fmt.Printf("  Beim ersten Start wird automatisch eine Standardkonfiguration erstellt.\n\n")
	fmt.Printf("Optional kannst du %s beim Starten mit Flags konfigurieren:\n", name)
	fmt.Printf("  -v               : Aktiviert den Verbose/Debug-Modus.\n")
	fmt.Printf("  -f <datei>       : Lädt eine Logdatei direkt in den Kontext.\n")
	fmt.Printf("  -p               : Sammelt Systeminformationen in eine statische Datenbank und beendet sich.\n")
	fmt.Printf("  -r               : Leert die Systemdatenbank und beendet sich.\n")
}

// createBackend creates the appropriate inference backend based on configuration.
// This factory lives in main to avoid import cycles between inference and ollama.
func createBackend(cfg *config.EugenConfig) (inference.Backend, error) {
	switch cfg.Backend {
	case config.BackendOllama:
		return ollama.NewClient(cfg.OllamaURL, cfg.OllamaModel, cfg.OllamaEmbedModel), nil
	case config.BackendOpenAI:
	    return openai.NewClient(cfg.OpenAIURL, cfg.OpenAIKey, cfg.OpenAIModel, cfg.OpenAIEmbedModel), nil
	// Future backends:
	// case config.BackendVLLM:
	//     return vllm.NewClient(cfg.VLLMUrl, cfg.VLLMModel), nil
	default:
		return nil, fmt.Errorf("unbekanntes Backend '%s' in eugen.conf. Unterstützt: %s, %s", cfg.Backend, config.BackendOllama, config.BackendOpenAI)
	}
}
