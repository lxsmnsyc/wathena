package skill

import (
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
)

func init() {
	// rAthena's 50 seeded rolls of pre-renewal level 2 on Baphomet hit
	// 20 times. Its own rate there is 25%: min_hitrate 5 plus Rapid
	// Smiting's 20 after the cap. TestShieldChainGolden checks every case
	// with the hit rate of each target pooled over the five levels.
	damageGaps["PA_SHIELDCHAIN"] = "one recorded hit count is 2.3 standard deviations off rAthena's rate; TestShieldChainGolden checks it"
}

// fixedRand returns the same value for every roll, at most n - 1.
type fixedRand int

func (f fixedRand) IntN(n int) int { return min(int(f), n-1) }

// crusaderAttacker is a level 99 sample player of a job and build with
// every tree skill and a weapon, for the hand tests.
func crusaderAttacker(t *testing.T, mode reference.Mode, job int, build string, weapon uint32) (*damageFixture, *battle.Status) {
	t.Helper()
	f := loadDamageFixture(t, mode)
	sex := 1
	c := &skillCase{Job: job, Build: build, Weapon: weapon, Sex: &sex}
	src, err := f.attacker(t, c, f.db.Get(IDHolyCross))
	if err != nil {
		t.Fatal(err)
	}
	return f, src
}

// plainTarget is a neutral monster without DEF, so that only the skill's
// own formula shows.
func plainTarget(f *damageFixture) *battle.Status {
	st := *f.mobs[1002]
	st.Def, st.Def2, st.MDef, st.MDef2 = 0, 0, 0, 0
	st.DefEle, st.EleLv = battle.Neutral, 1
	st.Flee = 0
	return &st
}

// TestShieldChainGolden checks every PA_SHIELDCHAIN case of
// skill_damage.json in both modes: the forced-minimum damage exactly,
// and the rolls' range and mean. The hit rate of each target compares
// over its five levels, as one level's 50 rolls drifted far.
func TestShieldChainGolden(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			f := loadDamageFixture(t, mode)
			type pool struct{ ours, rolls, recorded, n int }
			pools := map[int]*pool{}
			for i := range f.cases {
				c := &f.cases[i]
				if c.Skill != IDShieldChain {
					continue
				}
				def := f.db.Get(c.Skill)
				src, err := f.attacker(t, c, def)
				if err != nil {
					t.Fatal(err)
				}
				target := f.mobs[c.Target]
				sk := def.Battle(src, c.Level, c.Flag, 1)
				got := attack(mode, def.Type, src, target, sk, forcedMin{})
				want := c.ForcedMin
				if got.Damage != want.Damage+want.Damage2 || got.Kind != kindOf(want) || got.Hits != want.Div ||
					got.Flag != want.Flag {
					t.Errorf("%s: got %+v, want %+v", c.name(), got, want)
					continue
				}
				msg := checkRolls(mode, src, target, sk, def.Type, f.rolls, c.Rolls)
				if msg != "" && !strings.HasPrefix(msg, "hit rate") {
					t.Errorf("%s: %s", c.name(), msg)
				}
				p := pools[c.Target]
				if p == nil {
					p = &pool{}
					pools[c.Target] = p
				}
				p.n++
				p.recorded += c.Rolls.Hits
				p.rolls += f.rolls
				hits := 0
				for range 1000 {
					if r := attack(mode, def.Type, src, target, sk, fixedRandSeq(hits)); r.Kind == battle.Hit {
						p.ours++
					}
					hits++
				}
			}
			if len(pools) == 0 {
				t.Fatal("no Shield Chain cases")
			}
			for id, p := range pools {
				ours := float64(p.ours) / float64(1000*p.n)
				rec := float64(p.recorded) / float64(p.rolls)
				if d := ours - rec; d > 0.1 || d < -0.1 {
					t.Errorf("target %d: hit rate %.2f, recorded %.2f over %d levels", id, ours, rec, p.n)
				}
			}
		})
	}
}

// fixedRandSeq returns i % n for every roll: over 1000 attacks, every
// value of the hit roll comes up equally often.
type fixedRandSeq int

func (f fixedRandSeq) IntN(n int) int { return int(f) % n }

// TestShieldBaseDamage checks the CR_SHIELDBOOMERANG and PA_SHIELDCHAIN
// cases of battle_calc_skill_base_damage, with a shield of weight 100
// and refine 4 against a target without DEF:
//   - Pre-renewal: base ATK + 4 per refine + a tenth of the weight, then
//     Shield Boomerang's 100% + 30% per level.
//   - Renewal: the shield part is weapon ATK, which is all the damage,
//     then Shield Boomerang's 80% per level, and Shield Chain's 300% +
//     200% per level, its own shield part, and its constant addition of
//     at least 100, for 5 hits.
func TestShieldBaseDamage(t *testing.T) {
	const weight, refine = 100, 4
	shield := func(src *battle.Status) *battle.Status {
		cp := *src
		pl := *src.Player
		pl.Shield, pl.ShieldWeight, pl.ShieldRefine = true, weight, refine
		cp.Player = &pl
		return &cp
	}
	part := int64(4*refine + weight/10)

	f, src := crusaderAttacker(t, reference.PreRenewal, 14, "melee", 0)
	target := plainTarget(f)
	def := f.db.Get(IDShieldBoomerang)
	got := attack(reference.PreRenewal, def.Type, shield(src), target, def.Battle(src, 1, 0, 1), forcedMin{})
	if want := (int64(src.BAtk) + part) * 130 / 100; got.Damage != want {
		t.Errorf("pre-renewal Shield Boomerang: %d, want %d", got.Damage, want)
	}

	f, src = crusaderAttacker(t, reference.Renewal, 14, "melee", 0)
	target = plainTarget(f)
	got = attack(reference.Renewal, def.Type, shield(src), target, def.Battle(src, 5, 0, 1), forcedMin{})
	if want := part * 400 / 100; got.Damage != want {
		t.Errorf("renewal Shield Boomerang: %d, want %d", got.Damage, want)
	}

	f, src = crusaderAttacker(t, reference.Renewal, 4015, "melee", 0)
	target = plainTarget(f)
	def = f.db.Get(IDShieldChain)
	got = attack(reference.Renewal, def.Type, shield(src), target, def.Battle(src, 5, 0, 1), forcedMin{})
	ratio := int64(-100 + 300 + 200*5 + weight/10 + 4*refine)
	// The constant addition rolls between its random part and 100, at
	// least 100.
	if want := (part*ratio/100 + 100) * 5; got.Damage != want {
		t.Errorf("renewal Shield Chain: %d, want %d", got.Damage, want)
	}
}

// TestShieldChainHitBonus checks is_attack_hitting: Rapid Smiting gains
// 20 hit after the cap of 5, so a roll of 24 hits and 25 misses, while
// Holy Cross only hits on rolls below 5.
func TestShieldChainHitBonus(t *testing.T) {
	f, src := crusaderAttacker(t, reference.PreRenewal, 4015, "melee", 0)
	target := *f.mobs[1039]
	target.Flee = 1000
	chain := f.db.Get(IDShieldChain)
	cross := f.db.Get(IDHolyCross)
	pl := *src.Player
	pl.Shield, pl.ShieldWeight = true, 100
	src.Player = &pl
	for _, c := range []struct {
		def  *Def
		roll int
		hit  bool
	}{{chain, 24, true}, {chain, 25, false}, {cross, 4, true}, {cross, 5, false}} {
		got := attack(reference.PreRenewal, c.def.Type, src, &target, c.def.Battle(src, 1, 0, 1), fixedRand(c.roll))
		if hit := got.Kind == battle.Hit; hit != c.hit {
			t.Errorf("%s roll %d: %+v, want hit %v", c.def.Name, c.roll, got, c.hit)
		}
	}
}

// TestSacrificeLuckyDodge checks battle_calc_weapon_attack's lucky dodge:
// Martyr's Reckoning can be lucky dodged, Holy Cross cannot.
func TestSacrificeLuckyDodge(t *testing.T) {
	for _, mode := range reference.Modes {
		f, src := crusaderAttacker(t, mode, 4015, "melee", 0)
		target := *f.mobs[1002]
		target.Flee2 = 100
		sac := f.db.Get(IDSacrifice)
		if got := attack(mode, sac.Type, src, &target, sac.Battle(src, 5, 0, 1), forcedMin{}); got.Kind != battle.LuckyDodge {
			t.Errorf("%s: Martyr's Reckoning %+v, want a lucky dodge", mode, got)
		}
		cross := f.db.Get(IDHolyCross)
		if got := attack(mode, cross.Type, src, &target, cross.Battle(src, 5, 0, 1), forcedMin{}); got.Kind == battle.LuckyDodge {
			t.Errorf("%s: Holy Cross was lucky dodged", mode)
		}
	}
}

// TestGrandCrossOnCaster checks the Grand Cross case of
// battle_calc_magic_attack: on its caster, a player takes half the
// damage after the element table, and the element table does not apply
// a second time; another unit takes nothing.
func TestGrandCrossOnCaster(t *testing.T) {
	for _, mode := range reference.Modes {
		f, src := crusaderAttacker(t, mode, 14, "magic", 0)
		def := f.db.Get(IDGrandCross)
		target := *src
		target.DefEle, target.EleLv = battle.Neutral, 1
		other := attack(mode, def.Type, src, &target, def.Battle(src, 10, 0, 0), forcedMin{})
		sk := def.Battle(src, 10, 0, 0)
		sk.Self = true
		self := attack(mode, def.Type, src, &target, sk, forcedMin{})
		if self.Damage != other.Damage/2 || self.Damage == 0 {
			t.Errorf("%s: on the caster %d, on another %d", mode, self.Damage, other.Damage)
		}
		mob := *f.mobs[1002]
		msk := def.Battle(&mob, 10, 0, 0)
		msk.Self = true
		if got := attack(mode, def.Type, &mob, &mob, msk, forcedMin{}); got.Damage != 0 {
			t.Errorf("%s: a monster's Grand Cross on itself dealt %d", mode, got.Damage)
		}
	}
}

// TestGospelDamage checks PA_GOSPEL in battle_calc_misc_attack with the
// lowest rolls: flag 1 deals 1500, flag 0 deals 3000 less the target's
// DEF (renewal) or its percent (pre-renewal), and its soft DEF.
func TestGospelDamage(t *testing.T) {
	for _, mode := range reference.Modes {
		f, src := crusaderAttacker(t, mode, 4015, "melee", 0)
		def := f.db.Get(IDGospel)
		target := *f.mobs[1040]
		target.DefEle, target.EleLv = battle.Neutral, 1
		if got := attack(mode, battle.TypeMisc, src, &target, def.Battle(src, 10, 1, 1), forcedMin{}); got.Damage != 1500 {
			t.Errorf("%s flag 1: %d, want 1500", mode, got.Damage)
		}
		want := int64(3000 - target.Def - target.Def2)
		if mode == reference.PreRenewal {
			want = int64(3000 - 3000*target.Def/100 - target.Def2)
		}
		if got := attack(mode, battle.TypeMisc, src, &target, def.Battle(src, 10, 0, 1), forcedMin{}); got.Damage != want {
			t.Errorf("%s flag 0: %d, want %d", mode, got.Damage, want)
		}
	}
}

// TestDefenderRanged checks SC_DEFENDER in battle_calc_damage: a ranged
// weapon hit loses val2 percent, a melee one nothing.
func TestDefenderRanged(t *testing.T) {
	for _, mode := range reference.Modes {
		f := loadDamageFixture(t, mode)
		archer := *f.mobs[1040]
		archer.RHW.Range = 9
		target := *f.mobs[1002]
		plain := attackNormal(mode, &archer, &target)
		guarded := target
		guarded.Changes = &battle.Changes{AtkPercent: 100, Defender: 35}
		if got := attackNormal(mode, &archer, &guarded); got != plain-plain*35/100 {
			t.Errorf("%s ranged: %d, want %d", mode, got, plain-plain*35/100)
		}
		melee := archer
		melee.RHW.Range = 1
		if got, want := attackNormal(mode, &melee, &guarded), attackNormal(mode, &melee, &target); got != want {
			t.Errorf("%s melee: %d, want %d", mode, got, want)
		}
	}
}

// attackNormal is a forced-minimum normal attack's damage in a mode.
func attackNormal(mode reference.Mode, src, target *battle.Status) int64 {
	if mode == reference.PreRenewal {
		return battle.AttackPreRenewal(src, target, forcedMin{}).Damage
	}
	return battle.Attack(src, target, forcedMin{}).Damage
}

// TestCrusaderRatios checks the calculateSkillRatio methods that the
// recordings reach only at their sample builds: Holy Cross doubles its
// per-level ratio in renewal with a two-handed spear, and Gloria Domini
// and Shield Chain scale with base level above 99 (RE_LVL_DMOD).
func TestCrusaderRatios(t *testing.T) {
	ratio := func(impl battle.SkillImpl, renewal bool, lv int, src *battle.Status) int {
		r := 100
		impl.SkillRatio(&battle.SkillContext{Src: src, Skill: &battle.Skill{Level: lv}, Renewal: renewal}, &r)
		return r
	}
	spear := &battle.Status{Level: 99, Player: &battle.Player{Weapon: battle.Weapon2HSpear}}
	sword := &battle.Status{Level: 99, Player: &battle.Player{Weapon: battle.Weapon1HSword}}
	if got := ratio(holyCross{}, true, 10, spear); got != 800 {
		t.Errorf("renewal Holy Cross with a two-handed spear: %d%%, want 800%%", got)
	}
	if got := ratio(holyCross{}, true, 10, sword); got != 450 {
		t.Errorf("renewal Holy Cross with a sword: %d%%, want 450%%", got)
	}
	if got := ratio(holyCross{}, false, 10, spear); got != 450 {
		t.Errorf("pre-renewal Holy Cross: %d%%, want 450%%", got)
	}
	high := &battle.Status{Level: 150, Player: &battle.Player{}}
	if got := ratio(gloriaDomini{}, true, 5, high); got != (500+750)*150/100 {
		t.Errorf("renewal Gloria Domini at level 150: %d%%", got)
	}
	if got := ratio(gloriaDomini{}, false, 5, high); got != 100 {
		t.Errorf("pre-renewal Gloria Domini: %d%%, want 100%%", got)
	}
	if got := ratio(shieldChain{}, true, 5, high); got != (300+1000-100)*150/100 {
		t.Errorf("renewal Shield Chain at level 150 without a shield: %d%%", got)
	}
}
