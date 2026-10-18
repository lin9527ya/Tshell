package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"BackendTemplate/pkg/security"

	"github.com/gin-gonic/gin"
)

// 指标端点带着 C2 的运行情报（在线 beacon 数、任务吞吐），访问规则必须真的生效：
// 配了 token 就得带 token，跨机访问默认拒绝。

func metricsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/metrics", MetricsHandler)
	return r
}

func withMetricsOptions(t *testing.T, o MetricsOptions) {
	t.Helper()
	old := metricsOptions
	metricsOptions = o
	t.Cleanup(func() { metricsOptions = old })
}

func doMetrics(t *testing.T, r *gin.Engine, remoteAddr, authz, authz2 string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	if authz != "" {
		req.Header.Set("Authorization", authz)
	}
	if authz2 != "" {
		req.Header.Set("Authorization2", authz2)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestMetricsTokenRequired 配置 token 后：无 token / 错 token 一律 403，正确 token 放行；
// 且本机来源也不能免 token（否则 token 形同虚设）。
func TestMetricsTokenRequired(t *testing.T) {
	withMetricsOptions(t, MetricsOptions{Enabled: true, Token: "s3cr3t-metrics"})
	r := metricsRouter()

	if w := doMetrics(t, r, "127.0.0.1:5555", "", ""); w.Code != http.StatusForbidden {
		t.Fatalf("本机无 token 状态码 = %d，期望 403（配了 token 就不该有免 token 的口子）", w.Code)
	}
	if w := doMetrics(t, r, "10.1.2.3:5555", "Bearer wrong", ""); w.Code != http.StatusForbidden {
		t.Fatalf("错误 token 状态码 = %d，期望 403", w.Code)
	}
	w := doMetrics(t, r, "10.1.2.3:5555", "Bearer s3cr3t-metrics", "")
	if w.Code != http.StatusOK {
		t.Fatalf("正确 token 状态码 = %d，期望 200（%s）", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tshell_") {
		t.Fatal("未返回指标内容")
	}
}

// TestMetricsLoopbackWithoutToken 未配 token：本机放行、跨机拒绝
func TestMetricsLoopbackWithoutToken(t *testing.T) {
	withMetricsOptions(t, MetricsOptions{Enabled: true})
	r := metricsRouter()

	if w := doMetrics(t, r, "127.0.0.1:5555", "", ""); w.Code != http.StatusOK {
		t.Fatalf("本机抓取状态码 = %d，期望 200", w.Code)
	}
	if w := doMetrics(t, r, "[::1]:5555", "", ""); w.Code != http.StatusOK {
		t.Fatalf("IPv6 回环状态码 = %d，期望 200", w.Code)
	}
	if w := doMetrics(t, r, "203.0.113.9:5555", "", ""); w.Code != http.StatusForbidden {
		t.Fatalf("跨机抓取状态码 = %d，期望 403（默认不暴露运行情报）", w.Code)
	}
	// 显式 allow_remote 才放行
	withMetricsOptions(t, MetricsOptions{Enabled: true, AllowRemote: true})
	if w := doMetrics(t, r, "203.0.113.9:5555", "", ""); w.Code != http.StatusOK {
		t.Fatalf("allow_remote 下跨机抓取状态码 = %d，期望 200", w.Code)
	}
}

// TestMetricsAdminSession 运维用浏览器/脚本查看：admin 会话放行，非 admin 不放行
func TestMetricsAdminSession(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := security.InitJWTManager(2048, time.Hour); err != nil {
		t.Fatalf("初始化 JWT 管理器失败: %v", err)
	}
	withMetricsOptions(t, MetricsOptions{Enabled: true, Token: "tok"})
	r := metricsRouter()

	adminTok, _, err := security.GlobalJWTManager.GenerateTokenPair("1", "alice", "test")
	if err != nil {
		t.Fatalf("签发 token 失败: %v", err)
	}
	// GenerateTokenPair 在无 DB 时默认 admin 角色
	if w := doMetrics(t, r, "203.0.113.9:5555", "", "Bearer "+adminTok); w.Code != http.StatusOK {
		t.Fatalf("admin 会话状态码 = %d，期望 200（%s）", w.Code, w.Body.String())
	}
	if w := doMetrics(t, r, "203.0.113.9:5555", "", "Bearer not-a-jwt"); w.Code != http.StatusForbidden {
		t.Fatalf("无效会话状态码 = %d，期望 403", w.Code)
	}
}

// TestMetricsXForwardedForCannotSpoofLoopback 转发头不能冒充本机：
// gin 的 c.ClientIP() 默认信任任意来源的 X-Forwarded-For，若用它做回环判定，
// 远程攻击者加一个 "X-Forwarded-For: 127.0.0.1" 就能读到运行情报。
func TestMetricsXForwardedForCannotSpoofLoopback(t *testing.T) {
	withMetricsOptions(t, MetricsOptions{Enabled: true})
	r := metricsRouter()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "203.0.113.9:4444"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Real-IP", "127.0.0.1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("伪造转发头的远程请求状态码 = %d，期望 403（%s）", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "tshell_") {
		t.Fatal("伪造转发头的远程请求读到了指标内容")
	}
}

// TestMetricsDisabled 关闭时返回 404 而不是指标内容
func TestMetricsDisabled(t *testing.T) {
	withMetricsOptions(t, MetricsOptions{Enabled: false})
	r := metricsRouter()

	w := doMetrics(t, r, "127.0.0.1:5555", "", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("关闭时状态码 = %d，期望 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "tshell_") {
		t.Fatal("关闭时仍返回了指标内容")
	}
}
