package command

import "sync"

// 客户端下载制品的落盘根目录。
// 由 main 读取配置后通过 SetDownloadsDir 注入 —— command 不能直接导入 pkg/config
// （config → security → command 会构成导入环）。未注入时回落历史默认 "./Downloads"。
var (
	downloadsDirMu sync.RWMutex
	downloadsDir   = "./Downloads"
)

// SetDownloadsDir 注入下载制品根目录（启动时调用一次）。
func SetDownloadsDir(dir string) {
	if dir == "" {
		return
	}
	downloadsDirMu.Lock()
	downloadsDir = dir
	downloadsDirMu.Unlock()
}

// DownloadsDir 返回当前下载制品根目录。
func DownloadsDir() string {
	downloadsDirMu.RLock()
	defer downloadsDirMu.RUnlock()
	return downloadsDir
}
