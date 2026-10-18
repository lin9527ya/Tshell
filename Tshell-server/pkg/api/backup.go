package api

import (
	"BackendTemplate/pkg/backup"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 数据库备份管理（仅 admin）。
//
// 备份可以在线做（VACUUM INTO 一致性快照）；**恢复不行** —— SQLite 的库就是那个
// 文件，运行中的进程持有它，热替换会把两边都弄坏。因此这里只提供备份的
// 查看/创建/下载/导入/删除，恢复走 CLI（./tshell -restore <备份>），
// 步骤见 docs/BACKUP_RESTORE.md。

// BackupManager 全局备份管理器（main 启动时注入；未启用时为 nil）
var BackupManager *backup.Manager

// SetBackupManager 注入备份管理器
func SetBackupManager(m *backup.Manager) { BackupManager = m }

func backupUnavailable(c *gin.Context) bool {
	if BackupManager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "备份功能未启用（config.backup.enabled）"})
		return true
	}
	return false
}

// ListBackups 备份列表（GET /api/backup/list）
func ListBackups(c *gin.Context) {
	if backupUnavailable(c) {
		return
	}
	list, err := BackupManager.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取备份目录失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"dir":  BackupManager.Options().Dir,
			"keep": BackupManager.Options().Keep,
			"list": list,
		},
	})
}

// CreateBackup 手动触发备份（POST /api/backup/create）
func CreateBackup(c *gin.Context) {
	if backupUnavailable(c) {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body) // body 可选

	note := strings.TrimSpace(body.Note)
	if note == "" {
		note = "manual:" + operatorFromContext(c)
	}
	info, err := BackupManager.Create(note)
	if err != nil {
		auditLogEvent("BACKUP_CREATE", "FAILURE", "手动备份失败: "+err.Error(), nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "备份失败: " + err.Error()})
		return
	}
	removed, _ := BackupManager.Cleanup()
	auditLogEvent("BACKUP_CREATE", "SUCCESS",
		fmt.Sprintf("手动备份 %s (%.1f MB)", info.Name, float64(info.Size)/1024/1024), nil)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": info, "removed": removed})
}

// DownloadBackup 下载备份文件（GET /api/backup/download/:name）
// 用途：把备份取到本地归档，或送到另一台服务端用 -restore 恢复。
func DownloadBackup(c *gin.Context) {
	if backupUnavailable(c) {
		return
	}
	f, info, err := BackupManager.Open(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "备份不存在: " + err.Error()})
		return
	}
	defer f.Close()

	auditLogEvent("BACKUP_DOWNLOAD", "SUCCESS", "下载备份 "+info.Name, nil)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, info.Name))
	c.Header("Content-Type", "application/octet-stream")
	c.DataFromReader(http.StatusOK, info.Size, "application/octet-stream", f, nil)
}

// DeleteBackup 删除备份（DELETE /api/backup/:name）
func DeleteBackup(c *gin.Context) {
	if backupUnavailable(c) {
		return
	}
	name := c.Param("name")
	if err := BackupManager.Delete(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "删除失败: " + err.Error()})
		return
	}
	auditLogEvent("BACKUP_DELETE", "SUCCESS", "删除备份 "+name, nil)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

// ImportBackup 导入外部备份（POST /api/backup/import）
// 先校验再落盘：坏文件不占位置，也避免运维以为"备份已经在服务器上了"。
func ImportBackup(c *gin.Context) {
	if backupUnavailable(c) {
		return
	}
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		msg := "缺少备份文件"
		if err != nil {
			msg = "读取上传失败: " + err.Error()
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if file.Size > MaxBackupImportBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("备份过大: %d bytes（上限 %d bytes）", file.Size, MaxBackupImportBytes)})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "打开上传文件失败"})
		return
	}
	defer src.Close()
	content := make([]byte, 0, file.Size)
	buf := make([]byte, 64*1024)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			content = append(content, buf[:n]...)
		}
		if rerr != nil {
			break
		}
	}

	// 文件名只取基名并限定后缀，避免用上传名做路径
	name := sanitizeBackupName(file.Filename, BackupManager.Options().Prefix)
	info, err := BackupManager.Import(name, content)
	if err != nil {
		auditLogEvent("BACKUP_IMPORT", "FAILURE", "导入备份失败: "+err.Error(), nil)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditLogEvent("BACKUP_IMPORT", "SUCCESS", "导入备份 "+info.Name, nil)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": info})
}

// sanitizeBackupName 规整上传的备份名：只保留基名，统一后缀与前缀
func sanitizeBackupName(raw, prefix string) string {
	name := strings.TrimSpace(raw)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, name)
	// 上传名被规整成空（或只剩分隔符）时用时间戳兜底，避免落出 "tshell-backup-.db" 这种名字
	if strings.Trim(name, "._-") == "" {
		name = time.Now().Format("20060102-150405")
	}
	if !strings.HasPrefix(name, prefix) {
		name = prefix + "-" + name
	}
	if !strings.HasSuffix(name, ".db") {
		name += ".db"
	}
	if len(name) > 128 {
		name = name[len(name)-128:]
	}
	return name
}
