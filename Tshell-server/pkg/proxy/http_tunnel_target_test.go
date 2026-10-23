package proxy

import "testing"

func TestWithDefaultPort(t *testing.T) {
	cases := []struct{ host, port, want string }{
		{"example.com", "80", "example.com:80"},
		{"example.com:8080", "80", "example.com:8080"},
		{"[2001:db8::1]", "443", "[2001:db8::1]:443"},
		{"[2001:db8::1]:8443", "443", "[2001:db8::1]:8443"},
		{"2001:db8::1", "443", "[2001:db8::1]:443"},
		{"", "80", ""},
	}
	for _, c := range cases {
		if got := withDefaultPort(c.host, c.port); got != c.want {
			t.Errorf("withDefaultPort(%q,%q) = %q, want %q", c.host, c.port, got, c.want)
		}
	}
}
