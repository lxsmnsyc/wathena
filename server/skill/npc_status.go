package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The monster skills that cause statuses: the damage formulas of the
// SkillImpl classes in rAthena's src/map/skills/npc. Their other behavior
// is in server/zone/skills_npc_status.go.

// Skill IDs of the monster skills with a formula here, from rAthena's
// skill_db.yml.
const (
	idNPCPoison        = 176 // NPC_POISON
	idNPCBlindAttack   = 177 // NPC_BLINDATTACK
	idNPCSilenceAttack = 178 // NPC_SILENCEATTACK
	idNPCStunAttack    = 179 // NPC_STUNATTACK
	idNPCPetrifyAttack = 180 // NPC_PETRIFYATTACK
	idNPCCurseAttack   = 181 // NPC_CURSEATTACK
	idNPCSleepAttack   = 182 // NPC_SLEEPATTACK
	idNPCBleeding      = 660 // NPC_BLEEDING
	idNPCWideSuck      = 722 // NPC_WIDESUCK, renewal only
	idNPCBleeding2     = 764 // NPC_BLEEDING2, renewal only
)

type (
	// npcStatusAttack is the weapon attacks that may cause a status:
	// SkillPoisonAttack, SkillBlindAttack, SkillSilenceAttack,
	// SkillStunAttack, SkillPetrifyAttack, SkillCurseAttack,
	// SkillSleepAttack, SkillBleeding and SkillBleeding2.
	npcStatusAttack struct{ WeaponSkill }
	// npcWideSuck is SkillWideSuck, whose ground unit deals misc damage.
	npcWideSuck struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		idNPCPoison:        npcStatusAttack{},
		idNPCBlindAttack:   npcStatusAttack{},
		idNPCSilenceAttack: npcStatusAttack{},
		idNPCStunAttack:    npcStatusAttack{},
		idNPCPetrifyAttack: npcStatusAttack{},
		idNPCCurseAttack:   npcStatusAttack{},
		idNPCSleepAttack:   npcStatusAttack{},
		idNPCBleeding:      npcStatusAttack{},
		idNPCBleeding2:     npcStatusAttack{},
		idNPCWideSuck:      npcWideSuck{},
	})
}

// HitRate is modifyHitRate of the status attacks: 20% more.
func (npcStatusAttack) HitRate(_ *battle.SkillContext, hit *int) { *hit += *hit * 20 / 100 }

// BaseDamage is battle_calc_misc_attack for NPC_WIDESUCK: 15% of the
// target's max HP. rAthena multiplies the unsigned max_hp first.
func (npcWideSuck) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	return int64(uint32(a.Target.MaxHP)*15) / 100 // #nosec G115 -- rAthena's max_hp is a uint32.
}
