package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/database"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestBuildExecFrame 只有能力客户端且 taskID>0 时才加 EXD1 头。
func TestBuildExecFrame(t *testing.T) {
	body := []byte("BODY")

	legacy := buildExecFrame(command.ExecuteBof, body, 42, false)
	if len(legacy) != 4+len(body) {
		t.Fatalf("非能力客户端不应有额外头: %v", legacy)
	}
	if got := binary.BigEndian.Uint32(legacy[:4]); got != command.ExecuteBof {
		t.Fatalf("cmdType 错误: %d", got)
	}

	capable := buildExecFrame(command.ExecuteBof, body, 42, true)
	if len(capable) != 4+8+len(body) {
		t.Fatalf("能力客户端应带 8 字节 EXD1 头: %v", capable)
	}
	if string(capable[4:8]) != "EXD1" {
		t.Fatalf("缺少 EXD1 魔数: %q", capable[4:8])
	}
	if got := binary.BigEndian.Uint32(capable[8:12]); got != 42 {
		t.Fatalf("taskID 错误: %d", got)
	}

	// taskID=0 不加头
	zero := buildExecFrame(command.ExecuteBof, body, 0, true)
	if len(zero) != 4+len(body) {
		t.Fatalf("taskID=0 不应加头: %v", zero)
	}
}

// TestExecuteBinOSMatrix 平台/模式错配在服务端 400；允许的组合会继续走到下发（无连接 → 503）。
func TestExecuteBinOSMatrix(t *testing.T) {
	setupExecuteBinTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/bin/execute", ExecuteBin)

	type tc struct {
		uid, os, arch, mode string
		want                int
	}
	cases := []tc{
		{"win-1", "windows", "x64", "execute-assembly", 503},
		{"win-1", "windows", "x64", "bof", 503},
		{"win-1", "windows", "x64", "script", 400},
		{"lin-1", "linux", "x64", "script", 503},
		{"lin-1", "linux", "x64", "binary", 503},
		{"lin-1", "linux", "x64", "execute-assembly", 400},
		{"mac-1", "darwin", "x64", "exec-file", 400},
		{"win86-1", "windows", "x86", "execute-assembly", 400},
		{"win86-1", "windows", "x86", "bof", 400},
		{"win86-1", "windows", "x86", "exec-file", 503},
	}
	for _, uid := range []string{"win-1", "win86-1", "lin-1", "mac-1"} {
		database.Engine.Where("uid = ?", uid).Delete(&database.Clients{})
	}
	rows := []database.Clients{
		{Uid: "win-1", Os: "windows", Arch: "x64", Online: "1", Listener: "tcp"},
		{Uid: "win86-1", Os: "windows", Arch: "x86", Online: "1", Listener: "tcp"},
		{Uid: "lin-1", Os: "linux", Arch: "x64", Online: "1", Listener: "tcp"},
		{Uid: "mac-1", Os: "darwin", Arch: "x64", Online: "1", Listener: "tcp"},
	}
	for i := range rows {
		if _, err := database.Engine.Insert(&rows[i]); err != nil {
			t.Fatalf("插入客户端失败: %v", err)
		}
	}

	for _, c := range cases {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, executeBinRequest(t, c.uid, c.mode, ""))
		if w.Code != c.want {
			t.Errorf("uid=%s os=%s arch=%s mode=%s: got %d want %d body=%s",
				c.uid, c.os, c.arch, c.mode, w.Code, c.want, w.Body.String())
		}
	}
}

// TestGetOnlineClientsIncludesOsArch 前端平台 gating 依赖这两个字段。
func TestGetOnlineClientsIncludesOsArch(t *testing.T) {
	setupExecuteBinTest(t)
	database.Engine.Where("uid = ?", "online-os-1").Delete(&database.Clients{})
	if _, err := database.Engine.Insert(&database.Clients{
		Uid: "online-os-1", Computer: "prod-01", Os: "linux", Arch: "x64", Online: "1", Listener: "tcp",
	}); err != nil {
		t.Fatalf("插入客户端失败: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/client/online-clients", GetOnlineClients)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/client/online-clients", nil))

	var resp struct {
		Status int `json:"status"`
		Data   []struct {
			UID  string `json:"uid"`
			OS   string `json:"os"`
			Arch string `json:"arch"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应解析失败: %v body=%s", err, w.Body.String())
	}
	if resp.Status != 200 {
		t.Fatalf("status=%d body=%s", resp.Status, w.Body.String())
	}
	found := false
	for _, c := range resp.Data {
		if c.UID == "online-os-1" {
			found = true
			if c.OS != "linux" || c.Arch != "x64" {
				t.Fatalf("os/arch 字段错误: %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("在线客户端未返回: %s", w.Body.String())
	}
}
