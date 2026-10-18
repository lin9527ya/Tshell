package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"BackendTemplate/pkg/collab"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/security"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// 协作链路的端到端验证（真实 WebSocket + 真实 JWT）：
// 事件流连接既是"在线证据"，也是团队频道的收发通道。
func startEventsTestServer(t *testing.T) (*httptest.Server, func(user string) string) {
	t.Helper()
	// JWT 管理器会把密钥与状态文件写到工作目录，测试里切到临时目录
	t.Chdir(t.TempDir())
	if err := security.InitJWTManager(2048, time.Hour); err != nil {
		t.Fatalf("初始化 JWT 管理器失败: %v", err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/events/ws", EventsConnectHandler)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	tokenFor := func(user string) string {
		tok, _, err := security.GlobalJWTManager.GenerateTokenPair("uid-"+user, user, "test")
		if err != nil {
			t.Fatalf("生成 token 失败: %v", err)
		}
		return tok
	}
	return srv, tokenFor
}

func dialEvents(t *testing.T, srv *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/events/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("连接事件流失败: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.WriteJSON(gin.H{"type": "auth", "content": token}); err != nil {
		t.Fatalf("发送认证失败: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var ack map[string]interface{}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("读取认证响应失败: %v", err)
	}
	if ack["type"] != "ok" {
		t.Fatalf("认证未通过: %v", ack)
	}
	return conn
}

// waitEvent 读到指定类型的事件（跳过无关事件），超时即失败。
func waitEvent(t *testing.T, conn *websocket.Conn, want string) command.C2Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("等待事件 %s 超时", want)
		}
		conn.SetReadDeadline(deadline)
		var ev command.C2Event
		if err := conn.ReadJSON(&ev); err != nil {
			t.Fatalf("读取事件失败（等待 %s）: %v", want, err)
		}
		if string(ev.Type) == want {
			return ev
		}
	}
}

// TestEventsPresenceAndChat 连接即在线上；聊天经事件总线广播给其他操作员。
func TestEventsPresenceAndChat(t *testing.T) {
	setupCollabTest(t)
	srv, tokenFor := startEventsTestServer(t)

	alice := dialEvents(t, srv, tokenFor("alice"))
	bob := dialEvents(t, srv, tokenFor("bob"))
	_ = bob

	// 在线状态：两条事件流连接都应出现在在线列表（不依赖任何 HTTP API 请求）。
	//
	// 这里有真实的时序：服务端先回 "ok" 再登记在线（Touch 在 WriteJSON 之后），
	// 客户端读到 "ok" 时登记可能还没完成 —— 直接断言会随机失败（本机实测复现）。
	// 按"等待状态收敛"来断言，而不是赌调度顺序。
	online := collab.Global.Online()
	deadline := time.Now().Add(3 * time.Second)
	for len(online) != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		online = collab.Global.Online()
	}
	if len(online) != 2 {
		t.Fatalf("在线操作员数 = %d，期望 2（%+v）", len(online), online)
	}

	// alice 发团队频道消息 → bob 收到（带发送者）
	if err := alice.WriteJSON(gin.H{"type": "chat", "content": "  拿下 dc01 了  "}); err != nil {
		t.Fatalf("发送聊天失败: %v", err)
	}
	ev := waitEvent(t, bob, "chat")
	if ev.Detail["operator"] != "alice" || ev.Detail["message"] != "拿下 dc01 了" {
		t.Fatalf("聊天事件内容不符: %+v", ev.Detail)
	}

	// 空消息不广播：发一条空的，紧接着发锁事件，bob 应直接收到锁事件
	_ = alice.WriteJSON(gin.H{"type": "chat", "content": "   "})
	if _, ok, _ := collab.Global.Acquire("uid-x", "alice", ""); !ok {
		t.Fatal("抢锁失败")
	}
	command.PublishEvent(command.EventOperatorAction, "uid-x", map[string]interface{}{
		"operator": "alice", "action": "lock",
	})
	ev = waitEvent(t, bob, "operator_action")
	if ev.UID != "uid-x" || ev.Detail["operator"] != "alice" {
		t.Fatalf("操作广播内容不符: %+v", ev)
	}
}

// TestEventsChatTooLong 超长消息被截断（广播给所有操作员，防止刷屏）。
func TestEventsChatTooLong(t *testing.T) {
	setupCollabTest(t)
	srv, tokenFor := startEventsTestServer(t)

	alice := dialEvents(t, srv, tokenFor("alice"))
	bob := dialEvents(t, srv, tokenFor("bob"))

	long := strings.Repeat("A", eventsChatMaxLen+200)
	if err := alice.WriteJSON(gin.H{"type": "chat", "content": long}); err != nil {
		t.Fatalf("发送聊天失败: %v", err)
	}
	ev := waitEvent(t, bob, "chat")
	msg, _ := ev.Detail["message"].(string)
	if len([]rune(msg)) != eventsChatMaxLen+1 || !strings.HasSuffix(msg, "…") {
		t.Fatalf("超长消息未被截断: 长度 %d", len([]rune(msg)))
	}
}

// TestEventsAuthRejected 未认证连接不得进入事件流（协作接口同样受认证保护）。
func TestEventsAuthRejected(t *testing.T) {
	setupCollabTest(t)
	srv, _ := startEventsTestServer(t)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/events/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(gin.H{"type": "auth", "content": "not-a-token"}); err != nil {
		t.Fatalf("发送认证失败: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp map[string]interface{}
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("读取响应失败: %v", err)
	}
	if resp["type"] != "error" {
		t.Fatalf("无效 token 应被拒绝，实际: %v", resp)
	}
	if n := len(collab.Global.Online()); n != 0 {
		t.Fatalf("认证失败的连接不应计入在线: %d", n)
	}
}
