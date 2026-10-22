//go:build windows
// +build windows

package commands

import (
	"encoding/binary"
	"testing"
)

// buildTestPE 构造一个最小 PE：可选头 PE32 + 一个节 +（可选）CLI 头与元数据根。
// 只用于验证 detectAssemblyRuntime 的解析路径，不要求能被 Windows 加载。
func buildTestPE(t *testing.T, version string, withCLI bool) []byte {
	t.Helper()
	data := make([]byte, 0x400)
	copy(data[0:2], "MZ")
	binary.LittleEndian.PutUint32(data[60:64], 0x40) // e_lfanew
	copy(data[0x40:0x44], []byte{'P', 'E', 0, 0})
	coff := 0x44
	binary.LittleEndian.PutUint16(data[coff+2:], 1)    // NumberOfSections
	binary.LittleEndian.PutUint16(data[coff+16:], 224) // SizeOfOptionalHeader
	opt := 0x40 + 24                                   // 0x58
	binary.LittleEndian.PutUint16(data[opt:], 0x10b)   // PE32

	cliDir := opt + 96 + 14*8 // DataDirectory[14] = CLI Header
	if withCLI {
		binary.LittleEndian.PutUint32(data[cliDir:], 0x1000) // RVA
		binary.LittleEndian.PutUint32(data[cliDir+4:], 0x48) // Size
	}

	sec := opt + 224
	binary.LittleEndian.PutUint32(data[sec+8:], 0x400)   // VirtualSize
	binary.LittleEndian.PutUint32(data[sec+12:], 0x1000) // VirtualAddress
	binary.LittleEndian.PutUint32(data[sec+16:], 0x400)  // SizeOfRawData
	binary.LittleEndian.PutUint32(data[sec+20:], 0x200)  // PointerToRawData

	if !withCLI {
		return data
	}
	// CLI 头：RVA 0x1000 → 文件偏移 0x200
	binary.LittleEndian.PutUint32(data[0x200:], 0x48)     // cb
	binary.LittleEndian.PutUint32(data[0x200+8:], 0x1100) // MetaData RVA
	binary.LittleEndian.PutUint32(data[0x200+12:], 0x100) // MetaData Size
	// 元数据根：RVA 0x1100 → 文件偏移 0x300
	copy(data[0x300:0x304], "BSJB")
	binary.LittleEndian.PutUint16(data[0x300+4:], 1) // MajorVersion
	binary.LittleEndian.PutUint16(data[0x300+6:], 1) // MinorVersion
	ver := append([]byte(version), 0)
	binary.LittleEndian.PutUint32(data[0x300+12:], uint32(len(ver))) // Length
	copy(data[0x300+16:], ver)
	return data
}

// TestDetectAssemblyRuntime 元数据版本串应精确解析为 v2/v4 家族。
func TestDetectAssemblyRuntime(t *testing.T) {
	cases := []struct{ version, family string }{
		{"v4.0.30319", "v4"},
		{"v2.0.50727", "v2"},
	}
	for _, tc := range cases {
		family, version := detectAssemblyRuntime(buildTestPE(t, tc.version, true))
		if family != tc.family || version != tc.version {
			t.Fatalf("%s: got family=%q version=%q", tc.version, family, version)
		}
	}

	// 原生 PE（无 CLI 头）不是 .NET 程序集
	if family, version := detectAssemblyRuntime(buildTestPE(t, "", false)); family != "" || version != "" {
		t.Fatalf("原生 PE 不应识别出 CLR: family=%q version=%q", family, version)
	}

	// 畸形/截断输入不能 panic
	for _, n := range []int{0, 1, 63, 64, 0x40, 0x58, 0x200, 0x300} {
		detectAssemblyRuntime(make([]byte, n))
	}
}

// TestCLRVersionOrder 目标 v2 的程序集先试 v2；其余（含未知）先试 v4。
func TestCLRVersionOrder(t *testing.T) {
	if got := clrVersionOrder("v2"); len(got) != 2 || got[0] != "v2" {
		t.Fatalf("v2 顺序错误: %v", got)
	}
	for _, family := range []string{"v4", ""} {
		if got := clrVersionOrder(family); len(got) != 2 || got[0] != "v4" {
			t.Fatalf("%q 顺序错误: %v", family, got)
		}
	}
}
