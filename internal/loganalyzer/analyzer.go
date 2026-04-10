package loganalyzer

import (
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// Analyzer provides methods to fetch system logs.
type Analyzer struct{}

// NewAnalyzer creates a new Analyzer instance.
func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

// GetRecentErrors fetches the most recent critical/error log entries from journalctl.
func (a *Analyzer) GetRecentErrors(limit int) (string, error) {
	cmdStr := fmt.Sprintf("journalctl -p 3 -n %d --no-pager", limit)
	return a.runCommand(cmdStr)
}

// GetDmesgErrors fetches the most recent kernel errors.
func (a *Analyzer) GetDmesgErrors(limit int) (string, error) {
	// dmesg -l err,crit,alert,emerg limits output to error or worse. We pipe to tail.
	cmdStr := fmt.Sprintf("dmesg -l err,crit,alert,emerg | tail -n %d", limit)
	return a.runCommand(cmdStr)
}

func (a *Analyzer) runCommand(cmdStr string) (string, error) {
	cmd := exec.Command("sh", "-c", cmdStr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %s: %w", cmdStr, err)
	}

	result := string(out)
	if !utf8.ValidString(result) {
		// Clean non-UTF8 characters so JSON encoding to Ollama doesn't break
		result = strings.ToValidUTF8(result, "")
	}

	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return "Keine kritischen Fehler im betreffenden Log gefunden.", nil
	}

	return trimmed, nil
}
