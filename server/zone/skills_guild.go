package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/social"
)

// Guild skills the guild master uses (phase 5 step 5, decision 0021),
// as skill.cpp's GD_ cases:
//   - Battle Orders, Regeneration and Restore reach the caster's guild
//     members in the splash, the caster too. They work on GvG maps only,
//     as skill_check_condition_castbegin's map_flag_gvg2. No map is a GvG
//     map until sieges come.
//   - Emergency Call brings every online member to the caster, on any
//     map, where the emergency_call rule allows it. Its default, 11,
//     allows it on GvG maps only.
//   - Renewal gives each skill its own cooldown, from skill_db. Pre-renewal
//     (the skill_cooldown flag) blocks all four for the skill's
//     Duration2, as guild_block_skill.
//   - Only the master uses them, at the guild's level of the skill.

func init() {
	registerSkillImpls(map[int]any{
		social.SkillBattleOrder:   guildBuff{},
		social.SkillRegeneration:  guildBuff{},
		social.SkillRestore:       guildBuff{},
		social.SkillEmergencyCall: emergencyCall{},
	})
}

// guildCastBegin is the guild part of skill_check_condition_castbegin and
// skill_isNotOk. A player who is not its guild's master fails without a
// message.
func guildCastBegin(z *Zone, src *entity, d *skill.Def) wathenav1.SkillFailure {
	p := src.player
	if p == nil || !p.isGuildMaster() {
		return skillFailSilent
	}
	switch d.ID {
	case social.SkillBattleOrder, social.SkillRegeneration, social.SkillRestore:
		if !z.GvG {
			return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
		}
	case social.SkillEmergencyCall:
		// emergency_call: bit 1 outside sieges, 2 in sieges, 4 outside
		// GvG maps and 8 on them. Sieges do not exist yet, and maps have
		// no nowarpto flag yet (bit 16).
		ec := z.guildRules().EmergencyCall
		where := 4
		if z.GvG {
			where = 8
		}
		if ec&1 == 0 || ec&where == 0 {
			return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
		}
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

// guildSkillBlock is the pre-renewal guild_block_skill after a guild
// skill: the four skills wait the skill's Duration2.
func (z *Zone) guildSkillBlock(src *entity, d *skill.Def, lv int) {
	if p := src.player; p != nil && !z.worldRules().SkillCooldown {
		z.blockGuildSkills(p, int64(d.Duration2.At(lv)), false)
	}
}

// guildBuff is GD_BATTLEORDER, GD_REGENERATION and GD_RESTORE.
type guildBuff struct{}

func (guildBuff) castBegin(z *Zone, src *entity, d *skill.Def, _ int) wathenav1.SkillFailure {
	return guildCastBegin(z, src, d)
}

func (guildBuff) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 != 0 {
		// The same guild only; skill_area_sub let allies through.
		if g := guildOf(src); g == 0 || g != guildOf(target) {
			return
		}
		if d.ID == social.SkillRestore {
			if p := target.player; p != nil && target.status != nil {
				hp, _ := z.statusHeal(p, int64(target.status.MaxHP)*90/100, int64(target.status.MaxSP)*90/100)
				if h := z.skillsGet(skill.IDHeal); h != nil {
					z.skillEffect(src, target, h, int(hp), hp, true)
				}
			}
			return
		}
		z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
		return
	}
	if guildOf(src) == 0 {
		return
	}
	z.skillEffect(src, src, d, lv, int64(lv), true)
	f := *flag | bctGuild | 1
	for _, t := range z.findEntities(&query{area: areaRadius, x: src.x, y: src.y, radius: d.SplashFor(lv), kinds: queryPlayers}) {
		if checkTarget(src, t, f) {
			z.castendNoDamage(src, t, d, lv, f)
		}
	}
	z.guildSkillBlock(src, d, lv)
}

// emergencyCall is GD_EMERGENCYCALL.
type emergencyCall struct{}

func (emergencyCall) castBegin(z *Zone, src *entity, d *skill.Def, _ int) wathenav1.SkillFailure {
	return guildCastBegin(z, src, d)
}

func (emergencyCall) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil || p.soc == nil {
		return
	}
	z.skillEffect(src, src, d, lv, int64(lv), true)
	reach := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < z.grid.Width && y < z.grid.Height && !z.grid.NoPass(x, y)
	}
	z.socialHub().EmergencyCall(p.soc, z.Map, src.x, src.y, reach)
	z.guildSkillBlock(src, d, lv)
}
