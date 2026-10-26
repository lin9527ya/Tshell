package services

import "testing"

// TestParseModifyTime Web 端预填/提示用 "YYYY/MM/DD HH:mm:ss"，
// 旧脚本用 ISO 8601；两种都必须能解析（原实现只认 "2006-01-02T15:04"，
// 前端下发的时间永远回执 Invalid time format）。
func TestParseModifyTime(t *testing.T) {
	cases := []struct {
		in       string
		wantYear int
		wantMon  int
		wantDay  int
		wantHour int
		wantMin  int
		wantSec  int
	}{
		{"2026/10/26 14:30:59", 2026, 10, 26, 14, 30, 59},
		{"2026/10/26 14:30", 2026, 10, 26, 14, 30, 0},
		{"2026-10-26 14:30:59", 2026, 10, 26, 14, 30, 59},
		{"2026-10-26T14:30:59", 2026, 10, 26, 14, 30, 59},
		{"2026-10-26T14:30", 2026, 10, 26, 14, 30, 0},
		{"2026/10/26", 2026, 10, 26, 0, 0, 0},
	}
	for _, c := range cases {
		got, err := parseModifyTime(c.in)
		if err != nil {
			t.Fatalf("解析 %q 失败: %v", c.in, err)
		}
		if got.Year() != c.wantYear || int(got.Month()) != c.wantMon || got.Day() != c.wantDay ||
			got.Hour() != c.wantHour || got.Minute() != c.wantMin || got.Second() != c.wantSec {
			t.Fatalf("解析 %q 结果错误: %v", c.in, got)
		}
	}
}

func TestParseModifyTimeRejectsGarbage(t *testing.T) {
	if _, err := parseModifyTime("not-a-time"); err == nil {
		t.Fatal("非法时间应返回错误")
	}
}
