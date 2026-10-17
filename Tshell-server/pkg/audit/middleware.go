package audit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// isSkippablePath 无需审计的路径：
// 静态资源（前端加载 40+ JS/CSS chunk，每次全量缓冲响应体 + 写日志，开销大且无审计价值）
// 与投递路径（/d/ 下载被流量放大，不应计入审计）。
func isSkippablePath(path string) bool {
	if strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/css/") ||
		strings.HasPrefix(path, "/js/") || strings.HasPrefix(path, "/img/") ||
		path == "/favicon.ico" {
		return true
	}
	if strings.HasPrefix(path, "/d/") || strings.Contains(path, "/d/") {
		return true
	}
	return false
}

// maxBufferedResponseSize 审计中间件缓冲的响应体上限。
// 【安全修复】大响应（文件下载 MB 级、投递载荷）不做全量缓冲，
// 防止公开端点被利用放大内存 DoS。
const maxBufferedResponseSize = 256 * 1024 // 256KB（仅缓冲敏感 API 响应体）

// responseBodyWriter 包装gin的ResponseWriter以捕获响应体（限长）
type responseBodyWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	exceed bool
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	if !w.exceed && w.body.Len()+len(b) <= maxBufferedResponseSize {
		w.body.Write(b)
	} else {
		w.exceed = true
		w.body.Reset() // 释放已缓冲的大响应，只记元数据
	}
	return w.ResponseWriter.Write(b)
}

// AuditMiddleware 审计日志中间件
func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 【优化】静态资源与投递路径直接放行，不做响应缓冲与日志记录
		if isSkippablePath(c.Request.URL.Path) {
			c.Next()
			return
		}

		startTime := time.Now()

		// 创建响应体包装器
		bodyWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = bodyWriter

		// 读取请求体（如果有的话）——【安全修复】仅 ≤1MB 的请求体预读，
		// 大请求体（文件上传等）不预读，避免内存翻倍
		var requestBody []byte
		if c.Request.Body != nil && c.Request.ContentLength >= 0 && c.Request.ContentLength <= maxBufferedResponseSize {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 处理请求
		c.Next()

		// 计算处理时间
		duration := time.Since(startTime)

		// 获取错误信息
		var errorMsg string
		if len(c.Errors) > 0 {
			errorMsg = c.Errors.String()
		}

		// 记录API访问日志
		if GlobalAuditLogger != nil {
			GlobalAuditLogger.LogAPIAccess(c, c.Writer.Status(), duration, errorMsg)

			// 如果是敏感API，记录更详细的信息
			if isSensitiveAPI(c.Request.RequestURI) {
				details := map[string]interface{}{
					"request_size":  len(requestBody),
					"response_size": bodyWriter.body.Len(),
					"headers":       sanitizeHeaders(c.Request.Header),
				}

				// 对于某些API，记录请求体（需要脱敏处理）
				if shouldLogRequestBody(c.Request.RequestURI) {
					sanitizedBody := sanitizeRequestBody(requestBody)
					if sanitizedBody != nil {
						details["request_body"] = sanitizedBody
					}
				}

				GlobalAuditLogger.LogWithContext(c, LogLevelInfo, CategoryAPI, "SENSITIVE_API_ACCESS",
					getResultFromStatus(c.Writer.Status()), "敏感API访问", details)
			}
		}
	}
}

// LoginAuditMiddleware 登录审计中间件
func LoginAuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// 创建响应体包装器
		bodyWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = bodyWriter

		// 读取请求体获取用户名
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 处理请求
		c.Next()

		// 解析用户名
		username := extractUsernameFromRequest(requestBody)
		success := c.Writer.Status() == 200

		var errorMsg string
		if !success {
			if len(c.Errors) > 0 {
				errorMsg = c.Errors.String()
			} else {
				// 尝试从响应体获取错误信息
				var response map[string]interface{}
				if err := json.Unmarshal(bodyWriter.body.Bytes(), &response); err == nil {
					if msg, ok := response["message"]; ok {
						errorMsg = msg.(string)
					} else if err, ok := response["error"]; ok {
						errorMsg = err.(string)
					}
				}
			}
		}

		// 记录登录日志
		if GlobalAuditLogger != nil {
			GlobalAuditLogger.LogLogin(c.ClientIP(), username, success, errorMsg)

			// 如果登录失败，记录安全事件
			if !success {
				details := map[string]interface{}{
					"username":    username,
					"user_agent":  c.GetHeader("User-Agent"),
					"duration_ms": time.Since(startTime).Milliseconds(),
					"status_code": c.Writer.Status(),
				}
				GlobalAuditLogger.LogSecurityEvent(c.ClientIP(), "LOGIN_FAILURE",
					"登录失败尝试", details)
			}
		}
	}
}

// FileOperationAuditMiddleware 文件操作审计中间件
func FileOperationAuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// 创建响应体包装器
		bodyWriter := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           bytes.NewBufferString(""),
		}
		c.Writer = bodyWriter

		// 读取请求体
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 处理请求
		c.Next()

		// 提取文件操作信息
		operation, filePath := extractFileOperationInfo(c.Request.RequestURI, requestBody)
		if operation != "" {
			success := c.Writer.Status() < 400

			var errorMsg string
			if !success && len(c.Errors) > 0 {
				errorMsg = c.Errors.String()
			}

			username := ""
			if user, exists := c.Get("username"); exists {
				username = user.(string)
			}

			// 记录文件操作日志
			if GlobalAuditLogger != nil {
				GlobalAuditLogger.LogFileOperation(c.ClientIP(), username, operation, filePath, success, errorMsg)

				// 记录详细的文件操作信息
				details := map[string]interface{}{
					"operation":   operation,
					"file_path":   filePath,
					"duration_ms": time.Since(startTime).Milliseconds(),
					"status_code": c.Writer.Status(),
					"user_agent":  c.GetHeader("User-Agent"),
				}

				GlobalAuditLogger.LogWithContext(c, LogLevelInfo, CategoryFile, operation,
					getResultFromStatus(c.Writer.Status()), "文件操作", details)
			}
		}
	}
}

// 判断是否为敏感API
func isSensitiveAPI(uri string) bool {
	sensitiveAPIs := []string{
		"/api/login", "/api/logout", "/api/refresh",
		"/api/users", "/api/whitelist", "/api/security",
		"/api/file", "/api/upload", "/api/download",
		"/api/command", "/api/shell",
		// 插件仓库：上传/下载/删除的是可下发执行的载荷，需记录详细审计
		"/api/plugin",
	}

	for _, api := range sensitiveAPIs {
		if len(uri) >= len(api) && uri[:len(api)] == api {
			return true
		}
	}
	return false
}

// 判断是否应该记录请求体
func shouldLogRequestBody(uri string) bool {
	// 对于登录等敏感操作，不记录请求体（包含密码）
	excludeAPIs := []string{"/api/login", "/api/register", "/api/change-password"}

	for _, api := range excludeAPIs {
		if len(uri) >= len(api) && uri[:len(api)] == api {
			return false
		}
	}
	return true
}

// 脱敏处理请求头
func sanitizeHeaders(headers map[string][]string) map[string]interface{} {
	sanitized := make(map[string]interface{})

	sensitiveHeaders := map[string]bool{
		"authorization": true,
		"cookie":        true,
		"x-api-key":     true,
	}

	for key, values := range headers {
		lowerKey := key
		if sensitiveHeaders[lowerKey] {
			sanitized[key] = "[REDACTED]"
		} else {
			sanitized[key] = values
		}
	}

	return sanitized
}

// 脱敏处理请求体
func sanitizeRequestBody(body []byte) map[string]interface{} {
	if len(body) == 0 {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil
	}

	// 敏感字段列表
	sensitiveFields := map[string]bool{
		"password":         true,
		"old_password":     true,
		"new_password":     true,
		"confirm_password": true,
		"token":            true,
		"secret":           true,
		"key":              true,
	}

	// 脱敏处理
	for key, value := range data {
		if sensitiveFields[key] {
			data[key] = "[REDACTED]"
		} else if str, ok := value.(string); ok && len(str) > 100 {
			// 截断过长的字符串
			data[key] = str[:100] + "..."
		}
	}

	return data
}

// 从请求体中提取用户名
func extractUsernameFromRequest(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	if username, ok := data["username"]; ok {
		return username.(string)
	}

	return ""
}

// 从URI和请求体中提取文件操作信息
func extractFileOperationInfo(uri string, body []byte) (operation, filePath string) {
	// 根据URI判断操作类型
	if len(uri) >= 12 && uri[:12] == "/api/upload" {
		operation = "UPLOAD"
	} else if len(uri) >= 14 && uri[:14] == "/api/download" {
		operation = "DOWNLOAD"
	} else if len(uri) >= 12 && uri[:12] == "/api/delete" {
		operation = "DELETE"
	} else if len(uri) >= 11 && uri[:11] == "/api/mkdir" {
		operation = "CREATE_DIR"
	} else if len(uri) >= 9 && uri[:9] == "/api/move" {
		operation = "MOVE"
	} else if len(uri) >= 9 && uri[:9] == "/api/copy" {
		operation = "COPY"
	} else if len(uri) >= 9 && uri[:9] == "/api/file" {
		operation = "READ"
	} else if strings.HasPrefix(uri, "/api/plugin/upload") {
		operation = "UPLOAD"
	} else if strings.HasPrefix(uri, "/api/plugin/download/") {
		operation = "DOWNLOAD"
	} else if strings.HasPrefix(uri, "/api/plugin/delete/") {
		operation = "DELETE"
	} else {
		return "", ""
	}

	// 插件下载/删除的文件名在 URI 末段（不在请求体里）
	if filePath == "" {
		for _, prefix := range []string{"/api/plugin/download/", "/api/plugin/delete/"} {
			if !strings.HasPrefix(uri, prefix) {
				continue
			}
			name := uri[len(prefix):]
			if i := strings.IndexByte(name, '?'); i >= 0 {
				name = name[:i]
			}
			if unescaped, err := url.PathUnescape(name); err == nil {
				filePath = unescaped
			} else {
				filePath = name
			}
			break
		}
	}

	// 尝试从请求体中提取文件路径
	if len(body) > 0 {
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err == nil {
			if path, ok := data["path"]; ok {
				filePath = path.(string)
			} else if file, ok := data["file"]; ok {
				filePath = file.(string)
			} else if filename, ok := data["filename"]; ok {
				filePath = filename.(string)
			}
		}
	}

	return operation, filePath
}

// 根据HTTP状态码获取结果
func getResultFromStatus(statusCode int) string {
	if statusCode >= 200 && statusCode < 300 {
		return "SUCCESS"
	} else if statusCode >= 400 && statusCode < 500 {
		return "CLIENT_ERROR"
	} else if statusCode >= 500 {
		return "SERVER_ERROR"
	}
	return "UNKNOWN"
}
