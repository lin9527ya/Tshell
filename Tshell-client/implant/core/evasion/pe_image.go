// Package evasion —— PE 镜像解析（平台无关）。
//
// 为什么单独抽出来：unhook 的核心是"拿磁盘上的干净副本，覆盖内存里被改过的字节"。
// 这件事有两个容易写错的地方，都必须能在 Linux/CI 上用合成 PE 反复验证：
//
//  1. **RVA ≠ 文件偏移**。节表里 VirtualAddress 是加载后地址，PointerToRawData 才是
//     文件里的位置。对 ntdll 这类系统 DLL，.text 通常是 VirtualAddress=0x1000、
//     PointerToRawData=0x400 —— 直接用 RVA 索引文件读到的是别的数据。早期实现就
//     踩了这个坑：它把磁盘上"别处"的字节写进 ntdll .text，等于主动破坏代码段。
//  2. **系统 DLL 是被 ASLR 重定位过的**。内存里的 .text 可能因为基址重定位而与磁盘
//     不同，这是合法的差异，不能被当成 hook 覆盖掉。所以比较前要按 delta
//     （实际基址 − ImageBase）把磁盘副本里的重定位项也应用一遍。
package evasion

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// PESection PE 节表项
type PESection struct {
	Name            string
	VirtualAddress  uint32
	VirtualSize     uint32
	RawPointer      uint32
	RawSize         uint32
	Characteristics uint32
}

// PEReloc 基址重定位项（RVA 指目标在镜像内的位置）
type PEReloc struct {
	RVA  uint32
	Type uint16
}

// PEImage 解析结果
type PEImage struct {
	ImageBase uint64
	Sections  []PESection
	Relocs    []PEReloc
}

// 节属性
const (
	scnMemExecute = 0x20000000
	scnMemWrite   = 0x80000000
)

// 重定位类型（IMAGE_REL_BASED_*）
const (
	relBasedAbsolute = 0
	relBasedHighLow  = 3
	relBasedDir64    = 10
)

func errPE(msg string) error { return errors.New("PE: " + msg) }

// ParsePEImage 解析 PE32+ 镜像的节表与基址重定位表。
func ParsePEImage(data []byte) (*PEImage, error) {
	if len(data) < 0x40 || data[0] != 'M' || data[1] != 'Z' {
		return nil, errPE("缺少 MZ 头")
	}
	peOff := binary.LittleEndian.Uint32(data[0x3C:])
	if int(peOff)+24 > len(data) {
		return nil, errPE("e_lfanew 越界")
	}
	if string(data[peOff:peOff+4]) != "PE\x00\x00" {
		return nil, errPE("缺少 PE 签名")
	}
	coff := int(peOff) + 4
	numSections := int(binary.LittleEndian.Uint16(data[coff+2:]))
	optSize := int(binary.LittleEndian.Uint16(data[coff+16:]))
	optOff := coff + 20
	if optOff+optSize > len(data) || optSize < 240 {
		return nil, errPE("可选头越界")
	}
	if magic := binary.LittleEndian.Uint16(data[optOff:]); magic != 0x20B {
		return nil, fmt.Errorf("PE: 只支持 PE32+（magic=0x%x）", magic)
	}
	img := &PEImage{ImageBase: binary.LittleEndian.Uint64(data[optOff+24:])}

	secOff := optOff + optSize
	if secOff+numSections*40 > len(data) {
		return nil, errPE("节表越界")
	}
	for i := 0; i < numSections; i++ {
		s := data[secOff+i*40:]
		sec := PESection{
			Name:            strings.TrimRight(string(s[0:8]), "\x00"),
			VirtualSize:     binary.LittleEndian.Uint32(s[8:]),
			VirtualAddress:  binary.LittleEndian.Uint32(s[12:]),
			RawSize:         binary.LittleEndian.Uint32(s[16:]),
			RawPointer:      binary.LittleEndian.Uint32(s[20:]),
			Characteristics: binary.LittleEndian.Uint32(s[36:]),
		}
		// 原始数据越界直接拒绝：宁可整份不解析，也不要写出半截字节
		if sec.RawSize > 0 && uint64(sec.RawPointer)+uint64(sec.RawSize) > uint64(len(data)) {
			return nil, fmt.Errorf("PE: 节 %s 原始数据越界", sec.Name)
		}
		img.Sections = append(img.Sections, sec)
	}

	// 数据目录第 5 项 = 基址重定位表
	relocRVA := binary.LittleEndian.Uint32(data[optOff+112+5*8:])
	relocSize := binary.LittleEndian.Uint32(data[optOff+112+5*8+4:])
	if relocRVA != 0 && relocSize != 0 {
		relocs, err := parseRelocs(data, img.Sections, relocRVA, relocSize)
		if err != nil {
			return nil, err
		}
		img.Relocs = relocs
	}
	return img, nil
}

// parseRelocs 解析重定位块。重定位表本身也要走 RVA→文件偏移换算。
func parseRelocs(data []byte, secs []PESection, rva, size uint32) ([]PEReloc, error) {
	off, ok := fileOffsetForRVA(secs, rva)
	if !ok {
		return nil, errPE("重定位表 RVA 无法映射到文件偏移")
	}
	end := uint64(off) + uint64(size)
	if end > uint64(len(data)) {
		return nil, errPE("重定位表越界")
	}
	var out []PEReloc
	cur := int(off)
	for cur+8 <= int(end) {
		pageRVA := binary.LittleEndian.Uint32(data[cur:])
		blockSize := binary.LittleEndian.Uint32(data[cur+4:])
		if blockSize < 8 {
			break // 宽度不足直接停：宁可少恢复几个重定位，也不要读飞
		}
		next := cur + int(blockSize)
		if next > int(end) {
			break
		}
		n := (int(blockSize) - 8) / 2
		for i := 0; i < n; i++ {
			e := binary.LittleEndian.Uint16(data[cur+8+i*2:])
			typ := e >> 12
			if typ == relBasedAbsolute { // 填充项
				continue
			}
			out = append(out, PEReloc{RVA: pageRVA + uint32(e&0x0FFF), Type: typ})
		}
		cur = next
	}
	return out, nil
}

// fileOffsetForRVA 把镜像内 RVA 映射到文件偏移。落在节头之外（如头部本身）时
// 按"头部 = 文件头"处理——调用方只需要节内数据，这条分支用于导出表/重定位表。
func fileOffsetForRVA(secs []PESection, rva uint32) (uint32, bool) {
	for _, s := range secs {
		span := s.VirtualSize
		if s.RawSize > span {
			span = s.RawSize
		}
		if span == 0 {
			continue
		}
		if rva >= s.VirtualAddress && rva < s.VirtualAddress+span {
			return s.RawPointer + (rva - s.VirtualAddress), true
		}
	}
	return 0, false
}

// ExecutableSections 返回带执行属性的节（通常只有 .text）。
func (img *PEImage) ExecutableSections() []PESection {
	var out []PESection
	for _, s := range img.Sections {
		if s.Characteristics&scnMemExecute != 0 && s.VirtualSize > 0 && s.RawSize > 0 {
			out = append(out, s)
		}
	}
	return out
}

// SectionImage 用磁盘数据构造某个节的"加载后应当长成什么样"。
//
// delta = 实际基址 − ImageBase。非 0 时按重定位表把落在本节内的项应用一遍，
// 这样比较出来的差异才是真 hook，而不是 ASLR 造成的合法偏移。
// 越界/类型不支持时返回错误而不是猜。
func (img *PEImage) SectionImage(data []byte, sec PESection, delta int64) ([]byte, error) {
	size := sec.VirtualSize
	if sec.RawSize < size {
		size = sec.RawSize // 尾部未初始化部分（.bss 式）不在文件里
	}
	if size == 0 {
		return nil, errPE("节大小为 0")
	}
	if uint64(sec.RawPointer)+uint64(size) > uint64(len(data)) {
		return nil, errPE("节原始数据越界")
	}
	buf := make([]byte, size)
	copy(buf, data[sec.RawPointer:sec.RawPointer+size])
	if delta != 0 {
		if err := applyRelocsToSection(buf, img.Relocs, sec, delta); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// applyRelocsToSection 把重定位项应用到节缓冲上（偏移相对节起点）。
func applyRelocsToSection(buf []byte, relocs []PEReloc, sec PESection, delta int64) error {
	lo := sec.VirtualAddress
	hi := sec.VirtualAddress + uint32(len(buf))
	for _, r := range relocs {
		if r.RVA < lo || r.RVA >= hi {
			continue
		}
		off := int(r.RVA - lo)
		switch r.Type {
		case relBasedDir64:
			if off+8 > len(buf) {
				continue
			}
			v := binary.LittleEndian.Uint64(buf[off:]) + uint64(delta)
			binary.LittleEndian.PutUint64(buf[off:], v)
		case relBasedHighLow:
			if off+4 > len(buf) {
				continue
			}
			v := uint32(int64(binary.LittleEndian.Uint32(buf[off:])) + delta)
			binary.LittleEndian.PutUint32(buf[off:], v)
		default:
			// 其余类型（IA64/ARM 等）在 x64 上不该出现；出现就报错，不静默跳过
			if r.Type != relBasedAbsolute {
				return fmt.Errorf("PE: 不支持的 x64 重定位类型 %d", r.Type)
			}
		}
	}
	return nil
}
