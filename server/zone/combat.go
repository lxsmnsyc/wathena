package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Combat: units attack a target in range at their attack delay, and walk
// into range first. A monster that is hit and can attack strikes back.
// Dead monsters come back after their respawn time. Dead players return to
// the start cell with full HP.

const (
	// reviveMs is how long a dead player lies before returning.
	reviveMs = 3000
	// chaseMs is how often a unit that chases a moving target looks for a
	// new path.
	chaseMs = 500
	// modeCanAttack is rAthena's MD_CANATTACK mode flag.
	modeCanAttack = 0x80
)

// fight is an entity's combat state.
type fight struct {
	status *battle.Status
	hp     int64
	// hpChanged is set when HP changed this tick, so the player gets its
	// own HP.
	hpChanged bool
	dead      bool
	// diedAt is when a dead player fell, in zone milliseconds.
	diedAt int64

	target     *entity
	repeat     bool
	nextAttack int64
	nextChase  int64
	chaseCell  [2]int
}

// combat is a zone's combat state for one tick.
type combat struct {
	// hits are this tick's attacks, sent to players who see both sides.
	hits []hit
	// removed are entities that left this tick, and why.
	removed []removal
	// respawns are monsters waiting to come back.
	respawns []respawn
}

type hit struct {
	source, target *entity
	// caster is the skill's caster when source is a ground unit, which
	// viewers may not see. skill is the skill of a skill hit, or nil.
	caster     *entity
	skill      *skill.Def
	result     battle.Result
	delay      int
	hpPermille uint32
}

type removal struct {
	e      *entity
	reason wathenav1.VanishReason
}

type respawn struct {
	spawn *Spawn
	at    int64
}

func (e *entity) hpPermille() uint32 {
	if e.status == nil || e.status.MaxHP <= 0 || e.hp <= 0 {
		if e.status == nil {
			return 1000
		}
		return 0
	}
	return uint32(min(1000, e.hp*1000/int64(e.status.MaxHP))) // #nosec G115 -- 0 to 1000.
}

func chebyshev(a, b *entity) int { return max(abs(a.x-b.x), abs(a.y-b.y)) }

// fight runs every unit's attack for this tick.
func (z *Zone) fight() {
	for e := range z.entities.all() {
		t := e.target
		if t == nil {
			continue
		}
		if e.dead || t.dead || !z.entities.has(t) || chebyshev(e, t) > ViewRange {
			e.target = nil
			continue
		}
		// unit_can_attack: statuses that stop attacking end the attack, and
		// a target that cannot be targeted ends it too.
		if e.cant().Attack || !z.canTarget(e, t) {
			e.target = nil
			continue
		}
		// mob_ai_sub_hard: a monster drops a target under Gangster's
		// Paradise.
		if gangsterProtects(e, t) {
			e.target = nil
			continue
		}
		reach := max(e.status.RHW.Range, 1)
		if chebyshev(e, t) > reach {
			if !e.cant().Move {
				z.chase(e, t)
			}
			continue
		}
		// In range: stop walking, then attack at the attack delay.
		if len(e.path) > 0 {
			e.path = e.path[:0]
			e.stopped = true
		}
		if z.now < e.nextAttack || e.cast != nil && !freeCast(e) {
			continue
		}
		e.dir = facingTo(t.x-e.x, t.y-e.y)
		z.strike(e, t)
		// unit_set_attackdelay keeps a later attack time, such as a
		// combo's.
		e.nextAttack = max(e.nextAttack, z.now+int64(max(e.status.ADelay, 100)))
		if !e.repeat {
			e.target = nil
		}
	}
}

// chase walks toward a target, looking for a new path when the target
// moves.
func (z *Zone) chase(e, t *entity) {
	cell := [2]int{t.x, t.y}
	if len(e.path) > 0 && (cell == e.chaseCell || z.now < e.nextChase) {
		return
	}
	e.chaseCell, e.nextChase = cell, z.now+chaseMs
	before := len(e.path)
	z.walk(e, t.x, t.y)
	if before == 0 && len(e.path) == 0 {
		// No path: give up.
		e.target = nil
	}
}

// strike runs one attack and applies its damage.
func (z *Zone) strike(e, t *entity) {
	// battle_weapon_attack: Blade Stop and Raging Trifecta Blow replace
	// the attack, the attack ends Cloaking, Auto Counter takes the
	// attack, and Poison React may block it.
	if z.weaponAttackSkills(e, t) {
		return
	}
	z.weaponAttackCloaking(e)
	if z.autoCounterBlock(e, t) || z.poisonReactBlock(e, t) {
		return
	}
	// Martyr's Reckoning takes the attack.
	if z.sacrificeAttack(e, t) {
		return
	}
	z.syncBattlePlayer(e)
	var r battle.Result
	if z.renewal() {
		r = battle.Attack(e.status, z.defenderFor(e, t, 0), z.rand)
	} else {
		r = battle.AttackPreRenewal(e.status, z.defenderFor(e, t, 0), z.rand)
	}
	// battle_do_reflect: Shield Reflect returns part of the hit.
	z.reflectShield(e, t, 0, r)
	// Poison React's effects inside battle_calc_attack: the boosted
	// attack's poison, then the target's counter.
	z.poisonReactBoost(e, t, r)
	z.poisonReactCounter(e, t, r)
	z.rejectSword(e, t, r)
	z.logDamage(e, t, r.Damage)
	if r.Damage > 0 {
		t.hp -= r.Damage
		t.hpChanged = true
	}
	z.hits = append(z.hits, hit{source: e, target: t, result: r, delay: e.status.AMotion, hpPermille: t.hpPermille()})
	z.itemHitEvents(e, t, r)
	landed := !r.Blocked && (r.Kind == battle.Hit || r.Kind == battle.Critical)
	if landed {
		// status_damage: statuses that damage ends do, such as Sleep, and
		// a hit of at least 1 damage stops a cast.
		z.damaged(t, r.Damage, e.monster != nil)
		if r.Damage > 0 {
			z.castCancel(t, true)
		}
	}
	// battle_weapon_attack: Hindsight's spell, whether the attack hit or
	// not. rAthena deals the attack's damage later, so the spell finds
	// the target alive.
	if !e.sc.Empty() && e.sc.Has(sc.SCAutospell) && !e.dead {
		z.autoSpellStatus(e, t)
	}
	if t.hp <= 0 {
		z.die(t, e)
		return
	}
	if landed {
		// skill_additional_effect and skill_counter_additional_effect:
		// the statuses that equipment bonuses cause.
		long := r.Flag&battle.FlagLong != 0
		z.additionalEffect(e, t, long)
		z.normalAttackPoison(e, t)
		z.breakOnHit(e, t, 0, r.Flag)
		z.counterEffect(e, t, long)
		if !t.dead && !e.dead {
			z.autoSpellAttack(e, t, r.Flag)
		}
		if r.Damage > 0 && !t.dead && !e.dead {
			z.autoSpellWhenHit(e, t, r.Flag, 0)
		}
	}
	// A monster that can attack strikes back, at an enemy.
	if t.monster != nil && t.target == nil && t.status.Mode&modeCanAttack != 0 && isEnemy(t, e) {
		t.target, t.repeat = e, true
		t.path = t.path[:0]
		t.nextAttack = z.now + int64(t.status.DMotion)
	}
}

// itemHitEvents runs the "damage" handlers of the equipped items of the
// players on both sides of a hit that landed (0002a, "Item events").
func (z *Zone) itemHitEvents(e, t *entity, r battle.Result) {
	if z.scripts == nil || r.Kind == battle.Miss || r.Kind == battle.LuckyDodge {
		return
	}
	if e.player == nil && t.player == nil {
		return
	}
	h := modscript.Hit{Kind: "physical", Range: "melee", Element: int64(e.status.RHW.Ele),
		Critical: r.Kind == battle.Critical, Amount: r.Damage}
	if r.Flag&battle.FlagLong != 0 {
		h.Range = "ranged"
	}
	handle := func(x *entity) (int64, string) {
		if x.player != nil {
			return int64(x.player.charID), "player"
		}
		return z.monsterHandle(x), "monster"
	}
	h.Source, h.SourceKind = handle(e)
	h.Target, h.TargetKind = handle(t)
	if p := e.player; p != nil {
		h.WearerIsSource = true
		z.fireHit(p, h)
	}
	if p := t.player; p != nil {
		h.WearerIsSource = false
		z.fireHit(p, h)
	}
}

// die ends a unit killed by another, or by its statuses when killer is
// nil.
func (z *Zone) die(e, killer *entity) {
	if e.dead {
		return
	}
	if z.inHost() {
		// A script's cast killed it: the death, with its events and drop
		// hooks, comes once the script's call returns.
		z.deferred = append(z.deferred, func() {
			if e.hp <= 0 && z.entities.has(e) {
				z.die(e, killer)
			}
		})
		return
	}
	// pc_dead and mob_dead clear the statuses first. The cast stops and
	// the unit's ground skills end.
	z.statusesDied(e)
	z.clearTimerSkills(e)
	if p := e.player; p != nil {
		// pc_dead: the spirit spheres go.
		z.delSpiritBall(p, p.spiritBalls())
	}
	z.castCancel(e, false)
	e.pending = nil
	z.clearUnitGroups(e)
	z.clearTimerSkills(e)
	e.dead = true
	e.target = nil
	e.pickItem = nil
	e.path = e.path[:0]
	for o := range z.entities.all() {
		if o.target == e {
			o.target = nil
		}
	}
	if e.player != nil {
		// Players lie where they fell, then return to their save point.
		e.diedAt = z.now
		// status_damage resets the regen waits on death.
		e.player.regenTicks = regenState{}
		// pc_dead closes the conversation. The zone does it once no
		// entity list is walked.
		if e.player.talk != nil {
			e.player.endTalk = true
		}
		z.queueDeath(e, killer)
		return
	}
	z.removeMap(e)
	z.entities.remove(e)
	z.dropStatuses(e)
	z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_DIED})
	z.queueDeath(e, killer)
	z.reward(e, killer)
	s := e.monster.spawn
	if !s.noRespawn {
		z.respawns = append(z.respawns, respawn{spawn: s, at: z.now + int64(max(s.RespawnMs, 1000))})
	}
}

// forget removes an entity from every target.
func (z *Zone) forget(e *entity) {
	for o := range z.entities.all() {
		if o.target == e {
			o.target = nil
		}
	}
}

// revive brings back monsters whose respawn time has come, and dead
// players after reviveMs.
func (z *Zone) revive() {
	kept := z.respawns[:0]
	for _, r := range z.respawns {
		if z.now >= r.at {
			z.spawnOne(r.spawn)
		} else {
			kept = append(kept, r)
		}
	}
	z.respawns = kept
	var back []*Player
	for e := range z.entities.all() {
		if e.player == nil || !e.dead || z.now < e.diedAt+reviveMs {
			continue
		}
		back = append(back, e.player)
	}
	for _, p := range back {
		z.respawn(p)
	}
}

// respawn is pc_respawn after the respawn wait: the player stands with
// full HP and goes to its save point, as pc_setpos. A save point on
// another map hands it to that map's zone.
func (z *Zone) respawn(p *Player) {
	e := p.entity
	e.dead = false
	e.hp, e.hpChanged = int64(e.status.MaxHP), true
	to, x, y := z.savePoint(p)
	if to != nil && to != z && p.sess != nil && z.depart(p, to, x, y) {
		return
	}
	if to != z {
		x, y = z.startCell()
	}
	z.closeTalk(p)
	z.removeMap(e)
	e.x, e.y = z.arrivalCell(x, y)
	e.path = e.path[:0]
	// Everyone who saw the player sees it leave and appear again at the
	// save point. The player keeps its own ID.
	for o := range z.entities.all() {
		if o.player == nil {
			continue
		}
		if id, ok := o.player.netIDs.get(e); ok && o != e {
			o.player.netIDs.del(e)
			z.queue(o.player, frame(z.tick, o.player.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_TELEPORTED)))
		}
	}
	p.fresh = true
}

// combatEvents are the attacks, deaths, and own HP a player sees this
// tick.
func (z *Zone) combatEvents(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	for _, h := range z.hits {
		src, ok1 := p.netIDs.get(h.source)
		if !ok1 && h.caster != nil {
			src, ok1 = p.netIDs.get(h.caster)
		}
		tgt, ok2 := p.netIDs.get(h.target)
		if !ok1 || !ok2 {
			continue
		}
		var skillID uint32
		if h.skill != nil {
			skillID = shownSkill(h.skill)
		}
		events = append(events, wathenav1.Event_builder{Damage: wathenav1.Damage_builder{
			SkillId:          skillID,
			Source:           src,
			Target:           tgt,
			Amount:           h.result.Damage,
			Hits:             uint32(max(h.result.Hits, 1)), // #nosec G115 -- a small count.
			Kind:             damageKind(h.result.Kind),
			DelayMs:          uint32(max(h.delay, 0)), // #nosec G115 -- a motion time.
			TargetHpPermille: h.hpPermille,
		}.Build()}.Build())
	}
	// A pick-up shows before the item vanishes.
	events = append(events, z.takeEvents(p)...)
	for _, r := range z.removed {
		if id, ok := p.netIDs.get(r.e); ok {
			p.netIDs.del(r.e)
			events = append(events, vanish(id, r.reason))
		}
	}
	if me := p.entity; me.hpChanged && me.status != nil {
		events = append(events, wathenav1.Event_builder{Stats: wathenav1.StatUpdate_builder{Stats: []*wathenav1.Stat{
			wathenav1.Stat_builder{Id: wathenav1.StatId_STAT_ID_HP, Value: max(me.hp, 0)}.Build(),
			wathenav1.Stat_builder{Id: wathenav1.StatId_STAT_ID_MAX_HP, Value: int64(me.status.MaxHP)}.Build(),
		}}.Build()}.Build())
	}
	return events
}

func damageKind(k battle.Kind) wathenav1.DamageKind {
	switch k {
	case battle.Critical:
		return wathenav1.DamageKind_DAMAGE_KIND_CRITICAL
	case battle.Miss:
		return wathenav1.DamageKind_DAMAGE_KIND_MISS
	case battle.LuckyDodge:
		return wathenav1.DamageKind_DAMAGE_KIND_LUCKY_DODGE
	}
	return wathenav1.DamageKind_DAMAGE_KIND_NORMAL
}

func stop(id uint32, e *entity) *wathenav1.Event {
	return wathenav1.Event_builder{Stop: wathenav1.EntityStop_builder{
		Id: id,
		X:  uint32(e.x), // #nosec G115 -- map cells are at most 1024.
		Y:  uint32(e.y), // #nosec G115 -- as above.
	}.Build()}.Build()
}

func frame(tick, ack uint32, events ...*wathenav1.Event) *wathenav1.ServerMessage {
	return wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{Tick: tick, AckSeq: ack, Events: events}.Build()}.Build()
}

// facingTo is the protocol facing (0 south, clockwise) toward an offset.
func facingTo(dx, dy int) int {
	sx, sy := sign(dx), sign(dy)
	for f, d := range [8][2]int{{0, -1}, {-1, -1}, {-1, 0}, {-1, 1}, {0, 1}, {1, 1}, {1, 0}, {1, -1}} {
		if d == [2]int{sx, sy} {
			return f
		}
	}
	return 0
}

func sign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}

// canTarget is the part of status_check_skilluse that normal attacks
// read: no one attacks a unit playing dead, and only bosses and
// detectors see hidden units.
func (z *Zone) canTarget(e, t *entity) bool {
	// The attacker's own options: a hidden player and a unit in Chase
	// Walk do not attack.
	if l := e.sc; !l.Empty() {
		if l.Option&sc.OptionHide != 0 && e.player != nil || l.Option&optionChasewalk != 0 {
			return false
		}
	}
	l := t.sc
	if l.Empty() {
		return true
	}
	if l.Has(sc.SCTrickdead) {
		return false
	}
	if l.Option&(sc.OptionHide|sc.OptionCloak|optionChasewalk) != 0 {
		return e.status != nil && e.status.Mode&(modeStatusImmune|modeDetector) != 0
	}
	return true
}
