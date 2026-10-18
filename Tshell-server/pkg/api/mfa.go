package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"BackendTemplate/pkg/security/otp"
	"BackendTemplate/pkg/utils/logx"

	"github.com/gin-gonic/gin"
)

// ===== MFA/TOTP 端点（阶段三 §3.5）=====
//
// 登录路径分两步：口令 → 中间态令牌 → 动态码 → 真凭据。
// 中间态与访问令牌用 aud 严格隔离（见 pkg/security/jwt_manager.go 的说明），
// 相关约束都有测试：pending 不能当访问令牌、访问令牌不能打第二步。
//
// 动态码重放的防线：mfaLastStep 记录每个用户最后成功使用的时间步，
// 同一个 30 秒窗口内的码只能用一次。
var (
	mfaStepMu    sync.Mutex
	mfaLastSteps = map[string]uint64{}
)

// MFALoginHandler 两步登录的第二步：pending_token + 动态码（或恢复码）→ 签发真凭据。
//
// 挂在 /api 组内，与 /users/login **共用**同一套限流与暴破防护：
// 动态码只有 6 位，没有限流的话在线暴破是可行的。
func MFALoginHandler(c *gin.Context) {
	var req struct {
		PendingToken string `json:"pending_token" binding:"required"`
		Code         string `json:"totp_code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := errors.ValidateRequest(c, &req); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	claims, err := security.GlobalJWTManager.ValidatePendingToken(req.PendingToken)
	if err != nil {
		// 不区分"过期/伪造/类型不对"的细节，避免给攻击者反馈
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeInvalidPassword, "登录中间态已失效，请重新输入口令"))
		return
	}

	user, found, err := database.Users_().GetByName(claims.Username)
	if err != nil {
		errors.AbortWithError(c, errors.NewDatabaseError(err))
		return
	}
	// 用户在两步之间被删号：文案与"口令不对"一致，避免用户名枚举
	if !found || !user.MFAEnabled {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeInvalidPassword, "登录中间态已失效，请重新输入口令"))
		return
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		code = strings.TrimSpace(req.RecoveryCode)
	}
	if err := verifyUserTOTPOrRecovery(&user, code, c); err != nil {
		// 第二步失败按登录失败计数：此时口令已经验过，暴破面只剩 6 位动态码，
		// 由限流 + 计数双重挡。
		security.GlobalRateLimiter.RecordLoginFailure(c.ClientIP())
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeInvalidPassword, err.Error()))
		return
	}

	// 沿用第一步的设备指纹，否则刷新链路算不出同一个指纹
	deviceID := claims.DeviceID
	if deviceID == "" {
		deviceID = generateDeviceID(c)
	}
	user.LastLogin = time.Now()
	user.LoginAttempts = 0
	if _, err := database.Users_().UpdateFields(user.Username, &user); err != nil {
		logx.Infof("警告：更新用户登录信息失败: %v\n", err)
	}
	issueSession(c, user.Username, deviceID, user.Role)
}

// verifyUserTOTPOrRecovery 校验动态码或恢复码，并在成功时更新防重放状态。
//
// 恢复码路径会**立即落库**把用掉的那个码移除 —— 只更新内存等于同一码可无限复用。
func verifyUserTOTPOrRecovery(user *database.Users, code string, c *gin.Context) error {
	if strings.TrimSpace(code) == "" {
		return fmt.Errorf("请输入动态码")
	}

	// 恢复码是长串（含连字符或 10 位以上），动态码是 6 位纯数字 —— 先按形状分流，
	// 避免拿恢复码去试 TOTP（每次 TOTP 校验都要解密密钥，没必要）
	if isRecoveryCodeShape(code) {
		hashes, err := loadRecoveryHashes(user)
		if err != nil {
			return err
		}
		rest, ok := otp.UseRecoveryCode(hashes, code)
		if !ok {
			return fmt.Errorf("动态码或恢复码错误")
		}
		encoded, err := json.Marshal(rest)
		if err != nil {
			return fmt.Errorf("保存恢复码状态失败")
		}
		if _, err := database.Users_().UpdateRecoveryHashes(user.Username, string(encoded)); err != nil {
			return fmt.Errorf("保存恢复码状态失败")
		}
		user.MFARecoveryHashes = string(encoded)
		logx.Infof("🔐 [MFA] 用户 %s 使用了恢复码，剩余 %d 个\n", user.Username, len(rest))
		return nil
	}

	secret, err := security.OpenMFASecret(user.MFASecretEnc)
	if err != nil {
		// 解不开**不能**当成"没开 MFA"放过去 —— 那等于静默关掉第二因子。
		// 让登录失败并留下日志，救援路径是管理员 reset。
		logx.Errorf("❌ [MFA] 用户 %s 的 TOTP 密钥无法解密（JWT 私钥是否变更过？）: %v\n", user.Username, err)
		return fmt.Errorf("动态码校验暂不可用，请联系管理员重置 MFA")
	}

	res := otp.Verify(secret, code, time.Now())
	if !res.OK {
		return fmt.Errorf("动态码错误")
	}

	// 防重放：同一时间步的码只能用一次
	mfaStepMu.Lock()
	last, seen := mfaLastSteps[user.Username]
	if seen && res.Timestep <= last {
		mfaStepMu.Unlock()
		return fmt.Errorf("该动态码已被使用，请等待下一个动态码")
	}
	mfaLastSteps[user.Username] = res.Timestep
	mfaStepMu.Unlock()
	return nil
}

// isRecoveryCodeShape 区分"恢复码"与"动态码"。
//
// 依据是**长度**，不是"含不含数字"：恢复码的字母表里本来就有数字 2-9
// （只去掉了易混的 0/O/1/I/L），所以"有数字就当动态码"会把恢复码误判进 TOTP 分支、
// 报"动态码错误" —— 用户手里拿着正确的恢复码却进不来（本实现第一版就是这个错）。
//
// 恢复码：11 字符（5 + '-' + 5）或去掉分隔符后的 10 字符；
// 动态码：6 位纯数字。两者长度不重叠，可以据此分流。
func isRecoveryCodeShape(code string) bool {
	trimmed := strings.TrimSpace(code)
	plain := strings.ReplaceAll(strings.ReplaceAll(trimmed, "-", ""), " ", "")
	return len(plain) == recoveryCodePlainLen && len(trimmed) <= recoveryCodePlainLen+1
}

// recoveryCodePlainLen 恢复码去掉分隔符后的长度（与 pkg/security/otp 的字母表一致）
const recoveryCodePlainLen = 10

// loadRecoveryHashes 解析库里的恢复码哈希列表
func loadRecoveryHashes(user *database.Users) ([]string, error) {
	if strings.TrimSpace(user.MFARecoveryHashes) == "" {
		return nil, nil
	}
	var hashes []string
	if err := json.Unmarshal([]byte(user.MFARecoveryHashes), &hashes); err != nil {
		return nil, fmt.Errorf("恢复码状态损坏，请联系管理员重置 MFA")
	}
	return hashes, nil
}

// resetMFAReplayGuard 清掉某个用户的防重放记录。
//
// 关闭/重置 MFA 时必须清：否则用户重新开启 MFA 后，新的认证器可能算出与旧记录
// 相同或更早的时间步（认证器时间被校过、或手机时钟回拨），导致**刚开始用就报
// "该动态码已被使用"**，而用户完全无从理解。
func resetMFAReplayGuard(username string) {
	mfaStepMu.Lock()
	delete(mfaLastSteps, username)
	mfaStepMu.Unlock()
}

// ===== 自助管理（均需登录）=====

// MFAStartHandler 开始绑定：校验口令 → 返回密钥 + otpauth 链接 + 恢复码。
//
// **只在响应里返回一次**，不落库：落库发生在 confirm。这样"点开又放弃"不会
// 留下一个半绑定的状态（用户以为开了、其实没开，或反过来登不进去）。
func MFAStartHandler(c *gin.Context) {
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := errors.ValidateRequest(c, &req); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	username := currentUsername(c)
	user, ok := loadUserByName(c, username)
	if !ok {
		return
	}
	// 二次确认口令：MFA 是账号安全的关键改动，不能只凭一个已登录的会话就改
	if user.PasswordHash == "" || !database.CheckPasswordHash(req.Password, user.PasswordHash) {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeInvalidPassword, "口令错误"))
		return
	}
	if !security.MFACipherReady() {
		errors.AbortWithError(c, errors.NewInternalError("服务端 MFA 加密器未就绪，暂时无法开启"))
		return
	}

	secret, err := otp.GenerateSecret()
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("生成密钥失败").WithInternalError(err))
		return
	}
	codes, err := otp.NewRecoveryCodes(otp.RecoveryCodeCount)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("生成恢复码失败").WithInternalError(err))
		return
	}

	issuer := "Tshell"
	uri := otp.ProvisioningURI(issuer, username, secret, otp.DefaultTOTPConfig())

	// 恢复码只在这里返回一次；confirm 时才写哈希落库。
	// 为了不在两步之间"忘了用户拿到的是哪批码"，把明文码随 confirm 一起提交回来，
	// 由服务端哈希落库 —— 服务端不保存明文。
	errors.RespondWithSuccess(c, gin.H{
		"secret":         otp.EncodeSecret(secret),
		"otpauth_uri":    uri,
		"issuer":         issuer,
		"account":        username,
		"digits":         otp.DefaultTOTPConfig().Digits,
		"period_seconds": int(otp.DefaultTOTPConfig().Period.Seconds()),
		"recovery_codes": codes,
		"hint":           "请立即抄写恢复码；它们只会显示这一次。输入认证器中的动态码完成绑定。",
	})
}

// MFAConfirmHandler 确认绑定：校验口令 + 动态码 + 恢复码列表 → 加密落库并启用。
func MFAConfirmHandler(c *gin.Context) {
	var req struct {
		Password      string   `json:"password" binding:"required"`
		Secret        string   `json:"secret" binding:"required"`
		TOTPCode      string   `json:"totp_code" binding:"required"`
		RecoveryCodes []string `json:"recovery_codes"`
	}
	if err := errors.ValidateRequest(c, &req); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	username := currentUsername(c)
	user, ok := loadUserByName(c, username)
	if !ok {
		return
	}
	if user.PasswordHash == "" || !database.CheckPasswordHash(req.Password, user.PasswordHash) {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeInvalidPassword, "口令错误"))
		return
	}

	secret, err := otp.DecodeSecret(req.Secret)
	if err != nil {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeValidationFailed, "密钥格式非法"))
		return
	}
	// 用用户刚扫进去的密钥校验一次：验不过说明扫描/录入有问题，此时**不启用**
	if res := otp.Verify(secret, strings.TrimSpace(req.TOTPCode), time.Now()); !res.OK {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeValidationFailed, "动态码不正确，请确认认证器时间与手机时间一致后重试"))
		return
	}

	sealed, err := security.SealMFASecret(secret)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("保存密钥失败").WithInternalError(err))
		return
	}

	recHashes, err := otp.HashRecoveryCodes(req.RecoveryCodes)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("生成恢复码失败").WithInternalError(err))
		return
	}
	encoded, err := json.Marshal(recHashes)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("保存恢复码失败").WithInternalError(err))
		return
	}

	if _, err := database.Users_().SetMFA(username, sealed, true, string(encoded)); err != nil {
		errors.AbortWithError(c, errors.NewDatabaseError(err))
		return
	}
	// 换过认证器 → 清掉旧的重放记录，否则新码可能一开始就被判"已使用"
	resetMFAReplayGuard(username)
	// 改了安全设置 → 吊销该用户既有令牌，强制重新登录（防止旧会话绕过新策略）
	_ = security.GlobalJWTManager.RevokeUserTokens(username, time.Now())

	logx.Infof("🔐 [MFA] 用户 %s 已开启 MFA（恢复码 %d 个）\n", username, len(recHashes))
	errors.RespondWithSuccess(c, gin.H{
		"mfa_enabled":    true,
		"recovery_count": len(recHashes),
		"message":        "MFA 已开启。请重新登录（既有会话已失效）。",
	})
}

// MFADisableHandler 关闭 MFA：需要口令 + 动态码（或一个恢复码）。
func MFADisableHandler(c *gin.Context) {
	var req struct {
		Password     string `json:"password" binding:"required"`
		TOTPCode     string `json:"totp_code"`
		RecoveryCode string `json:"recovery_code"`
	}
	if err := errors.ValidateRequest(c, &req); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	username := currentUsername(c)
	user, ok := loadUserByName(c, username)
	if !ok {
		return
	}
	if user.PasswordHash == "" || !database.CheckPasswordHash(req.Password, user.PasswordHash) {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeInvalidPassword, "口令错误"))
		return
	}
	if !user.MFAEnabled {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeValidationFailed, "当前账号未开启 MFA"))
		return
	}

	code := strings.TrimSpace(req.TOTPCode)
	if code == "" {
		code = strings.TrimSpace(req.RecoveryCode)
	}
	if err := verifyUserTOTPOrRecovery(&user, code, c); err != nil {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeValidationFailed, err.Error()))
		return
	}

	if err := clearUserMFA(username); err != nil {
		errors.AbortWithError(c, errors.NewDatabaseError(err))
		return
	}
	_ = security.GlobalJWTManager.RevokeUserTokens(username, time.Now())
	logx.Infof("🔓 [MFA] 用户 %s 已关闭 MFA\n", username)
	errors.RespondWithSuccess(c, gin.H{"mfa_enabled": false, "message": "MFA 已关闭"})
}

// MFAQRHandler 把 otpauth 链接渲染成 SVG 二维码。
//
// **只接受 otpauth:// 前缀**：否则这个端点就成了"任意文本的二维码生成器"，
// 可以被拿去生成指向钓鱼站点的二维码（服务端出图，可信度更高）。
func MFAQRHandler(c *gin.Context) {
	text := strings.TrimSpace(c.Query("text"))
	if text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 text 参数"})
		return
	}
	if !strings.HasPrefix(text, "otpauth://") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只允许 otpauth:// 链接"})
		return
	}
	if len(text) > 512 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "链接过长"})
		return
	}
	svg, err := otp.SVG(text, 4)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "image/svg+xml")
	c.Header("Cache-Control", "no-store")
	c.String(http.StatusOK, svg)
}

// MFAResetHandler 管理员重置某个用户的 MFA（用户丢手机的救援路径）。
//
// 必须带**当前管理员的口令**：仅有会话不够 —— 被盗会话不该能悄悄关掉别人的第二因子。
func MFAResetHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := errors.ValidateRequest(c, &req); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	adminName := currentUsername(c)
	// 角色从 JWT Claims 取（与 RequireRole 中间件同源）。
	// 这是纵深防御：路由上也挂了 security.RequireRole(RoleAdmin)。
	if claims, ok := c.Get("claims"); !ok || claims.(*security.Claims).Role != security.RoleAdmin {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodePermissionDenied, "仅管理员可重置他人的 MFA"))
		return
	}
	admin, ok := loadUserByName(c, adminName)
	if !ok {
		return
	}
	if admin.PasswordHash == "" || !database.CheckPasswordHash(req.Password, admin.PasswordHash) {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeInvalidPassword, "管理员口令错误"))
		return
	}
	if strings.TrimSpace(req.Username) == adminName {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeValidationFailed, "请用「关闭 MFA」操作自己的账号"))
		return
	}

	if _, ok := loadUserByName(c, strings.TrimSpace(req.Username)); !ok {
		return
	}
	if err := clearUserMFA(req.Username); err != nil {
		errors.AbortWithError(c, errors.NewDatabaseError(err))
		return
	}
	_ = security.GlobalJWTManager.RevokeUserTokens(req.Username, time.Now())
	logx.Infof("🔐 [MFA] 管理员 %s 重置了用户 %s 的 MFA\n", adminName, req.Username)
	errors.RespondWithSuccess(c, gin.H{"username": req.Username, "mfa_enabled": false, "message": "已重置该用户的 MFA"})
}

// clearUserMFA 清空 MFA 三个字段
func clearUserMFA(username string) error {
	_, err := database.Users_().ClearMFA(username)
	if err == nil {
		resetMFAReplayGuard(username)
	}
	return err
}

// currentUsername 从上下文取当前登录用户名（AuthMiddleware 已写入）
func currentUsername(c *gin.Context) string {
	if v, ok := c.Get("username"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// loadUserByName 按用户名取用户；失败时已写好响应，调用方直接 return
func loadUserByName(c *gin.Context, username string) (database.Users, bool) {
	var user database.Users
	if strings.TrimSpace(username) == "" {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeUnauthorized, "未登录"))
		return user, false
	}
	user, found, err := database.Users_().GetByName(username)
	if err != nil {
		errors.AbortWithError(c, errors.NewDatabaseError(err))
		return user, false
	}
	if !found {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeUserNotFound, "用户不存在"))
		return user, false
	}
	return user, true
}

// MFAMeHandler 返回当前账号的 MFA 状态（设置页用）。
// 只回状态，绝不回密钥或恢复码。
func MFAMeHandler(c *gin.Context) {
	user, ok := loadUserByName(c, currentUsername(c))
	if !ok {
		return
	}
	remaining := 0
	if hashes, err := loadRecoveryHashes(&user); err == nil {
		remaining = len(hashes)
	}
	errors.RespondWithSuccess(c, gin.H{
		"mfa_enabled":              user.MFAEnabled,
		"recovery_codes_remaining": remaining,
	})
}
