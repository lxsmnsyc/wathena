package zone

import (
	"math"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Casting: a unit uses a skill, as rAthena's clif_parse_UseSkillToId and
// clif_parse_UseSkillToPos, unit_skilluse_id2 and unit_skilluse_pos2,
// skill_castend_id and skill_castend_pos. The settings are rAthena's
// shipped conf/battle values.

const (
	// securityCastTime is SECURITY_CASTTIME: a delay the cast start adds
	// to canact_tick, which the cast end takes back.
	securityCastTime = 100
	// defaultWalkDelay is default_walk_delay: a player cannot walk for
	// this long after a skill.
	defaultWalkDelay = 300
	// maxASPDDelay is pc_maxaspd / AMOTION_DIVIDER_PC with max_aspd 190:
	// the act delay a cast start adds to the attack motion.
	maxASPDDelay = 100
)

// casting is a unit's skill cast: rAthena's ud->skilltimer with
// skill_id, skill_lv, skilltarget and skillx/skilly.
type casting struct {
	def *skill.Def
	lv  int
	// target is the target of an id skill. A ground skill has none, and
	// x and y instead.
	target *entity
	x, y   int
	ground bool
	// end is when the cast ends, in zone milliseconds.
	end int64
	// cancel is ud->state.skillcastcancel: hits can stop the cast.
	cancel bool
}

// pendingSkill is a skill to cast once a walk brings the caster in range.
type pendingSkill struct {
	def    *skill.Def
	lv     int
	target *entity
	x, y   int
	ground bool
}

// skillFail is clif_skill_fail: the player sees why the skill failed.
func (z *Zone) skillFail(e *entity, id int, reason wathenav1.SkillFailure) {
	p := e.player
	if p == nil {
		return
	}
	p.pending = append(p.pending, wathenav1.Event_builder{SkillFailed: wathenav1.SkillFailed_builder{
		SkillId: uint32(id), Reason: reason, // #nosec G115 -- a skill ID.
	}.Build()}.Build())
}

// cooldownEvent tells the player that a skill, or every skill for 0, is
// on cooldown, as clif_skill_cooldown and the post-delay bar.
func cooldownEvent(id int, ms int64) *wathenav1.Event {
	return wathenav1.Event_builder{SkillCooldown: wathenav1.SkillCooldown_builder{
		SkillId: uint32(id), Ms: uint32(max(ms, 0)), // #nosec G115 -- small values.
	}.Build()}.Build()
}

// useSkill is clif_parse_UseSkillToId and clif_parse_UseSkillToPos: the
// checks before a player's skill starts.
func (z *Zone) useSkill(p *Player, c *useSkillCommand) {
	if !z.skillsReady() {
		return
	}
	e := p.entity
	def := z.Skills.Get(c.skill)
	if def == nil || def.Passive() {
		return
	}
	lv := max(c.level, 1)
	ground := def.Target == skill.TargetGround
	if ground != c.target.HasCell {
		return
	}
	// pc_cant_act2: a body state such as stun or stone stops every skill
	// before it starts. Overweight 90% is checked with the requirements.
	if l := e.sc; l != nil && l.Opt1 != 0 && l.Opt1 != sc.Opt1Burning {
		return
	}
	// skill_isNotOk: the skill's cooldown.
	if until, ok := p.cooldowns[def.ID]; ok && z.now < until {
		z.skillFail(e, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_SKILLINTERVAL)
		return
	}
	if e.cast != nil {
		return
	}
	if z.now < e.canAct {
		z.skillFail(e, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_SKILLINTERVAL)
		return
	}
	// The level is at most the one the player has.
	lv = min(lv, p.skillLevel(def.ID))
	if lv <= 0 {
		return
	}
	if ground {
		z.skillUsePos(e, def, lv, c.target.X, c.target.Y)
		return
	}
	target := e
	if def.Target != skill.TargetSelf && c.target.HasEntity {
		target = nil
		for o, id := range p.netIDs.all() {
			if id == c.target.Entity {
				target = o
				break
			}
		}
	}
	if target == nil || target.unit != nil || target.item != nil || target.warp != nil {
		return
	}
	z.skillUseID(e, target, def, lv)
}

// skillUseID is unit_skilluse_id2: a unit starts a skill on a target.
// It reports whether the cast started.
func (z *Zone) skillUseID(src, target *entity, def *skill.Def, lv int) bool {
	if src.dead || !z.entities.has(target) {
		return false
	}
	res := def.CastTimeFor(z.caster(src), lv, z.renewal(), src.player == nil || src.player.char.Skills[def.ID] > 0)
	z.castSideEffects(src, res)
	if src.cast != nil {
		return false
	}
	if def.Has("NoTargetSelf") && src == target {
		return false
	}
	if !z.statusCheckSkillUse(src, target, def, 0) {
		return false
	}
	if src.player != nil && !z.checkCastBegin(src, def, lv) {
		return false
	}
	if !z.hookCastAllowed(src, target, def, lv) {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return false
	}
	rng := def.RangeFor(src.status, lv)
	if src != target && !z.checkRange(src, target.x, target.y, rng) {
		// rAthena's stepaction: a walking player casts once a step brings
		// it in range. Here the server walks the player there.
		z.approach(src, &pendingSkill{def: def, lv: lv, target: target}, target.x, target.y)
		return false
	}
	src.pending = nil
	src.target = nil
	src.cast = &casting{def: def, lv: lv, target: target, cancel: def.CastCancel}
	return z.castBegin(src, z.hookCastTime(src, target, def, lv, res.Milliseconds))
}

// skillUsePos is unit_skilluse_pos2: a unit starts a ground skill on a
// cell.
func (z *Zone) skillUsePos(src *entity, def *skill.Def, lv, x, y int) bool {
	if src.dead || src.cast != nil {
		return false
	}
	if x < 0 || y < 0 || x >= z.grid.Width || y >= z.grid.Height {
		return false
	}
	res := def.CastTimeFor(z.caster(src), lv, z.renewal(), src.player == nil || src.player.char.Skills[def.ID] > 0)
	z.castSideEffects(src, res)
	if src.player != nil {
		if !z.checkCastBegin(src, def, lv) {
			return false
		}
		// Fire Wall checks its instance limit at the start too.
		if def.ID == skill.IDFireWall && !z.posMaxCount(src, def, lv, x, y, true) {
			return false
		}
	}
	if !z.statusCheckSkillUse(src, nil, def, 0) {
		return false
	}
	if !z.hookCastAllowed(src, nil, def, lv) {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return false
	}
	rng := def.RangeFor(src.status, lv)
	if !z.checkRange(src, x, y, rng) {
		z.approach(src, &pendingSkill{def: def, lv: lv, x: x, y: y, ground: true}, x, y)
		return false
	}
	src.pending = nil
	src.target = nil
	src.cast = &casting{def: def, lv: lv, x: x, y: y, ground: true}
	return z.castBegin(src, z.hookCastTime(src, nil, def, lv, res.Milliseconds))
}

// castSideEffects ends the statuses a cast time calculation used up.
func (z *Zone) castSideEffects(src *entity, res skill.CastResult) {
	en := z.statusEngine()
	if en == nil || src.sc == nil {
		return
	}
	if res.EndSuffragium {
		en.End(z.unit(src), sc.SCSuffragium, sc.ReasonRemoved)
	}
	if res.UseMemorize {
		if e := src.sc.Get(sc.SCMemorize); e != nil {
			e.Val2--
			if e.Val2 <= 0 {
				en.End(z.unit(src), sc.SCMemorize, sc.ReasonExpired)
			}
		}
	}
}

// castBegin is the end of unit_skilluse_id2 and unit_skilluse_pos2: the
// unit stops, everyone near sees the cast, the delays start, and the
// skill takes effect now or when the cast time ends.
func (z *Zone) castBegin(src *entity, castMs int) bool {
	c := src.cast
	def, lv := c.def, c.lv
	if len(src.path) > 0 {
		src.path = src.path[:0]
		src.stopped = true
	}
	if castMs <= 0 {
		c.cancel = false
	}
	ev := skillEvent{kind: skillEventCast, src: src, target: c.target, x: c.x, y: c.y, cell: c.ground,
		skill: def, lv: lv, castMs: castMs}
	z.skillEvents = append(z.skillEvents, ev)
	// unit_set_attackdelay and unit_set_castdelay.
	if src.player != nil && src.status != nil {
		src.canAct = max(src.canAct, z.now+int64(src.status.AMotion)+maxASPDDelay)
		src.nextAttack = max(src.nextAttack, z.now+int64(src.status.ADelay))
	}
	castDelay := int64(0)
	if def.CastTime.At(lv) != 0 {
		castDelay = int64(castMs)
	}
	src.canAct = max(src.canAct, z.now+max(castDelay, skill.MinSkillDelayLimit)) + securityCastTime
	if castMs > 0 {
		c.end = z.now + int64(castMs)
		return true
	}
	z.castEnd(src, false)
	return true
}

// advanceCasts runs the casts whose time ended.
func (z *Zone) advanceCasts() {
	var due []*entity
	for e := range z.entities.all() {
		if e.cast != nil && z.now >= e.cast.end {
			due = append(due, e)
		}
	}
	for _, e := range due {
		if e.cast != nil && z.entities.has(e) {
			z.castEnd(e, true)
		}
	}
	z.advancePending()
}

// castCancel is unit_skillcastcancel: the cast stops. With onlyIfAllowed
// set (type 2), it only stops a cast that hits may stop, and not for a
// player with Phen Card's bonus. The unit may act again at once.
func (z *Zone) castCancel(e *entity, onlyIfAllowed bool) bool {
	c := e.cast
	if c == nil {
		return false
	}
	if onlyIfAllowed {
		if !c.cancel {
			return false
		}
		if st := e.status; st != nil && st.Player != nil && (st.Player.Cast.NoCastCancel2 || st.Player.Cast.NoCastCancel) {
			// bNoCastCancel only works outside guild wars, which this
			// server does not have.
			return false
		}
	}
	e.canAct = z.now
	e.cast = nil
	z.skillEvents = append(z.skillEvents, skillEvent{kind: skillEventCancel, src: e})
	return true
}

// castEnd is skill_castend_id and skill_castend_pos: the cast time
// ended, or the skill had none. timed is set when the cast had a timer.
func (z *Zone) castEnd(src *entity, timed bool) {
	c := src.cast
	src.cast = nil
	if c == nil {
		return
	}
	ok := false
	if c.ground {
		ok = z.castEndPos(src, c, timed)
	} else {
		ok = z.castEndID(src, c, timed)
	}
	if !ok {
		// A failed cast clears the act delay.
		src.canAct = z.now
	}
}

// castEndID is skill_castend_id.
func (z *Zone) castEndID(src *entity, c *casting, timed bool) bool {
	def, lv, target := c.def, c.lv, c.target
	if src.dead {
		return false
	}
	if target == nil || !z.entities.has(target) || target.dead && def.ID != skill.IDResurrection {
		return false
	}
	if r := z.castEndIDCheck(src, target, def); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		if r != skillFailSilent {
			z.skillFail(src, def.ID, r)
		}
		return false
	}
	if timed && !z.statusCheckSkillUse(src, target, def, 1) {
		return false
	}
	// OFFICIAL_WALKPATH: a skill that is not a no-damage skill needs a
	// clear shot at cast end, and costs its requirements anyway.
	if def.CastType() != skill.CastNoDamage {
		if clear, _ := path.Shoot(z.grid, src.x, src.y, target.x, target.y); !clear {
			if p := src.player; p != nil {
				z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
				z.consumeRequirement(src, def, lv, 3)
			}
			return false
		}
	}
	if src.player != nil {
		if !z.checkCastEnd(src, def, lv) {
			return false
		}
		z.consumeRequirement(src, def, lv, 1)
		// A damage skill on a skill immune target fails after it paid.
		if src != target && skillImmune(target) && def.CastType() == skill.CastDamage {
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return false
		}
	}
	if len(src.path) > 0 {
		src.path = src.path[:0]
		src.stopped = true
	}
	z.afterCast(src, def, lv, false)
	if def.CastType() == skill.CastNoDamage {
		z.castendNoDamage(src, target, def, lv, 0)
	} else {
		z.castendDamage(src, target, def, lv, 0)
	}
	return true
}

// castEndIDCheck is skill_castend_id_check: whether the target fits the
// skill, as battle_check_target. Attack skills need an enemy, and skills
// with NoTargetEnemy a friend.
func (z *Zone) castEndIDCheck(src, target *entity, def *skill.Def) wathenav1.SkillFailure {
	if src != target && skillImmune(target) && def.CastType() == skill.CastNoDamage {
		return skillFailSilent
	}
	switch {
	case def.Target == skill.TargetEnemy || def.Target == skill.TargetSelf && def.Has("NoTargetSelf"):
		if !isEnemy(src, target) {
			return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
		}
	case def.Has("NoTargetEnemy"):
		if isEnemy(src, target) {
			return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
		}
	}
	if (def.Has("PartyOnly") || def.Has("GuildOnly")) && src != target {
		// Parties and guilds are not ported, so only the caster counts.
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

// skillFailSilent is USESKILL_FAIL_MAX: a failure the player is not told
// about.
const skillFailSilent = wathenav1.SkillFailure(-1)

// skillImmune reports whether a monster has MD_SKILLIMMUNE.
func skillImmune(e *entity) bool {
	return e.monster != nil && e.status != nil && e.status.Mode&modeSkillImmune != 0
}

// castEndPos is skill_castend_pos.
func (z *Zone) castEndPos(src *entity, c *casting, timed bool) bool {
	def, lv := c.def, c.lv
	if src.dead {
		return false
	}
	if !z.posMaxCount(src, def, lv, c.x, c.y, true) {
		return false
	}
	if timed && !z.statusCheckSkillUse(src, nil, def, 1) {
		return false
	}
	if src.player != nil {
		if def.ID != skill.IDWarp && !z.checkCastEnd(src, def, lv) {
			return false
		}
		z.consumeRequirement(src, def, lv, 1)
	}
	if len(src.path) > 0 {
		src.path = src.path[:0]
		src.stopped = true
	}
	z.afterCast(src, def, lv, true)
	z.castendPos(src, c.x, c.y, def, lv, 0)
	return true
}

// afterCast sets the delays of a cast that ends well: the after-cast
// delay (canact_tick), the cooldown, and the walk delay.
func (z *Zone) afterCast(src *entity, def *skill.Def, lv int, ground bool) {
	delay := int64(def.Delay(z.caster(src), lv))
	src.canAct = max(z.now+delay, src.canAct-securityCastTime)
	if p := src.player; p != nil {
		p.pending = append(p.pending, cooldownEvent(0, src.canAct-z.now))
		if cd := def.CooldownFor(z.caster(src), lv); cd > 0 {
			if p.cooldowns == nil {
				p.cooldowns = map[int]int64{}
			}
			p.cooldowns[def.ID] = z.now + int64(cd)
			p.pending = append(p.pending, cooldownEvent(def.ID, int64(cd)))
		}
	}
	walk := int64(def.AfterCastWalkDelay.At(lv))
	if src.player != nil {
		walk += defaultWalkDelay
	}
	// A ground skill always sets the walk delay.
	if (ground || def.Requires.State != stateMoveEnable) && walk > 0 {
		z.setWalkDelay(src, walk)
	}
}

// stateMoveEnable is ST_MOVE_ENABLE: a skill that leaves the caster free
// to walk.
const stateMoveEnable = 7

// setWalkDelay is unit_set_walkdelay with type 1: the unit cannot walk
// for a time. Bosses ignore it.
func (z *Zone) setWalkDelay(e *entity, ms int64) {
	if st := e.status; st != nil && st.Mode&modeStatusImmune != 0 {
		return
	}
	e.canMove = max(e.canMove, z.now+ms)
	if len(e.path) > 0 {
		e.path = e.path[:0]
		e.stopped = true
	}
}

// Monster modes that skills read, as rAthena's e_mode.
const (
	modeStatusImmune = 0x4000000
	modeSkillImmune  = 0x8000000
	modeKnockbackImm = 0x200000
	modeDetector     = 0x2000000
)

// approach walks a player toward a target it is out of range of, and
// keeps the skill to cast once it gets in range.
func (z *Zone) approach(src *entity, ps *pendingSkill, x, y int) {
	if src.player == nil || src.cant().Move || z.now < src.canMove {
		return
	}
	src.pending = ps
	src.target = nil
	z.walk(src, x, y)
	if len(src.path) == 0 {
		src.pending = nil
	}
}

// advancePending casts the skills of units that walked into range, and
// drops those whose walk ended without reaching it.
func (z *Zone) advancePending() {
	for e := range z.entities.all() {
		ps := e.pending
		if ps == nil {
			continue
		}
		if e.dead || e.cast != nil || ps.target != nil && (!z.entities.has(ps.target) || ps.target.dead) {
			e.pending = nil
			continue
		}
		x, y := ps.x, ps.y
		if ps.target != nil {
			x, y = ps.target.x, ps.target.y
		}
		rng := ps.def.RangeFor(e.status, ps.lv)
		// A step brings the unit within range - 1, as unit_walktoxy_timer.
		if z.checkRange(e, x, y, max(rng-1, 0)) || len(e.path) == 0 {
			e.pending = nil
			if len(e.path) > 0 {
				e.path = e.path[:0]
				e.stopped = true
			}
			if z.now < e.canAct {
				continue
			}
			if ps.ground {
				z.skillUsePos(e, ps.def, ps.lv, x, y)
			} else {
				z.skillUseID(e, ps.target, ps.def, ps.lv)
			}
		}
	}
}

// checkRange is battle_check_range: the distance, circular for players
// (check_distance_client) and square for other units, then a clear shot
// beyond 1 cell.
func (z *Zone) checkRange(src *entity, x, y, rng int) bool {
	dx, dy := x-src.x, y-src.y
	if src.player != nil {
		if clientDistance(dx, dy) > rng {
			return false
		}
	} else if max(abs(dx), abs(dy)) > rng {
		return false
	}
	d := max(abs(dx), abs(dy))
	if d < 2 {
		return true
	}
	if d > ViewRange {
		return false
	}
	clear, _ := path.Shoot(z.grid, src.x, src.y, x, y)
	return clear
}

// statusCheckSkillUse is status_check_skilluse: whether the caster's
// statuses let it use a skill, and whether the target can be targeted.
// flag is 0 when the skill starts, 1 when its cast ends, and 2 when it
// hits.
func (z *Zone) statusCheckSkillUse(src, target *entity, def *skill.Def, flag int) bool {
	if src != nil {
		if src.player == nil && src.dead {
			return false
		}
		if l := src.sc; !l.Empty() {
			if l.Opt1 != 0 && l.Opt1 != sc.Opt1StoneWait && l.Opt1 != sc.Opt1Burning {
				if flag != 1 {
					return false
				}
				if def.CastType() == skill.CastDamage {
					return false
				}
			}
			if l.Has(sc.SCTrickdead) && def.ID != skill.IDTrickDead {
				return false
			}
			if flag == 0 && l.Cant.Cast {
				return false
			}
			if l.Option&sc.OptionHide != 0 && src.player != nil && !def.Has("AllowWhenHidden") {
				return false
			}
		}
	}
	if target == nil || target == src {
		return true
	}
	hide := sc.OptionHide
	if flag == 0 {
		hide |= sc.OptionCloak | optionChasewalk
	}
	if def.Has("TargetHidden") {
		hide &^= sc.OptionHide
	}
	if l := target.sc; !l.Empty() && l.Option&hide != 0 {
		// A boss or a detector sees hidden units.
		if src == nil || src.status == nil || src.status.Mode&(modeStatusImmune|modeDetector) == 0 {
			return false
		}
	}
	return true
}

// caster is the unit as the cast formulas see it.
func (z *Zone) caster(e *entity) *skill.Caster {
	st := e.status
	if st == nil {
		st = &npcStatus
	}
	return &skill.Caster{Status: st, Changes: e.sc, Monster: e.player == nil}
}

// renewal reports whether the zone runs renewal's formulas.
func (z *Zone) renewal() bool {
	if z.Skills != nil {
		return z.Skills.Renewal()
	}
	if z.Status != nil {
		return z.Status.Renewal()
	}
	return true
}

// checkCastBegin is skill_check_condition_castbegin for a player: the
// requirements checked when the cast starts. It sends the failure.
func (z *Zone) checkCastBegin(src *entity, def *skill.Def, lv int) bool {
	p := src.player
	if p.overweight == item.Weight90 {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_WEIGHTOVER)
		return false
	}
	// skill_disable_check: a toggle skill whose status is on costs
	// nothing to turn off.
	if def.Has("Toggleable") && def.Status >= 0 && src.sc.Has(def.Status) {
		return true
	}
	req := z.requirement(src, def, lv)
	// The skill's own checks.
	if impl, ok := skillImpls[def.ID].(castBeginChecker); ok {
		if r := impl.castBegin(z, src, def, lv); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
			z.skillFail(src, def.ID, r)
			return false
		}
	}
	switch req.State {
	case skill.StateCart:
		if !z.hasCart(p) {
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_CART)
			return false
		}
	case skill.StateHidden:
		if src.sc.Empty() || src.sc.Option&(sc.OptionHide|sc.OptionCloak|optionChasewalk) == 0 {
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return false
		}
	case skill.StateShield:
		if p.Look.Shield == 0 {
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return false
		}
	case skill.StateWater:
		// Water cells are not in the map data yet.
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return false
	case skill.StateFalcon, skill.StateRiding:
		// Falcons and mounts are not ported.
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return false
	}
	for _, s := range req.Status {
		if !src.sc.Has(s) {
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return false
		}
	}
	for _, it := range req.Equipment {
		if !z.wears(p, uint32(it)) { // #nosec G115 -- item IDs are positive.
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return false
		}
	}
	if req.MaxHP > 0 && src.status != nil && src.status.MaxHP > 0 && src.hp*100/int64(src.status.MaxHP) > int64(req.MaxHP) {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_HP_INSUFFICIENT)
		return false
	}
	if req.Weapon != 0 && !checkWeaponType(src, req.Weapon) {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_THIS_WEAPON)
		return false
	}
	if req.SP > 0 && p.sp < int64(req.SP) {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_SP_INSUFFICIENT)
		return false
	}
	if req.Zeny > 0 && p.record.Zeny < int64(req.Zeny) {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_MONEY)
		return false
	}
	if req.SpiritSphere > 0 {
		// Spirit spheres are not ported.
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return false
	}
	return true
}

// checkCastEnd is skill_check_condition_castend for a player: HP, the
// weapon, ammunition and items.
func (z *Zone) checkCastEnd(src *entity, def *skill.Def, lv int) bool {
	p := src.player
	if p.overweight == item.Weight90 {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_WEIGHTOVER)
		return false
	}
	if def.Has("Toggleable") && def.Status >= 0 && src.sc.Has(def.Status) {
		return true
	}
	req := z.requirement(src, def, lv)
	if req.HP > 0 && src.hp <= int64(req.HP) {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_HP_INSUFFICIENT)
		return false
	}
	if req.Weapon != 0 && !checkWeaponType(src, req.Weapon) {
		z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_THIS_WEAPON)
		return false
	}
	if req.AmmoAmount > 0 {
		slot, ok := z.equippedAmmo(p)
		if !ok {
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_ARROW)
			return false
		}
		s := &p.inventory.Slots[slot]
		d := z.Items.Get(s.ID)
		if req.Ammo&(1<<d.Subtype) == 0 {
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_ARROW)
			return false
		}
		if s.Amount < req.AmmoAmount {
			z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_ARROW)
			return false
		}
	}
	for _, it := range req.Items {
		if z.countItem(p, uint32(it[0])) < it[1] { // #nosec G115 -- item IDs are positive.
			switch it[0] {
			case itemHolyWater:
				z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_HOLYWATER)
			case itemRedGemstone:
				z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_REDJAMSTONE)
			case itemBlueGemstone:
				z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_BLUEJAMSTONE)
			default:
				z.skillFail(src, def.ID, wathenav1.SkillFailure_SKILL_FAILURE_STUFF_INSUFFICIENT)
			}
			return false
		}
	}
	return true
}

// Items that skills use.
const (
	itemRedGemstone  = 716
	itemBlueGemstone = 717
	itemHolyWater    = 523
	itemEmptyBottle  = 713
	itemStone        = 7049
)

// consumeRequirement is skill_consume_requirement: with typ&1, the HP,
// SP and zeny a skill costs, before it takes effect; with typ&2, its
// items, after.
func (z *Zone) consumeRequirement(src *entity, def *skill.Def, lv, typ int) {
	p := src.player
	if p == nil {
		return
	}
	req := z.requirement(src, def, lv)
	if typ&1 != 0 {
		sp := int64(req.SP)
		hp := int64(req.HP)
		switch def.ID {
		case skill.IDAutoBerserk:
			// Auto Berserk needs SP and spends none.
			sp = 0
		case skill.IDMagnumBreak:
			// skill_consume_hpspap: Magnum Break's HP is required, not
			// taken.
			hp = 0
		}
		if def.Has("Toggleable") && def.Status >= 0 && src.sc.Has(def.Status) {
			hp, sp = 0, 0
		}
		if p.autocast {
			sp = 0
		}
		if hp > 0 || sp > 0 {
			// status_zap: passive, so it does not stop a cast.
			hp = min(hp, src.hp)
			if hp > 0 {
				src.hp -= hp
				src.hpChanged = true
			}
			if sp > 0 {
				p.sp = max(p.sp-sp, 0)
			}
			if s := z.battleStats(p); s != nil {
				p.pending = append(p.pending, s)
			}
		}
		if req.Zeny > 0 {
			z.setZeny(p, max(p.record.Zeny-int64(req.Zeny), 0))
		}
	}
	if typ&2 != 0 {
		for _, it := range req.Items {
			z.takeItems(p, uint32(it[0]), it[1]) // #nosec G115 -- item IDs are positive.
		}
	}
}

// checkWeaponType is pc_check_weapontype: the player's weapon type, or
// for a weapon in each hand either hand's, is in the mask.
func checkWeaponType(e *entity, mask int) bool {
	st := e.status
	if st == nil || st.Player == nil {
		return true
	}
	pl := st.Player
	if pl.Weapon < maxWeaponType {
		return mask&(1<<pl.Weapon) != 0
	}
	return mask&(1<<pl.RightWeapon|1<<pl.LeftWeapon|1<<pl.Weapon) != 0
}

// maxWeaponType is MAX_WEAPON_TYPE: weapon types from it on are the
// W_DOUBLE_ types of a weapon in each hand.
const maxWeaponType = 24

// clientDistance is distance_client: the distance the client uses for
// ranges, the length of the offset less 0.1, truncated.
func clientDistance(dx, dy int) int {
	d := math.Sqrt(float64(dx*dx+dy*dy)) - 0.1
	if d < 0 {
		return 0
	}
	return int(d)
}

// requirement is skill_get_requirement for a player, with the
// skill_cost hooks on the SP.
func (z *Zone) requirement(src *entity, def *skill.Def, lv int) skill.Requirement {
	var sp int64
	if p := src.player; p != nil {
		sp = p.sp
	}
	req := def.RequirementFor(z.caster(src), lv, src.hp, sp)
	req.SP = z.hookSkillCost(src, def, lv, req.SP)
	return req
}
