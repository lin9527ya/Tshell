package config

import (
	"strings"
	"testing"
)

// TestApplyOpsecUpdatesSetsFields 合法取值逐字段落到结构体上（含大小写/空白归一）。
func TestApplyOpsecUpdatesSetsFields(t *testing.T) {
	var o OpsecConfig
	err := applyOpsecUpdates(&o, map[string]interface{}{
		"etw_patch":   "1",
		"amsi_bypass": " 0 ",
		"auto_delete": "1",
		"quiet_mode":  "ONWARDTODO", // 占位：下面会被判非法，故先改成合法值
	})
	if err == nil {
		t.Fatal("非法 quiet_mode 应报错")
	}

	var p OpsecConfig
	if err := applyOpsecUpdates(&p, map[string]interface{}{
		"etw_patch":       "1",
		"amsi_bypass":     " 0 ",
		"auto_delete":     "1",
		"quiet_mode":      "0",
		"sleep_mask_hide": "1",
		"stack_spoof":     "1",
		"ppid_spoof":      "1",
		"block_dlls":      "1",
		"unhook_on_start": "0",
		"syscall_mode":    "DIRECT",
		"inject_tech":     " APC ",
	}); err != nil {
		t.Fatalf("applyOpsecUpdates err: %v", err)
	}
	if p.ETWPatch != "1" || p.AmsiBypass != "0" || p.AutoDelete != "1" || p.QuietMode != "0" {
		t.Fatalf("基础开关归一不正确: %+v", p)
	}
	if p.SleepMaskHide != "1" || p.StackSpoof != "1" || p.PpidSpoof != "1" || p.BlockDLLs != "1" || p.UnhookOnStart != "0" {
		t.Fatalf("P4 开关归一不正确: %+v", p)
	}
	if p.SyscallMode != "direct" || p.InjectTech != "apc" {
		t.Fatalf("枚举归一不正确: %+v", p)
	}
}

// TestApplyOpsecUpdatesRejectsInvalid 非法取值一律报错并带字段名。
//
// 配置里写错一个值，若静默忽略，运维会以为"团队默认已生效"，
// 而实际生成载荷走的是内置默认——这类静默失效正是 P4 要消灭的。
func TestApplyOpsecUpdatesRejectsInvalid(t *testing.T) {
	cases := []struct {
		field string
		val   interface{}
		want  string
	}{
		{"etw_patch", "yes", "opsec.etw_patch"},
		{"block_dlls", "2", "opsec.block_dlls"},
		{"syscall_mode", "syscall", "opsec.syscall_mode"},
		{"inject_tech", "thread", "opsec.inject_tech"},
		{"ppid_spoof", 1, "opsec.ppid_spoof"},
		{"inject_tech", true, "opsec.inject_tech"},
	}
	for _, c := range cases {
		var o OpsecConfig
		err := applyOpsecUpdates(&o, map[string]interface{}{c.field: c.val})
		if err == nil {
			t.Fatalf("%s=%v 应报错", c.field, c.val)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("错误文案 %q 未点名 %q", err.Error(), c.want)
		}
	}
}

// TestApplyOpsecUpdatesIgnoresUnknown 未知字段忽略（向前兼容：老服务端配置里多一个键
// 不该让整个保存失败）。
func TestApplyOpsecUpdatesIgnoresUnknown(t *testing.T) {
	var o OpsecConfig
	if err := applyOpsecUpdates(&o, map[string]interface{}{
		"future_switch": "1",
		"etw_patch":     "1",
	}); err != nil {
		t.Fatalf("未知字段不该报错: %v", err)
	}
	if o.ETWPatch != "1" {
		t.Fatalf("已知字段未生效: %+v", o)
	}

	// 空值表示"取消配置"，回到内置默认。
	if err := applyOpsecUpdates(&o, map[string]interface{}{"etw_patch": "", "syscall_mode": ""}); err != nil {
		t.Fatalf("空值应允许: %v", err)
	}
	if o.ETWPatch != "" || o.SyscallMode != "" {
		t.Fatalf("空值未清空配置: %+v", o)
	}
}

// TestApplyConfigUpdatesRoutesOpsec applyConfigUpdates 的 opsec 分支确实接上了
// （没接上时保存配置返回成功但字段不变——最难查的一类静默失效）。
func TestApplyConfigUpdatesRoutesOpsec(t *testing.T) {
	var cfg Config
	err := applyConfigUpdates(&cfg, map[string]interface{}{
		"opsec": map[string]interface{}{
			"stack_spoof":     "1",
			"syscall_mode":    "direct",
			"unhook_on_start": "1",
		},
	})
	if err != nil {
		t.Fatalf("applyConfigUpdates err: %v", err)
	}
	if cfg.Opsec.StackSpoof != "1" || cfg.Opsec.SyscallMode != "direct" || cfg.Opsec.UnhookOnStart != "1" {
		t.Fatalf("opsec 段未写入: %+v", cfg.Opsec)
	}
}
