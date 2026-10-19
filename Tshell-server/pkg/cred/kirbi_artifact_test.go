package cred

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===== kirbi 作为制品条目被服务端直接解析 =====
//
// 这条守的是一个真实的能力缺口：此前 `ParseTicketArtifact` **跳过** kirbi 条目，
// 完全依赖客户端清单里的 `lsa_tickets`。清单缺失/漏项时票据就登记不上。
// 现在服务端能自己解析 kirbi（enc-part 是明文，无需密钥），
// 所以"只有 .kirbi、没有清单"的制品也必须能登记出票据。

func kirbiFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "ccache-v4-kirbi"))
	if err != nil {
		t.Fatalf("读 kirbi 夹具失败: %v", err)
	}
	return b
}

// 没有清单、只有 kirbi：必须解析出票据
func TestParseTicketArtifactKirbiWithoutManifest(t *testing.T) {
	zipPath := buildTicketZip(t, map[string][]byte{
		"kirbi/krbtgt.kirbi": kirbiFixture(t),
	})
	art, err := ParseTicketArtifact(zipPath)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(art.Caches) != 1 {
		t.Fatalf("应登记 1 个票据缓存，实际 %d", len(art.Caches))
	}
	c := art.Caches[0]
	if c.Source != "lsa" {
		t.Errorf("来源应为 lsa，实际 %q", c.Source)
	}
	if c.Version != 0 {
		t.Errorf("kirbi 没有 ccache 版本号，Version 应为 0，实际 %d", c.Version)
	}
	if c.Principal != wantClient {
		t.Errorf("主体应为 %s，实际 %s", wantClient, c.Principal)
	}
	if c.Realm != wantRealm {
		t.Errorf("realm 应为 %s，实际 %s", wantRealm, c.Realm)
	}
	if c.Tickets != 1 {
		t.Errorf("票据数应为 1，实际 %d", c.Tickets)
	}
	if c.Expires == "" {
		t.Error("到期时间应被解析出来（kirbi 用 GeneralizedTime，能转成 RFC3339）")
	}
	if !strings.HasPrefix(c.Expires, "2022-02-1") {
		t.Errorf("到期时间应落在夹具的 2022-02-10，实际 %s", c.Expires)
	}
}

// 清单与 kirbi 同时存在时：以**二进制解析结果**为准，不重复登记
func TestParseTicketArtifactKirbiPreferredOverManifest(t *testing.T) {
	manifest := []byte(`{
	  "host": "WS01", "user": "user01", "collected_at": "2026-10-19T00:00:00Z",
	  "lsa_tickets": [
	    {"file": "kirbi/krbtgt.kirbi", "client": "user01", "realm": "INNOVATION.ROCKS",
	     "server": "krbtgt/INNOVATION.ROCKS", "end": "2022-02-10T03:06:49Z"}
	  ]
	}`)
	zipPath := buildTicketZip(t, map[string][]byte{
		"manifest.json":      manifest,
		"kirbi/krbtgt.kirbi": kirbiFixture(t),
	})
	art, err := ParseTicketArtifact(zipPath)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	// 同名条目只应登记一次
	if len(art.Caches) != 1 {
		t.Fatalf("同名 kirbi 不应重复登记，实际 %d 条: %+v", len(art.Caches), art.Caches)
	}
	c := art.Caches[0]
	// 二进制解析出的是完整主体名（含 realm），清单里的可能是拼出来的
	if c.Principal != wantClient {
		t.Errorf("应以二进制解析结果为准（%s），实际 %s", wantClient, c.Principal)
	}
	if c.Tickets != 1 {
		t.Errorf("票据数应为 1，实际 %d", c.Tickets)
	}
}

// 清单里有、但制品里没有对应 kirbi 文件：仍按清单登记（兼容老客户端）
func TestParseTicketArtifactManifestOnlyStillWorks(t *testing.T) {
	manifest := []byte(`{
	  "host": "WS01", "user": "user01", "collected_at": "2026-10-19T00:00:00Z",
	  "lsa_tickets": [
	    {"file": "kirbi/only-in-manifest.kirbi", "client": "bob", "realm": "CORP.LOCAL",
	     "server": "cifs/fileserver", "end": "2026-12-31T00:00:00Z"}
	  ]
	}`)
	zipPath := buildTicketZip(t, map[string][]byte{
		"manifest.json": manifest,
		// 一个无关条目，保证 ZIP 非空
		"README.txt": []byte("no binaries here"),
	})
	art, err := ParseTicketArtifact(zipPath)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(art.Caches) != 1 {
		t.Fatalf("应按清单登记 1 条，实际 %d", len(art.Caches))
	}
	c := art.Caches[0]
	if c.Source != "lsa" || c.Server != "cifs/fileserver" {
		t.Errorf("清单登记字段不对: %+v", c)
	}
	if c.Principal != "bob@CORP.LOCAL" {
		t.Errorf("主体应拼成 bob@CORP.LOCAL，实际 %s", c.Principal)
	}
}

// 坏掉的 kirbi 应给出可读错误，且不影响同制品里其它好票据
func TestParseTicketArtifactBadKirbiIsReported(t *testing.T) {
	zipPath := buildTicketZip(t, map[string][]byte{
		"kirbi/broken.kirbi": []byte("not a kirbi at all"),
		"kirbi/krbtgt.kirbi": kirbiFixture(t),
	})
	art, err := ParseTicketArtifact(zipPath)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(art.Caches) != 1 {
		t.Fatalf("好票据应仍被登记，实际 %d", len(art.Caches))
	}
	if len(art.Errors) == 0 {
		t.Error("坏掉的 kirbi 应记入 Errors，让操作员知道少了一张")
	}
	joined := strings.Join(art.Errors, " ")
	if !strings.Contains(joined, "broken.kirbi") {
		t.Errorf("错误信息应点出文件名，实际: %s", joined)
	}
}
