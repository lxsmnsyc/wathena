package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
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
	// oreItemGroup is IG_ORE, the ores of Finding Ore.
	oreItemGroup = "ORE"
)

// groundItem is an item lying on a cell, as rAthena's flooritem_data.
type groundItem struct {
	id     uint32
	amount int
	// refine and cards are a dropped equipment item's, and extra its
	// rolls and random options.
	refine     int
	cards      []uint32
	extra      item.Extra
	subX, subY int
	// from is the serial of the stack the item was split from, or zero.
	// The copy picked up gets a new serial and logs it.
	from   item.Serial
	rights progression.Rights
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
	items                []fallItem
	first, second, third uint32
	boss                 progression.BossType
	at                   int64
}

// fallItem is an item about to fall: a drop with its rolls, or what a
// looter picked up.
type fallItem struct {
	id     uint32
	amount int
	refine int
	cards  []uint32
	extra  item.Extra
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
	if p == nil || !z.entities.has(p.entity) {
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
	hooks := z.killHooks(e, killer)
	// mob_dead's common exp bonus from statuses.
	if b := z.richmanBonus(e, killer); b != 0 {
		if hooks == nil {
			hooks = &progression.Hooks{}
		}
		hooks.ExpBonus = b
	}
	kill := t.KillWith(mob, &e.monster.dmglog, killerID, z.attacker, hooks)
	for _, r := range kill.Rewards {
		if p := z.players[r.CharID]; p != nil {
			z.gainExp(p, r.BaseExp, r.JobExp)
		}
	}
	f := fall{x: e.x, y: e.y, first: kill.First, second: kill.Second, third: kill.Third, boss: mob.Boss(), at: z.now + t.DropDelay}
	for _, d := range t.RollDropsWith(mob, kill.DropModifier, k, kill.First != 0, z.rand, hooks) {
		if p := z.players[kill.First]; d.Egg && p != nil {
			// pet_create_egg puts the egg in the inventory when a slot
			// is free. The egg is lost otherwise.
			if p.inventory.Blank() > 0 {
				z.addItem(p, d.Item, 1)
			}
			continue
		}
		f.items = append(f.items, z.dropCopy(d.Item, d.RandomOptionGroup, e))
	}
	// Ore Discovery: a first looter with Finding Ore (BS_FINDINGORE)
	// may find an ore of the ORE group, whoever killed the monster.
	if p := z.players[kill.First]; p != nil && p.skillLevel(skill.IDFindingOre) > 0 && z.Mods != nil {
		if id, ok := z.Mods.Groups.RandomDrop(oreItemGroup, 1, z.rand); ok {
			f.items = append(f.items, z.dropCopy(id, "", e))
		}
	}
	// mob_dead: a looter drops what it picked up, with the same rights.
	if m := e.monster; len(m.loot) > 0 {
		f.items = append(f.items, m.loot...)
		m.loot = nil
	}
	if len(f.items) > 0 {
		z.falling = append(z.falling, f)
	}
	z.mvpReward(e, mob, kill)
}

// dropCopy is a monster's drop as it falls: one copy with its rolls,
// rolled from the zone's random stream, and the random options of its
// group, as mob_setdropitem and mob_setdropitem_option.
func (z *Zone) dropCopy(id uint32, group string, monster *entity) fallItem {
	return fallItem{id: id, amount: 1, extra: z.newExtra(z.Items.Get(id), true, rollSource{kind: sourceDrop, monster: monster}, group)}
}

// logSplit logs a copy picked up from part of a stack: its new serial,
// the stack it came from, and its rolls (strategy section 5.4,
// "Stacking").
func (z *Zone) logSplit(p *Player, g *groundItem) {
	if z.Mods == nil || z.Mods.Log == nil {
		return
	}
	for i := range p.inventory.Slots {
		s := &p.inventory.Slots[i]
		if s.ID == g.id && s.Extra.Equal(&g.extra) {
			z.Mods.Log.Info("item split", "character", p.Name, "item", g.id, "amount", g.amount,
				"serial", s.Serial.String(), "from", g.from.String(), "rolls", g.extra.Rolls.Key())
			return
		}
	}
}

// effectMVP is EF_MVP, the effect over the MVP player.
const effectMVP = 68

// mvpReward is the MVP part of mob_dead: the player with the most damage
// dealt and taken (get_mvp_player) gets the MVP exp and, of the MVP
// drops in order, the first that rolls, in its inventory or at its feet.
func (z *Zone) mvpReward(e *entity, m *progression.Mob, kill progression.Kill) {
	t := z.Progress
	if m.Boss() != progression.BossMVP {
		return
	}
	var mvp *Player
	var best int64
	count := 0
	for _, en := range e.monster.dmglog.Entries {
		p := z.players[en.CharID]
		if p == nil || !z.entities.has(p.entity) {
			continue
		}
		count++
		if p.entity.dead {
			continue
		}
		if sum := en.Damage + en.Tanked; sum > best {
			best, mvp = sum, p
		}
	}
	if mvp == nil {
		return
	}
	z.effects = append(z.effects, effect{mvp.entity, effectMVP})
	if exp := m.MvpExp; exp > 0 {
		pen := t.Penalty(progression.PenaltyMVPExp, mvp.progress.BaseLevel, m)
		exp = min(max(exp*int64(pen)/100, 0), progression.MaxExp)
		if t.ExpBonusAttacker > 0 && count > 1 {
			count = min(count, t.ExpBonusMaxAttacker)
			exp += exp * int64(t.ExpBonusAttacker*(count-1)) / 100
		}
		exp = min(max(exp, 1), progression.MaxExp)
		z.gainExp(mvp, exp, 0)
	}
	pen := t.Penalty(progression.PenaltyMVPDrop, mvp.progress.BaseLevel, m)
	for _, d := range m.MvpDrops {
		rate := min(max(d.Rate*pen/100, 0), 10000)
		if rate != 10000 {
			if rate <= 0 && !t.DropRate0Item {
				rate = 1
			}
			if z.rand.IntN(10000) >= rate {
				continue
			}
		}
		fi := z.dropCopy(d.Item, d.RandomOptionGroup, e)
		if r := z.addCopy(mvp, d.Item, 1, 0, nil, fi.extra); r != item.AddSuccess {
			pe := mvp.entity
			z.falling = append(z.falling, fall{x: pe.x, y: pe.y, items: []fallItem{fi},
				first: mvp.charID, second: kill.Second, third: kill.Third, boss: progression.BossMVP, at: z.now})
		}
		// item_drop_mvp_mode 0: one prize.
		break
	}
}

// gainExp gives a player exp, levels it up, and sends what changed. A
// level up computes the player's stats again, as pc_checkbaselevelup and
// pc_checkjoblevelup call status_calc_pc. A base level up then heals HP
// and SP fully, as status_percent_heal.
func (z *Zone) gainExp(p *Player, base, job int64) {
	before := z.progressStats(p, nil)
	g := z.Progress.GainExp(&p.progress, base, job)
	if changed := z.progressStats(p, before); changed != nil {
		p.pending = append(p.pending, changed)
	}
	e := p.entity
	if g.BaseLevels > 0 || g.JobLevels > 0 {
		z.recalc(p)
	}
	if g.BaseLevels > 0 {
		e.hp, e.hpChanged = int64(e.status.MaxHP), true
		p.sp = int64(e.status.MaxSP)
		z.effects = append(z.effects, effect{e, effectBaseLevelUp})
	}
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
	if g.JobLevels > 0 {
		z.effects = append(z.effects, effect{e, effectJobLevelUp})
		// New skill points change what the player can learn.
		z.sendSkillList(p)
	}
	if g.BaseLevels > 0 {
		z.scriptEventFor("base_level_up", modscript.Player(int64(p.charID)))
	}
	if g.JobLevels > 0 {
		z.scriptEventFor("job_level_up", modscript.Player(int64(p.charID)))
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
		stat(wathenav1.StatId_STAT_ID_JOB, int64(p.char.Job)),
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
		for _, it := range f.items {
			pl := progression.PlaceItem(z.cellFree, f.x, f.y, dir, z.rand)
			z.entities.add(&entity{x: pl.X, y: pl.Y, item: &groundItem{
				id: it.id, amount: max(it.amount, 1), subX: pl.SubX, subY: pl.SubY,
				refine: it.refine, cards: it.cards, extra: it.extra,
				rights:  z.Progress.NewRights(f.first, f.second, f.third, z.now, f.boss),
				expires: z.now + z.Progress.FloorItemLifetime,
				fresh:   true,
			}})
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
	for e := range z.entities.all() {
		if (e.player != nil || e.monster != nil) && e.x == x && e.y == y {
			return false
		}
	}
	return true
}

// expireItems removes items that lay on the ground too long, as
// map_clearflooritem_timer.
func (z *Zone) expireItems() {
	for e := range z.entities.all() {
		if e.item != nil && z.now >= e.item.expires {
			z.entities.remove(e)
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
	for e, id := range p.netIDs.all() {
		if id == net && e.item != nil && z.entities.has(e) {
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
	for e := range z.entities.all() {
		it := e.pickItem
		if it == nil {
			continue
		}
		p := e.player
		if !z.entities.has(it) {
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
	// party_share_loot gives the item with pc_additem. When it fails,
	// the item stays and the player learns why.
	if r := z.addCopy(p, g.id, g.amount, g.refine, g.cards, g.extra); r != item.AddSuccess {
		p.pending = append(p.pending, pickUpFailed(net, pickUpFailure(r)))
		return
	}
	if !g.from.IsZero() {
		z.logSplit(p, g)
	}
	// unit_stop_attack.
	p.entity.target = nil
	z.entities.remove(it)
	z.takes = append(z.takes, take{p.entity, it})
	z.removed = append(z.removed, removal{it, wathenav1.VanishReason_VANISH_REASON_PICKED_UP})
}

func pickUpFailed(item uint32, reason wathenav1.PickUpFailure) *wathenav1.Event {
	return wathenav1.Event_builder{PickUpFailed: wathenav1.PickUpFailed_builder{Item: item, Reason: reason}.Build()}.Build()
}

// takeEvents are the pick-ups a player sees this tick.
func (z *Zone) takeEvents(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	for _, t := range z.takes {
		src, ok1 := p.netIDs.get(t.source)
		item, ok2 := p.netIDs.get(t.item)
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
		if id, ok := p.netIDs.get(ef.e); ok {
			events = append(events, wathenav1.Event_builder{Effect: wathenav1.EffectPlay_builder{EffectId: ef.id, Entity: &id}.Build()}.Build())
		}
	}
	return events
}
