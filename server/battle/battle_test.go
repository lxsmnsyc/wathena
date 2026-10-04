package battle

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
)

// forcedMin is rAthena's rnd_force_min: rnd() returns 0, so every roll is
// lowest and every "rnd() % n < chance" check succeeds.
type forcedMin struct{}

func (forcedMin) IntN(int) int { return 0 }

type refDamage struct {
	Damage  int64 `json:"damage"`
	Damage2 int64 `json:"damage2"`
	Type    int   `json:"type"`
	Div     int   `json:"div"`
	DmgLv   int   `json:"dmg_lv"`
	Flag    int   `json:"flag"`
}

type refRolls struct {
	Hits  int   `json:"hits"`
	Crits int   `json:"crits"`
	Min   int64 `json:"min"`
	Max   int64 `json:"max"`
	Total int64 `json:"total"`
}

type mobDamage struct {
	RollsPerPair int `json:"rolls_per_pair"`
	Pairs        []struct {
		Attacker  int       `json:"attacker"`
		Target    int       `json:"target"`
		ForcedMin refDamage `json:"forced_min"`
		Rolls     refRolls  `json:"rolls"`
	} `json:"pairs"`
}

// kind maps rAthena's damage type and damage_lv to a Kind.
func (d refDamage) kind() Kind {
	switch {
	case damageLevel(d.DmgLv) == atkLucky:
		return LuckyDodge
	case damageLevel(d.DmgLv) != atkDef:
		return Miss
	case damageType(d.Type) == dmgCritical || damageType(d.Type) == dmgMultiHitCritical:
		return Critical
	}
	return Hit
}

// loadMobs reads every monster's status from a mode's mob_status.json,
// with its level and its DamageTaken.
func loadMobs(t testing.TB, mode reference.Mode) map[int]*Status {
	t.Helper()
	var ref struct {
		Mobs []struct {
			ID     int    `json:"id"`
			Level  int    `json:"level"`
			Status Status `json:"status"`
		} `json:"mobs"`
	}
	if err := reference.Load(mode, "mob_status", &ref); err != nil {
		t.Fatal(err)
	}
	// testdata/damage_taken.json holds DamageTaken from rAthena's
	// db/re/mob_db.yml (GPL-3.0), which mob_status.json does not record.
	// No monster in db/pre-re/mob_db.yml sets DamageTaken.
	var taken struct {
		DamageTaken map[string]int `json:"damage_taken"`
	}
	if mode == reference.Renewal {
		b, err := os.ReadFile("testdata/damage_taken.json")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &taken); err != nil {
			t.Fatal(err)
		}
	}
	mobs := make(map[int]*Status, len(ref.Mobs))
	for _, m := range ref.Mobs {
		st := m.Status
		st.Level = m.Level
		st.DamageTaken = taken.DamageTaken[strconv.Itoa(m.ID)]
		mobs[m.ID] = &st
	}
	return mobs
}

func loadMobDamage(t testing.TB, mode reference.Mode) *mobDamage {
	t.Helper()
	var ref mobDamage
	if err := reference.Load(mode, "mob_damage", &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Pairs) == 0 {
		t.Fatal("mob_damage.json has no pairs")
	}
	return &ref
}

func TestAttributeMatchesReference(t *testing.T) {
	var ref reference.Elements
	if err := reference.Load(reference.Renewal, "elements", &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Levels) != maxElementLevel {
		t.Fatalf("got %d levels, want %d", len(ref.Levels), maxElementLevel)
	}
	for _, lv := range ref.Levels {
		for atk, row := range lv.Table {
			for def, want := range row {
				if got := Attribute(lv.Level, atk, def); got != want {
					t.Errorf("level %d, %d against %d: got %d, want %d", lv.Level, atk, def, got, want)
				}
			}
		}
	}
}

func TestAttrFix(t *testing.T) {
	tests := []struct {
		damage             int64
		atk, def, lv, flag int
		want               int64
	}{
		{100, Neutral, Neutral, 1, 0, 100},
		{100, Neutral, Ghost, 1, 0, 90},
		{101, Neutral, Ghost, 1, 0, 91},
		{100, Fire, Water, 1, 0, 90},
		{99, Water, Fire, 1, 0, 148},
		{100, Holy, Holy, 1, 0, 0},
		{100, Neutral, Ghost, 4, 0, 0},
		{100, -1, Neutral, 1, 0, 100},
		{100, Neutral, Neutral, 5, 0, 100},
	}
	for _, tt := range tests {
		if got := AttrFix(tt.damage, tt.atk, tt.def, tt.lv, tt.flag); got != tt.want {
			t.Errorf("AttrFix(%d, %d, %d, %d) = %d, want %d", tt.damage, tt.atk, tt.def, tt.lv, got, tt.want)
		}
	}
}

// TestMobDamageForcedMin checks every pair against rAthena's result with
// rnd_force_min.
func TestMobDamageForcedMin(t *testing.T) {
	checkMobDamageForcedMin(t, reference.Renewal, Attack)
}

// attackFunc is Attack or AttackPreRenewal.
type attackFunc func(attacker, target *Status, r Rand) Result

// checkMobDamageForcedMin checks every pair of a mode's mob_damage.json
// against rAthena's result with rnd_force_min.
func checkMobDamageForcedMin(t *testing.T, mode reference.Mode, attack attackFunc) {
	t.Helper()
	mobs := loadMobs(t, mode)
	ref := loadMobDamage(t, mode)
	matched := 0
	for _, p := range ref.Pairs {
		src, dst := mobs[p.Attacker], mobs[p.Target]
		if src == nil || dst == nil {
			t.Errorf("%d on %d: monster missing from mob_status.json", p.Attacker, p.Target)
			continue
		}
		got := attack(src, dst, forcedMin{})
		want := p.ForcedMin
		if got.Damage == want.Damage+want.Damage2 && got.Left == want.Damage2 && got.Kind == want.kind() &&
			got.Hits == want.Div && got.Flag == want.Flag {
			matched++
			continue
		}
		t.Errorf("%d on %d: got %+v, want damage %d+%d, kind %d, div %d, flag %d",
			p.Attacker, p.Target, got, want.Damage, want.Damage2, want.kind(), want.Div, want.Flag)
	}
	t.Logf("%d of %d forced_min pairs match", matched, len(ref.Pairs))
}

// rollStats are statistics of many attacks, as the recorder keeps them.
type rollStats struct {
	attacks, hits, crits int
	min, max             int64
	sum, sumSq           float64
}

func sample(t *testing.T, attack attackFunc, src, dst *Status, r Rand, attacks int) rollStats {
	t.Helper()
	s := rollStats{attacks: attacks, min: -1}
	for range attacks {
		res := attack(src, dst, r)
		if res.Damage < 0 {
			t.Fatalf("negative damage %d", res.Damage)
		}
		if res.Kind != Hit && res.Kind != Critical {
			continue
		}
		s.hits++
		if res.Kind == Critical {
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

// TestMobDamageRolls compares many seeded attacks per pair with the
// recorded statistics. rAthena's exact rolls cannot be reproduced, so it
// checks that:
//   - the hit rate over 4000 attacks is within 4 standard errors of the
//     recorded rate, plus 2 points;
//   - critical hits appear only when the recorded rolls had them;
//   - over 20000 attacks that always hit, the damage range contains the
//     recorded minimum and maximum;
//   - the mean damage of those attacks is within 5 standard errors of
//     the recorded mean, plus 1.
//
// The damage is sampled against a copy of the target that cannot dodge,
// so pairs with a low hit rate still get enough hits. Dodging does not
// change the damage roll. The seed is fixed, so the result does not change
// between runs.
func TestMobDamageRolls(t *testing.T) {
	testrace.SkipUnderRace(t, "it compares recorded reference cases on one goroutine")
	checkMobDamageRolls(t, reference.Renewal, Attack)
}

// checkMobDamageRolls runs the checks of TestMobDamageRolls on a mode's
// mob_damage.json. In pre-renewal, the range check samples another 20000
// attacks with cornerRand, and the mean check keeps the uniform sample.
func checkMobDamageRolls(t *testing.T, mode reference.Mode, attack attackFunc) {
	t.Helper()
	const (
		rateAttacks   = 4000
		damageAttacks = 20000
	)
	mobs := loadMobs(t, mode)
	ref := loadMobDamage(t, mode)
	recorded := float64(ref.RollsPerPair)
	for i, p := range ref.Pairs {
		src, dst := mobs[p.Attacker], mobs[p.Target]
		if src == nil || dst == nil {
			t.Fatalf("%d on %d: monster missing from mob_status.json", p.Attacker, p.Target)
		}
		name := strconv.Itoa(p.Attacker) + " on " + strconv.Itoa(p.Target)
		rec := p.Rolls
		r := rand.New(rand.NewPCG(uint64(i+1), 0x77617468656E61)) // #nosec G404 -- a fixed seed for a repeatable test.

		s := sample(t, attack, src, dst, r, rateAttacks)
		rate := float64(s.hits) / rateAttacks
		recRate := float64(rec.Hits) / recorded
		if tol := 4*math.Sqrt(rate*(1-rate)/recorded) + 0.02; math.Abs(rate-recRate) > tol {
			t.Errorf("%s: hit rate %.3f, recorded %.3f (tolerance %.3f)", name, rate, recRate, tol)
		}
		if (s.crits > 0) != (rec.Crits > 0) {
			t.Errorf("%s: %d critical hits, recorded %d", name, s.crits, rec.Crits)
		}
		if rec.Hits == 0 {
			continue
		}

		sure := *dst
		sure.Flee, sure.Flee2 = -1<<20, 0
		s = sample(t, attack, src, &sure, r, damageAttacks)
		span := s
		if mode == reference.PreRenewal {
			span = sample(t, attack, src, &sure, cornerRand{r}, damageAttacks)
		}
		if span.min > rec.Min || span.max < rec.Max {
			t.Errorf("%s: damage range %d..%d does not contain recorded %d..%d", name, span.min, span.max, rec.Min, rec.Max)
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
}

// cornerRand returns 0 for a quarter of its draws, n - 1 for another
// quarter, and a uniform draw otherwise. rAthena's rnd() % n can return
// each of these values, so every result is one rAthena can produce. The
// pre-renewal range checks use it: the lowest and highest damage need
// both the damage roll and the random soft DEF at an end of their range,
// which a uniform generator rarely draws together.
type cornerRand struct{ r *rand.Rand }

func (c cornerRand) IntN(n int) int {
	switch c.r.IntN(4) {
	case 0:
		return 0
	case 1:
		return n - 1
	}
	return c.r.IntN(n)
}

// TestAttackTable checks hand-made cases for each step of the formula.
func TestAttackTable(t *testing.T) {
	base := Status{Hit: 300, RHW: Weapon{Atk: 100, Atk2: 100, Range: 1}, BAtk: 50}
	tests := []struct {
		name string
		src  func(*Status)
		dst  Status
		r    Rand
		want Result
	}{
		{"plain hit", nil, Status{}, forcedMin{}, Result{Kind: Hit, Damage: 150, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		{"hard DEF", nil, Status{Def: 100}, forcedMin{}, Result{Kind: Hit, Damage: 150 * 4100 / 5000, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		{"soft DEF floors at 1", nil, Status{Def2: 1000}, forcedMin{}, Result{Kind: Hit, Damage: 1, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		{"ranged", func(s *Status) { s.RHW.Range = 9 }, Status{}, forcedMin{}, Result{Kind: Hit, Damage: 150, Hits: 1, Flag: bfWeapon | bfNormal | bfLong}},
		{"ignore melee", nil, Status{Mode: ModeIgnoreMelee}, forcedMin{}, Result{Kind: Hit, Damage: 1, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		{"damage taken", nil, Status{DamageTaken: 10}, forcedMin{}, Result{Kind: Hit, Damage: 15, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		{"lucky dodge", nil, Status{Flee2: 10}, forcedMin{}, Result{Kind: LuckyDodge, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		{"no attack", func(s *Status) { s.RHW = Weapon{}; s.BAtk = 0 }, Status{}, forcedMin{}, Result{Kind: Miss, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		{"flee", nil, Status{Flee: 1000}, fixedRand(99), Result{Kind: Miss, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		// 150 * 1.4 = 210 from battle_calc_base_damage, then * 1.4 again.
		{"critical", func(s *Status) { s.Cri = 1000 }, Status{}, forcedMin{}, Result{Kind: Critical, Damage: 294, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
		// Res 100: 150 - int(0.2 * 0.8 * 150) = 126.
		{"res", nil, Status{Res: 100}, forcedMin{}, Result{Kind: Hit, Damage: 126, Hits: 1, Flag: bfWeapon | bfNormal | bfShort}},
	}
	for _, tt := range tests {
		src := base
		if tt.src != nil {
			tt.src(&src)
		}
		if got := Attack(&src, &tt.dst, tt.r); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// fixedRand returns the same roll, capped to each range.
type fixedRand int

func (f fixedRand) IntN(n int) int { return min(int(f), n-1) }

// FuzzAttack checks that Attack never panics and never returns negative
// damage.
func FuzzAttack(f *testing.F) {
	f.Add(uint64(1), int16(100), int16(50), int16(10), int16(5), uint16(10), uint16(20), int16(0), uint32(0), uint16(100), int16(0))
	f.Add(uint64(2), int16(-399), int16(-100), int16(-50), int16(-5), uint16(65535), uint16(0), int16(-1000), uint32(0xFFFFFFFF), uint16(1), int16(1000))
	f.Add(uint64(3), int16(-400), int16(32767), int16(32767), int16(1000), uint16(0), uint16(65535), int16(32767), uint32(ModeIgnoreRanged), uint16(0), int16(-32768))
	f.Fuzz(func(t *testing.T, seed uint64, def, def2, flee2, cri int16, atk, atk2 uint16, res int16, mode uint32, taken uint16, rng int16) {
		src := &Status{Hit: int(def2), Cri: int(cri), BAtk: int(res), RHW: Weapon{Atk: int(atk), Atk2: int(atk2), Range: int(rng)}}
		dst := &Status{Def: int(def), Def2: int(def2), Flee: int(def), Flee2: int(flee2), Res: int(res), Luk: int(cri), Mode: mode, DamageTaken: int(taken)}
		r := rand.New(rand.NewPCG(seed, seed)) // #nosec G404 -- test input, not security.
		for range 20 {
			res := Attack(src, dst, r)
			if res.Damage < 0 || res.Left < 0 {
				t.Fatalf("negative damage: %+v", res)
			}
		}
		if res := Attack(src, dst, forcedMin{}); res.Damage < 0 {
			t.Fatalf("negative damage with forced minimum: %+v", res)
		}
	})
}

func BenchmarkAttack(b *testing.B) {
	src := &Status{Hit: 300, BAtk: 100, RHW: Weapon{Atk: 100, Atk2: 300, Range: 1}}
	dst := &Status{Def: 50, Def2: 30, Flee: 150, Res: 20}
	r := rand.New(rand.NewPCG(1, 2)) // #nosec G404 -- benchmark input.
	for b.Loop() {
		Attack(src, dst, r)
	}
}
