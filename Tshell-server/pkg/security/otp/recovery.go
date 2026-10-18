package otp

import (
	"crypto/rand"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// 恢复码（recovery code）：认证器丢失时的唯一自助救援路径。
//
// 设计取舍：
//   - 用 base32（去掉易混的 0/O/1/I 影响）而不是纯数字：字符集小可读性高，且比 6 位数字熵大得多；
//   - 每个码 10 字符 = 50 bit 熵，一次性使用 —— 暴力猜测不现实；
//   - **只存 bcrypt 哈希**：与口令同等待遇。库被读走也拿不到可用的恢复码。
const (
	// RecoveryCodeCount 默认签发数量
	RecoveryCodeCount = 10
	// recoveryCodeChars 单个恢复码的字符数（不含分隔符）
	recoveryCodeChars = 10
)

// recoveryAlphabet base32 字符集去掉容易看错的字符（0/O、1/I/L）
const recoveryAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// NewRecoveryCodes 生成 n 个恢复码，形如 "ABCDE-FGHJK"（带分隔符便于抄写）。
// 返回的是**明文**，只在签发那一刻展示一次，之后只留哈希。
func NewRecoveryCodes(n int) ([]string, error) {
	if n <= 0 {
		n = RecoveryCodeCount
	}
	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		raw := make([]byte, recoveryCodeChars)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("生成恢复码失败: %w", err)
		}
		var b strings.Builder
		for j, v := range raw {
			if j == recoveryCodeChars/2 {
				b.WriteByte('-')
			}
			// 取模引入的偏置可忽略（256 % 31 != 0，偏差 < 0.4%），
			// 而恢复码的威胁模型是"猜中 50 bit"，不是"区分均匀分布"
			b.WriteByte(recoveryAlphabet[int(v)%len(recoveryAlphabet)])
		}
		codes = append(codes, b.String())
	}
	return codes, nil
}

// HashRecoveryCodes 把明文恢复码转成可入库的哈希列表
func HashRecoveryCodes(codes []string) ([]string, error) {
	hashes := make([]string, 0, len(codes))
	for _, c := range codes {
		h, err := bcrypt.GenerateFromPassword([]byte(normalizeRecoveryCode(c)), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("哈希恢复码失败: %w", err)
		}
		hashes = append(hashes, string(h))
	}
	return hashes, nil
}

// UseRecoveryCode 尝试用恢复码通过校验。
//
// 成功时返回**移除该码之后的剩余哈希列表**（调用方必须落库，否则同一个码能重复使用）。
// 失败时返回原列表与 false。逐个 bcrypt 比较是必要的：bcrypt 每次盐不同，不能用哈希查表。
func UseRecoveryCode(hashes []string, code string) ([]string, bool) {
	normalized := normalizeRecoveryCode(code)
	if normalized == "" {
		return hashes, false
	}
	for i, h := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(normalized)) == nil {
			rest := make([]string, 0, len(hashes)-1)
			rest = append(rest, hashes[:i]...)
			rest = append(rest, hashes[i+1:]...)
			return rest, true
		}
	}
	return hashes, false
}

// normalizeRecoveryCode 归一化用户输入：忽略大小写、空格与分隔符。
// 用户是照着手抄的纸输入的，不该因为大小写或漏掉连字符就判定失败。
func normalizeRecoveryCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		if r == '-' || r == ' ' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
