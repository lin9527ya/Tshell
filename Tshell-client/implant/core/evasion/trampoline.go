package evasion

import (
	"encoding/binary"
	"fmt"
)

// Trampoline 机器码生成（平台无关，便于在非 Windows 上做字节级校验）。
//
// 生成两段代码：
//   direct:   4c 8b d1 | b8 <ssn:imm32> | 0f 05 | c3
//             mov r10, rcx; mov eax, ssn; syscall; ret
//   indirect: 4c 8b d1 | b8 <ssn:imm32> | 48 b8 <addr:imm64> | ff e0
//             mov r10, rcx; mov eax, ssn; mov rax, <ntdll syscall 指令地址>; jmp rax
//
// 为什么 indirect 用 `mov rax, imm64; jmp rax` 而不是 `jmp rel32`：
// rel32 只有 ±2GB 射程，而 trampoline 是我们自己 VirtualAlloc 出来的地址，
// 与 ntdll 的 syscall 指令之间可能隔着远超 2GB（Windows 的 ASLR 会这么放）。
// 一旦超出，rel32 溢出成垃圾地址 —— 表现为随机崩溃，且只在部分机器上复现。
// mov rax/jmp rax 是绝对跳转，任何距离都成立，代价只是多 7 字节。

// TrampolineMode 生成模式
const (
	ModeDirect   = "direct"
	ModeIndirect = "indirect"
)

// NtdllStubPrefix ntdll Nt* 导出函数体前缀：mov r10, rcx
var NtdllStubPrefix = []byte{0x4C, 0x8B, 0xD1}

// ParseSyscallStub 从 ntdll 导出函数体前 64 字节解析出 SSN 与 syscall 指令地址。
// addr 为该导出函数的运行时地址；返回 (ssn, syscall 指令地址, 错误)。
//
// 函数体布局（x64）：
//
//	4c 8b d1            mov r10, rcx
//	b8 <imm32>          mov eax, ssn
//	0f 05               syscall
//	c3                  ret
func ParseSyscallStub(body []byte, addr uintptr) (uint32, uintptr, error) {
	if len(body) < 10 {
		return 0, 0, fmt.Errorf("函数体过短: %d 字节", len(body))
	}
	if body[0] != NtdllStubPrefix[0] || body[1] != NtdllStubPrefix[1] || body[2] != NtdllStubPrefix[2] {
		return 0, 0, fmt.Errorf("函数体前缀异常（可能已被 hook）: %x %x %x", body[0], body[1], body[2])
	}
	if body[3] != 0xB8 {
		return 0, 0, fmt.Errorf("未找到 mov eax,imm（可能已被 hook）")
	}
	ssn := binary.LittleEndian.Uint32(body[4:8])
	// syscall 指令在 mov eax 之后（相对函数入口偏移 8）
	if body[8] != 0x0F || body[9] != 0x05 {
		return 0, 0, fmt.Errorf("未找到 syscall 指令")
	}
	return ssn, addr + 8, nil
}

// BuildTrampoline 生成 trampoline 机器码。
// syscallAddr 仅 indirect 模式使用（ntdll 内 syscall 指令的运行时地址）。
func BuildTrampoline(mode string, ssn uint32, syscallAddr uintptr) ([]byte, error) {
	ssnBytes := []byte{byte(ssn), byte(ssn >> 8), byte(ssn >> 16), byte(ssn >> 24)}
	switch mode {
	case ModeDirect:
		code := make([]byte, 0, 12)
		code = append(code, NtdllStubPrefix...)
		code = append(code, 0xB8)
		code = append(code, ssnBytes...)
		code = append(code, 0x0F, 0x05, 0xC3) // syscall; ret
		return code, nil
	case ModeIndirect:
		if syscallAddr == 0 {
			return nil, fmt.Errorf("indirect 模式需要 ntdll syscall 指令地址")
		}
		code := make([]byte, 0, 20)
		code = append(code, NtdllStubPrefix...)
		code = append(code, 0xB8)
		code = append(code, ssnBytes...)
		code = append(code, 0x48, 0xB8) // mov rax, imm64
		addrBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(addrBytes, uint64(syscallAddr))
		code = append(code, addrBytes...)
		code = append(code, 0xFF, 0xE0) // jmp rax
		return code, nil
	default:
		return nil, fmt.Errorf("未知的 trampoline 模式: %q", mode)
	}
}
