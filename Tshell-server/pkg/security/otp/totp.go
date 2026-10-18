// Package otp 实现 TOTP（RFC 6238）与配套的二维码编码、恢复码工具。
//
// 为什么自实现而不引第三方库：
//   - TOTP 本体是 HMAC + 动态截断，标准文本很短，代码量远小于引入一个依赖链条；
//   - 二维码编码只服务一个用途（把 otpauth:// 交给认证器），
//     引 qrcode 库会连带图形/位图依赖，而服务端只需要输出 SVG 文本。
//
// 正确性由测试保证：RFC 6238 附录 B 官方向量与 QR 编解码往返（见 *_test.go）。
package otp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// TOTPConfig TOTP 参数（Google Authenticator / 微软认证器默认值）
type TOTPConfig struct {
	Digits int           // 动态码位数，默认 6
	Period time.Duration // 时间步长，默认 30s
	Skew   int           // 允许前后偏移的时间步数，默认 1（容忍 ±30s 时钟漂移）
}

// DefaultTOTPConfig 认证器 App 的通用默认值。
// 不要随意改 Digits/Period：改了两端就对不上，用户会看到"码一直错"。
func DefaultTOTPConfig() TOTPConfig {
	return TOTPConfig{Digits: 6, Period: 30 * time.Second, Skew: 1}
}

// normalized 兜底非法配置，避免除零与越界
func (c TOTPConfig) normalized() TOTPConfig {
	if c.Digits < 6 || c.Digits > 10 {
		c.Digits = 6
	}
	if c.Period <= 0 {
		c.Period = 30 * time.Second
	}
	if c.Skew < 0 {
		c.Skew = 0
	}
	return c
}

// SecretLength 密钥字节数。
// 20 字节 = HMAC-SHA1 的分组长度，也是 RFC 4226 建议的下限（≥128 位，且能被 SHA-1 完整利用）。
const SecretLength = 20

// base32NoPad 认证器约定的编码：大写 base32、不带 '=' 填充。
// 带填充的话部分 App 会拒绝，或让用户手抄时困惑。
var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret 生成一个新的 TOTP 密钥
func GenerateSecret() ([]byte, error) {
	buf := make([]byte, SecretLength)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("生成 TOTP 密钥失败: %w", err)
	}
	return buf, nil
}

// EncodeSecret 把密钥编码成认证器可录入的形式（大写 base32、无填充）
func EncodeSecret(secret []byte) string {
	return base32NoPad.EncodeToString(secret)
}

// DecodeSecret 解析用户/迁移数据里的 base32 密钥。
// 宽容处理：去空格、统一大写、容忍大小写混写与误加的 '='。
func DecodeSecret(s string) ([]byte, error) {
	clean := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	clean = strings.TrimRight(clean, "=")
	if clean == "" {
		return nil, fmt.Errorf("TOTP 密钥为空")
	}
	buf, err := base32NoPad.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("TOTP 密钥不是合法 base32: %w", err)
	}
	if len(buf) < 10 {
		return nil, fmt.Errorf("TOTP 密钥过短（%d 字节，至少 10 字节）", len(buf))
	}
	return buf, nil
}

// counter 时间步序号（RFC 6238：T = (当前时间 - T0) / 步长，T0=0）
func counter(t time.Time, period time.Duration) uint64 {
	return uint64(t.Unix() / int64(period.Seconds()))
}

// hotp 计算指定计数器的动态码（RFC 4226 的动态截断）
func hotp(secret []byte, counter uint64, digits int) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// 动态截断：取最后一字节低 4 位作偏移，从该偏移取 4 字节并抹掉最高位
	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	// 取低 digits 位（用取模而不是截字符串，避免前导零丢失）
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, code%mod)
}

// Code 计算某个时刻的动态码（供测试与"当前码"展示使用）
func Code(secret []byte, t time.Time) string {
	return CodeWith(secret, t, DefaultTOTPConfig())
}

// CodeWith 按指定配置计算动态码
func CodeWith(secret []byte, t time.Time, cfg TOTPConfig) string {
	cfg = cfg.normalized()
	return hotp(secret, counter(t, cfg.Period), cfg.Digits)
}

// VerifyResult 校验结果
type VerifyResult struct {
	OK       bool
	Timestep uint64 // 命中的时间步（OK 为 true 时有意义）
}

// Verify 校验动态码，返回命中的时间步。
//
// 返回 Timestep 是为了让调用方**做到防重放**：同一个时间步内的码只能用一次，
// 否则攻击者只要在 30 秒窗口里截获一次就能重复使用（这是 TOTP 实现最常见的漏洞）。
// 调用方应记录"每个用户最后成功使用的时间步"，并拒绝 <= 该值的结果。
func Verify(secret []byte, code string, t time.Time) VerifyResult {
	return VerifyWith(secret, code, t, DefaultTOTPConfig())
}

// VerifyWith 按指定配置校验动态码
func VerifyWith(secret []byte, code string, t time.Time, cfg TOTPConfig) VerifyResult {
	cfg = cfg.normalized()
	code = strings.TrimSpace(code)
	// 长度与字符集先做廉价筛除：非数字直接拒，别进 HMAC
	if len(code) != cfg.Digits || !isAllDigits(code) {
		return VerifyResult{}
	}

	now := counter(t, cfg.Period)
	for delta := -cfg.Skew; delta <= cfg.Skew; delta++ {
		// 负偏移在时间戳很小时会下溢，跳过而不是回绕到天文数字
		if delta < 0 && uint64(-delta) > now {
			continue
		}
		step := uint64(int64(now) + int64(delta))
		// 定长比较，避免按字符提前返回泄露信息
		if subtle.ConstantTimeCompare([]byte(hotp(secret, step, cfg.Digits)), []byte(code)) == 1 {
			return VerifyResult{OK: true, Timestep: step}
		}
	}
	return VerifyResult{}
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ValidateCodeFormat 只做格式校验（不接触密钥），用于在写库/查库之前挡掉明显非法的输入
func ValidateCodeFormat(code string, digits int) error {
	code = strings.TrimSpace(code)
	if len(code) != digits {
		return fmt.Errorf("动态码应为 %d 位数字", digits)
	}
	if !isAllDigits(code) {
		return fmt.Errorf("动态码只能是数字")
	}
	return nil
}

// ProvisioningURI 生成认证器可扫描的 otpauth:// 链接（RFC 6238 附录 A 的 Key Uri 格式）
//
// label 用 "<issuer>:<account>" 形式：主流 App 会据此分组显示。
// 参数需要百分号编码 —— 账号里带 '@'、空格、':' 都很常见，不编码会被 App 解析错。
func ProvisioningURI(issuer, account string, secret []byte, cfg TOTPConfig) string {
	cfg = cfg.normalized()
	label := account
	if issuer != "" {
		label = issuer + ":" + account
	}
	params := []string{
		"secret=" + urlEncode(EncodeSecret(secret)),
		"digits=" + fmt.Sprintf("%d", cfg.Digits),
		"period=" + fmt.Sprintf("%d", int(cfg.Period.Seconds())),
	}
	if issuer != "" {
		params = append(params, "issuer="+urlEncode(issuer))
	}
	return "otpauth://totp/" + urlEncode(label) + "?" + strings.Join(params, "&")
}

// urlEncode 按 RFC 3986 的 unreserved 集合做百分号编码。
// 不用 net/url 的 QueryEscape：它会把空格编成 '+'，而 otpauth 的 label 走的是路径段，
// '+' 在部分 App 里会原样显示成加号。
func urlEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0f])
		}
	}
	return b.String()
}
