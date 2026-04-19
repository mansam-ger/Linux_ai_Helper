package plugin

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Plugin represents a local admin script/tool
type Plugin struct {
	Name        string
	Path        string
	Description string
}

// LoadPlugins scans the given directory for executable files and extracts their description.
func LoadPlugins(pluginDir string) ([]Plugin, error) {
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, fmt.Errorf("could not create plugin directory: %w", err)
	}

	var plugins []Plugin

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("could not read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fullPath := filepath.Join(pluginDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Only consider executable files or .sh files
		isExecutable := info.Mode()&0111 != 0
		if !isExecutable && !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}

		desc := extractDescription(fullPath)
		if desc == "" {
			desc = "Keine Beschreibung vorhanden."
		}

		plugins = append(plugins, Plugin{
			Name:        entry.Name(),
			Path:        fullPath,
			Description: desc,
		})
	}

	return plugins, nil
}

// extractDescription reads the first few lines of a file and looks for '# Description: ...'
func extractDescription(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for i := 0; i < 20 && scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "# description:") || strings.HasPrefix(lowerLine, "# beschreibung:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// FormatPluginContext returns a string suitable for injection into the LLM system prompt.
func FormatPluginContext(plugins []Plugin) string {
	if len(plugins) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\nZusätzliche lokale Admin-Werkzeuge (Plugins):\n")
	sb.WriteString("Du kannst diese Skripte vorschlagen oder aufrufen, wenn sie zur Problemlösung passen. Nutze den vollen Pfad in Backticks.\n")
	
	for _, p := range plugins {
		sb.WriteString(fmt.Sprintf("- %s (%s): %s\n", p.Name, p.Path, p.Description))
	}
	
	return sb.String()
}
