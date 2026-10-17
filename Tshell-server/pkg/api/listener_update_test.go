package api

import (
	"BackendTemplate/pkg/database"
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestUpdateListenerGuardsRunningAddressAndType 运行中不允许改监听地址/协议类型。
// 所有句柄都按启动时的地址与类型登记，改了 DB 而句柄仍在旧值下，按新值关闭/删除
// 会找不到句柄：端口继续监听、系统显示已停止，且之后再也关不掉（僵尸监听器）。
func TestUpdateListenerGuardsRunningAddressAndType(t *testing.T) {
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
