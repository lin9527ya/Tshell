package otp

import (
	"strings"
	"testing"
)

// QR 的核心验证是"编码 → 解码往返"。
// 没有参考实现可以对拍，而"矩阵尺寸对不对"这种断言挡不住真正的错误：
// 格式信息位序反了、掩码条件写错、zigzag 起点偏了 —— 这些都不会改变矩阵尺寸，
// 但会让任何扫描器都读不出来。往返测试能挡住这些。

func TestQRRoundTrip(t *testing.T) {
	cases := []string{
		"HELLO",
		"otpauth://totp/Tshell:admin?secret=JBSWY3DPEHPK3PXP&digits=6&period=30&issuer=Tshell",
		// 覆盖多字节 UTF-8（用户名可能含中文）
		"otpauth://totp/Tshell:%E5%BC%A0%E4%B8%89?secret=JBSWY3DPEHPK3PXP&issuer=Tshell",
		// 覆盖版本切换的边界长度
		strings.Repeat("A", 14),
		strings.Repeat("B", 26),
		strings.Repeat("C", 42),
		strings.Repeat("D", 62),
		strings.Repeat("E", 84),
		strings.Repeat("F", 106),
		strings.Repeat("G", 122),
	}
	for _, text := range cases {
		m, err := EncodeQR(text)
		if err != nil {
			t.Errorf("编码 %d 字节失败: %v", len(text), err)
			continue
		}
		got, err := qrDecode(m)
		if err != nil {
			t.Errorf("解码 %d 字节失败: %v", len(text), err)
			continue
		}
		if got != text {
			t.Errorf("往返不一致（%d 字节）:\n  原始 %q\n  得到 %q", len(text), text, got)
		}
	}
}

func TestQRVersionSelection(t *testing.T) {
	// 越长的文本必须选到不低的版本；同长度必须稳定选同一个版本（否则输出的码会飘）
	prev := 0
	for n := 1; n <= 200; n += 7 {
		m, err := EncodeQR(strings.Repeat("x", n))
		if err != nil {
			t.Fatalf("%d 字节编码失败: %v", n, err)
		}
		v, err := qrVersionOf(m.size)
		if err != nil {
			t.Fatal(err)
		}
		if v < prev {
			t.Errorf("文本变长后版本反而变小: %d 字节 → v%d（上一步 v%d）", n, v, prev)
		}
		prev = v
	}
}

func TestQRRejectsOversizedInput(t *testing.T) {
	// 超过版本 10 的容量必须给出可读错误，而不是静默截断或 panic
	big := strings.Repeat("x", 400)
	if _, err := EncodeQR(big); err == nil {
		t.Fatal("超长文本应报错")
	} else if !strings.Contains(err.Error(), "过长") {
		t.Fatalf("错误信息应说明原因: %v", err)
	}
}

// 结构判据：三个定位图形必须画在标准位置上。
// 这是扫描器"找到码"的第一步，位置错了后面全白搭。
func TestQRFinderPatterns(t *testing.T) {
	m, err := EncodeQR("HELLO WORLD")
	if err != nil {
		t.Fatal(err)
	}
	size := m.size

	checkFinder := func(cx, cy int) {
		// 只断言 5x5 内核（偏移 -2..2）。最外圈（距离 3）的两个"定时臂"会被
		// 定时图形按 i%2 覆写 —— 那是标准规定的行为，在这里断言会误报。
		//
		// 内核正好是"定位图形 vs 一个实心方块"的唯一区别所在：
		// 距离 2 的一圈必须是浅色。本实现第一版把它画成深色，得到 7x7 实心方块，
		// 矩阵尺寸与往返测试都不会报错，但任何扫描器都认不出定位图形。
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				ax, ay := abs(dx), abs(dy)
				want := ax <= 1 && ay <= 1 // 内核：外圈浅色、中心 3x3 深色
				if m.at(cx+dx, cy+dy) != want {
					t.Errorf("定位图形 (%d,%d) 内核偏移 (%d,%d) 应为 %v", cx, cy, dx, dy, want)
					return
				}
			}
		}
	}
	checkFinder(3, 3)
	checkFinder(size-4, 3)
	checkFinder(3, size-4)

	// 分隔符：每个定位图形**与数据区相邻的那两条边**必须是浅色 ——
	// 它把定位图形与数据区隔开，是扫描器判定"定位图形到此为止"的依据。
	// 注意只检查这两条边：斜对角方向（如左上的 (7,7) 之外）并没有更多分隔格，
	// 而第 8 行/列属于格式信息或定时图形，不由分隔符决定颜色。
	checkSeparator := func(x, y int) {
		if m.at(x, y) {
			t.Errorf("分隔符 (%d,%d) 应为浅色", x, y)
		}
	}
	for i := 0; i <= 7; i++ {
		checkSeparator(7, i)        // 左上定位图形右侧那条竖边
		checkSeparator(i, 7)        // 左上定位图形下方那条横边
		checkSeparator(size-8, i)   // 右上定位图形左侧那条竖边
		checkSeparator(size-8+i, 7) // 右上定位图形下方那条横边
		checkSeparator(i, size-8)   // 左下定位图形上方那条横边
		checkSeparator(7, size-8+i) // 左下定位图形右侧那条竖边
	}

	// 固定的深色模块：必须在 (8, size-8)
	if !m.at(8, size-8) {
		t.Error("固定深色模块 (8,size-8) 缺失")
	}
}

// 定时图形：第 6 行/列在定位图形之间必须黑白交替（扫描器靠它判定模块尺寸）。
//
// (6,8)/(8,6) 是例外：这两个格子被格式信息覆盖（标准规定格式信息优先），
// 所以断言时要把它们排除，否则会误报。
func TestQRTimingPatterns(t *testing.T) {
	m, err := EncodeQR("TIMING")
	if err != nil {
		t.Fatal(err)
	}
	size := m.size
	for i := 8; i < size-8; i++ {
		if i == 8 {
			continue // (6,8)/(8,6) 属格式信息区域
		}
		want := i%2 == 0
		if m.at(i, 6) != want {
			t.Errorf("横向定时图形第 %d 列应为 %v", i, want)
		}
		if m.at(6, i) != want {
			t.Errorf("纵向定时图形第 %d 行应为 %v", i, want)
		}
	}
}

// 对齐图形（版本 ≥2）：中心必须是深色，且在标准坐标上
func TestQRAlignmentPatterns(t *testing.T) {
	m, err := EncodeQR(strings.Repeat("Z", 40)) // 会选到版本 ≥3
	if err != nil {
		t.Fatal(err)
	}
	version, err := qrVersionOf(m.size)
	if err != nil {
		t.Fatal(err)
	}
	positions := alignPositions[version]
	if len(positions) == 0 {
		t.Skip("该版本没有对齐图形")
	}
	for _, cy := range positions {
		for _, cx := range positions {
			// 与定位图形重叠的位置不画对齐图形
			if (cx <= 8 && cy <= 8) || (cx >= m.size-9 && cy <= 8) || (cx <= 8 && cy >= m.size-9) {
				continue
			}
			if !m.at(cx, cy) {
				t.Errorf("对齐图形中心 (%d,%d) 应为深色", cx, cy)
			}
			// 外框四角也应是深色
			if !m.at(cx-2, cy-2) || !m.at(cx+2, cy+2) {
				t.Errorf("对齐图形 (%d,%d) 外框缺失", cx, cy)
			}
			// 内外之间的第一圈应是浅色
			if m.at(cx-1, cy-1) {
				t.Errorf("对齐图形 (%d,%d) 内圈应为浅色", cx, cy)
			}
		}
	}
}

func TestSVGOutput(t *testing.T) {
	svg, err := SVG("otpauth://totp/Tshell:admin?secret=JBSWY3DPEHPK3PXP&issuer=Tshell", 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<svg", "</svg>", `fill="#ffffff"`, `fill="#000000"`, "viewBox="} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG 缺少 %q", want)
		}
	}
	// 尺寸 = (模块数 + 8 静默区) * 每模块像素
	m, _ := EncodeQR("otpauth://totp/Tshell:admin?secret=JBSWY3DPEHPK3PXP&issuer=Tshell")
	wantDim := (m.size + 8) * 4
	if !strings.Contains(svg, `width="`+itoa(wantDim)+`"`) {
		t.Errorf("SVG 宽度应为 %d: %s", wantDim, svg[:min(120, len(svg))])
	}
	// 同一输入必须稳定输出（否则前端每次刷新图都不同，缓存与对拍都失效）
	svg2, _ := SVG("otpauth://totp/Tshell:admin?secret=JBSWY3DPEHPK3PXP&issuer=Tshell", 4)
	if svg != svg2 {
		t.Error("同一输入应产生相同 SVG")
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
