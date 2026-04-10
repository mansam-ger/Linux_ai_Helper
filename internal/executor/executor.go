package executor

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// RiskScore represents the risk level of executing a command directly.
type RiskScore int

const (
	RiskLow RiskScore = iota
	RiskMedium
	RiskHigh
)

// Executor manages validation and execution of system commands.
type Executor struct {
	// Add risk thresholds or configuration here if needed
}

// NewExecutor creates a new system execution checker.
func NewExecutor() *Executor {
	return &Executor{}
}

// AnalyzeCommand runs simple heuristics on the proposed command to estimate danger.
func (e *Executor) AnalyzeCommand(command string) RiskScore {
	cmdLower := strings.ToLower(strings.TrimSpace(command))

	// Catch most destructive things natively
	highRiskPatterns := []*regexp.Regexp{
		regexp.MustCompile(`rm\s+-r?[f]`),
		regexp.MustCompile(`\bdd\b.*if=`),
		regexp.MustCompile(`\bmkfs\b`),
		regexp.MustCompile(`\bfdisk\b`),
		regexp.MustCompile(`\bparted\b`),
		regexp.MustCompile(`>\s*/etc/`),
		regexp.MustCompile(`>>\s*/etc/`),
	}

	for _, pattern := range highRiskPatterns {
		if pattern.MatchString(cmdLower) {
			return RiskHigh
		}
	}

	// Medium risk
	mediumRiskPatterns := []*regexp.Regexp{
		regexp.MustCompile(`\bchmod\b`),
		regexp.MustCompile(`\bchown\b`),
		regexp.MustCompile(`\bsystemctl\b\s+(stop|restart|disable)`),
		regexp.MustCompile(`\bzypper\b\s+(install|in|remove|rm)`),
	}

	for _, pattern := range mediumRiskPatterns {
		if pattern.MatchString(cmdLower) {
			return RiskMedium
		}
	}

	return RiskLow
}

// Execute runs the command directly attached to standard streams for interactive capability.
func (e *Executor) Execute(command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("command execution failed: %w", err)
	}
	return "", nil
}
