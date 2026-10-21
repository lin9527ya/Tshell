package connection

import (
	"sync"
	"testing"
	"time"
)

// TestOfflineDetectorConcurrentAccess Touch（每个消息都调）与注册/注销/扫描并发时
// 不能 panic、不能丢注册；sync.Map + 原子量替代全局锁后必须保持这些语义。
func TestOfflineDetectorConcurrentAccess(t *testing.T) {
	d := &OfflineDetector{}

	const clients = 64
	for i := 0; i < clients; i++ {
		d.Register(uidAt(i), "tcp", 10, 20)
	}

	var wg sync.WaitGroup

	// 多协程高频 Touch（模拟真实消息路径）
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 2000; i++ {
				d.Touch(uidAt(i % clients))
				d.GetClientState(uidAt(i % clients))
			}
		}()
	}

	// 并发注册/注销另一批客户端
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			uid := "churn-" + uidAt(i)
			d.Register(uid, "tcp", 5, 0)
			d.Touch(uid)
			d.Unregister(uid)
		}
	}()

	// 并发扫描（不会把活跃客户端误删：Touch 与扫描的竞态由删除后复查兜住）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			d.checkAll()
		}
	}()

	wg.Wait()

	for i := 0; i < clients; i++ {
		if !d.IsRegistered(uidAt(i)) {
			t.Fatalf("客户端 %s 在并发访问中丢失", uidAt(i))
		}
	}
}

// BenchmarkOfflineTouch 热路径基准：sync.Map + 原子写，无全局互斥锁。
func BenchmarkOfflineTouch(b *testing.B) {
	d := &OfflineDetector{}
	d.Register("bench", "tcp", 10, 20)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			d.Touch("bench")
		}
	})
}

func uidAt(i int) string {
	return "uid-" + string(rune('a'+i%26)) + "-" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [12]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// legacyTouchDetector 仅用于基准对照：改造前"全局互斥锁 + map"的 Touch 实现。
type legacyTouchDetector struct {
	mu      sync.RWMutex
	clients map[string]*legacyTouchActivity
}

type legacyTouchActivity struct{ lastActive time.Time }

func (d *legacyTouchDetector) touch(uid string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c, ok := d.clients[uid]; ok {
		c.lastActive = time.Now()
	}
}

// BenchmarkOfflineTouchLegacyMutex 改造前实现：所有客户端共享一把写锁。
func BenchmarkOfflineTouchLegacyMutex(b *testing.B) {
	d := &legacyTouchDetector{clients: map[string]*legacyTouchActivity{"bench": {}}}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			d.touch("bench")
		}
	})
}
