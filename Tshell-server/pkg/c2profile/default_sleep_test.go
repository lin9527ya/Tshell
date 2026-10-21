package c2profile

import (
	"os"
	"path/filepath"
	"testing"

	"BackendTemplate/pkg/config"

	"gopkg.in/yaml.v3"
)

// ===== OPSEC P1-2：心跳默认值不得与全局配置漂移 =====
//
// 这个测试守的是一个真实存在过的不一致：默认画像里写着 sleeptime_ms: 1000，
// 而全局 client.sleep 的默认值是 5（秒）。新建监听器时若没显式指定，
// 最终拿到哪一档取决于走哪条路径 —— 运维根本看不出来。
//
// 顺带说清为什么 1 秒是错的：1 秒心跳意味着单台主机全天约 8.6 万次请求，
// 这个节奏本身就是很显眼的流量特征，而"长期驻留"这个主要用途根本不需要它。

func parseDefaultProfile(t *testing.T) Profile {
	t.Helper()
	var p Profile
	if err := yaml.Unmarshal([]byte(defaultProfileYAML), &p); err != nil {
		t.Fatalf("默认画像解析失败: %v", err)
	}
	return p
}

// 默认画像的心跳必须等于全局默认
func TestDefaultProfileSleepMatchesGlobalDefault(t *testing.T) {
	p := parseDefaultProfile(t)

	// 全局默认从配置结构读（struct tag 的 default），避免两处硬编码各自漂移
	globalSec := config.GetGlobalConfig().Client.Sleep
	if globalSec <= 0 {
		t.Fatalf("全局默认心跳异常: %d", globalSec)
	}
	wantMs := globalSec * 1000
	if p.SleeptimeMs != wantMs {
		t.Errorf("默认画像 sleeptime_ms=%d，与全局默认 %d 秒（%d ms）不一致 —— "+
			"两处漂移会让新建监听器的心跳取决于走哪条路径，运维看不出来",
			p.SleeptimeMs, globalSec, wantMs)
	}
}

// 1 秒心跳对长期驻留太吵：留一条下限断言，防止有人为了"响应快"又调回去
func TestDefaultProfileSleepIsNotTooAggressive(t *testing.T) {
	p := parseDefaultProfile(t)
	if p.SleeptimeMs < 5000 {
		perDay := 86400000 / p.SleeptimeMs
		t.Errorf("默认心跳 %d ms 过于激进（全天约 %d 次请求）。"+
			"默认值应偏向隐蔽；需要快响应应在画像里显式配置并写清理由",
			p.SleeptimeMs, perDay)
	}
}
// 画像文件没写 sleeptime_ms 时，管理器兜底值也必须与默认画像一致。
// 原先兜底写死 1000 —— 运维只删一行配置就会把节奏悄悄降到 1 秒。
func TestManagerSleepFallbackMatchesDefault(t *testing.T) {
	dir := t.TempDir()
	body := `name: nosleep
http:
  get:
    uri: /x
`
	if err := os.WriteFile(filepath.Join(dir, "nosleep.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("写测试画像失败: %v", err)
	}
	if err := InitNoWatch(dir); err != nil {
		t.Fatalf("加载画像失败: %v", err)
	}
	p := Get("nosleep")
	if p == nil {
		t.Fatal("画像未加载")
	}
	if p.SleeptimeMs != defaultSleeptimeMs {
		t.Fatalf("缺省 sleeptime_ms 兜底 = %d，期望 %d", p.SleeptimeMs, defaultSleeptimeMs)
	}
}
