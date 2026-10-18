package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestUpdateListenerGuardsRunningAddressAndType 运行中不允许改监听地址/协议类型。
// 所有句柄都按启动时的地址与类型登记，改了 DB 而句柄仍在旧值下，按新值关闭/删除
// 会找不到句柄：端口继续监听、系统显示已停止，且之后再也关不掉（僵尸监听器）。
func TestUpdateListenerGuardsRunningAddressAndType(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	const (
		addr    = "0.0.0.0:18089"
		newAddr = "0.0.0.0:18088"
	)
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: addr, ConnectAddress: "c2.example.com:18089",
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt, Note: "old",
	})
	var lis database.Listener
	if has, _ := database.Engine.Where("listen_address = ?", addr).Get(&lis); !has {
		t.Fatal("测试前置：监听器未插入成功")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/update", UpdateListener)
	post := func(body string) (int, string) {
		_, st, msg := postJSON(t, router, "/listener/update", body)
		return st, msg
	}

	// 1) 运行中改地址 → 拒绝
	if st, msg := post(`{"id":` + fmt.Sprintf("%d", lis.Id) + `,"listenAddress":"` + newAddr + `","type":"tcp"}`); st != 400 {
		t.Errorf("运行中改地址应被拒，实际 status=%d msg=%s", st, msg)
	}
	// 2) 运行中改类型 → 拒绝
	if st, msg := post(`{"id":` + fmt.Sprintf("%d", lis.Id) + `,"listenAddress":"` + addr + `","type":"kcp"}`); st != 400 {
		t.Errorf("运行中改类型应被拒，实际 status=%d msg=%s", st, msg)
	}
	// 两次被拒都不应落库
	var unchanged database.Listener
	database.Engine.ID(lis.Id).Get(&unchanged)
	if unchanged.ListenAddress != addr || unchanged.Type != "tcp" || unchanged.Note != "old" {
		t.Errorf("被拒的更新不应落库: addr=%q type=%q note=%q", unchanged.ListenAddress, unchanged.Type, unchanged.Note)
	}

	// 3) 运行中只改其它字段 → 允许（避免过度拦截）
	if st, msg := post(`{"id":` + fmt.Sprintf("%d", lis.Id) + `,"listenAddress":"` + addr + `","type":"tcp","note":"edited"}`); st != 200 {
		t.Errorf("运行中改备注应允许，实际 status=%d msg=%s", st, msg)
	}
	var edited database.Listener
	database.Engine.ID(lis.Id).Get(&edited)
	if edited.Note != "edited" {
		t.Errorf("备注应已更新，实际 %q", edited.Note)
	}

	// 4) 停止后改地址 → 允许
	database.Engine.ID(lis.Id).Update(&database.Listener{Status: 2})
	if st, msg := post(`{"id":` + fmt.Sprintf("%d", lis.Id) + `,"listenAddress":"` + newAddr + `","type":"tcp"}`); st != 200 {
		t.Errorf("停止后改地址应允许，实际 status=%d msg=%s", st, msg)
	}
	var moved database.Listener
	database.Engine.ID(lis.Id).Get(&moved)
	if moved.ListenAddress != newAddr {
		t.Errorf("地址应已更新为 %q，实际 %q", newAddr, moved.ListenAddress)
	}
}

// TestListenerForClientPrefersPerClientAddress 迁移/注入取监听器记录时，优先用该客户端
// 实际连接的监听器，而不是按类型取最新一条——否则同类型多实例时会拿错地址与密钥。
func TestListenerForClientPrefersPerClientAddress(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:18085", ConnectAddress: "a.example.com:18085",
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:18084", ConnectAddress: "b.example.com:18084",
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	// 客户端实际连的是 A（id 较小，按类型 Desc(id) 会取到 B）
	connection.MuClientListenerType.Lock()
	connection.ClientListenerAddr["uid-on-a"] = "a.example.com:18085"
	connection.MuClientListenerType.Unlock()
	defer func() {
		connection.MuClientListenerType.Lock()
		delete(connection.ClientListenerAddr, "uid-on-a")
		connection.MuClientListenerType.Unlock()
	}()

	lis, has := listenerForClient("uid-on-a", "tcp")
	if !has || lis.ListenAddress != "0.0.0.0:18085" {
		t.Errorf("应按客户端实际连接的监听器定位，实际 %q (has=%v)", lis.ListenAddress, has)
	}

	// 无逐客户端地址 → 按类型兜底（取最新一条）
	lis, has = listenerForClient("uid-unknown", "tcp")
	if !has || lis.ListenAddress != "0.0.0.0:18084" {
		t.Errorf("无逐客户端地址时应按类型兜底取最新一条，实际 %q (has=%v)", lis.ListenAddress, has)
	}
}

// TestUpdateListenerKeyChangeHint 运行中改密钥时，响应应提示需要重启监听器——
// 运行中的监听器用启动时捕获的密钥，改库不会影响它；新生成的客户端会连不上。
func TestUpdateListenerKeyChangeHint(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	const addr = "0.0.0.0:18083"
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: addr, ConnectAddress: "c2.example.com:18083",
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})
	var lis database.Listener
	database.Engine.Where("listen_address = ?", addr).Get(&lis)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/update", UpdateListener)
	body := `{"id":` + fmt.Sprintf("%d", lis.Id) + `,"listenAddress":"` + addr + `","type":"tcp","vkey":"NewVKeyForRotation0123456789abcde"}`
	_, st, msg := postJSON(t, router, "/listener/update", body)
	if st != 200 {
		t.Fatalf("改密钥应允许，实际 status=%d msg=%s", st, msg)
	}
	if !strings.Contains(msg, "重启监听器") {
		t.Errorf("响应应提示需要重启监听器，实际 %q", msg)
	}
}
