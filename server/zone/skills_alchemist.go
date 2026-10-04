package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The Alchemist and Creator skills, as the SkillImpl classes of rAthena's
// src/map/skills/merchant, and the systems they need:
//   - skill_break_equip, which Bomb, Acid Terror and Acid Demonstration
//     roll and Chemical Protection blocks;
//   - potion_flag, the thrown potions of Aid Potion, Aid Berserk Potion
//     and Aid Condensed Potion, which run the potion's item script and
//     keep its heal;
//   - the summons of Summon Flora, Summon Marine Sphere and Plant
//     Cultivation, with their master and delete timer;
//   - skill_produce_mix for Twilight Alchemy.
//
// Their damage formulas are in server/skill/alchemist.go. Axe Mastery,
// Potion Research and Bioethics are passive.
//
// Skills that wait for another system fail before they start, so they
// cost nothing:
//   - Prepare Potion opens a recipe window (clif_skill_produce_mix_list),
//     which the client does not have yet.
//   - Call Homunculus, Vaporize and Homunculus Resurrection need a
//     homunculus, which does not exist yet.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDPharmacy:          notPorted{},
		skill.IDBomb:              bomb{},
		skill.IDAcidTerror:        acidTerror{},
		skill.IDPotionPitcher:     aidPotion{},
		skill.IDCannibalize:       summonPlant{},
		skill.IDSphereMine:        summonPlant{sphere: true},
		skill.IDCPWeapon:          chemicalProtection{pos: item.EqpWeapon},
		skill.IDCPShield:          chemicalProtection{pos: item.EqpShield},
		skill.IDCPArmor:           chemicalProtection{pos: item.EqpArmor},
		skill.IDCPHelm:            chemicalProtection{pos: item.EqpHeadTop},
		skill.IDCallHomunculus:    notPorted{},
		skill.IDVaporize:          notPorted{},
		skill.IDResurrectHomun:    notPorted{},
		skill.IDBerserkPitcher:    aidPotion{berserk: true},
		skill.IDTwilight1:         twilightAlchemy{},
		skill.IDTwilight2:         twilightAlchemy{},
		skill.IDTwilight3:         twilightAlchemy{},
		skill.IDSlimPitcher:       aidCondensedPotion{},
		skill.IDFullProtection:    fullProtection{},
		skill.IDAcidDemonstration: acidDemonstration{},
		skill.IDCultivation:       plantCultivation{},
	})
}

// Statuses, skills, monsters and items the Alchemist skills read.
const (
	scExtremityFist     = 91  // SC_EXTREMITYFIST
	scMeltdown          = 122 // SC_MELTDOWN
	scWaterInsignia     = 515 // SC_WATER_INSIGNIA
	idSoulLinkAlchemist = 445 // SL_ALCHEMIST
	idStripWeapon       = 215 // RG_STRIPWEAPON
	idStripShield       = 216 // RG_STRIPSHIELD
	idStripArmor        = 217 // RG_STRIPARMOR
	idStripHelm         = 218 // RG_STRIPHELM
	mobEmperium         = 1288
	mobMarineSphere     = 1142 // MOBID_MARINE_SPHERE
	mobBlackMushroom    = 1084 // MOBID_BLACK_MUSHROOM
	mobRedPlant         = 1078 // MOBID_RED_PLANT
	mobBluePlant        = 1079 // MOBID_BLUE_PLANT
	mobGreenPlant       = 1080 // MOBID_GREEN_PLANT
	mobYellowPlant      = 1081 // MOBID_YELLOW_PLANT
	mobWhitePlant       = 1082 // MOBID_WHITE_PLANT
	mobShiningPlant     = 1083 // MOBID_SHINING_PLANT
	// emotionHuk is ET_HUK.
	emotionHuk = 23
	// alchemistSummonSetting is alchemist_summon_setting: 15 in
	// pre-renewal, 12 in renewal. Bit 1 lets players hit floras, bit 2
	// lets them hit marine spheres, and bit 4 keeps a caster from having
	// floras of different kinds out.
	alchemistSummonSettingPre = 15
	alchemistSummonSettingRe  = 12
)

// floraSummons are SkillSummonFlora's monsters by level: Mandragora,
// Hydra, Flora, Parasite and Geographer of the Alchemist.
var floraSummons = [5]uint32{1589, 1579, 1575, 1555, 1590}

// Summoned monsters' AI, as rAthena's e_mob_ai.
const (
	aiNone   = 0
	aiAttack = 1 // AI_ATTACK
	aiFlora  = 2 // AI_FLORA
	aiSphere = 3 // AI_SPHERE
)

// summonSetting is alchemist_summon_setting in the zone's mode.
func (z *Zone) summonSetting() int {
	if z.renewal() {
		return alchemistSummonSettingRe
	}
	return alchemistSummonSettingPre
}

// bomb is SkillBomb: a ground unit that burns the enemies around it. The
// unit takes the ammunition's place (flag 1), as Bomb's castendPos2.
type bomb struct{ unitSkillImpl }

// additionalEffect is SkillBomb::applyAdditionalEffects: a chance to
// break the target's weapon, 3% per level in renewal and 1% in
// pre-renewal.
func (bomb) additionalEffect(z *Zone, src, target *entity, _ *skill.Def, lv int) {
	rate := 100 * lv
	if z.skillRenewal(skill.IDBomb) {
		rate = 300 * lv
	}
	z.breakEquip(src, target, item.EqpWeapon, rate, bctEnemy)
}

// acidTerror is SkillAcidTerror: a weapon hit that may cause bleeding
// and break the target's armor.
type acidTerror struct{ attackSkill }

// additionalEffect is SkillAcidTerror::applyAdditionalEffects.
func (acidTerror) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.acidTerrorEffect(src, target, d, lv)
}

// acidTerrorEffect is Acid Terror's bleeding (3% per level) and armor
// break: in renewal 10% per level less 5%, in pre-renewal its Duration1
// in hundredths. The target shows ET_HUK when its armor breaks.
func (z *Zone) acidTerrorEffect(src, target *entity, d *skill.Def, lv int) {
	z.scStart4(src, target, sc.SCBleeding, lv*3, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration2.At(lv)))
	rate := 100 * d.Duration1.At(lv)
	if z.skillRenewal(skill.IDAcidTerror) {
		rate = 1000*lv + 500 - 1000
	}
	if z.breakEquip(src, target, item.EqpArmor, rate, bctEnemy) != 0 {
		z.emotions = append(z.emotions, npcEmotion{target, emotionHuk})
	}
}

// acidDemonstration is SkillAcidDemonstration: a hit of the skill's type
// (a weapon hit in renewal, a misc hit in pre-renewal) that may break
// the weapon and the armor.
type acidDemonstration struct{ attackSkill }

// additionalEffect is SkillAcidDemonstration::applyAdditionalEffects: 1%
// per level for each.
func (acidDemonstration) additionalEffect(z *Zone, src, target *entity, _ *skill.Def, lv int) {
	z.breakEquip(src, target, item.EqpWeapon|item.EqpArmor, 100*lv, bctEnemy)
}

// chemicalProtection is SkillAlchemicalWeapon, SkillSynthesizedShield,
// SkillSyntheticArmor and SkillBiochemicalHelm: the skill's status on a
// player who wears an item at the position. A player caster's skill
// fails on anyone else, and keeps its items.
type chemicalProtection struct{ pos uint32 }

func (c chemicalProtection) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if src.player != nil && (target.player == nil || z.checkEquip(target.player, c.pos) < 0) {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		*flag |= skillNoConsumeReq
		return
	}
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// checkEquip is pc_checkequip: the inventory slot of the item worn at a
// position, or -1.
func (z *Zone) checkEquip(p *Player, pos uint32) int {
	if p.inventory == nil {
		return -1
	}
	for i, s := range p.inventory.Slots {
		if s.ID != 0 && s.Equip&pos != 0 {
			return i
		}
	}
	return -1
}

// fullProtection is SkillFullProtection: the four Chemical Protection
// statuses, each on a player who wears an item at its position.
type fullProtection struct{}

func (fullProtection) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	positions := [4]uint32{item.EqpWeapon, item.EqpShield, item.EqpArmor, item.EqpHeadTop}
	statuses := [4]int{scCPWeapon, scCPShield, scCPArmor, scCPHelm}
	started := 0
	for i, pos := range positions {
		if target.player == nil || z.checkEquip(target.player, pos) < 0 {
			continue
		}
		z.scStart(src, target, statuses[i], 100, lv, int64(d.Duration1.At(lv)))
		started++
	}
	if src.player != nil && started == 0 {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		*flag |= skillNoConsumeReq
		return
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// potionRun is rAthena's potion_flag, potion_target and the potion_hp,
// potion_sp, potion_per_hp and potion_per_sp they fill. While capture is
// set, an item script's heal is kept here instead of healing, and its
// statuses go on target for half their time (buildin_itemheal,
// buildin_percentheal, buildin_sc_start and buildin_sc_end).
type potionRun struct {
	capture              bool
	target               *entity
	hp, sp, perHP, perSP int
}

// runPotion runs an item's script with potion_flag set and returns what
// it heals. A script that cannot run heals nothing, as in rAthena an item
// without a heal.
func (z *Zone) runPotion(p *Player, id uint32, target *entity) potionRun {
	z.potion = potionRun{capture: true, target: target}
	if d, ok := z.Items.Lookup(id); ok && d.Script != "" && d.Unsupported == "" && !z.inHost() {
		if r := z.itemScripts(); r != nil {
			if err := r.UseItem(int64(p.charID), id, d.Script); err != nil && z.Mods != nil && z.Mods.Log != nil {
				z.Mods.Log.Warn("potion script failed", "map", z.Map, "item", d.AegisName, "err", err)
			}
		}
	}
	got := z.potion
	z.potion.capture, z.potion.target = false, nil
	return got
}

// potionHeal is the itemheal or percentheal of a script run with
// potion_flag: it reports whether it kept the heal.
func (z *Zone) potionHeal(percent bool, hp, sp [2]int) bool {
	if !z.potion.capture {
		return false
	}
	h := item.Heal{HP: hp, SP: sp}
	v, w := h.Roll(z.rand)
	if percent {
		z.potion.perHP, z.potion.perSP = v, w
	} else {
		z.potion.hp, z.potion.sp = v, w
	}
	return true
}

// potionStatusTarget is buildin_sc_start's and buildin_sc_end's
// potion_target: the unit a thrown potion's status goes on, or nil.
func (z *Zone) potionStatusTarget() *entity {
	if !z.potion.capture {
		return nil
	}
	return z.potion.target
}

// aidPotion is SkillAidPotion and SkillAidBerserkPotion: the potion of
// the level is thrown at the target, which it heals with the caster's
// skill bonuses. Its statuses go on the target for half their time.
type aidPotion struct{ berserk bool }

func (a aidPotion) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if target.monster != nil && target.monster.spawn.Mob == mobEmperium {
		*flag |= skillNoConsumeReq
		return
	}
	var hp, sp int
	tst := target.status
	if tst == nil {
		return
	}
	if p := src.player; p != nil {
		x := lv%11 - 1
		it := d.Requires.Items[max(x, 0)]
		id := uint32(it[0]) // #nosec G115 -- item IDs are positive.
		if x < 0 || it[0] <= 0 || z.countItem(p, id) < it[1] {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			*flag |= skillNoConsumeReq
			return
		}
		if a.berserk {
			// A target below the potion's level cannot drink it.
			if idef, ok := z.Items.Lookup(id); ok && target.player != nil && z.unit(target).Level() < idef.MinLevel {
				z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
				*flag |= skillNoConsumeReq
				return
			}
		}
		pr := z.runPotion(p, id, target)
		bonus := 100
		if e := src.sc.Get(sc.SCSpirit); e != nil && e.Val2 == idSoulLinkAlchemist {
			bonus += z.unit(src).Level()
		}
		skills := 100 + p.skillLevel(skill.IDPotionPitcher)*10 + p.skillLevel(skill.IDLearningPotion)*5
		if pr.perHP > 0 || pr.perSP > 0 {
			hp = tst.MaxHP * pr.perHP / 100
			hp = hp * skills * bonus / 10000
			if target.player != nil {
				sp = tst.MaxSP * pr.perSP / 100
				sp = sp * skills * bonus / 10000
			}
		} else {
			if pr.hp > 0 {
				hp = pr.hp * skills * bonus / 10000
				hp = hp * (100 + tst.Vit*2) / 100
				if tp := target.player; tp != nil {
					hp = hp * (100 + tp.skillLevel(skill.IDRecovery)*10) / 100
				}
			}
			if pr.sp > 0 {
				sp = pr.sp * skills * bonus / 10000
				sp = sp * (100 + tst.Int*2) / 100
				if tp := target.player; tp != nil {
					sp = sp * (100 + tp.skillLevel(skill.IDSPRecovery)*10) / 100
				}
			}
		}
		// The item group heal rates (bAddItemGroupHealRate) are not
		// ported.
		if b := skill.SkillHealBonus(src.status.Player, d.ID); b != 0 {
			hp += hp * b / 100
			sp += sp * b / 100
		}
	} else {
		// A monster's potion, by level.
		switch lv {
		case 1:
			hp = 45
		case 2:
			hp = 105
		case 3:
			hp = 175
		default:
			hp = 325
		}
		hp = (hp + z.rand.IntN(lv*20+1)) * (150 + lv*10) / 100
		hp = hp * (100 + tst.Vit*2) / 100
		if tp := target.player; tp != nil {
			hp = hp * (100 + tp.skillLevel(skill.IDRecovery)*10) / 100
		}
	}
	if target.player != nil {
		if b := skill.SkillHeal2Bonus(tst.Player, d.ID); b != 0 {
			hp += hp * b / 100
			sp += sp * b / 100
		}
	}
	if st := src.status; src.player != nil && st != nil && st.HPlus > 0 {
		hp += hp * st.HPlus / 100
		sp += sp * st.HPlus / 100
	}
	hp, sp = z.potionPenalty(target, hp, sp, false)
	// Homunculi, which renewal heals three times as much, do not exist.
	z.skillEffect(src, target, d, lv, int64(lv), true)
	if hp > 0 || sp <= 0 && !a.berserk {
		z.skillEffect(target, target, z.Skills.Get(skill.IDHeal), hp, int64(hp), true)
	}
	if sp > 0 {
		z.skillEffect(target, target, z.Skills.Get(skill.IDSPRecovery), sp, int64(sp), true)
	}
	if !target.sc.Empty() {
		if z.rules().HealFormula && target.sc.Has(scExtremityFist) {
			sp = 0
		}
		if target.sc.Has(sc.SCNorecoverState) {
			hp, sp = 0, 0
		}
	}
	z.potionStatusHeal(target, int64(hp), int64(sp))
}

// potionPenalty is the target's statuses' part of the thrown potions:
// Water Insignia at level 2 adds a tenth, Critical Wound and Death Hurt
// take a part, and SC_NORECOVER_STATE takes all. Aid Condensed Potion
// counts Death Hurt only when its val3 is 1 (deathHurtOne).
func (z *Zone) potionPenalty(target *entity, hp, sp int, deathHurtOne bool) (int, int) {
	if target.sc.Empty() {
		return hp, sp
	}
	penalty := 0
	if e := target.sc.Get(scWaterInsignia); e != nil && e.Val1 == 2 {
		hp += hp / 10
		sp += sp / 10
	}
	if e := target.sc.Get(sc.SCCriticalwound); e != nil {
		penalty += e.Val2
	}
	if e := target.sc.Get(sc.SCDeathhurt); e != nil && (e.Val3 == 1 || !deathHurtOne && e.Val3 != 0) {
		penalty += 20
	}
	if target.sc.Has(sc.SCNorecoverState) {
		penalty = 100
	}
	// rAthena keeps the penalty in a uint8.
	penalty &= 0xff
	if penalty > 0 {
		hp -= hp * penalty / 100
		sp -= sp * penalty / 100
	}
	return hp, sp
}

// potionStatusHeal is status_heal of a thrown potion: HP and SP, at most
// what is missing, and the player sees its SP.
func (z *Zone) potionStatusHeal(target *entity, hp, sp int64) {
	if p := target.player; p != nil {
		if _, gotSP := z.statusHeal(p, hp, sp); gotSP > 0 {
			if s := z.battleStats(p); s != nil {
				p.pending = append(p.pending, s)
			}
		}
		return
	}
	z.healHP(target, hp)
}

// aidCondensedPotion is SkillAidCondensedPotion: the potion of the level
// is thrown on a cell, and heals the caster's side around it.
type aidCondensedPotion struct{}

// castendPos is SkillAidCondensedPotion::castendPos2. The caster's
// skills raise the potion's heal before it lands. The heal reaches the
// caster, its own summons and its party members in the splash
// (BCT_PARTY and BCT_GUILD), and its guild members.
func (aidCondensedPotion) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	p := src.player
	if p == nil {
		// A monster's throw runs the item's script on the monster, which
		// waits for monster skills.
		return
	}
	i := lv%11 - 1
	it := d.Requires.Items[max(i, 0)]
	id := uint32(it[0]) // #nosec G115 -- item IDs are positive.
	if i < 0 || it[0] <= 0 || z.countItem(p, id) < it[1] {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		*flag |= skillNoConsumeReq
		return
	}
	pr := z.runPotion(p, id, nil)
	rate := p.skillLevel(skill.IDSlimPitcher)*10 + p.skillLevel(skill.IDPotionPitcher)*10 +
		p.skillLevel(skill.IDLearningPotion)*5 + skill.SkillHealBonus(src.status.Player, d.ID)
	hp := pr.hp * (100 + rate) / 100
	sp := pr.sp * (100 + rate) / 100
	if st := src.status; st != nil && st.HPlus > 0 {
		hp += hp * st.HPlus / 100
		sp += sp * st.HPlus / 100
	}
	z.potion.hp, z.potion.sp = hp, sp
	if hp <= 0 && sp <= 0 {
		return
	}
	r := d.SplashFor(lv)
	for _, t := range z.findEntities(&query{area: areaRadius, x: x, y: y, radius: r, kinds: queryPlayers | queryMonsters}) {
		if !checkTarget(src, t, bctParty|bctGuild) {
			continue
		}
		z.castendNoDamage(src, t, d, lv, *flag|1)
	}
}

// castendNoDamage is SkillAidCondensedPotion::castendNoDamageId: the
// heal of the thrown potion on one target.
func (aidCondensedPotion) castendNoDamage(z *Zone, _, target *entity, d *skill.Def, _ int, _ *int) {
	if target.monster != nil && target.monster.spawn.Mob == mobEmperium {
		return
	}
	hp, sp := z.potion.hp, z.potion.sp
	tst := target.status
	if hp == 0 && sp == 0 || tst == nil {
		return
	}
	hp = hp * (100 + tst.Vit*2) / 100
	sp = sp * (100 + tst.Int*2) / 100
	if tp := target.player; tp != nil {
		b := skill.SkillHeal2Bonus(tst.Player, d.ID)
		if hp != 0 {
			hp = hp * (100 + tp.skillLevel(skill.IDRecovery)*10 + b) / 100
		}
		if sp != 0 {
			sp = sp * (100 + tp.skillLevel(skill.IDSPRecovery)*10 + b) / 100
		}
	}
	hp, sp = z.potionPenalty(target, hp, sp, true)
	if hp > 0 {
		z.skillEffect(target, target, z.Skills.Get(skill.IDHeal), hp, int64(hp), true)
	}
	if sp > 0 {
		z.skillEffect(target, target, z.Skills.Get(skill.IDSPRecovery), sp, int64(sp), true)
	}
	z.potionStatusHeal(target, int64(hp), int64(sp))
}

// summonPlant is SkillSummonFlora and SkillSummonMarineSphere: a monster
// of the caster's on the cell, which leaves when its time runs out. What
// the summon does on its own (a flora attacks the caster's enemies, a
// marine sphere explodes when hit) is monster AI and monster skills,
// which wait for phase 4 step 5. Until then a summon stands where it
// appeared, and players may hit it as alchemist_summon_setting says.
type summonPlant struct{ sphere bool }

// castBegin is skill_check_condition_castbegin for AM_CANNIBALIZE and
// AM_SPHEREMINE (land_skill_limit covers players): at most 6 less the
// level of floras of the kind, or ActiveInstance marine spheres, and with
// alchemist_summon_setting 4, no floras of another kind.
func (s summonPlant) castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	ai, class, maxCount := aiFlora, floraSummons[min(max(lv, 1), 5)-1], 6-lv
	if s.sphere {
		ai, class, maxCount = aiSphere, uint32(mobMarineSphere), d.ActiveInstance.At(lv)
	}
	if maxCount <= 0 {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	same, all := 0, 0
	for e := range z.entities.all() {
		m := e.monster
		if m == nil || m.master != src || m.specialAI != ai {
			continue
		}
		all++
		if m.spawn.Mob == class {
			same++
		}
	}
	if same >= maxCount || !s.sphere && same != all && z.summonSetting()&4 != 0 {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (s summonPlant) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, _ *int) {
	class, ai := floraSummons[min(max(lv, 1), 5)-1], aiFlora
	if s.sphere {
		class, ai = mobMarineSphere, aiSphere
	}
	name := ""
	if p := src.player; p != nil {
		// The summon carries its master's name.
		name = p.Name
	}
	z.summonMonster(src, class, x, y, name, ai, int64(d.Duration1.At(lv)))
}

// summonMonster is mob_once_spawn_sub and mob_spawn for a monster a skill
// summons on a cell, with its delete timer (mob_timer_delete). A summon
// with an AI has src as its master (master_id). It returns nil when the
// monster has no record.
//   - A summon with a master gives no exp (mob_dead), and only a marine
//     sphere drops items (alchemist_summon_reward 1).
//   - Players may hit floras and marine spheres as
//     alchemist_summon_setting says.
func (z *Zone) summonMonster(src *entity, class uint32, x, y int, name string, ai int, lifetime int64) *entity {
	if z.Mods == nil || z.Mods.Monsters == nil {
		return nil
	}
	tmpl, ok := z.Mods.Monsters[class]
	if !ok {
		return nil
	}
	sp := tmpl
	sp.Amount, sp.noRespawn, sp.owner = 1, true, 0
	sp.X, sp.Y, sp.W, sp.H = x, y, 0, 0
	if name != "" {
		sp.Name = name
	}
	if ai != aiNone && sp.Reward != nil {
		r := *sp.Reward
		r.BaseExp, r.JobExp = 0, 0
		if ai != aiSphere {
			r.Drops = nil
		}
		sp.Reward = &r
	}
	speed := sp.Speed
	if speed <= 0 {
		speed = WalkSpeed
	}
	e := &entity{monster: &monster{spawn: &sp, nextWalk: -1, specialAI: ai}, x: x, y: y, dir: z.rand.IntN(8), speed: speed}
	if sp.Status != nil {
		st := *sp.Status
		e.status, e.hp = &st, int64(st.MaxHP)
	}
	if ai != aiNone {
		e.monster.master = src
		setting := z.summonSetting()
		e.monster.playersHit = ai == aiFlora && setting&1 != 0 || ai == aiSphere && setting&2 != 0
	}
	if lifetime > 0 {
		e.monster.deleteAt = z.now + lifetime
	}
	z.entities.add(e)
	z.monsterSpawned(e)
	return e
}

// summonThink is the part of the monster AI that summons have now:
//   - mob_timer_delete: a summon whose time ran out leaves (unit_free).
//   - mob_ai_sub_hard_slavemob's first check: a summon whose master died
//     or left dies.
//
// It reports whether the summon is gone.
func (z *Zone) summonThink(e *entity) bool {
	m := e.monster
	if m.deleteAt > 0 && z.now >= m.deleteAt {
		z.freeMonster(e)
		return true
	}
	if ms := m.master; ms != nil && (ms.dead || !z.entities.has(ms)) {
		// status_kill.
		e.hp = 0
		e.hpChanged = true
		z.die(e, nil)
		return true
	}
	return false
}

// freeMonster is unit_free for a monster: it leaves the map without
// dying, so it drops nothing.
func (z *Zone) freeMonster(e *entity) {
	z.statusesDied(e)
	z.castCancel(e, false)
	z.clearUnitGroups(e)
	z.clearTimerSkills(e)
	z.removeMap(e)
	z.entities.remove(e)
	z.forget(e)
	z.dropStatuses(e)
	z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_TELEPORTED})
	z.endMonsterScript(e)
	if h := e.monster.handle; h != 0 {
		delete(z.monsterHandles, h)
	}
}

// plantCultivation is SkillPlantCultivation: on a free cell, half the
// casts grow a mushroom (level 1) or a plant (level 2), which leaves
// after the skill's time. A cell with a character fails and keeps the
// items.
type plantCultivation struct{}

func (plantCultivation) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	if src.player == nil {
		return
	}
	for e := range z.entities.all() {
		if (e.player != nil || e.monster != nil) && !e.dead && e.x == x && e.y == y {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			*flag |= skillNoConsumeReq
			return
		}
	}
	z.skillPosEffect(src, d, lv, x, y)
	if z.rand.IntN(100) < 50 {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	var class uint32
	if lv == 1 {
		class = mobBlackMushroom + uint32(z.rand.IntN(2)) // #nosec G115 -- 0 or 1.
	} else {
		switch r := z.rand.IntN(100); {
		case r < 30:
			class = mobGreenPlant
		case r < 55:
			class = mobRedPlant
		case r < 80:
			class = mobYellowPlant
		case r < 90:
			class = mobWhitePlant
		case r < 98:
			class = mobBluePlant
		default:
			class = mobShiningPlant
		}
	}
	z.summonMonster(src, class, x, y, "", aiNone, int64(d.Duration1.At(lv)))
}

// twilightAlchemy is SkillTwilightAlchemy1, 2 and 3: many potions at
// once, through skill_produce_mix, without the recipe window.
type twilightAlchemy struct{}

// castBegin is skill_check_condition_castbegin for AM_TWILIGHT2 and
// AM_TWILIGHT3: party_skill_check needs an online Super Novice or
// Taekwon class in the caster's party. A player without a party fails.
func (twilightAlchemy) castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	if (d.ID == skill.IDTwilight2 || d.ID == skill.IDTwilight3) && !z.partySkillCheck(src, d.ID, lv) {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (twilightAlchemy) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	switch d.ID {
	case skill.IDTwilight1, skill.IDTwilight2:
		// 200 White Potions, or 200 White Slim Potions.
		z.skillEffect(src, target, d, lv, int64(lv), true)
		product := uint32(itemWhitePotion)
		if d.ID == skill.IDTwilight2 {
			product = itemWhiteSlimPotion
		}
		if !z.produceMix(p, d.ID, product, 200, -1) {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		}
	case skill.IDTwilight3:
		// 100 Alcohol, 50 Acid Bottles and 50 Bottle Grenades, for 200
		// Empty Bottles in all.
		alcohol := z.canProduceMix(p, itemAlcohol, 100)
		acid := z.canProduceMix(p, itemAcidBottle, 50)
		fire := z.canProduceMix(p, itemFireBottle, 50)
		if alcohol < 0 || acid < 0 || fire < 0 || z.countItem(p, itemEmptyBottle) < 200 {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return
		}
		z.skillEffect(src, target, d, lv, int64(lv), true)
		z.produceMix(p, d.ID, itemAlcohol, 100, alcohol)
		z.produceMix(p, d.ID, itemAcidBottle, 50, acid)
		z.produceMix(p, d.ID, itemFireBottle, 50, fire)
	}
}

// Items of Twilight Alchemy's recipes.
const (
	itemWhitePotion     = 504  // ITEMID_WHITE_POTION
	itemWhiteSlimPotion = 547  // ITEMID_WHITE_SLIM_POTION
	itemAlcohol         = 970  // ITEMID_ALCOHOL
	itemFireBottle      = 7135 // ITEMID_FIRE_BOTTLE
	itemAcidBottle      = 7136 // ITEMID_ACID_BOTTLE
)

// produceRecipe is an entry of rAthena's produce_db.txt: the product,
// the skill and level it needs, and its materials. A material of amount
// 0, such as a guide book, must be held but is not used.
type produceRecipe struct {
	item         uint32
	skill, level int
	materials    [][2]int
}

// twilightRecipes are the produce_db.txt entries Twilight Alchemy makes,
// the same in both modes (db/pre-re and db/re/produce_db.txt), which the
// extracts do not hold:
//   - 121: White Potion, from the Potion Creation Guide, a White Herb
//     and an Empty Potion Bottle.
//   - 125: White Slim Potion, from the Condensed Potion Creation Guide, a
//     White Potion, an Empty Test Tube and Witched Starsand.
//   - 128: Bottle Grenade, from the Bottle Grenade Creation Guide,
//     Alcohol, Fabric and an Empty Bottle.
//   - 129: Acid Bottle, from the Acid Bottle Creation Guide, an Empty
//     Bottle and Immortal Heart.
//   - 139: Alcohol, from the Alcohol Creation Guide, an Empty Bottle, 5
//     Poison Spores, 5 Stems and an Empty Test Tube. Entry 206, Alcohol by
//     another skill, is left out: Twilight Alchemy's casters learn Prepare
//     Potion, so entry 139 comes first.
var twilightRecipes = []produceRecipe{
	{itemWhitePotion, skill.IDPharmacy, 1, [][2]int{{7144, 0}, {509, 1}, {1093, 1}}},
	{itemWhiteSlimPotion, skill.IDPharmacy, 1, [][2]int{{7133, 0}, {504, 1}, {1092, 1}, {1061, 1}}},
	{itemFireBottle, skill.IDPharmacy, 1, [][2]int{{7128, 0}, {970, 1}, {1059, 1}, {713, 1}}},
	{itemAcidBottle, skill.IDPharmacy, 1, [][2]int{{7129, 0}, {713, 1}, {929, 1}}},
	{itemAlcohol, skill.IDPharmacy, 1, [][2]int{{7127, 0}, {713, 1}, {7033, 5}, {905, 5}, {1092, 1}}},
}

// canProduceMix is skill_can_produce_mix without a trigger: the index of
// the recipe a player can make qty of, or -1. The player needs the
// recipe's skill, room for the product, and the materials.
func (z *Zone) canProduceMix(p *Player, product uint32, qty int) int {
	if z.Items == nil {
		return -1
	}
	if _, ok := z.Items.Lookup(product); !ok {
		return -1
	}
	idx := -1
	for i, r := range twilightRecipes {
		if r.item != product {
			continue
		}
		if r.skill > 0 && p.skillLevel(r.skill) < r.level {
			continue
		}
		idx = i
		break
	}
	if idx < 0 {
		return -1
	}
	if z.overAmount(p, product, qty) {
		return -1
	}
	for _, m := range twilightRecipes[idx].materials {
		id := uint32(m[0]) // #nosec G115 -- item IDs are positive.
		if m[1] == 0 {
			if z.countItem(p, id) == 0 {
				return -1
			}
			continue
		}
		if z.countItem(p, id) < qty*m[1] {
			return -1
		}
	}
	return idx
}

// overAmount is pc_checkadditem's CHKADDITEM_OVERAMOUNT: qty of an item
// would not fit in its stack.
func (z *Zone) overAmount(p *Player, id uint32, qty int) bool {
	if qty > item.MaxAmount {
		return true
	}
	d := z.Items.Get(id)
	if !d.Stackable {
		return false
	}
	if d.StackLimit > 0 && qty > d.StackLimit {
		return true
	}
	for i, s := range p.inventory.Slots {
		if s.ID != id {
			continue
		}
		if qty > item.MaxAmount-s.Amount || d.StackLimit > 0 && qty > d.StackLimit-s.Amount {
			return true
		}
		return i >= p.inventory.Size
	}
	return false
}

// produceMix is skill_produce_mix for the potions of Prepare Potion and
// Twilight Alchemy: the materials of qty products are used, then each
// product succeeds at make_per in 10,000. idx is the recipe, or -1 to
// look it up. It reports whether any product was made.
//   - The created items' maker name (produce_item_name_input) is not
//     ported: the inventory has no field for it yet.
//   - The fame of the potion rankings is not ported either.
func (z *Zone) produceMix(p *Player, skillID int, product uint32, qty, idx int) bool {
	if idx < 0 {
		if idx = z.canProduceMix(p, product, qty); idx < 0 {
			return false
		}
	}
	qty = max(qty, 1)
	for _, m := range twilightRecipes[idx].materials {
		id := uint32(m[0]) // #nosec G115 -- item IDs are positive.
		if _, ok := z.Items.Lookup(id); !ok {
			continue
		}
		if z.countItem(p, id) == 0 {
			return false
		}
		z.takeItems(p, id, qty*m[1])
	}
	st := p.entity.status
	if st == nil {
		return false
	}
	makePer := p.skillLevel(skill.IDLearningPotion)*50 + p.skillLevel(skill.IDPharmacy)*300 +
		z.unit(p.entity).JobLevel()*20 + st.Int/2*10 + st.Dex*10 + st.Luk*10
	// Homunculi, whose Instruction Change adds to it, do not exist yet.
	switch product {
	case itemWhitePotion:
		makePer += (1+z.rand.IntN(100))*10 + 2000
	case itemAlcohol:
		makePer += (1+z.rand.IntN(100))*10 + 1000
	case itemFireBottle, itemAcidBottle:
		makePer += (1 + z.rand.IntN(100)) * 10
	case itemWhiteSlimPotion:
		makePer -= (1 + z.rand.IntN(100)) * 10
	}
	// potion_produce_rate is 100%. Baby jobs make 30% less.
	if z.Status != nil {
		if m, ok := z.Status.MapID(p.char.Job); ok && m&jcBaby != 0 {
			makePer = makePer * 70 / 100
		}
	}
	makePer = max(makePer, 1)
	made := 0
	if qty > 1 || z.rand.IntN(10000) < makePer {
		for range qty {
			if qty == 1 || z.rand.IntN(10000) < makePer {
				made++
			}
		}
	}
	if made == 0 {
		return false
	}
	// A full inventory loses them (skill_drop_items_full is off).
	z.addItem(p, product, made, skillOrigin(skillID))
	return true
}

// skillDamageBlocked is skill_additional_effect for a hit that a status
// such as Pneuma blocked: Acid Terror's bleeding and armor break still
// happen for a player's hit.
func (z *Zone) skillDamageBlocked(src, target *entity, d *skill.Def, lv int) {
	if d.ID == skill.IDAcidTerror && src.player != nil && !target.dead {
		z.acidTerrorEffect(src, target, d, lv)
	}
}
