package api

import (
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"BackendTemplate/pkg/sendcommand"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"BackendTemplate/pkg/utils/logx"
)

// 终端会话管理
type TerminalSession struct {
	ID            string
	UID           string
	Conn          *websocket.Conn
	Process       *exec.Cmd
	StdinW        *os.File
	StdoutR       *os.File
	StderrR       *os.File
	IsActive      bool
	authenticated bool   // H1: 首条 init 消息 JWT 认证通过后置位
	authUser      string // 认证用户名（审计用）
	mu            sync.Mutex
}

var (
	// 引用外部的jwtManager变量（在users.go中定义）
	// jwtManager 将在初始化时被设置
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// H1: 同源校验，禁止跨站 WebSocket 连接（原恒 true）
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // 非浏览器客户端（curl 等）不携带 Origin
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			return u.Host == r.Host
		},
	}
	// webTermSessions 记录 Web 终端（/api/shell/:uid）会话，用于接收客户端交互式输出
	webTermSessions    = make(map[string]*TerminalSession)
	webTermSessionsMu  sync.RWMutex
)

// DeliverToWebTerminal 向指定 UID 的 Web 终端会话投递输出（由 BroadcastToXTermSessions 调用）
func DeliverToWebTerminal(uid string, data []byte) {
	webTermSessionsMu.RLock()
	defer webTermSessionsMu.RUnlock()
	for _, s := range webTermSessions {
		if s.UID == uid && s.IsActive && s.Conn != nil {
			s.Conn.WriteJSON(map[string]interface{}{
				"type": "output",
				"data": string(data),
			})
		}
	}
}

// WebTerminalHandler 专门用于Web终端的WebSocket处理器
func WebTerminalHandler(c *gin.Context) {
	uid := c.Param("uid")
	if uid == "" {
		errors.AbortWithError(c, errors.NewBadRequestError("缺少客户端UID"))
		return
	}

	// 升级HTTP连接为WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("WebSocket连接升级失败").WithInternalError(err))
		return
	}
	defer conn.Close()

	// 创建终端会话
	session := &TerminalSession{
		ID:       fmt.Sprintf("web_terminal_%s_%d", uid, time.Now().UnixNano()),
		UID:      uid,
		Conn:     conn,
		IsActive: true,
	}

	// 注册到 Web 终端输出投递表
	webTermSessionsMu.Lock()
	webTermSessions[session.ID] = session
	webTermSessionsMu.Unlock()
	defer func() {
		webTermSessionsMu.Lock()
		delete(webTermSessions, session.ID)
		webTermSessionsMu.Unlock()
	}()

	// 发送初始连接成功消息
	conn.WriteJSON(map[string]interface{}{
		"type":      "status",
		"connected": false,
		"message":   "Web终端连接已建立",
	})

	// 处理WebSocket消息
	for {
		var message map[string]interface{}
		err := conn.ReadJSON(&message)
		if err != nil {
			logx.Infof("读取WebSocket消息失败: %v\n", err)
			break
		}

		msgType, ok := message["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "init":
		// H1: 强制首条消息 JWT 认证 + 角色校验（原未携带 token 仅警告即继续 → 未授权控制任意客户端）
		if !session.authenticated {
			token, ok := message["token"].(string)
			if !ok || token == "" {
				conn.WriteJSON(map[string]interface{}{"type": "error", "message": "认证失败：缺少 token"})
				return
			}
			claims, err := security.GlobalJWTManager.ValidateToken(token)
			if err != nil || claims == nil {
				conn.WriteJSON(map[string]interface{}{"type": "error", "message": "认证失败，token无效"})
				return
			}
			if claims.Role != security.RoleAdmin && claims.Role != security.RoleOperator {
				logx.Infof("🚫 [WebTerminal] 用户 %s 角色 %s 无权使用 Web 终端\n", claims.Username, claims.Role)
				conn.WriteJSON(map[string]interface{}{"type": "error", "message": "权限不足：需要 admin/operator 角色"})
				return
			}
			session.authenticated = true
			session.authUser = claims.Username
			conn.WriteJSON(map[string]interface{}{
				"type":      "status",
				"connected": false,
				"message":   "认证成功",
			})
		}
	case "start":
		// 启动Shell（转发到客户端交互式终端）
		if !session.authenticated {
			conn.WriteJSON(map[string]interface{}{"type": "error", "message": "未认证"})
			return
		}
		startWebTerminalShell(session)
	case "stop":
		// 停止Shell
		if !session.authenticated {
			conn.WriteJSON(map[string]interface{}{"type": "error", "message": "未认证"})
			return
		}
		stopWebTerminalShell(session)
	case "input":
		// 处理用户输入（转发到客户端交互式终端）
		if !session.authenticated {
			conn.WriteJSON(map[string]interface{}{"type": "error", "message": "未认证"})
			return
		}
		data, ok := message["data"].(string)
		if ok {
			handleWebTerminalInput(session, data)
		}
	case "resize":
		// 处理终端尺寸变化（客户端 ConPTY 自动适配，忽略）
		cols, _ := message["cols"].(float64)
		rows, _ := message["rows"].(float64)
		handleWebTerminalResize(session, int(cols), int(rows))
	}
	}
}

// startWebTerminalShell 启动Web终端Shell（转发 INTERACTIVE_SHELL_START 到客户端）
func startWebTerminalShell(session *TerminalSession) {
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.Process != nil {
		// Shell已经运行
		session.Conn.WriteJSON(map[string]interface{}{
			"type":      "status",
			"connected": true,
			"message":   "Shell已经运行中",
		})
		return
	}

	// 通过 sendcommand 向客户端发送交互式终端启动命令（客户端 ConPTY/原生 PTY）
	sendcommand.SendCommand(session.UID, "interactive_shell_start")

	session.IsActive = true

	// 发送启动成功消息
	session.Conn.WriteJSON(map[string]interface{}{
		"type":      "status",
		"connected": true,
		"message":   "Shell已启动",
	})

	// 发送初始输出
	session.Conn.WriteJSON(map[string]interface{}{
		"type": "output",
		"data": "交互式终端已启动\r\n",
	})
}

// stopWebTerminalShell 停止Web终端Shell（转发 INTERACTIVE_SHELL_STOP 到客户端）
func stopWebTerminalShell(session *TerminalSession) {
	session.mu.Lock()
	defer session.mu.Unlock()

	// 通知客户端停止交互式终端（ConPTY/原生 PTY）
	sendcommand.SendCommand(session.UID, "interactive_shell_stop")

	session.IsActive = false

	// 发送停止消息
	session.Conn.WriteJSON(map[string]interface{}{
		"type":      "status",
		"connected": false,
		"message":   "Shell已停止",
	})
}

// handleWebTerminalInput 处理Web终端输入（转发到客户端交互式终端输入）
func handleWebTerminalInput(session *TerminalSession, input string) {
	if !session.IsActive {
		return
	}

	// 通过客户端交互式终端输入通道转发
	sendcommand.SendCommand(session.UID, "interactive_shell_input "+input)
}

// handleWebTerminalResize 处理Web终端尺寸变化
func handleWebTerminalResize(session *TerminalSession, cols, rows int) {
	// 可以在这里处理终端尺寸变化
	logx.Infof("Web终端尺寸变化: %dx%d\n", cols, rows)
}

// cleanupWebTerminalSession 清理Web终端会话
func cleanupWebTerminalSession(session *TerminalSession) {
	session.mu.Lock()
	defer session.mu.Unlock()

	session.IsActive = false
	logx.Infof("Web终端会话已清理: %s\n", session.UID)
}
