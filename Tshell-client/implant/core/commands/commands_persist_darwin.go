//go:build darwin

package commands

import "errors"

// Persist 在 darwin 上暂不支持持久化（占位，后续按需实现 LaunchAgent/LaunchDaemon）。
func Persist(technique string) ([]byte, error) {
	return nil, errors.New("持久化在 darwin 暂不支持")
}
