//go:build !linux

package app

// daemonizeIfTTY 非 Linux 平台无前台终端语义，空实现。
func daemonizeIfTTY() {}
