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
