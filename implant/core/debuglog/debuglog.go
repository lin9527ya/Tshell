//go:build debug

package debuglog

import "fmt"

// Printf 调试日志输出（debug tag 构建：完整输出）。
// 生产构建（无 debug tag）使用空实现 —— 调用点被内联优化剔除，
// 日志字符串常量不再被引用 → 链接器从二进制中剔除（静态分析不可提取）。
func Printf(format string, args ...interface{}) {
	fmt.Printf(format, args...)
}

// Println 调试日志输出（debug 构建）
func Println(args ...interface{}) {
	fmt.Println(args...)
}
