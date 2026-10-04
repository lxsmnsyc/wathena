package worldevent

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
)

func record(name string, fields map[string]any) *mod.Record {
	f := map[string]any{"trigger": map[string]any{"daily": "20:00"}, "duration_minutes": 60}
	for k, v := range fields {
		f[k] = v
	}
	return &mod.Record{Name: name, Mod: "example.a", Fields: f, SetBy: map[string]string{"script": "example.a", "rewards": "example.a"}}
}

func TestParseTyped(t *testing.T) {
	d, err := Parse(record("example.a:Boss", map[string]any{
		"type": "boss", "monsters": []any{"ORK_HERO", "ORC_LORD"}, "level_band": []any{20, 40},
		"rewards": map[string]any{"to": "participants", "zeny": 1000, "exp": map[string]any{"base": 10, "job": 5},
			"items": []any{map[string]any{"item": "Apple", "amount": 3}}, "currency": map[string]any{"kafra": 2}, "min_share": 0.5},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if d.Type != TypeBoss || len(d.Boss.Monsters) != 2 || *d.Boss.Band != (Band{20, 40}) || !d.Boss.Announce || d.Script != "" {
		t.Fatalf("boss %+v %+v", d, d.Boss)
	}
	r := d.Rewards
	if r.To != ToParticipants || r.Zeny != 1000 || r.BaseExp != 10 || r.JobExp != 5 || r.Items[0] != (RewardItem{Item: "Apple", Amount: 3}) ||
		r.Currency["kafra"] != 2 || r.MinShare != 0.5 {
		t.Fatalf("rewards %+v", r)
	}

	d, err = Parse(record("example.a:Raid", map[string]any{
		"type": "invasion", "town": "prontera", "script": "scripts/raid.lua",
		"waves": []any{
			map[string]any{"monsters": []any{map[string]any{"monster": "PORING", "amount": 10}}, "start": "cleared"},
			map[string]any{"monsters": []any{map[string]any{"monster": "LUNATIC"}}, "start": "2m"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	w := d.Invasion.Waves
	if d.Invasion.Town != "map/prontera" || len(w) != 2 || !w[0].Cleared || w[0].Monsters[0].Amount != 10 || w[1].After != 2*time.Minute ||
		w[1].Monsters[0].Amount != 1 || d.Script != "example.a:scripts/raid.lua" || d.Rewards != nil {
		t.Fatalf("invasion %+v", d.Invasion)
	}

	d, err = Parse(record("example.a:Hunt", map[string]any{
		"type": "treasure_hunt", "chests": 4, "maps": []any{"prt_fild08", "prt_fild08"}, "chest": "TREASURE_BOX1",
		"hint_npcs": []any{map[string]any{"map": "prontera", "x": 150, "y": 180}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if tr := d.Treasure; tr.Chests != 4 || !slices.Equal(tr.Maps, []string{"map/prt_fild08"}) || tr.Hints[0].DisplayName != "Treasure Hunter" ||
		tr.Hints[0].Map != "map/prontera" {
		t.Fatalf("treasure %+v", tr)
	}
}

func TestParseTypedErrors(t *testing.T) {
	for _, c := range []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"type": "parade"}, "type must be boss, invasion or treasure_hunt"},
		{map[string]any{"monsters": []any{"PORING"}}, `"monsters" is a field of typed events; set type, such as type: boss`},
		{map[string]any{"type": "boss", "monsters": []any{"PORING"}}, "level_band or maps is required"},
		{map[string]any{"type": "boss", "monsters": []any{"PORING"}, "maps": []any{"a"}, "level_band": []any{1, 2}}, "cannot both be set"},
		{map[string]any{"type": "boss", "maps": []any{"a"}}, "monsters: want a list of monster names"},
		{map[string]any{"type": "boss", "monsters": []any{"PORING"}, "level_band": []any{40, 20}}, "level_band must be [lowest, highest]"},
		{map[string]any{"type": "boss", "monsters": []any{"PORING"}, "maps": []any{"a"}, "town": "prontera"}, `"town" is not a field; events of type boss have`},
		{map[string]any{"type": "invasion", "waves": []any{}}, "town is required"},
		{map[string]any{"type": "invasion", "town": "p", "waves": []any{map[string]any{"monsters": []any{map[string]any{"monster": "P"}}}}}, "start is required"},
		{map[string]any{"type": "invasion", "town": "p", "waves": []any{map[string]any{"monsters": []any{map[string]any{"monster": "P"}}, "start": "soon"}}}, `"soon" is not a time`},
		{map[string]any{"type": "invasion", "town": "p", "waves": []any{map[string]any{"monsters": []any{map[string]any{"monster": "P", "amount": 501}}, "start": "cleared"}}}, "the most is 500"},
		{map[string]any{"type": "treasure_hunt", "chests": 0, "maps": []any{"a"}, "chest": "BOX"}, "chests must be 1 to 200"},
		{map[string]any{"type": "treasure_hunt", "chests": 1, "maps": []any{"a"}}, "chest is required"},
		{map[string]any{"type": "treasure_hunt", "chests": 1, "maps": []any{"a"}, "chest": "B", "hint_npcs": []any{map[string]any{"map": "a"}}}, "x and y are required"},
		{map[string]any{"type": "boss", "monsters": []any{"P"}, "maps": []any{"a"}, "rewards": map[string]any{"to": "everyone"}}, "to must be killer, participants or everyone_on_map"},
		{map[string]any{"type": "boss", "monsters": []any{"P"}, "maps": []any{"a"}, "rewards": map[string]any{"to": "killer"}}, "give at least one of items, zeny, exp or currency"},
		{map[string]any{"type": "boss", "monsters": []any{"P"}, "maps": []any{"a"}, "rewards": map[string]any{"to": "killer", "zeny": 1, "min_share": 0}}, "min_share must be a percent"},
		{map[string]any{"script": "scripts/a.lua", "rewards": map[string]any{"to": "killer", "zeny": 1}}, "rewards are given by typed events"},
		{map[string]any{}, "script is required"},
	} {
		_, err := Parse(record("example.a:E", c.fields))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want %q", c.fields, err, c.want)
		}
	}
}

func TestApplyCoreEvents(t *testing.T) {
	core := Def{Name: "core:Field_Boss", Mod: mod.Core, Duration: time.Hour}
	own := Def{Name: "example.a:Night", Mod: "example.a", Duration: time.Hour}
	defs, err := Apply([]Def{core, own}, nil)
	if err != nil || len(defs) != 1 || defs[0].Name != own.Name {
		t.Fatalf("core's events are off by default: %+v %v", defs, err)
	}
	defs, err = Apply([]Def{core, own}, map[string]Override{core.Name: {Enabled: true, DurationMinutes: 5}})
	if err != nil || len(defs) != 2 || defs[0].Duration != 5*time.Minute {
		t.Fatalf("enabled: %+v %v", defs, err)
	}
	if _, err := Apply([]Def{core}, map[string]Override{core.Name: {Enabled: true, Disabled: true}}); err == nil {
		t.Fatal("enabled and disabled passed")
	}
}

func TestDirection(t *testing.T) {
	for _, c := range []struct {
		dx, dy int
		want   string
	}{{0, 5, "north"}, {5, 5, "northeast"}, {5, 0, "east"}, {3, -9, "south"}, {-7, -7, "southwest"}, {-9, 1, "west"}, {0, 0, "spot"}} {
		if got := Direction(c.dx, c.dy); got != c.want {
			t.Errorf("%d,%d: %s, want %s", c.dx, c.dy, got, c.want)
		}
	}
}

// fakeField records what Runs asks of the maps.
type fakeField struct {
	maps    []string
	log     []string
	alive   map[int64]int
	notices []Notice
	nextX   int
}

func (f *fakeField) Has(m string) bool { return slices.Contains(f.maps, m) }
func (f *fakeField) StartMap() string  { return f.maps[0] }
func (f *fakeField) StartScript(_ context.Context, m string, s ScriptRun) error {
	f.log = append(f.log, "script "+MapID(m)+" "+s.Entry)
	return nil
}

func (f *fakeField) EndScript(_ context.Context, m string, _ int64) error {
	f.log = append(f.log, "end script "+MapID(m))
	return nil
}

func (f *fakeField) Condition(context.Context, string, string, string, string) (bool, error) {
	return false, nil
}

func (f *fakeField) Spawn(_ context.Context, s Spawn) ([]Placed, error) {
	f.log = append(f.log, fmt.Sprintf("spawn %s %d x%d %s %s", MapID(s.Map), s.Monster, s.Amount, s.Role, s.Unit))
	if f.alive == nil {
		f.alive = map[int64]int{}
	}
	f.alive[s.ID] += s.Amount
	var out []Placed
	for range s.Amount {
		f.nextX += 10
		out = append(out, Placed{X: f.nextX, Y: 100, Name: fmt.Sprintf("M%d", s.Monster)})
	}
	return out, nil
}

func (f *fakeField) Alive(_ context.Context, _ string, run int64) (int, error) { return f.alive[run], nil }

func (f *fakeField) Hints(_ context.Context, m string, _ int64, hints []Hint) error {
	f.log = append(f.log, fmt.Sprintf("hints %s %d", MapID(m), len(hints)))
	return nil
}

func (f *fakeField) Reward(_ context.Context, r RewardRun) error {
	f.log = append(f.log, "reward "+MapID(r.Map)+" "+r.Unit)
	return nil
}

func (f *fakeField) Clear(_ context.Context, m string, run int64) error {
	f.log = append(f.log, "clear "+MapID(m))
	f.alive[run] = 0
	return nil
}

func (f *fakeField) Announce(text string) { f.log = append(f.log, "announce "+text) }
func (f *fakeField) Notice(n Notice)      { f.notices = append(f.notices, n) }

func (f *fakeField) take() string {
	s := strings.Join(f.log, "|")
	f.log = nil
	return s
}

func typedRuns(t *testing.T, defs []Def, field *fakeField, store *MemoryStore, now time.Time) (*Scheduler, *Runs) {
	t.Helper()
	runs := NewRuns(field, RunsOptions{Levels: map[string]int{"map/prt_fild08": 5, "map/gef_fild10": 60}, Rand: fixedRand(0), Progress: store})
	s := New(defs, Options{Runner: runs, Store: store, Rand: fixedRand(0)})
	runs.SetEvents(s.Defs())
	if err := s.Load(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	return s, runs
}

var bossEvent = Def{Name: "example.a:Boss", Mod: "example.a", Type: TypeBoss, Duration: time.Hour,
	Trigger: Trigger{Kind: Clock, Spec: bossTime},
	Boss:    &Boss{MonsterIDs: []uint32{1087}, Band: &Band{1, 10}, Announce: true},
	Rewards: &Rewards{To: ToParticipants, Zeny: 10, MinShare: 1}}

var bossTime = boss.Trigger.Spec

func TestBossRun(t *testing.T) {
	ctx := context.Background()
	field := &fakeField{maps: []string{"map/prontera", "map/prt_fild08", "map/gef_fild10"}}
	store := &MemoryStore{}
	s, runs := typedRuns(t, []Def{bossEvent}, field, store, at(19, 59))
	s.Check(ctx, at(20, 0))
	// The band [1, 10] picks prt_fild08, of level 5.
	if got := field.take(); got != "spawn prt_fild08 1087 x1 boss boss|announce M1087 has appeared on prt_fild08!" {
		t.Fatalf("start: %q", got)
	}
	if n := runs.Running(); len(n) != 1 || n[0].Map != "map/prt_fild08" || !n[0].Ends.Equal(at(21, 0)) {
		t.Fatalf("running %+v", n)
	}
	runs.Note(Note{Kind: NoteDied, Run: 1, Map: "map/prt_fild08", Unit: UnitBoss, Monster: "M1087", Killer: "Alice", MVP: "Bob"})
	s.Check(ctx, at(20, 1))
	if got := field.take(); got != "announce Alice defeated M1087 on prt_fild08! MVP: Bob.|clear prt_fild08" {
		t.Fatalf("death: %q", got)
	}
	if n := field.notices; len(n) != 2 || !n[0].Started || n[1].Result != ResultWon {
		t.Fatalf("notices %+v", n)
	}
	// The run ended, so a restart in its window does not start it again.
	field.notices = nil
	s, _ = typedRuns(t, []Def{bossEvent}, field, store, at(20, 2))
	s.Check(ctx, at(20, 2))
	if got := field.take(); got != "" {
		t.Fatalf("after a restart: %q", got)
	}
}

func TestBossResumesAfterRestart(t *testing.T) {
	ctx := context.Background()
	field := &fakeField{maps: []string{"map/prt_fild08"}}
	store := &MemoryStore{}
	s, _ := typedRuns(t, []Def{bossEvent}, field, store, at(19, 59))
	s.Check(ctx, at(20, 0))
	field.take()
	// The server stops at 20:10 with the boss alive, and starts at
	// 20:20: the boss comes back until 21:00.
	s, runs := typedRuns(t, []Def{bossEvent}, field, store, at(20, 20))
	s.Check(ctx, at(20, 20))
	if got := field.take(); got != "spawn prt_fild08 1087 x1 boss boss|announce M1087 has appeared on prt_fild08!" {
		t.Fatalf("resume: %q", got)
	}
	if n := runs.Running(); len(n) != 1 || !n[0].Ends.Equal(at(21, 0)) {
		t.Fatalf("running %+v", n)
	}
	// A boss that died before the stop, whose reward was saved, does not
	// come back.
	if err := store.RecordEventProgress(ctx, bossEvent.Name, at(20, 0), UnitBoss); err != nil {
		t.Fatal(err)
	}
	s, _ = typedRuns(t, []Def{bossEvent}, field, store, at(20, 30))
	s.Check(ctx, at(20, 30))
	if got := field.take(); got != "" {
		t.Fatalf("after the boss died: %q", got)
	}
	s.Check(ctx, at(21, 0))
	if got := field.take(); got != "" {
		t.Fatalf("at the end: %q", got)
	}
}

var raid = Def{Name: "example.a:Raid", Mod: "example.a", Type: TypeInvasion, Duration: 30 * time.Minute,
	Trigger: Trigger{Kind: Clock, Spec: bossTime},
	Invasion: &Invasion{Town: "map/prontera", Waves: []Wave{
		{Cleared: true, Monsters: []WaveMonster{{ID: 1002, Amount: 3}}},
		{After: 2 * time.Minute, Monsters: []WaveMonster{{ID: 1063, Amount: 2}}},
		{Cleared: true, Monsters: []WaveMonster{{ID: 1096, Amount: 1}}},
	}},
	Rewards: &Rewards{To: ToEveryone, Zeny: 10}}

func TestInvasionWaves(t *testing.T) {
	ctx := context.Background()
	field := &fakeField{maps: []string{"map/prontera"}}
	store := &MemoryStore{}
	s, _ := typedRuns(t, []Def{raid}, field, store, at(19, 59))
	s.Check(ctx, at(20, 0))
	if got := field.take(); got != "announce prontera is under attack! Defend the town!|spawn prontera 1002 x3 invader |announce Wave 1 of 3 attacks prontera!" {
		t.Fatalf("start: %q", got)
	}
	// Wave 2 comes two minutes after wave 1 started, cleared or not.
	s.Check(ctx, at(20, 1))
	if got := field.take(); got != "" {
		t.Fatalf("before wave 2: %q", got)
	}
	s.Check(ctx, at(20, 2))
	if got := field.take(); got != "spawn prontera 1063 x2 invader |announce Wave 2 of 3 attacks prontera!" {
		t.Fatalf("wave 2: %q", got)
	}
	// Wave 3 waits until the others died.
	field.alive[1] = 1
	s.Check(ctx, at(20, 3))
	if got := field.take(); got != "" {
		t.Fatalf("before wave 3: %q", got)
	}
	// A restart now goes on at wave 2, which comes again.
	s2, _ := typedRuns(t, []Def{raid}, &fakeField{maps: []string{"map/prontera"}}, store, at(20, 4))
	f2 := s2.opts.Runner.(*Runs).field.(*fakeField)
	s2.Check(ctx, at(20, 4))
	if got := f2.take(); got != "announce prontera is under attack! Defend the town!|spawn prontera 1063 x2 invader |announce Wave 2 of 3 attacks prontera!" {
		t.Fatalf("resume: %q", got)
	}
	field.alive[1] = 0
	s.Check(ctx, at(20, 4))
	if got := field.take(); got != "spawn prontera 1096 x1 invader |announce Wave 3 of 3 attacks prontera!" {
		t.Fatalf("wave 3: %q", got)
	}
	field.alive[1] = 0
	s.Check(ctx, at(20, 5))
	if got := field.take(); got != "reward prontera result|announce prontera drove back the invasion!|clear prontera" {
		t.Fatalf("won: %q", got)
	}
}

func TestInvasionLost(t *testing.T) {
	ctx := context.Background()
	field := &fakeField{maps: []string{"map/prontera"}}
	s, _ := typedRuns(t, []Def{raid}, field, &MemoryStore{}, at(19, 59))
	s.Check(ctx, at(20, 0))
	field.take()
	s.Check(ctx, at(20, 30))
	if got := field.take(); !strings.HasSuffix(got, "announce The invasion of prontera was not stopped in time.|clear prontera") {
		t.Fatalf("lost: %q", got)
	}
	if n := field.notices; n[len(n)-1].Result != ResultLost {
		t.Fatalf("notices %+v", n)
	}
}

var hunt = Def{Name: "example.a:Hunt", Mod: "example.a", Type: TypeTreasure, Duration: time.Hour,
	Trigger: Trigger{Kind: Clock, Spec: bossTime},
	Treasure: &Treasure{Chests: 2, Maps: []string{"map/prt_fild08"}, ChestID: 1324,
		Hints: []Hint{{Map: "map/prontera", X: 150, Y: 150, DisplayName: "Guide"}, {Map: "map/payon", X: 1, Y: 1, DisplayName: "Guide"}}}}

func TestTreasureHuntHints(t *testing.T) {
	ctx := context.Background()
	field := &fakeField{maps: []string{"map/prontera", "map/prt_fild08"}}
	s, runs := typedRuns(t, []Def{hunt}, field, &MemoryStore{}, at(19, 59))
	s.Check(ctx, at(20, 0))
	if got := field.take(); got != "spawn prt_fild08 1324 x1 chest chest:1|spawn prt_fild08 1324 x1 chest chest:2|hints prontera 1|"+
		"announce 2 treasure chests are hidden on the fields! Ask the Guide in prontera for hints." {
		t.Fatalf("start: %q", got)
	}
	// The chests are at 10,100 and 20,100 of prt_fild08.
	if got := strings.Join(runs.Hint(1, "map/prontera", 150, 150), "|"); got != "2 chests are still hidden.|One lies on prt_fild08, near 10,100.|One lies on prt_fild08, near 20,100." {
		t.Fatalf("hint from prontera: %q", got)
	}
	if got := strings.Join(runs.Hint(1, "map/prt_fild08", 20, 90), "|"); got != "2 chests are still hidden.|One lies to the northwest of here, 10 cells away.|One lies to the north of here, 10 cells away." {
		t.Fatalf("hint on the field: %q", got)
	}
	runs.Note(Note{Kind: NoteDied, Run: 1, Map: "map/prt_fild08", Unit: "chest:2", Killer: "Alice"})
	s.Check(ctx, at(20, 1))
	if got := field.take(); got != "announce Alice found a treasure chest on prt_fild08! 1 chest is left." {
		t.Fatalf("first chest: %q", got)
	}
	if got := strings.Join(runs.Hint(1, "map/prt_fild08", 20, 90), "|"); got != "1 chest is still hidden.|One lies to the northwest of here, 10 cells away." {
		t.Fatalf("hint after: %q", got)
	}
	runs.Note(Note{Kind: NoteDied, Run: 1, Map: "map/prt_fild08", Unit: "chest:1", Killer: "Bob"})
	s.Check(ctx, at(20, 2))
	if got := field.take(); got != "announce Bob found the last treasure chest on prt_fild08!|clear prt_fild08|clear prontera" {
		t.Fatalf("last chest: %q", got)
	}
}

func TestRequestsAndFinish(t *testing.T) {
	ctx := context.Background()
	field := &fakeField{maps: []string{"map/prontera", "map/prt_fild08"}}
	script := Def{Name: "example.a:Party", Mod: "example.a", Duration: 10 * time.Minute, Script: "example.a:scripts/party.lua",
		Trigger: Trigger{Kind: Clock, Spec: bossTime}}
	s, runs := typedRuns(t, []Def{script, raid}, field, &MemoryStore{}, at(9, 0))
	if ok, _ := runs.Request("Missing", "example.a", true); ok {
		t.Fatal("a missing event started")
	}
	if ok, _ := runs.Request("Party", "example.a", false); ok {
		t.Fatal("an event that does not run stopped")
	}
	if ok, _ := runs.Request("Party", "example.a", true); !ok {
		t.Fatal("start refused")
	}
	if ok, _ := runs.Request("example.a:Party", "other", true); ok {
		t.Fatal("a second start of the same event passed")
	}
	s.Check(ctx, at(9, 0))
	if got := field.take(); got != "script prontera example.a:scripts/party.lua" {
		t.Fatalf("start: %q", got)
	}
	if ok, _ := runs.Request("example.a:Party", "other", false); !ok {
		t.Fatal("stop refused")
	}
	s.Check(ctx, at(9, 1))
	if got := field.take(); got != "announce Party was stopped.|end script prontera" {
		t.Fatalf("stop: %q", got)
	}
	if n := field.notices; len(n) != 2 || n[1].Result != ResultStopped {
		t.Fatalf("notices %+v", n)
	}
	// run:finish of a typed event's script ends its run, and an
	// invasion that is won gives its rewards.
	if ok, _ := runs.Request("Raid", "example.a", true); !ok {
		t.Fatal("the raid did not start")
	}
	s.Check(ctx, at(9, 2))
	field.take()
	runs.Note(Note{Kind: NoteFinish, Run: 2, Result: ResultWon})
	s.Check(ctx, at(9, 3))
	if got := field.take(); got != "reward prontera result|announce prontera drove back the invasion!|clear prontera" {
		t.Fatalf("finish: %q", got)
	}
	// It may start again at once.
	if ok, _ := runs.Request("Raid", "example.a", true); !ok {
		t.Fatal("a finished event did not start again")
	}
}
