package config

import (
	"os"
	"path/filepath"
)

// GetPluginDir 获取插件目录。
// 解析优先级（首个存在或可创建的目录）：
//  1. 环境变量 TSHELL_PLUGIN_DIR（显式指定，最可靠）
//  2. 可执行文件目录/Plugins（原行为：源码目录运行 go run 时即 Tshell-server/Plugins）
//  3. 当前工作目录/Plugins（部署二进制在别处、cwd 为服务端目录时）
//  4. 当前工作目录/plugins（兼容小写目录）
//
// 修复：原实现仅依赖 os.Executable() 目录——部署二进制与源码目录分离时，
// 插件上传/列表指向二进制旁的 Plugins，用户往源码目录放的文件不显示。
func GetPluginDir() string {
	if env := os.Getenv("TSHELL_PLUGIN_DIR"); env != "" {
		return env
	}

	candidates := []string{}
	if execPath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(execPath), "Plugins"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "Plugins"), filepath.Join(cwd, "plugins"))
	}

	for _, dir := range candidates {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	// 全部不存在：返回可执行目录/Plugins（调用方会 MkdirAll 创建）
	if len(candidates) > 0 {
		return candidates[0]
	}
	return "Plugins"
}
