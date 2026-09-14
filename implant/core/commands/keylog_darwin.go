//go:build darwin

package commands

import "errors"

// StartKeylog 在 darwin 上暂不支持（需辅助功能权限 + IOKit 钩子）。
func StartKeylog() error { return errors.New("键盘记录在 darwin 暂不支持") }

// GetKeylog 返回缓冲（darwin 无）
func GetKeylog() []byte { return nil }

// StopKeylog 停止（darwin 无）
func StopKeylog() error { return errors.New("键盘记录在 darwin 暂不支持") }
