package database

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

// PostgreSQL 集成测试：默认跳过，设置 TSHELL_TEST_PG_DSN 后运行。
// CI 与本地用 `docker run postgres:16-alpine` 提供实例。
//
// 覆盖的是"切到 Postgres 之后真正会跑的路径"：建表（Sync2）、版本化迁移、
// xorm CRUD、**原生 SQL 占位符改写**、自增序列重置、shell 追加的方言语句。
func TestPostgresBackend(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TSHELL_TEST_PG_DSN"))
	if dsn == "" {
		t.Skip("未设置 TSHELL_TEST_PG_DSN，跳过 PostgreSQL 集成测试")
	}

	// 先探测可达性：ConnectDateBase 失败会 log.Fatalf（杀进程），不适合直接用来探测
	probe, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("解析 DSN 失败: %v", err)
	}
	if err := probe.Ping(); err != nil {
		probe.Close()
		t.Skipf("PostgreSQL 不可达: %v", err)
	}
	// 重置 schema，保证每次运行从干净状态开始
	if _, err := probe.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		probe.Close()
		t.Fatalf("重置 schema 失败: %v", err)
	}
	probe.Close()

	oldType, oldDSN, oldName := DBType, PGDSN, DatabaseName
	DBType, PGDSN = "postgres", dsn
	t.Cleanup(func() {
		if Engine != nil {
			Engine.Close()
		}
		DBType, PGDSN, DatabaseName = oldType, oldDSN, oldName
	})

	ConnectDateBase()

	// 1) 版本化迁移在 PG 上正常应用
	version, err := SchemaVersion()
	if err != nil {
		t.Fatalf("读取 schema 版本失败: %v", err)
	}
	want := 0
	for _, m := range migrations() {
		if m.Version > want {
			want = m.Version
		}
	}
	if version != want {
		t.Fatalf("schema 版本 = %d，期望 %d", version, want)
	}

	// 2) xorm CRUD
	c := &Clients{Uid: "pg-uid-1", Computer: "PGHOST", Online: "1", Os: "linux"}
	if _, err := Engine.Insert(c); err != nil {
		t.Fatalf("插入 clients 失败: %v", err)
	}
	if c.ID <= 0 {
		t.Fatalf("自增 ID 未回填: %+v", c)
	}
	var got Clients
	if has, err := Engine.Where("uid = ?", "pg-uid-1").Get(&got); err != nil || !has {
		t.Fatalf("查询 clients 失败: has=%v err=%v", has, err)
	}
	if got.Computer != "PGHOST" {
		t.Fatalf("查询结果不符: %+v", got)
	}

	// 3) 原生 SQL 占位符改写（下载进度更新：原来直接写 ?，PG 会报语法错误）
	if _, err := ExecSQL("INSERT INTO downloads (uid, file_name, file_path, file_size, downloaded_size) VALUES (?, ?, ?, ?, ?)",
		"pg-uid-1", "a.bin", "/tmp/a.bin", 0, 0); err != nil {
		t.Fatalf("插入 downloads 失败: %v", err)
	}
	if _, err := QueryStringSQL("UPDATE downloads SET file_size = ?, downloaded_size = ? WHERE uid = ? AND file_path = ?",
		1000, 400, "pg-uid-1", "/tmp/a.bin"); err != nil {
		t.Fatalf("更新 downloads 失败: %v", err)
	}
	rows, err := QueryStringSQL("SELECT downloaded_size FROM downloads WHERE uid = ? AND file_path = ?", "pg-uid-1", "/tmp/a.bin")
	if err != nil || len(rows) != 1 {
		t.Fatalf("查询 downloads 失败: %v %v", rows, err)
	}
	if rows[0]["downloaded_size"] != "400" {
		t.Fatalf("downloaded_size = %q，期望 400", rows[0]["downloaded_size"])
	}

	// 4) shell 追加的方言语句（SQLite substr / PG right）
	if _, err := Engine.Insert(&Shell{Uid: "pg-uid-1", ShellContent: "abc"}); err != nil {
		t.Fatalf("插入 shell 失败: %v", err)
	}
	execBatchSQL([]batchStmt{{sql: shellAppendSQL(), args: []interface{}{"def", 100, "pg-uid-1"}}})
	var sh Shell
	if has, err := Engine.Where("uid = ?", "pg-uid-1").Get(&sh); err != nil || !has {
		t.Fatalf("查询 shell 失败: %v %v", has, err)
	}
	if sh.ShellContent != "abcdef" {
		t.Fatalf("shell 追加结果 = %q，期望 abcdef", sh.ShellContent)
	}

	// 4b) 尾部读取（SQLite substr(x,-n) / PG right(x,n)）
	tail, err := LoadShellTail("pg-uid-1", 100)
	if err != nil || tail != "abcdef" {
		t.Fatalf("LoadShellTail 全文 = %q, %v", tail, err)
	}
	if tail, _ := LoadShellTail("pg-uid-1", 2); tail != "ef" {
		t.Fatalf("LoadShellTail 截断 = %q，期望 ef", tail)
	}

	// 5) 自增序列重置（sqlite_sequence → PG setval）
	if err := ResetAutoIncrement("clients", 42); err != nil {
		t.Fatalf("重置序列失败: %v", err)
	}
	next := &Clients{Uid: "pg-uid-2", Computer: "PGHOST"}
	if _, err := Engine.Insert(next); err != nil {
		t.Fatalf("重置后插入失败: %v", err)
	}
	if next.ID != 43 {
		t.Fatalf("重置后 ID = %d，期望 43", next.ID)
	}

	// 6) 列存在性判断走 information_schema
	cols := tableColumns("clients")
	for _, want := range []string{"id", "uid", "online"} {
		if !cols[want] {
			t.Errorf("clients 表应包含列 %s（实际 %v）", want, cols)
		}
	}
}
