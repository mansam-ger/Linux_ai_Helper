package cmdvalidator

import (
	"fmt"
	"os/exec"
	"strings"

	"eugen/internal/config"
	"eugen/internal/inference"
)

// Validator verifies extracted commands against their real --help / man output.
type Validator struct {
	backend inference.Backend
	cfg     *config.EugenConfig
}

// NewValidator creates a new command validator.
func NewValidator(backend inference.Backend, cfg *config.EugenConfig) *Validator {
	return &Validator{backend: backend, cfg: cfg}
}

// ValidateCommands takes a list of commands extracted from an LLM response,
// fetches the help text for the base binary, and asks the backend to correct
// any wrong flags or parameters.
// Returns the corrected list of commands.
func (v *Validator) ValidateCommands(cmds []string, verbose bool) []string {
	if len(cmds) == 0 {
		return cmds
	}

	// Collect help texts for every unique base command
	helpTexts := make(map[string]string)
	for _, cmd := range cmds {
		base := extractBaseCommand(cmd)
		if base == "" {
			continue
		}
		if _, seen := helpTexts[base]; seen {
			continue
		}
		helpTexts[base] = fetchHelpText(base)
	}

	// If we couldn't get help for anything, skip validation
	hasHelp := false
	for _, h := range helpTexts {
		if h != "" {
			hasHelp = true
			break
		}
	}
	if !hasHelp {
		return cmds
	}

	// Build a help context block
	var helpBlock strings.Builder
	for bin, help := range helpTexts {
		if help == "" {
			continue
		}
		helpBlock.WriteString(fmt.Sprintf("\n--- Hilfe für '%s' ---\n%s\n", bin, help))
	}

	// Build the validation prompt from config template
	cmdList := strings.Join(cmds, "\n")
	validationPrompt := v.cfg.RenderPrompt(v.cfg.PromptValidation, map[string]string{
		"commands": cmdList,
		"help":     helpBlock.String(),
	})

	if verbose {
		fmt.Printf("\033[33m[VERBOSE] Validierungs-Prompt:\033[0m\n%s\n\n", validationPrompt)
	}

	fmt.Printf("\033[36m🐨 %s validiert die Befehle gegen die echte --help Ausgabe...\033[0m\n", v.cfg.AssistantName)

	resp, err := v.backend.Generate(validationPrompt, cmdList, nil)
	if err != nil {
		// On error, just return originals
		return cmds
	}

	if verbose {
		fmt.Printf("\n\033[33m[VERBOSE] KI-Rohantwort der Validierung:\033[0m\n%s\n\n", resp)
	}

	// Parse validated commands from response
	validated := parseValidatedResponse(resp, len(cmds))

	// Deduplicate consecutive identical commands (thinking models often repeat the answer)
	if len(validated) > len(cmds) {
		deduped := []string{validated[0]}
		for i := 1; i < len(validated); i++ {
			if validated[i] != validated[i-1] {
				deduped = append(deduped, validated[i])
			}
		}
		validated = deduped
	}

	if verbose {
		fmt.Printf("\n\033[33m[VERBOSE] Validierungs-Rohantwort (%d Zeilen parsed, %d erwartet):\033[0m\n", len(validated), len(cmds))
		for i, v := range validated {
			fmt.Printf("  [%d] %s\n", i+1, v)
		}
		fmt.Println()
	}

	if len(validated) == len(cmds) {
		if verbose {
			fmt.Printf("\033[33m[VERBOSE] Validierungs-Ergebnis:\033[0m\n")
			for i := range cmds {
				if cmds[i] != validated[i] {
					fmt.Printf(" - Korrigiert: %s \033[31m->\033[0m \033[32m%s\033[0m\n", cmds[i], validated[i])
				} else {
					fmt.Printf(" - Unverändert (OK): %s\n", cmds[i])
				}
			}
			fmt.Println()
		}
		return validated
	}

	// Best-effort: if we got more commands than expected, take first N
	if len(validated) > len(cmds) {
		validated = validated[:len(cmds)]
		if verbose {
			fmt.Printf("\033[33m[VERBOSE] Validierung: Mehr Befehle als erwartet, nutze die ersten %d.\033[0m\n", len(cmds))
		}
		return validated
	}

	// If we got fewer, return originals for the missing ones
	if len(validated) > 0 && len(validated) < len(cmds) {
		if verbose {
			fmt.Printf("\033[33m[VERBOSE] Validierung: Weniger Befehle als erwartet (%d vs %d), ergänze fehlende aus Original.\033[0m\n", len(validated), len(cmds))
		}
		for i := len(validated); i < len(cmds); i++ {
			validated = append(validated, cmds[i])
		}
		return validated
	}

	// Fallback: nothing usable parsed
	if verbose {
		fmt.Printf("\n\033[31m[VERBOSE] Validierung fehlgeschlagen (keine Befehle aus Antwort extrahiert).\033[0m\n")
	}
	return cmds
}

// extractBaseCommand extracts the binary name from a full command string.
// e.g. "sudo zypper in -y nginx" -> "zypper"
// e.g. "systemctl restart sshd"  -> "systemctl"
func extractBaseCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}

	// Skip common prefixes
	i := 0
	for i < len(parts) {
		p := parts[i]
		if p == "sudo" || p == "doas" || p == "env" || strings.Contains(p, "=") {
			i++
			continue
		}
		break
	}

	if i >= len(parts) {
		return ""
	}

	return parts[i]
}

// fetchHelpText tries --help first, then man page (col stripped).
func fetchHelpText(binary string) string {
	// Try --help first (most programs support it)
	out, err := exec.Command("sh", "-c", binary+" --help 2>&1 | head -60").CombinedOutput()
	if err == nil {
		s := strings.TrimSpace(string(out))
		if len(s) > 30 {
			return truncate(s, 3000)
		}
	}

	// Fallback: man page
	out, err = exec.Command("sh", "-c", "man "+binary+" 2>/dev/null | col -b | head -80").CombinedOutput()
	if err == nil {
		s := strings.TrimSpace(string(out))
		if len(s) > 30 {
			return truncate(s, 3000)
		}
	}

	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n...[TRUNCATED]"
}

// isLikelyCommand checks if a line looks like a shell command (starts with a known binary or path).
func isLikelyCommand(line string) bool {
	if line == "" {
		return false
	}
	// Lines starting with common command prefixes
	prefixes := []string{"sudo ", "doas ", "/", "zypper ", "rpm ", "systemctl ", "journalctl ",
		"cat ", "echo ", "grep ", "find ", "ls ", "cp ", "mv ", "rm ", "mkdir ", "chmod ",
		"chown ", "sed ", "awk ", "curl ", "wget ", "tar ", "mount ", "umount ", "ip ",
		"nmcli ", "firewall-cmd ", "ss ", "df ", "du ", "free ", "top ", "ps ",
		"yast ", "SUSEConnect ", "transactional-update ", "snapper ", "btrfs ",
		"docker ", "podman ", "kubectl ", "helm ", "ansible ", "salt ",
		"vim ", "nano ", "less ", "head ", "tail ", "wc ", "sort ", "uniq ",
		"useradd ", "usermod ", "userdel ", "groupadd ", "passwd ",
		"modprobe ", "lsmod ", "dmesg ", "lsblk ", "fdisk ", "mkfs.",
		"service ", "chkconfig ", "update-alternatives ",
		"sysctl ", "tuned-adm ", "timedatectl ", "hostnamectl ", "localectl "}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	// Also allow if it looks like a path to a binary
	if strings.HasPrefix(line, "./") || strings.HasPrefix(line, "/usr/") || strings.HasPrefix(line, "/bin/") || strings.HasPrefix(line, "/sbin/") {
		return true
	}
	return false
}

// stripThinkBlocks removes chain-of-thought <think>...</think> blocks from LLM responses.
// Handles both complete blocks and unclosed opening tags (where </think> appears later).
func stripThinkBlocks(s string) string {
	for {
		start := strings.Index(s, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(s, "</think>")
		if end == -1 {
			// Unclosed think block - strip everything from <think> onwards
			s = s[:start]
			break
		}
		// Remove the entire <think>...</think> block
		s = s[:start] + s[end+len("</think>"):]
	}
	// Also handle bare </think> tags without opening tag
	s = strings.ReplaceAll(s, "</think>", "")
	return s
}

func parseValidatedResponse(resp string, expectedCount int) []string {
	// Strip chain-of-thought reasoning blocks first
	resp = stripThinkBlocks(resp)

	lines := strings.Split(strings.TrimSpace(resp), "\n")
	var result []string
	for _, l := range lines {
		l = strings.TrimSpace(l)

		// Skip empty lines
		if l == "" {
			continue
		}

		// Skip markdown code fences
		if strings.HasPrefix(l, "```") {
			continue
		}
		// Skip markdown language identifiers that may appear after code fences
		if l == "bash" || l == "sh" || l == "shell" || l == "zsh" {
			continue
		}

		// Skip comment/explanation lines (start with #, *, -, >)
		if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "*") || strings.HasPrefix(l, ">") {
			continue
		}
		// Skip lines that start with "- " (markdown list items that are explanations)
		if strings.HasPrefix(l, "- ") && !isLikelyCommand(strings.TrimPrefix(l, "- ")) {
			continue
		}

		// Skip lines that look like explanatory prose (contain typical German/English explanation markers)
		lLower := strings.ToLower(l)
		if strings.HasPrefix(lLower, "hinweis") || strings.HasPrefix(lLower, "anmerkung") ||
			strings.HasPrefix(lLower, "note:") || strings.HasPrefix(lLower, "erklärung") ||
			strings.HasPrefix(lLower, "der ") || strings.HasPrefix(lLower, "die ") ||
			strings.HasPrefix(lLower, "das ") || strings.HasPrefix(lLower, "hier ") ||
			strings.HasPrefix(lLower, "dieser ") || strings.HasPrefix(lLower, "diese ") ||
			strings.HasPrefix(lLower, "alle ") || strings.HasPrefix(lLower, "befehl") ||
			strings.HasPrefix(lLower, "the ") || strings.HasPrefix(lLower, "this ") ||
			strings.HasPrefix(lLower, "i ") || strings.HasPrefix(lLower, "both ") ||
			strings.HasPrefix(lLower, "command") || strings.HasPrefix(lLower, "korrigiert") ||
			strings.HasPrefix(lLower, "unverändert") || strings.HasPrefix(lLower, "unchanged") {
			continue
		}

		// Remove leading numbering like "1. " or "1) " or "1: "
		if len(l) > 2 && l[0] >= '0' && l[0] <= '9' {
			if idx := strings.IndexAny(l, ".):-"); idx > 0 && idx < 4 {
				candidate := strings.TrimSpace(l[idx+1:])
				if candidate != "" {
					l = candidate
				}
			}
		}

		// Remove surrounding backticks
		l = strings.Trim(l, "`")
		l = strings.TrimSpace(l)

		if l != "" && isLikelyCommand(l) {
			result = append(result, l)
		}
	}
	return result
}
