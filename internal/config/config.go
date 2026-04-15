package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigDir is the system-wide configuration directory.
const ConfigDir = "/etc/eugen"

// ConfigFileName is the name of the configuration file inside ConfigDir.
const ConfigFileName = "eugen.conf"

// DataDirName is the name of the directory for export and RAG data.
const DataDirName = "eugen_data"

// GetDataDir returns the path to the user's data directory.
func GetDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return DataDirName
	}
	return filepath.Join(home, DataDirName)
}

// Supported backend types
const (
	BackendOllama = "ollama"
	// Future backends:
	// BackendOpenAI = "openai"
	// BackendVLLM   = "vllm"
)

// EugenConfig holds all runtime configuration for Eugen.
type EugenConfig struct {
	// AssistantName is the name of the assistant (default: "Eugen").
	// Used in prompts via {name} placeholder.
	AssistantName string

	// Backend selects the inference backend ("ollama", future: "openai", "vllm")
	Backend string

	// Ollama settings
	OllamaURL        string
	OllamaModel      string
	OllamaEmbedModel string

	// Reserved for future backends
	// OpenAIURL string
	// OpenAIKey string
	// VLLMUrl   string

	// --- Prompt Templates ---
	// All prompts support the {name} placeholder which gets replaced with AssistantName.
	// They also support {context} where system context is injected.

	// PromptSystem is the main system prompt that defines the assistant's personality.
	PromptSystem string

	// PromptPlan is used by the planner for breaking tasks into command sequences.
	PromptPlan string

	// PromptValidation is used by the command validator for checking command correctness.
	PromptValidation string

	// PromptDiagnose is used when analyzing supportconfig output.
	PromptDiagnose string

	// PromptLogAnalysis is used when analyzing journalctl/dmesg logs.
	PromptLogAnalysis string

	// PromptHealthCheck is used when running a quick system health check.
	PromptHealthCheck string
	
	// ValidationEnabled determines if commands are validated against man/help pages.
	ValidationEnabled bool

	// RagEnabled determines if RAG documents are queried during conversation.
	RagEnabled bool
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *EugenConfig {
	return &EugenConfig{
		AssistantName:     "Eugen",
		Backend:           BackendOllama,
		OllamaURL:         "http://localhost:11434",
		OllamaModel:       "nemotron-cascade-2:latest",
		OllamaEmbedModel:  "nomic-embed-text",
		ValidationEnabled: true,
		RagEnabled:        true,

		PromptSystem: `Du bist {name}, ein hochintelligenter, ressourcenschonender Systemassistent für Administratoren.
Dein Setup: Komplett lokal ausgeführt (Air-Gapped fähig).
Zielgruppe: SLES/openSUSE Administratoren.
Antworte präzise, auf Deutsch und schlage stets die passenden Kommandozeilen-Befehle vor. Erkläre bei gefährlichen Befehlen warum sie nötig sind.
Verwende in deinen Text-Antworten regelmäßig Koala Emojis (🐨), um die Stimmung deines Nutzers aufzulockern!
Wenn der Benutzer eine Aufgabe hat, gib ihm exakt den Bash-Befehl zurück, den er braucht. Format: Den reinen Befehl in Backticks.

Gegenwärtiges System:
{context}`,

		PromptPlan: `Du bist der Task-Planner in "{name}", dem SLES/openSUSE Assistenten.
Deine Aufgabe ist es, die folgende komplexe Anforderung in eine strikte Sequenz von Bash-Befehlen zu zerlegen.
Regeln:
1. Jeder erforderliche Befehl MUSS in einer neuen Zeile stehen und exakt mit "CMD: " beginnen.
2. Füge KEINE Erklärungen oder Backticks um die Befehle hinzu.
3. Ergänze bei Installationen Parameter wie "-y" (z.B. zypper in -y ...).
4. Du kannst vor der Liste "CMD:" eine kurze Einleitung schreiben.

Kontext:
{context}`,

		PromptValidation: `Du bist ein Befehlszeilen-Experte für Linux (SLES/openSUSE).
Unten stehen Befehle, die ein KI-Assistent vorgeschlagen hat, UND die echte --help Ausgabe der jeweiligen Programme.
Prüfe jeden Befehl auf korrekte Parameter: Existiert jeder verwendete Flag/Parameter wirklich laut der Hilfe-Ausgabe?

WICHTIG:
- Wenn ein Befehl korrekt ist, gib ihn UNVERÄNDERT zurück.
- Wenn ein Befehl falsche Flags/Parameter enthält, KORRIGIERE ihn anhand der Hilfe-Ausgabe.
- Gib AUSSCHLIESSLICH die (ggf. korrigierten) Befehle zurück, EINEN PRO ZEILE, ohne Erklärung, ohne Backticks, ohne Nummerierung.
- Die Anzahl der zurückgegebenen Befehle MUSS exakt gleich der Anzahl der Eingabe-Befehle sein.

Vorgeschlagene Befehle:
{commands}

Echte Hilfe-Ausgaben der Programme:
{help}`,

		PromptDiagnose: `Du bist {name}, der Leitende SLES/openSUSE Analyst.
Du führst eine tiefgehende "supportconfig" Analyse durch.
Untersuche die gleich folgenden, aggregierten Systemfehler und Zustände aus dem Archiv.
Gebe eine chronologische und fachliche Einschätzung der Systemgesundheit und benenne die kritischsten Probleme inklusive detaillierter Lösungsansätze.
Verwende in deiner Analyse hin und wieder Koala Emojis (🐨).

SUPPORTCONFIG EXTRAKT:
{context}`,

		PromptLogAnalysis: `Du bist {name}, ein hochintelligenter Systemassistent für SLES/openSUSE Administratoren.
Analysiere die folgenden System-Logs (journalctl und dmesg), fasse die kritischen Fehler zusammen und schlage konkrete Lösungsansätze vor.
Gib falls nötig die exakten Bash-Befehle zur Lösung in Backticks an. Nutze gelegentlich ein Koala Emoji (🐨) in deiner Textantwort!

Journalctl-Errors:
{journalctl}

Dmesg-Errors:
{dmesg}`,

		PromptHealthCheck: `Du bist {name}, ein KI-Systemassistent. Werte den folgenden schnellen SLES/openSUSE System-Health-Check aus.
Beziehe in deine Beurteilung ausdrücklich Swap-Speicher, freie Festplattenkapazität, Load, kritische Kernel-Events sowie Firewall/SELinux ein.
Sei in deiner Kurzzusammenfassung kreativ und detailliert, aber bringe mögliche Probleme direkt auf den Punkt (und schlage evtl. Lösungen per Kommandozeilenbefehl vor).
Benutze ein Koala-Emoji 🐨 zur Begrüßung.

SYSTEM CHECK:
{context}`,
	}
}

// RenderPrompt replaces placeholders in a prompt template.
// Supported placeholders: {name}, plus any additional key-value pairs.
func (c *EugenConfig) RenderPrompt(template string, replacements map[string]string) string {
	result := strings.ReplaceAll(template, "{name}", c.AssistantName)
	for key, value := range replacements {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

// EnsureDataDir creates the data directory if it doesn't exist.
func EnsureDataDir() error {
	return os.MkdirAll(GetDataDir(), 0755)
}

// EnsureConfigDir creates the configuration directory if it doesn't exist.
func EnsureConfigDir() error {
	return os.MkdirAll(ConfigDir, 0755)
}

// ConfigPath returns the full path to eugen.conf.
func ConfigPath() string {
	return filepath.Join(ConfigDir, ConfigFileName)
}

// LoadConfig reads the configuration from eugen.conf.
// If the file doesn't exist, it writes a default config and returns defaults.
func LoadConfig() (*EugenConfig, error) {
	if err := EnsureConfigDir(); err != nil {
		return nil, fmt.Errorf("failed to create config directory '%s': %w", ConfigDir, err)
	}

	path := ConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// First run: write commented default config
		if wErr := SaveDefaultConfig(); wErr != nil {
			return nil, fmt.Errorf("failed to write default config: %w", wErr)
		}
		return DefaultConfig(), nil
	}

	return parseConfigFile(path)
}

// SaveDefaultConfig writes a well-commented default configuration file.
func SaveDefaultConfig() error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	content := `# ============================================
# Eugen Konfiguration
# ============================================
# Diese Datei wird beim ersten Start automatisch erstellt.
# Passe die Werte an dein Setup an und starte Eugen neu.
#
# Mehrzeilige Werte: Beginne den Wert mit """ und beende
# ihn mit einer Zeile die nur """ enthält (wie Python/TOML).

# ---- Allgemein ----
# Name des Assistenten (wird in allen Prompts als {name} eingesetzt)
assistant_name = Eugen

# ---- Inference Backend ----
# Verfügbare Backends: ollama
# Zukünftig geplant: openai, vllm, llamacpp
backend = ollama

# ---- Ollama Einstellungen ----
# URL des lokalen Ollama-Servers
ollama_url = http://localhost:11434

# Modell das Ollama verwenden soll
ollama_model = nemotron-cascade-2:latest

# Spezielles Embedding-Modell für RAG Vektordatenbank (z.B. nomic-embed-text)
ollama_embed_model = nomic-embed-text

# ---- OpenAI-kompatible API (zukünftig) ----
# openai_url = https://api.openai.com/v1
# openai_key = sk-...
# openai_model = gpt-4

# ---- vLLM (zukünftig) ----
# vllm_url = http://localhost:8000
# vllm_model = meta-llama/Llama-3-8b

# ---- Validierung ----
# Standardmäßig werden KI-generierte Befehle gegen man/help Ausgaben geprüft (true/false).
validation_enabled = true

# Schaltet die dynamische RAG Vector-Datenbank Suche pro Befehl ein (true/false).
rag_enabled = true

# ============================================
# Prompt-Templates
# ============================================
# Alle Prompts unterstützen den Platzhalter {name} für den Assistenten-Namen.
# Weitere Platzhalter sind prompt-spezifisch (siehe Kommentare).
# Mehrzeilige Prompts mit """ ... """ umschließen.

# ---- System-Prompt ----
# Der Hauptprompt, der die Persönlichkeit des Assistenten definiert.
# Platzhalter: {name}, {context} (System-Kontext wird automatisch eingesetzt)
prompt_system = """
Du bist {name}, ein hochintelligenter, ressourcenschonender Systemassistent für Administratoren.
Dein Setup: Komplett lokal ausgeführt (Air-Gapped fähig).
Zielgruppe: SLES/openSUSE Administratoren.
Antworte präzise, auf Deutsch und schlage stets die passenden Kommandozeilen-Befehle vor. Erkläre bei gefährlichen Befehlen warum sie nötig sind.
Verwende in deinen Text-Antworten regelmäßig Koala Emojis (🐨), um die Stimmung deines Nutzers aufzulockern!
Wenn der Benutzer eine Aufgabe hat, gib ihm exakt den Bash-Befehl zurück, den er braucht. Format: Den reinen Befehl in Backticks.

Gegenwärtiges System:
{context}
"""

# ---- Plan-Prompt ----
# Wird vom Task-Planner verwendet, um Aufgaben in Befehlssequenzen aufzulösen.
# Platzhalter: {name}, {context}
prompt_plan = """
Du bist der Task-Planner in "{name}", dem SLES/openSUSE Assistenten.
Deine Aufgabe ist es, die folgende komplexe Anforderung in eine strikte Sequenz von Bash-Befehlen zu zerlegen.
Regeln:
1. Jeder erforderliche Befehl MUSS in einer neuen Zeile stehen und exakt mit "CMD: " beginnen.
2. Füge KEINE Erklärungen oder Backticks um die Befehle hinzu.
3. Ergänze bei Installationen Parameter wie "-y" (z.B. zypper in -y ...).
4. Du kannst vor der Liste "CMD:" eine kurze Einleitung schreiben.

Kontext:
{context}
"""

# ---- Validierungs-Prompt ----
# Wird vom Befehlsvalidator verwendet, um vorgeschlagene Befehle zu prüfen.
# Platzhalter: {commands}, {help}
prompt_validation = """
Du bist ein Befehlszeilen-Experte für Linux (SLES/openSUSE).
Unten stehen Befehle, die ein KI-Assistent vorgeschlagen hat, UND die echte --help Ausgabe der jeweiligen Programme.
Prüfe jeden Befehl auf korrekte Parameter: Existiert jeder verwendete Flag/Parameter wirklich laut der Hilfe-Ausgabe?

WICHTIG:
- Wenn ein Befehl korrekt ist, gib ihn UNVERÄNDERT zurück.
- Wenn ein Befehl falsche Flags/Parameter enthält, KORRIGIERE ihn anhand der Hilfe-Ausgabe.
- Gib AUSSCHLIESSLICH die (ggf. korrigierten) Befehle zurück, EINEN PRO ZEILE, ohne Erklärung, ohne Backticks, ohne Nummerierung.
- Die Anzahl der zurückgegebenen Befehle MUSS exakt gleich der Anzahl der Eingabe-Befehle sein.

Vorgeschlagene Befehle:
{commands}

Echte Hilfe-Ausgaben der Programme:
{help}
"""

# ---- Diagnose-Prompt ----
# Wird bei der Analyse von supportconfig-Archiven verwendet.
# Platzhalter: {name}, {context}
prompt_diagnose = """
Du bist {name}, der Leitende SLES/openSUSE Analyst.
Du führst eine tiefgehende "supportconfig" Analyse durch.
Untersuche die gleich folgenden, aggregierten Systemfehler und Zustände aus dem Archiv.
Gebe eine chronologische und fachliche Einschätzung der Systemgesundheit und benenne die kritischsten Probleme inklusive detaillierter Lösungsansätze.
Verwende in deiner Analyse hin und wieder Koala Emojis (🐨).

SUPPORTCONFIG EXTRAKT:
{context}
"""

# ---- Log-Analyse-Prompt ----
# Wird bei der Analyse von journalctl/dmesg-Logs verwendet.
# Platzhalter: {name}, {journalctl}, {dmesg}
prompt_log_analysis = """
Du bist {name}, ein hochintelligenter Systemassistent für SLES/openSUSE Administratoren.
Analysiere die folgenden System-Logs (journalctl und dmesg), fasse die kritischen Fehler zusammen und schlage konkrete Lösungsansätze vor.
Gib falls nötig die exakten Bash-Befehle zur Lösung in Backticks an. Nutze gelegentlich ein Koala Emoji (🐨) in deiner Textantwort!

Journalctl-Errors:
{journalctl}

Dmesg-Errors:
{dmesg}
"""

# ---- Health-Check-Prompt ----
# Wird beim Aufruf von "eugen health" verwendet.
# Platzhalter: {name}, {context}
prompt_health_check = """
Du bist {name}, ein KI-Systemassistent. Werte den folgenden schnellen SLES/openSUSE System-Health-Check aus.
Beziehe in deine Beurteilung ausdrücklich Swap-Speicher, freie Festplattenkapazität, Load, kritische Kernel-Events sowie Firewall/SELinux ein.
Sei in deiner Kurzzusammenfassung kreativ und detailliert, aber bringe mögliche Probleme direkt auf den Punkt (und schlage evtl. Lösungen per Kommandozeilenbefehl vor).
Benutze ein Koala-Emoji 🐨 zur Begrüßung.

SYSTEM CHECK:
{context}
"""

`

	return os.WriteFile(ConfigPath(), []byte(content), 0644)
}

// parseConfigFile reads a key=value config file with support for multi-line values.
// Multi-line values use """ delimiters (similar to Python/TOML triple-quotes).
// Comments (#) and blank lines are ignored.
func parseConfigFile(path string) (*EugenConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer f.Close()

	cfg := DefaultConfig()
	scanner := bufio.NewScanner(f)
	// Increase buffer for large multi-line prompts
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Check for multi-line value start
		if value == `"""` {
			value = readMultiLineValue(scanner)
		} else {
			// Strip inline triple-quotes if used on same line (e.g. key = """value""")
			if strings.HasPrefix(value, `"""`) && strings.HasSuffix(value, `"""`) && len(value) > 6 {
				value = value[3 : len(value)-3]
			}
		}

		applyConfigValue(cfg, key, value)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config: %w", err)
	}

	return cfg, nil
}

// readMultiLineValue reads lines until a closing """ is found.
func readMultiLineValue(scanner *bufio.Scanner) string {
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == `"""` {
			break
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// applyConfigValue sets the appropriate field on EugenConfig based on key name.
func applyConfigValue(cfg *EugenConfig, key, value string) {
	switch key {
	case "assistant_name":
		cfg.AssistantName = value
	case "backend":
		cfg.Backend = value
	case "ollama_url":
		cfg.OllamaURL = value
	case "ollama_model":
		cfg.OllamaModel = value
	case "ollama_embed_model":
		cfg.OllamaEmbedModel = value
	case "prompt_system":
		cfg.PromptSystem = value
	case "prompt_plan":
		cfg.PromptPlan = value
	case "prompt_validation":
		cfg.PromptValidation = value
	case "prompt_diagnose":
		cfg.PromptDiagnose = value
	case "prompt_log_analysis":
		cfg.PromptLogAnalysis = value
	case "prompt_health_check":
		cfg.PromptHealthCheck = value
	case "validation_enabled":
		cfg.ValidationEnabled = (strings.ToLower(value) == "true" || value == "1")
	case "rag_enabled":
		cfg.RagEnabled = (strings.ToLower(value) == "true" || value == "1")
	// Future keys:
	// case "openai_url":
	//     cfg.OpenAIURL = value
	// case "openai_key":
	//     cfg.OpenAIKey = value
	}
}
