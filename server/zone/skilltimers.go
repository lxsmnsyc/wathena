package zone

import (
	"slices"

	"github.com/lxsmnsyc/wathena/server/skill"
)

// Timed skill effects: rAthena's skill_addtimerskill, skill_timerskill
// and skill_cleartimerskill. A skill that acts later, such as Jupitel
// Thunder's delayed hit or Water Ball's balls, queues a timer skill on
// its caster, and Lex Divina silences a second after the cast. The zone
// runs the ones that are due each tick, in the order of their time, then
// the order they were added in. Each skill's part of skill_timerskill is
// its behavior's timerSkill method.

const (
	// timerSkillInterval is TIMERSKILL_INTERVAL.
	timerSkillInterval = 150
	// maxTimerSkills is MAX_SKILLTIMERSKILL: the timer skills a unit may
	// have at once.
	maxTimerSkills = 50
)

// timerSkill is skill_timerskill: one timed skill effect.
type timerSkill struct {
	// at is when it runs, in zone milliseconds.
	at          int64
	src, target *entity
	x, y        int
	def         *skill.Def
	lv          int
	// typ and flag are skl->type and skl->flag: a value of the skill's
	// own, such as the number of the ball, and the skill_attack flag.
	typ, flag int
}

// timerSkiller is a skill's part of skill_timerskill.
type timerSkiller interface {
	timerSkill(z *Zone, t *timerSkill)
}

// addTimerSkill is skill_addtimerskill. It reports whether the unit had
// room for another timer skill.
func (z *Zone) addTimerSkill(src *entity, at int64, target *entity, x, y int, d *skill.Def, lv, typ, flag int) bool {
	if !z.entities.has(src) {
		return false
	}
	n := 0
	for _, t := range z.timerSkills {
		if t.src == src {
			n++
		}
	}
	if n >= maxTimerSkills {
		return false
	}
	z.timerSkills = append(z.timerSkills, &timerSkill{at: at, src: src, target: target, x: x, y: y, def: d, lv: lv,
		typ: typ, flag: flag})
	return true
}

// clearTimerSkills is skill_cleartimerskill: a unit that died or left
// loses its timer skills.
func (z *Zone) clearTimerSkills(e *entity) {
	z.timerSkills = slices.DeleteFunc(z.timerSkills, func(t *timerSkill) bool { return t.src == e })
}

// advanceTimerSkills runs the timer skills that are due.
func (z *Zone) advanceTimerSkills() {
	for {
		next := -1
		for i, t := range z.timerSkills {
			if t.at <= z.now && (next < 0 || t.at < z.timerSkills[next].at) {
				next = i
			}
		}
		if next < 0 {
			return
		}
		t := z.timerSkills[next]
		z.timerSkills = slices.Delete(z.timerSkills, next, next+1)
		z.runTimerSkill(t)
	}
}

// runTimerSkill is skill_timerskill for one timer skill: the checks on
// the caster and the target, then the skill's own part.
func (z *Zone) runTimerSkill(t *timerSkill) {
	src := t.src
	// The source left the map.
	if !z.entities.has(src) {
		return
	}
	impl, ok := skillImpls[t.def.ID].(timerSkiller)
	if target := t.target; target != nil {
		// The target left the map.
		if !z.entities.has(target) {
			return
		}
		// The caster is dead.
		if src.dead {
			return
		}
		if target.dead && t.def.ID != skill.IDWaterBall {
			return
		}
	}
	if ok {
		impl.timerSkill(z, t)
		return
	}
	// skill_timerskill's default case: one more skill_attack of the type,
	// such as Double Bolt's second bolt.
	if t.target != nil {
		z.skillAttack(t.typ, src, src, t.target, t.def, t.lv, t.flag)
	}
}
