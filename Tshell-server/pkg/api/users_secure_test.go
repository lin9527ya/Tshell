package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestIsSecureRequest 登录 Cookie 的 Secure 属性按请求判定。
// 原实现用全局 secureCookie（SetSecureCookieEnabled 全仓零调用）→ 永远 false，
// HTTPS 部署下 HttpOnly Cookie 会随明文请求发送；而 HTTP 部署下强制 Secure 又会让
// 浏览器拒绝保存 Cookie（登录即掉线）。
func TestIsSecureRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name   string
		tls    bool
		header string
		want   bool
	}{
		{"HTTP 直连", false, "", false},
		{"TLS 直连", true, "", true},
		{"反向代理 https", false, "https", true},
		{"反向代理 HTTPS（大小写不敏感）", false, "HTTPS", true},
		{"反向代理 http", false, "http", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tc.header != "" {
				req.Header.Set("X-Forwarded-Proto", tc.header)
			}
			c.Request = req
			if got := isSecureRequest(c); got != tc.want {
				t.Fatalf("isSecureRequest = %v，期望 %v", got, tc.want)
			}
		})
	}
}
