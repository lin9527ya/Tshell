package services

import (
	"archive/zip"
	"encoding/json"
	"io"
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

// TestWriteLSATicketBundle LSA 制品布局：manifest.json（lsa_tickets 元数据）+
// kirbi/<序号>-<服务名>.kirbi；服务名里的 / 与 . 都会被净化成 _。
func TestWriteLSATicketBundle(t *testing.T) {
	tickets := []lsaTicket{
		{
			Client: "alice", ClientRealm: "CORP.LOCAL",
			Server: "krbtgt/CORP.LOCAL", Start: 1700000000, End: 1700086400,
			Flags: 0x40e10000, EncType: 18, KerbCred: []byte{0x61, 0x82, 0x01, 0x02},
		},
		{
			Client: "alice", ClientRealm: "CORP.LOCAL",
			Server: "cifs/fs01.corp.local", End: 1700003600,
			EncType: 17, KerbCred: []byte{0x61, 0x82, 0x00, 0x10},
		},
		{Server: "empty", KerbCred: nil}, // 没有字节 → 不写条目
	}
	out := filepath.Join(t.TempDir(), "lsa.zip")
	n, err := writeLSATicketBundle(out, tickets, []string{"x: 导出失败"})
	if err != nil {
		t.Fatalf("打包失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("应写入 2 张票据，实际 %d", n)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	entries := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw := make([]byte, f.UncompressedSize64)
		_, _ = io.ReadFull(rc, raw)
		rc.Close()
		entries[f.Name] = raw
	}

	first := "kirbi/01-krbtgt_CORP_LOCAL.kirbi"
	second := "kirbi/02-cifs_fs01_corp_local.kirbi"
	if _, ok := entries[first]; !ok {
		t.Fatalf("缺少条目 %s（实际: %v）", first, keysOf(entries))
	}
	if _, ok := entries[second]; !ok {
		t.Fatalf("缺少条目 %s", second)
	}

	var m struct {
		Host       string   `json:"host"`
		Errors     []string `json:"errors"`
		LSATickets []struct {
			File   string `json:"file"`
			Client string `json:"client"`
			Realm  string `json:"realm"`
			Server string `json:"server"`
			End    string `json:"end"`
			Flags  uint32 `json:"flags"`
			Size   int    `json:"size"`
		} `json:"lsa_tickets"`
	}
	if err := json.Unmarshal(entries["manifest.json"], &m); err != nil {
		t.Fatalf("清单不是合法 JSON: %v", err)
	}
	if len(m.LSATickets) != 2 {
		t.Fatalf("清单应含 2 条 LSA 票据，实际 %d", len(m.LSATickets))
	}
	if m.LSATickets[0].Server != "krbtgt/CORP.LOCAL" || m.LSATickets[0].Size != 4 ||
		m.LSATickets[0].End == "" || m.LSATickets[0].Flags != 0x40e10000 {
		t.Fatalf("清单元数据不符: %+v", m.LSATickets[0])
	}
	if len(m.Errors) != 1 {
		t.Fatalf("导出错误应写进清单: %+v", m.Errors)
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
