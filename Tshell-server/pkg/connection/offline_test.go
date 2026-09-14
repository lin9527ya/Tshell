package connection

import (
	"testing"
	"time"
)

func TestOfflineDetectorRegisterTouchUnregister(t *testing.T) {
	d := &OfflineDetector{clients: make(map[string]*ClientActivity)}

	// Register
	d.Register("uid1", "tcp", 5)
	if !d.IsRegistered("uid1") {
		t.Error("uid1 should be registered")
	}
	lastActive, listener, ok := d.GetLastActive("uid1")
	if !ok {
		t.Error("GetLastActive should return ok=true")
	}
	if listener != "tcp" {
		t.Errorf("GetLastActive listener = %q, want tcp", listener)
	}

	// Touch 更新 LastActive
	time.Sleep(2 * time.Millisecond)
	d.Touch("uid1")
	newActive, _, _ := d.GetLastActive("uid1")
	if !newActive.After(lastActive) {
		t.Error("Touch should update LastActive")
	}

	// Unregister
	d.Unregister("uid1")
	if d.IsRegistered("uid1") {
		t.Error("uid1 should be unregistered")
	}
}

func TestOfflineDetectorCleanupCallback(t *testing.T) {
	d := &OfflineDetector{clients: make(map[string]*ClientActivity)}

	cleaned := false
	d.Register("uid2", "dns", 5, func() { cleaned = true })

	d.mu.Lock()
	if d.clients["uid2"].Cleanup == nil {
		d.mu.Unlock()
		t.Fatal("cleanup callback should be stored")
	}
	d.clients["uid2"].Cleanup()
	d.mu.Unlock()

	if !cleaned {
		t.Error("cleanup callback not called")
	}
}


// 回归：高心跳间隔客户端（如监听器配置 30s 心跳）不应在 30s 内被误判离线。
// 此前各协议注册时传"命令休眠"值（硬编码 5 / client.Sleep=1），阈值仅 20~22.5s，
// 而客户端实际心跳为监听器 HeartbeatInterval（可 30s/60s）→ 必然误判。
func TestOfflineDetectorHighHeartbeatNotMisjudged(t *testing.T) {
	d := &OfflineDetector{clients: make(map[string]*ClientActivity)}

	// 模拟监听器心跳间隔 30s 的注册（修复后各协议注册传 heartbeatInterval）
	d.Register("uid30", "websocket", 30)

	// 心跳按 30s 节奏到来（等价于每 30s 一次 Touch），阈值应为心跳间隔的数倍
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Millisecond)
		d.Touch("uid30")
	}

	// 阈值：quickThreshold=(30+15)*3=135s。29s 无活动远低于阈值，不应判离线
	d.clients["uid30"].LastActive = time.Now().Add(-29 * time.Second)
	d.checkAll()
	if !d.IsRegistered("uid30") {
		t.Fatal("30s 心跳客户端 29s 无活动不应判离线（修复前 22.5s 阈值会误判）")
	}

	// 真正离线（远超阈值）应判离线
	d.clients["uid30"].LastActive = time.Now().Add(-200 * time.Second)
	d.checkAll()
	if d.IsRegistered("uid30") {
		t.Fatal("200s 无活动应判离线")
	}
}
