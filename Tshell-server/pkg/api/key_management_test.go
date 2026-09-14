package api

import (
	"BackendTemplate/pkg/database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestGetListenerKeyConfigAPI 验证监听器密钥配置查询接口(不泄露敏感信息)
func TestGetListenerKeyConfigAPI(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	// 添加监听器(带密钥配置)
	database.Engine.Insert(&database.Listener{
		Type:            "tcp",
		ListenAddress:   "0.0.0.0:9999",
		VKey:            "testvkey123456789012345678901234",
		EncryptionSalt:  "testsalt12345678",
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/key/config", GetListenerKeyConfig)

	req := httptest.NewRequest(http.MethodGet, "/api/key/config?type=tcp&address=0.0.0.0:9999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Data["master_key_length"] != float64(32) {
		t.Fatalf("master_key_length 应为 32(vkey 32 字节), 实际 %v", resp.Data["master_key_length"])
	}
	if _, leaked := resp.Data["vkey"]; leaked {
		t.Fatal("接口泄露了 vkey 明文")
	}
	if _, leaked := resp.Data["encryption_salt"]; leaked {
		t.Fatal("接口泄露了 salt 明文")
	}
	t.Log("密钥配置查询接口验证通过(仅返回长度等非敏感信息)")
}

// TestGetAllListenerKeyConfigsAPI 验证批量查询接口
func TestGetAllListenerKeyConfigsAPI(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/key/configs", GetAllListenerKeyConfigs)

	req := httptest.NewRequest(http.MethodGet, "/api/key/configs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	t.Log("批量密钥配置查询验证通过")
}
