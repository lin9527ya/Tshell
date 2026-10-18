package security

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"
)

// 这两个方向的隔离是 MFA 的地基，必须有测试钉住：
//
//  1. pending token 不能当访问令牌用（否则第二因子形同虚设）
//  2. 访问令牌不能拿去打第二步（语义上就该拒）
func TestMFAPendingTokenAudienceIsolation(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitMFASecretCipher(key); err != nil {
		t.Fatal(err)
	}
	jm := &JWTManager{
		privateKey:    key,
		publicKey:     &key.PublicKey,
		keyID:         "test-kid",
		keyRotation:   time.Hour,
		blacklist:     make(map[string]time.Time),
		refreshTokens: make(map[string]*RefreshToken),
		userBlacklist: make(map[string]time.Time),
	}
	saved := GlobalJWTManager
	GlobalJWTManager = jm
	defer func() { GlobalJWTManager = saved }()

	pending, err := jm.GeneratePendingToken("admin", "device-1")
	if err != nil {
		t.Fatal(err)
	}

	// 1) pending token 必须是合法的 pending
	claims, err := jm.ValidatePendingToken(pending)
	if err != nil {
		t.Fatalf("pending token 应通过 ValidatePendingToken: %v", err)
	}
	if claims.Username != "admin" || claims.DeviceID != "device-1" {
		t.Fatalf("pending token 应带上用户名与设备指纹: %+v", claims)
	}

	// 2) **关键断言**：pending token 绝不能当访问令牌
	if _, err := jm.ValidateToken(pending); err == nil {
		t.Fatal("pending token 被 ValidateToken 接受了 —— 第二因子形同虚设")
	}

	// 3) 反过来：访问令牌不能打第二步
	//    （GenerateTokenPair 会读库取角色，这里直接手工签一个 aud=access 的令牌）
	access := signAccessForTest(t, jm)
	if _, err := jm.ValidatePendingToken(access); err == nil {
		t.Fatal("访问令牌不应被当作 MFA 中间态令牌")
	}
	if _, err := jm.ValidateToken(access); err != nil {
		t.Fatalf("访问令牌自身应有效: %v", err)
	}
}

// 中间态令牌必须短寿：它代表"口令已过、第二因子未过"的半开状态
func TestMFAPendingTokenLifetime(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	jm := &JWTManager{
		privateKey: key, publicKey: &key.PublicKey, keyID: "k",
		blacklist: map[string]time.Time{}, refreshTokens: map[string]*RefreshToken{},
		userBlacklist: map[string]time.Time{},
	}
	token, err := jm.GeneratePendingToken("u", "d")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := jm.ValidatePendingToken(token)
	if err != nil {
		t.Fatal(err)
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl > mfaPendingTTL+time.Minute || ttl < mfaPendingTTL-time.Minute {
		t.Fatalf("中间态有效期应约等于 %v，实际 %v", mfaPendingTTL, ttl)
	}
	if mfaPendingTTL > 10*time.Minute {
		t.Errorf("中间态有效期 %v 偏长：这是半开状态，越短越好", mfaPendingTTL)
	}
}

// 签发 pending token 时不能顺手发出刷新令牌（那等于绕过第二因子拿到长期凭据）
func TestPendingTokenHasNoRefreshToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	jm := &JWTManager{
		privateKey: key, publicKey: &key.PublicKey, keyID: "k",
		blacklist: map[string]time.Time{}, refreshTokens: make(map[string]*RefreshToken),
		userBlacklist: map[string]time.Time{},
	}
	if _, err := jm.GeneratePendingToken("u", "d"); err != nil {
		t.Fatal(err)
	}
	if n := len(jm.refreshTokens); n != 0 {
		t.Fatalf("签发中间态令牌不应产生刷新令牌，实际新增 %d 个", n)
	}
}
