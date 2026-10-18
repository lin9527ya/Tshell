package otp

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 附录 B 官方向量。
//
// 注意标准的坑：附录 B 列的 8 位值是 SHA-256/SHA-512 用更长种子的结果，
// 而**SHA-1 行**用的种子是 20 字节的 "12345678901234567890"。
// 本实现是 HMAC-SHA1 + 6 位（认证器 App 的通用配置），
// 所以断言的是"SHA-1 种子 → 动态截断 → 取低 6 位"，
// 也就是把官方向量 94287082 截成 287082。
func TestRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	cases := []struct {
		unix int64
		want string // SHA-1 行取低 6 位
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, c := range cases {
		got := Code(secret, time.Unix(c.unix, 0))
		if got != c.want {
			t.Errorf("T=%d: 动态码 = %s，RFC 6238 期望 %s", c.unix, got, c.want)
		}
	}
}

// 8 位模式也要对：标准里 SHA-1 行的原始值就是 8 位
func TestRFC6238VectorsEightDigits(t *testing.T) {
	secret := []byte("12345678901234567890")
	cfg := TOTPConfig{Digits: 8, Period: 30 * time.Second, Skew: 1}
	cases := map[int64]string{
		59:         "94287082",
		1111111109: "07081804",
		1111111111: "14050471",
		1234567890: "89005924",
	}
	for unix, want := range cases {
		if got := CodeWith(secret, time.Unix(unix, 0), cfg); got != want {
			t.Errorf("T=%d 8 位: 得到 %s，期望 %s", unix, got, want)
		}
	}
}

func TestVerifyAcceptsCurrentAndSkewedCodes(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1700000000, 0)
	code := Code(secret, now)

	res := Verify(secret, code, now)
	if !res.OK {
		t.Fatal("当前时刻的动态码应通过")
	}
	if res.Timestep != uint64(now.Unix()/30) {
		t.Fatalf("时间步 = %d，期望 %d", res.Timestep, now.Unix()/30)
	}

	// 前后各一个窗口（±30s）应通过：客户端与服务器时钟不可能严格同步
	for _, delta := range []time.Duration{-30 * time.Second, 30 * time.Second} {
		if r := Verify(secret, code, now.Add(delta)); !r.OK {
			t.Errorf("偏移 %v 应通过（Skew=1）", delta)
		}
	}

	// 超过 Skew 必须拒绝，否则窗口放大到 90 秒，暴力面变大
	for _, delta := range []time.Duration{-61 * time.Second, 61 * time.Second, -5 * time.Minute} {
		if r := Verify(secret, code, now.Add(delta)); r.OK {
			t.Errorf("偏移 %v 不应通过（超出 Skew=1）", delta)
		}
	}
}

// 同一个码在同一窗口内多次校验都返回**同一个时间步** ——
// 这是调用方做防重放的前提（它记录"最后用掉的时间步"，靠这个值判断）
func TestVerifyReportsTimestepForReplayProtection(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1700000000, 0)
	code := Code(secret, now)

	first := Verify(secret, code, now)
	second := Verify(secret, code, now.Add(3*time.Second)) // 仍在同一 30s 窗口内
	if !first.OK || !second.OK {
		t.Fatal("同一窗口内两次校验都应通过（拒绝与否由调用方按时间步判断）")
	}
	if first.Timestep != second.Timestep {
		t.Fatalf("同一窗口应返回相同时间步，得到 %d 与 %d", first.Timestep, second.Timestep)
	}
}

func TestVerifyRejectsMalformedInput(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1700000000, 0)
	for _, bad := range []string{"", "12345", "1234567", "abcdef", "12 456", "１２３４５６"} {
		if r := Verify(secret, bad, now); r.OK {
			t.Errorf("非法输入 %q 不应通过", bad)
		}
	}
	// 空密钥不应 panic，也不应通过任何码
	if r := Verify(nil, "123456", now); r.OK {
		t.Error("空密钥不应通过")
	}
}

func TestSecretRoundTrip(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != SecretLength {
		t.Fatalf("密钥长度 = %d，期望 %d", len(secret), SecretLength)
	}

	encoded := EncodeSecret(secret)
	if strings.Contains(encoded, "=") {
		t.Errorf("base32 不应带填充: %q", encoded)
	}
	decoded, err := DecodeSecret(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(secret) {
		t.Fatal("编解码往返不一致")
	}

	// 宽容输入：小写、空格、误加的填充都要能解出来（用户是照纸抄的）
	for _, variant := range []string{
		strings.ToLower(encoded),
		" " + encoded[:4] + " " + encoded[4:] + " ",
		encoded + "======",
	} {
		got, err := DecodeSecret(variant)
		if err != nil {
			t.Errorf("宽容输入 %q 解析失败: %v", variant, err)
			continue
		}
		if string(got) != string(secret) {
			t.Errorf("宽容输入 %q 解析结果不一致", variant)
		}
	}

	for _, bad := range []string{"", "   ", "!!!!", "ABC"} { // 过短/非 base32
		if _, err := DecodeSecret(bad); err == nil {
			t.Errorf("非法密钥 %q 应报错", bad)
		}
	}
}

func TestProvisioningURI(t *testing.T) {
	secret := []byte("12345678901234567890")
	uri := ProvisioningURI("Tshell", "admin", secret, DefaultTOTPConfig())

	for _, want := range []string{
		"otpauth://totp/",
		"secret=" + EncodeSecret(secret),
		"issuer=Tshell",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, want) {
			t.Errorf("URI 缺少 %q: %s", want, uri)
		}
	}

	// label 里的特殊字符必须编码：账号常带 @ 与空格，不编码会被 App 解析错
	uri2 := ProvisioningURI("Tshell", "user name@corp.local", secret, DefaultTOTPConfig())
	if strings.Contains(uri2, "user name@corp.local") {
		t.Errorf("label 未做百分号编码: %s", uri2)
	}
	if !strings.Contains(uri2, "user%20name%40corp.local") {
		t.Errorf("label 编码结果不符合预期: %s", uri2)
	}
	// 空格不能编成 '+'（那是 query 的约定，label 是路径段）
	labelPart := strings.TrimPrefix(uri2, "otpauth://totp/")
	labelPart = labelPart[:strings.Index(labelPart, "?")]
	if strings.Contains(labelPart, "+") {
		t.Errorf("label 中的空格不应编码成 '+': %s", labelPart)
	}
}

func TestValidateCodeFormat(t *testing.T) {
	if err := ValidateCodeFormat("123456", 6); err != nil {
		t.Errorf("合法 6 位码被拒: %v", err)
	}
	for _, bad := range []string{"", "12345", "1234567", "12a456"} {
		if err := ValidateCodeFormat(bad, 6); err == nil {
			t.Errorf("非法码 %q 应报错", bad)
		}
	}
}

// 配置兜底：非法 Digits/Period/Skew 不能让后续逻辑除零或越界
func TestConfigNormalization(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1700000000, 0)

	// Digits=0/Period=0 应被兜回默认，而不是 panic
	got := CodeWith(secret, now, TOTPConfig{Digits: 0, Period: 0, Skew: -5})
	if len(got) != 6 {
		t.Fatalf("非法 Digits 应兜回 6 位，得到 %q", got)
	}
	if want := CodeWith(secret, now, DefaultTOTPConfig()); got != want {
		t.Fatalf("非法配置应等价于默认配置：%q vs %q", got, want)
	}
	// 兜底后 Skew 不为负，校验不应 panic
	if r := VerifyWith(secret, got, now, TOTPConfig{Digits: 0, Period: 0, Skew: -5}); !r.OK {
		t.Fatal("兜底配置下当前码应通过")
	}
}
