package command

import (
	"sync"
	"time"
)

// ===== B11: 事件总线 =====
// 客户端上线/下线/命令执行等事件发布-订阅总线，
// 供 /api/events/ws WebSocket 推送前端实时展示（替代轮询）。

// EventType 事件类型
type EventType string

const (
	EventClientOnline   EventType = "client_online"   // 客户端上线
	EventClientOffline  EventType = "client_offline"  // 客户端离线
	EventClientNew      EventType = "client_new"      // 新客户端（FirstBlood）
	EventCmdOutput      EventType = "cmd_output"      // 命令输出
	EventDownloadDone   EventType = "download_done"   // 下载完成
	EventSecurityBlock  EventType = "security_block"  // 安全拦截
	EventLoginFailed    EventType = "login_failed"    // 登录失败
	EventOperatorAction EventType = "operator_action" // 操作员动作（协作：锁/命令/高危操作）
	EventChat           EventType = "chat"            // 操作员聊天（团队频道）
)

// C2Event 事件负载
type C2Event struct {
	Type      EventType              `json:"type"`
	UID       string                 `json:"uid,omitempty"`
	Timestamp int64                  `json:"timestamp"`
	Detail    map[string]interface{} `json:"detail,omitempty"`
}

// SubscriberCount 当前订阅者数（指标用）
func (b *EventBus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

// EventBus 事件总线（订阅者 chan 广播）
type EventBus struct {
	mu      sync.RWMutex
	subs    map[chan C2Event]struct{}
	buffers int
}

// GlobalEventBus 全局事件总线
var GlobalEventBus = &EventBus{
	subs:    make(map[chan C2Event]struct{}),
	buffers: 64,
}

// Subscribe 订阅事件，返回接收 channel
func (b *EventBus) Subscribe() chan C2Event {
	ch := make(chan C2Event, b.buffers)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe 取消订阅
func (b *EventBus) Unsubscribe(ch chan C2Event) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Publish 发布事件（非阻塞：订阅者缓冲满则丢弃，不影响主流程）
func (b *EventBus) Publish(ev C2Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// nowUnix 当前时间戳（秒）
func nowUnix() int64 { return time.Now().Unix() }

// 事件去抖：客户端上线/离线事件在短窗口内按 uid 合并，防止连接振荡
// （如 wsc 客户端双连接、快速断线重连）刷屏与状态误报。
// 窗口 3 秒：同一 uid 的同类事件 3s 内只推送一次；不同类事件互不影响。
var (
	eventDedupMu      sync.Mutex
	eventDedupWindow  = 3 * time.Second
	eventDedupLastPub = map[string]time.Time{} // key: "type:uid" → 上次发布时间
)

// PublishEvent 便捷发布（自动填时间戳 + 上线/离线事件按 uid 去抖）
func PublishEvent(evType EventType, uid string, detail map[string]interface{}) {
	if GlobalEventBus == nil {
		return
	}

	// 仅对客户端状态事件去抖（命令输出/安全拦截/登录失败等实时性要求高，不去抖）
	if (evType == EventClientOnline || evType == EventClientOffline || evType == EventClientNew) && uid != "" {
		key := string(evType) + ":" + uid
		eventDedupMu.Lock()
		if last, ok := eventDedupLastPub[key]; ok && time.Since(last) < eventDedupWindow {
			eventDedupMu.Unlock()
			return // 窗口内重复事件，丢弃
		}
		eventDedupLastPub[key] = time.Now()
		eventDedupMu.Unlock()

		// 防 map 无界增长：定期清理过期条目（每次发布时顺带清）
		if len(eventDedupLastPub) > 1024 {
			eventDedupMu.Lock()
			now := time.Now()
			for k, t := range eventDedupLastPub {
				if now.Sub(t) > eventDedupWindow*2 {
					delete(eventDedupLastPub, k)
				}
			}
			eventDedupMu.Unlock()
		}
	}

	GlobalEventBus.Publish(C2Event{
		Type:      evType,
		UID:       uid,
		Timestamp: nowUnix(),
		Detail:    detail,
	})
}
