package quest

import (
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
)

// testNames resolves the names the tests use.
var testNames = Names{
	Monster: func(name string) (uint32, bool) {
		id, ok := map[string]uint32{"PORING": 1002, "LUNATIC": 1063, "FABRE": 1007}[name]
		return id, ok
	},
	Item: func(name string) (uint32, bool, bool) {
		switch name {
		case "Jellopy":
			return 909, true, true
		case "Knife":
			return 1201, false, true
		}
		return 0, false, false
	},
	Constant: func(module, name string) (int64, bool) {
		v, ok := map[string]map[string]int64{
			"races":    {"plant": 3, "insect": 4, "demi_human": 7, "player_human": 10},
			"sizes":    {"small": 0, "medium": 1, "large": 2},
			"elements": {"neutral": 0, "water": 1, "earth": 2, "fire": 3},
		}[module][name]
		return v, ok
	},
}

func build(t *testing.T, fields map[string]any) *Record {
	t.Helper()
	r, err := Build("example.q:hunt", "example.q", 1_000_000, fields, testNames)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestLimitEnd checks quest_time for each kind of limit.
func TestLimitEnd(t *testing.T) {
	loc := time.FixedZone("test", 8*3600)
	// Monday 5 January 2026.
	at := func(h, m int) time.Time { return time.Date(2026, 1, 5, h, m, 0, 0, loc) }
	cases := []struct {
		name  string
		limit any
		now   time.Time
		want  time.Time
	}{
		{"seconds", 3600, at(12, 0), at(13, 0)},
		{"duration", "2h30m", at(12, 0), at(14, 30)},
		{"days", "1d", at(12, 0), at(12, 0).AddDate(0, 0, 1)},
		{"at later today", map[string]any{"at": "14:00"}, at(12, 0), at(14, 0)},
		{"at passed today", map[string]any{"at": "04:00"}, at(12, 0), at(4, 0).AddDate(0, 0, 1)},
		{"at the very second", map[string]any{"at": "12:00"}, at(12, 0), at(12, 0).AddDate(0, 0, 1)},
		{"at with days", map[string]any{"at": "04:00", "days": 7}, at(3, 0), at(4, 0).AddDate(0, 0, 7)},
		{"at with days, passed", map[string]any{"at": "04:00", "days": 7}, at(5, 0), at(4, 0).AddDate(0, 0, 8)},
		{"weekday today, later", map[string]any{"at": "04:00", "weekday": "monday"}, at(3, 0), at(4, 0)},
		{"weekday today, passed", map[string]any{"at": "04:00", "weekday": "monday"}, at(5, 0), at(4, 0).AddDate(0, 0, 7)},
		{"weekday later this week", map[string]any{"at": "04:00", "weekday": "friday"}, at(12, 0), at(4, 0).AddDate(0, 0, 4)},
		{"weekday earlier in the week", map[string]any{"at": "04:00", "weekday": "sunday"}, at(12, 0), at(4, 0).AddDate(0, 0, 6)},
	}
	for _, c := range cases {
		l, err := ParseLimit(c.limit)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := l.End(c.now); !got.Equal(c.want) {
			t.Errorf("%s: ends %v, want %v", c.name, got, c.want)
		}
	}
	if !(Limit{}).None() || !(Limit{}).End(at(1, 0)).IsZero() {
		t.Error("the zero limit is not none")
	}
}

// TestParseLimitErrors checks the messages of bad limits.
func TestParseLimitErrors(t *testing.T) {
	for _, c := range []struct {
		v    any
		want string
	}{
		{"3x", "not a duration"},
		{"", "not a duration"},
		{0, "from 1 to"},
		{map[string]any{"at": "25:00"}, "time of day"},
		{map[string]any{"at": "4:00"}, "time of day"},
		{map[string]any{"at": "04:00", "weekday": "someday"}, "weekday is one of"},
		{map[string]any{"at": "04:00", "days": 1, "weekday": "monday"}, "do not go together"},
		{map[string]any{"at": "04:00", "hours": 1}, `"hours" is not a field`},
		{true, "want a duration"},
	} {
		_, err := ParseLimit(c.v)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want %q", c.v, err, c.want)
		}
	}
	for _, s := range []string{"1d", "3h", "2h30m", "45s", "6d17h"} {
		l, err := ParseLimit(s)
		if err != nil || DurationText(l.Seconds) != s {
			t.Errorf("%s: %v, written back as %q", s, err, DurationText(l.Seconds))
		}
	}
}

// TestBuild checks the fields of a record and their errors.
func TestBuild(t *testing.T) {
	r := build(t, map[string]any{
		"title":      "Hunt",
		"time_limit": "1h",
		"targets": []any{
			map[string]any{"monster": "PORING", "count": 5},
			map[string]any{"count": 3, "max_level": 20, "race": "plant", "size": "small", "element": "earth", "map": "prt_fild08", "map_name": "Prontera Fields"},
			map[string]any{"count": 2, "monsters": []any{"LUNATIC", "FABRE", "LUNATIC"}},
		},
		"drops": []any{map[string]any{"item": "Jellopy", "count": 2, "rate": 5000}, map[string]any{"monster": "PORING", "item": "Knife", "rate": 10000}},
	})
	if r.Title != "Hunt" || r.Limit.Seconds != 3600 || len(r.Targets) != 3 || len(r.Drops) != 2 {
		t.Fatalf("got %+v", r)
	}
	if tg := r.Targets[1]; tg.MinLevel != 1 || tg.MaxLevel != 20 || tg.Race != 3 || tg.Size != 0 || tg.Element != 2 || tg.Map != "prt_fild08" {
		t.Errorf("filters %+v", tg)
	}
	if tg := r.Targets[2]; len(tg.Monsters) != 2 || tg.Race != -1 {
		t.Errorf("monsters %+v", tg)
	}
	if d := r.Drops[1]; d.Monster != 1002 || d.Item != 1201 || d.Count != 1 {
		t.Errorf("drop %+v", d)
	}
	for _, c := range []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{}, "title is text"},
		{map[string]any{"title": "x", "reward": 1}, `"reward" is not a field`},
		{map[string]any{"title": "x", "targets": []any{map[string]any{"monster": "PORING"}}}, "count is the kills"},
		{map[string]any{"title": "x", "targets": []any{map[string]any{"monster": "NOPE", "count": 1}}}, `no monster "NOPE"`},
		{map[string]any{"title": "x", "targets": []any{map[string]any{"monster": "PORING", "count": 1, "race": "plant"}}}, "race filters"},
		{map[string]any{"title": "x", "targets": []any{map[string]any{"count": 1, "race": "player_human"}}}, "race is a name of wa.races"},
		{map[string]any{"title": "x", "targets": []any{map[string]any{"count": 1, "size": "huge"}}}, `"huge" is not a name of wa.sizes`},
		{map[string]any{"title": "x", "targets": []any{map[string]any{"count": 1, "min_level": 30, "max_level": 20}}}, "min_level is above max_level"},
		{map[string]any{"title": "x", "targets": []any{
			map[string]any{"monster": "PORING", "count": 1}, map[string]any{"monster": "PORING", "count": 2}}}, "already counts this monster"},
		{map[string]any{"title": "x", "targets": []any{
			map[string]any{"count": 1}, map[string]any{"count": 1}, map[string]any{"count": 1}, map[string]any{"count": 1}}}, "at most 3"},
		{map[string]any{"title": "x", "drops": []any{map[string]any{"item": "Knife", "count": 2, "rate": 1}}}, "does not stack"},
		{map[string]any{"title": "x", "drops": []any{map[string]any{"item": "Jellopy"}}}, "rate is the chance"},
		{map[string]any{"title": "x", "drops": []any{map[string]any{"item": "Jellopy", "rate": 10001}}}, "rate is a whole number"},
	} {
		_, err := Build("x", "core", 1, c.fields, testNames)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want %q", c.fields, err, c.want)
		}
	}
}

// TestTargetCounts checks each filter of quest_update_objective.
func TestTargetCounts(t *testing.T) {
	r := build(t, map[string]any{"title": "x", "targets": []any{
		map[string]any{"monster": "PORING", "count": 1},
		map[string]any{"count": 1, "min_level": 10, "max_level": 20, "race": "plant", "size": "small", "element": "earth", "map": "prt_fild08"},
		map[string]any{"count": 1, "monsters": []any{"LUNATIC"}},
	}})
	poring := Kill{Monster: 1002, Level: 1, Race: 3, Size: 1, Element: 0, Map: "anywhere"}
	if !r.Targets[0].Counts(poring) || r.Targets[1].Counts(poring) || r.Targets[2].Counts(poring) {
		t.Error("a Poring counts only for the Poring objective")
	}
	ok := Kill{Monster: 1118, Level: 15, Race: 3, Size: 0, Element: 2, Map: "prt_fild08"}
	if !r.Targets[1].Counts(ok) {
		t.Error("a kill that passes every filter does not count")
	}
	for name, change := range map[string]func(k *Kill){
		"level below":    func(k *Kill) { k.Level = 9 },
		"level above":    func(k *Kill) { k.Level = 21 },
		"race":           func(k *Kill) { k.Race = 4 },
		"size":           func(k *Kill) { k.Size = 2 },
		"element":        func(k *Kill) { k.Element = 3 },
		"the player map": func(k *Kill) { k.Map = "prt_fild07" },
	} {
		k := ok
		change(&k)
		if r.Targets[1].Counts(k) {
			t.Errorf("%s: the kill counts", name)
		}
	}
	if !r.Targets[2].Counts(Kill{Monster: 1063}) || r.Targets[2].Counts(Kill{Monster: 1007}) {
		t.Error("monsters lets the wrong kills through")
	}
}

// TestLog checks the log rules and quest_check.
func TestLog(t *testing.T) {
	now := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	hunt := build(t, map[string]any{"title": "Hunt", "time_limit": "1h", "targets": []any{map[string]any{"monster": "PORING", "count": 2}}})
	free := &Record{ID: 7, Name: "7", Title: "Talk"}
	tab := NewTable([]*Record{hunt, free})
	var log []character.Quest
	if !Add(&log, hunt, now) || Add(&log, hunt, now) {
		t.Fatal("add twice")
	}
	if log[0].State != character.QuestActive || !log[0].End.Equal(now.Add(time.Hour)) {
		t.Fatalf("added %+v", log[0])
	}
	if got := Check(log, tab, hunt.ID, CheckHunting, now); got != 0 {
		t.Errorf("hunting before kills: %d", got)
	}
	poring := Kill{Monster: 1002}
	never := func(int) int { return 9999 }
	for range 3 {
		Count(log, tab, poring, never)
	}
	if log[0].Counts[0] != 2 {
		t.Errorf("counts %v, want the count to stop at 2", log[0].Counts)
	}
	if got := Check(log, tab, hunt.ID, CheckHunting, now); got != 2 {
		t.Errorf("hunting after kills: %d", got)
	}
	if got := Check(log, tab, hunt.ID, CheckPlaytime, now); got != 0 {
		t.Errorf("playtime: %d", got)
	}
	if got := Check(log, tab, hunt.ID, CheckPlaytime, now.Add(2*time.Hour)); got != 2 || !TimedOut(&log[0], now.Add(2*time.Hour)) {
		t.Errorf("playtime after the limit: %d", got)
	}
	if !SetState(log, hunt.ID, character.QuestInactive) || Check(log, tab, hunt.ID, CheckHave, now) != 1 {
		t.Error("an inactive quest checks as 1")
	}
	if !SetState(log, hunt.ID, character.QuestComplete) || SetState(log, hunt.ID, character.QuestActive) {
		t.Error("a complete quest changed state")
	}
	if Check(log, tab, hunt.ID, CheckHave, now) != 2 || Check(log, tab, hunt.ID, CheckHunting, now) != 0 {
		t.Error("complete checks")
	}
	if p, _ := Count(log, tab, poring, never); len(p) != 0 {
		t.Error("a complete quest counted a kill")
	}
	// A quest without a time limit is timed out for rAthena's checks,
	// but not for TimedOut.
	if !Add(&log, free, now) || Check(log, tab, free.ID, CheckPlaytime, now) != 2 || TimedOut(&log[1], now) {
		t.Error("a quest without a limit")
	}
	if Change(&log, hunt.ID, free, now) {
		t.Error("changed into a quest the log has")
	}
	other := &Record{ID: 8, Name: "8", Title: "Next"}
	tab = NewTable([]*Record{hunt, free, other})
	if Change(&log, hunt.ID, other, now) {
		t.Error("changed a complete quest")
	}
	if !Change(&log, free.ID, other, now) || log[1].ID != 8 || Find(log, free.ID) >= 0 {
		t.Errorf("change: %+v", log)
	}
	if !Remove(&log, hunt.ID) || Remove(&log, hunt.ID) || len(log) != 1 {
		t.Errorf("remove: %+v", log)
	}
	if Check(log, tab, 999, CheckHave, now) != -1 {
		t.Error("a quest the log lacks")
	}
}

// TestKillDrops checks the drops a kill rolls, and that a missing record
// leaves its entry alone.
func TestKillDrops(t *testing.T) {
	r := build(t, map[string]any{"title": "x", "drops": []any{
		map[string]any{"item": "Jellopy", "count": 3, "rate": 2500},
		map[string]any{"monster": "LUNATIC", "item": "Knife", "rate": 10000},
	}})
	tab := NewTable([]*Record{r})
	log := []character.Quest{{ID: r.ID, State: character.QuestInactive}, {ID: 5, State: character.QuestActive}}
	_, gifts := Count(log, tab, Kill{Monster: 1002}, func(int) int { return 2499 })
	if len(gifts) != 1 || gifts[0].Item != 909 || gifts[0].Count != 3 {
		t.Errorf("gifts %+v", gifts)
	}
	_, gifts = Count(log, tab, Kill{Monster: 1002}, func(int) int { return 2500 })
	if len(gifts) != 0 {
		t.Errorf("a roll of 2500 against 2500 gave %+v", gifts)
	}
	_, gifts = Count(log, tab, Kill{Monster: 1063}, func(int) int { return 9999 })
	if len(gifts) != 1 || gifts[0].Item != 1201 {
		t.Errorf("a sure drop gave %+v", gifts)
	}
}
