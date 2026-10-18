package database

import (
	"path/filepath"
	"strings"
	"testing"
)

// 方言层的验收点：占位符改写正确（含字符串字面量里的问号）、列/表判断跨方言、
// 自增序列重置在 SQLite 上语义不变。

func TestRebind(t *testing.T) {
	old := DBType
	t.Cleanup(func() { DBType = old })

	DBType = "sqlite3"
	in := "SELECT * FROM clients WHERE uid = ? AND online = ?"
	if got := Rebind(in); got != in {
		t.Fatalf("SQLite 应原样返回: %q", got)
	}

	DBType = "postgres"
	got := Rebind("UPDATE downloads SET file_size = ?, downloaded_size = ? WHERE uid = ?")
	want := "UPDATE downloads SET file_size = $1, downloaded_size = $2 WHERE uid = $3"
	if got != want {
		t.Fatalf("PostgreSQL 占位符改写不符:\n got %q\nwant %q", got, want)
	}
	// 字符串字面量里的问号不是占位符
	got = Rebind("SELECT '?' AS q, ? AS v, \"a?b\" AS w")
	want = "SELECT '?' AS q, $1 AS v, \"a?b\" AS w"
	if got != want {
		t.Fatalf("引号内的问号被误改:\n got %q\nwant %q", got, want)
	}
	// 无占位符时原样
	if got := Rebind("SELECT 1"); got != "SELECT 1" {
		t.Fatalf("无占位符不应改写: %q", got)
	}
}

func TestShellAppendSQLDialect(t *testing.T) {
	old := DBType
	t.Cleanup(func() { DBType = old })

	DBType = "sqlite3"
	if sql := shellAppendSQL(); !strings.Contains(sql, "substr(") {
		t.Fatalf("SQLite 应使用 substr: %s", sql)
	}
	DBType = "postgres"
	sql := shellAppendSQL()
	if !strings.Contains(sql, "right(") || strings.Contains(sql, "substr(") {
		t.Fatalf("PostgreSQL 应使用 right: %s", sql)
	}
	// 两种方言都不得使用反引号（PG 不支持）
	if strings.Contains(sql, "`") {
		t.Fatalf("SQL 不应含反引号: %s", sql)
	}
}

func TestTableColumnsAndSequenceSQLite(t *testing.T) {
	tmp := t.TempDir()
	oldName, oldType := DatabaseName, DBType
	DatabaseName, DBType = filepath.Join(tmp, "t.db"), "sqlite3"
	ConnectDateBase()
	DatabaseName, DBType = oldName, oldType
	t.Cleanup(func() {
		if Engine != nil {
			Engine.Close()
		}
	})

	cols := tableColumns("clients")
	for _, want := range []string{"id", "uid", "online", "listener"} {
		if !cols[want] {
			t.Errorf("clients 表应包含列 %s（实际 %v）", want, cols)
		}
	}
	if cols["no_such_column"] {
		t.Error("不存在的列不应命中")
	}
	if !columnExists("clients", "uid") || columnExists("clients", "nope") {
		t.Error("columnExists 判定不符")
	}

	// 自增序列重置：插入两条 → id 连续；重置到 42 后下一条应为 43
	insertClient := func(uid string) int64 {
		c := &Clients{Uid: uid}
		if _, err := Engine.Insert(c); err != nil {
			t.Fatalf("插入失败: %v", err)
		}
		return c.ID
	}
	first := insertClient("uid-a")
	second := insertClient("uid-b")
	if second != first+1 {
		t.Fatalf("自增 ID 不连续: %d → %d", first, second)
	}
	if err := ResetAutoIncrement("clients", 42); err != nil {
		t.Fatalf("重置序列失败: %v", err)
	}
	if next := insertClient("uid-c"); next != 43 {
		t.Fatalf("重置后下一条 ID = %d，期望 43", next)
	}

	// 尾部读取（SQLite 路径）
	if _, err := Engine.Insert(&Shell{Uid: "uid-a", ShellContent: "abcdef"}); err != nil {
		t.Fatalf("插入 shell 失败: %v", err)
	}
	if tail, err := LoadShellTail("uid-a", 100); err != nil || tail != "abcdef" {
		t.Fatalf("LoadShellTail 全文 = %q, %v", tail, err)
	}
	if tail, _ := LoadShellTail("uid-a", 2); tail != "ef" {
		t.Fatalf("LoadShellTail 截断 = %q，期望 ef", tail)
	}
}
