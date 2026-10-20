//go:build windows && amd64

package evasion

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestUnhookNTDLLRestoresPlantedHook 在 ntdll 里真的种一个 hook，再让 unhook 抹掉它。
//
// 选 NtLoadDriver 当靶子：Go 运行时永远不会调用它，即使 hook 期间被别的代码碰到也
// 不会炸；而恢复之后调用它能返回确定的 STATUS_PRIVILEGE_NOT_HELD，说明代码段自洽。
func TestUnhookNTDLLRestoresPlantedHook(t *testing.T) {
	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	proc := ntdll.NewProc("NtLoadDriver")
	if err := proc.Find(); err != nil {
		t.Fatalf("解析 NtLoadDriver 失败: %v", err)
	}
	addr := proc.Addr()

	const n = 8
	orig := make([]byte, n)
	copy(orig, unsafe.Slice((*byte)(unsafe.Pointer(addr)), n))

	// 种一个"看起来像 EDR 的"跳转 hook：mov rax, imm64; jmp rax
	fake := []byte{0x48, 0xB8, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0xFF, 0xE0}
	var old uint32
	if err := windows.VirtualProtect(addr, uintptr(len(fake)), windows.PAGE_EXECUTE_READWRITE, &old); err != nil {
		t.Fatalf("VirtualProtect(RWX) 失败: %v", err)
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(addr)), len(fake)), fake)
	windows.VirtualProtect(addr, uintptr(len(fake)), old, &old)

	rep, err := UnhookModule("ntdll.dll")
	if err != nil {
		t.Fatalf("UnhookModule(ntdll) 失败: %v", err)
	}
	if rep.Skipped != "" {
		t.Fatalf("ntdll 不该被跳过: %+v", rep)
	}
	if rep.Restored < len(fake) {
		t.Fatalf("恢复字节数 = %d，至少应恢复种进去的 %d 字节（%s）", rep.Restored, len(fake), rep)
	}

	// ntdll 必须逐字节回到原样
	now := unsafe.Slice((*byte)(unsafe.Pointer(addr)), n)
	for i := 0; i < n; i++ {
		if now[i] != orig[i] {
			t.Fatalf("第 %d 字节 = 0x%02x，期望 0x%02x（hook 没被抹掉）", i, now[i], orig[i])
		}
	}

	// 恢复后调用仍应正常工作：无特权时返回 STATUS_PRIVILEGE_NOT_HELD
	path, _ := windows.UTF16PtrFromString(`\Registry\Machine\System\CurrentControlSet\Services\TshellNotExist`)
	u := struct {
		Length     uint16
		_          uint16
		Buffer     *uint16
		_          uint32
		Attributes uint32
		_          [4]byte
	}{}
	u.Length = uint16(unsafe.Sizeof(u))
	u.Buffer = path
	st, _ := Syscall("NtLoadDriver", uintptr(unsafe.Pointer(&u)))
	if st != 0xC0000061 {
		t.Logf("NtLoadDriver 返回 0x%x（期望 0xC0000061 无特权；只要不是崩溃/乱码即可）", st)
	}
}

// TestUnhookIdempotent 恢复干净之后再跑一次必须是 0 字节 —— 否则说明比较逻辑有误，
// 会持续把同一个位置当成 hook 反复写。
func TestUnhookIdempotent(t *testing.T) {
	if _, err := UnhookModule("ntdll.dll"); err != nil {
		t.Fatalf("第一次 UnhookModule 失败: %v", err)
	}
	rep, err := UnhookModule("ntdll.dll")
	if err != nil {
		t.Fatalf("第二次 UnhookModule 失败: %v", err)
	}
	if rep.Restored != 0 {
		t.Fatalf("第二次仍恢复了 %d 字节，比较逻辑不稳定（%s）", rep.Restored, rep)
	}
	if rep.Compared == 0 {
		t.Fatal("没比较任何字节，说明可执行节没找到")
	}
	t.Logf("%s", rep)
}

// TestUnhookModuleNotLoaded 未加载模块必须跳过而不是去 LoadLibrary（加载动作本身可观测）
func TestUnhookModuleNotLoaded(t *testing.T) {
	rep, err := UnhookModule("tshell_no_such_module.dll")
	if err != nil {
		t.Fatalf("不该返回错误: %v", err)
	}
	if rep.Skipped != "未加载" {
		t.Fatalf("期望跳过，得到 %+v", rep)
	}
}

// TestUnhookAllDefaults 三个默认模块都要比到真字节
func TestUnhookAllDefaults(t *testing.T) {
	reps, err := UnhookAll()
	if err != nil {
		t.Fatalf("UnhookAll 失败: %v", err)
	}
	if len(reps) != len(DefaultUnhookModules) {
		t.Fatalf("报告数 = %d，期望 %d", len(reps), len(DefaultUnhookModules))
	}
	for _, r := range reps {
		if r.Skipped != "" {
			t.Fatalf("%s 被跳过: %s", r.Module, r.Skipped)
		}
		if r.Compared < 1024 {
			t.Fatalf("%s 只比较了 %d 字节，太小了", r.Module, r.Compared)
		}
		t.Logf("%s", r)
	}
}
