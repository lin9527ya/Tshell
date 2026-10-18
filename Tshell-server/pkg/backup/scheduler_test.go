package backup

import (
	"path/filepath"
	"testing"
	"time"
)

// 定时备份是主要机制（手动只是补充），必须有回归防线。

func TestSchedulerCreatesBackups(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := New(Options{DBPath: dbPath, Dir: filepath.Join(dir, "backups"), Keep: 3, Interval: 20 * time.Millisecond})
	stop := m.StartScheduler()
	defer stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if list, err := m.List(); err == nil && len(list) > 0 {
			if !list[0].Verified {
				t.Fatal("定时备份未通过校验")
			}
			if list[0].Note != "scheduled" {
				t.Fatalf("定时备份备注不符: %q", list[0].Note)
			}
			// 保留份数仍然生效（间隔很短，会不断产生新备份）。
			// 允许 +1：观察点可能落在"已创建、尚未清理"的瞬间。
			if len(list) > 4 {
				t.Fatalf("保留份数未生效: %d 份", len(list))
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("定时备份未产生任何备份")
}

func TestSchedulerDisabledWhenIntervalZero(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := New(Options{DBPath: dbPath, Dir: filepath.Join(dir, "backups"), Interval: 0})
	stop := m.StartScheduler()
	time.Sleep(60 * time.Millisecond)
	stop() // 停止函数在未启动定时器时也必须可调用（幂等、不 panic）

	list, err := m.List()
	if err != nil {
		t.Fatalf("列出失败: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("interval=0 时不应自动备份，实际 %d 份", len(list))
	}
}

func TestSchedulerStopHaltsBackups(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	newFixtureDB(t, dbPath, "uid-alpha")

	m := New(Options{DBPath: dbPath, Dir: filepath.Join(dir, "backups"), Keep: 50, Interval: 20 * time.Millisecond})
	stop := m.StartScheduler()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if list, _ := m.List(); len(list) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	time.Sleep(50 * time.Millisecond) // 让可能已在途的一轮结束
	before, _ := m.List()

	time.Sleep(200 * time.Millisecond)
	after, _ := m.List()
	if len(after) != len(before) {
		t.Fatalf("停止后仍在备份: %d -> %d 份", len(before), len(after))
	}
}
