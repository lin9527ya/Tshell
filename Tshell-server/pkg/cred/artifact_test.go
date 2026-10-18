package cred

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ===== 制品形态与命名 =====

func TestFormatAndExtensionByKind(t *testing.T) {
	cases := []struct {
		kind, format, ext string
	}{
		{KindLsass, FormatMinidump, ".dmp"},
		{KindDPAPI, FormatZip, ".zip"},
		{KindBrowser, FormatJSON, ".json"},
	}
	for _, tc := range cases {
		if got := FormatByKind(tc.kind); got != tc.format {
			t.Errorf("FormatByKind(%s) = %s，期望 %s", tc.kind, got, tc.format)
		}
		if got := ExtensionFor(tc.kind); got != tc.ext {
			t.Errorf("ExtensionFor(%s) = %s，期望 %s", tc.kind, got, tc.ext)
		}
	}
	// 未知类型退回 minidump/.dmp，但不代表它被支持
	if ExtensionFor("nope") != ".dmp" || FormatByKind("nope") != FormatMinidump {
		t.Fatal("未知类型应有稳定的兜底形态")
	}
}

func TestNewFileNameUsesKindExtension(t *testing.T) {
	now := time.Date(2026, 10, 18, 13, 14, 15, 0, time.UTC)
	if got := NewFileName(KindLsass, "DC01", now); got != "lsass-DC01-20261018-131415.dmp" {
		t.Fatalf("LSASS 制品名 = %s", got)
	}
	if got := NewFileName(KindDPAPI, "DC01", now); got != "dpapi-DC01-20261018-131415.zip" {
		t.Fatalf("DPAPI 制品名 = %s", got)
	}
	if got := NewFileName(KindBrowser, "WS01.corp.local", now); got != "browser-WS01_corp_local-20261018-131415.json" {
		t.Fatalf("浏览器制品名 = %s（主机名应被净化）", got)
	}
}

// ===== 类型/技术组合 =====

func TestTechsAreScopedByKind(t *testing.T) {
	// 支持的组合
	ok := []struct{ kind, tech string }{
		{KindLsass, "auto"}, {KindLsass, "comsvcs"}, {KindLsass, "dbgcore"},
		{KindDPAPI, "keys"},
		{KindBrowser, "all"}, {KindBrowser, "chrome"}, {KindBrowser, "edge"},
	}
	for _, c := range ok {
		if !IsKindSupported(c.kind) || !IsTechSupportedFor(c.kind, c.tech) {
			t.Errorf("%s/%s 应被支持", c.kind, c.tech)
		}
	}
	// 跨类型误配必须被拒：下发这种组合客户端只会报错
	bad := []struct{ kind, tech string }{
		{KindBrowser, "comsvcs"}, {KindDPAPI, "dbgcore"}, {KindLsass, "chrome"},
		{KindBrowser, ""}, {KindDPAPI, "auto"}, {"sam", "auto"},
	}
	for _, c := range bad {
		if IsKindSupported(c.kind) && IsTechSupportedFor(c.kind, c.tech) {
			t.Errorf("%s/%s 不应被支持", c.kind, c.tech)
		}
	}
	// 默认技术必须是该类型自己的第一项
	if DefaultTech(KindBrowser) != "all" || DefaultTech(KindDPAPI) != "keys" || DefaultTech(KindLsass) != "auto" {
		t.Fatal("默认技术应按类型取")
	}
}

func TestSanitizeFillsDefaults(t *testing.T) {
	// 旧调用方直接构造 ArtifactRequest（不带 Format/Kind/Tech）时，
	// Pending 返回给前端与回传校验都必须拿到完整字段
	var req ArtifactRequest
	req.Sanitize()
	if req.Kind != KindLsass || req.Tech != "auto" || req.Format != FormatMinidump {
		t.Fatalf("默认补全不符: %+v", req)
	}
	req2 := ArtifactRequest{Kind: KindBrowser}
	req2.Sanitize()
	if req2.Tech != "all" || req2.Format != FormatJSON {
		t.Fatalf("browser 默认补全不符: %+v", req2)
	}
}

func TestIssueSanitizesRecord(t *testing.T) {
	r := NewRegistry()
	if err := r.Issue(ArtifactRequest{UID: "u1", FileName: "browser-x.json", Kind: KindBrowser}, time.Hour); err != nil {
		t.Fatal(err)
	}
	got := r.Pending("u1")
	if len(got) != 1 || got[0].Tech != "all" || got[0].Format != FormatJSON {
		t.Fatalf("登记记录未补全字段: %+v", got)
	}
}

// ===== ZIP 制品校验 =====

// writeZip 造一个 ZIP 制品；crcBroken 时把某条目内容改掉但保留原 CRC，
// 用来模拟"文件下到一半/落盘损坏"的形态
func writeZip(t *testing.T, entries map[string]string, crcBroken bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dpapi-test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if crcBroken {
		corruptZipPayload(t, path)
	}
	return path
}

// corruptZipPayload 在"存储"（不压缩）条目的数据区翻一个字节：
// CRC 校验必须能发现它 —— 只看中央目录是看不出来的。
func corruptZipPayload(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// 找到本地文件头（PK\x03\x04），跳过 30 字节固定头 + 文件名/扩展区，改第一个数据字节
	for i := 0; i+30 < len(raw); i++ {
		if raw[i] == 'P' && raw[i+1] == 'K' && raw[i+2] == 3 && raw[i+3] == 4 {
			nameLen := int(raw[i+26]) | int(raw[i+27])<<8
			extraLen := int(raw[i+28]) | int(raw[i+29])<<8
			dataOff := i + 30 + nameLen + extraLen
			if dataOff < len(raw) {
				raw[dataOff] ^= 0xFF
				if err := os.WriteFile(path, raw, 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	t.Fatal("未找到 ZIP 本地文件头")
}

func TestValidateZipAcceptsCompleteArtifact(t *testing.T) {
	path := writeZip(t, map[string]string{
		"manifest.json": `{"host":"DC01","user":"CORP\\alice","keys":1}`,
		"keys/abc.json": `{"guid":"abc","masterkey":"0a1b"}`,
	}, false)

	info, err := ValidateZip(path)
	if err != nil {
		t.Fatalf("合法 ZIP 应通过: %v", err)
	}
	if info.Format != FormatZip || info.Entries != 2 || !info.HasManifest {
		t.Fatalf("校验结果不符: %+v", info)
	}
	if info.Size <= 0 || info.Describe() == "" {
		t.Fatalf("应给出体积与说明: %+v", info)
	}
}

func TestValidateZipRejectsIncomplete(t *testing.T) {
	cases := []struct {
		name string
		path func(t *testing.T) string
	}{
		{"空文件", func(t *testing.T) string {
			p := filepath.Join(t.TempDir(), "empty.zip")
			if err := os.WriteFile(p, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return p
		}},
		{"截断（只剩头部）", func(t *testing.T) string {
			full := writeZip(t, map[string]string{"manifest.json": `{"host":"DC01"}`}, false)
			raw, err := os.ReadFile(full)
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(t.TempDir(), "trunc.zip")
			if err := os.WriteFile(p, raw[:len(raw)/2], 0o600); err != nil {
				t.Fatal(err)
			}
			return p
		}},
		{"内容损坏（CRC 不符）", func(t *testing.T) string {
			return writeZip(t, map[string]string{"manifest.json": `{"host":"DC01"}`}, true)
		}},
		{"缺 manifest", func(t *testing.T) string {
			return writeZip(t, map[string]string{"keys/abc.json": `{}`}, false)
		}},
		{"空 ZIP", func(t *testing.T) string {
			return writeZip(t, map[string]string{}, false)
		}},
		{"文件不存在", func(t *testing.T) string {
			return filepath.Join(t.TempDir(), "nope.zip")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateZip(tc.path(t)); err == nil {
				t.Fatal("应校验失败")
			}
		})
	}
}

// ===== 浏览器制品解析 =====

func sampleBrowserArtifact() BrowserArtifact {
	return BrowserArtifact{
		Host: "WS01", User: `CORP\alice`, CollectedAt: "2026-10-18 13:14:15",
		Browsers: []BrowserProfile{
			{
				Name: "chrome", Profile: "Default", Tech: "v20", MasterKey: "AAECAwQ=",
				Credentials: []BrowserCredential{
					{URL: "https://portal.corp.local/login", User: "alice", Pass: "S3cr3t!"},
					{URL: "", User: "nobody", Pass: "ignored"},          // 缺 URL → 丢弃
					{URL: "https://ok.example/", User: "bob", Pass: ""}, // 缺口令 → 丢弃
				},
				Cookies: []BrowserCookie{
					{Host: ".corp.local", Name: "SESSIONID", Value: "abc123", Path: "/"},
					{Host: ".corp.local", Name: "", Value: "x"}, // 缺 name → 丢弃
				},
			},
		},
	}
}

func writeBrowserArtifact(t *testing.T, art interface{}) string {
	t.Helper()
	raw, err := json.Marshal(art)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "browser-x.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseBrowserArtifactFiltersIncompleteEntries(t *testing.T) {
	path := writeBrowserArtifact(t, sampleBrowserArtifact())

	art, err := ParseBrowserArtifact(path)
	if err != nil {
		t.Fatalf("合法制品应通过: %v", err)
	}
	if len(art.Browsers) != 1 {
		t.Fatalf("浏览器配置数 = %d", len(art.Browsers))
	}
	// 缺 URL/口令的条目必须被丢掉，不能污染凭据库
	if len(art.Browsers[0].Credentials) != 1 || art.Browsers[0].Credentials[0].User != "alice" {
		t.Fatalf("口令过滤不符: %+v", art.Browsers[0].Credentials)
	}
	if len(art.Browsers[0].Cookies) != 1 || art.Browsers[0].Cookies[0].Name != "SESSIONID" {
		t.Fatalf("Cookie 过滤不符: %+v", art.Browsers[0].Cookies)
	}
	// 丢弃条目时补齐 browser/profile 归属，便于入库
	if art.Browsers[0].Credentials[0].Browser != "chrome" || art.Browsers[0].Credentials[0].Profile != "Default" {
		t.Fatalf("应补齐来源归属: %+v", art.Browsers[0].Credentials[0])
	}

	info := art.Info()
	if info.Browsers != 1 || info.Credentials != 1 || info.Cookies != 1 {
		t.Fatalf("统计不符: %+v", info)
	}
	if info.Describe() == "" {
		t.Fatal("应能生成说明")
	}
}

func TestParseBrowserArtifactRejectsIncomplete(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	truncated := filepath.Join(t.TempDir(), "trunc.json")
	if err := os.WriteFile(truncated, []byte(`{"host":"WS01","browsers":[{"name":"chrome"`), 0o600); err != nil {
		t.Fatal(err)
	}
	noBrowser := filepath.Join(t.TempDir(), "nobrowser.json")
	if err := os.WriteFile(noBrowser, []byte(`{"host":"WS01","browsers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// 有配置但一条可用条目都没有：采集失败的真实形态，必须显式失败
	noEntries := filepath.Join(t.TempDir(), "noentries.json")
	if err := os.WriteFile(noEntries,
		[]byte(`{"host":"WS01","browsers":[{"name":"chrome","credentials":[],"cookies":[]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oversized := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(oversized, make([]byte, MaxBrowserArtifactBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"空文件": empty, "半截 JSON": truncated, "无浏览器配置": noBrowser,
		"无可用条目": noEntries, "体积超限": oversized,
		"文件不存在": filepath.Join(t.TempDir(), "nope.json"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseBrowserArtifact(path); err == nil {
				t.Fatal("应解析失败")
			}
		})
	}
}

func TestParseBrowserArtifactRejectsOversizedEntries(t *testing.T) {
	art := BrowserArtifact{Host: "WS01", Browsers: []BrowserProfile{{Name: "chrome", Profile: "Default"}}}
	for i := 0; i < MaxCredentialItems+1; i++ {
		art.Browsers[0].Credentials = append(art.Browsers[0].Credentials,
			BrowserCredential{URL: "https://x/", User: "u", Pass: "p"})
	}
	if _, err := ParseBrowserArtifact(writeBrowserArtifact(t, art)); err == nil {
		t.Fatal("口令条数超限应被拒")
	}

	art2 := BrowserArtifact{Host: "WS01", Browsers: []BrowserProfile{{Name: "edge", Profile: "Default"}}}
	for i := 0; i < MaxBrowserCookies+1; i++ {
		art2.Browsers[0].Cookies = append(art2.Browsers[0].Cookies,
			BrowserCookie{Host: ".x", Name: "c", Value: "v"})
	}
	if _, err := ParseBrowserArtifact(writeBrowserArtifact(t, art2)); err == nil {
		t.Fatal("Cookie 条数超限应被拒")
	}
}

func TestRealmFromURL(t *testing.T) {
	cases := map[string]string{
		"https://portal.corp.local/login?x=1": "portal.corp.local",
		"http://10.0.0.5:8080/admin":          "10.0.0.5",
		"corp.local":                          "corp.local",
		"":                                    "",
	}
	for in, want := range cases {
		if got := RealmFromURL(in); got != want {
			t.Errorf("RealmFromURL(%q) = %q，期望 %q", in, got, want)
		}
	}
	// 解析不出的超长字符串要截断，不能把整段垃圾写进 realm 列
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}
	if got := RealmFromURL(string(long)); len(got) > 64 {
		t.Fatalf("超长 realm 未截断: %d", len(got))
	}
}

// ===== 统一入口 =====

func TestValidateArtifactDispatchesByFormat(t *testing.T) {
	// format 为空 → 按 kind 推断（兼容旧登记记录）
	if _, err := ValidateArtifact(KindLsass, "", filepath.Join(t.TempDir(), "nope.dmp")); err == nil {
		t.Fatal("minidump 分支应报错")
	}
	if _, err := ValidateArtifact(KindDPAPI, "", writeZip(t, map[string]string{"manifest.json": "{}"}, false)); err != nil {
		t.Fatalf("zip 分支应通过: %v", err)
	}
	if _, err := ValidateArtifact(KindBrowser, "", writeBrowserArtifact(t, sampleBrowserArtifact())); err != nil {
		t.Fatalf("json 分支应通过: %v", err)
	}
	if _, err := ValidateArtifact(KindLsass, "magic", "x"); err == nil {
		t.Fatal("未知形态应报错")
	}
}

// 校验结果与 Describe 的兼容性：LSASS 路径的说明文案不能退化
func TestMinidumpInfoDescribeStillWorks(t *testing.T) {
	info := ArtifactInfo{Size: 2 << 20, Format: FormatMinidump, Entries: 5, HasMemory: true}
	got := info.Describe()
	if got == "" || !containsStr(got, "含完整内存") || !containsStr(got, "5 个数据流") {
		t.Fatalf("minidump 说明不符: %s", got)
	}
}
