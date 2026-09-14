//go:build windows

package commands

// RegQuery 查询 Windows 注册表路径/值（带超时，防止 reg 命令挂起阻塞客户端）
func RegQuery(path string) ([]byte, error) {
	return execWithTimeout("reg", "query", path)
}
