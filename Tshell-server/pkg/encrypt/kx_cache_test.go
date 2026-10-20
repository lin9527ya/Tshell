package encrypt

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"
)

// ===== 握手期临时主密钥缓存 =====
//
// 这些用例守的是三件事：TTL 真的生效（未认证请求不能无限期占内存）、
// 容量上限真的淘汰、以及过期/淘汰时**密钥材料被清零**而不是留给 GC。

func fakeClock(start time.Time) (func() time.Time, func(time.Duration)) {
	cur := start
	var mu sync.Mutex
	get := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return cur
	}
	adv := func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		cur = cur.Add(d)
	}
	return get, adv
}

func pub(n byte) []byte { return bytes.Repeat([]byte{n}, ECDHPublicKeySize) }
func mk(n byte) []byte  { return bytes.Repeat([]byte{n}, ECDHSharedKeySize) }

func TestKxCachePutGet(t *testing.T) {
	c := NewKxCache(time.Minute, 8)
	defer c.Close()

	key := mk(0x11)
	if _, err := c.Put(pub(1), key); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}
	got, ok := c.Get(pub(1))
	if !ok {
		t.Fatal("应能取到刚存入的条目")
	}
	if !bytes.Equal(got, key) {
		t.Error("取回的密钥与存入的不一致")
	}
	// Get 不应删除条目（同一会话后续请求还要用）
	if _, ok := c.Get(pub(1)); !ok {
		t.Error("Get 不应消耗条目")
	}
	if c.Len() != 1 {
		t.Errorf("条目数应为 1，实际 %d", c.Len())
	}
}

// TTL 生效：过期后取不到，且条目被移除
func TestKxCacheTTLExpiry(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk, adv := fakeClock(start)
	c := NewKxCache(30*time.Second, 8)
	c.SetClock(clk)
	defer c.Close()

	if _, err := c.Put(pub(1), mk(0x22)); err != nil {
		t.Fatal(err)
	}

	adv(29 * time.Second)
	if _, ok := c.Get(pub(1)); !ok {
		t.Error("未到期时不应失效")
	}

	// 命中会续期：再走 29s（总计 58s，早于"刷新后"的 59s）仍应有效。
	// 这里断言的是**滑动过期**语义 —— 否则活跃会话会在固定 TTL 到期后被回收，
	// 服务端下次请求重建密钥对，派生出与客户端不同的主密钥（见 Get 的注释）。
	adv(29 * time.Second)
	if _, ok := c.Get(pub(1)); !ok {
		t.Error("命中续期后，活跃会话不应失效")
	}

	// 但 TTL 仍必须回收**空闲**条目：此后不再命中，超过 TTL 即失效
	adv(40 * time.Second)
	if _, ok := c.Get(pub(1)); ok {
		t.Error("空闲超过 TTL 后不应再取到")
	}
	if c.Len() != 0 {
		t.Errorf("过期条目应被移除，实际剩 %d", c.Len())
	}
}

// Sweep 主动清理过期条目
func TestKxCacheSweep(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk, adv := fakeClock(start)
	c := NewKxCache(10*time.Second, 8)
	c.SetClock(clk)
	defer c.Close()

	for i := byte(1); i <= 3; i++ {
		if _, err := c.Put(pub(i), mk(i)); err != nil {
			t.Fatal(err)
		}
	}
	adv(11 * time.Second)
	if n := c.Sweep(); n != 3 {
		t.Errorf("应清理 3 个过期条目，实际 %d", n)
	}
	if c.Len() != 0 {
		t.Errorf("清理后应为空，实际 %d", c.Len())
	}
}

// 容量上限：超过后淘汰最旧的（未认证请求不能无限占内存）
func TestKxCacheEvictsWhenFull(t *testing.T) {
	c := NewKxCache(time.Minute, 3)
	defer c.Close()

	for i := byte(1); i <= 3; i++ {
		if _, err := c.Put(pub(i), mk(i)); err != nil {
			t.Fatal(err)
		}
	}
	if c.Len() != 3 {
		t.Fatalf("应为 3 条，实际 %d", c.Len())
	}

	// 第 4 条应淘汰最旧的 pub(1)
	if _, err := c.Put(pub(4), mk(4)); err != nil {
		t.Fatal(err)
	}
	if c.Len() != 3 {
		t.Errorf("容量应为 3，实际 %d", c.Len())
	}
	if _, ok := c.Get(pub(1)); ok {
		t.Error("最旧的条目应被淘汰")
	}
	if _, ok := c.Get(pub(4)); !ok {
		t.Error("新条目应能取到")
	}
}

// 同一公钥重复握手应幂等覆盖，不产生多个条目
func TestKxCachePutIsIdempotentPerKey(t *testing.T) {
	c := NewKxCache(time.Minute, 8)
	defer c.Close()

	if _, err := c.Put(pub(1), mk(0xAA)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put(pub(1), mk(0xBB)); err != nil {
		t.Fatal(err)
	}
	if c.Len() != 1 {
		t.Errorf("同一公钥应只有 1 条，实际 %d", c.Len())
	}
	got, _ := c.Get(pub(1))
	if !bytes.Equal(got, mk(0xBB)) {
		t.Error("重复 Put 应以新值为准")
	}
}

// 绑定 uid 后可反查
func TestKxCacheBindAndLookupByUID(t *testing.T) {
	c := NewKxCache(time.Minute, 8)
	defer c.Close()

	if _, err := c.Put(pub(7), mk(0x77)); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.LookupByUID("uid-1"); ok {
		t.Error("未绑定时不应能按 uid 查到")
	}
	if !c.Bind(pub(7), "uid-1") {
		t.Fatal("绑定应成功")
	}
	got, ok := c.LookupByUID("uid-1")
	if !ok || !bytes.Equal(got, mk(0x77)) {
		t.Error("绑定后应能按 uid 查到同一把密钥")
	}
	// 绑定不存在的公钥应失败
	if c.Bind(pub(9), "uid-2") {
		t.Error("对未知条目绑定应返回 false")
	}
}

// 过期条目不能再按 uid 查到（TTL 对所有查询一致）
func TestKxCacheLookupByUIDRespectsTTL(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clk, adv := fakeClock(start)
	c := NewKxCache(5*time.Second, 8)
	c.SetClock(clk)
	defer c.Close()

	if _, err := c.Put(pub(1), mk(1)); err != nil {
		t.Fatal(err)
	}
	c.Bind(pub(1), "uid-1")
	adv(6 * time.Second)
	if _, ok := c.LookupByUID("uid-1"); ok {
		t.Error("过期条目不应能按 uid 查到")
	}
}

// 参数不合法必须拒绝（短密钥不能被缓存起来当主密钥用）
func TestKxCacheRejectsInvalidInput(t *testing.T) {
	c := NewKxCache(time.Minute, 8)
	defer c.Close()

	if _, err := c.Put(nil, mk(1)); err == nil {
		t.Error("空公钥应被拒绝")
	}
	if _, err := c.Put(pub(1), make([]byte, 16)); err == nil {
		t.Error("过短的主密钥应被拒绝")
	}
	// nil 缓存不能 panic
	var nilCache *KxCache
	if _, err := nilCache.Put(pub(1), mk(1)); err == nil {
		t.Error("nil 缓存应返回错误而不是 panic")
	}
	if _, ok := nilCache.Get(pub(1)); ok {
		t.Error("nil 缓存的 Get 应返回 false")
	}
	if nilCache.Len() != 0 {
		t.Error("nil 缓存的 Len 应为 0")
	}
}

// 返回值是副本：调用方改它不能污染缓存
func TestKxCacheGetReturnsCopy(t *testing.T) {
	c := NewKxCache(time.Minute, 8)
	defer c.Close()
	if _, err := c.Put(pub(1), mk(0x33)); err != nil {
		t.Fatal(err)
	}
	got, _ := c.Get(pub(1))
	got[0] = 0xFF
	again, _ := c.Get(pub(1))
	if again[0] == 0xFF {
		t.Error("Get 应返回副本，否则调用方会污染缓存里的密钥")
	}
}

// 并发读写不应竞态（-race 下验证）
func TestKxCacheConcurrentAccess(t *testing.T) {
	c := NewKxCache(time.Minute, 64)
	defer c.Close()

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			p := pub(byte(n))
			for j := 0; j < 50; j++ {
				_, _ = c.Put(p, mk(byte(n)))
				_, _ = c.Get(p)
				c.Bind(p, "uid")
				c.LookupByUID("uid")
				c.Len()
			}
		}(i)
	}
	wg.Wait()
	if c.Len() > 64 {
		t.Errorf("并发下不应超过容量上限，实际 %d", c.Len())
	}
}

// Close 清空全部条目
func TestKxCacheClose(t *testing.T) {
	c := NewKxCache(time.Minute, 8)
	if _, err := c.Put(pub(1), mk(1)); err != nil {
		t.Fatal(err)
	}
	c.Close()
	if c.Len() != 0 {
		t.Errorf("Close 后应为空，实际 %d", c.Len())
	}
}

func TestKxCacheDefaults(t *testing.T) {
	c := NewKxCache(0, 0)
	defer c.Close()
	if c.ttl != 30*time.Second {
		t.Errorf("默认 TTL 应为 30s，实际 %v", c.ttl)
	}
	if c.max != 4096 {
		t.Errorf("默认容量应为 4096，实际 %d", c.max)
	}
}

// 确保 ErrKxCacheFull 是可判定的错误（而不是裸字符串）
func TestKxCacheFullErrorIsComparable(t *testing.T) {
	if !errors.Is(ErrKxCacheFull, ErrKxCacheFull) {
		t.Error("ErrKxCacheFull 应可被 errors.Is 判定")
	}
}
