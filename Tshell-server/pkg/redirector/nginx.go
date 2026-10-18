package redirector

import (
	"fmt"
	"strings"
)

// generateNginx 生成 nginx 重定向器配置。
//
// 关键点：
//   - 只对画像里的路径做 proxy_pass（用 `location =` 精确匹配，避免正则转义问题）；
//   - 其余请求全部 302 到诱饵站点（不给 404 —— 404 说明"这个域名上确实有东西"）；
//   - server_tokens off + 隐藏后端头：不泄露 nginx 版本与后端框架特征；
//   - WebSocket 路径单独一段，带上 Upgrade/Connection 头。
func generateNginx(p Params) string {
	profile := p.Profile
	wsPath := normalizePath(profile.WS.Path)
	getURI := normalizePath(profile.HTTP.Get.URI)
	postURI := normalizePath(profile.HTTP.Post.URI)

	var b strings.Builder
	if p.Full {
		b.WriteString("# Tshell redirector（nginx）—— 由 `tshell -gen-redirector nginx` 生成，请勿手改\n")
		b.WriteString("# 手改会与画像漂移：改了画像里的 uri 却没改这里，beacon 直接失联。\n\n")
		b.WriteString("worker_processes auto;\n")
		b.WriteString("error_log /var/log/nginx/tshell-redirector-error.log warn;\n")
		b.WriteString("pid /run/nginx-tshell.pid;\n\n")
		b.WriteString("events {\n    worker_connections 1024;\n}\n\n")
		b.WriteString("http {\n")
		b.WriteString("    include /etc/nginx/mime.types;\n")
		b.WriteString("    default_type application/octet-stream;\n")
		b.WriteString("    access_log /var/log/nginx/tshell-redirector-access.log;\n")
		b.WriteString("    sendfile on;\n")
		b.WriteString("    keepalive_timeout 65;\n")
		b.WriteString("    server_tokens off;\n")
		// 临时目录按前缀走：独立配置不该依赖编译默认的 /var/lib/nginx
		b.WriteString("    client_body_temp_path temp/client_body;\n")
		b.WriteString("    proxy_temp_path temp/proxy;\n")
		b.WriteString("    fastcgi_temp_path temp/fastcgi;\n")
		b.WriteString("    uwsgi_temp_path temp/uwsgi;\n")
		b.WriteString("    scgi_temp_path temp/scgi;\n\n")
		b.WriteString(indent(generateNginxServer(p, getURI, postURI, wsPath), "    "))
		b.WriteString("\n}\n")
		return b.String()
	}
	b.WriteString("# Tshell redirector（nginx server 片段）—— 由 `tshell -gen-redirector nginx` 生成，请勿手改\n")
	b.WriteString("# 用法：把本文件放进 /etc/nginx/conf.d/（或 include 进现有 http 块）\n\n")
	b.WriteString(generateNginxServer(p, getURI, postURI, wsPath))
	return b.String()
}

func generateNginxServer(p Params, getURI, postURI, wsPath string) string {
	backend := p.Backend
	var b strings.Builder

	b.WriteString("# 80：全部跳 443（不给任何内容）\n")
	b.WriteString("server {\n")
	b.WriteString("    listen 80;\n")
	b.WriteString(fmt.Sprintf("    server_name %s;\n", serverName(p)))
	b.WriteString("    return 301 https://$host$request_uri;\n")
	b.WriteString("}\n\n")

	b.WriteString("server {\n")
	// HTTP/2 写法取兼容式：`listen ... http2` 在 nginx 1.9.5~1.30 全都能解析
	// （≥1.25.1 只报 deprecated 警告），而独立的 `http2 on;` 需要 nginx ≥1.25.1 ——
	// Ubuntu 24.04(1.24)/22.04(1.18)、Debian 12(1.22)、RHEL 9(1.20) 上会直接
	// `unknown directive "http2"` 启动失败。模板是贴到别人服务器上的，取能跑的那一种。
	b.WriteString("    listen 443 ssl http2;   # ≥1.25.1 会提示 deprecated，可改为 `listen 443 ssl;` + `http2 on;`\n")
	b.WriteString(fmt.Sprintf("    server_name %s;\n\n", serverName(p)))
	b.WriteString("    # 证书：建议用真实域名签发（Let's Encrypt 或商业证书），自签证书是明显的重定向器特征\n")
	b.WriteString(fmt.Sprintf("    ssl_certificate     %s;\n", certPath(p)))
	b.WriteString(fmt.Sprintf("    ssl_certificate_key %s;\n", keyPath(p)))
	b.WriteString("    ssl_protocols TLSv1.2 TLSv1.3;\n")
	b.WriteString("    ssl_prefer_server_ciphers off;\n")
	b.WriteString("    ssl_session_cache shared:SSL:10m;\n")
	b.WriteString("    ssl_session_timeout 1d;\n\n")

	b.WriteString("    # 不泄露 nginx 版本；隐藏后端框架特征头\n")
	b.WriteString("    server_tokens off;\n")
	b.WriteString("    proxy_hide_header X-Powered-By;\n")
	b.WriteString("    proxy_hide_header Server;\n")
	b.WriteString("    proxy_hide_header X-AspNet-Version;\n\n")

	b.WriteString("    # 只放行画像声明的路径，其余一律送诱饵站点\n")
	b.WriteString("    location = / {\n")
	b.WriteString(fmt.Sprintf("        return 302 %s;\n", decoy(p)))
	b.WriteString("    }\n\n")

	if getURI != "" {
		b.WriteString(proxyLocation(getURI, backend, "http-get", false))
	}
	if postURI != "" {
		b.WriteString(proxyLocation(postURI, backend, "http-post", false))
	}
	if wsPath != "" {
		b.WriteString(proxyLocation(wsPath, backend, "websocket", true))
	}

	b.WriteString("    # 兜底：未声明的路径 → 诱饵站点（返回 404 等于告诉对方这个域名上确实有东西）\n")
	b.WriteString("    location / {\n")
	b.WriteString(fmt.Sprintf("        return 302 %s;\n", decoy(p)))
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

func proxyLocation(uri, backend, label string, websocket bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("    # %s\n", label))
	b.WriteString(fmt.Sprintf("    location = %s {\n", uri))
	b.WriteString(fmt.Sprintf("        proxy_pass http://%s;\n", backend))
	b.WriteString("        proxy_http_version 1.1;\n")
	if websocket {
		b.WriteString("        proxy_set_header Upgrade $http_upgrade;\n")
		b.WriteString("        proxy_set_header Connection \"upgrade\";\n")
		// WebSocket 会话可能长时间空闲，超时太短会被 nginx 掐断
		b.WriteString("        proxy_read_timeout 3600s;\n")
		b.WriteString("        proxy_send_timeout 3600s;\n")
	}
	b.WriteString("        proxy_set_header Host $host;\n")
	b.WriteString("        proxy_set_header X-Real-IP $remote_addr;\n")
	b.WriteString("        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n")
	b.WriteString("        proxy_set_header X-Forwarded-Proto $scheme;\n")
	b.WriteString("        proxy_buffering off;\n")
	b.WriteString("    }\n\n")
	return b.String()
}

func certPath(p Params) string {
	if p.CertPath != "" {
		return p.CertPath
	}
	return "/etc/nginx/certs/" + serverName(p) + ".crt"
}

func keyPath(p Params) string {
	if p.KeyPath != "" {
		return p.KeyPath
	}
	return "/etc/nginx/certs/" + serverName(p) + ".key"
}

// normalizePath 统一成以 / 开头的路径
func normalizePath(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if !strings.HasPrefix(u, "/") {
		return "/" + u
	}
	return u
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n") + "\n"
}
