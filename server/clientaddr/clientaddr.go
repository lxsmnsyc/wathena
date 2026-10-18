// Package clientaddr finds a request's client address through trusted
// proxies. The account service and the gateway use it, so their rate
// limits key on the same address.
package clientaddr

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// From returns the client's address of r. A request from a trusted
// proxy is followed back through X-Forwarded-For, from the right, past
// every trusted proxy. A remote address that does not parse gives the
// unspecified IPv4 address.
func From(r *http.Request, trusted []netip.Prefix) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.IPv4Unspecified()
	}
	addr = addr.Unmap()
	if !in(trusted, addr) {
		return addr
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return addr
		}
		addr = a.Unmap()
		if !in(trusted, addr) {
			return addr
		}
	}
	return addr
}

func in(trusted []netip.Prefix, a netip.Addr) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Key groups addresses for rate limits: an IPv4 address alone, and an
// IPv6 address by its /64, the block one customer usually gets, so one
// client cannot spread over many addresses.
func Key(a netip.Addr) netip.Prefix {
	a = a.Unmap()
	bits := 32
	if a.Is6() {
		bits = 64
	}
	p, err := a.Prefix(bits)
	if err != nil {
		return netip.Prefix{}
	}
	return p
}
