package security

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TestJWTManagerHonoursConfiguredTTL config.jwt.access_token_ttl /
// refresh_token_ttl 之前未被读取；这里钉住签发时的实际有效期。
func TestJWTManagerHonoursConfiguredTTL(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jm := &JWTManager{
		privateKey:      key,
		publicKey:       &key.PublicKey,
		keyID:           "test-kid",
		accessTokenTTL:  2 * time.Minute,
		refreshTokenTTL: 3 * time.Hour,
		blacklist:       make(map[string]time.Time),
		refreshTokens:   make(map[string]*RefreshToken),
		userBlacklist:   make(map[string]time.Time),
	}

	// GenerateTokenPair 会调用 saveJWTState() 写 jwt_state.json，切到临时目录避免污染仓库
	oldWd, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	access, refresh, err := jm.GenerateTokenPair("user-1", "alice", "device-1")
	if err != nil {
		t.Fatal(err)
	}

	claims := &Claims{}
	if _, _, err := jwt.NewParser().ParseUnverified(access, claims); err != nil {
		t.Fatal(err)
	}
	accessTTL := time.Until(claims.ExpiresAt.Time)
	if accessTTL < 110*time.Second || accessTTL > 125*time.Second {
		t.Fatalf("access token TTL = %v，期望约 2m", accessTTL)
	}

	rec, ok := jm.refreshTokens[refresh]
	if !ok {
		t.Fatal("刷新令牌未写入内存表")
	}
	refreshTTL := time.Until(rec.ExpiresAt)
	if refreshTTL < 175*time.Minute || refreshTTL > 185*time.Minute {
		t.Fatalf("refresh token TTL = %v，期望约 3h", refreshTTL)
	}
}
