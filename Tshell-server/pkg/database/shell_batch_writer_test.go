package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendShellContentBatchFlush(t *testing.T) {
	// 独立临时库，避免污染开发数据库
	tmpDir := t.TempDir()
	oldDBName := DatabaseName
	DatabaseName = filepath.Join(tmpDir, "test_shell_batch.db")
	defer func() {
		DatabaseName = oldDBName
		if Engine != nil {
			Engine.Close()
		}
	}()

	ConnectDateBase()
	if Engine == nil {
		t.Fatalf("Engine 初始化失败")
	}

	uid := "batch_test_uid_00000000000000000000000000000000"
	if _, err := Engine.Insert(&Shell{Uid: uid}); err != nil {
		t.Fatalf("insert shell: %v", err)
	}

	// 多次追加（模拟高频命令输出），应合并为批量落库且顺序保持
	AppendShellContent(uid, "line1\n")
	AppendShellContent(uid, "line2\n")
	AppendShellContent(uid, "line3\n")

	// 等待 flush 周期（500ms）+ 余量
	deadline := time.Now().Add(3 * time.Second)
	for {
		var s Shell
		has, _ := Engine.Where("uid = ?", uid).Get(&s)
		if has && len(s.ShellContent) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("flush 超时，shell_content 未写入")
		}
		time.Sleep(50 * time.Millisecond)
	}

	var s Shell
	if _, err := Engine.Where("uid = ?", uid).Get(&s); err != nil {
		t.Fatalf("query shell: %v", err)
	}
	want := "line1\nline2\nline3\n"
	if s.ShellContent != want {
		t.Fatalf("内容不一致: got %q want %q", s.ShellContent, want)
	}

	// 二次追加验证续写（截断逻辑由 SQL substr 保持）
	AppendShellContent(uid, "line4\n")
	deadline = time.Now().Add(3 * time.Second)
	for {
		var s2 Shell
		has, _ := Engine.Where("uid = ?", uid).Get(&s2)
		if has && len(s2.ShellContent) == len(want)+len("line4\n") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("二次 flush 超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
