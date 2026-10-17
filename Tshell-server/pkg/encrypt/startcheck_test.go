package encrypt

import (
	"strings"
	"testing"
)

// TestValidateListenerKeyConfigForStart 覆盖启动/入库前的密钥闸门：
// 空密钥与公开默认值/占位符都必须拒绝——它们可预测，会导致流量可解密、会话可被劫持。
func TestValidateListenerKeyConfigForStart(t *testing.T) {
	const (
		validVKey = "validVKey1234567890abcdefghijklmno"
		validSalt = "validSalt12345678"
		// 与 IsDefaultListenerKey 中的公开默认常量一致
		defaultVKey = "initial_master_key_12345678901"
		defaultSalt = "initial_salt_123"
		// 客户端二进制里的未替换占位符
		placeholderVKey = "LISTENER_MASTER_KEY_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		placeholderSalt = "LISTENER_SALT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	)

	tests := []struct {
		name    string
		vkey    string
		salt    string
		wantErr bool
	}{
		{"VKey为空", "", validSalt, true},
		{"加密盐为空", validVKey, "", true},
		{"两者都为空", "", "", true},
		{"VKey为公开默认值", defaultVKey, validSalt, true},
		{"加密盐为公开默认值", validVKey, defaultSalt, true},
		{"VKey为客户端占位符", placeholderVKey, validSalt, true},
		{"加密盐为客户端占位符", validVKey, placeholderSalt, true},
		{"自定义密钥", validVKey, validSalt, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateListenerKeyConfigForStart(tt.vkey, tt.salt)
			if tt.wantErr && err == nil {
				t.Fatalf("期望拒绝，但通过了校验")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("期望通过，但被拒绝: %v", err)
			}
		})
	}
}

// TestValidateListenerKeyConfigAcceptsSystemGeneratedFormats 回归：校验策略必须接受系统
// 自己生成的密钥格式。原实现限制 VKey ≤ 32 字节，而后端 GenerateListenerKeyConfig 产出
// base64(32B)=44 字符、前端「随机」按钮产出 hex(32B)=64 字符——两者都被误杀，表现为
// 「把监听器列表里的 VKey 复制到密钥管理页面被拒」「只改加密盐却报 VKey 长度超限」
// （UpdateListenerKeyConfig 会先合并旧值再校验，旧 VKey 是系统生成的 44 字符）。
func TestValidateListenerKeyConfigAcceptsSystemGeneratedFormats(t *testing.T) {
	vk, sl := GenerateListenerKeyConfig()
	if err := ValidateListenerKeyConfig(vk, sl); err != nil {
		t.Errorf("后端生成的密钥应通过校验: VKey=%d 字符 Salt=%d 字符, err=%v", len(vk), len(sl), err)
	}

	// 前端 EditListenerDialog「随机」按钮格式：hex(32B)=64 字符 / hex(16B)=32 字符
	hexVKey := strings.Repeat("ab", 32)
	hexSalt := strings.Repeat("cd", 16)
	if err := ValidateListenerKeyConfig(hexVKey, hexSalt); err != nil {
		t.Errorf("前端生成的密钥应通过校验: err=%v", err)
	}

	// 恰好等于派生所需长度（32/16）也应通过
	if err := ValidateListenerKeyConfig(strings.Repeat("a", 32), strings.Repeat("b", 16)); err != nil {
		t.Errorf("恰好 32/16 字节应通过校验: err=%v", err)
	}
}

// TestValidateListenerKeyLength 长度档：短于派生所需长度会被拒（否则派生密钥尾部是零填充，
// 有效强度低于字面长度）；空串表示「不修改」，跳过对应项——更新接口留空即沿用旧值，
// 否则编辑既有短密钥监听器的备注都会因旧密钥不合规而被拒。
func TestValidateListenerKeyLength(t *testing.T) {
	long := strings.Repeat("a", 32)
	salt := strings.Repeat("b", 16)

	tests := []struct {
		name    string
		vkey    string
		salt    string
		wantErr bool
	}{
		{"VKey 过短", strings.Repeat("a", 31), salt, true},
		{"加密盐过短", long, strings.Repeat("b", 15), true},
		{"恰好达标", long, salt, false},
		{"VKey 超长", strings.Repeat("a", 129), salt, true},
		{"两项留空_视为不修改", "", "", false},
		{"仅 VKey 留空_跳过该项", "", salt, false},
		{"仅加密盐留空_跳过该项", long, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateListenerKeyLength(tt.vkey, tt.salt)
			if tt.wantErr && err == nil {
				t.Fatal("期望拒绝，但通过了校验")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("期望通过，但被拒绝: %v", err)
			}
		})
	}
}
