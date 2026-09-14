//go:build darwin

package commands

import "errors"

// RegQuery 注册表仅 Windows 支持。
func RegQuery(path string) ([]byte, error) {
	return nil, errors.New("注册表仅 Windows 支持")
}
