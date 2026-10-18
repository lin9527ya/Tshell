package api

import (
	"BackendTemplate/pkg/database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func postJSON(t *testing.T, router *gin.Engine, path, body string) (*httptest.ResponseRecorder, int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var resp struct {
		Status int    `json:"status"`
		Data   string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		// 双写会把两个 JSON 对象拼在一起，这里直接失败并暴露原始 body
		t.Fatalf("%s 响应不是单个 JSON（疑似双写）: %v body=%s", path, err, w.Body.String())
	}
	return w, resp.Status, resp.Data
}

// TestAddListenerDedupIsExact 查重必须精确匹配。原实现用 LIKE "%"+地址：
// 它匹配的是"以该地址结尾"，所以 ":18093" 会被已存在的 "0.0.0.0:18093" 挡下
// （地址里的 _ / % 还会被当通配符，例如 "dns_1.example.com:53" 会匹配到
// "dnsX1.example.com:53"）。而其余接口一律按 listen_address 精确查库，判重口径必须一致。
func TestAddListenerDedupIsExact(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	// 已存在一条（不启动，避免占用端口）
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:18093", ConnectAddress: "c2.example.com:18093",
		Status: 2, VKey: testVKey, EncryptionSalt: testSalt,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/add", AddListener)
	add := func(addr, connect string) (int, string) {
		body := `{"type":"tcp","listenAddress":"` + addr + `","connectAddress":"` + connect + `","vkey":"` + testVKey + `","encryptionSalt":"` + testSalt + `"}`
		_, st, msg := postJSON(t, router, "/listener/add", body)
		return st, msg
	}

	// 1) 完全相同的地址 → 判为重复（对照：精确匹配下仍然生效）
	if st, msg := add("0.0.0.0:18093", "c2.example.com:18093"); st != 400 {
		t.Errorf("完全相同的地址应判为重复，实际 status=%d msg=%s", st, msg)
	}

	// 2) 是既有地址的后缀 → 不应判为重复（旧 LIKE 会误判）
	st, msg := add(":18093", ":18093")
	if st != 200 {
		t.Fatalf("后缀相同的地址不应判为重复，实际 status=%d msg=%s", st, msg)
	}
	defer func() {
		// 该监听器已真的启动（:18093 可绑定），测试结束前关掉，别把句柄留给同包其他测试
		if err := handleClosePort("tcp", ":18093"); err != nil {
			t.Logf("清理 :18093 监听器: %v", err)
		}
	}()
	var count int64
	count, _ = database.Engine.Where("listen_address = ?", ":18093").Count(&database.Listener{})
	if count != 1 {
		t.Errorf("后缀不同的地址应已入库，实际 %d 条", count)
	}
}

// TestCloseListenerMissingHandleSingleResponse 句柄缺失时只能有一次响应。
// 原实现里 handleClosePort 先写 400 "Listener not found"，调用方紧接着写 200，
// 构成双写（gin 报 headers already written，客户端看到的是先写进去的那份 400，
// 与调用方以为返回的结果不一致）。
// 语义上：状态为运行但句柄已丢（启动失败/异常退出残留）时关闭应算成功——
// 调用方已把状态置为已停止，"没有句柄可关"不构成失败。
func TestCloseListenerMissingHandleSingleResponse(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	const addr = "0.0.0.0:18091"
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: addr, ConnectAddress: "c2.example.com:18091",
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})
	// 刻意不注册任何句柄：模拟"状态为运行但句柄已丢"

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/close", CloseListener)

	_, st, msg := postJSON(t, router, "/listener/close", `{"listenAddress":"`+addr+`"}`)
	if st != 200 || msg != "Listener closed" {
		t.Errorf("应为调用方的成功响应，实际 status=%d data=%q", st, msg)
	}

	var got database.Listener
	database.Engine.Where("listen_address = ?", addr).Get(&got)
	if got.Status != 2 {
		t.Errorf("状态应置为已停止，实际 %d", got.Status)
	}
}

// TestDeleteListenerMissingHandleSingleResponse 删除路径同理：句柄缺失时也只回一次成功响应，
// 且删除照常完成。
func TestDeleteListenerMissingHandleSingleResponse(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	const addr = "0.0.0.0:18090"
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: addr, ConnectAddress: "c2.example.com:18090",
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/delete", DeleteListener)

	_, st, msg := postJSON(t, router, "/listener/delete", `{"listenAddress":"`+addr+`"}`)
	if st != 200 || msg != "Listener deleted" {
		t.Errorf("应为调用方的成功响应，实际 status=%d data=%q", st, msg)
	}
	var count int64
	count, _ = database.Engine.Where("listen_address = ?", addr).Count(&database.Listener{})
	if count != 0 {
		t.Errorf("记录应已删除，实际剩 %d 条", count)
	}
}
