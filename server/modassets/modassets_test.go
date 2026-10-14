package modassets_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modassets"
	"github.com/lxsmnsyc/wathena/tools/modkit"
)

const manifest = `id: example.lantern
name: Lantern
version: 0.1.0
api: 1.0
license: CC0-1.0
`

func png24(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 24, 24))
	for i := range 24 {
		img.SetNRGBA(i, i, color.NRGBA{200, 120, 40, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func write(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for p, data := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// newMod makes a mod folder with an icon and a still sprite in art/, and
// builds the art into its assets/ when built is set.
func newMod(t *testing.T, built bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "lantern")
	write(t, dir, map[string][]byte{
		"mod.yaml":                        []byte(manifest),
		"art/pack.yaml":                   []byte("license: CC0-1.0\n"),
		"art/icon/item/Paper_Lantern.png": png24(t),
		"art/sprite/npc/Lamplighter.png":  png24(t),
		"client/skills.yml":               []byte("skills:\n  - {id: 5, constant: SM_BASH, hit: [1, 17]}\n"),
	})
	if built {
		if _, err := modkit.BuildArt(dir, "", 1); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// mods resolves the mods at paths, without the game data.
func mods(t *testing.T, paths ...string) *gamedata.Mods {
	t.Helper()
	var found []*mod.Mod
	for _, p := range paths {
		m, err := mod.Read(p)
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, m)
	}
	set, err := mod.Resolve(found, mod.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return &gamedata.Mods{Set: set}
}

func load(t *testing.T, m *gamedata.Mods, opts modassets.Options) *modassets.Assets {
	t.Helper()
	a, err := modassets.Load(m, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

func get(t *testing.T, h http.Handler, p string, header ...string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, p, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

// storedFile returns the stored path of a name in a build folder.
func storedFile(t *testing.T, dir, name string) string {
	t.Helper()
	list, err := build.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := list[name]
	if !ok {
		t.Fatalf("%s is not in the build", name)
	}
	ext := map[string]string{"texture": "webp", "sprite": "wspr"}[e.Type]
	return filepath.Join(dir, build.StoredPath(e.Hash, ext))
}

func TestServeModBuild(t *testing.T) {
	dir := newMod(t, true)
	a := load(t, mods(t, dir), modassets.Options{})
	if len(a.Builds) != 1 || a.Builds[0].URL != "/assets/mods/example.lantern/0.1.0/" {
		t.Fatalf("builds %+v", a.Builds)
	}
	var doc modassets.ClientData
	if err := json.Unmarshal(a.ClientData, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 || len(doc.Mods) != 1 || doc.Mods[0].Assets != "/assets/mods/example.lantern/0.1.0/" {
		t.Errorf("client data %s", a.ClientData)
	}
	if doc.Tables == nil || !strings.Contains(string(a.ClientData), `"hit":[1,17]`) {
		t.Errorf("client data lacks the mod's table: %s", a.ClientData)
	}

	// The manifest, as core's, is checked each time.
	res := get(t, a, "/assets/mods/example.lantern/0.1.0/manifest.json")
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-cache" || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("manifest: %d %v", res.StatusCode, res.Header)
	}
	// A sprite is Brotli, and kept a year.
	p := storedFile(t, filepath.Join(dir, "assets"), "mod/example.lantern/sprite/npc/Lamplighter")
	rel, _ := filepath.Rel(filepath.Join(dir, "assets"), strings.TrimSuffix(p, ".br"))
	res = get(t, a, "/assets/mods/example.lantern/0.1.0/"+filepath.ToSlash(rel), "Accept-Encoding", "br")
	if res.StatusCode != 200 || res.Header.Get("Content-Encoding") != "br" || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("sprite: %d %v", res.StatusCode, res.Header)
	}
	body, _ := io.ReadAll(brotli.NewReader(res.Body))
	if !bytes.HasPrefix(body, []byte("WSPR")) {
		t.Errorf("sprite body %q", body[:min(8, len(body))])
	}
	for _, bad := range []string{
		"/assets/mods/example.lantern/0.2.0/manifest.json",
		"/assets/mods/example.other/0.1.0/manifest.json",
		"/assets/mods/example.lantern/0.1.0/../mod.yaml",
		"/assets/mods/example.lantern/0.1.0/.cache.json",
	} {
		if res := get(t, a, bad); res.StatusCode != 404 {
			t.Errorf("%s: %d", bad, res.StatusCode)
		}
	}

	// The client data is checked each time, with its ETag.
	res = get(t, a, modassets.ClientDataPath)
	etag := res.Header.Get("ETag")
	if res.StatusCode != 200 || etag == "" || res.Header.Get("Cache-Control") != "no-cache" || res.Header.Get("Content-Type") != "application/json" {
		t.Errorf("client data: %d %v", res.StatusCode, res.Header)
	}
	if res := get(t, a, modassets.ClientDataPath, "If-None-Match", etag); res.StatusCode != http.StatusNotModified {
		t.Errorf("client data again: %d", res.StatusCode)
	}
}

func TestServeZipMod(t *testing.T) {
	dir := newMod(t, false)
	out := t.TempDir()
	zipPath, _, _, err := modkit.Pack(dir, out)
	if err != nil {
		t.Fatal(err)
	}
	// The same files pack to the same bytes, build included.
	again, _, sum2, err := modkit.Pack(dir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _, sum1, _ := modkit.Pack(dir, out)
	if sum1 != sum2 {
		t.Errorf("packing twice gave %s and %s (%s)", sum1, sum2, again)
	}
	a := load(t, mods(t, zipPath), modassets.Options{})
	if len(a.Builds) != 1 || len(a.Builds[0].Names) != 2 {
		t.Fatalf("builds %+v", a.Builds)
	}
	res := get(t, a, "/assets/mods/example.lantern/0.1.0/manifest.json")
	if res.StatusCode != 200 {
		t.Errorf("manifest from the zip: %d", res.StatusCode)
	}
	// An icon is WebP, sent as it is, with ranges.
	var icon string
	list := a.Builds[0].Names
	for n := range list {
		if strings.Contains(n, "icon/item") {
			icon = n
		}
	}
	if icon == "" {
		t.Fatal("no icon")
	}
	var doc modassets.ClientData
	_ = json.Unmarshal(a.ClientData, &doc)
	if len(doc.Mods) != 1 {
		t.Errorf("client data mods %v", doc.Mods)
	}
}

func TestRefusedBuilds(t *testing.T) {
	cases := map[string]struct {
		change func(t *testing.T, dir string)
		opts   modassets.Options
		want   string
	}{
		"a corrupt file": {
			change: func(t *testing.T, dir string) {
				p := storedFile(t, filepath.Join(dir, "assets"), "mod/example.lantern/sprite/npc/Lamplighter")
				if err := os.WriteFile(p, []byte("not brotli at all"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "problems",
		},
		"a file with another hash": {
			change: func(t *testing.T, dir string) {
				p := storedFile(t, filepath.Join(dir, "assets"), "mod/example.lantern/icon/item/Paper_Lantern")
				data, err := os.ReadFile(p) // #nosec G304 -- a test file.
				if err != nil {
					t.Fatal(err)
				}
				data[len(data)-1] ^= 0xff
				if err := os.WriteFile(p, data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "does not match its hash",
		},
		"a missing file": {
			change: func(t *testing.T, dir string) {
				if err := os.Remove(storedFile(t, filepath.Join(dir, "assets"), "mod/example.lantern/icon/item/Paper_Lantern")); err != nil {
					t.Fatal(err)
				}
			},
			want: "problems",
		},
		"a build over the limit": {
			opts: modassets.Options{MaxBytes: 100},
			want: "over the limit of 100",
		},
		"another mod's names": {
			change: func(t *testing.T, dir string) {
				src := modkit.ArtSource(dir, "example.other")
				if _, err := build.Run(build.Options{Out: filepath.Join(dir, "assets"), Sources: []build.Source{src}, Brotli: 1}); err != nil {
					t.Fatal(err)
				}
			},
			want: "is not under mod/example.lantern/",
		},
		"a bad client table": {
			change: func(t *testing.T, dir string) {
				write(t, dir, map[string][]byte{"client/skills.yml": []byte("skills: [{id: 5}]\n")})
			},
			want: "client/skills.yml",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			dir := newMod(t, true)
			if c.change != nil {
				c.change(t, dir)
			}
			_, err := modassets.Load(mods(t, dir), c.opts)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err %v, want %q", err, c.want)
			}
		})
	}
}

func TestArtWithoutBuild(t *testing.T) {
	var logs bytes.Buffer
	dir := newMod(t, false)
	a := load(t, mods(t, dir), modassets.Options{Log: slog.New(slog.NewTextHandler(&logs, nil))})
	if len(a.Builds) != 0 || !strings.Contains(logs.String(), "no asset build") {
		t.Errorf("builds %v, log %s", a.Builds, logs.String())
	}
	if res := get(t, a, "/assets/mods/example.lantern/0.1.0/manifest.json"); res.StatusCode != 404 {
		t.Errorf("manifest: %d", res.StatusCode)
	}
}
