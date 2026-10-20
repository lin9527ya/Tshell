//go:build windows && !amd64

// 非 amd64 Windows 平台：unhook 依赖 PE 解析 + 机器码回写，暂只支持 amd64。
// 这里给出同签名空实现，保证 windows/386 与 arm64 也能编译（命令返回明确的不支持提示，
// 而不是编译失败或静默无操作）。
package evasion

import "errors"

// ModuleUnhookReport 单模块 unhook 结果（非 amd64 平台上恒为"Skipped"）
type ModuleUnhookReport struct {
	Module   string
	Base     uintptr
	Sections int
	Compared int
	Restored int
	Skipped  string
}

// String 一行人类可读描述
func (r ModuleUnhookReport) String() string {
	return r.Module + "：当前架构不支持 unhook（仅 amd64）"
}

// DefaultUnhookModules 默认恢复的模块（非 amd64 未生效，保留以对齐调用方）
var DefaultUnhookModules = []string{"ntdll.dll", "kernel32.dll", "kernelbase.dll"}

// UnhookModule 非 amd64 平台不支持
func UnhookModule(name string) (ModuleUnhookReport, error) {
	return ModuleUnhookReport{Module: name, Skipped: "架构不支持"}, errors.New("unhook 仅支持 windows/amd64")
}

// UnhookAll 非 amd64 平台不支持
func UnhookAll() ([]ModuleUnhookReport, error) {
	return nil, errors.New("unhook 仅支持 windows/amd64")
}
