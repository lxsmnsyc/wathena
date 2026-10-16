// Package quest holds the quests of data/quests.yml and the rules of a
// character's quest log (proposal 0002, review decision 45, decision
// 0043), as rAthena's quest.cpp:
//   - Core's records are converted from rAthena's quest_db.yml for the
//     mode (tools/convert). A core record's name is its quest ID, such as
//     7104. A mod's record is named with its ID, such as
//     example.hunts:wolves, and gets an ID from 1,000,000.
//   - A quest has a title, a time limit, up to 3 hunt objectives, and
//     item drops that kills give while the quest is in the log.
//   - A character's log holds each quest once, as active, inactive or
//     complete, with the end of its time limit and a kill count per
//     objective (log.go).
package quest

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/server/character"
)

// File is the data file of quests.
const File = "data/quests.yml"

// MaxTitle is the most characters of a title.
const MaxTitle = 255

// MaxCount is the most kills an objective asks for, and the most items
// a drop gives, as rAthena's uint16.
const MaxCount = 65535

// Record is one quest.
type Record struct {
	// ID is the quest's ID. Core's are rAthena's. Mod records get IDs
	// from 1,000,000.
	ID uint32
	// Name is the full name: core's ID as text, such as 7104, or
	// mod:name for a mod's, such as example.hunts:wolves.
	Name string
	// Mod is the mod that added the record.
	Mod   string
	Title string
	// Limit is the time limit. Its zero value is none.
	Limit Limit
	// Targets are the hunt objectives, at most
	// character.MaxQuestObjectives.
	Targets []Target
	// Drops are the items kills give while the quest is in the log.
	Drops []Drop
}

// Limit is a quest's time limit, as rAthena's TimeLimit. Its zero value
// is none. Seconds is a limit that ends that long after the quest
// starts. HasAt is a limit that ends at a time of day on the server's
// clock: the next time the clock shows At seconds after midnight, Days
// days later, or with HasWeekday the next Weekday at that time.
type Limit struct {
	Seconds    int64
	At         int
	HasAt      bool
	Days       int
	Weekday    time.Weekday
	HasWeekday bool
}

// None reports whether the quest has no time limit.
func (l Limit) None() bool { return l.Seconds == 0 && !l.HasAt }

// End is quest_time: when a quest that starts at now ends, or the zero
// time without a limit. A time of day follows now's location.
func (l Limit) End(now time.Time) time.Time {
	switch {
	case l.Seconds > 0:
		return now.Add(time.Duration(l.Seconds) * time.Second)
	case !l.HasAt:
		return time.Time{}
	}
	y, mo, d := now.Date()
	midnight := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
	today := int(now.Sub(midnight) / time.Second)
	shift := 0
	if today >= l.At {
		// Carry over to the next day.
		shift = 1
	}
	days := l.Days
	if l.HasWeekday {
		wd := int(now.Weekday())
		if int(l.Weekday) < wd+shift {
			shift = int(l.Weekday) + 7 - wd
		} else {
			shift = int(l.Weekday) - wd
		}
		days = 0
	}
	return midnight.AddDate(0, 0, shift+days).Add(time.Duration(l.At) * time.Second)
}

// Target is one hunt objective, as rAthena's s_quest_objective. A
// target with Monster counts that monster's kills on any map. A target
// without counts every kill its filters let through.
type Target struct {
	// Monster is a monster ID, or 0 for a target of filters.
	Monster uint32
	// Count is the kills the objective asks for.
	Count int
	// MinLevel and MaxLevel bound the monster's level, 0 for no bound.
	MinLevel, MaxLevel int
	// Race, Size and Element are the monster's, as rAthena's RC_, Size_
	// and ELE_ values, or -1 for any.
	Race, Size, Element int
	// Map is the map the player must be on, as scripts name it, such as
	// pay_d03_i or gen:field:2:3, or "" for any.
	Map string
	// MapName is the text the quest window shows for the map.
	MapName string
	// Monsters, when set, are the only monsters the target counts.
	Monsters []uint32
}

// Drop is an item that a kill gives while the quest is in the log, as
// rAthena's s_quest_dropitem.
type Drop struct {
	// Monster is the monster whose kills give it, or 0 for every
	// monster.
	Monster uint32
	Item    uint32
	Count   int
	// Rate is the chance out of 10,000.
	Rate int
}

// Kill is a monster's death as the quest rules read it.
type Kill struct {
	Monster             uint32
	Level               int
	Race, Size, Element int
	// Map is the killer's map, as scripts name it.
	Map string
}

// Counts reports whether a kill counts for the target, as
// quest_update_objective checks it.
func (t *Target) Counts(k Kill) bool {
	if t.Monster != 0 {
		return t.Monster == k.Monster
	}
	switch {
	case t.MinLevel != 0 && t.MinLevel > k.Level,
		t.MaxLevel != 0 && t.MaxLevel < k.Level,
		t.Race >= 0 && t.Race != k.Race,
		t.Size >= 0 && t.Size != k.Size,
		t.Element >= 0 && t.Element != k.Element,
		t.Map != "" && t.Map != k.Map,
		len(t.Monsters) > 0 && !slices.Contains(t.Monsters, k.Monster):
		return false
	}
	return true
}

// Names resolves the names a record's fields give.
type Names struct {
	// Monster finds a monster by its name, such as PORING or
	// example.mossy-fields:MOSS_PORING.
	Monster func(name string) (uint32, bool)
	// Item finds an item by its name, such as Jellopy, and reports
	// whether it stacks.
	Item func(name string) (id uint32, stackable, ok bool)
	// Constant finds a value of a constant module: races, sizes or
	// elements, by its readable name, such as demi_human.
	Constant func(module, name string) (int64, bool)
}

// Fields are the fields of a record.
var Fields = []string{"title", "time_limit", "targets", "drops"}

// targetFields are the fields of a target.
var targetFields = []string{"monster", "count", "min_level", "max_level", "race", "size", "element", "map", "map_name", "monsters"}

// dropFields are the fields of a drop.
var dropFields = []string{"monster", "item", "count", "rate"}

// Build makes a record from its fields. id is the record's ID.
func Build(name, mod string, id uint32, fields map[string]any, names Names) (*Record, error) {
	if err := only(fields, Fields); err != nil {
		return nil, err
	}
	r := &Record{ID: id, Name: name, Mod: mod}
	t, ok := fields["title"].(string)
	if !ok || t == "" || utf8.RuneCountInString(t) > MaxTitle {
		return nil, fmt.Errorf("title is text of 1 to %d characters", MaxTitle)
	}
	r.Title = t
	if v, ok := fields["time_limit"]; ok && v != nil {
		l, err := ParseLimit(v)
		if err != nil {
			return nil, fmt.Errorf("time_limit: %w", err)
		}
		r.Limit = l
	}
	if v, ok := fields["targets"]; ok && v != nil {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("targets is a list of hunt objectives")
		}
		if len(list) > character.MaxQuestObjectives {
			return nil, fmt.Errorf("targets has %d objectives; a quest has at most %d", len(list), character.MaxQuestObjectives)
		}
		for i, e := range list {
			t, err := buildTarget(e, names)
			if err != nil {
				return nil, fmt.Errorf("targets[%d]: %w", i, err)
			}
			for j, o := range r.Targets {
				if t.Monster != 0 && o.Monster == t.Monster {
					return nil, fmt.Errorf("targets[%d]: targets[%d] already counts this monster", i, j)
				}
			}
			r.Targets = append(r.Targets, t)
		}
	}
	if v, ok := fields["drops"]; ok && v != nil {
		list, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("drops is a list of item drops")
		}
		for i, e := range list {
			d, err := buildDrop(e, names)
			if err != nil {
				return nil, fmt.Errorf("drops[%d]: %w", i, err)
			}
			r.Drops = append(r.Drops, d)
		}
	}
	return r, nil
}

// only checks that a mapping has only the given keys, and the extra
// ones.
func only(m map[string]any, keys []string, extra ...string) error {
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if !slices.Contains(keys, k) && !slices.Contains(extra, k) {
			return fmt.Errorf("%q is not a field; the fields are %s", k, strings.Join(keys, ", "))
		}
	}
	return nil
}

// number reads a whole number from lo to hi.
func number(m map[string]any, key string, lo, hi int) (int, bool, error) {
	v, ok := m[key]
	if !ok {
		return 0, false, nil
	}
	n, isInt := v.(int)
	if !isInt || n < lo || n > hi {
		return 0, true, fmt.Errorf("%s is a whole number from %d to %d", key, lo, hi)
	}
	return n, true, nil
}

func buildTarget(e any, names Names) (Target, error) {
	t := Target{Race: -1, Size: -1, Element: -1}
	m, ok := e.(map[string]any)
	if !ok {
		return t, fmt.Errorf("an objective is a mapping, such as {monster: PORING, count: 10}")
	}
	if err := only(m, targetFields); err != nil {
		return t, err
	}
	n, ok, err := number(m, "count", 0, MaxCount)
	if err != nil {
		return t, err
	}
	if !ok {
		return t, fmt.Errorf("count is the kills the objective asks for")
	}
	t.Count = n
	if v, ok := m["monster"]; ok {
		id, err := monster(v, names)
		if err != nil {
			return t, err
		}
		for _, k := range targetFields {
			if _, set := m[k]; set && k != "monster" && k != "count" {
				return t, fmt.Errorf("%s filters the monsters of an objective without monster; an objective with monster counts that monster's kills", k)
			}
		}
		t.Monster = id
		return t, nil
	}
	if t.MinLevel, _, err = number(m, "min_level", 0, 1000); err != nil {
		return t, err
	}
	if t.MaxLevel, _, err = number(m, "max_level", 0, 1000); err != nil {
		return t, err
	}
	if t.MaxLevel > 0 && t.MinLevel > t.MaxLevel {
		return t, fmt.Errorf("min_level is above max_level")
	}
	if t.MinLevel == 0 && t.MaxLevel > 0 {
		// As rAthena: a max_level alone makes min_level 1.
		t.MinLevel = 1
	}
	for _, c := range []struct {
		key, module string
		to          *int
		refused     []string
	}{
		{"race", "races", &t.Race, []string{"player_human", "player_doram"}},
		{"size", "sizes", &t.Size, nil},
		{"element", "elements", &t.Element, nil},
	} {
		v, ok := m[c.key]
		if !ok {
			continue
		}
		s, isText := v.(string)
		if !isText || slices.Contains(c.refused, s) {
			return t, fmt.Errorf("%s is a name of wa.%s, such as %s", c.key, c.module, map[string]string{"races": "demi_human", "sizes": "small", "elements": "fire"}[c.module])
		}
		if s == "all" {
			continue
		}
		val, found := names.Constant(c.module, s)
		if !found {
			return t, fmt.Errorf("%s: %q is not a name of wa.%s", c.key, s, c.module)
		}
		*c.to = int(val)
	}
	if v, ok := m["map"]; ok {
		s, isText := v.(string)
		if !isText || s == "" || len(s) > 64 || strings.ContainsAny(s, " \t\n") {
			return t, fmt.Errorf("map is a map's name, such as pay_d03_i or gen:field:2:3")
		}
		t.Map = strings.TrimPrefix(s, "map/")
	}
	if v, ok := m["map_name"]; ok {
		s, isText := v.(string)
		if !isText || utf8.RuneCountInString(s) > 64 {
			return t, fmt.Errorf("map_name is text of at most 64 characters")
		}
		t.MapName = s
	}
	if v, ok := m["monsters"]; ok {
		list, isList := v.([]any)
		if !isList || len(list) == 0 {
			return t, fmt.Errorf("monsters is a list of monster names")
		}
		for _, e := range list {
			id, err := monster(e, names)
			if err != nil {
				return t, fmt.Errorf("monsters: %w", err)
			}
			if !slices.Contains(t.Monsters, id) {
				t.Monsters = append(t.Monsters, id)
			}
		}
	}
	return t, nil
}

// monster reads a monster's name.
func monster(v any, names Names) (uint32, error) {
	s, ok := v.(string)
	if !ok || s == "" {
		return 0, fmt.Errorf("monster is a monster's name, such as PORING")
	}
	id, found := names.Monster(s)
	if !found {
		return 0, fmt.Errorf("there is no monster %q", s)
	}
	return id, nil
}

func buildDrop(e any, names Names) (Drop, error) {
	var d Drop
	m, ok := e.(map[string]any)
	if !ok {
		return d, fmt.Errorf("a drop is a mapping, such as {item: Jellopy, rate: 5000}")
	}
	if err := only(m, dropFields); err != nil {
		return d, err
	}
	if v, ok := m["monster"]; ok {
		id, err := monster(v, names)
		if err != nil {
			return d, err
		}
		d.Monster = id
	}
	s, ok := m["item"].(string)
	if !ok || s == "" {
		return d, fmt.Errorf("item is an item's name, such as Jellopy")
	}
	id, stackable, found := names.Item(s)
	if !found {
		return d, fmt.Errorf("there is no item %q", s)
	}
	d.Item = id
	n, ok, err := number(m, "count", 1, MaxCount)
	if err != nil {
		return d, err
	}
	d.Count = 1
	if ok {
		if n > 1 && !stackable {
			return d, fmt.Errorf("count: %s does not stack, so a drop gives 1", s)
		}
		d.Count = n
	}
	rate, ok, err := number(m, "rate", 1, 10000)
	if err != nil {
		return d, err
	}
	if !ok {
		return d, fmt.Errorf("rate is the chance out of 10,000, from 1 to 10000")
	}
	d.Rate = rate
	return d, nil
}

// Weekdays are the names of a limit's weekday, from Sunday.
var Weekdays = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// ParseLimit reads a time_limit: a duration such as "3h", "2h30m" or
// "1d", or a whole number of seconds, for a limit that ends that long
// after the quest starts, or a mapping { at, days, weekday } for one that
// ends at a time of day, such as { at: "04:00", days: 7 }.
func ParseLimit(v any) (Limit, error) {
	var l Limit
	switch v := v.(type) {
	case int:
		if v < 1 || v > 366*86400 {
			return l, fmt.Errorf("a number of seconds is from 1 to %d", 366*86400)
		}
		l.Seconds = int64(v)
		return l, nil
	case string:
		n, err := parseDuration(v)
		if err != nil {
			return l, err
		}
		l.Seconds = n
		return l, nil
	case map[string]any:
		if err := only(v, []string{"at", "days", "weekday"}); err != nil {
			return l, err
		}
		at, ok := v["at"].(string)
		if !ok {
			return l, fmt.Errorf("at is a time of day, such as \"04:00\"")
		}
		secs, err := parseClock(at)
		if err != nil {
			return l, err
		}
		l.At, l.HasAt = secs, true
		days, set, err := number(v, "days", 0, 366)
		if err != nil {
			return l, err
		}
		l.Days = days
		if w, ok := v["weekday"]; ok {
			if set {
				return l, fmt.Errorf("days and weekday do not go together")
			}
			s, _ := w.(string)
			i := slices.Index(Weekdays, strings.ToLower(s))
			if i < 0 {
				return l, fmt.Errorf("weekday is one of %s", strings.Join(Weekdays, ", "))
			}
			l.Weekday, l.HasWeekday = time.Weekday(i), true
		}
		return l, nil
	}
	return l, fmt.Errorf("want a duration such as 3h, a number of seconds, or { at: \"04:00\" }")
}

// parseDuration reads "1d", "3h", "2h30m" or "45s".
func parseDuration(s string) (int64, error) {
	bad := fmt.Errorf("%q is not a duration: whole numbers with d, h, m or s, such as 3h or 2h30m", s)
	if s == "" {
		return 0, bad
	}
	var total int64
	units := map[byte]int64{'d': 86400, 'h': 3600, 'm': 60, 's': 1}
	for len(s) > 0 {
		i := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 || i == len(s) || i > 6 {
			return 0, bad
		}
		n, _ := strconv.ParseInt(s[:i], 10, 64)
		u, ok := units[s[i]]
		if !ok {
			return 0, bad
		}
		total += n * u
		s = strings.TrimLeft(s[i+1:], " ")
	}
	if total < 1 || total > 366*86400 {
		return 0, fmt.Errorf("a duration is from 1 second to 366 days")
	}
	return total, nil
}

// parseClock reads "HH:MM" or "HH:MM:SS" as seconds after midnight.
func parseClock(s string) (int, error) {
	parts := strings.Split(s, ":")
	bad := fmt.Errorf("at is a time of day, such as \"04:00\" or \"04:00:30\", not %q", s)
	if len(parts) < 2 || len(parts) > 3 {
		return 0, bad
	}
	limits := []int{23, 59, 59}
	secs := 0
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || len(p) != 2 || n < 0 || n > limits[i] {
			return 0, bad
		}
		secs += n * []int{3600, 60, 1}[i]
	}
	return secs, nil
}

// LimitText writes a limit as a record writes it, or "" for none.
func LimitText(l Limit) any {
	switch {
	case l.Seconds > 0:
		return DurationText(l.Seconds)
	case l.HasAt:
		m := map[string]any{"at": fmt.Sprintf("%02d:%02d", l.At/3600, l.At/60%60)}
		if l.At%60 != 0 {
			m["at"] = fmt.Sprintf("%02d:%02d:%02d", l.At/3600, l.At/60%60, l.At%60)
		}
		if l.Days > 0 {
			m["days"] = l.Days
		}
		if l.HasWeekday {
			m["weekday"] = Weekdays[l.Weekday]
		}
		return m
	}
	return ""
}

// DurationText writes seconds as a duration, such as 2h30m.
func DurationText(secs int64) string {
	var b strings.Builder
	for _, u := range []struct {
		n    int64
		unit string
	}{{86400, "d"}, {3600, "h"}, {60, "m"}, {1, "s"}} {
		if secs >= u.n {
			fmt.Fprintf(&b, "%d%s", secs/u.n, u.unit)
			secs %= u.n
		}
	}
	return b.String()
}

// Table holds the quests by ID and by name.
type Table struct {
	list   []*Record
	byID   map[uint32]*Record
	byName map[string]*Record
}

// NewTable makes a table of records.
func NewTable(list []*Record) *Table {
	t := &Table{byID: map[uint32]*Record{}, byName: map[string]*Record{}}
	for _, r := range list {
		t.list = append(t.list, r)
		t.byID[r.ID] = r
		t.byName[r.Name] = r
	}
	slices.SortFunc(t.list, func(a, b *Record) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		}
		return 0
	})
	return t
}

// All are the records, by ID.
func (t *Table) All() []*Record {
	if t == nil {
		return nil
	}
	return t.list
}

// ByID finds a record, or nil.
func (t *Table) ByID(id uint32) *Record {
	if t == nil {
		return nil
	}
	return t.byID[id]
}

// Resolve finds a record by a name a mod gave: a full name, or a plain
// name, which is the mod's own record first, then core's.
func (t *Table) Resolve(name, mod string) *Record {
	if t == nil {
		return nil
	}
	if strings.Contains(name, ":") {
		return t.byName[name]
	}
	if mod != "" && mod != "core" {
		if r := t.byName[mod+":"+name]; r != nil {
			return r
		}
	}
	return t.byName[name]
}
