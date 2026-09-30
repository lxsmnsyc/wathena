// Package serve serves an asset build over HTTP, as described in
// docs/specs/assets-v1.md, "Serving".
package serve

import (
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
)

// contentTypes maps stored extensions to the only content type each is
// served with.
var contentTypes = map[string]string{
	"json": "application/json",
	"wspr": "application/octet-stream",
	"wpal": "application/octet-stream",
	"wmap": "application/octet-stream",
	"weff": "application/octet-stream",
	"glb":  "model/gltf-binary",
	"webp": "image/webp",
	"webm": "audio/webm",
	"mp3":  "audio/mpeg",
}

// Handler serves a build folder. It serves manifest.json and files named
// by a content hash, and nothing else.
type Handler struct {
	root *os.Root
}

// New opens a build folder.
func New(dir string) (*Handler, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &Handler{root: root}, nil
}

// Close closes the build folder.
func (h *Handler) Close() error { return h.root.Close() }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hd := w.Header()
	hd.Set("X-Content-Type-Options", "nosniff")
	// Assets are public, and a CDN or dev server on another origin may
	// fetch them.
	hd.Set("Access-Control-Allow-Origin", "*")

	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "manifest.json" {
		// The manifest is the only file that changes between builds.
		hd.Set("Cache-Control", "no-cache")
		h.serveFile(w, r, p, "json")
		return
	}
	ext, ok := hashPath(p)
	if !ok {
		http.NotFound(w, r)
		return
	}
	hd.Set("Cache-Control", "public, max-age=31536000, immutable")
	if _, br := build.Stored(ext); !br {
		h.serveFile(w, r, p, ext)
		return
	}
	hd.Set("Vary", "Accept-Encoding")
	if acceptsBrotli(r.Header.Get("Accept-Encoding")) {
		h.serveEncoded(w, r, p+".br", ext)
		return
	}
	// Few clients lack Brotli, so decompressing for them is fine.
	f, err := h.root.Open(p + ".br")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	hd.Set("Content-Type", contentTypes[ext])
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, brotli.NewReader(f))
}

// serveFile serves a stored file. It supports range requests, which
// browsers use for music.
func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, p, ext string) {
	f, err := h.root.Open(p)
	if err != nil {
		w.Header().Del("Content-Encoding")
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		w.Header().Del("Content-Encoding")
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentTypes[ext])
	// Hashed files never change, so their modification time says nothing.
	http.ServeContent(w, r, "", time.Time{}, f)
}

// serveEncoded sends a stored Brotli file as it is. http.ServeContent
// leaves out Content-Length when Content-Encoding is set, which would
// send it in chunks and hide its size from download progress, so this
// sends the whole file with its length. Range requests get the whole
// file, which HTTP allows. Only music needs ranges, and it is not stored
// as Brotli.
func (h *Handler) serveEncoded(w http.ResponseWriter, r *http.Request, p, ext string) {
	f, err := h.root.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	hd := w.Header()
	hd.Set("Content-Encoding", "br")
	hd.Set("Content-Type", contentTypes[ext])
	hd.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, f)
}

// hashPath checks that p is "<2 hex>/<64 hex>.<ext>" with the first two
// digits of the hash and a known extension, and returns the extension.
func hashPath(p string) (string, bool) {
	dir, file, ok := strings.Cut(p, "/")
	if !ok || len(dir) != 2 {
		return "", false
	}
	ext := path.Ext(file)
	hash := strings.TrimSuffix(file, ext)
	ext = strings.TrimPrefix(ext, ".")
	if len(hash) != 64 || hash[:2] != dir || !lowerHex(hash) {
		return "", false
	}
	if _, ok := contentTypes[ext]; !ok {
		return "", false
	}
	return ext, true
}

func lowerHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// acceptsBrotli reports whether an Accept-Encoding header allows "br".
func acceptsBrotli(header string) bool {
	for part := range strings.SplitSeq(header, ",") {
		name, params, _ := strings.Cut(part, ";")
		name = strings.TrimSpace(name)
		if name != "br" && name != "*" {
			continue
		}
		q := 1.0
		for p := range strings.SplitSeq(params, ";") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					q = f
				}
			}
		}
		if q > 0 {
			return true
		}
	}
	return false
}
