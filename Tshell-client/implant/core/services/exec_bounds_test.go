package services

import (
	"encoding/binary"
	"testing"
)

// TestExecPayloadBounds 畸形执行载荷必须返回错误而不是 panic。
// 历史问题：fileLen == len(cmdBuf)-4 时直接切 argsLen 会越界 panic，
// dispatch 的 recover 只打日志、不回执行完成回执，服务端任务会挂到超时。
func TestExecPayloadBounds(t *testing.T) {
	// 文件长度恰好占满剩余字节，但没有 argsLen 字段
	exact := make([]byte, 8)
	binary.BigEndian.PutUint32(exact[:4], 4) // fileLen=4，剩下只有文件体
	copy(exact[4:], []byte("ABCD"))

	// argsLen 字段存在但声明的长度超出剩余字节
	badArgs := make([]byte, 12)
	binary.BigEndian.PutUint32(badArgs[:4], 4)
	copy(badArgs[4:8], []byte("ABCD"))
	binary.BigEndian.PutUint32(badArgs[8:12], 999)

	tests := []struct {
		name string
		fn   func([]byte) ([]byte, error)
	}{
		{"bof", CmdExecuteBof},
		{"linux-bin", CmdExecuteLinuxBin},
		{"exec-file", CmdExecuteFile},
	}
	for _, tc := range tests {
		for _, payload := range [][]byte{exact, badArgs} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s: 畸形载荷触发 panic: %v", tc.name, r)
					}
				}()
				if _, err := tc.fn(payload); err == nil {
					t.Errorf("%s: 畸形载荷应返回错误", tc.name)
				}
			}()
		}
	}

	// script：长度头合法、无 args 的最短载荷应能继续执行（到命令层再报平台/脚本错误均可）
	script := make([]byte, 4)
	binary.BigEndian.PutUint32(script[:4], 0)
	if _, err := CmdExecuteLinuxScript(script); err != nil {
		// 0 长度脚本会被 commands 层拒绝，这里只要求不 panic
		t.Logf("script empty -> %v", err)
	}
}
