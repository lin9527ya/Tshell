//go:build !windows

package services

import "errors"

// 非 Windows 平台没有 LSASS / DPAPI / 浏览器配置目录这些 Windows 专有路径。
// 明确报错而不是静默返回空文件：静默失败会让服务端把空制品当成"已获取"。
func dumpCredential(kind, tech, fileName string) (string, error) {
	return "", errors.New("凭据获取当前仅支持 Windows（" + kind + "）")
}

// collectDPAPIMasterKeys 非 Windows 无 DPAPI
func collectDPAPIMasterKeys(destPath string) (string, int, error) {
	return "", 0, errors.New("DPAPI 主密钥采集仅支持 Windows")
}

// collectBrowserCredentials 非 Windows 没有 Chromium 的 User Data 布局与 DPAPI 主密钥
func collectBrowserCredentials(tech string) (CollectedBrowserArtifact, error) {
	return CollectedBrowserArtifact{}, errors.New("浏览器凭据采集当前仅支持 Windows")
}
