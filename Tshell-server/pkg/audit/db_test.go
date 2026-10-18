package audit

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"BackendTemplate/pkg/database"
)

func setupAuditDB(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmp, "audit.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() { database.Engine.Close() })
}

// TestAuditDBPersistAndQuery 审计落库后：过滤语义与旧文件版一致、分页、LogID 幂等、保留清理。
func TestAuditDBPersistAndQuery(t *testing.T) {
	setupAuditDB(t)
	al, err := NewAuditLogger(t.TempDir(), 1<<20, 30)
	if err != nil {
		t.Fatalf("初始化 logger 失败: %v", err)
	}
	defer al.Close()

	now := time.Now()
	batch := []AuditLog{
		{ID: "a1", Timestamp: now.Add(-3 * time.Minute), Level: LogLevelInfo, Category: CategoryAPI,
			Action: "LIST", Username: "alice", ClientIP: "1.1.1.1", Result: "SUCCESS", Message: "查询列表"},
		{ID: "a2", Timestamp: now.Add(-2 * time.Minute), Level: LogLevelSecurity, Category: CategoryAuth,
			Action: "LOGIN", Username: "bob", ClientIP: "2.2.2.2", Result: "FAILURE",
			Message: "用户 bob 登录失败", Details: map[string]interface{}{"reason": "bad password"}},
		{ID: "a3", Timestamp: now.Add(-time.Minute), Level: LogLevelInfo, Category: CategoryFile,
			Action: "UPLOAD", Username: "carol", ClientIP: "3.3.3.3", Result: "SUCCESS", Message: "插件上传",
			Duration: 1500 * time.Millisecond},
	}
	al.persistBatch(batch)

	// 全量：时间倒序
	logs, total, err := queryLogsFromDB(LogQueryParams{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(logs) != 3 {
		t.Fatalf("全量查询不符: total=%d len=%d", total, len(logs))
	}
	if logs[0].ID != "a3" || logs[2].ID != "a1" {
		t.Fatalf("排序应为时间倒序: %v", []string{logs[0].ID, logs[1].ID, logs[2].ID})
	}
	if logs[2].Duration != 1500*time.Millisecond {
		// a3 是第一条（倒序），这里校验 Duration 往返用 a3
	}
	if logs[0].Duration != 1500*time.Millisecond {
		t.Fatalf("Duration 往返不符: %v", logs[0].Duration)
	}
	if logs[1].Details["reason"] != "bad password" {
		t.Fatalf("Details 往返不符: %+v", logs[1].Details)
	}

	// 分类 + 结果精确匹配
	logs, total, _ = queryLogsFromDB(LogQueryParams{Category: CategoryAuth, Result: "FAILURE", Page: 1, PageSize: 10})
	if total != 1 || logs[0].Username != "bob" {
		t.Fatalf("分类/结果过滤不符: total=%d logs=%+v", total, logs)
	}

	// 关键词包含匹配（message/action/username/client_ip）
	for _, kw := range []string{"bob", "上传", "UPLOAD", "3.3.3.3"} {
		if _, n, _ := queryLogsFromDB(LogQueryParams{Keyword: kw, Page: 1, PageSize: 10}); n != 1 {
			t.Fatalf("关键词 %q 应命中 1 条，实际 %d", kw, n)
		}
	}

	// 分页
	logs, total, _ = queryLogsFromDB(LogQueryParams{Page: 2, PageSize: 2})
	if total != 3 || len(logs) != 1 {
		t.Fatalf("分页不符: total=%d len=%d", total, len(logs))
	}

	// LogID 唯一：重复写入不产生重复行
	al.persistBatch(batch)
	if _, total, _ = queryLogsFromDB(LogQueryParams{Page: 1, PageSize: 10}); total != 3 {
		t.Fatalf("重复写入应幂等，实际 total=%d", total)
	}

	// IOC 报告的登录失败（走库）
	fails := QueryLoginFailures(now.Add(-time.Hour), 10)
	if len(fails) != 1 || !strings.Contains(fails[0], "bob@2.2.2.2") {
		t.Fatalf("QueryLoginFailures 不符: %v", fails)
	}

	// 保留清理：删掉早于 cutoff 的记录
	n, err := cleanupDBBefore(now.Add(-150 * time.Second))
	if err != nil || n != 1 {
		t.Fatalf("保留清理不符: n=%d err=%v", n, err)
	}
	if _, total, _ = queryLogsFromDB(LogQueryParams{Page: 1, PageSize: 10}); total != 2 {
		t.Fatalf("清理后应剩 2 条，实际 %d", total)
	}
}
