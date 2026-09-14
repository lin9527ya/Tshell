package app

import (
	"implant/core/debuglog"
	"os"
	"strings"
)

// GetListenerKeyConfig 返回监听器密钥配置的固定长度占位符。
// 占位符在客户端生成时由服务端二进制 patch 管线（pkg/api/server.go / pkg/delivery/payload.go）
// 替换为真实盐值/主密钥。各 adapter 在 FirstBlood 后调用本函数取占位符，
// 再用前 16 字节作盐、前 32 字节作主密钥设置会话密钥。
func GetListenerKeyConfig(listenerType string) ([]byte, []byte) {
	salt := make([]byte, 48)
	masterKey := make([]byte, 48)
	copy(salt, []byte("LISTENER_SALT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	copy(masterKey, []byte("LISTENER_MASTER_KEY_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))

	// K5: 占位符未替换检测（fail-closed）——二进制未被服务端 patch 管线替换时
	// 全客户端共享公开密钥，流量可解密、会话可被劫持；直接拒绝启动。
	// 服务端生成时同时校验（server.go 已拒绝默认密钥），此处双保险。
	// 注意：masterKey 缓冲 48 字节，52 字符字面量经 copy 截断为 20 前缀 + 28 个 'A'；
	// salt 缓冲 48 字节，48 字符字面量完整拷入（14 前缀 + 34 个 'A'）。
	// 检测常量必须与缓冲中"未 patch"的实际字节逐字节一致，否则比较恒 false 沦为死代码。
	masterPlaceholder := "LISTENER_MASTER_KEY_" + strings.Repeat("A", 28) // 20+28 = 48 字节
	saltPlaceholder := "LISTENER_SALT_" + strings.Repeat("A", 34)        // 14+34 = 48 字节
	if string(masterKey) == masterPlaceholder || string(salt) == saltPlaceholder {
		debuglog.Printf("🚫 [%s-CLIENT] 监听器密钥占位符未被替换，拒绝启动（客户端未配置自定义密钥）\n", strings.ToUpper(listenerType))
		os.Exit(1)
	}

	debuglog.Printf("🔑 [%s-CLIENT] 使用监听器特定的密钥配置\n", strings.ToUpper(listenerType))
	return salt, masterKey
}
