//go:build !windows

package commands

import "errors"

// EvasionControl 运行期对抗开关（非 Windows 无实现）。
//
// 不提供"空实现返回成功"：这类命令的语义是"改了就会影响本机行为"，
// 在 linux/darwin 上返回成功会让操作员以为开关起作用了。
func EvasionControl(cmdBuf []byte) ([]byte, error) {
	return nil, errors.New("对抗能力开关仅支持 Windows 客户端")
}
