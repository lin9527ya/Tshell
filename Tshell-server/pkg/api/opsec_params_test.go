package api

import (
	"strings"
	"testing"
)

// TestResolveOpsecBuiltinDefaults 空请求体走内置默认：对抗能力全关，autoDelete 保持 1。
//
// autoDelete 的 "1" 是刻意保留的既有语义（见 builtinOpsecDefaults 注释），
// 这条用例同时是"别顺手把默认值改成 0"的棘轮。
func TestResolveOpsecBuiltinDefaults(t *testing.T) {
	p, err := resolveOpsec(&genServerBody{}, "windows", "amd64")
	if err != nil {
		t.Fatalf("resolveOpsec err: %v", err)
	}
	want := opsecParams{
		ETW:           "0",
		AMSI:          "0",
		SyscallMode:   "none",
		AutoDelete:    "1",
		QuietMode:     "1",
		InjectTech:    "auto",
		SleepMaskHide: "0",
		StackSpoof:    "0",
		PpidSpoof:     "0",
		BlockDLLs:     "0",
		UnhookOnStart: "0",
	}
	if p != want {
		t.Fatalf("内置默认 = %+v\n期望 %+v", p, want)
	}
}

// TestResolveOpsecBodyWins 请求体里的值优先，且别名会被归一。
func TestResolveOpsecBodyWins(t *testing.T) {
	body := &genServerBody{
		ETWPatch:          "true",
		AmsiBypass:        "ON",
		SyscallMode:       "Direct",
		AutoDelete:        "no",
		QuietMode:         "yes",
		InjectTechDefault: "APC",
		SleepMaskHide:     "1",
		PpidSpoof:         "1",
		BlockDLLs:         "1",
	}
	p, err := resolveOpsec(body, "windows", "amd64")
	if err != nil {
		t.Fatalf("resolveOpsec err: %v", err)
	}
	if p.ETW != "1" || p.AMSI != "1" || p.SyscallMode != "direct" {
		t.Fatalf("别名归一不正确: %+v", p)
	}
	if p.AutoDelete != "0" || p.QuietMode != "1" || p.InjectTech != "apc" {
		t.Fatalf("自动删除/安静/注入技术归一不正确: %+v", p)
	}
	if p.SleepMaskHide != "1" || p.PpidSpoof != "1" || p.BlockDLLs != "1" {
		t.Fatalf("新开关未生效: %+v", p)
	}
	if p.StackSpoof != "0" || p.UnhookOnStart != "0" {
		t.Fatalf("未提供的开关应保持默认关: %+v", p)
	}
}

// TestResolveOpsecRejectsInvalidValues 非法取值必须在生成阶段报错并点名字段。
// 这条防的是"写了 true 但客户端按 ==\"1\" 判定，于是静默关闭"。
func TestResolveOpsecRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		body *genServerBody
		want string
	}{
		{"etw", &genServerBody{ETWPatch: "maybe"}, "etwPatch"},
		{"amsi", &genServerBody{AmsiBypass: "2"}, "amsiBypass"},
		{"autodelete", &genServerBody{AutoDelete: "keep"}, "autoDelete"},
		{"quiet", &genServerBody{QuietMode: "quiet"}, "quietMode"},
		{"alias-new", &genServerBody{SleepMaskHide: "on!"}, "sleepMaskHide"},
		{"syscall", &genServerBody{SyscallMode: "dirty"}, "syscallMode"},
		{"inject", &genServerBody{InjectTechDefault: "rwx"}, "injectTech"},
	}
	for _, c := range cases {
		if _, err := resolveOpsec(c.body, "windows", "amd64"); err == nil {
			t.Fatalf("%s: 非法取值应报错", c.name)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: 错误文案 %q 未点名 %q", c.name, err.Error(), c.want)
		}
	}
}

// TestResolveOpsecPlatformWindows386 386 上 syscall/摘链/伪栈/unhook 无实现 → 400；
// PPID/BlockDLLs 在 386 上是支持的（不依赖 amd64 代码）。
func TestResolveOpsecPlatformWindows386(t *testing.T) {
	if _, err := resolveOpsec(&genServerBody{SyscallMode: "direct"}, "windows", "386"); err == nil {
		t.Fatal("386 上的 direct syscall 应报错")
	} else if !strings.Contains(err.Error(), "amd64") {
		t.Fatalf("错误文案应提到 amd64: %v", err)
	}
	for _, body := range []*genServerBody{
		{SleepMaskHide: "1"},
		{StackSpoof: "1", SyscallMode: "direct"},
		{UnhookOnStart: "1"},
	} {
		if _, err := resolveOpsec(body, "windows", "386"); err == nil {
			t.Fatalf("386 上该组合应报错: %+v", body)
		}
	}
	if _, err := resolveOpsec(&genServerBody{PpidSpoof: "1", BlockDLLs: "1"}, "windows", "386"); err != nil {
		t.Fatalf("386 上 PPID/BlockDLLs 应允许: %v", err)
	}
}

// TestResolveOpsecPlatformNonWindows 非 windows 目标上任何 windows-only 开关都报错，
// 而不是生成一个"写着 1 却什么都不做"的载荷。
func TestResolveOpsecPlatformNonWindows(t *testing.T) {
	if _, err := resolveOpsec(&genServerBody{}, "linux", "amd64"); err != nil {
		t.Fatalf("linux 无任何开关时应通过: %v", err)
	}
	for _, body := range []*genServerBody{
		{ETWPatch: "1"},
		{AmsiBypass: "1"},
		{SyscallMode: "direct"},
		{SleepMaskHide: "1"},
		{PpidSpoof: "1"},
		{BlockDLLs: "1"},
		{UnhookOnStart: "1"},
	} {
		_, err := resolveOpsec(body, "linux", "amd64")
		if err == nil {
			t.Fatalf("linux 上该开关应报错: %+v", body)
		}
		if !strings.Contains(err.Error(), "仅 windows") {
			t.Fatalf("错误文案应说明仅 windows: %v", err)
		}
	}
}

// TestResolveOpsecStackSpoofRequiresDirect 伪栈只在 direct 下成立：
// indirect 靠 ntdll 的 ret 返回，布伪栈会把返回地址暴露在模块外。
func TestResolveOpsecStackSpoofRequiresDirect(t *testing.T) {
	if _, err := resolveOpsec(&genServerBody{StackSpoof: "1"}, "windows", "amd64"); err == nil {
		t.Fatal("stackSpoof=1 且 syscallMode=none 应报错")
	} else if !strings.Contains(err.Error(), "syscallMode=direct") {
		t.Fatalf("错误文案应指明需要 direct: %v", err)
	}
	if _, err := resolveOpsec(&genServerBody{StackSpoof: "1", SyscallMode: "indirect"}, "windows", "amd64"); err == nil {
		t.Fatal("stackSpoof=1 且 syscallMode=indirect 应报错")
	}
	if _, err := resolveOpsec(&genServerBody{StackSpoof: "1", SyscallMode: "direct"}, "windows", "amd64"); err != nil {
		t.Fatalf("stackSpoof=1 且 syscallMode=direct 应通过: %v", err)
	}
}

// TestNormalizeOnOffTable 归一函数本身的行为（含大小写与空白）。
func TestNormalizeOnOffTable(t *testing.T) {
	cases := map[string]string{
		"":      "",
		"  ":    "",
		"1":     "1",
		"TRUE":  "1",
		" on ":  "1",
		"Yes":   "1",
		"0":     "0",
		"False": "0",
		"off":   "0",
		"NO":    "0",
		"\t0\n": "0",
	}
	for in, want := range cases {
		got, err := normalizeOnOff("x", in)
		if err != nil {
			t.Fatalf("normalizeOnOff(%q) err: %v", in, err)
		}
		if got != want {
			t.Fatalf("normalizeOnOff(%q) = %q，期望 %q", in, got, want)
		}
	}
	for _, bad := range []string{"2", "onoff", "t", "-1"} {
		if _, err := normalizeOnOff("x", bad); err == nil {
			t.Fatalf("normalizeOnOff(%q) 应报错", bad)
		}
	}
}

// TestNormalizeEnums 枚举归一。
func TestNormalizeEnums(t *testing.T) {
	for _, v := range []string{"none", "DIRECT", " indirect "} {
		if _, err := normalizeSyscallMode(v); err != nil {
			t.Fatalf("normalizeSyscallMode(%q): %v", v, err)
		}
	}
	for _, v := range []string{"auto", "NTC", "apc", "crt"} {
		if _, err := normalizeInjectTech(v); err != nil {
			t.Fatalf("normalizeInjectTech(%q): %v", v, err)
		}
	}
	if _, err := normalizeSyscallMode("syscall"); err == nil {
		t.Fatal("非法 syscallMode 应报错")
	}
	if _, err := normalizeInjectTech("thread"); err == nil {
		t.Fatal("非法 injectTech 应报错")
	}
}
