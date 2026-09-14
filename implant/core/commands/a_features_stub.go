//go:build !windows

package commands

import (
	"errors"
)

// A1-A7 Windows 专属能力的非 Windows 占位（保持跨平台编译通过）

func CmdEtwPatch() ([]byte, error) {
	return nil, errors.New("ETW 补丁仅支持 Windows")
}

func CmdUnhookNtdll() ([]byte, error) {
	return nil, errors.New("ntdll unhooking 仅支持 Windows")
}

func CmdSandboxCheckEntry() ([]byte, error) {
	return nil, errors.New("沙箱检测仅支持 Windows")
}

func CmdAntidebugEntry() ([]byte, error) {
	return nil, errors.New("反调试检查仅支持 Windows")
}

func CmdWmiPersist(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("WMI 持久化仅支持 Windows")
}

func CmdPsexec(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("横向移动仅支持 Windows")
}

func CmdWmiExec(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("WMI 远程执行仅支持 Windows")
}

func CmdAdEnum(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("域枚举仅支持 Windows")
}

func CmdSelfUpdate(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("自我更新仅支持 Windows")
}

func CmdMakeToken(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("令牌操作仅支持 Windows")
}

func CmdStealToken(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("令牌操作仅支持 Windows")
}

func CmdRev2Self() ([]byte, error) {
	return nil, errors.New("令牌操作仅支持 Windows")
}
