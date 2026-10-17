package api

import (
	"BackendTemplate/pkg/database"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequireTunnelAuth socks5/http 的端口直接对外提供服务，必须配凭据；
// tcp/udp 是按目标地址的定向转发，不涉及。
func TestRequireTunnelAuth(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		user     string
		pass     string
		wantErr  bool
	}{
		{"socks5 无凭据", "socks5", "", "", true},
		{"socks5 只有用户名", "socks5", "op", "", true},
		{"socks5 只有密码", "socks5", "", "p", true},
		{"socks5 完整凭据", "socks5", "op", "p", false},
		{"http 无凭据", "http", "", "", true},
		{"http 完整凭据", "http", "op", "p", false},
		{"类型大小写", "SOCKS5", "", "", true},
		{"tcp 不需要", "tcp", "", "", false},
		{"udp 不需要", "udp", "", "", false},
	}
	for _, c := range cases {
		err := requireTunnelAuth(c.protocol, c.user, c.pass)
		if c.wantErr && err == nil {
			t.Errorf("%s: 期望拒绝，实际通过", c.name)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%s: 期望通过，实际 %v", c.name, err)
		}
	}
}

// TestCreateTunnelProxyRequiresAuth 创建入口同样拦截：socks5/http 缺凭据返回 400 且不入库
func TestCreateTunnelProxyRequiresAuth(t *testing.T) {
	setupMaskTestDB(t)

	database.Engine.Insert(&database.Clients{Uid: "uid-tunnel", Online: "1", Listener: "tcp"})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/tunnel-proxy/", CreateTunnelProxy)

	// 隧道接口的响应是 {message} 或 {error}（HTTP 状态码表达结果），与监听器接口不同，
	// 故不复用 postJSON
	create := func(protocol, user, pass string, port int) (int, string) {
		body := fmt.Sprintf(`{"client_uid":"uid-tunnel","protocol":%q,"server_port":%d,"username":%q,"password":%q}`,
			protocol, port, user, pass)
		req := httptest.NewRequest(http.MethodPost, "/tunnel-proxy/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var resp struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Error != "" {
			return w.Code, resp.Error
		}
		return w.Code, resp.Message
	}

	// socks5 缺凭据 → 拒绝且不落库
	if st, _ := create("socks5", "", "", 19001); st != 400 {
		t.Errorf("socks5 无凭据应返回 400，实际 %d", st)
	}
	var n int64
	n, _ = database.Engine.Where("server_port = ?", 19001).Count(&database.TunnelProxy{})
	if n != 0 {
		t.Errorf("被拒的隧道不应入库，实际 %d 条", n)
	}

	// socks5 带凭据 → 允许
	if st, msg := create("socks5", "op", "s3cret", 19002); st != 200 {
		t.Errorf("socks5 带凭据应成功，实际 %d %s", st, msg)
	}
	// tcp 不需要凭据 → 允许
	if st, msg := create("tcp", "", "", 19003); st != 200 {
		t.Errorf("tcp 不需要凭据，应成功，实际 %d %s", st, msg)
	}
}

// TestCheckTunnelPortAvailable 端口校验必须覆盖 Agent 端口（端口+1000）并交叉两张表：
// 原实现只校验 server_port 唯一，于是「端口 5000 的隧道」和「端口 6000 的隧道」会抢 6000，
// 新表与旧版 Socks5 表还能在同一端口上各建一条，直到 bind 才撞车。
func TestCheckTunnelPortAvailable(t *testing.T) {
	setupMaskTestDB(t)

	// 新表：已有隧道占用 5000（其 Agent 端口为 6000）
	database.Engine.Insert(&database.TunnelProxy{ClientUID: "u", Protocol: "tcp", ServerPort: 5000})
	// 旧表：已有代理占用 7000（Agent 端口 8000）
	database.Engine.Insert(&database.Socks5{UID: "u", Type: "socks5", Socks5port: "7000"})

	cases := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"本端口被新表占用", 5000, true},
		{"本端口是新表记录的 Agent 端口", 6000, true},
		{"本端口的 Agent 端口被新表占用", 4000, true},
		{"本端口被旧表占用", 7000, true},
		{"本端口是旧表记录的 Agent 端口", 8000, true},
		{"本端口的 Agent 端口被旧表占用", 6000, true},
		{"无冲突端口", 9000, false},
		{"Agent 端口超过 65535", 65000, true},
	}
	for _, c := range cases {
		err := checkTunnelPortAvailable(c.port, 0, 0)
		if c.wantErr && err == nil {
			t.Errorf("%s: 端口 %d 期望冲突，实际通过", c.name, c.port)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%s: 端口 %d 期望通过，实际 %v", c.name, c.port, err)
		}
	}

	// 更新自身记录时应排除自己
	var existing database.TunnelProxy
	database.Engine.Where("server_port = ?", 5000).Get(&existing)
	if err := checkTunnelPortAvailable(5000, existing.ID, 0); err != nil {
		t.Errorf("更新时排除自身后应通过，实际 %v", err)
	}

	// 旧表更新时排除自身
	var s5 database.Socks5
	database.Engine.Where("socks5port = ?", "7000").Get(&s5)
	if err := checkTunnelPortAvailable(7000, 0, s5.ID); err != nil {
		t.Errorf("旧表更新排除自身后应通过，实际 %v", err)
	}
}
