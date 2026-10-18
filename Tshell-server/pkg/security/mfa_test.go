package security

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strings"
	"testing"
)

func TestMFASecretRoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitMFASecretCipher(key); err != nil {
		t.Fatal(err)
	}
	if !MFACipherReady() {
		t.Fatal("初始化后加密器应可用")
	}

	secret := []byte("12345678901234567890")
	enc, err := SealMFASecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, mfaCipherLabel) {
		t.Errorf("密文应带版本前缀 %q，便于将来换算法", mfaCipherLabel)
	}
	// 密文里不能出现明文（哪怕 base64 之后）
	if strings.Contains(enc, string(secret)) {
		t.Fatal("密文中不应出现明文密钥")
	}

	got, err := OpenMFASecret(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(secret) {
		t.Fatalf("往返不一致: %q", got)
	}

	// 同一个密钥加密两次结果必须不同（nonce 随机），否则等于泄露"两个用户同密钥"
	enc2, _ := SealMFASecret(secret)
	if enc == enc2 {
		t.Error("相同明文两次加密应产生不同密文（nonce 必须随机）")
	}
}

// 未初始化时必须拒绝写入，而不是明文落库
func TestSealRejectsWhenNotInitialized(t *testing.T) {
	mfaCipherMu.Lock()
	saved := mfaAEAD
	mfaAEAD = nil
	mfaCipherMu.Unlock()
	defer func() {
		mfaCipherMu.Lock()
		mfaAEAD = saved
		mfaCipherMu.Unlock()
	}()

	if _, err := SealMFASecret([]byte("x")); err == nil {
		t.Fatal("未初始化时应拒绝写入（不允许明文落库）")
	} else if !strings.Contains(err.Error(), "明文") {
		t.Errorf("错误信息应说明为什么拒绝: %v", err)
	}
}

func TestOpenMFASecretErrors(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	if err := InitMFASecretCipher(key); err != nil {
		t.Fatal(err)
	}

	// 空密文 = 用户没开 MFA，与"解不开"必须区分开
	if _, err := OpenMFASecret(""); !errors.Is(err, ErrMFANotEnabled) {
		t.Fatalf("空密文应返回 ErrMFANotEnabled，实际 %v", err)
	}

	enc, err := SealMFASecret([]byte("12345678901234567890"))
	if err != nil {
		t.Fatal(err)
	}

	// 篡改密文必须解不开（不能悄悄返回一段垃圾当密钥）
	tampered := []byte(enc)
	tampered[len(tampered)-2] ^= 0x01
	if _, err := OpenMFASecret(string(tampered)); err == nil {
		t.Fatal("被篡改的密文不应解密成功")
	}

	// 格式不对的密文要有明确错误，而不是当成"没开启"
	for _, bad := range []string{"plaintext", "v2:abcd", "v1:!!!notbase64!!!", mfaCipherLabel} {
		if _, err := OpenMFASecret(bad); err == nil {
			t.Errorf("非法密文 %q 应报错", bad)
		} else if errors.Is(err, ErrMFANotEnabled) {
			t.Errorf("非法密文 %q 不应被当成\"未开启 MFA\"（那会静默降级为单因子）", bad)
		}
	}
}

// 换过私钥（或私钥被改写）时，旧密文必须解不开并报出可识别的错误
func TestOpenFailsAfterKeyChange(t *testing.T) {
	key1, _ := rsa.GenerateKey(rand.Reader, 2048)
	if err := InitMFASecretCipher(key1); err != nil {
		t.Fatal(err)
	}
	enc, err := SealMFASecret([]byte("12345678901234567890"))
	if err != nil {
		t.Fatal(err)
	}

	key2, _ := rsa.GenerateKey(rand.Reader, 2048)
	if err := InitMFASecretCipher(key2); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMFASecret(enc); err == nil {
		t.Fatal("换密钥后旧密文不应解密成功")
	} else if errors.Is(err, ErrMFANotEnabled) {
		t.Error("解不开不能报成\"未开启\"（否则 MFA 会被静默关掉）")
	}

	// 换回来应仍可解（派生是确定性的，这也验证了多副本/重启后可用）
	if err := InitMFASecretCipher(key1); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMFASecret(enc); err != nil {
		t.Fatalf("派生是确定性的，换回原私钥应能解开: %v", err)
	}
}
