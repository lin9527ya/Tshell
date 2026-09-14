package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ===== B11: 事件流 WebSocket =====
// /api/events/ws?uid=all 订阅实时 C2 事件（客户端上线/离线/命令输出等），
// 前端 Dashboard 实时展示，替代轮询。

var eventsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

// EventsConnectHandler 事件流 WebSocket 连接处理（JWT 首消息认证，同 XTerm 模式）
func EventsConnectHandler(c *gin.Context) {
	conn, err := eventsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("WebSocket升级失败").WithInternalError(err))
		return
	}
	defer conn.Close()

	// JWT 认证：首个消息携带 token
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, authData, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var authMsg struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(authData, &authMsg); err != nil || authMsg.Type != "auth" {
		return
	}
	// 双通道认证:页面刷新后 JS 内存 token 为空,凭 HttpOnly Cookie 恢复事件流
	// （WS 握手时浏览器自动携带 Cookie,JS 不可读但服务端可读）
	token := authMsg.Content
	if token == "" {
		token = apiCookieToken(c)
	}
	if _, err := security.GlobalJWTManager.ValidateToken(token); err != nil {
		conn.WriteJSON(gin.H{"type": "error", "content": "unauthorized"})
		return
	}
	conn.SetReadDeadline(time.Time{})
	conn.WriteJSON(gin.H{"type": "ok", "content": "events connected"})

	// 订阅事件总线
	ch := command.GlobalEventBus.Subscribe()
	defer command.GlobalEventBus.Unsubscribe(ch)

	// 写循环：推送事件（带写超时，浏览器断线不阻塞）
	writeTicker := time.NewTicker(20 * time.Second) // 心跳
	defer writeTicker.Stop()
	for {
		select {
		case ev := <-ch:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(ev); err != nil {
				return
			}
		case <-writeTicker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(gin.H{"type": "ping", "content": time.Now().Unix()}); err != nil {
				return
			}
		}
	}
}

// apiCookieToken 读取 HttpOnly access_token Cookie（WS/SSE 升级请求双通道认证用）
func apiCookieToken(c *gin.Context) string {
	if token, err := c.Cookie("access_token"); err == nil && token != "" {
		return token
	}
	return ""
}

var _ = log.Printf
