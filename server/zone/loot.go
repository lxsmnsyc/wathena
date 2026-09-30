package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/progression"
)

// Loot: a monster's death gives exp to the players who fought it and
// drops items around it, as rAthena's mob_dead. Items lie on the ground
// until someone with the right picks them up or they expire.

const (
	// pickUpRange is the farthest a player may be from an item to take
	// it, as pc_takeitem's distance check.
	pickUpRange = 2
	// edgeCells is how close to the map edge an item may not fall, as
	// map_cell_free with the default map_edge_size.
	edgeCells = 5
	// Effects from rAthena's clif_misceffect.
	effectBaseLevelUp = 371
	effectJobLevelUp  = 158
)

// groundItem is an item lying on a cell, as rAthena's flooritem_data.
type groundItem struct {
	id         uint32
	amount     int
	subX, subY int
	rights     progression.Rights
	// expires is when the item vanishes, in zone milliseconds.
	expires int64
	// fresh is set in the tick the item fell, so viewers see it drop.
	fresh bool
}

// pickUp is a player's walk to an item it wants.
type pickUp struct {
	pickItem *entity
	// pickNet is the connection's ID for the item, for PickUpFailed.
	pickNet uint32
	// pickReach is how close the player must get before taking it.
	pickReach int
}

// loot is a zone's loot state.
type loot struct {
	// falling are kills whose items have not fallen yet.
	falling []fall
	// takes are this tick's pick-ups, sent to players who see both sides.
	takes []take
	// effects are this tick's effects on entities.
	effects []effect
}

type fall struct {
	x, y                 int
	items                []uint32
	first, second, third uint32
	boss                 progression.BossType
	at                   int64
}

type take struct {
	source, item *entity
}

type effect struct {
	e  *entity
	id uint32
}

// logDamage is mob_log_damage for an attack between a player and a
// monster. A player's hit, even a miss, logs its damage. A monster's hit
// on a player logs the damage the player tanked.
func (z *Zone) logDamage(src, target *entity, damage int64) {
	switch {
	case target.monster != nil && src.player != nil:
		logged := damage
		if z.Progress != nil {
			logged = z.Progress.LoggedDamage(damage, target.hp)
		}
		target.monster.dmglog.Add(src.player.charID, logged, 0)
	case src.monster != nil && target.player != nil:
		src.monster.dmglog.Add(target.player.charID, 0, damage)
	}
}

// attacker reports on a player in a damage log.
func (z *Zone) attacker(charID uint32) progression.Attacker {
	p := z.players[charID]
	if p == nil || !z.entities[p.entity] {
		return progression.Attacker{}
	}
	return progression.Attacker{Online: true, Dead: p.entity.dead, BaseLevel: p.progress.BaseLevel}
}

// reward gives exp and drops for a dead monster, as mob_dead.
func (z *Zone) reward(e, killer *entity) {
	t, mob := z.Progress, e.monster.spawn.Reward
	if t == nil || mob == nil {
		return
	}
	var killerID uint32
	var k *progression.Killer
	if killer != nil && killer.player != nil {
		killerID = killer.player.charID
		k = &progression.Killer{Player: true, Luk: killer.status.Luk}
	}
	kill := t.Kill(mob, &e.monster.dmglog, killerID, z.attacker)
	for _, r := range kill.Rewards {
		if p := z.players[r.CharID]; p != nil {
			z.gainExp(p, r.BaseExp, r.JobExp)
		}
	}
	f := fall{x: e.x, y: e.y, first: kill.First, second: kill.Second, third: kill.Third, boss: mob.Boss(), at: z.now + t.DropDelay}
	for _, d := range t.RollDrops(mob, kill.DropModifier, k, kill.First != 0, z.rand) {
		if p := z.players[kill.First]; d.Egg && p != nil {
			// pet_create_egg puts the egg in the inventory.
			z.addItem(p, d.Item, 1)
			continue
		}
		f.items = append(f.items, d.Item)
	}
	if len(f.items) > 0 {
		z.falling = append(z.falling, f)
	}
}

// gainExp gives a player exp, levels it up, and sends what changed. A
// base level up heals fully, as pc_checkbaselevelup.
func (z *Zone) gainExp(p *Player, base, job int64) {
	before := z.progressStats(p, nil)
	g := z.Progress.GainExp(&p.progress, base, job)
	if changed := z.progressStats(p, before); changed != nil {
		p.pending = append(p.pending, changed)
	}
	e := p.entity
	if g.BaseLevels > 0 {
		e.hp, e.hpChanged = int64(e.status.MaxHP), true
		z.effects = append(z.effects, effect{e, effectBaseLevelUp})
	}
	if g.JobLevels > 0 {
		z.effects = append(z.effects, effect{e, effectJobLevelUp})
	}
}

// progressStats is a StatUpdate of the player's levels, exp and points.
// With a previous update, it holds only the values that changed, and is
// nil when none did.
func (z *Zone) progressStats(p *Player, before *wathenav1.Event) *wathenav1.Event {
	c := &p.progress
	t := z.Progress
	next := func(max bool, exp int64) int64 {
		if max {
			return 0
		}
		return exp
	}
	all := []*wathenav1.Stat{
		stat(wathenav1.StatId_STAT_ID_BASE_LEVEL, int64(c.BaseLevel)),
		stat(wathenav1.StatId_STAT_ID_JOB_LEVEL, int64(c.JobLevel)),
		stat(wathenav1.StatId_STAT_ID_BASE_EXP, c.BaseExp),
		stat(wathenav1.StatId_STAT_ID_JOB_EXP, c.JobExp),
		stat(wathenav1.StatId_STAT_ID_NEXT_BASE_EXP, next(t.IsMaxBaseLevel(c), t.NextBaseExp(c))),
		stat(wathenav1.StatId_STAT_ID_NEXT_JOB_EXP, next(t.IsMaxJobLevel(c), t.NextJobExp(c))),
		stat(wathenav1.StatId_STAT_ID_STATUS_POINTS, int64(c.StatusPoints)),
		stat(wathenav1.StatId_STAT_ID_SKILL_POINTS, int64(c.SkillPoints)),
	}
	if before == nil {
		return wathenav1.Event_builder{Stats: wathenav1.StatUpdate_builder{Stats: all}.Build()}.Build()
	}
	old := before.GetStats().GetStats()
	var changed []*wathenav1.Stat
	for i, s := range all {
		if s.GetValue() != old[i].GetValue() {
			changed = append(changed, s)
		}
	}
	if changed == nil {
		return nil
	}
	return wathenav1.Event_builder{Stats: wathenav1.StatUpdate_builder{Stats: changed}.Build()}.Build()
}

func stat(id wathenav1.StatId, v int64) *wathenav1.Stat {
	return wathenav1.Stat_builder{Id: id, Value: v}.Build()
}

// addItem puts items in a player's inventory. Weight is not checked yet.
func (z *Zone) addItem(p *Player, id uint32, amount int) {
	p.inventory[id] += amount
	p.pending = append(p.pending, wathenav1.Event_builder{InventoryAdd: wathenav1.InventoryAdd_builder{
		ItemId: id,
		Amount: uint32(amount), // #nosec G115 -- a stack of at most 30000.
	}.Build()}.Build())
}

// dropItems places the items of kills whose drop time has come, as
// mob_delay_item_drop and mob_process_drop_list: the first on the
// monster's cell, the rest around it.
func (z *Zone) dropItems() {
	kept := z.falling[:0]
	for _, f := range z.falling {
		if z.now < f.at {
			kept = append(kept, f)
			continue
		}
		dir := progression.DirCenter
		for _, id := range f.items {
			pl := progression.PlaceItem(z.cellFree, f.x, f.y, dir, z.rand)
			z.entities[&entity{x: pl.X, y: pl.Y, item: &groundItem{
				id: id, amount: 1, subX: pl.SubX, subY: pl.SubY,
				rights:  z.Progress.NewRights(f.first, f.second, f.third, z.now, f.boss),
				expires: z.now + z.Progress.FloorItemLifetime,
				fresh:   true,
			}}] = true
			dir = progression.NextDropDir(dir)
		}
	}
	z.falling = kept
}

// cellFree is map_cell_free for drops: the cell is walkable, away from
// the map edge, and has no player or monster on it.
func (z *Zone) cellFree(x, y int) bool {
	if x < edgeCells || x > z.grid.Width-edgeCells || y < edgeCells || y > z.grid.Height-edgeCells || z.grid.NoPass(x, y) {
		return false
	}
	for e := range z.entities {
		if (e.player != nil || e.monster != nil) && e.x == x && e.y == y {
			return false
		}
	}
	return true
}

// expireItems removes items that lay on the ground too long, as
// map_clearflooritem_timer.
func (z *Zone) expireItems() {
	for e := range z.entities {
		if e.item != nil && z.now >= e.item.expires {
			delete(z.entities, e)
			z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_EXPIRED})
		}
	}
}

// startPickUp handles a PickUp command. A player within pickUpRange takes
// the item at once. One farther away walks next to it first, as the
// client does.
func (z *Zone) startPickUp(p *Player, net uint32) {
	me := p.entity
	var it *entity
	for e, id := range p.netIDs {
		if id == net && e.item != nil && z.entities[e] {
			it = e
		}
	}
	if it == nil {
		p.pending = append(p.pending, pickUpFailed(net, wathenav1.PickUpFailure_PICK_UP_FAILURE_GONE))
		return
	}
	me.target = nil
	me.pickItem, me.pickNet, me.pickReach = it, net, 1
	if chebyshev(me, it) <= pickUpRange {
		me.pickReach = pickUpRange
		if len(me.path) > 0 {
			me.path = me.path[:0]
			me.stopped = true
		}
		return
	}
	z.walk(me, it.x, it.y)
}

// pickUps moves players toward the items they want and takes the items
// in reach.
func (z *Zone) pickUps() {
	for e := range z.entities {
		it := e.pickItem
		if it == nil {
			continue
		}
		p := e.player
		if !z.entities[it] {
			e.pickItem = nil
			p.pending = append(p.pending, pickUpFailed(e.pickNet, wathenav1.PickUpFailure_PICK_UP_FAILURE_GONE))
			continue
		}
		if chebyshev(e, it) > e.pickReach {
			if len(e.path) == 0 {
				// Walk again, or give up when there is no path.
				if z.walk(e, it.x, it.y); len(e.path) == 0 {
					e.pickItem = nil
				}
			}
			continue
		}
		if len(e.path) > 0 {
			e.path = e.path[:0]
			e.stopped = true
		}
		e.pickItem = nil
		z.take(p, it, e.pickNet)
	}
}

// take is pc_takeitem for a player without a party, once in reach.
func (z *Zone) take(p *Player, it *entity, net uint32) {
	g := it.item
	if !g.rights.CanTake(p.charID, z.now) {
		p.pending = append(p.pending, pickUpFailed(net, wathenav1.PickUpFailure_PICK_UP_FAILURE_NOT_YOURS))
		return
	}
	// unit_stop_attack.
	p.entity.target = nil
	delete(z.entities, it)
	z.takes = append(z.takes, take{p.entity, it})
	z.removed = append(z.removed, removal{it, wathenav1.VanishReason_VANISH_REASON_PICKED_UP})
	z.addItem(p, g.id, g.amount)
}

func pickUpFailed(item uint32, reason wathenav1.PickUpFailure) *wathenav1.Event {
	return wathenav1.Event_builder{PickUpFailed: wathenav1.PickUpFailed_builder{Item: item, Reason: reason}.Build()}.Build()
}

// takeEvents are the pick-ups a player sees this tick.
func (z *Zone) takeEvents(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	for _, t := range z.takes {
		src, ok1 := p.netIDs[t.source]
		item, ok2 := p.netIDs[t.item]
		if ok1 && ok2 {
			events = append(events, wathenav1.Event_builder{ItemTaken: wathenav1.ItemTaken_builder{Source: src, Item: item}.Build()}.Build())
		}
	}
	return events
}

// effectEvents are the effects a player sees this tick.
func (z *Zone) effectEvents(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	for _, ef := range z.effects {
		if id, ok := p.netIDs[ef.e]; ok {
			events = append(events, wathenav1.Event_builder{Effect: wathenav1.EffectPlay_builder{EffectId: ef.id, Entity: &id}.Build()}.Build())
		}
	}
	return events
}
