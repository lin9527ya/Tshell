package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"BackendTemplate/pkg/c2profile"
)

// per-listener 画像（阶段三 §3.6）的验收：
//
// 1. 监听器可以绑一个**非 default** 的画像；
// 2. HTTP 路由按它绑定的画像注册（GET/POST 各是画像里的 URI）；
// 3. WS 路径不再硬编码 /ws，而是画像里的 ws.path；
// 4. 生成载荷时的一致性基准是**该监听器的画像**，不是全局 default
//    —— 同一台服务器上两个监听器各用一套特征，互不干扰；
// 5. 画像缺失时回落 default 并告警，而不是拒绝启动（升级路径不能因为少文件就挂）。

// writeProfile 往画像目录写一个 yaml
func writeProfile(t *testing.T, dir, name, getURI, postURI, wsPath string) {
	t.Helper()
	yaml := "name: " + name + "\n" +
		"description: 测试画像 " + name + "\n" +
		"useragent: UA/" + name + "\n" +
		"sleeptime_ms: 1000\n" +
		"http:\n  get:\n    uri: " + getURI + "\n    padding: true\n" +
		"  post:\n    uri: " + postURI + "\n" +
		"ws:\n  path: " + wsPath + "\n" +
		"dns:\n  beacon_prefix: " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("写画像 %s 失败: %v", name, err)
	}
}

// setupTwoProfiles 初始化一个含 default 与 alt（特征完全不同）两份画像的注册表
func setupTwoProfiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeProfile(t, dir, "default", "/get", "/post", "/ws")
	writeProfile(t, dir, "alt", "/api/v2/query", "/api/v2/submit", "/chat/socket")
	if err := c2profile.InitNoWatch(dir); err != nil {
		t.Fatalf("初始化画像失败: %v", err)
	}
	return dir
}

// 非 default 画像的监听器必须按该画像注册路由
func TestRegisterHTTPRoutesUsesListenerProfile(t *testing.T) {
	setupTwoProfiles(t)

	mux := http.NewServeMux()
	// 只验证"注册了哪些路径"，不关心处理器行为
	registerHTTPRoutes(mux, "k", "s", "alt")

	probe := func(path string) int {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}

	if code := probe("/api/v2/query"); code == http.StatusNotFound {
		t.Error("按 alt 画像注册后，GET /api/v2/query 应已注册")
	}
	if code := probe("/api/v2/submit"); code == http.StatusNotFound {
		t.Error("按 alt 画像注册后，POST /api/v2/submit 应已注册")
	}
	// default 的路由不该出现（这个监听器用的是 alt）
	if code := probe("/get"); code != http.StatusNotFound {
		t.Errorf("按 alt 注册后不应再监听 default 的 /get，实际 %d", code)
	}
}

// WS 路径按画像走，不再硬编码 /ws
func TestResolveWSRouteFollowsProfile(t *testing.T) {
	setupTwoProfiles(t)

	if got := resolveWSRoute("alt"); got != "/chat/socket" {
		t.Errorf("alt 画像的 WS 路径应为 /chat/socket，实际 %q", got)
	}
	if got := resolveWSRoute(""); got != "/ws" {
		t.Errorf("空画像名应回落 default 的 /ws，实际 %q", got)
	}
	if got := resolveWSRoute("default"); got != "/ws" {
		t.Errorf("default 画像的 WS 路径应为 /ws，实际 %q", got)
	}
}

// 画像文件缺失时回落 default（升级路径：老库里的监听器 Profile 为空，且不能因为
// profiles/ 目录缺文件就起不来）
func TestProfileFallbackWhenMissing(t *testing.T) {
	setupTwoProfiles(t)

	name, p := resolveListenerProfile("")
	if name != c2profile.DefaultProfileName || p == nil {
		t.Fatalf("空画像名应回落 default，得到 %q / %v", name, p)
	}

	// 不存在的画像名：仍回落 default（不 panic、不返回 nil 路径）
	name2, p2 := resolveListenerProfile("no-such-profile")
	if name2 != "no-such-profile" {
		t.Errorf("画像名应原样保留以便日志定位，实际 %q", name2)
	}
	if p2 != nil {
		t.Error("不存在的画像应返回 nil，由调用方兜底")
	}

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, "k", "s", "no-such-profile")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c2profile.DefaultGetURI, nil))
	if w.Code == http.StatusNotFound {
		t.Errorf("画像缺失时应回落到 default 的 %s 而不是什么都不注册（实际状态 %d）",
			c2profile.DefaultGetURI, w.Code)
	}
	if got := resolveWSRoute("no-such-profile"); got != c2profile.DefaultWSPath {
		t.Errorf("画像缺失时 WS 路径应回落 %q，实际 %q", c2profile.DefaultWSPath, got)
	}
}

// 生成载荷的一致性基准必须是**该监听器的画像**：
// 用 alt 监听器 + alt 画像应放行；用 alt 监听器 + default 画像应拒绝。
func TestListenerProfileForPayloadPicksBoundProfile(t *testing.T) {
	setupTwoProfiles(t)
	setupTestDatabase(t)

	insertListener(t, "alt-listener", "0.0.0.0:19001", "alt")
	insertListener(t, "default-listener", "0.0.0.0:19002", "")

	alt := c2profile.Get("alt")
	def := c2profile.Get(c2profile.DefaultProfileName)

	// alt 监听器 → 基准是 alt 画像
	got := listenerProfileForPayload("http://0.0.0.0:19001")
	if got == nil || got.Name != "alt" {
		t.Fatalf("应取到 alt 画像，实际 %v", got)
	}
	if msg := profileListenerMismatch(alt, got); msg != "" {
		t.Errorf("alt 监听器 + alt 画像应放行，实际被拒: %s", msg)
	}
	if msg := profileListenerMismatch(def, got); msg == "" {
		t.Error("alt 监听器 + default 画像应被拒绝（否则生成一个必然连不上的载荷）")
	} else if !strings.Contains(msg, "alt") {
		t.Errorf("拒绝原因应指出目标监听器绑的是哪个画像: %s", msg)
	}

	// 空 Profile 的老监听器 → 基准回落 default，行为与升级前一致
	got2 := listenerProfileForPayload("http://0.0.0.0:19002")
	if got2 == nil || got2.Name != c2profile.DefaultProfileName {
		t.Fatalf("空 Profile 应回落 default，实际 %v", got2)
	}
	if msg := profileListenerMismatch(def, got2); msg != "" {
		t.Errorf("default 监听器 + default 画像应放行（升级路径不能变严），实际: %s", msg)
	}

	// 库里没有的地址（重定向器/域前置）→ 回落 default，不因此拒绝
	if got3 := listenerProfileForPayload("http://203.0.113.9:443"); got3 == nil {
		t.Error("查不到监听器时应回落 default 而不是 nil")
	}
}

// 同一台服务器上两个监听器各用一套特征：生成的 mux 互不干扰
func TestTwoListenersDifferentProfilesCoexist(t *testing.T) {
	setupTwoProfiles(t)

	muxDefault := http.NewServeMux()
	registerHTTPRoutes(muxDefault, "k1", "s1", "default")
	muxAlt := http.NewServeMux()
	registerHTTPRoutes(muxAlt, "k2", "s2", "alt")

	// 各自的路径都在各自的 mux 上
	for _, tc := range []struct {
		mux  *http.ServeMux
		path string
	}{
		{muxDefault, "/get"},
		{muxDefault, "/post"},
		{muxAlt, "/api/v2/query"},
		{muxAlt, "/api/v2/submit"},
	} {
		w := httptest.NewRecorder()
		tc.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code == http.StatusNotFound {
			t.Errorf("路径 %s 应在其所属 mux 上注册", tc.path)
		}
	}

	// 交叉验证：default 的 mux 上没有 alt 的路径（反之亦然）
	w := httptest.NewRecorder()
	muxDefault.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v2/query", nil))
	if w.Code != http.StatusNotFound {
		t.Error("default 监听器不应响应 alt 画像的路径")
	}
}
