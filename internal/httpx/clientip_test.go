package httpx

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func prefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	var out []netip.Prefix
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

func reqFrom(remote string, headers map[string][]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for k, vs := range headers {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	return r
}

func TestClientIP(t *testing.T) {
	proxies := prefixes(t, "10.0.0.0/8", "fd00::/8")
	tests := []struct {
		name    string
		remote  string
		xff     []string
		trusted []netip.Prefix
		want    string
	}{
		{"no proxies configured ignores the header", "203.0.113.5:4000", []string{"198.51.100.9"}, nil, "203.0.113.5"},
		{"untrusted peer cannot set the header", "203.0.113.5:4000", []string{"198.51.100.9"}, proxies, "203.0.113.5"},
		{"trusted proxy forwards the client", "10.0.0.2:4000", []string{"198.51.100.9"}, proxies, "198.51.100.9"},
		{"client-supplied prefix is skipped", "10.0.0.2:4000", []string{"1.2.3.4, 198.51.100.9"}, proxies, "198.51.100.9"},
		{"chain of trusted proxies", "10.0.0.2:4000", []string{"198.51.100.9, 10.0.0.7"}, proxies, "198.51.100.9"},
		{"several header lines are one chain", "10.0.0.2:4000", []string{"1.2.3.4", "198.51.100.9"}, proxies, "198.51.100.9"},
		{"all hops trusted gives the leftmost", "10.0.0.2:4000", []string{"10.0.0.9, 10.0.0.7"}, proxies, "10.0.0.9"},
		{"garbage hop stops the walk", "10.0.0.2:4000", []string{"198.51.100.9, nonsense"}, proxies, "10.0.0.2"},
		{"missing header falls back to the peer", "10.0.0.2:4000", nil, proxies, "10.0.0.2"},
		{"hop with a port", "10.0.0.2:4000", []string{"198.51.100.9:5555"}, proxies, "198.51.100.9"},
		{"ipv6 client", "[fd00::2]:4000", []string{"2001:db8::7"}, proxies, "2001:db8::7"},
		{"ipv4-mapped peer matches an ipv4 prefix", "[::ffff:10.0.0.2]:4000", []string{"198.51.100.9"}, proxies, "198.51.100.9"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var headers map[string][]string
			if tc.xff != nil {
				headers = map[string][]string{"X-Forwarded-For": tc.xff}
			}
			got := ClientIP(reqFrom(tc.remote, headers), tc.trusted)
			if got.String() != tc.want {
				t.Errorf("ClientIP = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClientIPWithUnparseablePeer(t *testing.T) {
	if got := ClientIP(reqFrom("@", nil), nil); got.IsValid() {
		t.Errorf("ClientIP = %s, want the zero address", got)
	}
}

func TestIsHTTPS(t *testing.T) {
	proxies := prefixes(t, "10.0.0.0/8")
	proto := func(v ...string) map[string][]string { return map[string][]string{"X-Forwarded-Proto": v} }

	if IsHTTPS(reqFrom("203.0.113.5:1", nil), proxies) {
		t.Error("plain request reported as https")
	}
	if !IsHTTPS(reqFrom("10.0.0.2:1", proto("https")), proxies) {
		t.Error("trusted proxy saying https was not believed")
	}
	if IsHTTPS(reqFrom("203.0.113.5:1", proto("https")), proxies) {
		t.Error("untrusted peer's X-Forwarded-Proto was believed")
	}
	if IsHTTPS(reqFrom("10.0.0.2:1", proto("http")), proxies) {
		t.Error("http reported as https")
	}
	if !IsHTTPS(reqFrom("10.0.0.2:1", proto("http, HTTPS")), proxies) {
		t.Error("the nearest proxy's value should win")
	}
	if IsHTTPS(reqFrom("10.0.0.2:1", proto("https")), nil) {
		t.Error("no trusted proxies configured, header must be ignored")
	}
	direct := reqFrom("203.0.113.5:1", nil)
	direct.TLS = &tls.ConnectionState{}
	if !IsHTTPS(direct, nil) {
		t.Error("direct TLS not reported as https")
	}
}
