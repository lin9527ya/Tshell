package proxy

import (
	"bytes"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// TestCheckTunnelProxyAuth HTTP 隧道代理认证：HTTP 侧此前完全没有认证，
// 端口对任何能连上的人开放。配置了密码后必须校验 Proxy-Authorization。
func TestCheckTunnelProxyAuth(t *testing.T) {
	newReq := func(header string) *http.Request {
		r := &http.Request{Header: http.Header{}, Method: http.MethodGet, URL: &url.URL{}}
		if header != "" {
			r.Header.Set("Proxy-Authorization", header)
		}
		return r
	}

	cases := []struct {
		name     string
		password string
		header   string
		want     bool
	}{
		{"未配置密码时不校验", "", "", true},
		{"未配置密码时忽略错误头", "", basicHeader("x", "y"), true},
		{"正确凭据", "s3cret", basicHeader("op", "s3cret"), true},
		{"密码错误", "s3cret", basicHeader("op", "wrong"), false},
		{"用户名错误", "s3cret", basicHeader("other", "s3cret"), false},
		{"缺少认证头", "s3cret", "", false},
		{"非 Basic 方案", "s3cret", "Bearer abc", false},
		{"Base64 非法", "s3cret", "Basic !!!notbase64", false},
		{"缺少冒号分隔", "s3cret", "Basic " + base64.StdEncoding.EncodeToString([]byte("noseparator")), false},
	}
	for _, c := range cases {
		if got := checkTunnelProxyAuth(newReq(c.header), "op", c.password); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// recordConn 记录写入内容的 net.Conn 桩
type recordConn struct{ buf bytes.Buffer }

func (c *recordConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *recordConn) Write(p []byte) (int, error)      { return c.buf.Write(p) }
func (c *recordConn) Close() error                     { return nil }
func (c *recordConn) LocalAddr() net.Addr              { return nil }
func (c *recordConn) RemoteAddr() net.Addr             { return nil }
func (c *recordConn) SetDeadline(time.Time) error      { return nil }
func (c *recordConn) SetReadDeadline(time.Time) error  { return nil }
func (c *recordConn) SetWriteDeadline(time.Time) error { return nil }

// TestWriteProxyAuthRequired 认证失败必须返回 407 并声明 Basic 认证方案
func TestWriteProxyAuthRequired(t *testing.T) {
	c := &recordConn{}
	writeProxyAuthRequired(c)
	out := c.buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 407 ") {
		t.Errorf("应为 407 响应，实际: %q", out)
	}
	if !strings.Contains(out, "Proxy-Authenticate: Basic") {
		t.Errorf("应声明 Basic 认证方案，实际: %q", out)
	}
}
