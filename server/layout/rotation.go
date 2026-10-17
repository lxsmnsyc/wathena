package layout

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/schedule"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Rotating dungeons and world changes (strategy section 8.8, proposal
// 0002 review decision 47, decision 0045):
//
//   - A dungeon style may set a rotation: daily or weekly, at a time of
//     the server's time zone. `wactl world new` marks one dungeon of a
//     world as rotating: the first, in the layout's order, whose style
//     has one.
//   - A rotating dungeon's floors carry the rotation's date in their map
//     IDs, such as gen:dungeon:crypt:2026-10-31:2, and generate from the
//     public node dungeon(id, date), child(child(public, "dungeon"), id,
//     20261031).
//   - A region is the fields whose nearest town, by warps, is one town,
//     ties to the lower number (decision 0042). A world change gives a
//     region its next season: its fields generate again from
//     child(field(x, y), "season", n) with a new biome. Map IDs, links
//     and level bands stay.
//   - The database keeps the layout as `wactl world new` made it. Apply
//     turns it into the layout that runs, from the active dates and the
//     regions' seasons.

// Rotation periods.
const (
	EveryDaily  = "daily"
	EveryWeekly = "weekly"
)

// RotationRule is a dungeon style's rotation: daily or weekly at a time
// of day, and the day of a weekly one.
type RotationRule struct {
	// Every is EveryDaily or EveryWeekly.
	Every string
	// At is the time of day, HH:MM, in the server's time zone.
	At string
	// Weekday is a weekly rotation's day, such as friday.
	Weekday string
}

// rotationFields are the fields of a style's rotation.
var rotationFields = []string{"every", "at", "weekday"}

// ParseRotation reads a style record's rotation field: { every, at,
// weekday }. at is 00:00 when left out, and weekday is required for a
// weekly rotation alone.
func ParseRotation(v any) (*RotationRule, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("rotation is { every: daily or weekly, at: HH:MM, weekday }")
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if !slices.Contains(rotationFields, k) {
			return nil, fmt.Errorf("rotation: %q is not a field; the fields are %s", k, strings.Join(rotationFields, ", "))
		}
	}
	r := &RotationRule{At: "00:00"}
	r.Every, _ = m["every"].(string)
	if v, set := m["at"]; set {
		if r.At, ok = v.(string); !ok {
			return nil, errors.New("rotation: at is a time of day, such as \"04:00\"")
		}
	}
	if v, set := m["weekday"]; set {
		if r.Weekday, ok = v.(string); !ok {
			return nil, errors.New("rotation: weekday is a day's name, such as friday")
		}
	}
	if err := r.Check(); err != nil {
		return nil, err
	}
	return r, nil
}

// Check returns the rule's first problem, or nil.
func (r RotationRule) Check() error {
	_, err := r.Spec()
	return err
}

// Spec returns the rule as a schedule.
func (r RotationRule) Spec() (schedule.Spec, error) {
	h, m, err := schedule.ParseClock(r.At)
	if err != nil {
		return schedule.Spec{}, fmt.Errorf("rotation: at: %w", err)
	}
	switch r.Every {
	case EveryDaily:
		if r.Weekday != "" {
			return schedule.Spec{}, errors.New("rotation: a daily rotation has no weekday")
		}
		return schedule.Spec{Kind: schedule.Daily, Hour: h, Minute: m}, nil
	case EveryWeekly:
		d, err := schedule.ParseWeekday(r.Weekday)
		if err != nil {
			return schedule.Spec{}, fmt.Errorf("rotation: weekday: %w", err)
		}
		return schedule.Spec{Kind: schedule.Weekly, Weekday: d, Hour: h, Minute: m}, nil
	}
	return schedule.Spec{}, fmt.Errorf("rotation: every is %s or %s", EveryDaily, EveryWeekly)
}

// String describes the rule, such as "weekly on friday at 20:00".
func (r RotationRule) String() string {
	s, err := r.Spec()
	if err != nil {
		return r.Every
	}
	return s.String()
}

// Rotation is a world's rotating dungeon: its ID and its style's rule.
type Rotation struct {
	Dungeon string
	// Style is the dungeon's style record.
	Style string
	Rule  RotationRule
}

// PickRotation returns the dungeon a new world rotates: the first
// dungeon, in the layout's order, whose style has a rotation. ok is
// false when no style of the world's dungeons has one.
func PickRotation(l *Layout, styles []StyleRecord) (Rotation, bool) {
	for _, m := range l.Maps {
		if m.Kind != gen.KindDungeon || m.Floor != 1 {
			continue
		}
		if s, ok := findStyle(styles, m.Record); ok && s.Rotation != nil {
			return Rotation{Dungeon: m.Dungeon, Style: s.Name, Rule: *s.Rotation}, true
		}
	}
	return Rotation{}, false
}

// dateLayout is the form of rotation dates.
const dateLayout = "2006-01-02"

// ValidDate reports whether s is a date, such as 2026-10-31.
func ValidDate(s string) bool {
	t, err := time.Parse(dateLayout, s)
	return err == nil && t.Format(dateLayout) == s
}

// DateOf returns the date of an instant in a time zone, such as
// 2026-10-31.
func DateOf(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format(dateLayout)
}

// DateNumber is a date as a number, 20261031, as the seed tree's index
// of a rotation's nodes. It is 0 for a bad date.
func DateNumber(date string) int64 { return dateIndex(date) }

// dateIndex is a date as a number, 20261031. It is 0 for a bad date.
func dateIndex(date string) int64 {
	if !ValidDate(date) {
		return 0
	}
	n, _ := strconv.ParseInt(strings.ReplaceAll(date, "-", ""), 10, 64)
	return n
}

// RotationNode returns the public node of a rotating dungeon's date:
// child(child(public, "dungeon"), id, yyyymmdd). Its floors are its
// children, as gen.FloorNode makes them.
func RotationNode(public rng.Node, dungeon, date string) rng.Node {
	return public.Child("dungeon").Child(dungeon, dateIndex(date))
}

// FloorID returns the map ID of a rotating dungeon's floor, such as
// gen:dungeon:crypt:2026-10-31:2.
func FloorID(dungeon, date string, floor int) string {
	return "gen:dungeon:" + dungeon + ":" + date + ":" + strconv.Itoa(floor)
}

// ParseFloorID reads a dungeon floor's map ID: gen:dungeon:<id>:<floor>,
// or a rotating floor's gen:dungeon:<id>:<date>:<floor>, whose date it
// returns too.
func ParseFloorID(mapID string) (dungeon, date string, floor int, ok bool) {
	if id, f, ok := gen.ParseDungeonMapID(mapID); ok {
		return id, "", f, true
	}
	rest, found := strings.CutPrefix(mapID, "gen:dungeon:")
	if !found {
		return "", "", 0, false
	}
	parts := strings.Split(rest, ":")
	if len(parts) != 3 || !gen.ValidDungeonID(parts[0]) || !ValidDate(parts[1]) {
		return "", "", 0, false
	}
	f, err := strconv.Atoi(parts[2])
	if err != nil || f < 1 || FloorID(parts[0], parts[1], f) != mapID {
		return "", "", 0, false
	}
	return parts[0], parts[1], f, true
}

// SeasonNode returns the public node of a field's season n, from 1:
// child(field(x, y), "season", n). Season 0 is the field's own node.
func SeasonNode(public rng.Node, x, y int64, season int) rng.Node {
	n := gen.FieldNode(public, x, y)
	if season <= 0 {
		return n
	}
	return n.Child("season", int64(season))
}

// MapNode returns the public node a map of a layout generates from: a
// field's, by its season, a town's, or a dungeon floor's, by its date.
// A home map has none.
func MapNode(public rng.Node, m Map) rng.Node {
	switch m.Kind {
	case gen.KindField:
		return SeasonNode(public, m.X, m.Y, m.Season)
	case gen.KindTown:
		return gen.TownNode(public, m.Town)
	case gen.KindDungeon:
		d := gen.DungeonNode(public, m.Dungeon)
		if m.Date != "" {
			d = RotationNode(public, m.Dungeon, m.Date)
		}
		return gen.FloorNode(d, m.Floor)
	}
	return rng.Node{}
}

// Region is one town's fields: those whose nearest town by warps is
// the town, ties to the lower number.
type Region struct {
	// Town is the town's ID, such as t2, which names the region.
	Town string
	// Fields are the fields' map IDs, in the layout's order.
	Fields []string
}

// Regions returns the regions of a layout, by town number. A town that
// is nearest to no field has none.
func Regions(l *Layout) []Region {
	a := NewAtlas(l, AtlasOptions{})
	var out []Region
	for _, t := range l.Towns() {
		r := Region{Town: gen.TownID(t.Town)}
		for _, m := range l.Maps {
			if m.Kind != gen.KindField {
				continue
			}
			if nt, ok := a.NearestTown(m.ID); ok && nt.ID == t.ID {
				r.Fields = append(r.Fields, m.ID)
			}
		}
		if len(r.Fields) > 0 {
			out = append(out, r)
		}
	}
	return out
}

// RegionOf returns the region of a field, by its coordinates: its town's
// ID. ok is false for a field the layout lacks.
func RegionOf(regions []Region, x, y int64) (string, bool) {
	id := gen.FieldID(x, y)
	for _, r := range regions {
		if slices.Contains(r.Fields, id) {
			return r.Town, true
		}
	}
	return "", false
}

// mainBiome returns the biome most of a region's fields have, ties to
// the lower index.
func mainBiome(l *Layout, fields []string) int {
	count := map[int]int{}
	best := -1
	for _, id := range fields {
		if m := l.Map(id); m != nil && m.Biome >= 0 {
			count[m.Biome]++
		}
	}
	for _, b := range slices.Sorted(maps.Keys(count)) {
		if best < 0 || count[b] > count[best] {
			best = b
		}
	}
	return best
}

// SeasonBiome returns the biome of a region's season n, from 1: a draw,
// by IntN over the generator's biomes but last, the biome of the season
// before, from the stream of child(child(child(public, "region"),
// town), "season", n). A generator with one biome keeps it.
func SeasonBiome(public rng.Node, town string, season, last, biomes int) int {
	if biomes <= 1 {
		return max(last, 0)
	}
	var others []int
	for b := range biomes {
		if b != last {
			others = append(others, b)
		}
	}
	s := public.Child("region").Child(town).Child("season", int64(season)).Stream()
	return others[s.IntN(len(others))]
}

// State is what changes the layout that runs: the active date of each
// rotating dungeon, by dungeon ID, and each region's season, by town ID.
// A region left out is in season 0.
type State struct {
	Dates   map[string]string
	Seasons map[string]int
}

// Clone returns a copy of the state.
func (s State) Clone() State {
	return State{Dates: maps.Clone(s.Dates), Seasons: maps.Clone(s.Seasons)}
}

// Apply returns the layout that runs in a state:
//
//   - Each floor of a dungeon with a date takes its dated map ID, the
//     date, its stored ID as Base, and nomemo. Links follow the IDs.
//   - Each field of a region in season n > 0 takes the season and its
//     biome: the region's biome of season 0, the one most of its fields
//     have, then SeasonBiome for each season to n.
//
// biomes is the number of the generator's biomes. Names, bands and the
// rest stay. l is not changed.
func (l *Layout) Apply(public rng.Node, biomes int, st State) *Layout {
	out := &Layout{Maps: slices.Clone(l.Maps), Links: slices.Clone(l.Links), Home: l.Home, Attempt: l.Attempt}
	ids := map[string]string{}
	for i, m := range out.Maps {
		if m.Kind != gen.KindDungeon {
			continue
		}
		date, ok := st.Dates[m.Dungeon]
		if !ok || date == "" {
			continue
		}
		base := m.ID
		if m.Base != "" {
			base = m.Base
		}
		id := FloorID(m.Dungeon, date, m.Floor)
		ids[m.ID] = id
		out.Maps[i].ID, out.Maps[i].Base, out.Maps[i].Date, out.Maps[i].NoMemo = id, base, date, true
	}
	for i, k := range out.Links {
		if id, ok := ids[k.A.Map]; ok {
			out.Links[i].A.Map = id
		}
		if id, ok := ids[k.B.Map]; ok {
			out.Links[i].B.Map = id
		}
	}
	if id, ok := ids[out.Home]; ok {
		out.Home = id
	}
	if len(st.Seasons) == 0 {
		return out
	}
	for _, r := range Regions(l) {
		n := st.Seasons[r.Town]
		if n <= 0 {
			continue
		}
		b := mainBiome(l, r.Fields)
		for k := 1; k <= n; k++ {
			b = SeasonBiome(public, r.Town, k, b, biomes)
		}
		for i, m := range out.Maps {
			if m.Kind == gen.KindField && slices.Contains(r.Fields, m.ID) {
				out.Maps[i].Season, out.Maps[i].Biome = n, b
			}
		}
	}
	return out
}

// StoredID returns the map ID a map has in the stored layout: a rotating
// floor's Base, or its ID.
func (m Map) StoredID() string {
	if m.Base != "" {
		return m.Base
	}
	return m.ID
}
