package app

import (
	"implant/core/config"
	"strings"
	"sync"
)

// B7: 多 endpoint 轮转。config.Host 可为逗号分隔的多个 C2 地址（"addr1,addr2,..."）。
// 单地址（无逗号）时始终返回该地址，向后兼容。adapter 连接失败时调用 AdvanceEndpoint 轮转。

var (
	endpoints  []string
	epIdx      int
	epMu       sync.Mutex
	epInitOnce sync.Once
)

func initEndpoints() {
	// 多传输构建：服务端 patch 了端点列表（scheme://addr,...），优先使用；
	// 单传输构建或未 patch 时回落到 config.Host（历史行为不变）。
	if multi := config.MultiEndpointsList(); len(multi) > 0 {
		endpoints = multi
		return
	}
	raw := strings.ReplaceAll(config.Host, " ", "")
	for _, h := range strings.Split(raw, ",") {
		h = strings.TrimSpace(h)
		if h != "" {
			endpoints = append(endpoints, h)
		}
	}
	if len(endpoints) == 0 {
		endpoints = []string{raw}
	}
}

// CurrentEndpoint 返回当前 C2 endpoint。
func CurrentEndpoint() string {
	epInitOnce.Do(initEndpoints)
	epMu.Lock()
	defer epMu.Unlock()
	if epIdx >= len(endpoints) {
		epIdx = 0
	}
	return endpoints[epIdx]
}

// endpointSelector 可选的"下一个端点"选择器：多传输构建注入健康度选择
// （见 cmd/single/transport_multi.go）。返回空串表示沿用默认轮转。
// 签名：current 为当前端点，all 为全部端点；返回应切换到的端点。
var endpointSelector func(current string, all []string) string

// SetEndpointSelector 注入端点选择器（仅多传输构建使用）
func SetEndpointSelector(f func(current string, all []string) string) {
	epMu.Lock()
	endpointSelector = f
	epMu.Unlock()
}

// AllEndpoints 返回全部端点的副本（健康探测用）
func AllEndpoints() []string {
	epInitOnce.Do(initEndpoints)
	epMu.Lock()
	defer epMu.Unlock()
	out := make([]string, len(endpoints))
	copy(out, endpoints)
	return out
}

// SetCurrentEndpoint 把当前端点切到指定值（存在时）。健康探测触发切换后调用，
// 保证后续 AdvanceEndpoint 的轮转顺序与实际使用的端点一致。
func SetCurrentEndpoint(ep string) {
	epInitOnce.Do(initEndpoints)
	epMu.Lock()
	defer epMu.Unlock()
	for i, e := range endpoints {
		if e == ep {
			epIdx = i
			return
		}
	}
}

// AdvanceEndpoint 轮转到下一个 endpoint（连接失败时调用），返回新的当前 endpoint。
// 若注入了选择器（多传输构建），优先选健康端点；全都不健康时退回顺序轮转。
func AdvanceEndpoint() string {
	epInitOnce.Do(initEndpoints)
	epMu.Lock()
	defer epMu.Unlock()
	if endpointSelector != nil {
		if next := endpointSelector(endpoints[epIdx], endpoints); next != "" {
			for i, e := range endpoints {
				if e == next {
					epIdx = i
					return endpoints[epIdx]
				}
			}
		}
	}
	epIdx = (epIdx + 1) % len(endpoints)
	return endpoints[epIdx]
}
