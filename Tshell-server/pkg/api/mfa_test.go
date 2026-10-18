package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"BackendTemplate/pkg/security/otp"

	"github.com/gin-gonic/gin"
)

// MFA 端到端：真的起一个 gin 路由、真的建库、真的签名令牌。
//
// 这里守的是整个 MFA 设计里最要紧的几条约束 —— 单测覆盖不到接线：
//  1. 未开 MFA 的账号：登录响应与改动前逐字段一致（回归）
//  2. 开 MFA 不带码：只回中间态，**不得**出现 access/refresh，**不得**设 Cookie
//  3. 中间态令牌**不能**进受保护路由（少了这条，第二因子形同虚设）
//  4. 错码不锁号、对码能登录、同码不能重放、恢复码一次性
//  5. disable 后回到单因子；管理员 reset 后目标用户可单因子登录

const testMFAPassword = "Str0ng-Passw0rd!"

func setupMFATest(t *testing.T) (*gin.Engine, func(username string) database.Users) {
	return setupMFATestWithUsers(t)
}

// setupMFATestWithUsers 同上，但可追加测试账号（用于需要独立用户名、避免共享
// 包级重放守卫互相干扰的用例）。
func setupMFATestWithUsers(t *testing.T, extraUsers ...string) (*gin.Engine, func(username string) database.Users) {
	t.Helper()
	// JWT 密钥/状态文件写到临时目录，避免污染工作区
	t.Chdir(t.TempDir())

	if err := security.InitJWTManager(2048, time.Hour); err != nil {
		t.Fatalf("初始化 JWT 管理器失败: %v", err)
	}
	if err := security.InitMFASecretCipher(security.GlobalJWTManager.PrivateKey()); err != nil {
		t.Fatalf("初始化 MFA 加密器失败: %v", err)
	}

	// 建库（含 MFA 三列）
	setupMFATestDB(t, extraUsers...)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 必须与生产一致地注册错误处理中间件：AbortWithError 只是"登记错误 + Abort"，
	// 真正把 AppError 翻译成 HTTP 状态码与响应体的是这个中间件（main.go 里也挂了）。
	// 不挂它，测试里所有失败路径都会得到"200 + 空 body"，从而把断言放过去。
	r.Use(errors.ErrorHandlerMiddleware())
	r.POST("/api/users/login", LoginHandler)
	r.POST("/api/users/login/mfa", MFALoginHandler)

	// 受保护路由：用来验证"中间态令牌进不来"
	protected := r.Group("/api")
	protected.Use(AuthMiddleware())
	protected.GET("/users/mfa", MFAMeHandler)

	// 自助管理（同样需要认证）
	mg := r.Group("/api")
	mg.Use(AuthMiddleware())
	mg.POST("/users/mfa/start", MFAStartHandler)
	mg.POST("/users/mfa/confirm", MFAConfirmHandler)
	mg.POST("/users/mfa/disable", MFADisableHandler)
	mg.GET("/users/mfa/qr", MFAQRHandler)

	// 管理员重置
	adminOnly := r.Group("/api")
	adminOnly.Use(AuthMiddleware(), security.RequireRole(security.RoleAdmin))
	adminOnly.POST("/users/mfa/reset", MFAResetHandler)

	load := func(username string) database.Users {
		var u database.Users
		if _, err := database.DB().Where("username = ?", username).Get(&u); err != nil {
			t.Fatalf("读取用户 %s 失败: %v", username, err)
		}
		return u
	}
	return r, load
}

// setupMFATestDB 建一个带 MFA 字段的临时库并塞入测试账号
func setupMFATestDB(t *testing.T, extraUsers ...string) {
	t.Helper()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(t.TempDir(), "mfa-test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() {
		if database.Engine != nil {
			database.Engine.Close()
		}
	})
	accounts := []struct {
		name string
		role string
	}{
		{"alice", security.RoleAdmin},
		{"bob", security.RoleOperator},
	}
	for _, name := range extraUsers {
		accounts = append(accounts, struct {
			name string
			role string
		}{name, security.RoleOperator})
	}
	for _, u := range accounts {
		hash, err := database.HashPassword(testMFAPassword)
		if err != nil {
			t.Fatalf("哈希口令失败: %v", err)
		}
		rec := &database.Users{Username: u.name, PasswordHash: hash, Role: u.role}
		if _, err := database.DB().Insert(rec); err != nil {
			t.Fatalf("插入用户 %s 失败: %v", u.name, err)
		}
	}
}

// enableMFAForTest 直接把某个用户置为"已开启 MFA"，返回其 TOTP 密钥
func enableMFAForTest(t *testing.T, username string) []byte {
	t.Helper()
	secret, err := otp.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := security.SealMFASecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	codes, err := otp.NewRecoveryCodes(3)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := otp.HashRecoveryCodes(codes)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(hashes)
	if _, err := database.DB().Where("username = ?", username).
		Cols("mfa_secret_enc", "mfa_enabled", "mfa_recovery_hashes").
		Update(&database.Users{MFASecretEnc: sealed, MFAEnabled: true, MFARecoveryHashes: string(encoded)}); err != nil {
		t.Fatalf("开启 MFA 失败: %v", err)
	}
	return secret
}

func mfaPost(t *testing.T, r *gin.Engine, path string, body interface{}, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// decodeData 取出统一响应体里的 data 字段
func mfaData(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var resp struct {
		Success bool                   `json:"success"`
		Code    int                    `json:"code"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是合法 JSON: %s", w.Body.String())
	}
	return resp.Data
}

// ===== 1. 回归：未开 MFA 的账号登录行为与改动前一致 =====
func TestLoginWithoutMFAResponseUnchanged(t *testing.T) {
	r, _ := setupMFATest(t)

	w := mfaPost(t, r, "/api/users/login", gin.H{
		"username": "bob", "password": testMFAPassword,
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("登录应成功，得到 %d: %s", w.Code, w.Body.String())
	}
	data := mfaData(t, w)
	for _, key := range []string{"access_token", "refresh_token", "expires_in", "token_type", "permissions", "username", "role"} {
		if _, ok := data[key]; !ok {
			t.Errorf("登录响应缺少字段 %s（前端依赖它，不能因为加了 MFA 就少）", key)
		}
	}
	if data["mfa_required"] == true {
		t.Error("未开 MFA 的账号不应要求第二步")
	}
	// 两个 Cookie 必须都设上（否则页面刷新就掉线）
	if !mfaHasCookie(w, "access_token") || !mfaHasCookie(w, "refresh_token") {
		t.Errorf("未开 MFA 的登录应设置两个 Cookie，实际: %v", w.Header().Values("Set-Cookie"))
	}
}

func mfaHasCookie(w *httptest.ResponseRecorder, name string) bool {
	for _, c := range w.Header().Values("Set-Cookie") {
		if strings.HasPrefix(c, name+"=") {
			return true
		}
	}
	return false
}

// ===== 2. 开 MFA 不带码：只回中间态，且不得泄露任何凭据 =====
func TestLoginWithMFAReturnsOnlyPendingToken(t *testing.T) {
	r, _ := setupMFATest(t)
	enableMFAForTest(t, "bob")

	w := mfaPost(t, r, "/api/users/login", gin.H{
		"username": "bob", "password": testMFAPassword,
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("第一步应返回 200（这不是失败，是还差一步），得到 %d: %s", w.Code, w.Body.String())
	}
	data := mfaData(t, w)
	if data["mfa_required"] != true {
		t.Fatalf("应要求第二步，实际: %v", data)
	}
	if _, ok := data["pending_token"]; !ok {
		t.Fatal("应返回 pending_token")
	}

	// **关键**：这一步绝不能给出任何真凭据
	for _, forbidden := range []string{"access_token", "refresh_token"} {
		if v, ok := data[forbidden]; ok {
			t.Errorf("第一步不得下发 %s（那等于第二因子形同虚设）: %v", forbidden, v)
		}
	}
	if len(w.Header().Values("Set-Cookie")) != 0 {
		t.Errorf("第一步不得设置任何 Cookie: %v", w.Header().Values("Set-Cookie"))
	}
}

// ===== 3. 中间态令牌不能进受保护路由 =====
func TestPendingTokenCannotAccessProtectedRoutes(t *testing.T) {
	r, _ := setupMFATest(t)
	enableMFAForTest(t, "bob")

	w := mfaPost(t, r, "/api/users/login", gin.H{
		"username": "bob", "password": testMFAPassword,
	}, nil)
	pending, _ := mfaData(t, w)["pending_token"].(string)
	if pending == "" {
		t.Fatal("没拿到 pending_token")
	}

	// Authorization2 头（AuthMiddleware 优先读它）
	req := httptest.NewRequest(http.MethodGet, "/api/users/mfa", nil)
	req.Header.Set("Authorization2", "Bearer "+pending)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("中间态令牌竟然能访问受保护路由 —— 这是不需要第二因子就能进后台的后门")
	}
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Fatalf("应返回 401/403，实际 %d: %s", rec.Code, rec.Body.String())
	}
}

// ===== 4. 第二步：错码不锁号、对码能登录 =====
func TestMFASecondStepRejectsWrongCodeWithoutLocking(t *testing.T) {
	r, load := setupMFATest(t)
	secret := enableMFAForTest(t, "bob")

	pending := loginPending(t, r, "bob")

	// 错码
	w := mfaPost(t, r, "/api/users/login/mfa", gin.H{
		"pending_token": pending, "totp_code": "000000",
	}, nil)
	if w.Code == http.StatusOK {
		t.Fatalf("错误动态码不应通过: %s", w.Body.String())
	}

	// **不得**把口令失败的锁号机制用在这里：用户手滑 5 次就被锁号，
	// 锁了连恢复码都用不了，等于把自己关在门外
	u := load("bob")
	if u.Locked {
		t.Fatal("动态码输错不应锁定账号（那是口令失败的惩罚机制）")
	}
	if u.LoginAttempts >= 5 {
		t.Fatalf("动态码输错不应累计到锁号阈值，实际 %d", u.LoginAttempts)
	}

	// 对码 → 成功
	code := otp.Code(secret, time.Now())
	w2 := mfaPost(t, r, "/api/users/login/mfa", gin.H{
		"pending_token": pending, "totp_code": code,
	}, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("正确动态码应通过，得到 %d: %s", w2.Code, w2.Body.String())
	}
	data := mfaData(t, w2)
	if _, ok := data["access_token"]; !ok {
		t.Fatal("第二步成功应签发 access_token")
	}
	if !mfaHasCookie(w2, "access_token") || !mfaHasCookie(w2, "refresh_token") {
		t.Error("第二步成功应设置两个 Cookie（与第一步成功的响应保持一致）")
	}
}

func loginPending(t *testing.T, r *gin.Engine, user string) string {
	t.Helper()
	w := mfaPost(t, r, "/api/users/login", gin.H{
		"username": user, "password": testMFAPassword,
	}, nil)
	tok, _ := mfaData(t, w)["pending_token"].(string)
	if tok == "" {
		t.Fatalf("未取得 pending_token: %s", w.Body.String())
	}
	return tok
}

// ===== 5. 同一动态码不能重放 =====
// 重放守卫是**包级**状态（真实部署里按用户名隔离，跨用户不会互相影响）。
// 这里用一个专属账号名，避免与其他用例的 bob 撞上同一个守卫条目。
func TestMFACodeCannotBeReplayed(t *testing.T) {
	r, _ := setupMFATestWithUsers(t, "replayuser")
	secret := enableMFAForTest(t, "replayuser")
	code := otp.Code(secret, time.Now())

	pending1 := loginPending(t, r, "replayuser")
	if w := mfaPost(t, r, "/api/users/login/mfa", gin.H{
		"pending_token": pending1, "totp_code": code,
	}, nil); w.Code != http.StatusOK {
		t.Fatalf("第一次使用应通过: %s", w.Body.String())
	}

	// 同一个码（仍在同一 30s 窗口内）再来一次必须被拒
	pending2 := loginPending(t, r, "replayuser")
	w := mfaPost(t, r, "/api/users/login/mfa", gin.H{
		"pending_token": pending2, "totp_code": code,
	}, nil)
	if w.Code == http.StatusOK {
		t.Fatal("同一个动态码被重复使用 —— 攻击者截获一次就能反复登录")
	}
}

// ===== 6. 恢复码一次性 =====
func TestMFARecoveryCodeIsSingleUse(t *testing.T) {
	r, load := setupMFATestWithUsers(t, "recuser")
	enableMFAForTest(t, "recuser")

	var u database.Users
	database.DB().Where("username = ?", "recuser").Get(&u)
	var hashes []string
	_ = json.Unmarshal([]byte(u.MFARecoveryHashes), &hashes)
	if len(hashes) == 0 {
		t.Fatal("前置条件：应有恢复码")
	}
	// 生成一个真实恢复码来用：直接替换哈希，拿到明文
	codes, _ := otp.NewRecoveryCodes(1)
	newHashes, _ := otp.HashRecoveryCodes(codes)
	database.DB().Where("username = ?", "recuser").Cols("mfa_recovery_hashes").
		Update(&database.Users{MFARecoveryHashes: mfaJSON(t, newHashes)})

	pending := loginPending(t, r, "recuser")
	if w := mfaPost(t, r, "/api/users/login/mfa", gin.H{
		"pending_token": pending, "recovery_code": codes[0],
	}, nil); w.Code != http.StatusOK {
		t.Fatalf("恢复码应能通过: %s", w.Body.String())
	}
	if n := len(load("recuser").MFARecoveryHashes); n == 0 {
		t.Fatal("用掉恢复码后应落库更新剩余哈希")
	}

	// 再用同一个恢复码必须失败
	pending2 := loginPending(t, r, "recuser")
	if w := mfaPost(t, r, "/api/users/login/mfa", gin.H{
		"pending_token": pending2, "recovery_code": codes[0],
	}, nil); w.Code == http.StatusOK {
		t.Fatal("同一个恢复码被重复使用 —— 一次性失效了")
	}
}

func mfaJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ===== 7. 关闭后回到单因子 =====
func TestMFADisableReturnsToSingleFactor(t *testing.T) {
	// 专属账号：重放守卫按用户名记录，与其他用例共用 bob 会让本用例的码被判"已使用"
	r, _ := setupMFATestWithUsers(t, "disableuser")
	secret := enableMFAForTest(t, "disableuser")

	access := mfaAccessTokenFor(t, "disableuser")
	w := mfaPost(t, r, "/api/users/mfa/disable", gin.H{
		"password": testMFAPassword, "totp_code": otp.Code(secret, time.Now()),
	}, map[string]string{"Authorization2": "Bearer " + access})
	if w.Code != http.StatusOK {
		t.Fatalf("关闭 MFA 应成功，得到 %d: %s", w.Code, w.Body.String())
	}

	// 之后登录应直接拿到凭据（不再要求第二步）
	w2 := mfaPost(t, r, "/api/users/login", gin.H{
		"username": "disableuser", "password": testMFAPassword,
	}, nil)
	data := mfaData(t, w2)
	if data["mfa_required"] == true {
		t.Fatal("关闭 MFA 后不应再要求第二步")
	}
	if _, ok := data["access_token"]; !ok {
		t.Fatal("关闭 MFA 后登录应直接签发凭据")
	}
}

// ===== 8. 管理员重置后目标用户可单因子登录 =====
func TestMFAResetByAdmin(t *testing.T) {
	r, _ := setupMFATest(t)
	enableMFAForTest(t, "bob")

	adminAccess := mfaAccessTokenFor(t, "alice")
	w := mfaPost(t, r, "/api/users/mfa/reset", gin.H{
		"username": "bob", "password": testMFAPassword,
	}, map[string]string{"Authorization2": "Bearer " + adminAccess})
	if w.Code != http.StatusOK {
		t.Fatalf("管理员重置应成功，得到 %d: %s", w.Code, w.Body.String())
	}

	w2 := mfaPost(t, r, "/api/users/login", gin.H{
		"username": "bob", "password": testMFAPassword,
	}, nil)
	if mfaData(t, w2)["mfa_required"] == true {
		t.Fatal("重置后应可单因子登录（这是丢手机的救援路径）")
	}
}

// 管理员重置必须校验"管理员自己的口令"：仅有会话不够，
// 被盗会话不该能悄悄关掉别人的第二因子
func TestMFAResetRequiresAdminPassword(t *testing.T) {
	r, _ := setupMFATest(t)
	enableMFAForTest(t, "bob")

	adminAccess := mfaAccessTokenFor(t, "alice")
	w := mfaPost(t, r, "/api/users/mfa/reset", gin.H{
		"username": "bob", "password": "wrong-password",
	}, map[string]string{"Authorization2": "Bearer " + adminAccess})
	if w.Code == http.StatusOK {
		t.Fatal("管理员口令错误时不应重置成功")
	}
}

// 非管理员不得重置他人 MFA
func TestMFAResetRequiresAdminRole(t *testing.T) {
	r, _ := setupMFATest(t)
	enableMFAForTest(t, "bob")

	// bob 是 operator
	opAccess := mfaAccessTokenFor(t, "bob")
	w := mfaPost(t, r, "/api/users/mfa/reset", gin.H{
		"username": "bob", "password": testMFAPassword,
	}, map[string]string{"Authorization2": "Bearer " + opAccess})
	if w.Code == http.StatusOK {
		t.Fatalf("非管理员不应能重置 MFA，实际 %d: %s", w.Code, w.Body.String())
	}
}

func mfaAccessTokenFor(t *testing.T, username string) string {
	t.Helper()
	tok, _, err := security.GlobalJWTManager.GenerateTokenPair("uid-"+username, username, "test-device")
	if err != nil {
		t.Fatalf("生成访问令牌失败: %v", err)
	}
	return tok
}

// ===== 附加：QR 端点只接受 otpauth:// =====
func TestMFAQROnlyAcceptsOtpauth(t *testing.T) {
	r, _ := setupMFATest(t)
	access := mfaAccessTokenFor(t, "bob")

	get := func(text string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/users/mfa/qr?text="+text, nil)
		req.Header.Set("Authorization2", "Bearer "+access)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := get("https%3A%2F%2Fevil.example%2Flogin"); code == http.StatusOK {
		t.Fatal("非 otpauth:// 文本不应被渲染（否则本端点成了任意二维码生成器，可用来做钓鱼）")
	}
	uri := otp.ProvisioningURI("Tshell", "bob", []byte("12345678901234567890"), otp.DefaultTOTPConfig())
	encoded := strings.ReplaceAll(strings.ReplaceAll(uri, ":", "%3A"), "/", "%2F")
	if code := get(encoded); code != http.StatusOK {
		t.Fatalf("合法 otpauth 链接应渲染成功，得到 %d", code)
	}
}

// ===== 附加：自助开启流程（start → confirm）=====
func TestMFAEnrollmentFlow(t *testing.T) {
	r, load := setupMFATest(t)
	access := mfaAccessTokenFor(t, "bob")

	w := mfaPost(t, r, "/api/users/mfa/start", gin.H{
		"password": testMFAPassword,
	}, map[string]string{"Authorization2": "Bearer " + access})
	if w.Code != http.StatusOK {
		t.Fatalf("start 应成功: %s", w.Body.String())
	}
	data := mfaData(t, w)
	secretStr, _ := data["secret"].(string)
	if secretStr == "" {
		t.Fatal("start 应返回密钥供手动录入")
	}
	if uri, _ := data["otpauth_uri"].(string); !strings.HasPrefix(uri, "otpauth://") {
		t.Fatalf("应返回 otpauth 链接，实际 %q", uri)
	}
	codesRaw, _ := data["recovery_codes"].([]interface{})
	if len(codesRaw) != otp.RecoveryCodeCount {
		t.Fatalf("应返回 %d 个恢复码，实际 %d", otp.RecoveryCodeCount, len(codesRaw))
	}

	// start 之后**不能**已经处于开启状态（半途而废不留残状态）
	if load("bob").MFAEnabled {
		t.Fatal("start 阶段不应写库开启 MFA")
	}

	// confirm
	secret, err := otp.DecodeSecret(secretStr)
	if err != nil {
		t.Fatal(err)
	}
	codes := make([]string, 0, len(codesRaw))
	for _, c := range codesRaw {
		codes = append(codes, fmt.Sprint(c))
	}
	w2 := mfaPost(t, r, "/api/users/mfa/confirm", gin.H{
		"password": testMFAPassword, "secret": secretStr,
		"totp_code": otp.Code(secret, time.Now()), "recovery_codes": codes,
	}, map[string]string{"Authorization2": "Bearer " + access})
	if w2.Code != http.StatusOK {
		t.Fatalf("confirm 应成功: %s", w2.Body.String())
	}
	u := load("bob")
	if !u.MFAEnabled {
		t.Fatal("confirm 后应已开启")
	}
	// 密钥必须是密文落库
	if !strings.HasPrefix(u.MFASecretEnc, "v1:") {
		t.Fatalf("密钥应密文落库，实际 %q", u.MFASecretEnc)
	}
	if strings.Contains(u.MFASecretEnc, secretStr) {
		t.Fatal("库里出现了明文密钥")
	}

	// confirm 用错码不应开启
	enableMFAForTest(t, "bob") // 已经开着，下面换个用户验证"错码不开启"
	accessAlice := mfaAccessTokenFor(t, "alice")
	w3 := mfaPost(t, r, "/api/users/mfa/start", gin.H{
		"password": testMFAPassword,
	}, map[string]string{"Authorization2": "Bearer " + accessAlice})
	s2, _ := mfaData(t, w3)["secret"].(string)
	w4 := mfaPost(t, r, "/api/users/mfa/confirm", gin.H{
		"password": testMFAPassword, "secret": s2, "totp_code": "000000",
	}, map[string]string{"Authorization2": "Bearer " + accessAlice})
	if w4.Code == http.StatusOK {
		t.Fatal("动态码错误时不应开启 MFA")
	}
}

// ===== 附加：状态查询不泄露密钥 =====
func TestMFAMeDoesNotLeakSecret(t *testing.T) {
	r, _ := setupMFATest(t)
	secret := enableMFAForTest(t, "bob")
	access := mfaAccessTokenFor(t, "bob")

	req := httptest.NewRequest(http.MethodGet, "/api/users/mfa", nil)
	req.Header.Set("Authorization2", "Bearer "+access)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态查询应成功: %s", rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, otp.EncodeSecret(secret)) || strings.Contains(body, "v1:") {
		t.Fatalf("状态接口泄露了密钥材料: %s", body)
	}
	data := mfaData(t, rec)
	if data["mfa_enabled"] != true {
		t.Fatalf("应报告已开启: %v", data)
	}
}
