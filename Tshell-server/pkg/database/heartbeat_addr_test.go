package database

import (
	"path/filepath"
	"testing"
)

// TestGetListenerHeartbeatConfigByAddr 多实例同类型时按地址定位心跳配置。
// 按类型查只能取到任意一行，入站处理与客户端生成可能用到另一个监听器的配置。
func TestGetListenerHeartbeatConfigByAddr(t *testing.T) {
	tmpDir := t.TempDir()
	old := DatabaseName
	DatabaseName = filepath.Join(tmpDir, "test_tshell.db")
	ConnectDateBase()
	DatabaseName = old
	defer Engine.Close()

	Engine.Insert(&Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:8081", ConnectAddress: "a.example.com:8081",
		Status: 1, HeartbeatTimeout: "11", HeartbeatInterval: "22",
	})
	Engine.Insert(&Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:8082", ConnectAddress: "b.example.com:8082",
		Status: 1, HeartbeatTimeout: "33", HeartbeatInterval: "44",
	})

	cases := []struct {
		name, addr            string
		wantTimeout, wantIntv string
	}{
		{"connect_address 精确", "b.example.com:8082", "33", "44"},
		{"listen_address 精确", "0.0.0.0:8081", "11", "22"},
		// 连接的本端地址是具体 IP，监听地址绑的是通配地址：只能按端口兜底
		{"本端具体 IP 按端口兜底", "192.168.1.5:8082", "33", "44"},
	}
	for _, c := range cases {
		gotT, gotI := GetListenerHeartbeatConfigByAddr(c.addr, "tcp")
		if gotT != c.wantTimeout || gotI != c.wantIntv {
			t.Errorf("%s: 得到 %s/%s，期望 %s/%s", c.name, gotT, gotI, c.wantTimeout, c.wantIntv)
		}
	}

	// 定位不到 / 空地址 → 回退按类型（返回有效值，不报错）
	for _, addr := range []string{"no-such-host:9999", ""} {
		gotT, gotI := GetListenerHeartbeatConfigByAddr(addr, "tcp")
		if gotT == "" || gotI == "" {
			t.Errorf("addr=%q 回退按类型应返回有效值，实际 %q/%q", addr, gotT, gotI)
		}
	}
}
