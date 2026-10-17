package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// isSecureRequest 判断当前请求是否经 HTTPS：直连 TLS（r.TLS != nil）或反向代理标记
// （X-Forwarded-Proto: https）。
//
// 登录 Cookie 的 Secure 属性按**请求**判定，而不是全局开关：HTTP 部署下强制 Secure
// 会让浏览器拒绝保存 Cookie（登录后立刻掉线），HTTPS 部署下不设置则明文请求会泄露会话。
// 原实现用一个从未被赋值的全局 secureCookie（SetSecureCookieEnabled 全仓零调用），
// 等于永远不设置 Secure；这里改为按请求判定，并去掉那个空开关。
func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

// DeviceFingerprint 设备指纹
type DeviceFingerprint struct {
	UserAgent string `json:"user_agent"`
	IP        string `json:"ip"`
	Timestamp int64  `json:"timestamp"`
}

// generateDeviceID 生成设备ID
func generateDeviceID(c *gin.Context) string {
	fingerprint := DeviceFingerprint{
		UserAgent: c.GetHeader("User-Agent"),
		IP:        c.ClientIP(),
		Timestamp: time.Now().Unix(),
	}

	data := fmt.Sprintf("%s|%s|%d", fingerprint.UserAgent, fingerprint.IP, fingerprint.Timestamp/3600)
	// B10 修复: 原为 hex.EncodeToString([]byte(data)) —— 只是原文十六进制编码而非摘要,
	// 泄露 UA/IP 原文且分布无雪崩;改 SHA-256 真摘要
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])[:16] // 取前16个字符作为设备ID
}

// 生成安全密钥
func generateSecureKey(length int) ([]byte, error) {
	key := make([]byte, length)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// 添加刷新令牌接口
func RefreshTokenHandler(c *gin.Context) {
	// refresh token 来源：优先 HttpOnly Cookie（页面刷新后保持登录），其次请求体（兼容旧前端）
	refreshToken := ""
	if cookieRT, err := c.Cookie("refresh_token"); err == nil && cookieRT != "" {
		refreshToken = cookieRT
	}
	if refreshToken == "" {
		var refreshData struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.ShouldBindJSON(&refreshData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
			return
		}
		refreshToken = refreshData.RefreshToken
	}

	accessToken, newRefreshToken, err := security.GlobalJWTManager.RefreshAccessToken(refreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// D43: 响应携带新 refresh token（旧 token 已吊销），并续期 HttpOnly Cookie
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "access_token", Value: accessToken, Path: "/", MaxAge: 15 * 60,
		Secure: isSecureRequest(c), HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "refresh_token", Value: newRefreshToken, Path: "/", MaxAge: 7 * 24 * 3600,
		Secure: isSecureRequest(c), HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": gin.H{
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
	}})
}

// AuthCheckHandler 轻量 Token 校验端点（protected 组，所有登录用户可访问）：
// 仅返回 username/role，不包含任何敏感配置——前端路由守卫用它校验 token 有效性。
// 原守卫调用 /api/config/current（adminOnly），operator/auditor 得到 403 → 前端误登出。
func AuthCheckHandler(c *gin.Context) {
	username, _ := c.Get("username")
	role := "admin"
	if claims, ok := c.Get("claims"); ok {
		if cl, ok := claims.(*security.Claims); ok && cl != nil {
			role = cl.Role
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"username": username,
		"role":     defaultRole(role),
	})
}

// 登录处理函数
func LoginHandler(c *gin.Context) {
	var loginData struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	// 验证请求参数
	if err := errors.ValidateRequest(c, &loginData); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	// 验证用户名格式
	if err := errors.ValidateUsername(loginData.Username); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	// 清理输入数据
	loginData.Username = errors.SanitizeInput(loginData.Username)

	// 查询用户并进行安全验证
	var users database.Users
	found, err := database.Engine.Where("username = ?", loginData.Username).Get(&users)
	if err != nil {
		errors.AbortWithError(c, errors.NewDatabaseError(err))
		return
	}

	if !found || users.Username == "" {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeUserNotFound, "用户不存在或密码错误"))
		return
	}

	// 检查账户是否被锁定
	if users.Locked {
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeUserLocked, "账户因多次登录失败被锁定"))
		return
	}

	// 使用安全的密码验证
	var isValidPassword bool
	if users.PasswordHash != "" {
		// 新版本哈希密码验证
		isValidPassword = database.CheckPasswordHash(loginData.Password, users.PasswordHash)
	} else {
		// 强制要求迁移到安全存储
		logx.Infof("警告：用户 %s 仍在使用不安全的密码存储\n", loginData.Username)
		isValidPassword = false
	}

	if !isValidPassword {
		// 记录失败的登录尝试
		users.LoginAttempts++
		if users.LoginAttempts >= 5 {
			users.Locked = true
			logx.Infof("警告：用户 %s 因多次登录失败被锁定\n", loginData.Username)
		}
		database.Engine.Where("username = ?", loginData.Username).Update(&users)
		// D17: 发布登录失败事件（供事件流/通知告警）
		if command.GlobalEventBus != nil {
			command.PublishEvent(command.EventLoginFailed, loginData.Username, map[string]interface{}{
				"username": loginData.Username,
				"attempts": users.LoginAttempts,
			})
		}
		errors.AbortWithError(c, errors.NewError(errors.ErrCodeInvalidPassword, "用户名或密码错误"))
		return
	}

	// 认证成功，更新登录信息
	users.LastLogin = time.Now()
	users.LoginAttempts = 0 // 重置失败计数
	if _, err := database.Engine.Where("username = ?", loginData.Username).Update(&users); err != nil {
		// 更新失败不影响登录，只记录警告
		logx.Infof("警告：更新用户登录信息失败: %v\n", err)
	}

	// 生成令牌对（包含设备指纹）
	deviceID := generateDeviceID(c)
	accessToken, refreshToken, err := security.GlobalJWTManager.GenerateTokenPair(loginData.Username, loginData.Username, deviceID)
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("生成令牌失败").WithInternalError(err))
		return
	}

	// 返回成功响应（C13 RBAC：附带角色）
	responseData := gin.H{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    15 * 60, // 15分钟
		"token_type":    "Bearer",
		"permissions":   1, // 兼容旧前端
		"username":      loginData.Username,
		"role":          defaultRole(users.Role), // C13: admin | operator | auditor
	}

	// 设置 JWT Cookie（http.SetCookie：Add 语义保留多个 Cookie，且原生支持 SameSite，
	// 避免手动 Get/Set 折叠丢失 refresh_token）。均 HttpOnly + SameSite=Lax（防 XSS 窃取 / CSRF 跨站携带）
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "access_token", Value: accessToken, Path: "/", MaxAge: 15 * 60,
		Secure: isSecureRequest(c), HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	// refresh token 持久化 HttpOnly Cookie（有效期与 jwt.refresh_token_ttl 一致，默认 7 天）：
	// 页面刷新后仍可凭 Cookie 续期，避免"过段时间自动退出登录"
	http.SetCookie(c.Writer, &http.Cookie{
		Name: "refresh_token", Value: refreshToken, Path: "/", MaxAge: 7 * 24 * 3600,
		Secure: isSecureRequest(c), HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})

	errors.RespondWithSuccess(c, responseData)
}

// JWT 验证中间件 - 增强错误处理
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// OPSEC: 双通道认证——优先 Authorization2 头（内存 token），
		// 其次 HttpOnly Cookie（JS 不可读，页面刷新后仍保持登录，防 XSS 窃取 token）
		authHeader := c.GetHeader("Authorization2")
		tokenString := ""
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = authHeader[len("Bearer "):]
		}
		if tokenString == "" {
			if cookieToken, err := c.Cookie("access_token"); err == nil && cookieToken != "" {
				tokenString = cookieToken
			}
		}
		if tokenString == "" {
			logx.Errorf("❌ JWT认证失败: 缺少认证凭证（Authorization2头或Cookie）\n")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token required"})
			c.Abort()
			return
		}

		// 验证token
		claims, err := security.GlobalJWTManager.ValidateToken(tokenString)
		if err != nil {
			logx.Errorf("❌ JWT认证失败: Token验证错误: %v\n", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token: " + err.Error()})
			c.Abort()
			return
		}

		// 认证成功，设置用户信息
		c.Set("username", claims.Username)
		c.Set("user_id", claims.UserID)
		c.Set("claims", claims) // C13 RBAC: 供 RequireRole 校验
		logx.Infof("✅ JWT认证成功: 用户=%s(角色:%s), 方法=%s %s\n", claims.Username, claims.Role, c.Request.Method, c.Request.URL.Path)
		c.Next()
	}
}

// 注销处理函数
func LogoutHandler(c *gin.Context) {
	// 获取token并将其加入黑名单
	authorizationHeader := c.GetHeader("Authorization2")
	if authToken := strings.TrimPrefix(authorizationHeader, "Bearer "); authToken != "" {
		security.GlobalJWTManager.RevokeToken(authToken)
	}

	// D43: 登出时吊销关联的 refresh token
	var logoutData struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.ShouldBindJSON(&logoutData); err == nil && logoutData.RefreshToken != "" {
		security.GlobalJWTManager.RevokeRefreshToken(logoutData.RefreshToken)
	}

	// 清除Cookie中的token
	c.SetCookie(
		"access_token", // cookie名称
		"",             // 空值
		-1,             // 过期时间（立即过期）
		"/",            // 路径
		"",             // 域
		false,          // secure
		true,           // httpOnly
	)

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Logged out successfully"})
}

// 修改密码处理函数
func ChangePasswordHandler(c *gin.Context) {
	var passwordData struct {
		OldPassword string `form:"old_password"`
		NewPassword string `form:"new_password"`
	}
	if err := c.ShouldBind(&passwordData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	// J7: 新密码强制密码策略（原仅判断新旧不同 → 空/单字符弱密码可继续使用）
	if err := errors.ValidatePassword(passwordData.NewPassword); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "密码不符合策略: " + err.Error()})
		return
	}

	// 处理密码修改逻辑
	if passwordData.OldPassword != passwordData.NewPassword {
		username := c.MustGet("username").(string)
		var users database.Users
		database.Engine.Where("username = ?", username).Get(&users)

		// 使用安全的密码验证
		var isValidOldPassword bool
		if users.PasswordHash != "" {
			// 新版本哈希密码验证
			isValidOldPassword = database.CheckPasswordHash(passwordData.OldPassword, users.PasswordHash)
		} else {
			// 兼容旧版本明文密码
			logx.Infof("警告：用户 %s 仍在使用不安全的明文密码存储\n", username)
			isValidOldPassword = false // 强制要求使用安全存储
		}

		if isValidOldPassword {
			// 生成新密码的哈希
			newPasswordHash, err := database.HashPassword(passwordData.NewPassword)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Password hashing failed"})
				return
			}

			// 更新密码哈希
			users.PasswordHash = newPasswordHash
			// 清除旧的明文密码字段（如果存在）
			// users.Password = "" // 如果数据库中还有这个字段

			database.Engine.Where("username = ?", username).Update(&users)
			// D46: 改密成功后吊销该用户所有旧令牌与刷新令牌（旧会话立即失效）
			security.GlobalJWTManager.RevokeUserTokens(username, time.Now())
			c.JSON(http.StatusOK, gin.H{"code": 200, "message": "Password changed successfully"})
		} else {
			c.JSON(http.StatusOK, gin.H{"code": 400, "message": "Old password verification failed"})
		}
	} else {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "New password must be different from old password"})
	}

}
// ================= 用户管理（仅 admin，RequireRole(RoleAdmin) 强制） =================

// UserListItem 用户列表项（不含任何密码/盐字段）
type UserListItem struct {
	Username   string `json:"username"`
	Role       string `json:"role"`
	Locked     bool   `json:"locked"`
	CreatedAt  string `json:"created_at"`
	LastLogin  string `json:"last_login"`
}

// GetUserListHandler 获取用户列表（分页 + 模糊搜索）
func GetUserListHandler(c *gin.Context) {
	var query struct {
		Page     int    `form:"page"`
		PageSize int    `form:"page_size"`
		Search   string `form:"search"`
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid query parameters"})
		return
	}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 10
	}
	// 【P2-1】page_size 上限 100：防止拉全表
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	session := database.Engine.NewSession()
	defer session.Close()

	if query.Search != "" {
		session = session.Where("username LIKE ? COLLATE NOCASE", "%"+query.Search+"%")
	}

	total, err := session.Count(new(database.Users))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count users"})
		return
	}

	users := make([]database.Users, 0)
	err = session.Limit(query.PageSize, (query.Page-1)*query.PageSize).Asc("created_at").Find(&users)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}

	list := make([]UserListItem, 0, len(users))
	for _, u := range users {
		item := UserListItem{
			Username: u.Username,
			Role:     defaultRole(u.Role),
			Locked:   u.Locked,
		}
		if !u.CreatedAt.IsZero() {
			item.CreatedAt = u.CreatedAt.Format("2006-01-02 15:04:05")
		}
		if !u.LastLogin.IsZero() {
			item.LastLogin = u.LastLogin.Format("2006-01-02 15:04:05")
		}
		list = append(list, item)
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": gin.H{"list": list, "total": total}})
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"` // admin | operator | auditor
}

// CreateUserHandler 创建用户（管理员）
func CreateUserHandler(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
		return
	}
	if err := errors.ValidatePassword(req.Password); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "密码不符合策略: " + err.Error()})
		return
	}
	role := defaultRole(req.Role)
	if role != security.RoleAdmin && role != security.RoleOperator && role != security.RoleAuditor {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的角色: " + role})
		return
	}

	exists, err := database.Engine.Where("username = ?", req.Username).Exist(new(database.Users))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户失败"})
		return
	}
	if exists {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "用户已存在: " + req.Username})
		return
	}

	user, err := database.CreateSecureUser(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建用户失败: " + err.Error()})
		return
	}
	user.Role = role
	if _, err := database.Engine.Insert(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "插入用户失败: " + err.Error()})
		return
	}

	logx.Infof("👤 [USER-MGMT] 管理员创建用户: %s (role=%s)\n", req.Username, role)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "用户创建成功"})
}

// DeleteUserRequest 删除用户请求
type DeleteUserRequest struct {
	Username string `json:"username"`
}

// DeleteUserHandler 删除用户（管理员；禁止删除自己与最后一个 admin）
func DeleteUserHandler(c *gin.Context) {
	var req DeleteUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
		return
	}

	// 禁止删除自己
	currentUsername := c.MustGet("username").(string)
	if req.Username == currentUsername {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "不能删除当前登录用户"})
		return
	}

	var target database.Users
	has, err := database.Engine.Where("username = ?", req.Username).Get(&target)
	if err != nil || !has {
		c.JSON(http.StatusOK, gin.H{"code": 404, "message": "用户不存在"})
		return
	}

	// 禁止删除最后一个 admin（保留至少一个管理员可登录）
	if defaultRole(target.Role) == security.RoleAdmin {
		adminCount, _ := database.Engine.Where("role = ?", security.RoleAdmin).Count(new(database.Users))
		if adminCount <= 1 {
			c.JSON(http.StatusOK, gin.H{"code": 400, "message": "不能删除最后一个管理员"})
			return
		}
	}

	if _, err := database.Engine.Where("username = ?", req.Username).Delete(new(database.Users)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户失败: " + err.Error()})
		return
	}

	// 撤销该用户所有令牌
	if security.GlobalJWTManager != nil {
		security.GlobalJWTManager.RevokeUserTokens(req.Username, time.Now())
	}

	logx.Infof("👤 [USER-MGMT] 管理员删除用户: %s\n", req.Username)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "用户删除成功"})
}

// AdminResetPasswordRequest 管理员重置密码请求
type AdminResetPasswordRequest struct {
	Username    string `json:"username"`
	NewPassword string `json:"new_password"`
}

// AdminResetPasswordHandler 管理员重置指定用户密码（同时吊销其所有令牌）
func AdminResetPasswordHandler(c *gin.Context) {
	var req AdminResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不能为空"})
		return
	}
	if err := errors.ValidatePassword(req.NewPassword); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 400, "message": "密码不符合策略: " + err.Error()})
		return
	}

	var user database.Users
	has, err := database.Engine.Where("username = ?", req.Username).Get(&user)
	if err != nil || !has {
		c.JSON(http.StatusOK, gin.H{"code": 404, "message": "用户不存在"})
		return
	}

	newHash, err := database.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码哈希失败"})
		return
	}
	user.PasswordHash = newHash
	if _, err := database.Engine.Where("username = ?", req.Username).Update(&user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新密码失败"})
		return
	}

	// 吊销该用户所有旧令牌（旧会话立即失效）
	if security.GlobalJWTManager != nil {
		security.GlobalJWTManager.RevokeUserTokens(req.Username, time.Now())
	}

	logx.Infof("👤 [USER-MGMT] 管理员重置用户 %s 的密码\n", req.Username)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "message": "密码重置成功"})
}

// defaultRole 角色空值时默认 admin（兼容旧用户）
func defaultRole(role string) string {
	if role == "" {
		return "admin"
	}
	return role
}