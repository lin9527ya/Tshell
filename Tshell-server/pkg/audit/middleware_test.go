package audit

import "testing"

// TestIsSensitiveAPI 插件仓库涉及可执行载荷的上传/下载/删除，必须进敏感 API 详细审计。
func TestIsSensitiveAPI(t *testing.T) {
	cases := []struct {
		uri  string
		want bool
	}{
		{"/api/plugin/list", true},
		{"/api/plugin/upload", true},
		{"/api/plugin/download/fscan.exe", true},
		{"/api/plugin/delete/fscan.exe", true},
		{"/api/login", true},
		{"/api/dashboard/stats", false},
	}
	for _, tc := range cases {
		if got := isSensitiveAPI(tc.uri); got != tc.want {
			t.Errorf("isSensitiveAPI(%q) = %v，期望 %v", tc.uri, got, tc.want)
		}
	}
}

// TestExtractFileOperationInfoPlugin 插件下载/删除的文件名在 URI 末段（不在请求体里），
// 上传的在 multipart 表单里；列表类接口不产生文件操作审计。
func TestExtractFileOperationInfoPlugin(t *testing.T) {
	cases := []struct {
		name          string
		uri           string
		body          []byte
		wantOperation string
		wantFile      string
	}{
		{"上传-无文件名（multipart 不在 body 里）", "/api/plugin/upload", nil, "UPLOAD", ""},
		{"上传-表单字段兜底", "/api/plugin/upload", []byte(`{"filename":"payload.exe"}`), "UPLOAD", "payload.exe"},
		{"下载-URI 末段", "/api/plugin/download/fscan.exe", nil, "DOWNLOAD", "fscan.exe"},
		{"下载-带查询串", "/api/plugin/download/fscan.exe?t=1", nil, "DOWNLOAD", "fscan.exe"},
		{"下载-URL 转义名", "/api/plugin/delete/a%20b.dll", nil, "DELETE", "a b.dll"},
		{"删除", "/api/plugin/delete/god.bin", nil, "DELETE", "god.bin"},
		{"列表不记文件操作", "/api/plugin/list", nil, "", ""},
		{"详情不记文件操作", "/api/plugin/details/fscan.exe", nil, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op, file := extractFileOperationInfo(tc.uri, tc.body)
			if op != tc.wantOperation || file != tc.wantFile {
				t.Fatalf("extractFileOperationInfo(%q) = (%q, %q)，期望 (%q, %q)",
					tc.uri, op, file, tc.wantOperation, tc.wantFile)
			}
		})
	}
}
