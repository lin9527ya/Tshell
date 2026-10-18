package otp

import (
	"fmt"
	"strings"
)

// ===== 测试用解码器 =====
//
// 存在的理由：QR 编码器没有"参考实现"可以对拍，而"我只检查了矩阵形状"这种测试
// 挡不住真正的错误（位序写反、掩码条件写错、格式信息方向反了 —— 这些都不会改变
// 矩阵尺寸）。这里按标准逆向读一遍：解掩码 → 按 zigzag 取回码字 → 分块取数据 →
// 解析模式/长度/内容。
//
// 往返能过，说明"选版本、位流、分块交织、纠错、掩码、格式信息"整条链自洽。
// 它证明不了"我实现的和标准字节级一致"（那需要真实扫描器），但能挡住绝大多数
// 内部不一致 —— 而没有这段代码，那些不一致根本不会被发现。

// qrDecode 从矩阵读回原始文本（仅支持本包生成的字节模式，纠错 M）
func qrDecode(m *qrMatrix) (string, error) {
	version, err := qrVersionOf(m.size)
	if err != nil {
		return "", err
	}
	level, mask, err := readFormatInfo(m)
	if err != nil {
		return "", err
	}
	if level != ECLMedium {
		return "", fmt.Errorf("解码器只支持纠错级别 M，实际读到 %v", level)
	}

	// 保留区必须与编码端**共用同一个函数**得到。
	// 这里曾经另写了一套简化版，少了 123 个格子，解码器把功能图形当数据读，
	// 掩码选错、整条链全错（症状是"读回 331 比特，期望 208"）。
	reserved := newQRMatrix(m.size)
	reserveFunctionModules(reserved, version)

	// 解掩码（只作用于数据区）
	unmasked := m.clone()
	applyMask(unmasked, reserved, mask)

	// 按 zigzag 顺序取回比特
	bits := readDataBits(unmasked, reserved, version)
	if len(bits) != totalCodewords[version]*8+remainderBits[version] {
		return "", fmt.Errorf("读回的比特数 %d 与版本 %d 期望的 %d 不符",
			len(bits), version, totalCodewords[version]*8+remainderBits[version])
	}

	// 数据码字 → 分块还原 → 拼接
	dataCw, err := extractDataCodewords(version, bits)
	if err != nil {
		return "", err
	}
	return parseByteModePayload(dataCw, version)
}

func qrVersionOf(size int) (int, error) {
	if size < 21 || (size-17)%4 != 0 {
		return 0, fmt.Errorf("矩阵尺寸非法: %d", size)
	}
	v := (size - 17) / 4
	if v < 1 || v > maxQRVersion {
		return 0, fmt.Errorf("不支持的版本: %d", v)
	}
	return v, nil
}

// readFormatInfo 读第一份格式信息副本并还原纠错级别与掩码
func readFormatInfo(m *qrMatrix) (ECLevel, int, error) {
	// 读回 15 位格式信息。位置与 drawFormatInfo 的同一套映射：
	// 低 8 位在 (8,0..5)、(8,7..8)，第 8..14 位在 (7,8)、(5,8)、(4,8)…(0,8)。
	// 注意 (8,6) 与 (6,8) 分别是定时图形的行列，不是格式位。
	bits := uint32(0)
	setBit := func(i int, dark bool) {
		if dark {
			bits |= 1 << uint(i)
		}
	}
	for i := 0; i < 6; i++ {
		setBit(i, m.at(8, i))
	}
	setBit(6, m.at(8, 7))
	setBit(7, m.at(8, 8))
	setBit(8, m.at(7, 8))
	setBit(9, m.at(5, 8))
	for i := 10; i < 15; i++ {
		setBit(i, m.at(14-i, 8))
	}
	// 去掉固定掩码。0x5412 是"对 M 级别 + 掩码 0 的整串 15 位"的恒定异或值，
	// 异或后的整串里高 5 位才是 (级别2位 ‖ 掩码3位) —— 必须在整串上先右移 10 位
	// 再取位。直接在整串上取位会把级别读错（本测试第一版就是这么错的，
	// 症状是 M 被读成 H，编码器其实是对的）。
	fmtData := (bits ^ 0x5412) >> 10
	mask := int(fmtData & 0b111)
	levelBits := (fmtData >> 3) & 0b11
	switch levelBits {
	case 0b00:
		return ECLMedium, mask, nil
	case 0b01:
		return ECLevel(1), mask, nil // L
	case 0b10:
		return ECLevel(2), mask, nil // H
	default:
		return ECLevel(3), mask, nil // Q
	}
}

// readDataBits 按 zigzag 顺序读数据区的比特（含末尾补位）
func readDataBits(m, reserved *qrMatrix, version int) []bool {
	size := m.size
	wantBits := totalCodewords[version]*8 + remainderBits[version]
	out := make([]bool, 0, wantBits)

	upward := true
	for right := size - 1; right >= 1; right -= 2 {
		// 与编码端同一套列配对规则：游标降到 5，保证最后一对是 (0,1)
		if right == 6 {
			right--
		}
		for i := 0; i < size; i++ {
			y := i
			if upward {
				y = size - 1 - i
			}
			for dx := 0; dx < 2; dx++ {
				x := right - dx
				if reserved.at(x, y) {
					continue
				}
				out = append(out, m.at(x, y))
			}
		}
		upward = !upward
	}
	return out
}

// extractDataCodewords 从比特流取回交织后的码字，再去交织还原原始数据码字
func extractDataCodewords(version int, bits []bool) ([]byte, error) {
	total := totalCodewords[version]
	if len(bits) < total*8 {
		return nil, fmt.Errorf("数据比特不足：%d < %d", len(bits), total*8)
	}
	raw := make([]byte, total)
	for i := 0; i < total*8; i++ {
		if bits[i] {
			raw[i/8] |= 1 << uint(7-i%8)
		}
	}

	groups := ecMTables[version]
	blockCount := 0
	maxData := 0
	for _, g := range groups {
		blockCount += g.blocks
		if g.dataCw > maxData {
			maxData = g.dataCw
		}
	}
	totalData := 0
	for _, g := range groups {
		totalData += g.blocks * g.dataCw
	}

	// 重建各块的数据部分（交织顺序：逐列取每块第 i 个码字，短块先取完）
	dataBlocks := make([][]byte, blockCount)
	for i := range dataBlocks {
		dataBlocks[i] = make([]byte, 0, maxData)
	}
	// 每块的数据长度（按组顺序展开）
	lens := make([]int, 0, blockCount)
	for _, g := range groups {
		for i := 0; i < g.blocks; i++ {
			lens = append(lens, g.dataCw)
		}
	}
	pos := 0
	for i := 0; i < maxData; i++ {
		for b := 0; b < blockCount; b++ {
			if i < lens[b] {
				if pos >= totalData {
					return nil, fmt.Errorf("交织数据越界")
				}
				dataBlocks[b] = append(dataBlocks[b], raw[pos])
				pos++
			}
		}
	}
	out := make([]byte, 0, totalData)
	for _, b := range dataBlocks {
		out = append(out, b...)
	}
	return out, nil
}

// parseByteModePayload 解析字节模式：模式(4) + 字符计数(8/16) + 内容
func parseByteModePayload(data []byte, version int) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("数据码字为空")
	}
	bitAt := func(i int) uint32 {
		if i/8 >= len(data) {
			return 0
		}
		return uint32((data[i/8] >> uint(7-i%8)) & 1)
	}
	read := func(start, n int) uint32 {
		var v uint32
		for i := 0; i < n; i++ {
			v = v<<1 | bitAt(start+i)
		}
		return v
	}

	mode := read(0, 4)
	if mode != 0b0100 {
		return "", fmt.Errorf("模式不是字节模式: %b", mode)
	}
	var countBits, header int
	if version <= 9 {
		countBits, header = 8, 4+8
	} else {
		countBits, header = 16, 4+16
	}
	n := int(read(4, countBits))
	if n < 0 || header+n*8 > len(data)*8 {
		return "", fmt.Errorf("字符计数越界: %d", n)
	}
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(byte(read(header+i*8, 8)))
	}
	return sb.String(), nil
}
