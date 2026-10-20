package tcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===== 源码棘轮：firstBlood 段不得重新取静态监听器密钥 =====
//
// 2026-10-20 的故障根因是"同一个密钥解析做了两次"：
//
//	MetaInfo 用 KX 派生密钥（resolveMetaKey / resolveWSMetaKey 解析），
//	会话密钥却在该分支里重新调 GetListenerKeyConfigForForward 取静态密钥。
//
// 结果 MetaInfo 能解开（客户端正常上线），会话密钥两侧不同（所有功能失效）。
//
// tcp 有端到端用例（kx_e2e_test.go）直接证明密钥一致；kcp 与 websocket
// 协议不同、复刻 e2e 成本高，所以这里用**调用点计数**做棘轮：
// 每个文件里监听器密钥解析的出现次数被钉住，新增即失败，
// 迫使改动者想清楚"这里是不是又开了第二个解析点"。
//
// 计数含义（都在唯一解析点或入口函数里）：
//
//	tcp.go       : resolveMetaKey 内 2 处（KX 分支取盐；非 KX 分支取密钥+盐）
//	kcp.go       : 入口解析 1 处
//	websocket.go : 入口解析 + 正向入口 + 盐回落 + 按地址解析，共 4 处
var listenerKeyLookupBudget = map[string]int{
	filepath.Join("tcp.go"):                          2,
	filepath.Join("..", "kcp", "kcp.go"):             1,
	filepath.Join("..", "websocket", "websocket.go"): 4,
}

func TestNoExtraListenerKeyLookupsInSessionKeyPath(t *testing.T) {
	for path, want := range listenerKeyLookupBudget {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v（测试需在 pkg/connection/tcp 目录下运行）", path, err)
		}
		src := string(data)
		got := strings.Count(src, "GetListenerKeyConfigForForward(") +
			strings.Count(src, "GetListenerKeyConfig(") +
			strings.Count(src, "GetListenerKeyConfigByAddress(")
		if got != want {
			t.Errorf("%s 的监听器密钥解析调用点 = %d，期望 %d。\n"+
				"若这是**新增**调用，先确认它是否在会话密钥路径上重复解析了密钥 —— "+
				"firstBlood 必须复用 resolveMetaKey/resolveWSMetaKey 的结果，"+
				"否则会重现「能上线但所有功能失效」；确认无误后再更新本棘轮的预算。",
				path, got, want)
		}
	}
}
