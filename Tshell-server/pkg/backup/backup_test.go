package backup

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// 备份系统的验收标准只有一条：**能恢复**。
// 因此这里不只测"文件生成了"，而是测快照内容、WAL 一致性、校验拦截、
// 保留策略边界与恢复后的数据状态。

// newFixtureDB 建一个结构最小但"是本系统"的 SQLite 库，并写入一行可辨认的数据。
func newFixtureDB(t *testing.T, path string, marker string) {
	t.Helper()
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT)`,
		`CREATE TABLE IF NOT EXISTS clients (id INTEGER PRIMARY KEY, uid TEXT)`,
		`CREATE TABLE IF NOT EXISTS listener (id INTEGER PRIMARY KEY, listen_address TEXT)`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("建表失败: %v", err)
		}
	}
	if marker != "" {
		if _, err := db.Exec(`INSERT INTO clients (uid) VALUES (?)`, marker); err != nil {
			t.Fatalf("写入数据失败: %v", err)
		}
	}
}

// markerCount 统计库里的标记行数（0 = 没有、1 = 有）
func markerCount(t *testing.T, path, marker string) int {
	t.Helper()
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM clients WHERE uid = ?`, marker).Scan(&n); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	return n
}

func newManager(t *testing.T, dbPath string) *Manager {
	t.Helper()
	return New(Options{DBPath: dbPath, Dir: filepath.Join(t.TempDir(), "backups"), Keep: 3, Prefix: "tshell-backup"})
}

// TestCreateProducesVerifiedSnapshot 备份可校验、内容与源库一致
func TestCreateProducesVerifiedSnapshot(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := newManager(t, dbPath)
	info, err := m.Create("test")
	if err != nil {
		t.Fatalf("备份失败: %v", err)
	}
	if !info.Verified || info.Size == 0 {
		t.Fatalf("备份元信息不符: %+v", info)
	}
	if err := Verify(info.Path); err != nil {
		t.Fatalf("备份校验失败: %v", err)
	}
	if got := markerCount(t, info.Path, "uid-alpha"); got != 1 {
		t.Fatalf("快照里没有源库的数据（找到 %d 行）", got)
	}
	// 权限：备份含全部凭据，不能让同机其他用户读
	st, err := os.Stat(info.Path)
	if err != nil {
		t.Fatalf("stat 失败: %v", err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("备份权限 = %o，期望 600", perm)
	}
}

// TestSnapshotIncludesUncheckpointedWAL 关键差异：直接 cp 会漏掉 WAL 里的数据，
// VACUUM INTO 必须包含它（否则备份"看起来正常但少了最近的写入"）。
func TestSnapshotIncludesUncheckpointedWAL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "")

	// 保持连接打开（不 checkpoint），写入后立刻备份
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO clients (uid) VALUES ('uid-in-wal')`); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if st, err := os.Stat(dbPath + "-wal"); err != nil || st.Size() == 0 {
		t.Fatalf("前置条件不成立：期望存在非空 WAL（err=%v）", err)
	}

	m := newManager(t, dbPath)
	info, err := m.Create("wal")
	if err != nil {
		t.Fatalf("备份失败: %v", err)
	}
	if got := markerCount(t, info.Path, "uid-in-wal"); got != 1 {
		t.Fatal("快照漏掉了 WAL 中未 checkpoint 的数据")
	}
}

// TestRetentionKeepsNewestAndLeavesForeignFiles 保留策略：只删自己的旧备份
func TestRetentionKeepsNewestAndLeavesForeignFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := newManager(t, dbPath) // Keep = 3
	// 目录里放两个不该被碰的文件
	if err := os.MkdirAll(m.Options().Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := []string{"notes.txt", "other-app-20260101.db"}
	for _, f := range foreign {
		if err := os.WriteFile(filepath.Join(m.Options().Dir, f), []byte("keep me"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	created := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		info, err := m.Create("retention")
		if err != nil {
			t.Fatalf("第 %d 次备份失败: %v", i+1, err)
		}
		created = append(created, info.Name)
	}
	removed, err := m.Cleanup()
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if len(removed) != 2 {
		t.Fatalf("应删除 2 份，实际 %d 份: %v", len(removed), removed)
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("列出失败: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("应保留 3 份，实际 %d 份", len(list))
	}
	// 同秒内多次备份靠后缀区分，删除的必须是最早的两份
	kept := map[string]bool{}
	for _, info := range list {
		kept[info.Name] = true
	}
	for _, name := range removed {
		if kept[name] {
			t.Fatalf("被删除的备份仍在列表里: %s", name)
		}
	}
	for _, name := range created[len(created)-3:] {
		if !kept[name] {
			t.Fatalf("最新的备份 %s 被误删", name)
		}
	}
	// 外来文件原封不动
	for _, f := range foreign {
		if _, err := os.Stat(filepath.Join(m.Options().Dir, f)); err != nil {
			t.Fatalf("外来文件 %s 被清理了: %v", f, err)
		}
	}
}

// TestNamesAreUniqueWithinSameSecond 同一秒内连续备份不能互相覆盖
func TestNamesAreUniqueWithinSameSecond(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := newManager(t, dbPath)
	names := map[string]bool{}
	for i := 0; i < 3; i++ {
		info, err := m.Create("same-second")
		if err != nil {
			t.Fatalf("备份失败: %v", err)
		}
		if names[info.Name] {
			t.Fatalf("备份名重复（后一份会覆盖前一份）: %s", info.Name)
		}
		names[info.Name] = true
	}
}

// TestVerifyRejectsNonTshellDatabase 结构对但不是本系统的库必须被拒
func TestVerifyRejectsNonTshellDatabase(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "other.db")
	db, err := sql.Open("sqlite3", other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE unrelated (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := Verify(other); err == nil {
		t.Fatal("缺少必需表的库不应通过校验")
	} else if !strings.Contains(err.Error(), "users") {
		t.Fatalf("错误信息应指明缺失的表: %v", err)
	}

	// 空文件
	empty := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Verify(empty); err == nil {
		t.Fatal("空文件不应通过校验")
	}
}

// TestMarkerDetectsTamperedBackup 备份文件被截断/改动后应显示为未校验
func TestMarkerDetectsTamperedBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := newManager(t, dbPath)
	info, err := m.Create("tamper")
	if err != nil {
		t.Fatalf("备份失败: %v", err)
	}
	list, _ := m.List()
	if len(list) != 1 || !list[0].Verified {
		t.Fatalf("刚创建的备份应为已校验: %+v", list)
	}

	// 追加几个字节（模拟传输/拷贝被截断或改动）
	f, err := os.OpenFile(info.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("junk")
	f.Close()

	list, _ = m.List()
	if len(list) != 1 || list[0].Verified {
		t.Fatalf("被改动过的备份应显示为未校验: %+v", list)
	}
}

// TestDeleteAndOpenRejectPathTraversal 备份名不能越出备份目录
func TestDeleteAndOpenRejectPathTraversal(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")
	m := newManager(t, dbPath)
	if _, err := m.Create("test"); err != nil {
		t.Fatal(err)
	}

	bad := []string{
		"../database.db",
		"tshell-backup-x/../../database.db",
		"/etc/passwd",
		"other-app-20260101.db", // 前缀不符
		"tshell-backup-x.txt",   // 后缀不符
		"",
	}
	for _, name := range bad {
		if err := m.Delete(name); err == nil {
			t.Errorf("Delete(%q) 应被拒绝", name)
		}
		if _, _, err := m.Open(name); err == nil {
			t.Errorf("Open(%q) 应被拒绝", name)
		}
	}
	// 源库仍在
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("源库被误删: %v", err)
	}
}

// TestPostgresBackendUnsupported PostgreSQL 后端不使用内置备份（VACUUM INTO 是 SQLite 语义）
func TestPostgresBackendUnsupported(t *testing.T) {
	m := New(Options{DBPath: "/tmp/nonexistent.db", Dir: t.TempDir(), Driver: "postgres"})
	if _, err := m.Create("pg"); !errors.Is(err, ErrNotSQLite) {
		t.Fatalf("PostgreSQL 下 Create 应返回 ErrNotSQLite，实际: %v", err)
	}
}

// TestImportValidatesBeforeStoring 导入先校验：坏文件不落盘
func TestImportValidatesBeforeStoring(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")
	m := newManager(t, dbPath)

	if _, err := m.Import("garbage.db", []byte("this is not a sqlite file")); err == nil {
		t.Fatal("非 SQLite 内容不应导入成功")
	}
	entries, _ := os.ReadDir(m.Options().Dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("失败导入残留临时文件: %s", e.Name())
		}
	}

	// 合法备份可以导入
	info, err := m.Create("source")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := m.Import("tshell-backup-imported.db", content)
	if err != nil {
		t.Fatalf("合法备份导入失败: %v", err)
	}
	if !imported.Verified {
		t.Fatal("导入的备份应标记为已校验")
	}
	if err := Verify(imported.Path); err != nil {
		t.Fatalf("导入后校验失败: %v", err)
	}
}

// appendRow 向库中追加一行（模拟备份点之后的变更）
func appendRow(t *testing.T, path, marker string) {
	t.Helper()
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO clients (uid) VALUES (?)`, marker); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
}

// newUnrelatedDB 建一个合法但不属于本系统的 SQLite 库
func newUnrelatedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE unrelated (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
}

// TestPartialFilesAreNotListed 写入中的 .partial 不能出现在列表里：
// 备份过程中（VACUUM 可能持续数秒）列表若显示半成品，运维会以为"已经有备份了"。
func TestPartialFilesAreNotListed(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := newManager(t, dbPath)
	if err := os.MkdirAll(m.Options().Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// 模拟一份正在写入的备份
	partial := filepath.Join(m.Options().Dir, "tshell-backup-20261018-120000.000.db"+partialSuffix)
	if err := os.WriteFile(partial, []byte("half written"), 0o600); err != nil {
		t.Fatal(err)
	}

	list, err := m.List()
	if err != nil {
		t.Fatalf("列出失败: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("写入中的备份不应出现在列表里: %+v", list)
	}
	// 保留策略也不能把它当成备份删掉/计数
	if removed, err := m.Cleanup(); err != nil || len(removed) != 0 {
		t.Fatalf("清理不应涉及写入中的文件: %v %v", removed, err)
	}
	if _, err := os.Stat(partial); err != nil {
		t.Fatalf("写入中的文件被动了: %v", err)
	}
}

// TestFailedBackupLeavesNothingBehind 校验失败时不留残留（既不占位置，也不误导运维）
func TestFailedBackupLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newUnrelatedDB(t, dbPath) // 合法 SQLite，但不是本系统的库

	m := newManager(t, dbPath)
	if _, err := m.Create("should-fail"); err == nil {
		t.Fatal("结构不符的库不应备份成功")
	}
	entries, err := os.ReadDir(m.Options().Dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("读取目录失败: %v", err)
	}
	for _, e := range entries {
		t.Fatalf("失败备份留下残留文件: %s", e.Name())
	}
}
