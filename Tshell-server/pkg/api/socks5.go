package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/proxy"
	"BackendTemplate/pkg/sendcommand"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// 说明：本文件是旧版 SOCKS5 代理接口（/socks5/*，写入 database.Socks5 表）。新实现见
// tunnel_proxy.go 的 /tunnel-proxy/*（database.TunnelProxy 表），二者功能重叠：都能建
// 反向 SOCKS5 隧道、都用「Agent 端口 = 业务端口 + 1000」，端口空间已交叉校验（见
// checkTunnelPortAvailable）。前端目前把旧页面作为隧道页的一个子视图保留；待前端可重建后
// 应移除本文件、/socks5 路由与 Socks5View.vue，统一到 /tunnel-proxy。

// Socks5List 获取SOCKS5代理列表
func Socks5List(c *gin.Context) {
	log.Printf("[SOCKS5-API] 📋 收到获取SOCKS5列表请求")

	// 获取所有SOCKS5代理记录
	var socks5 []database.Socks5
	err := database.Engine.Find(&socks5)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 数据库查询失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库查询失败"})
		return
	}

	// 转换为前端期望的格式
	var tunnelList []map[string]interface{}
	for _, s := range socks5 {
		// 解析agent端口
		agentPort, _ := strconv.Atoi(strings.TrimPrefix(s.ConnectAddress, ":"))
		socks5Port, _ := strconv.Atoi(s.Socks5port)

		tunnel := map[string]interface{}{
			"id":          s.ID,
			"client_uid":  s.UID,
			"protocol":    s.Type,
			"server_port": socks5Port,
			"note":        s.Note,
		"username":    s.UserName,
		"password":    maskSecret(s.Password), // D45: 密码脱敏，避免明文泄露
			"status":      s.Status,
			"created_at":  s.CreatedAt.Format("2006-01-02 15:04:05"),
			"updated_at":  s.UpdatedAt.Format("2006-01-02 15:04:05"),
			"agent_port":  agentPort,
		}
		tunnelList = append(tunnelList, tunnel)
	}

	log.Printf("[SOCKS5-API] ✅ 查询成功，找到 %d 个SOCKS5代理", len(socks5))
	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data":   tunnelList,
		"total":  len(tunnelList),
	})
}

// Socks5Start 启动SOCKS5代理
func Socks5Start(c *gin.Context) {
	log.Printf("[SOCKS5-API] 🚀 收到启动SOCKS5代理请求")

	var socks5Body struct {
		ClientUID  string `json:"client_uid"`
		Protocol   string `json:"protocol"`
		ServerPort int    `json:"server_port"`
		Note       string `json:"note"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}
	if err := c.ShouldBindJSON(&socks5Body); err != nil {
		log.Printf("[SOCKS5-API] ❌ 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证必要参数
	if socks5Body.ClientUID == "" {
		log.Printf("[SOCKS5-API] ❌ 客户端UID不能为空")
		c.JSON(http.StatusBadRequest, gin.H{"error": "客户端UID不能为空"})
		return
	}
	if socks5Body.ServerPort <= 0 || socks5Body.ServerPort > 65535 {
		log.Printf("[SOCKS5-API] ❌ 端口号无效: %d", socks5Body.ServerPort)
		c.JSON(http.StatusBadRequest, gin.H{"error": "端口号必须在1-65535之间"})
		return
	}
	// 无认证的 SOCKS5 端口等同于公开代理，强制要求凭据（与新隧道接口口径一致）
	if err := requireTunnelAuth("socks5", socks5Body.Username, socks5Body.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 与新隧道接口交叉校验端口（含 Agent 端口 +1000）：两套实现各管一张表，
	// 不交叉校验时同一端口会在两张表里各建一条，直到 bind 才撞车
	if err := checkTunnelPortAvailable(socks5Body.ServerPort, 0, 0); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SOCKS5-API] 📋 SOCKS5配置:")
	log.Printf("[SOCKS5-API]    - 客户端UID: %s", socks5Body.ClientUID)
	log.Printf("[SOCKS5-API]    - 协议: %s", socks5Body.Protocol)
	log.Printf("[SOCKS5-API]    - 服务端口: %d", socks5Body.ServerPort)
	log.Printf("[SOCKS5-API]    - 用户名: %s", socks5Body.Username)
	log.Printf("[SOCKS5-API]    - 密码长度: %d", len(socks5Body.Password))

	// 构建监听地址
	agentPort := socks5Body.ServerPort + 1000 // Agent端口 = SOCKS5端口 + 1000
	connectAddress := fmt.Sprintf(":%d", agentPort)
	socksAddress := fmt.Sprintf("0.0.0.0:%d", socks5Body.ServerPort)

	log.Printf("[SOCKS5-API] 🔍 检查Agent监听端口: %d", agentPort)
	inUse, err := isPortInUse(strconv.Itoa(agentPort))
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 检测端口 %d 时发生错误: %v", agentPort, err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "端口检测失败"})
		return
	}
	if inUse {
		log.Printf("[SOCKS5-API] ⚠️ Agent监听端口 %d 被占用", agentPort)
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": fmt.Sprintf("端口%d被占用", agentPort)})
		return
	}

	log.Printf("[SOCKS5-API] 🔍 检查SOCKS5端口: %d", socks5Body.ServerPort)
	inUse, err = isPortInUse(strconv.Itoa(socks5Body.ServerPort))
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 检测端口 %d 时发生错误: %v", socks5Body.ServerPort, err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "端口检测失败"})
		return
	}
	if inUse {
		log.Printf("[SOCKS5-API] ⚠️ SOCKS5端口 %d 被占用", socks5Body.ServerPort)
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": fmt.Sprintf("端口%d被占用", socks5Body.ServerPort)})
		return
	}

	log.Printf("[SOCKS5-API] 💾 保存SOCKS5配置到数据库...")
	socks5Record := &database.Socks5{
		UID:            socks5Body.ClientUID,
		Type:           socks5Body.Protocol,
		ConnectAddress: connectAddress,
		Socks5port:     strconv.Itoa(socks5Body.ServerPort),
		UserName:       socks5Body.Username,
		Password:       socks5Body.Password,
		Note:           socks5Body.Note,
		Status:         1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	_, err = database.Engine.Insert(socks5Record)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 保存数据库失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库保存失败"})
		return
	}

	log.Printf("[SOCKS5-API] 🔧 启动反向SOCKS服务器...")
	randomPSK := proxy.GenerateRandomPSK()
	if _, err := proxy.ReverseSocksServer(socks5Record.ConnectAddress, socksAddress, randomPSK, socks5Record.UserName, socks5Record.Password); err != nil {
		// 同步绑定失败：回置状态，避免"显示运行中、实际没监听"
		database.Engine.ID(socks5Record.ID).Update(&database.Socks5{Status: 0, UpdatedAt: time.Now()})
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": err.Error()})
		return
	}

	// 🔧 修复：直接使用监听器的外网连接地址
	log.Printf("[SOCKS5-API] 🔍 查找客户端对应的监听器外网连接地址...")

	// 查找客户端记录
	var client database.Clients
	has, err := database.Engine.Where("uid = ?", socks5Body.ClientUID).Get(&client)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 查找客户端记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "查找客户端记录失败"})
		return
	}
	if !has {
		log.Printf("[SOCKS5-API] ❌ 客户端记录不存在: %s", socks5Body.ClientUID)
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "客户端记录不存在"})
		return
	}

	// 查找监听器记录，获取外网连接地址
	var listener database.Listener
	has, err = database.Engine.Where("type = ? AND status = ?", client.Listener, 1).Get(&listener)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 查找监听器记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "查找监听器记录失败"})
		return
	}
	if !has {
		log.Printf("[SOCKS5-API] ❌ 监听器记录不存在，协议: %s", client.Listener)
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "监听器记录不存在"})
		return
	}

	// 使用监听器的外网连接地址
	serverIP := listener.ConnectAddress
	if serverIP == "" {
		log.Printf("[SOCKS5-API] ❌ 监听器外网连接地址为空")
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "监听器外网连接地址未配置"})
		return
	}

	// 去除端口部分，只保留IP地址
	if strings.Contains(serverIP, ":") {
		serverIP = strings.Split(serverIP, ":")[0]
	}

	log.Printf("[SOCKS5-API] 🌍 使用监听器外网连接地址: %s", serverIP)

	// 构建完整的Agent连接地址
	fullAgentAddress := fmt.Sprintf("%s%s", serverIP, socks5Record.ConnectAddress)

	log.Printf("[SOCKS5-API] 📤 发送SOCKS5命令到客户端...")
	log.Printf("[SOCKS5-API]    - 客户端UID: %s", socks5Body.ClientUID)
	log.Printf("[SOCKS5-API]    - 连接地址: %s", fullAgentAddress)
	commandStr := fmt.Sprintf("socks5 %s|%s|false", fullAgentAddress, randomPSK)
	sendcommand.SendCommand(socks5Body.ClientUID, commandStr)

	log.Printf("[SOCKS5-API] ✅ SOCKS5代理启动成功")
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "SOCKS5代理启动成功", "data": map[string]interface{}{
		"id":          socks5Record.ID,
		"agent_port":  agentPort,
		"socks5_port": socks5Body.ServerPort,
	}})
}

// Socks5Open 开启SOCKS5代理
func Socks5Open(c *gin.Context) {
	log.Printf("[SOCKS5-API] 🔄 收到开启SOCKS5代理请求")

	var requestBody struct {
		ID int `json:"id"`
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		log.Printf("[SOCKS5-API] ❌ 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SOCKS5-API] 🔍 查找代理记录 ID: %d", requestBody.ID)

	// 查找代理记录
	var socks5Record database.Socks5
	has, err := database.Engine.ID(requestBody.ID).Get(&socks5Record)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 数据库查询失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库查询失败"})
		return
	}
	if !has {
		log.Printf("[SOCKS5-API] ❌ 代理记录不存在 ID: %d", requestBody.ID)
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "data": "代理记录不存在"})
		return
	}
	// 历史记录可能没有凭据：重新开启时同样拦住
	if err := requireTunnelAuth("socks5", socks5Record.UserName, socks5Record.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SOCKS5-API] 📋 重新开启SOCKS5配置:")
	log.Printf("[SOCKS5-API]    - 客户端UID: %s", socks5Record.UID)
	log.Printf("[SOCKS5-API]    - 连接地址: %s", socks5Record.ConnectAddress)
	log.Printf("[SOCKS5-API]    - SOCKS5端口: %s", socks5Record.Socks5port)

	// 解析端口
	listenPort := strings.TrimPrefix(socks5Record.ConnectAddress, ":")
	socksAddress := "0.0.0.0:" + socks5Record.Socks5port

	// 检查端口可用性
	inUse, err := isPortInUse(listenPort)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 检测端口 %s 时发生错误: %v", listenPort, err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "端口检测失败"})
		return
	}
	if inUse {
		log.Printf("[SOCKS5-API] ⚠️ 端口 %s 被占用", listenPort)
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": listenPort + "端口被占用"})
		return
	}

	inUse, err = isPortInUse(socks5Record.Socks5port)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 检测端口 %s 时发生错误: %v", socks5Record.Socks5port, err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "端口检测失败"})
		return
	}
	if inUse {
		log.Printf("[SOCKS5-API] ⚠️ 端口 %s 被占用", socks5Record.Socks5port)
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": socks5Record.Socks5port + "端口被占用"})
		return
	}

	log.Printf("[SOCKS5-API] 💾 更新数据库状态为开启...")
	affected, err := database.Engine.ID(requestBody.ID).Update(&database.Socks5{Status: 1, UpdatedAt: time.Now()})
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 更新数据库失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库更新失败"})
		return
	}
	log.Printf("[SOCKS5-API] 📊 数据库更新影响行数: %d", affected)

	log.Printf("[SOCKS5-API] 🔧 启动反向SOCKS服务器...")
	randomPSK := proxy.GenerateRandomPSK()
	if _, err := proxy.ReverseSocksServer(socks5Record.ConnectAddress, socksAddress, randomPSK, socks5Record.UserName, socks5Record.Password); err != nil {
		// 同步绑定失败：回置状态，避免"显示运行中、实际没监听"
		database.Engine.ID(socks5Record.ID).Update(&database.Socks5{Status: 0, UpdatedAt: time.Now()})
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": err.Error()})
		return
	}

	// 🔧 修复：直接使用监听器的外网连接地址
	log.Printf("[SOCKS5-API] 🔍 查找客户端对应的监听器外网连接地址...")

	// 查找客户端记录
	var client database.Clients
	has, err = database.Engine.Where("uid = ?", socks5Record.UID).Get(&client)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 查找客户端记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "查找客户端记录失败"})
		return
	}
	if !has {
		log.Printf("[SOCKS5-API] ❌ 客户端记录不存在: %s", socks5Record.UID)
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "客户端记录不存在"})
		return
	}

	// 查找监听器记录，获取外网连接地址
	var listener database.Listener
	has, err = database.Engine.Where("type = ? AND status = ?", client.Listener, 1).Get(&listener)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 查找监听器记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "查找监听器记录失败"})
		return
	}
	if !has {
		log.Printf("[SOCKS5-API] ❌ 监听器记录不存在，协议: %s", client.Listener)
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "监听器记录不存在"})
		return
	}

	// 使用监听器的外网连接地址
	serverIP := listener.ConnectAddress
	if serverIP == "" {
		log.Printf("[SOCKS5-API] ❌ 监听器外网连接地址为空")
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "监听器外网连接地址未配置"})
		return
	}

	// 去除端口部分，只保留IP地址
	if strings.Contains(serverIP, ":") {
		serverIP = strings.Split(serverIP, ":")[0]
	}

	log.Printf("[SOCKS5-API] 🌍 使用监听器外网连接地址: %s", serverIP)

	// 构建完整的Agent连接地址
	fullAgentAddress := fmt.Sprintf("%s%s", serverIP, socks5Record.ConnectAddress)

	log.Printf("[SOCKS5-API] 📤 发送SOCKS5命令到客户端...")
	log.Printf("[SOCKS5-API]    - 客户端UID: %s", socks5Record.UID)
	log.Printf("[SOCKS5-API]    - 连接地址: %s", fullAgentAddress)
	commandStr := fmt.Sprintf("socks5 %s|%s|false", fullAgentAddress, randomPSK)
	sendcommand.SendCommand(socks5Record.UID, commandStr)

	log.Printf("[SOCKS5-API] ✅ SOCKS5代理重新开启成功")
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "SOCKS5代理重新开启成功"})
}

// Socks5Close 关闭SOCKS5代理
func Socks5Close(c *gin.Context) {
	log.Printf("[SOCKS5-API] 🛑 收到关闭SOCKS5代理请求")

	var requestBody struct {
		ID int `json:"id"`
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		log.Printf("[SOCKS5-API] ❌ 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SOCKS5-API] 🔍 查找代理记录 ID: %d", requestBody.ID)

	// 查找代理记录
	var socks5Record database.Socks5
	has, err := database.Engine.ID(requestBody.ID).Get(&socks5Record)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 数据库查询失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库查询失败"})
		return
	}
	if !has {
		log.Printf("[SOCKS5-API] ❌ 代理记录不存在 ID: %d", requestBody.ID)
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "data": "代理记录不存在"})
		return
	}

	log.Printf("[SOCKS5-API] 📋 关闭SOCKS5配置:")
	log.Printf("[SOCKS5-API]    - 客户端UID: %s", socks5Record.UID)
	log.Printf("[SOCKS5-API]    - 连接地址: %s", socks5Record.ConnectAddress)
	log.Printf("[SOCKS5-API]    - SOCKS5端口: %s", socks5Record.Socks5port)

	// 安全解析端口
	var listenPort string
	if strings.Contains(socks5Record.ConnectAddress, ":") {
		parts := strings.Split(socks5Record.ConnectAddress, ":")
		if len(parts) >= 2 {
			listenPort = parts[1]
		}
	}
	if listenPort == "" {
		log.Printf("[SOCKS5-API] ❌ 无法解析监听端口: %s", socks5Record.ConnectAddress)
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法解析监听端口"})
		return
	}

	log.Printf("[SOCKS5-API] 💾 更新数据库状态为关闭...")
	affected, err := database.Engine.ID(requestBody.ID).Update(&database.Socks5{Status: 2, UpdatedAt: time.Now()})
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 更新数据库失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库更新失败"})
		return
	}
	log.Printf("[SOCKS5-API] 📊 数据库更新影响行数: %d", affected)

	// 关闭Agent监听器（加锁取出并关闭，避免并发 map 竞争）
	if proxy.CloseSocks5Server(":" + listenPort) {
		log.Printf("[SOCKS5-API] 🔄 已关闭Agent监听器: :%s", listenPort)
	}

	// 关闭SOCKS5监听器
	if proxy.CloseSocks5Server("0.0.0.0:" + socks5Record.Socks5port) {
		log.Printf("[SOCKS5-API] 🔄 已关闭SOCKS5监听器: 0.0.0.0:%s", socks5Record.Socks5port)
	}

	log.Printf("[SOCKS5-API] 📤 发送关闭命令到客户端...")
	sendcommand.SendCommand(socks5Record.UID, "socks5Close")

	log.Printf("[SOCKS5-API] ✅ SOCKS5代理关闭成功")
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "SOCKS5代理关闭成功"})
}

// Socks5Delete 删除SOCKS5代理
func Socks5Delete(c *gin.Context) {
	log.Printf("[SOCKS5-API] 🗑️ 收到删除SOCKS5代理请求")

	var requestBody struct {
		ID int `json:"id"`
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		log.Printf("[SOCKS5-API] ❌ 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SOCKS5-API] 🔍 查找代理记录 ID: %d", requestBody.ID)

	// 查找代理记录
	var socks5Record database.Socks5
	has, err := database.Engine.ID(requestBody.ID).Get(&socks5Record)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 数据库查询失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库查询失败"})
		return
	}
	if !has {
		log.Printf("[SOCKS5-API] ❌ 代理记录不存在 ID: %d", requestBody.ID)
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "data": "代理记录不存在"})
		return
	}

	log.Printf("[SOCKS5-API] 📋 删除SOCKS5配置:")
	log.Printf("[SOCKS5-API]    - 客户端UID: %s", socks5Record.UID)
	log.Printf("[SOCKS5-API]    - 连接地址: %s", socks5Record.ConnectAddress)
	log.Printf("[SOCKS5-API]    - SOCKS5端口: %s", socks5Record.Socks5port)

	// 安全解析端口
	var listenPort string
	if strings.Contains(socks5Record.ConnectAddress, ":") {
		parts := strings.Split(socks5Record.ConnectAddress, ":")
		if len(parts) >= 2 {
			listenPort = parts[1]
		}
	}
	if listenPort == "" {
		log.Printf("[SOCKS5-API] ❌ 无法解析监听端口: %s", socks5Record.ConnectAddress)
		// 删除操作继续进行，只是跳过端口关闭
	} else {
		// 关闭所有相关监听器（类似Socks5Close的逻辑）
		if proxy.CloseSocks5Server(":" + listenPort) {
			log.Printf("[SOCKS5-API] 🔄 已关闭Agent监听器: :%s", listenPort)
		}

		if proxy.CloseSocks5Server("0.0.0.0:" + socks5Record.Socks5port) {
			log.Printf("[SOCKS5-API] 🔄 已关闭SOCKS5监听器: 0.0.0.0:%s", socks5Record.Socks5port)
		}

		log.Printf("[SOCKS5-API] 📤 发送关闭命令到客户端...")
		sendcommand.SendCommand(socks5Record.UID, "socks5Close")
	}

	log.Printf("[SOCKS5-API] 🗑️ 从数据库删除记录...")
	affected, err := database.Engine.ID(requestBody.ID).Delete(&database.Socks5{})
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 删除数据库记录失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库删除失败"})
		return
	}
	log.Printf("[SOCKS5-API] 📊 数据库删除影响行数: %d", affected)

	log.Printf("[SOCKS5-API] ✅ SOCKS5代理删除成功")
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "SOCKS5代理删除成功"})
}

// Socks5Update 更新SOCKS5代理信息（支持全字段更新）
func Socks5Update(c *gin.Context) {
	log.Printf("[SOCKS5-API] ✏️ 收到更新SOCKS5代理请求")

	var requestBody struct {
		ID         int    `json:"id"`
		ClientUID  string `json:"client_uid"`
		Protocol   string `json:"protocol"`
		ServerPort int    `json:"server_port"`
		Note       string `json:"note"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		log.Printf("[SOCKS5-API] ❌ 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SOCKS5-API] 🔍 更新代理记录 ID: %d", requestBody.ID)

	// 查找代理记录
	var socks5Record database.Socks5
	has, err := database.Engine.ID(requestBody.ID).Get(&socks5Record)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 数据库查询失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "error": "数据库查询失败"})
		return
	}
	if !has {
		log.Printf("[SOCKS5-API] ❌ 代理记录不存在 ID: %d", requestBody.ID)
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "error": "代理记录不存在"})
		return
	}

	// 校验合并后的有效凭据（留空即保留旧值）
	effUser, effPass := socks5Record.UserName, socks5Record.Password
	if requestBody.Username != "" {
		effUser = requestBody.Username
	}
	if requestBody.Password != "" {
		effPass = requestBody.Password
	}
	if err := requireTunnelAuth("socks5", effUser, effPass); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 与另一张表交叉校验端口（含 Agent 端口 +1000），排除自身记录
	updatePort := requestBody.ServerPort
	if updatePort == 0 {
		updatePort, _ = strconv.Atoi(socks5Record.Socks5port)
	}
	if err := checkTunnelPortAvailable(updatePort, 0, int64(requestBody.ID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 【修复】检查端口变更：与旧端口比较（原逻辑误用记录 ID 比较，端口冲突检查失效）
	oldPort, _ := strconv.Atoi(socks5Record.Socks5port)
	if requestBody.ServerPort != 0 && requestBody.ServerPort != oldPort {
		// 检查新端口是否被占用（含 agent 端口 +1000）
		var existingRecord database.Socks5
		exists, err := database.Engine.Where("socks5port = ? AND id != ?", strconv.Itoa(requestBody.ServerPort), requestBody.ID).Get(&existingRecord)
		if err != nil {
			log.Printf("[SOCKS5-API] ❌ 检查端口占用失败: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "error": "检查端口占用失败"})
			return
		}
		if exists {
			log.Printf("[SOCKS5-API] ❌ 端口 %d 已被占用", requestBody.ServerPort)
			c.JSON(http.StatusBadRequest, gin.H{"status": 400, "error": fmt.Sprintf("端口%d已被占用", requestBody.ServerPort)})
			return
		}
		// Agent 端口冲突检测（ServerPort+1000）
		agentPortNew := requestBody.ServerPort + 1000
		var agentRecord database.Socks5
		agentExists, err := database.Engine.Where("socks5port = ? AND id != ?", strconv.Itoa(agentPortNew), requestBody.ID).Get(&agentRecord)
		if err == nil && agentExists {
			log.Printf("[SOCKS5-API] ❌ Agent端口 %d 已被其他代理占用", agentPortNew)
			c.JSON(http.StatusBadRequest, gin.H{"status": 400, "error": fmt.Sprintf("Agent端口%d已被占用", agentPortNew)})
			return
		}
	}

	// 准备更新数据
	updateData := database.Socks5{
		UpdatedAt: time.Now(),
	}

	// 有选择性地更新字段
	if requestBody.ClientUID != "" {
		updateData.UID = requestBody.ClientUID
	}
	if requestBody.Protocol != "" {
		updateData.Type = requestBody.Protocol
	}
	if requestBody.ServerPort != 0 {
		updateData.Socks5port = strconv.Itoa(requestBody.ServerPort)
		// 更新连接地址（Agent端口 = SOCKS5端口 + 1000）
		agentPort := requestBody.ServerPort + 1000
		updateData.ConnectAddress = fmt.Sprintf(":%d", agentPort)
	}
	if requestBody.Note != "" {
		updateData.Note = requestBody.Note
	}
	if requestBody.Username != "" {
		updateData.UserName = requestBody.Username
	}
	if requestBody.Password != "" {
		updateData.Password = requestBody.Password
	}

	// 执行更新
	affected, err := database.Engine.ID(requestBody.ID).Update(&updateData)
	if err != nil {
		log.Printf("[SOCKS5-API] ❌ 更新数据库失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "error": "更新失败"})
		return
	}

	log.Printf("[SOCKS5-API] 📊 数据库更新影响行数: %d", affected)
	log.Printf("[SOCKS5-API] ✅ SOCKS5代理更新成功")

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "更新成功",
	})
}

// Socks5BatchDelete 批量删除SOCKS5代理
func Socks5BatchDelete(c *gin.Context) {
	log.Printf("[SOCKS5-API] 🗑️ 收到批量删除SOCKS5代理请求")

	var requestBody struct {
		IDs []int `json:"ids"`
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		log.Printf("[SOCKS5-API] ❌ 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(requestBody.IDs) == 0 {
		log.Printf("[SOCKS5-API] ❌ 没有提供要删除的ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有提供要删除的ID"})
		return
	}

	log.Printf("[SOCKS5-API] 🔍 批量删除代理记录，IDs: %v", requestBody.IDs)

	successCount := 0
	failedCount := 0
	var failedIDs []int

	for _, id := range requestBody.IDs {
		log.Printf("[SOCKS5-API] 🔍 处理删除代理记录 ID: %d", id)

		// 查找代理记录
		var socks5Record database.Socks5
		has, err := database.Engine.ID(id).Get(&socks5Record)
		if err != nil {
			log.Printf("[SOCKS5-API] ❌ 数据库查询失败 ID: %d, 错误: %v", id, err)
			failedCount++
			failedIDs = append(failedIDs, id)
			continue
		}
		if !has {
			log.Printf("[SOCKS5-API] ❌ 代理记录不存在 ID: %d", id)
			failedCount++
			failedIDs = append(failedIDs, id)
			continue
		}

		log.Printf("[SOCKS5-API] 📋 删除SOCKS5配置 ID: %d", id)
		log.Printf("[SOCKS5-API]    - 客户端UID: %s", socks5Record.UID)
		log.Printf("[SOCKS5-API]    - 连接地址: %s", socks5Record.ConnectAddress)
		log.Printf("[SOCKS5-API]    - SOCKS5端口: %s", socks5Record.Socks5port)

		// 安全解析端口
		var listenPort string
		if strings.Contains(socks5Record.ConnectAddress, ":") {
			parts := strings.Split(socks5Record.ConnectAddress, ":")
			if len(parts) >= 2 {
				listenPort = parts[1]
			}
		}
		if listenPort != "" {
			// 关闭所有相关监听器
			if proxy.CloseSocks5Server(":" + listenPort) {
				log.Printf("[SOCKS5-API] 🔄 已关闭Agent监听器: :%s", listenPort)
			}

			if proxy.CloseSocks5Server("0.0.0.0:" + socks5Record.Socks5port) {
				log.Printf("[SOCKS5-API] 🔄 已关闭SOCKS5监听器: 0.0.0.0:%s", socks5Record.Socks5port)
			}

			log.Printf("[SOCKS5-API] 📤 发送关闭命令到客户端...")
			sendcommand.SendCommand(socks5Record.UID, "socks5Close")
		}

		// 从数据库删除记录
		log.Printf("[SOCKS5-API] 🗑️ 从数据库删除记录 ID: %d", id)
		affected, err := database.Engine.ID(id).Delete(&database.Socks5{})
		if err != nil {
			log.Printf("[SOCKS5-API] ❌ 删除数据库记录失败 ID: %d, 错误: %v", id, err)
			failedCount++
			failedIDs = append(failedIDs, id)
			continue
		}

		log.Printf("[SOCKS5-API] 📊 数据库删除影响行数: %d, ID: %d", affected, id)
		if affected > 0 {
			successCount++
			log.Printf("[SOCKS5-API] ✅ SOCKS5代理删除成功 ID: %d", id)
		} else {
			failedCount++
			failedIDs = append(failedIDs, id)
			log.Printf("[SOCKS5-API] ❌ 删除失败，可能记录不存在 ID: %d", id)
		}
	}

	log.Printf("[SOCKS5-API] ✅ 批量删除完成: 成功 %d 个，失败 %d 个", successCount, failedCount)

	response := gin.H{
		"status":        http.StatusOK,
		"message":       fmt.Sprintf("批量删除完成: 成功 %d 个，失败 %d 个", successCount, failedCount),
		"success_count": successCount,
		"failed_count":  failedCount,
		"total_count":   len(requestBody.IDs),
	}

	if len(failedIDs) > 0 {
		response["failed_ids"] = failedIDs
	}

	c.JSON(http.StatusOK, response)
}

// isPortInUse 检测指定端口是否被占用
func isPortInUse(port string) (bool, error) {
	log.Printf("[SOCKS5-API] 🔍 检测端口是否被占用: %s", port)

	// 尝试监听该端口
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		// 如果监听失败，判断是否是端口被占用
		if opErr, ok := err.(*net.OpError); ok {
			if strings.Contains(opErr.Err.Error(), "bind: address already in use") ||
				strings.Contains(opErr.Err.Error(), "bind: Only one usage of each socket address") {
				log.Printf("[SOCKS5-API] ⚠️ 端口 %s 被占用", port)
				return true, nil // 端口被占用
			}
		}
		log.Printf("[SOCKS5-API] ❌ 端口检测发生其他错误: %v", err)
		return false, err // 其他错误
	}

	// 如果监听成功，关闭 listener 并返回未占用
	_ = listener.Close()
	log.Printf("[SOCKS5-API] ✅ 端口 %s 可用", port)
	return false, nil
}

// maskSecret 敏感字段脱敏（保留前 2 后 2 字符，其余掩码）
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "******"
	}
	return s[:2] + "******" + s[len(s)-2:]
}

// RestoreSocks5OnStartup 服务重启后恢复状态为运行中的 SOCKS5 代理监听。
// 注意：PSK 每次随机生成并经加密通道下发客户端；离线客户端无法提前获取 PSK，
// 恢复后需在 UI 重新"开启"或等客户端在线时手动触发。在线客户端会收到重新下发的连接命令。
func RestoreSocks5OnStartup() {
	var records []database.Socks5
	if err := database.Engine.Where("status = ?", 1).Find(&records); err != nil {
		log.Printf("[SOCKS5-API] 恢复查询失败: %v", err)
		return
	}
	restored := 0
	for _, r := range records {
		agentPort := strings.TrimPrefix(r.ConnectAddress, ":")
		if agentPort == "" {
			continue
		}
		socksAddr := "0.0.0.0:" + r.Socks5port
		psk := proxy.GenerateRandomPSK()
		if _, err := proxy.ReverseSocksServer(r.ConnectAddress, socksAddr, psk, r.UserName, r.Password); err != nil {
			// 恢复失败：回置状态，避免重启后一直显示运行中而实际没监听
			database.Engine.ID(r.ID).Update(&database.Socks5{Status: 0, UpdatedAt: time.Now()})
			log.Printf("[SOCKS5-API] ❌ 恢复失败（已置为停止）: 端口 %s, %v", r.Socks5port, err)
			continue
		}
		restored++
		log.Printf("[SOCKS5-API] 🔄 已恢复 SOCKS5 代理: 端口 %s (agent %s)", r.Socks5port, r.ConnectAddress)

		// 若客户端在线，重新下发 socks5 连接命令（PSK 以本次下发的为准）
		connection.MuClientListenerType.Lock()
		_, online := connection.ClientListenerType[r.UID]
		connection.MuClientListenerType.Unlock()
		if online {
			var client database.Clients
			if has, _ := database.Engine.Where("uid = ?", r.UID).Get(&client); has {
				var listener database.Listener
				if has2, _ := database.Engine.Where("type = ? AND status = ?", client.Listener, 1).Get(&listener); has2 && listener.ConnectAddress != "" {
					serverIP := strings.Split(listener.ConnectAddress, ":")[0]
					if serverIP != "" {
						sendcommand.SendCommand(r.UID, fmt.Sprintf("socks5 %s:%s|%s|false", serverIP, r.Socks5port, psk))
					}
				}
			}
		}
	}
	if restored > 0 {
		logx.Infof("[SOCKS5-API] ✅ 启动恢复完成：%d 个 SOCKS5 代理监听已拉起\n", restored)
	}
}
