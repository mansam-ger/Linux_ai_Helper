package diagnostic

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnsureSupportutils checks if supportconfig is installed, and if not installs via zypper
func EnsureSupportutils() error {
	_, err := exec.LookPath("supportconfig")
	if err == nil {
		return nil
	}
	
	if _, zErr := exec.LookPath("zypper"); zErr != nil {
		return fmt.Errorf("Diagnosefunktion 'supportconfig' wird nativ nur unter SLES/openSUSE unterstützt. Das Tool konnte auf diesem System nicht gefunden werden")
	}
	
	fmt.Println("[\u2139] 'supportconfig' nicht gefunden. Installiere Paket 'supportutils' via zypper (kann dauern)...")
	cmd := exec.Command("sh", "-c", "zypper in -y supportutils")
	out, inErr := cmd.CombinedOutput()
	if inErr != nil {
		return fmt.Errorf("fehler bei der Installation von supportutils: %v\nOutput: %s", inErr, string(out))
	}
	return nil
}

// RunSupportconfig runs the SLES diagnostic tool.
// Returns the absolute path to the generated tarball.
func RunSupportconfig(minimal bool) (string, error) {
	fmt.Println("[\u23F3] Generiere SLES System-Dump... (Das kann einige Minuten in Anspruch nehmen!)")

	var cmd *exec.Cmd
	if minimal {
		cmd = exec.Command("supportconfig", "-m")
	} else {
		// Just a basic supportconfig run
		cmd = exec.Command("supportconfig")
	}

	out, err := cmd.CombinedOutput()

	// Anstatt den Stdout-Text angreifbar zu parsen, scannen wir simpel /var/log/
	// nach der exakten, neuesten *.tbz Datei, die "scc_" oder "nts_" im Namen trägt.
	var newest string
	var newestTime int64
	
	files, _ := os.ReadDir("/var/log")
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".tbz") && (strings.HasPrefix(f.Name(), "scc_") || strings.HasPrefix(f.Name(), "nts_")) {
			info, iErr := f.Info()
			if iErr == nil {
				if info.ModTime().Unix() > newestTime {
					newestTime = info.ModTime().Unix()
					newest = filepath.Join("/var/log", f.Name())
				}
			}
		}
	}

	if newest != "" {
		return newest, nil
	}

	if err != nil {
		return "", fmt.Errorf("Fehler bei der Ausführung (bist du root/sudo?): %v\nOutput: %s", err, string(out)[:min(500, len(out))])
	}

	return "", fmt.Errorf("konnte den Tarball nicht in /var/log/ finden. Cmd-Output:\n%s", string(out)[:min(500, len(out))])
}

// Helper min function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ExtractAndFilter untars the archive and picks only critical logs for Ollama Context.
func ExtractAndFilter(tarballPath string) (string, error) {
	fmt.Printf("[\u23F3] Entpacke und filtere %s in /tmp...\n", filepath.Base(tarballPath))
	
	tmpDir := "/tmp/eugen_diag"
	os.RemoveAll(tmpDir)
	_ = os.MkdirAll(tmpDir, 0755)
	
	// Entpacken (bzip2)
	cmd := exec.Command("tar", "-xjf", tarballPath, "-C", tmpDir)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("fehler beim untar: %v", err)
	}

	// supportconfig extracts to a folder inside tmpDir, typically named like the tarball minus .tbz
	entries, _ := os.ReadDir(tmpDir)
	var extractDir string
	for _, e := range entries {
		if e.IsDir() {
			extractDir = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if extractDir == "" {
		extractDir = tmpDir // fallback if it extracted flat
	}

	var sb strings.Builder

	// Gekürzte Files
	appendFileHead(&sb, filepath.Join(extractDir, "basic-environment.txt"), 1500)
	
	// Error-gefilterte Files
	filterKeywords := []string{"error", "fail", "warn", "panic", "critical"}
	appendFileFiltered(&sb, filepath.Join(extractDir, "hardware.txt"), filterKeywords, 40)
	appendFileFiltered(&sb, filepath.Join(extractDir, "messages.txt"), filterKeywords, 50)
	appendFileFiltered(&sb, filepath.Join(extractDir, "journal.txt"), filterKeywords, 50)
	
	// Clean up /tmp
	os.RemoveAll(tmpDir)

	res := sb.String()
	// UTF-8 Validation
	res = strings.ToValidUTF8(res, "")
	
	// Abschließender Guard-Cut
	if len(res) > 35000 {
		res = res[len(res)-35000:]
	}

	return res, nil
}

func appendFileHead(sb *strings.Builder, path string, maxChars int) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	s := string(b)
	if strings.TrimSpace(s) == "" {
		return
	}

	sb.WriteString(fmt.Sprintf("\n--- %s ---\n", filepath.Base(path)))
	if len(s) > maxChars {
		sb.WriteString(s[:maxChars] + "\n...[TRUNCATED]\n")
	} else {
		sb.WriteString(s + "\n")
	}
}

func appendFileFiltered(sb *strings.Builder, path string, keywords []string, maxLines int) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	
	lines := strings.Split(string(b), "\n")
	var matched []string
	
	for _, l := range lines {
		lowerL := strings.ToLower(l)
		for _, k := range keywords {
			if strings.Contains(lowerL, k) {
				matched = append(matched, strings.TrimSpace(l))
				break
			}
		}
	}
	
	if len(matched) == 0 {
		return
	}
	
	if len(matched) > maxLines {
		matched = matched[len(matched)-maxLines:]
	}
	
	sb.WriteString(fmt.Sprintf("\n--- %s (Gefiltert auf Errors/Warnings) ---\n", filepath.Base(path)))
	for _, m := range matched {
		sb.WriteString(m + "\n")
	}
}
