//go:build procedural

package gen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

var update = flag.Bool("update", false, "rewrite the golden checksums of the newest generator; never for a released version")

// golden is a file of golden checksums for one generator version. The
// client's tests and the browser check read the same file.
type golden struct {
	Version int          `json:"version"`
	RNG     string       `json:"rng"`
	Maps    []goldenGrid `json:"maps"`
}

// goldenGrid is one map. A map with a secret and a region has the key of
// region(x, y)/terrain below the root of that secret (version 1). One
// with a secret and a field has the key of public/field(x, y) (decision
// 0039). A map without a secret gives its key alone. Counts are the cells
// of each type: walkable, blocked, water, gap. Versions with fields add
// the biome, the attempt that passed, the props and the SHA-256 of the
// WMAP.
type goldenGrid struct {
	Secret string  `json:"secret,omitempty"`
	Region []int64 `json:"region,omitempty"`
	Field  []int64 `json:"field,omitempty"`
	// Kind is dungeon or town for a map of Generator.Map, with its
	// params as hex. Town, or Dungeon and Floor, name its node below the
	// public branch of the secret (version 3).
	Kind    string `json:"kind,omitempty"`
	Params  string `json:"params,omitempty"`
	Town    int64  `json:"town,omitempty"`
	Dungeon string `json:"dungeon,omitempty"`
	Floor   int    `json:"floor,omitempty"`
	// Name names the style or template, for people.
	Name     string `json:"name,omitempty"`
	Key      string `json:"key"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Biome    int    `json:"biome,omitempty"`
	Water    int32  `json:"water"`
	Counts   [4]int `json:"counts"`
	Checksum string `json:"checksum"`
	Attempt  int    `json:"attempt,omitempty"`
	Objects  int    `json:"objects,omitempty"`
	WMAP     string `json:"wmap,omitempty"`
	// Places is the SHA-256 of the map's places (placesHash), for
	// version 3.
	Places string `json:"places,omitempty"`
}

func goldenFile(version int) string {
	return fmt.Sprintf("testdata/golden-v%d.json", version)
}

// goldenSeeds are the maps of each version's golden file.
var goldenSeeds = map[int][]goldenGrid{1: goldenSeedsV1, 2: goldenSeedsV2, 3: goldenSeedsV3()}

// goldenSeedsV1 are sizes from one cell to the largest map side, and a
// zero key.
var goldenSeedsV1 = []goldenGrid{
	{Secret: "golden-0", Region: []int64{0, 0}, Width: 1, Height: 1},
	{Secret: "golden-0", Region: []int64{0, 1}, Width: 7, Height: 5},
	{Secret: "golden-0", Region: []int64{3, -1}, Width: 64, Height: 64},
	{Secret: "golden-1", Region: []int64{-2, 5}, Width: 33, Height: 129},
	{Secret: "golden-1", Region: []int64{1 << 40, -1 << 40}, Width: 200, Height: 150},
	{Secret: "golden-2", Region: []int64{0, 0}, Width: 400, Height: 400},
	{Secret: "golden-3", Region: []int64{7, 7}, Width: 1024, Height: 8},
	{Key: hex.EncodeToString(make([]byte, 32)), Width: 48, Height: 48},
}

// goldenSeedsV2 are every biome, the smallest field, two 400 × 400
// fields, a 1024-cell side, field indexes near 2^40, and a zero key.
var goldenSeedsV2 = []goldenGrid{
	{Secret: "golden-0", Field: []int64{0, 0}, Width: 64, Height: 64, Biome: biomeGrassland},
	{Secret: "golden-0", Field: []int64{1, 0}, Width: 64, Height: 64, Biome: biomeForest},
	{Secret: "golden-0", Field: []int64{0, 1}, Width: 96, Height: 80, Biome: biomeDesert},
	{Secret: "golden-1", Field: []int64{-2, 5}, Width: 128, Height: 128, Biome: biomeSnow},
	{Secret: "golden-1", Field: []int64{3, -1}, Width: 64, Height: 100, Biome: biomeSwamp},
	{Secret: "golden-1", Field: []int64{1 << 40, -1 << 40}, Width: 80, Height: 64, Biome: biomeBeach},
	{Secret: "golden-2", Field: []int64{0, 0}, Width: 32, Height: 32, Biome: biomeGrassland},
	{Secret: "golden-2", Field: []int64{4, 4}, Width: 400, Height: 400, Biome: biomeGrassland},
	{Secret: "golden-3", Field: []int64{-7, 2}, Width: 400, Height: 400, Biome: biomeForest},
	{Secret: "golden-3", Field: []int64{7, 7}, Width: 1024, Height: 40, Biome: biomeDesert},
	{Key: hex.EncodeToString(make([]byte, 32)), Width: 48, Height: 48, Biome: biomeBeach},
}

// goldenSeedsV3 are two fields, every core dungeon style's first and
// last floor, a 256 × 256 floor, every core town template, the largest
// town, and zero keys.
func goldenSeedsV3() []goldenGrid {
	dungeon := func(secret, id string, s DungeonStyle, floor, floors int, name string) goldenGrid {
		b, err := EncodeDungeon(DungeonParams{Style: s, Floor: floor, Floors: floors})
		if err != nil {
			panic(err)
		}
		return goldenGrid{Secret: secret, Kind: KindDungeon, Dungeon: id, Floor: floor, Params: hex.EncodeToString(b), Name: name,
			Width: s.Width, Height: s.Height}
	}
	town := func(secret string, n int64, t TownTemplate, name string) goldenGrid {
		b, err := EncodeTown(TownParams{Template: t})
		if err != nil {
			panic(err)
		}
		return goldenGrid{Secret: secret, Kind: KindTown, Town: n, Params: hex.EncodeToString(b), Name: name, Width: t.Size, Height: t.Size}
	}
	out := []goldenGrid{
		{Secret: "golden-0", Field: []int64{0, 0}, Width: 64, Height: 64, Biome: biomeGrassland},
		{Secret: "golden-1", Field: []int64{2, -3}, Width: 160, Height: 120, Biome: biomeForest},
	}
	for i, c := range testStyles {
		secret := "golden-" + strconv.Itoa(i)
		out = append(out,
			dungeon(secret, c.name, c.style, 1, c.style.Floors, c.name),
			dungeon(secret, c.name, c.style, c.style.Floors, c.style.Floors, c.name))
	}
	big := testStyles[1].style
	big.Width, big.Height, big.RoomMax = 256, 256, 30
	out = append(out, dungeon("golden-4", "deep", big, 9, 10, "crypt 256"))
	for i, c := range testTemplates {
		out = append(out, town("golden-"+strconv.Itoa(i), int64(i+1), c.template, c.name))
	}
	largest := testTemplates[2].template
	largest.Size = MaxTownSize
	out = append(out, town("golden-5", 40, largest, "city 256"))
	zero := make([]byte, 32)
	for _, m := range []goldenGrid{dungeon("", "", testStyles[0].style, 1, 1, "cave"), town("", 0, testTemplates[0].template, "hamlet")} {
		m.Secret, m.Key = "", hex.EncodeToString(zero)
		out = append(out, m)
	}
	return out
}

// goldenKey returns a map's node.
func goldenKey(t *testing.T, m goldenGrid) rng.Node {
	t.Helper()
	if m.Secret == "" {
		b, err := hex.DecodeString(m.Key)
		if err != nil || len(b) != 32 {
			t.Fatalf("key %q: %v", m.Key, err)
		}
		return rng.FromKey([32]byte(b))
	}
	root, err := rng.NewRoot([]byte(m.Secret))
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case len(m.Field) == 2:
		return FieldNode(PublicBranch(root), m.Field[0], m.Field[1])
	case m.Kind == KindTown:
		return TownNode(PublicBranch(root), m.Town)
	case m.Kind == KindDungeon:
		return FloorNode(DungeonNode(PublicBranch(root), m.Dungeon), m.Floor)
	}
	return root.Child("region", m.Region...).Child("terrain")
}

// result fills a map's key, water level, counts and checksum.
func result(t *testing.T, g Generator, m goldenGrid) goldenGrid {
	t.Helper()
	node := goldenKey(t, m)
	p := Params{Width: m.Width, Height: m.Height, Biome: m.Biome}
	var grid Grid
	if m.Kind != "" {
		params, err := hex.DecodeString(m.Params)
		if err != nil {
			t.Fatal(err)
		}
		f, err := g.Map(node, MapParams{Kind: m.Kind, Params: params})
		if err != nil {
			t.Fatalf("%s %s: %v", m.Kind, m.Name, err)
		}
		grid = f.Grid
		m.Attempt, m.Objects = f.Attempt, len(f.Objects)
		sum := sha256.Sum256(f.WMAP)
		m.WMAP, m.Places = hex.EncodeToString(sum[:]), placesHash(f.Places)
	} else if g.Field != nil {
		f, err := g.Field(node, p)
		if err != nil {
			t.Fatalf("%dx%d: %v", m.Width, m.Height, err)
		}
		grid = f.Grid
		m.Attempt, m.Objects = f.Attempt, len(f.Objects)
		sum := sha256.Sum256(f.WMAP)
		m.WMAP = hex.EncodeToString(sum[:])
		if g.Version >= 3 {
			m.Places = placesHash(f.Places)
		}
	} else {
		var err error
		if grid, err = g.Grid(node, p); err != nil {
			t.Fatalf("%dx%d: %v", m.Width, m.Height, err)
		}
	}
	key := node.Key()
	m.Key = hex.EncodeToString(key[:])
	m.Water = int32(grid.WaterLevel)
	m.Counts = [4]int{}
	for _, c := range grid.Types {
		switch c {
		case CellWalkable:
			m.Counts[0]++
		case CellBlocked:
			m.Counts[1]++
		case CellWater:
			m.Counts[2]++
		case CellGap:
			m.Counts[3]++
		default:
			t.Fatalf("cell type %d", c)
		}
	}
	sum := grid.Checksum()
	m.Checksum = hex.EncodeToString(sum[:])
	return m
}

// TestGolden generates every map of every version's golden file and
// compares the checksums. A changed checksum needs a new generator
// version (strategy section 8.5, "Golden tests").
func TestGolden(t *testing.T) {
	for _, v := range Versions() {
		g, _ := Lookup(v)
		latest, _ := Latest()
		if *update && v == latest.Version {
			out := golden{Version: v, RNG: g.RNG}
			for _, m := range goldenSeeds[v] {
				out.Maps = append(out.Maps, result(t, g, m))
			}
			b, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(goldenFile(v), append(b, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		b, err := os.ReadFile(goldenFile(v))
		if err != nil {
			t.Fatalf("version %d has no golden file: %v", v, err)
		}
		var want golden
		if err := json.Unmarshal(b, &want); err != nil {
			t.Fatal(err)
		}
		if want.Version != v || want.RNG != g.RNG || len(want.Maps) == 0 {
			t.Fatalf("%s: version %d %s with %d maps", goldenFile(v), want.Version, want.RNG, len(want.Maps))
		}
		for _, m := range want.Maps {
			got := result(t, g, m)
			if got.Key != m.Key {
				t.Errorf("v%d %s %v: key %s, want %s", v, m.Secret, m.Region, got.Key, m.Key)
			}
			if got.Checksum != m.Checksum || got.Water != m.Water || got.Counts != m.Counts ||
				got.Attempt != m.Attempt || got.Objects != m.Objects || got.WMAP != m.WMAP || got.Places != m.Places {
				t.Errorf("v%d %s %v%v %s %s %dx%d: checksum %s water %d counts %v attempt %d objects %d wmap %s places %s, want %s %d %v %d %d %s %s",
					v, m.Secret, m.Region, m.Field, m.Kind, m.Name, m.Width, m.Height, got.Checksum, got.Water, got.Counts, got.Attempt, got.Objects, got.WMAP, got.Places,
					m.Checksum, m.Water, m.Counts, m.Attempt, m.Objects, m.WMAP, m.Places)
			}
		}
		t.Logf("v%d: %d golden maps match on %s/%s", v, len(want.Maps), runtime.GOOS, runtime.GOARCH)
	}
}

// TestGridV1Order: a map is the same whatever was generated before it,
// and every cell type occurs on a large map.
func TestGridV1Order(t *testing.T) {
	g, ok := Lookup(1)
	if !ok {
		t.Fatal("no generator 1")
	}
	first := result(t, g, goldenSeedsV1[2])
	for i := len(goldenSeedsV1) - 1; i >= 0; i-- {
		result(t, g, goldenSeedsV1[i])
	}
	if again := result(t, g, goldenSeedsV1[2]); again.Checksum != first.Checksum {
		t.Errorf("a second run differs: %+v, then %+v", first, again)
	}
	big := result(t, g, goldenSeedsV1[5])
	for i, n := range big.Counts {
		if n == 0 {
			t.Errorf("400x400 has no cells of kind %d: %v", i, big.Counts)
		}
	}
	if _, err := g.Grid(rng.Node{}, Params{Width: 0, Height: 5}); err == nil {
		t.Error("a 0-wide grid was made")
	}
}

// BenchmarkGridV1 times a 400 × 400 map with its checksum.
func BenchmarkGridV1(b *testing.B) {
	g, _ := Lookup(1)
	node := rng.FromKey([32]byte{1})
	for b.Loop() {
		grid, err := g.Grid(node, Params{Width: 400, Height: 400})
		if err != nil {
			b.Fatal(err)
		}
		grid.Checksum()
	}
}
