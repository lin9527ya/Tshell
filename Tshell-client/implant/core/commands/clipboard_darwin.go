//go:build darwin

package commands

import "os/exec"

// Clipboard 读取 macOS 剪贴板（pbpaste）。
func Clipboard() ([]byte, error) {
	return exec.Command("pbpaste").Output()
}
