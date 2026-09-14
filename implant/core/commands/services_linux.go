//go:build linux

package commands

import "os/exec"

// ListServices 列出 Linux systemd 服务。
func ListServices() ([]byte, error) {
	return exec.Command("systemctl", "list-units", "--type=service", "--no-pager").Output()
}

// StartService 启动 systemd 服务。
func StartService(name string) ([]byte, error) {
	return exec.Command("systemctl", "start", name).Output()
}

// StopService 停止 systemd 服务。
func StopService(name string) ([]byte, error) {
	return exec.Command("systemctl", "stop", name).Output()
}
