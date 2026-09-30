package serve

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
)

// placeholderBuild builds the placeholder pack and returns the folder and
// the hashes of its sprite and texture.
func placeholderBuild(t *testing.T) (dir, sprite, texture string) {
	t.Helper()
	dir = t.TempDir()
	if _, err := build.Run(build.Options{Out: dir, Brotli: 1, Sources: []build.Source{build.Placeholder{}}}); err != nil {
		t.Fatal(err)
	}
	var m build.Manifest
	readJSON(t, filepath.Join(dir, "manifest.json"), &m)
	find := func(name string) string {
		h := m.Shards[build.ShardOf(name)]
		var shard map[string]struct{ H string }
		readJSON(t, filepath.Join(dir, h[:2], h+".json.br"), &shard)
		return shard[name].H
	}
	return dir, find("placeholder/sprite"), find("placeholder/texture")
}

func readJSON(t *testing.T, p string, v any) {
	t.Helper()
	b, err := os.ReadFile(p) // #nosec G304 -- test file.
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(p, ".br") {
		if b, err = io.ReadAll(brotli.NewReader(bytes.NewReader(b))); err != nil {
			t.Fatal(err)
		}
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, h http.Handler, method, p, encoding string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, p, nil)
	if encoding != "" {
		req.Header.Set("Accept-Encoding", encoding)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func TestServe(t *testing.T) {
	dir, sprite, texture := placeholderBuild(t)
	h, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close() }()

	res := get(t, h, "GET", "/manifest.json", "")
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-cache" || res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("manifest: %d %v", res.StatusCode, res.Header)
	}

	// A binary format is sent as Brotli to clients that accept it.
	sp := "/" + sprite[:2] + "/" + sprite + ".wspr"
	res = get(t, h, "GET", sp, "gzip, deflate, br")
	if res.StatusCode != 200 || res.Header.Get("Content-Encoding") != "br" || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("sprite: %d %v", res.StatusCode, res.Header)
	}
	body, _ := io.ReadAll(brotli.NewReader(res.Body))
	if sum := sha256.Sum256(body); hex.EncodeToString(sum[:]) != sprite {
		t.Error("sprite does not match its hash")
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" || res.Header.Get("Vary") != "Accept-Encoding" || res.ContentLength <= 0 {
		t.Errorf("sprite headers %v", res.Header)
	}

	// Others get it decompressed.
	res = get(t, h, "GET", sp, "gzip, br;q=0")
	body, _ = io.ReadAll(res.Body)
	if sum := sha256.Sum256(body); res.Header.Get("Content-Encoding") != "" || hex.EncodeToString(sum[:]) != sprite {
		t.Errorf("decompressed sprite: %v", res.Header)
	}

	// Textures are sent as they are.
	res = get(t, h, "GET", "/"+texture[:2]+"/"+texture+".webp", "br")
	body, _ = io.ReadAll(res.Body)
	if sum := sha256.Sum256(body); res.Header.Get("Content-Type") != "image/webp" || res.Header.Get("Content-Encoding") != "" || hex.EncodeToString(sum[:]) != texture {
		t.Errorf("texture: %v", res.Header)
	}

	for _, p := range []string{
		"/",
		"/.cache.json",
		"/" + sprite[:2],
		"/" + sprite[:2] + "/" + sprite + ".wspr.br",
		"/" + sprite[:2] + "/" + sprite + ".exe",
		"/00/" + sprite + ".wspr",
		"/" + sprite[:2] + "/" + strings.ToUpper(sprite) + ".wspr",
		"/" + sprite[:2] + "/../manifest.json",
		"/" + strings.Repeat("0", 2) + "/" + strings.Repeat("0", 64) + ".wspr",
	} {
		res := get(t, h, "GET", p, "br")
		if res.StatusCode != 404 || res.Header.Get("Content-Encoding") != "" {
			t.Errorf("%s: %d %v", p, res.StatusCode, res.Header)
		}
	}
	if res := get(t, h, "POST", "/manifest.json", ""); res.StatusCode != 405 {
		t.Errorf("POST: %d", res.StatusCode)
	}
}

func TestAcceptsBrotli(t *testing.T) {
	for header, want := range map[string]bool{
		"":                   false,
		"br":                 true,
		"gzip, deflate, br":  true,
		"gzip;q=1, br;q=0.5": true,
		"br;q=0":             false,
		"brotli":             false,
		"*":                  true,
	} {
		if got := acceptsBrotli(header); got != want {
			t.Errorf("%q: %v", header, got)
		}
	}
}
