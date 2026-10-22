package database

import "testing"

// TestMigrateLegacySocks5ToTunnels 旧表数据并入 tunnel_proxy：
// 端口映射、凭据/备注/状态保留、同端口去重、可重复执行。
func TestMigrateLegacySocks5ToTunnels(t *testing.T) {
	setupMigrationTest(t)

	// 新库不再创建 socks5 表（Sync2 已移除），这里按旧库结构手工建表
	if _, err := Engine.Exec(`CREATE TABLE socks5 (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uid TEXT,
		type TEXT,
		connect_address TEXT,
		socks5port TEXT,
		user_name TEXT,
		password TEXT,
		note TEXT,
		status INTEGER,
		created_at DATETIME,
		updated_at DATETIME
	)`); err != nil {
		t.Fatalf("建旧表失败: %v", err)
	}

	// 新表已有一条同端口记录：迁移时必须跳过，不能撞唯一约束
	if _, err := Engine.Insert(&TunnelProxy{ClientUID: "u-new", Protocol: "socks5", ServerPort: 7000, Status: 0}); err != nil {
		t.Fatal(err)
	}

	legacy := []struct {
		uid, typ, port, user, pass, note string
		status                           int
	}{
		{"u-old-1", "socks5", "7000", "op1", "p1", "重复端口", 1},
		{"u-old-2", "socks5", "7001", "op2", "p2", "需迁移", 1},
		{"u-old-3", "", "7002", "", "", "停止状态", 2},
	}
	for _, r := range legacy {
		if _, err := ExecSQL(
			`INSERT INTO socks5 (uid, type, connect_address, socks5port, user_name, password, note, status, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.uid, r.typ, ":"+r.port, r.port, r.user, r.pass, r.note, r.status,
			"2026-01-02 03:04:05", "2026-01-02 03:04:05",
		); err != nil {
			t.Fatalf("插入旧记录失败: %v", err)
		}
	}

	if err := migrateLegacySocks5ToTunnels(); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 7001：字段完整迁移
	var got TunnelProxy
	has, err := Engine.Where("server_port = ?", 7001).Get(&got)
	if err != nil || !has {
		t.Fatalf("7001 未迁移: has=%v err=%v", has, err)
	}
	if got.ClientUID != "u-old-2" || got.Protocol != "socks5" ||
		got.Username != "op2" || got.Password != "p2" || got.Note != "需迁移" || got.Status != 1 {
		t.Fatalf("7001 迁移字段不对: %+v", got)
	}

	// 7002：type 为空按 socks5 处理；状态 2（停止）映射为 0
	var stopped TunnelProxy
	if has, _ := Engine.Where("server_port = ?", 7002).Get(&stopped); !has {
		t.Fatal("7002 未迁移")
	}
	if stopped.Protocol != "socks5" || stopped.Status != 0 {
		t.Fatalf("7002 迁移结果不对: %+v", stopped)
	}

	// 7000：保留新表原记录，不产生副本
	n, err := Engine.Where("server_port = ?", 7000).Count(&TunnelProxy{})
	if err != nil || n != 1 {
		t.Fatalf("7000 应只有 1 条，实际 %d（err=%v）", n, err)
	}
	var existing TunnelProxy
	Engine.Where("server_port = ?", 7000).Get(&existing)
	if existing.ClientUID != "u-new" {
		t.Fatalf("同端口应保留新表记录，实际 %+v", existing)
	}

	// 幂等：再跑一次不新增
	if err := migrateLegacySocks5ToTunnels(); err != nil {
		t.Fatalf("第二次迁移失败: %v", err)
	}
	total, _ := Engine.Count(&TunnelProxy{})
	if total != 3 {
		t.Fatalf("重复迁移产生了副本，总数 %d（期望 3）", total)
	}
}

// TestMigrateLegacySocks5SkipsWithoutTable 没有旧表（新部署）时直接跳过。
func TestMigrateLegacySocks5SkipsWithoutTable(t *testing.T) {
	setupMigrationTest(t)
	if err := migrateLegacySocks5ToTunnels(); err != nil {
		t.Fatalf("无旧表时应静默跳过: %v", err)
	}
}
