package status

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
)

// pcBonus is the part of map_session_data that item bonuses change:
// sd->bonus, the *_rate fields, indexed_bonus.param_bonus and
// param_equip, and the battle bonuses in player.
type pcBonus struct {
	p *pc
	// paramBonus and paramEquip are indexed_bonus.param_bonus and
	// param_equip: stats from cards and from equipment scripts.
	paramBonus, paramEquip traits
	hp, sp                 int
	hprate, sprate         int
	hprecovRate            int
	sprecovRate            int
	noHPRegen, noSPRegen   bool
	noWalkDelay            bool
	matkRate               int
	criticalRate, hitRate  int
	fleeRate, flee2Rate    int
	defRate, def2Rate      int
	mdefRate, mdef2Rate    int
	patkRate, smatkRate    int
	resRate, mresRate      int
	hplusRate, crateRate   int
	addMaxWeight           int
	eatk                   int
	ematk, ematkHidden     int
	weaponAtkRate          int
	weaponMatkRate         int
	atkRate                int
	// aspdAdd is bonus.aspd_add, the raw change of bAspd. aspdRate2 is
	// renewal's bAspdRate, and aspdRate pre-renewal's change to
	// aspd_rate.
	aspdAdd, aspdRate2, aspdRate int
	speedRate, speedAddRate      int
	doubleRate, doubleAddRate    int
	perfectHit, perfectHitAdd    int
	arrowAtk, arrowEle           int
	// weaponAtk is indexed_bonus.weapon_atk: bWeaponAtk by weapon type.
	weaponAtk [weaponTypes]int
	player    *battle.Player
	// decided holds the stacking decisions: bonuses another bonus of
	// their set replaced.
	skipped map[*Bonus]bool
	// resEff, addEff, addEffWhenHit, addEffOnSkill and regen are the
	// bonuses of statuses and regen, as Bonuses.
	resEff                               map[int]int
	addEff, addEffWhenHit, addEffOnSkill []AddEffect
	regen                                []RegenBonus
	// autoSpell, autoSpellWhenHit and autoSpellOnSkill are autospell,
	// autospell2 and autospell3. cardID is current_equip_card_id: the
	// card whose script runs, or 0.
	autoSpell, autoSpellWhenHit, autoSpellOnSkill []AutoSpell
	cardID                                        int
	noKnockback                                   bool
	unstripable                                   uint32
	addStealRate                                  int
	addHealRate, addHeal2Rate                     int
}

func newPCBonus(p *pc) *pcBonus {
	return &pcBonus{
		p: p, hprate: 100, sprate: 100, hprecovRate: 100, sprecovRate: 100, matkRate: 100,
		criticalRate: 100, hitRate: 100, fleeRate: 100, flee2Rate: 100, defRate: 100, def2Rate: 100,
		mdefRate: 100, mdef2Rate: 100, patkRate: 100, smatkRate: 100, resRate: 100, mresRate: 100,
		hplusRate: 100, crateRate: 100,
		player: &battle.Player{PreRenewalSizes: false, Cast: battle.CastBonus{CastRate: 100, SPRate: 100}},
	}
}

// tagged is a bonus with the lr flag of the script that added it, in
// rAthena's order.
type tagged struct {
	b  *Bonus
	lr int
}

// order lists every bonus of the equipment in the order status_calc_pc
// runs item scripts (0002a, "Stacking"): equipment by slot, then the
// ammunition, then cards by the slot of the item that holds them, then
// status scripts and timed bonuses.
func (pb *pcBonus) order(eq *Equipment) []tagged {
	var out []tagged
	p := pb.p
	for i := range item.EqiMax {
		e := eq.Items[i]
		if e == nil || i == item.EqiAmmo || p.sameEquipIndex(i) {
			continue
		}
		lr := lrNone
		switch {
		case e.Def.IsWeapon():
			if i == item.EqiHandL && e.Position == item.EqpHandL {
				lr = lrWeapon
			}
		case e.Def.IsArmor():
			if i == item.EqiHandL {
				lr = lrShield
			}
		case e.Def.IsShadowGear():
		default:
			continue
		}
		if pb.p.restricted(e.Def) {
			continue
		}
		for j := range e.Bonuses {
			out = append(out, tagged{&e.Bonuses[j], lr})
		}
	}
	if e := eq.Items[item.EqiAmmo]; e != nil {
		for j := range e.Bonuses {
			out = append(out, tagged{&e.Bonuses[j], lrArrow})
		}
	}
	for i := range item.EqiMax {
		e := eq.Items[i]
		if e == nil || i == item.EqiAmmo || p.sameEquipIndex(i) {
			continue
		}
		lr := lrNone
		if i == item.EqiHandL && e.Position == item.EqpHandL {
			lr = lrWeapon
		}
		for c := range e.Cards {
			if pb.p.restricted(e.Cards[c].Def) {
				continue
			}
			for j := range e.Cards[c].Bonuses {
				out = append(out, tagged{&e.Cards[c].Bonuses[j], lr})
			}
		}
	}
	// Random options, after every card.
	for i := range item.EqiMax {
		e := eq.Items[i]
		if e == nil || i == item.EqiAmmo || p.sameEquipIndex(i) || pb.p.restricted(e.Def) {
			continue
		}
		lr := lrNone
		if i == item.EqiHandL && e.Position == item.EqpHandL {
			lr = lrWeapon
		}
		for j := range e.Options {
			out = append(out, tagged{&e.Options[j], lr})
		}
	}
	for _, list := range [][]Bonus{p.c.StatusBonuses, p.c.TimedBonuses} {
		for j := range list {
			out = append(out, tagged{&list[j], lrNone})
		}
	}
	return out
}

// plan applies the stacking option (0002a, "Stacking") before the
// scripts run: in a set of bonuses, only the highest of the "highest"
// bonuses, the lowest of the "lowest" ones, and the winner of a
// one-value set apply. The others are skipped. "sum" bonuses all apply.
func (pb *pcBonus) plan(eq *Equipment) {
	type candidate struct {
		t        tagged
		pos      int
		value    int64
		priority int64
	}
	sets := map[string][]candidate{}
	var keys []string
	for pos, t := range pb.order(eq) {
		b := t.b
		spec, ok := bonus.Lookup(b.Name)
		if !ok || b.Options == nil {
			continue
		}
		st := b.Options.Stacking
		if st == "" || st == "sum" {
			if !spec.OneValue {
				continue
			}
			st = "last"
		}
		key := setKey(spec, b, t.lr, st)
		if _, ok := sets[key]; !ok {
			keys = append(keys, key)
		}
		c := candidate{t: t, pos: pos, priority: b.Options.Priority}
		switch {
		case spec.Proc:
			c.value = b.Chance
		case len(b.Args) > len(spec.Subject):
			c.value = b.Args[len(spec.Subject)]
		}
		sets[key] = append(sets[key], c)
	}
	pb.skipped = map[*Bonus]bool{}
	for _, key := range keys {
		set := sets[key]
		var win candidate
		switch set[0].t.b.Options.Stacking {
		case "highest":
			win = slices.MaxFunc(set, func(a, b candidate) int { return cmp.Compare(a.value, b.value) })
		case "lowest":
			win = slices.MinFunc(set, func(a, b candidate) int { return cmp.Compare(a.value, b.value) })
		default:
			// The highest priority wins, and among equal priorities the
			// last in rAthena's order.
			win = set[0]
			for _, c := range set[1:] {
				if c.priority >= win.priority {
					win = c
				}
			}
		}
		for _, c := range set {
			if c.t.b != win.t.b {
				pb.skipped[c.t.b] = true
			}
		}
	}
}

// setKey names a bonus's set: the name, the arguments before the amount,
// the options other than stacking, priority, chance and when, and the
// stacking group. A one-value bonus's argument is the value it keeps,
// such as the element of attack_element, so it is not part of the key.
// attack_element is kept apart by hand, since each hand and the arrow
// keep their own element.
func setKey(spec *bonus.Spec, b *Bonus, lr int, stacking string) string {
	var s strings.Builder
	s.WriteString(b.Name)
	if !spec.OneValue {
		for i := 0; i < len(spec.Subject) && i < len(b.Args); i++ {
			fmt.Fprintf(&s, "|%d", b.Args[i])
		}
	}
	o := b.Options
	fmt.Fprintf(&s, "|%t|%d|%d|%d|%s|%s|%s|%d|%d|%s|%s|%d|%t", o.Percent, o.Skill, o.Item, o.ItemGroup, o.Kind, o.Range,
		o.Attack, o.Element, o.WeaponType, o.On, o.Target, o.Duration, o.RandomLevel)
	for _, t := range []*bonus.Target{o.Against, o.From} {
		if t != nil {
			fmt.Fprintf(&s, "|%s=%d", t.Key, t.Value)
		} else {
			s.WriteString("|-")
		}
	}
	switch stacking {
	case "last", "priority":
		s.WriteString("|one")
	default:
		s.WriteString("|sum")
	}
	if b.Name == "attack_element" {
		fmt.Fprintf(&s, "|lr%d", lr)
	}
	return s.String()
}

// restricted is itemdb_isNoEquip: the item's script does not run on the
// map.
func (p *pc) restricted(d *item.Def) bool {
	return d != nil && d.NoEquip&p.eq.MapType != 0
}

// run is run_script for one script's bonuses: each bonus that stacking
// kept goes through pc_bonus in order, with the script's lr flag.
func (pb *pcBonus) run(list []Bonus, lr int, st *data) {
	for i := range list {
		b := &list[i]
		if pb.skipped[b] {
			continue
		}
		pb.apply(b, lr, st)
	}
}

// amount returns the bonus's amount: its first number after the subject.
func amount(b *Bonus) int {
	spec, ok := bonus.Lookup(b.Name)
	if !ok {
		return 0
	}
	i := len(spec.Subject)
	if i >= len(b.Args) {
		return 0
	}
	return clampInt(b.Args[i])
}

// clampInt turns a script number into an int32-sized int, as rAthena's
// bonus commands take int32 values.
func clampInt(v int64) int {
	return int(min(max(v, math.MinInt32), math.MaxInt32))
}

// paramIndex is the PARAM_ index of a stat bonus.
var paramIndex = map[string]int{
	"str": paramStr, "agi": paramAgi, "vit": paramVit, "int": paramInt, "dex": paramDex, "luk": paramLuk,
	"pow": paramPow, "sta": paramSta, "wis": paramWis, "spl": paramSpl, "con": paramCon, "crt": paramCrt,
}

// ammoWeapon reports whether a weapon type takes the arrow's element and
// range as its own: bows and guns, as pc_bonus's LR_FLAG_ARROW cases.
func ammoWeapon(weapon int) bool {
	switch weapon {
	case weaponBow, weaponRevolver, weaponRifle, weaponGatling, weaponShotgun, weaponGrenade:
		return true
	}
	return false
}

// apply is pc_bonus to pc_bonus5 for one bonus, by its name and options
// (proposal 0002a). The converter turned each rAthena form into a name;
// apply turns it back into the field rAthena changes, with rAthena's
// rules for the lr flag. Bonuses that only skills, procs, drops, healing
// and other later systems read are left out.
func (pb *pcBonus) apply(b *Bonus, lr int, st *data) {
	o := b.Options
	if o == nil {
		o = &bonus.Options{}
	}
	v := amount(b)
	arrow := lr == lrArrow
	pl := pb.player
	renewal := pb.p.t.renewal
	shortCap := func(x int) int { return capValue(x, math.MinInt16, math.MaxInt16) }
	if i, ok := paramIndex[b.Name]; ok {
		if !arrow {
			pb.paramBonus[i] += v
		}
		return
	}
	switch b.Name {
	case "all_stats":
		if !arrow {
			for i := paramStr; i <= paramLuk; i++ {
				pb.paramBonus[i] += v
			}
		}
	case "all_trait_stats":
		if !arrow {
			for i := paramPow; i <= paramCrt; i++ {
				pb.paramBonus[i] += v
			}
		}
	case "patk", "smatk", "res", "mres", "hplus", "crate":
		if arrow {
			return
		}
		if o.Percent {
			*map[string]*int{"patk": &pb.patkRate, "smatk": &pb.smatkRate, "res": &pb.resRate, "mres": &pb.mresRate,
				"hplus": &pb.hplusRate, "crate": &pb.crateRate}[b.Name] += v
			return
		}
		f := map[string]*int{"patk": &st.patk, "smatk": &st.smatk, "res": &st.Res, "mres": &st.MRes,
			"hplus": &st.hplus, "crate": &st.crate}[b.Name]
		*f = shortCap(*f + v)
	case "max_hp":
		if !arrow {
			if o.Percent {
				pb.hprate += v
			} else {
				pb.hp += v
			}
		}
	case "max_sp":
		if !arrow {
			if o.Percent {
				pb.sprate += v
			} else {
				pb.sp += v
			}
		}
	case "natural_hp_recovery":
		if !arrow {
			pb.hprecovRate += v
		}
	case "natural_sp_recovery":
		if !arrow {
			pb.sprecovRate += v
		}
	case "no_hp_recovery":
		if !arrow {
			pb.noHPRegen = true
		}
	case "no_sp_recovery":
		if !arrow {
			pb.noSPRegen = true
		}
	case "atk":
		if arrow {
			return
		}
		switch {
		case o.Has("weapon_type"):
			// bWeaponAtk.
			if w := int(o.WeaponType); w >= 0 && w < weaponTypes {
				pb.weaponAtk[w] += v
			}
		case o.Percent:
			// bAtkRate. rAthena refuses it in pre-renewal.
			if renewal {
				pb.atkRate += v
			}
		default:
			// bBaseAtk.
			if renewal {
				pb.eatk = shortCap(pb.eatk + v)
			} else {
				st.BAtk += v
			}
		}
	case "weapon_atk":
		if o.Percent {
			if !arrow {
				pb.weaponAtkRate += v
			}
			return
		}
		// bAtk.
		switch lr {
		case lrNone:
			st.RHW.Atk = capValue(st.RHW.Atk+v, 0, math.MaxUint16)
		case lrWeapon:
			st.LHW.Atk = capValue(st.LHW.Atk+v, 0, math.MaxUint16)
		}
	case "refine_atk":
		// bAtk2.
		switch lr {
		case lrNone:
			st.RHW.Atk2 = capValue(st.RHW.Atk2+v, 0, math.MaxUint16)
		case lrWeapon:
			st.LHW.Atk2 = capValue(st.LHW.Atk2+v, 0, math.MaxUint16)
		}
	case "matk":
		if !arrow {
			if o.Percent {
				pb.matkRate += v
			} else {
				pb.ematk += v
			}
		}
	case "hidden_matk":
		if !arrow {
			pb.ematkHidden += v
		}
	case "weapon_matk":
		if !arrow {
			pb.weaponMatkRate += v
		}
	case "def":
		if arrow {
			return
		}
		if o.Percent {
			pb.defRate += v
		} else {
			st.Def = pb.p.capDef(st.Def + v)
		}
	case "status_def":
		if arrow {
			return
		}
		if o.Percent {
			pb.def2Rate += v
		} else {
			st.Def2 = shortCap(st.Def2 + v)
		}
	case "mdef":
		if arrow {
			return
		}
		if o.Percent {
			pb.mdefRate += v
		} else {
			st.MDef = pb.p.capDef(st.MDef + v)
		}
	case "status_mdef":
		if arrow {
			return
		}
		if o.Percent {
			pb.mdef2Rate += v
		} else {
			st.MDef2 = shortCap(st.MDef2 + v)
		}
	case "attack_range":
		switch lr {
		case lrArrow:
			if ammoWeapon(pb.p.worn.Weapon) {
				st.RHW.Range += v
			}
		case lrWeapon:
			st.LHW.Range += v
		default:
			st.RHW.Range += v
		}
	case "hit":
		switch {
		case o.Percent:
			if !arrow {
				pb.hitRate += v
			}
		case arrow:
			pl.Arrow.Hit += v
		default:
			st.Hit = shortCap(st.Hit + v)
		}
	case "flee":
		if arrow {
			return
		}
		if o.Percent {
			pb.fleeRate += v
		} else {
			st.Flee = shortCap(st.Flee + v)
		}
	case "perfect_dodge":
		if arrow {
			return
		}
		if o.Percent {
			pb.flee2Rate += v
		} else {
			st.Flee2 = shortCap(st.Flee2 + v*10)
		}
	case "crit":
		switch {
		case o.Against != nil:
			// bCriticalAddRace.
			if !arrow {
				battle.Add(&pl.CritAddRace, int(o.Against.Value), v*10)
			}
		case o.Has("range"):
			// bCriticalLong.
			if arrow {
				pl.Arrow.Cri += v * 10
			} else {
				pl.CriticalRangeAtk += v * 10
			}
		case o.Percent:
			if !arrow {
				pb.criticalRate += v
			}
		case arrow:
			pl.Arrow.Cri += v * 10
		default:
			st.Cri = shortCap(st.Cri + v*10)
		}
	case "crit_chance_taken":
		// bCriticalDef, with the sign flipped.
		if !arrow {
			pl.CriticalDef -= v
		}
	case "perfect_hit_chance":
		if arrow {
			return
		}
		if o.Stacking == "highest" {
			// bPerfectHitRate keeps its highest value.
			pb.perfectHit = max(pb.perfectHit, v/100)
		} else {
			pb.perfectHitAdd += v / 100
		}
	case "double_attack_chance":
		if lr != lrNone {
			return
		}
		if o.Stacking == "highest" {
			pb.doubleRate = max(pb.doubleRate, v/100)
		} else {
			pb.doubleAddRate += v / 100
		}
	case "move_speed":
		if arrow {
			return
		}
		if o.Stacking == "highest" {
			// bSpeedRate keeps its highest increase.
			pb.speedRate = min(pb.speedRate, -v)
		} else {
			pb.speedAddRate -= v
		}
	case "aspd":
		if arrow {
			return
		}
		switch {
		case !o.Percent:
			pb.aspdAdd -= 10 * v
		case pb.p.t.renewalASPD:
			pb.aspdRate2 += v
		default:
			pb.aspdRate -= 10 * v
		}
	case "damage":
		pb.applyDamage(b, o, v, lr)
	case "damage_taken":
		pb.applyDamageTaken(b, o, v, lr)
	case "crit_damage":
		if !arrow {
			pl.CritAtkRate += v
		}
	case "non_crit_damage":
		if !arrow {
			pl.NonCritAtkRate += v
		}
	case "crit_damage_taken":
		// bCritDefRate, with the sign flipped.
		if !arrow {
			pl.CritDefRate -= v
		}
	case "damage_taken_cap":
		// bAbsorbDmgMaxHP2 keeps its highest value.
		pl.DamageTakenCap = max(pl.DamageTakenCap, v)
	case "always_endure":
		if !arrow {
			pb.noWalkDelay = true
		}
	case "ignore_size_penalty":
		if !arrow {
			pl.NoSizeFix = true
		}
	case "ignore_def":
		pb.applyIgnoreDef(o, v, lr, bonus.HasAmount(mustSpec(b.Name), toBonus(b)))
	case "ignore_res":
		if !arrow && o.Against != nil && o.Against.Key == "race" {
			battle.Add(&pl.IgnoreResRace, int(o.Against.Value), v)
		}
	case "def_as_damage":
		if o.Against == nil {
			return
		}
		h := pb.hand(lr)
		if h == nil {
			return
		}
		bit := uint32(1) << uint(o.Against.Value&31) // #nosec G115 -- element, race and class values are small.
		switch o.Against.Key {
		case "element":
			h.DefRatioEle |= bit
		case "race":
			h.DefRatioRace |= bit
		case "class":
			h.DefRatioClass |= bit
		}
	case "attack_element":
		if len(b.Args) == 0 {
			return
		}
		ele := clampInt(b.Args[0])
		switch lr {
		case lrArrow:
			if ammoWeapon(pb.p.worn.Weapon) {
				st.RHW.Ele = ele
			} else {
				pb.arrowEle = ele
			}
		case lrWeapon:
			st.LHW.Ele = ele
		default:
			st.RHW.Ele = ele
		}
	case "defense_element":
		if !arrow && len(b.Args) > 0 {
			st.DefEle = clampInt(b.Args[0])
		}
	case "max_weight":
		if !arrow {
			pb.addMaxWeight += v
		}
	case "status_resistance":
		// bResEff, added up and capped at 100%.
		if arrow || len(b.Args) < 2 {
			return
		}
		if pb.resEff == nil {
			pb.resEff = map[int]int{}
		}
		id := clampInt(b.Args[0])
		pb.resEff[id] = capValue(pb.resEff[id]+v, -10000, 10000)
	case "cause_status":
		pb.applyAddEff(b, o, lr)
	case "hp_gain_over_time", "sp_gain_over_time", "hp_loss_over_time", "sp_loss_over_time":
		// pc_bonus_regen: a value every interval, from 1 ms.
		if arrow || len(b.Args) < 2 {
			return
		}
		rate := clampInt(b.Args[1])
		if rate < 1 {
			return
		}
		pb.regen = append(pb.regen, RegenBonus{Kind: b.Name, Percent: o.Percent, Value: int(int16(v)), Rate: rate}) // #nosec G115 -- pc_bonus_regen keeps an int16.
	case "variable_cast_time", "fixed_cast_time", "after_cast_delay", "cooldown", "sp_cost":
		if !arrow {
			pb.applyCast(b.Name, o, v)
		}
	case "uninterruptible_cast":
		// bNoCastCancel.
		if !arrow {
			pl.Cast.NoCastCancel = true
		}
	case "uninterruptible_cast_everywhere":
		// bNoCastCancel2.
		if !arrow {
			pl.Cast.NoCastCancel2 = true
		}
	case "no_gemstone_cost":
		// bNoGemStone.
		if !arrow {
			pl.Cast.NoGemstone = true
		}
	case "no_knockback":
		// bNoKnockback.
		if !arrow {
			pb.noKnockback = true
		}
	case "unstrippable_weapon", "unstrippable_armor", "unstrippable_helm", "unstrippable_shield":
		// bUnstripableWeapon, bUnstripableArmor, bUnstripableHelm and
		// bUnstripableShield.
		if !arrow {
			pb.unstripable |= unstripablePos[b.Name]
		}
	case "steal_chance":
		// bAddStealRate.
		if !arrow {
			pb.addStealRate += v
		}
	case "healing_done":
		// bSkillHeal with a skill, bHealPower without.
		switch {
		case arrow:
		case o.Has("skill"):
			addItemBonus(&pl.SkillHeal, int(o.Skill), v, false)
		default:
			pb.addHealRate += v
			pl.AddHealRate += v
		}
	case "healing_received":
		// bSkillHeal2 with a skill, bHealPower2 without.
		switch {
		case arrow:
		case o.Has("skill"):
			addItemBonus(&pl.SkillHeal2, int(o.Skill), v, false)
		default:
			pb.addHeal2Rate += v
			pl.AddHeal2Rate += v
		}
	case "ignore_mdef":
		pb.applyIgnoreMDef(o, v, lr, bonus.HasAmount(mustSpec(b.Name), toBonus(b)))
	case "cast":
		pb.applyAutoSpell(b, o, lr)
	case "skill":
		// The skill command: an item grants a skill level while it is
		// worn, when it is higher than the learned one.
		if len(b.Args) >= 2 {
			id, lv := clampInt(b.Args[0]), clampInt(b.Args[1])
			if pb.p.granted == nil {
				pb.p.granted = map[int]int{}
			}
			pb.p.granted[id] = max(pb.p.granted[id], lv)
		}
	}
}

// hand returns the weapon data a bonus goes to for its lr flag: the
// right hand, or the left hand's for a left-hand weapon. Shields and
// arrows have none.
func (pb *pcBonus) hand(lr int) *battle.Hand {
	switch lr {
	case lrNone:
		return &pb.player.Right
	case lrWeapon:
		return &pb.player.Left
	}
	return nil
}

// applyIgnoreDef is bIgnoreDefEle, bIgnoreDefRace and bIgnoreDefClass
// without an amount, and bIgnoreDefRaceRate and bIgnoreDefClassRate with
// one.
func (pb *pcBonus) applyIgnoreDef(o *bonus.Options, v, lr int, withAmount bool) {
	if o.Against == nil || lr == lrArrow {
		return
	}
	pl := pb.player
	if withAmount {
		switch o.Against.Key {
		case "race":
			battle.Add(&pl.IgnoreDefRace, int(o.Against.Value), v)
		case "class":
			battle.Add(&pl.IgnoreDefClass, int(o.Against.Value), v)
		}
		return
	}
	h := pb.hand(lr)
	if h == nil {
		return
	}
	bit := uint32(1) << uint(o.Against.Value&31) // #nosec G115 -- element, race and class values are small.
	switch o.Against.Key {
	case "element":
		h.IgnoreDefEle |= bit
	case "race":
		h.IgnoreDefRace |= bit
	case "class":
		h.IgnoreDefClass |= bit
	}
}

// bfFlag turns the kind, range and attack options of a damage bonus back
// into rAthena's BF_ flags, with pc_bonus_addele's defaults.
func bfFlag(o *bonus.Options) int {
	const (
		bfWeapon = 0x0001
		bfMagic  = 0x0002
		bfMisc   = 0x0004
		bfShort  = 0x0010
		bfLong   = 0x0040
		bfSkill  = 0x0100
		bfNormal = 0x0200
	)
	flag := 0
	switch o.Kind {
	case "physical":
		flag |= bfWeapon
	case "magic":
		flag |= bfMagic
	case "misc":
		flag |= bfMisc
	default:
		flag |= bfWeapon | bfMagic | bfMisc
	}
	switch o.Range {
	case "melee":
		flag |= bfShort
	case "ranged":
		flag |= bfLong
	default:
		flag |= bfShort | bfLong
	}
	switch o.Attack {
	case "normal":
		flag |= bfNormal
	case "skill":
		flag |= bfSkill
	default:
		if flag&(bfMagic|bfMisc) != 0 {
			flag |= bfSkill
		}
		if flag&bfWeapon != 0 {
			flag |= bfNormal | bfSkill
		}
	}
	return flag
}

// addFlagged is pc_bonus_addele and its kin: an entry with the same key
// and flags gets the rate added, capped at 10000.
func addFlagged(list *[]battle.FlaggedRate, key, rate, flag int) {
	for i := range *list {
		e := &(*list)[i]
		if e.Key == key && e.Flag == flag {
			e.Rate = min(e.Rate+rate, 10000)
			return
		}
	}
	*list = append(*list, battle.FlaggedRate{Key: key, Rate: capValue(rate, -10000, 10000), Flag: flag})
}

// applyDamage is the physical part of the "damage" bonus: the sums of
// 0002a's "Groups" that normal attacks read. Magic sums wait for magic
// attacks.
func (pb *pcBonus) applyDamage(b *Bonus, o *bonus.Options, v, lr int) {
	pl := pb.player
	arrow := lr == lrArrow
	group := bonus.DamageGroup("damage", o)
	physical, _, _ := strings.Cut(group, "+")
	// lrHand is the weapon data of bAddRace and its kin: a shield's go to
	// the right hand.
	lrHand := &pl.Right
	if lr == lrWeapon {
		lrHand = &pl.Left
	}
	switch physical {
	case "bAddRace", "bAddClass", "bAddSize", "bAddEle":
		key := int(o.Against.Value)
		if arrow {
			m := map[string]*map[int]int{"bAddRace": &pl.Arrow.AddRace, "bAddClass": &pl.Arrow.AddClass,
				"bAddSize": &pl.Arrow.AddSize, "bAddEle": &pl.Arrow.AddEle}[physical]
			battle.Add(m, key, v)
			return
		}
		m := map[string]*map[int]int{"bAddRace": &lrHand.AddRace, "bAddClass": &lrHand.AddClass,
			"bAddSize": &lrHand.AddSize, "bAddEle": &lrHand.AddEle}[physical]
		battle.Add(m, key, v)
	case "bAddRace2":
		if arrow {
			battle.Add(&pl.Left.AddRace2, int(o.Against.Value), v)
		} else {
			battle.Add(&pl.Right.AddRace2, int(o.Against.Value), v)
		}
	case "bAddDamageClass":
		h := &pl.Right
		if lr != lrNone {
			h = &pl.Left
		}
		battle.Add(&h.AddDamage, int(o.Against.Value), v)
	case "bAddEle (bonus3)":
		if arrow {
			return
		}
		h := &pl.Right
		if lr != lrNone {
			h = &pl.Left
		}
		addFlagged(&h.AddEle2, int(o.Against.Value), v, bfFlag(o))
	case "bShortAtkRate":
		if !arrow {
			pl.ShortAtkRate += v
		}
	case "bLongAtkRate":
		if !arrow {
			pl.LongAtkRate += v
		}
	case "bWeaponDamageRate":
		if !arrow {
			battle.Add(&pl.WeaponDamageRate, int(o.WeaponType), v)
		}
	}
	if arrow {
		// The magic sums and bSkillAtk take nothing from ammunition.
		return
	}
	for _, sum := range strings.Split(group, "+") {
		m := &pl.Magic
		switch sum {
		case "bMagicAddRace":
			battle.Add(&m.AddRace, int(o.Against.Value), v)
		case "bMagicAddClass":
			battle.Add(&m.AddClass, int(o.Against.Value), v)
		case "bMagicAddSize":
			battle.Add(&m.AddSize, int(o.Against.Value), v)
		case "bMagicAddEle":
			battle.Add(&m.AddEle, int(o.Against.Value), v)
		case "bMagicAddRace2":
			battle.Add(&m.AddRace2, int(o.Against.Value), v)
		case "bAddMagicDamageClass":
			addItemBonus(&m.AddDamage, int(o.Against.Value), v, false)
		case "bMagicAtkEle":
			battle.Add(&m.AtkEle, int(o.Element), v)
		case "bSkillAtk":
			addItemBonus(&pl.SkillAtk, int(o.Skill), v, false)
		}
	}
}

// applyDamageTaken is the physical part of the "damage_taken" bonus. Its
// amount is the change, so rAthena's reduction is minus it.
func (pb *pcBonus) applyDamageTaken(b *Bonus, o *bonus.Options, v, lr int) {
	pl := pb.player
	sub := -v
	group := bonus.DamageGroup("damage_taken", o)
	// bSubSkill and bMagicSubDefEle also take ammunition's bonuses.
	switch group {
	case "bSubSkill":
		addItemBonus(&pl.SubSkill, int(o.Skill), sub, false)
		return
	case "bMagicSubDefEle":
		battle.Add(&pl.MagicSubDefEle, int(o.From.Value), sub)
		return
	}
	if lr == lrArrow {
		return
	}
	switch group {
	case "bMagicSubSize":
		battle.Add(&pl.MagicSubSize, int(o.From.Value), sub)
	case "bAddMDefMonster":
		addItemBonus(&pl.AddMDef, int(o.From.Value), sub, false)
	case "bMagicAtkDef":
		pl.MagicDefRate += sub
	case "bMiscAtkDef":
		pl.MiscDefRate += sub
	case "bSubRace":
		battle.Add(&pl.SubRace, int(o.From.Value), sub)
	case "bSubRace (bonus3)":
		addFlagged(&pl.SubRace3, int(o.From.Value), sub, bfFlag(o))
	case "bSubClass":
		battle.Add(&pl.SubClass, int(o.From.Value), sub)
	case "bSubSize":
		battle.Add(&pl.SubSize, int(o.From.Value), sub)
	case "bWeaponSubSize":
		battle.Add(&pl.WeaponSubSize, int(o.From.Value), sub)
	case "bSubEle":
		battle.Add(&pl.SubEle, int(o.Element), sub)
	case "bSubEle (bonus3)":
		addFlagged(&pl.SubEle2, int(o.Element), sub, bfFlag(o))
	case "bSubDefEle":
		battle.Add(&pl.SubDefEle, int(o.From.Value), sub)
	case "bSubRace2":
		battle.Add(&pl.SubRace2, int(o.From.Value), sub)
	case "bAddDefMonster":
		battle.Add(&pl.AddDef, int(o.From.Value), sub)
	case "bNearAtkDef":
		pl.NearAtkDef += sub
	case "bLongAtkDef":
		pl.LongAtkDef += sub
	}
}

// mustSpec returns a name's spec, or an empty one.
func mustSpec(name string) *bonus.Spec {
	if s, ok := bonus.Lookup(name); ok {
		return s
	}
	return &bonus.Spec{}
}

// toBonus makes the bonus.Bonus that bonus.HasAmount reads.
func toBonus(b *Bonus) *bonus.Bonus {
	out := &bonus.Bonus{Name: b.Name}
	for _, a := range b.Args {
		out.Args = append(out.Args, bonus.Num{Value: a})
	}
	return out
}

// applyAddEff is the bAddEff family through pc_bonus_addeff and
// pc_bonus_addeff_onskill: the options of cause_status (0002a, "Status
// effects and other procs") turn back into rAthena's ATF_ flags. Entries
// with the same status and flags add up their rates and keep the longer
// duration.
func (pb *pcBonus) applyAddEff(b *Bonus, o *bonus.Options, lr int) {
	if len(b.Args) == 0 {
		return
	}
	e := AddEffect{Status: clampInt(b.Args[0]), Duration: clampInt(o.Duration)}
	rate := clampInt(b.Chance)
	if e.Duration == 0 && pb.p.t.statuses != nil {
		// The Duration2 of the status's skill at level 7.
		if d := pb.p.t.statuses.Get(e.Status); d != nil {
			e.Duration = d.Duration2(7)
		}
	}
	if o.Skill != 0 {
		// bAddEffOnSkill.
		if lr == lrArrow {
			return
		}
		e.Skill, e.Rate = int(o.Skill), rate
		e.Flag = ATFTarget
		if o.Target == "self" {
			e.Flag = ATFSelf
		}
		for i := range pb.addEffOnSkill {
			x := &pb.addEffOnSkill[i]
			if x.Status == e.Status && x.Skill == e.Skill && x.Flag == e.Flag {
				x.Rate = min(x.Rate+e.Rate, math.MaxInt32)
				x.Duration = max(x.Duration, e.Duration)
				return
			}
		}
		pb.addEffOnSkill = append(pb.addEffOnSkill, e)
		return
	}
	whenHit := o.On == "hit_taken"
	if whenHit && lr == lrArrow {
		return
	}
	if lr == lrArrow {
		e.ArrowRate = rate
	} else {
		e.Rate = rate
	}
	switch o.Range {
	case "melee":
		e.Flag |= ATFShort
	case "ranged":
		e.Flag |= ATFLong
	default:
		e.Flag |= ATFShort | ATFLong
	}
	if o.Target == "self" {
		e.Flag |= ATFSelf
	} else {
		e.Flag |= ATFTarget
	}
	switch o.Kind {
	case "magic":
		e.Flag |= ATFMagic
	case "misc":
		e.Flag |= ATFMisc
	default:
		e.Flag |= ATFWeapon
	}
	list := &pb.addEff
	if whenHit {
		list = &pb.addEffWhenHit
	}
	for i := range *list {
		x := &(*list)[i]
		if x.Status == e.Status && x.Flag == e.Flag {
			x.Rate = min(x.Rate+e.Rate, math.MaxInt32)
			x.ArrowRate = min(x.ArrowRate+e.ArrowRate, math.MaxInt16)
			x.Duration = max(x.Duration, e.Duration)
			return
		}
	}
	*list = append(*list, e)
}

// addItemBonus is pc_bonus_itembonus: the value adds to the entry of an
// ID. With capRate, a sum is at most 10000, and a new entry is capped to
// -10000 and 10000.
func addItemBonus(m *map[int]int, id, v int, capRate bool) {
	if *m == nil {
		*m = map[int]int{}
	}
	old, ok := (*m)[id]
	switch {
	case !capRate:
		(*m)[id] = old + v
	case ok:
		(*m)[id] = min(old+v, 10000)
	default:
		(*m)[id] = capValue(v, -10000, 10000)
	}
}

// applyCast is pc_bonus and pc_bonus2 for the bonuses of cast times,
// delays, cooldowns and SP costs (0002a, "Speed, cast, and delay").
// Renewal's cast rules are RENEWAL_CAST; pre-renewal has no fixed cast
// time and ignores its bonuses.
func (pb *pcBonus) applyCast(name string, o *bonus.Options, v int) {
	cb := &pb.player.Cast
	renewal := pb.p.t.renewal
	skill := o.Has("skill")
	id := int(o.Skill)
	switch name {
	case "variable_cast_time":
		switch {
		case skill && o.Percent:
			// bVariableCastrate and bCastrate for a skill. Renewal keeps
			// the reduction.
			if renewal {
				addItemBonus(&cb.SkillCastRate, id, -v, true)
			} else {
				addItemBonus(&cb.SkillCastRate, id, v, true)
			}
		case skill:
			// bSkillVariableCast.
			addItemBonus(&cb.SkillVarCast, id, v, false)
		case o.Percent:
			// bVariableCastrate and bCastrate.
			if renewal {
				cb.VarCastRate -= v
			} else {
				cb.CastRate += v
			}
		default:
			// bVariableCast.
			cb.AddVarCast += v
		}
	case "fixed_cast_time":
		if !renewal {
			return
		}
		switch {
		case skill && o.Percent:
			// bFixedCastrate for a skill, as a reduction.
			addItemBonus(&cb.SkillFixCastRate, id, -v, true)
		case skill:
			// bSkillFixedCast.
			addItemBonus(&cb.SkillFixCast, id, v, false)
		case o.Percent:
			// bFixedCastrate keeps the lowest value.
			cb.FixCastRate = min(cb.FixCastRate, v)
		default:
			// bFixedCast.
			cb.AddFixCast += v
		}
	case "after_cast_delay":
		if skill {
			// bSkillDelay.
			addItemBonus(&cb.SkillDelay, id, v, false)
		} else {
			// bDelayrate, kept as a reduction.
			cb.DelayRate -= v
		}
	case "cooldown":
		// bSkillCooldown.
		addItemBonus(&cb.SkillCooldown, id, v, false)
	case "sp_cost":
		// The skill forms are reductions, so their amount is flipped.
		switch {
		case skill && o.Percent:
			// bSkillUseSPrate.
			addItemBonus(&cb.SkillUseSPRate, id, -v, true)
		case skill:
			// bSkillUseSP.
			addItemBonus(&cb.SkillUseSP, id, -v, false)
		default:
			// bUseSPrate.
			cb.SPRate += v
		}
	}
}

// applyIgnoreMDef is bIgnoreMdefEle and bIgnoreMdefRace without an
// amount, and bIgnoreMdefRaceRate and bIgnoreMdefClassRate with one.
// bIgnoreMdefRace2Rate has no field in battle.Player yet.
func (pb *pcBonus) applyIgnoreMDef(o *bonus.Options, v, lr int, withAmount bool) {
	if o.Against == nil || lr == lrArrow {
		return
	}
	pl := pb.player
	if withAmount {
		switch o.Against.Key {
		case "race":
			battle.Add(&pl.IgnoreMDefRaceRate, int(o.Against.Value), v)
		case "class":
			battle.Add(&pl.IgnoreMDefClassRate, int(o.Against.Value), v)
		}
		return
	}
	bit := uint32(1) << uint(o.Against.Value&31) // #nosec G115 -- element, race and class values are small.
	switch o.Against.Key {
	case "element":
		pl.IgnoreMDefEle |= bit
	case "race":
		pl.IgnoreMDefRace |= bit
	case "class":
		pl.IgnoreMDefClass |= bit
	}
}

// BF_ bits of autospells, as rAthena's e_battle_flag.
const (
	bfWeaponAS = 0x0001
	bfMagicAS  = 0x0002
	bfMiscAS   = 0x0004
	bfShortAS  = 0x0010
	bfLongAS   = 0x0040
	bfSkillAS  = 0x0100
	bfNormalAS = 0x0200
)

// applyAutoSpell is the "cast" bonus: bAutoSpell, bAutoSpellWhenHit and
// bAutoSpellOnSkill through pc_bonus_autospell and
// pc_bonus_autospell_onskill. The options turn back into rAthena's
// values:
//   - chance is per 10,000, and the rate per 1,000.
//   - on picks the list: "attack" (the default), "hit_taken" or
//     "skill_used", whose trigger is the skill option.
//   - target "enemy" or "self" sets the AUTOSPELL_FORCE_TARGET bit as the
//     list reads it. Without target, rAthena's bonus without a flag picks
//     the player for support skills and self skills, and the enemy for
//     others (Tables.SetAutoSpellSelf).
//   - random_level sets AUTOSPELL_FORCE_RANDOM_LEVEL.
//   - kind, range and attack are the BF_ flag, with pc_bonus_autospell's
//     defaults. A hit_taken bonus without attack is triggered by normal
//     attacks and skills, as bAutoSpellWhenHit without a flag.
func (pb *pcBonus) applyAutoSpell(b *Bonus, o *bonus.Options, lr int) {
	if lr == lrArrow || len(b.Args) < 2 {
		return
	}
	id, lv := clampInt(b.Args[0]), clampInt(b.Args[1])
	// The rate is an int16 in rAthena.
	rate := int(int16(clampInt(b.Chance) / 10)) // #nosec G115 -- rAthena keeps the rate in an int16.
	if rate == 0 {
		return
	}
	onSkill := o.On == "skill_used"
	// self is set when the skill is cast on the player.
	var self bool
	switch o.Target {
	case "self":
		self = true
	case "enemy":
		self = false
	default:
		self = pb.p.t.autoSpellSelf != nil && pb.p.t.autoSpellSelf(id)
	}
	flag := 0
	if self == onSkill {
		// AUTOSPELL_FORCE_TARGET casts on the enemy in the attack and
		// hit lists, and on the player in the skill list.
		flag |= AutoSpellForceTarget
	}
	if o.RandomLevel {
		flag |= AutoSpellForceRandomLevel
	}
	e := AutoSpell{Skill: id, Level: lv, Rate: capValue(rate, -1000, 1000), Flag: flag, Card: pb.cardID}
	if onSkill {
		e.TriggerSkill = int(o.Skill)
		pb.autoSpellOnSkill = append(pb.autoSpellOnSkill, e)
		return
	}

	bf := 0
	list := &pb.autoSpell
	if o.On == "hit_taken" {
		list = &pb.autoSpellWhenHit
		if o.Attack == "" {
			bf = bfNormalAS | bfSkillAS
		}
	}
	switch o.Kind {
	case "physical":
		bf |= bfWeaponAS
	case "magic":
		bf |= bfMagicAS
	case "misc":
		bf |= bfMiscAS
	}
	switch o.Range {
	case "melee":
		bf |= bfShortAS
	case "ranged":
		bf |= bfLongAS
	}
	switch o.Attack {
	case "normal":
		bf |= bfNormalAS
	case "skill":
		bf |= bfSkillAS
	}
	if bf&(bfShortAS|bfLongAS) == 0 {
		bf |= bfShortAS | bfLongAS
	}
	if bf&(bfWeaponAS|bfMagicAS|bfMiscAS) == 0 {
		bf |= bfWeaponAS
	}
	if bf&(bfSkillAS|bfNormalAS) == 0 {
		if bf&(bfMagicAS|bfMiscAS) != 0 {
			bf |= bfSkillAS
		}
		if bf&bfWeaponAS != 0 {
			bf |= bfNormalAS
		}
	}
	e.BattleFlag = bf
	for i := range *list {
		x := &(*list)[i]
		if (x.Card == e.Card || x.Rate < 0 || e.Rate < 0) && x.Skill == id && x.Level == lv && x.BattleFlag == bf && x.Flag == flag {
			// autospell_stacking is off: a second positive rate of the
			// same source is lost.
			if x.Rate > 0 && e.Rate > 0 {
				return
			}
			x.Rate = min(x.Rate+e.Rate, 1000)
			return
		}
	}
	*list = append(*list, e)
}

// unstripablePos are the EQP_ positions that the unstrippable bonuses
// keep on, as pc_bonus.
var unstripablePos = map[string]uint32{
	"unstrippable_weapon": item.EqpWeapon, "unstrippable_armor": item.EqpArmor,
	"unstrippable_helm": item.EqpHelm, "unstrippable_shield": item.EqpShield,
}
