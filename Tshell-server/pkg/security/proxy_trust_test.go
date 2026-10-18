package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 审计日志里的 client_ip、在线操作员的 last_ip 都取自 gin 的 ClientIP()。
// gin 默认信任任意来源的 X-Forwarded-For，直连部署下远程请求可以随便声称自己是谁
// （伪造审计记录）。这里守住"只有可信代理的转发头才被采信"。

func clientIPRouter(t *testing.T, applyTrusted bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if applyTrusted {
		if err := ApplyTrustedProxies(r); err != nil {
			t.Fatalf("ApplyTrustedProxies 失败: %v", err)
		}
	}
	r.GET("/whoami", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	return r
}

func whoami(r *gin.Engine, remoteAddr, xff string) string {
	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Body.String()
}

func TestTrustedProxiesRejectSpoofedForwardedFor(t *testing.T) {
	r := clientIPRouter(t, true)

	// 公网直连 + 伪造转发头 → 采信 TCP 对端，不采信转发头
	if got := whoami(r, "203.0.113.9:4444", "10.0.0.5"); got != "203.0.113.9" {
		t.Fatalf("公网来源的转发头被采信了：记录为 %q，期望 TCP 对端 203.0.113.9", got)
	}
	// 可信代理（本机/私网）转发 → 采信转发头里的真实客户端 IP
	if got := whoami(r, "127.0.0.1:4444", "198.51.100.7"); got != "198.51.100.7" {
		t.Fatalf("可信代理的转发头未被采信：记录为 %q，期望 198.51.100.7", got)
	}
	if got := whoami(r, "10.1.2.3:4444", "198.51.100.8"); got != "198.51.100.8" {
		t.Fatalf("私网代理的转发头未被采信：记录为 %q，期望 198.51.100.8", got)
	}
	// 无转发头时始终是对端地址
	if got := whoami(r, "198.51.100.9:4444", ""); got != "198.51.100.9" {
		t.Fatalf("无转发头时记录为 %q，期望 198.51.100.9", got)
	}
}

// TestUntrustedDefaultsAreSpoofable 反证：不应用可信代理设置时确实可被伪造
// （即上面那条测试守的是一个真实存在的差异，不是 gin 的默认行为）
func TestUntrustedDefaultsAreSpoofable(t *testing.T) {
	r := clientIPRouter(t, false)
	if got := whoami(r, "203.0.113.9:4444", "10.0.0.5"); got != "10.0.0.5" {
		t.Fatalf("gin 默认行为已改变（%q），请复核 ApplyTrustedProxies 的必要性", got)
	}
}
