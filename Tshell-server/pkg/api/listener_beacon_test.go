package api

import (
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func intPtr(v int) *int { return &v }

// TestAddListenerBeaconValidation 越界值必须在入库前拒绝。
func TestAddListenerBeaconValidation(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/add", AddListener)

	base := `{"type":"tcp","listenAddress":"127.0.0.1:19097","connectAddress":"c2.example.com:19097",` +
		`"vkey":"` + testVKey + `","encryptionSalt":"` + testSalt + `"`
	cases := []struct {
		name string
		body string
	}{
		{"sleep 0 非法", base + `,"beaconSleepSeconds":0}`},
		{"sleep 301 非法", base + `,"beaconSleepSeconds":301}`},
		{"jitter -2 非法", base + `,"beaconJitterPercent":-2}`},
		{"jitter 51 非法", base + `,"beaconJitterPercent":51}`},
	}
	for _, c := range cases {
		if _, status, msg := postJSON(t, router, "/listener/add", c.body); status != 400 {
			t.Errorf("%s: 期望 400，实际 %d msg=%s", c.name, status, msg)
		}
	}
	var count int64
	count, _ = database.Engine.Count(&database.Listener{})
	if count != 0 {
		t.Fatalf("非法参数不应落库，实际 %d 条", count)
	}
}

// TestAddListenerBeaconRoundTrip 合法值（含显式 0% jitter）落库，并在列表响应里
// 同时给出存储值与生效值。
func TestAddListenerBeaconRoundTrip(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/add", AddListener)

	body := `{"type":"tcp","listenAddress":"127.0.0.1:19098","connectAddress":"c2.example.com:19098",` +
		`"vkey":"` + testVKey + `","encryptionSalt":"` + testSalt + `",` +
		`"beaconSleepSeconds":30,"beaconJitterPercent":0}`
	if _, status, msg := postJSON(t, router, "/listener/add", body); status != 200 {
		t.Fatalf("合法参数应成功，status=%d msg=%s", status, msg)
	}

	var l database.Listener
	if has, _ := database.Engine.Where("connect_address = ?", "c2.example.com:19098").Get(&l); !has {
		t.Fatal("监听器未落库")
	}
	if l.BeaconSleepSeconds == nil || *l.BeaconSleepSeconds != 30 {
		t.Fatalf("sleep 未落库: %+v", l.BeaconSleepSeconds)
	}
	if l.BeaconJitterPercent == nil || *l.BeaconJitterPercent != 0 {
		t.Fatalf("显式 0%% jitter 必须保留: %+v", l.BeaconJitterPercent)
	}

	resp := maskListenerForResponse(l)
	if resp.BeaconSleepSeconds != 30 || resp.BeaconJitterPercent != 0 {
		t.Fatalf("响应存储值错误: %+v", resp)
	}
	if resp.EffectiveSleepSeconds != 30 || resp.EffectiveJitterPercent != 0 {
		t.Fatalf("生效值应等于监听器显式值: %+v", resp)
	}
}

// TestUpdateListenerBeaconPreserveAndClear nil = 保留；-1 = 清空（继承）。
func TestUpdateListenerBeaconPreserveAndClear(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	if _, err := database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "127.0.0.1:19099", ConnectAddress: "c2.example.com:19099",
		Status: 2, VKey: testVKey, EncryptionSalt: testSalt, Profile: "default",
		HeartbeatTimeout: "30", HeartbeatInterval: "10",
		BeaconSleepSeconds: intPtr(30), BeaconJitterPercent: intPtr(40),
	}); err != nil {
		t.Fatalf("插入监听器失败: %v", err)
	}
	var lis database.Listener
	if has, _ := database.Engine.Where("listen_address = ?", "127.0.0.1:19099").Get(&lis); !has {
		t.Fatal("监听器未插入")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/update", UpdateListener)

	// 1) 不提交 beacon 字段 → 保留原值；不提交 profile → 原画像不被清空
	body := `{"id":` + jsonInt(lis.Id) + `,"type":"tcp","listenAddress":"127.0.0.1:19099",` +
		`"connectAddress":"c2.example.com:19099","note":"edited"}`
	if _, status, msg := postJSON(t, router, "/listener/update", body); status != 200 {
		t.Fatalf("更新失败 status=%d msg=%s", status, msg)
	}
	var after database.Listener
	database.Engine.ID(lis.Id).Get(&after)
	if after.BeaconSleepSeconds == nil || *after.BeaconSleepSeconds != 30 ||
		after.BeaconJitterPercent == nil || *after.BeaconJitterPercent != 40 {
		t.Fatalf("省略字段时应保留原 beacon 配置: sleep=%+v jitter=%+v", after.BeaconSleepSeconds, after.BeaconJitterPercent)
	}
	if after.Profile != "default" {
		t.Fatalf("省略 profile 时原画像被清空: %q", after.Profile)
	}

	// 2) 显式 -1 → 清空为继承（NULL）
	body = `{"id":` + jsonInt(lis.Id) + `,"type":"tcp","listenAddress":"127.0.0.1:19099",` +
		`"beaconSleepSeconds":-1,"beaconJitterPercent":-1}`
	if _, status, msg := postJSON(t, router, "/listener/update", body); status != 200 {
		t.Fatalf("清空失败 status=%d msg=%s", status, msg)
	}
	var cleared database.Listener // 用新变量：xorm 不会把 NULL 回写到已填充的结构体字段
	database.Engine.ID(lis.Id).Get(&cleared)
	if cleared.BeaconSleepSeconds != nil || cleared.BeaconJitterPercent != nil {
		sv, jv := -999, -999
		if cleared.BeaconSleepSeconds != nil { sv = *cleared.BeaconSleepSeconds }
		if cleared.BeaconJitterPercent != nil { jv = *cleared.BeaconJitterPercent }
		t.Fatalf("-1 应清空为 NULL: sleep=%d jitter=%d", sv, jv)
	}
}

// TestListenerEffectiveBeaconFallsBack 未配置时按画像/全局计算生效值。
func TestListenerEffectiveBeaconFallsBack(t *testing.T) {
	ensureTestProfiles(t)
	setupMaskTestDB(t)

	l := database.Listener{Profile: "default"} // 测试画像 sleeptime_ms=1000
	sleep, jitter := listenerEffectiveBeacon(l)
	if sleep != 1 {
		t.Fatalf("画像 1000ms 应得到 1s，实际 %d", sleep)
	}
	if jitter != delivery.ResolveBeaconJitter("", delivery.BeaconUnset) {
		t.Fatalf("jitter 应回落到全局，实际 %d", jitter)
	}
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TestApplyListenerProfile 验证 WebDelivery/注入/迁移路径会按监听器绑定的画像
// 补齐 URI/WS/DNS/UA 等流量特征（否则绑非 default 画像的载荷永远连不上）。
func TestApplyListenerProfile(t *testing.T) {
	setupMaskTestDB(t)

	dir := t.TempDir()
	yaml := `name: alt
useragent: "UA/alt"
sleeptime_ms: 3000
http:
  get:
    uri: /alt/get
    padding: false
  post:
    uri: /alt/post
ws:
  path: /alt/ws
tls:
  ja3: firefox_121
dns:
  beacon_prefix: altupd
`
	if err := os.WriteFile(filepath.Join(dir, "alt.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("写测试画像失败: %v", err)
	}
	if err := c2profile.InitNoWatch(dir); err != nil {
		t.Fatalf("加载画像失败: %v", err)
	}
	if _, err := database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:19077", ConnectAddress: "c2.example.com:19077",
		Status: 2, Profile: "alt",
	}); err != nil {
		t.Fatalf("插入监听器失败: %v", err)
	}

	opts := delivery.PatchOptions{}
	applyListenerProfile(&opts, "tcp://c2.example.com:19077")
	if opts.HTTPGetURI != "/alt/get" || opts.HTTPPostURI != "/alt/post" || opts.WSPath != "/alt/ws" {
		t.Fatalf("URI/WS 未按画像补齐: %+v", opts)
	}
	if opts.TLSJA3 != "firefox_121" || opts.UserAgent != "UA/alt" {
		t.Fatalf("TLS/UA 未按画像补齐: ja3=%q ua=%q", opts.TLSJA3, opts.UserAgent)
	}
	if opts.DNSLabelPrefix != "altupd" || opts.Padding != "0" {
		t.Fatalf("DNS/Padding 未按画像补齐: dns=%q padding=%q", opts.DNSLabelPrefix, opts.Padding)
	}

	// 显式值优先，不被画像覆盖
	explicit := delivery.PatchOptions{HTTPGetURI: "/explicit", Padding: "1", UserAgent: "UA/explicit"}
	applyListenerProfile(&explicit, "tcp://c2.example.com:19077")
	if explicit.HTTPGetURI != "/explicit" || explicit.Padding != "1" || explicit.UserAgent != "UA/explicit" {
		t.Fatalf("显式值被画像覆盖: %+v", explicit)
	}
	if explicit.WSPath != "/alt/ws" {
		t.Fatalf("空字段仍应被画像补齐: ws=%q", explicit.WSPath)
	}
}
