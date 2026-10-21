package config

import "testing"

// 热路径对比：GetGlobalConfig 为保证调用方隔离会做 JSON 深拷贝，
// 不该出现在"每条消息/每次轮询"上；ClientTiming/BatchCommandsEnabled 只读两个字段。
func benchConfigManager() func() {
	old := GlobalConfig
	GlobalConfig = &ConfigManager{config: createDefaultConfig()}
	return func() { GlobalConfig = old }
}

func BenchmarkGetGlobalConfigDeepCopy(b *testing.B) {
	defer benchConfigManager()()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GetGlobalConfig()
	}
}

func BenchmarkClientTiming(b *testing.B) {
	defer benchConfigManager()()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = ClientTiming()
	}
}

func BenchmarkBatchCommandsEnabled(b *testing.B) {
	defer benchConfigManager()()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BatchCommandsEnabled()
	}
}

func BenchmarkDeliverySettings(b *testing.B) {
	defer benchConfigManager()()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = DeliverySettings()
	}
}

func BenchmarkGetDownloadsDir(b *testing.B) {
	defer benchConfigManager()()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GetDownloadsDir()
	}
}

// BenchmarkGetDownloadsDirLegacy 对照：改造前走 GetGlobalConfig 的深拷贝实现。
func BenchmarkGetDownloadsDirLegacy(b *testing.B) {
	defer benchConfigManager()()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if g := GetGlobalConfig(); g != nil {
			_ = g.Storage.DownloadsDir
		}
	}
}
