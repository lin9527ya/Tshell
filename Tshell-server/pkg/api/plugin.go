package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/utils/logx"
)

// pluginKindFromName 按扩展名给出插件"种类"（列表/详情展示用）。
// 注意别与 database.Plugin.Type 混用：那一列的语义是执行模式
// （execute-assembly / inline-bin / ...），由调用方提供；插件文件本身
// 能推断出来的只有扩展名。
func pluginKindFromName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe":
		return "exe"
	case ".dll":
		return "dll"
	case ".net":
		return "net"
	case ".so":
		return "so"
	case ".dylib":
		return "dylib"
	case ".elf":
		return "elf"
	case ".bin":
		return "bin"
	case ".sh":
		return "sh"
	case ".py":
		return "py"
	case ".ps1":
		return "ps1"
	case ".o", ".obj", ".coff":
		return "bof"
	}
	return "unknown"
}

// pluginOSFromName 按扩展名推断目标平台（口径同 database.Plugin.Os：windows/linux/darwin）
func pluginOSFromName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe", ".dll", ".net", ".ps1", ".o", ".obj", ".coff":
		return "windows"
	case ".so", ".elf", ".sh":
		return "linux"
	case ".dylib":
		return "darwin"
	}
	return "" // .bin/.py 等无法判定
}

// displayOS 展示用平台：优先 DB 值，缺失时按扩展名推断
func displayOS(name, dbOS string) string {
	if dbOS != "" {
		return dbOS
	}
	if inferred := pluginOSFromName(name); inferred != "" {
		return inferred
	}
	return "unknown"
}

// shortHash 哈希前 12 位（列表/详情展示用）
func shortHash(h string) string {
	if h == "" {
		return "-"
	}
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// fileSHA256 计算文件 SHA256
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// backfillPluginMeta 为未入库的插件文件补录元数据（哈希 + 平台推断）并写库。
// 每个文件只在第一次列表时算一次 SHA256，之后走 DB。
func backfillPluginMeta(pluginDir, name string, info os.FileInfo) database.Plugin {
	row := database.Plugin{
		Name:       name,
		Os:         pluginOSFromName(name),
		FileName:   name,
		FileSize:   info.Size(),
		UploadTime: info.ModTime(),
	}
	hash, err := fileSHA256(filepath.Join(pluginDir, name))
	if err != nil {
		logx.Warnf("[PLUGIN-API] ⚠️ 补录元数据失败（无法计算哈希）: %s: %v\n", name, err)
		return row
	}
	row.FileHash = hash
	if _, err := database.Engine.Insert(&row); err != nil {
		logx.Warnf("[PLUGIN-API] ⚠️ 补录元数据写库失败: %s: %v\n", name, err)
		return row
	}
	logx.Infof("[PLUGIN-API] 📝 已补录插件元数据（历史文件）: %s hash=%s\n", name, shortHash(hash))
	return row
}

// auditPluginOp 记录插件仓库的文件操作审计（上传/下载/删除）。
// 插件路由不挂 audit.FileOperationAuditMiddleware —— 该中间件对请求体做无上限
// io.ReadAll，50MB 的插件上传会整包驻留内存；这里直接记同一种日志。
// 成功与否取响应状态码，与中间件口径一致（<400 视为成功）。
func auditPluginOp(c *gin.Context, operation, pluginName string) {
	if audit.GlobalAuditLogger == nil {
		return
	}
	username := ""
	if u, exists := c.Get("username"); exists {
		if s, ok := u.(string); ok {
			username = s
		}
	}
	audit.GlobalAuditLogger.LogFileOperation(c.ClientIP(), username, operation, pluginName,
		c.Writer.Status() < 400, "")
}

// PluginInfo 插件信息结构体
type PluginInfo struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	Type         string    `json:"type"`
	Description  string    `json:"description,omitempty"`
}

// GetPlugins 获取插件列表
type GetPluginsResponse struct {
	Status  int          `json:"status"`
	Message string       `json:"message"`
	Data    []PluginInfo `json:"data"`
}

func GetPlugins(c *gin.Context) {
	plugginDir := config.GetPluginDir()
	if _, err := os.Stat(plugginDir); os.IsNotExist(err) {
		os.MkdirAll(plugginDir, os.ModePerm)
	}

	// 文件系统是事实来源：插件文件可以被手工拷入/删除，DB 只用来补元数据。
	// 原实现是"DB 非空就完全不看目录"（文件被删仍显示幽灵条目）、"DB 空才扫目录"
	// （元数据全丢、类型退化成扩展名）——两套口径来回切。
	files, err := os.ReadDir(plugginDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "无法读取插件目录: " + err.Error()})
		return
	}

	var dbPlugins []database.Plugin
	_ = database.Engine.Find(&dbPlugins)
	byName := make(map[string]database.Plugin, len(dbPlugins))
	for _, p := range dbPlugins {
		byName[p.Name] = p
	}

	plugins := make([]PluginInfo, 0, len(files))
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileInfo, err := file.Info()
		if err != nil {
			continue
		}
		name := file.Name()
		seen[name] = true

		row, ok := byName[name]
		if !ok {
			// 未入库（历史文件 / 手工拷入）：补录哈希与平台，之后走 DB
			row = backfillPluginMeta(plugginDir, name, fileInfo)
		}
		plugins = append(plugins, PluginInfo{
			Name:         name,
			Size:         fileInfo.Size(),
			LastModified: fileInfo.ModTime(),
			Type:         pluginKindFromName(name),
			Description:  fmt.Sprintf("OS:%s Hash:%s", displayOS(name, row.Os), shortHash(row.FileHash)),
		})
	}

	// DB 有行但文件已不在：不再显示幽灵条目，仅告警
	for name := range byName {
		if !seen[name] {
			logx.Warnf("[PLUGIN-API] ⚠️ 插件元数据无对应文件，已从列表隐藏: %s\n", name)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "success", "data": plugins})
}

// GetPluginDetails 获取插件详细信息
type GetPluginDetailsRequest struct {
	PluginName string `json:"plugin_name"`
}

type GetPluginDetailsResponse struct {
	Status  int        `json:"status"`
	Message string     `json:"message"`
	Data    PluginInfo `json:"data"`
}

func GetPluginDetails(c *gin.Context) {
	// 获取插件名称
	pluginName := c.Param("pluginName")
	if err := errors.ValidatePluginName(pluginName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": err.Error()})
		return
	}

	// 获取插件目录
	pluginDir := config.GetPluginDir()

	// 获取文件信息
	filePath := filepath.Join(pluginDir, pluginName)
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "message": "插件不存在"})
		return
	}

	// 构建插件信息（类型推断与 /plugin/list 共用一套，避免两处口径漂移）
	pluginInfo := PluginInfo{
		Name:         pluginName,
		Size:         fileInfo.Size(),
		LastModified: fileInfo.ModTime(),
		Type:         pluginKindFromName(pluginName),
	}

	// 与 /plugin/list 同口径：补上 DB 里的平台与哈希
	var dbPlugin database.Plugin
	if has, _ := database.Engine.Where("name = ?", pluginName).Get(&dbPlugin); has {
		pluginInfo.Description = fmt.Sprintf("OS:%s Hash:%s",
			displayOS(pluginName, dbPlugin.Os), shortHash(dbPlugin.FileHash))
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "success", "data": pluginInfo})
}

// DownloadPlugin 下载插件文件
func DownloadPlugin(c *gin.Context) {
	// 获取插件名称
	plugginName := c.Param("pluginName")
	defer func() { auditPluginOp(c, "PLUGIN_DOWNLOAD", plugginName) }()
	if err := errors.ValidatePluginName(plugginName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": err.Error()})
		return
	}

	// 获取插件目录
	plugginDir := config.GetPluginDir()

	// 获取文件路径
	filePath := filepath.Join(plugginDir, plugginName)
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "message": "插件不存在"})
		return
	}

	// 打开一次，校验与发送共用同一个句柄：原实现先 os.Open 校验、再 c.File 二次打开，
	// 两次打开之间存在被替换的窗口；且 os.Open 失败时 `if err == nil` 会让校验被静默跳过。
	f, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "打开插件文件失败: " + err.Error()})
		return
	}
	defer f.Close()
	fileSize := fileInfo.Size() // 用磁盘真实大小：DB 无行时 FileSize 为 0，会发出空 body

	// P5: 下载前校验文件完整性（与 DB 存储的 SHA256 比对，防篡改）
	var dbPlugin database.Plugin
	has, dbErr := database.Engine.Where("name = ?", plugginName).Get(&dbPlugin)
	if dbErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "读取插件元数据失败: " + dbErr.Error()})
		return
	}
	if !has || dbPlugin.FileHash == "" {
		// 未走过 /plugin/list 的历史文件：没有基准哈希可比，放行但留痕
		logx.Warnf("[PLUGIN-DOWNLOAD] ⚠️ 插件缺少完整性元数据，跳过校验: %s\n", plugginName)
	} else {
		hasher := sha256.New()
		if _, err := io.Copy(hasher, f); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "计算哈希失败: " + err.Error()})
			return
		}
		if hex.EncodeToString(hasher.Sum(nil)) != dbPlugin.FileHash {
			logx.Warnf("[PLUGIN-DOWNLOAD] ⚠️ 完整性校验失败（疑似被篡改）: %s\n", plugginName)
			c.JSON(http.StatusForbidden, gin.H{"status": 403, "message": "插件文件完整性校验失败(可能被篡改)，拒绝下载"})
			return
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "读取插件文件失败: " + err.Error()})
			return
		}
	}

	// 设置防缓存响应头 - 确保每次都下载最新文件
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	c.Header("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	c.Header("ETag", fmt.Sprintf("\"%d\"", time.Now().UnixNano()))

	// 设置响应头
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", "attachment; filename="+plugginName)
	c.Header("Content-Type", "application/octet-stream")

	// 返回文件（复用已校验的句柄，避免二次打开）
	logx.Infof("[PLUGIN-DOWNLOAD] ✅ 插件下载成功: %s\n", plugginName)
	c.DataFromReader(http.StatusOK, fileSize, "application/octet-stream", f, nil)
}

// UploadPlugin 上传插件
type UploadPluginResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    struct {
		FileName string `json:"file_name"`
	} `json:"data,omitempty"`
}

func UploadPlugin(c *gin.Context) {
	// 审计用的文件名：解析 multipart 之前还不知道，defer 读的是最终值
	auditName := ""
	defer func() { auditPluginOp(c, "PLUGIN_UPLOAD", auditName) }()
	file, err := c.FormFile("plugin")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "获取上传文件失败: " + err.Error()})
		return
	}
	// P5: 文件大小限制 (50MB)
	const maxFileSize = 50 * 1024 * 1024
	if file.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": fmt.Sprintf("文件过大: %d bytes (限制 %d bytes)", file.Size, maxFileSize)})
		return
	}
	filename := file.Filename
	auditName = filename
	if err := errors.ValidatePluginName(filename); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "文件名非法: " + err.Error()})
		return
	}
	if err := errors.ValidatePluginExtension(filename); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": err.Error()})
		return
	}
	pluginDir := config.GetPluginDir()
	dst := filepath.Join(pluginDir, filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "保存文件失败: " + err.Error()})
		return
	}

	// P5: 计算 SHA256 完整性校验（下载时以此为基准比对）
	fileHash, err := fileSHA256(dst)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "计算哈希失败: " + err.Error()})
		return
	}
	fileSize := file.Size

	// Os/Type 由调用方提供时以调用方为准；前端目前不传，Os 按扩展名推断
	// （database.Plugin.Os 的口径就是 windows/linux/darwin）。
	// Type 列是执行模式（execute-assembly/inline-bin/...），推断不出来就不猜。
	osType := c.PostForm("os")
	if osType == "" {
		osType = pluginOSFromName(filename)
	}
	pluginType := c.PostForm("type")

	// P1: 写入 DB（如已存在同名则更新）
	var existing database.Plugin
	has, _ := database.Engine.Where("name = ?", filename).Get(&existing)
	plugin := &database.Plugin{
		Name:       filename,
		Os:         osType,
		Type:       pluginType,
		FileName:   filename,
		FileHash:   fileHash,
		FileSize:   fileSize,
		UploadTime: time.Now(),
	}
	if has {
		plugin.Id = existing.Id
		database.Engine.ID(existing.Id).Update(plugin)
	} else {
		database.Engine.Insert(plugin)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  200,
		"message": "success",
		"data": gin.H{
			"file_name": filename,
			"hash":      fileHash,
			"size":      fileSize,
		},
	})
}

// DeletePlugin 删除插件（DB + 文件）
func DeletePlugin(c *gin.Context) {
	pluginName := c.Param("pluginName")
	defer func() { auditPluginOp(c, "PLUGIN_DELETE", pluginName) }()
	if err := errors.ValidatePluginName(pluginName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": err.Error()})
		return
	}
	// 删 DB
	database.Engine.Where("name = ?", pluginName).Delete(&database.Plugin{})
	// 删文件
	pluginPath := filepath.Join(config.GetPluginDir(), pluginName)
	os.Remove(pluginPath)
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "删除成功"})
}

// GetOnlineClients 获取在线客户端
type GetOnlineClientsResponse struct {
	Status  int          `json:"status"`
	Message string       `json:"message"`
	Data    []ClientInfo `json:"data"`
}

// ClientInfo 客户端信息结构体
type ClientInfo struct {
	Uid        string `json:"uid"` // 添加缺失的Uid字段
	Computer   string `json:"computer"`
	ExternalIP string `json:"external_ip"`
	Username   string `json:"username"`
	Online     string `json:"online"`
	Listener   string `json:"listener"`
}

func GetOnlineClients(c *gin.Context) {
	logx.Infof("[PLUGIN-API] 🔍 获取在线客户端列表\n")

	// 从数据库获取在线客户端
	onlineClients, err := database.GetOnlineClients()
	if err != nil {
		logx.Infof("[PLUGIN-API] ❌ 获取在线客户端失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "获取在线客户端失败: " + err.Error()})
		return
	}

	logx.Infof("[PLUGIN-API] 📱 找到 %d 个在线客户端\n", len(onlineClients))

	// 转换数据格式
	var clientInfos []ClientInfo
	for _, client := range onlineClients {
		clientInfos = append(clientInfos, ClientInfo{
			Uid:        client.Uid, // 确保包含Uid字段
			Computer:   client.Computer,
			ExternalIP: client.ExternalIP,
			Username:   client.Username,
			Online:     client.Online,
			Listener:   client.Listener,
		})
		// D45: UID 不足 8 位时避免切片越界
		shortUid := client.Uid
		if len(client.Uid) > 8 {
			shortUid = client.Uid[:8]
		}
		logx.Infof("[PLUGIN-API] 👤 客户端: %s (%s) - %s [%s]\n",
			shortUid, client.Computer, client.ExternalIP, client.Listener)
	}

	logx.Infof("[PLUGIN-API] ✅ 成功返回 %d 个客户端信息\n", len(clientInfos))
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "success", "data": clientInfos})
}
