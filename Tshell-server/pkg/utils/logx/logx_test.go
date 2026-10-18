package logx

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// logx 的格式/输出/级别三项都来自配置（config.logging.*）。
// 它们此前是死配置：无论怎么配都是 text + stdout。这里守住"配了就生效"。

func withOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	oldOut := out
	oldJSON := jsonMode
	oldLvl := lvl
	buf := &bytes.Buffer{}
	SetOutput(buf)
	t.Cleanup(func() {
		mu.Lock()
		out, jsonMode, lvl = oldOut, oldJSON, oldLvl
		mu.Unlock()
	})
	return buf
}

// TestTextFormatKeepsEmojiAndNewline text 模式保持人类可读输出（emoji + 调用方换行）
func TestTextFormatKeepsEmojiAndNewline(t *testing.T) {
	buf := withOutput(t)
	SetFormat("text")
	SetLevel(LvlInfo)

	Infof("✅ 监听器已启动: %s\n", "tcp:8080")
	got := buf.String()
	if !strings.Contains(got, "✅ 监听器已启动: tcp:8080") {
		t.Fatalf("text 输出不符: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("text 输出应保留调用方换行: %q", got)
	}
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Fatalf("text 模式不应输出 JSON: %q", got)
	}
}

// TestJSONFormatIsOneLinePerRecord json 模式每行一个对象，且字段完整
func TestJSONFormatIsOneLinePerRecord(t *testing.T) {
	buf := withOutput(t)
	SetFormat("json")
	SetLevel(LvlInfo)

	Infof("beacon 上线: %s\n", "uid-1")
	Warnf("队列积压 %d\n", 7)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("应有 2 行 JSON，实际 %d 行: %q", len(lines), buf.String())
	}
	for i, line := range lines {
		var rec map[string]string
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("第 %d 行不是合法 JSON: %v (%q)", i+1, err, line)
		}
		for _, k := range []string{"time", "level", "msg"} {
			if rec[k] == "" {
				t.Fatalf("第 %d 行缺少字段 %s: %q", i+1, k, line)
			}
		}
		if strings.HasSuffix(rec["msg"], "\n") {
			t.Fatalf("json 模式应去掉消息里的换行: %q", rec["msg"])
		}
	}
	var first map[string]string
	_ = json.Unmarshal([]byte(lines[0]), &first)
	if first["level"] != "info" || !strings.Contains(first["msg"], "beacon 上线: uid-1") {
		t.Fatalf("首行内容不符: %+v", first)
	}
}

// TestLevelFiltering 级别过滤：低级别日志不输出
func TestLevelFiltering(t *testing.T) {
	buf := withOutput(t)
	SetFormat("text")
	SetLevel(LvlWarn)

	Infof("这条不该出现\n")
	Errorf("这条应该出现\n")

	got := buf.String()
	if strings.Contains(got, "不该出现") {
		t.Fatalf("warn 级别下不应输出 info: %q", got)
	}
	if !strings.Contains(got, "这条应该出现") {
		t.Fatalf("error 应始终输出: %q", got)
	}
	if ParseLevel("debug") != LvlDebug || ParseLevel("WARN") != LvlWarn || ParseLevel("") != LvlInfo {
		t.Fatal("ParseLevel 映射不符")
	}
	if CurrentLevel() != LvlWarn {
		t.Fatalf("CurrentLevel = %v，期望 LvlWarn", CurrentLevel())
	}
}

// TestSetOutputFile 文件输出：目录内落盘，权限 0600（日志含主机名/UID 等敏感信息）
func TestSetOutputFile(t *testing.T) {
	oldOut := out
	t.Cleanup(func() { SetOutput(oldOut) })

	path := filepath.Join(t.TempDir(), "tshell.log")
	if err := SetOutputFile(path); err != nil {
		t.Fatalf("SetOutputFile 失败: %v", err)
	}
	SetFormat("text")
	SetLevel(LvlInfo)
	Infof("写入文件 %s\n", "ok")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取日志文件失败: %v", err)
	}
	if !strings.Contains(string(data), "写入文件 ok") {
		t.Fatalf("日志未写入文件: %q", string(data))
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat 失败: %v", err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("日志文件权限 = %o，期望 600", perm)
	}
	// 打不开的路径应返回错误（调用方据此回退 stdout 并告警）
	if err := SetOutputFile(filepath.Join(t.TempDir(), "no-such-dir", "x.log")); err == nil {
		t.Fatal("目录不存在时应返回错误")
	}
}
