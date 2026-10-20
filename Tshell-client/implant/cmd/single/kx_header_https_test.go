//go:build transport_https

package main

import (
	"net/http"
	"testing"

	"implant/core/encrypt"
)

// ===== 回归守卫：HTTPS 与 HTTP 同一条不变量 =====
//
// 服务端对每个 HTTPS 请求同样独立解析密钥，所以带 MetaInfo 的每个请求
// 都必须带 X-Kx。详见 kx_header_http_test.go 里的事故说明。

func TestApplyMetadataCarriesKxHeader(t *testing.T) {
	c := &HTTPSClient{}
	c.kxPub = "TEST_PUBLIC_KEY_BASE64"

	req, err := http.NewRequest("GET", "https://example.invalid/", nil)
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

func TestApplyMetadataNoKxHeaderWhenNotExchanged(t *testing.T) {
	c := &HTTPSClient{} // kxPub 为空

	req, err := http.NewRequest("GET", "https://example.invalid/", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.applyMetadata(req)

	if got := req.Header.Get(encrypt.KxHeaderName); got != "" {
		t.Fatalf("未完成交换时不应携带 %s 头（实际 %q）", encrypt.KxHeaderName, got)
	}
}
