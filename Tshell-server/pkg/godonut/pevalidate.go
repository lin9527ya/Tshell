package godonut

import (
	"encoding/binary"
	"fmt"
)

// validatePEReloc 在 godonut 解析 PE 前预检 BASERELOC 重定位表。
//
// 【背景】github.com/Binject/debug/pe 的 readBaseRelocationTable 对重定位块头
// 没有任何边界检查：块头 SizeOfBlock 为 0/垃圾值时，(SizeOfBlock-8)/2 在
// uint32 下溢为 0x7FFFFFFC，make([]BlockItem, ...) 直接分配 8GB+ 内存，
// 服务端会被 OOM kill（曾因 patch 撑大模板文件、节偏移错位导致全零块头而触发）。
// 此函数按与该库完全相同的顺序遍历重定位块，任何越界/下溢模式都提前报错。
func validatePEReloc(data []byte) error {
	const (
		imageDirEntryBasereloc = 5
	)
	if len(data) < 0x40 || string(data[0:2]) != "MZ" {
		return fmt.Errorf("not a PE (missing MZ)")
	}
	peOff := int(binary.LittleEndian.Uint32(data[0x3c:0x40]))
	if peOff+24 > len(data) || string(data[peOff:peOff+4]) != "PE\x00\x00" {
		return fmt.Errorf("not a PE (bad PE signature)")
	}
	coff := peOff + 4
	numSections := int(binary.LittleEndian.Uint16(data[coff+2:]))
	optSize := int(binary.LittleEndian.Uint16(data[coff+16:]))
	opt := coff + 20
	if opt+optSize > len(data) {
		return fmt.Errorf("corrupt PE: optional header out of bounds")
	}
	magic := binary.LittleEndian.Uint16(data[opt:])
	var ddOff, numDDOff int
	switch magic {
	case 0x20b: // PE32+
		ddOff, numDDOff = opt+112, opt+108
	case 0x10b: // PE32
		ddOff, numDDOff = opt+96, opt+92
	default:
		return fmt.Errorf("corrupt PE: unknown optional header magic 0x%x", magic)
	}
	if int(binary.LittleEndian.Uint32(data[numDDOff:])) <= imageDirEntryBasereloc {
		return nil // 无 BASERELOC 目录，库内循环不会执行
	}
	va := binary.LittleEndian.Uint32(data[ddOff+imageDirEntryBasereloc*8:])
	size := binary.LittleEndian.Uint32(data[ddOff+imageDirEntryBasereloc*8+4:])
	if size == 0 {
		return nil // 空重定位表，库内循环不会执行
	}

	// 与库一致：找 VirtualAddress == dd.VirtualAddress 的节
	secOff := opt + optSize
	var secStart, secEnd int
	found := false
	for i := 0; i < numSections; i++ {
		s := secOff + i*40
		if s+40 > len(data) {
			return fmt.Errorf("corrupt PE: section header out of bounds")
		}
		if binary.LittleEndian.Uint32(data[s+12:]) == va {
			rawSize := int64(binary.LittleEndian.Uint32(data[s+16:]))
			rawPtr := int64(binary.LittleEndian.Uint32(data[s+20:]))
			secStart = int(rawPtr)
			secEnd = int(rawPtr + rawSize)
			if secEnd > len(data) {
				secEnd = len(data)
			}
			found = true
			break
		}
	}
	if !found || secStart >= secEnd {
		// 库会用空 reader 读并安全返回错误，这里同样拒绝
		return fmt.Errorf("corrupt PE: BASERELOC directory (va=0x%x size=%d) has no matching section data", va, size)
	}

	// 按库的遍历顺序逐块检查：bytesRead < dd.Size，每块读 8 字节头再读项
	sec := data[secStart:secEnd]
	var bytesRead uint32
	for bytesRead < size {
		if int(bytesRead)+8 > len(sec) {
			return fmt.Errorf("corrupt PE: relocation table (%d bytes) exceeds section data (%d bytes)", size, len(sec))
		}
		blkSize := binary.LittleEndian.Uint32(sec[bytesRead+4 : bytesRead+8])
		if blkSize < 8 {
			// (blkSize-8)/2 uint32 下溢 -> 库内 8GB+ makeslice
			return fmt.Errorf("corrupt PE: relocation block #%d SizeOfBlock=%d (<8, would underflow to %d entries)",
				bytesRead, blkSize, (blkSize-8)/2)
		}
		if int64(bytesRead)+int64(blkSize) > int64(len(sec)) {
			return fmt.Errorf("corrupt PE: relocation block SizeOfBlock=%d exceeds remaining section data (%d bytes)", blkSize, len(sec)-int(bytesRead))
		}
		bytesRead += blkSize
	}
	return nil
}
