package commands

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestDarwinStubsDoNotSilentlySucceed Darwin 桩实现不能"静默假装成功"。
// HideFile/UnhideFile/StopScreenMonitor 原来返回 nil，UI 会显示"已隐藏/已停止"，
// 实际什么都没做 —— 这类假成功比明确报错更危险（用户会误以为操作生效）。
func TestDarwinStubsDoNotSilentlySucceed(t *testing.T) {
	src, err := os.ReadFile("commands_darwin_stubs.go")
	if err != nil {
		t.Fatalf("读取 Darwin 桩文件失败: %v", err)
	}
	text := string(src)

	for _, fn := range []string{"HideFile", "UnhideFile", "StopScreenMonitor"} {
		re := regexp.MustCompile(`(?s)func ` + fn + `\([^)]*\) error \{(.*?)\n\}`)
		m := re.FindStringSubmatch(text)
		if m == nil {
			t.Fatalf("找不到 %s 的实现", fn)
		}
		body := m[1]
		if strings.Contains(body, "return nil") {
			t.Errorf("%s 在 Darwin 上仍然是静默成功（return nil），应返回明确错误", fn)
		}
		if !strings.Contains(body, "errors.New") {
			t.Errorf("%s 应返回明确的平台不支持错误", fn)
		}
	}
}
