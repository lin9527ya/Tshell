package services

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindCCacheFilesViaEnv KRB5CCNAME（FILE: 前缀）必须优先被采纳
func TestFindCCacheFilesViaEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "krb5cc_test")
	if err := os.WriteFile(p, []byte("ccache-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KRB5CCNAME", "FILE:"+p)

	files, _ := findCCacheFiles()
	found := false
	for _, f := range files {
		if f == p {
			found = true
		}
	}
	if !found {
		t.Fatalf("KRB5CCNAME 指定的文件未被采集: %v", files)
	}
}

// TestFindCCacheFilesNonFileBackend KEYRING:/DIR: 这类后端无法打包成文件，
// 必须记一条说明而不是静默当作"没票据"。
func TestFindCCacheFilesNonFileBackend(t *testing.T) {
	t.Setenv("KRB5CCNAME", "KEYRING:persistent:1000")
	_, notes := findCCacheFiles()
	joined := strings.Join(notes, "；")
	if !strings.Contains(joined, "非文件后端") {
		t.Fatalf("应记录非文件后端说明: %v", notes)
	}
}

// TestWriteTicketBundle 制品布局：manifest.json + tickets/<name>，
// 清单里带 host/user/files，客户端不解析票据内容。
func TestWriteTicketBundle(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "krb5cc_1000")
	b := filepath.Join(dir, "krb5cc_alice")
	if err := os.WriteFile(a, []byte("cache-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("cache-b"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "tickets.zip")
	n, err := writeTicketBundle(out, []string{a, b}, []string{"note-1"})
	if err != nil {
		t.Fatalf("打包失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("应写入 2 个缓存，实际 %d", n)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	seen := map[string]bool{}
	for _, f := range zr.File {
		seen[f.Name] = true
	}
	for _, want := range []string{"manifest.json", "tickets/krb5cc_1000", "tickets/krb5cc_alice"} {
		if !seen[want] {
			t.Fatalf("制品缺少条目 %s（实际: %v）", want, seen)
		}
	}
}

// TestCredTicketTechGating ticket 只接受 files，且默认技术是 files
func TestCredTicketTechGating(t *testing.T) {
	if !credTechAllowed("ticket", "files") {
		t.Fatal("ticket 应接受 files")
	}
	if credTechAllowed("ticket", "comsvcs") {
		t.Fatal("ticket 不应接受 comsvcs")
	}
	if got := defaultCredTech("ticket"); got != "files" {
		t.Fatalf("默认技术应为 files，实际 %s", got)
	}
}
