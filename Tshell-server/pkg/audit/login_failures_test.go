package audit

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"BackendTemplate/pkg/database"
)

// TestQueryLoginFailures IOC 报告的"登录失败"字段原来恒为空（初始化后再没填充过）。
// 审计落库后该查询走库：按 分类=认证 + 结果=失败 + 时间窗 过滤。
func TestQueryLoginFailures(t *testing.T) {
	tmp := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmp, "audit.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	now := time.Now()
	al, err := NewAuditLogger(t.TempDir(), 1<<20, 30)
	if err != nil {
		t.Fatalf("初始化 logger 失败: %v", err)
	}
	defer al.Close()
	al.persistBatch([]AuditLog{
		{ID: "1", Timestamp: now.Add(-time.Hour), Level: LogLevelSecurity, Category: CategoryAuth,
			Action: "LOGIN", Username: "alice", ClientIP: "1.1.1.1", Result: "FAILURE"},
		{ID: "2", Timestamp: now.Add(-2 * time.Hour), Level: LogLevelInfo, Category: CategoryAuth,
			Action: "LOGIN", Username: "bob", ClientIP: "2.2.2.2", Result: "SUCCESS"},
		{ID: "3", Timestamp: now.Add(-3 * time.Hour), Level: LogLevelError, Category: CategoryFile,
			Action: "UPLOAD", Username: "carol", ClientIP: "3.3.3.3", Result: "FAILURE"},
		{ID: "4", Timestamp: now.Add(-48 * time.Hour), Level: LogLevelSecurity, Category: CategoryAuth,
			Action: "LOGIN", Username: "old", ClientIP: "4.4.4.4", Result: "FAILURE"},
	})

	got := QueryLoginFailures(now.Add(-24*time.Hour), 10)
	if len(got) != 1 {
		t.Fatalf("应只返回时间窗内的认证失败记录，实际 %d 条: %v", len(got), got)
	}
	if !strings.Contains(got[0], "alice@1.1.1.1") {
		t.Fatalf("记录内容不符: %q", got[0])
	}
	if got := QueryLoginFailures(now.Add(-72*time.Hour), 1); len(got) != 1 {
		t.Fatalf("limit 未生效: %d", len(got))
	}
}
