package config

import "testing"

// storage.downloads_dir：配置生效 + 空值回落 ./Downloads（历史默认）。
func TestGetDownloadsDir(t *testing.T) {
	old := GlobalConfig
	defer func() { GlobalConfig = old }()

	// 1) 未初始化：回落 ./Downloads
	GlobalConfig = nil
	if got := GetDownloadsDir(); got != "./Downloads" {
		t.Fatalf("未初始化时应回落 ./Downloads，得到 %q", got)
	}

	// 2) 配置生效
	cfg := createDefaultConfig()
	cfg.Storage.DownloadsDir = "/data/downloads"
	GlobalConfig = &ConfigManager{config: cfg}
	if got := GetDownloadsDir(); got != "/data/downloads" {
		t.Fatalf("应返回配置值 /data/downloads，得到 %q", got)
	}

	// 3) 显式置空：回落默认
	cfg2 := createDefaultConfig()
	cfg2.Storage.DownloadsDir = ""
	GlobalConfig = &ConfigManager{config: cfg2}
	if got := GetDownloadsDir(); got != "./Downloads" {
		t.Fatalf("空值应回落 ./Downloads，得到 %q", got)
	}
}

// 默认配置里该字段的默认值必须与实现回落值一致，避免"文档与行为两套默认"。
func TestDownloadsDirDefaultTag(t *testing.T) {
	cfg := createDefaultConfig()
	if cfg.Storage.DownloadsDir != "./Downloads" {
		t.Fatalf("默认值应为 ./Downloads，得到 %q", cfg.Storage.DownloadsDir)
	}
}
