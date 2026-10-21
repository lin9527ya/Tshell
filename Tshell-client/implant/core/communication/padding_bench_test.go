package communication

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"testing"
)

// buildPaddingLegacy 改造前的实现：每次 crypto/rand 生成 n 字节 + base64 编码。
func buildPaddingLegacy(n int) string {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		for i := range raw {
			raw[i] = 'A'
		}
		return base64.StdEncoding.EncodeToString(raw)[:n]
	}
	out := base64.StdEncoding.EncodeToString(raw)
	if len(out) >= n {
		return out[:n]
	}
	return out + base64.StdEncoding.EncodeToString(make([]byte, n-len(out)))[:n-len(out)]
}

func BenchmarkBuildPadding(b *testing.B) {
	for _, n := range []int{40, 212, 4096} {
		n := n
		b.Run(fmt.Sprintf("window/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = BuildPadding(n)
			}
		})
		b.Run(fmt.Sprintf("legacy/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = buildPaddingLegacy(n)
			}
		})
	}
}
