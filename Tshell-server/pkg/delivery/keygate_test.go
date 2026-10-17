package delivery

import (
	"strings"
	"testing"
)

// TestPatchClientBinaryRejectsDefaultOrEmptyKeys 生成闸门：PatchClientBinary 是 WebDelivery /
// 内存注入 / 会话迁移三条路径的共同出口，密钥解析落到公开默认值或为空时必须拒绝生成。
// 客户端自身的 fail-closed 只认「占位符未被替换」，认不出「被替换成了默认值」，
// 于是载荷会带着公开密钥上线——流量可解密、会话可伪造。（/server/gen 自带此闸门，
// 另外三条原先都没有。）
func TestPatchClientBinaryRejectsDefaultOrEmptyKeys(t *testing.T) {
	base := func() PatchOptions {
		return PatchOptions{
			ListenerConfig: "tcp://1.2.3.4:4444",
			OS:             "windows",
			Arch:           "amd64",
			Format:         "exe",
		}
	}

	// 未提供密钥（无 DB 记录可查）→ 拒绝
	if _, err := PatchClientBinary(fakeTemplate(), base()); err == nil {
		t.Error("未提供密钥时应拒绝生成")
	} else if !strings.Contains(err.Error(), "拒绝生成") {
		t.Errorf("错误信息应说明是拒绝生成: %v", err)
	}

	// 显式传公开默认密钥 → 拒绝
	opt := base()
	opt.VKey = "initial_master_key_12345678901"
	opt.EncryptSalt = "initial_salt_123"
	if _, err := PatchClientBinary(fakeTemplate(), opt); err == nil {
		t.Error("显式传公开默认密钥时应拒绝生成")
	}

	// 客户端未替换的占位符 → 拒绝
	opt = base()
	opt.VKey = "LISTENER_MASTER_KEY_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	opt.EncryptSalt = "LISTENER_SALT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if _, err := PatchClientBinary(fakeTemplate(), opt); err == nil {
		t.Error("传客户端占位符时应拒绝生成")
	}

	// 对照组：有效自定义密钥 → 通过
	opt = base()
	opt.VKey = "0123456789abcdef0123456789abcdef"
	opt.EncryptSalt = "0123456789abcdef"
	if _, err := PatchClientBinary(fakeTemplate(), opt); err != nil {
		t.Errorf("有效自定义密钥应通过: %v", err)
	}
}
