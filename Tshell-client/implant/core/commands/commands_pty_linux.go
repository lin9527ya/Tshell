//go:build linux

package commands

// interactiveShellPath 交互式终端使用的 shell。Linux 上 bash 普遍存在且行为稳定。
func interactiveShellPath() string { return "/bin/bash" }
