//go:build darwin

package commands

import (
	"os/exec"
)

// Wifi 列出 macOS 保存的 WiFi 网络（密码需 keychain 权限，此处仅列出）。
func Wifi() ([]byte, error) {
	return exec.Command("networksetup", "-listpreferredwirelessnetworks", "en0").Output()
}
