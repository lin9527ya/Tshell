package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 恢复的验收标准：恢复后数据库内容回到备份时刻，且出错时现有数据毫发无损。

func TestRestoreReplacesDatabaseContent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-before")

	m := newManager(t, dbPath)
	info, err := m.Create("before-change")
	if err != nil {
		t.Fatalf("备份失败: %v", err)
	}

	// 备份之后继续写入（模拟"备份点之后发生的变更"）
	appendRow(t, dbPath, "uid-after")
	if markerCount(t, dbPath, "uid-after") != 1 {
		t.Fatal("前置条件不成立：变更未写入")
	}

	safety, err := Restore(info.Path, dbPath, false)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if markerCount(t, dbPath, "uid-before") != 1 {
		t.Fatal("恢复后缺少备份时刻的数据")
	}
	if markerCount(t, dbPath, "uid-after") != 0 {
		t.Fatal("恢复后仍能看到备份点之后的数据（恢复没生效）")
	}
	if safety == "" {
		t.Fatal("恢复前应保存一份快照，便于回退")
	}
	if markerCount(t, safety, "uid-after") != 1 {
		t.Fatal("恢复前快照里应有被覆盖掉的那份数据")
	}
	// 旧的 -wal/-shm 必须清掉，否则新库会读到不匹配的日志
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + suffix); err == nil {
			t.Fatalf("恢复后残留 %s，新库可能读到旧库的日志", suffix)
		}
	}
}

func TestRestoreRefusesUnusableBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-keep")

	bad := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(bad, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}

	safety, err := Restore(bad, dbPath, false)
	if err == nil {
		t.Fatal("不可用的备份不应被恢复")
	}
	if safety != "" {
		t.Fatalf("校验失败时不应产生快照: %s", safety)
	}
	// 现有数据毫发无损
	if markerCount(t, dbPath, "uid-keep") != 1 {
		t.Fatal("恢复失败却改动了现有数据库")
	}
}

func TestRestoreRefusesNonTshellBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-keep")

	// 合法 SQLite，但不是本系统的库
	other := filepath.Join(dir, "tshell-backup-other.db")
	newUnrelatedDB(t, other)

	if _, err := Restore(other, dbPath, false); err == nil {
		t.Fatal("结构不符的库不应被恢复")
	} else if !strings.Contains(err.Error(), "备份不可用") {
		t.Fatalf("错误信息应说明备份不可用: %v", err)
	}
	if markerCount(t, dbPath, "uid-keep") != 1 {
		t.Fatal("拒绝恢复时不应改动现有数据库")
	}
}

// TestRestoreRefusesWhenDatabaseLooksInUse 非空 WAL = 可能正在被服务端使用
func TestRestoreRefusesWhenDatabaseLooksInUse(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-keep")

	m := newManager(t, dbPath)
	info, err := m.Create("snapshot")
	if err != nil {
		t.Fatal(err)
	}
	// 制造"服务端还在跑"的痕迹：非空 WAL
	if err := os.WriteFile(dbPath+"-wal", []byte("wal-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(info.Path, dbPath, false); err == nil {
		t.Fatal("检测到非空 WAL 时应拒绝恢复")
	} else if !strings.Contains(err.Error(), "停止服务端") {
		t.Fatalf("错误信息应提示先停服务端: %v", err)
	}
	// -force 可跳过该检查（运维明确知道自己在做什么）
	if _, err := Restore(info.Path, dbPath, true); err != nil {
		t.Fatalf("force 恢复应成功: %v", err)
	}
}

// TestRestoreRejectsSameFile 备份与目标同一个文件时必须拒绝（否则等于清库）
func TestRestoreRejectsSameFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-keep")

	if _, err := Restore(dbPath, dbPath, true); err == nil {
		t.Fatal("备份与目标是同一文件时应拒绝")
	}
	if markerCount(t, dbPath, "uid-keep") != 1 {
		t.Fatal("拒绝时不应改动数据库")
	}
}

// TestResolveBackupAcceptsNameOrPath CLI 既接受路径也接受备份名
func TestResolveBackupAcceptsNameOrPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := newManager(t, dbPath)
	info, err := m.Create("cli")
	if err != nil {
		t.Fatal(err)
	}

	if got, err := ResolveBackup(m, info.Path); err != nil || got != info.Path {
		t.Fatalf("按路径解析失败: %v %q", err, got)
	}
	if got, err := ResolveBackup(m, info.Name); err != nil || got != info.Path {
		t.Fatalf("按名字解析失败: %v %q", err, got)
	}
	if _, err := ResolveBackup(m, "no-such-backup.db"); err == nil {
		t.Fatal("不存在的备份应报错")
	}
	if _, err := ResolveBackup(m, ""); err == nil {
		t.Fatal("空参数应报错")
	}
}
