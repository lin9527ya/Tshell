package command

import (
	"strings"
	"testing"
)

// TestCmdHistoryHook 验证 AddCommand/GetCommand 挂钩记录
func TestCmdHistoryHook(t *testing.T) {
	uid := "hist-test-uid"
	// 入队 2 条
	if !CommandQueues.AddCommand(uid, []byte("shell whoami")) {
		t.Fatal("AddCommand 1 failed")
	}
	if !CommandQueues.AddCommand(uid, []byte("ps")) {
		t.Fatal("AddCommand 2 failed")
	}
	recs, lastPull, _ := GlobalCmdHistory.Snapshot(uid)
	if len(recs) != 2 {
		t.Fatalf("期望 2 条记录, 实际 %d", len(recs))
	}
	if recs[0].Status != "queued" || recs[0].Command != "ps" {
		t.Fatalf("首条应为最近命令 ps/queued: %+v", recs[0])
	}
	// 客户端取走首条
	if _, ok := CommandQueues.GetCommand(uid); !ok {
		t.Fatal("GetCommand failed")
	}
	recs, lastPull, _ = GlobalCmdHistory.Snapshot(uid)
	foundRemoved := false
	for _, r := range recs {
		if r.Status == "removed" {
			foundRemoved = true
		}
	}
	if !foundRemoved {
		t.Fatal("应有 removed 记录")
	}
	if lastPull.IsZero() {
		t.Fatal("lastPull 应已更新")
	}
	if CommandQueues.QueueLen(uid) != 1 {
		t.Fatalf("队列应剩 1 条, 实际 %d", CommandQueues.QueueLen(uid))
	}
	_ = strings.TrimSpace("")
}
