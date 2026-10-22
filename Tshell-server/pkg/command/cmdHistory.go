package command

import (
	"sync"
	"time"
)

// ===== 任务队列可视化:单会话命令下发时间线 =====
// 记录每客户端已下发命令(入队/客户端取走),供前端"任务"面板展示。
// 注意:协议无逐命令 ACK,状态为近似(queued=服务端待取 / removed=客户端已取走);
// 是否回执由操作员在控制台输出观察。

// CmdRecord 命令下发记录
type CmdRecord struct {
	Command string    `json:"command"`
	Status  string    `json:"status"` // queued | removed
	SentAt  time.Time `json:"sent_at"`
}

// clientCmdHistory 单客户端命令历史
type clientCmdHistory struct {
	mu       sync.Mutex
	records  []*CmdRecord // 最近记录(头部最新)
	max      int
	lastPull time.Time // 客户端最近一次取走命令时间
	lastSeen time.Time // 最近一次活动(入队/取走)
}

// CmdHistory 全局命令历史管理
type CmdHistory struct {
	mu      sync.Mutex
	clients map[string]*clientCmdHistory
	ttl     time.Duration
}

// GlobalCmdHistory 全局命令历史
var GlobalCmdHistory = &CmdHistory{
	clients: make(map[string]*clientCmdHistory),
	ttl:     30 * time.Minute,
}

// MaxRecordedCommandBytes 单条命令进入"任务时间线"历史的大小上限。
// 插件/内存执行可携带几十 MB 二进制；轮询传输下整包命令会再复制成 string，
// 100 条历史会成倍放大内存，因此大命令只更新时间戳、不记录内容。
const MaxRecordedCommandBytes = 1 << 20

// Touch 只更新最近活动时间（大命令不记录内容时使用）。
func (h *CmdHistory) Touch(clientID string) {
	if h == nil {
		return
	}
	now := time.Now()
	h.mu.Lock()
	ch := h.clients[clientID]
	h.mu.Unlock()
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.lastSeen = now
	ch.mu.Unlock()
}

// TouchPull 更新"客户端已取走"时间与最近活动时间（大命令跳过内容匹配时使用）。
func (h *CmdHistory) TouchPull(clientID string) {
	if h == nil {
		return
	}
	now := time.Now()
	h.mu.Lock()
	ch := h.clients[clientID]
	h.mu.Unlock()
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.lastPull = now
	ch.lastSeen = now
	ch.mu.Unlock()
}

// Record queued 入队记录(在 ClientCommandQueue.AddCommand 挂钩)
func (h *CmdHistory) Record(clientID, command string) {
	now := time.Now()
	h.mu.Lock()
	ch := h.clients[clientID]
	if ch == nil {
		ch = &clientCmdHistory{max: 100}
		h.clients[clientID] = ch
	}
	h.mu.Unlock()

	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.records = append([]*CmdRecord{{Command: command, Status: "queued", SentAt: now}}, ch.records...)
	if len(ch.records) > ch.max {
		ch.records = ch.records[:ch.max]
	}
	ch.lastSeen = now
}

// MarkRemoved 客户端取走命令(在 ClientCommandQueue.GetCommand 挂钩)
func (h *CmdHistory) MarkRemoved(clientID string, command string) {
	now := time.Now()
	h.mu.Lock()
	ch := h.clients[clientID]
	h.mu.Unlock()
	if ch == nil {
		return
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.lastPull = now
	ch.lastSeen = now
	// 匹配最近一条同命令的 queued 记录
	for _, r := range ch.records {
		if r.Status == "queued" && r.Command == command {
			r.Status = "removed"
			break
		}
	}
}

// Snapshot 返回客户端命令历史快照(时间倒序)
func (h *CmdHistory) Snapshot(clientID string) (records []*CmdRecord, lastPull time.Time, lastSeen time.Time) {
	h.mu.Lock()
	ch := h.clients[clientID]
	h.mu.Unlock()
	if ch == nil {
		return nil, time.Time{}, time.Time{}
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	out := make([]*CmdRecord, len(ch.records))
	copy(out, ch.records)
	return out, ch.lastPull, ch.lastSeen
}

// CleanupExpired 清理 TTL 过期客户端记录(由 main 定时调用)
func (h *CmdHistory) CleanupExpired() {
	cutoff := time.Now().Add(-h.ttl)
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.clients {
		ch.mu.Lock()
		expired := ch.lastSeen.Before(cutoff)
		ch.mu.Unlock()
		if expired {
			delete(h.clients, id)
		}
	}
}
