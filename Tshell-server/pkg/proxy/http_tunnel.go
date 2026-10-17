package proxy

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/proxy/bufferpool"
	"BackendTemplate/pkg/proxy/mux"
	"BackendTemplate/pkg/sendcommand"
)

// HTTPTunnelManager HTTP隧道管理器
type HTTPTunnelManager struct {
	mu        sync.RWMutex
	tunnels   map[string]*HTTPTunnelInstance
	listeners map[int]*net.TCPListener
}

// HTTPTunnelInstance HTTP隧道实例
type HTTPTunnelInstance struct {
	ID            int64
	ClientUID     string
	ServerPort    int
	AgentPort     int // Agent监听端口
	Username      string
	Password      string
	ProxyListener *net.TCPListener // HTTP代理监听器
	AgentListener *net.TCPListener // Agent连接监听器
	Status        string
	CreatedAt     time.Time
	// Session 由 Agent 回连 goroutine 写入、代理处理路径读取，用原子指针保证无竞争
	Session atomic.Pointer[mux.Mux] // 多路复用会话
	PSK     string                  // 预共享密钥
}

var (
	// 全局HTTP隧道管理器
	globalHTTPTunnelManager = &HTTPTunnelManager{
		tunnels:   make(map[string]*HTTPTunnelInstance),
		listeners: make(map[int]*net.TCPListener),
	}

	// HTTP隧道专用缓冲池
	httpBufferPool = bufferpool.NewPool(65536)

	// HTTP隧道魔法包（与SOCKS5不同，避免冲突）
	httpMagicPacket = [64]byte{
		0x48, 0x54, 0x54, 0x50, 0x54, 0x55, 0x4e, 0x4e, // "HTTPTUNN"
		0x45, 0x4c, 0x5f, 0x4d, 0x41, 0x47, 0x49, 0x43, // "EL_MAGIC"
		0xa1, 0xb2, 0xc3, 0xd4, 0xe5, 0xf6, 0x17, 0x28,
		0x39, 0x4a, 0x5b, 0x6c, 0x7d, 0x8e, 0x9f, 0xa0,
		0xb1, 0xc2, 0xd3, 0xe4, 0xf5, 0x06, 0x17, 0x28,
		0x39, 0x4a, 0x5b, 0x6c, 0x7d, 0x8e, 0x9f, 0xa0,
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00,
	}
)

// StartHTTPTunnel 启动HTTP隧道（新架构：基于SOCKS5设计）
func StartHTTPTunnel(tunnel database.TunnelProxy) error {
	log.Printf("[HTTP-TUNNEL] 🚀 启动HTTP隧道代理 (新架构)")

	// 获取客户端信息
	var client database.Clients
	exists, err := database.Engine.Where("uid = ? AND online = '1'", tunnel.ClientUID).Get(&client)
	if err != nil || !exists {
		return fmt.Errorf("客户端不在线或不存在")
	}

	// 获取监听器信息以确定客户端连接地址
	var listener database.Listener
	exists, err = database.Engine.Where("type = ? AND status = ?", client.Listener, 1).Get(&listener)
	if err != nil || !exists {
		return fmt.Errorf("监听器不存在或未运行")
	}

	log.Printf("[HTTP-TUNNEL] 📋 配置信息:")
	log.Printf("[HTTP-TUNNEL]    - 客户端UID: %s", tunnel.ClientUID)
	log.Printf("[HTTP-TUNNEL]    - HTTP代理端口: %d", tunnel.ServerPort)
	log.Printf("[HTTP-TUNNEL]    - 用户名: %s", tunnel.Username)
	log.Printf("[HTTP-TUNNEL]    - 密码长度: %d", len(tunnel.Password))

	// 检查端口是否被占用
	globalHTTPTunnelManager.mu.Lock()
	defer globalHTTPTunnelManager.mu.Unlock()

	if _, exists := globalHTTPTunnelManager.listeners[tunnel.ServerPort]; exists {
		return fmt.Errorf("HTTP代理端口 %d 已被占用", tunnel.ServerPort)
	}

	// 分配Agent监听端口（HTTP代理端口+1000）
	agentPort := tunnel.ServerPort + 1000
	if _, exists := globalHTTPTunnelManager.listeners[agentPort]; exists {
		return fmt.Errorf("Agent端口 %d 已被占用", agentPort)
	}

	// 从监听器连接地址中提取IP地址
	serverIP := listener.ConnectAddress
	if strings.Contains(serverIP, ":") {
		serverIP = strings.Split(serverIP, ":")[0]
	}

	// 生成随机 PSK（替代原先由 uid+端口推导的可预测密钥），经加密 C2 通道下发
	psk := GenerateRandomPSK()

	// 创建隧道实例
	instance := &HTTPTunnelInstance{
		ID:         tunnel.ID,
		ClientUID:  tunnel.ClientUID,
		ServerPort: tunnel.ServerPort,
		AgentPort:  agentPort,
		Username:   tunnel.Username,
		Password:   tunnel.Password,
		Status:     "starting",
		CreatedAt:  time.Now(),
		PSK:        psk,
	}

	// 启动Agent监听器（等待客户端连接）
	agentListener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: agentPort})
	if err != nil {
		return fmt.Errorf("启动Agent监听器失败: %v", err)
	}
	instance.AgentListener = agentListener

	// 启动HTTP代理监听器
	proxyListener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: tunnel.ServerPort})
	if err != nil {
		agentListener.Close()
		return fmt.Errorf("启动HTTP代理监听器失败: %v", err)
	}
	instance.ProxyListener = proxyListener

	// 保存到管理器
	tunnelKey := fmt.Sprintf("%s:%d", tunnel.ClientUID, tunnel.ServerPort)
	globalHTTPTunnelManager.tunnels[tunnelKey] = instance
	globalHTTPTunnelManager.listeners[tunnel.ServerPort] = proxyListener
	globalHTTPTunnelManager.listeners[agentPort] = agentListener

	log.Printf("[HTTP-TUNNEL] 🔗 Agent监听端口: %d", agentPort)
	log.Printf("[HTTP-TUNNEL] 🌐 HTTP代理监听端口: %d", tunnel.ServerPort)

	// 启动Agent连接处理器
	go httpTunnelAgentServer(instance)

	// 【关键修复】发送连接命令给客户端，使用正确的服务器IP地址
	connectCmd := fmt.Sprintf("%s:%d|%s", serverIP, agentPort, psk)
	sendcommand.SendCommand(tunnel.ClientUID, "http_tunnel_connect "+connectCmd)
	log.Printf("[HTTP-TUNNEL] 📤 已向客户端发送连接命令: 服务器地址=%s:%d, PSK长度=%d", serverIP, agentPort, len(psk))

	// 记录审计日志
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategorySystem, "HTTP_TUNNEL_START", "SUCCESS",
			fmt.Sprintf("HTTP隧道 %d 启动成功，HTTP代理端口: %d, Agent端口: %d", tunnel.ID, tunnel.ServerPort, agentPort),
			map[string]interface{}{
				"tunnel_id":  tunnel.ID,
				"protocol":   "http",
				"proxy_port": tunnel.ServerPort,
				"agent_port": agentPort,
				"client_uid": tunnel.ClientUID,
				"server_ip":  serverIP,
			})
	}

	log.Printf("[HTTP-TUNNEL] ✅ HTTP隧道启动成功")
	return nil
}

// StopHTTPTunnel 停止HTTP隧道（新架构）
func StopHTTPTunnel(tunnel database.TunnelProxy) error {
	log.Printf("[HTTP-TUNNEL] 🛑 停止HTTP隧道代理 (新架构)")

	globalHTTPTunnelManager.mu.Lock()
	defer globalHTTPTunnelManager.mu.Unlock()

	// 从管理器中移除
	tunnelKey := fmt.Sprintf("%s:%d", tunnel.ClientUID, tunnel.ServerPort)
	instance, exists := globalHTTPTunnelManager.tunnels[tunnelKey]
	if !exists {
		return fmt.Errorf("HTTP隧道不存在或未运行")
	}

	// 关闭多路复用会话
	if s := instance.Session.Load(); s != nil {
		s.Close()
		log.Printf("[HTTP-TUNNEL] 多路复用会话已关闭")
	}

	// 关闭Agent监听器
	if instance.AgentListener != nil {
		instance.AgentListener.Close()
		log.Printf("[HTTP-TUNNEL] Agent监听器已关闭: %d", instance.AgentPort)
	}

	// 关闭HTTP代理监听器
	if instance.ProxyListener != nil {
		instance.ProxyListener.Close()
		log.Printf("[HTTP-TUNNEL] HTTP代理监听器已关闭: %d", instance.ServerPort)
	}

	// 从管理器中移除
	delete(globalHTTPTunnelManager.tunnels, tunnelKey)
	delete(globalHTTPTunnelManager.listeners, tunnel.ServerPort)
	delete(globalHTTPTunnelManager.listeners, instance.AgentPort)

	// 记录审计日志
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategorySystem, "HTTP_TUNNEL_STOP", "SUCCESS",
			fmt.Sprintf("HTTP隧道 %d 停止成功，HTTP代理端口: %d, Agent端口: %d", tunnel.ID, tunnel.ServerPort, instance.AgentPort),
			map[string]interface{}{
				"tunnel_id":  tunnel.ID,
				"protocol":   "http",
				"proxy_port": tunnel.ServerPort,
				"agent_port": instance.AgentPort,
				"client_uid": tunnel.ClientUID,
			})
	}

	log.Printf("[HTTP-TUNNEL] ✅ HTTP隧道停止成功")
	return nil
}

// httpTunnelAgentServer Agent连接处理服务器（类似ReverseSocksServer）
func httpTunnelAgentServer(instance *HTTPTunnelInstance) {
	log.Printf("[HTTP-TUNNEL] 🔍 等待Agent连接: %d", instance.AgentPort)

	for {
		conn, err := instance.AgentListener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("[HTTP-TUNNEL] 🔚 Agent监听器已关闭，退出主循环")
				break
			} else {
				log.Printf("[HTTP-TUNNEL] ❌ 接受Agent连接失败: %v", err)
				continue
			}
		}

		log.Printf("[HTTP-TUNNEL] 🔗 Agent已连接: %s", conn.RemoteAddr().String())

		// 验证魔法包
		deadline := time.Now().Add(time.Second * 5)
		conn.SetReadDeadline(deadline)
		buffer := make([]byte, 64)
		log.Printf("[HTTP-TUNNEL] ⏱️ 等待HTTP隧道魔法包验证（5秒超时）...")
		// 用 io.ReadFull 读取完整 64 字节，避免 TCP 短读导致验证失败
		if _, err := io.ReadFull(conn, buffer); err != nil {
			log.Printf("[HTTP-TUNNEL] ❌ 读取魔法包失败: %v", err)
			conn.Close()
			continue
		}
		magic := DeriveMagicPacket(instance.PSK)
		if bytes.Equal(buffer, magic[:]) {
			log.Printf("[HTTP-TUNNEL] ✅ HTTP隧道魔法包验证成功！")
		} else {
			log.Printf("[HTTP-TUNNEL] ❌ HTTP隧道魔法包验证失败")
			conn.Close()
			continue
		}
		conn.SetReadDeadline(time.Time{})

		// 创建多路复用会话
		log.Printf("[HTTP-TUNNEL] 🔀 创建多路复用会话...")
		session := mux.Client(conn, instance.PSK)
		instance.Session.Store(session)
		log.Printf("[HTTP-TUNNEL] ✅ 多路复用会话已创建，启动HTTP代理服务器...")

		// 更新状态
		instance.Status = "running"

		// 启动HTTP代理处理器
		go func() {
			httpTunnelProxyServer(instance)
			log.Printf("[HTTP-TUNNEL] 🔚 HTTP代理服务器已关闭")
			session.Close()
		}()

		// agent 断开(mux 关闭)时关闭代理监听器，使 httpTunnelProxyServer 退出 Accept，避免监听器/端口泄漏
		go func() {
			<-session.Done()
			instance.ProxyListener.Close()
		}()

		// 只处理一个连接，因为HTTP隧道是1对1的
		break
	}
}

// httpTunnelProxyServer HTTP代理服务器主循环（使用多路复用）
func httpTunnelProxyServer(instance *HTTPTunnelInstance) {
	log.Printf("[HTTP-TUNNEL] 🔄 HTTP代理服务器开始监听连接")

	// P1-3: 并发连接上限（信号量），防止恶意客户端洪泛打开无限 goroutine + mux 流
	const maxConcurrent = 256
	sem := make(chan struct{}, maxConcurrent)

	for {
		conn, err := instance.ProxyListener.Accept()
		if err != nil {
			// 监听器关闭或出错，退出
			log.Printf("[HTTP-TUNNEL] HTTP代理监听器关闭: %v", err)
			return
		}

		// 超过上限：拒绝并关闭连接（保持背压，不阻塞 Accept 循环）
		select {
		case sem <- struct{}{}:
			go func(c net.Conn) {
				defer func() { <-sem }()
				handleHTTPTunnelProxyConnection(c, instance)
			}(conn)
		default:
			log.Printf("[HTTP-TUNNEL] ⚠️ 并发连接数超过上限 %d，拒绝连接: %s", maxConcurrent, conn.RemoteAddr())
			conn.Close()
		}
	}
}

// handleHTTPTunnelProxyConnection 处理HTTP代理连接（新架构：使用多路复用）
// checkTunnelProxyAuth 校验 Proxy-Authorization（Basic）。password 为空表示未配置认证。
func checkTunnelProxyAuth(req *http.Request, username, password string) bool {
	if password == "" {
		return true
	}
	const prefix = "Basic "
	h := req.Header.Get("Proxy-Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(h[len(prefix):]))
	if err != nil {
		return false
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok {
		return false
	}
	// 常量时间比较，避免按字节泄露凭据
	return subtle.ConstantTimeCompare([]byte(user), []byte(username)) == 1 &&
		subtle.ConstantTimeCompare([]byte(pass), []byte(password)) == 1
}

// writeProxyAuthRequired 返回 407 并声明 Basic 认证
func writeProxyAuthRequired(conn net.Conn) {
	resp := "HTTP/1.1 407 Proxy Authentication Required\r\n" +
		"Proxy-Authenticate: Basic realm=\"tshell-tunnel\"\r\n" +
		"Content-Length: 0\r\n" +
		"Connection: close\r\n\r\n"
	_, _ = conn.Write([]byte(resp))
}

func handleHTTPTunnelProxyConnection(clientConn net.Conn, instance *HTTPTunnelInstance) {
	defer clientConn.Close()

	connID := fmt.Sprintf("%s_%d", clientConn.RemoteAddr().String(), time.Now().UnixNano())
	log.Printf("[HTTP-TUNNEL] 📥 新HTTP代理连接: %s -> %s (ID: %s)", clientConn.RemoteAddr(), clientConn.LocalAddr(), connID)

	// 检查多路复用会话是否可用
	if instance.Session.Load() == nil {
		log.Printf("[HTTP-TUNNEL] ❌ 多路复用会话不可用，关闭连接: %s", connID)
		return
	}

	// 读取HTTP请求（设置读超时防 slowloris：慢速请求头占满连接/goroutine）
	clientConn.SetReadDeadline(time.Now().Add(30 * time.Second))
	reader := bufio.NewReader(clientConn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		log.Printf("[HTTP-TUNNEL] 读取HTTP请求失败: %v", err)
		return
	}
	clientConn.SetReadDeadline(time.Time{})

	log.Printf("[HTTP-TUNNEL] 📋 HTTP请求:")
	log.Printf("[HTTP-TUNNEL]    - 方法: %s", req.Method)
	log.Printf("[HTTP-TUNNEL]    - URL: %s", req.URL.String())
	log.Printf("[HTTP-TUNNEL]    - Host: %s", req.Host)

	// 代理认证（配置了密码才校验）：HTTP 侧此前完全没有认证，端口对任何能连上的人开放
	if !checkTunnelProxyAuth(req, instance.Username, instance.Password) {
		log.Printf("[HTTP-TUNNEL] ⛔ 代理认证失败: %s", connID)
		writeProxyAuthRequired(clientConn)
		return
	}

	// 处理HTTP CONNECT方法 (用于HTTPS代理)
	if req.Method == http.MethodConnect {
		handleHTTPTunnelConnect(clientConn, req, instance, connID)
	} else {
		// 处理普通HTTP代理
		handleHTTPTunnelProxy(clientConn, req, instance, connID)
	}
}

// handleHTTPTunnelConnect 处理HTTP CONNECT方法（使用多路复用）
func handleHTTPTunnelConnect(clientConn net.Conn, req *http.Request, instance *HTTPTunnelInstance, connID string) {
	log.Printf("[HTTP-TUNNEL] 🔗 处理CONNECT请求: %s", req.Host)

	// 通过多路复用与客户端建立流连接
	session := instance.Session.Load()
	if session == nil {
		log.Printf("[HTTP-TUNNEL] ❌ 多路复用会话不可用: %s", connID)
		return
	}
	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[HTTP-TUNNEL] 创建多路复用流失败: %v", err)
		// 发送错误响应
		errorResponse := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		clientConn.Write([]byte(errorResponse))
		return
	}
	defer stream.Close()

	// 向流发送目标地址信息
	targetHost := req.Host
	if !strings.Contains(targetHost, ":") {
		targetHost = targetHost + ":443" // HTTPS默认端口
	}

	// 发送目标地址给客户端
	log.Printf("[HTTP-TUNNEL] 发送目标地址给Agent: %s", targetHost)
	_, err = stream.Write([]byte(targetHost + "\n"))
	if err != nil {
		log.Printf("[HTTP-TUNNEL] 发送目标地址失败: %v", err)
		errorResponse := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		clientConn.Write([]byte(errorResponse))
		return
	}

	// 等待客户端连接确认（用 io.ReadFull 读取完整2字节，避免短读误判为失败）
	buf := make([]byte, 2)
	_, err = io.ReadFull(stream, buf)
	if err != nil || string(buf) != "OK" {
		log.Printf("[HTTP-TUNNEL] 客户端连接失败: %v", err)
		errorResponse := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		clientConn.Write([]byte(errorResponse))
		return
	}

	// 发送CONNECT成功响应
	response := "HTTP/1.1 200 Connection Established\r\n\r\n"
	_, err = clientConn.Write([]byte(response))
	if err != nil {
		log.Printf("[HTTP-TUNNEL] 发送CONNECT响应失败: %v", err)
		return
	}

	log.Printf("[HTTP-TUNNEL] ✅ CONNECT隧道已建立: %s", req.Host)

	// 开始双向数据转发
	startHTTPTunnelBidirectionalForward(clientConn, stream, connID)
}

// handleHTTPTunnelProxy 处理普通HTTP代理（使用多路复用）
func handleHTTPTunnelProxy(clientConn net.Conn, req *http.Request, instance *HTTPTunnelInstance, connID string) {
	log.Printf("[HTTP-TUNNEL] 🌐 处理HTTP代理请求: %s", req.URL.String())

	// 通过多路复用与客户端建立流连接
	session := instance.Session.Load()
	if session == nil {
		log.Printf("[HTTP-TUNNEL] ❌ 多路复用会话不可用: %s", connID)
		return
	}
	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[HTTP-TUNNEL] 创建多路复用流失败: %v", err)
		// 发送错误响应
		errorResponse := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		clientConn.Write([]byte(errorResponse))
		return
	}
	defer stream.Close()

	// 解析目标URL
	targetURL := req.URL
	if targetURL.Scheme == "" {
		// 相对URL，需要添加scheme
		targetURL.Scheme = "http"
	}
	if targetURL.Host == "" {
		targetURL.Host = req.Host
	}

	// 构建目标地址
	targetHost := targetURL.Host
	if !strings.Contains(targetHost, ":") {
		if targetURL.Scheme == "https" {
			targetHost = targetHost + ":443"
		} else {
			targetHost = targetHost + ":80"
		}
	}

	// 发送目标地址给客户端
	log.Printf("[HTTP-TUNNEL] 发送目标地址给Agent: %s", targetHost)
	_, err = stream.Write([]byte(targetHost + "\n"))
	if err != nil {
		log.Printf("[HTTP-TUNNEL] 发送目标地址失败: %v", err)
		errorResponse := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		clientConn.Write([]byte(errorResponse))
		return
	}

	// 等待客户端连接确认（用 io.ReadFull 读取完整2字节，避免短读误判为失败）
	buf := make([]byte, 2)
	_, err = io.ReadFull(stream, buf)
	if err != nil || string(buf) != "OK" {
		log.Printf("[HTTP-TUNNEL] 客户端连接失败: %v", err)
		errorResponse := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		clientConn.Write([]byte(errorResponse))
		return
	}

	// 重新构建HTTP请求并发送给客户端
	err = forwardHTTPRequestToAgent(req, stream)
	if err != nil {
		log.Printf("[HTTP-TUNNEL] 转发HTTP请求到Agent失败: %v", err)
		errorResponse := "HTTP/1.1 502 Bad Gateway\r\n\r\n"
		clientConn.Write([]byte(errorResponse))
		return
	}

	log.Printf("[HTTP-TUNNEL] ✅ HTTP代理请求已转发")

	// 开始双向数据转发
	startHTTPTunnelBidirectionalForward(clientConn, stream, connID)
}

// startHTTPTunnelBidirectionalForward 开始HTTP隧道双向数据转发（类似SOCKS5）
func startHTTPTunnelBidirectionalForward(clientConn net.Conn, agentStream net.Conn, connID string) {
	log.Printf("[HTTP-TUNNEL] 📡 开始双向数据转发: %s", connID)

	// 使用缓冲池提高性能
	var wg sync.WaitGroup
	wg.Add(2)

	// 从客户端到Agent
	go func() {
		defer wg.Done()
		buf := httpBufferPool.Get()
		defer httpBufferPool.Put(buf)
		log.Printf("[HTTP-TUNNEL] 📤 开始转发: client -> agent")
		n, err := io.CopyBuffer(agentStream, clientConn, buf[:cap(buf)])
		log.Printf("[HTTP-TUNNEL] 📤 转发结束: client -> agent, 字节数: %d, %s", n, relayEndState(err))
		// 本方向(client->agent)结束:关闭自己的dst(agentStream)=对端src,解除对端Read阻塞
		agentStream.Close()
	}()

	// 从Agent到客户端
	go func() {
		defer wg.Done()
		buf := httpBufferPool.Get()
		defer httpBufferPool.Put(buf)
		log.Printf("[HTTP-TUNNEL] 📥 开始转发: agent -> client")
		n, err := io.CopyBuffer(clientConn, agentStream, buf[:cap(buf)])
		log.Printf("[HTTP-TUNNEL] 📥 转发结束: agent -> client, 字节数: %d, %s", n, relayEndState(err))
		// 本方向(agent->client)结束:关闭自己的dst(clientConn)=对端src
		clientConn.Close()
	}()

	wg.Wait()
	log.Printf("[HTTP-TUNNEL] ✅ 双向数据转发完成: %s", connID)
}

// forwardHTTPRequestToAgent 转发HTTP请求到Agent（P1-3: 请求体流式转发，
// 原实现将整个请求体读入 bytes.Buffer 再整体写出——大上传全量驻留内存且无大小上限）
func forwardHTTPRequestToAgent(req *http.Request, agentStream net.Conn) error {
	// 请求行 + 请求头仅占头部大小，可用小缓冲构建
	var headerData bytes.Buffer

	// 写入请求行
	headerData.WriteString(fmt.Sprintf("%s %s %s\r\n", req.Method, req.URL.RequestURI(), req.Proto))

	// 写入请求头
	req.Header.Write(&headerData)

	// 写入请求头结束标记
	headerData.WriteString("\r\n")

	// 发送头部到Agent
	if _, err := agentStream.Write(headerData.Bytes()); err != nil {
		return fmt.Errorf("发送请求头失败: %v", err)
	}

	// 请求体流式转发（不整体驻留内存）
	if req.Body != nil {
		defer req.Body.Close()
		buf := httpBufferPool.Get()
		defer httpBufferPool.Put(buf)
		if _, err := io.CopyBuffer(agentStream, req.Body, buf[:cap(buf)]); err != nil {
			return fmt.Errorf("转发请求体失败: %v", err)
		}
	}

	return nil
}

// GetHTTPTunnelStats 获取HTTP隧道统计信息（新架构）
func GetHTTPTunnelStats() map[string]interface{} {
	globalHTTPTunnelManager.mu.RLock()
	defer globalHTTPTunnelManager.mu.RUnlock()

	stats := map[string]interface{}{
		"total_tunnels":    len(globalHTTPTunnelManager.tunnels),
		"active_listeners": len(globalHTTPTunnelManager.listeners),
		"tunnel_instances": []map[string]interface{}{},
	}

	for key, instance := range globalHTTPTunnelManager.tunnels {
		sessionStatus := "disconnected"
		if instance.Session.Load() != nil {
			sessionStatus = "connected"
		}

		tunnelInfo := map[string]interface{}{
			"key":            key,
			"client_uid":     instance.ClientUID,
			"proxy_port":     instance.ServerPort,
			"agent_port":     instance.AgentPort,
			"status":         instance.Status,
			"session_status": sessionStatus,
			"created_at":     instance.CreatedAt.Format(time.RFC3339),
		}

		stats["tunnel_instances"] = append(stats["tunnel_instances"].([]map[string]interface{}), tunnelInfo)
	}

	return stats
}
