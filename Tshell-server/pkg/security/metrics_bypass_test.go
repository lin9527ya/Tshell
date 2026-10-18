package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// 隐藏访问启用时，Prometheus 抓取方无法完成隐藏入口握手（它只会带 token/UA）。
// /metrics 只在指标功能启用时放行；关闭时不放行 —— 否则扫描者能靠
// "伪装页 vs 404" 的差异发现这个特殊路径。

func newHiddenAccessRouter(t *testing.T) *gin.Engine {
	t.Helper()
	old := GlobalHiddenAccessManager
	GlobalHiddenAccessManager = &HiddenAccessManager{
		enabled:     true,
		accessPaths: []string{"/"},
		paramNames:  []string{"key"},
		secretKey:   "s3cret",
		sessionTTL:  time.Hour,
		sessionMap:  make(map[string]time.Time),
	}
	t.Cleanup(func() { GlobalHiddenAccessManager = old })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(HiddenAccessMiddleware())
	r.GET("/metrics", func(c *gin.Context) { c.String(http.StatusOK, "metrics-body") })
	return r
}

func getPath(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "203.0.113.9:1234"
	r.ServeHTTP(w, req)
	return w
}

func TestMetricsBypassHiddenAccessWhenEnabled(t *testing.T) {
	old := metricsPathBypass.Load()
	t.Cleanup(func() { SetMetricsPathBypass(old) })

	SetMetricsPathBypass(true)
	r := newHiddenAccessRouter(t)
	if w := getPath(r, "/metrics"); w.Code != http.StatusOK || w.Body.String() != "metrics-body" {
		t.Fatalf("启用指标时 /metrics 应放行，实际 %d %q", w.Code, w.Body.String())
	}

	// 关闭指标：/metrics 与其他未知路径一样被隐藏入口拦下（不再有特殊待遇）
	SetMetricsPathBypass(false)
	if w := getPath(r, "/metrics"); w.Code == http.StatusOK && w.Body.String() == "metrics-body" {
		t.Fatal("关闭指标时 /metrics 不应绕过隐藏访问")
	}
}
