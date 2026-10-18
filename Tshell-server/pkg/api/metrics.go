package api

import (
	"BackendTemplate/pkg/metrics"
	"BackendTemplate/pkg/security"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// MetricsOptions /metrics 端点的访问策略（来自 config.metrics）
type MetricsOptions struct {
	Enabled     bool
	Token       string
	AllowRemote bool
}

// metricsOptions 由 main 在启动时注入（未注入时默认只允许本机）
var metricsOptions = MetricsOptions{Enabled: true}

// SetMetricsOptions 注入指标端点配置（main 启动时调用）
func SetMetricsOptions(o MetricsOptions) { metricsOptions = o }

// MetricsEnabled 指标端点是否启用
func MetricsEnabled() bool { return metricsOptions.Enabled }

// MetricsHandler /metrics（Prometheus 文本格式）。
//
// 指标里有 beacon 在线数、任务吞吐这类情报，不能像普通监控那样裸奔。访问规则：
//  1. 配置了 metrics.token 时必须带 Authorization: Bearer <token>（常量时间比较）；
//  2. 否则只允许回环来源（Prometheus 与本机同机部署）；
//  3. 或持 admin 角色的 JWT（用浏览器/运维脚本临时查看）。
//
// metrics.allow_remote 只是给"确实要跨机抓取"的部署留的口子，且此时必须配 token ——
// 否则等于把 C2 的运行情报暴露给任何能访问端口的人。
func MetricsHandler(c *gin.Context) {
	if !metricsOptions.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "指标端点未启用"})
		return
	}
	if !metricsAuthorized(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "指标端点需要授权（Bearer token 或 admin 会话）"})
		return
	}
	metrics.Handler().ServeHTTP(c.Writer, c.Request)
}

// metricsAuthorized 判定本次抓取是否放行
func metricsAuthorized(c *gin.Context) bool {
	// 1) 预共享 token（Prometheus bearer_token 或 scrape 配置的 authorization）
	if metricsOptions.Token != "" {
		got := bearerToken(c.GetHeader("Authorization"))
		if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(metricsOptions.Token)) == 1 {
			return true
		}
	}
	// 2) admin 会话
	if claims := adminClaimsFromRequest(c); claims != nil {
		return true
	}
	// 3) 无 token 时的来源判定：本机放行；跨机必须显式 allow_remote
	//    （配了 token 就不再有"本机免 token"的口子，否则 token 形同虚设）
	if metricsOptions.Token == "" {
		if isLoopbackIP(c.ClientIP()) {
			return true
		}
		return metricsOptions.AllowRemote
	}
	return false
}

// bearerToken 从 Authorization 头取 Bearer 值
func bearerToken(h string) string {
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}

// adminClaimsFromRequest 校验 Authorization2 / Cookie 里的 JWT 且角色为 admin
func adminClaimsFromRequest(c *gin.Context) *security.Claims {
	if security.GlobalJWTManager == nil {
		return nil
	}
	token := bearerToken(c.GetHeader("Authorization2"))
	if token == "" {
		token = apiCookieToken(c)
	}
	if token == "" {
		return nil
	}
	claims, err := security.GlobalJWTManager.ValidateToken(token)
	if err != nil || claims == nil {
		return nil
	}
	if claims.Role != security.RoleAdmin {
		return nil
	}
	return claims
}

// isLoopbackIP 判定来源是否为本机
func isLoopbackIP(ip string) bool {
	if ip == "" {
		return false
	}
	if ip == "localhost" {
		return true
	}
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}
