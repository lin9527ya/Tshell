// Package logx 服务端统一日志分级输出。
// 级别：error < warn < info < debug。
// - error：错误（始终输出，必要信息）
// - warn：警告 + 错误
// - info：事件（上线/离线/监听器操作）+ 警告 + 错误（默认，开发/生产通用）
// - debug：全部（含每消息/每连接的调试噪音）
package logx

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level 日志级别
type Level int

const (
	LvlError Level = iota
	LvlWarn
	LvlInfo
	LvlDebug
)

var (
	mu      sync.Mutex
	writeMu sync.Mutex
	lvl     = LvlInfo
	// jsonMode 输出格式（config.logging.format：text|json）。json 模式每行一个 JSON 对象，
	// 便于采集/检索；默认 text 保持原有带 emoji 的人类可读输出。
	jsonMode bool
	// out 输出目标（config.logging.output：stdout|file）。
	out io.Writer = os.Stdout
)

// SetFormat 设置输出格式：json 或 text（其它值按 text）
func SetFormat(f string) {
	mu.Lock()
	jsonMode = strings.EqualFold(f, "json")
	mu.Unlock()
}

// SetOutput 设置输出目标（默认 stdout）
func SetOutput(w io.Writer) {
	if w == nil {
		return
	}
	mu.Lock()
	out = w
	mu.Unlock()
}

// SetOutputFile 以追加方式打开日志文件并切换输出（权限 0600：日志可能含主机名/UID 等敏感信息）
func SetOutputFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	SetOutput(f)
	return nil
}

// logf 统一输出：text 模式按调用方格式原样输出；json 模式包成单行 JSON。
func logf(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	mu.Lock()
	jm, w := jsonMode, out
	mu.Unlock()

	writeMu.Lock()
	defer writeMu.Unlock()
	if jm {
		b, err := json.Marshal(map[string]string{
			"time":  time.Now().Format(time.RFC3339),
			"level": level,
			"msg":   strings.TrimRight(msg, "\n"),
		})
		if err == nil {
			fmt.Fprintf(w, "%s\n", b)
			return
		}
	}
	fmt.Fprintf(w, "%s", msg)
}

// SetLevel 设置全局日志级别
func SetLevel(l Level) {
	mu.Lock()
	defer mu.Unlock()
	lvl = l
}

// ParseLevel 从配置字符串解析级别（debug/info/warn/error，未知值回退 info）
func ParseLevel(s string) Level {
	switch s {
	case "debug", "DEBUG", "Debug", "trace":
		return LvlDebug
	case "warn", "warning", "WARN":
		return LvlWarn
	case "error", "Error", "ERROR", "fatal":
		return LvlError
	default:
		return LvlInfo
	}
}

// CurrentLevel 当前级别
func CurrentLevel() Level {
	mu.Lock()
	defer mu.Unlock()
	return lvl
}

// Debugf 调试级日志（每消息/每连接等高频噪音，仅 debug 级别输出）
func Debugf(format string, args ...interface{}) {
	mu.Lock()
	l := lvl
	mu.Unlock()
	if l >= LvlDebug {
		logf("debug", format, args...)
	}
}

// Infof 事件级日志（上线/离线/操作等必要信息）
func Infof(format string, args ...interface{}) {
	mu.Lock()
	l := lvl
	mu.Unlock()
	if l >= LvlInfo {
		logf("info", format, args...)
	}
}

// Warnf 警告级日志
func Warnf(format string, args ...interface{}) {
	mu.Lock()
	l := lvl
	mu.Unlock()
	if l >= LvlWarn {
		logf("warn", format, args...)
	}
}

// Errorf 错误级日志（始终输出）
func Errorf(format string, args ...interface{}) {
	logf("error", format, args...)
}

// Println 事件级行输出
func Println(args ...interface{}) {
	mu.Lock()
	l := lvl
	mu.Unlock()
	if l >= LvlInfo {
		logf("info", "%s", fmt.Sprintln(args...))
	}
}
