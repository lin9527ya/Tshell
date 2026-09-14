package security

import (
	"regexp"
	"strconv"
	"strings"
)

// dynamicRandRe 匹配伪装 header 值中的随机占位符 {{RAND:N}}。
// initObfuscationPatterns / initMimicProfiles 在启动时只写入占位符，
// 由 materializeDynamicHeaders 在每次构建请求时替换为新生成的随机串，
// 避免“启动时一次性生成、全生命周期不变且所有客户端共用”导致的静态指纹。
var dynamicRandRe = regexp.MustCompile(`\{\{RAND:(\d+)\}\}`)

// materializeDynamicHeaders 将 headers 值中的 {{RAND:N}} 占位符替换为每请求新生成的随机串。
// 同一值内的多个占位符各自独立生成。
func materializeDynamicHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return headers
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		v = dynamicRandRe.ReplaceAllStringFunc(v, func(match string) string {
			sub := dynamicRandRe.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}
			n, err := strconv.Atoi(sub[1])
			if err != nil || n <= 0 {
				return match
			}
			return generateRandomString(n)
		})
		v = strings.ReplaceAll(v, "{{WSKEY}}", generateWebSocketKey())
		out[k] = v
	}
	return out
}
