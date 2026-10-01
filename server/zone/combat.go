package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
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
	result         battle.Result
	delay          int
	hpPermille     uint32
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
		reach := max(e.status.RHW.Range, 1)
		if chebyshev(e, t) > reach {
			z.chase(e, t)
			continue
		}
		// In range: stop walking, then attack at the attack delay.
		if len(e.path) > 0 {
			e.path = e.path[:0]
			e.stopped = true
		}
		if z.now < e.nextAttack {
			continue
		}
		e.dir = facingTo(t.x-e.x, t.y-e.y)
		z.strike(e, t)
		e.nextAttack = z.now + int64(max(e.status.ADelay, 100))
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
	r := battle.Attack(e.status, t.status, z.rand)
	z.logDamage(e, t, r.Damage)
	if r.Damage > 0 {
		t.hp -= r.Damage
		t.hpChanged = true
	}
	z.hits = append(z.hits, hit{source: e, target: t, result: r, delay: e.status.AMotion, hpPermille: t.hpPermille()})
	if t.hp <= 0 {
		z.die(t, e)
		return
	}
	// A monster that can attack strikes back.
	if t.monster != nil && t.target == nil && t.status.Mode&modeCanAttack != 0 {
		t.target, t.repeat = e, true
		t.path = t.path[:0]
		t.nextAttack = z.now + int64(t.status.DMotion)
	}
}

// die ends a unit killed by another.
func (z *Zone) die(e, killer *entity) {
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
		// Players lie where they fell, then return to the start.
		e.diedAt = z.now
		// status_damage resets the regen waits on death.
		e.player.regenTicks = regenState{}
		return
	}
	z.entities.remove(e)
	z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_DIED})
	if killer != nil && killer.player != nil {
		z.monsterKilled(killer.player, e)
	}
	z.reward(e, killer)
	s := e.monster.spawn
	z.respawns = append(z.respawns, respawn{spawn: s, at: z.now + int64(max(s.RespawnMs, 1000))})
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
	for e := range z.entities.all() {
		if e.player == nil || !e.dead || z.now < e.diedAt+reviveMs {
			continue
		}
		e.dead = false
		e.hp, e.hpChanged = int64(e.status.MaxHP), true
		e.x, e.y = z.startCell()
		// Everyone who saw the player sees it leave and appear again at
		// the start. The player keeps its own ID.
		for o := range z.entities.all() {
			if o.player == nil {
				continue
			}
			if id, ok := o.player.netIDs.get(e); ok && o != e {
				o.player.netIDs.del(e)
				z.queue(o.player, frame(z.tick, o.player.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_TELEPORTED)))
			}
		}
		e.player.fresh = true
	}
}

// combatEvents are the attacks, deaths, and own HP a player sees this
// tick.
func (z *Zone) combatEvents(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	for _, h := range z.hits {
		src, ok1 := p.netIDs.get(h.source)
		tgt, ok2 := p.netIDs.get(h.target)
		if !ok1 || !ok2 {
			continue
		}
		events = append(events, wathenav1.Event_builder{Damage: wathenav1.Damage_builder{
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
