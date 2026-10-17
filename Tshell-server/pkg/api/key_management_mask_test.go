package api

import (
	"BackendTemplate/pkg/database"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestKeyManagementAllConfigsMasksAddress 密钥管理的 all-configs 也不得回传明文地址：
// oss 的 listen_address 承载 endpoint:AK:SK:bucket，之前只脱敏了列表/详情，这里漏了。
func TestKeyManagementAllConfigsMasksAddress(t *testing.T) {
	setupMaskTestDB(t)

	database.Engine.Insert(&database.Listener{
		Type: "oss", ListenAddress: testOSS, ConnectAddress: testOSS,
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:18086", ConnectAddress: "c2.example.com:18086",
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/key-management/all-configs", GetAllListenerKeyConfigs)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/key-management/all-configs", nil))

	body := w.Body.String()
	// 凭证与密钥明文一律不得出现
	for _, secret := range []string{testVKey, testSalt, "LTAI5tKeyId", "LTAI5tKeySecret"} {
		if strings.Contains(body, secret) {
			t.Errorf("all-configs 泄露了 %q\n%s", secret, body)
		}
	}
	// 脱敏地址与非 oss 地址应原样保留形状
	if !strings.Contains(body, testOSSMasked) {
		t.Errorf("应包含脱敏后的 oss 地址 %q\n%s", testOSSMasked, body)
	}
	if !strings.Contains(body, "0.0.0.0:18086") {
		t.Errorf("非 oss 地址不应被改动\n%s", body)
	}
}

// TestKeyManagementResolvesMaskedAddress 密钥管理的更新/重置接口必须能处理
// 前端从列表回传的脱敏地址，否则找不到记录、操作全部失效。
func TestKeyManagementResolvesMaskedAddress(t *testing.T) {
	setupMaskTestDB(t)

	database.Engine.Insert(&database.Listener{
		Type: "oss", ListenAddress: testOSS, ConnectAddress: testOSS,
		Status: 2, VKey: testVKey, EncryptionSalt: testSalt,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/key-management/update", UpdateListenerKeyConfig)
	router.POST("/key-management/reset/:address", ResetListenerKeyConfig)

	// 1) 用脱敏地址更新密钥 → 应作用于真实记录
	newVKey := "UpdatedVKey0123456789abcdefghijk"
	body := `{"listen_address":"` + testOSSMasked + `","vkey":"` + newVKey + `","encryption_salt":"` + testSalt + `"}`
	req := httptest.NewRequest(http.MethodPost, "/key-management/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `"status":200`) {
		t.Fatalf("用脱敏地址更新密钥应成功，实际 %s", w.Body.String())
	}
	var got database.Listener
	database.Engine.Where("listen_address = ?", testOSS).Get(&got)
	if got.VKey != newVKey {
		t.Errorf("真实记录的 VKey 应已更新，实际 %q", got.VKey)
	}
	if got.ListenAddress != testOSS {
		t.Errorf("地址不应被脱敏值覆盖，实际 %q", got.ListenAddress)
	}

	// 2) 用脱敏地址重置密钥 → 应生成新随机密钥
	req = httptest.NewRequest(http.MethodPost, "/key-management/reset/"+url.PathEscape(testOSSMasked), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `"status":200`) {
		t.Fatalf("用脱敏地址重置密钥应成功，实际 %s", w.Body.String())
	}
	var reset database.Listener
	database.Engine.Where("listen_address = ?", testOSS).Get(&reset)
	if reset.VKey == newVKey {
		t.Error("重置后应生成新的随机密钥")
	}
}
