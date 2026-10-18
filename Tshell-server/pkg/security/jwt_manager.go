package security

import (
	"BackendTemplate/pkg/database"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTManager 增强的JWT管理器
type JWTManager struct {
	privateKey     *rsa.PrivateKey
	publicKey      *rsa.PublicKey
	keyID          string
	prevPrivateKey *rsa.PrivateKey // 上一轮换周期的密钥（重叠期内旧令牌仍可验证）
	prevPublicKey  *rsa.PublicKey
	prevKeyID      string
	keyCreatedAt   time.Time
	keyRotation    time.Duration
	mu             sync.RWMutex
	blacklist      map[string]time.Time // token黑名单
	refreshTokens  map[string]*RefreshToken
	userBlacklist  map[string]time.Time // 用户级别黑名单，用于撤销特定用户的所有令牌
}

// RefreshToken 刷新令牌
type RefreshToken struct {
	Token     string
	UserID    string
	Username  string // H2: 刷新时回填角色用
	ExpiresAt time.Time
	DeviceID  string
	LastUsed  time.Time
}

// Claims JWT声明
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	DeviceID string `json:"device_id"`
	KeyID    string `json:"kid"`
	Role     string `json:"role"` // C13 RBAC: admin | operator | auditor
	jwt.RegisteredClaims
}

// 令牌受众（aud）。用 aud 把"两步登录的中间态"与真正的访问令牌彻底分开：
//
//	MFA 第一步（口令通过、还差动态码）签发的 pending token 带 AudienceMFAPending；
//
// 如果 ValidateToken 不检查 aud，这个 pending token 就是一个**不需要第二因子
// 就能进后台的后门** —— 这是整个 MFA 设计里最要紧的一条约束，
// 两侧（拒绝 pending 当访问令牌 / 第二步只认 pending）都必须有测试钉住。
const (
	AudienceAccess     = "tshell-access"
	AudienceMFAPending = "tshell-mfa-pending"
)

// mfaPendingTTL 两步登录中间态的有效期。
// 短：它代表"口令已验证但第二因子未过"的半开状态；5 分钟足够用户掏出手机。
const mfaPendingTTL = 5 * time.Minute

// GlobalJWTManager 全局JWT管理器
var GlobalJWTManager *JWTManager

// jwtStateFile JWT 运行时状态持久化文件（黑名单/刷新令牌/用户黑名单）。
// 【P2-4】原实现纯内存：服务重启后已吊销令牌复活、刷新令牌失效（用户被强制下线）。
// 与 RSA 密钥同目录存放，权限 0600（含刷新令牌这类长期凭证）。
const jwtStateFile = "./jwt_state.json"

// jwtState 持久化快照结构
type jwtState struct {
	Blacklist     map[string]time.Time     `json:"blacklist"`
	RefreshTokens map[string]*RefreshToken `json:"refresh_tokens"`
	UserBlacklist map[string]time.Time     `json:"user_blacklist"`
}

// loadJWTState 从磁盘加载持久化状态（调用方：InitJWTManager，此时无并发）
func (jm *JWTManager) loadJWTState() {
	data, err := os.ReadFile(jwtStateFile)
	if err != nil {
		return // 首次启动/文件缺失：保持空状态
	}
	var st jwtState
	if err := json.Unmarshal(data, &st); err != nil {
		fmt.Printf("⚠️ [JWT] 状态文件解析失败（忽略，重新开始）: %v\n", err)
		return
	}
	if st.Blacklist != nil {
		jm.blacklist = st.Blacklist
	}
	if st.RefreshTokens != nil {
		jm.refreshTokens = st.RefreshTokens
	}
	if st.UserBlacklist != nil {
		jm.userBlacklist = st.UserBlacklist
	}
	// 加载后清理过期条目
	jm.cleanupExpiredTokens()
	fmt.Printf("🔐 [JWT] 已加载持久化状态: 黑名单=%d 刷新令牌=%d 用户黑名单=%d\n",
		len(jm.blacklist), len(jm.refreshTokens), len(jm.userBlacklist))
}

// saveJWTState 持久化当前状态（调用方必须已持有 jm.mu 写锁）
func (jm *JWTManager) saveJWTState() {
	st := jwtState{
		Blacklist:     jm.blacklist,
		RefreshTokens: jm.refreshTokens,
		UserBlacklist: jm.userBlacklist,
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	tmp := jwtStateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, jwtStateFile)
}

// InitJWTManager 初始化JWT管理器
func InitJWTManager(keySize int, rotationInterval time.Duration) error {
	manager := &JWTManager{
		keyRotation:   rotationInterval,
		blacklist:     make(map[string]time.Time),
		refreshTokens: make(map[string]*RefreshToken),
		userBlacklist: make(map[string]time.Time),
	}

	// 生成或加载RSA密钥对
	err := manager.loadOrGenerateKeys(keySize)
	if err != nil {
		return fmt.Errorf("初始化JWT密钥失败: %v", err)
	}

	GlobalJWTManager = manager

	manager.keyCreatedAt = time.Now()

	// 【P2-4】加载持久化的黑名单/刷新令牌状态（重启后吊销与登录态不丢失）
	manager.loadJWTState()

	// 启动密钥轮换和清理任务
	go manager.startMaintenanceTasks()

	return nil
}

// loadOrGenerateKeys 加载或生成RSA密钥对
func (jm *JWTManager) loadOrGenerateKeys(keySize int) error {
	privateKeyFile := "./jwt_private.pem"
	publicKeyFile := "./jwt_public.pem"

	// 尝试加载现有密钥
	if _, err := os.Stat(privateKeyFile); err == nil {
		return jm.loadKeysFromFile(privateKeyFile, publicKeyFile)
	}

	// 生成新密钥对
	return jm.generateAndSaveKeys(keySize, privateKeyFile, publicKeyFile)
}

// generateAndSaveKeys 生成并保存RSA密钥对
func (jm *JWTManager) generateAndSaveKeys(keySize int, privFile, pubFile string) error {
	// 生成RSA密钥对
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return fmt.Errorf("生成RSA密钥失败: %v", err)
	}

	jm.privateKey = privateKey
	jm.publicKey = &privateKey.PublicKey

	// 生成密钥ID
	keyIDBytes := make([]byte, 16)
	rand.Read(keyIDBytes)
	jm.keyID = hex.EncodeToString(keyIDBytes)

	// 保存私钥
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("序列化私钥失败: %v", err)
	}

	privKeyPEM := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	}

	err = os.WriteFile(privFile, pem.EncodeToMemory(privKeyPEM), 0600)
	if err != nil {
		return fmt.Errorf("保存私钥文件失败: %v", err)
	}

	// 保存公钥
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(jm.publicKey)
	if err != nil {
		return fmt.Errorf("序列化公钥失败: %v", err)
	}

	pubKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}

	err = os.WriteFile(pubFile, pem.EncodeToMemory(pubKeyPEM), 0644)
	if err != nil {
		return fmt.Errorf("保存公钥文件失败: %v", err)
	}

	return nil
}

// loadKeysFromFile 从文件加载RSA密钥对
func (jm *JWTManager) loadKeysFromFile(privFile, pubFile string) error {
	// 加载私钥
	privKeyData, err := os.ReadFile(privFile)
	if err != nil {
		return fmt.Errorf("读取私钥文件失败: %v", err)
	}

	privKeyBlock, _ := pem.Decode(privKeyData)
	if privKeyBlock == nil {
		return fmt.Errorf("解析私钥PEM失败")
	}

	privateKey, err := x509.ParsePKCS8PrivateKey(privKeyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("解析私钥失败: %v", err)
	}

	rsaPrivateKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("私钥不是RSA类型")
	}

	jm.privateKey = rsaPrivateKey
	jm.publicKey = &rsaPrivateKey.PublicKey

	// 生成新的密钥ID
	keyIDBytes := make([]byte, 16)
	rand.Read(keyIDBytes)
	jm.keyID = hex.EncodeToString(keyIDBytes)

	return nil
}

// GenerateTokenPair 生成访问令牌和刷新令牌对
func (jm *JWTManager) GenerateTokenPair(userID, username, deviceID string) (string, string, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	now := time.Now()

	// C13 RBAC：从数据库读取用户角色写入 Claims
	role := "admin"
	if database.DB() != nil {
		var user database.Users
		if has, err := database.DB().Where("username = ?", username).Get(&user); err == nil && has && user.Role != "" {
			role = user.Role
		}
	}

	// 生成访问令牌 (15分钟有效)
	accessClaims := &Claims{
		UserID:   userID,
		Username: username,
		DeviceID: deviceID,
		KeyID:    jm.keyID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "Tshell-Server",
			Subject:   userID,
			Audience:  jwt.ClaimStrings{AudienceAccess},
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(jm.privateKey)
	if err != nil {
		return "", "", fmt.Errorf("生成访问令牌失败: %v", err)
	}

	// 生成刷新令牌 (7天有效期)
	refreshTokenBytes := make([]byte, 32)
	rand.Read(refreshTokenBytes)
	refreshTokenString := hex.EncodeToString(refreshTokenBytes)

	refreshToken := &RefreshToken{
		Token:     refreshTokenString,
		UserID:    userID,
		Username:  username,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		DeviceID:  deviceID,
		LastUsed:  now,
	}

	jm.refreshTokens[refreshTokenString] = refreshToken
	jm.saveJWTState() // 【P2-4】持久化新签发刷新令牌

	return accessTokenString, refreshTokenString, nil
}

// ValidateToken 验证访问令牌
func (jm *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	// 检查黑名单（只读；过期条目由 cleanupExpiredTokens 持写锁清理）
	if expiry, exists := jm.blacklist[tokenString]; exists {
		if time.Now().Before(expiry) {
			return nil, fmt.Errorf("令牌已被吊销")
		}
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("意外的签名方法: %v", token.Header["alg"])
		}
		// 优先当前密钥；不匹配再尝试上一轮换周期的密钥（重叠期验证）
		if kid, _ := token.Header["kid"].(string); kid != "" && kid == jm.prevKeyID && jm.prevPublicKey != nil {
			return jm.prevPublicKey, nil
		}
		return jm.publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("解析令牌失败: %v", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("无效的令牌")
	}

	// 验证密钥ID（当前或上一密钥）
	if claims.KeyID != jm.keyID && claims.KeyID != jm.prevKeyID {
		return nil, fmt.Errorf("令牌密钥ID不匹配")
	}

	// 拒绝两步登录的中间态令牌。
	// 少了这一条，MFA 第一步返回的 pending token 就能直接当访问令牌用 ——
	// 等于第二因子形同虚设。（aud 只在这里与 ValidatePendingToken 里判断，
	// 两侧都必须检查；只查一边就等于没查。）
	if hasAudience(claims, AudienceMFAPending) {
		return nil, fmt.Errorf("该令牌仅用于 MFA 第二步，不能作为访问令牌")
	}

	// 检查用户级别黑名单
	if expiry, exists := jm.userBlacklist[claims.Username]; exists {
		if time.Now().Before(expiry) && claims.IssuedAt.Time.Before(expiry) {
			return nil, fmt.Errorf("用户所有令牌已被撤销")
		}
		// 清理过期的用户黑名单条目
		if time.Now().After(expiry) {
			delete(jm.userBlacklist, claims.Username)
		}
	}

	return claims, nil
}

// hasAudience 令牌受众里是否包含指定的值
func hasAudience(claims *Claims, want string) bool {
	for _, a := range claims.Audience {
		if a == want {
			return true
		}
	}
	return false
}

// PrivateKey 暴露私钥只读引用，供 MFA 密钥派生使用（InitMFASecretCipher）。
// 不提供导出/序列化接口：调用方拿到的是内存里的 *rsa.PrivateKey。
func (jm *JWTManager) PrivateKey() *rsa.PrivateKey {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	return jm.privateKey
}

// GeneratePendingToken 签发"口令已过、待第二因子"的中间态令牌（MFA 第一步）。
// 它带上了 deviceID：第二步要沿用同一个设备指纹，否则两步算出来的指纹不一致
// （generateDeviceID 依附请求上下文，第二步是一个新请求）。
func (jm *JWTManager) GeneratePendingToken(username, deviceID string) (string, error) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	now := time.Now()
	claims := &Claims{
		UserID:   username,
		Username: username,
		DeviceID: deviceID,
		KeyID:    jm.keyID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(mfaPendingTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "Tshell-Server",
			Subject:   username,
			Audience:  jwt.ClaimStrings{AudienceMFAPending},
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(jm.privateKey)
}

// ValidatePendingToken 校验两步登录的中间态令牌。
//
// 要求 aud **恰好**是 MFA pending：拿访问令牌来打第二步必须被拒
// （否则"已登录的人"可以绕过动态码重新走一遍 MFA 流程去换新令牌，
// 虽然影响有限，但语义上就是错的）。
func (jm *JWTManager) ValidatePendingToken(tokenString string) (*Claims, error) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("意外的签名方法: %v", token.Header["alg"])
		}
		if kid, _ := token.Header["kid"].(string); kid != "" && kid == jm.prevKeyID && jm.prevPublicKey != nil {
			return jm.prevPublicKey, nil
		}
		return jm.publicKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("解析中间态令牌失败: %v", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("无效的中间态令牌")
	}
	if claims.KeyID != jm.keyID && claims.KeyID != jm.prevKeyID {
		return nil, fmt.Errorf("令牌密钥ID不匹配")
	}
	if !hasAudience(claims, AudienceMFAPending) {
		return nil, fmt.Errorf("不是 MFA 中间态令牌")
	}
	return claims, nil
}

// RefreshAccessToken 使用刷新令牌生成新的访问令牌（D43：签发新 refresh token 并吊销旧 token）
func (jm *JWTManager) RefreshAccessToken(refreshTokenString string) (string, string, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	refreshToken, exists := jm.refreshTokens[refreshTokenString]
	if !exists {
		return "", "", fmt.Errorf("无效的刷新令牌")
	}

	if time.Now().After(refreshToken.ExpiresAt) {
		delete(jm.refreshTokens, refreshTokenString)
		return "", "", fmt.Errorf("刷新令牌已过期")
	}

	// 更新最后使用时间
	refreshToken.LastUsed = time.Now()

	// H2: 刷新时从 DB 回填角色与用户名（原 access claims 缺 Role → 空 role 被 RequireRole 当 admin，普通用户可提权）
	now := time.Now()
	username := refreshToken.Username
	role := ""
	if database.DB() != nil {
		var user database.Users
		if has, err := database.DB().Where("username = ?", refreshToken.Username).Get(&user); err == nil && has && user.Role != "" {
			role = user.Role
			username = user.Username
		}
	}
	if role == "" {
		// 用户已删除或角色缺失：拒绝刷新，防止空 role 被默认为 admin
		delete(jm.refreshTokens, refreshTokenString)
		return "", "", fmt.Errorf("用户角色缺失，刷新被拒绝")
	}
	accessClaims := &Claims{
		UserID:   refreshToken.UserID,
		Username: username,
		DeviceID: refreshToken.DeviceID,
		KeyID:    jm.keyID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "Tshell-Server",
			Subject:   refreshToken.UserID,
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(jm.privateKey)
	if err != nil {
		return "", "", err
	}

	// D43: 刷新令牌轮换——签发新 refresh token 并吊销旧 token（被盗的 refresh token 无法长期复用）
	newRefreshBytes := make([]byte, 32)
	if _, err := rand.Read(newRefreshBytes); err != nil {
		return "", "", fmt.Errorf("生成刷新令牌失败: %v", err)
	}
	newRefreshTokenString := hex.EncodeToString(newRefreshBytes)

	newRefreshToken := &RefreshToken{
		Token:     newRefreshTokenString,
		UserID:    refreshToken.UserID,
		Username:  refreshToken.Username,
		ExpiresAt: refreshToken.ExpiresAt, // 沿用原有效期（不无限延长）
		DeviceID:  refreshToken.DeviceID,
		LastUsed:  now,
	}
	jm.refreshTokens[newRefreshTokenString] = newRefreshToken
	delete(jm.refreshTokens, refreshTokenString)
	jm.saveJWTState() // 【P2-4】持久化刷新令牌轮换结果

	return accessTokenString, newRefreshTokenString, nil
}

// RevokeToken 吊销令牌
func (jm *JWTManager) RevokeToken(tokenString string) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	// 解析令牌获取过期时间
	token, _ := jwt.ParseWithClaims(tokenString, &Claims{}, nil)
	if claims, ok := token.Claims.(*Claims); ok {
		jm.blacklist[tokenString] = claims.ExpiresAt.Time
		jm.saveJWTState() // 【P2-4】持久化吊销（重启后仍生效）
	}
}

// RevokeRefreshToken 吊销刷新令牌
func (jm *JWTManager) RevokeRefreshToken(refreshTokenString string) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	delete(jm.refreshTokens, refreshTokenString)
	jm.saveJWTState() // 【P2-4】持久化吊销
}

// startMaintenanceTasks 启动维护任务
func (jm *JWTManager) startMaintenanceTasks() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		jm.cleanupExpiredTokens()
		jm.maybeRotateKeys()
	}
}

// maybeRotateKeys 到达轮换周期时更换签名密钥（旧密钥保留一个轮换周期用于验证存量令牌）。
// 【安全】原实现签名密钥永久不变，私钥一旦泄露即可伪造任意令牌且无法撤销；
// 轮换后泄露的旧密钥只能伪造已过期令牌（访问令牌 TTL 15 分钟），显著缩小危害窗口。
func (jm *JWTManager) maybeRotateKeys() {
	if jm.keyRotation <= 0 {
		return
	}
	jm.mu.RLock()
	due := time.Since(jm.keyCreatedAt) >= jm.keyRotation
	jm.mu.RUnlock()
	if !due {
		return
	}
	if err := jm.rotateKeys(); err != nil {
		fmt.Printf("⚠️ [JWT] 密钥轮换失败: %v\n", err)
	}
}

// rotateKeys 生成并保存新密钥对，旧密钥转为上一密钥（重叠期验证）。
func (jm *JWTManager) rotateKeys() error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	// 当前密钥降级为上一密钥
	jm.prevPrivateKey = jm.privateKey
	jm.prevPublicKey = jm.publicKey
	jm.prevKeyID = jm.keyID

	keySize := jm.privateKey.N.BitLen()
	if err := jm.generateAndSaveKeys(keySize, "./jwt_private.pem", "./jwt_public.pem"); err != nil {
		// 生成失败：回滚，保留原密钥
		jm.privateKey = jm.prevPrivateKey
		jm.publicKey = jm.prevPublicKey
		jm.keyID = jm.prevKeyID
		jm.prevPrivateKey = nil
		jm.prevPublicKey = nil
		jm.prevKeyID = ""
		return err
	}
	jm.keyCreatedAt = time.Now()
	fmt.Printf("🔄 [JWT] 签名密钥已轮换: 新KeyID=%s（旧KeyID=%s 重叠期内仍可验证）\n", jm.keyID, jm.prevKeyID)
	return nil
}

// cleanupExpiredTokens 清理过期令牌
func (jm *JWTManager) cleanupExpiredTokens() {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	now := time.Now()

	// 清理过期的黑名单条目
	for token, expiry := range jm.blacklist {
		if now.After(expiry) {
			delete(jm.blacklist, token)
		}
	}

	// 清理过期的用户黑名单条目
	for username, expiry := range jm.userBlacklist {
		if now.After(expiry) {
			delete(jm.userBlacklist, username)
		}
	}

	// 清理过期的刷新令牌
	for token, refreshToken := range jm.refreshTokens {
		if now.After(refreshToken.ExpiresAt) {
			delete(jm.refreshTokens, token)
		}
	}

	// 【P2-4】清理后同步持久化（周期维护任务内）
	jm.saveJWTState()
}

// RevokeUserTokens 撤销指定用户的所有令牌
func (jm *JWTManager) RevokeUserTokens(username string, expireTime time.Time) error {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	// 将用户加入黑名单，所有在此时间之前签发的令牌都将被撤销
	jm.userBlacklist[username] = expireTime

	// 删除用户的所有刷新令牌
	for token, refreshToken := range jm.refreshTokens {
		if refreshToken.UserID == username {
			delete(jm.refreshTokens, token)
		}
	}
	jm.saveJWTState() // 【P2-4】持久化用户级撤销

	return nil
}
