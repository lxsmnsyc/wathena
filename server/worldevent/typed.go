package worldevent

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Typed events (proposal 0002, review decision 31): the built-in event
// types of strategy section 8.8 that run without a script.
//   - A record with type boss, invasion or treasure_hunt is a typed
//     event. A record without type is a script event, as before. A
//     typed event may still have a script, which gets the run with
//     context:get_event().
//   - The fields of each type are fields of the record itself, beside
//     type, such as monsters and level_band for a boss.
//   - Monster, item and currency names are checked against the game
//     data when the mods load (server/gamedata). Maps are checked when
//     the server starts, since the maps that run are known then.

// Types of typed events.
const (
	TypeScript   = ""
	TypeBoss     = "boss"
	TypeInvasion = "invasion"
	TypeTreasure = "treasure_hunt"
)

// Who gets rewards.
const (
	ToKiller       = "killer"
	ToParticipants = "participants"
	ToEveryone     = "everyone_on_map"
)

// Limits of typed events.
const (
	// MaxChests is the most chests of a treasure hunt.
	MaxChests = 200
	// MaxWaves is the most waves of an invasion, and MaxWaveMonsters
	// the most monsters of one wave.
	MaxWaves        = 50
	MaxWaveMonsters = 500
	// MaxHints is the most hint NPCs of a treasure hunt.
	MaxHints = 20
	// MaxRewardItems is the most item lines of a reward.
	MaxRewardItems = 10
	// MaxLevel is the highest level of a level band.
	MaxLevel = 1000
	// DefaultMinShare is the share of the damage, in percent, that makes
	// a player a participant.
	DefaultMinShare = 1.0
)

// Band is a level band: the maps whose monsters' level falls in it.
type Band struct{ Min, Max int }

// Contains reports whether a level is in the band.
func (b Band) Contains(level int) bool { return level >= b.Min && level <= b.Max }

// Boss is a boss event: one of Monsters appears on a map of the band,
// or of Maps.
type Boss struct {
	// Monsters are the monsters' names; MonsterIDs their IDs, which
	// server/gamedata fills.
	Monsters   []string
	MonsterIDs []uint32
	Band       *Band
	Maps       []string
	// Announce announces the boss to the world when it appears, dies or
	// leaves, as rAthena's boss_monster spawns show the boss.
	Announce bool
}

// Invasion is an invasion of a town by waves of monsters.
type Invasion struct {
	// Town is the map's asset name, such as map/prontera.
	Town  string
	Waves []Wave
}

// Wave is one wave of an invasion.
type Wave struct {
	Monsters []WaveMonster
	// Cleared starts the wave once the waves before it died. Otherwise
	// After is how long after the wave before it started this one
	// starts, or after the run started for the first wave.
	Cleared bool
	After   time.Duration
}

// WaveMonster is one kind of monster of a wave.
type WaveMonster struct {
	Monster string
	ID      uint32
	Amount  int
}

// Treasure is a treasure hunt: chests on random cells of the band's
// maps, or of Maps, with NPCs that give hints.
type Treasure struct {
	Chests int
	Band   *Band
	Maps   []string
	// Chest is the chest's monster name, and ChestID its ID. Its drops
	// are the loot.
	Chest   string
	ChestID uint32
	Hints   []Hint
}

// Hint is an NPC of a treasure hunt that tells the map and direction of
// a chest that is still there.
type Hint struct {
	// Map is the map's asset name.
	Map  string
	X, Y int
	// Sprite is the NPC sprite's name, and SpriteID its ID, which
	// server/gamedata fills.
	Sprite      string
	SpriteID    uint32
	DisplayName string
}

// Rewards are what a typed event gives.
type Rewards struct {
	To       string
	Items    []RewardItem
	Zeny     int64
	BaseExp  int64
	JobExp   int64
	Currency map[string]int64
	// MinShare is the share of the damage, in percent, that makes a
	// player a participant.
	MinShare float64
}

// RewardItem is one item line of a reward.
type RewardItem struct {
	Item   string
	ID     uint32
	Amount int
}

// typedFields are the fields of each type, beside the common ones.
var typedFields = map[string][]string{
	TypeBoss:     {"monsters", "level_band", "maps", "announce"},
	TypeInvasion: {"town", "waves"},
	TypeTreasure: {"chests", "level_band", "maps", "chest", "hint_npcs"},
}

// commonFields are the fields of every event.
var commonFields = []string{"trigger", "duration_minutes", "script", "map", "type", "rewards"}

// knownFields are the fields of an event of a type.
func knownFields(typ string) []string { return append(slices.Clone(commonFields), typedFields[typ]...) }

func parseTyped(d *Def, f map[string]any) []string {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	switch d.Type {
	case TypeBoss:
		b := &Boss{Announce: true}
		names, err := nameList(f["monsters"])
		if err != nil {
			add("monsters: %v", err)
		}
		b.Monsters = names
		b.Band, b.Maps, err = place(f)
		if err != nil {
			add("%v", err)
		}
		if v, ok := f["announce"]; ok {
			a, ok := v.(bool)
			if !ok {
				add("announce must be true or false")
			}
			b.Announce = a
		}
		d.Boss = b
	case TypeInvasion:
		in := &Invasion{}
		m, ok := f["town"].(string)
		if !ok || !mapName(m) {
			add("town is required: a town's map name, such as prontera")
		} else {
			in.Town = AssetOf(m)
		}
		waves, err := parseWaves(f["waves"])
		if err != nil {
			add("waves: %v", err)
		}
		in.Waves = waves
		d.Invasion = in
	case TypeTreasure:
		t := &Treasure{}
		n, ok := f["chests"].(int)
		if !ok || n < 1 || n > MaxChests {
			add("chests must be 1 to %d", MaxChests)
		}
		t.Chests = n
		var err error
		t.Band, t.Maps, err = place(f)
		if err != nil {
			add("%v", err)
		}
		t.Chest, _ = f["chest"].(string)
		if t.Chest == "" {
			add("chest is required: a monster whose drops are the loot, such as TREASURE_BOX1")
		}
		t.Hints, err = parseHints(f["hint_npcs"])
		if err != nil {
			add("hint_npcs: %v", err)
		}
		d.Treasure = t
	}
	if v, ok := f["rewards"]; ok {
		r, err := parseRewards(v)
		if err != nil {
			add("rewards: %v", err)
		}
		d.Rewards = r
	}
	return errs
}

func mapName(m string) bool { return m != "" && len(m) <= 64 && !strings.ContainsAny(m, " \\") }

// nameList reads a list of names, such as monster names.
func nameList(v any) ([]string, error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil, errors.New("want a list of monster names, such as [ORK_HERO, ORC_LORD]")
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		s, ok := e.(string)
		if !ok || s == "" {
			return nil, errors.New("want a list of monster names, such as [ORK_HERO, ORC_LORD]")
		}
		out = append(out, s)
	}
	return out, nil
}

// place reads level_band or maps: one of them is required.
func place(f map[string]any) (*Band, []string, error) {
	bv, hasBand := f["level_band"]
	mv, hasMaps := f["maps"]
	switch {
	case hasBand && hasMaps:
		return nil, nil, errors.New("level_band and maps cannot both be set")
	case hasBand:
		list, ok := bv.([]any)
		if ok && len(list) == 2 {
			lo, ok1 := list[0].(int)
			hi, ok2 := list[1].(int)
			if ok1 && ok2 && lo >= 1 && hi >= lo && hi <= MaxLevel {
				return &Band{lo, hi}, nil, nil
			}
		}
		return nil, nil, fmt.Errorf("level_band must be [lowest, highest], levels from 1 to %d, such as [20, 40]", MaxLevel)
	case hasMaps:
		list, ok := mv.([]any)
		if !ok || len(list) == 0 {
			return nil, nil, errors.New("maps must be a list of map names, such as [prt_fild08]")
		}
		var out []string
		for _, e := range list {
			s, ok := e.(string)
			if !ok || !mapName(s) {
				return nil, nil, errors.New("maps must be a list of map names, such as [prt_fild08]")
			}
			a := AssetOf(s)
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
		return nil, out, nil
	}
	return nil, nil, errors.New("level_band or maps is required: level_band picks fields whose monsters fall in it, such as [20, 40]")
}

func parseWaves(v any) ([]Wave, error) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 || len(list) > MaxWaves {
		return nil, fmt.Errorf("want a list of 1 to %d waves, each with monsters and start", MaxWaves)
	}
	var out []Wave
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("wave %d: want a mapping with monsters and start", i+1)
		}
		for k := range m {
			if k != "monsters" && k != "start" {
				return nil, fmt.Errorf("wave %d: %q is not a field; a wave has monsters and start", i+1, k)
			}
		}
		var w Wave
		switch s := m["start"].(type) {
		case string:
			if s == "cleared" {
				w.Cleared = true
				break
			}
			d, err := ParseSpan(s)
			if err != nil {
				return nil, fmt.Errorf("wave %d: start: %w", i+1, err)
			}
			w.After = d
		default:
			return nil, fmt.Errorf("wave %d: start is required: cleared, or a time after the wave before, such as \"120s\"", i+1)
		}
		ml, ok := m["monsters"].([]any)
		if !ok || len(ml) == 0 {
			return nil, fmt.Errorf("wave %d: monsters must be a list such as [{ monster: PORING, amount: 10 }]", i+1)
		}
		total := 0
		for _, x := range ml {
			xm, ok := x.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("wave %d: monsters must be a list such as [{ monster: PORING, amount: 10 }]", i+1)
			}
			for k := range xm {
				if k != "monster" && k != "amount" {
					return nil, fmt.Errorf("wave %d: %q is not a field; a wave's monster has monster and amount", i+1, k)
				}
			}
			name, _ := xm["monster"].(string)
			if name == "" {
				return nil, fmt.Errorf("wave %d: each monster needs monster, a monster name", i+1)
			}
			n := 1
			if a, ok := xm["amount"]; ok {
				n, ok = a.(int)
				if !ok || n < 1 {
					return nil, fmt.Errorf("wave %d: amount must be 1 or more", i+1)
				}
			}
			total += n
			w.Monsters = append(w.Monsters, WaveMonster{Monster: name, Amount: n})
		}
		if total > MaxWaveMonsters {
			return nil, fmt.Errorf("wave %d has %d monsters; the most is %d", i+1, total, MaxWaveMonsters)
		}
		out = append(out, w)
	}
	return out, nil
}

// ParseSpan reads a time such as "120s", "5m" or "1h", up to a week.
func ParseSpan(s string) (time.Duration, error) {
	bad := fmt.Errorf("%q is not a time such as \"120s\", \"5m\" or \"1h\"", s)
	if len(s) < 2 {
		return 0, bad
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 0 {
		return 0, bad
	}
	unit := map[byte]time.Duration{'s': time.Second, 'm': time.Minute, 'h': time.Hour}[s[len(s)-1]]
	if unit == 0 {
		return 0, bad
	}
	d := time.Duration(n) * unit
	if d > MaxDuration*time.Minute {
		return 0, fmt.Errorf("%q is longer than a week", s)
	}
	return d, nil
}

func parseHints(v any) ([]Hint, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok || len(list) > MaxHints {
		return nil, fmt.Errorf("want a list of at most %d NPCs, each with map, x and y", MaxHints)
	}
	var out []Hint
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("NPC %d: want a mapping with map, x and y", i+1)
		}
		for k := range m {
			if !slices.Contains([]string{"map", "x", "y", "sprite", "display_name"}, k) {
				return nil, fmt.Errorf("NPC %d: %q is not a field; a hint NPC has map, x, y, sprite and display_name", i+1, k)
			}
		}
		h := Hint{Sprite: "4_M_MANAGER", DisplayName: "Treasure Hunter"}
		mn, _ := m["map"].(string)
		if !mapName(mn) {
			return nil, fmt.Errorf("NPC %d: map is required: a map name, such as prontera", i+1)
		}
		h.Map = AssetOf(mn)
		x, okX := m["x"].(int)
		y, okY := m["y"].(int)
		if !okX || !okY || x < 0 || y < 0 || x > math.MaxInt16 || y > math.MaxInt16 {
			return nil, fmt.Errorf("NPC %d: x and y are required: a cell of the map", i+1)
		}
		h.X, h.Y = x, y
		if s, ok := m["sprite"]; ok {
			h.Sprite, _ = s.(string)
			if h.Sprite == "" {
				return nil, fmt.Errorf("NPC %d: sprite must be an NPC sprite name, such as 4_M_MANAGER", i+1)
			}
		}
		if s, ok := m["display_name"]; ok {
			h.DisplayName, _ = s.(string)
			if h.DisplayName == "" || len(h.DisplayName) > 24 {
				return nil, fmt.Errorf("NPC %d: display_name must be 1 to 24 characters", i+1)
			}
		}
		out = append(out, h)
	}
	return out, nil
}

func parseRewards(v any) (*Rewards, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("want a mapping such as { to: participants, zeny: 1000 }")
	}
	r := &Rewards{MinShare: DefaultMinShare}
	var errs []string
	for k := range m {
		if !slices.Contains([]string{"to", "items", "zeny", "exp", "currency", "min_share"}, k) {
			errs = append(errs, fmt.Sprintf("%q is not a field; rewards have to, items, zeny, exp, currency and min_share", k))
		}
	}
	r.To, _ = m["to"].(string)
	if r.To != ToKiller && r.To != ToParticipants && r.To != ToEveryone {
		errs = append(errs, "to must be killer, participants or everyone_on_map")
	}
	if v, ok := m["items"]; ok {
		list, ok := v.([]any)
		if !ok || len(list) == 0 || len(list) > MaxRewardItems {
			errs = append(errs, fmt.Sprintf("items must be a list of 1 to %d { item, amount }", MaxRewardItems))
		}
		for _, e := range list {
			im, ok := e.(map[string]any)
			name, _ := im["item"].(string)
			if !ok || name == "" {
				errs = append(errs, "items: each needs item, an item name such as Old_Blue_Box")
				continue
			}
			n := 1
			if a, ok := im["amount"]; ok {
				n, ok = a.(int)
				if !ok || n < 1 || n > 30000 {
					errs = append(errs, "items: amount must be 1 to 30000")
					continue
				}
			}
			for k := range im {
				if k != "item" && k != "amount" {
					errs = append(errs, fmt.Sprintf("items: %q is not a field; an item has item and amount", k))
				}
			}
			r.Items = append(r.Items, RewardItem{Item: name, Amount: n})
		}
	}
	if v, ok := m["zeny"]; ok {
		n, ok := v.(int)
		if !ok || n < 1 || n > math.MaxInt32 {
			errs = append(errs, "zeny must be 1 to 2147483647")
		}
		r.Zeny = int64(n)
	}
	if v, ok := m["exp"]; ok {
		em, ok := v.(map[string]any)
		if !ok {
			errs = append(errs, "exp must be a mapping such as { base: 1000, job: 500 }")
		}
		for k, x := range em {
			n, ok := x.(int)
			switch {
			case k != "base" && k != "job":
				errs = append(errs, fmt.Sprintf("exp: %q is not a field; exp has base and job", k))
			case !ok || n < 1 || n > math.MaxInt32:
				errs = append(errs, "exp: base and job must be 1 to 2147483647")
			case k == "base":
				r.BaseExp = int64(n)
			default:
				r.JobExp = int64(n)
			}
		}
	}
	if v, ok := m["currency"]; ok {
		cm, ok := v.(map[string]any)
		if !ok || len(cm) == 0 {
			errs = append(errs, "currency must be a mapping of currency names to amounts, such as { kafra: 5 }")
		}
		r.Currency = map[string]int64{}
		for k, x := range cm {
			n, ok := x.(int)
			if !ok || n < 1 || n > math.MaxInt32 {
				errs = append(errs, fmt.Sprintf("currency: %s must be 1 to 2147483647", k))
				continue
			}
			r.Currency[k] = int64(n)
		}
	}
	if v, ok := m["min_share"]; ok {
		var f float64
		switch x := v.(type) {
		case int:
			f = float64(x)
		case float64:
			f = x
		default:
			f = -1
		}
		if !(f > 0 && f <= 100) {
			errs = append(errs, "min_share must be a percent above 0 and at most 100, such as 1 or 0.5")
		}
		r.MinShare = f
	}
	if len(r.Items) == 0 && r.Zeny == 0 && r.BaseExp == 0 && r.JobExp == 0 && len(r.Currency) == 0 && len(errs) == 0 {
		errs = append(errs, "give at least one of items, zeny, exp or currency")
	}
	if len(errs) > 0 {
		slices.Sort(errs)
		return r, errors.New(strings.Join(errs, "; "))
	}
	return r, nil
}

// MapsOf returns the maps a boss or treasure hunt may use: its maps, or
// the fields among maps whose level is in its band. levels has the
// level of each field that runs, by asset name.
func MapsOf(band *Band, list []string, levels map[string]int) []string {
	if band == nil {
		return slices.Clone(list)
	}
	var out []string
	for m, l := range levels {
		if band.Contains(l) {
			out = append(out, m)
		}
	}
	slices.Sort(out)
	return out
}

// DisplayName is the name players see for an event: its name without
// the mod prefix, with spaces for underscores.
func (d Def) DisplayName() string {
	_, n, ok := strings.Cut(d.Name, ":")
	if !ok {
		n = d.Name
	}
	return strings.ReplaceAll(n, "_", " ")
}

// MapID is a map's ID from its asset name, such as prontera.
func MapID(asset string) string { return strings.TrimPrefix(asset, "map/") }

// AssetOf is a map name's asset name, as waserver's --map reads it: a
// plain name such as prontera is map/prontera, and a name with a slash,
// such as placeholder/map, stays as it is.
func AssetOf(name string) string {
	if strings.Contains(name, "/") {
		return name
	}
	return "map/" + name
}
