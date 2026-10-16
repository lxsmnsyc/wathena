// Package fields runs the generated fields of a procedural world
// (strategy section 8.5, decision 0039):
//
//   - Generate makes each field from its public node at zone start.
//   - Set serves each field's public node and checksum, so the web client
//     generates the field itself, and the field's WMAP for a client whose
//     result differs or that lacks the generator.
//   - The tileset table maps the generator's tileset names to asset
//     names, with placeholders by default.
package fields

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Prefix starts the paths that Set serves.
const Prefix = "/gen/"

// Spec names a field to run: its coordinates, and its biome, or "" for
// the world's climate.
type Spec struct {
	X, Y  int64
	Biome string
}

// ParseSpec reads "x:y" or "x:y:biome".
func ParseSpec(s string) (Spec, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 && len(parts) != 3 {
		return Spec{}, fmt.Errorf("field %q: want x:y or x:y:biome", s)
	}
	x, errX := strconv.ParseInt(parts[0], 10, 32)
	y, errY := strconv.ParseInt(parts[1], 10, 32)
	if errX != nil || errY != nil {
		return Spec{}, fmt.Errorf("field %q: x and y are whole numbers", s)
	}
	sp := Spec{X: x, Y: y}
	if len(parts) == 3 {
		sp.Biome = parts[2]
	}
	return sp, nil
}

// Field is a generated field that the server runs.
type Field struct {
	// ID is the map ID, such as gen:field:3:-1.
	ID   string
	X, Y int64
	// Version and RNG are the generator version and its rng spec.
	Version int
	RNG     string
	// Key is the field's public node, which clients get.
	Key   [32]byte
	Biome int
	// BiomeName is the biome's name in the generator's table.
	BiomeName string
	Field     gen.Field
	// Checksum is the grid's checksum, WMAPSum the WMAP's SHA-256.
	Checksum, WMAPSum [32]byte

	gzOnce sync.Once
	gz     []byte
}

// gzipped returns the field's WMAP compressed with gzip, made once.
func gzipped(f *Field) []byte {
	f.gzOnce.Do(func() {
		var b bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&b, gzip.BestSpeed)
		_, _ = zw.Write(f.Field.WMAP)
		_ = zw.Close()
		if b.Len() < len(f.Field.WMAP) {
			f.gz = b.Bytes()
		}
	})
	return f.gz
}

// Grid returns the field's cells for the zone: walkable and shootable
// flags only.
func (f *Field) Grid() *path.Grid {
	cells := make([]uint8, len(f.Field.Types))
	for i, t := range f.Field.Types {
		cells[i] = gen.Flags(t) & (path.Walkable | path.Shootable)
	}
	return &path.Grid{Width: f.Field.Width, Height: f.Field.Height, Cells: cells}
}

// Generate makes the fields from the world's public branch, several at
// once. Every field is size × size cells. A field that fails validation
// in every attempt is an error.
func Generate(g gen.Generator, public rng.Node, specs []Spec, size int) ([]*Field, error) {
	if g.Field == nil {
		return nil, fmt.Errorf("map generator %d makes no fields; fields need generator 2 or later", g.Version)
	}
	out := make([]*Field, len(specs))
	errs := make([]error, len(specs))
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(specs)) {
		wg.Go(func() {
			for i := range next {
				out[i], errs[i] = generate(g, public, specs[i], size)
			}
		})
	}
	for i := range specs {
		next <- i
	}
	close(next)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, f := range out {
		if seen[f.ID] {
			return nil, fmt.Errorf("field %s is listed twice", f.ID)
		}
		seen[f.ID] = true
	}
	return out, nil
}

func generate(g gen.Generator, public rng.Node, s Spec, size int) (*Field, error) {
	id := gen.FieldID(s.X, s.Y)
	biome := g.Climate(public, s.X, s.Y)
	if s.Biome != "" {
		b, ok := g.BiomeIndex(s.Biome)
		if !ok {
			return nil, fmt.Errorf("field %s: unknown biome %q; generator %d has %s", id, s.Biome, g.Version, strings.Join(g.Biomes, ", "))
		}
		biome = b
	}
	node := gen.FieldNode(public, s.X, s.Y)
	field, err := g.Field(node, gen.Params{Width: size, Height: size, Biome: biome})
	if err != nil {
		// The node is public, so the log may name it for investigation.
		key := node.Key()
		return nil, fmt.Errorf("field %s (%s, node %s): %w", id, g.Biomes[biome], hex.EncodeToString(key[:]), err)
	}
	return &Field{
		ID: id, X: s.X, Y: s.Y, Version: g.Version, RNG: g.RNG,
		Key: node.Key(), Biome: biome, BiomeName: g.Biomes[biome],
		Field: field, Checksum: field.Checksum(), WMAPSum: sha256.Sum256(field.WMAP),
	}, nil
}

// Set is the fields a server runs, with what clients download.
type Set struct {
	fields   []*Field
	tilesets []byte
}

// NewSet makes a set of fields. tilesets is the tileset table as JSON,
// from Tilesets.
func NewSet(fields []*Field, tilesets []byte) *Set {
	s := &Set{fields: slices.Clone(fields), tilesets: tilesets}
	slices.SortFunc(s.fields, func(a, b *Field) int { return strings.Compare(a.ID, b.ID) })
	return s
}

// Fields lists the fields by ID.
func (s *Set) Fields() []*Field { return s.fields }

// Get returns a field by map ID, or nil.
func (s *Set) Get(id string) *Field {
	i, ok := slices.BinarySearchFunc(s.fields, id, func(f *Field, id string) int { return strings.Compare(f.ID, id) })
	if !ok {
		return nil
	}
	return s.fields[i]
}

// Info is what /gen/maps/<id>.json tells a client about a field.
type Info struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	RNG     string `json:"rng"`
	// Key is the field's public node, as hex.
	Key    string `json:"key"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// Biome is the biome's index, BiomeName its name.
	Biome     int    `json:"biome"`
	BiomeName string `json:"biomeName"`
	// Checksum is the grid's checksum, as hex.
	Checksum string `json:"checksum"`
	// WMAP is the path of the field's WMAP, WMAPSHA256 its SHA-256 and
	// WMAPBytes its length.
	WMAP       string `json:"wmap"`
	WMAPSHA256 string `json:"wmapSha256"`
	WMAPBytes  int    `json:"wmapBytes"`
}

// Info returns a field's info.
func (f *Field) Info() Info {
	return Info{
		ID: f.ID, Version: f.Version, RNG: f.RNG, Key: hex.EncodeToString(f.Key[:]),
		Width: f.Field.Width, Height: f.Field.Height, Biome: f.Biome, BiomeName: f.BiomeName,
		Checksum: hex.EncodeToString(f.Checksum[:]),
		WMAP:     Prefix + "maps/" + f.ID + ".wmap", WMAPSHA256: hex.EncodeToString(f.WMAPSum[:]),
		WMAPBytes: len(f.Field.WMAP),
	}
}

// ServeHTTP serves:
//
//   - GET /gen/maps/<id>.json: the field's Info.
//   - GET /gen/maps/<id>.wmap: the field's WMAP, gzip-compressed for a
//     client that accepts it.
//   - GET /gen/tilesets.json: the tileset table.
//
// Public nodes are public by design (strategy section 8.3), so these
// need no login. Any origin may read them, as the assets.
func (s *Set) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p, _ := strings.CutPrefix(r.URL.Path, Prefix)
	if p == "tilesets.json" {
		h.Set("Content-Type", "application/json")
		h.Set("Cache-Control", "no-cache")
		_, _ = w.Write(s.tilesets)
		return
	}
	rest, ok := strings.CutPrefix(p, "maps/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if id, ok := strings.CutSuffix(rest, ".json"); ok {
		f := s.Get(id)
		if f == nil {
			http.NotFound(w, r)
			return
		}
		h.Set("Content-Type", "application/json")
		h.Set("Cache-Control", "no-cache")
		_ = json.NewEncoder(w).Encode(f.Info())
		return
	}
	if id, ok := strings.CutSuffix(rest, ".wmap"); ok {
		f := s.Get(id)
		if f == nil {
			http.NotFound(w, r)
			return
		}
		serveWMAP(w, r, f)
		return
	}
	http.NotFound(w, r)
}

// serveWMAP sends a field's WMAP. Its ETag is the WMAP's SHA-256, so a
// client that has it gets 304.
func serveWMAP(w http.ResponseWriter, r *http.Request, f *Field) {
	h := w.Header()
	etag := `"` + hex.EncodeToString(f.WMAPSum[:]) + `"`
	h.Set("ETag", etag)
	h.Set("Cache-Control", "no-cache")
	h.Set("Content-Type", "application/octet-stream")
	h.Add("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	data := f.Field.WMAP
	if gz := gzipped(f); gz != nil && acceptsGzip(r) {
		h.Set("Content-Encoding", "gzip")
		data = gz
	}
	h.Set("Content-Length", strconv.Itoa(len(data)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

func acceptsGzip(r *http.Request) bool {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		name, q, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.TrimSpace(name) == "gzip" && strings.ReplaceAll(q, " ", "") != "q=0" {
			return true
		}
	}
	return false
}
