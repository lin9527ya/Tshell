package redirector

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"BackendTemplate/pkg/c2profile"
)

// 重定向器配置由画像生成，最容易出的问题是"与画像漂移"：改了 uri 忘了改配置，
// beacon 直接失联，而现场只表现为"上线失败"。因此这里守两件事：
//   1) 画像里的每个路径都必须出现在生成的配置里；
//   2) 生成的配置必须真的能被 nginx / apache 解析（本机有就验，没有则跳过）。

func testProfile() *c2profile.Profile {
	p := &c2profile.Profile{Name: "default", UserAgent: "Mozilla/5.0 Chrome/120"}
	p.HTTP.Get.URI = "/tencent/mcp/pc/pcsearch"
	p.HTTP.Post.URI = "/tencent/sensearch/collection/item/check"
	p.WS.Path = "/ws"
	return p
}

func testParams() Params {
	return Params{
		Profile:    testProfile(),
		Backend:    "10.10.0.5:8089",
		ServerName: "cdn.example.com",
		DecoyURL:   "https://www.example.com",
		CertPath:   "/etc/nginx/certs/cdn.example.com.crt",
		KeyPath:    "/etc/nginx/certs/cdn.example.com.key",
	}
}

func TestURIsFromProfile(t *testing.T) {
	got := URIs(testProfile())
	want := []string{"/tencent/mcp/pc/pcsearch", "/tencent/sensearch/collection/item/check", "/ws"}
	if len(got) != len(want) {
		t.Fatalf("路径数 = %d，期望 %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 个路径 = %q，期望 %q", i, got[i], want[i])
		}
	}
	// 缺前导斜杠时补齐；空路径不产出
	p := &c2profile.Profile{}
	p.HTTP.Get.URI = "api/v1"
	if got := URIs(p); len(got) != 1 || got[0] != "/api/v1" {
		t.Fatalf("路径规范化不符: %v", got)
	}
}

func TestGeneratedConfigsContainProfilePaths(t *testing.T) {
	for _, typ := range Types {
		t.Run(typ, func(t *testing.T) {
			p := testParams()
			p.Full = typ == TypeNginx // apache 只产出 vhost 片段
			out, err := Generate(typ, p)
			if err != nil {
				t.Fatalf("生成失败: %v", err)
			}
			for _, uri := range URIs(p.Profile) {
				if !strings.Contains(out, uri) {
					t.Errorf("%s 配置缺少画像路径 %s（beacon 会失联）", typ, uri)
				}
			}
			for _, want := range []string{p.Backend, p.ServerName, p.DecoyURL} {
				if !strings.Contains(out, want) {
					t.Errorf("%s 配置缺少 %s", typ, want)
				}
			}
			// 不泄露版本与后端特征
			switch typ {
			case TypeNginx:
				if !strings.Contains(out, "server_tokens off;") {
					t.Error("nginx 配置应关闭 server_tokens")
				}
				if !strings.Contains(out, "proxy_hide_header X-Powered-By;") {
					t.Error("nginx 配置应隐藏后端特征头")
				}
			case TypeApache:
				if !strings.Contains(out, "ServerTokens Prod") {
					t.Error("apache 配置应设置 ServerTokens Prod")
				}
				if !strings.Contains(out, "Header unset X-Powered-By") {
					t.Error("apache 配置应隐藏后端特征头")
				}
			}
			// 兜底必须是跳诱饵而不是 404
			if !strings.Contains(out, "302") {
				t.Errorf("%s 配置缺少跳诱饵的兜底", typ)
			}
			if !strings.Contains(out, "生成，请勿手改") {
				t.Errorf("%s 配置缺少生成来源提示", typ)
			}
		})
	}
}

func TestGenerateRejectsBadInput(t *testing.T) {
	base := testParams()
	cases := []struct {
		name   string
		mutate func(p *Params)
		typ    string
	}{
		{"缺少画像", func(p *Params) { p.Profile = nil }, TypeNginx},
		{"缺少后端", func(p *Params) { p.Backend = "" }, TypeNginx},
		{"后端带协议前缀", func(p *Params) { p.Backend = "http://10.0.0.1:80" }, TypeNginx},
		{"配置注入-后端", func(p *Params) { p.Backend = "127.0.0.1; } server { listen 80" }, TypeNginx},
		{"配置注入-域名", func(p *Params) { p.ServerName = "a.com;\nreturn 200" }, TypeNginx},
		{"诱饵非 URL", func(p *Params) { p.DecoyURL = "javascript:alert(1)" }, TypeNginx},
		{"完整配置缺证书", func(p *Params) { p.Full = true; p.CertPath = ""; p.KeyPath = "" }, TypeNginx},
		{"证书相对路径", func(p *Params) { p.Full = true; p.CertPath = "certs/x.crt" }, TypeNginx},
		{"apache 不支持完整配置", func(p *Params) { p.Full = true }, TypeApache},
		{"画像无路径", func(p *Params) { p.Profile = &c2profile.Profile{} }, TypeNginx},
		{"未知类型", func(p *Params) {}, "caddy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			if out, err := Generate(tc.typ, p); err == nil {
				t.Fatalf("应报错，却生成了 %d 字节配置", len(out))
			}
		})
	}
}

// nginxListenPortRe 匹配 `listen <port>`（含 `listen 443 ssl http2;` 这类带参数的写法）
var nginxListenPortRe = regexp.MustCompile(`listen\s+(\d+)`)

// TestNginxAcceptsGeneratedConfig 用真实 nginx 校验语法
func TestNginxAcceptsGeneratedConfig(t *testing.T) {
	bin, err := exec.LookPath("nginx")
	if err != nil {
		t.Skip("本机没有 nginx，跳过语法校验")
	}
	dir := t.TempDir()
	for _, sub := range []string{"logs", "certs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	p := testParams()
	p.Full = true
	p.CertPath = filepath.Join(dir, "certs", "cdn.example.com.crt")
	p.KeyPath = filepath.Join(dir, "certs", "cdn.example.com.key")
	out, err := Generate(TypeNginx, p)
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	// 证书文件：nginx -t 会读取证书内容
	genCert(t, p.CertPath, p.KeyPath)

	// 日志与 pid 指到临时目录（生成的配置里是部署路径，测试环境不可写）
	out = strings.ReplaceAll(out, "/var/log/nginx/tshell-redirector-error.log", filepath.Join(dir, "logs", "error.log"))
	out = strings.ReplaceAll(out, "/var/log/nginx/tshell-redirector-access.log", filepath.Join(dir, "logs", "access.log"))
	out = strings.ReplaceAll(out, "/run/nginx-tshell.pid", filepath.Join(dir, "logs", "nginx.pid"))
	// 端口挪到非特权区间：部分 nginx 版本（CI 的 1.24）在 `-t` 时会真的 bind 监听端口，
	// 非 root 下 listen 80/443 直接 EACCES —— "syntax is ok" 之后紧跟
	// "bind() to 0.0.0.0:80 failed (13: Permission denied)"，语法检查变成了权限检查。
	// 这里只校验"配置能否被 nginx 解析"，端口号与语法无关。
	// 注意必须落到 ≥1024：早期写法用"前缀 1"把 80 变成 180，仍属特权端口，
	// CI 上照样 EACCES —— 所以统一 +10000（80→10080、443→10443）。
	out = nginxListenPortRe.ReplaceAllStringFunc(out, func(m string) string {
		sub := nginxListenPortRe.FindStringSubmatch(m)
		port, err := strconv.Atoi(sub[1])
		if err != nil {
			return m
		}
		return "listen " + strconv.Itoa(port+10000)
	})
	cfg := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(cfg, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "temp"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-t", "-c", cfg, "-p", dir)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nginx -t 拒绝生成的配置: %v\n%s", err, outBytes)
	}
}

// TestApacheAcceptsGeneratedConfig 用真实 apache 校验语法。
// 生成的是 vhost 片段（模块集按发行版而异），因此这里搭一个脚手架配置：
// 按系统实际情况加载所需模块，再把片段 include 进来。
func TestApacheAcceptsGeneratedConfig(t *testing.T) {
	bin, err := exec.LookPath("apache2")
	if err != nil {
		if bin, err = exec.LookPath("httpd"); err != nil {
			t.Skip("本机没有 apache2/httpd，跳过语法校验")
		}
	}
	builtin := apacheBuiltinModules(t, bin)

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "certs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "run"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := testParams()
	p.CertPath = filepath.Join(dir, "certs", "cdn.example.com.crt")
	p.KeyPath = filepath.Join(dir, "certs", "cdn.example.com.key")
	snippet, err := Generate(TypeApache, p)
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	genCert(t, p.CertPath, p.KeyPath)

	// 所需模块：已编译进内核的跳过，其余按 .so 路径加载；缺模块则跳过本用例
	// alias 提供 Redirect（80→443 那段）
	required := []string{"mpm_event", "authz_core", "unixd", "alias", "headers", "proxy", "proxy_http", "proxy_wstunnel", "rewrite", "ssl"}
	var scaffold strings.Builder
	scaffold.WriteString("ServerRoot \"/etc/apache2\"\n")
	scaffold.WriteString("DefaultRuntimeDir " + filepath.Join(dir, "run") + "\n")
	scaffold.WriteString("Mutex file:" + filepath.Join(dir, "run") + " default\n")
	scaffold.WriteString("PidFile " + filepath.Join(dir, "apache.pid") + "\n")
	scaffold.WriteString("ErrorLog " + filepath.Join(dir, "error.log") + "\n")
	scaffold.WriteString("Listen 8443\n")
	scaffold.WriteString("User " + currentUser(t) + "\nGroup " + currentGroup(t) + "\n")
	scaffold.WriteString("<Directory />\n    AllowOverride None\n    Require all denied\n</Directory>\n")
	for _, mod := range required {
		if builtin[mod] {
			continue
		}
		so := "/usr/lib/apache2/modules/mod_" + mod + ".so"
		if _, err := os.Stat(so); err != nil {
			t.Skipf("缺少模块 %s（%s），跳过 apache 语法校验", mod, so)
		}
		scaffold.WriteString("LoadModule " + mod + "_module " + so + "\n")
	}
	cfg := filepath.Join(dir, "apache.conf")
	if err := os.WriteFile(cfg, []byte(scaffold.String()+"\n"+snippet), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "-f", cfg, "-t")
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("apache configtest 拒绝生成的配置: %v\n%s", err, outBytes)
	}
}

// apacheBuiltinModules 解析 `apache2 -l`，返回编译进内核的模块名集合
// （这些模块不能再 LoadModule，否则报 "module is built-in and can't be loaded"）
func apacheBuiltinModules(t *testing.T, bin string) map[string]bool {
	t.Helper()
	out, err := exec.Command(bin, "-l").Output()
	if err != nil {
		return map[string]bool{}
	}
	builtin := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Debian: "mod_unixd.c"；部分构建: "unixd_module (built-in)"
		if i := strings.Index(line, " (built-in)"); i > 0 {
			builtin[strings.TrimSuffix(line[:i], "_module")] = true
			continue
		}
		if strings.HasSuffix(line, ".c") {
			name := strings.TrimSuffix(line, ".c")
			name = strings.TrimPrefix(name, "mod_")
			builtin[name] = true
		}
	}
	return builtin
}

func currentUser(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "nobody"
}

func currentGroup(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("id", "-gn").Output()
	if err != nil {
		return "nogroup"
	}
	return strings.TrimSpace(string(out))
}

// genCert 生成自签证书供 nginx/apache 语法校验读取
func genCert(t *testing.T, certPath, keyPath string) {
	t.Helper()
	cmd := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
		"-keyout", keyPath, "-out", certPath, "-days", "1", "-subj", "/CN=cdn.example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("生成测试证书失败（跳过语法校验）: %v\n%s", err, out)
	}
}
