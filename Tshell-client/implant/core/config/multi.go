package config

import "strings"

// 多传输 endpoint 列表（阶段三 · 单二进制多传输）。
//
// 512 字节定长占位符：服务端 patch 管线要求**等长替换**（超长替换会让 PE 节偏移
// 错位，曾导致 OOM）。生成多传输载荷时服务端把它替换成逗号分隔的
// "scheme://address" 列表（不足部分以空格填充）。
//
// 未 patch 时保持占位符原样；单传输构建不读取它。
const MultiEndpointsPlaceholderPrefix = "MULTI_ENDPOINTS_PLACEHOLDER_"

// MultiEndpoints 端点列表占位符（512 字节）
var MultiEndpoints = "MULTI_ENDPOINTS_PLACEHOLDER_0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"

// MultiEndpointsActive 是否已被服务端 patch 为真实端点列表
func MultiEndpointsActive() bool {
	return !strings.HasPrefix(MultiEndpoints, MultiEndpointsPlaceholderPrefix)
}

// MultiEndpointsList 返回端点列表（已 patch 时），否则返回 nil。
// 服务端以空格右填充到定长，这里去掉尾部填充与空白项。
func MultiEndpointsList() []string {
	if !MultiEndpointsActive() {
		return nil
	}
	out := make([]string, 0, 4)
	for _, item := range strings.Split(MultiEndpoints, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
