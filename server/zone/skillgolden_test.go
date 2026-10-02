package zone

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/rng"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// The golden tests of skill_status.json: the statuses a skill starts
// through its Go behavior, with their values and timers, and the stats
// and normal attack damage they change, against rAthena's recording.

// forcedMinSource makes rng.Rand's IntN return 0 for every bound, as
// rAthena's rnd_force_min: every roll is lowest and every chance
// succeeds. Uint64N multiplies the value by the bound and keeps the high
// half, which is 0 for a value of 1, and the low half is the bound, which
// is never below the rejection threshold.
type forcedMinSource struct{}

func (forcedMinSource) Uint64() uint64 { return 1 }

// statusGaps are skills with Go behavior whose recorded statuses do not
// match yet, with the reason. Each job's test file adds its own in init.
var statusGaps = map[string]string{}

// goldenSkillStatus are the skills skill_status.json covers whose cases
// must match: every skill with Go behavior that has no gap.
func goldenSkillStatus(id int, name string) bool {
	_, gap := statusGaps[name]
	_, ported := skillImpls[id]
	return ported && !gap
}

type statusEntryCase struct {
	Type  int   `json:"type"`
	Val1  int   `json:"val1"`
	Val2  int   `json:"val2"`
	Val3  int   `json:"val3"`
	Val4  int   `json:"val4"`
	Timer int64 `json:"timer"`
}

type goldenDamage struct {
	Damage  int64 `json:"damage"`
	Damage2 int64 `json:"damage2"`
	Type    int   `json:"type"`
	Div     int   `json:"div"`
	DmgLv   int   `json:"dmg_lv"`
}

type skillStatusCase struct {
	Skill        int               `json:"skill"`
	SkillName    string            `json:"skill_name"`
	Level        int               `json:"level"`
	Job          int               `json:"job"`
	Sex          int               `json:"sex"`
	Build        string            `json:"build"`
	WeaponType   int               `json:"weapon_type"`
	Weapon       uint32            `json:"weapon"`
	Ammo         uint32            `json:"ammo"`
	Target       uint32            `json:"target"`
	AttackBefore *goldenDamage     `json:"attack_before"`
	AttackAfter  *goldenDamage     `json:"attack_after"`
	Before       map[string]any    `json:"before"`
	After        map[string]any    `json:"after"`
	Statuses     []statusEntryCase `json:"statuses"`
}

type goldenPlayer struct {
	Job      int      `json:"job"`
	Build    string   `json:"build"`
	Sex      int      `json:"sex"`
	JobLevel int      `json:"job_level"`
	Skills   [][2]int `json:"skills"`
}

// goldenBuilds are the recorder's PC_SAMPLE_BUILDS.
var goldenBuilds = map[string]status.Stats{
	"all_1":  {Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1},
	"melee":  {Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1},
	"magic":  {Str: 1, Agi: 1, Vit: 30, Int: 99, Dex: 80, Luk: 1},
	"ranged": {Str: 30, Agi: 40, Vit: 1, Int: 1, Dex: 99, Luk: 30},
}

// goldenData is what a mode's golden zones share.
type goldenData struct {
	items    *item.Table
	tables   *status.Tables
	statuses *sc.DB
	skills   *skill.DB
	trees    *skill.Trees
	mobs     map[uint32]*battle.Status
}

var goldenCache sync.Map

// loadGoldenData loads a mode's data once per test binary.
func loadGoldenData(t *testing.T, mode reference.Mode) *goldenData {
	t.Helper()
	if v, ok := goldenCache.Load(mode); ok {
		return v.(*goldenData)
	}
	g := &goldenData{mobs: map[uint32]*battle.Status{}}
	var d item.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/items.json", &d); err != nil {
		t.Fatal(err)
	}
	var e item.EquipData
	if err := reference.LoadFile("extracts/"+string(mode)+"/equipment.json", &e); err != nil {
		t.Fatal(err)
	}
	d.MergeEquipment(&e)
	g.items = item.New(&d)
	var jd status.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &jd); err != nil {
		t.Fatal(err)
	}
	var err error
	if g.tables, err = status.New(&jd); err != nil {
		t.Fatal(err)
	}
	core, err := convert.LoadCore(mode)
	if err != nil {
		t.Fatal(err)
	}
	if g.statuses, _, err = core.LoadStatuses(mode); err != nil {
		t.Fatal(err)
	}
	g.skills, g.trees = testSkills(t, mode)
	var mobs struct {
		Mobs []struct {
			ID     int           `json:"id"`
			Level  int           `json:"level"`
			Status battle.Status `json:"status"`
		} `json:"mobs"`
	}
	if err := reference.Load(mode, "mob_status", &mobs); err != nil {
		t.Fatal(err)
	}
	var taken struct {
		DamageTaken map[string]int `json:"damage_taken"`
	}
	b, err := os.ReadFile("../battle/testdata/damage_taken.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &taken); err != nil {
		t.Fatal(err)
	}
	for _, m := range mobs.Mobs {
		st := m.Status
		st.Level, st.ID = m.Level, m.ID
		if mode == reference.Renewal {
			st.DamageTaken = taken.DamageTaken[strconv.Itoa(m.ID)]
		}
		g.mobs[uint32(m.ID)] = &st // #nosec G115 -- monster IDs are positive.
	}
	v, _ := goldenCache.LoadOrStore(mode, g)
	return v.(*goldenData)
}

// goldenZone makes a zone of a mode with the full item, job, status and
// skill data, rolls forced to their lowest value, and the given monsters
// next to the middle of the map.
func goldenZone(t *testing.T, mode reference.Mode, mobs ...uint32) *Zone {
	t.Helper()
	g := loadGoldenData(t, mode)
	var spawns []Spawn
	for i, id := range mobs {
		st := *g.mobs[id]
		spawns = append(spawns, Spawn{Mob: id, Name: fmt.Sprint(id), Amount: 1, X: 20 + i, Y: 22, Speed: 400, Status: &st,
			RespawnMs: 60000})
	}
	z := New("map/test", open(40), rng.New(forcedMinSource{}), spawns)
	z.Items = g.items
	z.Progress = testTables()
	z.Status = g.tables
	z.Start = status.Character{Male: true, Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}}
	z.Statuses = g.statuses
	z.Skills, z.SkillTrees = g.skills, g.trees
	return z
}

// goldenPlayerIn adds a level 99 sample player to a golden zone, with its
// learned skills, its weapon and its ammunition, at the middle of the
// map.
func goldenPlayerIn(t *testing.T, z *Zone, sp goldenPlayer, weapon, ammo uint32) *Player {
	t.Helper()
	p := add(z, "golden")
	p.char.Job = sp.Job
	p.char.Male = sp.Sex == 1
	p.char.Stats = goldenBuilds[sp.Build]
	p.char.Skills = map[int]int{}
	for _, s := range sp.Skills {
		p.char.Skills[s[0]] = s[1]
	}
	p.progress.Job = sp.Job
	p.progress.BaseLevel, p.progress.JobLevel = 99, sp.JobLevel
	p.maxWeight = 1 << 30
	p.entity.x, p.entity.y = 20, 20
	z.recalc(p)
	for _, id := range []uint32{weapon, ammo} {
		if id == 0 {
			continue
		}
		amount := 1
		if id == ammo {
			amount = 100
		}
		slot := give(t, z, p, id, amount)
		equip(z, p, slot)
	}
	z.recalc(p)
	p.entity.hp = int64(p.entity.status.MaxHP)
	p.sp = int64(p.entity.status.MaxSP)
	events(p)
	return p
}

// monsterOf finds a golden zone's monster of an ID.
func monsterOf(z *Zone, id uint32) *entity {
	for e := range z.entities.all() {
		if e.monster != nil && e.monster.spawn.Mob == id {
			return e
		}
	}
	return nil
}

// statusFields are the battle status fields of a recording, as JSON.
func statusFields(st *battle.Status) map[string]any {
	b, _ := json.Marshal(st)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// diffFields lists the recorded fields that differ, skipping those the
// recording does not have.
func diffFields(got, want map[string]any) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(want)) {
		w, g := want[k], got[k]
		if wm, ok := w.(map[string]any); ok {
			gm, _ := g.(map[string]any)
			for _, d := range diffFields(gm, wm) {
				out = append(out, k+"."+d)
			}
			continue
		}
		if fmt.Sprint(w) != fmt.Sprint(g) {
			out = append(out, fmt.Sprintf("%s %v, want %v", k, g, w))
		}
	}
	return out
}

// goldenAttack is a forced-minimum normal attack, as the recorder's.
func goldenAttack(z *Zone, src, target *entity) battle.Result {
	if z.renewal() {
		return battle.Attack(src.status, z.defender(target), z.rand)
	}
	return battle.AttackPreRenewal(src.status, z.defender(target), z.rand)
}

// sameAttack compares a normal attack with a recorded one.
func sameAttack(got battle.Result, want *goldenDamage) bool {
	kind := battle.Hit
	switch {
	case want.DmgLv == 1:
		kind = battle.LuckyDodge
	case want.DmgLv != 5:
		kind = battle.Miss
	case want.Type == 10 || want.Type == 13:
		kind = battle.Critical
	}
	return got.Damage == want.Damage+want.Damage2 && got.Left == want.Damage2 && got.Kind == kind
}

// TestSkillStatusGolden casts each recorded skill through its Go
// behavior, as the recorder's skill_castend_nodamage_id, and compares the
// statuses, the stats and the normal attack damage with rAthena's.
func TestSkillStatusGolden(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			var ref struct {
				Players []goldenPlayer    `json:"players"`
				Cases   []skillStatusCase `json:"cases"`
			}
			if err := reference.Load(mode, "skill_status", &ref); err != nil {
				t.Fatal(err)
			}
			players := map[string]goldenPlayer{}
			for _, p := range ref.Players {
				players[fmt.Sprintf("%d %s %d", p.Job, p.Build, p.Sex)] = p
			}
			matched, total := 0, 0
			bad, left := map[string]bool{}, map[string]bool{}
			for i := range ref.Cases {
				c := &ref.Cases[i]
				if !goldenSkillStatus(c.Skill, c.SkillName) {
					left[c.SkillName] = true
					continue
				}
				total++
				name := fmt.Sprintf("%s lv %d (job %d)", c.SkillName, c.Level, c.Job)
				if diffs := runSkillStatusCase(t, mode, c, players[fmt.Sprintf("%d %s %d", c.Job, c.Build, c.Sex)]); len(diffs) > 0 {
					bad[c.SkillName] = true
					t.Errorf("%s: %v", name, diffs)
					continue
				}
				matched++
			}
			t.Logf("%s: %d of %d skill status cases match; skills with a mismatch: %v; not ported: %v", mode, matched, total,
				slices.Sorted(maps.Keys(bad)), slices.Sorted(maps.Keys(left)))
		})
	}
}

// timerSlack is how much later than the cast rAthena's timers may run:
// the recorder reads the real clock, which moves while a skill runs.
const timerSlack = 50

// unitIDValue reports whether a recorded status value is a unit's ID:
// the recorder's players have account IDs from 2,100,000, and rAthena's
// monsters IDs from 110,000,000 (START_NPC_NUM). wAthena's handles differ,
// so any value other than 0 matches them.
func unitIDValue(v int) bool { return v >= 2100000 && v < 2200000 || v >= 110000000 }

// sameStatuses compares statuses with a recording, timers within
// timerSlack.
func sameStatuses(got, want []statusEntryCase) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		g, w := got[i], want[i]
		for _, v := range []struct{ g, w *int }{{&g.Val1, &w.Val1}, {&g.Val2, &w.Val2}, {&g.Val3, &w.Val3}, {&g.Val4, &w.Val4}} {
			if unitIDValue(*v.w) && *v.g != 0 {
				*v.g = *v.w
			}
		}
		gt, wt := g.Timer, w.Timer
		g.Timer, w.Timer = 0, 0
		if g != w || (gt < 0) != (wt < 0) || wt < gt || wt > gt+timerSlack {
			return false
		}
	}
	return true
}

// runSkillStatusCase runs one case and returns its differences.
func runSkillStatusCase(t *testing.T, mode reference.Mode, c *skillStatusCase, sp goldenPlayer) []string {
	t.Helper()
	mobs := []uint32{1040}
	if c.Target != 0 {
		mobs = []uint32{c.Target, 1040}
	}
	z := goldenZone(t, mode, mobs...)
	p := goldenPlayerIn(t, z, sp, c.Weapon, c.Ammo)
	src := p.entity
	golem := monsterOf(z, 1040)
	// The recorder's Golem stands apart from the caster, so area skills
	// such as Frost Nova do not reach it.
	golem.x, golem.y = 35, 35
	target := src
	if c.Target != 0 {
		target = monsterOf(z, c.Target)
	}
	var diffs []string
	if c.AttackBefore != nil {
		if got := goldenAttack(z, src, golem); !sameAttack(got, c.AttackBefore) {
			diffs = append(diffs, fmt.Sprintf("attack before %+v, want %+v", got, *c.AttackBefore))
		}
	}
	if d := diffFields(statusFields(target.status), c.Before); len(d) > 0 {
		diffs = append(diffs, fmt.Sprintf("before: %v", d))
	}
	def := z.Skills.Get(c.Skill)
	start := z.now
	z.castendNoDamage(src, target, def, c.Level, 0)
	z.flushStats()
	var got []statusEntryCase
	if !target.sc.Empty() {
		for _, e := range target.sc.Sorted() {
			timer := int64(-1)
			if e.Timer() >= 0 {
				timer = e.Timer() - start
			}
			got = append(got, statusEntryCase{Type: e.Type, Val1: e.Val1, Val2: e.Val2, Val3: e.Val3, Val4: e.Val4, Timer: timer})
		}
	}
	if !sameStatuses(got, c.Statuses) {
		want := c.Statuses
		diffs = append(diffs, fmt.Sprintf("statuses %v, want %v", got, want))
	}
	if d := diffFields(statusFields(target.status), c.After); len(d) > 0 {
		diffs = append(diffs, fmt.Sprintf("after: %v", d))
	}
	if c.AttackAfter != nil {
		if got := goldenAttack(z, src, golem); !sameAttack(got, c.AttackAfter) {
			diffs = append(diffs, fmt.Sprintf("attack after %+v, want %+v", got, *c.AttackAfter))
		}
	}
	return diffs
}
