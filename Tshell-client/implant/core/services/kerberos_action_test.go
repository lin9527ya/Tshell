package services

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestParseKerberosAction list/purge/ptt 三种动作与用法错误
func TestParseKerberosAction(t *testing.T) {
	blob := []byte{0x61, 0x82, 0x01, 0x02}
	b64 := base64.StdEncoding.EncodeToString(blob)

	ok := []struct {
		in   string
		kind string
		arg  string
	}{
		{"", kerberosActionList, ""},
		{"list", kerberosActionList, ""},
		{"  LIST  ", kerberosActionList, ""},
		{"purge", kerberosActionPurge, ""},
		{"purge krbtgt/CORP.LOCAL", kerberosActionPurge, "krbtgt/CORP.LOCAL"},
		{"ptt " + b64, kerberosActionPTT, ""},
	}
	for _, c := range ok {
		act, err := parseKerberosAction([]byte(c.in))
		if err != nil {
			t.Fatalf("%q 解析失败: %v", c.in, err)
		}
		if act.Kind != c.kind || (c.arg != "" && act.Arg != c.arg) {
			t.Fatalf("%q 解析结果不符: %+v", c.in, act)
		}
		if c.kind == kerberosActionPTT && string(act.Blob) != string(blob) {
			t.Fatalf("ptt 字节不符: %x", act.Blob)
		}
	}

	bad := []string{
		"list extra",
		"purge a b",
		"ptt",
		"ptt !!!not-base64!!!",
		"unknown",
	}
	for _, c := range bad {
		if _, err := parseKerberosAction([]byte(c)); err == nil {
			t.Fatalf("%q 应报用法错误", c)
		}
	}
}

// TestParseKerberosActionPTTVariants base64 的四种常见变体都要能解
func TestParseKerberosActionPTTVariants(t *testing.T) {
	blob := []byte{0x61, 0x82, 0x00, 0x10, 0x01, 0x02}
	for name, enc := range map[string]*base64.Encoding{
		"std": base64.StdEncoding, "raw-std": base64.RawStdEncoding,
		"url": base64.URLEncoding, "raw-url": base64.RawURLEncoding,
	} {
		act, err := parseKerberosAction([]byte("ptt " + enc.EncodeToString(blob)))
		if err != nil {
			t.Fatalf("%s 变体解析失败: %v", name, err)
		}
		if string(act.Blob) != string(blob) {
			t.Fatalf("%s 变体字节不符: %x", name, act.Blob)
		}
	}
}

// TestParseKerberosActionPTTTooLarge 超过上限要拒绝（防把命令通道打爆）
func TestParseKerberosActionPTTTooLarge(t *testing.T) {
	huge := base64.StdEncoding.EncodeToString(make([]byte, maxPTTBlob+1))
	if _, err := parseKerberosAction([]byte("ptt " + huge)); err == nil || !strings.Contains(err.Error(), "过大") {
		t.Fatalf("超大票据应被拒绝: %v", err)
	}
}
