//go:build linux

package commands

import (
	"os/exec"
)

// Wifi 提取 Linux 保存的 WiFi 配置（NetworkManager）。
func Wifi() ([]byte, error) {
	return exec.Command("bash", "-c",
		"cat /etc/NetworkManager/system-connections/* 2>/dev/null || echo '无 NetworkManager 配置'").Output()
}
