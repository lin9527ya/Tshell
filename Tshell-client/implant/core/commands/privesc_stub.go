//go:build !windows

package commands

import "errors"

// CmdGetSystem 提权是 Windows 专有能力（令牌操纵 + UAC 劫持）。
// 显式报错而不是静默返回成功文本：提权是"结果决定下一步"的操作，
// 假成功会让操作员以为已经拿到 SYSTEM 而继续做需要高权限的动作。
func CmdGetSystem(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("提权（getsystem）仅支持 Windows")
}
