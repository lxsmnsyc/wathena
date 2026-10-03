package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The Assassin and Assassin Cross skills, as the SkillImpl classes of
// rAthena's src/map/skills/thief, and the parts of their statuses that
// the zone runs: Cloaking's wall checks, Poison React's block, counter
// and boost, the poison of Enchant Poison and Enchant Deadly Poison on
// normal attacks, and Venom Splasher's explosion. Their damage formulas
// are in server/skill and server/battle. The masteries and Sonic
// Acceleration are passive and only change damage.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDCloaking:      cloaking{},
		skill.IDSonicBlow:     sonicBlow{},
		skill.IDGrimtooth:     grimtooth{},
		skill.IDEnchantPoison: enchantPoison{},
		skill.IDPoisonReact:   statusSkill{},
		skill.IDVenomDust:     unitSkillImpl{},
		skill.IDVenomSplasher: venomSplasher{},
		skill.IDVenomKnife:    venomKnife{},
		skill.IDEnchantDeadly: enchantDeadlyPoison{},
		skill.IDSoulBreaker:   attackSkill{},
		skill.IDMeteorAssault: meteorAssault{},
		skill.IDCreateDeadly:  createDeadlyPoison{},
	})
}

// Skills, items and settings the Assassin skills read.
const (
	idSoulLinkAssassin = 457 // SL_ASSASIN
	itemPoisonBottle   = 678 // ITEMID_POISON_BOTTLE
	elePoison          = 5   // ELE_POISON
	// edpItemGroup is IG_EDP: the items Enchant Deadly Poison uses in
	// renewal.
	edpItemGroup = "EDP"
	// pcCloakCheckType is player_cloak_check_type: 1 checks walls.
	pcCloakCheckType = 1
	// switchRemoveEDP is switch_remove_edp: 2 ends EDP when the weapon
	// comes off, and keeps a player without a weapon from using it, in
	// renewal only (1 would do it in pre-renewal).
	switchRemoveEDP = 2
)

// cloaking is SkillCloaking: a toggle of SC_CLOAKING.
type cloaking struct{}

// castBegin is skill_check_condition_castbegin for AS_CLOAKING: below
// level 3, a wall must be next to the caster.
func (cloaking) castBegin(z *Zone, src *entity, _ *skill.Def, lv int) wathenav1.SkillFailure {
	if lv < 3 && pcCloakCheckType&1 != 0 && !z.nearWall(src) {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (cloaking) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if target.sc.Has(d.Status) {
		ok := z.scEnd(target, d.Status)
		if ok {
			z.skillEffect(src, target, d, lv, int64(lv), true)
		} else if src.player != nil {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		}
		return
	}
	if z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv))) {
		z.skillEffect(src, target, d, lv, int64(lv), true)
	} else if src.player != nil {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	}
}

// nearWall is the wall check of skill_check_cloaking: a cell next to the
// unit cannot be entered (CELL_CHKNOPASS).
func (z *Zone) nearWall(e *entity) bool {
	for d := range 8 {
		if z.grid.NoPass(e.x+dirX[d], e.y+dirY[d]) {
			return true
		}
	}
	return false
}

// NearWall is statuschange.WallChecker for the zone's map.
func (v scEnv) NearWall(u sc.Unit) bool { return v.z.nearWall(u.(scUnit).e) }

// cloakingMoved is map_moveblock's Cloaking check for a unit that moved
// a cell.
func (z *Zone) cloakingMoved(e *entity) {
	if en := z.statusEngine(); en != nil && e.sc.Has(sc.SCCloaking) {
		en.CloakingMoved(z.unit(e))
	}
}

// cloakingWalk is clif_parse_WalkToXY's Cloaking check: a walk request
// checks the walls, as skill_check_cloaking with the status.
func (z *Zone) cloakingWalk(e *entity) {
	if en := z.statusEngine(); en != nil && e.sc.Has(sc.SCCloaking) {
		en.CheckCloaking(z.unit(e))
	}
}

// sonicBlow is SkillSonicBlow: an eight hit weapon attack that may stun.
type sonicBlow struct{ attackSkill }

// additionalEffect is SkillSonicBlow::applyAdditionalEffects. The
// Assassin's soul link doubles the chance; there are no guild wars or
// battlegrounds where it would not.
func (sonicBlow) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	rate := 2*lv + 10
	if src.sc.Has(sc.SCSpirit) && src.sc.Val2(sc.SCSpirit) == idSoulLinkAssassin {
		rate = 4*lv + 20
	}
	z.scStart(src, target, sc.SCStun, rate, lv, int64(d.Duration2.At(lv)))
}

// grimtooth is SkillGrimtooth: a splash around the target, which the
// caster uses while hidden (its requirement), with a preamble.
type grimtooth struct{}

func (grimtooth) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.recursiveSplash(src, target, d, lv, *flag|sdPreamble)
}

// additionalEffect is SkillGrimtooth::applyAdditionalEffects: a monster
// that is not immune to statuses is slowed by Quagmire.
func (grimtooth) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if target.monster == nil || target.status == nil || target.status.Mode&modeStatusImmune != 0 {
		return
	}
	z.scStart(src, target, sc.SCQuagmire, 100, 0, int64(d.Duration2.At(lv)))
}

// enchantPoison is SkillEnchantPoison: the target's weapon turns to
// poison.
type enchantPoison struct{}

func (enchantPoison) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv))) {
		z.skillEffect(src, target, d, lv, int64(lv), true)
		return
	}
	z.skillEffect(src, target, d, lv, int64(lv), false)
	if src.player != nil {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	}
}

// venomSplasher is SkillVenomSplasher: a mark on the target that
// explodes when its time runs out (SC_SPLASHER), as a splash around it.
type venomSplasher struct{}

// castendNoDamage is SkillVenomSplasher::castendNoDamageId. Bosses
// cannot be marked, and in pre-renewal neither can a target above 3/4 of
// its HP.
func (venomSplasher) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	st := target.status
	if st == nil || st.Mode&modeStatusImmune != 0 || !z.skillRenewal(skill.IDVenomSplasher) && target.hp > int64(st.MaxHP)*3/4 {
		if src.player != nil {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		}
		return
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	// sc_start4 with the time as val4 and a first timer after a second.
	su := z.unit(src)
	ok := en.Start(su, z.unit(target), sc.Start{Type: d.Status, Rate: 10000, Val1: lv, Val2: d.ID, Val3: int(su.Ref().ID),
		Val4: d.Duration1.At(lv), Duration: 1000, Reason: sc.ReasonSkill, Source: su.Ref()})
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// castendDamage is SkillVenomSplasher::castendDamageId: the explosion,
// which the status casts when it ends. It does not use a second gem.
func (venomSplasher) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.recursiveSplash(src, target, d, lv, *flag)
	if *flag&1 == 0 {
		*flag |= skillNoConsumeReq
	}
}

// searchSize is SkillVenomSplasher::getSearchSize: the targets that
// share the damage are counted in a 3x3 square.
func (venomSplasher) searchSize(int) int { return 1 }

// additionalEffect is SkillVenomSplasher::applyAdditionalEffects.
func (venomSplasher) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart4(src, target, sc.SCPoison, 100, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration2.At(lv)))
}

// CastendDamage is statuschange.SkillCaster: a status's skill, such as
// Venom Splasher's explosion, as skill_castend_damage_id.
func (v scEnv) CastendDamage(src sc.Source, target sc.Unit, id, lv, flag int) {
	z := v.z
	t := target.(scUnit).e
	e, err := scriptHost{z}.entityOf(entityRef(src))
	if err != nil || z.Skills == nil {
		return
	}
	d := z.Skills.Get(id)
	if d == nil {
		return
	}
	z.castendDamage(e, t, d, lv, flag)
}

// venomKnife is SkillThrowVenomKnife: a thrown knife that poisons.
type venomKnife struct{ attackSkill }

// additionalEffect is SkillThrowVenomKnife::applyAdditionalEffects.
func (venomKnife) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart4(src, target, sc.SCPoison, 100, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration2.At(lv)))
}

// enchantDeadlyPoison is SkillEnchantDeadlyPoison: SC_EDP, and 25% of
// the weapon's ATK as poison damage (SC_SUB_WEAPONPROPERTY in renewal,
// SC_WATK_ELEMENT in pre-renewal).
type enchantDeadlyPoison struct{ statusSkill }

// castBegin is skill_check_condition_castbegin for ASC_EDP: in renewal,
// a player without a weapon cannot use it.
func (enchantDeadlyPoison) castBegin(z *Zone, src *entity, d *skill.Def, _ int) wathenav1.SkillFailure {
	bit := 1
	if z.skillRenewal(skill.IDEnchantDeadly) {
		bit = 2
	}
	if st := src.status; switchRemoveEDP&bit != 0 && st != nil && st.Player != nil && st.Player.RightWeapon == battle.WeaponFist {
		return wathenav1.SkillFailure_SKILL_FAILURE_THIS_WEAPON
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

// castEnd is skill_check_condition_castend for ASC_EDP: in renewal it
// uses an item of the EDP group, such as a Poison Bottle, which
// skill_db does not list.
func (enchantDeadlyPoison) castEnd(z *Zone, src *entity, d *skill.Def, _ int) wathenav1.SkillFailure {
	p := src.player
	if !z.skillRenewal(skill.IDEnchantDeadly) || p == nil {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	for slot, s := range p.inventory.Slots {
		if s.ID != 0 && s.Amount > 0 && z.Mods != nil && z.Mods.Groups != nil && z.Mods.Groups.Contains(edpItemGroup, s.ID) {
			z.deleteItem(p, slot, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
			return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
		}
	}
	// rAthena names the Poison Bottle it needs (USESKILL_FAIL_NEED_ITEM).
	return wathenav1.SkillFailure_SKILL_FAILURE_STUFF_INSUFFICIENT
}

func (s enchantDeadlyPoison) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	s.statusSkill.castendNoDamage(z, src, target, d, lv, flag)
	if z.skillRenewal(skill.IDEnchantDeadly) {
		z.scStart4(src, src, sc.SCSubWeaponPro, 100, elePoison, 25, d.ID, 0, int64(d.Duration1.At(lv)))
	} else {
		z.scStart4(src, src, sc.SCWatkElement, 100, elePoison, 25, 0, 0, int64(d.Duration1.At(lv)))
	}
}

// meteorAssault is SkillMeteorAssault: a splash around the caster.
type meteorAssault struct{}

func (meteorAssault) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	// skill_castend_damage_id on the caster: the splash around it.
	z.recursiveSplash(src, target, d, lv, *flag)
}

// additionalEffect is SkillMeteorAssault::applyAdditionalEffects: one of
// Blind, Stun or Bleeding, each with the time of its own level.
func (meteorAssault) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	rate := 5 + lv*5
	switch z.rand.IntN(3) {
	case 0:
		z.scStart(src, target, sc.SCBlind, rate, lv, int64(d.Duration2.At(1)))
	case 1:
		z.scStart(src, target, sc.SCStun, rate, lv, int64(d.Duration2.At(2)))
	default:
		z.scStart4(src, target, sc.SCBleeding, rate, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration2.At(3)))
	}
}

// createDeadlyPoison is SkillCreateDeadlyPoison: skill_produce_mix of a
// Poison Bottle, which needs no window.
type createDeadlyPoison struct{}

func (createDeadlyPoison) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	if z.produceDeadlyPoison(p) {
		z.skillEffect(src, target, d, lv, int64(lv), true)
		return
	}
	z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_STUFF_INSUFFICIENT)
}

// deadlyPoisonRecipe is the Poison Bottle's entry of produce_db.txt (the
// same in both modes): Venom Canine, Cactus Needle, Bee Sting, Poison
// Spore, Karvodailnirol, Berserk Potion and an Empty Bottle, one each,
// for a player with ASC_CDP.
var deadlyPoisonRecipe = []uint32{937, 952, 939, 7033, 972, 657, 713}

// produceDeadlyPoison is skill_produce_mix for ASC_CDP with one Poison
// Bottle: skill_can_produce_mix checks the skill and the materials, then
// the materials are used. It succeeds at 20% plus 0.4% per DEX and 0.2%
// per LUK. A failure costs a quarter of the player's max HP. It returns
// whether the bottle was made.
func (z *Zone) produceDeadlyPoison(p *Player) bool {
	if z.Items == nil || p.skillLevel(skill.IDCreateDeadly) < 1 {
		return false
	}
	if _, ok := z.Items.Lookup(itemPoisonBottle); !ok {
		return false
	}
	for _, id := range deadlyPoisonRecipe {
		if z.countItem(p, id) < 1 {
			return false
		}
	}
	for _, id := range deadlyPoisonRecipe {
		if _, ok := z.Items.Lookup(id); ok {
			z.takeItems(p, id, 1)
		}
	}
	st := p.entity.status
	if st == nil {
		return false
	}
	makePer := max(2000+40*st.Dex+20*st.Luk, 1)
	if z.rand.IntN(10000) < makePer {
		// A full inventory loses the bottle (skill_drop_items_full is
		// off).
		z.addItem(p, itemPoisonBottle, 1, skillOrigin(skill.IDCreateDeadly))
		return true
	}
	// status_percent_damage of 25% of max HP, which can kill.
	scEnv{z}.Damage(z.unit(p.entity), int64(st.MaxHP)*25/100, 0, true, false)
	return false
}

// poisonReactBlock is Poison React's part of battle_weapon_attack: a
// normal attack of the poison element, or by a monster of the poison
// element, on a unit waiting with Poison React is blocked. The unit then
// boosts its next normal attack and attacks the attacker once.
func (z *Zone) poisonReactBlock(src, target *entity) bool {
	e := target.sc.Get(sc.SCPoisonreact)
	if e == nil || e.Val4 != 0 || src.status == nil {
		return false
	}
	if (src.player != nil || src.status.DefEle != elePoison) && src.status.RHW.Ele != elePoison {
		return false
	}
	e.Val4 = 1
	z.refreshChanges(target)
	// unit_attack without continuing.
	if !target.dead && z.canTarget(target, src) {
		target.target, target.repeat = src, false
	}
	return true
}

// poisonReactBoost is the rest of Poison React's part of
// battle_calc_attack_skill_ratio: a boosted normal attack that hit may
// poison the target, and ends the status.
func (z *Zone) poisonReactBoost(src, target *entity, r battle.Result) {
	e := src.sc.Get(sc.SCPoisonreact)
	if e == nil || e.Val4 != 1 || r.Kind != battle.Hit && r.Kind != battle.Critical || infiniteDefense(target, r.Flag) {
		return
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	d := z.Skills.Get(skill.IDPoisonReact)
	su := z.unit(src)
	delay := int64(0)
	if src.status != nil {
		delay = int64(src.status.AMotion)
	}
	dur := int64(0)
	if d != nil {
		dur = int64(d.Duration2.At(e.Val1))
	}
	en.Start(su, z.unit(target), sc.Start{Type: sc.SCPoison, Rate: e.Val3 * 100, Val1: e.Val1, Val2: int(su.Ref().ID),
		Duration: dur, Delay: delay, Reason: sc.ReasonSkill, Source: su.Ref()})
	z.scEnd(src, sc.SCPoisonreact)
}

// infiniteDefense is is_infinite_defense for a weapon attack's flags.
func infiniteDefense(t *entity, flag int) bool {
	st := t.status
	if st == nil {
		return false
	}
	if st.Mode&battle.ModeIgnoreMelee != 0 && flag&(battle.FlagWeapon|battle.FlagShort) == battle.FlagWeapon|battle.FlagShort {
		return true
	}
	return st.Mode&battle.ModeIgnoreRanged != 0 && flag&(battle.FlagWeapon|battle.FlagLong) == battle.FlagWeapon|battle.FlagLong
}

// poisonReactCounter is Poison React's part of
// battle_calc_weapon_final_atk_modifiers: a weapon attack that deals
// damage to a unit with Poison React has a chance to make it strike back
// with a level 5 Envenom. Each try uses a counter, even when the strike
// cannot be made.
func (z *Zone) poisonReactCounter(src, target *entity, r battle.Result) {
	e := target.sc.Get(sc.SCPoisonreact)
	if e == nil || r.Damage-r.Left <= 0 || target.dead || src.dead {
		return
	}
	// rnd_chance_official(val3, 100).
	if z.rand.IntN(20001)%100 >= e.Val3 {
		return
	}
	if d := z.Skills.Get(skill.IDEnvenom); d != nil && z.statusCheckSkillUse(target, src, d, 0) {
		z.skillAttack(battle.TypeWeapon, target, target, src, d, 5, 0)
	}
	if e = target.sc.Get(sc.SCPoisonreact); e != nil {
		e.Val2--
		if e.Val2 <= 0 {
			z.scEnd(target, sc.SCPoisonreact)
		}
	}
}

// normalAttackPoison is the part of skill_additional_effect for normal
// attacks that statuses of the attacker cause: Enchant Poison's poison
// (its chance is per 10,000) and Enchant Deadly Poison's deadly poison.
func (z *Zone) normalAttackPoison(src, target *entity) {
	en := z.statusEngine()
	if en == nil || src.sc.Empty() || target.dead {
		return
	}
	su, tu := z.unit(src), z.unit(target)
	if e := src.sc.Get(sc.SCEncpoison); e != nil {
		dur := 0
		if d := z.Skills.Get(skill.IDEnchantPoison); d != nil {
			dur = d.Duration2.At(e.Val1)
		}
		en.Start(su, tu, sc.Start{Type: sc.SCPoison, Rate: e.Val2, Val1: e.Val1, Val2: int(su.Ref().ID),
			Duration: int64(dur), Reason: sc.ReasonSkill, Source: su.Ref()})
	}
	if e := src.sc.Get(sc.SCEdp); e != nil && !target.dead {
		dur := 0
		if d := z.Skills.Get(skill.IDEnchantDeadly); d != nil {
			dur = d.Duration2.At(e.Val1)
		}
		z.scStart4(src, target, sc.SCDpoison, e.Val2, e.Val1, int(su.Ref().ID), 0, 0, int64(dur))
	}
}

// weaponAttackCloaking is battle_weapon_attack's Cloaking part: a normal
// attack ends the attacker's Cloaking, unless its val4 has bit 2.
func (z *Zone) weaponAttackCloaking(src *entity) {
	if e := src.sc.Get(sc.SCCloaking); e != nil && e.Val4&2 == 0 {
		z.scEnd(src, sc.SCCloaking)
	}
}

// castCloaking is unit_skilluse_id2's and unit_skilluse_pos2's Cloaking
// part: a cast ends the caster's Cloaking, unless its val4 has bit 4. A
// cast of Cloaking on a target keeps it, for the toggle.
func (z *Zone) castCloaking(src *entity, d *skill.Def, ground bool) {
	if e := src.sc.Get(sc.SCCloaking); e != nil && e.Val4&4 == 0 && (ground || d.ID != skill.IDCloaking) {
		z.scEnd(src, sc.SCCloaking)
	}
}

// unequipStatuses is the status part of pc_unequipitem for an item taken
// off a position:
//   - Taking off the right hand's item ends EDP in renewal
//     (switch_remove_edp).
//   - Statuses with RemoveOnUnequip end, those with
//     RemoveOnUnequipWeapon when a weapon leaves either hand, such as
//     Enchant Poison, and those with RemoveOnUnequipArmor when the armor
//     comes off.
func (z *Zone) unequipStatuses(p *Player, slot int, pos uint32) {
	e := p.entity
	if e.sc.Empty() || z.Statuses == nil {
		return
	}
	bit := 1
	if z.skillRenewal(skill.IDEnchantDeadly) {
		bit = 2
	}
	if pos&item.EqpHandR != 0 && switchRemoveEDP&bit != 0 {
		z.scEnd(e, sc.SCEdp)
	}
	weapon := false
	if z.Items != nil && slot >= 0 && slot < len(p.inventory.Slots) {
		weapon = z.Items.Get(p.inventory.Slots[slot].ID).IsWeapon()
	}
	for _, en := range e.sc.Sorted() {
		def := z.Statuses.Get(en.Type)
		if def == nil {
			continue
		}
		if def.Flags.Has(sc.FlagRemoveOnUnequip) || pos&item.EqpArms != 0 && weapon && def.Flags.Has(sc.FlagRemoveOnUnequipWeapon) ||
			pos&item.EqpArmor != 0 && def.Flags.Has(sc.FlagRemoveOnUnequipArmor) {
			z.scEnd(e, en.Type)
		}
	}
}
