package security

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===== OPSEC P0-1：战果静态加密 =====

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 7)
	}
	return k
}

func initTestCipher(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "k")
	if err := os.WriteFile(p, testKey(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InitAtRestCipherFromKeyFile(p); err != nil {
		t.Fatal(err)
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	initTestCipher(t)
	for _, plain := range []string{"p@ssw0rd!", "a5f3c1...NTLM", "票据密钥", strings.Repeat("x", 500)} {
		sealed, err := Seal(plain)
		if err != nil {
			t.Fatalf("加密 %q 失败: %v", plain, err)
		}
		if !IsSealed(sealed) {
			t.Errorf("密文应带 v1: 前缀")
		}
		if strings.Contains(sealed, plain) && len(plain) > 3 {
			t.Errorf("密文里不应出现原文")
		}
		got, err := Open(sealed)
		if err != nil {
			t.Fatalf("解密失败: %v", err)
		}
		if got != plain {
			t.Errorf("往返不一致：%q -> %q", plain, got)
		}
	}
}

// 同一明文两次加密必须产生不同密文（GCM 随机 nonce）——
// 这正是"不能用 Secret 列本身做去重键"的根因，指纹列存在的前提。
func TestSealIsRandomized(t *testing.T) {
	initTestCipher(t)
	a, err := Seal("same-secret")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Seal("same-secret")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("两次加密结果相同：nonce 未随机化，去重指纹的设计前提不成立")
	}
	if pa, _ := Open(a); pa != "same-secret" {
		t.Error("第一份密文解错")
	}
	if pb, _ := Open(b); pb != "same-secret" {
		t.Error("第二份密文解错")
	}
}

func TestSealEmptyIsEmpty(t *testing.T) {
	initTestCipher(t)
	sealed, err := Seal("")
	if err != nil || sealed != "" {
		t.Errorf("空串应原样返回空，得到 %q err=%v", sealed, err)
	}
	if got, err := Open(""); err != nil || got != "" {
		t.Errorf("空串解密应返回空，得到 %q err=%v", got, err)
	}
}

// 历史明文（无前缀）必须原样返回：否则升级后老库直接不可读
func TestOpenLegacyPlaintextPassesThrough(t *testing.T) {
	initTestCipher(t)
	got, err := Open("legacy-plain-password")
	if err != nil {
		t.Fatalf("历史明文不应报错: %v", err)
	}
	if got != "legacy-plain-password" {
		t.Errorf("历史明文应原样返回，得到 %q", got)
	}
}

// 密钥不匹配时必须**报错**，不能返回空串让上层以为"这条是空的"
func TestOpenWrongKeyFailsLoudly(t *testing.T) {
	initTestCipher(t)
	sealed, err := Seal("secret")
	if err != nil {
		t.Fatal(err)
	}
	// 换一把密钥
	dir := t.TempDir()
	other := bytes.Repeat([]byte{0xEE}, 32)
	p := filepath.Join(dir, "k2")
	if err := os.WriteFile(p, other, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InitAtRestCipherFromKeyFile(p); err != nil {
		t.Fatal(err)
	}
	got, err := Open(sealed)
	if err == nil {
		t.Fatal("换密钥后解密必须报错，不能静默返回空串")
	}
	if got != "" {
		t.Errorf("失败时不应返回内容，得到 %q", got)
	}
}

func TestSealBeforeInitFails(t *testing.T) {
	// 未初始化时必须硬失败（不能退化为明文）
	atRestMu.Lock()
	oldAE, oldFP := atRestAE, atRestFPK
	atRestAE, atRestFPK = nil, nil
	atRestMu.Unlock()
	defer func() {
		atRestMu.Lock()
		atRestAE, atRestFPK = oldAE, oldFP
		atRestMu.Unlock()
	}()

	if _, err := Seal("x"); err == nil {
		t.Error("未初始化时 Seal 必须报错")
	}
	if _, err := Fingerprint("x"); err == nil {
		t.Error("未初始化时 Fingerprint 必须报错")
	}
}

// 指纹：确定性 + 不泄露原文 + 不随加密随机化
func TestFingerprintIsStableAndOpaque(t *testing.T) {
	initTestCipher(t)
	a1, err := Fingerprint("mypass123")
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := Fingerprint("mypass123")
	b, _ := Fingerprint("mypass124")
	if a1 != a2 {
		t.Error("同一口令的指纹必须稳定（去重要靠它）")
	}
	if a1 == b {
		t.Error("不同口令的指纹不应相同")
	}
	if len(a1) != 64 {
		t.Errorf("指纹应为 64 位 hex（SHA256），实际 %d", len(a1))
	}
	if strings.Contains(a1, "mypass") {
		t.Error("指纹不得包含原文")
	}
	if strings.Contains(a1, hex.EncodeToString([]byte("mypass123"))) {
		t.Error("指纹不得是裸哈希（应带密钥 HMAC，防字典反查）")
	}
}

// 密钥文件三种形态都接受：原始 32 字节 / hex / base64
func TestInitAcceptsKeyFileFormats(t *testing.T) {
	k := testKey()
	cases := map[string][]byte{
		"raw":    k,
		"hex":    []byte(hex.EncodeToString(k)),
		"base64": []byte(base64.StdEncoding.EncodeToString(k)),
	}
	for name, content := range cases {
		dir := t.TempDir()
		p := filepath.Join(dir, "k")
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := InitAtRestCipherFromKeyFile(p); err != nil {
			t.Errorf("%s 形态应被接受: %v", name, err)
		}
	}
}

func TestInitRejectsBadKeyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "k")
	if err := os.WriteFile(p, []byte("too-short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InitAtRestCipherFromKeyFile(p); err == nil {
		t.Error("长度不对的密钥文件必须被拒绝")
	}
	if err := InitAtRestCipherFromKeyFile(filepath.Join(dir, "missing")); err == nil {
		t.Error("缺失的密钥文件必须报错")
	}
}
