package api

import (
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/redirector"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetRedirectorTemplate GET /api/redirector/template?type=nginx&backend=...&domain=...&decoy=...&full=1
//
// 按当前画像生成重定向器配置。放在接口上是为了让运维不用登录服务器也能取到
// 与画像一致的配置（手写重定向器最容易与画像漂移，beacon 会直接失联）。
func GetRedirectorTemplate(c *gin.Context) {
	typ := strings.TrimSpace(c.DefaultQuery("type", redirector.TypeNginx))
	profileName := strings.TrimSpace(c.DefaultQuery("profile", "default"))

	profile := c2profile.Get(profileName)
	if profile == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "画像不存在: " + profileName})
		return
	}
	full := c.Query("full") == "1" || strings.EqualFold(c.Query("full"), "true")

	out, err := redirector.Generate(typ, redirector.Params{
		Profile:    profile,
		Backend:    strings.TrimSpace(c.Query("backend")),
		ServerName: strings.TrimSpace(c.Query("domain")),
		CertPath:   strings.TrimSpace(c.Query("cert")),
		KeyPath:    strings.TrimSpace(c.Query("key")),
		DecoyURL:   strings.TrimSpace(c.Query("decoy")),
		Full:       full,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	auditLogEvent("REDIRECTOR_TEMPLATE", "SUCCESS",
		"生成 "+typ+" 重定向器配置（画像 "+profileName+"，后端 "+c.Query("backend")+"）", nil)

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": gin.H{
		"type":     typ,
		"profile":  profileName,
		"uris":     redirector.URIs(profile),
		"content":  out,
		"lines":    strings.Count(out, "\n") + 1,
		"full":     full,
		"filename": redirectorFileName(typ, full),
	}})
}

func redirectorFileName(typ string, full bool) string {
	if typ == redirector.TypeNginx && full {
		return "nginx.conf"
	}
	if typ == redirector.TypeNginx {
		return "tshell-redirector.conf"
	}
	return "tshell-redirector-vhost.conf"
}
