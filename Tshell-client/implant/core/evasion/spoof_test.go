package evasion

// 伪栈桩是直接写进可执行内存的机器码：错一个字节就是随机崩溃，且只在部分机器上
// 复现。因此这里做字节级校验（平台无关，Linux 上也能跑）。

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestBuildSpoofStubBytes 桩字节必须精确匹配
func TestBuildSpoofStubBytes(t *testing.T) {
	const slot = 0x7FFE00001234
	code := BuildSpoofStub(0x18, slot)
	want := []byte{
		0x53,             // push rbx
		0x48, 0x8B, 0xDC, // mov rbx, rsp
		0x48, 0xA1, // mov rax, [imm64]
		0x34, 0x12, 0x00, 0x00, 0xFE, 0x7F, 0x00, 0x00, // slot 小端
		0x48, 0x89, 0xC4, // mov rsp, rax
		0x4C, 0x8B, 0xD1, // mov r10, rcx
		0xB8, 0x18, 0x00, 0x00, 0x00, // mov eax, 0x18
		0x0F, 0x05, // syscall
		0x48, 0x89, 0xDC, // mov rsp, rbx
		0x5B, // pop rbx
		0xC3, // ret
	}
	if !bytes.Equal(code, want) {
		t.Fatalf("伪栈桩字节不符:\n got % x\nwant % x", code, want)
	}
	if len(code) != SpoofStubSize {
		t.Fatalf("桩长度 %d 与 SpoofStubSize=%d 不一致", len(code), SpoofStubSize)
	}
}

// TestBuildSpoofStubPivotsBeforeSyscall 枢轴必须在 syscall 之前、恢复必须在之后。
// 顺序写反了不会编译报错，只会在真机上表现为栈错乱 —— 这里按指令序列钉死。
func TestBuildSpoofStubPivotsBeforeSyscall(t *testing.T) {
	code := BuildSpoofStub(0x55, 0x1000)
	push := bytes.Index(code, []byte{0x53})
	movRBX := bytes.Index(code, []byte{0x48, 0x8B, 0xDC})
	movRAX := bytes.Index(code, []byte{0x48, 0xA1})
	pivot := bytes.Index(code, []byte{0x48, 0x89, 0xC4})
	sysc := bytes.Index(code, []byte{0x0F, 0x05})
	restore := bytes.LastIndex(code, []byte{0x48, 0x89, 0xDC})
	pop := bytes.LastIndex(code, []byte{0x5B})
	ret := bytes.LastIndex(code, []byte{0xC3})
	order := []struct {
		name string
		idx  int
	}{
		{"push rbx", push}, {"mov rbx,rsp", movRBX}, {"mov rax,[slot]", movRAX},
		{"mov rsp,rax（枢轴）", pivot}, {"syscall", sysc},
		{"mov rsp,rbx（恢复）", restore}, {"pop rbx", pop}, {"ret", ret},
	}
	for i := 1; i < len(order); i++ {
		if order[i-1].idx < 0 || order[i].idx < 0 {
			t.Fatalf("指令缺失: %s=%d %s=%d", order[i-1].name, order[i-1].idx, order[i].name, order[i].idx)
		}
		if order[i].idx <= order[i-1].idx {
			t.Fatalf("指令顺序错误: %s(@%d) 必须在 %s(@%d) 之后",
				order[i].name, order[i].idx, order[i-1].name, order[i-1].idx)
		}
	}
}

// TestBuildSpoofStubSSNEncoding SSN 按小端写入 imm32
func TestBuildSpoofStubSSNEncoding(t *testing.T) {
	for _, ssn := range []uint32{0, 1, 0x18, 0xFF, 0x1234} {
		code := BuildSpoofStub(ssn, 0x1000)
		off := bytes.Index(code, []byte{0xB8})
		if off < 0 {
			t.Fatalf("ssn=%#x: 未找到 mov eax,imm32", ssn)
		}
		got := uint32(code[off+1]) | uint32(code[off+2])<<8 | uint32(code[off+3])<<16 | uint32(code[off+4])<<24
		if got != ssn {
			t.Fatalf("ssn=%#x 编码成 %#x", ssn, got)
		}
	}
}

// TestBuildSpoofFrameLayout 伪栈帧布局：返回地址 / shadow space / 栈传参 / 帧链 / 终止
func TestBuildSpoofFrameLayout(t *testing.T) {
	const btit = 0x7FFE11111114 // BaseThreadInitThunk+0x14
	const ruts = 0x7FFE22222221 // RtlUserThreadStart+0x21
	frame, err := BuildSpoofFrame(btit, ruts, []uintptr{0x11, 0x22})
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	if len(frame)%16 != 0 {
		t.Fatalf("帧长度 %d 不是 16 的倍数（枢轴后栈未对齐）", len(frame))
	}
	if got := le64At(t, frame, 0); got != btit {
		t.Fatalf("[0x00] 应为 BaseThreadInitThunk+0x14（syscall 返回地址），实为 %#x", got)
	}
	for off := 8; off < SpoofArgOffset; off += 8 {
		if got := le64At(t, frame, off); got != 0 {
			t.Fatalf("[0x%02x] shadow space 应为 0，实为 %#x", off, got)
		}
	}
	if got := le64At(t, frame, SpoofArgOffset); got != 0x11 {
		t.Fatalf("[0x%02x] 第 5 个参数应为 0x11，实为 %#x", SpoofArgOffset, got)
	}
	if got := le64At(t, frame, SpoofArgOffset+8); got != 0x22 {
		t.Fatalf("[0x%02x] 第 6 个参数应为 0x22，实为 %#x", SpoofArgOffset+8, got)
	}
	chain := SpoofArgOffset + 16
	if got := le64At(t, frame, chain); got != ruts {
		t.Fatalf("[0x%02x] 帧链应为 RtlUserThreadStart+0x21，实为 %#x", chain, got)
	}
	if got := le64At(t, frame, chain+8); got != 0 {
		t.Fatalf("[0x%02x] 帧链终止应为 0，实为 %#x", chain+8, got)
	}
}

// TestBuildSpoofFrameNoStackArgs 无栈传参时也必须是有序帧（shadow space 之后直接接帧链）
func TestBuildSpoofFrameNoStackArgs(t *testing.T) {
	frame, err := BuildSpoofFrame(0x1000, 0x2000, nil)
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	if got := le64At(t, frame, SpoofArgOffset); got != 0x2000 {
		t.Fatalf("无栈传参时 [0x%02x] 应直接是帧链地址，实为 %#x", SpoofArgOffset, got)
	}
}

// TestBuildSpoofFrameRejects Bad 输入必须报错而不是生成坏帧
func TestBuildSpoofFrameRejects(t *testing.T) {
	if _, err := BuildSpoofFrame(0, 0x2000, nil); err == nil {
		t.Fatal("返回点地址为 0 时应报错")
	}
	if _, err := BuildSpoofFrame(0x1000, 0, nil); err == nil {
		t.Fatal("帧链地址为 0 时应报错")
	}
	if _, err := BuildSpoofFrame(0x1000, 0x2000, make([]uintptr, SpoofMaxStackArgs+1)); err == nil {
		t.Fatal("栈传参超限时应报错")
	}
}

// TestSqlSpoofRatchet 源码级棘轮：direct 模式必须尝试装配伪栈，indirect 必须不装。
// 这两条是设计取舍（见 spoof.go 顶部注释），不写死就容易被"顺手统一一下"改坏。
func TestSqlSpoofRatchet(t *testing.T) {
	src, err := os.ReadFile("syscall.go")
	if err != nil {
		t.Fatalf("读取 syscall.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "PrepareSpoof()") {
		t.Fatal("syscall.go 未调用 PrepareSpoof()：direct 模式的栈伪造没有接上")
	}
	if !strings.Contains(text, "mode == ModeDirect") {
		t.Fatal("syscall.go 未按 direct 模式门控伪栈装配")
	}
	// indirect 分支不得装配伪栈：它靠 jmp 进 ntdll 再由 ntdll 的 ret 返回，
	// 枢轴会把返回地址落到伪栈上，反而暴露"返回地址不在模块内"。
	if strings.Contains(text, "mode == ModeIndirect") && strings.Contains(text, "PrepareSpoof") {
		t.Fatal("indirect 模式不应装配伪栈")
	}
}

// le64At 读取 frame 中 off 处的 8 字节小端整数
func le64At(t *testing.T, frame []byte, off int) uintptr {
	t.Helper()
	if off+8 > len(frame) {
		t.Fatalf("读越界: off=%d len=%d", off, len(frame))
	}
	var v uintptr
	for i := 0; i < 8; i++ {
		v |= uintptr(frame[off+i]) << (8 * uint(i))
	}
	return v
}
