//go:build windows && amd64

// 真实 ntdll 上的解析回归：P2 之前 ParseSyscallStub 写死 syscall 偏移 8，
// 在 Windows 11 24H2 上所有系统调用都解析失败，PrepareTrampolines 返回错误后
// 调用方静默退化成普通 kernel32 调用 —— 没有任何运行期症状，只有实机测试能抓到。
// 这组用例就是把「名单里的每一个都必须解析成功」钉死。

package evasion

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestPrepareTrampolinesResolvesEveryName 实机上名单里每一个 NtXxx 都必须解析成功
func TestPrepareTrampolinesResolvesEveryName(t *testing.T) {
	if err := PrepareTrampolines(ModeDirect); err != nil {
		t.Fatalf("PrepareTrampolines(direct) 失败: %v", err)
	}
	var missing []string
	for _, name := range syscallNames {
		if !HasTrampoline(name) {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("以下系统调用未解析到（会被静默回退成 kernel32 调用）: %v", missing)
	}
	// SSN 必须是合理的非零值，且不同函数不该撞号
	seen := map[uintptr]string{}
	for _, name := range syscallNames {
		info, ok := resolved[name]
		if !ok || info.ssn == 0 {
			t.Errorf("%s 的 SSN 无效: %+v", name, info)
			continue
		}
		if prev, dup := seen[info.ssn]; dup {
			t.Errorf("%s 与 %s 的 SSN 相同（0x%x），解析有问题", name, prev, info.ssn)
		}
		seen[info.ssn] = name
		if info.syscallAddr == 0 {
			t.Errorf("%s 的 syscall 指令地址为 0", name)
		}
	}
}

// TestIndirectModeAlsoResolves indirect 用同一个解析器，但拦的是 ntdll 内的 syscall 指令
func TestIndirectModeAlsoResolves(t *testing.T) {
	if err := PrepareTrampolines(ModeIndirect); err != nil {
		t.Fatalf("PrepareTrampolines(indirect) 失败: %v", err)
	}
	for _, name := range syscallNames {
		if !HasTrampoline(name) {
			t.Fatalf("%s 在 indirect 模式下未解析到", name)
		}
	}
	// 收尾：把模式复位，避免影响同包其它用例（它们默认按 direct 断言）
	if err := PrepareTrampolines(ModeDirect); err != nil {
		t.Fatalf("复位 direct 失败: %v", err)
	}
}

// TestSyscallReachesKernel 走一次完整链路：解析 → trampoline → 内核。
// 用 NtQueryInformationProcess 读本进程 PEB，成功即证明 syscall 真的进了内核
// （而不是被 ntdll hook 拦下后假装成功）。
func TestSyscallReachesKernel(t *testing.T) {
	if err := PrepareTrampolines(ModeDirect); err != nil {
		t.Fatalf("PrepareTrampolines(direct) 失败: %v", err)
	}
	if !HasTrampoline("NtQueryInformationProcess") {
		t.Skip("当前环境未解析到 NtQueryInformationProcess")
	}
	h, err := windows.GetCurrentProcess()
	if err != nil {
		t.Fatalf("取当前进程句柄: %v", err)
	}
	var pbi [6]uintptr
	status, err := Syscall("NtQueryInformationProcess", uintptr(h), 0,
		uintptr(unsafe.Pointer(&pbi[0])), 48, 0)
	if err != nil {
		t.Fatalf("Syscall 调用失败: %v", err)
	}
	if status != 0 {
		t.Fatalf("NTSTATUS=0x%x，期望 0（成功）", status)
	}
	if pbi[1] == 0 {
		t.Fatal("PEB 地址为 0，说明返回值没被正确读出")
	}
}
