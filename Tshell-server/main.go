//go:build !debug
// +build !debug

package main

import (
	"BackendTemplate/pkg/api"
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/backup"
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/cli"
	"BackendTemplate/pkg/cna"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/handlers"
	"BackendTemplate/pkg/metrics"
	"BackendTemplate/pkg/notification"
	"BackendTemplate/pkg/proxy"
	"BackendTemplate/pkg/security"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/signing"
	"BackendTemplate/pkg/utils/logx"
	"context"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed dist
var embedFS embed.FS

// 构建期注入（-ldflags "-X main.Version=..."）：用于 /api/version 与产物溯源。
// 未注入时保持 dev/unknown，避免把本地开发构建误认为发布版本。
var (
	Version   = "1.0.0"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

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
	// 备份/恢复 CLI：恢复必须在服务端未运行时进行，因此这几个开关处理完即退出，
	// 不进入正常的服务端启动流程
	var backupNow = flag.Bool("backup", false, "立即创建一份数据库备份并退出")
	var restoreFrom = flag.String("restore", "", "用指定备份恢复数据库并退出（需先停止服务端）")
	var restoreForce = flag.Bool("force", false, "恢复时跳过“数据库可能正在使用”检查")
	var listBackups = flag.Bool("list-backups", false, "列出备份并退出")
	// OPSEC：弱口令时生成的随机管理员密码默认只写入 0600 文件，不进日志。
	// 需要人工查看时显式加这个开关（并确认当前输出不会被留存）。
	// OPSEC 阶段三：审计日志哈希链校验（只读，校验后退出）
	var verifyAudit = flag.String("verify-audit", "", "校验审计日志文件的哈希链并退出（传入文件路径）")
	var printInitPassword = flag.Bool("print-initial-password", false,
		"把生成的初始管理员密码打印到 stdout（默认只写入 0600 文件）")
	// 重定向器配置生成（阶段三）：按当前画像生成 nginx/apache 配置后退出
	var genRedirector = flag.String("gen-redirector", "", "生成重定向器配置（nginx|apache）并退出")
	var rdBackend = flag.String("backend", "", "重定向器后端地址 host:port（真正的 C2 服务端）")
	var rdDomain = flag.String("domain", "", "重定向器对外域名（TLS 证书名）")
	var rdCert = flag.String("cert", "", "TLS 证书路径（-full 时必填）")
	var rdKey = flag.String("key", "", "TLS 私钥路径（-full 时必填）")
	var rdDecoy = flag.String("decoy", "", "诱饵站点 URL（非画像请求的落点）")
	var rdProfile = flag.String("profile", "default", "使用的画像名")
	var rdOut = flag.String("out", "", "输出文件（缺省打印到 stdout）")
	var rdFull = flag.Bool("full", false, "生成完整可用的配置文件（nginx）而非 include 片段")
	flag.Parse()

	// 加载配置
	if err := config.InitGlobalConfig(*configFile); err != nil {
		fmt.Printf("配置系统初始化失败: %v\n", err)
		os.Exit(1)
	}

	appConfig := config.GetGlobalConfig()

	// 下载制品目录（config.storage.downloads_dir）：缺失时按需创建，失败只告警
	// —— 下载目录不可用不应阻断服务启动（与历史行为一致）
	downloadsDir := config.GetDownloadsDir()
	if st, statErr := os.Stat(downloadsDir); statErr != nil || !st.IsDir() {
		if mkErr := os.MkdirAll(downloadsDir, os.ModePerm); mkErr != nil {
			fmt.Printf("⚠️ 创建下载目录失败(%s): %v\n", downloadsDir, mkErr)
		}
	}
	command.SetDownloadsDir(downloadsDir)

	// ===== 备份/恢复 CLI（处理完即退出）=====
	cliBackupMgr := backup.New(backup.Options{
		DBPath: database.DatabaseName,
		Dir:    appConfig.Backup.Dir,
		Keep:   appConfig.Backup.Keep,
		Prefix: "tshell-backup",
	})
	if *verifyAudit != "" {
		// 审计哈希链校验：篡改/删除/重排都会定位到具体行号。
		// 退出码 0=完好、1=断链、2=无法读取（便于脚本化巡检）。
		res, verr := audit.VerifyAuditFile(*verifyAudit)
		if verr != nil {
			fmt.Printf("❌ 无法读取审计日志 %s: %v\n", *verifyAudit, verr)
			os.Exit(2)
		}
		if res.OK() {
			fmt.Printf("✅ 审计链完好：%s（%d 条，链尾 %s）\n",
				*verifyAudit, res.Entries, shortHash(res.LastHash))
			os.Exit(0)
		}
		fmt.Printf("❌ 审计链断裂：%s\n   位置：第 %d 行（此前 %d 条校验通过）\n   原因：%s\n",
			*verifyAudit, res.BadLine, res.Entries, res.Reason)
		os.Exit(1)
	}
	if *listBackups {
		os.Exit(cli.RunListBackups(cliBackupMgr))
	}
	if *backupNow {
		os.Exit(cli.RunBackupNow(cliBackupMgr))
	}
	if *restoreFrom != "" {
		os.Exit(cli.RunRestore(cliBackupMgr, *restoreFrom, database.DatabaseName, *restoreForce))
	}
	if *genRedirector != "" {
		// 画像从 profiles/ 目录读取（与服务端同一来源，避免生成用的画像与实际部署的不一致）
		if err := c2profile.InitNoWatch("./profiles"); err != nil {
			fmt.Printf("❌ 加载画像失败: %v\n", err)
			os.Exit(1)
		}
		os.Exit(cli.RunGenRedirector(*genRedirector, cli.RedirectorOptions{
			Backend: *rdBackend, Domain: *rdDomain, Cert: *rdCert, Key: *rdKey,
			Decoy: *rdDecoy, Profile: *rdProfile, Out: *rdOut, Full: *rdFull,
		}))
	}

	// 日志级别：debug|info|warn|error（服务端输出内容开关）
	logx.SetLevel(logx.ParseLevel(appConfig.Logging.Level))
	// 日志格式与输出（config.logging.format/output 此前是死配置：logx 恒为 text+stdout）
	logx.SetFormat(appConfig.Logging.Format)

	// 代码签名（可选）：启用后生成的 Windows 载荷必须签名成功，否则生成失败（不静默降级）。
	// 校验放在启动期：证书缺失/工具不在 PATH 属于配置错误，越早暴露越好。
	signingCfg := signing.Config{
		Enabled:  appConfig.Signing.Enable,
		Command:  appConfig.Signing.Command,
		Args:     appConfig.Signing.Args,
		PFX:      appConfig.Signing.PFX,
		Password: appConfig.Signing.Password,
		Timeout:  time.Duration(appConfig.Signing.TimeoutSeconds) * time.Second,
	}
	if err := signing.Validate(signingCfg); err != nil {
		fmt.Printf("❌ 代码签名配置错误: %v\n", err)
		os.Exit(1)
	}
	signing.Configure(signingCfg)
	if signing.Enabled() {
		fmt.Printf("✍️  代码签名已启用（%s）\n", signing.Current().Command)
	}
	if strings.EqualFold(appConfig.Logging.Output, "file") {
		logDir := appConfig.Logging.AuditLogDir
		if logDir == "" {
			logDir = "./logs"
		}
		_ = os.MkdirAll(logDir, 0o750)
		if ferr := logx.SetOutputFile(filepath.Join(logDir, "tshell.log")); ferr != nil {
			logx.Warnf("⚠️ [配置] 日志文件打开失败，回退 stdout: %v\n", ferr)
		}
	}
	// jwt.algorithm 此前完全未被读取：实现固定 RS256（pkg/security/jwt_manager.go）。
	// 显式提示，避免运维以为改成了 HS256 却仍在用 RS256。
	if alg := appConfig.JWT.Algorithm; alg != "" && !strings.EqualFold(alg, "RS256") {
		logx.Warnf("⚠️ [配置] jwt.algorithm=%s 不受支持（实现固定 RS256），已忽略\n", alg)
	}

	// 启动安全自检：敏感文件权限（含密钥/战利品明文）与默认弱口令
	for _, p := range insecurePermPaths(
		*configFile,
		appConfig.Database.Path,
		appConfig.JWT.SecretKeyFile,
		"jwt_private.pem",
	) {
		if st, err := os.Stat(p); err == nil {
			logx.Warnf("⚠️ [安全] %s 权限为 %#o（建议 0600）：该文件含密钥/凭据明文，请执行 chmod 600 %s\n",
				p, st.Mode().Perm(), p)
		}
	}

	// OPSEC P0-4：自己管理的数据目录做一次权限收敛（目录 0700）。
	//
	// 历史部署里这些目录是 0777 建的，MkdirAll 不会修正已存在的目录，
	// 所以每次启动收敛一次。改过哪些目录要记下来 —— 静默改权限不合适：
	// 运维排查"共享账号读不到文件了"时需要知道是启动时收紧的。
	if changed := security.TightenDataDirs(
		config.GetDownloadsDir(),
		config.GetPluginDir(),
		"./sessions",
	); len(changed) > 0 {
		logx.Warnf("⚠️ [安全] 已把以下数据目录权限收敛为 0700（原权限过宽）：%s\n",
			strings.Join(changed, ", "))
	}

	if strings.EqualFold(appConfig.Auth.DefaultPassword, "admin") || len(appConfig.Auth.DefaultPassword) < 8 {
		logx.Warnf("⚠️ [安全] auth.default_password 为弱口令（长度<8 或为 admin），请修改配置\n")
	}
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
	//
	// jwt.algorithm 从"死配置"变成真约束：配置与实际实现不符时拒绝启动。
	// 它曾经默认写 HS256 而实现走 RS256 —— 一个"配了但没用上"的算法配置比没有
	// 更危险，因为运维会以为已经换了算法。
	if g := config.GetGlobalConfig(); g != nil {
		if err := config.ValidateJWTAlgorithm(g.JWT.Algorithm); err != nil {
			fmt.Printf("JWT 配置错误: %v\n", err)
			os.Exit(1)
		}
		// HSTS 开关（security.force_hsts）注入安全头中间件
		security.InitSecurityHeaders(g.Security.ForceHSTS)
	}
	err = security.InitJWTManager(2048, 24*time.Hour) // 2048位RSA密钥，24小时轮换
	if err != nil {
		fmt.Printf("JWT管理器初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ JWT管理器初始化成功")

	// 初始化 MFA 密钥加密器（阶段三 §3.5）：由 JWT 私钥经 HKDF 派生，
	// 供 TOTP 密钥加密落库使用。必须在 InitJWTManager 之后 —— 它依赖私钥。
	if err := security.InitMFASecretCipher(security.GlobalJWTManager.PrivateKey()); err != nil {
		fmt.Printf("MFA 密钥加密器初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ MFA 密钥加密器初始化成功")

	// OPSEC P0-1：战果（口令/哈希/票据密钥）静态加密。
	// 优先用独立的密钥文件（可与数据库分开保管）；未配置则由 JWT 私钥派生。
	// 初始化失败**直接退出** —— 不能带着"战果会以明文落库"的状态继续跑：
	// 那正是本项要消除的风险，静默降级等于白做。
	if keyFile := appConfig.Security.AtRestKeyFile; keyFile != "" {
		if err := security.InitAtRestCipherFromKeyFile(keyFile); err != nil {
			fmt.Printf("战果静态加密初始化失败（security.at_rest_key_file=%s）: %v\n", keyFile, err)
			os.Exit(1)
		}
		fmt.Printf("✅ 战果静态加密已启用（独立密钥文件 %s）\n", keyFile)
	} else {
		if err := security.InitAtRestCipherFromPrivateKey(security.GlobalJWTManager.PrivateKey()); err != nil {
			fmt.Printf("战果静态加密初始化失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ 战果静态加密已启用（由 JWT 私钥派生；建议改用 security.at_rest_key_file 分离保管）")
	}
	// 注入到领域层（pkg/security 依赖 pkg/database，不能反向 import，故用注入）
	database.SetLootCrypto(security.Seal, security.Open, security.Fingerprint)

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

	// 存储后端选择（config.database.type/dsn）：sqlite3（默认）或 postgres
	database.DBType = strings.ToLower(strings.TrimSpace(appConfig.Database.Type))
	database.PGDSN = strings.TrimSpace(appConfig.Database.DSN)
	if database.DBType == "postgres" || database.DBType == "postgresql" {
		database.DBType = "postgres"
		if database.PGDSN == "" {
			logx.Errorf("❌ [数据库] database.type=postgres 但 database.dsn 为空\n")
			os.Exit(1)
		}
		logx.Infof("🗄️ [数据库] 使用 PostgreSQL 后端\n")
	}
	database.ConnectDateBase()
	defer database.DB().Close()
	// 连接池参数（config.database.* 此前是死配置：ConnectDateBase 硬编码 10/5/5m）
	database.ConfigurePool(appConfig.Database.MaxOpenConns, appConfig.Database.MaxIdleConns, appConfig.Database.MaxLifetime)

	// OPSEC P0-1：把存量明文战果加密回写（幂等，只在有新明文时动行）。
	// 放在连库之后、业务起来之前 —— 早于任何读取，避免"读到旧明文又当成新数据"。
	if n, err := database.MigrateLootSecrets(); err != nil {
		// 迁移失败不阻断启动（已有密文行仍可正常读写），但必须喊出来：
		// 沉默的后果是"以为加密了，其实还有明文行留在库里"。
		logx.Errorf("❌ [安全] 战果明文迁移失败（库中可能仍有明文行）: %v\n", err)
	} else if n > 0 {
		logx.Warnf("⚠️ [安全] 已把 %d 条历史明文战果加密回写\n", n)
	}

	// ===== 数据库备份 =====
	if appConfig.Backup.Enabled && database.IsPostgres() {
		// 备份子系统是 SQLite 语义（VACUUM INTO + 文件替换），PostgreSQL 下不启用；
		// 保留 /api/backup/* 的 503 语义，避免操作员以为备份在工作。
		logx.Warnf("⚠️ [备份] PostgreSQL 后端不使用内置备份（VACUUM INTO 仅适用于 SQLite），请用 pg_dump / pg_basebackup\n")
	} else if appConfig.Backup.Enabled {
		backupMgr := backup.New(backup.Options{
			DBPath:   database.DatabaseName,
			Driver:   appConfig.Database.Type,
			Dir:      appConfig.Backup.Dir,
			Keep:     appConfig.Backup.Keep,
			Interval: appConfig.Backup.Interval,
		})
		api.SetBackupManager(backupMgr)
		if appConfig.Backup.OnStartup {
			if info, err := backupMgr.Create("startup"); err != nil {
				logx.Errorf("❌ [备份] 启动备份失败: %v\n", err)
			} else {
				logx.Infof("🗄️ [备份] 启动备份完成 %s (%.1f MB)\n", info.Name, float64(info.Size)/1024/1024)
			}
		}
		stopBackup := backupMgr.StartScheduler()
		defer stopBackup()
		logx.Infof("🗄️ [备份] 已启用: 目录=%s 间隔=%v 保留=%d 份（恢复需停服务后用 -restore，见 docs/BACKUP_RESTORE.md）\n",
			appConfig.Backup.Dir, appConfig.Backup.Interval, appConfig.Backup.Keep)
	}

	// 初始化统一离线检测器（B13: 替代各协议 duplicated 的 checkHeartbeats）
	connection.InitOfflineDetector()

	// 启动批量任务记录 TTL 清理（防止内存无界增长）
	api.StartTaskCleanup()

	// OPSEC P0-5：战果保留期清理（默认关闭，由 security.loot_retention_days 开启）。
	// 战果默认永久留存，与任务的 7 天、审计的按天清理不一致 ——
	// 留存越久泄露面越大，所以给出显式开关，但**不替运维默认打开**
	// （删除战果不可逆，不该由一次升级替人做决定）。
	if days := appConfig.Security.LootRetentionDays; days > 0 {
		stopLootRetention := api.StartLootRetention(days)
		defer stopLootRetention()
		logx.Infof("🧹 [安全] 战果保留期已启用：%d 天\n", days)
	} else {
		logx.Infof("ℹ️ [安全] 战果保留期为永久（security.loot_retention_days=0）；如需自动清理请显式配置\n")
	}

	// 任务队列可视化:命令历史 TTL 清理
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			command.GlobalCmdHistory.CleanupExpired()
		}
	}()

	// 从配置文件初始化默认管理员用户。
	// 密码文件与 database.db 同目录，便于运维一起做权限收敛与清理。
	err = database.InitializeDefaultUserFromConfig(
		appConfig.Auth.DefaultUsername,
		appConfig.Auth.DefaultPassword,
		database.InitOptions{
			PrintPasswordToStdout: *printInitPassword,
			PasswordFileDir:       initialPasswordDir(appConfig),
		},
	)
	if err != nil {
		fmt.Printf("初始化默认管理员用户失败: %v\n", err)
		// 不退出，继续启动，但记录警告
	} else {
		fmt.Printf("✅ 管理员用户初始化成功：%s\n", appConfig.Auth.DefaultUsername)
	}

	// 初始化审计日志系统（目录/大小/天数来自 config.logging.*，此前硬编码 ./logs、100MB、30 天）
	auditDir := appConfig.Logging.AuditLogDir
	if auditDir == "" {
		auditDir = "./logs"
	}
	auditMaxSize := appConfig.Logging.MaxFileSize
	if auditMaxSize <= 0 {
		auditMaxSize = 100 * 1024 * 1024
	}
	auditMaxDays := appConfig.Logging.MaxRetainDays
	if auditMaxDays <= 0 {
		auditMaxDays = 30
	}
	err = audit.InitAuditLogger(auditDir, auditMaxSize, auditMaxDays)
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

	// ===== 可观测性：Prometheus 指标 =====
	// 访问策略来自 config.metrics（默认只允许本机抓取；跨机需 token）
	api.SetMetricsOptions(api.MetricsOptions{
		Enabled:     appConfig.Metrics.Enabled,
		Token:       appConfig.Metrics.Token,
		AllowRemote: appConfig.Metrics.AllowRemote,
	})
	// 启用指标时放行 /metrics 通过隐藏访问入口（抓取方无法完成隐藏入口握手）
	security.SetMetricsPathBypass(appConfig.Metrics.Enabled)
	metrics.Register()
	metrics.BindSnapshots(metrics.SnapshotFuncs{
		Beacons: func() (int64, int64) {
			if database.DB() == nil {
				return 0, 0
			}
			online, _ := database.DB().Where("online = ?", "1").Count(&database.Clients{})
			total, _ := database.DB().Count(&database.Clients{})
			return online, total
		},
		Connections: func() map[string]int64 {
			out := make(map[string]int64)
			if connection.GlobalConnectionManager == nil {
				return out
			}
			for _, ci := range connection.GlobalConnectionManager.GetAllConnections() {
				out[ci.ProtocolSafe()]++
			}
			return out
		},
		CommandQueues: func() (int64, int64) {
			depth, maxPerClient, _ := command.CommandQueues.Stats()
			return depth, maxPerClient
		},
		EventSubscribers: func() int64 {
			if command.GlobalEventBus == nil {
				return 0
			}
			return int64(command.GlobalEventBus.SubscriberCount())
		},
		DBConnections: func() (int64, int64, int64, int64) {
			st, ok := database.DBStats()
			if !ok {
				return 0, 0, 0, 0
			}
			return int64(st.OpenConnections), int64(st.InUse), int64(st.Idle), st.WaitCount
		},
	})
	if appConfig.Metrics.Enabled {
		mode := "仅本机抓取"
		if appConfig.Metrics.Token != "" {
			mode = "需 Bearer token"
		} else if appConfig.Metrics.AllowRemote {
			mode = "⚠️ 允许跨机且未配 token（任何人可读运行情报，建议配置 metrics.token）"
		}
		logx.Infof("📊 [指标] /metrics 已启用（%s）\n", mode)
	}

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
	// 命令回包识别：WiFi 密码等产出自动入战利品库（识别逻辑在 pkg/api，避免反向依赖）
	command.LootIngestFunc = func(uid, output string) { api.IngestLootFromOutput(uid, output) }
	// 任务回显归属：回包到达时把该 uid 最早一条 dispatched 任务置 completed
	command.TaskOutputFunc = api.OnTaskOutput
	// 插件/内存执行完成回执（EXEC_DONE）：按 taskID 精确收敛 exec 任务
	command.ExecDoneFunc = api.OnExecDone
	// 能力客户端：通知它把执行过程输出切到 EXEC_OUTPUT（不再触发任务完成）
	command.ExecOutputEnableFunc = api.EnableClientExecOutput

	// 下载终态（完成/中断）→ 收敛任务状态；下载协议无结束报文，只能由数据层判定
	command.DownloadEndFunc = api.OnDownloadEnd
	// 订阅客户端上线事件：触发 cna beacon_initial 事件（tcp/ws/kcp/http 各协议统一入口）
	if command.GlobalEventBus != nil {
		go func() {
			// 事件分发跑在裸 goroutine 里，没有 Gin Recovery 兜底：单个脚本的
			// panic 若逃逸会直接终止整个 C2 进程。脚本自身的护栏在 pkg/cna 内，
			// 这里再兜一层，覆盖引擎之外（如事件总线本身）的异常。
			defer func() {
				if r := recover(); r != nil {
					logx.Errorf("❌ [CNA] 事件分发 panic 已捕获: %v\n", r)
				}
			}()
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

	database.DB().Update(&database.Clients{Online: "2"})

	// 监听器不随服务重启自动恢复：重启即自动对外开放 C2 监听端口风险更高，故统一置停，
	// 由操作员手动启动（WebDelivery 相反——它保留 status=1 由 Delivery Hub 恢复）。
	// 但要先把原本在跑的列出来，否则操作员只看到"全部已停止"，不知道重启前有哪些在跑。
	var listenersRunningBeforeRestart []database.Listener
	if err := database.DB().Where("status = ?", 1).Find(&listenersRunningBeforeRestart); err != nil {
		fmt.Printf("⚠️ 读取重启前监听器状态失败: %v\n", err)
	}
	database.DB().Update(&database.Listener{Status: 2})
	if n := len(listenersRunningBeforeRestart); n > 0 {
		fmt.Printf("⚠️ 已置停 %d 个监听器（不自动恢复，需手动启动）：\n", n)
		for _, lis := range listenersRunningBeforeRestart {
			fmt.Printf("   %s://%s\n", lis.Type, lis.ListenAddress)
		}
	}

	// WebDelivery 不再整体置关闭：由 Delivery Hub 按库中 status=1 记录恢复
	delivery.GlobalHub.SetMainPort(*bindPort)

	r := gin.New()
	// 可信代理网段：gin 默认信任任意来源的 X-Forwarded-For，直连部署下
	// 审计日志的 client_ip / 在线操作员的 last_ip 可被远程请求伪造（见 pkg/security/proxy_trust.go）
	if err := security.ApplyTrustedProxies(r); err != nil {
		logx.Warnf("⚠️ [网络] 可信代理网段设置失败，将按 gin 默认（信任任意来源转发头）运行: %v\n", err)
	}

	// 添加错误处理中间件
	r.Use(errors.Recovery())
	r.Use(errors.RequestIDMiddleware())
	// 可观测性：请求计数/耗时直方图 + 慢请求日志（放在认证之前，
	// 这样未认证请求与限流耗时也在观测范围内）
	r.Use(metrics.Middleware())
	r.Use(errors.TimestampMiddleware())
	r.Use(errors.ErrorHandlerMiddleware())
	// J9: 全局请求体大小限制（10MB，防超大 JSON 内存 DoS）
	// 默认 10MB；上传类接口按各自 handler 声明的上限放宽（+1MB 作为 multipart 边界余量），
	// 让 handler 里的"文件过大"检查成为真正生效的那道闸并给出准确文案。
	r.Use(errors.MaxBodyBytesMiddlewareWithLimits(10*1024*1024, map[string]int64{
		"/api/plugin/upload":         api.MaxPluginUploadBytes + 1<<20,
		"/api/bin/execute":           api.MaxExecuteBinBytes + 1<<20,
		"/api/client/file/upload":    500<<20 + 1<<20,
		"/api/webdelivery/host-file": 50<<20 + 1<<20,
		"/api/client/inject-mem":     32<<20 + 1<<20,
		"/api/cna/upload":            5<<20 + 1<<20,
		"/api/backup/import":         api.MaxBackupImportBytes + 1<<20,
	}))

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
		deliveryPrefix := delivery.RoutePrefix()
		if strings.HasPrefix(path, deliveryPrefix+"/") || strings.Contains(path, deliveryPrefix+"/") || strings.HasPrefix(path, "/d/") {
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
		// MFA 第二步（阶段三 §3.5）：口令已过、待动态码。
		// **必须挂在同一组**：动态码只有 6 位，缺了登录限流与暴破防护就等于开了在线暴破的门。
		a.POST("/users/login/mfa", api.MFALoginHandler)
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

	// 构建信息（所有登录角色）：排障与产物溯源（版本/commit/构建时间由 ldflags 注入）
	// Prometheus 指标端点：默认只允许本机抓取（指标含 beacon 在线数等运行情报），
	// 跨机抓取需配 metrics.token；见 api.MetricsHandler 的访问规则
	r.GET("/metrics", api.MetricsHandler)

	protected.GET("/version", func(c *gin.Context) {
		// schema_version 让运维能一眼看出"这个二进制对应的库结构到哪一步了"
		schemaVersion, _ := database.SchemaVersion()
		c.JSON(http.StatusOK, gin.H{
			"version":        Version,
			"commit":         GitCommit,
			"build_time":     BuildTime,
			"schema_version": schemaVersion,
		})
	})

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

	// ===== MFA/TOTP（阶段三 §3.5）：自助管理，任意登录用户可管自己的账号 =====
	protected.GET("/users/mfa", api.MFAMeHandler)
	protected.POST("/users/mfa/start", api.MFAStartHandler)
	protected.POST("/users/mfa/confirm", api.MFAConfirmHandler)
	protected.POST("/users/mfa/disable", api.MFADisableHandler)
	// 二维码渲染限定 otpauth:// 前缀（见 MFAQRHandler：否则会变成任意文本的二维码生成器）
	protected.GET("/users/mfa/qr", api.MFAQRHandler)

	// 用户管理（仅 admin；用户列表/创建/删除/重置密码 —— 权限分离）
	// 数据库备份：仅 admin（备份含全部凭据与战利品；恢复走 CLI，不提供热恢复）
	adminOnly.GET("/backup/list", api.ListBackups)
	adminOnly.POST("/backup/create", api.CreateBackup)
	adminOnly.GET("/backup/download/:name", api.DownloadBackup)
	adminOnly.DELETE("/backup/:name", api.DeleteBackup)
	adminOnly.POST("/backup/import", api.ImportBackup)

	adminOnly.POST("/users/user_setting/get_user_list", api.GetUserListHandler)
	adminOnly.POST("/users/user_setting/create_user", api.CreateUserHandler)
	adminOnly.POST("/users/user_setting/delete_user", api.DeleteUserHandler)
	adminOnly.POST("/users/user_setting/admin_reset_password", api.AdminResetPasswordHandler)
	// 重置他人的 MFA（丢手机的救援路径）：仅 admin，且要求带上管理员自己的口令
	adminOnly.POST("/users/mfa/reset", api.MFAResetHandler)

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
	// 客户端升级：重新生成载荷 + self_update（仅独立 EXE payload；破坏性操作需 confirm）
	operatorOnly.POST("/client/upgrade", api.UpgradeClients)
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
	// 任务一等对象：明细查询（可过滤/分页）与取消（仅 dispatched 可取消）
	operatorOnly.GET("/tasks", api.ListTaskRows)
	operatorOnly.GET("/tasks/:id", api.GetTaskRow)
	operatorOnly.POST("/tasks/:id/cancel", api.CancelTask)

	// 多操作员协作：在线状态（读：全角色）+ 会话锁（写：admin/operator）
	protected.GET("/collab/operators", api.ListOperators)
	protected.GET("/collab/locks", api.ListSessionLocks)
	operatorOnly.POST("/collab/locks/:uid", api.AcquireSessionLock)
	operatorOnly.DELETE("/collab/locks/:uid", api.ReleaseSessionLock)
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
	// 凭据获取（阶段三）：制品的登记与校验在 pkg/cred，回传复用下载通道
	operatorOnly.POST("/cred/dump", api.DumpCredential)
	operatorOnly.GET("/cred/artifacts", api.ListCredArtifacts)

	// 重定向器配置：按当前画像生成 nginx/apache 配置（手写最容易与画像漂移）
	operatorOnly.GET("/redirector/template", api.GetRedirectorTemplate)

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
	operatorOnly.POST("/listener/update", api.UpdateListener)   // 添加更新监听器API
	operatorOnly.POST("/listener/note", api.UpdateListenerNote) // 列表内联编辑备注（只写 note 列）
	operatorOnly.POST("/listener/delete", api.DeleteListener)
	operatorOnly.POST("/listener/batch-delete", api.BatchDeleteListeners)

	// 投递列表/命令矩阵会返回 access_token、pass 与带 ?t= 的完整 URL，
	// 不能对 auditor 开放（原先挂 protected，只读账号可直接调用 API 读到）。
	operatorOnly.GET("/webdelivery/list", api.ListWebDelivery)
	operatorOnly.POST("/webdelivery/start", api.StartWebDelivery)
	operatorOnly.POST("/webdelivery/update", api.UpdateWebDelivery) // 仅允许编辑已关闭的投递
	// 文件托管: 任意文件经 Delivery Hub /d/ 通道对外分发(复用其访问控制)
	operatorOnly.POST("/webdelivery/host-file", api.HostFileDelivery)
	operatorOnly.POST("/webdelivery/close", api.CloseWebDelivery)
	operatorOnly.POST("/webdelivery/open", api.OpenWebDelivery)
	operatorOnly.POST("/webdelivery/delete", api.DeleteWebDelivery)
	// StagerToken 门禁令牌（防盗门）：生成/列表/删除
	operatorOnly.POST("/webdelivery/token/create", api.CreateStagerToken)
	operatorOnly.GET("/webdelivery/token/list", api.ListStagerTokens)
	operatorOnly.POST("/webdelivery/token/delete", api.DeleteStagerToken)
	operatorOnly.GET("/webdelivery/commands", api.GetDeliveryCommands)
	protected.GET("/webdelivery/logs", api.GetDeliveryAccessLogs)
	protected.GET("/webdelivery/defaults", api.GetDeliveryDefaults)
	protected.POST("/webdelivery/preview-urls", api.PreviewDeliveryURLs)

	// Delivery Hub 公开投递路由（无需 JWT）：{prefix}/{key}/full|stager|sc|woff
	// 同时注册 base_path 前缀（CDN 回源保留前缀时，如 /static{prefix}/...）
	// r.Any 已覆盖 GET/POST/PUT/PATCH/HEAD/OPTIONS/DELETE，勿再单独注册 HEAD（会与 *filepath 冲突 panic）
	deliveryHandler := func(c *gin.Context) {
		delivery.GlobalHub.ServeHTTP(c.Writer, c.Request)
	}
	deliveryPrefix := delivery.RoutePrefix()
	security.SetDeliveryRoutePrefix(deliveryPrefix)
	audit.SetDeliveryRoutePrefix(deliveryPrefix)
	r.Any(deliveryPrefix+"/*filepath", deliveryHandler)
	if deliveryPrefix != "/d" {
		// 改前缀后继续接旧 /d/ 路由，避免升级瞬间已发出去的旧载荷全体 404
		r.Any("/d/*filepath", deliveryHandler)
	}
	if gcfg := config.GetGlobalConfig(); gcfg != nil {
		bp := strings.Trim(gcfg.Delivery.BasePath, "/")
		if bp != "" {
			// 注意：通配符参数名必须与已有路由一致（*filepath），且路径不能与主投递路由重复
			prefix := "/" + bp + deliveryPrefix + "/*filepath"
			if prefix != deliveryPrefix+"/*filepath" {
				r.Any(prefix, deliveryHandler)
				fmt.Println("Delivery Hub CDN prefix:", "/"+bp+deliveryPrefix+"/{key}/...")
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
		keyManagement.GET("/metrics", api.GetKeyMetrics)
		keyManagement.PUT("/rotation-config", api.ConfigureKeyRotation)
	}

	// 隧道代理管理API（读 protected，写 operator+）。
	// 原有一套并行的 /socks5/* 旧实现（Socks5 表 + Socks5View.vue），协议上只是
	// 隧道的 socks5 模式，已并入此处；历史数据由数据库迁移 v6 搬入 tunnel_proxy。
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
	// 读接口开放给 admin + auditor：auditor 的职责就是看审计（前端角色表也给 auditor 授了
	// 权限码 '14' Audit），原来四个接口全是 adminOnly，导致 auditor 能点开「日志」页签却必然 403。
	// 导出（外带全量日志）与清理（破坏性）仍限 admin。
	auditRead := protected.Group("/audit")
	auditRead.Use(security.RequireRole(security.RoleAdmin, security.RoleAuditor))
	{
		auditRead.GET("/logs", audit.GetAuditLogs)
		auditRead.GET("/statistics", audit.GetAuditStatistics)
	}
	auditAPI := adminOnly.Group("/audit")
	{
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
	// 读接口保持全角色（auditor 可查看连接状态），写操作（断开连接/改池与工作池配置）
	// 收紧到 admin+operator —— 原来整组 protected，只读角色 auditor 也能关连接、改池配置。
	// health-check 是非破坏性探测，留在读组。
	connectionAPI := protected.Group("/connection")
	{
		connectionAPI.GET("/pool/stats", connection.GetConnectionPoolStats)
		connectionAPI.GET("/worker/stats", connection.GetWorkerPoolStats)
		connectionAPI.GET("/details", connection.GetConnectionDetails)
		connectionAPI.GET("/protocol/:protocol", connection.GetConnectionsByProtocol)
		connectionAPI.POST("/health-check", connection.PerformHealthCheck)
		connectionAPI.GET("/uid/:uid", connection.GetConnectionByUID)
	}
	connectionWrite := operatorOnly.Group("/connection")
	{
		connectionWrite.DELETE("/:id", connection.CloseConnection)
		connectionWrite.PUT("/pool/config", connection.UpdatePoolConfig)
		connectionWrite.PUT("/worker/resize", connection.ResizeWorkerPool)
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
	// 读接口（仅元数据：名称/大小/哈希）保留全角色，与 /client/clientslist、/client/file/tree 同级；
	// 下载会外带可下发执行的载荷二进制，上传/删除会写入攻击载荷仓库 → 收紧到 admin+operator
	// （口径同 /client/file/download、/bin/execute）。
	// 注意：不要挂到 operatorFile —— FileOperationAuditMiddleware 会对请求体做无上限
	// io.ReadAll，50MB 插件上传会整包驻留内存；插件审计在 handler 内直接记（见 plugin.go）。
	protected.GET("/plugin/list", api.GetPlugins)
	protected.GET("/plugin/details/:pluginName", api.GetPluginDetails)
	operatorOnly.GET("/plugin/download/:pluginName", api.DownloadPlugin)
	operatorOnly.POST("/plugin/upload", api.UploadPlugin)
	operatorOnly.POST("/plugin/execute", api.ExecutePlugin)
	operatorOnly.DELETE("/plugin/delete/:pluginName", api.DeletePlugin)
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
	// 与同为载荷生成的 /client/GenServer(operatorOnly) 口径一致：
	// 原来挂 protected，只读角色 auditor 也能生成 stager/shellcode。
	shellcodeAPI := operatorOnly.Group("/shellcode")
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
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// 恢复 status=1 的 WebDelivery 到 Delivery Hub（shared/dedicated）
	go func() {
		// 稍等主端口已写入 Hub
		time.Sleep(200 * time.Millisecond)
		api.RestoreWebDeliveriesOnStartup()
		// 恢复运行中的隧道代理（等客户端重连后拉起，见 RestoreTunnelsOnStartup）
		api.RestoreTunnelsOnStartup()
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

// insecurePermPaths 返回权限过宽（组/其他用户可访问）的敏感文件路径。
// 数据库与配置里存着监听器 VKey/Salt、战利品明文口令与平台密钥——
// 评估发现默认部署下它们常是 644/664，同机任意用户可读。
// initialPasswordDir 初始管理员密码文件的落盘目录。
//
// 优先与 database.db 同目录：运维做磁盘权限收敛和"删掉初始密码文件"时
// 只需要盯一个目录。PostgreSQL 或未配置路径时回落到当前工作目录。
// shortHash 展示用短哈希（前 12 位足够区分，且日志里不刷屏）
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
func initialPasswordDir(c *config.Config) string {
	if c == nil {
		return "."
	}
	if c.Database.Type == "sqlite3" && c.Database.Path != "" {
		if dir := filepath.Dir(c.Database.Path); dir != "" {
			return dir
		}
	}
	return "."
}

// insecurePermPaths 列出权限过宽的路径（供启动自检告警）。
// 只报告，不自动 chmod：部署可能有意让备份账号读取，改权限应由运维决定。
func insecurePermPaths(paths ...string) []string {
	var bad []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.Mode().Perm()&0o077 != 0 {
			bad = append(bad, p)
		}
	}
	return bad
}
