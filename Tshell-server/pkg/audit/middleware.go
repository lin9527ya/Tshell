package audit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"regexp"
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

// 文件操作审计的请求体预读上限：
// 不超过 maxFileAuditBodyBytes 的整包读（JSON 请求体远小于此）；
// 超过的只预读 fileAuditPeekBytes 前缀，避免大文件上传被整包缓冲进内存。
const (
	maxFileAuditBodyBytes = 256 * 1024
	fileAuditPeekBytes    = 64 * 1024
)

// responseBodyWriter 包装gin的ResponseWriter以捕获响应体（限长）
type responseBodyWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	exceed bool
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
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

// readFileAuditBody 读取用于提取目标路径的请求体，返回读到的字节，
// 并把请求体原样还原给 handler（大请求体只预读前缀，必须用 MultiReader 拼回去）。
//
// 必须限量：文件上传的请求体可达数百 MB，无上限 ReadAll 会让每个请求先整包驻留内存
// ——放宽上传上限之后，这里就是一个内存放大器。
func readFileAuditBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	if c.Request.ContentLength >= 0 && c.Request.ContentLength <= maxFileAuditBodyBytes {
		body, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
		return body
	}
	// 大请求体 / 分块传输：只预读前缀（够覆盖 multipart 头与 JSON 前几个字段）
	peek := make([]byte, fileAuditPeekBytes)
	n, _ := io.ReadFull(c.Request.Body, peek)
	body := peek[:n]
	c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), c.Request.Body))
	return body
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

		requestBody := readFileAuditBody(c)

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

// fileOperationRoute 一条需要记录文件操作审计的路由。
//
// 按精确路径登记，而不是像原来那样猜前缀：中间件挂在 fileAPI/operatorFile 上，
// 挂上去的每条路由都必须在这里有对应项，否则该路由不会产生任何文件操作审计。
// 原实现只匹配 /api/upload、/api/file 这类前缀，而实际挂载的全是 /api/client/file/*，
// 于是一条日志都没产生过；请求体字段名也全对不上（找的是 path/file/filename，
// 实际用的是 filePath/dirPath/oldPath/uploadPath）。route_policy_test 会校验对应关系。
type fileOperationRoute struct {
	path      string   // 精确请求路径；以 "/" 结尾表示前缀匹配（路径参数在末段）
	operation string   // 审计里记录的操作名
	pathKeys  []string // 从请求体提取目标路径的字段名，按优先级取第一个非空
	joinPaths bool     // 多个字段都要记（rename/copy 的新旧路径）时拼成 "旧 → 新"
	fromURI   bool     // 路径取自 URI 末段（文件名在路径里，不在请求体里）
}

var fileOperationRoutes = []fileOperationRoute{
	// 浏览 / 读取
	{path: "/api/client/file/tree", operation: "BROWSE", pathKeys: []string{"dirPath"}},
	{path: "/api/client/file/search", operation: "SEARCH", pathKeys: []string{"dirPath", "keyword", "pattern"}},
	{path: "/api/client/file/drives", operation: "LIST_DRIVES"},
	{path: "/api/client/file/edit/read", operation: "READ", pathKeys: []string{"filePath"}},
	{path: "/api/client/downloads/info", operation: "LIST_DOWNLOADS"},
	// 变更
	{path: "/api/client/file/create", operation: "CREATE_FILE", pathKeys: []string{"filePath"}},
	{path: "/api/client/file/mkdir", operation: "CREATE_DIR", pathKeys: []string{"dirPath"}},
	{path: "/api/client/file/edit/write", operation: "WRITE", pathKeys: []string{"filePath"}},
	{path: "/api/client/file/delete", operation: "DELETE", pathKeys: []string{"filePath"}},
	{path: "/api/client/file/rename", operation: "MOVE", pathKeys: []string{"oldPath", "newPath"}, joinPaths: true},
	{path: "/api/client/file/copy", operation: "COPY", pathKeys: []string{"oldPath", "newPath"}, joinPaths: true},
	{path: "/api/client/file/modifytime", operation: "MODIFY_TIME", pathKeys: []string{"filePath"}},
	{path: "/api/client/file/hide", operation: "HIDE", pathKeys: []string{"filePath"}},
	{path: "/api/client/file/unhide", operation: "UNHIDE", pathKeys: []string{"filePath"}},
	// 上传 / 下载
	{path: "/api/client/file/upload", operation: "UPLOAD", pathKeys: []string{"filename", "uploadPath"}},
	{path: "/api/client/file/download", operation: "DOWNLOAD", pathKeys: []string{"filePath"}},
	{path: "/api/client/downloads/downloaded_file", operation: "DOWNLOAD", pathKeys: []string{"filePath"}},
	{path: "/api/client/downloads/delete", operation: "DELETE", pathKeys: []string{"filePath"}},
	// 插件仓库：当前挂在 operatorOnly 上，handler 内已单独记 LogFileOperation；
	// 这里登记是为了万一改挂本中间件时同样生效
	{path: "/api/plugin/upload", operation: "UPLOAD", pathKeys: []string{"filename"}},
	{path: "/api/plugin/download/", operation: "DOWNLOAD", fromURI: true},
	{path: "/api/plugin/delete/", operation: "DELETE", fromURI: true},
}

// CoversFileOperation 报告该路径是否会记文件操作审计。
// 供路由策略测试守卫"挂在审计组上的路由必须被登记"这一不变式。
func CoversFileOperation(path string) bool {
	_, ok := matchFileOperationRoute(path)
	return ok
}

// matchFileOperationRoute 按请求路径匹配路由（忽略查询串），先精确后前缀
func matchFileOperationRoute(uri string) (fileOperationRoute, bool) {
	path := uri
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	for _, r := range fileOperationRoutes {
		if r.path == path {
			return r, true
		}
	}
	for _, r := range fileOperationRoutes {
		if strings.HasSuffix(r.path, "/") && strings.HasPrefix(path, r.path) {
			return r, true
		}
	}
	return fileOperationRoute{}, false
}

// 从URI和请求体中提取文件操作信息
func extractFileOperationInfo(uri string, body []byte) (operation, filePath string) {
	route, ok := matchFileOperationRoute(uri)
	if !ok {
		return "", ""
	}
	if route.fromURI {
		return route.operation, pathFromURI(uri, route.path)
	}
	return route.operation, extractFilePath(route, body)
}

// pathFromURI 取 URI 末段的文件名（下载/删除类把文件名放在路径里）
func pathFromURI(uri, prefix string) string {
	if i := strings.IndexByte(uri, '?'); i >= 0 {
		uri = uri[:i]
	}
	name := strings.TrimPrefix(uri, prefix)
	if unescaped, err := url.PathUnescape(name); err == nil {
		return unescaped
	}
	return name
}

// extractFilePath 按字段名从请求体提取目标路径。
// 三种情况都要覆盖：正常 JSON、被截断的 JSON（大请求体只预读前缀）、multipart 表单。
func extractFilePath(route fileOperationRoute, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if vals := jsonPathValues(route.pathKeys, body); len(vals) > 0 {
		return joinPathValues(vals, route.joinPaths)
	}
	// 截断的 JSON / multipart：json.Unmarshal 会失败，但字段名与值仍在前缀里
	vals := regexPathValues(route.pathKeys, body)
	if len(vals) == 0 {
		// multipart：文件名字段在 part 头里，普通字段在空行之后
		vals = multipartValues(route.pathKeys, body)
	}
	return joinPathValues(vals, route.joinPaths)
}

func joinPathValues(vals []string, join bool) string {
	if len(vals) == 0 {
		return ""
	}
	if join {
		return strings.Join(vals, " → ")
	}
	return vals[0]
}

// jsonPathValues 解析完整 JSON 体并按键顺序取非空字符串值
func jsonPathValues(keys []string, body []byte) []string {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil
	}
	var out []string
	for _, k := range keys {
		if v, ok := data[k].(string); ok && v != "" {
			out = append(out, v)
		}
	}
	return out
}

// regexPathValues 从（可能被截断的）请求体里按 `"key":"value"` 抓值
func regexPathValues(keys []string, body []byte) []string {
	var out []string
	for _, k := range keys {
		re := regexp.MustCompile(`"` + regexp.QuoteMeta(k) + `"\s*:\s*"((?:[^"\\]|\\.)*)"`)
		if m := re.FindSubmatch(body); m != nil && len(m[1]) > 0 {
			out = append(out, string(m[1]))
		}
	}
	return out
}

// multipartValues 从 multipart 体里取字段值：filename="..." 或 name="k" 空行后的值
func multipartValues(keys []string, body []byte) []string {
	var out []string
	for _, k := range keys {
		if k == "filename" {
			if m := multipartFilenameRe.FindSubmatch(body); m != nil && len(m[1]) > 0 {
				out = append(out, string(m[1]))
			}
			continue
		}
		re := regexp.MustCompile(`name="` + regexp.QuoteMeta(k) + `"\r?\n\r?\n([^\r\n]*)`)
		if m := re.FindSubmatch(body); m != nil && len(m[1]) > 0 {
			out = append(out, string(m[1]))
		}
	}
	return out
}

var multipartFilenameRe = regexp.MustCompile(`filename="([^"]*)"`)

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
