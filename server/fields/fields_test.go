//go:build procedural

package fields

import (
	"bytes"
	"compress/gzip"
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

	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func testSet(t *testing.T) (*Set, rng.Node) {
	t.Helper()
	g, ok := gen.Lookup(2)
	if !ok {
		t.Fatal("no generator 2")
	}
	root, err := rng.NewRoot([]byte("fields-test"))
	if err != nil {
		t.Fatal(err)
	}
	public := gen.PublicBranch(root)
	fs, err := Generate(g, public, []Spec{{X: 0, Y: 0}, {X: 1, Y: 0, Biome: "desert"}, {X: 0, Y: 1, Biome: "snow"}}, 64)
	if err != nil {
		t.Fatal(err)
	}
	tilesets, err := LoadTilesets(g, "")
	if err != nil {
		t.Fatal(err)
	}
	return NewSet(fs, tilesets.JSON()), public
}

// TestGenerate: each field comes from its public node under the public
// branch, and a client that generates from the node it is sent gets the
// same checksum and WMAP.
func TestGenerate(t *testing.T) {
	s, public := testSet(t)
	g, _ := gen.Lookup(2)
	if len(s.Fields()) != 3 || s.Get("gen:field:1:0") == nil || s.Get("gen:field:9:9") != nil {
		t.Fatalf("fields %d", len(s.Fields()))
	}
	for _, f := range s.Fields() {
		node := gen.FieldNode(public, f.X, f.Y)
		if f.Key != node.Key() || f.ID != gen.FieldID(f.X, f.Y) {
			t.Errorf("%s: key %x", f.ID, f.Key)
		}
		// The client's side: only the key, the size and the biome.
		got, err := g.Field(rng.FromKey(f.Key), gen.Params{Width: 64, Height: 64, Biome: f.Biome})
		if err != nil || got.Checksum() != f.Checksum || sha256.Sum256(got.WMAP) != f.WMAPSum {
			t.Errorf("%s: the client's field differs: %v", f.ID, err)
		}
		grid := f.Grid()
		for i, c := range f.Field.Types {
			walk := c == gen.CellWalkable || c == gen.CellWater
			if (grid.Cells[i]&path.Walkable != 0) != walk {
				t.Fatalf("%s cell %d: type %d, flags %d", f.ID, i, c, grid.Cells[i])
			}
		}
	}
	if s.Get("gen:field:1:0").BiomeName != "desert" || s.Get("gen:field:0:1").BiomeName != "snow" {
		t.Error("a biome was not the one asked for")
	}
	if want := g.Biomes[g.Climate(public, 0, 0)]; s.Get("gen:field:0:0").BiomeName != want {
		t.Errorf("field 0:0 is %s, the climate says %s", s.Get("gen:field:0:0").BiomeName, want)
	}
	if _, err := Generate(g, public, []Spec{{X: 0, Y: 0, Biome: "moon"}}, 64); err == nil || !strings.Contains(err.Error(), "moon") {
		t.Errorf("an unknown biome: %v", err)
	}
	if _, err := Generate(g, public, []Spec{{X: 2, Y: 2}, {X: 2, Y: 2}}, 64); err == nil {
		t.Error("a field listed twice was taken")
	}
	if _, err := Generate(g, public, []Spec{{X: 2, Y: 2}}, 8); err == nil {
		t.Error("an 8-cell field was made")
	}
	v1, _ := gen.Lookup(1)
	if _, err := Generate(v1, public, []Spec{{}}, 64); err == nil {
		t.Error("generator 1 made fields")
	}
}

// TestServe: the info, the WMAP with gzip and ETag, the tileset table,
// and refusals.
func TestServe(t *testing.T) {
	s, _ := testSet(t)
	srv := httptest.NewServer(s)
	defer srv.Close()
	f := s.Get("gen:field:0:0")

	res, err := http.Get(srv.URL + "/gen/maps/gen:field:0:0.json")
	if err != nil {
		t.Fatal(err)
	}
	var info Info
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.Header.Get("Access-Control-Allow-Origin") != "*" || info.ID != f.ID || info.Version != 2 || info.RNG != rng.Version ||
		info.Key != hex.EncodeToString(f.Key[:]) || info.Checksum != hex.EncodeToString(f.Checksum[:]) ||
		info.Width != 64 || info.Height != 64 || info.Biome != f.Biome || info.WMAPBytes != len(f.Field.WMAP) ||
		info.WMAP != "/gen/maps/gen:field:0:0.wmap" {
		t.Errorf("info %+v, headers %v", info, res.Header)
	}

	get := func(path string, header map[string]string) (*http.Response, []byte) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		// The transport must not undo the gzip itself.
		tr := &http.Transport{DisableCompression: true}
		res, err := (&http.Client{Transport: tr}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res, b
	}
	res, body := get(info.WMAP, nil)
	if res.StatusCode != http.StatusOK || !bytes.Equal(body, f.Field.WMAP) || res.Header.Get("Content-Encoding") != "" {
		t.Errorf("wmap: %d, %d bytes", res.StatusCode, len(body))
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != info.WMAPSHA256 {
		t.Error("the WMAP's SHA-256 differs from the info")
	}
	res, body = get(info.WMAP, map[string]string{"Accept-Encoding": "br, gzip"})
	if res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("no gzip: %v", res.Header)
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if plain, _ := io.ReadAll(zr); !bytes.Equal(plain, f.Field.WMAP) {
		t.Error("the gzip WMAP differs")
	}
	if res, _ := get(info.WMAP, map[string]string{"If-None-Match": res.Header.Get("ETag")}); res.StatusCode != http.StatusNotModified {
		t.Errorf("If-None-Match: %d", res.StatusCode)
	}
	res, body = get("/gen/tilesets.json", nil)
	var ts Tilesets
	if err := json.Unmarshal(body, &ts); err != nil || ts.Version != 1 || ts.Aliases["texture/gen/grassland/grass"] != "placeholder/ground/grass" {
		t.Errorf("tilesets %d: %+v %v", res.StatusCode, ts, err)
	}
	for _, p := range []string{"/gen/maps/gen:field:9:9.json", "/gen/maps/gen:field:9:9.wmap", "/gen/maps/x", "/gen/other"} {
		if res, _ := get(p, nil); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", p, res.StatusCode)
		}
	}
	if res, err := http.Post(srv.URL+info.WMAP, "text/plain", nil); err != nil || res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: %v %v", res, err)
	}
}

// TestTilesets: a file maps slots to the operator's assets, and the rest
// stay placeholders. Unknown tilesets and slots, and bad names, are
// errors.
func TestTilesets(t *testing.T) {
	g, _ := gen.Lookup(2)
	dir := t.TempDir()
	write := func(text string) string {
		p := filepath.Join(dir, "tilesets.yml")
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ts, err := LoadTilesets(g, write("tilesets:\n  grassland:\n    textures: {grass: texture/kro/grass01}\n    models: {tree: model/kro/tree}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if ts.Aliases["texture/gen/grassland/grass"] != "texture/kro/grass01" || ts.Aliases["model/gen/grassland/tree"] != "model/kro/tree" ||
		ts.Aliases["texture/gen/grassland/dirt"] != "placeholder/ground/dirt" || ts.Aliases["model/gen/desert/cactus"] != "placeholder/prop/cactus" {
		t.Errorf("aliases %v", ts.Aliases)
	}
	for _, bad := range []string{
		"tilesets:\n  moon:\n    textures: {grass: texture/a}\n",
		"tilesets:\n  grassland:\n    textures: {lava: texture/a}\n",
		"tilesets:\n  grassland:\n    models: {tree: model/A:b}\n",
		"tileset: {}\n",
	} {
		if _, err := LoadTilesets(g, write(bad)); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
	// Every slot of every tileset has an alias.
	def, _ := LoadTilesets(g, "")
	for _, ts := range g.Tilesets {
		for _, s := range ts.Textures {
			if def.Aliases[gen.TextureName(ts.Name, s)] == "" {
				t.Errorf("%s %s has no alias", ts.Name, s)
			}
		}
	}
}

// TestWarps: touching fields link their facing portals, and a player
// lands on a walkable cell.
func TestWarps(t *testing.T) {
	s, _ := testSet(t)
	warps := s.Warps()
	east := warps["gen:field:0:0"]
	if len(east) != 2 {
		t.Fatalf("warps of 0:0: %+v", east)
	}
	for id, list := range warps {
		for _, w := range list {
			to := s.Get(w.To)
			if to == nil {
				t.Fatalf("%s: warp to %s", id, w.To)
			}
			if to.Grid().NoPass(w.ToX, w.ToY) {
				t.Errorf("%s: %s lands on %d,%d, which is not walkable", id, w.Name, w.ToX, w.ToY)
			}
			from := s.Get(id)
			if from.Grid().NoPass(w.X, w.Y) {
				t.Errorf("%s: warp %s at a blocked cell", id, w.Name)
			}
		}
	}
	if len(warps["gen:field:1:0"]) != 1 || warps["gen:field:1:0"][0].To != "gen:field:0:0" {
		t.Errorf("warps of 1:0: %+v", warps["gen:field:1:0"])
	}
}

func TestParseSpec(t *testing.T) {
	for in, want := range map[string]Spec{"0:0": {}, "3:-1": {X: 3, Y: -1}, " 2:5:desert ": {X: 2, Y: 5, Biome: "desert"}} {
		if got, err := ParseSpec(in); err != nil || got != want {
			t.Errorf("%q: %+v %v", in, got, err)
		}
	}
	for _, in := range []string{"", "1", "a:1", "1:2:3:4", "1:99999999999"} {
		if _, err := ParseSpec(in); err == nil {
			t.Errorf("%q parsed", in)
		}
	}
}
