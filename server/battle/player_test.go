package battle

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

type pcCase struct {
	Job        int       `json:"job"`
	Build      string    `json:"build"`
	WeaponType int       `json:"weapon_type"`
	Weapon     int       `json:"weapon"`
	Target     int       `json:"target"`
	Attacker   Status    `json:"attacker_status"`
	ForcedMin  refDamage `json:"forced_min"`
	Rolls      refRolls  `json:"rolls"`
}

func (c *pcCase) name() string {
	return fmt.Sprintf("job %d %s, weapon %d (type %d) on %d", c.Job, c.Build, c.Weapon, c.WeaponType, c.Target)
}

type pcDamage struct {
	RollsPerCase int      `json:"rolls_per_case"`
	Cases        []pcCase `json:"cases"`
}

// pcWeapons is testdata/pc_weapons.json.
type pcWeapons struct {
	Weapons map[string]struct {
		EAtk          int         `json:"eatk"`
		AtkRate       int         `json:"atk_rate"`
		AddRace       map[int]int `json:"add_race"`
		IgnoreDefRace map[int]int `json:"ignore_def_race"`
	} `json:"weapons"`
	TwoHanded []int `json:"two_handed"`
	Ammo      map[string]struct {
		Atk int `json:"atk"`
		Ele int `json:"ele"`
	} `json:"ammo"`
}

// The recorder's sample players are base level 99, and each gets basic
// ammunition for its bow or gun.
const (
	pcLevel     = 99
	ammoArrow   = 1750
	ammoBullet  = 13200
	ammoGrenade = 13203
)

// thiefJobs are the sample jobs whose class is a thief class.
var thiefJobs = []int{6, 12, 17, 4013, 4018}

type pcFixture struct {
	mobs    map[int]*Status
	ref     *pcDamage
	skills  map[string]map[int]int
	weapons pcWeapons
}

func loadPCFixture(t testing.TB) *pcFixture {
	t.Helper()
	f := &pcFixture{mobs: loadMobs(t), ref: &pcDamage{}}
	if err := reference.Load(reference.Renewal, "pc_damage", f.ref); err != nil {
		t.Fatal(err)
	}
	if len(f.ref.Cases) == 0 {
		t.Fatal("pc_damage.json has no cases")
	}

	// pc_status.json lists the skills each sample player learns with
	// all_skills, which is what the damage recorder does too.
	var status struct {
		Players []struct {
			Job       int      `json:"job"`
			BaseLevel int      `json:"base_level"`
			Build     string   `json:"build"`
			AllSkills bool     `json:"all_skills"`
			Skills    [][2]int `json:"skills"`
		} `json:"players"`
	}
	if err := reference.Load(reference.Renewal, "pc_status", &status); err != nil {
		t.Fatal(err)
	}
	f.skills = make(map[string]map[int]int)
	for _, p := range status.Players {
		if p.BaseLevel != pcLevel || !p.AllSkills {
			continue
		}
		skills := make(map[int]int, len(p.Skills))
		for _, s := range p.Skills {
			skills[s[0]] = s[1]
		}
		f.skills[strconv.Itoa(p.Job)+" "+p.Build] = skills
	}

	b, err := os.ReadFile("testdata/pc_weapons.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &f.weapons); err != nil {
		t.Fatal(err)
	}
	return f
}

// attacker builds the recorded player of a case.
func (f *pcFixture) attacker(t testing.TB, c *pcCase) *Status {
	t.Helper()
	skills, ok := f.skills[strconv.Itoa(c.Job)+" "+c.Build]
	if !ok {
		t.Fatalf("%s: pc_status.json has no level %d %s player with all skills", c.name(), pcLevel, c.Build)
	}
	st := c.Attacker
	st.Level = pcLevel
	p := &Player{
		Weapon:      c.WeaponType,
		RightWeapon: c.WeaponType,
		LeftWeapon:  WeaponFist,
		Skills:      skills,
		// The recorder calls battle_calc_attack directly. rAthena sets
		// state.arrow_atk only in battle_weapon_attack, so the recorded
		// attacks never use ammunition, even with a bow or a gun.
		ArrowAttack: false,
	}
	if slices.Contains(f.weapons.TwoHanded, c.Weapon) {
		p.LeftWeapon = c.WeaponType
	}
	if slices.Contains(thiefJobs, c.Job) {
		p.DualWield = DualWieldThief
	}
	if w, ok := f.weapons.Weapons[strconv.Itoa(c.Weapon)]; ok {
		st.EAtk = w.EAtk
		p.AtkRate = w.AtkRate
		p.AddRace = w.AddRace
		p.IgnoreDefRace = w.IgnoreDefRace
	}
	ammo := 0
	switch c.WeaponType {
	case WeaponBow:
		ammo = ammoArrow
	case WeaponGrenade:
		ammo = ammoGrenade
	case WeaponRevolver, WeaponRifle, WeaponGatling, WeaponShotgun:
		ammo = ammoBullet
	}
	if ammo != 0 {
		a, ok := f.weapons.Ammo[strconv.Itoa(ammo)]
		if !ok {
			t.Fatalf("ammunition %d missing from testdata/pc_weapons.json", ammo)
		}
		p.AmmoAtk, p.AmmoEle = a.Atk, a.Ele
	}
	st.Player = p
	return &st
}

func (f *pcFixture) target(t testing.TB, c *pcCase) *Status {
	t.Helper()
	dst := f.mobs[c.Target]
	if dst == nil {
		t.Fatalf("%s: monster missing from mob_status.json", c.name())
	}
	return dst
}

// TestPCDamageForcedMin checks every player case against rAthena's result
// with rnd_force_min.
func TestPCDamageForcedMin(t *testing.T) {
	f := loadPCFixture(t)
	matched := 0
	for i := range f.ref.Cases {
		c := &f.ref.Cases[i]
		got := Attack(f.attacker(t, c), f.target(t, c), forcedMin{})
		want := c.ForcedMin
		if got.Damage == want.Damage+want.Damage2 && got.Left == want.Damage2 && got.Kind == want.kind() &&
			got.Hits == want.Div && got.Flag == want.Flag {
			matched++
			continue
		}
		t.Errorf("%s: got %+v, want damage %d+%d, kind %d, div %d, flag %d",
			c.name(), got, want.Damage, want.Damage2, want.kind(), want.Div, want.Flag)
	}
	t.Logf("%d of %d forced_min cases match", matched, len(f.ref.Cases))
}

// pcSample is sample, counting critical hits as the recorder does: only
// DMG_CRITICAL counts, not a double attack that is also critical.
func pcSample(t *testing.T, src, dst *Status, r Rand, attacks int) rollStats {
	t.Helper()
	s := rollStats{attacks: attacks, min: -1}
	for range attacks {
		res := Attack(src, dst, r)
		if res.Damage < 0 || res.Left < 0 {
			t.Fatalf("negative damage %+v", res)
		}
		if res.Kind != Hit && res.Kind != Critical {
			continue
		}
		s.hits++
		if res.Kind == Critical && res.Hits == 1 {
			s.crits++
		}
		if s.min < 0 || res.Damage < s.min {
			s.min = res.Damage
		}
		s.max = max(s.max, res.Damage)
		d := float64(res.Damage)
		s.sum += d
		s.sumSq += d * d
	}
	return s
}

// TestPCDamageRolls compares many seeded attacks per case with the
// recorded statistics, with the same checks as TestMobDamageRolls, and
// also checks the critical rate. The recorded rolls count a double attack
// as one hit with the damage of both hits.
func TestPCDamageRolls(t *testing.T) {
	f := loadPCFixture(t)
	// Each job runs as a parallel subtest, because the race detector
	// makes the player formula slow.
	byJob := make(map[int][]int)
	var jobs []int
	for i, c := range f.ref.Cases {
		if byJob[c.Job] == nil {
			jobs = append(jobs, c.Job)
		}
		byJob[c.Job] = append(byJob[c.Job], i)
	}
	for _, job := range jobs {
		t.Run("job "+strconv.Itoa(job), func(t *testing.T) {
			t.Parallel()
			for _, i := range byJob[job] {
				checkPCRolls(t, f, i)
			}
		})
	}
}

// checkPCRolls runs the checks of TestPCDamageRolls on one case.
func checkPCRolls(t *testing.T, f *pcFixture, i int) {
	t.Helper()
	// There are ten times as many cases as monster pairs, so each case
	// gets fewer attacks than in TestMobDamageRolls.
	const (
		rateAttacks   = 2000
		damageAttacks = 4000
	)
	recorded := float64(f.ref.RollsPerCase)
	c := &f.ref.Cases[i]
	src, dst := f.attacker(t, c), f.target(t, c)
	name := c.name()
	rec := c.Rolls
	r := rand.New(rand.NewPCG(uint64(i+1), 0x70635F64616D)) // #nosec G404 -- a fixed seed for a repeatable test.

	s := pcSample(t, src, dst, r, rateAttacks)
	rate := float64(s.hits) / rateAttacks
	recRate := float64(rec.Hits) / recorded
	if tol := 4*math.Sqrt(rate*(1-rate)/recorded) + 0.02; math.Abs(rate-recRate) > tol {
		t.Errorf("%s: hit rate %.3f, recorded %.3f (tolerance %.3f)", name, rate, recRate, tol)
	}
	// The critical rate gets the same tolerance as the hit rate. Most
	// cases have a critical rate of a few percent, where 200 rolls vary a
	// lot, so the 2 points matter. One recorded case (job 4008 with weapon
	// 1100 on 1026) has 9 criticals where 2.2 are expected, which is
	// outside 4 standard errors alone.
	critRate := float64(s.crits) / rateAttacks
	recCrit := float64(rec.Crits) / recorded
	if tol := 4*math.Sqrt(critRate*(1-critRate)/recorded) + 0.02; math.Abs(critRate-recCrit) > tol {
		t.Errorf("%s: critical rate %.3f, recorded %.3f (tolerance %.3f)", name, critRate, recCrit, tol)
	}
	if rec.Hits == 0 {
		return
	}

	sure := *dst
	sure.Flee, sure.Flee2 = -1<<20, 0
	s = pcSample(t, src, &sure, r, damageAttacks)
	if s.min > rec.Min || s.max < rec.Max {
		t.Errorf("%s: damage range %d..%d does not contain recorded %d..%d", name, s.min, s.max, rec.Min, rec.Max)
	}
	if rec.Hits >= 20 {
		mean := s.sum / float64(s.hits)
		sd := math.Sqrt(max(s.sumSq/float64(s.hits)-mean*mean, 0))
		recMean := float64(rec.Total) / float64(rec.Hits)
		if tol := 5*sd/math.Sqrt(float64(rec.Hits)) + 1; math.Abs(mean-recMean) > tol {
			t.Errorf("%s: mean damage %.1f, recorded %.1f (tolerance %.1f)", name, mean, recMean, tol)
		}
	}
}

// seqRand returns its rolls in order, each capped to its range, then 0.
type seqRand struct{ rolls []int }

func (s *seqRand) IntN(n int) int {
	if len(s.rolls) == 0 {
		return 0
	}
	v := s.rolls[0]
	s.rolls = s.rolls[1:]
	return min(v, n-1)
}

// TestPlayerAttackTable checks hand-made cases for each player rule,
// including rules the recorded cases do not reach: ammunition, dual
// wielding and the luk factor against a player.
func TestPlayerAttackTable(t *testing.T) {
	const short = bfWeapon | bfNormal | bfShort
	// Weapon ATK 100, level 1, STR 100: the roll is 145..155 before the
	// size penalty. Status ATK is 50 * 2.
	base := func() Status {
		return Status{
			Level: 99, Hit: 300, Str: 100, Dex: 50, BAtk: 50, WAtk: 100,
			RHW:    Weapon{Atk: 100, Wlv: 1, Range: 1},
			Player: &Player{Weapon: WeaponMace, RightWeapon: WeaponMace},
		}
	}
	medium := Status{Size: SizeMedium, EleLv: 1}
	tests := []struct {
		name string
		src  func(*Status)
		dst  func(*Status)
		r    Rand
		want Result
	}{
		{"plain hit", nil, nil, forcedMin{}, Result{Kind: Hit, Damage: 245, Hits: 1, Flag: short}},
		// 50 * 2 + 155, then * 1.4.
		{"critical", func(s *Status) { s.Cri = 1000 }, nil, forcedMin{}, Result{Kind: Critical, Damage: 357, Hits: 1, Flag: short}},
		{"size penalty", nil, func(d *Status) { d.Size = SizeSmall }, forcedMin{}, Result{Kind: Hit, Damage: 208, Hits: 1, Flag: short}},
		// A bow uses DEX: 100 - 5 + 25.
		{"dex weapon", func(s *Status) { s.Player.Weapon, s.Player.RightWeapon = WeaponBow, WeaponBow }, nil, forcedMin{},
			Result{Kind: Hit, Damage: 220, Hits: 1, Flag: short}},
		// Fire ammunition against Water 1 (90%): weapon ATK 120 -> 108,
		// ammunition ATK 30 -> 27, status ATK stays neutral.
		{"ammunition", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon = WeaponBow, WeaponBow
			s.Player.ArrowAttack, s.Player.AmmoAtk, s.Player.AmmoEle = true, 30, Fire
		}, func(d *Status) { d.DefEle = Water }, forcedMin{}, Result{Kind: Hit, Damage: 235, Hits: 1, Flag: bfWeapon | bfNormal | bfLong}},
		{"equipment ATK", func(s *Status) { s.EAtk = 40 }, nil, forcedMin{}, Result{Kind: Hit, Damage: 285, Hits: 1, Flag: short}},
		// 145 * 50%.
		{"ATK rate", func(s *Status) { s.Player.AtkRate = 50 }, nil, forcedMin{}, Result{Kind: Hit, Damage: 317, Hits: 1, Flag: short}},
		// 145 * 120%.
		{"race bonus", func(s *Status) { s.Player.AddRace = map[int]int{RaceDemiHuman: 20} },
			func(d *Status) { d.Race = RaceDemiHuman }, forcedMin{}, Result{Kind: Hit, Damage: 274, Hits: 1, Flag: short}},
		// DEF 100 and 20 become 50 and 10: 245 * 4050 / 4500 - 10.
		{"ignore DEF", func(s *Status) { s.Player.IgnoreDefRace = map[int]int{RaceAll: 50} },
			func(d *Status) { d.Def, d.Def2 = 100, 20 }, forcedMin{}, Result{Kind: Hit, Damage: 210, Hits: 1, Flag: short}},
		{"Demon Bane", func(s *Status) { s.Player.Skills = map[int]int{skillDemonBane: 10} },
			func(d *Status) { d.Race = RaceDemon }, forcedMin{}, Result{Kind: Hit, Damage: 324, Hits: 1, Flag: short}},
		{"Taekwon Mission", func(s *Status) { s.Player.Skills = map[int]int{skillTKPower: 5} }, nil, forcedMin{},
			Result{Kind: Hit, Damage: 490, Hits: 1, Flag: short}},
		// Dagger against medium is 75%: (100 + 108) * 2.
		{"double attack", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon = WeaponDagger, WeaponDagger
			s.Player.Skills = map[int]int{skillDoubleAttack: 10}
		}, nil, forcedMin{}, Result{Kind: Hit, Damage: 416, Hits: 2, Flag: short}},
		{"chain action", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon, s.Player.LeftWeapon = WeaponRevolver, WeaponRevolver, WeaponRevolver
			s.Player.Skills = map[int]int{skillChainAction: 10}
		}, nil, forcedMin{}, Result{Kind: Hit, Damage: 440, Hits: 2, Flag: short}},
		// A miss after a double attack shows as one hit.
		{"double attack miss", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon = WeaponDagger, WeaponDagger
			s.Player.Skills = map[int]int{skillDoubleAttack: 10}
			s.Hit = 0
		}, func(d *Status) { d.Flee = 1000 }, &seqRand{rolls: []int{0, 99}}, Result{Kind: Miss, Hits: 1, Flag: short}},
		// 100 + 145 + 30 mastery, and a second hit of 21%.
		{"katar", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon, s.Player.LeftWeapon = WeaponKatar, WeaponKatar, WeaponKatar
			s.Player.Skills = map[int]int{skillDoubleAttack: 10, skillKatarMastery: 10}
		}, nil, forcedMin{}, Result{Kind: Hit, Damage: 332, Left: 57, Hits: 1, Flag: short}},
		{"katar on a plant", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon, s.Player.LeftWeapon = WeaponKatar, WeaponKatar, WeaponKatar
		}, func(d *Status) { d.Mode = ModeIgnoreMelee }, forcedMin{}, Result{Kind: Hit, Damage: 1, Hits: 1, Flag: short}},
		// Two daggers: 208 at 100% on the right, and 50 + 108 at 80% on
		// the left. Left status ATK is not doubled.
		{"dual wield", func(s *Status) {
			s.Player.Weapon, s.Player.RightWeapon, s.Player.LeftWeapon = 25, WeaponDagger, WeaponDagger // W_DOUBLE_DD
			s.Player.DualWield = DualWieldThief
			s.Player.Skills = map[int]int{skillRightHandMastery: 5, skillLeftHandMastery: 5}
			s.WAtk2, s.LHW = 100, Weapon{Atk: 100, Wlv: 1, Range: 1}
		}, nil, forcedMin{}, Result{Kind: Hit, Damage: 334, Left: 126, Hits: 1, Flag: short}},
	}
	for _, tt := range tests {
		src, dst := base(), medium
		if tt.src != nil {
			tt.src(&src)
		}
		if tt.dst != nil {
			tt.dst(&dst)
		}
		if got := Attack(&src, &dst, tt.r); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// TestCriticalLukFactor checks that a monster's critical rate drops by 3
// per luk of a player target, and by 2 per luk of a monster.
func TestCriticalLukFactor(t *testing.T) {
	src := &Status{Hit: 300, Cri: 30, RHW: Weapon{Atk: 100, Atk2: 100, Range: 1}}
	if got := Attack(src, &Status{Luk: 10}, forcedMin{}); got.Kind != Critical {
		t.Errorf("against a monster: got %+v, want a critical hit", got)
	}
	if got := Attack(src, &Status{Luk: 10, Player: &Player{}}, forcedMin{}); got.Kind != Hit {
		t.Errorf("against a player: got %+v, want a hit", got)
	}
}

func TestSizeFix(t *testing.T) {
	tests := []struct{ weapon, size, want int }{
		{WeaponFist, SizeLarge, 100},
		{WeaponDagger, SizeLarge, 50},
		{WeaponKnuckle, SizeLarge, 75},
		{WeaponWhip, SizeSmall, 75},
		{Weapon1HAxe, SizeSmall, 50},
		{-1, SizeSmall, 100},
		{WeaponDagger, 3, 100},
	}
	for _, tt := range tests {
		if got := SizeFix(tt.weapon, tt.size); got != tt.want {
			t.Errorf("SizeFix(%d, %d) = %d, want %d", tt.weapon, tt.size, got, tt.want)
		}
	}
}

// FuzzPlayerAttack checks that a player's attack never panics and never
// returns negative damage.
func FuzzPlayerAttack(f *testing.F) {
	f.Add(uint64(1), uint8(WeaponDagger), uint8(WeaponFist), int16(100), int16(10), uint16(120), uint8(4), int16(50), int16(0), uint8(10), false)
	f.Add(uint64(2), uint8(WeaponKatar), uint8(WeaponKatar), int16(-399), int16(-50), uint16(65535), uint8(0), int16(-100), int16(1000), uint8(0), true)
	f.Add(uint64(3), uint8(WeaponFist), uint8(WeaponDagger), int16(32767), int16(32767), uint16(0), uint8(255), int16(32767), int16(-32768), uint8(255), false)
	f.Fuzz(func(t *testing.T, seed uint64, right, left uint8, def, def2 int16, atk uint16, wlv uint8, stat, rate int16, lv uint8, arrow bool) {
		p := &Player{
			Weapon: int(right), RightWeapon: int(right), LeftWeapon: int(left),
			Skills:      map[int]int{skillDoubleAttack: int(lv), skillChainAction: int(lv), skillKatarMastery: int(lv), skillTKPower: int(lv)},
			ArrowAttack: arrow, AmmoAtk: int(stat), AmmoEle: int(lv) % elementCount,
			AtkRate: int(rate), AddRace: map[int]int{RaceAll: int(rate)}, IgnoreDefRace: map[int]int{RaceAll: int(rate)},
			DualWield: DualWield(lv % 3),
		}
		src := &Status{
			Level: 99, Hit: int(def2), Cri: int(rate), Str: int(stat), Dex: int(stat), BAtk: int(stat), EAtk: int(stat),
			WAtk: int(atk), WAtk2: int(atk), RHW: Weapon{Atk: int(atk), Wlv: int(wlv)}, LHW: Weapon{Atk: int(atk) % 2, Wlv: int(wlv)},
			Player: p,
		}
		dst := &Status{Def: int(def), Def2: int(def2), Flee: int(def), Size: int(wlv) % 4, Race: int(lv) % 13, DefEle: int(lv) % 11, EleLv: int(wlv)%4 + 1}
		r := rand.New(rand.NewPCG(seed, seed)) // #nosec G404 -- test input, not security.
		for range 20 {
			if res := Attack(src, dst, r); res.Damage < 0 || res.Left < 0 {
				t.Fatalf("negative damage: %+v", res)
			}
		}
		if res := Attack(src, dst, forcedMin{}); res.Damage < 0 || res.Left < 0 {
			t.Fatalf("negative damage with forced minimum: %+v", res)
		}
	})
}
