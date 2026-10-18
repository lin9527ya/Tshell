//go:build !windows

package services

import "errors"

// cryptUnprotect 在非 Windows 上没有 DPAPI，浏览器凭据模块不可用。
// 显式报错交给上层（服务端也会在非 Windows 会话上直接拒绝），不做静默降级。
func cryptUnprotect(data []byte) ([]byte, error) {
	return nil, errors.New("DPAPI 仅 Windows 可用")
}
