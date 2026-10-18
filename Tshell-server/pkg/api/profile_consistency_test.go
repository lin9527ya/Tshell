package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"BackendTemplate/pkg/c2profile"
)

// ensureTestProfiles 在临时目录里初始化画像注册表（含 default）。
//
// 为什么需要：监听器现在要求"绑定的画像必须存在"，而 api 包的测试进程默认没有
// 加载过 profiles/ 目录 —— 不初始化的话每个用例都会得到"C2 画像不存在: default"。
// 用 t.TempDir() 而不是仓库里的 profiles/：测试不该依赖工作目录里有没有那个文件夹。
func ensureTestProfiles(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	yaml := `name: default
description: 测试用默认画像
useragent: UA/1
sleeptime_ms: 1000
http:
  get:
    uri: /get
    padding: true
  post:
    uri: /post
ws:
  path: /ws
dns:
  beacon_prefix: upd
`
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("写测试画像失败: %v", err)
	}
	if err := c2profile.InitNoWatch(dir); err != nil {
		t.Fatalf("初始化画像注册表失败: %v", err)
	}
	if c2profile.Get(c2profile.DefaultProfileName) == nil {
		t.Fatal("初始化后 default 画像应存在")
	}
}

func baseProfile() *c2profile.Profile {
	p := &c2profile.Profile{Name: "default", UserAgent: "UA/1", SleeptimeMs: 1000}
	p.HTTP.Get.URI = "/get"
	p.HTTP.Post.URI = "/post"
	p.HTTP.Get.Padding = true
	p.WS.Path = "/ws"
	p.DNS.BeaconPrefix = "upd"
	return p
}

// TestProfileListenerMismatch 守卫"载荷画像与监听器画像不一致就必然连不上"的一致性检查：
//
// 监听器侧特征（HTTP get/post 路由、WS 路径、DNS 前缀、padding）在监听器启动时按它
// **绑定的画像**注册，所以载荷必须用同一个画像；纯客户端侧字段（UA/JA3/心跳/channel）
// 则允许不同。
//
// 注意测试里两个画像的 Name 都是 "default"：这正是为了确认**逐字段比较**生效
// （曾经按 Name 短路，导致本用例在改了字段后仍然通过 —— 空转的测试）。
func TestProfileListenerMismatch(t *testing.T) {
	def := baseProfile()

	cases := []struct {
		name      string
		mutate    func(p *c2profile.Profile)
		wantMatch string // 期望错误里出现的关键字；空=允许
	}{
		{"完全一致", func(p *c2profile.Profile) {}, ""},
		{"仅客户端侧字段不同", func(p *c2profile.Profile) {
			p.UserAgent = "UA/2"
			p.TLS.JA3 = "firefox_121"
			p.SleeptimeMs = 5000
			p.HTTP.Get.MetadataChannel = "url"
		}, ""},
		{"get uri 不同", func(p *c2profile.Profile) { p.HTTP.Get.URI = "/x" }, "http.get.uri"},
		{"post uri 不同", func(p *c2profile.Profile) { p.HTTP.Post.URI = "/y" }, "http.post.uri"},
		{"ws 路径非 /ws", func(p *c2profile.Profile) { p.WS.Path = "/wsx" }, "ws.path"},
		{"dns 前缀不同", func(p *c2profile.Profile) { p.DNS.BeaconPrefix = "other" }, "dns.beacon_prefix"},
		{"padding 不同", func(p *c2profile.Profile) { p.HTTP.Get.Padding = false }, "http.get.padding"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := baseProfile()
			tc.mutate(p)
			got := profileListenerMismatch(p, def)
			if tc.wantMatch == "" {
				if got != "" {
					t.Fatalf("应允许，实际被拒: %s", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantMatch) {
				t.Fatalf("拒绝原因应提到 %q，实际: %q", tc.wantMatch, got)
			}
		})
	}

	// nil 安全
	if got := profileListenerMismatch(nil, def); got != "" {
		t.Fatalf("nil profile 不应报错: %s", got)
	}
}
