package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestHTTPMetricsUseRouteTemplate 路由标签必须是模板而非真实 URL：
// 用真实 URL 会让每个 uid/任务 id 变成独立时间线，指标基数随规模线性膨胀。
func TestHTTPMetricsUseRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	r.GET("/api/tasks/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, id := range []string{"1", "2", "3"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/"+id, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("请求失败: %d", w.Code)
		}
	}

	// 模板标签计数为 3，真实 URL 标签不存在
	if n := testutil.CollectAndCount(httpRequests, "tshell_http_requests_total"); n != 1 {
		t.Fatalf("不同 id 应归并到同一路由标签，实际 %d 条时间线", n)
	}
	got := testutil.ToFloat64(httpRequests.WithLabelValues("GET", "/api/tasks/:id", "2xx"))
	if got != 3 {
		t.Fatalf("路由模板计数 = %v，期望 3（说明标签用了真实 URL 或未记录）", got)
	}
	if got := testutil.ToFloat64(httpRequests.WithLabelValues("GET", "/api/tasks/1", "2xx")); got != 0 {
		t.Fatalf("不应存在真实 URL 标签，实际 %v", got)
	}
}

// TestHTTPMetricsStatusClass 状态码归并到 2xx/4xx/5xx
func TestHTTPMetricsStatusClass(t *testing.T) {
	cases := map[int]string{200: "2xx", 301: "3xx", 401: "4xx", 409: "4xx", 503: "5xx"}
	for code, want := range cases {
		if got := statusClass(code); got != want {
			t.Errorf("statusClass(%d) = %s，期望 %s", code, got, want)
		}
	}
}

// TestRecordHTTPUnmatched 未匹配到路由的请求归入 unmatched（404 扫描不产生基数爆炸）
func TestRecordHTTPUnmatched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/wp-admin.php", nil))

	if got := testutil.ToFloat64(httpRequests.WithLabelValues("GET", "unmatched", "4xx")); got != 1 {
		t.Fatalf("未匹配路由计数 = %v，期望 1", got)
	}
}

// TestBusinessCounters 任务/审计/安全/协作计数可被记录并抓取
func TestBusinessCounters(t *testing.T) {
	TaskCreated("console")
	TaskFinished("console", "completed", 1500*time.Millisecond)
	TaskFinished("batch", "timeout", 0) // dur=0 不记直方图
	AuditDropped(3)
	SecurityBlock("hidden_access")
	OperatorAction("lock")

	if got := testutil.ToFloat64(tasksTotal.WithLabelValues("console", "dispatched")); got < 1 {
		t.Fatalf("任务创建计数 = %v", got)
	}
	if got := testutil.ToFloat64(tasksTotal.WithLabelValues("console", "completed")); got < 1 {
		t.Fatalf("任务完成计数 = %v", got)
	}
	if got := testutil.ToFloat64(auditDroppedTotal); got < 3 {
		t.Fatalf("审计丢弃计数 = %v，期望 >= 3", got)
	}
	if got := testutil.ToFloat64(securityBlockTotal.WithLabelValues("hidden_access")); got < 1 {
		t.Fatalf("安全拦截计数 = %v", got)
	}
	if got := testutil.ToFloat64(operatorActionsTotal.WithLabelValues("lock")); got < 1 {
		t.Fatalf("操作员动作计数 = %v", got)
	}
	// 直方图应有观测值（dur=0 的那次不记）
	if n := testutil.CollectAndCount(taskDuration, "tshell_task_duration_seconds"); n == 0 {
		t.Fatal("任务耗时直方图未记录")
	}
}

// TestHandlerExposesTextFormat /metrics 输出 Prometheus 文本格式且带自描述
func TestHandlerExposesTextFormat(t *testing.T) {
	Register() // 幂等
	TaskCreated("exec")

	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"# HELP tshell_tasks_total",
		"# TYPE tshell_tasks_total counter",
		`tshell_tasks_total{kind="exec",status="dispatched"}`,
		"go_goroutines", // Go runtime 采集器
	} {
		if !strings.Contains(body, want) {
			t.Errorf("指标输出缺少 %q", want)
		}
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q，期望 Prometheus 文本格式", ct)
	}
}

// TestBindSnapshots 现算类指标：抓取时求值，无需后台维护
func TestBindSnapshots(t *testing.T) {
	BindSnapshots(SnapshotFuncs{
		Beacons:          func() (int64, int64) { return 7, 42 },
		Connections:      func() map[string]int64 { return map[string]int64{"tcp": 3, "http": 2} },
		CommandQueues:    func() (int64, int64) { return 11, 4 },
		EventSubscribers: func() int64 { return 2 },
		DBConnections:    func() (int64, int64, int64, int64) { return 5, 1, 4, 9 },
	})
	// 重复绑定不应 panic（MustRegister 会）
	BindSnapshots(SnapshotFuncs{Beacons: func() (int64, int64) { return 1, 1 }})

	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := w.Body.String()

	for _, want := range []string{
		"tshell_beacons_online 7",
		"tshell_beacons_total 42",
		`tshell_connections_active{protocol="tcp"} 3`,
		`tshell_connections_active{protocol="http"} 2`,
		"tshell_command_queue_depth 11",
		"tshell_command_queue_max_per_client 4",
		"tshell_event_subscribers 2",
		"tshell_db_connections_open 5",
		"tshell_db_wait_total 9",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("指标输出缺少 %q", want)
		}
	}
}
