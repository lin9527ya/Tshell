package database

import (
	"encoding/json"
	"testing"
)

// TestDownloadsJSONContract 下载记录经 /client/report 直接序列化：
// 前端按 snake_case 读取（file_name / downloaded_size 等），
// 结构体缺 tag 时 Go 会输出 PascalCase，报告里的文件名与大小会全空。
func TestDownloadsJSONContract(t *testing.T) {
	row := Downloads{
		Uid:            "uid-1",
		FileName:       "a.txt",
		FilePath:       "/tmp/a.txt",
		FileSize:       10,
		DownloadedSize: 5,
		CreatedAt:      "2026-01-01 00:00:00",
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	for _, key := range []string{"uid", "file_name", "file_path", "file_size", "downloaded_size", "created_at"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("下载记录缺少 snake_case 键 %q，实际: %v", key, got)
		}
	}
}
