//go:build linux

package commands

import (
	"fmt"
	"os/exec"
)

// Clipboard 读取 Linux 剪贴板（需 xclip 或 xsel）。
func Clipboard() ([]byte, error) {
	if out, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output(); err == nil {
		return out, nil
	}
	if out, err := exec.Command("xsel", "--clipboard", "--output").Output(); err == nil {
		return out, nil
	}
	return nil, fmt.Errorf("剪贴板读取失败（需安装 xclip 或 xsel）")
}
