package context

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"eugen/internal/config"
)

// PackageManager represents the detected package manager backend
type PackageManager struct {
	Name      string
	SearchCmd string
	CheckCmd  string
}

func DetectPackageManager() PackageManager {
	if _, err := exec.LookPath("zypper"); err == nil {
		return PackageManager{
			Name: "zypper",
			SearchCmd: "zypper search -t package --match-substrings %s | head -n 15",
			CheckCmd:  "rpm -qa | grep -i %s | head -n 5",
		}
	}
	if _, err := exec.LookPath("pacman"); err == nil {
		return PackageManager{
			Name: "pacman",
			SearchCmd: "pacman -Ss %s | head -n 15",
			CheckCmd:  "pacman -Qs %s | head -n 5",
		}
	}
	if _, err := exec.LookPath("apt-get"); err == nil {
		return PackageManager{
			Name: "apt",
			SearchCmd: "apt-cache search %s | head -n 15",
			CheckCmd:  "dpkg -l | grep -i %s | head -n 5",
		}
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return PackageManager{
			Name: "dnf",
			SearchCmd: "dnf search %s | head -n 15",
			CheckCmd:  "rpm -qa | grep -i %s | head -n 5",
		}
	}
	return PackageManager{}
}

// GatherSystemInfo fetches baseline details about the environment.
// For example, what OS version is running to provide contextual hints to the LLM.
func GatherSystemInfo() string {
	var info strings.Builder
	
	osName := config.GetOSName()
	if osName != "Linux" {
		info.WriteString(fmt.Sprintf("OS: %s\n", osName))
	} else {
		info.WriteString("OS Information: Unavailable\n")
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
	pm := DetectPackageManager()
	if pm.SearchCmd == "" {
		return ""
	}

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
		return ""
	}

	if len(keyword) < 3 || keyword == "ein" || keyword == "den" {
		return ""
	}

	cmdStr := fmt.Sprintf(pm.SearchCmd, keyword)
	out, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
	if err != nil {
		return ""
	}

	res := strings.TrimSpace(string(out))
	if res == "" || strings.Contains(res, "No matching items found") {
		return ""
	}

	return fmt.Sprintf("\nLokale Paket-Treffer in %s für '%s':\n%s\nBitte verwende zum Installieren die exakten Namen aus dieser Tabelle.\n", pm.Name, keyword, res)
}

// CheckInstalledPackages checks if the user questions whether a package is installed
// and dynamically runs rpm -q to feed context to the LLM.
func CheckInstalledPackages(input string) string {
	pm := DetectPackageManager()
	if pm.CheckCmd == "" {
		return ""
	}

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

	cmdStr := fmt.Sprintf(pm.CheckCmd, keyword)
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
