package api

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// HostFileDelivery 文件托管(P2-11): 上传任意文件经 Delivery Hub 对外分发,
// 复用 /d/{key}/full 通道与其全部访问控制(AccessToken/UA 白名单/一次性/
// 配额/访问日志)。与 stager 类投递共用 SitesView 列表与生命周期管理
// (close/open/delete 均走现有 WebDelivery 接口)。
//
// 请求(multipart): file + contentType(可选,默认按扩展名推断) + uriPath(可选伪装)
// + oneTime(可选 "1") + maxDownloads(可选) + note(可选)
// 响应: {url, publicKey}
func HostFileDelivery(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少文件: " + err.Error()})
		return
	}
	// Payload 常驻 Hub 内存,限制单文件 50MB
	const maxHostFileSize = 50 * 1024 * 1024
	if file.Size <= 0 || file.Size > maxHostFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("文件大小非法: %d(上限 50MB,Hub 载荷常驻内存)", file.Size)})
		return
	}

	// 文件名清洗: 仅保留基名,禁止路径分隔符(防 ../ 与路径注入)
	fileName := filepath.Base(strings.ReplaceAll(file.Filename, "\\", "/"))
	if fileName == "" || fileName == "." || fileName == ".." || strings.ContainsAny(fileName, "/\\:\x00") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名非法"})
		return
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "打开文件失败"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取文件失败: " + err.Error()})
		return
	}

	// ContentType: 用户指定优先,否则按扩展名推断,兜底二进制流
	contentType := strings.TrimSpace(c.PostForm("contentType"))
	if contentType == "" {
		contentType = mimeByExt(fileName)
	}

	// 构造 WebDelivery 记录: shared 模式挂主端口 /d/ 命名空间
	meta := database.WebDelivery{
		ListenerConfig: "host-file", // 标识来源(非监听器投递)
		OS:             "any",
		Arch:           "any",
		ListeningPort:  "shared",
		Status:         1,
		FileName:       fileName,
		ContentType:    contentType,
		DownloadPath:   "host-" + delivery.RandomKey(4) + "-" + fileName, // SitesView 列表显示名
		Note:           strings.TrimSpace(c.PostForm("note")),
	}
	if c.PostForm("oneTime") == "1" {
		meta.OneTime = 1
	}
	if n := strings.TrimSpace(c.PostForm("maxDownloads")); n != "" {
		var max int
		if _, err := fmt.Sscanf(n, "%d", &max); err == nil && max > 0 {
			meta.MaxDownloads = max
		}
	}
	delivery.EnsureMetaDefaults(&meta)
	if uriPath := strings.TrimSpace(c.PostForm("uriPath")); uriPath != "" {
		meta.URIPath = uriPath
	}

	if _, err := database.Engine.Insert(&meta); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "入库失败: " + err.Error()})
		return
	}

	// 注册 Hub 并生成公开 URL(与 StartWebDelivery 同款流程)
	delivery.GlobalHub.Register(meta, delivery.Payload{Full: data, FileName: fileName})
	delivery.BuildPublicURLs(&meta, delivery.GlobalHub.MainPort())
	database.Engine.ID(meta.ID).Cols("server_address").Update(&meta.ServerAddress)

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategoryAPI, "HOST_FILE",
			"SUCCESS", "托管文件 "+fileName, map[string]interface{}{
				"file_name": fileName, "size": file.Size, "content_type": contentType, "key": meta.PublicKey,
			})
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"url":        meta.ServerAddress,
		"publicKey":  meta.PublicKey,
		"fileName":   fileName,
		"size":       file.Size,
		"oneTime":    meta.OneTime,
		"maxDownloads": meta.MaxDownloads,
	}})
}

// mimeByExt 按扩展名推断常见 MIME(白名单,未知回退二进制流)
func mimeByExt(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".txt", ".log", ".md":
		return "text/plain; charset=utf-8"
	case ".json":
		return "application/json"
	case ".js":
		return "application/javascript"
	case ".css":
		return "text/css"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".exe", ".dll":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}
