package security

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signAccessForTest 手工签一个 aud=access 的令牌（GenerateTokenPair 会读库取角色）
func signAccessForTest(t *testing.T, jm *JWTManager) string {
	t.Helper()
	now := time.Now()
	claims := &Claims{
		UserID: "admin", Username: "admin", DeviceID: "device-1", KeyID: jm.keyID,
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "Tshell-Server",
			Subject:   "admin",
			Audience:  jwt.ClaimStrings{AudienceAccess},
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(jm.privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
