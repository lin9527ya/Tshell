package config

import (
	"strings"
	"testing"
)

// resetEvasion 每个用例前后清掉运行期覆盖，避免用例互相污染。
func resetEvasion(t *testing.T) {
	t.Helper()
	ClearEvasionOverrides()
	t.Cleanup(func() { ClearEvasionOverrides() })
}

// TestEvasionCapabilitiesShape 能力表本身必须自洽：7 项、键唯一、名字非空。
func TestEvasionCapabilitiesShape(t *testing.T) {
	caps := EvasionCapabilities()
	if len(caps) != 7 {
		t.Fatalf("能力数量 = %d，期望 7", len(caps))
	}
	seen := map[EvasionKey]bool{}
	for _, c := range caps {
		if c.Key == "" || c.Zh == "" || c.En == "" {
			t.Fatalf("能力字段为空: %+v", c)
		}
		if seen[c.Key] {
			t.Fatalf("能力键重复: %s", c.Key)
		}
		seen[c.Key] = true
	}
	for _, want := range []EvasionKey{
		EvasionETW, EvasionAMSI, EvasionModHide, EvasionSpoof,
		EvasionPPID, EvasionBlockDLLs, EvasionUnhook,
	} {
		if !seen[want] {
			t.Fatalf("能力表缺 %s", want)
		}
	}
}

// TestEvasionCompiledDefaultsOff 未打补丁的构建里，7 项编译期值都必须是关。
// 这条是"占位符被意外替换成 1"的棘轮：模板没打补丁时客户端不该自带对抗行为。
func TestEvasionCompiledDefaultsOff(t *testing.T) {
	resetEvasion(t)
	for _, c := range EvasionCapabilities() {
		if EvasionCompiled(c.Key) {
			t.Fatalf("%s 编译期值为开，占位符可能被误替换", c.Key)
		}
	}
}

// TestEvasionOverrideTogglesActive 覆盖优先于编译期值，且状态行能区分来源。
func TestEvasionOverrideTogglesActive(t *testing.T) {
	resetEvasion(t)

	rows := EvasionStatus()
	for _, r := range rows {
		if r.Active || r.Overridden {
			t.Fatalf("%s 初始状态应为关且未覆盖: %+v", r.Key, r)
		}
	}

	if err := SetEvasionOverride(EvasionETW, true); err != nil {
		t.Fatalf("SetEvasionOverride: %v", err)
	}
	if !EvasionEnabled(EvasionETW) {
		t.Fatal("覆盖为开之后 EvasionEnabled 仍为关")
	}
	if EvasionCompiled(EvasionETW) {
		t.Fatal("覆盖不该改变编译期值")
	}

	var found bool
	for _, r := range EvasionStatus() {
		if r.Key != EvasionETW {
			continue
		}
		found = true
		if !r.Active || !r.Overridden {
			t.Fatalf("状态行不正确: %+v", r)
		}
	}
	if !found {
		t.Fatal("状态表里找不到 etw")
	}

	// 显式关掉：与"本来就没开"在状态行里要能区分。
	if err := SetEvasionOverride(EvasionETW, false); err != nil {
		t.Fatalf("SetEvasionOverride off: %v", err)
	}
	for _, r := range EvasionStatus() {
		if r.Key == EvasionETW && (!r.Overridden || r.Active) {
			t.Fatalf("显式关闭的状态行不正确: %+v", r)
		}
	}
}

// TestClearEvasionOverrides 清除后回到编译期值，并返回被改过的键（顺序稳定）。
func TestClearEvasionOverrides(t *testing.T) {
	resetEvasion(t)
	_ = SetEvasionOverride(EvasionPPID, true)
	_ = SetEvasionOverride(EvasionBlockDLLs, true)
	_ = SetEvasionOverride(EvasionPPID, false) // 重复键不应重复记录

	keys := ClearEvasionOverrides()
	if len(keys) != 2 || keys[0] != EvasionPPID || keys[1] != EvasionBlockDLLs {
		t.Fatalf("被清理的键 = %v，期望 [ppid blockdlls]", keys)
	}
	if EvasionEnabled(EvasionPPID) {
		t.Fatal("清理覆盖后仍为开")
	}
	if again := ClearEvasionOverrides(); len(again) != 0 {
		t.Fatalf("二次清理应返回空: %v", again)
	}
}

// TestSetEvasionOverrideRejectsUnknown 未知键必须报错，不能静默塞进表里。
func TestSetEvasionOverrideRejectsUnknown(t *testing.T) {
	resetEvasion(t)
	if err := SetEvasionOverride("nope", true); err == nil {
		t.Fatal("未知能力应报错")
	}
	if r := EvasionImpossibleReason("nope"); !strings.Contains(r, "未知能力") {
		t.Fatalf("未知能力原因 = %q", r)
	}
}

// TestEvasionImpossibleReasonSpoofNeedsDirect 伪栈只在 direct 模式下可能生效；
// 未打补丁的构建 syscallMode 是 none，所以必须给出明确原因。
func TestEvasionImpossibleReasonSpoofNeedsDirect(t *testing.T) {
	resetEvasion(t)
	if got := SyscallMode(); got != "none" {
		t.Skipf("构建的 syscallMode=%s，不适用本用例", got)
	}
	reason := EvasionImpossibleReason(EvasionSpoof)
	if !strings.Contains(reason, "syscallMode=direct") {
		t.Fatalf("伪栈不可用原因 = %q，应说明需要 direct", reason)
	}
	if reason := EvasionImpossibleReason(EvasionPPID); reason != "" {
		t.Fatalf("ppid 无前置条件，原因应为空，得到 %q", reason)
	}
}

// TestEvasionStatusStartupNote 启动期能力要带"关掉会发生什么"的说明，避免操作员误解。
func TestEvasionStatusStartupNote(t *testing.T) {
	resetEvasion(t)
	for _, r := range EvasionStatus() {
		switch r.Key {
		case EvasionETW, EvasionAMSI, EvasionUnhook:
			if !strings.Contains(r.Note, "启动期") {
				t.Fatalf("%s 缺启动期说明: %q", r.Key, r.Note)
			}
		default:
			if r.Note != "" {
				t.Fatalf("%s 不该有启动期说明: %q", r.Key, r.Note)
			}
		}
	}
}

// TestHostEvasionCompiledAny 全关时网关必须为 false（默认不带对抗行为出网）。
func TestHostEvasionCompiledAny(t *testing.T) {
	resetEvasion(t)
	if hostEvasionCompiledAny() {
		t.Fatal("全关时 hostEvasionCompiledAny 应为 false")
	}
	_ = SetEvasionOverride(EvasionAMSI, true)
	if !hostEvasionCompiledAny() {
		t.Fatal("打开 AMSI 后 hostEvasionCompiledAny 应为 true")
	}
}
