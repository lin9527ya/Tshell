package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"BackendTemplate/pkg/command"
	credpkg "BackendTemplate/pkg/cred"
	"BackendTemplate/pkg/database"
)

// sampleCardsJSON 一份 cards 技术（Chromium Web Data）的制品：
// 1 张卡 + 2 条自动填充，外加一条缺卡号的卡（应被丢弃）。
func sampleCardsJSON(t *testing.T) []byte {
	t.Helper()
	art := credpkg.BrowserArtifact{
		Host: "WS01", User: `CORP\alice`, CollectedAt: "2026-10-19 22:30:00",
		Browsers: []credpkg.BrowserProfile{
			{
				Name: "chrome", Profile: "Default", Tech: "v10/v20 (Web Data)",
				Cards: []credpkg.BrowserCard{
					{Name: "ALICE ZHANG", Number: "4111111111111111", ExpMonth: 12, ExpYear: 2029, Nickname: "corp"},
					{Name: "NOBODY", Number: ""}, // 缺卡号 → 丢弃
				},
				Autofill: []credpkg.BrowserAutofill{
					{Field: "email", Value: "alice@corp.local", Count: 7},
					{Field: "phone", Value: "13800000000"},
				},
			},
		},
	}
	raw, err := json.Marshal(art)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestCardsArtifactIngest cards 技术：信用卡进凭据库（type=card），
// 自动填充**不入库**（噪声大，只留在制品 JSON 里）。
func TestCardsArtifactIngest(t *testing.T) {
	setupCredTest(t)
	taskID, fileName := issueKindRequest(t, "uid-win", credpkg.KindBrowser, "cards")

	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, sampleCardsJSON(t), 0o600); err != nil {
		t.Fatal(err)
	}
	OnDownloadEnd("uid-win", path, command.DownloadStatusCompleted)

	var task database.Task
	database.Engine.ID(taskID).Get(&task)
	if task.Status != taskStatusCompleted {
		t.Fatalf("制品回传后任务应 completed: %+v", task)
	}

	var loot []database.Loot
	database.Engine.Where("uid = ?", "uid-win").Asc("id").Find(&loot)
	if len(loot) != 2 {
		t.Fatalf("应登记 2 条（1 张卡 + 1 条制品本体；自动填充不入库），实际 %d: %+v", len(loot), loot)
	}

	card := loot[0]
	if card.Type != "card" || card.Secret != "4111111111111111" ||
		card.User != "ALICE ZHANG" || card.Source != "browser:card" || card.Host != "WS01" {
		t.Fatalf("信用卡条目字段不符: %+v", card)
	}
	if card.Note == "" {
		t.Fatal("信用卡条目应带来源/有效期说明")
	}

	artifact := loot[1]
	if artifact.Type != "dump" || artifact.Source != credpkg.KindBrowser {
		t.Fatalf("制品本体条目不符: %+v", artifact)
	}

	// 任务结果里应体现卡数（前端任务中心据此显示战果）
	if task.Result == "" {
		t.Fatal("任务结果不应为空")
	}
}

// TestCardsTechRegistered 锁住 cards 技术的接线（服务端技术表 + 前端清单同源）
func TestCardsTechRegistered(t *testing.T) {
	if !credpkg.IsTechSupportedFor(credpkg.KindBrowser, "cards") {
		t.Fatal("browser 应支持 cards 技术")
	}
}
