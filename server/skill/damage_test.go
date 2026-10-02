package skill

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
)

// forcedMin is rAthena's rnd_force_min: every roll is lowest and every
// chance succeeds.
type forcedMin struct{}

func (forcedMin) IntN(int) int { return 0 }

// builds are the recorder's PC_SAMPLE_BUILDS.
var builds = map[string]status.Stats{
	"all_1":    {Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1},
	"melee":    {Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1},
	"critical": {Str: 70, Agi: 90, Vit: 1, Int: 1, Dex: 40, Luk: 50},
	"magic":    {Str: 1, Agi: 1, Vit: 30, Int: 99, Dex: 80, Luk: 1},
	"ranged":   {Str: 30, Agi: 40, Vit: 1, Int: 1, Dex: 99, Luk: 30},
	"tank":     {Str: 60, Agi: 1, Vit: 99, Int: 1, Dex: 30, Luk: 1},
}

// The recorder's basic ammunition, by weapon type.
var basicAmmo = map[int]uint32{item.WeaponBow: 1750, item.WeaponGrenade: 13203}

type refDamage struct {
	Damage  int64 `json:"damage"`
	Damage2 int64 `json:"damage2"`
	Type    int   `json:"type"`
	Div     int   `json:"div"`
	DmgLv   int   `json:"dmg_lv"`
	Flag    int   `json:"flag"`
}

type skillCase struct {
	ForcedMin  refDamage `json:"forced_min"`
	Rolls      refRolls  `json:"rolls"`
	Flag       int       `json:"flag"`
	Skill      int       `json:"skill"`
	SkillName  string    `json:"skill_name"`
	Level      int       `json:"level"`
	AttackType int       `json:"attack_type"`
	Job        int       `json:"job"`
	Sex        *int      `json:"sex"`
	Build      string    `json:"build"`
	WeaponType int       `json:"weapon_type"`
	Weapon     uint32    `json:"weapon"`
	Ammo       *uint32   `json:"ammo"`
	CartWeight int       `json:"cart_weight"`
	Target     int       `json:"target"`
}

// male reports whether the case's player is male: rAthena's SEX_MALE is
// 1. Cases recorded before the recorder named the sex are male.
func (c *skillCase) male() bool { return c.Sex == nil || *c.Sex == 1 }

// samplePlayer is one entry of a recording's players: the skills a sample
// player learned and its job level.
type samplePlayer struct {
	Job      int      `json:"job"`
	Build    string   `json:"build"`
	Sex      int      `json:"sex"`
	JobLevel int      `json:"job_level"`
	Skills   [][2]int `json:"skills"`
}

// playerKey is the key of a sample player: job, build and sex.
func playerKey(job int, build string, male bool) string {
	sex := 0
	if male {
		sex = 1
	}
	return fmt.Sprintf("%d %s %d", job, build, sex)
}

type refRolls struct {
	Hits  int   `json:"hits"`
	Crits int   `json:"crits"`
	Min   int64 `json:"min"`
	Max   int64 `json:"max"`
	Total int64 `json:"total"`
}

// damageFixture holds what a mode's skill damage cases need.
type damageFixture struct {
	mode   reference.Mode
	db     *DB
	tables *status.Tables
	items  *item.Table
	mobs   map[int]*battle.Status
	// skills are the skills each sample player learns, and jobLevels its
	// job level at base level 99, by playerKey.
	skills    map[string]map[int]int
	jobLevels map[string]int
	cases     []skillCase
	rolls     int
}

func loadDB(t testing.TB, mode reference.Mode) *DB {
	t.Helper()
	var d Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/skills.json", &d); err != nil {
		t.Fatal(err)
	}
	db, err := Load(&d)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func loadDamageFixture(t testing.TB, mode reference.Mode) *damageFixture {
	t.Helper()
	f := &damageFixture{mode: mode, db: loadDB(t, mode), skills: map[string]map[int]int{}, jobLevels: map[string]int{}}
	var jobs status.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &jobs); err != nil {
		t.Fatal(err)
	}
	var err error
	if f.tables, err = status.New(&jobs); err != nil {
		t.Fatal(err)
	}
	var d item.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/items.json", &d); err != nil {
		t.Fatal(err)
	}
	var e item.EquipData
	if err := reference.LoadFile("extracts/"+string(mode)+"/equipment.json", &e); err != nil {
		t.Fatal(err)
	}
	d.MergeEquipment(&e)
	f.items = item.New(&d)

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
	// server/battle's testdata holds DamageTaken from rAthena's mob_db.yml,
	// which mob_status.json does not record.
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
	f.mobs = map[int]*battle.Status{}
	for _, m := range mobs.Mobs {
		st := m.Status
		st.Level = m.Level
		st.ID = m.ID
		// The recorder's monsters have full HP, which an HP of 0 stands
		// for (CurrentHP).
		if mode == reference.Renewal {
			// Pre-renewal's mob_db has no DamageTaken.
			st.DamageTaken = taken.DamageTaken[strconv.Itoa(m.ID)]
		}
		f.mobs[m.ID] = &st
	}

	var pcs struct {
		Players []struct {
			Job       int      `json:"job"`
			BaseLevel int      `json:"base_level"`
			JobLevel  int      `json:"job_level"`
			Build     string   `json:"build"`
			AllSkills bool     `json:"all_skills"`
			Skills    [][2]int `json:"skills"`
		} `json:"players"`
	}
	if err := reference.Load(mode, "pc_status", &pcs); err != nil {
		t.Fatal(err)
	}
	for _, p := range pcs.Players {
		if p.BaseLevel != 99 || !p.AllSkills {
			continue
		}
		skills := map[int]int{}
		for _, s := range p.Skills {
			skills[s[0]] = s[1]
		}
		key := playerKey(p.Job, p.Build, true)
		f.skills[key] = skills
		f.jobLevels[key] = p.JobLevel
	}

	var ref struct {
		RollsPerCase int            `json:"rolls_per_case"`
		Players      []samplePlayer `json:"players"`
		Cases        []skillCase    `json:"cases"`
	}
	if err := reference.Load(mode, "skill_damage", &ref); err != nil {
		t.Fatal(err)
	}
	// The recording's own players come first: they have the right sex.
	for _, p := range ref.Players {
		key := playerKey(p.Job, p.Build, p.Sex == 1)
		f.skills[key] = learnedMap(p.Skills)
		f.jobLevels[key] = p.JobLevel
	}
	f.cases, f.rolls = ref.Cases, ref.RollsPerCase
	return f
}

// learnedMap turns a recording's [skill, level] pairs into a map.
func learnedMap(pairs [][2]int) map[int]int {
	m := make(map[int]int, len(pairs))
	for _, s := range pairs {
		m[s[0]] = s[1]
	}
	return m
}

// build is the stat build create_skill_user picks for a skill and a
// weapon type.
func build(def *Def, weaponType int) string {
	switch def.Type {
	case battle.TypeMagic:
		return "magic"
	case battle.TypeMisc:
		return "ranged"
	}
	if weaponType == item.WeaponBow || weaponType >= item.WeaponRevolver && weaponType <= item.WeaponGrenade {
		return "ranged"
	}
	return "melee"
}

// attacker is the recorder's skill user: a level 99 player of the case's
// job and sex with every tree skill and the case's weapon, with the
// ammunition the case names, or basic ammunition for a bow or a gun in
// cases recorded before the recorder named it.
func (f *damageFixture) attacker(t testing.TB, c *skillCase, def *Def) (*battle.Status, error) {
	t.Helper()
	b := c.Build
	if b == "" {
		b = build(def, c.WeaponType)
	}
	key := playerKey(c.Job, b, c.male())
	skills, ok := f.skills[key]
	if !ok {
		return nil, fmt.Errorf("no level 99 %s player of job %d with all skills", b, c.Job)
	}
	ch := status.Character{Job: c.Job, Male: c.male(), BaseLevel: 99, JobLevel: f.jobLevels[key], Stats: builds[b], Skills: skills}
	ammo := uint32(0)
	if c.Weapon != 0 {
		inv := &item.Inventory{}
		wd, ok := f.items.Lookup(c.Weapon)
		if !ok {
			return nil, fmt.Errorf("item %d missing", c.Weapon)
		}
		eq := &status.Equipment{MapType: item.MapNormal}
		inv.Slots[0] = item.Slot{ID: c.Weapon, Amount: 1, Equip: uint32(wd.Location)}
		eq.Items[item.EqiHandR] = &status.Equipped{Def: wd, Position: uint32(wd.Location)}
		if uint32(wd.Location)&item.EqpHandL != 0 {
			eq.Items[item.EqiHandL] = eq.Items[item.EqiHandR]
		}
		a := basicAmmo[c.WeaponType]
		if c.Ammo != nil {
			a = *c.Ammo
		}
		if a != 0 {
			if ad, ok := f.items.Lookup(a); ok && ad.Jobs != nil {
				ammo = a
				inv.Slots[1] = item.Slot{ID: a, Amount: 100, Equip: item.EqpAmmo}
				eq.Items[item.EqiAmmo] = &status.Equipped{Def: ad, Position: item.EqpAmmo}
			}
		}
		eq.Worn = inv.Worn(f.items)
		ch.Equipment = eq
	}
	res, err := f.tables.Calc(ch)
	if err != nil {
		return nil, err
	}
	st := res.Battle
	// skill_check_condition_castbegin sets state.arrow_atk when the skill
	// requires ammunition.
	if st.Player != nil {
		// The recorder's players have 1 SP, which Asura Strike reads.
		st.Player.SP = 1
		st.Player.ArrowAttack = def.Requires.Ammo != 0 && ammo != 0
		if c.CartWeight > 0 {
			// The recorder's pc_setcart and the cart weight it sets.
			st.Player.CartWeight, st.Player.CartWeightMax = c.CartWeight, MaxCartWeight
		}
	}
	return &st, nil
}

func (c *skillCase) name() string {
	return fmt.Sprintf("%s lv %d (job %d, weapon %d) on %d", c.SkillName, c.Level, c.Job, c.Weapon, c.Target)
}

// kindOf maps rAthena's damage type and damage_lv to a battle.Kind.
func kindOf(d refDamage) battle.Kind {
	switch {
	case d.DmgLv == 1:
		return battle.LuckyDodge
	case d.DmgLv != 5:
		return battle.Miss
	case d.Type == 10 || d.Type == 13:
		return battle.Critical
	}
	return battle.Hit
}

// firstClassDamage are the damaging skills of the first classes and
// their quest skills, with Heal's damage to the undead and Ruwach's.
var firstClassDamage = map[string]bool{
	"SM_BASH": true, "SM_MAGNUM": true, "MG_NAPALMBEAT": true, "MG_SOULSTRIKE": true, "MG_COLDBOLT": true,
	"MG_FROSTDIVER": true, "MG_FIREBALL": true, "MG_FIREWALL": true, "MG_FIREBOLT": true, "MG_LIGHTNINGBOLT": true,
	"MG_THUNDERSTORM": true, "AC_DOUBLE": true, "AC_SHOWER": true, "AC_CHARGEARROW": true, "MC_MAMMONITE": true,
	"MC_CARTREVOLUTION": true, "TF_POISON": true, "TF_SPRINKLESAND": true, "TF_THROWSTONE": true,
	"AL_HOLYLIGHT": true, "AL_HEAL": true, "AL_RUWACH": true,
}

// damageGaps are ported skills whose recorded damage does not match yet,
// with the reason. Each job's test file adds its own in init.
var damageGaps = map[string]string{}

// rollGaps are cases whose forced minimum must match but whose seeded
// rolls are a statistical outlier of rAthena's recording, by the case's
// name, with the reason. Each job's test file adds its own in init.
var rollGaps = map[string]string{}

// goldenDamage reports whether a skill's cases must match: the first
// class skills, and every skill with Go behavior that has no gap.
func goldenDamage(name string, id int) bool {
	if firstClassDamage[name] {
		return true
	}
	_, gap := damageGaps[name]
	return Ported(id) && !gap
}

// TestSkillDamage checks every skill with Go behavior against
// skill_damage.json: rAthena's forced_min result exactly, and the seeded
// rolls' range and mean. Skills without behavior are counted only.
func TestSkillDamage(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			f := loadDamageFixture(t, mode)
			matched, total := 0, 0
			skillsSeen := map[string]bool{}
			skillsBad := map[string]bool{}
			left := map[string]bool{}
			for i := range f.cases {
				c := &f.cases[i]
				if !goldenDamage(c.SkillName, c.Skill) {
					left[c.SkillName] = true
					continue
				}
				total++
				skillsSeen[c.SkillName] = true
				def := f.db.Get(c.Skill)
				if def == nil {
					t.Fatalf("%s: skill missing", c.name())
				}
				src, err := f.attacker(t, c, def)
				if err != nil {
					t.Fatalf("%s: %v", c.name(), err)
				}
				target := f.mobs[c.Target]
				sk := def.Battle(src, c.Level, c.Flag, 1)
				got := attack(mode, def.Type, src, target, sk, forcedMin{})
				want := c.ForcedMin
				if got.Damage != want.Damage+want.Damage2 || got.Left != want.Damage2 || got.Kind != kindOf(want) ||
					got.Hits != want.Div || got.Flag != want.Flag {
					skillsBad[c.SkillName] = true
					t.Errorf("%s: got %+v, want damage %d+%d, kind %d, div %d, flag %d",
						c.name(), got, want.Damage, want.Damage2, kindOf(want), want.Div, want.Flag)
					continue
				}
				if _, skip := rollGaps[string(mode)+": "+c.name()]; skip {
					matched++
					continue
				}
				if msg := checkRolls(mode, src, target, sk, def.Type, f.rolls, c.Rolls); msg != "" {
					skillsBad[c.SkillName] = true
					t.Errorf("%s: %s", c.name(), msg)
					continue
				}
				matched++
			}
			t.Logf("%s: %d of %d cases match; %d of %d skills; %d recorded skills not ported: %v", mode, matched, total,
				len(skillsSeen)-len(skillsBad), len(skillsSeen), len(left), slices.Sorted(maps.Keys(left)))
		})
	}
}

// attack is battle_calc_attack for a skill in a mode.
func attack(mode reference.Mode, typ int, src, target *battle.Status, sk *battle.Skill, r battle.Rand) battle.Result {
	if mode == reference.PreRenewal {
		return battle.SkillAttackPreRenewal(typ, src, target, sk, r)
	}
	return battle.SkillAttack(typ, src, target, sk, r)
}

// checkRolls draws many attacks and compares their hit count, range and
// mean with rAthena's seeded rolls. The draws differ, so only statistics
// compare.
func checkRolls(mode reference.Mode, src, target *battle.Status, sk *battle.Skill, typ, n int, want refRolls) string {
	const attacks = 4000
	r := rand.New(rand.NewPCG(1, 2))
	hits, lo, hi, total := 0, int64(-1), int64(0), int64(0)
	// The mean compares the hits rAthena's rolls could have: only
	// non-critical ones when its rolls had no critical hit. sum and sq
	// are their sum and sum of squares.
	means, sum, sq := 0, 0.0, 0.0
	for range attacks {
		res := attack(mode, typ, src, target, sk, r)
		if res.Kind != battle.Hit && res.Kind != battle.Critical {
			continue
		}
		hits++
		if lo < 0 || res.Damage < lo {
			lo = res.Damage
		}
		hi = max(hi, res.Damage)
		total += res.Damage
		if want.Crits == 0 && res.Kind == battle.Critical {
			continue
		}
		means++
		sum += float64(res.Damage)
		sq += float64(res.Damage) * float64(res.Damage)
	}
	if want.Hits == 0 {
		// rAthena's rolls never hit. At the minimum hit rate of 5%, 50
		// rolls all miss 8% of the time, so allow a rate up to 10%.
		if hits > attacks/10 {
			return fmt.Sprintf("hit %d of %d, rAthena never hit", hits, attacks)
		}
		return ""
	}
	if hits == 0 {
		return "never hit"
	}
	if want.Min < lo || want.Max > hi {
		return fmt.Sprintf("damage range %d..%d does not contain recorded %d..%d", lo, hi, want.Min, want.Max)
	}
	if means == 0 {
		return "never hit without a critical hit"
	}
	mean := sum / float64(means)
	wantMean := float64(want.Total) / float64(want.Hits)
	// The recorded mean of few hits varies by about the spread of the
	// damage over the square root of their count. The spread comes from
	// the recorded range, or from these draws when the recording has too
	// few hits to show it.
	sd := math.Sqrt(max(0, sq/float64(means)-mean*mean))
	tol := max(1, float64(want.Max-want.Min)/math.Sqrt(float64(want.Hits))*2, 3*sd/math.Sqrt(float64(want.Hits)))
	if math.Abs(mean-wantMean) > tol {
		return fmt.Sprintf("mean damage %.1f, recorded %.1f (tolerance %.1f)", mean, wantMean, tol)
	}
	rate := float64(hits) / attacks
	wantRate := float64(want.Hits) / float64(n)
	// rAthena rolls n times, so its rate is off from the true rate by
	// up to three standard deviations of a binomial of n draws.
	rateTol := max(0.15, 3*math.Sqrt(rate*(1-rate)/float64(n)))
	if math.Abs(rate-wantRate) > rateTol {
		return fmt.Sprintf("hit rate %.2f, recorded %.2f", rate, wantRate)
	}
	return ""
}
