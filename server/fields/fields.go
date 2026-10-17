// Package fields runs the generated maps of a procedural world:
// fields (strategy section 8.5, decision 0039), and dungeon floors and
// towns (decision 0040):
//
//   - Generate makes each field, and GenerateMaps each floor and town,
//     from its public node at zone start.
//   - Set serves each map's public node and checksum, with a floor's or
//     a town's params, so the web client generates the map itself, and
//     the map's WMAP for a client whose result differs or that lacks the
//     generator.
//   - The tileset table maps the generator's tileset names to asset
//     names, with placeholders by default.
//   - The dungeon styles and town templates are records that mods
//     change and add (records.go).
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

// Field is a generated map that the server runs: a field, a dungeon
// floor or a town.
type Field struct {
	// ID is the map ID, such as gen:field:3:-1, gen:town:t1 or
	// gen:dungeon:crypt:2.
	ID string
	// Kind is gen.KindField, gen.KindDungeon or gen.KindTown.
	Kind string
	// X and Y are a field's coordinates.
	X, Y int64
	// Params are a floor's or a town's params, which clients get with
	// the node. Name is its style's or template's record name.
	Params []byte
	Name   string
	// Town is a town's number. Dungeon, Floor and Floors name a floor,
	// and Levels is its level band, the lowest and highest level.
	Town          int64
	Dungeon       string
	Floor, Floors int
	Levels        [2]int
	// DisplayName is the map's name for players, from the world layout
	// (decision 0041), or empty.
	DisplayName string
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
	// Added are the places that procedural content and generation
	// layers added, after the generated ones, and Objects the layers'
	// objects (decision 0044). AddContent sets them.
	Added   []gen.Place
	Objects []Object

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
	return generateField(g, gen.FieldNode(public, s.X, s.Y), s.X, s.Y, biome, size)
}

// generateField makes a field from a node: its own, or a season's
// (decision 0045).
func generateField(g gen.Generator, node rng.Node, x, y int64, biome, size int) (*Field, error) {
	id := gen.FieldID(x, y)
	field, err := g.Field(node, gen.Params{Width: size, Height: size, Biome: biome})
	if err != nil {
		// The node is public, so the log may name it for investigation.
		key := node.Key()
		return nil, fmt.Errorf("field %s (%s, node %s): %w", id, g.Biomes[biome], hex.EncodeToString(key[:]), err)
	}
	return &Field{
		ID: id, Kind: gen.KindField, X: x, Y: y, Version: g.Version, RNG: g.RNG,
		Key: node.Key(), Biome: biome, BiomeName: g.Biomes[biome],
		Field: field, Checksum: field.Checksum(), WMAPSum: sha256.Sum256(field.WMAP),
	}, nil
}

// Set is the fields a server runs, with what clients download. Update
// changes its maps while the server runs (decision 0045); its methods
// are safe for concurrent use.
type Set struct {
	mu       sync.RWMutex
	fields   []*Field
	tilesets []byte
	// names gives the current display names, which change with
	// @renametown, or is nil.
	names func(id string) (string, bool)
}

// SetNames makes Info read display names from f, such as an atlas's
// Name, so a renamed map shows its new name. Call it before serving.
func (s *Set) SetNames(f func(id string) (string, bool)) { s.names = f }

// NewSet makes a set of fields. tilesets is the tileset table as JSON,
// from Tilesets.
func NewSet(fields []*Field, tilesets []byte) *Set {
	s := &Set{fields: slices.Clone(fields), tilesets: tilesets}
	slices.SortFunc(s.fields, func(a, b *Field) int { return strings.Compare(a.ID, b.ID) })
	return s
}

// Fields lists the fields by ID.
func (s *Set) Fields() []*Field {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.fields)
}

// Get returns a field by map ID, or nil.
func (s *Set) Get(id string) *Field {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := slices.BinarySearchFunc(s.fields, id, func(f *Field, id string) int { return strings.Compare(f.ID, id) })
	if !ok {
		return nil
	}
	return s.fields[i]
}

// With returns a new set of the set's fields without those of remove,
// and with add, which replace fields of the same ID. It shares the
// tileset table and the names. The set does not change.
func (s *Set) With(add []*Field, remove []string) *Set {
	s.mu.RLock()
	list := make([]*Field, 0, len(s.fields)+len(add))
	for _, f := range s.fields {
		if !slices.Contains(remove, f.ID) && !slices.ContainsFunc(add, func(a *Field) bool { return a.ID == f.ID }) {
			list = append(list, f)
		}
	}
	out := &Set{fields: append(list, add...), tilesets: s.tilesets, names: s.names}
	s.mu.RUnlock()
	slices.SortFunc(out.fields, func(a, b *Field) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// Update makes the set's maps those of next, such as a set from With,
// for the clients that ask from now on.
func (s *Set) Update(next *Set) {
	list := next.Fields()
	s.mu.Lock()
	s.fields = list
	s.mu.Unlock()
}

// Info is what /gen/maps/<id>.json tells a client about a map.
type Info struct {
	ID string `json:"id"`
	// Kind is field, dungeon, town or home. Params are a floor's or a
	// town's params as hex, and Name its style or template, or a home
	// map's mod:folder.
	Kind string `json:"kind"`
	// DisplayName is the map's name for players, from the world layout.
	DisplayName string `json:"displayName,omitempty"`
	Params      string `json:"params,omitempty"`
	Name        string `json:"name,omitempty"`
	Version     int    `json:"version"`
	RNG         string `json:"rng"`
	// Key is the map's public node, as hex. A home map has none: a
	// client downloads its WMAP.
	Key    string `json:"key,omitempty"`
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
	// Town is a town's number. Dungeon, Floor and Floors describe a
	// floor. Levels is the map's level band, for every map of a world
	// layout and every floor.
	Town    int64  `json:"town,omitempty"`
	Dungeon string `json:"dungeon,omitempty"`
	Floor   int    `json:"floor,omitempty"`
	Floors  int    `json:"floors,omitempty"`
	Levels  []int  `json:"levels,omitempty"`
	// Places are the map's named places. They are public layout, which
	// the client can make itself, so the checks may read them here. The
	// places that content and generation layers added follow the
	// generated ones.
	Places []InfoPlace `json:"places,omitempty"`
	// Objects are what generation layers put on the map (decision 0044):
	// the client draws each model at its cell.
	Objects []InfoObject `json:"objects,omitempty"`
}

// InfoObject is an object in Info: a model's asset name, such as
// model/gen/forest/rock, a cell, and 0 to 3 quarter turns.
type InfoObject struct {
	Model string `json:"model"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Turn  int    `json:"turn,omitempty"`
}

// Object is an object of a generation layer on a map: a prop of the
// map's tileset.
type Object struct {
	// Model is the model's asset name, such as model/gen/forest/rock.
	Model      string
	X, Y, Turn int
}

// InfoPlace is a named place in Info.
type InfoPlace struct {
	Tags []string `json:"tags"`
	X    int      `json:"x"`
	Y    int      `json:"y"`
}

// Info returns a field's info.
func (f *Field) Info() Info {
	in := Info{
		ID: f.ID, Kind: f.Kind, Params: hex.EncodeToString(f.Params), Name: f.Name,
		Town: f.Town, Dungeon: f.Dungeon, Floor: f.Floor, Floors: f.Floors, DisplayName: f.DisplayName,
		Version: f.Version, RNG: f.RNG, Key: hex.EncodeToString(f.Key[:]),
		Width: f.Field.Width, Height: f.Field.Height, Biome: f.Biome, BiomeName: f.BiomeName,
		Checksum: hex.EncodeToString(f.Checksum[:]),
		WMAP:     Prefix + "maps/" + f.ID + ".wmap", WMAPSHA256: hex.EncodeToString(f.WMAPSum[:]),
		WMAPBytes: len(f.Field.WMAP),
	}
	if f.Kind == gen.KindDungeon || f.Levels != [2]int{} {
		in.Levels = f.Levels[:]
	}
	if f.Key == [32]byte{} {
		in.Key = ""
	}
	for _, p := range f.AllPlaces() {
		in.Places = append(in.Places, InfoPlace{Tags: p.Tags, X: p.X, Y: p.Y})
	}
	for _, o := range f.Objects {
		in.Objects = append(in.Objects, InfoObject(o))
	}
	return in
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
		in := f.Info()
		if s.names != nil {
			if n, ok := s.names(f.ID); ok {
				in.DisplayName = n
			}
		}
		_ = json.NewEncoder(w).Encode(in)
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
