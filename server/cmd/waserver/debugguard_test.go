package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDebugGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	h := debugGuard(ok)
	for _, c := range []struct {
		name, method, host string
		header             map[string]string
		want               int
	}{
		{"curl", "POST", "127.0.0.1:8301", nil, http.StatusOK},
		{"localhost", "GET", "localhost:8301", nil, http.StatusOK},
		{"IPv6 loopback", "GET", "[::1]:8301", nil, http.StatusOK},
		{"Node fetch", "POST", "127.0.0.1:8301", map[string]string{"Sec-Fetch-Mode": "cors"}, http.StatusOK},
		{"typed address", "GET", "127.0.0.1:8301", map[string]string{"Sec-Fetch-Site": "none"}, http.StatusOK},
		{"pprof link", "GET", "127.0.0.1:8301", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusOK},
		{"same origin", "POST", "127.0.0.1:8301", map[string]string{"Origin": "http://127.0.0.1:8301"}, http.StatusOK},
		// A DNS rebinding page has its own domain in Host.
		{"rebinding", "GET", "evil.example:8301", nil, http.StatusForbidden},
		{"rebinding without port", "GET", "evil.example", nil, http.StatusForbidden},
		// A cross-site form post carries Origin and Sec-Fetch-Site.
		{"cross-site post", "POST", "127.0.0.1:8301", map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"foreign origin", "POST", "127.0.0.1:8301", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"opaque origin", "POST", "127.0.0.1:8301", map[string]string{"Origin": "null"}, http.StatusForbidden},
		{"other local site", "POST", "localhost:8301", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"other local port", "POST", "127.0.0.1:8301", map[string]string{"Origin": "http://127.0.0.1:5173"}, http.StatusForbidden},
	} {
		r := httptest.NewRequest(c.method, "http://"+c.host+"/debug/wathena/staff?name=x&role=admin", nil)
		r.Host = c.host
		for k, v := range c.header {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s: status %d, want %d", c.name, w.Code, c.want)
		}
	}
}
