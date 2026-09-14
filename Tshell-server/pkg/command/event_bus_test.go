package command

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestEventBusPublishSubscribe 验证事件总线：订阅者收到事件、Unsubscribe 后不再收到
func TestEventBusPublishSubscribe(t *testing.T) {
	bus := &EventBus{subs: make(map[chan C2Event]struct{}), buffers: 64}

	ch := bus.Subscribe()
	var received atomic.Int32
	go func() {
		for range ch {
			received.Add(1)
		}
	}()

	bus.Publish(C2Event{Type: EventClientOnline, UID: "uid123", Timestamp: 123, Detail: map[string]interface{}{"ip": "1.2.3.4"}})
	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("订阅者应收到 1 条事件, 实际 %d", received.Load())
	}

	bus.Unsubscribe(ch)
	bus.Publish(C2Event{Type: EventClientOnline, UID: "uid456", Timestamp: 124})
	time.Sleep(100 * time.Millisecond)
	if received.Load() != 1 {
		t.Fatalf("取消订阅后不应再收到事件, 实际 %d", received.Load())
	}
	t.Log("事件总线订阅/退订验证通过")
}

// TestEventBusNoSubscriber 无订阅者发布不 panic
func TestEventBusNoSubscriber(t *testing.T) {
	PublishEvent(EventClientOnline, "uid", nil) // 不应 panic
	time.Sleep(50 * time.Millisecond)
	t.Log("无订阅者发布验证通过")
}

// TestEventDedup 验证上线/离线事件按 uid 去抖：
// 3s 窗口内同 uid 同类事件只发一次；不同 uid / 不同类事件不受影响。
func TestEventDedup(t *testing.T) {
	bus := &EventBus{subs: make(map[chan C2Event]struct{}), buffers: 64}
	GlobalEventBus = bus
	ch := bus.Subscribe()
	defer bus.Unsubscribe(ch)

	var received atomic.Int32
	go func() {
		for range ch {
			received.Add(1)
		}
	}()

	// 同一 uid 上线两次（模拟连接振荡）→ 只应推 1 次
	PublishEvent(EventClientOnline, "uidA", nil)
	PublishEvent(EventClientOnline, "uidA", nil)
	PublishEvent(EventClientOffline, "uidA", nil)
	PublishEvent(EventClientOffline, "uidA", nil)
	// 不同 uid 上线 → 应推 1 次
	PublishEvent(EventClientOnline, "uidB", nil)
	// 非状态事件（不去抖）
	PublishEvent(EventSecurityBlock, "uidA", nil)

	time.Sleep(200 * time.Millisecond)
	if got := received.Load(); got != 4 {
		t.Fatalf("预期 4 条事件（A上线1+A离线1+B上线1+安全1），实际 %d", got)
	}

	// 清空去抖表后同 uid 再发 → 应再推（验证窗口过期后可再次发布）
	eventDedupMu.Lock()
	eventDedupLastPub = map[string]time.Time{}
	eventDedupMu.Unlock()
	PublishEvent(EventClientOnline, "uidA", nil)
	time.Sleep(200 * time.Millisecond)
	if got := received.Load(); got != 5 {
		t.Fatalf("清空去抖表后再发布应收到, 实际 %d", got)
	}
}
