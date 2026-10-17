package encrypt

import "testing"

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
