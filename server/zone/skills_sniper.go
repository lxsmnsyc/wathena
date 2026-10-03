package zone

import (
	"github.com/lxsmnsyc/wathena/server/skill"
)

// The Sniper's skills: the SkillImpl classes of rAthena's
// src/map/skills/archer.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDTrueSight:     statusSkill{},
		skill.IDFalconAssault: attackSkill{},
		skill.IDSharpShooting: sharpShooting{},
		skill.IDWindWalk:      windWalk{},
	})
}

// windWalk is SkillWindWalker: the party around the caster gets the
// status. Parties do not exist yet, so the caster alone gets it, as
// rAthena does for a player without a party.
type windWalk struct{ statusSkill }

// sharpShooting is SkillFocusedArrowStrike. In renewal it hits the target
// and the enemies around it (SkillImplRecursiveDamageSplash), and each
// hit ends the caster's Camouflage. In pre-renewal it hits the enemies in
// a line toward the target (skill_eightpath_algorithm, map_foreachindir),
// the target included; when the line deals no damage, the target is hit
// once more.
type sharpShooting struct{}

func (sharpShooting) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if z.skillRenewal(skill.IDSharpShooting) {
		z.recursiveSplash(src, target, d, lv, *flag)
		return
	}
	// Flag 2 picks the line's formula for monsters.
	f := *flag | 2
	z.area = areaTemp{main: target}
	var total int64
	for _, t := range z.inDir(src.x, src.y, target.x, target.y, d.SplashFor(lv), d.ActiveInstance.At(lv), 0) {
		total += z.skillAttackArea(d.Type, src, src, t, d, lv, f)
	}
	if total == 0 {
		// Only the units in the area are hit; the target is hit when the
		// area deals nothing.
		z.skillAttack(d.Type, src, src, target, d, lv, f&^2)
	}
}

// splashDamage is SkillFocusedArrowStrike's recursive castendDamageId in
// renewal: the hit, then the caster's Camouflage ends.
func (sharpShooting) splashDamage(z *Zone, src, target *entity, d *skill.Def, lv, flag int) {
	z.skillAttack(d.Type, src, src, target, d, lv, flag)
	z.scEnd(src, scCamouflage)
}

// skillAttackArea is skill_attack_area: the skill's main target takes a
// full hit; other units must be enemies the skill can reach, and take a
// hit without its animation. It returns the damage dealt.
func (z *Zone) skillAttackArea(typ int, src, dsrc, t *entity, d *skill.Def, lv, flag int) int64 {
	if t.dead {
		return 0
	}
	if t == z.area.main {
		return z.skillAttack(typ, src, dsrc, t, d, lv, flag)
	}
	if !checkTarget(dsrc, t, bctEnemy) || !z.statusCheckSkillUse(nil, t, d, 2) {
		return 0
	}
	return z.skillAttack(typ, src, dsrc, t, d, lv, flag|sdAnimation)
}
