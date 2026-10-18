package database

import (
	"fmt"
	"strings"

	// PostgreSQL 驱动注册（xorm "postgres" 方言依赖 database/sql 的驱动名 "postgres"）
	_ "github.com/lib/pq"
)

// 存储方言层：让同一套代码跑在 SQLite 与 PostgreSQL 上。
//
// 设计边界（与 docs/STORAGE.md 对应）：
//   - xorm 的 Where/Update/Insert 等构建器自己处理方言，不需要在这里干预；
//   - 只有**原生 SQL** 与**SQLite 专有语句**（PRAGMA、sqlite_sequence）需要隔离；
//   - 占位符：lib/pq 只认 $1/$2…，SQLite 只认 ?。原生 SQL 统一经 ExecSQL /
//     QueryStringSQL 走 Rebind，业务代码不再关心方言。
var (
	// DBType 数据库类型："sqlite3"（默认）或 "postgres"
	DBType = "sqlite3"
	// PGDSN PostgreSQL 连接串（host=... port=... user=... password=... dbname=... sslmode=...）
	PGDSN = ""
)

// IsPostgres 当前是否使用 PostgreSQL
func IsPostgres() bool { return strings.EqualFold(DBType, "postgres") }

// Rebind 把 ? 占位符改写为 PostgreSQL 的 $1..$n；SQLite 原样返回。
// 引号内的 ? 不处理（字符串字面量里的问号不是占位符）。
func Rebind(query string) string {
	if !IsPostgres() {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	var quote byte
	for i := 0; i < len(query); i++ {
		c := query[i]
		if quote != 0 {
			b.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			b.WriteByte(c)
		case '?':
			n++
			fmt.Fprintf(&b, "$%d", n)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// ExecSQL 执行原生 SQL（自动处理占位符方言）
func ExecSQL(query string, args ...interface{}) (int64, error) {
	if Engine == nil {
		return 0, fmt.Errorf("数据库引擎未初始化")
	}
	res, err := Engine.Exec(append([]interface{}{Rebind(query)}, args...)...)
	if err != nil || res == nil {
		return 0, err
	}
	return res.RowsAffected()
}

// QueryStringSQL 执行原生查询（自动处理占位符方言）
func QueryStringSQL(query string, args ...interface{}) ([]map[string]string, error) {
	if Engine == nil {
		return nil, fmt.Errorf("数据库引擎未初始化")
	}
	return Engine.QueryString(append([]interface{}{Rebind(query)}, args...)...)
}

// tableColumns 返回表的列名集合（跨方言）
func tableColumns(table string) map[string]bool {
	out := make(map[string]bool)
	if Engine == nil {
		return out
	}
	if IsPostgres() {
		rows, err := Engine.QueryString(
			"SELECT column_name FROM information_schema.columns WHERE table_name = $1", table)
		if err != nil {
			return out
		}
		for _, r := range rows {
			out[r["column_name"]] = true
		}
		return out
	}
	rows, err := Engine.QueryString("PRAGMA table_info(" + table + ")")
	if err != nil {
		return out
	}
	for _, row := range rows {
		out[row["name"]] = true
	}
	return out
}

// columnExists 判断表是否存在指定列（跨方言）。
func columnExists(table, column string) bool {
	if Engine == nil {
		return false
	}
	return tableColumns(table)[column]
}

// ResetAutoIncrement 把表的自增序列重置到 to（下一个插入从 to+1 开始）。
// SQLite 用 sqlite_sequence；PostgreSQL 用 setval + pg_get_serial_sequence。
func ResetAutoIncrement(table string, to int64) error {
	if TableExists(table) == false {
		return fmt.Errorf("表不存在: %s", table)
	}
	if IsPostgres() {
		var seq string
		if _, err := Engine.SQL(
			"SELECT COALESCE(pg_get_serial_sequence(?, 'id'), '')", table).Get(&seq); err != nil {
			return err
		}
		if seq == "" {
			return nil // 非 serial 列，无需重置
		}
		_, err := Engine.Exec("SELECT setval($1, $2, true)", seq, to)
		return err
	}
	if _, err := ExecSQL("DELETE FROM sqlite_sequence WHERE name = ?", table); err != nil {
		return err
	}
	_, err := ExecSQL("INSERT INTO sqlite_sequence (name, seq) VALUES (?, ?)", table, to)
	return err
}

// TableExists 判断表是否存在（跨方言）
func TableExists(table string) bool {
	if Engine == nil {
		return false
	}
	ok, err := Engine.IsTableExist(table)
	return err == nil && ok
}

// LoadShellTail 读取会话 shell 内容的尾部（最多 maxBytes 字节），跨方言。
// SQLite 用 substr(x, -n) 取末尾；PostgreSQL 用 right(x, n)（n 超过长度时返回全文）。
func LoadShellTail(uid string, maxBytes int) (string, error) {
	if Engine == nil {
		return "", fmt.Errorf("数据库引擎未初始化")
	}
	var sh Shell
	var query string
	var args []interface{}
	if IsPostgres() {
		query = "SELECT right(shell_content, ?) AS shell_content FROM shell WHERE uid = ?"
		args = []interface{}{maxBytes, uid}
	} else {
		query = "SELECT substr(shell_content, -?, ?) AS shell_content FROM shell WHERE uid = ?"
		args = []interface{}{maxBytes, maxBytes, uid}
	}
	_, err := Engine.SQL(query, args...).Get(&sh)
	return sh.ShellContent, err
}
