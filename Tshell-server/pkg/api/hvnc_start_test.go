package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type hvncStartResp struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    struct {
		SID string `json:"sid"`
	} `json:"data"`
}

func postHvncStart(t *testing.T, router *gin.Engine, uid string) (int, hvncStartResp) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"uid": uid})
	req := httptest.NewRequest(http.MethodPost, "/client/hvnc/start", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var got hvncStartResp
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	return w.Code, got
}

// TestStartHvncGuards HVNC 只支持 Windows 且客户端必须在线：
// 原实现无条件返回 sid，前端会显示"启动成功"却永远没有画面。
func TestStartHvncGuards(t *testing.T) {
	setupMaskTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/client/hvnc/start", StartHvnc)

	// 1) 不存在
	if code, resp := postHvncStart(t, router, "no-such"); code != 400 || !strings.Contains(resp.Message, "会话不存在") {
		t.Fatalf("不存在会话应 400: code=%d msg=%s", code, resp.Message)
	}

	// 2) 非 Windows
	if _, err := database.DB().Insert(&database.Clients{Uid: "lin-1", Os: "linux", Listener: "tcp", Arch: "x64"}); err != nil {
		t.Fatalf("插入客户端失败: %v", err)
	}
	if code, resp := postHvncStart(t, router, "lin-1"); code != 400 || !strings.Contains(resp.Message, "仅支持 Windows") {
		t.Fatalf("非 Windows 应 400: code=%d msg=%s", code, resp.Message)
	}

	// 3) Windows 但不在线
	if _, err := database.DB().Insert(&database.Clients{Uid: "win-1", Os: "windows", Listener: "tcp", Arch: "x64"}); err != nil {
		t.Fatalf("插入客户端失败: %v", err)
	}
	if code, resp := postHvncStart(t, router, "win-1"); code != 400 || !strings.Contains(resp.Message, "不在线") {
		t.Fatalf("离线客户端应 400: code=%d msg=%s", code, resp.Message)
	}

	// 4) Windows + 在线但无实际连接 → 下发失败必须如实返回（不再是 200 + sid）
	connection.MuClientListenerType.Lock()
	connection.ClientListenerType["win-1"] = "tcp"
	connection.MuClientListenerType.Unlock()
	t.Cleanup(func() {
		connection.MuClientListenerType.Lock()
		delete(connection.ClientListenerType, "win-1")
		connection.MuClientListenerType.Unlock()
	})
	if code, resp := postHvncStart(t, router, "win-1"); code != 502 || !strings.Contains(resp.Message, "HVNC_START") {
		t.Fatalf("下发失败应 502: code=%d msg=%s", code, resp.Message)
	}
}
