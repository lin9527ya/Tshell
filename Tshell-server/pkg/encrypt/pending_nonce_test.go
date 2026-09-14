package encrypt

import (
	"testing"
)

// TestPendingNonceLifecycle 验证轮询协议 nonce 协商状态机：Store -> Take(幂等) -> HasPending
func TestPendingNonceLifecycle(t *testing.T) {
	uid := "pendingtest0000000000000000000001"
	if HasPendingNonce(uid) {
		t.Fatal("初始不应存在 pending")
	}
	StorePendingNonce(uid, &PendingNonce{InitialKey: []byte("k1"), Nonce: []byte("n1")})
	if !HasPendingNonce(uid) {
		t.Fatal("Store 后应存在")
	}
	p := TakePendingNonce(uid)
	if p == nil || string(p.Nonce) != "n1" || string(p.InitialKey) != "k1" {
		t.Fatalf("Take 返回值错误: %+v", p)
	}
	if HasPendingNonce(uid) {
		t.Fatal("Take 后应清除（幂等）")
	}
	if TakePendingNonce(uid) != nil {
		t.Fatal("重复 Take 应返回 nil")
	}
	t.Log("pending nonce 生命周期验证通过")
}
