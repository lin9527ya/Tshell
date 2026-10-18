// Package metrics Prometheus 指标：把 C2 的运行状态（在线 beacon、连接、队列、
// 任务吞吐、API 延迟）暴露成可抓取的文本端点，供容量规划与告警使用。
//
// 设计取舍：
//   - 标签基数受控：HTTP 用 gin 的 FullPath（路由模板，不是真实 URL），
//     连接按传输类型聚合而非按 uid —— 否则千级 beacon 的部署会把指标基数撑爆，
//     变成 Prometheus 侧的内存问题（指标系统最常见的自伤方式）。
//   - 现算类指标（在线数/连接数/队列积压）用 GaugeFunc / 自定义 Collector 在
//     抓取时求值（默认 15s 一次，代价可忽略），不靠后台任务维护一致性。
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// registry 独立注册表（不用 prometheus.DefaultRegisterer）：
// 只暴露明确登记的东西，避免间接依赖顺手注册的指标泄漏实现细节。
var registry = prometheus.NewRegistry()

// ---------- 计数器 / 直方图（业务侧主动记录） ----------

var (
	tasksTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "tshell_tasks_total",
		Help: "任务数（按类型与终态）",
	}, []string{"kind", "status"})
	taskDuration = promauto.With(registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tshell_task_duration_seconds",
		Help:    "任务从下发到终态的耗时",
		Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	}, []string{"kind"})

	httpRequests = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "tshell_http_requests_total",
		Help: "HTTP 请求数（route 为路由模板，不含真实参数）",
	}, []string{"method", "route", "status"})
	httpDuration = promauto.With(registry).NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tshell_http_request_duration_seconds",
		Help:    "HTTP 请求耗时",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"method", "route"})

	auditDroppedTotal = promauto.With(registry).NewCounter(prometheus.CounterOpts{
		Name: "tshell_audit_dropped_total",
		Help: "因落盘队列满而被丢弃的审计日志条数（审计缺口信号）",
	})
	securityBlockTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "tshell_security_block_total",
		Help: "安全拦截次数（按原因）",
	}, []string{"reason"})
	operatorActionsTotal = promauto.With(registry).NewCounterVec(prometheus.CounterOpts{
		Name: "tshell_operator_actions_total",
		Help: "操作员动作数（协作审计：谁在做什么）",
	}, []string{"action"})
)

// collectorsRegistered 保证采集器只注册一次（重复调用是安全的空操作）
var collectorsRegistered bool

// Register 注册运行时采集器（Go runtime / 进程）。main 启动时调用一次。
func Register() {
	if collectorsRegistered {
		return
	}
	collectorsRegistered = true
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// Handler 返回 /metrics 的 HTTP handler（Prometheus 文本格式）
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// Gatherer 暴露注册表（测试用）
func Gatherer() prometheus.Gatherer { return registry }

// ---------- 业务侧记录入口 ----------

// TaskCreated 任务建立（状态 dispatched）
func TaskCreated(kind string) {
	tasksTotal.WithLabelValues(kind, "dispatched").Inc()
}

// TaskFinished 任务到达终态并记录耗时（dur <= 0 时不记直方图，例如批量超时收敛）
func TaskFinished(kind, status string, dur time.Duration) {
	tasksTotal.WithLabelValues(kind, status).Inc()
	if dur > 0 {
		taskDuration.WithLabelValues(kind).Observe(dur.Seconds())
	}
}

// RecordHTTP 记录一次 HTTP 请求（route 必须是路由模板，避免基数爆炸）
func RecordHTTP(method, route string, status int, dur time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	httpRequests.WithLabelValues(method, route, statusClass(status)).Inc()
	httpDuration.WithLabelValues(method, route).Observe(dur.Seconds())
}

// AuditDropped 审计日志被丢弃（落盘队列满）
func AuditDropped(n int) {
	if n > 0 {
		auditDroppedTotal.Add(float64(n))
	}
}

// SecurityBlock 安全拦截（隐藏访问/限流/白名单等）
func SecurityBlock(reason string) {
	securityBlockTotal.WithLabelValues(reason).Inc()
}

// OperatorAction 操作员动作（锁/命令/批量/执行）
func OperatorAction(action string) {
	operatorActionsTotal.WithLabelValues(action).Inc()
}

// statusClass 把状态码归并成 2xx/3xx/4xx/5xx：标签组合更少，面板查询更简单
func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

// ---------- 现算类指标（抓取时求值） ----------

// SnapshotFuncs 采集回调集合：由 main.go 注入实际数据源，
// 避免 metrics 反向依赖业务包（database/command/connection）。
type SnapshotFuncs struct {
	// Beacons 返回 (在线数, 总数)
	Beacons func() (online, total int64)
	// Connections 返回 传输类型 -> 活跃连接数
	Connections func() map[string]int64
	// CommandQueues 返回 (全部队列积压总数, 单客户端最大积压)
	CommandQueues func() (depth, maxPerClient int64)
	// EventSubscribers 返回事件流订阅者数
	EventSubscribers func() int64
	// DBConnections 返回 (open, inUse, idle, waitCount)
	DBConnections func() (open, inUse, idle, waitCount int64)
}

// snapshotsBound 防止重复绑定（MustRegister 会 panic）
var snapshotsBound bool

// BindSnapshots 注册采集回调（数据源初始化完成后调用一次）
func BindSnapshots(f SnapshotFuncs) {
	if snapshotsBound {
		return
	}
	snapshotsBound = true

	gaugeFunc := func(name, help string, fn func() float64) {
		registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: name, Help: help}, fn))
	}
	if f.Beacons != nil {
		gaugeFunc("tshell_beacons_online", "当前在线 beacon 数", func() float64 {
			o, _ := f.Beacons()
			return float64(o)
		})
		gaugeFunc("tshell_beacons_total", "累计上线过的 beacon 数（含离线）", func() float64 {
			_, t := f.Beacons()
			return float64(t)
		})
	}
	if f.Connections != nil {
		registry.MustRegister(connectionsCollector{fn: f.Connections})
	}
	if f.CommandQueues != nil {
		gaugeFunc("tshell_command_queue_depth", "全部客户端下行命令队列的待发命令总数", func() float64 {
			d, _ := f.CommandQueues()
			return float64(d)
		})
		gaugeFunc("tshell_command_queue_max_per_client", "单客户端队列最长积压（背压预警）", func() float64 {
			_, m := f.CommandQueues()
			return float64(m)
		})
	}
	if f.EventSubscribers != nil {
		gaugeFunc("tshell_event_subscribers", "事件流 WebSocket 订阅者数", func() float64 {
			return float64(f.EventSubscribers())
		})
	}
	if f.DBConnections != nil {
		gaugeFunc("tshell_db_connections_open", "数据库打开连接数", func() float64 {
			o, _, _, _ := f.DBConnections()
			return float64(o)
		})
		gaugeFunc("tshell_db_connections_in_use", "数据库在用连接数", func() float64 {
			_, u, _, _ := f.DBConnections()
			return float64(u)
		})
		gaugeFunc("tshell_db_connections_idle", "数据库空闲连接数", func() float64 {
			_, _, i, _ := f.DBConnections()
			return float64(i)
		})
		gaugeFunc("tshell_db_wait_total", "等待数据库连接的累计次数（连接池不足信号）", func() float64 {
			_, _, _, w := f.DBConnections()
			return float64(w)
		})
	}
}

// connectionsDesc 活跃连接数（按传输类型）
var connectionsDesc = prometheus.NewDesc(
	"tshell_connections_active",
	"连接池中活跃连接数（按传输类型）",
	[]string{"protocol"}, nil,
)

// connectionsCollector 抓取时按传输类型现算连接数（标签值来自运行时数据，
// 无法用 GaugeVec 预建，故实现自定义 Collector）
type connectionsCollector struct {
	fn func() map[string]int64
}

func (c connectionsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- connectionsDesc
}

func (c connectionsCollector) Collect(ch chan<- prometheus.Metric) {
	for proto, n := range c.fn() {
		ch <- prometheus.MustNewConstMetric(connectionsDesc, prometheus.GaugeValue, float64(n), proto)
	}
}
