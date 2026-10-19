package api

import (
	"strings"
	"testing"
	"time"

	"BackendTemplate/pkg/database"
)

// ===== OPSEC P0-5：战果保留期清理 =====

func insertLootAt(t *testing.T, uid, secret, createdAt string) {
	t.Helper()
	if _, err := database.Engine.Insert(&database.Loot{
		Uid: uid, Type: "pwd", Host: "H", Secret: secret, Source: "test",
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("插入战果失败: %v", err)
	}
}

// days<=0 表示不清理：绝不能因为配置缺省就把战果删了
func TestPurgeExpiredLootDisabled(t *testing.T) {
	setupLootTest(t)
	insertLootAt(t, "uid-1", "pw", "2000-01-01 00:00:00")

	for _, days := range []int{0, -1, -100} {
		n, err := PurgeExpiredLoot(days)
		if err != nil {
			t.Fatalf("days=%d 不应报错: %v", days, err)
		}
		if n != 0 {
			t.Errorf("days=%d 不应删除任何记录，实际删了 %d", days, n)
		}
	}
	var total int64
	if total, _ = database.Engine.Count(new(database.Loot)); total != 1 {
		t.Errorf("记录应保留，实际 %d 条", total)
	}
}

func TestPurgeExpiredLootDeletesOnlyOld(t *testing.T) {
	setupLootTest(t)
	now := time.Now()
	insertLootAt(t, "uid-1", "old-pw", now.AddDate(0, 0, -40).Format("2006-01-02 15:04:05"))
	insertLootAt(t, "uid-1", "fresh-pw", now.AddDate(0, 0, -1).Format("2006-01-02 15:04:05"))

	n, err := PurgeExpiredLoot(30)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if n != 1 {
		t.Errorf("应删除 1 条，实际 %d", n)
	}

	rows, err := database.LootStore_().ListByUID("uid-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("应剩 1 条，实际 %d", len(rows))
	}
	if rows[0].Secret != "fresh-pw" {
		t.Errorf("留下的应是新记录，实际 %q", rows[0].Secret)
	}
}

// 边界：恰好 N 天前的记录不删（只删"更早于"截止时刻的）
func TestPurgeExpiredLootBoundary(t *testing.T) {
	setupLootTest(t)
	exact := time.Now().AddDate(0, 0, -30)
	// 略微晚于截止时刻，确保落在"保留"一侧（避免与清理时刻的毫秒差竞争）
	insertLootAt(t, "uid-1", "borderline", exact.Add(time.Minute).Format("2006-01-02 15:04:05"))

	n, err := PurgeExpiredLoot(30)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("边界记录不应被删，实际删了 %d", n)
	}
}

// 清理要留审计（删除不可逆，必须能追溯"删了多少、按什么策略"）
func TestPurgeExpiredLootIsAudited(t *testing.T) {
	setupLootTest(t)
	getCalls := captureLootAudit(t)
	insertLootAt(t, "uid-1", "old", time.Now().AddDate(0, 0, -90).Format("2006-01-02 15:04:05"))

	if _, err := PurgeExpiredLoot(30); err != nil {
		t.Fatal(err)
	}
	var purge auditCall
	for _, c := range getCalls() {
		if c.action == "LOOT_PURGE" {
			purge = c
		}
	}
	if purge.action == "" {
		t.Fatal("清理战果必须留审计")
	}
	if purge.result != "SUCCESS" {
		t.Errorf("result 应为 SUCCESS，实际 %q", purge.result)
	}
	if purge.details["retention_days"] != 30 {
		t.Errorf("审计应记录保留天数，实际 %v", purge.details["retention_days"])
	}
	// 审计不应带上被删的口令本体
	if s := flattenCalls([]auditCall{purge}); strings.Contains(s, "old") {
		t.Errorf("清理审计不应包含战果内容: %s", s)
	}
}

// 未开启时不启动 goroutine、返回可安全调用的空停止函数
func TestStartLootRetentionDisabledReturnsNoop(t *testing.T) {
	stop := StartLootRetention(0)
	if stop == nil {
		t.Fatal("应返回可调用的停止函数（不能是 nil，否则调用方要到处判空）")
	}
	stop() // 不应 panic
}
