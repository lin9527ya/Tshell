package cred

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestParseBrowserArtifactCards cards/autofill 的保留、丢弃与统计：
//   - 缺卡号的卡丢弃、缺字段的自动填充丢弃
//   - 只有卡（没有口令/Cookie）时制品仍然合法 —— 这是 cards 技术的正常形态
func TestParseBrowserArtifactCards(t *testing.T) {
	art := BrowserArtifact{
		Host: "WS01",
		Browsers: []BrowserProfile{{
			Name: "chrome", Profile: "Default",
			Cards: []BrowserCard{
				{Name: "ALICE", Number: "4111111111111111", ExpMonth: 12, ExpYear: 2029},
				{Name: "NOBODY", Number: "   "}, // 空白卡号 → 丢弃
			},
			Autofill: []BrowserAutofill{
				{Field: "email", Value: "a@b.c"},
				{Field: "", Value: "x"},     // 缺字段名 → 丢弃
				{Field: "phone", Value: ""}, // 缺值 → 丢弃
			},
		}},
	}
	raw, err := json.Marshal(art)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cards.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ParseBrowserArtifact(path)
	if err != nil {
		t.Fatalf("只有卡的制品应合法: %v", err)
	}
	if len(got.Browsers[0].Cards) != 1 || len(got.Browsers[0].Autofill) != 1 {
		t.Fatalf("过滤结果不符: cards=%d autofill=%d",
			len(got.Browsers[0].Cards), len(got.Browsers[0].Autofill))
	}
	info := got.Info()
	if info.Cards != 1 || info.Autofill != 1 || info.Entries != 2 {
		t.Fatalf("统计不符: %+v", info)
	}
	if !contains(info.Describe(), "信用卡") {
		t.Fatalf("Describe 应体现信用卡: %s", info.Describe())
	}
}

// TestParseBrowserArtifactEmptyCards 全空制品必须失败（不能登记半截制品）
func TestParseBrowserArtifactEmptyCards(t *testing.T) {
	art := BrowserArtifact{Browsers: []BrowserProfile{{Name: "chrome", Cards: []BrowserCard{{Number: ""}}}}}
	raw, _ := json.Marshal(art)
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseBrowserArtifact(path); err == nil {
		t.Fatal("没有任何可用条目时应失败")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
