package skill

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
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
	WeaponType int       `json:"weapon_type"`
	Weapon     uint32    `json:"weapon"`
	Target     int       `json:"target"`
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
	// skills are the skills each sample player learns, by "job build",
	// and jobLevels its job level at base level 99.
	skills    map[string]map[int]int
	jobLevels map[int]int
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
	f := &damageFixture{mode: mode, db: loadDB(t, mode), skills: map[string]map[int]int{}, jobLevels: map[int]int{}}
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
		f.skills[fmt.Sprintf("%d %s", p.Job, p.Build)] = skills
		f.jobLevels[p.Job] = p.JobLevel
	}

	var ref struct {
		RollsPerCase int         `json:"rolls_per_case"`
		Cases        []skillCase `json:"cases"`
	}
	if err := reference.Load(mode, "skill_damage", &ref); err != nil {
		t.Fatal(err)
	}
	f.cases, f.rolls = ref.Cases, ref.RollsPerCase
	return f
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

// attacker is the recorder's skill user: a level 99 male player of the
// case's job with every tree skill and the case's weapon, with basic
// ammunition for a bow or a gun.
func (f *damageFixture) attacker(t testing.TB, c *skillCase, def *Def) (*battle.Status, error) {
	t.Helper()
	b := build(def, c.WeaponType)
	skills, ok := f.skills[fmt.Sprintf("%d %s", c.Job, b)]
	if !ok {
		return nil, fmt.Errorf("pc_status.json has no level 99 %s player of job %d with all skills", b, c.Job)
	}
	ch := status.Character{Job: c.Job, Male: true, BaseLevel: 99, JobLevel: f.jobLevels[c.Job], Stats: builds[b], Skills: skills}
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
		if a := basicAmmo[c.WeaponType]; a != 0 {
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
		st.Player.ArrowAttack = def.Requires.Ammo != 0 && ammo != 0
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

// TestSkillDamage checks the first class skills against skill_damage.json:
// rAthena's forced_min result exactly, and the seeded rolls' range and
// mean.
func TestSkillDamage(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			f := loadDamageFixture(t, mode)
			matched, total := 0, 0
			skillsSeen := map[string]bool{}
			skillsBad := map[string]bool{}
			for i := range f.cases {
				c := &f.cases[i]
				if !firstClassDamage[c.SkillName] {
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
				if msg := checkRolls(mode, src, target, sk, def.Type, f.rolls, c.Rolls); msg != "" {
					skillsBad[c.SkillName] = true
					t.Errorf("%s: %s", c.name(), msg)
					continue
				}
				matched++
			}
			t.Logf("%s: %d of %d first class skill cases match; %d of %d skills", mode, matched, total,
				len(skillsSeen)-len(skillsBad), len(skillsSeen))
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
	mean := float64(total) / float64(hits)
	wantMean := float64(want.Total) / float64(want.Hits)
	tol := max(1, float64(want.Max-want.Min)/math.Sqrt(float64(want.Hits))*2)
	if math.Abs(mean-wantMean) > tol {
		return fmt.Sprintf("mean damage %.1f, recorded %.1f (tolerance %.1f)", mean, wantMean, tol)
	}
	rate := float64(hits) / attacks
	wantRate := float64(want.Hits) / float64(n)
	if math.Abs(rate-wantRate) > 0.15 {
		return fmt.Sprintf("hit rate %.2f, recorded %.2f", rate, wantRate)
	}
	return ""
}
