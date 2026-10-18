package commands

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Darwin 桩实现不能"静默假装成功"：返回 nil 会让 UI 显示"已隐藏/已停止"，
// 实际什么都没做 —— 这类假成功比明确报错更危险（用户会误以为操作生效）。
//
// 本用例对所有仍留在桩文件里的 error 函数做通用检查（新增桩也自动覆盖），
// 并锁定几个已经从"桩"升级为"真实实现"的函数确实有实现。
func TestDarwinStubsDoNotSilentlySucceed(t *testing.T) {
	src, err := os.ReadFile("commands_darwin_stubs.go")
	if err != nil {
		t.Fatalf("读取 Darwin 桩文件失败: %v", err)
	}
	text := string(src)

	// 抓出每个“返回 error 的函数”的函数体
	re := regexp.MustCompile(`(?s)func (\w+)\([^)]*\)[^{]*error \{(.*?)\n\}`)
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		t.Fatal("桩文件里没有解析到任何函数（文件结构变了？）")
	}
	for _, m := range matches {
		name, body := m[1], m[2]
		// 只含 return nil 的桩 = 假成功
		trimmed := strings.TrimSpace(strings.ReplaceAll(body, "\n", " "))
		trimmed = strings.Join(strings.Fields(trimmed), " ")
		if trimmed == "return nil" || trimmed == "" {
			t.Errorf("%s 在 Darwin 上是静默成功（函数体只有 return nil），应返回明确错误", name)
		}
	}
}

// TestDarwinStubbedFeaturesNowImplemented 这几项已经从桩升级为真实实现
// （chflags / screencapture / 拉取式监控），不能再退回"平台不支持"。
func TestDarwinStubbedFeaturesNowImplemented(t *testing.T) {
	src, err := os.ReadFile("commands_darwin_hidpi.go")
	if err != nil {
		t.Fatalf("读取 Darwin 实现文件失败: %v", err)
	}
	text := string(src)
	for _, fn := range []string{"HideFile", "UnhideFile", "TakeScreenshot", "StartScreenMonitor", "StopScreenMonitor", "GetMonitorFrame"} {
		if !regexp.MustCompile(`func ` + fn + `\(`).MatchString(text) {
			t.Errorf("%s 应在 commands_darwin_hidpi.go 中有真实实现", fn)
		}
	}
	// 实现必须真的调用系统工具/API，而不是换了个地方报不支持
	for _, want := range []string{"/usr/bin/chflags", "/usr/sbin/screencapture"} {
		if !strings.Contains(text, want) {
			t.Errorf("Darwin 实现缺少对 %s 的调用", want)
		}
	}
}
