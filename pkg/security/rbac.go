package security

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// ===== C13: RBAC 角色校验 =====

// RoleAdmin 管理员（全部权限）
const RoleAdmin = "admin"

// RoleOperator 操作员（客户端操作，不可改系统配置）
const RoleOperator = "operator"

// RoleAuditor 审计员（只读 + 审计查看）
const RoleAuditor = "auditor"

// RequireRole 返回要求指定角色的中间件（从 JWT Claims 读取 role）。
// 必须挂在 AuthMiddleware（JWT 认证）之后。
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool)
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		claims, exists := c.Get("claims")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			c.Abort()
			return
		}
		cl, ok := claims.(*Claims)
		if !ok || cl == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "认证信息无效"})
			c.Abort()
			return
		}
		if cl.Role == "" {
			// H2: 空 role 一律拒绝（原默认视为管理员 → 普通用户经 refresh 可提权为 admin）
			logx.Infof("🚫 [RBAC] 用户 %s 令牌缺失角色，拒绝访问 %s\n", cl.Username, c.Request.URL.Path)
			c.JSON(http.StatusForbidden, gin.H{"error": "令牌缺失角色，请重新登录"})
			c.Abort()
			return
		}
		if !allowed[cl.Role] {
			logx.Infof("🚫 [RBAC] 用户 %s 角色 %s 无权访问 %s\n", cl.Username, cl.Role, c.Request.URL.Path)
			c.JSON(http.StatusForbidden, gin.H{"error": "权限不足：需要角色 " + joinRoles(roles)})
			c.Abort()
			return
		}
		c.Next()
	}
}

// joinRoles 拼接角色列表（提示用）
func joinRoles(roles []string) string {
	s := ""
	for i, r := range roles {
		if i > 0 {
			s += "/"
		}
		s += r
	}
	return s
}
