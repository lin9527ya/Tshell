package connection

import (
	"testing"
	"time"
)

func TestThresholds_FixedRules(t *testing.T) {
	cases := []struct {
		sleep, jitter int
		wantQuick     time.Duration
	}{
		{10, 0, 15 * time.Second},           // expected=10s → quick=15s
		{10, 30, 19500 * time.Millisecond},  // expected=13s → quick=19.5s
		{5, 0, 7500 * time.Millisecond},     // expected=5s → quick=7.5s
		{1, 0, 1500 * time.Millisecond},     // sleep<5 走固定下限，但 quick 仍按公式
		{10, 99, 15 * time.Second},          // jitter 越界（>50）夹到 0
		{10, -5, 15 * time.Second},          // jitter 负值夹到 0
	}
	for _, c := range cases {
		if c.sleep == 1 {
			// sleep<5 时 critical 触发固定 20s 下限，不走 3×quick
			continue
		}
		quick, warning, critical := Thresholds(c.sleep, c.jitter)
		if quick != c.wantQuick {
			t.Errorf("sleep=%d jitter=%d: quick=%v want %v", c.sleep, c.jitter, quick, c.wantQuick)
		}
		if warning != quick*2 {
			t.Errorf("sleep=%d jitter=%d: warning=%v want %v (=2×quick)", c.sleep, c.jitter, warning, quick*2)
		}
		if critical != quick*3 {
			t.Errorf("sleep=%d jitter=%d: critical=%v want %v (=3×quick)", c.sleep, c.jitter, critical, quick*3)
		}
	}
}

func TestThresholds_SmallSleepCriticalFloor(t *testing.T) {
	// sleep<5 时 critical 至少 20s
	for _, sleep := range []int{1, 2, 3, 4} {
		_, _, critical := Thresholds(sleep, 0)
		if critical < 20*time.Second {
			t.Errorf("sleep=%d: critical=%v 低于 20s 下限", sleep, critical)
		}
	}
}

func TestResolveSleepSeconds(t *testing.T) {
	cases := []struct {
		osArray       []string
		listenerSleep int
		profileSleep  int
		globalSleep   int
		want          int
	}{
		{[]string{"a", "b", "c", "linux", "5"}, -1, 10, 5, 5},     // 客户端上报 5
		{[]string{"a", "b", "c", "linux", "600"}, -1, 10, 5, 300}, // 上报超 300 钳制
		{[]string{"a", "b", "c", "linux", "0"}, 30, 10, 5, 30},    // 上报 0 → 监听器显式
		{[]string{"a", "b", "c", "linux"}, 30, 10, 5, 30},         // 旧客户端 → 监听器显式
		{[]string{"a", "b", "c", "linux"}, -1, 10, 5, 10},         // 监听器未配置 → 画像
		{[]string{"a", "b", "c", "linux"}, -1, 0, 7, 7},           // 画像未配置 → 全局
		{[]string{"a", "b", "c"}, -1, 0, 0, 5},                    // 全空 → 5s
	}
	for _, c := range cases {
		if got := ResolveSleepSeconds(c.osArray, c.listenerSleep, c.profileSleep, c.globalSleep); got != c.want {
			t.Errorf("osArray=%v listener=%d profile=%d global=%d: got %d want %d",
				c.osArray, c.listenerSleep, c.profileSleep, c.globalSleep, got, c.want)
		}
	}
}
