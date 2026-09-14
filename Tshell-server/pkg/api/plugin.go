package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/utils/logx"
)

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

	// 优先从 DB 查询（有元数据：Os/Type/Hash）
	var dbPlugins []database.Plugin
	_ = database.Engine.Find(&dbPlugins)
	if len(dbPlugins) > 0 {
		var plugins []PluginInfo
		for _, p := range dbPlugins {
			hash := p.FileHash
			if len(hash) > 12 {
				hash = hash[:12]
			}
			plugins = append(plugins, PluginInfo{
				Name:         p.FileName,
				Size:         p.FileSize,
				LastModified: p.UploadTime,
				Type:         p.Type,
				Description:  fmt.Sprintf("OS:%s Hash:%s", p.Os, hash),
			})
		}
		c.JSON(http.StatusOK, gin.H{"status": 200, "message": "success", "data": plugins})
		return
	}

	// DB 为空时回退到文件系统扫描（兼容已有文件）
	files, err := os.ReadDir(plugginDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "无法读取插件目录: " + err.Error()})
		return
	}
	var plugins []PluginInfo
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileInfo, err := file.Info()
		if err != nil {
			continue
		}
		ext := filepath.Ext(file.Name())
		fileType := "unknown"
		switch ext {
		case ".exe":
			fileType = "exe"
		case ".dll":
			fileType = "dll"
		case ".so":
			fileType = "so"
		case ".bin":
			fileType = "bin"
		case ".sh":
			fileType = "sh"
		}
		plugins = append(plugins, PluginInfo{
			Name:         file.Name(),
			Size:         fileInfo.Size(),
			LastModified: fileInfo.ModTime(),
			Type:         fileType,
		})
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

	// 获取文件类型
	fileType := "unknown"
	ext := filepath.Ext(pluginName)

	switch ext {
	case ".exe":
		fileType = "exe"
	case ".dll":
		fileType = "dll"
	case ".so":
		fileType = "so"
	case ".py":
		fileType = "py"
	case ".sh":
		fileType = "sh"
	}

	// 构建插件信息
	pluginInfo := PluginInfo{
		Name:         pluginName,
		Size:         fileInfo.Size(),
		LastModified: fileInfo.ModTime(),
		Type:         fileType,
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "success", "data": pluginInfo})
}

// DownloadPlugin 下载插件文件
func DownloadPlugin(c *gin.Context) {
	// 获取插件名称
	plugginName := c.Param("pluginName")
	if err := errors.ValidatePluginName(plugginName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": err.Error()})
		return
	}

	// 获取插件目录
	plugginDir := config.GetPluginDir()

	// 获取文件路径
	filePath := filepath.Join(plugginDir, plugginName)
	_, err := os.Stat(filePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "message": "插件不存在"})
		return
	}

	// P5: 下载前校验文件完整性（与 DB 存储的 SHA256 比对，防篡改）
	var dbPlugin database.Plugin
	has, _ := database.Engine.Where("name = ?", plugginName).Get(&dbPlugin)
	if has && dbPlugin.FileHash != "" {
		f, err := os.Open(filePath)
		if err == nil {
			hasher := sha256.New()
			io.Copy(hasher, f)
			f.Close()
			actualHash := hex.EncodeToString(hasher.Sum(nil))
			if actualHash != dbPlugin.FileHash {
				c.JSON(http.StatusForbidden, gin.H{"status": 403, "message": "插件文件完整性校验失败(可能被篡改)，拒绝下载"})
				return
			}
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

	// 返回文件
	logx.Infof("[PLUGIN-DOWNLOAD] ✅ 插件下载成功: %s\n", plugginName)
	c.File(filePath)
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
	if err := errors.ValidatePluginName(filename); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "文件名非法: " + err.Error()})
		return
	}
	pluginDir := config.GetPluginDir()
	dst := filepath.Join(pluginDir, filename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "保存文件失败: " + err.Error()})
		return
	}

	// P5: 计算 SHA256 完整性校验
	f, err := os.Open(dst)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "打开文件失败: " + err.Error()})
		return
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": "计算哈希失败: " + err.Error()})
		return
	}
	fileHash := hex.EncodeToString(hasher.Sum(nil))
	fileSize := file.Size

	// 获取可选的 Os/Type 参数（前端可传，默认空）
	osType := c.PostForm("os")
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
