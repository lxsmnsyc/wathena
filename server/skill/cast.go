package skill

import (
	"math"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/rules"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Cast settings from rAthena's conf/battle/skill.conf.
const (
	// castrateDexScale is castrate_dex_scale: in pre-renewal, a DEX of
	// this much casts instantly.
	castrateDexScale = 150
	// vcastStatScale is vcast_stat_scale: in renewal, DEX * 2 + INT of
	// this much removes the variable cast time.
	vcastStatScale = 530
	// defaultFixedCastrate is default_fixed_castrate: the part of the
	// cast time that is fixed for a skill without FixedCastTime.
	defaultFixedCastrate = 20
	// castRate and delayRate are cast_rate and delay_rate.
	castRate  = 100
	delayRate = 100
	// MinSkillDelayLimit is min_skill_delay_limit, and the delay of
	// monsters, which no_skill_delay covers.
	MinSkillDelayLimit = 100
)

// Caster is what the cast formulas read of a unit: its battle status,
// whose Player is set for a player, its statuses, and whether it is a
// monster or an NPC, whose cast times are not changed.
type Caster struct {
	Status  *battle.Status
	Changes *sc.List
	// Monster is set for a monster or an NPC.
	Monster bool
	// TargetDistance is distance_math_bl from the caster to the target
	// of an id skill, which Charge Attack's cast time reads.
	TargetDistance int
	// SpiritBalls is a player's spirit spheres, as sd->spiritball.
	SpiritBalls int
	// Renewal is set in renewal mode. Rules, when set, holds the world's
	// ruleset flags, which some requirements and delays read. Without
	// Rules, every flag follows Renewal.
	Renewal bool
	Rules   *rules.Rules
	// Encore is the dance that costs half its SP after Encore
	// (skill_id_old BD_ENCORE and skill_id_dance), or 0.
	Encore int
}

// ruleset returns the caster's rules.
func (c *Caster) ruleset() *rules.Rules {
	if c.Rules != nil {
		return c.Rules
	}
	return rules.Of(c.Renewal)
}

func (c *Caster) cast() *battle.CastBonus {
	if c.Status == nil || c.Status.Player == nil {
		return nil
	}
	return &c.Status.Player.Cast
}

// CastResult is a cast time, and the statuses its calculation used up,
// which the caller ends: Suffragium in pre-renewal, and Foresight's
// count.
type CastResult struct {
	Milliseconds int
	// EndSuffragium is set when Suffragium ends with the cast.
	EndSuffragium bool
	// UseMemorize is set when Foresight's count goes down by one.
	UseMemorize bool
}

// CastTimeFor is the cast time of unit_skilluse_id2: skill_castfix, then
// skill_vfcastfix with renewal's cast time (RENEWAL_CAST) or
// skill_castfix_sc with pre-renewal's. renewal is the cast_time flag.
// learned is whether the caster learned the skill, which Foresight
// reads.
func (d *Def) CastTimeFor(c *Caster, lv int, renewal, learned bool) CastResult {
	var res CastResult
	t := float64(d.CastTime.At(lv))
	if !renewal {
		t = d.castfixPreRenewal(c, t, lv, learned, &res)
		if t < 0 {
			res.Milliseconds = 0
			return res
		}
		if d.ID == IDChargeAttack {
			// unit_skilluse_id2: 0 to 3 cells cast at the base time, 4
			// to 6 at twice and 7 or more at three times.
			k := min(max((c.TargetDistance-1)/3, 0), 2)
			t += float64(int(t) * k)
		}
		t = monkCastTime(d, c, lv, t)
		res.Milliseconds = d.castfixSC(c, t, &res)
		return res
	}
	// skill_castfix in renewal only applies cast_rate.
	t = float64(max(int(t*castRate/100), 0))
	t = monkCastTime(d, c, lv, t)
	res.Milliseconds = d.vfcastfix(c, t, lv, learned, &res)
	return res
}

// castfixPreRenewal is the pre-renewal part of skill_castfix: DEX, item
// bonuses, Magic Strings and Foresight. It returns -1 for an instant
// cast.
func (d *Def) castfixPreRenewal(c *Caster, t float64, lv int, learned bool, res *CastResult) float64 {
	flag := d.CastTimeFlags
	reduce := 0
	if flag&CastIgnoreDex == 0 {
		scale := castrateDexScale - c.Status.Dex
		if scale <= 0 {
			return -1
		}
		t = t * float64(float32(scale)) / castrateDexScale
	}
	if b := c.cast(); b != nil {
		if flag&CastIgnoreItemBonus == 0 {
			if b.CastRate != 100 {
				reduce += 100 - b.CastRate
			}
			if b.AddVarCast != 0 {
				t += float64(b.AddVarCast)
			}
		}
		if v, ok := b.SkillCastRate[d.ID]; ok {
			t += t * float64(v) / 100
		}
		if v, ok := b.SkillVarCast[d.ID]; ok {
			t += float64(v)
		}
	}
	if l := c.Changes; l != nil && !l.Empty() {
		if flag&CastIgnoreStatus == 0 {
			reduce += l.Val2(sc.SCPoembragi)
		}
		if l.Has(sc.SCMemorize) && (c.cast() == nil || learned) {
			if flag&CastIgnoreStatus == 0 {
				t -= t * 50 / 100
			}
			res.UseMemorize = true
		}
	}
	t *= float64(1 - float32(reduce)/100)
	t = t * castRate / 100
	return float64(max(int(t), 0))
}

// castfixSC is skill_castfix_sc in pre-renewal: statuses that change the
// cast time. Monsters and NPCs keep theirs.
func (d *Def) castfixSC(c *Caster, t float64, res *CastResult) int {
	if t < 0 {
		return 0
	}
	if c.Monster {
		return int(t)
	}
	if l := c.Changes; l != nil && !l.Empty() {
		if d.CastTimeFlags&CastIgnoreStatus == 0 {
			if e := l.Get(sc.SCSlowcast); e != nil {
				t += t * float64(e.Val2) / 100
			}
		}
		if e := l.Get(sc.SCSuffragium); e != nil {
			if d.CastTimeFlags&CastIgnoreStatus == 0 {
				t -= t * float64(e.Val2) / 100
			}
			res.EndSuffragium = true
		}
	}
	return int(max(t, 0))
}

// vfcastfix is skill_vfcastfix in renewal: the fixed and variable cast
// times, item bonuses, statuses, and the reduction by DEX and INT.
// Monsters and NPCs keep theirs.
func (d *Def) vfcastfix(c *Caster, t float64, lv int, learned bool, res *CastResult) int {
	if t < 0 {
		return 0
	}
	if c.Monster {
		return int(t)
	}
	flag := d.CastTimeFlags
	fixed := d.FixedCastTime.At(lv)
	if !d.fixedCastSet {
		// Pre-renewal's skill_db has no FixedCastTime, which rAthena
		// reads as unset (-1).
		fixed = -1
	}
	fixcastR, varcastR, reduce := 0, 0, 0
	if fixed < 0 {
		if defaultFixedCastrate > 0 {
			fixed = int(t) * defaultFixedCastrate / 100
			t = t * (100 - defaultFixedCastrate) / 100
		} else {
			fixed = 0
		}
	}
	if b := c.cast(); b != nil && flag&CastIgnoreItemBonus == 0 {
		reduce += b.VarCastRate
		fixcastR -= b.FixCastRate
		t += float64(b.AddVarCast)
		fixed += b.AddFixCast
		if v, ok := b.SkillFixCast[d.ID]; ok {
			fixed += v
		}
		if v, ok := b.SkillVarCast[d.ID]; ok {
			t += float64(v)
		}
		if v, ok := b.SkillCastRate[d.ID]; ok {
			reduce += v
		}
		if v, ok := b.SkillFixCastRate[d.ID]; ok {
			fixcastR = max(fixcastR, v)
		}
	}
	// varcast is rAthena's VARCAST_REDUCTION: the value adds to
	// varcast_r, and while the sum is above 0 the cast time goes down by
	// the value at once.
	varcast := func(val int) {
		if varcastR += val; varcastR != 0 && varcastR >= 0 {
			t *= float64(1 - float32(min(val, 100))/100)
		}
	}
	if l := c.Changes; l != nil && !l.Empty() && flag&CastIgnoreStatus == 0 {
		if e := l.Get(sc.SCSlowcast); e != nil {
			varcast(-e.Val2)
		}
		if e := l.Get(sc.SCSuffragium); e != nil {
			// Renewal keeps Suffragium after the cast.
			varcast(e.Val2)
		}
		if l.Has(sc.SCMemorize) && (c.cast() == nil || learned) {
			reduce += 50
			res.UseMemorize = true
		}
		reduce += l.Val2(sc.SCPoembragi)
	}
	if varcastR < 0 {
		t *= float64(1 - float32(min(varcastR, 100))/100)
	}
	if flag&CastIgnoreDex == 0 {
		st := c.Status
		t *= 1 - math.Sqrt(float64(float32(st.Dex*2+st.Int)/vcastStatScale))
	}
	t *= float64(1 - float32(min(reduce, 100))/100)
	return int(float64(max(int(t), 0)) + float64(1-float32(min(fixcastR, 100))/100)*float64(max(fixed, 0)))
}

// Delay is skill_delayfix: the after-cast delay. Monsters have
// min_skill_delay_limit (no_skill_delay). delay_dependon_dex and
// delay_dependon_agi are off. The caller keeps the delay at
// min_skill_delay_limit or more.
func (d *Def) Delay(c *Caster, lv int) int {
	if c.Monster {
		return MinSkillDelayLimit
	}
	t := float64(d.AfterCastActDelay.At(lv))
	if t < 0 {
		t = -t + float64(c.Status.AMotion)
	}
	switch d.ID {
	case IDTripleAttack, IDChainCombo, IDComboFinish, IDTigerFist, IDChainCrush:
		// Monk combos are reduced by AGI and DEX.
		if t == 0 {
			t = 1000
		}
		t -= float64(4*c.Status.Agi + 2*c.Status.Dex)
	case IDBasilica:
		// Pre-renewal Basilica has no delay when it starts, only when it
		// is cancelled. A player always has a status list.
		if !c.ruleset().Skill(IDBasilica) && !c.Changes.Has(sc.SCBasilica) {
			t = 0
		}
	}
	delay := 0
	if d.CastDelayFlags&2 == 0 {
		if l := c.Changes; l != nil {
			delay += l.Val3(sc.SCPoembragi)
		}
	}
	if b := c.cast(); b != nil && d.CastDelayFlags&4 == 0 {
		delay += b.DelayRate
		if v, ok := b.SkillDelay[d.ID]; ok {
			t += float64(v)
		}
	}
	if delay != 0 {
		t *= float64(1 - float32(min(delay, 100))/100)
	}
	t = t * delayRate / 100
	return max(int(t), 0)
}

// CooldownFor is the cooldown that skill_blockpc_start gets: skill_db's
// Cooldown with the bSkillCooldown bonus.
func (d *Def) CooldownFor(c *Caster, lv int) int {
	t := d.Cooldown.At(lv)
	if b := c.cast(); b != nil {
		if v, ok := b.SkillCooldown[d.ID]; ok {
			t += v
		}
	}
	return max(t, 0)
}

// Requirement is s_skill_condition: what a skill costs and needs at a
// level, after the caster's bonuses.
type Requirement struct {
	HP, MaxHP, SP, Zeny int
	// Weapon and Ammo are bits of 1 << W_ and 1 << AMMO_ types.
	Weapon, Ammo, AmmoAmount int
	State                    int
	SpiritSphere             int
	// Items are the items used, in order: [item, amount].
	Items [][2]int
	// Status lists the statuses the caster needs.
	Status []int
	// Equipment lists the items the caster must wear.
	Equipment []int
}

// gemstones are the items of rAthena's IG_GEMSTONE group: the Blue,
// Red and Yellow Gemstones.
var gemstones = map[int]bool{715: true, 716: true, 717: true}

// RequirementFor is skill_get_requirement for a player: the costs of skill
// data, with HP and SP rates on the current values, the caster's SP
// bonuses, gemstone bonuses, and the skill reductions of Unfair Trick.
// hp and sp are the caster's current HP and SP.
func (d *Def) RequirementFor(c *Caster, lv int, hp, sp int64) Requirement {
	lv = min(max(lv, 1), MaxLevel)
	st := c.Status
	q := &d.Requires
	r := Requirement{}
	r.HP = q.HP.At(lv)
	if rate := q.HPRate.At(lv); rate > 0 {
		r.HP += int(hp * int64(rate) / 100)
	} else {
		r.HP += st.MaxHP * -rate / 100
	}
	r.SP = q.SP.At(lv)
	if c.Encore != 0 && d.ID == c.Encore {
		r.SP /= 2
	}
	if rate := q.SPRate.At(lv); rate > 0 {
		r.SP += int(sp * int64(rate) / 100)
	} else {
		r.SP += st.MaxSP * -rate / 100
	}
	spSkillRate := 100
	if b := c.cast(); b != nil {
		if b.SPRate != 100 {
			r.SP = r.SP * b.SPRate / 100
		}
		if v, ok := b.SkillUseSPRate[d.ID]; ok {
			spSkillRate -= v
		}
		if v, ok := b.SkillUseSP[d.ID]; ok {
			r.SP -= v
		}
	}
	r.SP = min(max(r.SP*spSkillRate/100, 0), math.MaxInt16)
	// Renewal's Adaptation lowers the SP of songs by 20%, and Ring of
	// Nibelungen's tenth effect all SP by 30% (ensemble_songs).
	if l := c.Changes; c.ruleset().EnsembleSongs && !l.Empty() {
		if l.Has(sc.SCAdaptation) && d.Has("IsSong") {
			r.SP -= r.SP * 20 / 100
		}
		if l.Val2(sc.SCNibelungen) == ringSPConsume {
			r.SP -= r.SP * 30 / 100
		}
	}
	r.Zeny = q.Zeny.At(lv)
	r.SpiritSphere = q.SpiritSphere.At(lv)
	r.State = q.State
	r.MaxHP = q.MaxHPTrigger.At(lv)
	r.Weapon = q.Weapon
	r.AmmoAmount = q.AmmoAmount.At(lv)
	if r.AmmoAmount != 0 {
		r.Ammo = q.Ammo
	}
	r.Status = q.Status
	r.Equipment = q.Equipment

	noGem := false
	if b := c.cast(); b != nil {
		noGem = b.NoGemstone
	}
	// Into the Abyss: no gemstones either.
	if c.Status != nil && c.Status.Player != nil && c.Changes.Has(sc.SCIntoabyss) {
		noGem = true
	}
	add := func(it [2]int) {
		if it[0] == 0 {
			return
		}
		if gemstones[it[0]] && noGem {
			// Mistress Card: no gemstones.
			return
		}
		r.Items = append(r.Items, it)
	}
	if q.ItemLevelDependent {
		if lv <= len(q.Items) {
			add(q.Items[lv-1])
		}
	} else {
		for i, it := range q.Items {
			switch d.ID {
			case IDPotionPitcher, IDSlimPitcher, IDCultivation:
				// Only the item of the level is used.
				if i != lv%11-1 {
					continue
				}
			case IDCallHomunculus:
				// No player has a homunculus yet. Renewal then takes only
				// the first item. Pre-renewal takes them all, but its
				// skill_db lists only one.
				if i > 0 {
					continue
				}
			}
			add(it)
		}
	}

	// The cost reductions of skills: Unfair Trick lowers the zeny cost
	// of Mammonite, and in renewal of Cart Termination, by 20% in
	// renewal and 10% in pre-renewal. Each skill's skill_rebalance flag
	// picks.
	renewal := c.ruleset().Skill(d.ID)
	switch {
	case d.ID == IDMammonite, d.ID == IDCartTermination && renewal:
		if st.Player != nil && st.Player.Skills[IDUnfairTrick] > 0 {
			if renewal {
				r.Zeny -= r.Zeny * 20 / 100
			} else {
				r.Zeny -= r.Zeny * 10 / 100
			}
		}
	case d.ID == IDChainCombo, d.ID == IDComboFinish, d.ID == IDTigerFist, d.ID == IDChainCrush,
		d.ID == IDBodyRelocation, d.ID == IDExtremityFist:
		monkRequirement(d, c, &r, renewal)
	}
	return r
}

// Skill IDs that the cast formulas read.
const (
	IDTripleAttack = 263  // MO_TRIPLEATTACK
	IDChainCombo   = 272  // MO_CHAINCOMBO
	IDComboFinish  = 273  // MO_COMBOFINISH
	IDTigerFist    = 371  // CH_TIGERFIST
	IDChainCrush   = 372  // CH_CHAINCRUSH
	IDUnfairTrick  = 1012 // BS_UNFAIRLYTRICK
)

// MonsterCastTime is the cast time of a monster's skill: the cast time
// of its mob_skill_db entry, which unit_skilluse_id2 and
// unit_skilluse_pos2 change with skill_castfix_sc or skill_vfcastfix, as
// the cast_time flag (renewal) picks. A monster has no cast time of its
// own stats.
func (d *Def) MonsterCastTime(c *Caster, ms, lv int, renewal bool) CastResult {
	var res CastResult
	if renewal {
		res.Milliseconds = d.vfcastfix(c, float64(ms), lv, false, &res)
		return res
	}
	res.Milliseconds = d.castfixSC(c, float64(ms), &res)
	return res
}
