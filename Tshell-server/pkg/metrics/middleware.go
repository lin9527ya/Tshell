package metrics

import (
	"BackendTemplate/pkg/utils/logx"
	"time"

	"github.com/gin-gonic/gin"
)

// slowRequestThreshold 慢请求阈值：超过则单独告警（便于定位"页面卡住"的接口）
const slowRequestThreshold = 2 * time.Second

// Middleware 记录每次请求的计数与耗时，并对慢请求打日志。
//
// 路由标签用 c.FullPath()（路由模板，如 /api/tasks/:id）：真实 URL 会把每个
// beacon 的 uid 变成一个独立时间线，指标基数随规模线性膨胀 —— 这是 Prometheus
// 使用中最常见的自伤方式。未匹配到路由（404）归入 unmatched。
//
// 放在中间件链靠前的位置：包含认证与限流耗时，才是用户实际感受到的延迟。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		dur := time.Since(start)

		route := c.FullPath()
		RecordHTTP(c.Request.Method, route, c.Writer.Status(), dur)

		if dur >= slowRequestThreshold {
			// 慢请求日志：带 request_id 便于与错误响应关联
			logx.Warnf("⏱️ 慢请求 %s %s 路由=%s 状态=%d 耗时=%.2fs request_id=%s 用户=%s\n",
				c.Request.Method, c.Request.URL.Path, route, c.Writer.Status(),
				dur.Seconds(), c.GetString("request_id"), c.GetString("username"))
		}
	}
}
