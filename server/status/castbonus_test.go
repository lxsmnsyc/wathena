package status

import (
	"reflect"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
)

// opt makes the options of a bonus from its fields, and marks each
// option given.
func opt(o bonus.Options, given ...string) *bonus.Options {
	o.Set = map[string]bool{}
	for _, g := range given {
		o.Set[g] = true
	}
	return &o
}

// calcWith computes a novice with an armor that holds the bonuses.
func calcWith(t *testing.T, mode reference.Mode, bonuses ...Bonus) Result {
	t.Helper()
	tb := load(t, mode)
	c := Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["magic"],
		Equipment: wearing(map[int]*Equipped{item.EqiArmor: armor(bonuses...)})}
	r, err := tb.Calc(c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestCastBonuses checks that the cast, delay, cooldown and SP cost
// bonuses of 0002a set the fields of battle.CastBonus as pc_bonus and
// pc_bonus2 do.
func TestCastBonuses(t *testing.T) {
	const sk = 19 // MG_FIREBOLT
	b := func(name string, v int64, o *bonus.Options) Bonus {
		return Bonus{Name: name, Args: []int64{v}, Options: o}
	}
	pct := opt(bonus.Options{Percent: true}, "percent")
	skill := opt(bonus.Options{Skill: sk}, "skill")
	skillPct := opt(bonus.Options{Skill: sk, Percent: true}, "skill", "percent")
	bonuses := []Bonus{
		b("variable_cast_time", -10, pct),
		b("variable_cast_time", 300, nil),
		b("variable_cast_time", -200, skill),
		b("variable_cast_time", -15, skillPct),
		b("fixed_cast_time", -20, opt(bonus.Options{Percent: true, Stacking: "lowest"}, "percent", "stacking")),
		b("fixed_cast_time", -50, opt(bonus.Options{Percent: true, Stacking: "lowest"}, "percent", "stacking")),
		b("fixed_cast_time", 100, nil),
		b("fixed_cast_time", -400, skill),
		b("fixed_cast_time", -30, skillPct),
		b("after_cast_delay", -10, pct),
		b("after_cast_delay", 500, skill),
		b("cooldown", -1000, skill),
		b("sp_cost", -5, pct),
		b("sp_cost", -3, skill),
		b("sp_cost", -20, skillPct),
		{Name: "uninterruptible_cast", Options: opt(bonus.Options{})},
		{Name: "uninterruptible_cast_everywhere", Options: opt(bonus.Options{})},
		{Name: "no_gemstone_cost", Options: opt(bonus.Options{})},
	}

	r := calcWith(t, reference.Renewal, bonuses...)
	cb := r.Battle.Player.Cast
	want := battle.CastBonus{
		CastRate: 100, VarCastRate: 10, FixCastRate: -50, AddVarCast: 300, AddFixCast: 100,
		DelayRate: 10, SPRate: 95, NoCastCancel: true, NoCastCancel2: true, NoGemstone: true,
	}
	got := cb
	got.SkillCastRate, got.SkillVarCast, got.SkillFixCast, got.SkillFixCastRate = nil, nil, nil, nil
	got.SkillDelay, got.SkillCooldown, got.SkillUseSPRate, got.SkillUseSP = nil, nil, nil, nil
	if !reflect.DeepEqual(got, want) {
		t.Errorf("renewal cast bonus\n got %+v\nwant %+v", got, want)
	}
	maps := []struct {
		name string
		m    map[int]int
		want int
	}{
		{"skill cast rate", cb.SkillCastRate, 15},
		{"skill variable cast", cb.SkillVarCast, -200},
		{"skill fixed cast", cb.SkillFixCast, -400},
		{"skill fixed cast rate", cb.SkillFixCastRate, 30},
		{"skill delay", cb.SkillDelay, 500},
		{"skill cooldown", cb.SkillCooldown, -1000},
		{"skill SP rate", cb.SkillUseSPRate, 20},
		{"skill SP", cb.SkillUseSP, 3},
	}
	for _, m := range maps {
		if m.m[sk] != m.want {
			t.Errorf("renewal %s: %d, want %d", m.name, m.m[sk], m.want)
		}
	}

	// Pre-renewal: castrate takes the percent, a skill's cast rate keeps
	// its sign, and fixed cast time bonuses do nothing.
	cb = calcWith(t, reference.PreRenewal, bonuses...).Battle.Player.Cast
	if cb.CastRate != 90 || cb.VarCastRate != 0 || cb.FixCastRate != 0 || cb.AddFixCast != 0 || cb.AddVarCast != 300 {
		t.Errorf("pre-renewal cast bonus %+v", cb)
	}
	if cb.SkillCastRate[sk] != -15 || len(cb.SkillFixCast) != 0 || len(cb.SkillFixCastRate) != 0 || cb.SkillVarCast[sk] != -200 {
		t.Errorf("pre-renewal skill cast bonuses %+v", cb)
	}
}

// TestSkillDamageBonuses checks the skill, magic and healing bonuses of
// 0002a against the battle.Player fields.
func TestSkillDamageBonuses(t *testing.T) {
	const sk = 19
	target := func(key string, v int64) *bonus.Target { return &bonus.Target{Key: key, Value: v} }
	bonuses := []Bonus{
		{Name: "damage", Args: []int64{10}, Options: opt(bonus.Options{Skill: sk}, "skill")},
		{Name: "damage_taken", Args: []int64{-15}, Options: opt(bonus.Options{Skill: sk}, "skill")},
		{Name: "damage", Args: []int64{5}, Options: opt(bonus.Options{Kind: "magic", Against: target("race", 2)}, "kind", "against")},
		// No kind adds to the physical and the magic sum.
		{Name: "damage", Args: []int64{7}, Options: opt(bonus.Options{Against: target("size", 1)}, "against")},
		{Name: "damage", Args: []int64{4}, Options: opt(bonus.Options{Kind: "magic", Against: target("class", 1)}, "kind", "against")},
		{Name: "damage", Args: []int64{6}, Options: opt(bonus.Options{Kind: "magic", Against: target("element", 3)}, "kind", "against")},
		{Name: "damage", Args: []int64{8}, Options: opt(bonus.Options{Kind: "magic", Against: target("monster_group", 4)}, "kind", "against")},
		{Name: "damage", Args: []int64{9}, Options: opt(bonus.Options{Kind: "magic", Against: target("monster", 1002)}, "kind", "against")},
		{Name: "damage", Args: []int64{11}, Options: opt(bonus.Options{Kind: "magic", Element: 4}, "kind", "element")},
		{Name: "damage_taken", Args: []int64{-12}, Options: opt(bonus.Options{Kind: "magic", From: target("size", 2)}, "kind", "from")},
		{Name: "damage_taken", Args: []int64{-13}, Options: opt(bonus.Options{Kind: "magic", From: target("element", 1)}, "kind", "from")},
		{Name: "damage_taken", Args: []int64{-14}, Options: opt(bonus.Options{Kind: "magic", From: target("monster", 1002)}, "kind", "from")},
		{Name: "damage_taken", Args: []int64{-3}, Options: opt(bonus.Options{Kind: "magic"}, "kind")},
		{Name: "damage_taken", Args: []int64{-2}, Options: opt(bonus.Options{Kind: "misc"}, "kind")},
		{Name: "healing_done", Args: []int64{20}, Options: opt(bonus.Options{Skill: 28}, "skill")},
		{Name: "healing_done", Args: []int64{6}, Options: opt(bonus.Options{})},
		{Name: "healing_received", Args: []int64{25}, Options: opt(bonus.Options{Skill: 28}, "skill")},
		{Name: "healing_received", Args: []int64{3}, Options: opt(bonus.Options{})},
		{Name: "ignore_mdef", Options: opt(bonus.Options{Against: target("race", 6)}, "against")},
		{Name: "ignore_mdef", Options: opt(bonus.Options{Against: target("element", 7)}, "against")},
		{Name: "ignore_mdef", Args: []int64{30}, Options: opt(bonus.Options{Percent: true, Against: target("race", 2)}, "percent", "against")},
		{Name: "ignore_mdef", Args: []int64{40}, Options: opt(bonus.Options{Percent: true, Against: target("class", 1)}, "percent", "against")},
		{Name: "no_knockback", Options: opt(bonus.Options{})},
		{Name: "steal_chance", Args: []int64{500}, Options: opt(bonus.Options{})},
	}
	r := calcWith(t, reference.Renewal, bonuses...)
	pl := r.Battle.Player
	checks := []struct {
		name      string
		got, want int
	}{
		{"skill atk", pl.SkillAtk[sk], 10},
		{"sub skill", pl.SubSkill[sk], 15},
		{"magic add race", pl.Magic.AddRace[2], 5},
		{"magic add size", pl.Magic.AddSize[1], 7},
		{"physical add size", pl.Right.AddSize[1], 7},
		{"magic add class", pl.Magic.AddClass[1], 4},
		{"magic add element", pl.Magic.AddEle[3], 6},
		{"magic add race2", pl.Magic.AddRace2[4], 8},
		{"magic add damage", pl.Magic.AddDamage[1002], 9},
		{"magic atk element", pl.Magic.AtkEle[4], 11},
		{"magic sub size", pl.MagicSubSize[2], 12},
		{"magic sub def element", pl.MagicSubDefEle[1], 13},
		{"add mdef", pl.AddMDef[1002], 14},
		{"magic def rate", pl.MagicDefRate, 3},
		{"misc def rate", pl.MiscDefRate, 2},
		{"skill heal", pl.SkillHeal[28], 20},
		{"skill heal2", pl.SkillHeal2[28], 25},
		{"heal power", r.Bonuses.AddHealRate, 6},
		{"heal power2", r.Bonuses.AddHeal2Rate, 3},
		{"ignore mdef race rate", pl.IgnoreMDefRaceRate[2], 30},
		{"ignore mdef class rate", pl.IgnoreMDefClassRate[1], 40},
		{"steal rate", r.Bonuses.AddStealRate, 500},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: %d, want %d", c.name, c.got, c.want)
		}
	}
	if pl.IgnoreMDefRace != 1<<6 || pl.IgnoreMDefEle != 1<<7 {
		t.Errorf("ignore mdef race %#x, element %#x", pl.IgnoreMDefRace, pl.IgnoreMDefEle)
	}
	if !r.Bonuses.NoKnockback {
		t.Error("no knockback")
	}
}

// TestAutoSpellBonuses checks the "cast" bonus against rAthena's
// autospell lists.
func TestAutoSpellBonuses(t *testing.T) {
	const (
		bolt = 19 // MG_FIREBOLT
		heal = 28 // AL_HEAL
	)
	cast := func(sk, lv, chance int64, o bonus.Options, given ...string) Bonus {
		return Bonus{Name: "cast", Args: []int64{sk, lv}, Chance: chance, Options: opt(o, append(given, "chance")...)}
	}
	tb := load(t, reference.Renewal)
	tb.SetAutoSpellSelf(func(sk int) bool { return sk == heal })
	c := Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["magic"],
		Equipment: wearing(map[int]*Equipped{item.EqiArmor: armor(
			// bonus3 bAutoSpell,MG_FIREBOLT,3,50: 5% on normal weapon
			// attacks, on the enemy.
			cast(bolt, 3, 500, bonus.Options{}),
			// The same again does not stack.
			cast(bolt, 3, 500, bonus.Options{}),
			// A heal is cast on the player.
			cast(heal, 1, 100, bonus.Options{}),
			// bonus5 with BF_MAGIC and flag 3: random level, on the enemy.
			cast(bolt, 5, 200, bonus.Options{Kind: "magic", Target: "enemy", RandomLevel: true}, "kind", "target", "random_level"),
			// bAutoSpellWhenHit: normal attacks and skills.
			cast(bolt, 2, 300, bonus.Options{On: "hit_taken"}, "on"),
			// bAutoSpellOnSkill: cast on the player.
			cast(heal, 3, 1000, bonus.Options{On: "skill_used", Skill: bolt, Target: "self"}, "on", "skill", "target"),
		)})}
	r, err := tb.Calc(c)
	if err != nil {
		t.Fatal(err)
	}
	const (
		weapon = 0x0001
		magic  = 0x0002
		short  = 0x0010
		long   = 0x0040
		skill  = 0x0100
		normal = 0x0200
	)
	wantAttack := []AutoSpell{
		{Skill: bolt, Level: 3, Rate: 50, BattleFlag: weapon | short | long | normal, Flag: AutoSpellForceTarget},
		{Skill: heal, Level: 1, Rate: 10, BattleFlag: weapon | short | long | normal},
		{Skill: bolt, Level: 5, Rate: 20, BattleFlag: magic | short | long | skill, Flag: AutoSpellForceTarget | AutoSpellForceRandomLevel},
	}
	if len(r.Bonuses.AutoSpell) != len(wantAttack) {
		t.Fatalf("autospell %+v", r.Bonuses.AutoSpell)
	}
	for i, w := range wantAttack {
		if r.Bonuses.AutoSpell[i] != w {
			t.Errorf("autospell %d: %+v, want %+v", i, r.Bonuses.AutoSpell[i], w)
		}
	}
	wantHit := []AutoSpell{{Skill: bolt, Level: 2, Rate: 30, BattleFlag: weapon | short | long | normal | skill, Flag: AutoSpellForceTarget}}
	if len(r.Bonuses.AutoSpellWhenHit) != 1 || r.Bonuses.AutoSpellWhenHit[0] != wantHit[0] {
		t.Errorf("autospell when hit %+v", r.Bonuses.AutoSpellWhenHit)
	}
	wantSkill := AutoSpell{Skill: heal, Level: 3, Rate: 100, Flag: AutoSpellForceTarget, TriggerSkill: bolt}
	if len(r.Bonuses.AutoSpellOnSkill) != 1 || r.Bonuses.AutoSpellOnSkill[0] != wantSkill {
		t.Errorf("autospell on skill %+v", r.Bonuses.AutoSpellOnSkill)
	}
}
