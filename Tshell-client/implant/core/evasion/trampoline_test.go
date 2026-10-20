package evasion

import (
	"bytes"
	"os"
	"testing"
)

// readFileForTest 读取同包源文件（源码级守卫用）
func readFileForTest(name string) ([]byte, error) { return os.ReadFile(name) }

// trampoline 是直接写进可执行内存的机器码：错一个字节就是随机崩溃，
// 而且只在部分机器上复现。因此这里做字节级校验（平台无关，Linux 上也能跑）。

// TestBuildTrampolineDirect direct 模式：mov r10,rcx | mov eax,ssn | syscall | ret
func TestBuildTrampolineDirect(t *testing.T) {
	code, err := BuildTrampoline(ModeDirect, 0x18, 0)
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	want := []byte{
		0x4C, 0x8B, 0xD1, // mov r10, rcx
		0xB8, 0x18, 0x00, 0x00, 0x00, // mov eax, 0x18
		0x0F, 0x05, // syscall
		0xC3, // ret
	}
	if !bytes.Equal(code, want) {
		t.Fatalf("direct trampoline 字节不符:\n got % x\nwant % x", code, want)
	}
}

// TestBuildTrampolineIndirectUsesAbsoluteJump indirect 模式必须用绝对跳转。
// 原来的 `jmp rel32` 只有 ±2GB 射程：trampoline 是我们自己分配的地址，
// 与 ntdll 的 syscall 指令可能隔着远超 2GB（ASLR），rel32 会溢出成垃圾地址 ——
// 表现为随机崩溃，且只在部分机器上复现。
func TestBuildTrampolineIndirectUsesAbsoluteJump(t *testing.T) {
	const syscallAddr = 0x7FFE12345678 // 典型 ntdll 地址
	code, err := BuildTrampoline(ModeIndirect, 0x55, syscallAddr)
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	want := []byte{
		0x4C, 0x8B, 0xD1,
		0xB8, 0x55, 0x00, 0x00, 0x00,
		0x48, 0xB8, // mov rax, imm64
		0x78, 0x56, 0x34, 0x12, 0xFE, 0x7F, 0x00, 0x00,
		0xFF, 0xE0, // jmp rax
	}
	if !bytes.Equal(code, want) {
		t.Fatalf("indirect trampoline 字节不符:\n got % x\nwant % x", code, want)
	}
	// 不得再出现 rel32 形式的 jmp（E9）
	if bytes.Contains(code, []byte{0xE9}) {
		t.Fatal("indirect 模式不应使用 rel32 跳转（射程只有 ±2GB）")
	}
}

// TestBuildTrampolineSSNEncoding SSN 按小端写入 imm32，且不影响后续指令位置
func TestBuildTrampolineSSNEncoding(t *testing.T) {
	for _, ssn := range []uint32{0, 1, 0x18, 0xFF, 0x1234, 0xFFFFFF} {
		code, err := BuildTrampoline(ModeDirect, ssn, 0)
		if err != nil {
			t.Fatalf("ssn=%#x 生成失败: %v", ssn, err)
		}
		got := uint32(code[4]) | uint32(code[5])<<8 | uint32(code[6])<<16 | uint32(code[7])<<24
		if got != ssn {
			t.Errorf("ssn 编码不符: got %#x want %#x", got, ssn)
		}
		// syscall 指令必须紧跟在 mov eax 之后（偏移 8），否则 trampoline 行为完全不同
		if code[8] != 0x0F || code[9] != 0x05 {
			t.Errorf("ssn=%#x 时 syscall 指令位置不对: % x", ssn, code[8:10])
		}
	}
}

func TestBuildTrampolineRejectsBadInput(t *testing.T) {
	if _, err := BuildTrampoline("magic", 1, 0); err == nil {
		t.Fatal("未知模式应报错")
	}
	if _, err := BuildTrampoline(ModeIndirect, 1, 0); err == nil {
		t.Fatal("indirect 缺 syscall 地址应报错")
	}
}

// TestParseSyscallStub 从 ntdll 函数体解析 SSN 与 syscall 指令地址
func TestParseSyscallStub(t *testing.T) {
	const addr = uintptr(0x7FFE00001000)
	body := []byte{0x4C, 0x8B, 0xD1, 0xB8, 0x2A, 0x00, 0x00, 0x00, 0x0F, 0x05, 0xC3, 0x00}
	ssn, syscallAddr, err := ParseSyscallStub(body, addr)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if ssn != 0x2A {
		t.Fatalf("ssn = %#x，期望 0x2A", ssn)
	}
	if syscallAddr != addr+8 {
		t.Fatalf("syscall 指令地址 = %#x，期望 %#x", syscallAddr, addr+8)
	}
}

// TestParseSyscallStubWin11_24H2 24H2 起 ntdll 函数体多了 SharedUserData 判断，
// syscall 指令不在固定偏移 8。写死偏移会让所有系统调用静默解析失败（P2 实机发现）。
func TestParseSyscallStubWin11_24H2(t *testing.T) {
	const addr = uintptr(0x7FFE00001000)
	// 实测 NtClose 函数体（Windows 11 24H2）
	body := []byte{
		0x4C, 0x8B, 0xD1, // mov r10, rcx
		0xB8, 0x0F, 0x00, 0x00, 0x00, // mov eax, 0xf
		0xF6, 0x04, 0x25, 0x08, 0x03, 0xFE, 0x7F, 0x01, // test byte ptr ds:0x7ffe0308, 1
		0x75, 0x03, // jne +3
		0x0F, 0x05, // syscall  ← 偏移 0x12
		0xC3,       // ret
		0xCD, 0x2E, // int 2eh（老式路径）
		0xC3, // ret
	}
	ssn, syscallAddr, err := ParseSyscallStub(body, addr)
	if err != nil {
		t.Fatalf("24H2 布局应能解析: %v", err)
	}
	if ssn != 0x0F {
		t.Fatalf("ssn = %#x，期望 0x0F", ssn)
	}
	if syscallAddr != addr+0x12 {
		t.Fatalf("syscall 指令地址 = %#x，期望 %#x", syscallAddr, addr+0x12)
	}
}

// TestParseSyscallStubRejectsHooked 被 hook 的函数体必须被识别（前缀被改成 jmp）
func TestParseSyscallStubRejectsHooked(t *testing.T) {
	const addr = uintptr(0x7FFE00001000)
	cases := map[string][]byte{
		"前缀被改（jmp 到检测代码）": {0xE9, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		"没有 mov eax,imm":  {0x4C, 0x8B, 0xD1, 0x90, 0x90, 0x90, 0x90, 0x90, 0x0F, 0x05, 0xC3, 0x00},
		"没有 syscall 指令":   {0x4C, 0x8B, 0xD1, 0xB8, 0x01, 0x00, 0x00, 0x00, 0x90, 0x90, 0xC3, 0x00},
		"函数体过短":           {0x4C, 0x8B},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ParseSyscallStub(body, addr); err == nil {
				t.Fatal("应解析失败")
			}
		})
	}
}

// TestSyscallTrampolineMemoryIsWX 源码级守卫：trampoline 内存必须是 RW→RX 两段式，
// 不得出现 PAGE_EXECUTE_READWRITE（RWX 是 EDR 最醒目的特征之一）。
func TestSyscallTrampolineMemoryIsWX(t *testing.T) {
	src, err := readFileForTest("syscall.go")
	if err != nil {
		t.Fatalf("读取 syscall.go 失败: %v", err)
	}
	// 只看实际调用（注释里会解释"原来用的是 RWX"，不该被算作违规）
	if bytes.Contains(src, []byte("windows.PAGE_EXECUTE_READWRITE")) {
		t.Error("trampoline 内存不应使用 windows.PAGE_EXECUTE_READWRITE（应 RW 写入后 VirtualProtect 为 RX）")
	}
	for _, want := range []string{"windows.PAGE_READWRITE", "windows.PAGE_EXECUTE_READ", "windows.VirtualProtect"} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("syscall.go 缺少 %s（W^X 两段式分配）", want)
		}
	}
}
