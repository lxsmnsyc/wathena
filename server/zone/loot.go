package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/transfer"
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
	// origin is the serial of the stack the item was dropped from, or
	// zero. The copy picked up gets a new serial and logs it. from is
	// where the item came from when no stack did, such as the monster
	// that dropped it.
	origin item.Serial
	from   transfer.Owner
	rights progression.Rights
	// looters are the names of the first, second and third looters, for
	// shared pick-up, which checks their parties when the item is taken.
	looters [3]string
	// expires is when the item vanishes, in zone milliseconds.
	expires int64
	// fresh is set in the tick the item fell, so viewers see it drop.
	fresh bool
	// dropper is the player that dropped the item from its inventory,
	// or nil, for maxPlayerDrops.
	dropper *Player
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
	// origin and from are as in groundItem.
	origin item.Serial
	from   transfer.Owner
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
		z.eventDamage(src, target, logged)
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
	a := progression.Attacker{Online: true, Dead: p.entity.dead, BaseLevel: p.progress.BaseLevel}
	if p.soc != nil && z.Social != nil {
		if sh, ok := z.Social.ShareOf(p.soc); ok {
			a.Party, a.EvenShare = sh.Party, sh.Options.ExpEven
		}
	}
	return a
}

// shareExp is party_exp_share for each party pot of a kill: the members
// on this map who are alive, in slot order, split the pot.
func (z *Zone) shareExp(mob *progression.Mob, kill progression.Kill, hooks *progression.Hooks) {
	if len(kill.Pots) == 0 || z.Social == nil {
		return
	}
	bonus := rules.PartyOrDefault(z.worldRules()).EvenShareBonus
	for _, pot := range kill.Pots {
		var members []progression.ShareMember
		for _, p := range z.partyHere(pot.Party) {
			if !p.entity.dead {
				members = append(members, progression.ShareMember{CharID: p.charID, BaseLevel: p.progress.BaseLevel})
			}
		}
		for _, r := range z.Progress.ShareExp(mob, pot, members, bonus, hooks) {
			if p := z.players[r.CharID]; p != nil {
				z.gainExp(p, r.BaseExp, r.JobExp)
			}
		}
	}
}

// partyHere are a party's members in this zone, in slot order, as
// p->data[i] with sd->m == the map.
func (z *Zone) partyHere(party int64) []*Player {
	sh, ok := z.Social.ShareByID(party)
	if !ok {
		return nil
	}
	return z.slotPlayers(sh)
}

// slotPlayers are a party's members in this zone, in slot order.
func (z *Zone) slotPlayers(sh social.Share) []*Player {
	var out []*Player
	for _, p := range z.slotsHere(sh) {
		if p != nil {
			out = append(out, p)
		}
	}
	return out
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
	z.shareExp(mob, kill, hooks)
	f := fall{x: e.x, y: e.y, first: kill.First, second: kill.Second, third: kill.Third, boss: mob.Boss(), at: z.now + t.DropDelay}
	for _, d := range t.RollDropsWith(mob, kill.DropModifier, k, kill.First != 0, z.rand, hooks) {
		if p := z.players[kill.First]; d.Egg && p != nil {
			// pet_create_egg puts the egg in the inventory when a slot
			// is free. The egg is lost otherwise.
			if p.inventory.Blank() > 0 {
				z.addItem(p, d.Item, 1, origin{action: transfer.ActionLoot, from: monsterOwner(e)})
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
	return fallItem{id: id, amount: 1, extra: z.newExtra(z.Items.Get(id), true, rollSource{kind: sourceDrop, monster: monster}, group),
		from: monsterOwner(monster)}
}

// effectMVP is EF_MVP, the effect over the MVP player.
const effectMVP = 68

// mvpItemText starts the line the game client shows for
// ZC_MVP_GETTING_ITEM, which ends with the item's name and "!!".
const mvpItemText = "Congratulations! You are the MVP! Your reward item is "

// mvpReward is the MVP part of mob_dead: the player with the most damage
// dealt and taken (get_mvp_player) gets the MVP exp and, of the MVP
// drops in order, the first that rolls, in its inventory or at its feet.
// What rAthena announces of it:
//   - clif_mvp_effect: the MVP effect over the player, which the players
//     near it see.
//   - clif_mvp_item: the prize, to the MVP player only.
//   - clif_mvp_exp sends nothing: PACKETVER 20211103 has no packet for
//     it, and mvp_exp_reward_message is off.
//   - rare_drop_announce is 0, so no prize is announced to the world.
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
		// clif_mvp_item: the MVP player is told its prize, before the
		// item goes into its inventory.
		name, _ := scriptHost{z}.ItemName(d.Item)
		mvp.pending = append(mvp.pending, systemMessage(mvpItemText+name+"!!"))
		if r := z.addCopy(mvp, d.Item, 1, 0, nil, fi.extra, origin{action: transfer.ActionLoot, from: fi.from}); r != item.AddSuccess {
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
func (z *Zone) gainExp(p *Player, base, job int64) { z.gainExpFlags(p, base, job, true) }

// gainExpFlags is pc_gainexp. tax is unset for exp_flag 2, which getexp2
// gives: no guild tax.
func (z *Zone) gainExpFlags(p *Player, base, job int64, tax bool) {
	// pc_gainexp: the guild takes its tax of base exp first.
	if tax {
		base = z.payGuildTax(p, base)
	}
	before := z.progressStats(p, nil)
	g := z.Progress.GainExp(&p.progress, base, job)
	if changed := z.progressStats(p, before); changed != nil {
		p.pending = append(p.pending, changed)
	}
	e := p.entity
	if g.BaseLevels > 0 || g.JobLevels > 0 {
		z.recalc(p)
		// pc_baselevelchanged and pc_checkjoblevelup show the quest icons
		// again (pc.cpp 8314 and 8356).
		p.questInfoChanged()
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
		looters := z.looterNames(f.first, f.second, f.third)
		// PlaceItem looks 1 cell past a direction's cell, so 2 cells.
		free := z.freeNear(f.x, f.y, 2)
		for _, it := range f.items {
			pl := progression.PlaceItem(free, f.x, f.y, dir, z.rand)
			z.entities.add(&entity{x: pl.X, y: pl.Y, item: &groundItem{
				id: it.id, amount: max(it.amount, 1), subX: pl.SubX, subY: pl.SubY,
				refine: it.refine, cards: it.cards, extra: it.extra,
				origin: it.origin, from: it.from,
				rights:  z.Progress.NewRights(f.first, f.second, f.third, z.now, f.boss),
				looters: looters,
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
	if !z.cellOpen(x, y) {
		return false
	}
	for e := range z.entities.all() {
		if (e.player != nil || e.monster != nil) && e.x == x && e.y == y {
			return false
		}
	}
	return true
}

// cellOpen is the part of cellFree that only reads the map.
func (z *Zone) cellOpen(x, y int) bool {
	return x >= edgeCells && x <= z.grid.Width-edgeCells && y >= edgeCells && y <= z.grid.Height-edgeCells && !z.grid.NoPass(x, y)
}

// freeNear is cellFree for the cells within r of (x, y), which walks the
// entities once instead of once per cell tried. Placing items moves no
// player or monster, so the answer holds while the items of one drop
// are placed. Cells farther away fall back to cellFree.
func (z *Zone) freeNear(x, y, r int) func(x, y int) bool {
	side := 2*r + 1
	taken := make([]bool, side*side)
	for e := range z.entities.all() {
		if (e.player != nil || e.monster != nil) && abs(e.x-x) <= r && abs(e.y-y) <= r {
			taken[(e.y-y+r)*side+e.x-x+r] = true
		}
	}
	return func(cx, cy int) bool {
		if abs(cx-x) > r || abs(cy-y) > r {
			return z.cellFree(cx, cy)
		}
		return z.cellOpen(cx, cy) && !taken[(cy-y+r)*side+cx-x+r]
	}
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

// looterNames are the names of the players of three charIDs, or "".
func (z *Zone) looterNames(ids ...uint32) [3]string {
	var out [3]string
	for i, id := range ids {
		if p := z.players[id]; id != 0 && p != nil {
			out[i] = p.Name
		}
	}
	return out
}

// take is pc_takeitem, once in reach, with party_share_loot:
//   - With shared pick-up, a player may take what its party's looters
//     may take, while they are online.
//   - With even item share, the item goes to a member on the map who is
//     alive, at random or in turn by party_item_share_type. A member who
//     cannot carry it is skipped, and the picker gets it when no one
//     can.
func (z *Zone) take(p *Player, it *entity, net uint32) {
	g := it.item
	var sh social.Share
	inParty := false
	if p.soc != nil && z.Social != nil {
		sh, inParty = z.Social.ShareOf(p.soc)
	}
	shared := inParty && sh.Options.PickupShared
	sameParty := func(i int) bool {
		if g.looters[i] == "" {
			return false
		}
		o, ok := z.Social.ShareOfName(g.looters[i])
		return ok && o.Party == sh.Party
	}
	if !g.rights.CanTakeParty(p.charID, z.now, shared, sameParty) {
		p.pending = append(p.pending, pickUpFailed(net, wathenav1.PickUpFailure_PICK_UP_FAILURE_NOT_YOURS))
		return
	}
	// shareLoot gives the item itself when it picks a member, even when
	// it picks the picker, so the picker only gets it here when the item
	// was not shared.
	target := p
	var sharedTo *Player
	if inParty {
		sharedTo = z.shareLoot(p, sh, g)
	}
	if sharedTo != nil {
		target = sharedTo
	} else {
		// party_share_loot gives the item with pc_additem. When it fails,
		// the item stays and the player learns why.
		if r := z.addCopy(p, g.id, g.amount, g.refine, g.cards, g.extra, g.pickUpOrigin()); r != item.AddSuccess {
			p.pending = append(p.pending, pickUpFailed(net, pickUpFailure(r)))
			return
		}
	}
	// unit_stop_attack.
	p.entity.target = nil
	z.entities.remove(it)
	z.takes = append(z.takes, take{p.entity, it})
	z.removed = append(z.removed, removal{it, wathenav1.VanishReason_VANISH_REASON_PICKED_UP})
	if inParty && showPicker(z.Items.Get(g.id)) {
		// clif_party_show_picker, with show_party_share_picker and
		// show_picker.item_type's default: armor, weapons and cards.
		z.Social.ItemShared(sh.Party, z.Map, social.ItemShared{Name: target.Name, Item: g.id, Amount: g.amount})
	}
}

// showPicker reports whether an item's type is in show_picker.item_type's
// default, 112: IT_ARMOR, IT_WEAPON and IT_CARD.
func showPicker(d *item.Def) bool {
	return d != nil && (d.Type == "Armor" || d.Type == "Weapon" || d.Type == "Card")
}

// shareLoot is the member pick of party_share_loot. It gives the item to
// the member it picks and returns that member, or nil when the item is
// not shared or no member could take it.
func (z *Zone) shareLoot(p *Player, sh social.Share, g *groundItem) *Player {
	shareType := rules.PartyOrDefault(z.worldRules()).ItemShareType
	if !social.SharesLoot(sh.Options, shareType, g.rights.First != 0) {
		return nil
	}
	slots := z.slotsHere(sh)
	eligible := func(i int) bool { return i < len(slots) && slots[i] != nil && !slots[i].entity.dead }
	give := func(i int) bool {
		return z.addCopy(slots[i], g.id, g.amount, g.refine, g.cards, g.extra, g.pickUpOrigin()) == item.AddSuccess
	}
	if shareType&social.ShareRoundRobin != 0 {
		slot, ok := social.RoundRobin(sh.ItemC, len(sh.Slots), eligible, give)
		if !ok {
			return nil
		}
		z.Social.SetItemC(sh.Party, slot)
		return slots[slot]
	}
	var candidates []int
	for i := range slots {
		if eligible(i) {
			candidates = append(candidates, i)
		}
	}
	slot, ok := social.RandomPick(candidates, z.rand.IntN, give)
	if !ok {
		return nil
	}
	return slots[slot]
}

// slotsHere maps a party's slots to this zone's players, by slot: nil
// for a hole or a member who is not on this map.
func (z *Zone) slotsHere(sh social.Share) []*Player {
	byKey := map[string]*Player{}
	for _, p := range z.players {
		if !p.gone() && p.soc != nil && z.entities.has(p.entity) {
			byKey[p.soc.Key()] = p
		}
	}
	out := make([]*Player, len(sh.Slots))
	for i, k := range sh.Slots {
		if k != "" {
			out[i] = byKey[k]
		}
	}
	return out
}

// pickUpOrigin is where a ground item comes from, for the log of its
// pick-up: the ground, with the serial of the stack it was dropped from,
// or the monster that dropped it.
func (g *groundItem) pickUpOrigin() origin {
	o := origin{action: transfer.ActionPickUp, from: transfer.End(transfer.KindGround), source: g.origin}
	if g.origin.IsZero() && g.from.Kind != transfer.KindNone {
		o.from = g.from
	}
	return o
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
