package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ===== cleanup 纯逻辑单测 =====
// 平台副作用（reg delete / schtasks / crontab / launchctl）不在单测里跑，
// 但凡是"决定删哪个对象"的判断都在这层，必须钉死：
// 判断错的后果是删掉别人的持久化项，比少删一条严重得多。

func TestParseCleanupSpec(t *testing.T) {
	cases := []struct {
		spec    string
		want    cleanupScopes
		wantErr bool
	}{
		{"", cleanupScopes{persist: true, files: true, uac: true, wmi: true}, false},
		{"all", cleanupScopes{persist: true, files: true, uac: true, wmi: true}, false},
		{"persist", cleanupScopes{persist: true}, false},
		{"FILES Persist", cleanupScopes{persist: true, files: true}, false},
		{"uac wmi", cleanupScopes{uac: true, wmi: true}, false},
		{"bogus", cleanupScopes{}, true},
		// 已解析的部分保留（回执里能看出解析到哪一步），但整体报错
		{"persist bogus", cleanupScopes{persist: true}, true},
	}
	for _, c := range cases {
		got, err := parseCleanupSpec(c.spec)
		if (err != nil) != c.wantErr {
			t.Fatalf("parseCleanupSpec(%q) err=%v wantErr=%v", c.spec, err, c.wantErr)
		}
		if got != c.want {
			t.Fatalf("parseCleanupSpec(%q) = %+v, want %+v", c.spec, got, c.want)
		}
	}
}

func TestCleanupRejectsUnknownScope(t *testing.T) {
	if _, err := Cleanup("persist bogus"); err == nil {
		t.Fatal("未知范围应返回错误，否则操作员会以为清理已执行")
	}
}

func TestRegRunValueNames(t *testing.T) {
	exe := `C:\Users\a\AppData\Local\Temp\x.exe`
	out := "HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Run\r\n" +
		"    SvcHost    REG_SZ    C:\\Users\\a\\AppData\\Local\\Temp\\x.exe\r\n" +
		"    Other      REG_SZ    \"C:\\Program Files\\Some App\\app.exe\"\r\n" +
		"    Expand     REG_EXPAND_SZ    C:\\Users\\a\\AppData\\Local\\Temp\\x.exe\r\n"

	got := regRunValueNames(out, exe)
	if len(got) != 2 || got[0] != "SvcHost" || got[1] != "Expand" {
		t.Fatalf("regRunValueNames = %v, want [SvcHost Expand]", got)
	}
	// 注册表路径大小写不敏感：换个大小写也必须命中同一个值
	if got := regRunValueNames(out, `c:\USERS\a\appdata\local\temp\X.EXE`); len(got) != 2 {
		t.Fatalf("大小写变体未命中: %v", got)
	}
	// 非本程序的值、非路径类型都不得被选中
	if got := regRunValueNames(out, `C:\Windows\System32\calc.exe`); len(got) != 0 {
		t.Fatalf("误选其它值的项: %v", got)
	}
}

func TestRegRunValueNamesEmptyInput(t *testing.T) {
	if got := regRunValueNames("", `C:\x.exe`); len(got) != 0 {
		t.Fatalf("空输入应返回空: %v", got)
	}
	if got := regRunValueNames("blah", ""); len(got) != 0 {
		t.Fatalf("空 exe 应返回空: %v", got)
	}
}

func TestTaskNamesForExeEnglish(t *testing.T) {
	exe := `C:\Users\a\AppData\Local\Temp\x.exe`
	out := "HostName:      PC\r\n" +
		"TaskName:      \\SvcTask\r\n" +
		"Next Run Time: N/A\r\n" +
		"Status:        Ready\r\n" +
		"Task To Run:   C:\\Users\\a\\AppData\\Local\\Temp\\x.exe\r\n" +
		"\r\n" +
		"TaskName:      \\OtherTask\r\n" +
		"Task To Run:   C:\\Windows\\System32\\calc.exe\r\n"

	got := taskNamesForExe(out, exe)
	if len(got) != 1 || got[0] != `\SvcTask` {
		t.Fatalf("taskNamesForExe(en) = %v, want [\\SvcTask]", got)
	}
}

func TestTaskNamesForExeChinese(t *testing.T) {
	exe := `C:\Users\a\AppData\Local\Temp\x.exe`
	out := "主机名:        PC\r\n" +
		"任务名:        \\计划任务一\r\n" +
		"下次运行时间:  N/A\r\n" +
		"要运行的任务:  C:\\Users\\a\\AppData\\Local\\Temp\\x.exe\r\n" +
		"\r\n" +
		"任务名:        \\别的任务\r\n" +
		"要运行的任务:  C:\\Windows\\System32\\calc.exe\r\n"

	got := taskNamesForExe(out, exe)
	if len(got) != 1 || got[0] != `\计划任务一` {
		t.Fatalf("taskNamesForExe(zh) = %v, want [\\计划任务一]", got)
	}
}

// 控制台代码页把标签转成 GBK 时，UTF-8 标签对不上——退化为"值以 \ 开头"的行，
// 任务名本身按原始字节回传，仍然是可用的。
func TestTaskNamesForExeFallbackWhenLabelsUnreadable(t *testing.T) {
	exe := `C:\Users\a\AppData\Local\Temp\x.exe`
	out := "HOST:  PC\n" +
		"NAMEX:  \\SvcTask\n" +
		"AUTHOR: C:\\Windows\\System32\\svchost.exe\n" +
		"RUNX:   C:\\Users\\a\\AppData\\Local\\Temp\\x.exe\n"

	got := taskNamesForExe(out, exe)
	if len(got) != 1 || got[0] != `\SvcTask` {
		t.Fatalf("fallback 解析失败: %v", got)
	}
}

func TestTaskNamesForExeIgnoresPathInAuthor(t *testing.T) {
	exe := `C:\Users\a\AppData\Local\Temp\x.exe`
	// 作者字段里出现同路径不应被当成任务（否则会去删别的任务）
	out := "TaskName:  \\RealTask\n" +
		"Author:    C:\\Users\\a\\AppData\\Local\\Temp\\x.exe\n" +
		"Task To Run: C:\\Windows\\System32\\calc.exe\n"
	if got := taskNamesForExe(out, exe); len(got) != 0 {
		t.Fatalf("作者字段误命中: %v", got)
	}
}

func TestStartupFileMatches(t *testing.T) {
	exe := `C:\Users\a\AppData\Local\Temp\x.exe`
	const exeSize = int64(2048)

	if !startupFileMatches("x.exe", 999, exe, exeSize) {
		t.Error("同名副本应命中")
	}
	if !startupFileMatches("X.EXE", 999, exe, exeSize) {
		t.Error("同名大小写变体应命中")
	}
	if !startupFileMatches("Svc12.exe", exeSize, exe, exeSize) {
		t.Error("同大小同扩展名应命中（persist 会用伪装修饰名拷贝）")
	}
	if startupFileMatches("Svc12.exe", exeSize+1, exe, exeSize) {
		t.Error("大小不同不应命中")
	}
	if startupFileMatches("readme.txt", exeSize, exe, exeSize) {
		t.Error("扩展名不同不应命中")
	}
	if startupFileMatches("", exeSize, exe, exeSize) {
		t.Error("空名不应命中")
	}
}

func TestFilterLinesReferencing(t *testing.T) {
	exe := "/opt/.cache/x"
	text := "@reboot /opt/.cache/x\n" +
		"0 3 * * * /usr/bin/backup.sh\n" +
		"@reboot /opt/.cache/x\n"

	kept, removed := filterLinesReferencing(text, exe)
	if len(removed) != 2 {
		t.Fatalf("应删除 2 行，实际 %d: %v", len(removed), removed)
	}
	if strings.Contains(kept, exe) {
		t.Fatalf("保留文本里仍有目标行: %q", kept)
	}
	if !strings.Contains(kept, "backup.sh") {
		t.Fatalf("误删无关行: %q", kept)
	}
}

func TestPlistReferencesPath(t *testing.T) {
	plist := `<plist><dict><key>ProgramArguments</key><array><string>/Users/a/Library/x</string></array></dict></plist>`
	if !plistReferencesPath(plist, "/Users/a/Library/x") {
		t.Error("应命中 plist 中的程序路径")
	}
	if plistReferencesPath(plist, "/Users/a/Other/y") {
		t.Error("不应命中其它路径")
	}
	if plistReferencesPath(plist, "") {
		t.Error("空路径不应命中")
	}
}

func TestRemoveWithRetry(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.old")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeWithRetry(p, 3, time.Millisecond); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("文件仍存在")
	}
	// 不存在视为成功（幂等）
	if err := removeWithRetry(p, 1, time.Millisecond); err != nil {
		t.Fatalf("删除不存在文件应成功: %v", err)
	}
}

func TestFindUpdateResidues(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"~svc_update_123.exe", "~svc_update_9.dll", "unrelated.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := findUpdateResidues(dir)
	if len(got) != 2 {
		t.Fatalf("应找到 2 个更新残留，实际 %d: %v", len(got), got)
	}
	for _, p := range got {
		if strings.Contains(p, "unrelated") {
			t.Fatalf("误匹配无关文件: %s", p)
		}
	}
	if n := len(findUpdateResidues("")); n != 0 {
		t.Fatalf("空目录应返回空，实际 %d", n)
	}
}

func TestRenderCleanupReport(t *testing.T) {
	sc := cleanupScopes{persist: true, files: true}
	out := renderCleanupReport(sc, []cleanupItem{
		cOK("A 已删除"),
		cWarn("B 删除失败"),
		cSkip("C 无此项"),
	})
	for _, want := range []string{"范围=persist+files", "✅ A 已删除", "⚠️ B 删除失败", "⏭️ C 无此项", "已清理 1 项，失败 1 项，跳过 1 项"} {
		if !strings.Contains(out, want) {
			t.Fatalf("回执缺少 %q:\n%s", want, out)
		}
	}
}

// cleanupScopes.String 空范围（只给了非法项的极端情况）不应输出空串
func TestCleanupScopesStringNone(t *testing.T) {
	if got := (cleanupScopes{}).String(); got != "none" {
		t.Fatalf("空范围 String = %q, want none", got)
	}
}
