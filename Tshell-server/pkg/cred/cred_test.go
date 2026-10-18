package cred

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 凭据链路的服务端部分：请求登记、按文件名精确匹配、过期清理，以及制品校验。
// 这里守的是"归属正确"——回传的文件必须能对上具体某次请求、某个任务、某个操作员。

func newTestRegistry(t *testing.T) (*Registry, *time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	r := NewRegistry()
	r.now = func() time.Time { return now }
	return r, &now
}

func TestRegistryIssueAndMatch(t *testing.T) {
	r, _ := newTestRegistry(t)
	req := ArtifactRequest{UID: "uid-1", Kind: KindLsass, FileName: "lsass-host-1.dmp", TaskID: 7, Operator: "alice"}
	if err := r.Issue(req, time.Hour); err != nil {
		t.Fatalf("登记失败: %v", err)
	}

	// 文件名不匹配 → 不认领（宁可漏认也不错认）
	if _, ok := r.Match("uid-1", "other-file.dmp"); ok {
		t.Fatal("文件名不符时不应匹配")
	}
	// 别的会话回传同名文件也不匹配
	if _, ok := r.Match("uid-2", "lsass-host-1.dmp"); ok {
		t.Fatal("不同 uid 不应匹配")
	}

	got, ok := r.Match("uid-1", "lsass-host-1.dmp")
	if !ok {
		t.Fatal("应匹配到请求")
	}
	if got.TaskID != 7 || got.Operator != "alice" || got.Kind != KindLsass {
		t.Fatalf("匹配到的请求信息不符: %+v", got)
	}
	// 匹配即消费：同一文件不会认领两次
	if _, ok := r.Match("uid-1", "lsass-host-1.dmp"); ok {
		t.Fatal("已匹配的请求不应再次匹配")
	}
	if n := len(r.Pending("")); n != 0 {
		t.Fatalf("匹配后不应有未完成请求: %d", n)
	}
}

func TestRegistryRejectsDuplicateName(t *testing.T) {
	r, _ := newTestRegistry(t)
	req := ArtifactRequest{UID: "uid-1", Kind: KindLsass, FileName: "same.dmp"}
	if err := r.Issue(req, time.Hour); err != nil {
		t.Fatal(err)
	}
	// 同名请求共存会让回传的制品归属到错的任务上，必须拒绝
	if err := r.Issue(req, time.Hour); err == nil {
		t.Fatal("同名未完成请求应被拒绝")
	}
	if err := r.Issue(ArtifactRequest{UID: "uid-1", FileName: ""}, time.Hour); err == nil {
		t.Fatal("空文件名应被拒绝")
	}
	if err := r.Issue(ArtifactRequest{UID: "", FileName: "x.dmp"}, time.Hour); err == nil {
		t.Fatal("空 uid 应被拒绝")
	}
}

func TestRegistryExpiry(t *testing.T) {
	r, now := newTestRegistry(t)
	if err := r.Issue(ArtifactRequest{UID: "uid-1", FileName: "a.dmp"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if n := len(r.Pending("uid-1")); n != 1 {
		t.Fatalf("未完成请求数 = %d", n)
	}

	// 超期后：既不匹配，也不出现在"等待回传"列表里（否则前端永远显示等待中）
	*now = now.Add(2 * time.Minute)
	if _, ok := r.Match("uid-1", "a.dmp"); ok {
		t.Fatal("过期请求不应再匹配")
	}
	if n := len(r.Pending("uid-1")); n != 0 {
		t.Fatalf("过期请求应被清理，实际 %d", n)
	}
	if n := r.Sweep(); n != 0 {
		t.Fatalf("惰性清理已生效，Sweep 不应再有可清理项: %d", n)
	}
}

func TestRegistryCancelAndPending(t *testing.T) {
	r, _ := newTestRegistry(t)
	_ = r.Issue(ArtifactRequest{UID: "uid-1", FileName: "a.dmp", Issued: time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)}, time.Hour)
	_ = r.Issue(ArtifactRequest{UID: "uid-1", FileName: "b.dmp", Issued: time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC)}, time.Hour)
	_ = r.Issue(ArtifactRequest{UID: "uid-2", FileName: "c.dmp"}, time.Hour)

	r.Cancel("uid-1", "a.dmp")
	pend := r.Pending("uid-1")
	if len(pend) != 1 || pend[0].FileName != "b.dmp" {
		t.Fatalf("取消后未完成请求不符: %+v", pend)
	}
	// uid 为空时列出全部，且按发起时间倒序
	all := r.Pending("")
	if len(all) != 2 {
		t.Fatalf("全部未完成请求数 = %d", len(all))
	}
	if all[0].FileName != "c.dmp" && all[0].Issued.Before(all[1].Issued) {
		t.Fatalf("应按发起时间倒序: %+v", all)
	}
}

func TestNewFileNameSanitizesHost(t *testing.T) {
	now := time.Date(2026, 10, 18, 13, 14, 15, 0, time.UTC)
	got := NewFileName(KindLsass, "DC01.corp.local", now)
	if got != "lsass-DC01_corp_local-20261018-131415.dmp" {
		t.Fatalf("文件名不符: %q", got)
	}
	// 路径分隔符等必须被净化（文件名会被拼进目标路径）
	for _, host := range []string{"../../etc", `a\b`, "very-long-hostname-that-exceeds-the-limit-x"} {
		name := NewFileName(KindLsass, host, now)
		if len(name) > 80 {
			t.Errorf("文件名过长: %q", name)
		}
		for _, bad := range []string{"/", "\\", ".."} {
			if containsStr(name, bad) {
				t.Errorf("主机名 %q 未被净化: %q", host, name)
			}
		}
	}
	if got := NewFileName(KindLsass, "", now); got != "lsass-unknown-20261018-131415.dmp" {
		t.Fatalf("空主机名兜底不符: %q", got)
	}
}

func TestKindAndTechValidation(t *testing.T) {
	if !IsKindSupported(KindLsass) || IsKindSupported("nope") {
		t.Fatal("凭据类型校验不符")
	}
	for _, tech := range Techs {
		if !IsTechSupported(tech) {
			t.Errorf("技术 %s 应被支持", tech)
		}
	}
	if IsTechSupported("magic") {
		t.Fatal("未知技术不应被支持")
	}
}

func containsStr(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// ---------- minidump 校验 ----------

// buildMinidump 构造最小可用的 minidump：32 字节头 + 目录 + 各流数据
func buildMinidump(streams []int, opts ...func([]byte)) []byte {
	dirRVA := uint32(headerSize)
	dirLen := len(streams) * directoryEntryLen
	dataRVA := uint32(headerSize + dirLen)

	header := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(header[0:4], minidumpSignature)
	binary.LittleEndian.PutUint16(header[4:6], minidumpVersion)
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(streams)))
	binary.LittleEndian.PutUint32(header[12:16], dirRVA)

	dir := make([]byte, dirLen)
	data := make([]byte, 0, 64)
	for i, st := range streams {
		off := i * directoryEntryLen
		binary.LittleEndian.PutUint32(dir[off:off+4], uint32(st))
		binary.LittleEndian.PutUint32(dir[off+4:off+8], 16) // 每个流 16 字节数据
		binary.LittleEndian.PutUint32(dir[off+8:off+12], dataRVA+uint32(len(data)))
		data = append(data, make([]byte, 16)...)
	}
	out := append(append(header, dir...), data...)
	for _, o := range opts {
		o(out)
	}
	return out
}

func writeDump(t *testing.T, dir string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, "test.dmp")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateMinidumpAcceptsRealDump(t *testing.T) {
	dir := t.TempDir()
	// SystemInfo(7) + Memory64List(9)：MiniDumpWithFullMemory 的典型形态
	path := writeDump(t, dir, buildMinidump([]int{streamSystemInfo, streamMemory64List, streamThreadList}))

	info, err := ValidateMinidump(path)
	if err != nil {
		t.Fatalf("有效转储应通过校验: %v", err)
	}
	if !info.HasMemory || !info.HasSystemInfo || info.Streams != 3 {
		t.Fatalf("校验结果不符: %+v", info)
	}
	if info.Size == 0 || info.Describe() == "" {
		t.Fatalf("缺少描述信息: %+v", info)
	}
	// 只有 ModuleList(4) 没有 SystemInfo 的形态也应接受（部分工具的精简转储）
	path2 := writeDump(t, t.TempDir(), buildMinidump([]int{streamModuleList, streamMemoryList}))
	if _, err := ValidateMinidump(path2); err != nil {
		t.Fatalf("含模块列表的转储应通过校验: %v", err)
	}
}

func TestValidateMinidumpRejectsIncomplete(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
		wantSub string
	}{
		{"空文件", nil, "过小"},
		{"太短", []byte("MDMP"), "过小"},
		{"签名不符", append([]byte("XXXX"), buildMinidump([]int{streamSystemInfo})[4:]...), "签名"},
		{"零个流", buildMinidump(nil), "数据流"},
		{"缺少系统信息流", buildMinidump([]int{streamThreadList}), "完整的进程转储"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeDump(t, t.TempDir(), tc.content)
			_, err := ValidateMinidump(path)
			if err == nil {
				t.Fatal("应校验失败")
			}
			if !containsStr(err.Error(), tc.wantSub) {
				t.Fatalf("错误信息 %q 未包含 %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// TestValidateMinidumpRejectsTruncated 截断的文件必须被拒：
// 客户端在转储写一半时被中断（进程被杀/磁盘满）是很常见的失败形态
func TestValidateMinidumpRejectsTruncated(t *testing.T) {
	full := buildMinidump([]int{streamSystemInfo, streamMemory64List})
	// 砍掉尾部数据
	path := writeDump(t, t.TempDir(), full[:len(full)-8])
	if _, err := ValidateMinidump(path); err == nil {
		t.Fatal("截断的转储应校验失败")
	}
	// 目录本身越界
	path2 := writeDump(t, t.TempDir(), full[:headerSize+4])
	if _, err := ValidateMinidump(path2); err == nil {
		t.Fatal("目录越界的转储应校验失败")
	}
	// 文件不存在
	if _, err := ValidateMinidump(filepath.Join(t.TempDir(), "nope.dmp")); err == nil {
		t.Fatal("不存在的文件应报错")
	}
}

// TestValidateMinidumpRejectsHugeStreamCount 畸形头（声称几万个流）不能拖垮服务端
func TestValidateMinidumpRejectsHugeStreamCount(t *testing.T) {
	content := buildMinidump([]int{streamSystemInfo})
	binary.LittleEndian.PutUint32(content[8:12], 0xFFFFFF)
	path := writeDump(t, t.TempDir(), content)
	if _, err := ValidateMinidump(path); err == nil {
		t.Fatal("异常目录条数应被拒绝")
	}
}
