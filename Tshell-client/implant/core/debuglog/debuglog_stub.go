//go:build !debug

package debuglog

// Printf 空实现（生产构建）：调用点被内联优化，字符串常量被链接器剔除。
func Printf(format string, args ...interface{}) {}

// Println 空实现（生产构建）
func Println(args ...interface{}) {}
