package audit

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestExtractFileOperationInfoRealRoutes 中间件实际挂载的路由必须都能识别出操作与目标路径。
// 修复前：前缀表只认 /api/upload、/api/file 等，而挂载的全是 /api/client/file/*，
// 加上字段名找的是 path/file/filename（实际是 filePath/dirPath/oldPath/uploadPath），
// 于是一条文件操作审计都没产生过。
func TestExtractFileOperationInfoRealRoutes(t *testing.T) {
	cases := []struct {
		name          string
		uri           string
		body          string
		wantOperation string
		wantFile      string
	}{
		{"浏览目录", "/api/client/file/tree", `{"uid":"u1","dirPath":"C:\\Users"}`, "BROWSE", `C:\Users`},
		{"递归搜索", "/api/client/file/search", `{"uid":"u1","dirPath":"/etc","keyword":"passwd"}`, "SEARCH", "/etc"},
		{"列盘符", "/api/client/file/drives", `{"uid":"u1"}`, "LIST_DRIVES", ""},
		{"读文件", "/api/client/file/edit/read", `{"uid":"u1","filePath":"/etc/shadow"}`, "READ", "/etc/shadow"},
		{"写文件", "/api/client/file/edit/write", `{"uid":"u1","filePath":"/tmp/x","content":"hi"}`, "WRITE", "/tmp/x"},
		{"新建文件", "/api/client/file/create", `{"uid":"u1","filePath":"/tmp/new.txt"}`, "CREATE_FILE", "/tmp/new.txt"},
		{"新建目录", "/api/client/file/mkdir", `{"uid":"u1","dirPath":"/tmp/d"}`, "CREATE_DIR", "/tmp/d"},
		{"删除文件", "/api/client/file/delete", `{"uid":"u1","filePath":"/tmp/a"}`, "DELETE", "/tmp/a"},
		{"重命名记新旧路径", "/api/client/file/rename", `{"uid":"u1","oldPath":"/tmp/a","newPath":"/tmp/b"}`, "MOVE", "/tmp/a → /tmp/b"},
		{"复制记新旧路径", "/api/client/file/copy", `{"uid":"u1","oldPath":"/tmp/a","newPath":"/tmp/b"}`, "COPY", "/tmp/a → /tmp/b"},
		{"改时间", "/api/client/file/modifytime", `{"uid":"u1","filePath":"/tmp/a","newTime":"2024-01-01T00:00:00"}`, "MODIFY_TIME", "/tmp/a"},
		{"隐藏", "/api/client/file/hide", `{"uid":"u1","filePath":"/tmp/a"}`, "HIDE", "/tmp/a"},
		{"取消隐藏", "/api/client/file/unhide", `{"uid":"u1","filePath":"/tmp/a"}`, "UNHIDE", "/tmp/a"},
		{"下发到客户端", "/api/client/file/upload", `{"uid":"u1","uploadPath":"C:\\temp"}`, "UPLOAD", `C:\temp`},
		{"取回文件", "/api/client/file/download", `{"uid":"u1","filePath":"/etc/passwd"}`, "DOWNLOAD", "/etc/passwd"},
		{"下载已回传文件", "/api/client/downloads/downloaded_file", `{"uid":"u1","filePath":"/tmp/loot"}`, "DOWNLOAD", "/tmp/loot"},
		{"删除已回传文件", "/api/client/downloads/delete", `{"uid":"u1","filePath":"/tmp/loot"}`, "DELETE", "/tmp/loot"},
		{"列出已回传", "/api/client/downloads/info", `{"uid":"u1"}`, "LIST_DOWNLOADS", ""},
		// 带查询串
		{"带查询串", "/api/client/file/delete?x=1", `{"uid":"u1","filePath":"/tmp/a"}`, "DELETE", "/tmp/a"},
		// 不相关路由不产生文件操作审计
		{"非文件路由", "/api/dashboard/stats", `{}`, "", ""},
		{"文件路由下的未知子路径", "/api/client/file/unknown", `{"filePath":"/tmp/a"}`, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op, file := extractFileOperationInfo(tc.uri, []byte(tc.body))
			if op != tc.wantOperation || file != tc.wantFile {
				t.Fatalf("extractFileOperationInfo(%q) = (%q, %q)，期望 (%q, %q)",
					tc.uri, op, file, tc.wantOperation, tc.wantFile)
			}
		})
	}
}

// TestExtractFileOperationInfoLargeBody 大请求体只预读前缀，JSON 解析必然失败，
// 仍要能把路径抠出来（上传的文件名在 multipart 头里、写文件的路径在 JSON 前部）
func TestExtractFileOperationInfoLargeBody(t *testing.T) {
	t.Run("截断的 JSON（写文件带大内容）", func(t *testing.T) {
		body := []byte(`{"uid":"u1","filePath":"/tmp/big.bin","content":"` + strings.Repeat("A", 200*1024))
		op, file := extractFileOperationInfo("/api/client/file/edit/write", body)
		if op != "WRITE" || file != "/tmp/big.bin" {
			t.Fatalf("截断 JSON 未提取到路径: (%q, %q)", op, file)
		}
	})

	t.Run("multipart 上传（文件名在 part 头）", func(t *testing.T) {
		var buf bytes.Buffer
		buf.WriteString("--BOUNDARY\r\n")
		buf.WriteString(`Content-Disposition: form-data; name="file"; filename="payload.exe"` + "\r\n")
		buf.WriteString("Content-Type: application/octet-stream\r\n\r\n")
		buf.Write(bytes.Repeat([]byte("M"), 1024))
		buf.WriteString("\r\n--BOUNDARY\r\n")
		buf.WriteString(`Content-Disposition: form-data; name="uploadPath"` + "\r\n\r\n")
		buf.WriteString("C:\\temp\r\n--BOUNDARY--\r\n")

		op, file := extractFileOperationInfo("/api/client/file/upload", buf.Bytes())
		if op != "UPLOAD" || file != "payload.exe" {
			t.Fatalf("multipart 未提取到文件名: (%q, %q)", op, file)
		}
	})
}

// TestFileOperationMiddlewarePreservesBody 中间件必须把请求体原样交给 handler。
// 这里是最容易出错的地方：大请求体只预读前缀后要用 MultiReader 拼回去，
// 拼错就会破坏所有文件上传（handler 拿到残缺的 multipart）。
func TestFileOperationMiddlewarePreservesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name    string
		payload []byte
		chunked bool
	}{
		{"小请求体（整包预读）", []byte(`{"uid":"u1","filePath":"/tmp/a"}`), false},
		{"大请求体（只预读前缀）", bytes.Repeat([]byte("X"), 512*1024), false},
		{"分块传输（ContentLength=-1）", bytes.Repeat([]byte("Y"), 300*1024), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []byte
			var readErr error
			r := gin.New()
			r.Use(FileOperationAuditMiddleware())
			r.POST("/api/client/file/upload", func(c *gin.Context) {
				got, readErr = io.ReadAll(c.Request.Body)
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodPost, "/api/client/file/upload", bytes.NewReader(tc.payload))
			if tc.chunked {
				req.ContentLength = -1
				req.TransferEncoding = []string{"chunked"}
			}
			r.ServeHTTP(httptest.NewRecorder(), req)

			if readErr != nil {
				t.Fatalf("handler 读取请求体失败: %v", readErr)
			}
			if !bytes.Equal(got, tc.payload) {
				t.Fatalf("请求体被破坏: 收到 %d 字节，期望 %d 字节", len(got), len(tc.payload))
			}
		})
	}
}

// TestCoversFileOperation 路由覆盖判定（route_policy_test 依赖它）
func TestCoversFileOperation(t *testing.T) {
	covered := []string{
		"/api/client/file/tree", "/api/client/file/delete", "/api/client/file/upload",
		"/api/client/downloads/delete", "/api/plugin/upload", "/api/plugin/download/x.exe",
	}
	for _, p := range covered {
		if !CoversFileOperation(p) {
			t.Errorf("%s 应被文件操作审计覆盖", p)
		}
	}
	for _, p := range []string{"/api/dashboard/stats", "/api/client/shell/sendcommand", "/api/plugin/list"} {
		if CoversFileOperation(p) {
			t.Errorf("%s 不应被文件操作审计覆盖", p)
		}
	}
}

// TestReadFileAuditBodyIsBounded 预读必须有上限：放宽上传上限后，无上限 ReadAll
// 会让每个大文件上传先整包驻留内存（500MB 请求体就是 500MB 额外内存）
func TestReadFileAuditBodyIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name        string
		size        int
		chunked     bool
		wantMaxRead int
	}{
		{"小请求体整包读", 1 << 10, false, 1 << 10},
		{"大请求体只读前缀", 1 << 20, false, fileAuditPeekBytes},
		{"分块传输只读前缀", 1 << 20, true, fileAuditPeekBytes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []byte
			r := gin.New()
			r.POST("/api/client/file/upload", func(c *gin.Context) {
				got = readFileAuditBody(c)
				c.Status(http.StatusOK)
			})

			payload := bytes.Repeat([]byte("Z"), tc.size)
			req := httptest.NewRequest(http.MethodPost, "/api/client/file/upload", bytes.NewReader(payload))
			if tc.chunked {
				req.ContentLength = -1
				req.TransferEncoding = []string{"chunked"}
			}
			r.ServeHTTP(httptest.NewRecorder(), req)

			if len(got) > tc.wantMaxRead {
				t.Fatalf("预读 %d 字节，超过上限 %d", len(got), tc.wantMaxRead)
			}
			if len(got) == 0 {
				t.Fatal("预读为空（路径提取会失效）")
			}
		})
	}
}
