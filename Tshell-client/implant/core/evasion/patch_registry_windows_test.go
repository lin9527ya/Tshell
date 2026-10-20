//go:build windows

package evasion

import (
	"errors"
	"testing"
)

// ===== 启动期能力撤销（阶段四）=====
//
// 改造前 PatchETW / PatchAMSI **不保存原始字节**，打了就回不去。
// 这里用注入的内存读写替换真实进程内存（真去改 ntdll 的字节，
// 会把测试进程自己的 ETW 打掉，后续用例全部失真、也无法在 CI 上跑）。
//
// 重点验证的是**台账语义**，而不是"能不能写内存"：
// 逆序撤销、重复 patch 保留最早原始字节、被外部改动时跳过而非盲写。

// fakeMemory 一个可注入的内存模型
type fakeMemory struct {
	data map[uintptr][]byte
}

func newFakeMemory() *fakeMemory { return &fakeMemory{data: map[uintptr][]byte{}} }

func (f *fakeMemory) read(addr uintptr, n int) []byte {
	src, ok := f.data[addr]
	if !ok || len(src) < n {
		return nil
	}
	out := make([]byte, n)
	copy(out, src[:n])
	return out
}

func (f *fakeMemory) write(addr uintptr, data []byte) error {
	cur, ok := f.data[addr]
	if !ok {
		f.data[addr] = append([]byte(nil), data...)
		return nil
	}
	if len(cur) < len(data) {
		f.data[addr] = append([]byte(nil), data...)
		return nil
	}
	copy(cur, data)
	return nil
}

// withFakeMemory 替换全局内存读写并在结束时还原
func withFakeMemory(t *testing.T, f *fakeMemory) {
	t.Helper()
	oldRead, oldWrite := readMemory, writeMemory
	readMemory = f.read
	writeMemory = f.write
	ResetPatchRegistry()
	t.Cleanup(func() {
		readMemory, writeMemory = oldRead, oldWrite
		ResetPatchRegistry()
	})
}

// 打补丁后撤销，必须回到原始字节
func TestRestorePatchesWritesBackOriginal(t *testing.T) {
	f := newFakeMemory()
	withFakeMemory(t, f)

	const addr = uintptr(0x1000)
	original := []byte{0x48, 0x89, 0x5C, 0x24, 0x08, 0x48}
	f.data[addr] = append([]byte(nil), original...)

	patch := []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3}
	// 模拟 patchExports 的行为：读取原始 → 写入 → 记账
	orig := readMemory(addr, len(patch))
	if err := writeMemory(addr, patch); err != nil {
		t.Fatal(err)
	}
	recordPatch(TagETW, "ntdll.dll", "EtwEventWrite", addr, orig, patch)

	if PatchedCount() != 1 {
		t.Fatalf("台账应有 1 条，实际 %d", PatchedCount())
	}

	rep, err := RestorePatches()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Restored() != 1 {
		t.Fatalf("应还原 1 个目标，实际 %d", rep.Restored())
	}
	// 内存里应恢复成原始字节的前 5 字节
	for i := 0; i < len(original); i++ {
		if f.data[addr][i] != original[i] {
			t.Fatalf("第 %d 字节未还原: %#x != %#x", i, f.data[addr][i], original[i])
		}
	}
	if PatchedCount() != 0 {
		t.Errorf("还原后台账应清空，实际 %d", PatchedCount())
	}
}

// 台账为空时撤销应返回明确错误（而不是静默成功）
func TestRestorePatchesEmptyRegistry(t *testing.T) {
	f := newFakeMemory()
	withFakeMemory(t, f)

	if _, err := RestorePatches(); !errors.Is(err, ErrNoPatches) {
		t.Fatalf("空台账应返回 ErrNoPatches，实际 %v", err)
	}
}

// 同一地址被 patch 两次：原始字节必须保留**最早**那份，
// 否则撤销只会回到第一次的补丁（等于没撤销）
func TestRecordPatchKeepsEarliestOriginal(t *testing.T) {
	f := newFakeMemory()
	withFakeMemory(t, f)

	const addr = uintptr(0x2000)
	trueOriginal := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
	f.data[addr] = append([]byte(nil), trueOriginal...)

	first := []byte{0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA}
	second := []byte{0xBB, 0xBB, 0xBB, 0xBB, 0xBB, 0xBB}

	orig1 := readMemory(addr, len(first))
	writeMemory(addr, first)
	recordPatch(TagETW, "ntdll.dll", "Fn", addr, orig1, first)

	orig2 := readMemory(addr, len(second)) // 此时读到的是 first
	writeMemory(addr, second)
	recordPatch(TagETW, "ntdll.dll", "Fn", addr, orig2, second)

	if PatchedCount() != 1 {
		t.Fatalf("同一地址应只有 1 条台账，实际 %d", PatchedCount())
	}

	rep, err := RestorePatches()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Restored() != 1 {
		t.Fatalf("应还原 1 个，实际 %d", rep.Restored())
	}
	for i := range trueOriginal {
		if f.data[addr][i] != trueOriginal[i] {
			t.Fatalf("第 %d 字节应回到**最早**的原始值 %#x，实际 %#x（说明台账存了中间态）",
				i, trueOriginal[i], f.data[addr][i])
		}
	}
}

// 补丁期间被外部改过（EDR 重新 hook）：必须跳过并报告，不能盲写
func TestRestorePatchesSkipsWhenMemoryChangedExternally(t *testing.T) {
	f := newFakeMemory()
	withFakeMemory(t, f)

	const addr = uintptr(0x3000)
	f.data[addr] = []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}

	patch := []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3}
	orig := readMemory(addr, len(patch))
	writeMemory(addr, patch)
	recordPatch(TagETW, "ntdll.dll", "Fn", addr, orig, patch)

	// 外部把补丁区域改掉（模拟 EDR 重新 hook）
	external := []byte{0xE9, 0x01, 0x02, 0x03, 0x04, 0x05}
	copy(f.data[addr], external)

	rep, err := RestorePatches()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Skipped() != 1 {
		t.Fatalf("应跳过 1 个目标，实际 %d（Restored=%d）", rep.Skipped(), rep.Restored())
	}
	if rep.Restored() != 0 {
		t.Error("被外部改过时不应盲写")
	}
	// 外部写入的字节必须保持原样
	for i := range external {
		if f.data[addr][i] != external[i] {
			t.Fatalf("不应覆盖外部改动：第 %d 字节 %#x != %#x", i, f.data[addr][i], external[i])
		}
	}
	// 被跳过的条目保留在台账里，便于排查/重试
	if PatchedCount() != 1 {
		t.Errorf("被跳过的条目应保留在台账中，实际 %d", PatchedCount())
	}
}

// 多个目标必须**逆序**撤销
func TestRestorePatchesReverseOrder(t *testing.T) {
	f := newFakeMemory()
	withFakeMemory(t, f)

	const addrA = uintptr(0x4000)
	const addrB = uintptr(0x5000)
	f.data[addrA] = []byte{0x01, 0x01, 0x01, 0x01, 0x01, 0x01}
	f.data[addrB] = []byte{0x02, 0x02, 0x02, 0x02, 0x02, 0x02}

	patch := []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3}
	for _, a := range []uintptr{addrA, addrB} {
		orig := readMemory(a, len(patch))
		writeMemory(a, patch)
		recordPatch(TagETW, "ntdll.dll", "Fn", a, orig, patch)
	}

	rep, err := RestorePatches()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Restored() != 2 {
		t.Fatalf("应还原 2 个，实际 %d", rep.Restored())
	}
	// 报告顺序应为逆序：B 先于 A
	if rep.Targets[0].Addr != addrB {
		t.Errorf("撤销应按逆序（先 B 后 A），实际第一个是 0x%x", rep.Targets[0].Addr)
	}
	if rep.Targets[1].Addr != addrA {
		t.Errorf("撤销应按逆序，实际第二个是 0x%x", rep.Targets[1].Addr)
	}
}

// 读原始字节失败（Original 为空）时，还原应跳过而不是写空
func TestRestorePatchesSkipsWhenOriginalUnreadable(t *testing.T) {
	f := newFakeMemory()
	withFakeMemory(t, f)

	const addr = uintptr(0x6000)
	// 内存里没有任何数据 → readMemory 返回 nil
	patch := []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3}
	writeMemory(addr, patch)
	recordPatch(TagETW, "ntdll.dll", "Fn", addr, nil, patch)

	rep, err := RestorePatches()
	if err != nil {
		t.Fatal(err)
	}
	if rep.Restored() != 0 {
		t.Error("原始字节不可读时不应声称还原成功")
	}
	if rep.Skipped() != 1 {
		t.Errorf("原始字节不可读时应跳过，实际 Skipped=%d", rep.Skipped())
	}
}

// 按能力撤销必须只撤对应能力：关 ETW 不能顺手把 AMSI 也撤了
func TestRestorePatchesByTagIsScoped(t *testing.T) {
	f := newFakeMemory()
	withFakeMemory(t, f)

	const addrEtw = uintptr(0x7000)
	const addrAmsi = uintptr(0x8000)
	patch := []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3}
	orig := []byte{0x48, 0x89, 0x5C, 0x24, 0x08, 0x48}
	f.data[addrEtw] = append([]byte(nil), orig...)
	f.data[addrAmsi] = append([]byte(nil), orig...)

	for _, tc := range []struct {
		tag  string
		addr uintptr
	}{{TagETW, addrEtw}, {TagAMSI, addrAmsi}} {
		o := readMemory(tc.addr, len(patch))
		writeMemory(tc.addr, patch)
		recordPatch(tc.tag, "dll", "Fn", tc.addr, o, patch)
	}
	if PatchedCount() != 2 {
		t.Fatalf("台账应有 2 条，实际 %d", PatchedCount())
	}

	rep, err := RestorePatchesByTag(TagETW)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Restored() != 1 {
		t.Fatalf("只应还原 ETW 的 1 个入口，实际 %d", rep.Restored())
	}
	for i := range orig {
		if f.data[addrEtw][i] != orig[i] {
			t.Fatalf("ETW 第 %d 字节未还原", i)
		}
	}
	// AMSI 的补丁必须还在
	for i := range patch {
		if f.data[addrAmsi][i] != patch[i] {
			t.Fatalf("AMSI 的补丁被误撤销了（第 %d 字节）—— 关一项不能变成关一组", i)
		}
	}
	if PatchedCount() != 1 {
		t.Errorf("台账应剩 1 条（AMSI），实际 %d", PatchedCount())
	}
}

// 未知 tag 撤销应返回 ErrNoPatches，而不是静默成功
func TestRestorePatchesByUnknownTag(t *testing.T) {
	f := newFakeMemory()
	withFakeMemory(t, f)

	const addr = uintptr(0x9000)
	patch := []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3}
	f.data[addr] = []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
	o := readMemory(addr, len(patch))
	writeMemory(addr, patch)
	recordPatch(TagETW, "ntdll.dll", "Fn", addr, o, patch)

	if _, err := RestorePatchesByTag(TagAMSI); !errors.Is(err, ErrNoPatches) {
		t.Fatalf("未知 tag 应返回 ErrNoPatches，实际 %v", err)
	}
}
