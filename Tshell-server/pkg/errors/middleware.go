package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// ResponseFormat 统一响应格式
type ResponseFormat struct {
	Success   bool        `json:"success"`
	Code      ErrorCode   `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Detail    string      `json:"detail,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Timestamp int64       `json:"timestamp"`
}

// ErrorHandlerMiddleware 错误处理中间件
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// 处理错误
		if len(c.Errors) > 0 {
			err := c.Errors.Last()
			handleError(c, err.Err)
		}
	}
}

// handleError 处理错误并返回统一格式
func handleError(c *gin.Context, err error) {
	var appErr *AppError

	// 检查是否为AppError
	if ae, ok := err.(*AppError); ok {
		appErr = ae
	} else {
		// 包装为内部错误
		appErr = NewInternalError("内部服务器错误").WithInternalError(err)
	}

	// 设置请求ID
	if requestID := c.GetString("request_id"); requestID != "" {
		appErr.RequestID = requestID
	}

	// 记录错误日志
	logError(c, appErr)

	// 返回错误响应
	RespondWithError(c, appErr)
}

// RespondWithError 返回错误响应
func RespondWithError(c *gin.Context, appErr *AppError) {
	response := ResponseFormat{
		Success:   false,
		Code:      appErr.Code,
		Message:   appErr.Message,
		Detail:    appErr.Detail,
		RequestID: appErr.RequestID,
		Timestamp: appErr.Timestamp.Unix(),
	}

	// 在开发环境中包含堆栈跟踪
	if gin.Mode() == gin.DebugMode && appErr.StackTrace != "" {
		response.Data = map[string]string{
			"stack_trace": appErr.StackTrace,
		}
	}

	c.JSON(appErr.HTTPStatus, response)
}

// RespondWithSuccess 返回成功响应
func RespondWithSuccess(c *gin.Context, data interface{}) {
	response := ResponseFormat{
		Success:   true,
		Code:      ErrCodeSuccess,
		Message:   "操作成功",
		Data:      data,
		Timestamp: c.GetInt64("timestamp"),
	}

	if requestID := c.GetString("request_id"); requestID != "" {
		response.RequestID = requestID
	}

	c.JSON(http.StatusOK, response)
}

// RespondWithPagination 返回分页响应
func RespondWithPagination(c *gin.Context, data interface{}, pagination map[string]interface{}) {
	responseData := map[string]interface{}{
		"list":       data,
		"pagination": pagination,
	}

	RespondWithSuccess(c, responseData)
}

// logError 记录错误日志
func logError(c *gin.Context, appErr *AppError) {
	logData := map[string]interface{}{
		"error_code":    appErr.Code,
		"error_message": appErr.Message,
		"error_detail":  appErr.Detail,
		"error_level":   appErr.Level,
		"request_id":    appErr.RequestID,
		"method":        c.Request.Method,
		"url":           c.Request.URL.String(),
		"client_ip":     c.ClientIP(),
		"user_agent":    c.GetHeader("User-Agent"),
	}

	// 添加用户信息
	if userID, exists := c.Get("user_id"); exists {
		logData["user_id"] = userID
	}
	if username, exists := c.Get("username"); exists {
		logData["username"] = username
	}

	// 添加内部错误信息
	if appErr.InternalErr != nil {
		logData["internal_error"] = appErr.InternalErr.Error()
	}

	// 序列化日志
	logJSON, _ := json.Marshal(logData)

	// 根据错误级别记录日志
	switch appErr.Level {
	case LevelCritical:
		logx.Infof("🔴 CRITICAL ERROR: %s\n", string(logJSON))
	case LevelError:
		logx.Errorf("❌ ERROR: %s\n", string(logJSON))
	case LevelWarning:
		logx.Warnf("⚠️ WARNING: %s\n", string(logJSON))
	default:
		logx.Infof("ℹ️ INFO: %s\n", string(logJSON))
	}
}

// Recovery 恢复中间件，捕获panic
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		var err error

		switch x := recovered.(type) {
		case string:
			err = fmt.Errorf("panic: %s", x)
		case error:
			err = fmt.Errorf("panic: %v", x)
		default:
			err = fmt.Errorf("panic: %v", recovered)
		}

		appErr := NewInternalError("服务器发生严重错误").WithInternalError(err).WithStackTrace()

		// 记录panic日志
		logError(c, appErr)

		RespondWithError(c, appErr)
	})
}

// RequestIDMiddleware 请求ID中间件
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// TimestampMiddleware 时间戳中间件
func TimestampMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("timestamp", c.GetInt64("start_time"))
		c.Next()
	}
}

// generateRequestID 生成请求ID
func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// AbortWithError 中止请求并返回错误
func AbortWithError(c *gin.Context, appErr *AppError) {
	c.Error(appErr)
	c.Abort()
}

// AbortWithBadRequest 中止请求并返回400错误
func AbortWithBadRequest(c *gin.Context, detail string) {
	AbortWithError(c, NewBadRequestError(detail))
}

// AbortWithUnauthorized 中止请求并返回401错误
func AbortWithUnauthorized(c *gin.Context, detail string) {
	AbortWithError(c, NewUnauthorizedError(detail))
}

// AbortWithForbidden 中止请求并返回403错误
func AbortWithForbidden(c *gin.Context, detail string) {
	AbortWithError(c, NewForbiddenError(detail))
}

// AbortWithNotFound 中止请求并返回404错误
func AbortWithNotFound(c *gin.Context, detail string) {
	AbortWithError(c, NewNotFoundError(detail))
}

// AbortWithInternalError 中止请求并返回500错误
func AbortWithInternalError(c *gin.Context, detail string) {
	AbortWithError(c, NewInternalError(detail))
}

// ValidateRequest 验证请求参数的通用函数
func ValidateRequest(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		return NewValidationError(fmt.Sprintf("请求参数验证失败: %v", err))
	}
	return nil
}

// ParseIntParam 解析整数参数
func ParseIntParam(c *gin.Context, param string) (int, error) {
	value := c.Param(param)
	if value == "" {
		return 0, NewBadRequestError(fmt.Sprintf("参数 %s 不能为空", param))
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return 0, NewBadRequestError(fmt.Sprintf("参数 %s 必须是整数", param))
	}

	return intValue, nil
}

// ParseIntQuery 解析整数查询参数
func ParseIntQuery(c *gin.Context, query string, defaultValue int) int {
	value := c.Query(query)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return intValue
}

// MaxBodyBytesMiddleware 全局请求体大小限制（J9: 防止认证用户提交超大 JSON 造成内存 DoS）
// 所有请求体超过 limit 直接 413 拒绝。
func MaxBodyBytesMiddleware(limit int64) gin.HandlerFunc {
	return MaxBodyBytesMiddlewareWithLimits(limit, nil)
}

// MaxBodyBytesMiddlewareWithLimits 带按路径覆盖的请求体上限。
//
// 上传类接口的上限必须在这里放宽，**不能**在 handler 里重新包一层 MaxBytesReader：
// 嵌套时生效的是内层（更小）的上限——外层读到内层返回的 MaxBytesError 后原样透传，
// 所以 handler 里包一个更大的上限是无效的（client_files.go 原来那种写法就是这个问题）。
//
// pathLimits 的键是精确请求路径（c.Request.URL.Path）。
func MaxBodyBytesMiddlewareWithLimits(limit int64, pathLimits map[string]int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.Next()
			return
		}
		effective := limit
		if override, ok := pathLimits[c.Request.URL.Path]; ok {
			effective = override
		}
		// 不再要求 ContentLength > 0：分块传输（chunked）时 ContentLength 为 -1，
		// 原条件会让这类请求完全不受限，全局 DoS 防护被整体绕过。
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, effective)
		c.Next()
	}
}
