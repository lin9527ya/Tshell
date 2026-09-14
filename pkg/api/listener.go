package api

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/api/communication"
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/certutil"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/connection"

	k "BackendTemplate/pkg/connection/kcp"
	"BackendTemplate/pkg/connection/oss"
	dns_listener "BackendTemplate/pkg/connection/dns"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/connection/websocket"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/xtaci/kcp-go/v5"
	"BackendTemplate/pkg/utils/logx"
)

func AddListener(c *gin.Context) {
	var listener struct {
		Type           string `json:"type"`
		ListenAddress  string `json:"listenAddress"`
		ConnectAddress string `json:"connectAddress"`
		Retries        string `json:"retries"`        // 心跳超时次数
		Interval       string `json:"interval"`       // 心跳检测间隔
		Note           string `json:"note"`           // 监听器备注
		VKey           string `json:"vkey"`           // VKey
		EncryptionSalt string `json:"encryptionSalt"` // 流量加密盐
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if exists, _ := database.Engine.Where("listen_address LIKE ?", "%"+listener.ListenAddress).Exist(&database.Listener{}); exists {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener already exists"})
		return
	}

	// 设置默认值（如果前端没有传值）
	heartbeatTimeout := listener.Retries
	if heartbeatTimeout == "" {
		heartbeatTimeout = "30"
	}
	heartbeatInterval := listener.Interval
	if heartbeatInterval == "" {
		heartbeatInterval = "10"
	}

	// 若未指定密钥/盐，随机生成并持久化，避免使用公开默认常量（见 encrypt.GetListenerKeyConfig 回退值）
	if listener.VKey == "" || listener.EncryptionSalt == "" {
		vk, salt := encrypt.GenerateListenerKeyConfig()
		if listener.VKey == "" {
			listener.VKey = vk
		}
		if listener.EncryptionSalt == "" {
			listener.EncryptionSalt = salt
		}
	}

	database.Engine.Insert(&database.Listener{
		Type:              listener.Type,
		ListenAddress:     listener.ListenAddress,
		ConnectAddress:    listener.ConnectAddress,
		Status:            1,
		HeartbeatTimeout:  heartbeatTimeout,
		HeartbeatInterval: heartbeatInterval,
		Note:              listener.Note,
		VKey:              listener.VKey,
		EncryptionSalt:    listener.EncryptionSalt,
	})
	encrypt.InvalidateListenerKeyCache()
	// D18: 业务审计
	auditLogEvent("LISTENER_ADD", "SUCCESS", "新增监听器 "+listener.Type+"://"+listener.ListenAddress, map[string]interface{}{"type": listener.Type, "address": listener.ListenAddress})
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener added"})
	go handleOpenPort(listener.Type, listener.ListenAddress, listener.VKey, listener.EncryptionSalt)
}
func ListListener(c *gin.Context) {
	var listeners []database.Listener
	database.Engine.Find(&listeners)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": listeners})
}
func OpenListener(c *gin.Context) {
	var listener struct {
		ListenAddress string `json:"listenAddress"`
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var lis database.Listener
	database.Engine.Where("listen_address = ?", listener.ListenAddress).Get(&lis)
	if lis.Status == 2 {
		database.Engine.Where("listen_address = ?", listener.ListenAddress).Update(&database.Listener{Status: 1})

		encrypt.InvalidateListenerKeyCache()
		go handleOpenPort(lis.Type, listener.ListenAddress, lis.VKey, lis.EncryptionSalt)
		auditLogEvent("LISTENER_OPEN", "SUCCESS", "启动监听器 "+lis.Type+"://"+listener.ListenAddress, nil)
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener opened"})
	} else {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener already opened"})
	}

}
func CloseListener(c *gin.Context) {
	var listener struct {
		ListenAddress string `json:"listenAddress"`
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var lis database.Listener
	database.Engine.Where("listen_address = ?", listener.ListenAddress).Get(&lis)
	if lis.Status == 1 {
		database.Engine.Where("listen_address = ?", listener.ListenAddress).Update(&database.Listener{Status: 2})
		encrypt.InvalidateListenerKeyCache()
		handleClosePort(lis.Type, lis.ListenAddress, c)
		auditLogEvent("LISTENER_CLOSE", "SUCCESS", "关闭监听器 "+lis.Type+"://"+listener.ListenAddress, nil)
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener closed"})
	} else {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener already closed"})
	}

}
func DeleteListener(c *gin.Context) {
	var listener struct {
		ListenAddress string `json:"listenAddress"`
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var lis database.Listener
	database.Engine.Where("listen_address = ?", listener.ListenAddress).Get(&lis)
	if lis.Status == 1 {
		handleClosePort(lis.Type, lis.ListenAddress, c)
	}
	database.Engine.Where("listen_address = ?", listener.ListenAddress).Delete(&database.Listener{})
	encrypt.InvalidateListenerKeyCache()
	auditLogEvent("LISTENER_DELETE", "SUCCESS", "删除监听器 "+lis.Type+"://"+listener.ListenAddress, nil)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener deleted"})
}

// UpdateListener 更新监听器配置
func UpdateListener(c *gin.Context) {
	var listener struct {
		Id                int64  `json:"id"`             // 监听器ID，用于唯一标识
		ListenAddress     string `json:"listenAddress"`
		Type              string `json:"type"`
		ConnectAddress    string `json:"connectAddress"`
		HeartbeatTimeout  string `json:"retries"`        // 心跳超时次数
		HeartbeatInterval string `json:"interval"`       // 心跳检测间隔
		Note              string `json:"note"`           // 监听器备注
		VKey              string `json:"vkey"`           // VKey
		EncryptionSalt    string `json:"encryptionSalt"` // 流量加密盐
	}

	// 检查请求上下文
	if c == nil {
		log.Printf("❌ [监听器更新] 请求上下文为空")
		return
	}

	if err := c.BindJSON(&listener); err != nil {
		log.Printf("❌ [监听器更新] 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数绑定失败: " + err.Error()})
		return
	}

	// 检查必要参数
	if listener.Id <= 0 {
		log.Printf("❌ [监听器更新] 监听器ID不能为空或无效")
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "监听器ID不能为空或无效"})
		return
	}
	if listener.ListenAddress == "" {
		log.Printf("❌ [监听器更新] 监听地址不能为空")
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "监听地址不能为空"})
		return
	}

	// 设置默认值（如果前端没有传值）
	heartbeatTimeout := listener.HeartbeatTimeout
	if heartbeatTimeout == "" {
		heartbeatTimeout = "30"
	}
	heartbeatInterval := listener.HeartbeatInterval
	if heartbeatInterval == "" {
		heartbeatInterval = "10"
	}

	// 检查数据库引擎是否初始化
	if database.Engine == nil {
		log.Printf("❌ [监听器更新] 数据库引擎未初始化")
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库引擎未初始化"})
		return
	}

	// 首先获取当前监听器的状态，确保更新时不改变状态
	var currentListener database.Listener
	// 使用ID查询当前监听器信息
	has, err := database.Engine.ID(listener.Id).Get(&currentListener)
	if err != nil {
		log.Printf("❌ [监听器更新] 查询当前监听器信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "查询当前监听器信息失败: " + err.Error()})
		return
	}
	if !has {
		log.Printf("❌ [监听器更新] 监听器不存在: ID=%d", listener.Id)
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "data": fmt.Sprintf("监听器不存在: ID=%d", listener.Id)})
		return
	}

	// 记录更新前的状态
	log.Printf("🔍 [监听器更新] 更新前状态 - ID: %d, 地址: %s -> %s, 状态: %d", listener.Id, currentListener.ListenAddress, listener.ListenAddress, currentListener.Status)

	// 更新数据库中的监听器记录，保持原有状态
	updateData := &database.Listener{
		Type:              listener.Type,
		ListenAddress:     listener.ListenAddress,     // 允许更新监听地址
		ConnectAddress:    listener.ConnectAddress,
		HeartbeatTimeout:  heartbeatTimeout,
		HeartbeatInterval: heartbeatInterval,
		Note:              listener.Note,
		Status:            currentListener.Status, // 保持原有状态
	}
	// 密钥字段仅在显式提供时更新：编辑保存时留空视为不修改，
	// 避免前端表单未填写密钥就把监听器密钥清空（清空后监听器无法启动）
	if listener.VKey != "" {
		updateData.VKey = listener.VKey
	} else {
		updateData.VKey = currentListener.VKey
	}
	if listener.EncryptionSalt != "" {
		updateData.EncryptionSalt = listener.EncryptionSalt
	} else {
		updateData.EncryptionSalt = currentListener.EncryptionSalt
	}

	// 使用ID更新记录，而不是listenAddress
	// 【零值修复】AllCols 显式写入全部字段：请求即完整配置，空字符串也须落库
	// （默认零值跳过会导致清空 Note/密钥等字段静默失效）
	affected, err := database.Engine.ID(listener.Id).AllCols().Update(updateData)
	if err != nil {
		log.Printf("❌ [监听器更新] 更新监听器失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "更新监听器失败: " + err.Error()})
		return
	}
	encrypt.InvalidateListenerKeyCache()

	// 记录更新后的状态
	log.Printf("✅ [监听器更新] 更新成功 - ID: %d, 新地址: %s, 影响行数: %d, 状态: %d", listener.Id, listener.ListenAddress, affected, currentListener.Status)

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "监听器更新成功"})
}

// BatchDeleteListeners 批量删除监听器
func BatchDeleteListeners(c *gin.Context) {
	log.Printf("[LISTENER-API] 🗑️ 收到批量删除监听器请求")

	var requestBody struct {
		ListenAddresses []string `json:"listen_addresses"`
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		log.Printf("[LISTENER-API] ❌ 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(requestBody.ListenAddresses) == 0 {
		log.Printf("[LISTENER-API] ❌ 没有提供要删除的监听地址")
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有提供要删除的监听地址"})
		return
	}

	log.Printf("[LISTENER-API] 🔍 批量删除监听器，地址: %v", requestBody.ListenAddresses)

	successCount := 0
	failedCount := 0
	var failedAddresses []string

	for _, address := range requestBody.ListenAddresses {
		log.Printf("[LISTENER-API] 🔍 处理删除监听器 地址: %s", address)

		// 查找监听器记录
		var listener database.Listener
		has, err := database.Engine.Where("listen_address = ?", address).Get(&listener)
		if err != nil {
			log.Printf("[LISTENER-API] ❌ 数据库查询失败 地址: %s, 错误: %v", address, err)
			failedCount++
			failedAddresses = append(failedAddresses, address)
			continue
		}
		if !has {
			log.Printf("[LISTENER-API] ❌ 监听器记录不存在 地址: %s", address)
			failedCount++
			failedAddresses = append(failedAddresses, address)
			continue
		}

		log.Printf("[LISTENER-API] 📋 删除监听器配置 地址: %s", address)

		// 如果监听器在线，先关闭它
		if listener.Status == 1 {
			log.Printf("[LISTENER-API] 🔄 关闭活跃监听器 地址: %s", address)
			handleClosePort(listener.Type, listener.ListenAddress, c)
		}

		// 从数据库删除监听器记录
		log.Printf("[LISTENER-API] 🗑️ 从数据库删除监听器记录 地址: %s", address)
		affected, err := database.Engine.Where("listen_address = ?", address).Delete(&database.Listener{})
		if err != nil {
			log.Printf("[LISTENER-API] ❌ 删除数据库记录失败 地址: %s, 错误: %v", address, err)
			failedCount++
			failedAddresses = append(failedAddresses, address)
			continue
		}
		log.Printf("[LISTENER-API] 📊 数据库删除影响行数: %d, 地址: %s", affected, address)
		if affected > 0 {
			successCount++
			log.Printf("[LISTENER-API] ✅ 监听器删除成功 地址: %s", address)
		} else {
			failedCount++
			failedAddresses = append(failedAddresses, address)
			log.Printf("[LISTENER-API] ❌ 删除失败，可能记录不存在 地址: %s", address)
		}
	}
	encrypt.InvalidateListenerKeyCache()

	log.Printf("[LISTENER-API] ✅ 批量删除完成: 成功 %d 个，失败 %d 个", successCount, failedCount)

	response := gin.H{
		"status":        200,
		"message":       fmt.Sprintf("批量删除完成: 成功 %d 个，失败 %d 个", successCount, failedCount),
		"success_count": successCount,
		"failed_count":  failedCount,
		"total_count":   len(requestBody.ListenAddresses),
	}

	if len(failedAddresses) > 0 {
		response["failed_addresses"] = failedAddresses
	}

	c.JSON(http.StatusOK, response)
}
// handleOpenPort 启动监听器。vkey/salt 为该监听器自身的密钥配置，
// 传递给连接处理器用于会话密钥派生，避免同类型多监听器时按 type 查库取错密钥。

// registerHTTPRoutes 按 C2 Profile 注册 http/https 监听器路由（默认 profile 路径兼容）
func registerHTTPRoutes(mux *http.ServeMux) {
	p := c2profile.GetDefault()
	getURI, postURI := "/tencent/mcp/pc/pcsearch", "/tencent/sensearch/collection/item/check"
	if p != nil {
		if p.HTTP.Get.URI != "" {
			getURI = p.HTTP.Get.URI
		}
		if p.HTTP.Post.URI != "" {
			postURI = p.HTTP.Post.URI
		}
	}
	mux.HandleFunc(getURI, communication.GetHttp)
	mux.HandleFunc(postURI, communication.PostHttp)
}

func handleOpenPort(listenerType string, listenerAddress string, vkey string, salt string) {
	// 崩溃防护：任何监听器 panic 都不能杀死整个 C2 进程
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("❌ [监听器-RECOVER] 监听器 %s (%s) panic已捕获: %v\n", listenerType, listenerAddress, r)
		}
	}()

	// J4: 启动强制——监听器无自定义密钥（默认/空）时拒绝启动
	// （默认密钥为公开常量：流量可解密、可伪造客户端劫持会话）
	mk := make([]byte, 32)
	copy(mk, []byte(vkey))
	sl := make([]byte, 16)
	copy(sl, []byte(salt))
	if vkey == "" || salt == "" || encrypt.IsDefaultListenerKey(mk, sl) {
		logx.Errorf("🚫 [监听器] 拒绝启动 %s://%s：请先配置自定义 VKey/加密盐（当前为默认/空密钥，公开且可预测）\n", listenerType, listenerAddress)
		return
	}

	switch listenerType {
	case "websocket":
		mux := http.NewServeMux()
		mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
			if vkey != "" && salt != "" {
				websocket.HandleWebSocketWithKeys(w, r, []byte(vkey), []byte(salt))
			} else {
				websocket.HandleWebSocket(w, r)
			}
		})

		server := &http.Server{
			Addr:           listenerAddress, // 确保listener.ListenAddress包含了端口号，如":8080"
			Handler:        mux,
			ReadTimeout:    15 * time.Second,
			WriteTimeout:   15 * time.Second,
			MaxHeaderBytes: 1 << 20,
		}
		connection.MuClientListenerType.Lock()
		connection.HttpServer[listenerAddress] = server
		connection.MuClientListenerType.Unlock()
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 不用 log.Fatal：单端口 bind 失败不应拖垮整个 C2（其他监听器与已上线客户端）
			logx.Errorf("❌ [监听器管理] WebSocket 监听 %s 启动失败: %v\n", listenerAddress, err)
			connection.MuClientListenerType.Lock()
			delete(connection.HttpServer, listenerAddress)
			connection.MuClientListenerType.Unlock()
		}

	case "tcp":
		tcpListener, err := net.Listen("tcp", listenerAddress)
		if err != nil {
			logx.Println("Error listening:", err)
			return
		}
		connection.MuClientListenerType.Lock()
		connection.TCPServer[listenerAddress] = tcpListener
		connection.MuClientListenerType.Unlock()
		logx.Println("Listening on:", listenerAddress)

		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				logx.Println("Error accepting connection:", err)
				break
			}

			go tcp.HandleTcpConnectionForWithKeys(conn, "tcp", []byte(vkey), []byte(salt))
		}
	case "kcp":
		lis, err := kcp.ListenWithOptions(listenerAddress, nil, 10, 3)
		if err != nil {
			// 不用 log.Fatal：单端口失败不拖垮整个 C2
			logx.Errorf("❌ [监听器管理] KCP 监听 %s 启动失败: %v\n", listenerAddress, err)
			return
		}
		logx.Println("Server listening on", listenerAddress)
		connection.MuClientListenerType.Lock()
		connection.KCPServer[listenerAddress] = lis
		connection.MuClientListenerType.Unlock()
		// 循环等待客户端连接
		for {
			conn, err := lis.AcceptKCP()
			if err != nil {
				log.Println("Accept error:", err)
				break
			}
			logx.Println("Client connected:", conn.RemoteAddr())

			// 处理客户端连接
			go k.HandleKCPConnectionForWithKeys(conn, "kcp", []byte(vkey), []byte(salt))
		}
	case "http":
		mux := http.NewServeMux()
		registerHTTPRoutes(mux)

		server := &http.Server{
			Addr:              listenerAddress,
			Handler:           mux,
			ReadHeaderTimeout: 30 * time.Second, // 防 Slowloris：慢速请求头占满连接
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		// 存储服务器实例
		connection.MuClientListenerType.Lock()
		connection.HttpServer[listenerAddress] = server
		connection.MuClientListenerType.Unlock()

		// 启动服务器（非阻塞）
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logx.Println(err)
			}
		}()
	case "https":
		// HTTPS C2 监听器：与 http 相同的轮询路由，但走 TLS。
		// 证书优先取 config.yaml https.cert_file/key_file；为空则自动生成自签名证书兜底
		//（SAN 含 connect_address 主机名与回环）。客户端严格校验证书——生产请配置真实证书。
		certFile, keyFile := "", ""
		if g := config.GetGlobalConfig(); g != nil {
			certFile, keyFile = g.HTTPS.CertFile, g.HTTPS.KeyFile
		}
		if certFile == "" || keyFile == "" {
			var sanHosts []string
			var lis database.Listener
			if has, _ := database.Engine.Where("listen_address = ?", listenerAddress).Get(&lis); has && lis.ConnectAddress != "" {
				// connect_address 形如 host:port 或 host（取 host 部分作 SAN）
				ca := strings.Split(lis.ConnectAddress, ":")
				sanHosts = append(sanHosts, ca[0])
			}
			var err error
			certFile, keyFile, err = certutil.GenerateSelfSigned(sanHosts, "certs")
			if err != nil {
				logx.Errorf("❌ [HTTPS] 生成自签名证书失败: %v，监听器 %s 未启动\n", err, listenerAddress)
				return
			}
			logx.Infof("🔐 [HTTPS] 未配置证书，已生成自签名证书: %s（生产请配置真实证书）\n", certFile)
		}

		mux := http.NewServeMux()
		registerHTTPRoutes(mux)

		server := &http.Server{
			Addr:              listenerAddress,
			Handler:           mux,
			ReadHeaderTimeout: 30 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		// 存储服务器实例（与 http 共用 HttpServer map，close 时统一处理）
		connection.MuClientListenerType.Lock()
		connection.HttpServer[listenerAddress] = server
		connection.MuClientListenerType.Unlock()

		// 启动 TLS 服务器（非阻塞）
		go func() {
			if err := server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				logx.Println(err)
			}
		}()
	case "oss":
		tmp := strings.Split(listenerAddress, ":")
		connection.MuClientListenerType.Lock()
		// I6: 有缓冲(1)停止通道——OSS 循环可能在 List/process_server 中（无 select），
		// 无缓冲 + 非阻塞发送会丢信号导致 OSS 轮询 goroutine 永久泄漏
		connection.StopChan[listenerAddress] = make(chan bool, 1)
		connection.MuClientListenerType.Unlock()
		go oss.HandleOSSConnection(tmp[0], tmp[1], tmp[2], tmp[3])
	case "dns":
		tmp := strings.Split(listenerAddress, ":")
		dnsAddr := tmp[0] + ":" + tmp[1]
		domain := "dns.tshell."
		if len(tmp) > 2 {
			domain = tmp[2] + "."
		}
		// OPSEC: DNS 标签前缀随 Profile（客户端生成时同步 patch 同一值）
		if p := c2profile.GetDefault(); p != nil {
			dns_listener.SetDNSLabelPrefix(sanitizeMasquerade(p.DNS.BeaconPrefix))
		}
		go dns_listener.HandleDNSConnection(dnsAddr, domain)
	}
}
func handleClosePort(listenerType string, listenerAddress string, c *gin.Context) {
	logx.Infof("🔄 [监听器管理] 开始关闭 %s 监听器: %s\n", listenerType, listenerAddress)

	// 首先清理该监听器下的所有客户端连接
	cleanupClientsForListener(listenerType, listenerAddress)

	switch listenerType {
	case "websocket":
		// 锁内取句柄并 delete，解锁后再 Close，避免与 handleOpenPort 写 map 竞态
		connection.MuClientListenerType.Lock()
		srv := connection.HttpServer[listenerAddress]
		delete(connection.HttpServer, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if srv == nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener not found"})
			return
		}
		err := srv.Close()
		logx.Infof("✅ [监听器管理] WebSocket监听器已关闭: %s\n", listenerAddress)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener closed failed"})
			return
		}
	case "tcp":
		connection.MuClientListenerType.Lock()
		lis := connection.TCPServer[listenerAddress]
		delete(connection.TCPServer, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if lis == nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener not found"})
			return
		}
		err := lis.Close()
		logx.Infof("✅ [监听器管理] TCP监听器已关闭: %s\n", listenerAddress)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener closed failed"})
			return
		}
	case "kcp":
		connection.MuClientListenerType.Lock()
		lis := connection.KCPServer[listenerAddress]
		delete(connection.KCPServer, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if lis == nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener not found"})
			return
		}
		err := lis.Close()
		logx.Infof("✅ [监听器管理] KCP监听器已关闭: %s\n", listenerAddress)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener closed failed"})
			return
		}
	case "http", "https":
		connection.MuClientListenerType.Lock()
		srv := connection.HttpServer[listenerAddress]
		delete(connection.HttpServer, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if srv == nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener not found"})
			return
		}
		err := srv.Close()
		logx.Infof("✅ [监听器管理] %s监听器已关闭: %s\n", strings.ToUpper(listenerType), listenerAddress)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener closed failed"})
			return
		}
	case "oss":
		// 先取通道（锁内），再锁外发信号，最后删除 map 条目
		// （避免"先删后发"导致 OSS 循环读到 nil 通道而永不接收，发送永久阻塞）
		connection.MuClientListenerType.Lock()
		ch := connection.StopChan[listenerAddress]
		connection.MuClientListenerType.Unlock()
		if ch != nil {
			select {
			case ch <- true:
			default:
				// 非阻塞发送：OSS 循环可能正处于 select 之外，避免卡死关闭接口
			}
		}
		connection.MuClientListenerType.Lock()
		delete(connection.StopChan, listenerAddress)
		connection.MuClientListenerType.Unlock()
		logx.Infof("✅ [监听器管理] OSS监听器已关闭: %s\n", listenerAddress)
	case "dns":
		dns_listener.Stop()
		logx.Infof("✅ [监听器管理] DNS监听器已关闭: %s\n", listenerAddress)
	}

	logx.Infof("🎯 [监听器管理] %s 监听器 %s 关闭完成，所有客户端已清理\n", listenerType, listenerAddress)
}

// cleanupClientsForListener 清理特定监听器下的所有客户端连接
func cleanupClientsForListener(listenerType string, listenerAddress string) {
	logx.Infof("🧹 [客户端清理] 开始清理 %s 监听器 %s 下的客户端连接\n", listenerType, listenerAddress)

	// 获取该监听器下的所有客户端
	var clients []database.Clients
	database.Engine.Where("listener = ? AND online = '1'", listenerType).Find(&clients)

	cleanupCount := 0
	for _, client := range clients {
		uid := client.Uid
		logx.Infof("🔄 [客户端清理] 清理客户端: %s (协议: %s)\n", uid, listenerType)

		// 根据协议类型清理相应的连接管理器
		switch listenerType {
		case "tcp", "tcp_forward", "http_forward", "https_forward":
			tcp.Mutex.Lock()
			if conn, exists := tcp.TCPClientManger[uid]; exists && conn != nil {
				conn.Close()
				delete(tcp.TCPClientManger, uid)
				delete(tcp.ClientTimeManager, uid)
				logx.Infof("🔌 [客户端清理] TCP连接已断开: %s\n", uid)
			}
			tcp.Mutex.Unlock()

		case "kcp", "kcp_forward":
			k.Mutex.Lock()
			if conn, exists := k.KCPClientManger[uid]; exists && conn != nil {
				conn.Close()
				delete(k.KCPClientManger, uid)
				delete(k.ClientTimeManager, uid)
				logx.Infof("🔌 [客户端清理] KCP连接已断开: %s\n", uid)
			}
			k.Mutex.Unlock()

		case "websocket", "ws_forward":
			websocket.MuClientManager.Lock()
			if conn, exists := websocket.ClientManager[uid]; exists && conn != nil {
				// 发送关闭消息并关闭WebSocket连接
				conn.WriteMessage(gorillaws.CloseMessage, gorillaws.FormatCloseMessage(gorillaws.CloseNormalClosure, "监听器关闭"))
				conn.Close()
				delete(websocket.ClientManager, uid)
				delete(websocket.ClientTimeManager, uid)
				logx.Infof("🔌 [客户端清理] WebSocket连接已断开: %s\n", uid)
			}
			websocket.MuClientManager.Unlock()

		case "http":
			communication.MuClientManager.Lock()
			if clientTime, exists := communication.ClientTimeManager[uid]; exists {
				// HTTP是无状态的，只需清理时间管理器
				delete(communication.ClientTimeManager, uid)
				_ = clientTime // 避免未使用变量警告
				logx.Infof("🔌 [客户端清理] HTTP客户端状态已清理: %s\n", uid)
			}
			communication.MuClientManager.Unlock()

		case "oss":
			// OSS协议的客户端清理
			logx.Infof("🔌 [客户端清理] OSS客户端状态已清理: %s\n", uid)
		}

		// 清理全局连接类型映射
		connection.MuClientListenerType.Lock()
		delete(connection.ClientListenerType, uid)
		connection.MuClientListenerType.Unlock()

		// 统一离线入口：置 Online=2 + 离线事件 + 队列清理（幂等）
		connection.MarkOffline(uid, listenerType, "监听器关闭清理")
		logx.Infof("📝 [客户端清理] 数据库状态已更新为离线: %s\n", uid)

		cleanupCount++
	}

	logx.Infof("✅ [客户端清理] 完成清理 %s 监听器，共清理 %d 个客户端连接\n", listenerType, cleanupCount)
}
// auditLogEvent 业务审计快捷入口（D18）
func auditLogEvent(action, result, message string, details map[string]interface{}) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategoryAPI, action, result, message, details)
	}
}