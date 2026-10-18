package c2profile

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadProfileAndIgnoreLegacyFields 验证两件事：
//  1. 现役字段（URI/WS/TLS/DNS/UA/心跳/channel/padding）能正常解析；
//  2. 已删除的死字段（jitter_percent / headers / metadata_cookie_prepend / query_type）
//     若仍残留在旧 YAML 里，加载不会失败（yaml 非严格模式忽略未知键）——
//     否则升级后老部署会因 profile 解析失败而起不来。
func TestLoadProfileAndIgnoreLegacyFields(t *testing.T) {
	dir := t.TempDir()
	legacy := `
name: legacy
useragent: "UA/1.0"
sleeptime_ms: 3000
jitter_percent: 20
http:
  get:
    uri: "/get"
    headers:
      X-Test: "1"
    metadata_cookie_prepend: "BDUSS="
    metadata_channel: cookie
    padding: true
  post:
    uri: "/post"
ws:
  path: "/wsx"
tls:
  ja3: "chrome_120"
dns:
  beacon_prefix: "upd"
  query_type: "TXT"
`
	if err := os.WriteFile(filepath.Join(dir, "legacy.yaml"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InitNoWatch(dir); err != nil {
		t.Fatalf("加载 profile 失败: %v", err)
	}

	p := Get("legacy")
	if p == nil {
		t.Fatal("legacy profile 未加载")
	}
	if p.Name != "legacy" || p.UserAgent != "UA/1.0" || p.SleeptimeMs != 3000 {
		t.Fatalf("基础字段解析不符: %+v", p)
	}
	if p.HTTP.Get.URI != "/get" || p.HTTP.Post.URI != "/post" {
		t.Fatalf("HTTP URI 解析不符: %+v", p.HTTP)
	}
	if p.HTTP.Get.MetadataChannel != "cookie" || !p.HTTP.Get.Padding {
		t.Fatalf("channel/padding 解析不符: %+v", p.HTTP.Get)
	}
	if p.WS.Path != "/wsx" || p.TLS.JA3 != "chrome_120" || p.DNS.BeaconPrefix != "upd" {
		t.Fatalf("WS/TLS/DNS 解析不符: ws=%q ja3=%q dns=%q", p.WS.Path, p.TLS.JA3, p.DNS.BeaconPrefix)
	}

	// 默认画像必须自动生成且可解析（ensureDefault）
	if d := GetDefault(); d == nil || d.HTTP.Get.URI == "" || d.WS.Path == "" {
		t.Fatalf("默认画像缺失或字段为空: %+v", d)
	}
}
