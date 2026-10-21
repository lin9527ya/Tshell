package app

import (
	"implant/core/config"
	"testing"
	"time"
)

// TestRecvDeadlineIsFinite 读超时必须落在"2×最坏拍 + 5s"量级。
//
// 原实现把 time.Duration 又乘了一次 time.Second，sleep=5s/jitter=20% 时读超时
// 约 380 年 —— 半开连接永远等不到超时，"服务端黑洞死亡 → 快速重连"形同虚设。
func TestRecvDeadlineIsFinite(t *testing.T) {
	old := config.WaitTime
	config.WaitTime = 5 * time.Second
	defer func() { config.WaitTime = old }()

	got := RecvDeadline()
	// 5s × 1.2 × 2 + 5s = 17s（JitterPercent 未 patch 时默认 20%）
	if got < 15*time.Second || got > 20*time.Second {
		t.Fatalf("RecvDeadline=%v，期望约 17s（单位错误时会是 380 年级别）", got)
	}
}

func TestRecvDeadlineScalesWithSleep(t *testing.T) {
	old := config.WaitTime
	defer func() { config.WaitTime = old }()

	config.WaitTime = 60 * time.Second
	got := RecvDeadline()
	// 60s × 1.2 × 2 + 5s = 149s
	if got < 140*time.Second || got > 160*time.Second {
		t.Fatalf("sleep=60s 时 RecvDeadline=%v，期望约 149s", got)
	}
}
