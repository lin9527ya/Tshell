package security

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequireRoleDecision 覆盖 RequireRole 的判定契约（C13 RBAC 首个测试）。
// 插件仓库的写接口（/api/plugin/upload、/api/plugin/delete）与下载接口依赖
// RequireRole(RoleAdmin, RoleOperator) 把只读角色 auditor 挡在外面，这里固定该契约。
func TestRequireRoleDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name       string
		role       string
		setClaims  bool
		wrongType  bool
		wantStatus int
	}{
		{name: "auditor 被拒", role: RoleAuditor, setClaims: true, wantStatus: http.StatusForbidden},
		{name: "operator 放行", role: RoleOperator, setClaims: true, wantStatus: http.StatusOK},
		{name: "admin 放行", role: RoleAdmin, setClaims: true, wantStatus: http.StatusOK},
		{name: "空角色被拒（H2 提权防护）", role: "", setClaims: true, wantStatus: http.StatusForbidden},
		{name: "无 claims 未认证", setClaims: false, wantStatus: http.StatusUnauthorized},
		{name: "claims 类型错误", setClaims: true, wrongType: true, wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			g := r.Group("")
			g.Use(func(c *gin.Context) {
				if !tc.setClaims {
					c.Next()
					return
				}
				if tc.wrongType {
					c.Set("claims", "not-a-claims-pointer")
				} else {
					c.Set("claims", &Claims{Username: "u1", Role: tc.role})
				}
				c.Next()
			})
			g.Use(RequireRole(RoleAdmin, RoleOperator))
			g.POST("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/probe", nil)
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("role=%q setClaims=%v → 状态码 %d，期望 %d", tc.role, tc.setClaims, w.Code, tc.wantStatus)
			}
		})
	}
}
