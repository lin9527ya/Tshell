package services

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	stdpbkdf2 "crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"testing"

	xpbkdf2 "golang.org/x/crypto/pbkdf2"
)

// ===== PBKDF2 差分测试（本文件证据最强的一项）=====
//
// pbes2AES256CBCDecrypt 内部用 PBKDF2-HMAC-SHA256 派生密钥。
// 与其"自己派生自己解密"（只能证明自洽），这里与**两个独立实现**对比：
//   - Go 标准库 crypto/pbkdf2（Go 1.24+，按 RFC 8018）
//   - golang.org/x/crypto/pbkdf2（按 RFC 2898）
// 两个实现互相独立、且都经过广泛验证；三方一致才说明我的调用口径没错
// （尤其是"password 按原始字节还是按字符串"这一处 —— 用错会静默派生错密钥）。

// TestPBKDF2MatchesStdlib 随机输入下与标准库逐字节一致
func TestPBKDF2MatchesStdlib(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 40; i++ {
		pwLen := rnd.Intn(33)
		saltLen := 1 + rnd.Intn(32)
		iter := 1 + rnd.Intn(3000)
		pw := make([]byte, pwLen)
		salt := make([]byte, saltLen)
		rnd.Read(pw)
		rnd.Read(salt)

		// 被测路径：与 pbes2AES256CBCDecrypt 内部完全相同的调用
		mine, err := stdpbkdf2.Key(sha256.New, string(pw), salt, iter, 32)
		if err != nil {
			t.Fatalf("stdlib 派生失败: %v", err)
		}
		// 注意 x/crypto 的签名是 (password, salt []byte, iter, keyLen, h)，哈希函数在最后
		ref := xpbkdf2.Key(pw, salt, iter, 32, sha256.New)
		if !bytes.Equal(mine, ref) {
			t.Fatalf("第 %d 组与 x/crypto 不一致\npw=%x salt=%x iter=%d\nmine=%x\nref =%x",
				i, pw, salt, iter, mine, ref)
		}
	}
}

// TestPBKDF2NonUTF8Password 关键：口令按**原始字节**传入时不能被 UTF-8 替换破坏。
//
// NSS 这一层喂进来的是 SHA1 摘要（含非法 UTF-8 序列），若用 string 转换再
// 被上层按 rune 处理，密钥就错了 —— 症状是"口令明明对却解不开"。
func TestPBKDF2NonUTF8Password(t *testing.T) {
	// 0xff 0xfe 不是合法 UTF-8
	pw := []byte{0xff, 0xfe, 0x80, 0x01}
	salt := []byte("saltsalt")
	mine, err := stdpbkdf2.Key(sha256.New, string(pw), salt, 100, 32)
	if err != nil {
		t.Fatal(err)
	}
	ref := xpbkdf2.Key(pw, salt, 100, 32, sha256.New)
	if !bytes.Equal(mine, ref) {
		t.Fatalf("非法 UTF-8 口令下与 x/crypto 不一致\nmine=%x\nref =%x", mine, ref)
	}
}

// 迭代次数/盐/口令任一变化都必须改变派生结果
func TestPBKDF2Sensitivity(t *testing.T) {
	base, _ := stdpbkdf2.Key(sha256.New, "pw", []byte("salt"), 1000, 32)
	for _, tc := range []struct {
		name string
		got  []byte
	}{
		{"salt 变化", mustPBKDF2(t, "pw", []byte("salt2"), 1000)},
		{"迭代变化", mustPBKDF2(t, "pw", []byte("salt"), 1001)},
		{"口令变化", mustPBKDF2(t, "pw2", []byte("salt"), 1000)},
	} {
		if bytes.Equal(base, tc.got) {
			t.Errorf("%s 应改变派生结果", tc.name)
		}
	}
}

func mustPBKDF2(t *testing.T, pw string, salt []byte, iter int) []byte {
	t.Helper()
	k, err := stdpbkdf2.Key(sha256.New, pw, salt, iter, 32)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// ===== PBES2 往返 =====

// TestPBES2RoundTrip 验证管道：PBKDF2 → AES-256-CBC → 去填充
func TestPBES2RoundTrip(t *testing.T) {
	password := []byte("hashed-master-password!!") // 20 字节，模拟 SHA1 摘要
	salt := []byte("0123456789abcdef")
	iv := []byte("abcdefghijklmnop")
	plaintext := []byte("https://example.com\talice\tS3cret!")

	key, err := stdpbkdf2.Key(sha256.New, string(password), salt, 2048, 32)
	if err != nil {
		t.Fatal(err)
	}
	ct := aesCBCEncryptForTest(t, key, iv, plaintext)

	got, err := pbes2AES256CBCDecrypt(password, salt, iv, ct, 2048)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("往返不一致\n期望: %q\n实际: %q", plaintext, got)
	}
}

func TestPBES2RejectsBadParams(t *testing.T) {
	pw := []byte("pw")
	salt := []byte("salt")
	if _, err := pbes2AES256CBCDecrypt(pw, salt, []byte("short"), make([]byte, 16), 100); err == nil {
		t.Error("IV 长度不对应报错")
	}
	if _, err := pbes2AES256CBCDecrypt(pw, salt, make([]byte, 16), make([]byte, 15), 100); err == nil {
		t.Error("密文非整块应报错")
	}
	if _, err := pbes2AES256CBCDecrypt(pw, salt, make([]byte, 16), make([]byte, 16), 0); err == nil {
		t.Error("迭代次数 0 应报错")
	}
}

func TestPBES2WrongPasswordFails(t *testing.T) {
	salt := []byte("0123456789abcdef")
	iv := []byte("abcdefghijklmnop")
	key, _ := stdpbkdf2.Key(sha256.New, "right", salt, 512, 32)
	ct := aesCBCEncryptForTest(t, key, iv, []byte("secret data here"))
	if _, err := pbes2AES256CBCDecrypt([]byte("wrong"), salt, iv, ct, 512); err == nil {
		t.Error("错口令应失败（而不是返回垃圾）")
	}
}

// ===== 裸 3DES-CBC（logins.json 旧格式）=====

func TestNSS3DESRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 24)
	iv := bytes.Repeat([]byte{0x22}, 8)
	plaintext := []byte("https://site\tuser\tpass")

	block, _ := des.NewTripleDESCipher(key)
	ct := cbcEncryptForTest(block, iv, plaintext, des.BlockSize)

	got, err := nssTripleDESCBCDecrypt(key, iv, ct)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("往返不一致: %q vs %q", plaintext, got)
	}
}

func TestNSS3DESRejectsBadKeyLength(t *testing.T) {
	for _, n := range []int{0, 8, 16, 32} {
		if _, err := nssTripleDESCBCDecrypt(make([]byte, n), make([]byte, 8), make([]byte, 8)); err == nil {
			t.Errorf("密钥长度 %d 应被拒绝（3DES 需 24 字节）", n)
		}
	}
}

// ===== 裸 AES-256-CBC（logins.json Firefox 144+）=====

func TestNSSAES256RoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x33}, 32)
	iv := bytes.Repeat([]byte{0x44}, 16)
	plaintext := []byte("firefox-144+ login entry")
	ct := aesCBCEncryptForTest(t, key, iv, plaintext)

	got, err := nssAES256CBCDecrypt(key, iv, ct)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("往返不一致: %q vs %q", plaintext, got)
	}
}

// 48 字节主密钥必须截断到前 32 —— 这是参考实现的做法，不截断会直接报错
func TestNSSAES256Truncates48ByteKey(t *testing.T) {
	full := make([]byte, 48)
	for i := range full {
		full[i] = byte(i)
	}
	iv := bytes.Repeat([]byte{0x55}, 16)
	plaintext := []byte("payload")
	// 用截断后的 32 字节加密
	ct := aesCBCEncryptForTest(t, full[:32], iv, plaintext)

	got, err := nssAES256CBCDecrypt(full, iv, ct)
	if err != nil {
		t.Fatalf("48 字节主密钥应可用（截断到 32）: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("48 字节主密钥截断后的结果不正确")
	}
}

func TestNSSAES256RejectsBadKeyLength(t *testing.T) {
	for _, n := range []int{0, 16, 24, 31, 33, 47, 64} {
		if _, err := nssAES256CBCDecrypt(make([]byte, n), make([]byte, 16), make([]byte, 16)); err == nil {
			t.Errorf("主密钥长度 %d 应被拒绝（只接受 32 或 48）", n)
		}
	}
}

// ===== NSS 的 IV 怪癖 =====

func TestNSSIVFromOctetStringQuirk(t *testing.T) {
	// 14 字节内容 → NSS 把 DER 的 tag/len 当成前两字节，凑成 16
	raw := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
	got, err := nssIVFromOctetString(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 16 {
		t.Fatalf("应补成 16 字节，实际 %d", len(got))
	}
	if got[0] != 0x04 || got[1] != 0x0e {
		t.Errorf("前两字节应为 DER OCTET STRING 的 tag/len (04 0e)，实际 %02x %02x", got[0], got[1])
	}
	if !bytes.Equal(got[2:], raw) {
		t.Error("后 14 字节应等于原内容")
	}

	// 已经是 16 字节的原样返回
	sixteen := make([]byte, 16)
	for i := range sixteen {
		sixteen[i] = byte(i)
	}
	got2, err := nssIVFromOctetString(sixteen)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got2, sixteen) {
		t.Error("16 字节 IV 应原样返回")
	}

	// 其它长度报错（而不是猜一个）
	for _, n := range []int{0, 8, 15, 17, 32} {
		if _, err := nssIVFromOctetString(make([]byte, n)); err == nil {
			t.Errorf("长度 %d 应被拒绝", n)
		}
	}
}

// ===== Mozilla KDF（结构测试；端到端需夹具）=====

// 形状：key 24 字节、iv 8 字节，且都来自同一个 40 字节 K
func TestNSSMozKDFShape(t *testing.T) {
	key, iv := nssMozKDF([]byte("globalSalt16bytes"), nil, []byte("entrySalt20bytes!!!!"))
	if len(key) != 24 {
		t.Errorf("key 应为 24 字节，实际 %d", len(key))
	}
	if len(iv) != 8 {
		t.Errorf("iv 应为 8 字节，实际 %d", len(iv))
	}
	// iv 必须是 K 的末 8 字节、key 是前 24 —— 两者不重叠
	if bytes.Equal(key[:8], iv) {
		t.Error("key 前 8 字节与 iv 不应相同（它们取自 K 的不同位置）")
	}
}

func TestNSSMozKDFSensitivity(t *testing.T) {
	gs := []byte("globalSalt16bytes")
	es := []byte("entrySalt20bytes!!!!")
	k0, i0 := nssMozKDF(gs, nil, es)

	for _, tc := range []struct {
		name       string
		gs, mp, es []byte
	}{
		{"globalSalt 变化", []byte("globalSalt16bytES"), nil, es},
		{"masterPassword 变化", gs, []byte("pw"), es},
		{"entrySalt 变化", gs, nil, []byte("entrySalt20bytes!!!?")},
	} {
		k, i := nssMozKDF(tc.gs, tc.mp, tc.es)
		if bytes.Equal(k, k0) && bytes.Equal(i, i0) {
			t.Errorf("%s 应改变派生结果", tc.name)
		}
	}
}

// 空主口令（未设主密码，最常见情形）必须可用且不 panic
func TestNSSMozKDFEmptyMasterPassword(t *testing.T) {
	key, iv := nssMozKDF([]byte("salt"), nil, []byte("es"))
	if len(key) != 24 || len(iv) != 8 {
		t.Errorf("空主口令也应产出 24/8 字节，实际 %d/%d", len(key), len(iv))
	}
	key2, iv2 := nssMozKDF([]byte("salt"), []byte{}, []byte("es"))
	if !bytes.Equal(key, key2) || !bytes.Equal(iv, iv2) {
		t.Error("nil 与空切片的主口令应等价")
	}
}

// entrySalt 短于 20 字节时，必须只在构造 pes 时右侧补零；
// 而 k1 的 `pes || entrySalt` 里那个 entrySalt 是**原始未补零**的。
//
// 这一处很容易写错：如果顺手把 entrySalt 本身补到 20 再用，k1 的输入就变成
// 40 字节（20+20），派生结果全错 —— 而且症状只是"解不开"。
// 所以这里直接对着分步公式比，而不是拿"补零后的 entrySalt"去比
// （后者会把两种不同输入当成等价，是错的）。
func TestNSSMozKDFPadsShortEntrySalt(t *testing.T) {
	globalSalt := []byte("gs")
	short := []byte("short") // 5 字节 < 20

	hp := sha1.Sum(concat(globalSalt, nil))
	chp := sha1.Sum(concat(hp[:], short))
	pes := make([]byte, 20)
	copy(pes, short)                           // pes 补零到 20
	k1 := hmacSHA1(chp[:], concat(pes, short)) // 但这里用**原始** short
	tk := hmacSHA1(chp[:], pes)
	k2 := hmacSHA1(chp[:], concat(tk, short))
	K := concat(k1, k2)

	gotKey, gotIV := nssMozKDF(globalSalt, nil, short)
	if !bytes.Equal(gotKey, K[:24]) {
		t.Errorf("key 不符\n期望 %x\n实际 %x", K[:24], gotKey)
	}
	if !bytes.Equal(gotIV, K[len(K)-8:]) {
		t.Errorf("iv 不符\n期望 %x\n实际 %x", K[len(K)-8:], gotIV)
	}

	// 反证：把 entrySalt 本身补零到 20 再用，结果应当**不同** ——
	// 这钉住了"k1 里用的是原始 entrySalt"这一点
	padded := make([]byte, 20)
	copy(padded, short)
	keyPadded, _ := nssMozKDF(globalSalt, nil, padded)
	if bytes.Equal(gotKey, keyPadded) {
		t.Error("把 entrySalt 预先补零竟然得到相同结果 —— 说明 k1 用了补零后的值，与参考实现不符")
	}
}

// 与参考实现的分步公式逐项对齐（firepwd.py 的 decryptMoz3DES）
//
// 这是**结构测试**：验证我把参考实现的每一步都照做了。
// 它不能证明参考实现本身对真实文件也正确 —— 那需要真实 profile 夹具。
func TestNSSMozKDFMatchesReferenceSteps(t *testing.T) {
	globalSalt := []byte("0123456789abcdef")
	masterPassword := []byte("")
	entrySalt := []byte("entrySaltHere12345")

	hp := sha1.Sum(concat(globalSalt, masterPassword))
	chp := sha1.Sum(concat(hp[:], entrySalt))
	pes := make([]byte, 20)
	copy(pes, entrySalt)
	k1 := hmacSHA1(chp[:], concat(pes, entrySalt))
	tk := hmacSHA1(chp[:], pes)
	k2 := hmacSHA1(chp[:], concat(tk, entrySalt))
	K := concat(k1, k2)

	wantKey, wantIV := K[:24], K[len(K)-8:]
	gotKey, gotIV := nssMozKDF(globalSalt, masterPassword, entrySalt)

	if hex.EncodeToString(gotKey) != hex.EncodeToString(wantKey) {
		t.Errorf("key 不符\n期望 %x\n实际 %x", wantKey, gotKey)
	}
	if hex.EncodeToString(gotIV) != hex.EncodeToString(wantIV) {
		t.Errorf("iv 不符\n期望 %x\n实际 %x", wantIV, gotIV)
	}
	if len(K) != 40 {
		t.Errorf("K 应为 40 字节（两个 20 字节 HMAC-SHA1），实际 %d", len(K))
	}
}

// ===== 测试辅助：加密端（生产代码只需解密）=====

func aesCBCEncryptForTest(t *testing.T, key, iv, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	return cbcEncryptForTest(block, iv, plaintext, aes.BlockSize)
}

func cbcEncryptForTest(block cipher.Block, iv, plaintext []byte, blockSize int) []byte {
	pad := blockSize - len(plaintext)%blockSize
	padded := append(append([]byte{}, plaintext...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out
}
