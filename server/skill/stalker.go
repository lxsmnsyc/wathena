package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The Stalker's skills (job 4018), from rAthena's src/map/skills/thief.
// None of them deals damage; their behavior is in the zone's
// skills_rogue.go.

// Skill IDs of the Stalker, from rAthena's skill_db.yml.
const (
	IDChaseWalk   = 389 // ST_CHASEWALK
	IDRejectSword = 390 // ST_REJECTSWORD
	IDPreserve    = 475 // ST_PRESERVE
	IDFullStrip   = 476 // ST_FULLSTRIP
)

type (
	// chaseWalk is SkillStealth.
	chaseWalk struct{ Base }
	// rejectSword is SkillCounterInstinct, a StatusSkillImpl.
	rejectSword struct{ Base }
	// preserve is ST_PRESERVE's StatusSkillImpl.
	preserve struct{ Base }
	// fullStrip is SkillDivestAll.
	fullStrip struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDChaseWalk:   chaseWalk{},
		IDRejectSword: rejectSword{},
		IDPreserve:    preserve{},
		IDFullStrip:   fullStrip{},
	})
}
