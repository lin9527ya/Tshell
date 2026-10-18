package otp

// QR 编码器（字节模式，纠错级别 M，版本 1–10）。
//
// 为什么只做到版本 10：本仓库唯一的用途是把 otpauth:// 链接交给认证器 App。
// 版本 10 / M 能装 213 字节，而 otpauth 链接通常 <120 字节 —— 再往上加版本，
// 收益只是"更长的链接"，代价是数据表翻倍、出错面变大。
//
// 表来源：ISO/IEC 18004 的公开常量（对齐图形坐标、每版本纠错块结构）。
// 正确性由 qrcode_test.go 的"编码→解码往返"保证：往返能过，说明模式/计数/填充/
// 纠错/交织/掩码/格式位整条链自洽；表若是错的，往返必然失败。

import (
	"fmt"
	"strings"
)

// ECLevel 纠错级别
type ECLevel int

// 只实现 M（约 15% 冗余）：认证器在手机屏幕上扫描，M 是通行折中
const ECLMedium ECLevel = 0

// qrBlockGroup 纠错块结构的一组：blocks 块、每块 dataCw 个数据码字、每块 ecPerBlk 个纠错码字。
//
// 表来源：ISO/IEC 18004 的纠错块表（纠错级别 M）。
// 校验方式：Σ blocks*(dataCw+ecPerBlk) 必须等于该版本的总码字数
// （见 TestBlockTableMatchesCodewordTotals），并且"可用数据模块数"必须等于
// 总码字数*8 + 补位数（见 TestDataModuleCapacityPerVersion）。
// 两张表互相约束，转录错了会被测试直接顶出来 —— V7 就曾被我写成 4×27（正确是 4×31，
// 差 16 个码字），症状是"矩阵尺寸对、内容错位"。
type qrBlockGroup struct {
	blocks   int
	dataCw   int
	ecPerBlk int // 同版本各级别下所有块相同
}

var ecMTables = map[int][]qrBlockGroup{
	1:  {{1, 16, 10}},
	2:  {{1, 28, 16}},
	3:  {{1, 44, 26}},
	4:  {{2, 32, 18}},
	5:  {{2, 43, 24}},
	6:  {{4, 27, 16}},
	7:  {{4, 31, 18}},
	8:  {{2, 38, 22}, {2, 39, 22}},
	9:  {{3, 36, 22}, {2, 37, 22}},
	10: {{4, 43, 26}, {1, 44, 26}},
}

// alignPositions 各版本的对齐图形中心坐标（ISO/IEC 18004 附录 E）
var alignPositions = map[int][]int{
	1:  {},
	2:  {6, 18},
	3:  {6, 22},
	4:  {6, 26},
	5:  {6, 30},
	6:  {6, 34},
	7:  {6, 22, 38},
	8:  {6, 24, 42},
	9:  {6, 26, 46},
	10: {6, 28, 50},
}

// remainderBits 各版本在矩阵末尾剩余的补位比特数（矩阵对齐用，不是码字）
var remainderBits = map[int]int{
	1: 0, 2: 7, 3: 7, 4: 7, 5: 7, 6: 7,
	7: 0, 8: 0, 9: 0, 10: 0,
}

// totalCodewords 各版本总码字数（数据 + 纠错）
var totalCodewords = map[int]int{
	1: 26, 2: 44, 3: 70, 4: 100, 5: 134,
	6: 172, 7: 196, 8: 242, 9: 292, 10: 346,
}

// maxQRVersion 支持的最高版本
const maxQRVersion = 10

// qrMatrix 编码结果：true = 深色模块
type qrMatrix struct {
	size int
	mods [][]bool
}

func newQRMatrix(size int) *qrMatrix {
	m := &qrMatrix{size: size, mods: make([][]bool, size)}
	for i := range m.mods {
		m.mods[i] = make([]bool, size)
	}
	return m
}

func (m *qrMatrix) set(x, y int, dark bool) {
	if x < 0 || y < 0 || x >= m.size || y >= m.size {
		return
	}
	m.mods[y][x] = dark
}

func (m *qrMatrix) at(x, y int) bool {
	if x < 0 || y < 0 || x >= m.size || y >= m.size {
		return false
	}
	return m.mods[y][x]
}

// EncodeQR 把文本编码成 QR 矩阵（纠错级别 M）。
// 返回 nil 表示文本超出支持范围（调用方应给出可读错误）。
func EncodeQR(text string) (*qrMatrix, error) {
	data := []byte(text)

	// 1) 选版本：字节模式开销 = 4 位模式 + 8/16 位字符计数
	version := 0
	for v := 1; v <= maxQRVersion; v++ {
		capacity := dataCapacityBytes(v, ECLMedium)
		if capacity <= 0 {
			continue
		}
		if len(data) <= capacity {
			version = v
			break
		}
	}
	if version == 0 {
		return nil, fmt.Errorf("文本过长（%d 字节），版本 1–%d(纠错M) 最多容纳 %d 字节",
			len(data), maxQRVersion, dataCapacityBytes(maxQRVersion, ECLMedium))
	}

	// 2) 位流：模式(0100) + 字符计数 + 数据 + 结束符 + 补齐
	bits := &bitBuffer{}
	bits.append(0b0100, 4)
	// 版本 1–9 用 8 位字符计数，10–26 用 16 位
	if version <= 9 {
		bits.append(uint32(len(data)), 8)
	} else {
		bits.append(uint32(len(data)), 16)
	}
	for _, b := range data {
		bits.append(uint32(b), 8)
	}

	totalDataCw := 0
	for _, g := range ecMTables[version] {
		totalDataCw += g.blocks * g.dataCw
	}
	wantBits := totalDataCw * 8

	// 结束符：最多 4 个 0，但不能越过容量
	term := 4
	if wantBits-bits.len() < term {
		term = wantBits - bits.len()
	}
	bits.append(0, term)
	// 补到字节边界
	if rem := bits.len() % 8; rem != 0 {
		bits.append(0, 8-rem)
	}
	// 交替填充 0xEC / 0x11
	for i := 0; bits.len() < wantBits; i++ {
		if i%2 == 0 {
			bits.append(0xEC, 8)
		} else {
			bits.append(0x11, 8)
		}
	}

	dataCw := bits.bytes()

	// 3) 分块 + 逐块纠错 + 交织
	// 注意：第二个返回值是**交织后的纠错码字**，不是"每块纠错码字数"。
	// 两者混淆过一次（把数量当字节拼进码字流），症状是矩阵尺寸对但内容全错。
	dataInterleaved, ecInterleaved, err := splitAndInterleave(version, dataCw)
	if err != nil {
		return nil, err
	}

	// 4) 画功能图形
	size := version*4 + 17
	m := newQRMatrix(size)
	reserved := newQRMatrix(size) // 记录已占用的模块（数据区不能覆盖功能图形）
	drawFunctionPatterns(m, reserved, version)

	// 5) 放置数据（zigzag：从右下角起，两列一组，向上/向下交替）
	// 码字流顺序：先全部数据码字，再全部纠错码字（各自已交织）
	codewords := make([]byte, 0, len(dataInterleaved)+len(ecInterleaved))
	codewords = append(codewords, dataInterleaved...)
	codewords = append(codewords, ecInterleaved...)
	if err := placeData(m, reserved, version, codewords, remainderBits[version]); err != nil {
		return nil, err
	}

	// 6) 选最优掩码（8 种，按标准罚分取最低）
	bestScore := -1
	var bestMatrix *qrMatrix
	for mask := 0; mask < 8; mask++ {
		cand := m.clone()
		applyMask(cand, reserved, mask)
		drawFormatInfo(cand, ECLMedium, mask)
		if version >= 7 {
			drawVersionInfo(cand, version)
		}
		if score := penalty(cand); bestScore < 0 || score < bestScore {
			bestScore, bestMatrix = score, cand
		}
	}
	if bestMatrix == nil {
		return nil, fmt.Errorf("二维码编码失败：没有可用掩码")
	}
	return bestMatrix, nil
}

// dataCapacityBytes 该版本在字节模式下能装多少字节。
//
// 算法：数据比特 = 数据码字数 * 8；每字节 8 位，外加模式指示符(4 位)与
// 字符计数(版本 ≤9 时 8 位，否则 16 位)。
// 数据码字数与"可用模块几何"必须一致 —— 由 TestDataModuleCapacityPerVersion
// 与 TestBlockTableMatchesCodewordTotals 两面夹住，转录错会被直接顶出来。
func dataCapacityBytes(version int, level ECLevel) int {
	total := totalDataCodewords(version)
	if total == 0 {
		return 0
	}
	overheadBits := 4
	if version <= 9 {
		overheadBits += 8
	} else {
		overheadBits += 16
	}
	if total*8 <= overheadBits {
		return 0
	}
	_ = level
	return (total*8 - overheadBits) / 8
}

// totalDataCodewords 该版本(纠错 M)的数据码字总数
func totalDataCodewords(version int) int {
	total := 0
	for _, g := range ecMTables[version] {
		total += g.blocks * g.dataCw
	}
	return total
}

// ===== 位流 =====

type bitBuffer struct {
	buf []byte
	n   int // 已写入的比特数
}

func (b *bitBuffer) append(v uint32, bits int) {
	for i := bits - 1; i >= 0; i-- {
		bit := (v >> uint(i)) & 1
		if b.n%8 == 0 {
			b.buf = append(b.buf, 0)
		}
		if bit == 1 {
			b.buf[b.n/8] |= 1 << uint(7-b.n%8)
		}
		b.n++
	}
}

func (b *bitBuffer) len() int { return b.n }

func (b *bitBuffer) bytes() []byte { return b.buf }

// ===== 纠错（GF(256)，本原多项式 0x11D）=====

var (
	gfExp [512]byte
	gfLog [256]byte
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	// 扩展一倍，省掉乘法里的取模
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[int(gfLog[a])+int(gfLog[b])]
}

// rsGenerator 生成 degree 次的 RS 生成多项式
func rsGenerator(degree int) []byte {
	g := []byte{1}
	for i := 0; i < degree; i++ {
		next := make([]byte, len(g)+1)
		for j, c := range g {
			next[j] ^= gfMul(c, 1)
			next[j+1] ^= gfMul(c, gfExp[i])
		}
		g = next
	}
	return g
}

// rsEncode 计算 data 的纠错码字（多项式除法取余）
func rsEncode(data []byte, ecLen int) []byte {
	gen := rsGenerator(ecLen)
	res := make([]byte, ecLen)
	for _, d := range data {
		factor := d ^ res[0]
		copy(res, res[1:])
		res[ecLen-1] = 0
		if factor != 0 {
			for i := 0; i < ecLen; i++ {
				res[i] ^= gfMul(gen[i+1], factor)
			}
		}
	}
	return res
}

// splitAndInterleave 把数据码字分块、逐块算纠错、再按标准交织
func splitAndInterleave(version int, data []byte) (dataInterleaved, ecInterleaved []byte, err error) {
	groups := ecMTables[version]
	expected := 0
	for _, g := range groups {
		expected += g.blocks * g.dataCw
	}
	if len(data) != expected {
		return nil, nil, fmt.Errorf("内部错误：数据码字数 %d 与版本 %d 期望的 %d 不符", len(data), version, expected)
	}

	type block struct {
		data []byte
		ec   []byte
	}
	var blocks []block
	pos := 0
	ecPerBlock := groups[0].ecPerBlk
	for _, g := range groups {
		for i := 0; i < g.blocks; i++ {
			d := make([]byte, g.dataCw)
			copy(d, data[pos:pos+g.dataCw])
			pos += g.dataCw
			blocks = append(blocks, block{data: d, ec: rsEncode(d, ecPerBlock)})
		}
	}

	// 数据交织：逐列取每块的第 i 个码字（短块先取完）
	maxData := 0
	for _, b := range blocks {
		if len(b.data) > maxData {
			maxData = len(b.data)
		}
	}
	for i := 0; i < maxData; i++ {
		for _, b := range blocks {
			if i < len(b.data) {
				dataInterleaved = append(dataInterleaved, b.data[i])
			}
		}
	}
	// 纠错交织：所有块的纠错长度相同
	for i := 0; i < ecPerBlock; i++ {
		for _, b := range blocks {
			ecInterleaved = append(ecInterleaved, b.ec[i])
		}
	}
	return dataInterleaved, ecInterleaved, nil
}

// ===== 矩阵绘制 =====

// reserveFunctionModules 标记"功能图形/非数据"占用的模块。
//
// **编码与解码共用这一份**：曾经解码侧另写一套简化版，结果保留区少了 123 个格子，
// 解码器把功能图形当数据读、掩码选错、整条链全错（症状是"比特数 331 vs 期望 208"）。
// 只要两边共用，就不存在"两套保留规则悄悄漂移"的可能。
func reserveFunctionModules(reserved *qrMatrix, version int) {
	size := reserved.size

	// 三个定位图形 + 分隔符：各占一个 8x8 角块
	markCorner := func(ox, oy int) {
		for dy := 0; dy <= 7; dy++ {
			for dx := 0; dx <= 7; dx++ {
				reserved.set(ox+dx, oy+dy, true)
			}
		}
	}
	markCorner(0, 0)
	markCorner(size-8, 0)
	markCorner(0, size-8)

	// 定时图形：整行 6 与整列 6
	for i := 0; i < size; i++ {
		reserved.set(6, i, true)
		reserved.set(i, 6, true)
	}

	// 对齐图形
	for _, cy := range alignPositions[version] {
		for _, cx := range alignPositions[version] {
			if overlapsFinderArea(cx, cy, size) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					reserved.set(cx+dx, cy+dy, true)
				}
			}
		}
	}

	// 格式信息：第一份在 (0..8,8) 与 (8,0..8)（跳定时行列），第二份在右下与左下
	for i := 0; i <= 8; i++ {
		if i != 6 {
			reserved.set(i, 8, true)
			reserved.set(8, i, true)
		}
	}
	for i := 0; i < 8; i++ {
		reserved.set(size-1-i, 8, true)
		reserved.set(8, size-1-i, true)
	}
	// 固定深色模块
	reserved.set(8, size-8, true)

	// 版本信息（版本 ≥7）：两块 6x3 区域
	if version >= 7 {
		for i := 0; i < 6; i++ {
			for j := 0; j < 3; j++ {
				reserved.set(size-11+j, i, true)
				reserved.set(i, size-11+j, true)
			}
		}
	}
}

func drawFunctionPatterns(m, reserved *qrMatrix, version int) {
	size := m.size

	// 三个定位图形（含分隔符）
	drawFinder := func(cx, cy int) {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := cx+dx, cy+dy
				if x < 0 || y < 0 || x >= size || y >= size {
					continue
				}
				// 7x7 定位图形的标准形状（注意边界裁剪，|dx| 或 |dy| 到 4 的格子
				// 属于分隔符，必须是浅色）：
				//   外框(距离 3)深色、其内一圈(距离 2)浅色、中心 3x3 深色。
				// 距离 2 那一圈写成深色会得到一个 7x7 实心方块，扫描器认不出定位图形。
				ax, ay := abs(dx), abs(dy)
				dark := false
				if ax <= 3 && ay <= 3 {
					dark = (ax == 3 || ay == 3) || (ax <= 1 && ay <= 1)
				}
				m.set(x, y, dark)
			}
		}
	}
	drawFinder(3, 3)
	drawFinder(size-4, 3)
	drawFinder(3, size-4)

	// 定时图形
	for i := 0; i < size; i++ {
		if !reserved.at(6, i) {
			m.set(6, i, i%2 == 0)
		}
		if !reserved.at(i, 6) {
			m.set(i, 6, i%2 == 0)
		}
	}

	// 对齐图形（5x5：外框深色、内圈浅色、中心深色）。
	//
	// 跳过规则必须**只**看三个定位图形及其分隔符占用的 8x8 角块 —— 不能简单地
	// "中心格已被占用就跳过"。v7 起会出现中心落在第 6 行/列的合法对齐图形
	// （如 (22,6)、(6,22)）：它们与定时图形有 5 格重叠，但**必须画**，
	// 否则少 2*25-10=40 个模块，可用模块数比标准多 40，
	// 症状是"能编码、码字放得下、却整块错位"。
	for _, cy := range alignPositions[version] {
		for _, cx := range alignPositions[version] {
			if overlapsFinderArea(cx, cy, size) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					ax, ay := abs(dx), abs(dy)
					dark := ax == 2 || ay == 2 || (ax == 0 && ay == 0)
					m.set(cx+dx, cy+dy, dark)
				}
			}
		}
	}

	// 固定的深色模块
	m.set(8, size-8, true)

	// 保留区统一由 reserveFunctionModules 标出（编码/解码共用同一份规则）
	reserveFunctionModules(reserved, version)
}

// overlapsFinderArea 判断以 (cx,cy) 为中心的对齐图形是否会压到定位图形/分隔符。
// 三个定位图形（含分隔符）各占一个 8x8 角块；对齐图形半宽 2，
// 所以中心落在 [0,9] 或 [size-10, size-1] 的角落区间内就算重叠。
func overlapsFinderArea(cx, cy, size int) bool {
	const limit = 9 // 0..7 是定位图形+分隔符，再多留 2 格作为对齐图形自身的半径
	inTopLeft := cx <= limit && cy <= limit
	inTopRight := cx >= size-1-limit && cy <= limit
	inBottomLeft := cx <= limit && cy >= size-1-limit
	return inTopLeft || inTopRight || inBottomLeft
}

// placeData 按标准 zigzag 顺序放置交织后的码字。
//
// 两道前置检查（都返回错误而不是静默填一半）：
//  1. 传入的码字总数必须等于该版本的标准总码字数；
//  2. 可用数据模块数必须等于 码字比特数 + 补位数。
//
// 少了任何一道，一处表错就会产出"尺寸正常、内容错位"的矩阵 ——
// 那是最难查的一类故障（扫码器只是"读不出来"）。
func placeData(m, reserved *qrMatrix, version int, codewords []byte, remainders int) error {
	if want := totalCodewords[version]; len(codewords) != want {
		return fmt.Errorf("内部错误：版本 %d 的码字数为 %d，期望 %d", version, len(codewords), want)
	}
	size := m.size
	bitIdx := 0
	totalBits := len(codewords)*8 + remainders

	// 数一遍可用模块，把布局错误挡在写之前
	free := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if !reserved.at(x, y) {
				free++
			}
		}
	}
	if free != totalBits {
		return fmt.Errorf("内部错误：版本 %d 的可用数据模块 %d 与码字比特数 %d 不符（差 %d）——"+
			"功能图形布局有误", version, free, totalBits, free-totalBits)
	}

	// 比特读取器：**必须**以"码字自带的比特数"（len(codewords)*8）为硬上限，
	// 不能以 totalBits(= 码字比特 + 补位数) 为上限 —— 补位没有对应的数据来源，
	// 拿它当上限会多读那几个比特，越界读到切片之外。补位一律填 0（标准要求）。
	dataBits := len(codewords) * 8
	nextBit := func() bool {
		if bitIdx >= dataBits {
			return false
		}
		b := codewords[bitIdx/8]
		bit := (b >> uint(7-bitIdx%8)) & 1
		bitIdx++
		return bit == 1
	}

	// 自由格必须被**恰好**遍历一遍。用"访问计数"而不是只看总数：
	// 总数相等但遍历顺序有误（例如某列被跳过两次、另一列被覆盖）时，
	// 矩阵照样能画出来、尺寸也对，但内容整块错位 —— 扫码器只会"读不出来"。
	visited := 0

	upward := true
	for right := size - 1; right >= 1; right -= 2 {
		// 第 6 列是定位图形（定时图形）所在的整列，不能作数据列。
		// 标准做法：把它并入 (5,6) 这一对的一侧，并把**游标**降到 5 ——
		// 这样下一对才是 (3,4)，最后一对能落到 (0,1)。
		// 若只写 `right = 5`（游标仍停在 6，下一轮减 2 得 4），配对会变成
		// (3,4)、(1,2)，**第 0 列永远不被访问**：矩阵少填一列，
		// 而"可用模块数"是对的 —— 症状正是越界写穿。
		if right == 6 {
			right-- // 6 → 5，使本轮配对为 (5,4)
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
				visited++
				if visited > totalBits {
					return fmt.Errorf("内部错误：版本 %d 的数据区遍历超出 %d 个模块"+
						"（遍历顺序与保留布局不一致）", version, totalBits)
				}
				m.set(x, y, nextBit())
			}
		}
		upward = !upward
	}
	if visited != totalBits {
		return fmt.Errorf("内部错误：版本 %d 实际遍历 %d 个模块，期望 %d",
			version, visited, totalBits)
	}
	return nil
}

func applyMask(m, reserved *qrMatrix, mask int) {
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if reserved.at(x, y) {
				continue
			}
			if maskCondition(mask, x, y) {
				m.mods[y][x] = !m.mods[y][x]
			}
		}
	}
}

// maskCondition 8 种掩码条件（ISO/IEC 18004 表 10）
func maskCondition(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (y/2+x/3)%2 == 0
	case 5:
		return (x*y)%2+(x*y)%3 == 0
	case 6:
		return ((x*y)%2+(x*y)%3)%2 == 0
	case 7:
		return ((x+y)%2+(x*y)%3)%2 == 0
	}
	return false
}

// drawFormatInfo 写入格式信息（纠错级别 + 掩码），带 BCH(15,5) 纠错与固定掩码。
//
// 两份副本的位置是标准硬性规定，且**两份的位序方向相反**（第一份从右上往左下、
// 第二份从左上往右下），写反了矩阵看起来正常但任何扫描器都读不出来：
//
//	第一份：bit i 同时落在 (8, y) 竖条与 (x, 8) 横条上
//	第二份：bit i 落在右下角 (8, size-7+i) 与左下角 (size-1-i, 8)
func drawFormatInfo(m *qrMatrix, level ECLevel, mask int) {
	// level 的两位编码：标准取值为 L=01, M=00, Q=11, H=10
	levelBits := map[ECLevel]uint32{ECLMedium: 0b00}[level]
	data := levelBits<<3 | uint32(mask)

	// BCH(15,5)：生成多项式 0x537，最后与固定掩码 0x5412 异或
	rem := data << 10
	for i := 14; i >= 10; i-- {
		if (rem>>uint(i))&1 == 1 {
			rem ^= 0x537 << uint(i-10)
		}
	}
	bits := (data<<10 | rem) ^ 0x5412

	size := m.size
	bitAt := func(i int) bool { return (bits>>uint(i))&1 == 1 }

	// 第一份副本（竖条 x=8 与横条 y=8）。
	//
	// 位序是标准硬性规定，写反了整个码就"看着正常但读不出来"：
	//   竖条：bit 0..5 → (8,0..5)，bit 6..7 → (8,7..8)，bit 8..14 → (8,9..15 方向)
	// 注意**只有 14 位**走 y=i+1 的跳行映射；bit 14 单独落在 (8, size-8)，
	// 那里同时也是标准的"固定深色模块"位置。
	for i := 0; i < 14; i++ {
		y := i
		if i >= 6 {
			y = i + 1
		}
		m.set(8, y, bitAt(i))
	}
	m.set(8, size-8, bitAt(14))

	// 横条：bit i → (size-1-i, 8)，跳过第 6 列
	for i := 0; i < 15; i++ {
		x := size - 1 - i
		if i >= 8 {
			x = 14 - i
		}
		m.set(x, 8, bitAt(i))
	}

	// 第二份副本：右下角竖条与左下角横条（位序与第一份相反）
	for i := 0; i < 8; i++ {
		m.set(8, size-1-i, bitAt(i))    // (8, size-1) ← bit 0 … (8, size-8) ← bit 7
		m.set(size-1-i, 8, bitAt(14-i)) // (size-1, 8) ← bit 14 … (size-8, 8) ← bit 7
	}

	// 固定深色模块（标准要求始终深色，覆盖上面写入的 bit 14）
	m.set(8, size-8, true)
}

// drawVersionInfo 写入版本信息（版本 ≥7），BCH(18,6)，生成多项式 0x1F25
func drawVersionInfo(m *qrMatrix, version int) {
	size := m.size
	rem := uint32(version) << 12
	for i := 17; i >= 12; i-- {
		if (rem>>uint(i))&1 == 1 {
			rem ^= 0x1F25 << uint(i-12)
		}
	}
	bits := uint32(version)<<12 | rem

	for i := 0; i < 18; i++ {
		bit := (bits>>uint(i))&1 == 1
		x := size - 11 + i%3
		y := i / 3
		m.set(x, y, bit)
		m.set(y, x, bit)
	}
}

// penalty 标准四条例子的罚分（掩码选择用）
func penalty(m *qrMatrix) int {
	size := m.size
	score := 0

	// 规则 1：同色连续 ≥5
	countRun := func(get func(i, j int) bool) {
		for i := 0; i < size; i++ {
			run, prev := 0, false
			for j := 0; j < size; j++ {
				cur := get(i, j)
				if j == 0 || cur == prev {
					run++
				} else {
					if run >= 5 {
						score += 3 + (run - 5)
					}
					run = 1
				}
				prev = cur
			}
			if run >= 5 {
				score += 3 + (run - 5)
			}
		}
	}
	countRun(func(i, j int) bool { return m.at(j, i) }) // 行
	countRun(func(i, j int) bool { return m.at(i, j) }) // 列

	// 规则 2：2x2 同色块
	for y := 0; y < size-1; y++ {
		for x := 0; x < size-1; x++ {
			v := m.at(x, y)
			if m.at(x+1, y) == v && m.at(x, y+1) == v && m.at(x+1, y+1) == v {
				score += 3
			}
		}
	}

	// 规则 3：出现 1011101 0000 或 0000 1011101 这类"像定位图形"的模式
	pattern1 := []bool{true, false, true, true, true, false, true, false, false, false, false}
	pattern2 := []bool{false, false, false, false, true, false, true, true, true, false, true}
	// get(i, j) 抽象出"按行扫/按列扫"，避免把同样的比较写两遍
	matchAt := func(get func(i, j int) bool, i, start int, pat []bool) bool {
		for k, want := range pat {
			if get(i, start+k) != want {
				return false
			}
		}
		return true
	}
	for i := 0; i < size; i++ {
		for j := 0; j+11 <= size; j++ {
			// 行扫描：get(a,b) = m.at(b, a)
			row := func(a, b int) bool { return m.at(b, a) }
			if matchAt(row, i, j, pattern1) || matchAt(row, i, j, pattern2) {
				score += 40
			}
			// 列扫描：get(a,b) = m.at(a, b)
			col := func(a, b int) bool { return m.at(a, b) }
			if matchAt(col, j, i, pattern1) || matchAt(col, j, i, pattern2) {
				score += 40
			}
		}
	}

	// 规则 4：深色比例偏离 50% 的惩罚
	dark := 0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if m.at(x, y) {
				dark++
			}
		}
	}
	ratio := dark * 100 / (size * size)
	deviation := ratio - 50
	if deviation < 0 {
		deviation = -deviation
	}
	score += (deviation / 5) * 10

	return score
}

func (m *qrMatrix) clone() *qrMatrix {
	c := newQRMatrix(m.size)
	for i := range m.mods {
		copy(c.mods[i], m.mods[i])
	}
	return c
}

// SVG 把文本渲染成 SVG（每模块 1 单位，含 4 模块静默区）。
//
// 用 SVG 而不是 PNG：纯文本、无图形依赖，前端直接塞进 <img src> 即可；
// 且测试可以对输出做断言（不用解析位图）。
func SVG(text string, moduleSize int) (string, error) {
	m, err := EncodeQR(text)
	if err != nil {
		return "", err
	}
	if moduleSize <= 0 {
		moduleSize = 4
	}
	const quiet = 4
	dim := (m.size + quiet*2) * moduleSize

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" shape-rendering="crispEdges">`,
		dim, dim, dim, dim)
	// 浅色底：显式画白底，避免在深色主题下扫码器读不出对比度
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#ffffff"/>`, dim, dim)
	b.WriteString(`<path fill="#000000" d="`)
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if !m.at(x, y) {
				continue
			}
			px := (x + quiet) * moduleSize
			py := (y + quiet) * moduleSize
			fmt.Fprintf(&b, "M%d %dh%dv%dh-%dz", px, py, moduleSize, moduleSize, moduleSize)
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String(), nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
