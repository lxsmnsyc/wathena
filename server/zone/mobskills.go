package zone

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Monster skills: rAthena's mobskill_use and mobskill_event, with the
// skills list of a monster's record (proposal 0002, review decision 22).

// Events of mobskill_use besides the conditions that name one.
const (
	// mobEventNone is event -1: the AI's own check, which tests the
	// conditions that are states.
	mobEventNone mob.Condition = -1
	// mobEventDamage is event -2: damage from an attack that is neither
	// melee nor ranged, which only damage_above answers.
	mobEventDamage mob.Condition = -2
)

// maxEnemySearch is MAX_ENEMY_SEARCH_COUNT.
const maxEnemySearch = 32

// mobSkillUse is mobskill_use for an event that is not a skill hit.
func (z *Zone) mobSkillUse(e *entity, event mob.Condition, damage int64) bool {
	return z.mobSkillUseEvent(e, event, 0, damage)
}

// mobSkillUseEvent is mobskill_use: the monster walks its skills in
// order and uses the first one whose delay passed, whose state matches,
// whose rate rolls and whose condition holds. eventSkill is the skill of
// a hit_by_skill event. It reports whether a skill was used.
func (z *Zone) mobSkillUseEvent(e *entity, event mob.Condition, eventSkill int, damage int64) bool {
	m := e.monster
	ms := m.spawn.Skills
	if e.cast != nil || len(ms) == 0 || e.status == nil || e.status.Mode&mob.ModeNoCast != 0 || z.Skills == nil {
		return false
	}
	if len(m.skillDelay) != len(ms) {
		m.skillDelay = make([]int64, len(ms))
	}
	if event == mobEventNone {
		m.lastSkillCheck = z.now
	}
	for i := range ms {
		s := &ms[i]
		if z.now < m.skillDelay[i] {
			continue
		}
		if !mob.StateMatches(s.State, m.ai.State, m.target != nil) {
			continue
		}
		if z.rand.IntN(10000) > s.Rate {
			continue
		}
		var fbl *entity
		ok := false
		switch {
		case s.Condition == event && event != mob.CondHitBySkill:
			// rAthena passes a skill hit as MSC_SKILLUSED with the skill
			// in its high bits, which no condition equals.
			ok = true
		case s.Condition == mob.CondHitBySkill:
			ok = event == mob.CondHitBySkill && (eventSkill == s.Value || s.Value == 0)
		case s.Condition == mob.CondHitByGroundSkill && damage > 0:
			if event == mob.CondHitBySkill {
				d := z.Skills.Get(eventSkill)
				ok = d != nil && d.Target == skill.TargetGround
			}
		case s.Condition == mob.CondDamageAbove && damage > 0 && event != mob.CondHitBySkill:
			ok = damage > int64(s.Value)
		case event == mobEventNone:
			ok, fbl = z.mobCondition(e, s)
		}
		if !ok {
			continue
		}
		d := z.Skills.Get(s.ID)
		if d == nil {
			break
		}
		if !z.mobCastEntry(e, i, s, d, fbl) {
			// mob_ai 0x1000 is off: a failed skill ends the check.
			break
		}
		if s.Chat != "" {
			z.mobChat(e, s.Chat, s.ChatLineColor())
		}
		return true
	}
	m.skillIdx = -1
	return false
}

// mobUse is the entry a monster used last, and where: a unit, or a cell
// for a ground skill.
type mobUse struct {
	idx    int
	target *entity
	x, y   int
	ground bool
}

// mobCondition checks a condition that is a state of the monster, and
// returns the friend it found for friend conditions.
func (z *Zone) mobCondition(e *entity, s *mob.Skill) (bool, *entity) {
	m := e.monster
	c2 := s.Value
	switch s.Condition {
	case mob.CondAlways:
		return true, nil
	case mob.CondHPBelow:
		return hpPercent(e) <= c2, nil
	case mob.CondHPBetween:
		p := hpPercent(e)
		return p >= c2 && p <= s.Value2, nil
	case mob.CondHasStatus, mob.CondLacksStatus:
		if e.sc.Empty() {
			return false, nil
		}
		return statusCondition(e, s.Condition, c2), nil
	case mob.CondFriendHPBelow:
		f := z.mobFriendHP(e, 0, c2)
		return f != nil, f
	case mob.CondFriendHPBetween:
		f := z.mobFriendHP(e, c2, s.Value2)
		return f != nil, f
	case mob.CondFriendHasStatus, mob.CondFriendLacksStatus:
		f := z.mobFriendStatus(e, s.Condition, c2)
		return f != nil, f
	case mob.CondSlavesBelow:
		return z.countSlaves(e) < c2, nil
	case mob.CondSlavesAtMost:
		return z.countSlaves(e) <= c2, nil
	case mob.CondAttackersAbove:
		return z.countTargeted(e) > c2, nil
	case mob.CondAttackersAtLeast:
		return z.countTargeted(e) >= c2, nil
	case mob.CondAfterSkill:
		return m.lastSkill == c2, nil
	case mob.CondHitWhileStuck:
		if m.attackedCount >= rudeAttackedCount {
			m.attackedCount = 0
			return true, nil
		}
		return false, nil
	case mob.CondMasterHPBelow:
		if ms := m.master; ms != nil && z.entities.has(ms) && hpPercent(ms) < c2 {
			return true, ms
		}
		return false, nil
	case mob.CondMasterAttacked:
		ms := m.master
		return ms != nil && z.entities.has(ms) && z.countTargeted(ms) > 0, ms
	case mob.CondSummonedByAlchemist:
		return m.specialAI != aiNone && m.trickcasting == 0 && e.hp < int64(e.status.MaxHP), nil
	case mob.CondMonstersNearbyAbove:
		n := 0
		for o := range z.entities.all() {
			if o.monster != nil && !o.dead && chebyshev(e, o) <= ViewRange {
				n++
			}
		}
		return n > c2, nil
	case mob.CondTargetCasting:
		return m.trickcasting > 0, nil
	}
	return false, nil
}

// hpPercent is get_percentage of HP.
func hpPercent(e *entity) int {
	if e.status == nil || e.status.MaxHP <= 0 {
		return 100
	}
	return int(max(e.hp, 0) * 100 / int64(e.status.MaxHP))
}

// commonStatuses are SC_COMMON_MIN to SC_COMMON_MAX: the ailments that
// "any" matches.
var commonStatuses = []int{sc.SCStone, sc.SCFreeze, sc.SCStun, sc.SCSleep, sc.SCPoison, sc.SCCurse,
	sc.SCSilence, sc.SCConfusion, sc.SCBlind, sc.SCBleeding, sc.SCDpoison, sc.SCStonewait}

// statusCondition is mob_getstatus_sub.
func statusCondition(e *entity, c mob.Condition, status int) bool {
	found := false
	if status == mob.AnyBadStatus {
		for _, id := range commonStatuses {
			if e.sc.Has(id) {
				found = true
				break
			}
		}
	} else {
		found = e.sc.Has(status)
	}
	switch c {
	case mob.CondHasStatus, mob.CondFriendHasStatus:
		return found
	}
	return !found
}

// mobFriendHP is mob_getfriendhprate: an ally within 8 cells whose HP is
// within the percents, monsters for a monster and players for a summon
// with a special AI. The monster itself does not count.
func (z *Zone) mobFriendHP(e *entity, lo, hi int) *entity {
	m := e.monster
	for o := range z.entities.all() {
		if o == e || o.dead || chebyshev(e, o) > 8 {
			continue
		}
		if m.specialAI != aiNone {
			if o.player == nil {
				continue
			}
		} else if o.monster == nil {
			continue
		}
		if isEnemy(e, o) {
			continue
		}
		if p := hpPercent(o); p >= lo && p <= hi {
			return o
		}
	}
	return nil
}

// mobFriendStatus is mob_getfriendstatus: a monster ally within 8 cells
// whose statuses fit the condition.
func (z *Zone) mobFriendStatus(e *entity, c mob.Condition, status int) *entity {
	var found *entity
	for o := range z.entities.all() {
		if o == e || o.monster == nil || o.dead || chebyshev(e, o) > 8 || isEnemy(e, o) {
			continue
		}
		if statusCondition(o, c, status) {
			found = o
		}
	}
	return found
}

// countSlaves is mob_countslave.
func (z *Zone) countSlaves(e *entity) int {
	n := 0
	for o := range z.entities.all() {
		if o.monster != nil && o.monster.master == e && !o.dead {
			n++
		}
	}
	return n
}

// countTargeted is unit_counttargeted: the units that attack e.
func (z *Zone) countTargeted(e *entity) int {
	n := 0
	for o := range z.entities.all() {
		if o.target == e && !o.dead {
			n++
		}
	}
	return n
}

// mobGetEnemy is battle_getenemy: a random enemy within range that the
// monster sees, of the first 32 found.
func (z *Zone) mobGetEnemy(e *entity, rng int) *entity {
	var list []*entity
	for _, o := range z.enemyCandidates(e, rng) {
		if len(list) >= maxEnemySearch {
			break
		}
		if z.visible(e, o, false) && isEnemy(e, o) {
			list = append(list, o)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list[z.rand.IntN(len(list))]
}

// mobCastEntry is the execution part of mobskill_use: it picks the target
// of the entry and starts the skill with the entry's cast time.
func (z *Zone) mobCastEntry(e *entity, i int, s *mob.Skill, d *skill.Def, fbl *entity) bool {
	m := e.monster
	rng := d.RangeFor(e.status, s.Level)
	o := &castOverride{ms: s.CastTime, cancel: s.Cancelable}
	var bl *entity
	if d.Target == skill.TargetGround {
		switch {
		case s.Target == mob.TargetRandom:
			bl = z.mobGetEnemy(e, rng)
		case s.Target == mob.TargetTarget || s.Target >= mob.TargetAround5 && s.Target <= mob.TargetAround8:
			bl = m.target
		case s.Target == mob.TargetMaster && m.master != nil:
			bl = m.master
		case s.Target == mob.TargetMaster || s.Target == mob.TargetFriend:
			bl = e
			if fbl != nil {
				bl = fbl
			}
		default:
			bl = e
		}
		if bl == nil || !z.entities.has(bl) {
			return false
		}
		x, y := bl.x, bl.y
		if s.Target.Around() {
			size, _ := s.Target.AroundSize()
			x, y, _ = z.searchFreeCellReach(x, y, size, size, e)
		}
		m.skillIdx = i
		m.lastUse = mobUse{idx: i, x: x, y: y, ground: true}
		if !z.inRange(e, bl, rng) || !z.skillUsePosWith(e, d, s.Level, x, y, o) {
			return false
		}
		m.lastSkill = s.ID
		return true
	}
	switch s.Target {
	case mob.TargetRandom:
		bl = z.mobGetEnemy(e, rng)
	case mob.TargetTarget:
		bl = m.target
		// A monster that cannot attack aims at its last attacker.
		if bl == nil && e.status.Mode&mob.ModeCanAttack == 0 {
			bl = m.attacked
		}
	case mob.TargetMaster:
		bl = e
		if m.master != nil {
			bl = m.master
		}
	case mob.TargetFriend:
		bl = e
		if fbl != nil {
			bl = fbl
		}
	default:
		bl = e
	}
	if bl == nil || !z.entities.has(bl) {
		return false
	}
	m.skillIdx = i
	m.lastUse = mobUse{idx: i, target: bl}
	if bl != e && !z.inRange(e, bl, rng) || !z.skillUseIDWith(e, bl, d, s.Level, o) {
		return false
	}
	m.lastSkill = s.ID
	return true
}

// mobSkillEvent is mobskill_event: a hit, a skill or a cast on the
// monster may make it use a skill. kind is the condition the hit
// matches: hit_melee, hit_ranged, hit_by_skill with skillID,
// targeted_by_cast, or mobEventDamage.
func (z *Zone) mobSkillEvent(e, src *entity, kind mob.Condition, skillID int, damage int64) bool {
	m := e.monster
	if m == nil || e.dead || !z.entities.has(e) {
		return false
	}
	prev := m.target
	if prev == nil {
		// mob_changetarget_byskill is off: only a monster without a
		// target looks at the source.
		m.target = src
	}
	used := z.mobSkillUseEvent(e, kind, skillID, damage)
	if !used || !isEnemy(e, src) {
		m.target = prev
	}
	return used
}

// mobHitEvents is the monster part of battle_damage for a hit that
// connected: the skill events of the hit, then the mob_attacked timer.
// flag is the hit's battle flags, and shown the delay before its damage
// shows, which battle_delay_damage waits for.
func (z *Zone) mobHitEvents(src, t *entity, damage int64, hits, flag, skillID int, normal bool, shown int64) {
	if t.monster == nil || src == nil || t.dead {
		return
	}
	if src != t {
		if damage > 0 {
			switch {
			case flag&battle.FlagShort != 0:
				z.mobSkillEvent(t, src, mob.CondHitMelee, 0, damage)
			case flag&battle.FlagLong != 0 && flag&battle.FlagMagic == 0:
				z.mobSkillEvent(t, src, mob.CondHitRanged, 0, damage)
			default:
				z.mobSkillEvent(t, src, mobEventDamage, 0, damage)
			}
		}
		if skillID > 0 && !t.dead {
			z.mobSkillEvent(t, src, mob.CondHitBySkill, skillID, damage)
		}
	}
	if !t.dead && z.entities.has(t) {
		z.mobAttackedLater(t, src, damage, hits, normal, shown)
	}
}

// mobCastEnd is the monster part of the start of skill_castend_id and
// skill_castend_pos, which happens even when the skill fails: the attack
// delay, the AI's rest, the skill delays and the entry's emotion.
func (z *Zone) mobCastEnd(e *entity) {
	m := e.monster
	if m == nil || e.status == nil {
		return
	}
	e.nextAttack = z.now + int64(e.status.ADelay)
	m.nextThink = max(z.now+int64(e.status.AMotion), m.nextThink)
	z.mobSkillDelay(e)
	if m.skillIdx >= 0 && m.skillIdx < len(m.spawn.Skills) {
		// The entry's Emotion column shows here. NPC_EMOTION's own
		// emotion, val1, shows when it takes effect, at the same time,
		// so the zone shows both here, in turn.
		for _, em := range m.spawn.Skills[m.skillIdx].Emotions {
			if em >= 0 {
				z.emotions = append(z.emotions, npcEmotion{e, uint32(em)}) // #nosec G115 -- checked above.
			}
		}
	}
}

// mobCastCancel is the monster part of unit_skillcastcancel: the attack
// delay and the skill delays, and no skill in use.
func (z *Zone) mobCastCancel(e *entity) {
	m := e.monster
	if m == nil || e.status == nil {
		return
	}
	e.nextAttack = z.now + int64(e.status.ADelay)
	z.mobSkillDelay(e)
	m.skillIdx = -1
}

// mobSkillDelay is mobskill_delay: the delay of the skill just used
// applies to every entry of that skill.
func (z *Zone) mobSkillDelay(e *entity) {
	m := e.monster
	ms := m.spawn.Skills
	if m.skillIdx < 0 || m.skillIdx >= len(ms) {
		return
	}
	delay := mob.DelayEntries(ms, m.skillIdx, m.ai.State, m.target != nil)
	id := ms[m.skillIdx].ID
	for i := range ms {
		if ms[i].ID == id {
			m.skillDelay[i] = z.now + int64(delay)
		}
	}
}

// mobChat is mob_chat_display_message: the monster says a line to the
// players near it. rAthena sends "name : line"; the protocol carries the
// name apart, and the client shows it before the line.
func (z *Zone) mobChat(e *entity, text string, color uint32) {
	name := e.monster.spawn.Name
	for i := range len(name) {
		if name[i] == '#' {
			name = name[:i]
			break
		}
	}
	z.chats = append(z.chats, chatLine{speaker: e, name: name, text: text, color: color})
}

// mobDeadSkills is the start of mob_dead: the monster's dead state
// skills.
func (z *Zone) mobDeadSkills(e *entity) {
	m := e.monster
	if m == nil || len(m.spawn.Skills) == 0 {
		return
	}
	m.ai.SetState(mob.StateDead, e.status.Mode)
	z.mobSkillUse(e, mobEventNone, 0)
}

// castSensor is the cast sensor of unit_skilluse_id2: a player's cast on
// a monster tells it so (targeted_by_cast), and a monster that senses
// casts takes the caster as its target while idle or chasing.
func (z *Zone) castSensor(src, target *entity, castMs int) {
	if src.player == nil || target == nil || target.monster == nil {
		return
	}
	if !z.renewal() && castMs <= 0 {
		return
	}
	z.mobSkillEvent(target, src, mob.CondTargetedByCast, 0, 0)
	mode := target.status.Mode
	if mode&(mob.ModeCastSensorIdle|mob.ModeCastSensorChase) == 0 || !isEnemy(target, src) {
		return
	}
	m := target.monster
	switch m.ai.State {
	case mob.StateRush, mob.StateFollow:
		if mode&mob.ModeCastSensorChase == 0 {
			return
		}
	case mob.StateIdle, mob.StateWalk:
		if mode&mob.ModeCastSensorIdle == 0 {
			return
		}
	default:
		return
	}
	m.target = src
	m.ai.Aggressive = mode&mob.ModeAngry != 0
}
