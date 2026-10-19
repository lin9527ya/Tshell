package cred

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTicketZip 用真实 ccache 夹具打一个制品（与客户端 writeTicketBundle 的布局一致）。
func buildTicketZip(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tickets.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, raw := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestParseTicketArtifact 真实夹具：v3 + v4 两个缓存都要解析出来，
// 坏条目只进 Errors，不影响其它缓存。
func TestParseTicketArtifact(t *testing.T) {
	v3, err := os.ReadFile(filepath.Join("testdata", "ccache-v3"))
	if err != nil {
		t.Fatal(err)
	}
	v4, err := os.ReadFile(filepath.Join("testdata", "ccache-v4"))
	if err != nil {
		t.Fatal(err)
	}
	path := buildTicketZip(t, map[string][]byte{
		"manifest.json":      []byte(`{"host":"WS01","user":"alice","collected_at":"2026-10-19 21:00:00"}`),
		"tickets/ccache-v3":  v3,
		"tickets/ccache-v4":  v4,
		"tickets/corrupt.cc": []byte("not a ccache at all"),
	})

	art, err := ParseTicketArtifact(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if art.Host != "WS01" || art.User != "alice" {
		t.Fatalf("manifest 元信息未读到: %+v", art)
	}
	if len(art.Caches) != 2 {
		t.Fatalf("应解析出 2 个缓存，实际 %d（errors: %v）", len(art.Caches), art.Errors)
	}
	for _, c := range art.Caches {
		if c.Principal == "" || c.Tickets == 0 || c.Version == 0 {
			t.Fatalf("缓存字段不完整: %+v", c)
		}
	}
	if len(art.Errors) != 1 || !strings.Contains(art.Errors[0], "corrupt") {
		t.Fatalf("坏条目应只记 error: %v", art.Errors)
	}

	info := art.Info()
	if info.TicketCaches != 2 || info.Tickets == 0 || !info.HasManifest {
		t.Fatalf("Info 统计不符: %+v", info)
	}
	if !strings.Contains(info.Describe(), "Kerberos") {
		t.Fatalf("Describe 未体现票据: %s", info.Describe())
	}
}

// TestParseTicketArtifactNoCcache 没有可解析缓存时必须显式失败（不登记半截制品）
func TestParseTicketArtifactNoCcache(t *testing.T) {
	path := buildTicketZip(t, map[string][]byte{
		"manifest.json": []byte(`{"host":"WS01"}`),
		"tickets/x.cc":  []byte("garbage"),
	})
	if _, err := ParseTicketArtifact(path); err == nil {
		t.Fatal("没有可用 ccache 时应失败")
	}
}

// TestValidateArtifactDispatchesTicket ZIP 形态按 kind 分派：票据走票据解析，
// 不能落进 DPAPI 的通用 ZIP 校验（那条要求 manifest 且不解析 ccache）。
func TestValidateArtifactDispatchesTicket(t *testing.T) {
	v4, err := os.ReadFile(filepath.Join("testdata", "ccache-v4"))
	if err != nil {
		t.Fatal(err)
	}
	path := buildTicketZip(t, map[string][]byte{"tickets/a.ccache": v4})

	info, err := ValidateArtifact(KindTicket, "", path)
	if err != nil {
		t.Fatalf("校验失败: %v", err)
	}
	if info.TicketCaches != 1 || info.Tickets == 0 {
		t.Fatalf("票据统计不符: %+v", info)
	}
	if info.Format != FormatZip {
		t.Fatalf("形态应为 zip: %s", info.Format)
	}
}

// TestTicketKindWiring 锁住类型/技术/扩展名/形态的接线
func TestTicketKindWiring(t *testing.T) {
	if !IsKindSupported(KindTicket) {
		t.Fatal("ticket 未登记为制品类型")
	}
	if !IsTechSupportedFor(KindTicket, "files") {
		t.Fatal("ticket 缺少 files 技术")
	}
	if DefaultTech(KindTicket) != "files" {
		t.Fatalf("默认技术应为 files: %s", DefaultTech(KindTicket))
	}
	if ExtensionFor(KindTicket) != ".zip" {
		t.Fatalf("扩展名应为 .zip: %s", ExtensionFor(KindTicket))
	}
	if FormatByKind(KindTicket) != FormatZip {
		t.Fatalf("形态应为 zip: %s", FormatByKind(KindTicket))
	}
}

// TestParseTicketArtifactLSA LSA 制品：kirbi 不解析二进制，元数据来自清单；
// principal 由 client+realm 拼出，来源标记为 lsa。
func TestParseTicketArtifactLSA(t *testing.T) {
	manifest := []byte(`{
	  "host": "WS01", "user": "alice", "collected_at": "2026-10-19 22:30:00",
	  "errors": ["cifs/fs02: 导出失败"],
	  "lsa_tickets": [
	    {"file":"kirbi/01-krbtgt_CORP_LOCAL.kirbi","client":"alice","realm":"CORP.LOCAL",
	     "server":"krbtgt/CORP.LOCAL","end":"2026-10-20T08:00:00Z","flags":1088421888,"size":4},
	    {"file":"kirbi/02-cifs_fs01_corp_local.kirbi","client":"alice@CORP.LOCAL","realm":"CORP.LOCAL",
	     "server":"cifs/fs01.corp.local","end":"2026-10-19T23:00:00Z","size":4}
	  ]
	}`)
	path := buildTicketZip(t, map[string][]byte{
		"manifest.json":                       manifest,
		"kirbi/01-krbtgt_CORP_LOCAL.kirbi":    {0x61, 0x82, 0x01, 0x02},
		"kirbi/02-cifs_fs01_corp_local.kirbi": {0x61, 0x82, 0x00, 0x10},
	})

	art, err := ParseTicketArtifact(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(art.Caches) != 2 {
		t.Fatalf("应解析出 2 条 LSA 票据，实际 %d（errors: %v）", len(art.Caches), art.Errors)
	}
	first := art.Caches[0]
	if first.Source != "lsa" || first.Server != "krbtgt/CORP.LOCAL" ||
		first.Principal != "alice@CORP.LOCAL" || first.Tickets != 1 || first.Expires == "" {
		t.Fatalf("LSA 条目字段不符: %+v", first)
	}
	if art.Caches[1].Principal != "alice@CORP.LOCAL" {
		t.Fatalf("已带 @ 的 principal 不应重复拼接: %q", art.Caches[1].Principal)
	}
	info := art.Info()
	if info.TicketCaches != 2 || info.Tickets != 2 {
		t.Fatalf("Info 统计不符: %+v", info)
	}

	// 制品校验入口同样要认 LSA 制品
	v, err := ValidateArtifact(KindTicket, "", path)
	if err != nil || v.TicketCaches != 2 {
		t.Fatalf("ValidateArtifact 未按票据解析: %+v, %v", v, err)
	}
}
