package security

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// HSTS 的判定规则：只在**确实走 HTTPS** 的请求上下发。
//
// 无条件下发是有害的：HSTS 是浏览器强制的"以后都用 HTTPS 访问我"，
// 浏览器会忽略非 HTTPS 响应里的 HSTS —— 于是纯 HTTP 部署上它毫无作用，
// 却让人以为"已经启用 HSTS 了"。宁可明确地不发。
func TestHSTSOnlyOnSecureRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitSecurityHeaders(false)
	defer InitSecurityHeaders(false)

	r := gin.New()
	r.Use(SecurityHeadersMiddleware())
	r.GET("/", func(c *gin.Context) { c.String(200, "ok") })

	probe := func(forwardedProto string) string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		if forwardedProto != "" {
			req.Header.Set("X-Forwarded-Proto", forwardedProto)
		}
		r.ServeHTTP(w, req)
		return w.Header().Get("Strict-Transport-Security")
	}

	// 纯 HTTP：不发
	if v := probe(""); v != "" {
		t.Errorf("纯 HTTP 请求不应下发 HSTS（浏览器会忽略，只会造成\"已启用\"的错觉），实际 %q", v)
	}

	// 反代标记 https：发
	if v := probe("https"); v == "" {
		t.Error("X-Forwarded-Proto: https 时应下发 HSTS")
	} else if v != hstsHeaderValue {
		t.Errorf("HSTS 值应为 %q，实际 %q", hstsHeaderValue, v)
	}
}

// 多级代理会把 X-Forwarded-Proto 追加成 "https, http"：必须取第一段
// （最靠近客户端的那一跳），否则会被后面的 http 覆盖判定
func TestHSTSForwardedProtoChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	InitSecurityHeaders(false)
	defer InitSecurityHeaders(false)

	r := gin.New()
	r.Use(SecurityHeadersMiddleware())
	r.GET("/", func(c *gin.Context) { c.String(200, "ok") })

	for _, c := range []struct {
		proto string
		want  bool
	}{
		{"https", true},
		{"HTTPS", true},
		{"https, http", true},
		{"http", false},
		{"", false},
		{"  https  ", true},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		if c.proto != "" {
			req.Header.Set("X-Forwarded-Proto", c.proto)
		}
		r.ServeHTTP(w, req)
		got := w.Header().Get("Strict-Transport-Security") != ""
		if got != c.want {
			t.Errorf("X-Forwarded-Proto=%q 时期望发 HSTS=%v，实际 %v", c.proto, c.want, got)
		}
	}
}

// force_hsts=true 时纯 HTTP 也要发：用于"反代不传 X-Forwarded-Proto"的部署，
// 由运维确认整条链路确实是 HTTPS 后手动打开
func TestHSTSForceFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	defer InitSecurityHeaders(false)

	r := gin.New()
	r.Use(SecurityHeadersMiddleware())
	r.GET("/", func(c *gin.Context) { c.String(200, "ok") })

	probe := func() string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		return w.Header().Get("Strict-Transport-Security")
	}

	InitSecurityHeaders(false)
	if v := probe(); v != "" {
		t.Errorf("未强制时纯 HTTP 不应发 HSTS，实际 %q", v)
	}
	InitSecurityHeaders(true)
	if v := probe(); v == "" {
		t.Error("force_hsts=true 时纯 HTTP 也应下发 HSTS")
	}
}

// 其它安全头与 HSTS 互不影响（别因为改 HSTS 把 CSP 弄丢了）
func TestSecurityHeadersStillPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeadersMiddleware())
	r.GET("/", func(c *gin.Context) { c.String(200, "ok") })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	for _, h := range []string{
		"X-Frame-Options", "X-Content-Type-Options", "Referrer-Policy", "Content-Security-Policy",
	} {
		if w.Header().Get(h) == "" {
			t.Errorf("安全头 %s 缺失", h)
		}
	}
}
