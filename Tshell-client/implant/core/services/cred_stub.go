//go:build !windows

package services

import "errors"

// dumpCredential 非 Windows 平台没有 LSASS 这类进程内存转储能力。
// 明确报错而不是静默返回空文件：静默失败会让服务端把空制品当成"已获取"。
func dumpCredential(kind, tech, fileName string) (string, error) {
	return "", errors.New("凭据获取当前仅支持 Windows（" + kind + "）")
}
