package proxy

import (
	"testing"

	"BackendTemplate/pkg/database"
)

func TestDialableHost(t *testing.T) {
	cases := []struct {
		addr          string
		allowLoopback bool
		want          string
	}{
		{"1.2.3.4:8089", false, "1.2.3.4"},
		{"tcp://1.2.3.4:8089", false, "1.2.3.4"},
		{"[2001:db8::1]:8089", false, "2001:db8::1"},
		{"0.0.0.0:8089", false, ""},
		{"[::]:8089", false, ""},
		{"127.0.0.1:8089", false, ""},
		{"127.0.0.1:8089", true, "127.0.0.1"},
		{"example.com:8089", false, "example.com"},
	}
	for _, c := range cases {
		if got := dialableHost(c.addr, c.allowLoopback); got != c.want {
			t.Errorf("dialableHost(%q, %v) = %q, want %q", c.addr, c.allowLoopback, got, c.want)
		}
	}
}

// TestTunnelAdvertisedHostSkipsWildcard 0.0.0.0 不能直接下发给客户端；
// 应回退到监听地址里的具体 host。
func TestTunnelAdvertisedHostSkipsWildcard(t *testing.T) {
	l := database.Listener{
		ConnectAddress: "0.0.0.0:8089",
		ListenAddress:  "10.1.2.3:8089",
	}
	if got := tunnelAdvertisedHost(l); got != "10.1.2.3" {
		t.Fatalf("应回退到 listen_address 的具体 host，实际 %q", got)
	}

	// 回环 connect 地址也应让位给具体 listen 地址
	l = database.Listener{
		ConnectAddress: "127.0.0.1:8089",
		ListenAddress:  "10.1.2.3:8089",
	}
	if got := tunnelAdvertisedHost(l); got != "10.1.2.3" {
		t.Fatalf("回环地址应被跳过，实际 %q", got)
	}
}
