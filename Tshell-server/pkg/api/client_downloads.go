package api

import (
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"fmt"
	"log"

	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ===== client_downloads(拆分自原 clients.go) =====

// resolveDownloadDir 将客户端 UID 映射为下载存储目录名（客户端数字 ID）。
// 与 downloadFiles.go 的存储目录规则一致：目录用 Clients.ID（数字），便于识别与归档。
func resolveDownloadDir(uid string) string {
	if uid == "" {
		return uid
	}
	dlClient, has, err := database.Clients_().GetByUID(uid)
	if err == nil && has && dlClient.ID > 0 {
		return strconv.FormatInt(dlClient.ID, 10)
	}
	return uid
}

func DownloadFile(c *gin.Context) {
	var fileBody struct {
		Uid      string `json:"uid"`
		FilePath string `json:"filePath"`
	}
	if err := c.ShouldBindJSON(&fileBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(fileBody.FilePath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}

	// H4: Uid 严格校验（原未净化 → ../../ 目录穿越创建任意目录）
	if err := errors.ValidateUID(fileBody.Uid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 下载目录使用客户端数字 ID（便于识别与归档）
	dirName := fileBody.Uid
	if dlClient, has, err := database.Clients_().GetByUID(fileBody.Uid); err == nil && has && dlClient.ID > 0 {
		dirName = strconv.FormatInt(dlClient.ID, 10)
	}
	downloadsRoot := config.GetDownloadsDir()
	if _, statErr := os.Stat(filepath.Join(downloadsRoot, dirName)); os.IsNotExist(statErr) {
		// 文件夹不存在，创建文件夹
		// 0700：目录里是客户端下载回来的文件（可能是凭据/配置等敏感内容），
		// 原实现用 os.ModePerm(0777)，同机任何本地用户都能读走
		if mkErr := security.SecureMkdirAll(filepath.Join(downloadsRoot, dirName)); mkErr != nil {
			log.Printf("创建下载目录失败: %v", mkErr)
		}
	}

	exist, _ := database.DownloadsStore_().ExistsByPath(fileBody.Uid, fileBody.FilePath)
	if !exist {
		_, _ = database.DownloadsStore_().Insert(&database.Downloads{Uid: fileBody.Uid, FileName: filepath.Base(fileBody.FilePath), FilePath: fileBody.FilePath})
	} else {
		sql := `
UPDATE downloads
SET file_size = ?, downloaded_size = ?
WHERE uid = ? AND file_path = ?;
`
		database.QueryStringSQL(sql, 0, 0, fileBody.Uid, fileBody.FilePath)
	}
	// 任务一等对象：下载此前不落任务，任务中心看不到"谁在拖哪个文件"，
	// 前端只能靠轮询字节数猜完成（超时就报错）。这里建任务，终态由
	// command.DownloadEndFunc → OnDownloadEnd 收敛（写满声明大小=completed，
	// 客户端断线/空闲超时=failed）。
	downloadCmd := "download " + fileBody.FilePath
	var taskID int64
	if _, rows := createTaskRows([]string{fileBody.Uid}, operatorFromContext(c), "download", downloadCmd); len(rows) > 0 {
		taskID = rows[0].Id
	}
	if err := sendcommand.SendCommandChecked(fileBody.Uid, downloadCmd); err != nil {
		finishTask(taskID, taskStatusFailed, "", err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": 503, "error": "下载指令下发失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "task_id": taskID})
}

type DownloadsInfo struct {
	FileName       string `json:"fileName"`
	FilePath       string `json:"filePath"`
	FileSize       string `json:"fileSize"`
	DownloadedPart string `json:"downloadPart"`
}

func GetDownloadsInfo(c *gin.Context) {
	var downloadBody struct {
		Uid string `form:"uid"`
	}
	if err := c.BindQuery(&downloadBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	var downloads []database.Downloads
	downloads, _ = database.DownloadsStore_().ListByUID(downloadBody.Uid)
	var result []DownloadsInfo
	for _, download := range downloads {
		var tmpDownloadsInfo DownloadsInfo
		tmpDownloadsInfo.FileName = download.FileName
		tmpDownloadsInfo.FilePath = download.FilePath
		tmpDownloadsInfo.FileSize = utils.BytesToSize(strconv.Itoa(download.FileSize))
		if download.FileSize != 0 {
			tmpDownloadsInfo.DownloadedPart = strconv.Itoa(download.DownloadedSize * 100 / download.FileSize)
		} else {
			tmpDownloadsInfo.DownloadedPart = "0"
		}

		result = append(result, tmpDownloadsInfo)
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": result})
}

func DownloadDownloadedFile(c *gin.Context) {
	var downloadBody struct {
		Uid      string `json:"uid"`
		FilePath string `json:"filePath"`
	}
	if err := c.BindJSON(&downloadBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// H4: Uid 严格校验（原未净化 → uid=../../ 任意文件读取）
	if err := errors.ValidateUID(downloadBody.Uid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 指定文件的完整路径（目录名使用客户端数字 ID，与 DownloadFile 存储一致）
	fileph := filepath.Join(config.GetDownloadsDir(), resolveDownloadDir(downloadBody.Uid), filepath.Base(downloadBody.FilePath))

	// 获取原始文件名（保持中文字符）
	originalFileName := filepath.Base(downloadBody.FilePath)

	// 设置响应头，支持中文文件名
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	// 使用 RFC 5987 格式支持 UTF-8 编码的文件名
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.QueryEscape(originalFileName)))
	c.Header("Content-Type", "application/octet-stream")

	// 发送文件
	c.File(fileph)
}

func DeleteDownloadedFile(c *gin.Context) {
	var deleteBody struct {
		Uid      string `json:"uid"`
		FilePath string `json:"filePath"`
	}
	if err := c.BindJSON(&deleteBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// H4: Uid 严格校验（原未净化 → 任意文件删除）
	if err := errors.ValidateUID(deleteBody.Uid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 构造服务端文件路径（目录名使用客户端数字 ID，与 DownloadFile 存储一致）
	fileName := filepath.Base(deleteBody.FilePath)
	serverFilePath := filepath.Join(config.GetDownloadsDir(), resolveDownloadDir(deleteBody.Uid), fileName)

	// 删除物理文件
	if err := os.Remove(serverFilePath); err != nil {
		log.Printf("删除文件失败: %v", err)
		// 即使物理文件删除失败，也继续删除数据库记录
	}

	// 删除数据库记录
	affected, err := database.DownloadsStore_().DeleteByPath(deleteBody.Uid, deleteBody.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除数据库记录失败"})
		return
	}

	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到要删除的文件记录"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "文件删除成功"})
}

// ================ 屏幕监控API ================

// StartScreenMonitor 开始屏幕监控
