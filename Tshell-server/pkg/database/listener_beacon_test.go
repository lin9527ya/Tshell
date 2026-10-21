package database

import (
	"path/filepath"
	"testing"
)

// setupListenerBeaconTest 建一个临时库（与 heartbeat_addr_test 相同的隔离方式）
func setupListenerBeaconTest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	old := DatabaseName
	DatabaseName = filepath.Join(tmpDir, "listener_beacon_test.db")
	ConnectDateBase()
	DatabaseName = old
	t.Cleanup(func() {
		if Engine != nil {
			Engine.Close()
		}
	})
}

func intPtr(v int) *int { return &v }

// TestGetListenerBeaconConfigByAddr 校验按地址定位与"监听器级值不丢失"的语义。
func TestGetListenerBeaconConfigByAddr(t *testing.T) {
	setupListenerBeaconTest(t)

	if _, err := Engine.Insert(&Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:19081", ConnectAddress: "a.example.com:19081",
		Status: 1, HeartbeatTimeout: "11", HeartbeatInterval: "22",
		BeaconSleepSeconds: intPtr(30), BeaconJitterPercent: intPtr(0), Profile: "alt",
	}); err != nil {
		t.Fatalf("插入监听器失败: %v", err)
	}
	if _, err := Engine.Insert(&Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:19082", ConnectAddress: "b.example.com:19082",
		Status: 1, BeaconSleepSeconds: intPtr(-1), BeaconJitterPercent: intPtr(-1),
	}); err != nil {
		t.Fatalf("插入监听器失败: %v", err)
	}

	cases := []struct {
		name        string
		addr        string
		wantSleep   int
		wantJitter  int
		wantProfile string
	}{
		{"connect_address 精确", "a.example.com:19081", 30, 0, "alt"},
		{"listen_address 精确", "0.0.0.0:19081", 30, 0, "alt"},
		{"本端具体 IP 按端口兜底", "192.168.1.5:19081", 30, 0, "alt"},
		{"显式 -1 视为未配置", "b.example.com:19082", -1, -1, ""},
		{"查不到地址按类型兜底", "no-such-host:9999", 30, 0, "alt"}, // 类型兜底取到任意一条活跃记录
	}
	for _, c := range cases {
		got := GetListenerBeaconConfigByAddr(c.addr, "tcp")
		if got.SleepSeconds != c.wantSleep || got.JitterPercent != c.wantJitter || got.Profile != c.wantProfile {
			t.Errorf("%s: 得到 sleep=%d jitter=%d profile=%q，期望 %d/%d/%q",
				c.name, got.SleepSeconds, got.JitterPercent, got.Profile, c.wantSleep, c.wantJitter, c.wantProfile)
		}
	}

	// 空地址 → 按类型兜底；没有活跃记录时全部继承
	if got := GetListenerBeaconConfigByAddr("", "kcp"); got.SleepSeconds != -1 || got.JitterPercent != -1 {
		t.Errorf("无监听器时应返回 -1/-1，实际 %d/%d", got.SleepSeconds, got.JitterPercent)
	}
}

// TestGetListenerBeaconConfigOutOfRange 库里被写了越界值时按未配置处理，
// 避免把非法节奏生成进载荷（客户端会静默钳制，UI 与真实值分裂）。
func TestGetListenerBeaconConfigOutOfRange(t *testing.T) {
	setupListenerBeaconTest(t)

	// 不填指针 = 未配置，读回必须是 nil
	if _, err := Engine.Insert(&Listener{
		Type: "kcp", ListenAddress: "0.0.0.0:19084", ConnectAddress: "n.example.com:19084", Status: 1,
	}); err != nil {
		t.Fatalf("插入监听器失败: %v", err)
	}
	var nilRow Listener
	if has, _ := Engine.Where("connect_address = ?", "n.example.com:19084").Get(&nilRow); !has {
		t.Fatal("监听器未读回")
	}
	if nilRow.BeaconSleepSeconds != nil || nilRow.BeaconJitterPercent != nil {
		t.Fatalf("未配置应为 nil: sleep=%+v jitter=%+v", nilRow.BeaconSleepSeconds, nilRow.BeaconJitterPercent)
	}

	if _, err := Engine.Insert(&Listener{
		Type: "kcp", ListenAddress: "0.0.0.0:19085", ConnectAddress: "c.example.com:19085",
		Status: 1, BeaconSleepSeconds: intPtr(999), BeaconJitterPercent: intPtr(99),
	}); err != nil {
		t.Fatalf("插入监听器失败: %v", err)
	}
	got := GetListenerBeaconConfigByAddr("c.example.com:19085", "kcp")
	if got.SleepSeconds != -1 || got.JitterPercent != -1 {
		t.Errorf("越界值应视为未配置 (-1/-1)，实际 %d/%d", got.SleepSeconds, got.JitterPercent)
	}
}

// TestListenerBeaconColumnsMigration 迁移函数幂等，且显式 0 能原样读回。
func TestListenerBeaconColumnsMigration(t *testing.T) {
	setupListenerBeaconTest(t)

	// 新库由 Sync2 建列；迁移函数再跑两遍必须无错（幂等）
	if err := migrateListenerBeaconColumns(); err != nil {
		t.Fatalf("第一次迁移失败: %v", err)
	}
	if err := migrateListenerBeaconColumns(); err != nil {
		t.Fatalf("第二次迁移失败: %v", err)
	}
	cols := tableColumns("listener")
	if !cols["beacon_sleep_seconds"] || !cols["beacon_jitter_percent"] {
		t.Fatalf("迁移后缺少 beacon 列: %+v", cols)
	}

	if _, err := Engine.Insert(&Listener{
		Type: "http", ListenAddress: "0.0.0.0:19086", ConnectAddress: "d.example.com:19086",
		Status: 2, BeaconSleepSeconds: intPtr(15), BeaconJitterPercent: intPtr(0),
	}); err != nil {
		t.Fatalf("插入监听器失败: %v", err)
	}
	var l Listener
	if has, _ := Engine.Where("connect_address = ?", "d.example.com:19086").Get(&l); !has {
		t.Fatal("监听器未读回")
	}
	if l.BeaconSleepSeconds == nil || *l.BeaconSleepSeconds != 15 {
		t.Errorf("sleep 指针未正确落库: %+v", l.BeaconSleepSeconds)
	}
	if l.BeaconJitterPercent == nil || *l.BeaconJitterPercent != 0 {
		t.Errorf("显式 0%% jitter 必须原样保留: %+v", l.BeaconJitterPercent)
	}
}
