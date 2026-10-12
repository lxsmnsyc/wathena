package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
)

// clientTypes maps the web client's file extensions to their content
// types. Other files are served as application/octet-stream.
var clientTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json",
	".map":         "application/json",
	".webmanifest": "application/manifest+json",
	".wasm":        "application/wasm",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".webp":        "image/webp",
	".ico":         "image/x-icon",
	".woff2":       "font/woff2",
	".txt":         "text/plain; charset=utf-8",
}

// compressible lists the types worth compressing.
var compressible = map[string]bool{
	".html": true, ".js": true, ".mjs": true, ".css": true, ".json": true,
	".map": true, ".webmanifest": true, ".wasm": true, ".svg": true, ".txt": true,
}

// maxClientBytes stops a wrong client_dir, such as a whole disk, from
// being read into memory.
const maxClientBytes = 64 << 20

type clientFile struct {
	data, gzip, br []byte
	ctype, etag    string
	immutable      bool
}

// clientHandler serves the built web client from memory:
//   - Files under app/ have content hashes in their names and never
//     change, so browsers keep them for a year.
//   - index.html, sw.js and other files keep their names across
//     releases, so browsers check them each time (no-cache with an ETag).
//   - Files are compressed once at startup, with Brotli and gzip.
//   - Every response has the client page's security headers.
//   - /admin serves admin.html, the admin screens' page.
type clientHandler struct {
	files   map[string]*clientFile
	headers http.Header
}

func newClientHandler(fsys fs.FS, csp string, hsts bool) (*clientHandler, error) {
	h := &clientHandler{files: map[string]*clientFile{}, headers: clientHeaders(csp, hsts)}
	total := 0
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		if total += len(data); total > maxClientBytes {
			return errors.New("the web client is larger than 64 MiB")
		}
		ext := path.Ext(p)
		f := &clientFile{data: data, ctype: clientTypes[ext], immutable: strings.HasPrefix(p, "app/")}
		if f.ctype == "" {
			f.ctype = "application/octet-stream"
		}
		sum := sha256.Sum256(data)
		f.etag = `"` + hex.EncodeToString(sum[:12]) + `"`
		if compressible[ext] && len(data) > 1024 {
			f.gzip, f.br = compress(data)
		}
		h.files[p] = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	if h.files["index.html"] == nil {
		return nil, errors.New("the web client has no index.html")
	}
	return h, nil
}

// compress returns data compressed with gzip and Brotli. A form that is
// not smaller is nil.
func compress(data []byte) (gz, br []byte) {
	var b bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	_, _ = zw.Write(data)
	_ = zw.Close()
	if b.Len() < len(data) {
		gz = bytes.Clone(b.Bytes())
	}
	b.Reset()
	bw := brotli.NewWriterLevel(&b, 9)
	_, _ = bw.Write(data)
	_ = bw.Close()
	if b.Len() < len(data) {
		br = bytes.Clone(b.Bytes())
	}
	return gz, br
}

// clientCSP is the client page's Content-Security-Policy:
//   - Scripts and workers only from this origin. WebAssembly may compile
//     ('wasm-unsafe-eval'), which pathfinding needs, but eval stays off.
//   - Fetches and the WebSocket only to this origin. ws is the
//     WebSocket form of the public origin, which older browsers do not
//     match with 'self'.
//   - Images and sounds from this origin and from blob: URLs, which the
//     client makes for item icons and music.
//   - No plugins, no framing, no <base>, and forms only to this origin.
func clientCSP(ws string) string {
	connect := "'self'"
	if ws != "" {
		connect += " " + ws
	}
	return "default-src 'none'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self'" +
		"; connect-src " + connect +
		"; img-src 'self' blob:; media-src 'self' blob:; style-src 'self'; font-src 'self'; manifest-src 'self'" +
		"; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
}

// wsOrigin turns an http(s) origin into its ws(s) form.
func wsOrigin(origin string) string {
	if rest, ok := strings.CutPrefix(origin, "https://"); ok {
		return "wss://" + rest
	}
	if rest, ok := strings.CutPrefix(origin, "http://"); ok {
		return "ws://" + rest
	}
	return ""
}

func clientHeaders(csp string, hsts bool) http.Header {
	h := http.Header{}
	h.Set("Content-Security-Policy", csp)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	if hsts {
		h.Set("Strict-Transport-Security", "max-age=31536000")
	}
	return h
}

func (h *clientHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	for k, v := range h.headers {
		hd[k] = v
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		hd.Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	switch p {
	case "":
		p = "index.html"
	case "admin", "admin/":
		// The admin screens' page. It loads the admin bundle only after
		// the admin API confirms a staff role (decision 0028).
		p = "admin.html"
	}
	f := h.files[p]
	if f == nil {
		http.NotFound(w, r)
		return
	}
	if f.immutable {
		hd.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		hd.Set("Cache-Control", "no-cache")
	}
	hd.Set("Content-Type", f.ctype)
	hd.Set("ETag", f.etag)
	if f.gzip != nil || f.br != nil {
		hd.Set("Vary", "Accept-Encoding")
	}
	if match := r.Header.Get("If-None-Match"); match != "" && etagMatches(match, f.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	body := f.data
	accept := r.Header.Get("Accept-Encoding")
	switch {
	case f.br != nil && accepts(accept, "br"):
		body = f.br
		hd.Set("Content-Encoding", "br")
	case f.gzip != nil && accepts(accept, "gzip"):
		body = f.gzip
		hd.Set("Content-Encoding", "gzip")
	}
	hd.Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

// etagMatches reports whether an If-None-Match header names the ETag.
// The W/ prefix is ignored, as the weak comparison asks.
func etagMatches(header, etag string) bool {
	for v := range strings.SplitSeq(header, ",") {
		v = strings.TrimPrefix(strings.TrimSpace(v), "W/")
		if v == etag || v == "*" {
			return true
		}
	}
	return false
}

// accepts reports whether an Accept-Encoding header allows an encoding.
func accepts(header, enc string) bool {
	for part := range strings.SplitSeq(header, ",") {
		name, params, _ := strings.Cut(part, ";")
		if !strings.EqualFold(strings.TrimSpace(name), enc) {
			continue
		}
		for p := range strings.SplitSeq(params, ";") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
				if q, err := strconv.ParseFloat(v, 64); err == nil && q == 0 {
					return false
				}
			}
		}
		return true
	}
	return false
}

// health answers the container's liveness and readiness checks. The
// answers say only "ok" or "not ready", never why.
type health struct {
	// ready is true from the moment the server listens until shutdown
	// starts.
	ready atomic.Bool
	// checks must all pass for the server to be ready, such as database
	// pings.
	checks []func(context.Context) error
}

// live answers /healthz: the process runs and serves HTTP.
func (h *health) live(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

// readyz answers /readyz: the world runs, the databases answer, and the
// server is not shutting down.
func (h *health) readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	ok := h.ready.Load()
	if ok {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		for _, c := range h.checks {
			if c(ctx) != nil {
				ok = false
				break
			}
		}
	}
	if !ok {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready\n"))
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}
