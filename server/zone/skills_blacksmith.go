package zone

import (
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The Blacksmith and Whitesmith skills, as the SkillImpl classes of
// rAthena's src/map/skills/merchant. The damage formula of Cart
// Termination is in server/skill/blacksmith.go; the statuses' effects are
// in server/status, server/statuschange and server/battle; the breaking
// of Power Thrust and Melt Down is in breakequip.go; Ore Discovery is in
// loot.go.
//
// The forging skills (Iron Tempering to Spear Smithing, Enchanted Stone
// Craft and Oridecon Research) and the passives (Hilt Binding, Ore
// Discovery, Weapon Research, Skin Tempering and Unfair Trick) have no
// cast. Weaponry Research and Hilt Binding are in the stats and in
// battle, Skin Tempering in the stats, Unfair Trick in the requirements.
// The forging skills wait for item creation (skill_produce_mix), which
// needs a client window.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDHammerFall:      hammerFall{},
		skill.IDAdrenaline:      smithStatus{},
		skill.IDWeaponPerfect:   smithStatus{},
		skill.IDOverThrust:      smithStatus{},
		skill.IDMaximize:        statusSkill{toggle: true},
		skill.IDAdrenaline2:     smithStatus{},
		skill.IDGreed:           greed{},
		skill.IDMeltdown:        statusSkill{},
		skill.IDCartBoost:       statusSkill{},
		skill.IDCartTermination: cartTermination{},
		skill.IDOverThrustMax:   statusSkill{},

		// Weapon Repair (SkillWeaponRepair) and Upgrade Weapon
		// (SkillUpgradeWeapon) open item lists
		// (clif_item_repair_list and clif_item_refine_list), and the
		// choice comes back from the window. The client has no such
		// window yet.
		skill.IDRepairWeapon: notPorted{},
		skill.IDWeaponRefine: notPorted{},
	})
}

// smithStatus is SkillAdrenalineRush, SkillAdvancedAdrenalineRush,
// SkillWeaponPerfection and SkillPowerThrust: the party buffs of the
// Blacksmith. A player in a party casts them on the members in the
// splash on its map. The caster alone gets the status otherwise. A
// player target needs one of the skill's weapons. val2 is 1 on the
// caster, whose status is stronger than a party member's.
type smithStatus struct{}

func (smithStatus) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 == 0 && inParty(src) {
		z.partyCastNoDamage(src, d, lv, *flag)
		return
	}
	if w := d.Requires.Weapon; w != 0 && target.player != nil && !checkWeaponType(target, w) {
		return
	}
	self := 0
	if src == target {
		self = 1
	}
	ok := z.scStart4(src, target, d.Status, 100, lv, self, 0, 0, int64(d.Duration1.At(lv)))
	z.skillEffect(target, target, d, lv, int64(lv), ok)
}

// hammerFall is SkillHammerFall: every enemy around the cell gets a
// stun chance a second later.
type hammerFall struct{}

// castendPos is SkillHammerFall::castendPos2: skill_castend_nodamage_id
// on each enemy in the square of the splash, without a wall check.
func (hammerFall) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	for _, t := range z.areaTargets(src, x, y, d.SplashFor(lv), false) {
		z.castendNoDamage(src, t, d, lv, *flag|bctEnemy|2)
	}
}

// castendNoDamage is SkillHammerFall::castendNoDamageId: the stun waits
// a second, as a timer skill with the chance in its type.
func (hammerFall) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.addTimerSkill(src, z.now+1000, target, 0, 0, d, lv, min(20+10*lv, 50+5*lv), *flag)
}

// timerSkill is Hammer Fall's part of skill_timerskill: the stun.
func (hammerFall) timerSkill(z *Zone, t *timerSkill) {
	z.scStart(t.src, t.target, sc.SCStun, t.typ, t.lv, int64(t.def.Duration2.At(t.lv)))
}

// cartTermination is SkillCartTermination: a weapon hit that may stun.
type cartTermination struct{ attackSkill }

// additionalEffect is SkillCartTermination::applyAdditionalEffects.
func (cartTermination) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCStun, 5*lv, lv, int64(d.Duration2.At(lv)))
}

// greed is SkillGreed: a player picks up every item around the target,
// as skill_greed and pc_takeitem.
type greed struct{}

func (greed) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	r := d.SplashFor(lv)
	var items []*entity
	for e := range z.entities.all() {
		if e.item != nil && checkDistance(e.x-target.x, e.y-target.y, r, false) {
			items = append(items, e)
		}
	}
	for _, it := range items {
		if z.entities.has(it) {
			z.greedTake(p, it)
		}
	}
}

// greedTake is pc_takeitem for Greed: the distance check does not
// apply. A status that stops picking up, or an item the player has no
// right to yet, leaves the item without a message. An item that does
// not fit stays, and the player learns why.
func (z *Zone) greedTake(p *Player, it *entity) {
	if l := p.entity.sc; !l.Empty() && l.Cant.PickUp {
		return
	}
	if !it.item.rights.CanTake(p.charID, z.now) {
		return
	}
	net, _ := p.netIDs.get(it)
	z.take(p, it, net)
}
