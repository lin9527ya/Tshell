package encrypt

import (
	"testing"
	"time"
)

func benchKeyManager() *SecureKeyManager {
	skm := &SecureKeyManager{overlapDuration: time.Minute}
	skm.sessionKeys.Store("bench", &SessionKeySet{
		CurrentKey: &SessionKey{
			KeyID:     "k1",
			Version:   1,
			CreatedAt: time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(2 * time.Hour),
		},
	})
	return skm
}

// BenchmarkGetKeyStatus 心跳路径改造前的调用方式（多 map + 字符串格式化）。
func BenchmarkGetKeyStatus(b *testing.B) {
	skm := benchKeyManager()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		status := skm.GetKeyStatus("bench")
		if data, ok := status["current_key"].(map[string]interface{}); ok {
			_ = data["time_to_expiry"].(string)
		}
	}
}

// BenchmarkTimeToExpiry 心跳路径改造后：只读一个时间字段。
func BenchmarkTimeToExpiry(b *testing.B) {
	skm := benchKeyManager()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = skm.TimeToExpiry("bench")
	}
}
