package zone

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// The parity tests of the monster AI and monster skills against the
// recordings mob_ai.json and mob_skill_use.json, which the rAthena fork's
// recorder made on an open part of prt_fild08 with every roll at its
// lowest (reference/rathena/README.md, "Monster AI recordings").

// parityCenter is the recordings' center cell on prt_fild08. The test
// zone is the same size, with every cell walkable, so map-wide searches
// such as Teleport land on the same cells.
var parityCenter = [2]int{218, 46}

// mobDefs are the monsters of a mode, by ID, with their battle status and
// their skills lists, read once.
type mobDefs struct {
	defs   map[uint32]*mob.Def
	status map[uint32]*battle.Status
	skills map[uint32][]mob.Skill
	// tmpl are templates of every monster, for summons.
	tmpl map[uint32]Spawn
}

var mobDefsCache sync.Map // reference.Mode -> *mobDefs

func loadMobDefs(t *testing.T, mode reference.Mode, skills *skill.DB) *mobDefs {
	t.Helper()
	if v, ok := mobDefsCache.Load(mode); ok {
		return v.(*mobDefs)
	}
	d, err := mob.Load(mode)
	if err != nil {
		t.Fatal(err)
	}
	out := &mobDefs{defs: map[uint32]*mob.Def{}, status: map[uint32]*battle.Status{}, skills: map[uint32][]mob.Skill{}, tmpl: map[uint32]Spawn{}}
	for i := range d.Monsters {
		out.defs[d.Monsters[i].ID] = &d.Monsters[i]
	}
	ground := func(id int) bool {
		s := skills.Get(id)
		return s != nil && s.Target == skill.TargetGround
	}
	exists := func(id uint32) bool { return out.defs[id] != nil }
	for id, def := range out.defs {
		st := mob.Calc(def, mode == reference.Renewal)
		out.status[id] = &st
		for _, r := range def.Skills {
			s, _, err := mob.FromRaw(r, def.Mode, ground, exists, d.ChatLine)
			if err != nil {
				t.Fatal(err)
			}
			out.skills[id] = append(out.skills[id], s)
		}
		out.tmpl[id] = Spawn{Mob: id, Name: def.Name, Amount: 1, Speed: st.Speed, CanMove: st.Mode&mob.ModeCanMove != 0,
			ViewRange: def.SkillRange, ChaseRange: def.ChaseRange, Status: out.status[id], Skills: out.skills[id]}
	}
	v, _ := mobDefsCache.LoadOrStore(mode, out)
	return v.(*mobDefs)
}

// parityZone makes a zone of a mode for one recorded case: the full data,
// a 400 × 400 open map, rolls at their lowest, and every monster as a
// template.
func parityZone(t *testing.T, mode reference.Mode) (*Zone, *mobDefs) {
	t.Helper()
	g := loadGoldenData(t, mode)
	z := New("map/test", parityGrid(), rng.New(forcedMinSource{}), nil)
	z.Items = g.items
	z.Progress = testTables()
	z.Status = g.tables
	z.Statuses = g.statuses
	z.Skills, z.SkillTrees = g.skills, g.trees
	defs := loadMobDefs(t, mode, g.skills)
	z.Mods = &Mods{Monsters: defs.tmpl}
	return z, defs
}

// parityGrid is prt_fild08's size with the recordings' open square: the
// cells within 20 of the center are walkable and the rest are walls. A
// map-wide search with every roll at its lowest, as Teleport makes,
// finds no cell, as on prt_fild08. The middle cell is walkable too.
func parityGrid() *path.Grid {
	parityGridOnce.Do(func() { parityGridShared = makeParityGrid() })
	return parityGridShared
}

var (
	parityGridOnce   sync.Once
	parityGridShared *path.Grid
)

func makeParityGrid() *path.Grid {
	g := &path.Grid{Width: 400, Height: 400, Cells: make([]uint8, 400*400)}
	for y := parityCenter[1] - 20; y <= parityCenter[1]+20; y++ {
		for x := parityCenter[0] - 20; x <= parityCenter[0]+20; x++ {
			g.Cells[x+y*g.Width] = path.Walkable | path.Shootable
		}
	}
	// The map's middle cell, where new players start, so the zone finds
	// it at once.
	g.Cells[200+200*g.Width] = path.Walkable | path.Shootable
	return g
}

// parityPlayer adds a player of a job, build and levels on a cell, with
// full HP and SP and no perfect dodge, as the recorder's sample players.
func parityPlayer(t *testing.T, z *Zone, job int, build string, base, jobLv, x, y int) *Player {
	t.Helper()
	p := add(z, fmt.Sprintf("p%d", len(z.players)))
	p.char.Job = job
	p.char.Male = true
	p.char.Stats = goldenBuilds[build]
	p.char.Skills = map[int]int{}
	p.progress.Job = job
	p.progress.BaseLevel, p.progress.JobLevel = base, jobLv
	p.maxWeight = 1 << 30
	p.entity.x, p.entity.y = x, y
	z.recalc(p)
	p.entity.status.Flee2 = 0
	p.entity.hp = int64(p.entity.status.MaxHP)
	p.sp = int64(p.entity.status.MaxSP)
	events(p)
	return p
}

// parityMob places a monster on a cell with its data, without its spawn
// skills firing, and with skills when set.
func parityMob(t *testing.T, z *Zone, defs *mobDefs, id uint32, x, y int, mode uint32, skills []mob.Skill) *entity {
	t.Helper()
	st := *defs.status[id]
	st.Mode = mode
	sp := defs.tmpl[id]
	sp.Status = &st
	sp.Skills = nil
	sp.CanMove = st.Mode&mob.ModeCanMove != 0
	sp.X, sp.Y = x, y
	e := z.spawnOne(&sp)
	if e == nil {
		t.Fatal("no cell")
	}
	e.x, e.y = x, y
	sp.Skills = skills
	e.monster.skillDelay = make([]int64, len(skills))
	return e
}

// mobAICase is one case of mob_ai.json.
type mobAICase struct {
	Probe     uint32 `json:"probe"`
	AI        string `json:"ai"`
	Case      string `json:"case"`
	DX        int    `json:"dx"`
	DY        int    `json:"dy"`
	Level     int    `json:"player_level"`
	Normal    bool   `json:"normal"`
	State     string `json:"state"`
	OtherBusy bool   `json:"other_busy"`
	Result    struct {
		Target         string           `json:"target"`
		State          string           `json:"state"`
		Aggressive     bool             `json:"aggressive"`
		Walking        bool             `json:"walking"`
		To             []int            `json:"to"`
		PathLen        int              `json:"path_len"`
		TargetTo       string           `json:"target_to"`
		AttackTarget   string           `json:"attack_target"`
		AttackContinue bool             `json:"attack_continue"`
		AttackTimer    bool             `json:"attack_timer"`
		PlayerHPLost   map[string]int64 `json:"player_hp_lost"`
		PlayersDead    []string         `json:"players_dead"`
		OtherTarget    string           `json:"other_target"`
		Looted         *struct {
			Count    int  `json:"count"`
			ItemGone bool `json:"item_gone"`
		} `json:"looted"`
		CastStarted *bool `json:"cast_started"`
	} `json:"result"`
}

// TestMobAIParity runs every case of mob_ai.json in each mode: one
// decision of the hard AI of a probe monster with each AI type's modes,
// and compares what the monster does.
func TestMobAIParity(t *testing.T) {
	testrace.SkipUnderRace(t, "it compares recorded reference cases on one goroutine")
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			var rec struct {
				Cases []mobAICase `json:"cases"`
			}
			if err := reference.Load(mode, "mob_ai", &rec); err != nil {
				t.Skip(err)
			}
			matched := 0
			bad := map[string]int{}
			for i, c := range rec.Cases {
				got := runMobAICase(t, mode, c)
				want := c.Result
				var diffs []string
				check := func(name string, g, w any) {
					if !reflect.DeepEqual(g, w) {
						diffs = append(diffs, fmt.Sprintf("%s %v, want %v", name, g, w))
					}
				}
				check("target", got.Target, want.Target)
				check("state", got.State, want.State)
				check("aggressive", got.Aggressive, want.Aggressive)
				check("walking", got.Walking, want.Walking)
				if want.Walking {
					check("to", got.To, want.To)
					check("path_len", got.PathLen, want.PathLen)
				}
				check("target_to", got.TargetTo, want.TargetTo)
				check("attack_target", got.AttackTarget, want.AttackTarget)
				check("attack_continue", got.AttackContinue, want.AttackContinue)
				check("attack_timer", got.AttackTimer, want.AttackTimer)
				check("player_hp_lost", got.PlayerHPLost, want.PlayerHPLost)
				if c.Case == "assist" {
					check("other_target", got.OtherTarget, want.OtherTarget)
				}
				if want.Looted != nil {
					check("looted", *got.Looted, *want.Looted)
				}
				if want.CastStarted != nil {
					check("cast_started", *got.CastStarted, *want.CastStarted)
				}
				if len(diffs) == 0 {
					matched++
					continue
				}
				key := c.Case + "/" + c.AI
				bad[key]++
				if bad[key] <= 2 && !testing.Short() {
					t.Logf("case %d %s probe %d AI %s dx %d dy %d lv %d state %q normal %v busy %v: %s", i, c.Case, c.Probe, c.AI, c.DX, c.DY,
						c.Level, c.State, c.Normal, c.OtherBusy, strings.Join(diffs, "; "))
				}
			}
			t.Logf("%s: %d of %d mob_ai.json cases match", mode, matched, len(rec.Cases))
			if matched != len(rec.Cases) {
				keys := slices.Sorted(maps.Keys(bad))
				var parts []string
				for _, k := range keys {
					parts = append(parts, fmt.Sprintf("%s %d", k, bad[k]))
				}
				t.Errorf("mismatches by case and AI: %s", strings.Join(parts, ", "))
			}
		})
	}
}

// aiResult is what runMobAICase reads, in the recording's shape.
type aiResult struct {
	Target, State               string
	Aggressive, Walking         bool
	To                          []int
	PathLen                     int
	TargetTo, AttackTarget      string
	AttackContinue, AttackTimer bool
	PlayerHPLost                map[string]int64
	OtherTarget                 string
	Looted                      *struct {
		Count    int  `json:"count"`
		ItemGone bool `json:"item_gone"`
	}
	CastStarted *bool
}

// runMobAICase sets up one recorded case and runs it.
func runMobAICase(t *testing.T, mode reference.Mode, c mobAICase) aiResult {
	z, defs := parityZone(t, mode)
	cx, cy := parityCenter[0], parityCenter[1]
	modes, ok := mob.AIModes(c.AI)
	if !ok {
		t.Fatalf("AI %s", c.AI)
	}
	labels := map[*entity]string{}
	players := map[string]*Player{}
	addPlayer := func(label string, level, x, y int) *Player {
		var p *Player
		if level == 99 {
			p = parityPlayer(t, z, 7, "melee", 99, 50, x, y)
		} else {
			p = parityPlayer(t, z, 0, "all_1", 1, 1, x, y)
		}
		labels[p.entity] = label
		players[label] = p
		return p
	}
	m := parityMob(t, z, defs, c.Probe, cx, cy, modes, []mob.Skill{})
	m.monster.level = 50
	// mob_spawn_dataset gives a monster loot slots when mob_db's modes
	// make it a looter, before the recorder changes its modes.
	m.monster.canLoot = defs.status[c.Probe].Mode&mob.ModeLooter != 0
	labels[m] = "self"
	z.now = 5000
	var other *entity
	var it *entity
	var mage *Player
	switch c.Case {
	case "search":
		addPlayer("player", c.Level, cx+c.DX, cy)
		z.mobThink(m)
	case "hit":
		p := addPlayer("player", 99, cx+c.DX, cy)
		z.mobSetAttacked(m, p.entity, c.Normal)
	case "change":
		a := addPlayer("player", 99, cx+1, cy)
		b := addPlayer("player2", 1, cx+c.DX, cy+c.DY)
		st, _ := mob.ParseState(c.State)
		m.monster.target, m.monster.ai.State = a.entity, st
		if st == mob.StateBerserk {
			// The recorder sets ud.target and attack_continue without an
			// attack timer, which the zone has no state for: the AI sees
			// no attack in progress.
			m.monster.attackContinue = true
		}
		z.mobSetAttacked(m, b.entity, c.Normal)
	case "assist":
		addPlayer("player", 1, cx+1, cy)
		other = parityMob(t, z, defs, c.Probe, cx+c.DX, cy+c.DY, modes, []mob.Skill{})
		other.monster.level = 50
		other.monster.canLoot = defs.status[c.Probe].Mode&mob.ModeLooter != 0
		labels[other] = "mob2"
		if c.OtherBusy {
			b := addPlayer("player2", 99, cx, cy-5)
			other.monster.target = b.entity
		}
		z.mobThink(m)
	case "loot":
		it = &entity{x: cx + c.DX, y: cy, item: &groundItem{id: 909, amount: 1, expires: 1 << 40}}
		z.entities.add(it)
		labels[it] = "item"
		z.mobThink(m)
	case "chase":
		p := addPlayer("player", 99, cx+c.DX, cy)
		m.monster.target, m.monster.ai.State = p.entity, mob.StateRush
		z.mobThink(m)
	case "cast":
		mage = parityPlayer(t, z, 2, "magic", 99, 50, cx+c.DX, cy)
		mage.char.Skills[skill.IDFireBolt] = 10
		labels[mage.entity] = "player"
		players["player"] = mage
		if c.State == "chase" {
			b := addPlayer("player2", 1, cx-1, cy)
			m.monster.target, m.monster.ai.State = b.entity, mob.StateRush
		}
		z.skillUseID(mage.entity, m, z.Skills.Get(skill.IDFireBolt), 1)
	default:
		t.Fatalf("case %q", c.Case)
	}
	// The attack that unit_attack makes at once.
	hp := map[string]int64{}
	for label, p := range players {
		hp[label] = p.entity.hp
	}
	if c.Case != "cast" {
		z.fight()
	}
	label := func(e *entity) string {
		if e == nil {
			return "none"
		}
		if l, ok := labels[e]; ok {
			return l
		}
		return "other"
	}
	m2 := m.monster
	attack := m.target
	if attack == nil && m2.ai.State == mob.StateLoot && m2.chaseTo != nil && m2.chaseTo.item != nil {
		// rAthena keeps the item a looter walks to as ud.target
		// (unit_set_target); the zone keeps it as the walk's target.
		attack = m2.chaseTo
	}
	r := aiResult{Target: label(m2.target), State: m2.ai.State.String(), Aggressive: m2.ai.Aggressive,
		Walking: len(m.path) > 0, TargetTo: label(m2.chaseTo), AttackTarget: label(attack),
		AttackContinue: m2.attackContinue || m.target != nil && m.repeat,
		AttackTimer:    m.target != nil && m.repeat && m.nextAttack > z.now,
		PlayerHPLost:   map[string]int64{}}
	if r.Walking {
		x, y := m.x, m.y
		for _, d := range m.path {
			dx, dy := d.Step()
			x, y = x+dx, y+dy
		}
		r.To, r.PathLen = []int{x - cx, y - cy}, len(m.path)
	}
	for l, p := range players {
		r.PlayerHPLost[l] = int64(p.entity.status.MaxHP) - max(p.entity.hp, 0)
	}
	if other != nil {
		r.OtherTarget = label(other.monster.target)
	}
	if it != nil {
		r.Looted = &struct {
			Count    int  `json:"count"`
			ItemGone bool `json:"item_gone"`
		}{len(m2.loot), !z.entities.has(it)}
	}
	if mage != nil {
		started := mage.entity.cast != nil
		r.CastStarted = &started
	}
	return r
}

// mobSkillCase is one case of mob_skill_use.json.
type mobSkillCase struct {
	Mob        uint32 `json:"mob"`
	Case       string `json:"case"`
	State      string `json:"state"`
	HP         int    `json:"hp"`
	Used       int    `json:"used"`
	Skill      int    `json:"skill"`
	Level      int    `json:"level"`
	Target     string `json:"target"`
	Cell       []int  `json:"cell"`
	CastMs     *int64 `json:"cast_ms"`
	CastJitter int64  `json:"cast_jitter"`
	Damage     int64  `json:"damage"`
	EventSkill int    `json:"event_skill"`
	Effects    struct {
		PlayerHPLost   int64    `json:"player_hp_lost"`
		PlayerDead     bool     `json:"player_dead"`
		PlayerStatuses []int    `json:"player_statuses"`
		PlayerMoved    any      `json:"player_moved"`
		MobGone        bool     `json:"mob_gone"`
		MobHPChange    int64    `json:"mob_hp_change"`
		MobStatuses    []int    `json:"mob_statuses"`
		MobMoved       []int    `json:"mob_moved"`
		MobMode        *uint32  `json:"mob_mode"`
		MobChangedTo   uint32   `json:"mob_changed_to"`
		Summoned       []uint32 `json:"summoned"`
	} `json:"effects"`
}

// TestMobSkillUseParity runs every case of mob_skill_use.json in each
// mode: which entry of a monster's skills list mobskill_use picks in a
// state, at an HP, or for an event, and the cast it starts. These are
// "selection" counts. The effects of the skill are compared too, and
// counted apart, since many monster skills have no port yet.
func TestMobSkillUseParity(t *testing.T) {
	testrace.SkipUnderRace(t, "it compares recorded reference cases on one goroutine")
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			var rec struct {
				Cases []mobSkillCase `json:"cases"`
			}
			if err := reference.Load(mode, "mob_skill_use", &rec); err != nil {
				t.Skip(err)
			}
			// WATHENA_MOB_SKILLS lists skills, such as NPC_FIREATTACK,
			// whose every mismatch is logged, for porting them.
			watch := map[string]bool{}
			for _, n := range strings.Split(os.Getenv("WATHENA_MOB_SKILLS"), ",") {
				if n != "" {
					watch[n] = true
				}
			}
			sel, eff, effTotal := 0, 0, 0
			badSel := map[string]int{}
			badEff := map[string]int{}
			for i, c := range rec.Cases {
				got := runMobSkillCase(t, mode, c)
				var diffs []string
				check := func(name string, g, w any) {
					if !reflect.DeepEqual(g, w) {
						diffs = append(diffs, fmt.Sprintf("%s %v, want %v", name, g, w))
					}
				}
				check("used", got.Used, c.Used)
				if c.Used >= 0 {
					check("skill", got.Skill, c.Skill)
					check("level", got.Level, c.Level)
					check("target", got.Target, c.Target)
					if c.Target == "cell" {
						check("cell", got.Cell, c.Cell)
					}
					switch {
					case c.CastMs == nil && got.CastMs != nil:
						diffs = append(diffs, fmt.Sprintf("cast %d ms, want none", *got.CastMs))
					case c.CastMs != nil && got.CastMs == nil:
						diffs = append(diffs, fmt.Sprintf("no cast, want %d ms", *c.CastMs))
					case c.CastMs != nil && (*got.CastMs < *c.CastMs-c.CastJitter-1 || *got.CastMs > *c.CastMs+1):
						diffs = append(diffs, fmt.Sprintf("cast %d ms, want %d", *got.CastMs, *c.CastMs))
					}
				}
				name := fmt.Sprint(c.Skill)
				if d := z0Skills(t, mode).Get(c.Skill); d != nil {
					name = d.Name
				}
				if len(diffs) > 0 {
					badSel[c.Case]++
					if badSel[c.Case] <= 3 && !testing.Short() {
						t.Logf("case %d mob %d %s state %s hp %d: %s", i, c.Mob, c.Case, c.State, c.HP, strings.Join(diffs, "; "))
					}
					continue
				}
				sel++
				if c.Used < 0 {
					continue
				}
				effTotal++
				var ed []string
				ec := func(n string, g, w any) {
					if !reflect.DeepEqual(g, w) {
						ed = append(ed, fmt.Sprintf("%s %v, want %v", n, g, w))
					}
				}
				w := c.Effects
				ec("player_hp_lost", got.Effects.PlayerHPLost, w.PlayerHPLost)
				ec("player_dead", got.Effects.PlayerDead, w.PlayerDead)
				ec("player_statuses", nilEmpty(got.Effects.PlayerStatuses), nilEmpty(w.PlayerStatuses))
				ec("mob_gone", got.Effects.MobGone, w.MobGone)
				if !w.MobGone {
					ec("mob_hp_change", got.Effects.MobHPChange, w.MobHPChange)
					ec("mob_statuses", nilEmpty(got.Effects.MobStatuses), nilEmpty(w.MobStatuses))
					ec("mob_moved", got.Effects.MobMoved, w.MobMoved)
					ec("mob_changed_to", got.Effects.MobChangedTo, w.MobChangedTo)
					ec("mob_mode", modeOf(got.Effects.MobMode), modeOf(w.MobMode))
				}
				ec("summoned", nilEmpty32(got.Effects.Summoned), nilEmpty32(w.Summoned))
				if len(ed) == 0 {
					eff++
					continue
				}
				badEff[name]++
				if watch[name] || badEff[name] <= 1 && !testing.Short() {
					t.Logf("effects: case %d mob %d %s %s lv %d: %s", i, c.Mob, c.Case, name, c.Level, strings.Join(ed, "; "))
				}
			}
			t.Logf("%s: selection %d of %d cases match; effects %d of %d used skills match", mode, sel, len(rec.Cases), eff, effTotal)
			if len(badEff) > 0 {
				type kv struct {
					k string
					n int
				}
				var list []kv
				for k, n := range badEff {
					list = append(list, kv{k, n})
				}
				sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n || list[i].n == list[j].n && list[i].k < list[j].k })
				var parts []string
				for _, e := range list {
					parts = append(parts, fmt.Sprintf("%s %d", e.k, e.n))
				}
				t.Logf("effect mismatches by skill: %s", strings.Join(parts, ", "))
			}
			if sel != len(rec.Cases) {
				t.Errorf("selection mismatches by case: %v", badSel)
			}
		})
	}
}

// modeOf shows a recorded mode change, or "none".
func modeOf(m *uint32) string {
	if m == nil {
		return "none"
	}
	return fmt.Sprintf("0x%X", *m)
}

func nilEmpty(s []int) []int {
	if len(s) == 0 {
		return nil
	}
	return s
}

func nilEmpty32(s []uint32) []uint32 {
	if len(s) == 0 {
		return nil
	}
	return s
}

// z0Skills is the skill database of a mode.
func z0Skills(t *testing.T, mode reference.Mode) *skill.DB { return loadGoldenData(t, mode).skills }

// runMobSkillCase sets up one recorded case of mob_skill_use.json and
// runs it.
func runMobSkillCase(t *testing.T, mode reference.Mode, c mobSkillCase) mobSkillCase {
	z, defs := parityZone(t, mode)
	cx, cy := parityCenter[0], parityCenter[1]
	p := parityPlayer(t, z, 7, "melee", 99, 50, cx+1, cy)
	skills := defs.skills[c.Mob]
	m := parityMob(t, z, defs, c.Mob, cx, cy, defs.status[c.Mob].Mode, skills)
	mm := m.monster
	m.hp = max(int64(m.status.MaxHP)*int64(c.HP)/100, 1)
	st, _ := mob.ParseState(c.State)
	mm.ai.State = st
	if st == mob.StateBerserk || st == mob.StateAngry || st == mob.StateRush || st == mob.StateFollow {
		mm.target = p.entity
	}
	z.now = 2000
	hp0, mode0 := m.hp, m.status.Mode
	used := false
	switch c.Case {
	case "hp":
		used = z.mobSkillUse(m, mobEventNone, 0)
	case "closedattacked":
		used = z.mobSkillEvent(m, p.entity, mob.CondHitMelee, 0, 100)
	case "longrangeattacked":
		used = z.mobSkillEvent(m, p.entity, mob.CondHitRanged, 0, 100)
	case "skillused":
		used = z.mobSkillEvent(m, p.entity, mob.CondHitBySkill, c.EventSkill, 100)
	case "casttargeted":
		used = z.mobSkillEvent(m, p.entity, mob.CondTargetedByCast, 0, 0)
	case "groundattacked":
		used = z.mobSkillEvent(m, p.entity, mob.CondHitBySkill, skill.IDFireWall, 100)
	case "damagedgt":
		used = z.mobSkillEvent(m, p.entity, mob.CondHitMelee, 0, c.Damage)
	case "rudeattacked":
		mm.attackedCount = 1
		used = z.mobSkillUse(m, mobEventNone, 0)
	case "spawn":
		used = z.mobSkillUse(m, mob.CondSpawned, 0)
	default:
		t.Fatalf("case %q", c.Case)
	}
	out := mobSkillCase{Used: -1}
	if used {
		u := mm.lastUse
		s := skills[u.idx]
		out.Used, out.Skill, out.Level = u.idx, s.ID, s.Level
		switch {
		case u.ground:
			out.Target, out.Cell = "cell", []int{u.x - cx, u.y - cy}
		case u.target == m:
			out.Target = "self"
		case u.target == p.entity:
			out.Target = "player"
		case u.target != nil && u.target == mm.master:
			out.Target = "master"
		case u.target != nil && u.target.monster != nil:
			out.Target = "friend"
		default:
			out.Target = "none"
		}
		if m.cast != nil {
			ms := m.cast.end - z.now
			out.CastMs = &ms
			z.castEnd(m, true)
		}
	}
	z.runScripts()
	e := &out.Effects
	e.PlayerHPLost = int64(p.entity.status.MaxHP) - max(p.entity.hp, 0)
	e.PlayerDead = p.entity.dead
	for _, s := range p.entity.sc.Sorted() {
		e.PlayerStatuses = append(e.PlayerStatuses, s.Type)
	}
	if !z.entities.has(m) || m.dead {
		e.MobGone = true
	} else {
		e.MobHPChange = m.hp - hp0
		for _, s := range m.sc.Sorted() {
			e.MobStatuses = append(e.MobStatuses, s.Type)
		}
		if m.x != cx || m.y != cy {
			e.MobMoved = []int{m.x - cx, m.y - cy}
		}
		if mm.spawn.Mob != c.Mob {
			e.MobChangedTo = mm.spawn.Mob
		}
		if md := m.status.Mode; md != mode0 {
			e.MobMode = &md
		}
	}
	for o := range z.entities.all() {
		if o.monster != nil && o != m {
			e.Summoned = append(e.Summoned, o.monster.spawn.Mob)
		}
	}
	slices.Sort(e.Summoned)
	return out
}

var _ = json.Marshal
