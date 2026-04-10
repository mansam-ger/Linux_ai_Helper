package context

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// GatherSystemInfo fetches baseline details about the environment.
// For example, what OS version is running to provide contextual hints to the LLM.
func GatherSystemInfo() string {
	var info strings.Builder
	
	// Try reading /etc/os-release safely
	content, err := os.ReadFile("/etc/os-release")
	if err == nil {
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				info.WriteString("OS: ")
				info.WriteString(strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\""))
				info.WriteString("\n")
			}
		}
	} else {
		info.WriteString("OS Information: Unavailable or not running SUSE/openSUSE\n")
	}

	if hostname, err := os.Hostname(); err == nil {
		info.WriteString(fmt.Sprintf("Hostname: %s\n", hostname))
	}

	if out, err := exec.Command("uname", "-r").Output(); err == nil {
		info.WriteString(fmt.Sprintf("Kernel: %s\n", strings.TrimSpace(string(out))))
	}
	
	info.WriteString(fmt.Sprintf("Architecture: %s\n", runtime.GOARCH))

	if out, err := exec.Command("uptime", "-p").Output(); err == nil {
		info.WriteString(fmt.Sprintf("Uptime: %s\n", strings.TrimSpace(string(out))))
	}

	return info.String()
}

// SearchPackageKeywords checks if the user wants to install something
// and runs a background zypper search to feed context to the LLM.
func SearchPackageKeywords(input string) string {
	lowerInput := strings.ToLower(input)
	
	// Quick heuristic check
	if !strings.Contains(lowerInput, "install") && 
	   !strings.Contains(lowerInput, "paket") &&
	   !strings.Contains(lowerInput, "aufsetzen") &&
	   !strings.Contains(lowerInput, "einrichten") {
		return ""
	}

	// Try to find the actual software name
	re := regexp.MustCompile(`(?:installiere|richte|suche|brauche)\s+(?:ein\s+|einen\s+|das\s+)?([a-zA-Z0-9_\-]+)`)
	matches := re.FindStringSubmatch(lowerInput)
	
	keyword := ""
	if len(matches) > 1 {
		keyword = matches[1]
	} else {
		// Fallback: If "nginx" is in the text, keyword could just be that, but we can't guess easily without NLP.
		// For simplicity, skip if no clear verb-noun structure is found.
		return ""
	}

	if len(keyword) < 3 || keyword == "ein" || keyword == "den" {
		return ""
	}

	cmdStr := fmt.Sprintf("zypper search -t package --match-substrings %s | head -n 15", keyword)
	out, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
	if err != nil {
		return ""
	}

	res := strings.TrimSpace(string(out))
	if res == "" || strings.Contains(res, "No matching items found") {
		return ""
	}

	return fmt.Sprintf("\nLokale Paket-Treffer in zypper für '%s':\n%s\nBitte verwende zum Installieren die exakten Namen aus dieser Tabelle.\n", keyword, res)
}

// CheckInstalledPackages checks if the user questions whether a package is installed
// and dynamically runs rpm -q to feed context to the LLM.
func CheckInstalledPackages(input string) string {
	lowerInput := strings.ToLower(input)
	
	// Quick heuristic check for intent
	if !strings.Contains(lowerInput, "installiert") && 
	   !strings.Contains(lowerInput, "haben wir") &&
	   !strings.Contains(lowerInput, "gibt es") &&
	   !strings.Contains(lowerInput, "läuft") {
		return ""
	}

	// Try to find the actual software name
	re := regexp.MustCompile(`(?:ist|haben\swir|gibt\ses)(?:\s+ein|\s+den|\s+das)?\s+([a-zA-Z0-9_\-]+)(?:\s+installiert|\s+hier|\s+darauf)?`)
	matches := re.FindStringSubmatch(lowerInput)
	
	keyword := ""
	if len(matches) > 1 {
		keyword = matches[1]
	} else {
		// Fallback for simple "ist X installiert"
		re2 := regexp.MustCompile(`([a-zA-Z0-9_\-]+)\s+installiert`)
		matches2 := re2.FindStringSubmatch(lowerInput)
		if len(matches2) > 1 {
			keyword = matches2[1]
		} else {
			return ""
		}
	}

	if len(keyword) < 2 || keyword == "es" || keyword == "was" {
		return ""
	}

	// We use rpm -qa and grep to find partial matches
	cmdStr := fmt.Sprintf("rpm -qa | grep -i %s | head -n 5", keyword)
	out, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
	if err != nil {
		return ""
	}

	res := strings.TrimSpace(string(out))
	if res == "" {
		return fmt.Sprintf("\nPaket-Status für '%s': Das Paket scheint nicht installiert zu sein.\n", keyword)
	}

	return fmt.Sprintf("\nPaket-Status für '%s': Folgende zugehörige Pakete sind aktuell installiert:\n%s\n", keyword, res)
}
