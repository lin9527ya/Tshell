package security

import (
	"testing"
	"time"
)

func TestRateLimiterUpdateConfigFullAndEnabled(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	rl.UpdateConfigFull(7, 70, 2*time.Minute, 30*time.Second)
	login, api, block := rl.GetConfigSnapshot()
	if login != 7 || api != 70 || block != 2*time.Minute {
		t.Fatalf("限流配置未更新: login=%d api=%d block=%v", login, api, block)
	}
	// 0 值不应覆盖已有配置
	rl.UpdateConfigFull(0, 0, 0, 0)
	login, api, block = rl.GetConfigSnapshot()
	if login != 7 || api != 70 || block != 2*time.Minute {
		t.Fatalf("0 值不应覆盖配置: login=%d api=%d block=%v", login, api, block)
	}

	if !rl.IsEnabled() {
		t.Fatal("新建 RateLimiter 默认应启用")
	}
	rl.SetEnabled(false)
	if rl.IsEnabled() {
		t.Fatal("SetEnabled(false) 后仍为启用")
	}
	rl.SetEnabled(true)
	if !rl.IsEnabled() {
		t.Fatal("SetEnabled(true) 后仍为禁用")
	}
}
