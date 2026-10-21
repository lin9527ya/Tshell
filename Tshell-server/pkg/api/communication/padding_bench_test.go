package communication

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"BackendTemplate/pkg/c2profile"
)

// BenchmarkResponsePadFor 每次 HTTP 轮询响应都会调用（默认画像 padding=开）。
func BenchmarkResponsePadFor(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ResponsePadFor(300)
	}
}

// BenchmarkWrapResponsePad 轮询响应整形：含一次 json.Marshal 用于量体积 + pad 生成。
func BenchmarkWrapResponsePad(b *testing.B) {
	dir := b.TempDir()
	yaml := `name: default
sleeptime_ms: 5000
http:
  get:
    uri: /x
    padding: true
`
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), []byte(yaml), 0o644); err != nil {
		b.Fatalf("写画像失败: %v", err)
	}
	if err := c2profile.InitNoWatch(dir); err != nil {
		b.Fatalf("初始化画像失败: %v", err)
	}
	resp := map[string]interface{}{
		"status": 200,
		"data": map[string]interface{}{
			"meta": "0123456789abcdef",
			"cmd":  "QUJDREVGR0hJSktMTU5PUA==",
		},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = wrapResponsePad(resp)
	}
}

// BenchmarkEncodeVsMarshal 对比轮询响应的两种写法（Encoder 每次分配缓冲并多写换行）。
func BenchmarkEncodeVsMarshal(b *testing.B) {
	resp := map[string]interface{}{"status": 200, "data": map[string]interface{}{"cmd": "QUJD"}}
	b.Run("Encoder", func(b *testing.B) {
		b.ReportAllocs()
		var buf bytes.Buffer
		for i := 0; i < b.N; i++ {
			buf.Reset()
			_ = json.NewEncoder(&buf).Encode(resp)
		}
	})
	b.Run("Marshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = json.Marshal(resp)
		}
	})
}
