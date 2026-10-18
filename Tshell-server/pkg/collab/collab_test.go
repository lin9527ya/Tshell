package collab

import (
	"testing"
	"time"
)

func newTestRegistry(t *testing.T) (*Registry, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	r := NewRegistry()
	r.now = func() time.Time { return now }
	return r, &now
}

// TestPresence 在线状态：Touch 续期、超 TTL 视为离线。
func TestPresence(t *testing.T) {
	r, now := newTestRegistry(t)
	r.Touch("alice", "admin", "1.1.1.1")
	r.Touch("bob", "auditor", "2.2.2.2")
	if got := r.Online(); len(got) != 2 || got[0].Username != "alice" || got[1].Username != "bob" {
		t.Fatalf("在线列表不符: %+v", got)
	}

	// bob 超时离线，alice 续期后仍在
	*now = now.Add(PresenceTTL + time.Second)
	r.Touch("alice", "admin", "1.1.1.1")
	if got := r.Online(); len(got) != 1 || got[0].Username != "alice" {
		t.Fatalf("TTL 清理不符: %+v", got)
	}
	if got := r.Online(); len(got) != 1 {
		t.Fatalf("再次查询不应复活过期条目: %+v", got)
	}
}

// TestSessionLock 抢锁/续期/冲突/过期/释放权限。
func TestSessionLock(t *testing.T) {
	r, now := newTestRegistry(t)

	l, ok, _ := r.Acquire("uid-1", "alice", "排查")
	if !ok || l.Owner != "alice" || l.ExpiresAt.IsZero() {
		t.Fatalf("首次抢锁应成功: %+v", l)
	}
	// 同持有人 → 续期成功
	if _, ok, _ := r.Acquire("uid-1", "alice", "排查"); !ok {
		t.Fatal("同持有人应可续期")
	}
	// 他人 → 冲突并返回持有人
	if _, ok, holder := r.Acquire("uid-1", "bob", ""); ok || holder != "alice" {
		t.Fatalf("他人抢锁应冲突: ok=%v holder=%q", ok, holder)
	}
	if h := r.Holder("uid-1"); h != "alice" {
		t.Fatalf("Holder 不符: %q", h)
	}
	// 非持有人不能释放
	if r.Release("uid-1", "bob") {
		t.Fatal("非持有人不应能释放")
	}
	if !r.Release("uid-1", "alice") || r.Holder("uid-1") != "" {
		t.Fatal("持有人释放失败")
	}

	// 过期自动失效
	r.Acquire("uid-2", "alice", "")
	*now = now.Add(DefaultLockTTL + time.Second)
	if h := r.Holder("uid-2"); h != "" {
		t.Fatalf("过期锁应失效，实际持有人 %q", h)
	}
	if _, ok, _ := r.Acquire("uid-2", "bob", ""); !ok {
		t.Fatal("过期后他人应可抢锁")
	}
	if got := r.Locks(); len(got) != 1 || got[0].Owner != "bob" {
		t.Fatalf("锁列表不符: %+v", got)
	}
}
