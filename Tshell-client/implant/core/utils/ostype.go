package utils

import "runtime"

// osTypeName 客户端 OS 类型标记(随元数据上报;服务端 FirstBlood 解析存 Os 字段)
// 值: linux / windows / darwin / other
func osTypeName() string {
	switch runtime.GOOS {
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	case "darwin":
		return "darwin"
	}
	return "other"
}
