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

func HideFile(filePath string) error {
	return nil
}

func UnhideFile(filePath string) error {
	return nil
}

func TakeScreenshot(quality int) ([]byte, error) {
	return nil, errors.New("屏幕截图功能在Darwin平台暂不支持")
}

func StartScreenMonitor(frameRate int) error {
	return errors.New("屏幕监控功能在Darwin平台暂不支持")
}

func StopScreenMonitor() error {
	return nil
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
