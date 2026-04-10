package diagnostic

import (
	"fmt"
	"os/exec"
	"strings"
)

// QuickHealthCheck runs lightweight commands to gather system metrics instantly.
// It acts as a fast alternative to the heavy supportconfig diagnosis.
func QuickHealthCheck() string {
	commands := []struct {
		Label string
		Cmd   string
	}{
		{"Uptime & Load Average", "uptime"},
		{"Memory & Swap", "free -h"},
		{"Disk Space (Excerpt)", "df -h -x tmpfs -x devtmpfs -x overlay -x squashfs"},
		{"Top Processes by CPU", "ps -eo pid,comm,%mem,%cpu --sort=-%cpu | head -n 7"},
		{"Failed Systemd Services", "systemctl --failed --no-pager"},
		{"Btrfs Root Usage", "btrfs fi usage / | head -n 12 || echo 'Not a Btrfs root filesystem'"},
		{"Firewall State", "firewall-cmd --state 2>/dev/null || echo 'Nicht aktiv / Fehler'"},
		{"SELinux State", "sestatus 2>/dev/null || getenforce 2>/dev/null || echo 'Nicht installiert / Inaktiv'"},
		{"Critical Kernel Events (dmesg)", "dmesg -T --level=err,crit,alert,emerg | tail -n 8 || echo 'No critical kernel events'"},
	}

	var sb strings.Builder
	for _, c := range commands {
		sb.WriteString(fmt.Sprintf("\n--- %s ---\n", c.Label))
		out, err := exec.Command("bash", "-c", c.Cmd).CombinedOutput()
		strOut := strings.TrimSpace(string(out))
		
		if err != nil && strOut == "" {
			strOut = fmt.Sprintf("Error executing check: %v", err)
		} else if strOut == "" {
			strOut = "Keine passenden Einträge / Keine Fehler"
		}
		
		sb.WriteString(strOut)
		sb.WriteString("\n")
	}

	return strings.ToValidUTF8(sb.String(), "")
}
