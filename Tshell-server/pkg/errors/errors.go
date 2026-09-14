package errors

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"time"
)

// ErrorCode 错误码类型
type ErrorCode int

// 错误码定义
const (
	// 通用错误码 (1000-1999)
	ErrCodeSuccess        ErrorCode = 0
	ErrCodeBadRequest     ErrorCode = 1000
	ErrCodeUnauthorized   ErrorCode = 1001
	ErrCodeForbidden      ErrorCode = 1002
	ErrCodeNotFound       ErrorCode = 1003
	ErrCodeInternalError  ErrorCode = 1004
	ErrCodeServiceUnavail ErrorCode = 1005
	ErrCodeTooManyReqs    ErrorCode = 1006

	// 认证相关错误 (2000-2999)
	ErrCodeInvalidToken     ErrorCode = 2000
	ErrCodeTokenExpired     ErrorCode = 2001
	ErrCodeInvalidPassword  ErrorCode = 2002
	ErrCodeUserNotFound     ErrorCode = 2003
	ErrCodeUserLocked       ErrorCode = 2004
	ErrCodeLoginFailed      ErrorCode = 2005
	ErrCodePermissionDenied ErrorCode = 2006

	// 数据相关错误 (3000-3999)
	ErrCodeDatabaseError    ErrorCode = 3000
	ErrCodeDataNotFound     ErrorCode = 3001
	ErrCodeDataConflict     ErrorCode = 3002
	ErrCodeValidationFailed ErrorCode = 3003
	ErrCodeSerializationErr ErrorCode = 3004

	// 网络连接错误 (4000-4999)
	ErrCodeConnectionFailed  ErrorCode = 4000
	ErrCodeConnectionTimeout ErrorCode = 4001
	ErrCodeConnectionLost    ErrorCode = 4002
	ErrCodeNetworkError      ErrorCode = 4003
	ErrCodeDNSError          ErrorCode = 4004

	// 文件操作错误 (5000-5999)
	ErrCodeFileNotFound   ErrorCode = 5000
	ErrCodeFilePermission ErrorCode = 5001
	ErrCodeFileCorrupted  ErrorCode = 5002
	ErrCodeDiskFull       ErrorCode = 5003
	ErrCodeUploadFailed   ErrorCode = 5004
	ErrCodeDownloadFailed ErrorCode = 5005

	// 系统资源错误 (6000-6999)
	ErrCodeOutOfMemory       ErrorCode = 6000
	ErrCodeCPUOverload       ErrorCode = 6001
	ErrCodeResourceExhausted ErrorCode = 6002
	ErrCodeTimeout           ErrorCode = 6003

	// 配置错误 (7000-7999)
	ErrCodeConfigMissing    ErrorCode = 7000
	ErrCodeConfigInvalid    ErrorCode = 7001
	ErrCodeConfigLoadFailed ErrorCode = 7002

	// 安全相关错误 (8000-8999)
	ErrCodeSecurityViolation  ErrorCode = 8000
	ErrCodeRateLimitExceeded  ErrorCode = 8001
	ErrCodeIPBlocked          ErrorCode = 8002
	ErrCodeSuspiciousActivity ErrorCode = 8003
)

// ErrorLevel 错误级别
type ErrorLevel string

const (
	LevelInfo     ErrorLevel = "INFO"
	LevelWarning  ErrorLevel = "WARNING"
	LevelError    ErrorLevel = "ERROR"
	LevelCritical ErrorLevel = "CRITICAL"
)

// AppError 应用错误结构体
type AppError struct {
	Code        ErrorCode   `json:"code"`
	Message     string      `json:"message"`
	Detail      string      `json:"detail,omitempty"`
	Level       ErrorLevel  `json:"level"`
	Timestamp   time.Time   `json:"timestamp"`
	RequestID   string      `json:"request_id,omitempty"`
	StackTrace  string      `json:"stack_trace,omitempty"`
	Context     interface{} `json:"context,omitempty"`
	HTTPStatus  int         `json:"-"`
	InternalErr error       `json:"-"`
}

// Error 实现error接口
func (e *AppError) Error() string {
	return fmt.Sprintf("[%d] %s: %s", e.Code, e.Message, e.Detail)
}

// WithDetail 添加详细信息
func (e *AppError) WithDetail(detail string) *AppError {
	newErr := *e
	newErr.Detail = detail
	return &newErr
}

// WithContext 添加上下文信息
func (e *AppError) WithContext(ctx interface{}) *AppError {
	newErr := *e
	newErr.Context = ctx
	return &newErr
}

// WithStackTrace 添加堆栈跟踪
func (e *AppError) WithStackTrace() *AppError {
	newErr := *e
	newErr.StackTrace = getStackTrace()
	return &newErr
}

// WithInternalError 添加内部错误
func (e *AppError) WithInternalError(err error) *AppError {
	newErr := *e
	newErr.InternalErr = err
	if newErr.Detail == "" {
		newErr.Detail = err.Error()
	}
	return &newErr
}

// 获取堆栈跟踪
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// 错误码到HTTP状态码的映射
var errorCodeToHTTPStatus = map[ErrorCode]int{
	ErrCodeSuccess:           http.StatusOK,
	ErrCodeBadRequest:        http.StatusBadRequest,
	ErrCodeUnauthorized:      http.StatusUnauthorized,
	ErrCodeForbidden:         http.StatusForbidden,
	ErrCodeNotFound:          http.StatusNotFound,
	ErrCodeInternalError:     http.StatusInternalServerError,
	ErrCodeServiceUnavail:    http.StatusServiceUnavailable,
	ErrCodeTooManyReqs:       http.StatusTooManyRequests,
	ErrCodeInvalidToken:      http.StatusUnauthorized,
	ErrCodeTokenExpired:      http.StatusUnauthorized,
	ErrCodeInvalidPassword:   http.StatusUnauthorized,
	ErrCodeUserNotFound:      http.StatusNotFound,
	ErrCodeUserLocked:        http.StatusForbidden,
	ErrCodeLoginFailed:       http.StatusUnauthorized,
	ErrCodePermissionDenied:  http.StatusForbidden,
	ErrCodeDatabaseError:     http.StatusInternalServerError,
	ErrCodeDataNotFound:      http.StatusNotFound,
	ErrCodeDataConflict:      http.StatusConflict,
	ErrCodeValidationFailed:  http.StatusBadRequest,
	ErrCodeConnectionFailed:  http.StatusServiceUnavailable,
	ErrCodeConnectionTimeout: http.StatusRequestTimeout,
	ErrCodeFileNotFound:      http.StatusNotFound,
	ErrCodeFilePermission:    http.StatusForbidden,
	ErrCodeUploadFailed:      http.StatusBadRequest,
	ErrCodeDownloadFailed:    http.StatusInternalServerError,
	ErrCodeOutOfMemory:       http.StatusInternalServerError,
	ErrCodeTimeout:           http.StatusRequestTimeout,
	ErrCodeConfigMissing:     http.StatusInternalServerError,
	ErrCodeConfigInvalid:     http.StatusInternalServerError,
	ErrCodeRateLimitExceeded: http.StatusTooManyRequests,
	ErrCodeIPBlocked:         http.StatusForbidden,
}

// 错误码到消息的映射
var errorCodeToMessage = map[ErrorCode]string{
	ErrCodeSuccess:            "操作成功",
	ErrCodeBadRequest:         "请求参数错误",
	ErrCodeUnauthorized:       "未授权访问",
	ErrCodeForbidden:          "访问被拒绝",
	ErrCodeNotFound:           "资源未找到",
	ErrCodeInternalError:      "内部服务器错误",
	ErrCodeServiceUnavail:     "服务不可用",
	ErrCodeTooManyReqs:        "请求过于频繁",
	ErrCodeInvalidToken:       "令牌无效",
	ErrCodeTokenExpired:       "令牌已过期",
	ErrCodeInvalidPassword:    "密码错误",
	ErrCodeUserNotFound:       "用户不存在",
	ErrCodeUserLocked:         "用户账户已锁定",
	ErrCodeLoginFailed:        "登录失败",
	ErrCodePermissionDenied:   "权限不足",
	ErrCodeDatabaseError:      "数据库操作错误",
	ErrCodeDataNotFound:       "数据不存在",
	ErrCodeDataConflict:       "数据冲突",
	ErrCodeValidationFailed:   "数据验证失败",
	ErrCodeSerializationErr:   "数据序列化错误",
	ErrCodeConnectionFailed:   "连接失败",
	ErrCodeConnectionTimeout:  "连接超时",
	ErrCodeConnectionLost:     "连接丢失",
	ErrCodeNetworkError:       "网络错误",
	ErrCodeDNSError:           "DNS解析错误",
	ErrCodeFileNotFound:       "文件不存在",
	ErrCodeFilePermission:     "文件权限不足",
	ErrCodeFileCorrupted:      "文件已损坏",
	ErrCodeDiskFull:           "磁盘空间不足",
	ErrCodeUploadFailed:       "文件上传失败",
	ErrCodeDownloadFailed:     "文件下载失败",
	ErrCodeOutOfMemory:        "内存不足",
	ErrCodeCPUOverload:        "CPU负载过高",
	ErrCodeResourceExhausted:  "系统资源耗尽",
	ErrCodeTimeout:            "操作超时",
	ErrCodeConfigMissing:      "配置缺失",
	ErrCodeConfigInvalid:      "配置无效",
	ErrCodeConfigLoadFailed:   "配置加载失败",
	ErrCodeSecurityViolation:  "安全违规",
	ErrCodeRateLimitExceeded:  "访问频率超限",
	ErrCodeIPBlocked:          "IP地址被封禁",
	ErrCodeSuspiciousActivity: "检测到可疑活动",
}

// NewError 创建新的应用错误
func NewError(code ErrorCode, detail ...string) *AppError {
	message := errorCodeToMessage[code]
	if message == "" {
		message = "未知错误"
	}

	httpStatus := errorCodeToHTTPStatus[code]
	if httpStatus == 0 {
		httpStatus = http.StatusInternalServerError
	}

	level := LevelError
	if code < 2000 {
		level = LevelWarning
	}
	if code >= 8000 || httpStatus >= 500 {
		level = LevelCritical
	}

	err := &AppError{
		Code:       code,
		Message:    message,
		Level:      level,
		Timestamp:  time.Now(),
		HTTPStatus: httpStatus,
	}

	if len(detail) > 0 {
		err.Detail = detail[0]
	}

	return err
}

// NewBadRequestError 创建请求参数错误
func NewBadRequestError(detail string) *AppError {
	return NewError(ErrCodeBadRequest, detail)
}

// NewUnauthorizedError 创建未授权错误
func NewUnauthorizedError(detail string) *AppError {
	return NewError(ErrCodeUnauthorized, detail)
}

// NewForbiddenError 创建访问被拒绝错误
func NewForbiddenError(detail string) *AppError {
	return NewError(ErrCodeForbidden, detail)
}

// NewNotFoundError 创建资源未找到错误
func NewNotFoundError(detail string) *AppError {
	return NewError(ErrCodeNotFound, detail)
}

// NewInternalError 创建内部服务器错误
func NewInternalError(detail string) *AppError {
	return NewError(ErrCodeInternalError, detail).WithStackTrace()
}

// NewDatabaseError 创建数据库错误
func NewDatabaseError(err error) *AppError {
	return NewError(ErrCodeDatabaseError).WithInternalError(err)
}

// NewValidationError 创建验证错误
func NewValidationError(detail string) *AppError {
	return NewError(ErrCodeValidationFailed, detail)
}

// NewConnectionError 创建连接错误
func NewConnectionError(detail string) *AppError {
	return NewError(ErrCodeConnectionFailed, detail)
}

// NewFileError 创建文件操作错误
func NewFileError(code ErrorCode, detail string) *AppError {
	return NewError(code, detail)
}

// NewSecurityError 创建安全相关错误
func NewSecurityError(code ErrorCode, detail string) *AppError {
	return NewError(code, detail)
}

// WrapError 包装现有错误
func WrapError(err error, code ErrorCode, detail ...string) *AppError {
	if err == nil {
		return nil
	}

	// 如果已经是AppError，直接返回
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}

	appErr := NewError(code)
	if len(detail) > 0 {
		appErr.Detail = detail[0]
	} else {
		appErr.Detail = err.Error()
	}
	appErr.InternalErr = err

	return appErr
}

// SanitizeError 内部错误脱敏（P2-2）：错误原文仅服务端日志留存，客户端只收到
// 通用文案，避免 SQL/文件系统路径/内部实现细节泄露给调用方。
// 仅用于 5xx 内部错误的对外回显；参数校验类 4xx 错误保留原文（对操作者有诊断价值）。
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	log.Printf("⚠️ [API] 内部错误(已脱敏): %v", err)
	return "服务器内部错误"
}
