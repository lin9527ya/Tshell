//go:build darwin

package commands

import "os/exec"

// ListServices 列出 macOS launchctl 服务。
func ListServices() ([]byte, error) {
	return exec.Command("launchctl", "list").Output()
}

// StartService 启动 launchctl 服务。
func StartService(name string) ([]byte, error) {
	return exec.Command("launchctl", "start", name).Output()
}

// StopService 停止 launchctl 服务。
func StopService(name string) ([]byte, error) {
	return exec.Command("launchctl", "stop", name).Output()
}
