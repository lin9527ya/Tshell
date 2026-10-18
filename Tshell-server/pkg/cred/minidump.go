package cred

import (
	"encoding/binary"
	"fmt"
	"os"
)

// Windows minidump 结构（见 MS 文档 MINIDUMP_HEADER / MINIDUMP_DIRECTORY）。
// 这里只做"这是不是一个完整可用的 minidump"的判定，不解析内存内容 ——
// 服务端不做凭据提取，解析交给操作员本地工具（pypykatz/mimikatz）。
const (
	minidumpSignature = 0x504D444D // "MDMP"
	minidumpVersion   = 0xA793     // MINIDUMP_VERSION
	headerSize        = 32
	directoryEntryLen = 12
	maxStreams        = 4096 // 目录条数上限（防御畸形头导致的大循环）
)

// minidump 流类型（只关心判定用得到的几个）
const (
	streamThreadList   = 3
	streamModuleList   = 4
	streamMemoryList   = 5
	streamSystemInfo   = 7
	streamMemory64List = 9
	streamHandleData   = 12
)

// DumpInfo 制品校验结果
type DumpInfo struct {
	Size          int64 `json:"size"`
	Streams       int   `json:"streams"`
	StreamTypes   []int `json:"stream_types,omitempty"`
	HasMemory     bool  `json:"has_memory"`      // 含内存流（MiniDumpWithFullMemory 的产物）
	HasSystemInfo bool  `json:"has_system_info"` // 含系统信息流（真转储都会有）
}

// ValidateMinidump 校验制品确实是完整的 Windows minidump。
//
// 为什么必须校验：客户端可能因为权限不足（PPL 保护的 LSASS）、磁盘写满或进程被杀
// 而只落下一个空文件/半截文件，然后照样经下载通道回传。不校验就会把它登记成
// "凭据制品"，操作员在解析时才发现是废文件 —— 凭证获取的失败必须是显式的。
func ValidateMinidump(path string) (DumpInfo, error) {
	var info DumpInfo
	st, err := os.Stat(path)
	if err != nil {
		return info, err
	}
	info.Size = st.Size()
	if st.Size() < headerSize {
		return info, fmt.Errorf("文件过小（%d 字节），不是有效的 minidump", st.Size())
	}

	f, err := os.Open(path)
	if err != nil {
		return info, err
	}
	defer f.Close()

	head := make([]byte, headerSize)
	if _, err := f.ReadAt(head, 0); err != nil {
		return info, fmt.Errorf("读取文件头失败: %w", err)
	}
	if binary.LittleEndian.Uint32(head[0:4]) != minidumpSignature {
		return info, fmt.Errorf("文件头不是 minidump 签名（MDMP）")
	}
	// 版本字段是两个 uint16：签名 + 版本号
	if binary.LittleEndian.Uint16(head[4:6]) != minidumpVersion {
		return info, fmt.Errorf("minidump 版本不符: 0x%X", binary.LittleEndian.Uint16(head[4:6]))
	}
	streamCount := binary.LittleEndian.Uint32(head[8:12])
	dirRVA := binary.LittleEndian.Uint32(head[12:16])
	if streamCount == 0 {
		return info, fmt.Errorf("minidump 不含任何数据流（转储未完成）")
	}
	if streamCount > maxStreams {
		return info, fmt.Errorf("minidump 目录条数异常: %d", streamCount)
	}
	info.Streams = int(streamCount)

	dirLen := int64(streamCount) * directoryEntryLen
	if int64(dirRVA)+dirLen > st.Size() {
		return info, fmt.Errorf("minidump 目录越界（文件被截断？）")
	}
	dir := make([]byte, dirLen)
	if _, err := f.ReadAt(dir, int64(dirRVA)); err != nil {
		return info, fmt.Errorf("读取流目录失败: %w", err)
	}

	for i := 0; i < int(streamCount); i++ {
		off := i * directoryEntryLen
		streamType := int(binary.LittleEndian.Uint32(dir[off : off+4]))
		dataSize := int64(binary.LittleEndian.Uint32(dir[off+4 : off+8]))
		rva := int64(binary.LittleEndian.Uint32(dir[off+8 : off+12]))
		if rva+dataSize > st.Size() {
			return info, fmt.Errorf("第 %d 个数据流越界（文件被截断？）", i)
		}
		info.StreamTypes = append(info.StreamTypes, streamType)
		switch streamType {
		case streamMemoryList, streamMemory64List:
			info.HasMemory = true
		case streamSystemInfo:
			info.HasSystemInfo = true
		}
	}
	// 真转储至少要有系统信息或模块列表之一；两者皆无说明这不是 MiniDumpWriteDump 的产物
	if !info.HasSystemInfo && !containsStream(info.StreamTypes, streamModuleList) {
		return info, fmt.Errorf("缺少系统信息/模块列表流，不是完整的进程转储")
	}
	return info, nil
}

func containsStream(types []int, want int) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

// Describe 生成给操作员看的一句话说明（写入制品备注）
func (d DumpInfo) Describe() string {
	kind := "仅句柄/线程等元数据"
	if d.HasMemory {
		kind = "含完整内存"
	}
	return fmt.Sprintf("%.1f MB / %d 个数据流 / %s", float64(d.Size)/1024/1024, d.Streams, kind)
}

var _ = streamThreadList
var _ = streamHandleData
