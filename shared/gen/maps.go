package gen

import (
	"errors"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Towns, dungeons and named places (strategy sections 8.5 and 8.6,
// proposal 0002, review decision 42). These types build in both
// variants, so the server checks town templates and dungeon styles
// without the procedural tag. The generators that use them need it.
//
// The seed tree adds two kinds of public nodes:
//
//	public
//	├── town(n)                  TownNode: a town's public node
//	└── dungeon
//	    └── <id>
//	        └── floor(f)         FloorNode: a floor's public node
//
// A client gets the node of each map it enters, as for fields.

// Kinds of generated maps.
const (
	KindField   = "field"
	KindDungeon = "dungeon"
	KindTown    = "town"
)

// Place is a named place of a generated map: a cell with tags, such as
// "blacksmith", "square", "gate" or "entrance". Content finds places by
// tag, not by coordinates (strategy section 8.6).
type Place struct {
	// Tags are the place's tags. The first is its main tag.
	Tags []string
	X, Y int
}

// HasTag reports whether the place has a tag.
func (p Place) HasTag(tag string) bool {
	for _, t := range p.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// EncodePlaces returns the canonical encoding of a map's places, which
// the golden files hash: for each place, its cell as two big-endian u16
// values, its tag count, and each tag with a length byte.
func EncodePlaces(places []Place) []byte {
	var b []byte
	for _, p := range places {
		b = append(b, byte(p.X>>8), byte(p.X), byte(p.Y>>8), byte(p.Y), byte(len(p.Tags))) // #nosec G115 -- cells are below 1024, and a place has a few tags.
		for _, tag := range p.Tags {
			b = append(b, byte(len(tag))) // #nosec G115 -- tags are at most 64 bytes.
			b = append(b, tag...)
		}
	}
	return b
}

// TownNode returns the public node of town n under the public branch.
func TownNode(public rng.Node, n int64) rng.Node { return public.Child("town", n) }

// DungeonNode returns the public node of a dungeon under the public
// branch. Its floors are its children.
func DungeonNode(public rng.Node, id string) rng.Node { return public.Child("dungeon").Child(id) }

// FloorNode returns the public node of a dungeon's floor, from 1.
func FloorNode(dungeon rng.Node, floor int) rng.Node { return dungeon.Child("floor", int64(floor)) }

// Map ID prefixes (strategy section 8.10).
const (
	townPrefix    = "gen:town:"
	dungeonPrefix = "gen:dungeon:"
)

// TownID returns the ID of town n, such as t7. Places name it in their
// area, such as town:t7.
func TownID(n int64) string { return "t" + strconv.FormatInt(n, 10) }

// TownMapID returns the map ID of town n, such as gen:town:t7.
func TownMapID(n int64) string { return townPrefix + TownID(n) }

// ParseTownMapID reads a town's number from its map ID.
func ParseTownMapID(id string) (int64, bool) {
	rest, ok := strings.CutPrefix(id, townPrefix+"t")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || n < 1 || TownMapID(n) != id {
		return 0, false
	}
	return n, true
}

// MaxDungeonID is the longest dungeon ID.
const MaxDungeonID = 32

// ValidDungeonID reports whether a dungeon ID is 1 to 32 lowercase
// letters, digits, _ and -.
func ValidDungeonID(id string) bool {
	if id == "" || len(id) > MaxDungeonID {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// DungeonMapID returns the map ID of a dungeon's floor, such as
// gen:dungeon:crypt:2.
func DungeonMapID(id string, floor int) string {
	return dungeonPrefix + id + ":" + strconv.Itoa(floor)
}

// ParseDungeonMapID reads a dungeon's ID and floor from a map ID.
func ParseDungeonMapID(mapID string) (id string, floor int, ok bool) {
	rest, found := strings.CutPrefix(mapID, dungeonPrefix)
	if !found {
		return "", 0, false
	}
	id, fs, found := strings.Cut(rest, ":")
	if !found || !ValidDungeonID(id) {
		return "", 0, false
	}
	f, err := strconv.Atoi(fs)
	if err != nil || f < 1 || DungeonMapID(id, f) != mapID {
		return "", 0, false
	}
	return id, f, true
}

// Area returns the area of a generated map, which places report: the map
// ID without "gen:", such as town:t7, dungeon:crypt:2 or field:3:-1.
func Area(mapID string) string { return strings.TrimPrefix(mapID, "gen:") }

// MaxTag is the longest place tag.
const MaxTag = 64

// ValidTag reports whether a place tag is 1 to 64 lowercase letters,
// digits, _, -, . and :, such as blacksmith or shop:tool.
func ValidTag(tag string) bool {
	if tag == "" || len(tag) > MaxTag {
		return false
	}
	for _, r := range tag {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune("_-.:", r) {
			return false
		}
	}
	return true
}

// validSlot reports whether a tileset or slot name is 1 to 32 lowercase
// letters, digits and _.
func validSlot(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// ValidSlot reports whether a tileset, model or texture name is 1 to 32
// lowercase letters, digits and _.
func ValidSlot(s string) bool { return validSlot(s) }

// Dungeon methods (strategy section 8.5, "Dungeons").
const (
	// MethodCave makes caves with cellular automata.
	MethodCave = "cave"
	// MethodRooms makes rooms joined by corridors: crypts, towers and
	// ruins.
	MethodRooms = "rooms"
)

// DungeonStyle is a record of data/dungeon_styles.yml: how a dungeon's
// floors look and how hard they get.
type DungeonStyle struct {
	// Method is MethodCave or MethodRooms.
	Method string
	// Floors is the number of floors a dungeon of the style has by
	// default.
	Floors int
	// Width and Height are a floor's size in cells: even, 48 to 512.
	Width, Height int
	// RoomMin and RoomMax are the sides of rooms in cells: even, 6 to
	// 40. Caves ignore them.
	RoomMin, RoomMax int
	// Tileset names the dungeon tileset of the generator.
	Tileset string
	// LevelStep is how many levels each floor's band is above the one
	// before it, 1 to 50.
	LevelStep int
}

// Limits of styles and templates.
const (
	MaxFloors      = 20
	MinDungeonSide = 48
	MaxDungeonSide = 512
	MinRoomSide    = 6
	MaxRoomSide    = 40
	MaxLevelStep   = 50
	MinTownSize    = 64
	MaxTownSize    = 256
	MaxBuildings   = 32
	MaxBuildingSum = 200
	MaxFootprint   = 12
	MaxPlaces      = 8
	MaxDecorations = 8
)

// Check returns the style's first problem, or nil. It does not check the
// tileset against a generator.
func (s DungeonStyle) Check() error {
	switch {
	case s.Method != MethodCave && s.Method != MethodRooms:
		return errors.New("method is cave or rooms")
	case s.Floors < 1 || s.Floors > MaxFloors:
		return errors.New("floors is 1 to " + strconv.Itoa(MaxFloors))
	case !evenIn(s.Width, MinDungeonSide, MaxDungeonSide) || !evenIn(s.Height, MinDungeonSide, MaxDungeonSide):
		return errors.New("floor_size is two even numbers from " + strconv.Itoa(MinDungeonSide) + " to " + strconv.Itoa(MaxDungeonSide))
	case !evenIn(s.RoomMin, MinRoomSide, MaxRoomSide) || !evenIn(s.RoomMax, s.RoomMin, MaxRoomSide):
		return errors.New("room_size is two even numbers from " + strconv.Itoa(MinRoomSide) + " to " + strconv.Itoa(MaxRoomSide) + ", the smaller first")
	case 3*s.RoomMax > min(s.Width, s.Height):
		return errors.New("room_size's largest room is above a third of the floor's smaller side")
	case !validSlot(s.Tileset):
		return errors.New("tileset is 1 to 32 lowercase letters, digits and _")
	case s.LevelStep < 1 || s.LevelStep > MaxLevelStep:
		return errors.New("level_step is 1 to " + strconv.Itoa(MaxLevelStep))
	}
	return nil
}

func evenIn(v, lo, hi int) bool { return v >= lo && v <= hi && v%2 == 0 }

// DungeonParams are the inputs of one floor besides its node.
type DungeonParams struct {
	Style DungeonStyle
	// Floor is the floor, from 1. Floors is the dungeon's floor count:
	// the last floor has the boss room and no exit.
	Floor, Floors int
}

// Check returns the floor's first problem, or nil.
func (p DungeonParams) Check() error {
	if err := p.Style.Check(); err != nil {
		return err
	}
	if p.Floors < 1 || p.Floors > 99 || p.Floor < 1 || p.Floor > p.Floors {
		return errors.New("a floor is 1 to the dungeon's floors, at most 99")
	}
	return nil
}

// TownTemplate is a record of data/town_templates.yml: a town's size,
// gates, walls and buildings.
type TownTemplate struct {
	// Size is the town map's side in cells: even, 64 to 256.
	Size int
	// Gates is 2 to 4: the gates in the town's sides, each with a main
	// road to the square.
	Gates int
	// Walls puts a wall around the town. Without it, a hedge of trees.
	Walls bool
	// Tileset names the town tileset of the generator.
	Tileset string
	// Buildings are placed in order. A building with places must be
	// placed, or the attempt fails.
	Buildings []Building
	// Decorations are props of the tileset that fill empty lots, such
	// as tree, well or crate.
	Decorations []string
}

// Building is a building of a town template.
type Building struct {
	// Model is the building's model slot in the town's tileset, such as
	// smithy. The tileset table maps it to an asset.
	Model string
	// Width and Depth are its footprint in cells, facing south: even, 2
	// to 12.
	Width, Depth int
	// Count is how many to place, 1 to 64.
	Count int
	// Places are the tags of the named places at its door, such as
	// blacksmith or shop:tool. Each gets its own cell.
	Places []string
}

// Check returns the template's first problem, or nil. It does not check
// the tileset and decorations against a generator.
func (t TownTemplate) Check() error {
	switch {
	case !evenIn(t.Size, MinTownSize, MaxTownSize):
		return errors.New("size is an even number from " + strconv.Itoa(MinTownSize) + " to " + strconv.Itoa(MaxTownSize))
	case t.Gates < 2 || t.Gates > 4:
		return errors.New("gates is 2 to 4")
	case !validSlot(t.Tileset):
		return errors.New("tileset is 1 to 32 lowercase letters, digits and _")
	case len(t.Buildings) < 1 || len(t.Buildings) > MaxBuildings:
		return errors.New("buildings is a list of 1 to " + strconv.Itoa(MaxBuildings))
	case len(t.Decorations) > MaxDecorations:
		return errors.New("decorations is a list of at most " + strconv.Itoa(MaxDecorations))
	}
	for _, d := range t.Decorations {
		if !validSlot(d) {
			return errors.New("decorations: " + strconv.Quote(d) + " is not a prop name: 1 to 32 lowercase letters, digits and _")
		}
	}
	sum := 0
	for i, b := range t.Buildings {
		where := "buildings[" + strconv.Itoa(i) + "]: "
		switch {
		case !validSlot(b.Model):
			return errors.New(where + "model is 1 to 32 lowercase letters, digits and _")
		case !evenIn(b.Width, 2, MaxFootprint) || !evenIn(b.Depth, 2, MaxFootprint):
			return errors.New(where + "footprint is two even numbers from 2 to " + strconv.Itoa(MaxFootprint))
		case b.Count < 1 || b.Count > 64:
			return errors.New(where + "count is 1 to 64")
		case len(b.Places) > MaxPlaces:
			return errors.New(where + "places is a list of at most " + strconv.Itoa(MaxPlaces))
		case len(b.Places) > b.Width/2:
			return errors.New(where + "places has more tags than the footprint's width has cells for, one every 2 cells")
		}
		for _, p := range b.Places {
			if !ValidTag(p) {
				return errors.New(where + "places: " + strconv.Quote(p) + " is not a tag: 1 to 64 lowercase letters, digits, _, -, . and :")
			}
		}
		sum += b.Count
	}
	if sum > MaxBuildingSum {
		return errors.New("the buildings' counts add up to more than " + strconv.Itoa(MaxBuildingSum))
	}
	return nil
}

// TownParams are the inputs of a town besides its node.
type TownParams struct {
	Template TownTemplate
}

// Map params are the canonical encoding of a dungeon floor's or a town's
// inputs. The server sends them with the map's node, and the client's
// module decodes the same bytes, so both run the same inputs.
const (
	paramsDungeon = 1
	paramsTown    = 2
	paramsVersion = 1
	// MaxParams is the longest encoding.
	MaxParams = 16 << 10
)

// ErrParams is the error for map params that do not decode or do not
// pass their checks.
var ErrParams = errors.New("gen: invalid map params")

type paramWriter struct{ b []byte }

func (w *paramWriter) u8(v int)  { w.b = append(w.b, byte(v)) }             // #nosec G115 -- checked values below 256.
func (w *paramWriter) u16(v int) { w.b = append(w.b, byte(v>>8), byte(v)) } // #nosec G115 -- checked values below 65536.
func (w *paramWriter) str(s string) {
	w.u8(len(s))
	w.b = append(w.b, s...)
}

type paramReader struct {
	b   []byte
	bad bool
}

func (r *paramReader) u8() int {
	if len(r.b) < 1 {
		r.bad = true
		return 0
	}
	v := int(r.b[0])
	r.b = r.b[1:]
	return v
}

func (r *paramReader) u16() int {
	if len(r.b) < 2 {
		r.bad = true
		return 0
	}
	v := int(r.b[0])<<8 | int(r.b[1])
	r.b = r.b[2:]
	return v
}

func (r *paramReader) str() string {
	n := r.u8()
	if len(r.b) < n {
		r.bad = true
		return ""
	}
	s := string(r.b[:n])
	r.b = r.b[n:]
	return s
}

// EncodeDungeon returns the params of a floor. It checks them first.
func EncodeDungeon(p DungeonParams) ([]byte, error) {
	if err := p.Check(); err != nil {
		return nil, wrapError(ErrParams, err.Error())
	}
	w := &paramWriter{}
	w.u8(paramsDungeon)
	w.u8(paramsVersion)
	method := 0
	if p.Style.Method == MethodRooms {
		method = 1
	}
	w.u8(method)
	w.u16(p.Style.Width)
	w.u16(p.Style.Height)
	w.u16(p.Style.RoomMin)
	w.u16(p.Style.RoomMax)
	w.str(p.Style.Tileset)
	w.u8(p.Style.Floors)
	w.u8(p.Style.LevelStep)
	w.u8(p.Floor)
	w.u8(p.Floors)
	return w.b, nil
}

// DecodeDungeon reads a floor's params.
func DecodeDungeon(b []byte) (DungeonParams, error) {
	r := &paramReader{b: b}
	if r.u8() != paramsDungeon || r.u8() != paramsVersion {
		return DungeonParams{}, wrapError(ErrParams, "not dungeon params of version 1")
	}
	var p DungeonParams
	switch r.u8() {
	case 0:
		p.Style.Method = MethodCave
	case 1:
		p.Style.Method = MethodRooms
	default:
		r.bad = true
	}
	p.Style.Width, p.Style.Height = r.u16(), r.u16()
	p.Style.RoomMin, p.Style.RoomMax = r.u16(), r.u16()
	p.Style.Tileset = r.str()
	p.Style.Floors, p.Style.LevelStep = r.u8(), r.u8()
	p.Floor, p.Floors = r.u8(), r.u8()
	if r.bad || len(r.b) != 0 {
		return DungeonParams{}, wrapError(ErrParams, "the dungeon params are cut short or too long")
	}
	if err := p.Check(); err != nil {
		return DungeonParams{}, wrapError(ErrParams, err.Error())
	}
	return p, nil
}

// EncodeTown returns the params of a town. It checks them first.
func EncodeTown(p TownParams) ([]byte, error) {
	t := p.Template
	if err := t.Check(); err != nil {
		return nil, wrapError(ErrParams, err.Error())
	}
	w := &paramWriter{}
	w.u8(paramsTown)
	w.u8(paramsVersion)
	w.u16(t.Size)
	w.u8(t.Gates)
	walls := 0
	if t.Walls {
		walls = 1
	}
	w.u8(walls)
	w.str(t.Tileset)
	w.u8(len(t.Decorations))
	for _, d := range t.Decorations {
		w.str(d)
	}
	w.u8(len(t.Buildings))
	for _, b := range t.Buildings {
		w.str(b.Model)
		w.u8(b.Width)
		w.u8(b.Depth)
		w.u8(b.Count)
		w.u8(len(b.Places))
		for _, pl := range b.Places {
			w.str(pl)
		}
	}
	if len(w.b) > MaxParams {
		return nil, wrapError(ErrParams, "the town's params are too long")
	}
	return w.b, nil
}

// DecodeTown reads a town's params.
func DecodeTown(b []byte) (TownParams, error) {
	if len(b) > MaxParams {
		return TownParams{}, wrapError(ErrParams, "the town's params are too long")
	}
	r := &paramReader{b: b}
	if r.u8() != paramsTown || r.u8() != paramsVersion {
		return TownParams{}, wrapError(ErrParams, "not town params of version 1")
	}
	var t TownTemplate
	t.Size = r.u16()
	t.Gates = r.u8()
	switch r.u8() {
	case 0:
	case 1:
		t.Walls = true
	default:
		r.bad = true
	}
	t.Tileset = r.str()
	n := r.u8()
	for range min(n, MaxDecorations+1) {
		t.Decorations = append(t.Decorations, r.str())
	}
	n = r.u8()
	for range min(n, MaxBuildings+1) {
		var bd Building
		bd.Model = r.str()
		bd.Width, bd.Depth, bd.Count = r.u8(), r.u8(), r.u8()
		k := r.u8()
		for range min(k, MaxPlaces+1) {
			bd.Places = append(bd.Places, r.str())
		}
		t.Buildings = append(t.Buildings, bd)
		if r.bad {
			break
		}
	}
	if r.bad || len(r.b) != 0 {
		return TownParams{}, wrapError(ErrParams, "the town params are cut short or too long")
	}
	if err := t.Check(); err != nil {
		return TownParams{}, wrapError(ErrParams, err.Error())
	}
	return TownParams{Template: t}, nil
}

// MapParams are the inputs of a dungeon floor or a town for
// Generator.Map: the kind and the encoded params.
type MapParams struct {
	// Kind is KindDungeon or KindTown.
	Kind   string
	Params []byte
}

// fieldError is an error with details after a sentinel error.
type fieldError struct {
	err    error
	detail string
}

func (e *fieldError) Error() string { return e.err.Error() + ": " + e.detail }
func (e *fieldError) Unwrap() error { return e.err }

func wrapError(err error, detail string) error { return &fieldError{err, detail} }

// Errors of dungeons and towns.
var (
	// ErrNoMap is the error when every attempt at a dungeon floor or a
	// town failed validation.
	ErrNoMap = errors.New("gen: no attempt made a valid map")
	// ErrTileset is the error for a tileset, prop or decoration the
	// generator lacks.
	ErrTileset = errors.New("gen: unknown tileset or prop")
	// ErrKind is the error for a map kind the generator does not make.
	ErrKind = errors.New("gen: the generator does not make this kind of map")
)
