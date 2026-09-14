package audit

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// LogLevel 日志级别
type LogLevel string

const (
	LogLevelInfo     LogLevel = "INFO"
	LogLevelWarn     LogLevel = "WARN"
	LogLevelError    LogLevel = "ERROR"
	LogLevelSecurity LogLevel = "SECURITY"
)

// LogCategory 日志分类
type LogCategory string

const (
	CategoryAuth       LogCategory = "AUTH"       // 认证相关
	CategoryAPI        LogCategory = "API"        // API调用
	CategoryFile       LogCategory = "FILE"       // 文件操作
	CategorySystem     LogCategory = "SYSTEM"     // 系统操作
	CategorySecurity   LogCategory = "SECURITY"   // 安全事件
	CategoryConnection LogCategory = "CONNECTION" // 连接管理
	CategoryCommand    LogCategory = "COMMAND"    // 命令执行
)

// AuditLog 审计日志结构体
type AuditLog struct {
	ID         string                 `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	Level      LogLevel               `json:"level"`
	Category   LogCategory            `json:"category"`
	Action     string                 `json:"action"`
	UserID     string                 `json:"user_id,omitempty"`
	Username   string                 `json:"username,omitempty"`
	ClientIP   string                 `json:"client_ip"`
	UserAgent  string                 `json:"user_agent,omitempty"`
	RequestURI string                 `json:"request_uri,omitempty"`
	Method     string                 `json:"method,omitempty"`
	StatusCode int                    `json:"status_code,omitempty"`
	Duration   time.Duration          `json:"duration,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Result     string                 `json:"result"`
	Message    string                 `json:"message"`
	ErrorMsg   string                 `json:"error_msg,omitempty"`
}

// AuditLogger 审计日志管理器
type AuditLogger struct {
	mu         sync.RWMutex
	flushMu    sync.Mutex // 串行化落盘（锁外执行，避免 fsync/轮转阻塞日志写入）
	logFile    *os.File
	logger     *log.Logger
	logDir     string
	maxSize    int64 // 最大文件大小 (bytes)
	maxDays    int   // 日志保留天数
	logBuffer  []AuditLog
	bufferSize int
	flushTimer *time.Timer
}

// NewAuditLogger 创建新的审计日志管理器
func NewAuditLogger(logDir string, maxSize int64, maxDays int) (*AuditLogger, error) {
	// 创建日志目录
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	al := &AuditLogger{
		logDir:     logDir,
		maxSize:    maxSize,
		maxDays:    maxDays,
		bufferSize: 100, // 缓冲区大小
		logBuffer:  make([]AuditLog, 0, 100),
	}

	// 打开当前日志文件
	if err := al.openLogFile(); err != nil {
		return nil, err
	}

	// 启动定时刷新
	al.startFlushTimer()

	// 启动清理任务
	go al.cleanupOldLogs()

	return al, nil
}

// 打开日志文件
func (al *AuditLogger) openLogFile() error {
	fileName := fmt.Sprintf("audit_%s.log", time.Now().Format("2006-01-02"))
	filePath := filepath.Join(al.logDir, fileName)

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %v", err)
	}

	if al.logFile != nil {
		al.logFile.Close()
	}

	al.logFile = file
	al.logger = log.New(file, "", 0)

	return nil
}

// 生成唯一ID
func (al *AuditLogger) generateID() string {
	return fmt.Sprintf("%d_%d", time.Now().UnixNano(), os.Getpid())
}

// Log 记录审计日志
func (al *AuditLogger) Log(level LogLevel, category LogCategory, action, result, message string, details map[string]interface{}) {
	auditLog := AuditLog{
		ID:        al.generateID(),
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Action:    action,
		Result:    result,
		Message:   message,
		Details:   details,
	}

	al.writeLog(auditLog)
}

// LogWithContext 基于Gin上下文记录日志
func (al *AuditLogger) LogWithContext(c *gin.Context, level LogLevel, category LogCategory, action, result, message string, details map[string]interface{}) {
	auditLog := AuditLog{
		ID:         al.generateID(),
		Timestamp:  time.Now(),
		Level:      level,
		Category:   category,
		Action:     action,
		ClientIP:   c.ClientIP(),
		UserAgent:  c.GetHeader("User-Agent"),
		RequestURI: c.Request.RequestURI,
		Method:     c.Request.Method,
		Result:     result,
		Message:    message,
		Details:    details,
	}

	// 从上下文获取用户信息
	if userID, exists := c.Get("user_id"); exists {
		auditLog.UserID = fmt.Sprintf("%v", userID)
	}
	if username, exists := c.Get("username"); exists {
		auditLog.Username = fmt.Sprintf("%v", username)
	}

	al.writeLog(auditLog)
}

// LogLogin 记录登录日志
func (al *AuditLogger) LogLogin(clientIP, username string, success bool, errorMsg string) {
	result := "SUCCESS"
	level := LogLevelInfo
	message := fmt.Sprintf("用户 %s 登录成功", username)

	if !success {
		result = "FAILURE"
		level = LogLevelSecurity
		message = fmt.Sprintf("用户 %s 登录失败", username)
	}

	auditLog := AuditLog{
		ID:        al.generateID(),
		Timestamp: time.Now(),
		Level:     level,
		Category:  CategoryAuth,
		Action:    "LOGIN",
		Username:  username,
		ClientIP:  clientIP,
		Result:    result,
		Message:   message,
		ErrorMsg:  errorMsg,
	}

	al.writeLog(auditLog)
}

// LogAPIAccess 记录API访问日志
func (al *AuditLogger) LogAPIAccess(c *gin.Context, statusCode int, duration time.Duration, errorMsg string) {
	level := LogLevelInfo
	result := "SUCCESS"

	if statusCode >= 400 {
		level = LogLevelError
		result = "FAILURE"
	}

	auditLog := AuditLog{
		ID:         al.generateID(),
		Timestamp:  time.Now(),
		Level:      level,
		Category:   CategoryAPI,
		Action:     "API_ACCESS",
		ClientIP:   c.ClientIP(),
		UserAgent:  c.GetHeader("User-Agent"),
		RequestURI: c.Request.RequestURI,
		Method:     c.Request.Method,
		StatusCode: statusCode,
		Duration:   duration,
		Result:     result,
		Message:    fmt.Sprintf("API请求: %s %s", c.Request.Method, c.Request.RequestURI),
		ErrorMsg:   errorMsg,
	}

	// 从上下文获取用户信息
	if userID, exists := c.Get("user_id"); exists {
		auditLog.UserID = fmt.Sprintf("%v", userID)
	}
	if username, exists := c.Get("username"); exists {
		auditLog.Username = fmt.Sprintf("%v", username)
	}

	al.writeLog(auditLog)
}

// LogFileOperation 记录文件操作日志
func (al *AuditLogger) LogFileOperation(clientIP, username, operation, filePath string, success bool, errorMsg string) {
	result := "SUCCESS"
	level := LogLevelInfo

	if !success {
		result = "FAILURE"
		level = LogLevelError
	}

	auditLog := AuditLog{
		ID:        al.generateID(),
		Timestamp: time.Now(),
		Level:     level,
		Category:  CategoryFile,
		Action:    operation,
		Username:  username,
		ClientIP:  clientIP,
		Result:    result,
		Message:   fmt.Sprintf("文件操作: %s %s", operation, filePath),
		ErrorMsg:  errorMsg,
		Details: map[string]interface{}{
			"file_path": filePath,
			"operation": operation,
		},
	}

	al.writeLog(auditLog)
}

// LogSecurityEvent 记录安全事件
func (al *AuditLogger) LogSecurityEvent(clientIP, eventType, description string, details map[string]interface{}) {
	auditLog := AuditLog{
		ID:        al.generateID(),
		Timestamp: time.Now(),
		Level:     LogLevelSecurity,
		Category:  CategorySecurity,
		Action:    eventType,
		ClientIP:  clientIP,
		Result:    "DETECTED",
		Message:   description,
		Details:   details,
	}

	al.writeLog(auditLog)
}

// auditFlushCh 审计落盘任务队列（缓冲满时投递，由后台 worker 落盘，避免 fsync 阻塞请求线程）
var auditFlushCh = make(chan []AuditLog, 16)
var auditFlushOnce sync.Once

// ensureFlushWorker 确保后台落盘 worker 已启动
func ensureFlushWorker() {
	auditFlushOnce.Do(func() {
		go func() {
			for batch := range auditFlushCh {
				GlobalAuditLogger.flushBatch(batch)
			}
		}()
	})
}

// writeLog 写入日志到缓冲区。缓冲满时仅做 swap（锁内轻量），
// 实际落盘（JSON 序列化 + 写文件 + fsync + 轮转）投递给后台 worker，
// 避免高 RPS 下 fsync/轮转阻塞请求线程。
func (al *AuditLogger) writeLog(auditLog AuditLog) {
	al.mu.Lock()
	al.logBuffer = append(al.logBuffer, auditLog)
	needFlush := len(al.logBuffer) >= al.bufferSize
	if needFlush {
		batch := al.logBuffer
		al.logBuffer = make([]AuditLog, 0, al.bufferSize)
		al.mu.Unlock()
		ensureFlushWorker()
		select {
		case auditFlushCh <- batch:
		default:
			// 队列已满：丢弃本批（避免内存堆积；审计丢失可接受，实时性优先）
			logx.Infof("[AUDIT] 落盘队列已满，丢弃 %d 条审计日志\n", len(batch))
		}
		return
	}
	al.mu.Unlock()
}

// flushBatch 将一批日志序列化并落盘（须在 mu 锁外调用）。
func (al *AuditLogger) flushBatch(batch []AuditLog) {
	if len(batch) == 0 {
		return
	}
	al.flushMu.Lock()
	defer al.flushMu.Unlock()

	// 检查是否需要轮转日志文件
	al.checkAndRotateLog()

	for _, logEntry := range batch {
		logData, err := json.Marshal(logEntry)
		if err != nil {
			logx.Infof("序列化日志失败: %v\n", err)
			continue
		}

		al.logger.Printf("%s\n", string(logData))
	}

	// 强制刷新到磁盘
	if al.logFile != nil {
		al.logFile.Sync()
	}
}

// flushBuffer 刷新当前缓冲区（锁内 swap 后投递后台落盘）。
func (al *AuditLogger) flushBuffer() {
	al.mu.Lock()
	batch := al.logBuffer
	al.logBuffer = make([]AuditLog, 0, al.bufferSize)
	al.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	ensureFlushWorker()
	select {
	case auditFlushCh <- batch:
	default:
	}
}

// 检查并轮转日志文件
func (al *AuditLogger) checkAndRotateLog() {
	if al.logFile == nil {
		return
	}

	// 检查文件大小
	fileInfo, err := al.logFile.Stat()
	if err != nil {
		return
	}

	// 检查是否需要按日期轮转
	currentDate := time.Now().Format("2006-01-02")
	expectedFileName := fmt.Sprintf("audit_%s.log", currentDate)
	if filepath.Base(al.logFile.Name()) != expectedFileName || fileInfo.Size() > al.maxSize {
		al.openLogFile()
	}
}

// 启动定时刷新
func (al *AuditLogger) startFlushTimer() {
	al.flushTimer = time.AfterFunc(30*time.Second, func() {
		al.flushBuffer()
		al.startFlushTimer() // 重新启动定时器
	})
}

// 清理旧日志
func (al *AuditLogger) cleanupOldLogs() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		al.performCleanup()
	}
}

// 执行清理操作
func (al *AuditLogger) performCleanup() {
	cutoff := time.Now().AddDate(0, 0, -al.maxDays)

	err := filepath.Walk(al.logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.ModTime().Before(cutoff) {
			if filepath.Ext(path) == ".log" {
				logx.Infof("🗑️ 删除过期日志文件: %s\n", path)
				return os.Remove(path)
			}
		}

		return nil
	})

	if err != nil {
		logx.Infof("清理日志文件时出错: %v\n", err)
	}
}

// Close 关闭审计日志管理器
func (al *AuditLogger) Close() error {
	// 停止定时器
	if al.flushTimer != nil {
		al.flushTimer.Stop()
	}

	// 刷新剩余日志（flushBuffer 内部自锁 swap + flushMu 落盘，不能在持有 mu 时调用）
	al.flushBuffer()

	// 关闭文件
	al.flushMu.Lock()
	defer al.flushMu.Unlock()
	if al.logFile != nil {
		return al.logFile.Close()
	}

	return nil
}

// 全局审计日志实例
var GlobalAuditLogger *AuditLogger

// InitAuditLogger 初始化全局审计日志
func InitAuditLogger(logDir string, maxSize int64, maxDays int) error {
	var err error
	GlobalAuditLogger, err = NewAuditLogger(logDir, maxSize, maxDays)
	return err
}
