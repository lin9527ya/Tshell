package api

import (
	"strings"
	"testing"

	"BackendTemplate/pkg/c2profile"
)

func baseProfile() *c2profile.Profile {
	p := &c2profile.Profile{Name: "default", UserAgent: "UA/1", SleeptimeMs: 1000}
	p.HTTP.Get.URI = "/get"
	p.HTTP.Post.URI = "/post"
	p.HTTP.Get.Padding = true
	p.WS.Path = "/ws"
	p.DNS.BeaconPrefix = "upd"
	return p
}

// TestProfileListenerMismatch 守卫"选非默认画像生成必然连不上"的一致性检查：
// 监听器只按 default 注册 HTTP 路由 / DNS 前缀 / padding，WS 固定 /ws；
// 纯客户端侧字段（UA/JA3/心跳/channel）则允许不同。
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
