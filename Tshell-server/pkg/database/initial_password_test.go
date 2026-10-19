package database

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ===== OPSEC P0-3：初始管理员密码不得进日志 =====

func TestWriteInitialPasswordFilePermissionsAndContent(t *testing.T) {
	dir := t.TempDir()
	path, err := writeInitialPasswordFile(InitOptions{PasswordFileDir: dir}, "admin", "S3cret-Generated-Pw")
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("文件应落在指定目录 %s，实际 %s", dir, path)
	}
	if filepath.Base(path) != InitialPasswordFileName {
		t.Errorf("文件名应为 %s，实际 %s", InitialPasswordFileName, filepath.Base(path))
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// 0600：密码文件不能让同机其他用户读
	if got := st.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Errorf("权限应为 0600，实际 %04o", got)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, "admin") || !strings.Contains(s, "S3cret-Generated-Pw") {
		t.Errorf("文件应含用户名与密码，实际内容:\n%s", s)
	}
	// 要提示删除：否则这个文件会长期留在服务器上
	if !strings.Contains(s, "删除") {
		t.Error("文件应提示首次登录后删除")
	}
}

// 已存在的宽权限文件必须被收敛回 0600（O_CREATE 不会改已存在文件的权限）
func TestWriteInitialPasswordFileTightensExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不体现 POSIX 权限位")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, InitialPasswordFileName)
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := writeInitialPasswordFile(InitOptions{PasswordFileDir: dir}, "u", "p"); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o600 {
		t.Errorf("已存在文件应被收敛为 0600，实际 %04o", got)
	}
}

// 目录不存在时返回错误而不是 panic（由调用方决定是否告警重启）
func TestWriteInitialPasswordFileMissingDir(t *testing.T) {
	_, err := writeInitialPasswordFile(InitOptions{PasswordFileDir: filepath.Join(t.TempDir(), "nope")}, "u", "p")
	if err == nil {
		t.Error("目录不存在时应返回错误")
	}
}

// 回归守卫：初始化路径里不允许再把密码拼进日志。
//
// 这是源码级断言 —— 有点粗，但这类"顺手加一条带密码的日志"最容易在
// 后续排障时被加回来，而单元测试很难覆盖到真实的 log.Printf 调用点。
func TestNoPasswordInLogStatements(t *testing.T) {
	src, err := os.ReadFile("sqlite3.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)

	// 允许出现的：-print-initial-password 显式开关那一条（带"显式要求"标记）
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "log.Printf") && !strings.Contains(line, "log.Println") {
			continue
		}
		if !strings.Contains(line, "password=%s") && !strings.Contains(line, "password:") {
			continue
		}
		if strings.Contains(line, "显式要求") {
			continue // 显式开关，运维已确认输出不被留存
		}
		t.Errorf("sqlite3.go:%d 疑似把密码写进日志：%s", i+1, strings.TrimSpace(line))
	}

	if strings.Contains(s, "已生成随机密码：username=%s, password=%s") {
		t.Error("旧的明文密码日志仍在（OPSEC P0-3 回归）")
	}
}
