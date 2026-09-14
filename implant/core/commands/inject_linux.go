//go:build linux

package commands

import "errors"

// CmdInjectMem 内存注入仅 Windows 支持。
func CmdInjectMem(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("内存注入仅 Windows 支持")
}
