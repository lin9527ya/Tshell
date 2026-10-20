//go:build windows

package commands

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ===== 假实现 =====
//
// 这一层的测试目标只有一个：把"哪一步用了哪条路、参数是什么、顺序对不对"钉死。
// 这些问题在实机上全都不可见 —— syscall 接线写错时功能照常工作（只是回到 userland
// hook 下面），镂空申请 RWX 时功能也照常工作（只是留下最刺眼的内存特征）。

type memoryOp struct {
	kind    string // open/alloc/write/read/protect/unmap/peb/close
	at      uintptr
	addr    uintptr
	size    uintptr
	protect uint32
	n       uintptr
	data    []byte
}

type fakeMemory struct {
	ops        []memoryOp
	nextAddr   uintptr
	failAlloc  map[uintptr]bool
	shortWrite bool
	failProt   bool
	failRead   bool
}

func newFakeMemory() *fakeMemory { return &fakeMemory{nextAddr: 0x100000} }

func (f *fakeMemory) OpenProcess(pid int, access uint32) (uintptr, error) {
	f.ops = append(f.ops, memoryOp{kind: "open", n: uintptr(pid)})
	return 0x500, nil
}

func (f *fakeMemory) Alloc(h uintptr, at, size uintptr, protect uint32) (uintptr, error) {
	f.ops = append(f.ops, memoryOp{kind: "alloc", at: at, size: size, protect: protect})
	if f.failAlloc[at] {
		return 0, errors.New("分配失败（测试注入）")
	}
	if at != 0 {
		return at, nil
	}
	return f.nextAddr, nil
}

func (f *fakeMemory) Write(h, dst uintptr, data []byte) (uintptr, error) {
	f.ops = append(f.ops, memoryOp{kind: "write", addr: dst, size: uintptr(len(data)), data: append([]byte(nil), data...)})
	if f.shortWrite {
		return uintptr(len(data)) / 2, nil
	}
	return uintptr(len(data)), nil
}

func (f *fakeMemory) Read(h, src uintptr, n int) ([]byte, error) {
	f.ops = append(f.ops, memoryOp{kind: "read", addr: src, size: uintptr(n)})
	if f.failRead {
		return nil, errors.New("读取失败（测试注入）")
	}
	return make([]byte, n), nil
}

func (f *fakeMemory) Protect(h, addr, size uintptr, protect uint32) (uint32, error) {
	f.ops = append(f.ops, memoryOp{kind: "protect", addr: addr, size: size, protect: protect})
	if f.failProt {
		return 0, errors.New("改保护失败（测试注入）")
	}
	return pageReadWrite, nil
}

func (f *fakeMemory) Unmap(h, base uintptr) (uintptr, error) {
	f.ops = append(f.ops, memoryOp{kind: "unmap", addr: base})
	return 0, nil
}

func (f *fakeMemory) PEBAddress(h uintptr) (uintptr, error) {
	f.ops = append(f.ops, memoryOp{kind: "peb"})
	return 0x7fff0000, nil
}

func (f *fakeMemory) Close(h uintptr) { f.ops = append(f.ops, memoryOp{kind: "close"}) }

func (f *fakeMemory) Backend() string { return "fake" }

func (f *fakeMemory) Trace() []string {
	out := make([]string, 0, len(f.ops))
	for _, op := range f.ops {
		out = append(out, op.kind)
	}
	return out
}

// kinds 返回调用序列（只取 kind）。
func (f *fakeMemory) kinds() []string {
	out := make([]string, 0, len(f.ops))
	for _, op := range f.ops {
		out = append(out, op.kind)
	}
	return out
}

// opsOf 返回指定 kind 的全部调用。
func (f *fakeMemory) opsOf(kind string) []memoryOp {
	var out []memoryOp
	for _, op := range f.ops {
		if op.kind == kind {
			out = append(out, op)
		}
	}
	return out
}

// ===== 注入：RW → 写 → RX =====

func TestStagePayloadAllocWriteThenExecute(t *testing.T) {
	sc := []byte{0x90, 0x90, 0xC3}
	rm := newFakeMemory()

	addr, written, err := stagePayload(rm, 0x600, sc)
	if err != nil {
		t.Fatalf("stagePayload 不应失败: %v", err)
	}
	if written != uintptr(len(sc)) {
		t.Errorf("写入字节数 = %d, 期望 %d", written, len(sc))
	}
	if addr == 0 {
		t.Fatal("返回地址为 0")
	}

	if got, want := strings.Join(rm.kinds(), ","), "alloc,write,protect"; got != want {
		t.Fatalf("调用序列 = %s, 期望 %s（先写后执行，缺一步都会留下 RWX 或写不进去）", got, want)
	}
	alloc := rm.opsOf("alloc")[0]
	if alloc.protect != pageReadWrite {
		t.Errorf("分配保护 = 0x%x, 期望 PAGE_READWRITE(0x%x)", alloc.protect, pageReadWrite)
	}
	if alloc.size != uintptr(len(sc)) {
		t.Errorf("分配长度 = 0x%x, 期望 0x%x", alloc.size, len(sc))
	}
	prot := rm.opsOf("protect")[0]
	if prot.protect != pageExecuteRead {
		t.Errorf("最终保护 = 0x%x, 期望 PAGE_EXECUTE_READ(0x%x)", prot.protect, pageExecuteRead)
	}
	// 整个流程任何一步都不允许出现 RWX
	for _, op := range rm.ops {
		if op.protect == pageExecuteReadWrite {
			t.Errorf("%s 使用了 PAGE_EXECUTE_READWRITE", op.kind)
		}
	}
}

func TestStagePayloadShortWriteIsError(t *testing.T) {
	rm := newFakeMemory()
	rm.shortWrite = true

	if _, _, err := stagePayload(rm, 0x600, []byte{1, 2, 3, 4}); err == nil {
		t.Fatal("写入不完整应报错（否则症状是注入报成功但进程无反应）")
	}
	if got := len(rm.opsOf("protect")); got != 0 {
		t.Errorf("写入失败后不应再改保护，实际调用 %d 次", got)
	}
}

func TestStagePayloadProtectFailureIsError(t *testing.T) {
	rm := newFakeMemory()
	rm.failProt = true

	if _, _, err := stagePayload(rm, 0x600, []byte{1, 2, 3}); err == nil {
		t.Fatal("改 RX 失败应报错（此时内存仍可写，不能当成功返回）")
	}
}

// ===== 镂空：预留 / 写入 / 收紧 =====

func testImageInfo() *peImageInfo {
	return &peImageInfo{
		imageBase:           0x140000000,
		sizeOfImage:         0x5000,
		sizeOfHeaders:       0x400,
		addressOfEntryPoint: 0x1000,
		hasReloc:            true,
		sections: []peSectionInfo{
			// .text：可执行 + 可读
			{virtualAddress: 0x1000, virtualSize: 0x900, rawSize: 0x800, characteristics: 0x60000020, data: bytes.Repeat([]byte{0xCC}, 0x800)},
			// .rdata：可读（IAT 通常落在这一节，loader 要写）
			{virtualAddress: 0x2000, virtualSize: 0x300, rawSize: 0x200, characteristics: 0x40000040, data: bytes.Repeat([]byte{0x11}, 0x200)},
			// .data：可读可写
			{virtualAddress: 0x3000, virtualSize: 0x200, rawSize: 0x200, characteristics: 0xC0000040, data: bytes.Repeat([]byte{0x22}, 0x200)},
		},
	}
}

func TestReserveImagePrefersPreferredBase(t *testing.T) {
	info := testImageInfo()
	rm := newFakeMemory()

	addr, fellBack, err := reserveImage(rm, 0x600, info)
	if err != nil {
		t.Fatalf("reserveImage 失败: %v", err)
	}
	if addr != info.imageBase {
		t.Errorf("应优先在 preferred base 分配, 得到 0x%x", addr)
	}
	if fellBack {
		t.Error("拿到 preferred base 时不应标记为回退")
	}
	allocs := rm.opsOf("alloc")
	if len(allocs) != 1 {
		t.Fatalf("分配次数 = %d, 期望 1", len(allocs))
	}
	if allocs[0].at != info.imageBase || allocs[0].protect != pageReadWrite {
		t.Errorf("分配参数 = at:0x%x prot:0x%x, 期望 at:0x%x prot:RW", allocs[0].at, allocs[0].protect, info.imageBase)
	}
	if allocs[0].size != uintptr(info.sizeOfImage) {
		t.Errorf("分配长度 = 0x%x, 期望 SizeOfImage 0x%x", allocs[0].size, info.sizeOfImage)
	}
}

func TestReserveImageFallsBackToAnyAddress(t *testing.T) {
	info := testImageInfo()
	rm := newFakeMemory()
	rm.failAlloc = map[uintptr]bool{info.imageBase: true}

	addr, fellBack, err := reserveImage(rm, 0x600, info)
	if err != nil {
		t.Fatalf("回退分配失败: %v", err)
	}
	if !fellBack {
		t.Error("未拿到 preferred base 时必须标记回退（决定是否需要重定位写）")
	}
	allocs := rm.opsOf("alloc")
	if len(allocs) != 2 {
		t.Fatalf("分配次数 = %d, 期望 2（preferred 失败后回退）", len(allocs))
	}
	if allocs[1].at != 0 {
		t.Errorf("回退分配应请求任意地址, 实际 at=0x%x", allocs[1].at)
	}
	for _, a := range allocs {
		if a.protect == pageExecuteReadWrite {
			t.Error("预留阶段不允许申请 RWX")
		}
	}
	if addr == 0 {
		t.Error("回退后仍应拿到地址")
	}
}

func TestWriteImageWritesHeadersThenSections(t *testing.T) {
	info := testImageInfo()
	rm := newFakeMemory()
	peData := make([]byte, 0x1000)

	if err := writeImage(rm, 0x600, 0x140000000, info, peData); err != nil {
		t.Fatalf("writeImage 失败: %v", err)
	}
	writes := rm.opsOf("write")
	if len(writes) != 1+len(info.sections) {
		t.Fatalf("写入次数 = %d, 期望 %d（头 + 各节）", len(writes), 1+len(info.sections))
	}
	if writes[0].addr != 0x140000000 || writes[0].size != uintptr(info.sizeOfHeaders) {
		t.Errorf("首个写入应是 PE 头 @0x140000000 长度 0x%x, 实际 addr=0x%x size=0x%x",
			info.sizeOfHeaders, writes[0].addr, writes[0].size)
	}
	if writes[1].addr != 0x140001000 {
		t.Errorf(".text 应写到 base+0x1000, 实际 0x%x", writes[1].addr)
	}
	// 写入阶段不允许改保护（保护在 hardenImage 里做）
	if n := len(rm.opsOf("protect")); n != 0 {
		t.Errorf("writeImage 不应改保护，实际调用 %d 次", n)
	}
}

func TestHardenImageOnlyTightensExecutableSections(t *testing.T) {
	info := testImageInfo()
	rm := newFakeMemory()

	notes, err := hardenImage(rm, 0x600, 0x140000000, info, false)
	if err != nil {
		t.Fatalf("hardenImage 失败: %v", err)
	}
	prots := rm.opsOf("protect")
	if len(prots) != 1 {
		t.Fatalf("改保护次数 = %d, 期望 1（只有 .text 可执行）: %v", len(prots), rm.kinds())
	}
	if prots[0].addr != 0x140001000 || prots[0].size != 0x900 {
		t.Errorf("保护范围 = 0x%x+0x%x, 期望 0x140001000+0x900", prots[0].addr, prots[0].size)
	}
	if prots[0].protect != pageExecuteRead {
		t.Errorf("可执行节保护 = 0x%x, 期望 RX(0x%x)", prots[0].protect, pageExecuteRead)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "RX") {
		t.Errorf("诊断应记录 .text 已收紧为 RX, 实际 %v", notes)
	}
}

// .rdata 与 .data 必须保持可写：镂空是私有内存，没有 SEC_IMAGE 的写时复制兜底，
// 把 IAT 所在页设为只读会让 loader 解析导入表时访问违例。
func TestHardenImageKeepsNonExecutableSectionsWritable(t *testing.T) {
	info := testImageInfo()
	rm := newFakeMemory()

	if _, err := hardenImage(rm, 0x600, 0x140000000, info, false); err != nil {
		t.Fatalf("hardenImage 失败: %v", err)
	}
	for _, p := range rm.opsOf("protect") {
		if p.addr == 0x140002000 || p.addr == 0x140003000 {
			t.Errorf("非可执行节 0x%x 被改了保护 —— loader 写 IAT/全局时会违例", p.addr)
		}
	}
}

func TestHardenImageRelocatingKeepsExecutableWritable(t *testing.T) {
	info := testImageInfo()
	rm := newFakeMemory()

	notes, err := hardenImage(rm, 0x600, 0x140000000, info, true)
	if err != nil {
		t.Fatalf("hardenImage 失败: %v", err)
	}
	prots := rm.opsOf("protect")
	if len(prots) != 1 {
		t.Fatalf("改保护次数 = %d, 期望 1", len(prots))
	}
	// 需重定位时保留写权限是 loader 行为决定的，但必须在诊断里显式标出来
	if prots[0].protect != pageExecuteReadWrite {
		t.Errorf("重定位期保护 = 0x%x, 期望 RWX(0x%x)", prots[0].protect, pageExecuteReadWrite)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "重定位") {
		t.Errorf("诊断必须标出 RWX 是重定位期妥协, 实际 %v", notes)
	}
}

func TestHardenImageClampsToImageSize(t *testing.T) {
	info := &peImageInfo{
		sizeOfImage: 0x2000,
		sections: []peSectionInfo{
			{virtualAddress: 0x1000, virtualSize: 0x3000, rawSize: 0x1000, characteristics: 0x60000020},
		},
	}
	rm := newFakeMemory()

	if _, err := hardenImage(rm, 0x600, 0x1000000, info, false); err != nil {
		t.Fatalf("hardenImage 失败: %v", err)
	}
	prots := rm.opsOf("protect")
	if len(prots) != 1 {
		t.Fatalf("改保护次数 = %d, 期望 1", len(prots))
	}
	if prots[0].size != 0x1000 {
		t.Errorf("保护长度应被夹到镜像边界内 0x1000, 实际 0x%x", prots[0].size)
	}
}

// ===== syscall 优先 / 宿主回退 =====

type fakeNT struct {
	calls     []string
	fail      map[string]bool
	allocBase uintptr
	handle    uintptr
}

func (f *fakeNT) backend() ntBackend {
	return ntBackend{
		ready: func(name string) bool { return true },
		invoke: func(name string, args ...uintptr) (uintptr, error) {
			f.calls = append(f.calls, name)
			if f.fail[name] {
				return 0xC0000005, nil // STATUS_ACCESS_VIOLATION：模拟被 hook 后失败
			}
			switch name {
			case "NtOpenProcess":
				*(*uintptr)(unsafe.Pointer(args[0])) = f.handle
			case "NtAllocateVirtualMemory":
				base := (*uintptr)(unsafe.Pointer(args[1]))
				if *base == 0 {
					*base = f.allocBase
				}
			case "NtWriteVirtualMemory":
				*(*uintptr)(unsafe.Pointer(args[4])) = args[3] // NumberOfBytesWritten = 请求长度
			case "NtReadVirtualMemory":
				*(*uintptr)(unsafe.Pointer(args[4])) = args[3]
			case "NtProtectVirtualMemory":
				*(*uint32)(unsafe.Pointer(args[4])) = pageReadWrite
			}
			return 0, nil
		},
	}
}

type fakeHost struct {
	addr       uintptr
	openCalls  int
	allocCalls int
	writeCalls int
	readCalls  int
	protCalls  int
	unmapCalls int
	pebCalls   int
}

func (f *fakeHost) backend() hostBackend {
	addr := f.addr
	if addr == 0 {
		addr = 0x40000
	}
	return hostBackend{
		openProcess: func(pid int, access uint32) (uintptr, error) { f.openCalls++; return 0x700, nil },
		alloc:       func(h, at, size uintptr, protect uint32) (uintptr, error) { f.allocCalls++; return addr, nil },
		write:       func(h, dst uintptr, data []byte) (uintptr, error) { f.writeCalls++; return uintptr(len(data)), nil },
		read:        func(h, src uintptr, n int) ([]byte, error) { f.readCalls++; return make([]byte, n), nil },
		protect: func(h, addr, size uintptr, protect uint32) (uint32, error) {
			f.protCalls++
			return pageReadWrite, nil
		},
		unmap: func(h, base uintptr) (uintptr, error) { f.unmapCalls++; return 0, nil },
		peb:   func(h uintptr) (uintptr, error) { f.pebCalls++; return 0x9000, nil },
		close: func(h uintptr) {},
	}
}

func TestRemoteMemoryPrefersSyscall(t *testing.T) {
	nt := &fakeNT{allocBase: 0x30000, handle: 0x1234}
	host := &fakeHost{}
	rm := &hybridMemory{mode: "direct", useNt: true, nt: nt.backend(), host: host.backend()}

	h, err := rm.OpenProcess(1234, 0x1F0FFF)
	if err != nil {
		t.Fatalf("OpenProcess 失败: %v", err)
	}
	if h != 0x1234 || host.openCalls != 0 {
		t.Errorf("应走 NtOpenProcess (h=0x%x), 宿主回退次数 = %d", h, host.openCalls)
	}

	addr, err := rm.Alloc(h, 0, 0x1000, pageReadWrite)
	if err != nil || addr != 0x30000 {
		t.Fatalf("Alloc = 0x%x, %v；期望走 syscall 拿到 0x30000", addr, err)
	}
	if host.allocCalls != 0 {
		t.Error("syscall 成功时不应调用宿主 API")
	}
	if !strings.Contains(strings.Join(nt.calls, ","), "NtAllocateVirtualMemory") {
		t.Errorf("syscall 调用序列缺少 NtAllocateVirtualMemory: %v", nt.calls)
	}

	// 写：syscall 返回的 written 必须被校验（假实现返回正确长度）
	if _, err := rm.Write(h, addr, []byte{1, 2, 3}); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}
	if host.writeCalls != 0 {
		t.Error("syscall 成功时不应回退 WriteProcessMemory")
	}
	if _, err := rm.Protect(h, addr, 3, pageExecuteRead); err != nil {
		t.Fatalf("Protect 失败: %v", err)
	}
	if host.protCalls != 0 {
		t.Error("syscall 成功时不应回退 VirtualProtectEx")
	}
	if got := rm.Backend(); got != "syscall:direct" {
		t.Errorf("Backend() = %s, 期望 syscall:direct", got)
	}
}

func TestRemoteMemoryFallsBackWhenSyscallFails(t *testing.T) {
	nt := &fakeNT{
		allocBase: 0x30000,
		fail:      map[string]bool{"NtAllocateVirtualMemory": true, "NtWriteVirtualMemory": true, "NtProtectVirtualMemory": true},
	}
	host := &fakeHost{}
	rm := &hybridMemory{mode: "indirect", useNt: true, nt: nt.backend(), host: host.backend()}

	addr, err := rm.Alloc(0x600, 0, 0x1000, pageReadWrite)
	if err != nil {
		t.Fatalf("syscall 失败后应回退成功, 实际 %v", err)
	}
	if host.allocCalls != 1 || addr != 0x40000 {
		t.Errorf("回退分配 addr=0x%x hostCalls=%d, 期望走宿主拿到 0x40000", addr, host.allocCalls)
	}
	if _, err := rm.Write(0x600, addr, []byte{1, 2}); err != nil {
		t.Fatalf("写入回退失败: %v", err)
	}
	if host.writeCalls != 1 {
		t.Errorf("宿主写入次数 = %d, 期望 1", host.writeCalls)
	}
	if _, err := rm.Protect(0x600, addr, 2, pageExecuteRead); err != nil {
		t.Fatalf("改保护回退失败: %v", err)
	}
	if host.protCalls != 1 {
		t.Errorf("宿主改保护次数 = %d, 期望 1", host.protCalls)
	}
}

func TestRemoteMemoryUsesHostWhenSyscallDisabled(t *testing.T) {
	nt := &fakeNT{allocBase: 0x30000}
	host := &fakeHost{}
	rm := &hybridMemory{mode: "none", useNt: false, nt: nt.backend(), host: host.backend()}

	if _, err := rm.Alloc(0x600, 0, 0x1000, pageReadWrite); err != nil {
		t.Fatalf("Alloc 失败: %v", err)
	}
	if len(nt.calls) != 0 {
		t.Errorf("none 模式不应调用任何 NtXxx, 实际 %v", nt.calls)
	}
	if host.allocCalls != 1 {
		t.Errorf("宿主分配次数 = %d, 期望 1", host.allocCalls)
	}
	if got := rm.Backend(); got != "host" {
		t.Errorf("Backend() = %s, 期望 host", got)
	}
	if len(rm.Trace()) == 0 || !strings.Contains(rm.Trace()[0], "alloc(host)") {
		t.Errorf("Trace 应记录实际后端, 实际 %v", rm.Trace())
	}
}

// ===== 真实宿主后端：在本进程里跑一遍 RW → 写 → RX =====

// TestHostBackendRoundTripInOwnProcess 用真实 kernel32 验证宿主回退这条路本身可用，
// 并断言 RX 之后写不进去（W^X 的实际效果，而不是只看参数）。
func TestHostBackendRoundTripInOwnProcess(t *testing.T) {
	rm := &hybridMemory{mode: "none", useNt: false, host: defaultHostBackend()}

	const (
		processQueryInfo   = 0x0400
		processVMOperation = 0x0008
		processVMWrite     = 0x0020
		processVMRead      = 0x0010
		access             = processQueryInfo | processVMOperation | processVMWrite | processVMRead
	)

	h, err := rm.OpenProcess(os.Getpid(), access)
	if err != nil {
		t.Fatalf("打开自身进程失败: %v", err)
	}
	defer rm.Close(h)

	addr, err := rm.Alloc(h, 0, 0x1000, pageReadWrite)
	if err != nil {
		t.Fatalf("分配失败: %v", err)
	}
	defer freeOwnMemory(t, addr)

	payload := []byte("Tshell-W^X-probe")
	if n, err := rm.Write(h, addr, payload); err != nil || n != uintptr(len(payload)) {
		t.Fatalf("写入失败: n=%d err=%v", n, err)
	}
	got, err := rm.Read(h, addr, len(payload))
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("回读内容不符: %q", got)
	}

	if _, err := rm.Protect(h, addr, 0x1000, pageExecuteRead); err != nil {
		t.Fatalf("改 RX 失败: %v", err)
	}

	// 断言落在**实际保护位**上（VirtualQuery），而不是"再写一次应该失败"：
	// 实测 WriteProcessMemory 写自己进程的 RX 页会成功（对自身进程不做页保护校验），
	// 拿它当断言会一直假绿 —— 真正要紧的是目标进程那页确实被改成了 RX。
	var mbi windows.MemoryBasicInformation
	if err := windows.VirtualQuery(addr, &mbi, unsafe.Sizeof(mbi)); err != nil {
		t.Fatalf("查询保护位失败: %v", err)
	}
	if mbi.State != windows.MEM_COMMIT {
		t.Errorf("页状态 = 0x%x, 期望 MEM_COMMIT(0x%x)", mbi.State, windows.MEM_COMMIT)
	}
	if mbi.Protect != windows.PAGE_EXECUTE_READ {
		t.Errorf("实际保护 = 0x%x, 期望 PAGE_EXECUTE_READ(0x%x)", mbi.Protect, windows.PAGE_EXECUTE_READ)
	}
}

// freeOwnMemory 释放本进程内测试分配的内存。
func freeOwnMemory(t *testing.T, addr uintptr) {
	t.Helper()
	if err := windows.VirtualFree(addr, 0, windows.MEM_RELEASE); err != nil {
		t.Logf("释放测试内存失败（不影响断言）: %v", err)
	}
}

// ===== 源码级棘轮 =====
//
// 注入/镂空一旦又直接调 kernel32 的分配/写入/改保护，syscall 模式会静默失效：
// 功能全在、没有任何运行期症状，只有 EDR 看得见。用源码断言把它钉死。
func TestInjectionPathsDoNotBypassRemoteMemory(t *testing.T) {
	banned := []string{
		"VirtualAllocEx.Call",
		"WriteProcessMemory.Call",
		"ReadProcessMemory.Call",
		"VirtualProtectEx.Call",
	}
	for _, name := range []string{"inject_windows.go", "hollow_windows.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", name, err)
		}
		text := string(src)
		for _, b := range banned {
			if strings.Contains(text, b) {
				t.Errorf("%s 仍直接调用 %s —— 应走 remoteMemory（syscall 优先/宿主回退）", name, b)
			}
		}
	}
}
