package otp

import "testing"

// 逐个版本核对"可用数据模块数"是否等于几何期望值。
//
// 可用模块数不对 → 码字放不下 → 矩阵尺寸正常但内容错位，
// 这是 QR 编码里最难靠肉眼发现的一类错误，必须用计数断言钉住。
// 本用例抓出过 V7-M 的块表转录错误（4×27 应为 4×31）。
func TestDataModuleCapacityPerVersion(t *testing.T) {
	for v := 1; v <= maxQRVersion; v++ {
		size := v*4 + 17
		m := newQRMatrix(size)
		reserved := newQRMatrix(size)
		drawFunctionPatterns(m, reserved, v)

		free := 0
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				if !reserved.at(x, y) {
					free++
				}
			}
		}
		want := totalCodewords[v]*8 + remainderBits[v]
		if free != want {
			t.Errorf("版本 %d: 可用数据模块 %d，期望 %d（差 %d）",
				v, free, want, free-want)
		}
	}
}

// 纠错块表必须自洽：Σ 块数*(数据+纠错) == 该版本总码字数。
// 与上面的模块计数互为约束 —— 一个盯"几何"，一个盯"码字表"。
func TestBlockTableMatchesCodewordTotals(t *testing.T) {
	for v := 1; v <= maxQRVersion; v++ {
		groups := ecMTables[v]
		if len(groups) == 0 {
			t.Fatalf("版本 %d 缺少纠错块表", v)
		}
		ecPerBlock := groups[0].ecPerBlk
		total := 0
		for _, g := range groups {
			if g.ecPerBlk != ecPerBlock {
				t.Errorf("版本 %d: 同一版本内每块纠错码字数应一致，得到 %d 与 %d",
					v, g.ecPerBlk, ecPerBlock)
			}
			total += g.blocks * (g.dataCw + g.ecPerBlk)
		}
		if total != totalCodewords[v] {
			t.Errorf("版本 %d: 块表合计 %d 个码字，与总码字数 %d 不符（差 %d）",
				v, total, totalCodewords[v], total-totalCodewords[v])
		}
	}
}

// 版本选择：长文本不能选到更低的版本，且容量必须真的够装
func TestVersionCapacityIsSufficient(t *testing.T) {
	for n := 1; n <= 200; n++ {
		text := make([]byte, n)
		for i := range text {
			text[i] = 'a' + byte(i%26) // 保持可打印，避免误触其他模式
		}
		m, err := EncodeQR(string(text))
		if err != nil {
			t.Fatalf("%d 字节编码失败: %v", n, err)
		}
		v, err := qrVersionOf(m.size)
		if err != nil {
			t.Fatal(err)
		}
		// 编出来的版本必须真的装得下这段文本，且再低一版装不下
		if cap := dataCapacityBytes(v, ECLMedium); n > cap {
			t.Errorf("%d 字节选了 v%d，但其字节容量只有 %d", n, v, cap)
		}
		if v > 1 {
			if cap := dataCapacityBytes(v-1, ECLMedium); n <= cap {
				t.Errorf("%d 字节本可用更小的 v%d（容量 %d），却选了 v%d", n, v-1, cap, v)
			}
		}
	}
}
