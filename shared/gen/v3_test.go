//go:build procedural

package gen

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// testStyles are core's dungeon styles (server/fields/core), copied so
// this package's tests stand alone.
var testStyles = []struct {
	name  string
	style DungeonStyle
}{
	{"cave", DungeonStyle{Method: MethodCave, Floors: 3, Width: 128, Height: 128, RoomMin: 6, RoomMax: 12, Tileset: "cave", LevelStep: 5}},
	{"crypt", DungeonStyle{Method: MethodRooms, Floors: 3, Width: 128, Height: 128, RoomMin: 8, RoomMax: 20, Tileset: "crypt", LevelStep: 6}},
	{"tower", DungeonStyle{Method: MethodRooms, Floors: 5, Width: 80, Height: 80, RoomMin: 8, RoomMax: 16, Tileset: "tower", LevelStep: 4}},
	{"ruins", DungeonStyle{Method: MethodRooms, Floors: 2, Width: 160, Height: 160, RoomMin: 10, RoomMax: 24, Tileset: "ruins", LevelStep: 8}},
}

// testTemplates are core's town templates, copied as testStyles are.
var testTemplates = []struct {
	name     string
	template TownTemplate
}{
	{"hamlet", TownTemplate{Size: 96, Gates: 2, Tileset: "town", Decorations: []string{"tree", "well", "crate"}, Buildings: []Building{
		{Model: "smithy", Width: 8, Depth: 6, Count: 1, Places: []string{"blacksmith"}},
		{Model: "inn", Width: 8, Depth: 8, Count: 1, Places: []string{"inn"}},
		{Model: "storehouse", Width: 6, Depth: 6, Count: 1, Places: []string{"storage"}},
		{Model: "tool_shop", Width: 6, Depth: 6, Count: 1, Places: []string{"shop:tool"}},
		{Model: "house", Width: 6, Depth: 6, Count: 10},
	}}},
	{"village", TownTemplate{Size: 128, Gates: 3, Walls: true, Tileset: "town", Decorations: []string{"tree", "well", "bush"}, Buildings: []Building{
		{Model: "smithy", Width: 8, Depth: 6, Count: 1, Places: []string{"blacksmith"}},
		{Model: "inn", Width: 8, Depth: 8, Count: 1, Places: []string{"inn"}},
		{Model: "storehouse", Width: 6, Depth: 6, Count: 1, Places: []string{"storage"}},
		{Model: "tool_shop", Width: 6, Depth: 6, Count: 1, Places: []string{"shop:tool"}},
		{Model: "weapon_shop", Width: 6, Depth: 6, Count: 1, Places: []string{"shop:weapon"}},
		{Model: "armor_shop", Width: 6, Depth: 6, Count: 1, Places: []string{"shop:armor"}},
		{Model: "chapel", Width: 8, Depth: 10, Count: 1, Places: []string{"healer"}},
		{Model: "house", Width: 6, Depth: 6, Count: 16},
		{Model: "cottage", Width: 4, Depth: 4, Count: 12},
	}}},
	{"city", TownTemplate{Size: 192, Gates: 4, Walls: true, Tileset: "town", Decorations: []string{"tree", "well", "bush", "crate"}, Buildings: []Building{
		{Model: "smithy", Width: 8, Depth: 6, Count: 1, Places: []string{"blacksmith"}},
		{Model: "inn", Width: 8, Depth: 8, Count: 1, Places: []string{"inn"}},
		{Model: "storehouse", Width: 6, Depth: 6, Count: 1, Places: []string{"storage", "storage:guild"}},
		{Model: "tool_shop", Width: 6, Depth: 6, Count: 1, Places: []string{"shop:tool"}},
		{Model: "weapon_shop", Width: 6, Depth: 6, Count: 1, Places: []string{"shop:weapon"}},
		{Model: "armor_shop", Width: 6, Depth: 6, Count: 1, Places: []string{"shop:armor"}},
		{Model: "chapel", Width: 8, Depth: 10, Count: 1, Places: []string{"healer"}},
		{Model: "warp_tower", Width: 6, Depth: 6, Count: 1, Places: []string{"teleport"}},
		{Model: "manor", Width: 10, Depth: 8, Count: 6},
		{Model: "house", Width: 6, Depth: 6, Count: 40},
		{Model: "cottage", Width: 4, Depth: 4, Count: 30},
	}}},
}

// mapParams encodes a floor or a town.
func dungeonParams(t testing.TB, s DungeonStyle, floor, floors int) MapParams {
	t.Helper()
	b, err := EncodeDungeon(DungeonParams{Style: s, Floor: floor, Floors: floors})
	if err != nil {
		t.Fatal(err)
	}
	return MapParams{Kind: KindDungeon, Params: b}
}

func townParams(t testing.TB, tt TownTemplate) MapParams {
	t.Helper()
	b, err := EncodeTown(TownParams{Template: tt})
	if err != nil {
		t.Fatal(err)
	}
	return MapParams{Kind: KindTown, Params: b}
}

// cellReach returns, for each cell, whether it is reached from cell
// (x, y) along the 4 axes over walkable cells.
func cellReach(types []uint8, w, h, x, y int) []bool {
	seen := make([]bool, w*h)
	start := y*w + x
	if x < 0 || y < 0 || x >= w || y >= h || !walkableType(types[start]) {
		return seen
	}
	seen[start] = true
	stack := []int{start}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		cx, cy := c%w, c/w
		for _, d := range v2Neighbors8[:4] {
			nx, ny := cx+d[0], cy+d[1]
			if nx < 0 || ny < 0 || nx >= w || ny >= h {
				continue
			}
			j := ny*w + nx
			if !seen[j] && walkableType(types[j]) {
				seen[j] = true
				stack = append(stack, j)
			}
		}
	}
	return seen
}

// checkMap checks what every dungeon floor and town must hold: walkable
// and blocked cells only, every place on a walkable cell reached from
// the first, the tags it must have, and the WMAP's cells.
func checkMap(t *testing.T, f Field, need ...string) {
	t.Helper()
	for i, c := range f.Types {
		if c != CellWalkable && c != CellBlocked {
			t.Fatalf("cell %d has type %d", i, c)
		}
	}
	if len(f.Places) == 0 {
		t.Fatal("no places")
	}
	p0 := f.Places[0]
	seen := cellReach(f.Types, f.Width, f.Height, p0.X, p0.Y)
	for _, p := range f.Places {
		if len(p.Tags) == 0 {
			t.Fatalf("a place without tags at %d,%d", p.X, p.Y)
		}
		for _, tag := range p.Tags {
			if !ValidTag(tag) {
				t.Errorf("tag %q", tag)
			}
		}
		if !seen[p.Y*f.Width+p.X] {
			t.Errorf("place %v at %d,%d is not reached from %v", p.Tags, p.X, p.Y, p0.Tags)
		}
	}
	for _, tag := range need {
		found := false
		for _, p := range f.Places {
			found = found || p.HasTag(tag)
		}
		if !found {
			t.Errorf("no place tagged %s", tag)
		}
	}
	checkWMAP(t, f)
}

// TestDungeonV3 makes every floor of each style from several nodes and
// checks each: the entrance, the exit or the boss room on the last
// floor, and the same floor twice.
func TestDungeonV3(t *testing.T) {
	g, ok := Lookup(3)
	if !ok || g.Map == nil || !g.Makes(KindDungeon) {
		t.Fatal("generator 3 makes no dungeons")
	}
	public := PublicBranch(rng.FromKey([32]byte{3, 1}))
	retried, n := 0, 0
	for _, c := range testStyles {
		for k := range 4 {
			d := DungeonNode(public, c.name+strings.Repeat("x", k))
			for floor := 1; floor <= c.style.Floors; floor++ {
				p := dungeonParams(t, c.style, floor, c.style.Floors)
				f, err := g.Map(FloorNode(d, floor), p)
				if err != nil {
					t.Fatalf("%s %d floor %d: %v", c.name, k, floor, err)
				}
				n++
				if f.Attempt > 0 {
					retried++
				}
				if f.Width != c.style.Width || f.Height != c.style.Height {
					t.Errorf("%s: %d×%d", c.name, f.Width, f.Height)
				}
				if !f.Places[0].HasTag("entrance") {
					t.Errorf("%s: the first place is %v", c.name, f.Places[0].Tags)
				}
				if floor == c.style.Floors {
					checkMap(t, f, "entrance", "boss_room")
				} else {
					checkMap(t, f, "entrance", "exit")
				}
				again, err := g.Map(FloorNode(d, floor), p)
				if err != nil || again.Checksum() != f.Checksum() || !bytes.Equal(again.WMAP, f.WMAP) || placesHash(again.Places) != placesHash(f.Places) {
					t.Errorf("%s floor %d: a second run differs: %v", c.name, floor, err)
				}
			}
		}
	}
	t.Logf("%d of %d floors needed a retry", retried, n)
}

// TestTownV3 makes towns of each template and checks each: the square,
// a gate and a portal per gate, and the places of every building.
func TestTownV3(t *testing.T) {
	g, _ := Lookup(3)
	public := PublicBranch(rng.FromKey([32]byte{3, 2}))
	retried, n := 0, 0
	for _, c := range testTemplates {
		need := []string{"square", "gate", "portal"}
		for _, b := range c.template.Buildings {
			need = append(need, b.Places...)
		}
		for k := range 8 {
			p := townParams(t, c.template)
			node := TownNode(public, int64(k+1))
			f, err := g.Map(node, p)
			if err != nil {
				t.Fatalf("%s %d: %v", c.name, k, err)
			}
			n++
			if f.Attempt > 0 {
				retried++
			}
			if f.Width != c.template.Size || !f.Places[0].HasTag("square") {
				t.Errorf("%s: %d wide, first place %v", c.name, f.Width, f.Places[0].Tags)
			}
			gates, portals := 0, 0
			for _, pl := range f.Places {
				if pl.HasTag("gate") {
					gates++
				}
				if pl.HasTag("portal") {
					portals++
				}
			}
			if gates != c.template.Gates || portals != c.template.Gates {
				t.Errorf("%s: %d gates and %d portals, want %d", c.name, gates, portals, c.template.Gates)
			}
			checkMap(t, f, need...)
			again, err := g.Map(node, p)
			if err != nil || again.Checksum() != f.Checksum() || !bytes.Equal(again.WMAP, f.WMAP) {
				t.Errorf("%s %d: a second run differs: %v", c.name, k, err)
			}
		}
	}
	t.Logf("%d of %d towns needed a retry", retried, n)
}

// TestFieldV3Places: a version 3 field is version 2's, with a place per
// portal and clearings on open ground.
func TestFieldV3Places(t *testing.T) {
	v2, _ := Lookup(2)
	v3, _ := Lookup(3)
	public := PublicBranch(rng.FromKey([32]byte{3, 3}))
	clearings := 0
	for b := range v3.Biomes {
		node := FieldNode(public, int64(b), 0)
		p := Params{Width: 160, Height: 160, Biome: b}
		f2, err := v2.Field(node, p)
		if err != nil {
			t.Fatal(err)
		}
		f3, err := v3.Field(node, p)
		if err != nil {
			t.Fatal(err)
		}
		if f2.Checksum() != f3.Checksum() || !bytes.Equal(f2.WMAP, f3.WMAP) || len(f2.Places) != 0 {
			t.Errorf("%s: version 3's field differs from version 2's", v3.Biomes[b])
		}
		for i, port := range f3.Portals {
			pl := f3.Places[i]
			if pl.X != port.X || pl.Y != port.Y || !pl.HasTag("portal") || !pl.HasTag("portal:"+EdgeNames[port.Edge]) {
				t.Errorf("place %d is %+v for portal %+v", i, pl, port)
			}
		}
		for _, pl := range f3.Places[len(f3.Portals):] {
			clearings++
			if !pl.HasTag("clearing") || f3.Types[pl.Y*f3.Width+pl.X] != CellWalkable {
				t.Errorf("clearing %+v", pl)
			}
		}
	}
	if clearings == 0 {
		t.Error("no field has a clearing")
	}
}

// TestMapV3Retries: a map that fails in every attempt is ErrNoMap with
// each attempt's problem, and a passing attempt is the map its node
// gives.
func TestMapV3Retries(t *testing.T) {
	node := rng.FromKey([32]byte{9, 9})
	_, err := attempts(node, func(rng.Node) (*v3Blocks, string) { return nil, "no" })
	if !errors.Is(err, ErrNoMap) || strings.Count(err.Error(), "; attempt ")+1 != v3Attempts {
		t.Fatalf("error %v, want ErrNoMap with %d attempts", err, v3Attempts)
	}
	// A town too full for its buildings fails every attempt.
	full := testTemplates[0].template
	full.Size = 64
	full.Buildings = []Building{{Model: "keep", Width: 12, Depth: 12, Count: 9, Places: []string{"keep"}}}
	g, _ := Lookup(3)
	if _, err := g.Map(node, townParams(t, full)); !errors.Is(err, ErrNoMap) || !strings.Contains(err.Error(), "no lot for the building keep") {
		t.Errorf("a full town: %v", err)
	}
	// The first floor that needed a retry equals the floor of its
	// attempt's node.
	for k := range 64 {
		n := rng.FromKey([32]byte{byte(k), 7})
		p := dungeonParams(t, testStyles[0].style, 1, 3)
		f, err := g.Map(n, p)
		if err != nil {
			t.Fatal(err)
		}
		if f.Attempt == 0 {
			continue
		}
		dp, _ := DecodeDungeon(p.Params)
		again, err := dungeonV3(n.Child("attempt", int64(f.Attempt)), dp)
		if err != nil || again.Attempt != 0 || again.Checksum() != f.Checksum() {
			t.Errorf("attempt %d does not match the floor: %v", f.Attempt, err)
		}
		return
	}
	t.Log("no cave floor of 64 needed a retry")
}

// TestMapV3Inputs: unknown kinds, tilesets, props and bad params are
// errors.
func TestMapV3Inputs(t *testing.T) {
	g, _ := Lookup(3)
	node := rng.FromKey([32]byte{4})
	style := testStyles[1].style
	style.Tileset = "moon"
	b, _ := EncodeDungeon(DungeonParams{Style: style, Floor: 1, Floors: 1})
	if _, err := g.Map(node, MapParams{Kind: KindDungeon, Params: b}); !errors.Is(err, ErrTileset) {
		t.Errorf("tileset moon: %v", err)
	}
	style.Tileset = "town"
	b, _ = EncodeDungeon(DungeonParams{Style: style, Floor: 1, Floors: 1})
	if _, err := g.Map(node, MapParams{Kind: KindDungeon, Params: b}); !errors.Is(err, ErrTileset) {
		t.Errorf("a dungeon on the town tileset: %v", err)
	}
	town := testTemplates[0].template
	town.Decorations = []string{"stalagmite"}
	if _, err := g.Map(node, townParams(t, town)); !errors.Is(err, ErrTileset) {
		t.Errorf("a town decoration of another tileset: %v", err)
	}
	town = testTemplates[0].template
	town.Buildings = []Building{{Model: "well", Width: 4, Depth: 4, Count: 1}}
	if _, err := g.Map(node, townParams(t, town)); !errors.Is(err, ErrTileset) {
		t.Errorf("a building named as a prop: %v", err)
	}
	if _, err := g.Map(node, MapParams{Kind: KindField}); !errors.Is(err, ErrKind) {
		t.Errorf("a field through Map: %v", err)
	}
	if _, err := g.Map(node, MapParams{Kind: KindTown, Params: []byte{2, 1, 0}}); !errors.Is(err, ErrParams) {
		t.Errorf("cut params: %v", err)
	}
	if v2, _ := Lookup(2); v2.Map != nil || v2.Makes(KindTown) || !v2.Makes(KindField) {
		t.Error("generator 2 makes towns")
	}
}

// TestParams: the encodings round-trip and refuse what fails a check.
func TestParams(t *testing.T) {
	for _, c := range testStyles {
		p := DungeonParams{Style: c.style, Floor: 2, Floors: 2}
		b, err := EncodeDungeon(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeDungeon(b)
		if err != nil || got != p {
			t.Errorf("%s: %+v %v", c.name, got, err)
		}
		if _, err := DecodeDungeon(append(b, 0)); !errors.Is(err, ErrParams) {
			t.Errorf("%s with a byte more: %v", c.name, err)
		}
		if _, err := DecodeDungeon(b[:len(b)-1]); !errors.Is(err, ErrParams) {
			t.Errorf("%s cut short: %v", c.name, err)
		}
	}
	for _, c := range testTemplates {
		b, err := EncodeTown(TownParams{Template: c.template})
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeTown(b)
		if err != nil {
			t.Fatal(err)
		}
		again, _ := EncodeTown(got)
		if !bytes.Equal(again, b) {
			t.Errorf("%s does not round-trip", c.name)
		}
		if _, err := DecodeTown(b[:len(b)-2]); !errors.Is(err, ErrParams) {
			t.Errorf("%s cut short: %v", c.name, err)
		}
	}
	bad := []DungeonStyle{
		{Method: "maze", Floors: 1, Width: 64, Height: 64, RoomMin: 6, RoomMax: 6, Tileset: "cave", LevelStep: 1},
		{Method: MethodCave, Floors: 0, Width: 64, Height: 64, RoomMin: 6, RoomMax: 6, Tileset: "cave", LevelStep: 1},
		{Method: MethodCave, Floors: 1, Width: 63, Height: 64, RoomMin: 6, RoomMax: 6, Tileset: "cave", LevelStep: 1},
		{Method: MethodCave, Floors: 1, Width: 64, Height: 64, RoomMin: 8, RoomMax: 6, Tileset: "cave", LevelStep: 1},
		{Method: MethodRooms, Floors: 1, Width: 64, Height: 64, RoomMin: 6, RoomMax: 22, Tileset: "crypt", LevelStep: 1},
		{Method: MethodCave, Floors: 1, Width: 64, Height: 64, RoomMin: 6, RoomMax: 6, Tileset: "Cave", LevelStep: 1},
		{Method: MethodCave, Floors: 1, Width: 64, Height: 64, RoomMin: 6, RoomMax: 6, Tileset: "cave", LevelStep: 0},
	}
	for _, s := range bad {
		if _, err := EncodeDungeon(DungeonParams{Style: s, Floor: 1, Floors: 1}); !errors.Is(err, ErrParams) {
			t.Errorf("%+v: %v", s, err)
		}
	}
	tt := testTemplates[0].template
	for _, change := range []func(*TownTemplate){
		func(t *TownTemplate) { t.Size = 63 },
		func(t *TownTemplate) { t.Gates = 5 },
		func(t *TownTemplate) { t.Buildings = nil },
		func(t *TownTemplate) { t.Buildings = []Building{{Model: "a", Width: 3, Depth: 4, Count: 1}} },
		func(t *TownTemplate) {
			t.Buildings = []Building{{Model: "a", Width: 4, Depth: 4, Count: 1, Places: []string{"x", "y", "z"}}}
		},
		func(t *TownTemplate) {
			t.Buildings = []Building{{Model: "a", Width: 4, Depth: 4, Count: 1, Places: []string{"Bad Tag"}}}
		},
		func(t *TownTemplate) { t.Decorations = []string{"tree!"} },
	} {
		c := tt
		c.Buildings = append([]Building(nil), tt.Buildings...)
		change(&c)
		if _, err := EncodeTown(TownParams{Template: c}); !errors.Is(err, ErrParams) {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

// TestMapIDs: town and dungeon map IDs and areas.
func TestMapIDs(t *testing.T) {
	if id := TownMapID(7); id != "gen:town:t7" || Area(id) != "town:t7" {
		t.Errorf("town 7: %s", id)
	}
	if n, ok := ParseTownMapID("gen:town:t12"); !ok || n != 12 {
		t.Errorf("gen:town:t12: %d %v", n, ok)
	}
	for _, id := range []string{"gen:town:7", "gen:town:t0", "gen:town:t07", "gen:town:t-1"} {
		if _, ok := ParseTownMapID(id); ok {
			t.Errorf("%s parsed", id)
		}
	}
	if id := DungeonMapID("crypt", 2); id != "gen:dungeon:crypt:2" || Area(id) != "dungeon:crypt:2" {
		t.Errorf("crypt 2: %s", id)
	}
	if d, f, ok := ParseDungeonMapID("gen:dungeon:old-crypt_2:10"); !ok || d != "old-crypt_2" || f != 10 {
		t.Errorf("%s %d %v", d, f, ok)
	}
	for _, id := range []string{"gen:dungeon:crypt", "gen:dungeon:Crypt:1", "gen:dungeon:crypt:0", "gen:dungeon:a:b:1", "gen:dungeon:crypt:01"} {
		if _, _, ok := ParseDungeonMapID(id); ok {
			t.Errorf("%s parsed", id)
		}
	}
	if Area(FieldID(3, -1)) != "field:3:-1" {
		t.Error(Area(FieldID(3, -1)))
	}
}

// placesHash is the SHA-256 of a map's places, as the golden files
// record it.
func placesHash(places []Place) string {
	sum := sha256.Sum256(EncodePlaces(places))
	return hex.EncodeToString(sum[:])
}

// FuzzDungeonV3 makes floors from any key and style. Each is a valid
// floor or ErrNoMap, never a panic or a hang.
func FuzzDungeonV3(f *testing.F) {
	f.Add([]byte{1}, uint8(0), uint8(10), uint8(10), uint8(1), uint8(3), uint8(1), uint8(1))
	f.Add([]byte("seed"), uint8(1), uint8(40), uint8(20), uint8(2), uint8(5), uint8(3), uint8(3))
	f.Add(make([]byte, 32), uint8(1), uint8(0), uint8(0), uint8(0), uint8(0), uint8(2), uint8(2))
	f.Fuzz(func(t *testing.T, key []byte, method, w, h, room, span, floor, floors uint8) {
		var k [32]byte
		copy(k[:], key)
		s := DungeonStyle{Method: MethodCave, Floors: 1, LevelStep: 1, Tileset: "cave"}
		if method%2 == 1 {
			s.Method, s.Tileset = MethodRooms, [3]string{"crypt", "tower", "ruins"}[method/2%3]
		}
		s.Width = MinDungeonSide + 2*(int(w)%57)
		s.Height = MinDungeonSide + 2*(int(h)%57)
		s.RoomMin = MinRoomSide + 2*(int(room)%4)
		s.RoomMax = min(s.RoomMin+2*(int(span)%6), min(s.Width, s.Height)/3/2*2)
		if s.RoomMax < s.RoomMin {
			s.RoomMax = s.RoomMin
		}
		p := DungeonParams{Style: s, Floors: 1 + int(floors)%4}
		p.Floor = 1 + int(floor)%p.Floors
		if p.Check() != nil {
			return
		}
		fl, err := dungeonV3(rng.FromKey(k), p)
		if err != nil {
			if !errors.Is(err, ErrNoMap) {
				t.Fatalf("%+v: %v", p, err)
			}
			return
		}
		checkMap(t, fl, "entrance")
	})
}

// FuzzTownV3 makes towns from any key, size, gates and buildings. Each
// is a valid town or ErrNoMap.
func FuzzTownV3(f *testing.F) {
	f.Add([]byte{1}, uint8(16), uint8(0), true, uint8(2), uint8(3))
	f.Add([]byte("seed"), uint8(0), uint8(2), false, uint8(5), uint8(0))
	f.Fuzz(func(t *testing.T, key []byte, size, gates uint8, walls bool, houses, shops uint8) {
		var k [32]byte
		copy(k[:], key)
		tt := TownTemplate{
			Size: MinTownSize + 2*(int(size)%97), Gates: 2 + int(gates)%3, Walls: walls, Tileset: "town",
			Decorations: []string{"tree", "well"},
			Buildings: []Building{
				{Model: "shop", Width: 6, Depth: 6, Count: 1 + int(shops)%3, Places: []string{"shop", "shop:tool"}},
				{Model: "house", Width: 4, Depth: 6, Count: 1 + int(houses)%20},
			},
		}
		town, err := townV3(rng.FromKey(k), TownParams{Template: tt})
		if err != nil {
			if !errors.Is(err, ErrNoMap) {
				t.Fatalf("%+v: %v", tt, err)
			}
			return
		}
		checkMap(t, town, "square", "gate", "portal")
	})
}

// BenchmarkDungeonV3 times one floor of each core style.
func BenchmarkDungeonV3(b *testing.B) {
	g, _ := Lookup(3)
	for b.Loop() {
		for i, c := range testStyles {
			if _, err := g.Map(rng.FromKey([32]byte{1, byte(i)}), dungeonParams(b, c.style, c.style.Floors, c.style.Floors)); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkTownV3 times a town of each core template.
func BenchmarkTownV3(b *testing.B) {
	g, _ := Lookup(3)
	for b.Loop() {
		for i, c := range testTemplates {
			if _, err := g.Map(rng.FromKey([32]byte{2, byte(i)}), townParams(b, c.template)); err != nil {
				b.Fatal(err)
			}
		}
	}
}
