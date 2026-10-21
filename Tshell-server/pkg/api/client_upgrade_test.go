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

type upgradeResp struct {
	Status int `json:"status"`
	Data   struct {
		Results []struct {
			UID     string `json:"uid"`
			OK      bool   `json:"ok"`
			Message string `json:"message"`
		} `json:"results"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	} `json:"data"`
}

func postUpgrade(t *testing.T, router *gin.Engine, body string) (int, upgradeResp) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/client/upgrade", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var got upgradeResp
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	return w.Code, got
}

// TestUpgradeClientsGuards 升级是破坏性操作：未确认、批量超限、非 Windows、
// 宿主进程（DLL/注入/迁移形态）都必须在生成载荷之前被拒。
func TestUpgradeClientsGuards(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/client/upgrade", UpgradeClients)

	// 1) 未确认
	if code, _ := postUpgrade(t, router, `{"uids":["u1"]}`); code != 400 {
		t.Fatalf("未 confirm 应 400，实际 %d", code)
	}
	// 2) 空列表
	if code, _ := postUpgrade(t, router, `{"uids":[],"confirm":true}`); code != 400 {
		t.Fatalf("空 uids 应 400，实际 %d", code)
	}
	// 3) 批量超限
	many := make([]string, clientUpgradeMaxBatch+1)
	for i := range many {
		many[i] = "uid-" + itoaSmall(i)
	}
	body, _ := json.Marshal(map[string]interface{}{"uids": many, "confirm": true})
	if code, _ := postUpgrade(t, router, string(body)); code != 400 {
		t.Fatalf("超过批量上限应 400，实际 %d", code)
	}

	// 4) 非 Windows
	if _, err := database.DB().Insert(&database.Clients{Uid: "linux-1", Os: "linux", Listener: "tcp", Arch: "x64"}); err != nil {
		t.Fatalf("插入客户端失败: %v", err)
	}
	_, resp := postUpgrade(t, router, `{"uids":["linux-1"],"confirm":true}`)
	if len(resp.Data.Results) != 1 || resp.Data.Results[0].OK || !strings.Contains(resp.Data.Results[0].Message, "仅支持 Windows") {
		t.Fatalf("非 Windows 客户端应被拒: %+v", resp.Data)
	}

	// 5) 宿主进程
	if _, err := database.DB().Insert(&database.Clients{Uid: "calc-1", Os: "windows", Process: "rundll32.exe", Listener: "tcp", Arch: "x64"}); err != nil {
		t.Fatalf("插入客户端失败: %v", err)
	}
	_, resp = postUpgrade(t, router, `{"uids":["calc-1"],"confirm":true}`)
	if resp.Data.Results[0].OK || !strings.Contains(resp.Data.Results[0].Message, "不是独立 EXE") {
		t.Fatalf("宿主进程应被拒: %+v", resp.Data.Results[0])
	}

	// 6) 独立 EXE 但监听器不存在
	if _, err := database.DB().Insert(&database.Clients{Uid: "bare-1", Os: "windows", Process: "svcabcdef.exe", Listener: "tcp", Arch: "x64"}); err != nil {
		t.Fatalf("插入客户端失败: %v", err)
	}
	_, resp = postUpgrade(t, router, `{"uids":["bare-1"],"confirm":true}`)
	if resp.Data.Results[0].OK || !strings.Contains(resp.Data.Results[0].Message, "监听器") {
		t.Fatalf("无监听器应被拒: %+v", resp.Data.Results[0])
	}
}

func itoaSmall(i int) string {
	if i == 0 {
		return "0"
	}
	var b [8]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
