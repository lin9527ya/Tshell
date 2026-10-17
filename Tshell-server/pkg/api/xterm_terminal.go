package api

import (
	"BackendTemplate/pkg/api/communication"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/connection/dns"
	"BackendTemplate/pkg/connection/kcp"
	"BackendTemplate/pkg/connection/oss"
	"BackendTemplate/pkg/connection/tcp"
	ws "BackendTemplate/pkg/connection/websocket"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// XTermSession XTerm.js终端会话
type XTermSession struct {
	ID         string
	UID        string
	Conn       *websocket.Conn
	ClientConn interface{} // 底层 net.Conn 或 *websocket.Conn
	IsActive   bool
	Cols       uint16
	Rows       uint16
	mu         sync.Mutex
	recording  *os.File // C15: 会话录像文件
}

// openSessionRecording 创建会话录像文件
func openSessionRecording(uid string) *os.File {
	dir := "./sessions"
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return nil
	}
	path := fmt.Sprintf("%s/%s_%d.log", dir, filepath.Base(uid), time.Now().UnixNano()/1000)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	return f
}

// closeRecording 关闭会话录像
func (s *XTermSession) closeRecording() {
	if s.recording != nil {
		s.recording.Close()
		s.recording = nil
	}
}

// WindowSize 终端窗口大小
type WindowSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
	X    uint16 `json:"x"`
	Y    uint16 `json:"y"`
}

// XTermMessage XTerm消息结构
type XTermMessage struct {
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
	Size    *WindowSize `json:"size,omitempty"`
}

var (
	xtermSessions = make(map[string]*XTermSession)
	xtermMutex    sync.RWMutex
	xtermUpgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

// 初始化XTerm相关回调函数
func init() {
	// 设置各监听器的XTerm输出路由回调函数，使交互式终端输出(cmdType 36)
	// 在所有传输类型(TCP/WebSocket/KCP/HTTP)下都能路由到对应的XTerm会话
	tcp.BroadcastToXTermSessions = BroadcastToXTermSessions
	ws.BroadcastToXTermSessions = BroadcastToXTermSessions
	kcp.BroadcastToXTermSessions = BroadcastToXTermSessions
	communication.BroadcastToXTermSessions = BroadcastToXTermSessions
	oss.BroadcastToXTermSessions = BroadcastToXTermSessions  // B4 收敛后 OSS 回包走统一处理器，补注入
	dns.BroadcastToXTermSessions = BroadcastToXTermSessions  // B4 收敛后 DNS 回包走统一处理器，补注入
}

// BroadcastToXTermSessions 广播数据到指定UID的所有XTerm会话
func BroadcastToXTermSessions(uid string, data []byte) {
	xtermMutex.RLock()
	defer xtermMutex.RUnlock()

	// 只在数据长度较大时才输出日志，避免频繁的小数据包日志
	if len(data) > 10 {
		log.Printf("📡 [XTerm] 广播数据到 UID %s，数据长度: %d", uid, len(data))
	}

	broadcastCount := 0
	for _, session := range xtermSessions {
		if session.UID == uid && session.IsActive {
			session.SendOutput(data)
			broadcastCount++
		}
	}

	// 同时投递到 Web 终端（/api/shell/:uid）会话
	DeliverToWebTerminal(uid, data)

	if broadcastCount == 0 && len(data) > 10 {
		log.Printf("⚠️ [XTerm] 未找到 UID %s 的活跃会话", uid)
	}
}

// XTermConnectHandler XTerm.js WebSocket连接处理器
func XTermConnectHandler(c *gin.Context) {
	uid := c.Query("uid")
	log.Printf("🔌 [XTerm] 收到WebSocket连接请求, UID: %s", uid)

	// J6: uid 严格校验（原未校验 → 录像文件名 uid=../../../tmp/evil 可任意路径创建/追加文件）
	if err := errors.ValidateUID(uid); err != nil {
		log.Printf("❌ [XTerm] 非法客户端UID: %v", err)
		errors.AbortWithError(c, errors.NewBadRequestError("非法客户端UID"))
		return
	}

	// 升级WebSocket连接
	conn, err := xtermUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("WebSocket连接升级失败").WithInternalError(err))
		return
	}
	defer conn.Close()

	// JWT认证
	var authMessage XTermMessage
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	err = conn.ReadJSON(&authMessage)
	if err != nil {
		conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: "认证超时或数据格式错误",
		})
		return
	}

	if authMessage.Type != "auth" {
		conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: "需要认证信息",
		})
		return
	}

	token, _ := authMessage.Content.(string)
	if token == "" {
		// 双通道认证：页面刷新/新标签页后 JS 内存 token 为空（access token 不持久化），
		// 凭 HttpOnly Cookie 续接——WS 握手浏览器自动携带 Cookie，JS 不可读但服务端可读
		token = apiCookieToken(c)
	}
	if token == "" {
		conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: "认证令牌无效",
		})
		return
	}

	// 验证JWT token
	claims, err := security.GlobalJWTManager.ValidateToken(token)
	if err != nil {
		conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: fmt.Sprintf("认证失败: %v", err),
		})
		return
	}
	// 【RBAC修复】WS 端点不经过 RequireRole 中间件，需在握手时显式校验角色：
	// xterm 是给被控端的交互式 shell（与 /api/shell/:uid 同级能力），
	// auditor（只读）不可使用 —— 原来只验身份不验角色，只读角色可拿到被控端 shell。
	if claims.Role != security.RoleAdmin && claims.Role != security.RoleOperator {
		logx.Infof("🚫 [XTERM] 用户 %s 角色 %s 无权接入交互式终端\n", claims.Username, claims.Role)
		conn.WriteJSON(XTermMessage{Type: "error", Content: "forbidden"})
		return
	}

	conn.SetReadDeadline(time.Time{})

	// 创建XTerm会话
	session := &XTermSession{
		ID:       fmt.Sprintf("xterm_%s_%d", uid, time.Now().UnixNano()),
		UID:      uid,
		Conn:     conn,
		IsActive: true,
		Cols:     80,
		Rows:     24,
	}

	xtermMutex.Lock()
	xtermSessions[session.ID] = session
	xtermMutex.Unlock()

	// 发送认证成功
	conn.WriteJSON(XTermMessage{
		Type:    "auth_success",
		Content: "XTerm认证成功",
	})

	log.Printf("🖥️ [XTerm] 新会话已创建: %s for client %s", session.ID, uid)

	// 启动XTerm终端会话
	go session.handleXTermSession()

	// 处理WebSocket消息（带读超时：浏览器静默断线后 60s 无消息即判定死连接并清理）
	readDeadline := time.Now().Add(60 * time.Second)
	conn.SetReadDeadline(readDeadline)
	for {
		// 尝试读取原始消息
		msgType, msgData, err := conn.ReadMessage()
		if err != nil {
			// I5: 任何读错误（含普通断线 close）统一清理会话与录像 FD。
			// 原仅超时错误调用 cleanup：浏览器正常断线 → session 残留 + 录像文件句柄泄漏，
			// 直到下次输出写失败才懒清理（客户端无输出则永久泄漏）。
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("⚠️ [XTerm] 连接空闲超时（60s），判定浏览器已断线，清理会话并停止客户端终端: %s", session.UID)
			} else {
				log.Printf("❌ [XTerm] 读取消息失败，清理会话: %v", err)
			}
			session.cleanup()
			break
		}
		// 重置读超时
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// 处理不同类型的消息
		switch msgType {
		case websocket.TextMessage:
			// 文本消息，尝试解析JSON或作为原始输入处理
			msgStr := string(msgData)
			// 只在消息较长时输出日志，避免单字符输入的日志滥出
			if len(msgStr) > 5 {
				log.Printf("📝 [XTerm] 收到文本消息: %s", msgStr[:min(len(msgStr), 50)])
			}

			// 尝试解析为JSON
			var message XTermMessage
			if err := json.Unmarshal(msgData, &message); err == nil {
				// JSON消息
				session.handleMessage(message)
			} else {
				// 原始输入数据，直接作为用户输入处理
				msg := XTermMessage{
					Type:    "input",
					Content: msgStr,
				}
				session.handleMessage(msg)
			}

		case websocket.BinaryMessage:
			// 二进制消息，作为用户输入处理
			// 只在数据较大时输出日志
			if len(msgData) > 5 {
				log.Printf("📛 [XTerm] 收到二进制消息: %d bytes", len(msgData))
			}
			msg := XTermMessage{
				Type:    "binary",
				Content: string(msgData),
			}
			session.handleMessage(msg)
		}
	}

	// 清理会话
	session.cleanup()
}

// handleXTermSession 处理XTerm会话
func (s *XTermSession) handleXTermSession() {
	// 获取客户端连接类型
	connection.MuClientListenerType.Lock()
	listenerType, exists := connection.ClientListenerType[s.UID]
	connection.MuClientListenerType.Unlock()

	if !exists {
		s.Conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: "客户端连接类型未找到",
		})
		return
	}

	log.Printf("🔍 [XTerm] 客户端 %s 使用连接类型: %s", s.UID, listenerType)

	// 根据连接类型获取对应的客户端连接
	var clientConn interface{}
	var connExists bool

	switch listenerType {
	case "tcp", "tcp_forward", "http_forward", "https_forward":
		tcp.Mutex.Lock()
		clientConn, connExists = tcp.TCPClientManger[s.UID]
		tcp.Mutex.Unlock()
	case "websocket", "ws_forward":
		ws.MuClientManager.Lock()
		clientConn, connExists = ws.ClientManager[s.UID]
		ws.MuClientManager.Unlock()
	case "kcp", "kcp_forward":
		kcp.Mutex.Lock()
		clientConn, connExists = kcp.KCPClientManger[s.UID]
		kcp.Mutex.Unlock()
	case "http":
		// HTTP客户端无持久连接，通过命令队列轮询收发；只要客户端已注册即视为可连接
		clientConn = nil
		connExists = true
	case "https":
		// HTTPS客户端与http相同：无持久连接，经命令队列轮询收发
		clientConn = nil
		connExists = true
	default:
		s.Conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: fmt.Sprintf("不支持的连接类型(交互式终端支持 websocket/tcp/kcp/http): %s", listenerType),
		})
		return
	}

	if !connExists {
		s.Conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: fmt.Sprintf("客户端未连接 (类型: %s)", listenerType),
		})
		return
	}

	// 设置客户端连接（保持原始类型）
	s.ClientConn = clientConn

	// 发送启动交互式终端命令到客户端
	err := s.sendXTermCommand(command.INTERACTIVE_SHELL_START, []byte{})
	if err != nil {
		s.Conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: fmt.Sprintf("启动终端失败: %v", err),
		})
		return
	}

	// 发送初始终端大小
	s.setTerminalSize(s.Cols, s.Rows)

	// 发送shell启动成功消息给前端
	s.Conn.WriteJSON(XTermMessage{
		Type:    "shell_started",
		Content: "交互式终端已成功启动",
	})

	log.Printf("✅ [XTerm] 终端会话已启动: %s (连接类型: %s)", s.ID, listenerType)
}

// sendXTermCommand 发送XTerm命令到客户端（按客户端传输类型分发，复用sendcommand的加密方式）
func (s *XTermSession) sendXTermCommand(cmdType uint32, data []byte) error {
	// 构造命令数据
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, cmdType)
	byteToSend := append(cmdTypeBytes, data...)

	// 获取客户端连接类型
	connection.MuClientListenerType.Lock()
	listenerType, exists := connection.ClientListenerType[s.UID]
	connection.MuClientListenerType.Unlock()
	if !exists {
		return fmt.Errorf("客户端连接类型不存在: %s", s.UID)
	}

	switch listenerType {
	case "websocket", "ws_forward":
		ws.MuClientManager.Lock()
		conn, ok := ws.ClientManager[s.UID]
		ws.MuClientManager.Unlock()
		if !ok || conn == nil {
			return fmt.Errorf("WebSocket连接不存在或已关闭: %s", s.UID)
		}
		cmdBytes, err := encrypt.EncryptWithSessionKey(byteToSend, s.UID)
		if err != nil {
			// 【加密体系改造】加密失败直接返回
			return fmt.Errorf("会话密钥加密失败: %v", err)
		}
		cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)
		// 每连接写互斥：与 SendCommand 并发写时防止帧交错
		writeLock := ws.GetWriteLock(s.UID)
		writeLock.Lock()
		err = conn.WriteMessage(websocket.BinaryMessage, cmdBase64)
		writeLock.Unlock()
		if err != nil {
			log.Printf("❌ [XTerm] WebSocket发送命令失败: %v", err)
			return err
		}
		return nil

	case "tcp", "tcp_forward", "http_forward", "https_forward":
		tcp.Mutex.Lock()
		conn, ok := tcp.TCPClientManger[s.UID]
		tcp.Mutex.Unlock()
		if !ok || conn == nil {
			return fmt.Errorf("TCP连接不存在或已关闭: %s", s.UID)
		}
		// 每连接写互斥：与 SendCommand 并发写时防止帧交错
		writeLock := tcp.GetWriteLock(s.UID)
		writeLock.Lock()
		err := s.writeFramedEncrypted(conn, byteToSend)
		writeLock.Unlock()
		return err

	case "kcp", "kcp_forward":
		kcp.Mutex.Lock()
		conn, ok := kcp.KCPClientManger[s.UID]
		kcp.Mutex.Unlock()
		if !ok || conn == nil {
			return fmt.Errorf("KCP连接不存在或已关闭: %s", s.UID)
		}
		// 每连接写互斥：与 SendCommand 并发写时防止帧交错
		writeLock := kcp.GetWriteLock(s.UID)
		writeLock.Lock()
		err := s.writeFramedEncrypted(conn, byteToSend)
		writeLock.Unlock()
		return err

	case "http":
		// HTTP客户端通过命令队列轮询获取命令，加密在GetHttp投递时完成
		if !command.CommandQueues.AddCommand(s.UID, byteToSend) {
			return fmt.Errorf("命令队列已满（客户端 %s 积压过多，请稍后重试）", s.UID)
		}
		return nil
	case "https":
		// HTTPS客户端与http相同：经命令队列轮询获取命令
		if !command.CommandQueues.AddCommand(s.UID, byteToSend) {
			return fmt.Errorf("命令队列已满（客户端 %s 积压过多，请稍后重试）", s.UID)
		}
		return nil

	default:
		return fmt.Errorf("不支持的连接类型用于交互式终端: %s", listenerType)
	}
}

// writeFramedEncrypted 加密+Base64+4字节长度帧，写入流式连接（TCP/KCP通用）
func (s *XTermSession) writeFramedEncrypted(conn net.Conn, byteToSend []byte) error {
	cmdBytes, err := encrypt.EncryptWithSessionKey(byteToSend, s.UID)
	if err != nil {
		// 【加密体系改造】加密失败直接返回
		return fmt.Errorf("会话密钥加密失败: %v", err)
	}
	cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)
	cmdLenBytes := utils.WriteInt(len(cmdBase64))
	msgToSend := utils.BytesCombine(cmdLenBytes, cmdBase64)
	writer := bufio.NewWriter(conn)
	if _, err := writer.Write(msgToSend); err != nil {
		log.Printf("❌ [XTerm] 流式连接写入失败: %s, 错误: %v", s.UID, err)
		return err
	}
	if err := writer.Flush(); err != nil {
		log.Printf("❌ [XTerm] 流式连接刷新失败: %s, 错误: %v", s.UID, err)
		return err
	}
	return nil
}

// handleMessage 处理XTerm消息
func (s *XTermSession) handleMessage(message XTermMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.IsActive {
		return
	}

	switch message.Type {
	case "start_shell":
		// 处理启动shell命令
		s.startInteractiveShell()

	case "input":
		// 处理用户输入
		if input, ok := message.Content.(string); ok {
			// 检查是否是终端大小调整消息（以\x01开头）
			if len(input) > 0 && input[0] == '\x01' {
				// 解析终端大小数据
				var size WindowSize
				if err := json.Unmarshal([]byte(input[1:]), &size); err == nil {
					s.Cols = size.Cols
					s.Rows = size.Rows
					s.setTerminalSize(s.Cols, s.Rows)
					log.Printf("🔧 [XTerm] 终端大小调整: %dx%d", s.Cols, s.Rows)
				} else {
					log.Printf("⚠️ [XTerm] 解析终端大小失败: %v", err)
				}
			} else {
				// 普通用户输入 - 只在输入长度>1时输出日志（表示完整命令）
				if len(input) > 1 {
					log.Printf("📤 [XTerm] 发送完整命令: %q", input)
				}
				s.sendInput(input)
			}
		}

	case "resize":
		// 处理终端大小调整
		if message.Size != nil {
			s.Cols = message.Size.Cols
			s.Rows = message.Size.Rows
			s.setTerminalSize(s.Cols, s.Rows)
			log.Printf("🔧 [XTerm] 终端大小调整: %dx%d", s.Cols, s.Rows)
		}

	case "binary":
		// 处理二进制数据（如特殊键盘输入）
		if data, ok := message.Content.(string); ok {
			s.sendInput(data)
		}

	case "ping":
		// 心跳检测
		s.Conn.WriteJSON(XTermMessage{
			Type:    "pong",
			Content: time.Now().Unix(),
		})

	default:
		log.Printf("⚠️ [XTerm] 未知消息类型: %s", message.Type)
	}
}

// sendInput 发送输入到客户端
func (s *XTermSession) sendInput(input string) {
	if !s.IsActive {
		return
	}

	// 发送输入命令到客户端
	err := s.sendXTermCommand(command.INTERACTIVE_SHELL_INPUT, []byte(input))
	if err != nil {
		log.Printf("❌ [XTerm] 发送输入失败: %v", err)
		s.Conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: fmt.Sprintf("发送输入失败: %v", err),
		})
	}
}

// setTerminalSize 设置终端大小
func (s *XTermSession) setTerminalSize(cols, rows uint16) {
	if !s.IsActive {
		return
	}

	// 构造终端大小数据
	sizeData := WindowSize{
		Rows: rows,
		Cols: cols,
		X:    0,
		Y:    0,
	}

	sizeJSON, err := json.Marshal(sizeData)
	if err != nil {
		log.Printf("❌ [XTerm] 序列化终端大小失败: %v", err)
		return
	}

	// 发送终端大小调整消息（特殊格式：\x01 + JSON）
	message := append([]byte{0x01}, sizeJSON...)

	err = s.sendXTermCommand(command.INTERACTIVE_SHELL_INPUT, message)
	if err != nil {
		log.Printf("❌ [XTerm] 设置终端大小失败: %v", err)
	} else {
		log.Printf("🔧 [XTerm] 终端大小已设置: %dx%d", cols, rows)
	}
}

// SendOutput 发送输出到前端（供其他模块调用）
func (s *XTermSession) SendOutput(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// C15: 输出写入会话录像
	if s.recording != nil {
		s.recording.Write(data)
	}

	if !s.IsActive || s.Conn == nil {
		return
	}

	// 只在数据较大时输出日志，避免单字符输出的日志滥出
	if len(data) > 10 {
		log.Printf("📤 [XTerm] 发送输出数据: %s, 数据长度: %d", s.UID, len(data))
	}

	// 尝试直接发送二进制数据
	err := s.Conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		log.Printf("❌ [XTerm] 发送二进制数据失败: %v", err)

		// 如果二进制发送失败，尝试JSON格式
		message := XTermMessage{
			Type:    "output",
			Content: string(data),
		}
		err = s.Conn.WriteJSON(message)
		if err != nil {
			log.Printf("❌ [XTerm] 发送JSON数据也失败: %v", err)
			// 锁内清理（cleanupLocked 不重复加锁，避免死锁）
			s.cleanupLocked()
		}
	}
}

// startInteractiveShell 启动交互式shell（C15: 会话录像）
func (s *XTermSession) startInteractiveShell() {
	log.Printf("🚀 [XTerm] 正在启动交互式终端: %s", s.UID)

	// C15: 打开会话录像文件（./sessions/<uid>_<ts>.log）
	s.recording = openSessionRecording(s.UID)
	if s.recording != nil {
		log.Printf("📼 [XTerm] 会话录像已开启: %s", s.recording.Name())
	}

	// 发送启动交互式终端命令到客户端
	err := s.sendXTermCommand(command.INTERACTIVE_SHELL_START, []byte{})
	if err != nil {
		log.Printf("❌ [XTerm] 启动交互式终端失败: %v", err)
		s.closeRecording()
		s.Conn.WriteJSON(XTermMessage{
			Type:    "error",
			Content: fmt.Sprintf("启动终端失败: %v", err),
		})
		return
	}

	// 发送成功响应到前端
	s.Conn.WriteJSON(XTermMessage{
		Type:    "shell_started",
		Content: "交互式终端已成功启动",
	})

	log.Printf("✅ [XTerm] 交互式终端已启动: %s", s.UID)
}

// cleanup 清理会话
func (s *XTermSession) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
}

// cleanupLocked 会话清理（调用者需已持有 s.mu；SendOutput 等锁内路径必须调用本函数避免死锁）
func (s *XTermSession) cleanupLocked() {
	if !s.IsActive {
		return
	}

	s.IsActive = false

	// C15: 关闭会话录像
	s.closeRecording()

	// 发送停止命令到客户端（sendXTermCommand按传输类型分发，HTTP走队列、流式连接直接写；不依赖 s.mu）
	s.sendXTermCommand(command.INTERACTIVE_SHELL_STOP, []byte{})

	// 从会话管理器中移除
	xtermMutex.Lock()
	delete(xtermSessions, s.ID)
	xtermMutex.Unlock()

	log.Printf("🗑️ [XTerm] 会话已清理: %s", s.ID)
}

// GetXTermSession 获取XTerm会话（供其他模块调用）
func GetXTermSession(uid string) *XTermSession {
	xtermMutex.RLock()
	defer xtermMutex.RUnlock()

	for _, session := range xtermSessions {
		if session.UID == uid && session.IsActive {
			return session
		}
	}
	return nil
}

