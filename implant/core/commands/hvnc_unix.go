//go:build !windows
// +build !windows

package commands

import "errors"

// HVNC（隐藏桌面）仅支持 Windows 平台

// StartHvnc 启动 HVNC 会话
func StartHvnc(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("HVNC 仅支持 Windows 客户端")
}

// StopHvnc 停止 HVNC 会话
func StopHvnc(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("HVNC 仅支持 Windows 客户端")
}

// SendHvncInput 注入 HVNC 输入事件
func SendHvncInput(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("HVNC 仅支持 Windows 客户端")
}
