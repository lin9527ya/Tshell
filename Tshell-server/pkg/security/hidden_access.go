package security

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/metrics"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// HiddenAccessManager 隐藏访问管理器
type HiddenAccessManager struct {
	enabled     bool
	accessPaths []string             // 支持的访问路径
	paramNames  []string             // 支持的参数名
	secretKey   string               // 访问密钥
	sessionTTL  time.Duration        // 会话有效期
	sessionMap  map[string]time.Time // IP -> 最后访问时间
	mu          sync.RWMutex         // 保护 sessionMap 的读写锁
}

// GlobalHiddenAccessManager 全局隐藏访问管理器
var GlobalHiddenAccessManager *HiddenAccessManager

// InitHiddenAccessManager 初始化隐藏访问管理器
func InitHiddenAccessManager(enabled bool, accessPaths []string, paramNames []string, secretKey string, sessionTTL int) {
	GlobalHiddenAccessManager = &HiddenAccessManager{
		enabled:     enabled,
		accessPaths: accessPaths,
		paramNames:  paramNames,
		secretKey:   secretKey,
		sessionTTL:  time.Duration(sessionTTL) * time.Hour,
		sessionMap:  make(map[string]time.Time),
	}

	// 设置默认值
	if len(GlobalHiddenAccessManager.accessPaths) == 0 {
		GlobalHiddenAccessManager.accessPaths = []string{"/"}
	}
	if len(GlobalHiddenAccessManager.paramNames) == 0 {
		GlobalHiddenAccessManager.paramNames = []string{"key"}
	}
	if GlobalHiddenAccessManager.sessionTTL == 0 {
		GlobalHiddenAccessManager.sessionTTL = 2 * time.Hour
	}

	// 如果没有设置访问密钥，自动生成一个
	if enabled && secretKey == "" {
		GlobalHiddenAccessManager.secretKey = generateRandomKey()
		logx.Infof("🔑 自动生成隐藏访问密钥: %s\n", GlobalHiddenAccessManager.secretKey)
	}

	if enabled {
		logx.Infof("🔒 隐藏访问保护已启用\n")
		// 【安全修复】不打印明文密钥：任何能读到服务端日志的人不应直接获得密钥
		keyPreview := "******"
		if len(GlobalHiddenAccessManager.secretKey) > 4 {
			keyPreview = GlobalHiddenAccessManager.secretKey[:4] + "******"
		}
		logx.Infof("🗝️ 访问密钥: %s（前4位预览，完整密钥请通过安全设置查看）\n", keyPreview)
		logx.Infof("🛤️ 支持的访问路径: %v\n", GlobalHiddenAccessManager.accessPaths)
		logx.Infof("🔗 支持的参数名: %v\n", GlobalHiddenAccessManager.paramNames)
		logx.Infof("⏰ 会话有效期: %v\n", GlobalHiddenAccessManager.sessionTTL)

		// 打印示例访问链接（【安全修复】密钥部分掩码显示）
		logx.Println("📝 示例访问链接（密钥已掩码）:")
		for _, path := range GlobalHiddenAccessManager.accessPaths {
			for _, param := range GlobalHiddenAccessManager.paramNames {
				logx.Infof("   %s?%s=%s\n", path, param, keyPreview)
			}
		}
	}
}

// generateRandomKey 生成随机访问密钥
func generateRandomKey() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// HiddenAccessMiddleware 隐藏访问中间件
func HiddenAccessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 如果没有启用隐藏访问保护，直接通过
		if GlobalHiddenAccessManager == nil || !GlobalHiddenAccessManager.enabled {
			c.Next()
			return
		}

		clientIP := getRealIP(c)
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 检查是否是API请求（API请求需要JWT认证，不受隐藏访问影响）
		if strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}

		// Prometheus 指标端点：抓取方无法完成隐藏入口握手（它只会带 token/UA），
		// 且该端点自带访问控制（见 api.MetricsHandler）。仅在指标功能启用时绕过，
		// 关闭时不绕过 —— 否则扫描者能靠 404/伪装页的差异发现这个特殊路径。
		if metricsPathBypass.Load() && path == "/metrics" {
			c.Next()
			return
		}

		// Delivery Hub 公开投递路径 /d/{key}/... 不走隐藏入口（自身可配 token/UA）
		// 【修复】精确匹配 /d/ 前缀段：原 Contains 匹配过宽（如 /admin/foo/d/... 也绕过）
		if path == "/d/" || strings.HasPrefix(path, "/d/") {
			c.Next()
			return
		}

		// 检查是否是静态资源请求
		if strings.HasPrefix(path, "/static/") ||
			strings.HasPrefix(path, "/css/") ||
			strings.HasPrefix(path, "/js/") ||
			strings.HasPrefix(path, "/img/") ||
			path == "/favicon.ico" {
			// 只有已认证的会话才能访问静态资源
			if !GlobalHiddenAccessManager.isSessionValid(clientIP) {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
			c.Next()
			return
		}

		// 检查根路径访问
		if GlobalHiddenAccessManager.isValidAccessPath(path) {
			// 检查是否提供了正确的访问密钥
			if GlobalHiddenAccessManager.checkAccessKey(c) {
				// 访问密钥正确，记录会话并允许访问
				GlobalHiddenAccessManager.recordSession(clientIP)
				logx.Infof("🔓 隐藏访问认证成功: IP=%s, Path=%s\n", clientIP, path)
				c.Next()
				return
			}

			// 检查是否已有有效会话
			if GlobalHiddenAccessManager.isSessionValid(clientIP) {
				c.Next()
				return
			}

			// J5: 密钥尝试超限的 IP 直接 404（不产生会话、不继续消耗资源）
			if hamIsAccessLimited(clientIP) {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}

			// 没有正确的密钥且无有效会话，返回404
			GlobalHiddenAccessManager.handleUnauthorizedAccess(c, clientIP, path, query)
			return
		}

		// 其他路径检查是否有有效会话
		if !GlobalHiddenAccessManager.isSessionValid(clientIP) {
			GlobalHiddenAccessManager.handleUnauthorizedAccess(c, clientIP, path, query)
			return
		}

		c.Next()
	}
}

// recordSession 记录有效会话
func (ham *HiddenAccessManager) recordSession(ip string) {
	ham.mu.Lock()
	defer ham.mu.Unlock()
	ham.sessionMap[ip] = time.Now()
}

// isSessionValid 检查会话是否有效
func (ham *HiddenAccessManager) isSessionValid(ip string) bool {
	ham.mu.Lock()
	defer ham.mu.Unlock()

	if lastAccess, exists := ham.sessionMap[ip]; exists {
		// 使用配置的会话有效期
		if time.Since(lastAccess) < ham.sessionTTL {
			// 更新最后访问时间
			ham.sessionMap[ip] = time.Now()
			return true
		}
		// 会话过期，删除记录
		delete(ham.sessionMap, ip)
	}
	return false
}

// isValidAccessPath 检查是否为有效的访问路径
func (ham *HiddenAccessManager) isValidAccessPath(path string) bool {
	for _, validPath := range ham.accessPaths {
		if path == validPath {
			return true
		}
	}
	// 默认也检查一些常见的页面路径
	if path == "/login" || path == "/dashboard" {
		return true
	}
	return false
}

// checkAccessKey 检查访问密钥（常数时间比较，防时序侧信道）
// J5: 独立限流状态（每 IP 时间窗内最多 failLimit 次密钥尝试，防止弱密钥在线猜解）
type accessLimitEntry struct {
	count    int
	windowAt time.Time
}

var accessLimitMu sync.Mutex
var accessLimitMap = make(map[string]*accessLimitEntry)

const accessFailLimit = 10
const accessFailWindow = 5 * time.Minute

func (ham *HiddenAccessManager) checkAccessKey(c *gin.Context) bool {
	// 检查所有支持的参数名
	for _, paramName := range ham.paramNames {
		if key := c.Query(paramName); key != "" {
			if subtle.ConstantTimeCompare([]byte(key), []byte(ham.secretKey)) == 1 {
				return true
			}
		}
	}
	// J5: 失败计数限流
	ip := getRealIP(c)
	now := time.Now()
	accessLimitMu.Lock()
	defer accessLimitMu.Unlock()
	e, ok := accessLimitMap[ip]
	if !ok || now.Sub(e.windowAt) > accessFailWindow {
		accessLimitMap[ip] = &accessLimitEntry{count: 1, windowAt: now}
		return false
	}
	e.count++
	if e.count > accessFailLimit {
		logx.Warnf("🚫 [隐藏访问] IP %s 在 %v 内密钥尝试超限，已封禁至窗口结束\n", ip, accessFailWindow)
	}
	return false
}

// metricsPathBypass /metrics 是否绕过隐藏访问入口（由 main 按 config.metrics.enabled 注入）
var metricsPathBypass atomic.Bool

// SetMetricsPathBypass 设置 /metrics 是否绕过隐藏访问入口
func SetMetricsPathBypass(v bool) { metricsPathBypass.Store(v) }

// handleUnauthorizedAccess 处理未授权访问
func (ham *HiddenAccessManager) handleUnauthorizedAccess(c *gin.Context, ip, path, query string) {
	// 记录可疑访问
	logx.Infof("🚫 未授权访问被阻止: IP=%s, Path=%s, Query=%s\n", ip, path, query)

	// 指标：安全拦截次数（按原因）——与事件流/通知互补，供告警规则使用
	metrics.SecurityBlock("hidden_access")

	// D17: 发布安全拦截事件（供事件流/通知告警）
	if command.GlobalEventBus != nil {
		command.PublishEvent(command.EventSecurityBlock, "", map[string]interface{}{
			"ip":    ip,
			"path":  path,
			"query": query,
		})
	}

	// 随机选择不同的服务器伪装
	serverTypes := []string{
		"Apache/2.4.41 (Ubuntu)",
		"nginx/1.18.0 (Ubuntu)",
		"Apache/2.4.52 (Win64)",
		"Microsoft-IIS/10.0",
	}

	// 使用IP地址最后一位数字来选择服务器类型，保证同一IP始终看到相同的伪装
	var serverIndex int
	for _, char := range ip {
		serverIndex += int(char)
	}
	selectedServer := serverTypes[serverIndex%len(serverTypes)]

	// 设置服务器响应头
	c.Header("Server", selectedServer)

	// 根据服务器类型选择不同的404样式
	if strings.Contains(selectedServer, "Apache") {
		// Apache风格的简洁404页面
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(`<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
<html><head>
<title>404 Not Found</title>
</head><body>
<h1>Not Found</h1>
<p>The requested URL `+path+` was not found on this server.</p>
<hr>
<address>`+selectedServer+`</address>
</body></html>`))
	} else if strings.Contains(selectedServer, "nginx") {
		// Nginx风格的404页面
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(`<html>
<head><title>404 Not Found</title></head>
<body>
<center><h1>404 Not Found</h1></center>
<hr><center>`+selectedServer+`</center>
</body>
</html>`))
	} else if strings.Contains(selectedServer, "IIS") {
		// IIS风格的404页面
		c.Data(http.StatusNotFound, "text/html; charset=utf-8", []byte(`<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-strict.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<title>IIS Windows Server</title>
</head>
<body>
<div id="header"><h1>Server Error</h1></div>
<div id="content">
<div class="content-container">
<h2>404 - File or directory not found.</h2>
<p>The resource you are looking for might have been removed, had its name changed, or is temporarily unavailable.</p>
</div>
</div>
</body>
</html>`))
	} else {
		// 默认使用我们的精美404页面
		c.HTML(http.StatusNotFound, "404.html", gin.H{
			"title":   "404 Not Found",
			"message": "The requested resource was not found on this server.",
			"server":  selectedServer,
		})
	}

	c.Abort()
}

// GetAccessKey 获取当前访问密钥
func (ham *HiddenAccessManager) GetAccessKey() string {
	if ham == nil {
		return ""
	}
	return ham.secretKey
}

// IsEnabled 检查是否启用隐藏访问
func (ham *HiddenAccessManager) IsEnabled() bool {
	if ham == nil {
		return false
	}
	return ham.enabled
}

// RegenerateKey 重新生成访问密钥
func (ham *HiddenAccessManager) RegenerateKey() string {
	ham.mu.Lock()
	defer ham.mu.Unlock()

	ham.secretKey = generateRandomKey()
	// 清空所有会话，强制重新认证
	ham.sessionMap = make(map[string]time.Time)
	logx.Infof("🔄 隐藏访问密钥已重新生成: %s\n", ham.secretKey)
	return ham.secretKey
}

// GetSessionCount 获取当前活跃会话数
func (ham *HiddenAccessManager) GetSessionCount() int {
	if ham == nil {
		return 0
	}

	ham.mu.Lock()
	defer ham.mu.Unlock()

	// 清理过期会话
	now := time.Now()
	for ip, lastAccess := range ham.sessionMap {
		if now.Sub(lastAccess) > ham.sessionTTL {
			delete(ham.sessionMap, ip)
		}
	}

	return len(ham.sessionMap)
}

// ClearSessions 清空所有会话
func (ham *HiddenAccessManager) ClearSessions() {
	if ham == nil {
		return
	}
	ham.mu.Lock()
	defer ham.mu.Unlock()

	ham.sessionMap = make(map[string]time.Time)
	logx.Println("🧹 所有隐藏访问会话已清空")
}

// GetConfig 获取当前配置信息
func (ham *HiddenAccessManager) GetConfig() map[string]interface{} {
	if ham == nil {
		return make(map[string]interface{})
	}

	// 使用读锁获取配置信息
	ham.mu.RLock()
	enabled := ham.enabled
	accessPaths := make([]string, len(ham.accessPaths))
	copy(accessPaths, ham.accessPaths)
	paramNames := make([]string, len(ham.paramNames))
	copy(paramNames, ham.paramNames)
	secretKey := ham.secretKey
	sessionTTL := int(ham.sessionTTL.Hours())
	ham.mu.RUnlock()

	return map[string]interface{}{
		"enabled":       enabled,
		"access_paths":  accessPaths,
		"param_names":   paramNames,
		"secret_key":    secretKey,
		"session_ttl":   sessionTTL,
		"session_count": ham.GetSessionCount(), // 这个方法内部已经有锁保护
	}
}

// hamIsAccessLimited 判断 IP 是否处于密钥尝试限流封禁窗口（J5）
func hamIsAccessLimited(ip string) bool {
	accessLimitMu.Lock()
	defer accessLimitMu.Unlock()
	e, ok := accessLimitMap[ip]
	if !ok {
		return false
	}
	if time.Since(e.windowAt) > accessFailWindow {
		delete(accessLimitMap, ip)
		return false
	}
	return e.count > accessFailLimit
}
