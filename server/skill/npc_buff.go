package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The formulas of the monster self skills of rAthena's src/map/skills/npc
// that deal damage. Their behaviors are in the zone's skills_npc_buff.go.

// Skill IDs of the monster self skills with a damage formula.
const (
	idNpcSelfDestruction = 173 // NPC_SELFDESTRUCTION
	idNpcSmoking         = 195 // NPC_SMOKING
	idNpcChangeUndead    = 348 // NPC_CHANGEUNDEAD
	idNpcVampireGift     = 679 // NPC_VAMPIRE_GIFT
)

// npcSuicideBombing is SkillSuicideBombing.
type npcSuicideBombing struct{ Base }

// BaseDamage is NPC_SELFDESTRUCTION's case of battle_calc_misc_attack:
// the caster's HP.
func (npcSuicideBombing) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	return a.Src.CurrentHP()
}

// npcSmoking is SkillSmoking.
type npcSmoking struct{ Base }

// BaseDamage is NPC_SMOKING's case of battle_calc_misc_attack: 3.
func (npcSmoking) BaseDamage(*battle.SkillContext, battle.Rand) int64 { return 3 }

// npcUndeadAttributeChange is SkillUndeadAttributeChange.
type npcUndeadAttributeChange struct{ Base }

// HitRate is SkillUndeadAttributeChange::modifyHitRate: 20% more.
func (npcUndeadAttributeChange) HitRate(_ *battle.SkillContext, hit *int) { *hit += *hit * 20 / 100 }

// npcVampireGift is SkillVampireGift.
type npcVampireGift struct{ Base }

// SkillRatio is SkillVampireGift::calculateSkillRatio: 100% more per
// level, and levels 6 to 10 as levels 1 to 5.
func (npcVampireGift) SkillRatio(a *battle.SkillContext, ratio *int) {
	*ratio += ((a.Level()-1)%5 + 1) * 100
}

func init() {
	register(map[int]battle.SkillImpl{
		idNpcSelfDestruction: npcSuicideBombing{},
		idNpcSmoking:         npcSmoking{},
		idNpcChangeUndead:    npcUndeadAttributeChange{},
		idNpcVampireGift:     npcVampireGift{},
	})
}
