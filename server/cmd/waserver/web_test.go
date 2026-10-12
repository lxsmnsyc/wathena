package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
)

func testClient(t *testing.T) *clientHandler {
	t.Helper()
	js := strings.Repeat("console.log('wathena');\n", 200)
	h, err := newClientHandler(fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>wAthena</title>")},
		"admin.html":           {Data: []byte("<!doctype html><title>wAthena admin</title>")},
		"sw.js":                {Data: []byte(js)},
		"app/main-abc123.js":   {Data: []byte(js)},
		"app/path-abc123.wasm": {Data: []byte("\x00asm\x01\x00\x00\x00")},
	}, clientCSP(wsOrigin("https://game.example.com")), true)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(h http.Handler, method, path string, header ...string) *http.Response {
	req := httptest.NewRequest(method, path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestClientCaching(t *testing.T) {
	h := testClient(t)
	for _, c := range []struct {
		path, ctype, cache string
	}{
		{"/", "text/html; charset=utf-8", "no-cache"},
		{"/index.html", "text/html; charset=utf-8", "no-cache"},
		{"/admin", "text/html; charset=utf-8", "no-cache"},
		{"/admin/", "text/html; charset=utf-8", "no-cache"},
		{"/sw.js", "text/javascript; charset=utf-8", "no-cache"},
		{"/app/main-abc123.js", "text/javascript; charset=utf-8", "public, max-age=31536000, immutable"},
		{"/app/path-abc123.wasm", "application/wasm", "public, max-age=31536000, immutable"},
	} {
		resp := get(h, http.MethodGet, c.path)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d", c.path, resp.StatusCode)
			continue
		}
		if got := resp.Header.Get("Content-Type"); got != c.ctype {
			t.Errorf("%s: Content-Type %q", c.path, got)
		}
		if got := resp.Header.Get("Cache-Control"); got != c.cache {
			t.Errorf("%s: Cache-Control %q", c.path, got)
		}
		for _, k := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Strict-Transport-Security", "ETag"} {
			if resp.Header.Get(k) == "" {
				t.Errorf("%s: no %s", c.path, k)
			}
		}
	}
	if b, _ := io.ReadAll(get(h, http.MethodGet, "/admin").Body); !strings.Contains(string(b), "wAthena admin") {
		t.Errorf("/admin serves %q", b)
	}
	for _, p := range []string{"/app/", "/app", "/nope.js", "/../go.mod", "/account", "/admin/staff"} {
		if resp := get(h, http.MethodGet, p); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d", p, resp.StatusCode)
		}
	}
	if resp := get(h, http.MethodPost, "/"); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d", resp.StatusCode)
	}
	etag := get(h, http.MethodGet, "/sw.js").Header.Get("ETag")
	if resp := get(h, http.MethodGet, "/sw.js", "If-None-Match", etag); resp.StatusCode != http.StatusNotModified {
		t.Errorf("If-None-Match: status %d", resp.StatusCode)
	}
}

func TestClientCSP(t *testing.T) {
	csp := clientCSP("wss://game.example.com")
	for _, want := range []string{"script-src 'self' 'wasm-unsafe-eval'", "connect-src 'self' wss://game.example.com", "frame-ancestors 'none'", "default-src 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("no %q in %s", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "'unsafe-eval'") {
		t.Errorf("unsafe source in %s", csp)
	}
	if wsOrigin("http://localhost:8300") != "ws://localhost:8300" || wsOrigin("") != "" {
		t.Error("wsOrigin")
	}
}

func TestClientCompression(t *testing.T) {
	h := testClient(t)
	want := strings.Repeat("console.log('wathena');\n", 200)
	for _, c := range []struct {
		accept, enc string
		read        func(io.Reader) io.Reader
	}{
		{"gzip, deflate, br", "br", func(r io.Reader) io.Reader { return brotli.NewReader(r) }},
		{"gzip", "gzip", func(r io.Reader) io.Reader { z, _ := gzip.NewReader(r); return z }},
		{"br;q=0, gzip", "gzip", func(r io.Reader) io.Reader { z, _ := gzip.NewReader(r); return z }},
		{"", "", func(r io.Reader) io.Reader { return r }},
	} {
		resp := get(h, http.MethodGet, "/app/main-abc123.js", "Accept-Encoding", c.accept)
		if got := resp.Header.Get("Content-Encoding"); got != c.enc {
			t.Errorf("%q: Content-Encoding %q", c.accept, got)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		plain, err := io.ReadAll(c.read(bytes.NewReader(body)))
		if err != nil || string(plain) != want {
			t.Errorf("%q: body differs: %v", c.accept, err)
		}
	}
}

func TestClientNeedsIndex(t *testing.T) {
	if _, err := newClientHandler(fstest.MapFS{"sw.js": {Data: []byte("x")}}, "", false); err == nil {
		t.Error("a client without index.html was accepted")
	}
}

func TestHealth(t *testing.T) {
	h := &health{}
	if resp := get(http.HandlerFunc(h.live), http.MethodGet, "/healthz"); resp.StatusCode != http.StatusOK {
		t.Errorf("live: %d", resp.StatusCode)
	}
	ready := func() int { return get(http.HandlerFunc(h.readyz), http.MethodGet, "/readyz").StatusCode }
	if got := ready(); got != http.StatusServiceUnavailable {
		t.Errorf("before start: %d", got)
	}
	h.ready.Store(true)
	if got := ready(); got != http.StatusOK {
		t.Errorf("started: %d", got)
	}
	h.checks = append(h.checks, func(context.Context) error { return errors.New("database down") })
	resp := get(http.HandlerFunc(h.readyz), http.MethodGet, "/readyz")
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusServiceUnavailable || strings.Contains(string(body), "database") {
		t.Errorf("failed check: %d %q", resp.StatusCode, body)
	}
}
