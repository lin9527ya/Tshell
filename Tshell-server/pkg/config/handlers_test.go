package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSanitizeConfigRedactsNewSecrets 指标 token 与签名密码此前会原样返回给前端/导出。
func TestSanitizeConfigRedactsNewSecrets(t *testing.T) {
	cfg := &Config{}
	cfg.Metrics.Token = "metrics-secret"
	cfg.Signing.Password = "signing-secret"
	cfg.Auth.DefaultPassword = "admin-secret"
	cfg.Notifications.DingTalk.Token = "ding-secret"

	got := sanitizeConfig(cfg)
	if got.Metrics.Token != "[REDACTED]" {
		t.Fatalf("metrics.token 未脱敏: %q", got.Metrics.Token)
	}
	if got.Signing.Password != "[REDACTED]" {
		t.Fatalf("signing.password 未脱敏: %q", got.Signing.Password)
	}
	if got.Auth.DefaultPassword != "[REDACTED]" {
		t.Fatalf("auth.default_password 未脱敏: %q", got.Auth.DefaultPassword)
	}
	if got.Notifications.DingTalk.Token != "[REDACTED]" {
		t.Fatalf("notifications token 未脱敏: %q", got.Notifications.DingTalk.Token)
	}
	// 脱敏不能改到原配置，否则后续保存会把 [REDACTED] 写回
	if cfg.Metrics.Token != "metrics-secret" || cfg.Signing.Password != "signing-secret" {
		t.Fatal("sanitizeConfig 不应修改原配置")
	}
}

func TestDurationValue(t *testing.T) {
	if d, ok := durationValue(float64(30 * time.Second)); !ok || d != 30*time.Second {
		t.Fatalf("数字 duration 解析失败: %v %v", d, ok)
	}
	if d, ok := durationValue("15m"); !ok || d != 15*time.Minute {
		t.Fatalf("字符串 duration 解析失败: %v %v", d, ok)
	}
	if _, ok := durationValue("not-a-duration"); ok {
		t.Fatal("非法 duration 应返回 false")
	}
}

// TestPersistRateLimitConfig 安全中心修改限流参数后必须落盘，重启不能回默认值。
func TestPersistRateLimitConfig(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	cm := &ConfigManager{configFile: file, config: createDefaultConfig()}
	if err := cm.PersistRateLimitConfig(9, 90, 5); err != nil {
		t.Fatalf("PersistRateLimitConfig: %v", err)
	}
	got := cm.GetConfig()
	if got.Security.LoginRateLimit != 9 || got.Security.APIRateLimit != 90 ||
		got.Security.BlockDuration != 5*time.Minute {
		t.Fatalf("内存配置未更新: %+v", got.Security)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"login_rate_limit: 9", "api_rate_limit: 90", "block_duration: 5m0s"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("配置文件缺少 %q:\n%s", want, data)
		}
	}
}
