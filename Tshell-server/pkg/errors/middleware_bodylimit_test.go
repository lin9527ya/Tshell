package errors

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// readBodyHandler 读取请求体并把读取错误交给测试
func readBodyHandler(got *error, n *int) gin.HandlerFunc {
	return func(c *gin.Context) {
		data, err := io.ReadAll(c.Request.Body)
		*got = err
		*n = len(data)
		c.Status(http.StatusOK)
	}
}

// TestMaxBodyBytesPerPath 默认上限与按路径覆盖
func TestMaxBodyBytesPerPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const mb = 1 << 20
	body := bytes.Repeat([]byte("A"), 3*mb)

	cases := []struct {
		name       string
		path       string
		pathLimits map[string]int64
		wantErr    bool
	}{
		{"默认上限生效", "/api/other", map[string]int64{"/api/upload": 50 * mb}, true},
		{"覆盖路径放宽", "/api/upload", map[string]int64{"/api/upload": 50 * mb}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var readErr error
			var n int
			r := gin.New()
			r.Use(MaxBodyBytesMiddlewareWithLimits(1*mb, tc.pathLimits))
			r.POST(tc.path, readBodyHandler(&readErr, &n))

			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(body))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if tc.wantErr {
				var maxErr *http.MaxBytesError
				if !asMaxBytesError(readErr, &maxErr) {
					t.Fatalf("3MB 请求体在 1MB 上限下未被拦截，err=%v 读到 %d 字节", readErr, n)
				}
			} else if readErr != nil {
				t.Fatalf("覆盖路径应放行 3MB，实际 err=%v", readErr)
			} else if n != len(body) {
				t.Fatalf("覆盖路径读取不完整: %d/%d", n, len(body))
			}
		})
	}
}

// TestMaxBodyBytesCoversChunked 分块传输（ContentLength = -1）也必须受限。
// 原实现只在 `c.Request.ContentLength > 0` 时包裹，chunked 请求完全不受限——
// 全局 DoS 防护被整体绕过。
func TestMaxBodyBytesCoversChunked(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const mb = 1 << 20
	body := bytes.Repeat([]byte("A"), 3*mb)

	var readErr error
	var n int
	r := gin.New()
	r.Use(MaxBodyBytesMiddleware(1 * mb))
	r.POST("/api/chunked", readBodyHandler(&readErr, &n))

	req := httptest.NewRequest(http.MethodPost, "/api/chunked", bytes.NewReader(body))
	req.ContentLength = -1 // 模拟 chunked
	req.TransferEncoding = []string{"chunked"}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var maxErr *http.MaxBytesError
	if !asMaxBytesError(readErr, &maxErr) {
		t.Fatalf("chunked 请求体未被上限拦截，err=%v 读到 %d 字节", readErr, n)
	}
}

// asMaxBytesError 判断错误链里是否含 *http.MaxBytesError
func asMaxBytesError(err error, target **http.MaxBytesError) bool {
	for err != nil {
		if e, ok := err.(*http.MaxBytesError); ok {
			*target = e
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}
