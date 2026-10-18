package otp

import (
	"strings"
	"testing"
)

func TestNewRecoveryCodesShape(t *testing.T) {
	codes, err := NewRecoveryCodes(RecoveryCodeCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("应生成 %d 个恢复码，实际 %d", RecoveryCodeCount, len(codes))
	}

	seen := map[string]bool{}
	for _, c := range codes {
		// 形如 "ABCDE-FGHJK"：10 个字符 + 1 个分隔符
		if len(c) != recoveryCodeChars+1 {
			t.Errorf("恢复码 %q 长度异常（应为 %d 个字符加一个分隔符）", c, recoveryCodeChars)
		}
		if c[recoveryCodeChars/2] != '-' {
			t.Errorf("恢复码 %q 应在中间有分隔符", c)
		}
		for _, r := range strings.ReplaceAll(c, "-", "") {
			if !strings.ContainsRune(recoveryAlphabet, r) {
				t.Errorf("恢复码 %q 含非法字符 %q（不应出现易混字符 0/O/1/I/L）", c, r)
			}
		}
		if seen[c] {
			t.Errorf("恢复码重复: %q", c)
		}
		seen[c] = true
	}
}

func TestRecoveryCodeHashAndUse(t *testing.T) {
	codes, err := NewRecoveryCodes(3)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := HashRecoveryCodes(codes)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != len(codes) {
		t.Fatalf("哈希数量 %d 与恢复码数量 %d 不符", len(hashes), len(codes))
	}
	// 哈希里不能出现明文（这是它存在的意义）
	for i, h := range hashes {
		if strings.Contains(h, strings.ReplaceAll(codes[i], "-", "")) {
			t.Fatal("哈希里竟然包含恢复码明文")
		}
	}

	// 用掉第二个码：应返回"移除该码之后"的列表
	rest, ok := UseRecoveryCode(hashes, codes[1])
	if !ok {
		t.Fatal("正确的恢复码应通过校验")
	}
	if len(rest) != len(hashes)-1 {
		t.Fatalf("用掉一个后应剩 %d 个，实际 %d", len(hashes)-1, len(rest))
	}
	// 同一个码不能再用（防重放）
	if _, ok := UseRecoveryCode(rest, codes[1]); ok {
		t.Fatal("已用过的恢复码不应再次通过（否则等于可以无限复用）")
	}
	// 其它码仍然可用
	if _, ok := UseRecoveryCode(rest, codes[2]); !ok {
		t.Fatal("未使用的恢复码应仍然有效")
	}
}

// 用户是照着手抄的纸输入的：大小写、空格、漏掉连字符都不该导致失败
func TestRecoveryCodeFuzzyInput(t *testing.T) {
	codes, err := NewRecoveryCodes(1)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := HashRecoveryCodes(codes)
	if err != nil {
		t.Fatal(err)
	}
	plain := strings.ReplaceAll(codes[0], "-", "")

	for _, variant := range []string{
		codes[0],
		strings.ToLower(codes[0]),
		plain,
		" " + codes[0] + " ",
		strings.ToLower(plain),
		"\t" + plain + " ",
	} {
		if _, ok := UseRecoveryCode(hashes, variant); !ok {
			t.Errorf("宽容输入 %q 应被接受", variant)
		}
	}
}

func TestRecoveryCodeRejectsWrong(t *testing.T) {
	codes, err := NewRecoveryCodes(2)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := HashRecoveryCodes(codes)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "   ", "ZZZZZ-ZZZZZ", "ABCDE-FGHI", plainOf(codes[0]) + "X"} {
		rest, ok := UseRecoveryCode(hashes, bad)
		if ok {
			t.Errorf("错误恢复码 %q 不应通过", bad)
		}
		if len(rest) != len(hashes) {
			t.Errorf("校验失败时不应改动哈希列表（%q 让长度从 %d 变成 %d）", bad, len(hashes), len(rest))
		}
	}
}

func plainOf(code string) string { return strings.ReplaceAll(code, "-", "") }
