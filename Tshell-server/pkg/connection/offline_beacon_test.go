package connection

import (
	"testing"
	"time"
)

// TestRegisterUsesListenerJitter 监听器级 jitter 必须进入离线阈值；
// -1 = 未配置时才回落到全局 client.jitter（测试环境无配置 → 0）。
func TestRegisterUsesListenerJitter(t *testing.T) {
	d := &OfflineDetector{}

	d.Register("u30", "tcp", 10, 30)
	if st, ok := d.GetClientState("u30"); !ok || st.JitterPercent != 30 {
		t.Fatalf("监听器 jitter=30 未生效: %+v ok=%v", st, ok)
	}

	d.Register("uzero", "tcp", 10, 0)
	if st, _ := d.GetClientState("uzero"); st.JitterPercent != 0 {
		t.Fatalf("显式 0%% jitter 必须保留，实际 %d", st.JitterPercent)
	}

	d.Register("uunset", "tcp", 10, -1)
	if st, _ := d.GetClientState("uunset"); st.JitterPercent != beaconJitterPercent() {
		t.Fatalf("未配置时应回落全局 client.jitter=%d，实际 %d", beaconJitterPercent(), st.JitterPercent)
	}
}

// TestThresholdsPerClientJitter 两个客户端同 sleep、不同 jitter：
// 50 秒无活动时，jitter=0 的（critical 45s）判离线，jitter=40 的（63s）仍在线。
func TestThresholdsPerClientJitter(t *testing.T) {
	d := &OfflineDetector{}
	d.Register("tight", "tcp", 10, 0)
	d.Register("loose", "tcp", 10, 40)

	old := time.Now().Add(-50 * time.Second)
	testActivity(t, d, "tight").setLastActive(old)
	testActivity(t, d, "loose").setLastActive(old)

	d.checkAll()

	if _, ok := d.GetClientState("tight"); ok {
		t.Fatal("jitter=0（critical 45s）的客户端应被判离线")
	}
	if _, ok := d.GetClientState("loose"); !ok {
		t.Fatal("jitter=40（critical 63s）的客户端不应被判离线")
	}
}

// TestThresholdsFormula 守住"最坏拍 × 4.5"的既有语义，防止阈值公式被顺手改掉。
func TestThresholdsFormula(t *testing.T) {
	_, _, critical := Thresholds(10, 0)
	if critical != 45*time.Second {
		t.Fatalf("sleep=10 jitter=0 critical=%v，期望 45s", critical)
	}
	_, _, critical = Thresholds(10, 40)
	if critical != 63*time.Second {
		t.Fatalf("sleep=10 jitter=40 critical=%v，期望 63s", critical)
	}
	// 小 sleep 的 20s 下限
	_, _, critical = Thresholds(2, 0)
	if critical != 20*time.Second {
		t.Fatalf("sleep=2 critical=%v，期望 20s 下限", critical)
	}
}

// TestNextCheckDelayTracksEarliestDeadline 调度器按"最早截止时间"计算睡眠时长。
func TestNextCheckDelayTracksEarliestDeadline(t *testing.T) {
	d := &OfflineDetector{wakeCh: make(chan struct{}, 1)}
	d.Register("a", "tcp", 10, 0) // critical 45s
	d.Register("b", "tcp", 30, 0) // critical 135s

	// b 还有 ~2s 到期，a 还有 ~0.2s 到期 → 应取 a
	testActivity(t, d, "a").setLastActive(time.Now().Add(-45*time.Second + 200*time.Millisecond))
	testActivity(t, d, "b").setLastActive(time.Now().Add(-135*time.Second + 2*time.Second))

	delay, ok := d.nextCheckDelay()
	if !ok {
		t.Fatal("有客户端时应返回 ok=true")
	}
	if delay < 50*time.Millisecond || delay > 500*time.Millisecond {
		t.Fatalf("应按最早截止时间 ~200ms 计时，实际 %v", delay)
	}
}

// TestOfflineDetectorRealtimeDeadline 阈值一到就判离线，不再等固定 5s ticker。
func TestOfflineDetectorRealtimeDeadline(t *testing.T) {
	d := &OfflineDetector{
		stopCh: make(chan struct{}),
		wakeCh: make(chan struct{}, 1),
	}
	go d.run()
	defer d.Stop()

	d.Register("realtime", "tcp", 10, 0) // critical = 45s
	expireAt := time.Now().Add(120 * time.Millisecond)
	testActivity(t, d, "realtime").setLastActive(expireAt.Add(-45 * time.Second))
	d.wake() // 截止时间被前移（测试模拟），按新值重新计时

	waitUntil := expireAt.Add(2 * time.Second)
	for time.Now().Before(waitUntil) {
		if !d.IsRegistered("realtime") {
			if late := time.Since(expireAt); late > time.Second {
				t.Fatalf("判离线晚了 %v（应 ≤1s，固定 5s ticker 会明显超时）", late)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("客户端在截止时间过后 2s 仍未判离线（实时调度未生效）")
}
