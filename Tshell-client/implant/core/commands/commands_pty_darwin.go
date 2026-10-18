//go:build darwin

package commands

import "os"

// interactiveShellPath 交互式终端使用的 shell。
// macOS 默认 shell 是 zsh（/bin/zsh），bash 仍是 3.2 老版本；优先 zsh，
// 缺失时回退 bash（极少见，但避免因路径不存在直接启动失败）。
func interactiveShellPath() string {
	if _, err := os.Stat("/bin/zsh"); err == nil {
		return "/bin/zsh"
	}
	return "/bin/bash"
}
