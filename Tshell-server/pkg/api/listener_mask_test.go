package api

import (
	"BackendTemplate/pkg/database"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const (
	testVKey      = "VKeySecretValue0123456789abcdefg"
	testSalt      = "SaltSecretValue1"
	testOSS       = "oss-cn-hangzhou.aliyuncs.com:LTAI5tKeyId:LTAI5tKeySecret:my-bucket"
	testOSSMasked = "oss-cn-hangzhou.aliyuncs.com:" + maskedMarker + ":" + maskedMarker + ":my-bucket"
)

func setupMaskTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() {
		if database.Engine != nil {
			database.Engine.Close()
		}
	})
}

// TestMaskSecretPreview 预览格式与前端 maskSecret 规则一致，且对其幂等——
// 前端拿到预览后再掩码一次结果不变，故无需同步改前端。
func TestMaskSecretPreview(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"abc", maskedMarker},
		{"abcd", maskedMarker},
		{"abcde", "abcd****de"},
		{testVKey, "VKey****fg"},
	}
	for _, tt := range tests {
		got := maskSecretPreview(tt.in)
		if got != tt.want {
			t.Errorf("maskSecretPreview(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if again := maskSecretPreview(got); again != got {
			t.Errorf("非幂等: %q -> %q -> %q", tt.in, got, again)
		}
		// 前端 maskSecret 的规则（slice(0,4)+'****'+slice(-2)）作用于预览必须是不动点
		if len(got) > 4 && got[:4]+maskedMarker+got[len(got)-2:] != got {
			t.Errorf("预览经前端掩码后发生变化: %q", got)
		}
	}
}

// TestMaskListenerAddress 仅 oss 的地址承载凭证；其余类型（含 dns 的 addr:port:domain）不动
func TestMaskListenerAddress(t *testing.T) {
	if got := maskListenerAddress("oss", testOSS); got != testOSSMasked {
		t.Errorf("oss 地址脱敏错误: %q", got)
	}
	// 大小写不敏感
	if got := maskListenerAddress("OSS", testOSS); got != testOSSMasked {
		t.Errorf("oss 类型判定应大小写不敏感: %q", got)
	}
	for _, ty := range []string{"tcp", "kcp", "websocket", "http", "https"} {
		addr := "0.0.0.0:8081"
		if got := maskListenerAddress(ty, addr); got != addr {
			t.Errorf("%s 地址不应被改动: %q", ty, got)
		}
	}
	dns := "0.0.0.0:53:dns.example.com"
	if got := maskListenerAddress("dns", dns); got != dns {
		t.Errorf("dns 地址不含凭证，不应改动: %q", got)
	}
	// 段数不足的脏数据原样返回，不 panic
	if got := maskListenerAddress("oss", "ep:AK"); got != "ep:AK" {
		t.Errorf("段数不足应原样返回: %q", got)
	}
	if got := maskListenerAddress("oss", ""); got != "" {
		t.Errorf("空地址应原样返回: %q", got)
	}
}

// TestListListenerMasksSecrets 列表接口是唯一把监听器配置送回浏览器的入口，
// 不得再返回明文 VKey/加密盐，也不得返回 oss 地址里的 AK/SK。
func TestListListenerMasksSecrets(t *testing.T) {
	setupMaskTestDB(t)

	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:8081", ConnectAddress: "c2.example.com:8081",
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})
	database.Engine.Insert(&database.Listener{
		Type: "oss", ListenAddress: testOSS, ConnectAddress: testOSS,
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/listener/list", ListListener)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/listener/list", nil))

	body := w.Body.String()
	for _, secret := range []string{testVKey, testSalt, "LTAI5tKeyId", "LTAI5tKeySecret"} {
		if strings.Contains(body, secret) {
			t.Errorf("列表响应泄露了 %q\n%s", secret, body)
		}
	}
	// 预览与脱敏地址要在（否则前端展示会与改动前不一致）
	for _, want := range []string{"VKey****fg", "Salt****e1", testOSSMasked} {
		if !strings.Contains(body, want) {
			t.Errorf("响应缺少脱敏预览 %q\n%s", want, body)
		}
	}
}

// TestUpdateListenerKeepsMaskedSecrets 掩码即未修改：前端把列表里的脱敏值原样回传时，
// 密钥与地址必须保持原值——否则 oss 监听器一编辑就会把 AK/SK 写成占位符、密钥写成掩码。
func TestUpdateListenerKeepsMaskedSecrets(t *testing.T) {
	setupMaskTestDB(t)

	database.Engine.Insert(&database.Listener{
		Type: "oss", ListenAddress: testOSS, ConnectAddress: testOSS,
		Status: 2, VKey: testVKey, EncryptionSalt: testSalt, Note: "old",
	})
	var lis database.Listener
	if has, _ := database.Engine.Where("listen_address = ?", testOSS).Get(&lis); !has {
		t.Fatal("测试前置：监听器未插入成功")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/update", UpdateListener)

	// 完全模拟前端：地址取列表里的脱敏值、密钥取预览值，只改备注
	body := fmt.Sprintf(`{"id":%d,"listenAddress":%q,"connectAddress":%q,"type":"oss","vkey":%q,"encryptionSalt":%q,"note":"edited"}`,
		lis.Id, testOSSMasked, testOSSMasked, maskSecretPreview(testVKey), maskSecretPreview(testSalt))
	req := httptest.NewRequest(http.MethodPost, "/listener/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Status int    `json:"status"`
		Data   string `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != 200 {
		t.Fatalf("更新应成功，实际 status=%d data=%s", resp.Status, resp.Data)
	}

	var got database.Listener
	database.Engine.Where("id = ?", lis.Id).Get(&got)
	if got.ListenAddress != testOSS || got.ConnectAddress != testOSS {
		t.Errorf("地址被脱敏值覆盖: listen=%q connect=%q", got.ListenAddress, got.ConnectAddress)
	}
	if got.VKey != testVKey || got.EncryptionSalt != testSalt {
		t.Errorf("密钥被预览值覆盖: VKey=%q Salt=%q", got.VKey, got.EncryptionSalt)
	}
	if got.Note != "edited" {
		t.Errorf("备注未更新: %q", got.Note)
	}
}

// TestResolveListenerAddress 掩码地址还原为库中真实地址；非掩码走快路径原样返回；
// 匹配不到时原样返回（由调用方按原逻辑报"监听器不存在"）。
func TestResolveListenerAddress(t *testing.T) {
	setupMaskTestDB(t)
	database.Engine.Insert(&database.Listener{
		Type: "oss", ListenAddress: testOSS, ConnectAddress: testOSS, Status: 1,
	})

	if got := resolveListenerAddress(testOSSMasked, "listen"); got != testOSS {
		t.Errorf("脱敏地址应还原为真实地址: %q", got)
	}
	if got := resolveListenerAddress(testOSSMasked, "connect"); got != testOSS {
		t.Errorf("connect 字段同样应还原: %q", got)
	}
	if got := resolveListenerAddress("0.0.0.0:8081", "listen"); got != "0.0.0.0:8081" {
		t.Errorf("非脱敏值应原样返回: %q", got)
	}
	unmatched := "other.endpoint.com:" + maskedMarker + ":" + maskedMarker + ":nope"
	if got := resolveListenerAddress(unmatched, "listen"); got != unmatched {
		t.Errorf("匹配不到时应原样返回: %q", got)
	}
}

// TestAddListenerRejectsMaskedValues 脱敏预览不是真实配置：从列表复制粘贴过来新建
// 会把占位符存成真实值（地址查不到、密钥是掩码），必须拒绝。
func TestAddListenerRejectsMaskedValues(t *testing.T) {
	setupMaskTestDB(t)

	const addr = "0.0.0.0:18094"
	body := `{"type":"tcp","listenAddress":"` + addr + `","connectAddress":"c2.example.com:18094",` +
		`"vkey":"` + maskSecretPreview(testVKey) + `","encryptionSalt":"0123456789abcdef"}`

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/add", AddListener)
	req := httptest.NewRequest(http.MethodPost, "/listener/add", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Status int    `json:"status"`
		Data   string `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != 400 {
		t.Errorf("脱敏值应被拒绝，实际 status=%d data=%s", resp.Status, resp.Data)
	}
	var count int64
	count, _ = database.Engine.Where("listen_address = ?", addr).Count(&database.Listener{})
	if count != 0 {
		t.Errorf("被拒的监听器不应入库，实际 %d 条", count)
	}
}

// TestPadForwardKeysIgnoresMasked 正向「连接」按钮携带的是列表里的脱敏密钥，必须视为未提供，
// 交给连接路径按监听器配置取真实密钥；否则会拿掩码当密钥去派生会话密钥。
func TestPadForwardKeysIgnoresMasked(t *testing.T) {
	if mk, sl := padForwardKeys(maskSecretPreview(testVKey), maskSecretPreview(testSalt)); mk != nil || sl != nil {
		t.Errorf("脱敏密钥应视为未提供，实际 mk=%v sl=%v", mk, sl)
	}
	if mk, sl := padForwardKeys("", ""); mk != nil || sl != nil {
		t.Errorf("空密钥应视为未提供，实际 mk=%v sl=%v", mk, sl)
	}
	mk, sl := padForwardKeys(testVKey, testSalt)
	if len(mk) != 32 || len(sl) != 16 {
		t.Errorf("真实密钥应按 32/16 字节归一化，实际 %d/%d", len(mk), len(sl))
	}
}
