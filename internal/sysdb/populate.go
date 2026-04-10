package sysdb

import (
	"fmt"
	"os/exec"
	"strings"
)

// GatherSystemPopulate runs multiple system utilities to build a snapshot.
func GatherSystemPopulate() (*SystemData, error) {
	fmt.Println("[\u23F3] Sammle Hardware-Infos (lscpu)...")
	hw, _ := runSysCmd("lscpu | grep -E 'Model name|^CPU\\(s\\):|Thread|Core'")

	fmt.Println("[\u23F3] Sammle Netzwerk-Infos (ip a)...")
	net, _ := runSysCmd("ip -br a")

	fmt.Println("[\u23F3] Sammle aktive Dienste (systemctl)...")
	srv, _ := runSysCmd("systemctl list-units --type=service --state=running --no-pager | grep -v 'systemd-' | head -n 30")

	return &SystemData{
		HardwareInfo: hw,
		NetworkInfo:  net,
		Services:     srv,
	}, nil
}

func runSysCmd(cmdStr string) (string, error) {
	out, err := exec.Command("sh", "-c", cmdStr).CombinedOutput()
	if err != nil {
		return "", err
	}
	res := strings.TrimSpace(string(out))
	// make sure output is valid UTF-8 before sending it eventually to JSON
	res = strings.ToValidUTF8(res, "")
	return res, nil
}
