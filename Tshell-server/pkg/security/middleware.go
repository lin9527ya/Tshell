package security

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/gin-gonic/gin"
)

// ConfigGetter 接口，用于获取/回写配置而不引起循环导入
type ConfigGetter interface {
	GetLogLevel() string
	// PersistRateLimitConfig 把安全中心修改的限流参数写回配置文件，
	// 避免重启后回到默认值。
	PersistRateLimitConfig(loginRateLimit, apiRateLimit, blockMinutes int) error
}

// GlobalConfigGetter 全局配置获取器
var GlobalConfigGetter ConfigGetter

// 登录速率限制中间件
func LoginRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GlobalRateLimiter == nil || !GlobalRateLimiter.IsEnabled() {
			c.Next()
			return
		}
		ip := getRealIP(c)

		allowed, retryAfter := GlobalRateLimiter.CheckLoginRateLimit(ip)
		if !allowed {
			// 记录被阻止的访问
			logx.Infof("🚫 IP %s 登录请求被速率限制阻止，重试时间: %v\n", ip, retryAfter)

			c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Too many login attempts",
				"message":     "登录尝试过于频繁，请稍后再试",
				"retry_after": int(retryAfter.Seconds()),
				"blocked_ip":  ip,
			})
			c.Abort()
			return
		}

		c.Next()

		// 检查登录是否失败，如果失败记录
		if c.Writer.Status() == http.StatusUnauthorized {
			GlobalRateLimiter.RecordLoginFailure(ip)
		}
	}
}

// API速率限制中间件
func APIRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GlobalRateLimiter == nil || !GlobalRateLimiter.IsEnabled() {
			c.Next()
			return
		}
		ip := getRealIP(c)

		allowed, retryAfter := GlobalRateLimiter.CheckAPIRateLimit(ip)
		if !allowed {
			logx.Infof("🚫 IP %s API请求被速率限制阻止，重试时间: %v\n", ip, retryAfter)

			c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", GlobalRateLimiter.apiRateLimit))
			c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(retryAfter).Unix()))

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"message":     "API请求频率超过限制",
				"retry_after": int(retryAfter.Seconds()),
				"rate_limit":  GlobalRateLimiter.apiRateLimit,
				"window":      "1 minute",
			})
			c.Abort()
			return
		}

		// 添加速率限制相关头部
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", GlobalRateLimiter.apiRateLimit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", GlobalRateLimiter.apiRateLimit-getCurrentCount(ip)))

		c.Next()
	}
}

// WhitelistEnforced 是否启用 IP 白名单强制拦截。
// 【修复】默认 false（仅放行）：注册中间件但不破坏公网部署；
// 用户在安全设置中显式启用白名单强制模式后，非白名单 IP 将被拒绝。
var WhitelistEnforced = false

// IP白名单中间件
func IPWhitelistMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 未启用强制模式时放行（白名单仍用于限流豁免）
		if !WhitelistEnforced {
			c.Next()
			return
		}

		ip := getRealIP(c)

		// 如果IP不在白名单中且不是公开端点，则拒绝访问
		if !GlobalRateLimiter.ipWhitelist.IsWhitelisted(ip) {
			// 检查是否为公开端点
			if !isPublicEndpoint(c.Request.URL.Path) {
				logx.Infof("🚫 IP %s 不在白名单中，访问被拒绝: %s\n", ip, c.Request.URL.Path)
				c.JSON(http.StatusForbidden, gin.H{
					"error":   "Access denied",
					"message": "您的IP地址不在允许访问的白名单中",
					"ip":      ip,
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// 检查是否为公开端点
func isPublicEndpoint(path string) bool {
	publicEndpoints := []string{
		"/api/users/login",
		"/api/users/refresh",
		"/static/",
		"/",
		"/favicon.ico",
	}

	for _, endpoint := range publicEndpoints {
		if path == endpoint || (endpoint[len(endpoint)-1] == '/' &&
			len(path) >= len(endpoint) && path[:len(endpoint)] == endpoint) {
			return true
		}
	}

	return false
}

// 获取当前IP的请求计数
func getCurrentCount(ip string) int {
	return GlobalRateLimiter.GetCurrentCount(ip)
}

// 安全信息中间件（添加安全头）
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置安全相关的HTTP头
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		// 修复CSP策略，允许data: URI格式的图片，支持屏幕截图功能
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-eval' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; media-src 'self' data: blob:")

		// HSTS：只在**确实走 HTTPS** 的请求上下发。
		//
		// 为什么不能无条件发：HSTS 是浏览器强制的"以后都用 HTTPS 访问我"。
		// 在纯 HTTP 部署上撒这个头毫无作用（浏览器会忽略非 HTTPS 响应里的 HSTS），
		// 却会让"我们以为已启用 HSTS"这种错觉成立 —— 比不发更危险。
		// 反向代理终止 TLS 的场景，请求本身是 http，但 X-Forwarded-Proto 会告诉我们
		// 客户端看到的是 https，isSecureRequest 已处理这一点。
		if isSecureRequest(c) || ForceHSTS() {
			c.Header("Strict-Transport-Security", hstsHeaderValue)
		}

		// 移除可能泄露服务器信息的头部
		c.Header("Server", "Tshell")

		c.Next()
	}
}

// hstsHeaderValue 一年 + 子域 + preload。
// 不把 max-age 先调小"观察一下"：实践中那往往变成永远不调大。
const hstsHeaderValue = "max-age=31536000; includeSubDomains; preload"

// forceHSTS 安装期开关：由 InitSecurityHeaders 设置，供"反代不传
// X-Forwarded-Proto"这类部署由运维显式确认整条链路是 HTTPS 后打开。
var (
	hstsForceMu sync.RWMutex
	hstsForce   bool
)

// InitSecurityHeaders 由启动流程调用，注入配置（force_hsts）
func InitSecurityHeaders(forceHSTS bool) {
	hstsForceMu.Lock()
	hstsForce = forceHSTS
	hstsForceMu.Unlock()
}

// ForceHSTS 当前是否强制下发 HSTS
func ForceHSTS() bool {
	hstsForceMu.RLock()
	defer hstsForceMu.RUnlock()
	return hstsForce
}

// isSecureRequest 判断请求是否经 HTTPS（直连 TLS 或反代标记 X-Forwarded-Proto）。
// 与 pkg/api/users.go 中的同名函数语义一致；安全头与 Cookie 的 Secure 判定必须同源，
// 否则会出现"Cookie 当 HTTPS 设了 Secure，但 HSTS 没发"这种不一致。
func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	if i := strings.Index(proto, ","); i >= 0 {
		proto = proto[:i] // 多级代理会追加，取第一段（最靠近客户端的那一跳）
	}
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

// 请求日志中间件
func RequestLoggerMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// 从全局配置获取日志级别
		if GlobalConfigGetter != nil {
			logLevel := strings.ToLower(GlobalConfigGetter.GetLogLevel())

			// 请求日志（每请求一条）属于调试噪音：仅 debug 级别显示
			if logLevel != "debug" {
				return "" // 不输出日志
			}
		}

		return fmt.Sprintf("🌐 %s | %s | %d | %v | %s | %s | %s \"%s\"\n",
			param.TimeStamp.Format("2006/01/02 15:04:05"),
			param.ClientIP,
			param.StatusCode,
			param.Latency,
			param.Method,
			param.Path,
			param.ErrorMessage,
			param.Request.UserAgent(),
		)
	})
}

// 暴力破解防护中间件（针对特定敏感端点）
func BruteForceProtectionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if GlobalRateLimiter == nil || !GlobalRateLimiter.IsEnabled() {
			c.Next()
			return
		}
		ip := getRealIP(c)
		path := c.Request.URL.Path

		// 对敏感端点进行额外保护
		sensitiveEndpoints := []string{
			"/api/users/login",
			"/api/users/user_setting/ChangePassword",
		}

		isSensitive := false
		for _, endpoint := range sensitiveEndpoints {
			if path == endpoint {
				isSensitive = true
				break
			}
		}

		if isSensitive {
			// 对敏感端点使用更严格的速率限制
			allowed, retryAfter := GlobalRateLimiter.CheckLoginRateLimit(ip)
			if !allowed {
				logx.Infof("🛡️ IP %s 对敏感端点的暴力破解尝试被阻止: %s\n", ip, path)
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":         "Brute force protection activated",
					"message":       "检测到潜在的暴力破解攻击，访问已被限制",
					"retry_after":   int(retryAfter.Seconds()),
					"blocked_until": time.Now().Add(retryAfter).Format("2006-01-02 15:04:05"),
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// UTF-8编码中间件，确保JSON响应使用正确的字符编码
func UTF8EncodingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 拦截响应写入
		w := &responseWriter{
			ResponseWriter: c.Writer,
			c:              c,
		}
		c.Writer = w
		c.Next()
	}
}

// 自定义ResponseWriter包装器
type responseWriter struct {
	gin.ResponseWriter
	c *gin.Context
}

// Write方法拦截响应写入，确保设置正确的Content-Type
func (w *responseWriter) Write(data []byte) (int, error) {
	// 检查是否是JSON响应且未设置Content-Type
	contentType := w.Header().Get("Content-Type")
	if contentType == "" {
		// 检查请求路径是否为API路径
		if strings.HasPrefix(w.c.Request.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
	} else if strings.HasPrefix(contentType, "application/json") && !strings.Contains(contentType, "charset") {
		// 如果是JSON但没有charset，添加UTF-8
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}

	return w.ResponseWriter.Write(data)
}
