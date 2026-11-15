package httpx

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP returns the address a request really came from; X-Forwarded-For is believed only for hops that are inside trusted.
func ClientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	client := peerAddr(r)
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	// Walk from the hop nearest to us outwards for as long as the address we reached is itself a trusted proxy.
	for i := len(hops) - 1; i >= 0 && inPrefixes(client, trusted); i-- {
		hop, ok := parseHop(hops[i])
		if !ok {
			break
		}
		client = hop
	}
	return client
}

// IsHTTPS reports whether the client reached the server over TLS, believing X-Forwarded-Proto only from a trusted proxy.
func IsHTTPS(r *http.Request, trusted []netip.Prefix) bool {
	if r.TLS != nil {
		return true
	}
	if !inPrefixes(peerAddr(r), trusted) {
		return false
	}
	protos := strings.Split(strings.Join(r.Header.Values("X-Forwarded-Proto"), ","), ",")
	return strings.EqualFold(strings.TrimSpace(protos[len(protos)-1]), "https")
}

// peerAddr is the address of the TCP peer, or the zero Addr when RemoteAddr is unparseable.
func peerAddr(r *http.Request) netip.Addr {
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return ap.Addr().Unmap().WithZone("")
	}
	if a, err := netip.ParseAddr(r.RemoteAddr); err == nil {
		return a.Unmap().WithZone("")
	}
	return netip.Addr{}
}

// parseHop reads one X-Forwarded-For entry, which some proxies write as host:port or [v6]:port.
func parseHop(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if a, err := netip.ParseAddr(s); err == nil {
		return a.Unmap().WithZone(""), true
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		if a, err := netip.ParseAddr(host); err == nil {
			return a.Unmap().WithZone(""), true
		}
	}
	return netip.Addr{}, false
}

func inPrefixes(a netip.Addr, prefixes []netip.Prefix) bool {
	if !a.IsValid() {
		return false
	}
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
