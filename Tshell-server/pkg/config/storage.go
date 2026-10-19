package config

// GetDownloadsDir 返回客户端下载制品的落盘根目录。
// 未配置时回落 "./Downloads"（与历史行为一致）。
func GetDownloadsDir() string {
	if g := GetGlobalConfig(); g != nil && g.Storage.DownloadsDir != "" {
		return g.Storage.DownloadsDir
	}
	return "./Downloads"
}
