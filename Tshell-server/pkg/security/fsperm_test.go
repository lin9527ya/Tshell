package security

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ===== OPSEC P0-4：数据目录/敏感文件的权限收敛 =====

func requirePOSIX(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows 不体现 POSIX 权限位")
	}
}

func perm(t *testing.T, path string) os.FileMode {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return st.Mode().Perm()
}

func TestSecureMkdirAllCreates0700(t *testing.T) {
	requirePOSIX(t)
	dir := filepath.Join(t.TempDir(), "downloads", "1")
	if err := SecureMkdirAll(dir); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, dir); got != DirMode {
		t.Errorf("目录权限应为 %04o，实际 %04o", DirMode, got)
	}
}

// 已存在的宽权限目录必须被收敛 —— MkdirAll 本身不会改已存在目录的权限，
// 而历史部署里这些目录正是 0777 建的。
func TestSecureMkdirAllTightensExisting(t *testing.T) {
	requirePOSIX(t)
	dir := filepath.Join(t.TempDir(), "plugins")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, dir); got != 0o777 {
		t.Fatalf("前置条件不成立：权限应为 0777，实际 %04o", got)
	}
	if err := SecureMkdirAll(dir); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, dir); got != DirMode {
		t.Errorf("已存在目录应收敛为 %04o，实际 %04o", DirMode, got)
	}
}

func TestSecureCreateAndAppendUse0600(t *testing.T) {
	requirePOSIX(t)
	dir := t.TempDir()

	f, err := SecureCreate(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("x")
	f.Close()
	if got := perm(t, filepath.Join(dir, "a.txt")); got != FileMode {
		t.Errorf("SecureCreate 权限应为 %04o，实际 %04o", FileMode, got)
	}

	// 已存在 0644 文件再 Append：应收敛
	p := filepath.Join(dir, "b.log")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := SecureAppend(p)
	if err != nil {
		t.Fatal(err)
	}
	g.WriteString("new")
	g.Close()
	if got := perm(t, p); got != FileMode {
		t.Errorf("SecureAppend 应把已存在文件收敛为 %04o，实际 %04o", FileMode, got)
	}
}

// 不该把运维**有意设置得更严**的权限放宽
func TestTightenIfWiderNeverLoosens(t *testing.T) {
	requirePOSIX(t)
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	changed, err := TightenIfWider(p, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("当前权限比目标更严时不应改动")
	}
	if got := perm(t, p); got != 0o400 {
		t.Errorf("权限不应被放宽，实际 %04o", got)
	}
}

func TestTightenDataDirsReportsChanges(t *testing.T) {
	requirePOSIX(t)
	base := t.TempDir()
	wide := filepath.Join(base, "wide")
	tight := filepath.Join(base, "tight")
	missing := filepath.Join(base, "missing")

	for _, d := range []string{wide, tight} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(wide, 0o777); err != nil {
		t.Fatal(err)
	}

	changed := TightenDataDirs(wide, tight, missing, "")
	// 只报告真正改了权限的目录；不存在的目录不报（不存在比权限宽更安全）
	if len(changed) != 1 || changed[0] != wide {
		t.Errorf("应只报告被收敛的目录，实际 %v", changed)
	}
	if got := perm(t, wide); got != DirMode {
		t.Errorf("宽权限目录应收敛为 %04o，实际 %04o", DirMode, got)
	}
	if got := perm(t, tight); got != 0o700 {
		t.Errorf("本就合规的目录不应被改动，实际 %04o", got)
	}
}

// 幂等：连续两次收敛，第二次不再报告变更（避免每次启动都刷告警）
func TestTightenDataDirsIdempotent(t *testing.T) {
	requirePOSIX(t)
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if got := TightenDataDirs(dir); len(got) != 1 {
		t.Fatalf("第一次应收敛 1 个，实际 %v", got)
	}
	if got := TightenDataDirs(dir); len(got) != 0 {
		t.Errorf("第二次不应再报告变更，实际 %v", got)
	}
}

// ===== 平台无关的判定逻辑（本机 Windows 也能真跑）=====
//
// 上面依赖真实权限位的用例在 Windows 会 skip —— 那样本机等于零证据。
// 判定本身（"该不该收紧"）是与平台无关的位运算，单独测它。

func TestNeedsTighten(t *testing.T) {
	cases := []struct {
		cur, want os.FileMode
		exp       bool
		why       string
	}{
		{0o777, 0o700, true, "0777 比 0700 宽（组/其他可读写执行）"},
		{0o755, 0o700, true, "0755 组/其他可读可执行"},
		{0o750, 0o700, true, "0750 组可读"},
		{0o701, 0o700, true, "0701 其他可执行"},
		{0o700, 0o700, false, "已合规"},
		{0o600, 0o700, false, "比目标更严，不得放宽"},
		{0o400, 0o700, false, "只读，比目标更严"},
		{0o644, 0o600, true, "文件 0644 → 0600 需要收紧"},
		{0o600, 0o600, false, "文件已合规"},
		{0o400, 0o600, false, "文件只读，不得放宽"},
		{0o666, 0o600, true, "0666 组/其他可读写"},
	}
	for _, c := range cases {
		if got := needsTighten(c.cur, c.want); got != c.exp {
			t.Errorf("%s：needsTighten(%04o, %04o) = %v，期望 %v",
				c.why, c.cur, c.want, got, c.exp)
		}
	}
}

// 权限常量本身要被钉住：改宽了等于悄悄放松全项目的落盘安全
func TestModeConstantsArePrivate(t *testing.T) {
	if DirMode != 0o700 {
		t.Errorf("DirMode 必须为 0700，实际 %04o", DirMode)
	}
	if FileMode != 0o600 {
		t.Errorf("FileMode 必须为 0600，实际 %04o", FileMode)
	}
}
