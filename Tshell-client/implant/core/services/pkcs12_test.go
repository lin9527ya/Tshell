package services

import (
	"bytes"
	"crypto/cipher"
	"crypto/des"
	"encoding/hex"
	"strings"
	"testing"
)

// newCBCEncrypterForTest 测试用的 CBC 加密器。
// 生产代码只需要解密，加密只在测试里用来造往返样本。
func newCBCEncrypterForTest(block cipher.Block, iv []byte) cipher.BlockMode {
	return cipher.NewCBCEncrypter(block, iv)
}

// ===== 已知答案向量（KAT）=====
//
// 向量来源：Go 官方 x/crypto 的 `pkcs12/pbkdf_test.go`。
// 其中 "sesame" 那组是 PKCS#12 规范里的经典测试向量。
//
// 为什么必须用 KAT 而不是往返测试：往返测试（自己派生密钥→自己解密）只能证明
// **自洽** —— KDF 步骤理解错了两边会一起错，测试照样全绿。只有对着外部权威
// 期望值比对，才能证明实现与规范一致。

// TestPBKDFKnownAnswerSesame 经典 "sesame" 向量。
//
// salt=ff*8, password=bmp("sesame"), r=2048, ID=1, n=24
func TestPBKDFKnownAnswerSesame(t *testing.T) {
	salt := []byte("\xff\xff\xff\xff\xff\xff\xff\xff")
	password := bmpString("sesame")
	got := pkcs12PBKDF(sha1KDFParams, salt, password, 2048, 1, 24)

	const want = "7cd9fd3e2b3be7691a44e3bef0f9ea0fb9b897d4e325d9d1"
	if hex.EncodeToString(got) != want {
		t.Fatalf("派生结果不符\n期望: %s\n实际: %s", want, hex.EncodeToString(got))
	}
}

// TestPBKDFKnownAnswerLeadingZeros 前导零回归向量。
//
// salt=f37e05b518324b4b, password=0000, r=2048, ID=1, n=24
//
// 这组专门覆盖"I_j 加法后出现前导零"：用 big.Int 实现会把前导零吃掉、
// 写回时块长不足 v，派生结果就错了（x/crypto 历史上确有这个 bug）。
// 本实现用定长字节加法，结构上不会出现该问题 —— 这条测试就是钉住这一点。
func TestPBKDFKnownAnswerLeadingZeros(t *testing.T) {
	salt, _ := hex.DecodeString("f37e05b518324b4b")
	password, _ := hex.DecodeString("0000")
	got := pkcs12PBKDF(sha1KDFParams, salt, password, 2048, 1, 24)

	const want = "00f759ff47d14dd03665d5943cb3c4a39a2555c02aed66e1"
	if hex.EncodeToString(got) != want {
		t.Fatalf("派生结果不符（前导零处理有误）\n期望: %s\n实际: %s", want, hex.EncodeToString(got))
	}
}

// 派生结果长度必须精确等于请求长度（不是 u 的整数倍时也要截断正确）
func TestPBKDFOutputLength(t *testing.T) {
	salt := []byte("01234567")
	pw := bmpString("pw")
	for _, n := range []int{1, 7, 8, 20, 21, 24, 40, 41, 100} {
		got := pkcs12PBKDF(sha1KDFParams, salt, pw, 16, 1, n)
		if len(got) != n {
			t.Errorf("请求 %d 字节，实际得到 %d 字节", n, len(got))
		}
	}
}

// 不同 ID 必须派生出不同结果（否则密钥与 IV 会相同，等于把 IV 固定）
func TestPBKDFDifferentIDsDiffer(t *testing.T) {
	salt := []byte("01234567")
	pw := bmpString("pw")
	k := pkcs12PBKDF(sha1KDFParams, salt, pw, 100, 1, 24)
	iv := pkcs12PBKDF(sha1KDFParams, salt, pw, 100, 2, 8)
	mac := pkcs12PBKDF(sha1KDFParams, salt, pw, 100, 3, 20)
	if bytes.Equal(k[:8], iv) {
		t.Error("ID=1 与 ID=2 的前 8 字节相同 —— 密钥与 IV 不应相同")
	}
	if bytes.Equal(k[:20], mac) {
		t.Error("ID=1 与 ID=3 相同 —— 密钥与 MAC 密钥不应相同")
	}
}

// 迭代次数与 salt 变化必须改变结果
func TestPBKDFSensitiveToInputs(t *testing.T) {
	pw := bmpString("pw")
	base := pkcs12PBKDF(sha1KDFParams, []byte("salt1"), pw, 100, 1, 24)
	if bytes.Equal(base, pkcs12PBKDF(sha1KDFParams, []byte("salt2"), pw, 100, 1, 24)) {
		t.Error("salt 变化应改变派生结果")
	}
	if bytes.Equal(base, pkcs12PBKDF(sha1KDFParams, []byte("salt1"), pw, 101, 1, 24)) {
		t.Error("迭代次数变化应改变派生结果")
	}
	if bytes.Equal(base, pkcs12PBKDF(sha1KDFParams, []byte("salt1"), bmpString("pw2"), 100, 1, 24)) {
		t.Error("口令变化应改变派生结果")
	}
}

// 空 salt / 空口令不能 panic（RFC 附录明确：为空则 S/P 为空）
func TestPBKDFEmptyInputs(t *testing.T) {
	if got := pkcs12PBKDF(sha1KDFParams, nil, nil, 10, 1, 24); len(got) != 24 {
		t.Errorf("空 salt/口令也应产出 24 字节，实际 %d", len(got))
	}
	if got := pkcs12PBKDF(sha1KDFParams, []byte("s"), nil, 10, 1, 24); len(got) != 24 {
		t.Errorf("空口令也应产出 24 字节，实际 %d", len(got))
	}
}

func TestPBKDFZeroSize(t *testing.T) {
	if got := pkcs12PBKDF(sha1KDFParams, []byte("s"), []byte("p"), 10, 1, 0); got != nil {
		t.Errorf("size=0 应返回 nil，实际 %v", got)
	}
}

// 迭代次数为 0 或负数时按 1 处理（而不是死循环或返回空）
func TestPBKDFIterationFloor(t *testing.T) {
	salt := []byte("01234567")
	pw := bmpString("pw")
	a := pkcs12PBKDF(sha1KDFParams, salt, pw, 0, 1, 24)
	b := pkcs12PBKDF(sha1KDFParams, salt, pw, 1, 1, 24)
	if !bytes.Equal(a, b) {
		t.Error("迭代次数 0 应按 1 处理")
	}
}

// ===== BMPString 编码 =====

func TestBMPString(t *testing.T) {
	// RFC 7292 B.1 的原例："Beavis" →
	// 00 42 00 65 00 61 00 76 00 69 00 73 00 00
	got := bmpString("Beavis")
	const want = "0042006500610076006900730000"
	if hex.EncodeToString(got) != want {
		t.Fatalf("BMPString 编码不符\n期望: %s\n实际: %s", want, hex.EncodeToString(got))
	}
}

func TestBMPStringEmpty(t *testing.T) {
	// 空口令 = 仅两个 NULL 字节（不是零字节）
	got := bmpString("")
	if hex.EncodeToString(got) != "0000" {
		t.Errorf("空口令应为 0000，实际 %s", hex.EncodeToString(got))
	}
}

func TestBMPStringNonASCII(t *testing.T) {
	// 非 ASCII 走 UTF-16BE；结尾仍是两字节 NULL
	got := bmpString("中")
	if hex.EncodeToString(got) != "4e2d0000" {
		t.Errorf("中文应编码为 4e2d0000，实际 %s", hex.EncodeToString(got))
	}
	// 代理对（emoji）占 4 字节 + 2 字节 NULL
	got2 := bmpString("😀")
	if len(got2) != 6 {
		t.Errorf("代理对应为 4+2=6 字节，实际 %d", len(got2))
	}
	if !strings.HasSuffix(hex.EncodeToString(got2), "0000") {
		t.Error("代理对结尾也应是两字节 NULL")
	}
}

// ===== 3DES-CBC PBE 解密 =====

// 用**同一个 KDF** 加密再解密，只能验证 CBC/填充这一层；
// KDF 本身的正确性由上面的 KAT 保证。两者合起来才覆盖整条链路。
func TestPBE3DESRoundTrip(t *testing.T) {
	salt := []byte("01234567")
	pw := "secret"
	plaintext := []byte("Firefox master key material 24")

	// 加密端（仅测试用）：同样的 KDF 派生 key/iv
	key := pkcs12PBKDF(sha1KDFParams, salt, bmpString(pw), 1024, 1, 24)
	iv := pkcs12PBKDF(sha1KDFParams, salt, bmpString(pw), 1024, 2, des.BlockSize)
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	// PKCS#7 填充
	pad := des.BlockSize - len(plaintext)%des.BlockSize
	padded := append(append([]byte{}, plaintext...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(padded))
	mode := newCBCEncrypterForTest(block, iv)
	mode.CryptBlocks(ct, padded)

	got, err := pkcs12PBE3DESDecrypt(salt, 1024, pw, ct)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("往返不一致\n期望: %q\n实际: %q", plaintext, got)
	}
}

func TestPBE3DESWrongPassword(t *testing.T) {
	salt := []byte("01234567")
	key := pkcs12PBKDF(sha1KDFParams, salt, bmpString("right"), 512, 1, 24)
	iv := pkcs12PBKDF(sha1KDFParams, salt, bmpString("right"), 512, 2, des.BlockSize)
	block, _ := des.NewTripleDESCipher(key)
	plain := bytes.Repeat([]byte("A"), 32)
	ct := make([]byte, len(plain))
	newCBCEncrypterForTest(block, iv).CryptBlocks(ct, plain)

	// 错口令：绝大多数情况下填充校验会失败
	if _, err := pkcs12PBE3DESDecrypt(salt, 512, "wrong", ct); err == nil {
		t.Error("错口令应返回错误（而不是静默返回垃圾）")
	}
}

func TestPBE3DESBadCiphertextLength(t *testing.T) {
	// 长度不是块大小整数倍 → 明确报错，不硬解
	if _, err := pkcs12PBE3DESDecrypt([]byte("01234567"), 100, "pw", []byte("1234567")); err == nil {
		t.Error("非整块长度应报错")
	}
	if _, err := pkcs12PBE3DESDecrypt([]byte("01234567"), 100, "pw", nil); err == nil {
		t.Error("空密文应报错")
	}
}

func TestUnpadRejectsBadPadding(t *testing.T) {
	// 末字节为 0：非法
	if _, err := pkcs12Unpad([]byte{1, 2, 3, 0}); err == nil {
		t.Error("末字节 0 应判为非法填充")
	}
	// 末字节大于块大小：非法
	if _, err := pkcs12Unpad([]byte{1, 2, 3, 9}); err == nil {
		t.Error("填充值大于块大小应判为非法")
	}
	// 填充字节不一致：非法
	if _, err := pkcs12Unpad([]byte{1, 2, 3, 2, 3}); err == nil {
		t.Error("填充字节不一致应判为非法")
	}
	// 合法填充：8 字节块 = 4 字节数据 + 4 字节填充值 4
	got, err := pkcs12Unpad([]byte{1, 2, 3, 4, 4, 4, 4, 4})
	if err != nil {
		t.Fatalf("合法填充被判非法: %v", err)
	}
	if !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Errorf("去填充结果应为 [1 2 3 4]，实际 %v", got)
	}
}

// 定长块加法：必须保持块长不变（前导零不得丢失）
func TestAddIntoVBitBlocksKeepsLength(t *testing.T) {
	v := 8
	I := make([]byte, v*3) // 全 0
	B := make([]byte, v)
	B[v-1] = 0xff // 0x00 + 0xff + 1 = 0x100 → 低字节 0x00，进位进到块内 v-2
	addIntoVBitBlocks(I, B, v)
	// RFC 的累加是**逐 v 字节块独立**做的，所以每个块都会各自产生进位
	for off := 0; off+v <= len(I); off += v {
		for j := 0; j < v; j++ {
			want := byte(0)
			if j == v-2 {
				want = 1
			}
			if I[off+j] != want {
				t.Fatalf("块 %d 第 %d 字节应为 %d，实际 %d", off/v, j, want, I[off+j])
			}
		}
	}
	if len(I) != v*3 {
		t.Fatal("块长必须守恒")
	}

	// 进位在**块内**传播（整个块是 mod 2^v 的整数，不是逐字节独立）
	I2 := make([]byte, v)
	I2[v-1] = 0x01
	B2 := make([]byte, v)
	B2[v-1] = 0xff
	addIntoVBitBlocks(I2, B2, v) // 0x01 + 0xff + 1 = 0x101 → 低字节 0x01，进位进到 v-2
	if I2[v-1] != 0x01 {
		t.Errorf("最低字节应为 0x01，实际 %#x", I2[v-1])
	}
	if I2[v-2] != 0x01 {
		t.Errorf("进位应落到 v-2 字节为 0x01，实际 %#x", I2[v-2])
	}
	if len(I2) != v {
		t.Fatal("块长必须守恒")
	}
}

func TestFillWithRepeats(t *testing.T) {
	if got := fillWithRepeats([]byte{1, 2, 3}, 7); !bytes.Equal(got, []byte{1, 2, 3, 1, 2, 3, 1}) {
		t.Errorf("重复填充不符: %v", got)
	}
	if got := fillWithRepeats([]byte{9}, 3); !bytes.Equal(got, []byte{9, 9, 9}) {
		t.Errorf("单字节重复不符: %v", got)
	}
	if got := fillWithRepeats(nil, 5); got != nil {
		t.Errorf("空 pattern 应返回 nil，实际 %v", got)
	}
	if got := fillWithRepeats([]byte{1}, 0); got != nil {
		t.Errorf("n=0 应返回 nil，实际 %v", got)
	}
}
