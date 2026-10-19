package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ===== 初始管理员密码的落盘方式 =====
//
// 背景（OPSEC P0-3）：弱口令时服务端会生成随机密码，原实现把它**直接打印到 stdout**：
//
//	log.Printf("...已生成随机密码：username=%s, password=%s（请立即登录并修改）", ...)
//
// 而 stdout 会被 systemd/journald、Docker 日志驱动、终端回滚、CI 输出、截图
// 原样留存 —— 等于把一个可登录的管理员凭据写进了一堆运维从来没打算当密钥保管的地方。
// 生成随机密码本来就是为了避免弱口令，结果它落到了比 config.yaml 更松的地方。
//
// 现在的做法：
//  1. 密码写入 0600 的 initial-admin-password.txt（同 DB 目录），日志只打印**路径**；
//  2. 需要人工看密码时显式加 -print-initial-password（交互式首装场景）；
//  3. 文件内容含"首次登录后请删除"的提示，避免长期留在服务器上。

// InitialPasswordFileName 初始密码文件名（固定名便于运维找到并删除）
const InitialPasswordFileName = "initial-admin-password.txt"

// InitOptions 默认管理员初始化选项
type InitOptions struct {
	// PrintPasswordToStdout 显式把生成的密码打印到 stdout。
	// 默认 false —— 只有确认当前输出不会被留存时才该打开。
	PrintPasswordToStdout bool
	// PasswordFileDir 密码文件目录；为空时用当前工作目录。
	// 调用方通常传 DB 文件所在目录，便于和 database.db 一起做权限收敛。
	PasswordFileDir string
}

// writeInitialPasswordFile 把账号信息写入 0600 文件并返回路径。
//
// 写入失败时**不阻断启动**（否则运维会为了启动而改回打印日志），
// 但要把失败原因返回给调用方，由调用方在日志里显式告警。
func writeInitialPasswordFile(opts InitOptions, username, password string) (string, error) {
	dir := opts.PasswordFileDir
	if dir == "" {
		dir = "."
	}
	path := filepath.Join(dir, InitialPasswordFileName)

	content := fmt.Sprintf(
		"Tshell 初始管理员账号（首次登录后请立即修改密码并删除本文件）\n"+
			"生成时间: %s\n"+
			"用户名: %s\n"+
			"密码: %s\n\n"+
			"为什么写在文件里而不是日志里：\n"+
			"  stdout 会被 journald / 容器日志驱动 / 终端回滚长期留存，\n"+
			"  把可登录凭据留在那里等于扩大泄露面。\n",
		time.Now().Format(time.RFC3339), username, password)

	// 先用 0600 创建再写入：不要先写后 chmod（中间有一段窗口是默认权限）
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return path, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return path, err
	}
	if err := f.Close(); err != nil {
		return path, err
	}
	// 已存在文件的权限不会被 O_CREATE 改变，这里显式收敛一次
	if err := os.Chmod(path, 0o600); err != nil {
		return path, err
	}
	return path, nil
}
