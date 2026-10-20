//go:build windows && amd64

// 伪栈 syscall 的真机往返测试（本文件只在 windows/amd64 上编译）。
//
// 为什么必须真跑一次：桩代码、枢轴、栈传参复制任何一环错了，单测只能证明
// "字节长得对"，证明不了"内核真的按我们摆的栈把第 5/6 个参数读对了"。
// 用 NtAllocateVirtualMemory 做探针正是因为它有 6 个参数 —— 后两个走栈。

package evasion

import (
	"sync"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestSpoofAllocatesWithStackArgs 6 参数 syscall：后两个参数（类型/保护）走伪栈
func TestSpoofAllocatesWithStackArgs(t *testing.T) {
	if err := PrepareSpoof(); err != nil {
		t.Fatalf("装配伪栈失败: %v", err)
	}
	if !SpoofReady() {
		t.Fatal("装配成功后 SpoofReady 仍为 false")
	}
	info, err := resolveSyscallInfo("NtAllocateVirtualMemory")
	if err != nil {
		t.Fatalf("解析 NtAllocateVirtualMemory: %v", err)
	}

	size := uintptr(0x1000)
	var base uintptr
	status, err := syscallSpoofed(uint32(info.ssn), []uintptr{
		uintptr(^uintptr(0)), // NtCurrentProcess
		uintptr(unsafe.Pointer(&base)),
		0,
		uintptr(unsafe.Pointer(&size)),
		uintptr(windows.MEM_COMMIT | windows.MEM_RESERVE), // 第 5 个参数（栈）
		uintptr(windows.PAGE_READWRITE),                   // 第 6 个参数（栈）
	})
	if err != nil {
		t.Fatalf("伪栈 syscall 失败: %v", err)
	}
	if status != 0 {
		t.Fatalf("NtAllocateVirtualMemory 返回 %#x（栈传参没被正确复制？）", status)
	}
	if base == 0 {
		t.Fatal("分配成功但基址为 0")
	}
	defer func() { _ = windows.VirtualFree(base, 0, windows.MEM_RELEASE) }()

	// 保护属性必须真是 PAGE_READWRITE：如果第 6 个参数没送到内核，
	// 内核会用垃圾值或默认值，这里就抓得到。
	var mbi windows.MemoryBasicInformation
	if err := windows.VirtualQuery(base, &mbi, unsafe.Sizeof(mbi)); err != nil {
		t.Fatalf("VirtualQuery: %v", err)
	}
	if mbi.Protect != windows.PAGE_READWRITE {
		t.Fatalf("保护属性为 %#x，期望 PAGE_READWRITE(%#x)", mbi.Protect, windows.PAGE_READWRITE)
	}
	if mbi.State != windows.MEM_COMMIT {
		t.Fatalf("状态为 %#x，期望 MEM_COMMIT", mbi.State)
	}
}

// TestSpoofMatchesUnspoofed 伪栈路径与 trampoline 直连路径必须给出同样结果
func TestSpoofMatchesUnspoofed(t *testing.T) {
	if err := PrepareTrampolines(ModeDirect); err != nil {
		t.Fatalf("装配 trampoline: %v", err)
	}
	info, ok := resolved["NtQueryInformationProcess"]
	if !ok {
		t.Skip("当前系统未解析到 NtQueryInformationProcess")
	}
	type pbi struct {
		Reserved1       uintptr
		PebBaseAddress  uintptr
		Reserved2       [2]uintptr
		UniqueProcessID uintptr
		InheritedFrom   uintptr
	}
	var got, want pbi
	var retLen uint32

	// 1) 伪栈路径
	st, err := syscallSpoofed(uint32(info.ssn), []uintptr{
		uintptr(^uintptr(0)), 0, uintptr(unsafe.Pointer(&got)), unsafe.Sizeof(got), uintptr(unsafe.Pointer(&retLen)),
	})
	if err != nil || st != 0 {
		t.Fatalf("伪栈 NtQueryInformationProcess: status=%#x err=%v", st, err)
	}
	// 2) 直连 trampoline 路径（绕过 Syscall 的伪栈优先逻辑，直接调 trampoline）
	args := []uintptr{trampolines["NtQueryInformationProcess"], uintptr(^uintptr(0)), 0,
		uintptr(unsafe.Pointer(&want)), unsafe.Sizeof(want), uintptr(unsafe.Pointer(&retLen))}
	if s, _, _ := syscall.SyscallN(args[0], args[1:]...); s != 0 {
		t.Fatalf("直连 NtQueryInformationProcess: status=%#x", s)
	}
	if got.PebBaseAddress == 0 || got.PebBaseAddress != want.PebBaseAddress {
		t.Fatalf("两条路径的 PEB 不一致: spoofed=%#x direct=%#x", got.PebBaseAddress, want.PebBaseAddress)
	}
}

// TestSpoofRejectsTooManyArgs 超过 4+4 参数必须报错（不能把参数丢在真实栈上让内核读垃圾）
func TestSpoofRejectsTooManyArgs(t *testing.T) {
	if err := PrepareSpoof(); err != nil {
		t.Fatalf("装配伪栈失败: %v", err)
	}
	if _, err := syscallSpoofed(0x18, make([]uintptr, SpoofMaxArgs+1)); err == nil {
		t.Fatal("参数超限时应报错")
	}
}

// TestSpoofConcurrent 并发调用：slot 与伪栈是单例，靠互斥锁串行化
func TestSpoofConcurrent(t *testing.T) {
	if err := PrepareSpoof(); err != nil {
		t.Fatalf("装配伪栈失败: %v", err)
	}
	info, err := resolveSyscallInfo("NtAllocateVirtualMemory")
	if err != nil {
		t.Skip("未解析到 NtAllocateVirtualMemory")
	}
	var wg sync.WaitGroup
	errs := make(chan string, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			size := uintptr(0x1000)
			var base uintptr
			st, err := syscallSpoofed(uint32(info.ssn), []uintptr{
				uintptr(^uintptr(0)), uintptr(unsafe.Pointer(&base)), 0,
				uintptr(unsafe.Pointer(&size)),
				uintptr(windows.MEM_COMMIT | windows.MEM_RESERVE), uintptr(windows.PAGE_READWRITE),
			})
			if err != nil || st != 0 || base == 0 {
				errs <- "并发伪栈 syscall 失败"
				return
			}
			_ = windows.VirtualFree(base, 0, windows.MEM_RELEASE)
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}
