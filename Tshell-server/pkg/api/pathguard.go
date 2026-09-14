package api

import (
	"strings"
)

// ValidateDangerousPath 危险路径防御（仅拦截危险路径，不做严格校验）：
// 拒绝路径穿越(..)、主目录展开(~)、控制字符、空字节、超长路径、
// Windows 保留设备名与通配符。绝对路径与盘符路径正常放行。
// 返回 "" 表示安全；否则返回错误描述。
func ValidateDangerousPath(path string) string {
	if len(path) == 0 {
		return "路径为空"
	}
	if len(path) > 260 {
		return "路径长度超过260字符"
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7F {
			return "路径包含控制字符"
		}
	}
	if strings.ContainsRune(path, 0x00) {
		return "路径包含空字节"
	}
	if strings.Contains(path, "~") {
		return "路径不允许包含~字符"
	}
	if strings.Contains(path, "*") || strings.Contains(path, "?") {
		return "路径不允许包含通配符"
	}

	// 路径穿越检测（按分隔符切分，检查 .. 段）
	seps := func(r rune) bool { return r == '/' || r == '\\' }
	parts := strings.FieldsFunc(path, seps)
	for _, p := range parts {
		if p == ".." {
			return "路径不允许包含..穿越"
		}
	}
	if strings.HasSuffix(path, "..") {
		return "路径不允许包含..穿越"
	}

	// Windows 保留设备名（任意扩展名/子路径均拒绝）
	reserved := []string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	}
	first := strings.ToUpper(strings.TrimLeft(path, "/\\"))
	if dot := strings.IndexAny(first, "./\\"); dot > 0 {
		first = first[:dot]
	}
	if slash := strings.IndexAny(first, "/\\"); slash > 0 {
		first = first[:slash]
	}
	for _, r := range reserved {
		if first == r {
			return "路径使用了保留设备名: " + r
		}
	}
	return ""
}
