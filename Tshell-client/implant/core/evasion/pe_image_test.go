package evasion

import (
	"encoding/binary"
	"testing"
)

// buildTestPE 造一个最小 PE32+：两个节 + 一条 DIR64 重定位。
//
// 布局刻意让 **RVA ≠ 文件偏移**（.text: VA=0x1000 / raw=0x400），因为真 ntdll 就是
// 这样；早期 unhook 实现把 RVA 当文件偏移用，读到的全是别处的字节。
func buildTestPE(relocType uint16) []byte {
	const (
		e_lfanew    = 0x40
		optSize     = 240
		secTableOff = e_lfanew + 4 + 20 + optSize
		fileSize    = 0x800
	)
	data := make([]byte, fileSize)
	data[0], data[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(data[0x3C:], e_lfanew)
	copy(data[e_lfanew:], "PE\x00\x00")

	coff := e_lfanew + 4
	binary.LittleEndian.PutUint16(data[coff:], 0x8664) // Machine = x64
	binary.LittleEndian.PutUint16(data[coff+2:], 2)    // NumberOfSections
	binary.LittleEndian.PutUint16(data[coff+16:], optSize)

	opt := coff + 20
	binary.LittleEndian.PutUint16(data[opt:], 0x20B)          // PE32+
	binary.LittleEndian.PutUint64(data[opt+24:], 0x140000000) // ImageBase
	binary.LittleEndian.PutUint32(data[opt+112+5*8:], 0x2000) // BaseReloc RVA
	binary.LittleEndian.PutUint32(data[opt+112+5*8+4:], 12)   // BaseReloc Size

	// .text: VA 0x1000 / raw 0x400，可执行
	sec := data[secTableOff:]
	copy(sec[0:], ".text")
	binary.LittleEndian.PutUint32(sec[8:], 0x200)   // VirtualSize
	binary.LittleEndian.PutUint32(sec[12:], 0x1000) // VirtualAddress
	binary.LittleEndian.PutUint32(sec[16:], 0x200)  // SizeOfRawData
	binary.LittleEndian.PutUint32(sec[20:], 0x400)  // PointerToRawData
	binary.LittleEndian.PutUint32(sec[36:], scnMemExecute|0x60000020)

	// .rdata: VA 0x2000 / raw 0x600，不可执行
	sec2 := sec[40:]
	copy(sec2[0:], ".rdata")
	binary.LittleEndian.PutUint32(sec2[8:], 0x200)
	binary.LittleEndian.PutUint32(sec2[12:], 0x2000)
	binary.LittleEndian.PutUint32(sec2[16:], 0x200)
	binary.LittleEndian.PutUint32(sec2[20:], 0x600)
	binary.LittleEndian.PutUint32(sec2[36:], 0x40000040)

	// .text 先填成可辨识的图案
	for i := 0; i < 0x200; i++ {
		data[0x400+i] = 0x90 // nop 填充
	}
	// .text 里放一个绝对地址，等重定位去改它
	binary.LittleEndian.PutUint64(data[0x410:], 0x140001000)

	// 重定位块（在 .rdata 里）：pageRVA=0x1000，一条 entry → RVA 0x1010
	rel := data[0x600:]
	binary.LittleEndian.PutUint32(rel[0:], 0x1000)
	binary.LittleEndian.PutUint32(rel[4:], 12) // 8 字节头 + 2 字节 entry + 2 字节对齐
	binary.LittleEndian.PutUint16(rel[8:], relocType<<12|0x010)
	return data
}

func TestParsePEImageSections(t *testing.T) {
	img, err := ParsePEImage(buildTestPE(relBasedDir64))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if img.ImageBase != 0x140000000 {
		t.Fatalf("ImageBase = %#x", img.ImageBase)
	}
	if len(img.Sections) != 2 {
		t.Fatalf("节数 = %d，期望 2", len(img.Sections))
	}
	exec := img.ExecutableSections()
	if len(exec) != 1 || exec[0].Name != ".text" {
		t.Fatalf("可执行节 = %+v，期望只有 .text", exec)
	}
	if len(img.Relocs) != 1 || img.Relocs[0].RVA != 0x1010 || img.Relocs[0].Type != relBasedDir64 {
		t.Fatalf("重定位 = %+v，期望 1 条 0x1010/DIR64", img.Relocs)
	}
}

// TestFileOffsetForRVA 这条是那个真 bug 的回归：RVA 必须经节表换算成文件偏移
func TestFileOffsetForRVA(t *testing.T) {
	img, err := ParsePEImage(buildTestPE(relBasedDir64))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	off, ok := fileOffsetForRVA(img.Sections, 0x1010)
	if !ok {
		t.Fatal("0x1010 应该能映射")
	}
	if off != 0x410 {
		t.Fatalf("0x1010 -> 0x%x，期望 0x410（把 RVA 当偏移用会得到 0x1010）", off)
	}
	if _, ok := fileOffsetForRVA(img.Sections, 0x9000); ok {
		t.Fatal("超出所有节的 RVA 不该映射成功")
	}
}

func TestSectionImageNoDelta(t *testing.T) {
	data := buildTestPE(relBasedDir64)
	img, _ := ParsePEImage(data)
	sec := img.ExecutableSections()[0]
	buf, err := img.SectionImage(data, sec, 0)
	if err != nil {
		t.Fatalf("构造节镜像失败: %v", err)
	}
	if len(buf) != 0x200 {
		t.Fatalf("节长度 = %d，期望 0x200", len(buf))
	}
	if buf[0] != 0x90 {
		t.Fatalf("首字节 = 0x%x，期望 0x90（取自 raw 0x400）", buf[0])
	}
	if got := binary.LittleEndian.Uint64(buf[0x10:]); got != 0x140001000 {
		t.Fatalf("delta=0 时绝对地址不该变，得到 %#x", got)
	}
}

func TestSectionImageAppliesRelocations(t *testing.T) {
	data := buildTestPE(relBasedDir64)
	img, _ := ParsePEImage(data)
	sec := img.ExecutableSections()[0]
	buf, err := img.SectionImage(data, sec, 0x1000) // 实际基址比 ImageBase 高 0x1000
	if err != nil {
		t.Fatalf("构造节镜像失败: %v", err)
	}
	if got := binary.LittleEndian.Uint64(buf[0x10:]); got != 0x140002000 {
		t.Fatalf("重定位后 = %#x，期望 0x140002000", got)
	}
}

// TestSectionImageRejectsUnsupportedReloc 不认识的类型必须报错，不能猜
func TestSectionImageRejectsUnsupportedReloc(t *testing.T) {
	data := buildTestPE(7) // IMAGE_REL_BASED_IA64_IMM64，x64 上不该出现
	img, _ := ParsePEImage(data)
	sec := img.ExecutableSections()[0]
	if _, err := img.SectionImage(data, sec, 0x1000); err == nil {
		t.Fatal("不支持的 x64 重定位类型应该报错")
	}
}

// TestParsePEImageRejects 坏输入必须拒绝而不是产出半截镜像
func TestParsePEImageRejects(t *testing.T) {
	cases := map[string]func([]byte) []byte{
		"缺 MZ":     func(b []byte) []byte { b[0] = 0; return b },
		"节越界":      func(b []byte) []byte { binary.LittleEndian.PutUint32(b[0x40+4+20+240+20:], 0x10); return b[:0x500] },
		"PE32 不支持": func(b []byte) []byte { binary.LittleEndian.PutUint16(b[0x40+24:], 0x10B); return b },
	}
	for name, mut := range cases {
		if _, err := ParsePEImage(mut(buildTestPE(relBasedDir64))); err == nil {
			t.Errorf("%s：应该解析失败", name)
		}
	}
}
