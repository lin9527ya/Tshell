package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestQueryLoginFailures IOC 报告的"登录失败"字段原来恒为空（初始化后再没填充过）。
// 这里验证从落盘审计日志按 分类=认证 + 结果=失败 + 时间窗 过滤。
func TestQueryLoginFailures(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	logs := []AuditLog{
		{ID: "1", Timestamp: now.Add(-time.Hour), Level: LogLevelSecurity, Category: CategoryAuth,
			Action: "LOGIN", Username: "alice", ClientIP: "1.1.1.1", Result: "FAILURE"},
		{ID: "2", Timestamp: now.Add(-2 * time.Hour), Level: LogLevelInfo, Category: CategoryAuth,
			Action: "LOGIN", Username: "bob", ClientIP: "2.2.2.2", Result: "SUCCESS"},
		{ID: "3", Timestamp: now.Add(-3 * time.Hour), Level: LogLevelError, Category: CategoryFile,
			Action: "UPLOAD", Username: "carol", ClientIP: "3.3.3.3", Result: "FAILURE"},
		{ID: "4", Timestamp: now.Add(-48 * time.Hour), Level: LogLevelSecurity, Category: CategoryAuth,
			Action: "LOGIN", Username: "old", ClientIP: "4.4.4.4", Result: "FAILURE"},
	}
	for _, l := range logs {
		if err := enc.Encode(l); err != nil {
			t.Fatal(err)
		}
	}
	logFile := filepath.Join(dir, "audit_"+now.Format("2006-01-02")+".log")
	if err := os.WriteFile(logFile, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InitAuditLogger(dir, 100<<20, 30); err != nil {
		t.Fatalf("初始化审计日志失败: %v", err)
	}
	defer func() { GlobalAuditLogger = nil }()

	got := QueryLoginFailures(now.Add(-24*time.Hour), 10)
	if len(got) != 1 {
		t.Fatalf("应只返回时间窗内的认证失败记录，实际 %d 条: %v", len(got), got)
	}
	if !strings.Contains(got[0], "alice@1.1.1.1") {
		t.Fatalf("记录内容不符: %q", got[0])
	}
	if !strings.Contains(got[0], now.Format("2006-01-02")) {
		t.Fatalf("记录缺少时间: %q", got[0])
	}

	// limit 生效
	if got := QueryLoginFailures(now.Add(-72*time.Hour), 1); len(got) != 1 {
		t.Fatalf("limit 未生效: %d", len(got))
	}
}
