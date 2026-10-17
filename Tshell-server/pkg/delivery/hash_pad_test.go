package delivery

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
)

// fakeTemplate 构造包含全部必需占位符的假模板（与 patch_opsec_test.go 同源）
func fakeTemplate() []byte {
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
	return bin
}

// TestHashPadChangesHash 同一模板 + 相同配置两次生成，hash 必须不同（Hash 轮换）
func TestHashPadChangesHash(t *testing.T) {
	tmpl := fakeTemplate()
	opt := PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "websocket://127.0.0.1:6666",
		HashPad:        true,
		Format:         "exe",
	}
	a, err := PatchClientBinary(tmpl, opt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PatchClientBinary(tmpl, opt)
	if err != nil {
		t.Fatal(err)
	}
	ha := sha256.Sum256(a)
	hb := sha256.Sum256(b)
	if bytes.Equal(ha[:], hb[:]) {
		t.Fatal("hash 轮换失败: 两次生成 hash 相同")
	}
	if len(a) <= len(tmpl) || len(b) <= len(tmpl) {
		t.Fatal("pad 未追加")
	}
}

// TestHashPadOffSameHash 关闭 pad 时 hash 一致（幂等）
func TestHashPadOffSameHash(t *testing.T) {
	tmpl := fakeTemplate()
	opt := PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://127.0.0.1:2222",
		HashPad:        false,
		Format:         "exe",
	}
	a, err := PatchClientBinary(tmpl, opt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PatchClientBinary(tmpl, opt)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("hash_pad=false 时应幂等")
	}
}

// TestHashPadSkippedForShellcode shellcode 格式不追加 pad（长度敏感）
func TestHashPadSkippedForShellcode(t *testing.T) {
	tmpl := fakeTemplate()
	opt := PatchOptions{
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		ListenerConfig: "tcp://127.0.0.1:2222",
		HashPad:        true,
		Format:         "bin",
	}
	a, err := PatchClientBinary(tmpl, opt)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PatchClientBinary(tmpl, opt)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("shellcode 格式不应追加 pad")
	}
}
