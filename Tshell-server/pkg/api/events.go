package api

import (
	"BackendTemplate/pkg/collab"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ===== B11: 事件流 WebSocket =====
// /api/events/ws?uid=all 订阅实时 C2 事件（客户端上线/离线/命令输出等），
// 前端 Dashboard 实时展示，替代轮询。

const (
	// eventsReadTimeout 读侧空闲上限（前端每 20s 收一次 ping 并回 pong）
	eventsReadTimeout = 90 * time.Second
	// eventsChatMaxLen 单条聊天消息上限（广播给所有操作员，防止刷屏）
	eventsChatMaxLen = 500
)

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
	claims, err := security.GlobalJWTManager.ValidateToken(token)
	if err != nil {
		conn.WriteJSON(gin.H{"type": "error", "content": "unauthorized"})
		return
	}
	conn.SetReadDeadline(time.Time{})
	conn.WriteJSON(gin.H{"type": "ok", "content": "events connected"})

	// 协作：事件流连接本身也是在线的证据（否则只开着一个页面不发请求的操作员会显示离线）
	collab.Global.Touch(claims.Username, claims.Role, c.ClientIP())

	// 订阅事件总线
	ch := command.GlobalEventBus.Subscribe()
	defer command.GlobalEventBus.Unsubscribe(ch)

	// 读循环：此前只用于首消息认证，之后不再读——团队频道的消息因此无处可去
	// （前端"操作员聊天"只能是本地 echo）。这里补上读侧：chat 广播给所有订阅者，
	// pong 用于保活。读失败即关闭连接，写循环随之退出。
	go func() {
		defer conn.Close()
		for {
			conn.SetReadDeadline(time.Now().Add(eventsReadTimeout))
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg struct {
				Type    string `json:"type"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "pong":
				// 保活
			case "chat":
				text := strings.TrimSpace(msg.Content)
				if text == "" {
					continue
				}
				if len(text) > eventsChatMaxLen {
					text = text[:eventsChatMaxLen] + "…"
				}
				command.PublishEvent(command.EventChat, "", map[string]interface{}{
					"operator": claims.Username,
					"message":  text,
				})
			}
		}
	}()

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
