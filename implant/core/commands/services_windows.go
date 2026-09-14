//go:build windows

package commands

import (
	"context"
	"os/exec"
	"time"
)

// execTimeout 外部命令执行超时（15s），防止 sc/reg/netsh 等命令挂起阻塞客户端命令处理循环
const execTimeout = 15 * time.Second

// execWithTimeout 带超时执行外部命令并返回输出
func execWithTimeout(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

// ListServices 列出 Windows 服务（sc query）
func ListServices() ([]byte, error) {
	return execWithTimeout("sc", "query")
}

// StartService 启动 Windows 服务
func StartService(name string) ([]byte, error) {
	return execWithTimeout("sc", "start", name)
}

// StopService 停止 Windows 服务
func StopService(name string) ([]byte, error) {
	return execWithTimeout("sc", "stop", name)
}
