package godonut

// 回归测试：validatePEReloc 必须在 GenShellcode 前拦截重定位表损坏的 PE，
// 防止 Binject/debug/pe 在 readBaseRelocationTable 中因 SizeOfBlock 下溢
// 分配 8GB+ 内存（服务端 OOM kill 的根因）。
import (
	"encoding/binary"
	"strings"
	"testing"
)

// buildTestPE 构造带 BASERELOC 目录的最小 PE32+ 骨架。
// relocBlocks 为原始 .reloc 节数据，ddSize 为数据目录声称的大小。
func buildTestPE(relocBlocks []byte, ddSize uint32) []byte {
	const (
		dosSize    = 0x40
		coffSize   = 20
		optSize    = 240 // PE32+ 标准可选头
		secHdrSize = 40
	)
	rawPtr := dosSize + 4 + coffSize + optSize + secHdrSize
	data := make([]byte, rawPtr+len(relocBlocks))

	copy(data[0:2], "MZ")
	binary.LittleEndian.PutUint32(data[0x3c:0x40], 0x40)
	copy(data[0x40:0x44], "PE\x00\x00")

	coff := 0x44
	binary.LittleEndian.PutUint16(data[coff:], 0x8664) // AMD64
	binary.LittleEndian.PutUint16(data[coff+2:], 1)    // 1 个节
	binary.LittleEndian.PutUint16(data[coff+16:], optSize)

	opt := coff + 20
	binary.LittleEndian.PutUint16(data[opt:], 0x20b) // PE32+ magic
	binary.LittleEndian.PutUint32(data[opt+108:], 16)
	dd := opt + 112
	binary.LittleEndian.PutUint32(data[dd+5*8:], 0x1000)      // BASERELOC VA
	binary.LittleEndian.PutUint32(data[dd+5*8+4:], ddSize)    // BASERELOC Size
	binary.LittleEndian.PutUint32(data[dd+5*8+4:], ddSize)

	sec := opt + optSize
	copy(data[sec:sec+6], ".reloc")
	binary.LittleEndian.PutUint32(data[sec+12:], 0x1000)                  // VirtualAddress
	binary.LittleEndian.PutUint32(data[sec+16:], uint32(len(relocBlocks))) // SizeOfRawData
	binary.LittleEndian.PutUint32(data[sec+20:], uint32(rawPtr))           // PointerToRawData
	copy(data[rawPtr:], relocBlocks)
	return data
}

func relocBlock(pageRVA, sizeOfBlock uint32, entries int) []byte {
	b := make([]byte, 8+entries*2)
	binary.LittleEndian.PutUint32(b[0:4], pageRVA)
	binary.LittleEndian.PutUint32(b[4:8], sizeOfBlock)
	return b
}

func TestValidatePERelocAcceptsValidBlocks(t *testing.T) {
	blocks := append(relocBlock(0x1000, 0x18, 8), relocBlock(0x2000, 0x10, 4)...)
	data := buildTestPE(blocks, uint32(len(blocks)))
	if err := validatePEReloc(data); err != nil {
		t.Fatalf("valid reloc must pass: %v", err)
	}
}

func TestValidatePERelocRejectsZeroSizeOfBlock(t *testing.T) {
	// SizeOfBlock=0 -> (0-8)/2 下溢为 0x7FFFFFFC，即 OOM 根因
	blocks := relocBlock(0x1000, 0, 0)
	data := buildTestPE(blocks, uint32(len(blocks)))
	err := validatePEReloc(data)
	if err == nil || !strings.Contains(err.Error(), "underflow") {
		t.Fatalf("SizeOfBlock=0 must be rejected with underflow error, got: %v", err)
	}
}

func TestValidatePERelocRejectsBlockBeyondSection(t *testing.T) {
	blocks := relocBlock(0x1000, 0x1000, 4) // 声称 4096 字节但只有 16 字节
	data := buildTestPE(blocks, uint32(len(blocks)))
	if err := validatePEReloc(data); err == nil {
		t.Fatal("block exceeding section data must be rejected")
	}
}

func TestValidatePERelocRejectsDirSizeBeyondSection(t *testing.T) {
	blocks := relocBlock(0x1000, 0x18, 8)
	data := buildTestPE(blocks, 0x10000) // dd.Size 远超节数据
	if err := validatePEReloc(data); err == nil {
		t.Fatal("dd.Size beyond section data must be rejected")
	}
}

func TestValidatePERelocRejectsNonPE(t *testing.T) {
	if err := validatePEReloc([]byte("ELF\x02\x01\x01")); err == nil {
		t.Fatal("non-PE must be rejected")
	}
}

func TestValidatePERelocEmptyDirOK(t *testing.T) {
	data := buildTestPE(nil, 0)
	if err := validatePEReloc(data); err != nil {
		t.Fatalf("empty BASERELOC must pass: %v", err)
	}
}
