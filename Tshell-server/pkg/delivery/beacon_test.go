package delivery

import (
	"path/filepath"
	"strings"
	"testing"

	"BackendTemplate/pkg/database"
)

// TestResolveBeaconSleepPrecedence 监听器 > 画像 > 全局 > 硬默认。
func TestResolveBeaconSleepPrecedence(t *testing.T) {
	cases := []struct {
		name       string
		override   string
		listener   int
		profileMs  int
		global     int
		want       int
	}{
		{"显式覆盖最优先", "7", 30, 10000, 5, 7},
		{"监听器覆盖画像", "", 30, 10000, 5, 30},
		{"监听器未配置用画像", "", -1, 10000, 5, 10},
		{"画像未配置用全局", "", -1, 0, 7, 7},
		{"全空用硬默认", "", -1, 0, 0, 5},
		{"非法覆盖值被忽略", "0", 30, 10000, 5, 30},
		{"画像毫秒向下取整且至少 1s", "", -1, 500, 5, 1},
		{"越界监听器值被忽略", "", 999, 10000, 5, 10},
	}
	for _, c := range cases {
		if got := resolveBeaconSleep(c.override, c.listener, c.profileMs, c.global); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

// TestResolveBeaconJitterPrecedence 显式 0% 是合法配置，不能被当成"未配置"。
func TestResolveBeaconJitterPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		override string
		listener int
		global   int
		want     int
	}{
		{"监听器显式 0 必须保留", "", 0, 20, 0},
		{"监听器未配置用全局", "", -1, 20, 20},
		{"显式覆盖最优先", "35", 10, 20, 35},
		{"全局越界回硬默认", "", -1, 99, 20},
		{"非法覆盖值被忽略", "51", 10, 20, 10},
	}
	for _, c := range cases {
		if got := resolveBeaconJitter(c.override, c.listener, c.global); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

// fakeBeaconTemplate 构造含 beacon 占位符的最小二进制（等长替换可用）。
func fakeBeaconTemplate() []byte {
	placeholders := []string{
		"LISTENER_MASTER_KEY_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"LISTENER_SALT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"HEARTBEAT_INTERVAL_PLACEHOLDER_12345",
		"HEARTBEAT_TIMEOUT_PLACEHOLDER_12345",
		"JITTER_PERCENT_PLACEHOLDER_12345",
		"SESSION_KEY_INFO_PLACEHOLDER_12345",
		"PASSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"HOSTAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	var bin []byte
	for _, ph := range placeholders {
		bin = append(bin, []byte(ph)...)
	}
	return append(bin, []byte(strings.Repeat("Y", 512))...)
}

// TestPatchBeaconFromProfile jitter 必须**无条件**写入（否则客户端
// VerifyNoPlaceholderResidue 会 fail-closed 拒绝启动）。
func TestPatchBeaconFromProfile(t *testing.T) {
	out, err := PatchClientBinary(fakeBeaconTemplate(), PatchOptions{
		ListenerConfig: "tcp://1.2.3.4:4444",
		OS:             "windows",
		Arch:           "amd64",
		Pass:           "",
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		HeartbeatMs:    "10000", // 画像 10s
		// 不传 JitterPercent：走全局/硬默认链
	})
	if err != nil {
		t.Fatalf("patch err: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "JITTER_PERCENT_PLACEHOLDER_12345") {
		t.Fatal("jitter 占位符残留：客户端会 fail-closed 拒绝启动（WebDelivery/注入/迁移路径回归）")
	}
	if strings.Contains(s, "HEARTBEAT_INTERVAL_PLACEHOLDER_12345") {
		t.Fatal("heartbeat 占位符残留")
	}
	// 画像 10000ms → 10s（padRight 到占位符长度，前后都是空格）
	if !strings.Contains(s, "10"+strings.Repeat(" ", len("HEARTBEAT_INTERVAL_PLACEHOLDER_12345")-2)) {
		t.Fatalf("画像 sleep 未写入，输出中找不到 10s 填充值")
	}
}

// TestPatchBeaconOverride 显式参数优先于画像。
func TestPatchBeaconOverride(t *testing.T) {
	out, err := PatchClientBinary(fakeBeaconTemplate(), PatchOptions{
		ListenerConfig: "tcp://1.2.3.4:4444",
		OS:             "windows",
		Arch:           "amd64",
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		HeartbeatMs:    "10000",
		SleepSeconds:   "25",
		JitterPercent:  "0",
	})
	if err != nil {
		t.Fatalf("patch err: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "25"+strings.Repeat(" ", len("HEARTBEAT_INTERVAL_PLACEHOLDER_12345")-2)) {
		t.Fatal("显式 sleep=25 未覆盖画像值")
	}
	if !strings.Contains(s, "0"+strings.Repeat(" ", len("JITTER_PERCENT_PLACEHOLDER_12345")-1)) {
		t.Fatal("显式 jitter=0 未写入")
	}
}
// TestPatchBeaconFromListenerDB 生成管线确实读取监听器级配置（不只是纯函数单测）。
func TestPatchBeaconFromListenerDB(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "delivery-beacon.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() {
		if database.Engine != nil {
			database.Engine.Close()
		}
	})

	sleep, jitter := 30, 5
	if _, err := database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:19101", ConnectAddress: "1.2.3.4:19101",
		Status: 1, BeaconSleepSeconds: &sleep, BeaconJitterPercent: &jitter,
	}); err != nil {
		t.Fatalf("插入监听器失败: %v", err)
	}

	out, err := PatchClientBinary(fakeBeaconTemplate(), PatchOptions{
		ListenerConfig: "tcp://1.2.3.4:19101",
		OS:             "windows",
		Arch:           "amd64",
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		HeartbeatMs:    "10000", // 画像 10s 应被监听器 30s 覆盖
	})
	if err != nil {
		t.Fatalf("patch err: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "30"+strings.Repeat(" ", len("HEARTBEAT_INTERVAL_PLACEHOLDER_12345")-2)) {
		t.Fatal("监听器 sleep=30 未写入载荷")
	}
	if !strings.Contains(s, "5"+strings.Repeat(" ", len("JITTER_PERCENT_PLACEHOLDER_12345")-1)) {
		t.Fatal("监听器 jitter=5 未写入载荷")
	}
}
// TestPatchSessionKeyInfo 会话密钥盐上下文必须被写入：显式值优先，未显式传时
// 由 delivery 回落全局配置/内置默认（WebDelivery/注入/迁移原先可能留下默认值，
// 与运维改过的 security.session_key_info 不一致 → 载荷会话密钥永远对不上）。
func TestPatchSessionKeyInfo(t *testing.T) {
	phLen := len("SESSION_KEY_INFO_PLACEHOLDER_12345")

	// 显式值优先
	out, err := PatchClientBinary(fakeBeaconTemplate(), PatchOptions{
		ListenerConfig: "tcp://1.2.3.4:4444",
		OS:             "windows",
		Arch:           "amd64",
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		SessionKeyInfo: "customctx01",
	})
	if err != nil {
		t.Fatalf("patch err: %v", err)
	}
	want := "customctx01" + strings.Repeat(" ", phLen-len("customctx01"))
	if !strings.Contains(string(out), want) {
		t.Fatal("显式 SessionKeyInfo 未写入载荷")
	}

	// 未显式传：也必须替换为默认值（不能残留占位符）
	out, err = PatchClientBinary(fakeBeaconTemplate(), PatchOptions{
		ListenerConfig: "tcp://1.2.3.4:4444",
		OS:             "windows",
		Arch:           "amd64",
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("patch err: %v", err)
	}
	if strings.Contains(string(out), "SESSION_KEY_INFO_PLACEHOLDER_12345") {
		t.Fatal("SessionKeyInfo 占位符残留")
	}
}
