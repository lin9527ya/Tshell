// Package logx 服务端统一日志分级输出。
// 级别：error < warn < info < debug。
// - error：错误（始终输出，必要信息）
// - warn：警告 + 错误
// - info：事件（上线/离线/监听器操作）+ 警告 + 错误（默认，开发/生产通用）
// - debug：全部（含每消息/每连接的调试噪音）
package logx

import (
	"fmt"
	"sync"
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
	mu  sync.Mutex
	lvl = LvlInfo
)

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
		fmt.Printf(format, args...)
	}
}

// Infof 事件级日志（上线/离线/操作等必要信息）
func Infof(format string, args ...interface{}) {
	mu.Lock()
	l := lvl
	mu.Unlock()
	if l >= LvlInfo {
		fmt.Printf(format, args...)
	}
}

// Warnf 警告级日志
func Warnf(format string, args ...interface{}) {
	mu.Lock()
	l := lvl
	mu.Unlock()
	if l >= LvlWarn {
		fmt.Printf(format, args...)
	}
}

// Errorf 错误级日志（始终输出）
func Errorf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Println 事件级行输出
func Println(args ...interface{}) {
	mu.Lock()
	l := lvl
	mu.Unlock()
	if l >= LvlInfo {
		fmt.Println(args...)
	}
}
