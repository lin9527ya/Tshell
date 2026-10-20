package encrypt

import (
	"testing"
	"time"
)

// ===== 滑动过期（回归守卫）=====
//
// 这是一个真实上线事故的守卫：HTTP 家族每个请求都独立解析密钥，会话存活期间
// 会持续携带 X-Kx 头。若缓存按**固定** TTL 过期，服务端会在下次请求时
// 重新生成临时密钥对，派生出一把与客户端**不同**的主密钥（客户端那把是用
// 旧临时公钥派生的）→ MetaInfo 解不开 → 表现是"跑了一段时间后命令全失效，
// 重启客户端又好"，极易误判成网络问题。
//
// 所以：命中必须续期。下面两条用例分别钉住 Get 与 GetExchange。

func newTestCache(t *testing.T) (*KxCache, *time.Time) {
	t.Helper()
	now := time.Unix(1700000000, 0)
	c := NewKxCache(30*time.Second, 16)
	c.SetClock(func() time.Time { return now })
	return c, &now
}

func TestKxCacheGetSlidesExpiry(t *testing.T) {
	c, now := newTestCache(t)
	mk := make([]byte, 32)
	for i := range mk {
		mk[i] = byte(i + 1)
	}
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i + 100)
	}
	if _, err := c.Put(pub, mk); err != nil {
		t.Fatal(err)
	}

	// 每隔 20s 命中一次（小于 30s TTL），持续到远超原始 TTL 的时长
	for i := 0; i < 10; i++ {
		*now = now.Add(20 * time.Second)
		if _, ok := c.Get(pub); !ok {
			t.Fatalf("第 %d 次命中失败（总计 %v）：活跃会话被过期了，"+
				"服务端会重建密钥对并派生出与客户端不同的主密钥", i+1, time.Duration(i+1)*20*time.Second)
		}
	}
	if c.Len() != 1 {
		t.Errorf("条目应仍在缓存中，实际 %d 条", c.Len())
	}
}

func TestKxCacheGetExchangeSlidesExpiry(t *testing.T) {
	c, now := newTestCache(t)
	mk := make([]byte, 32)
	for i := range mk {
		mk[i] = byte(i + 7)
	}
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i + 50)
	}
	srvPub := make([]byte, 32)
	for i := range srvPub {
		srvPub[i] = byte(i + 9)
	}
	if _, err := c.PutExchange(pub, srvPub, mk); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		*now = now.Add(20 * time.Second)
		gotMK, gotSrv, ok := c.GetExchange(pub)
		if !ok {
			t.Fatalf("第 %d 次 GetExchange 命中失败：活跃会话被过期了", i+1)
		}
		if len(gotMK) != 32 || len(gotSrv) != 32 {
			t.Fatal("返回的密钥/公钥长度不对")
		}
	}
}

// 反过来也要守住：**不命中**的条目仍然必须按 TTL 过期（不能因为加了续期就不回收）
func TestKxCacheStillExpiresWhenIdle(t *testing.T) {
	c, now := newTestCache(t)
	mk := make([]byte, 32)
	pub := make([]byte, 32)
	pub[0] = 0xAB
	if _, err := c.Put(pub, mk); err != nil {
		t.Fatal(err)
	}

	*now = now.Add(31 * time.Second) // 超过 TTL 且期间无任何命中
	if _, ok := c.Get(pub); ok {
		t.Fatal("空闲超过 TTL 的条目必须过期回收，否则缓存会无限增长")
	}
	if c.Len() != 0 {
		t.Errorf("过期条目应从缓存移除，实际还有 %d 条", c.Len())
	}
}
