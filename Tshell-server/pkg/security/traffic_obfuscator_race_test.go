package security

import (
	"sync"
	"testing"
)

// TestObfuscatorConcurrentRace 验证 A1 修复的 ObfuscateRequestWithClientIP 在并发场景下
// 不会出现 RWMutex 死锁或 data race。同时并发调用 ObfuscateRequestWithClientIP（读路径）
// 和 SwitchProtocol（写路径），用 go test -race 检测数据竞争。
func TestObfuscatorConcurrentRace(t *testing.T) {
	if err := InitTrafficObfuscator(); err != nil {
		t.Fatalf("InitTrafficObfuscator: %v", err)
	}
	if GlobalTrafficObfuscator == nil {
		t.Fatal("GlobalTrafficObfuscator is nil after init")
	}

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*iterations)

	// 并发读：ObfuscateRequestWithClientIP（含 A1 修复的锁逻辑）
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := GlobalTrafficObfuscator.ObfuscateRequest(
					[]byte("test-payload"), "chrome",
				)
				if err != nil {
					errCh <- err
				}
			}
		}(i)
	}

	// 并发写：SwitchProtocol（写锁路径，与读路径交叉触发 A1 修复前的死锁场景）
	profiles := []string{"chrome", "mobile", "qq", "douyin", "weibo", "baidu"}
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				p := profiles[(idx+j)%len(profiles)]
				// 忽略错误（目标是不 panic / 不死锁）
				GlobalTrafficObfuscator.SwitchProtocol(p, "race-test")
			}
		}(i)
	}

	// 并发读当前 profile
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				GlobalTrafficObfuscator.GetCurrentProfile()
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("ObfuscateRequest returned error: %v", err)
	}
}

// TestObfuscatorProfileSwitch 验证 SwitchProtocol 不存在的 profile 返回错误，
// 存在的 profile 切换成功且 GetCurrentProfile 一致。
func TestObfuscatorProfileSwitch(t *testing.T) {
	if GlobalTrafficObfuscator == nil {
		if err := InitTrafficObfuscator(); err != nil {
			t.Fatalf("InitTrafficObfuscator: %v", err)
		}
	}
	// 存在的 profile
	if err := GlobalTrafficObfuscator.SwitchProtocol("mobile", "test"); err != nil {
		t.Errorf("SwitchProtocol(mobile) unexpected error: %v", err)
	}
	if got := GlobalTrafficObfuscator.GetCurrentProfile(); got != "mobile" {
		t.Errorf("GetCurrentProfile() = %q, want mobile", got)
	}
	// 不存在的 profile
	if err := GlobalTrafficObfuscator.SwitchProtocol("nonexistent_profile_xyz", "test"); err == nil {
		t.Error("SwitchProtocol(nonexistent) expected error, got nil")
	}
}
