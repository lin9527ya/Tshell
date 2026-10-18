package redirector

import (
	"strings"
)

// generateApache 生成 apache httpd 重定向器配置。
//
// 只产出 **VirtualHost 片段**，不产出"完整配置"：apache 的模块集与路径按发行版
// 差异极大（Debian 用 a2enmod + /etc/apache2/mods-enabled，RHEL 是 conf.modules.d，
// 还有编译进内核的模块不能再次 LoadModule）。生成一份"看起来完整"的 apache 配置
// 只会在别的发行版上装不起来。运维把它 include 进自己已经配好模块的 apache 即可，
// 所需模块在文件头列出。
func generateApache(p Params) string {
	getURI := normalizePath(p.Profile.HTTP.Get.URI)
	postURI := normalizePath(p.Profile.HTTP.Post.URI)
	wsPath := normalizePath(p.Profile.WS.Path)
	backend := p.Backend

	var b strings.Builder
	b.WriteString("# Tshell redirector（apache httpd VirtualHost 片段）\n")
	b.WriteString("# 由 `tshell -gen-redirector apache` 生成，请勿手改（手改会与画像漂移：\n")
	b.WriteString("# 改了画像里的 uri 却没改这里，beacon 直接失联）。\n")
	b.WriteString("#\n")
	b.WriteString("# 前置模块（按发行版启用）：\n")
	b.WriteString("#   Debian/Ubuntu: a2enmod ssl proxy proxy_http proxy_wstunnel headers rewrite\n")
	b.WriteString("#   RHEL/CentOS:   确保 mod_ssl / mod_proxy / mod_proxy_http / mod_proxy_wstunnel / mod_headers 已加载\n")
	b.WriteString("# 用法：放入 sites-available 后 a2ensite（或 include 进现有 vhost 配置）\n")
	b.WriteString("#\n")
	b.WriteString("# 另需在**全局配置**（不能写在 VirtualHost 内，apache 会报错）里确认：\n")
	b.WriteString("#   ServerTokens Prod      # 不暴露版本号\n")
	b.WriteString("#   ServerSignature Off    # 错误页不附版本\n\n")

	b.WriteString("# 80：全部跳 443\n")
	b.WriteString("<VirtualHost *:80>\n")
	b.WriteString("    ServerName " + serverName(p) + "\n")
	b.WriteString("    Redirect permanent / https://" + serverName(p) + "/\n")
	b.WriteString("</VirtualHost>\n\n")

	b.WriteString("<VirtualHost *:443>\n")
	b.WriteString("    ServerName " + serverName(p) + "\n\n")
	b.WriteString("    SSLEngine on\n")
	b.WriteString("    SSLCertificateFile    " + certPath(p) + "\n")
	b.WriteString("    SSLCertificateKeyFile " + keyPath(p) + "\n")
	b.WriteString("    SSLProtocol -all +TLSv1.2 +TLSv1.3\n")
	b.WriteString("    SSLHonorCipherOrder off\n\n")
	b.WriteString("    # 不泄露后端框架特征（版本信息见文件头的全局指令说明）\n")
	b.WriteString("    Header unset X-Powered-By\n")
	b.WriteString("    Header always unset X-Powered-By\n\n")
	b.WriteString("    # 后端只在本地/内网可达；不保留 Host，避免后端特征外泄\n")
	b.WriteString("    ProxyPreserveHost Off\n")
	b.WriteString("    ProxyRequests Off\n\n")

	if wsPath != "" {
		b.WriteString("    # websocket：apache 不会自动升级，必须显式走 mod_proxy_wstunnel\n")
		b.WriteString("    RewriteEngine On\n")
		b.WriteString("    RewriteCond %{HTTP:Upgrade} =websocket [NC]\n")
		b.WriteString("    RewriteRule ^" + wsPath + "$ ws://" + backend + wsPath + " [P,L]\n")
		b.WriteString("    ProxyPass " + wsPath + " ws://" + backend + wsPath + "\n")
		b.WriteString("    ProxyPassReverse " + wsPath + " ws://" + backend + wsPath + "\n\n")
	}
	if getURI != "" {
		b.WriteString("    # http-get\n")
		b.WriteString("    ProxyPass " + getURI + " http://" + backend + getURI + "\n")
		b.WriteString("    ProxyPassReverse " + getURI + " http://" + backend + getURI + "\n\n")
	}
	if postURI != "" {
		b.WriteString("    # http-post\n")
		b.WriteString("    ProxyPass " + postURI + " http://" + backend + postURI + "\n")
		b.WriteString("    ProxyPassReverse " + postURI + " http://" + backend + postURI + "\n\n")
	}

	b.WriteString("    # 兜底：未声明的路径 → 诱饵站点（返回 404 等于告诉对方这个域名上确实有东西）\n")
	b.WriteString("    RewriteEngine On\n")
	b.WriteString("    RewriteRule ^/(.*)$ " + decoy(p) + "/$1 [R=302,L]\n")
	b.WriteString("</VirtualHost>\n")
	return b.String()
}
