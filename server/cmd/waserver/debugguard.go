package main

import (
	"net"
	"net/http"
	"strings"
)

// debugGuard refuses requests to the debug server that a web page could
// send (review 2026-10). The debug server listens on loopback, but a page
// in the owner's browser can still reach it:
//   - A cross-site form or fetch sends a POST to http://127.0.0.1:8301
//     without reading the answer. The browser sends Origin with it, and
//     Sec-Fetch-Site: cross-site or same-site.
//   - A DNS rebinding page points its own domain at 127.0.0.1 and reads
//     the answers. Its requests carry its domain in Host.
//
// So a request must name a loopback host, and must not come from another
// page's origin. Tools such as curl, Go's http package and Node's fetch
// send neither header. A page the owner opens by typing its address, such
// as /debug/pprof/, has Sec-Fetch-Site none, and its links same-origin.
func debugGuard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !loopbackHost(strings.ToLower(host)) {
			http.Error(w, "the debug server answers only a loopback host", http.StatusForbidden)
			return
		}
		switch r.Header.Get("Sec-Fetch-Site") {
		case "", "none", "same-origin":
		default:
			http.Error(w, "the debug server answers no other site", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			http.Error(w, "the debug server answers no other origin", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}
