package security

import (
	"strings"
	"testing"
)

func TestMaterializeDynamicHeaders(t *testing.T) {
	in := map[string]string{
		"X-TT-TOKEN":    "{{RAND:32}}",
		"Authorization": "Bearer {{RAND:110}}",
		"Cookie":        "BAIDUID={{RAND:32}}:FG=1; BIDUPSID={{RAND:32}}",
		"Static":        "no-placeholder-here",
		"WS":            "{{WSKEY}}",
	}
	out := materializeDynamicHeaders(in)

	if out["Static"] != "no-placeholder-here" {
		t.Errorf("static header should be unchanged, got %q", out["Static"])
	}
	if strings.Contains(out["X-TT-TOKEN"], "{{RAND:") {
		t.Errorf("placeholder not replaced: %q", out["X-TT-TOKEN"])
	}
	if len(out["X-TT-TOKEN"]) != 32 {
		t.Errorf("expected 32 chars, got %d: %q", len(out["X-TT-TOKEN"]), out["X-TT-TOKEN"])
	}
	if !strings.HasPrefix(out["Authorization"], "Bearer ") || len(out["Authorization"]) != len("Bearer ")+110 {
		t.Errorf("Authorization wrong: %q", out["Authorization"])
	}
	// 同一值内两个占位符必须各自独立替换
	if strings.Contains(out["Cookie"], "{{RAND:") {
		t.Errorf("cookie placeholders not replaced: %q", out["Cookie"])
	}
	// WSKEY 占位符替换为非空 base64 串
	if strings.Contains(out["WS"], "{{WSKEY}}") || len(out["WS"]) == 0 {
		t.Errorf("WSKEY not replaced: %q", out["WS"])
	}
	// 每次调用应生成新的随机值（非静态）
	out2 := materializeDynamicHeaders(in)
	if out["X-TT-TOKEN"] == out2["X-TT-TOKEN"] {
		t.Errorf("expected fresh random per call, got identical %q", out["X-TT-TOKEN"])
	}
}
