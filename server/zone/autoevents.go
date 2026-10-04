package zone

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Typed world events in a zone (proposal 0002, review decision 31,
// decision 0025). The world's event manager (server/worldevent) decides
// what a run does; the zone does it on its map:
//   - It places a run's monsters: a boss, an invasion's waves at the
//     map's edges, or treasure chests, each on a random walkable cell.
//   - Every hit of a player on a run's monster adds to the run's tally,
//     as mob_log_damage does for the monster's own damage log.
//   - A boss's or a chest's death gives the run's rewards at once, and
//     tells the manager. An invasion's rewards come when the manager
//     decides the raid is won.
//   - Rewards go through transfers, with the action "event". Items that
//     do not fit and zeny over MAX_ZENY are mailed. Each reward writes a
//     progress row in the same commit, so a restart never gives it
//     twice.
//   - Invaders are aggressive. With no target, they walk toward the
//     nearest player on the map.
//   - Hint NPCs answer from the manager, which knows the chests of every
//     map.

// zoneRun is a typed run's part on this map.
type zoneRun struct {
	info worldevent.RunInfo
	// minShare is the share of the damage, in percent, that makes a
	// player a participant.
	minShare float64
	// damage is each player's damage to the run's monsters on this map,
	// by name, and total their sum.
	damage map[string]int64
	total  int64
	// lastKiller is the name of the player who killed the run's last
	// monster here.
	lastKiller string
	monsters   map[*entity]bool
	npcs       []*entity
	// rewarded are the units of progress rewarded here.
	rewarded map[string]bool
}

// eventTag marks a typed run's monster.
type eventTag struct {
	run      *zoneRun
	role     string
	unit     string
	rewards  *worldevent.Rewards
	announce bool
}

// EventUnit is a unit of a run's progress that a commit saves: a reward
// that was given, so a restart does not give it again.
type EventUnit struct {
	Event     string
	Scheduled time.Time
	Unit      string
}

// hintBase is where the script handles of hint NPCs start, above the
// handles of the map's NPCs.
const hintBase = int64(1) << 40

// edgeDepth is how far from the map's border an invader's cell may be
// sought, in cells, before the search gives up and picks any cell.
const edgeDepth = 400

// marchReach is how far an invader with no target walks at once toward
// the nearest player.
const marchReach = 12

// runOf returns the zone's part of a run, made on first use.
func (z *Zone) runOf(info worldevent.RunInfo, rewards *worldevent.Rewards) *zoneRun {
	if z.typedRuns == nil {
		z.typedRuns = map[int64]*zoneRun{}
	}
	r := z.typedRuns[info.ID]
	if r == nil {
		r = &zoneRun{info: info, minShare: worldevent.DefaultMinShare, damage: map[string]int64{},
			monsters: map[*entity]bool{}, rewarded: map[string]bool{}}
		z.typedRuns[info.ID] = r
	}
	if rewards != nil {
		r.minShare = rewards.MinShare
	}
	return r
}

// SpawnEvent places a typed run's monsters. It waits until the zone has
// done it.
func (z *Zone) SpawnEvent(ctx context.Context, s worldevent.Spawn) ([]worldevent.Placed, error) {
	var out []worldevent.Placed
	err := z.do(ctx, func() error {
		var err error
		out, err = z.spawnEvent(s)
		return err
	})
	return out, err
}

// EventAlive counts a run's monsters that live here.
func (z *Zone) EventAlive(ctx context.Context, run int64) (int, error) {
	var n int
	err := z.do(ctx, func() error { n = z.eventAlive(run); return nil })
	return n, err
}

// EventHints places a treasure hunt's hint NPCs.
func (z *Zone) EventHints(ctx context.Context, run int64, hints []worldevent.Hint) error {
	return z.do(ctx, func() error { z.eventHints(run, hints); return nil })
}

// EventReward gives a run's rewards for a unit of progress.
func (z *Zone) EventReward(ctx context.Context, rr worldevent.RewardRun) error {
	return z.do(ctx, func() error { z.eventReward(rr); return nil })
}

// ClearEvent removes a run's monsters and NPCs, and forgets the run.
func (z *Zone) ClearEvent(ctx context.Context, run int64) error {
	return z.do(ctx, func() error { z.clearEvent(run); return nil })
}

// WorldEventNotice tells the zone that a run started or ended. It never
// waits: when the zone is far behind, the notice is dropped.
func (z *Zone) WorldEventNotice(n worldevent.Notice) {
	select {
	case z.notices <- n:
	default:
	}
}

func (z *Zone) spawnEvent(s worldevent.Spawn) ([]worldevent.Placed, error) {
	if z.Mods == nil || z.Mods.Monsters == nil {
		return nil, errors.New("there are no monster records")
	}
	tmpl, ok := z.Mods.Monsters[s.Monster]
	if !ok {
		return nil, fmt.Errorf("there is no monster %d", s.Monster)
	}
	r := z.runOf(s.RunInfo, s.Rewards)
	if s.Role == worldevent.RoleInvader && tmpl.Status != nil {
		// Invaders attack players on sight, and walk.
		st := *tmpl.Status
		st.Mode |= mob.ModeAggressive | mob.ModeCanMove | mob.ModeCanAttack
		tmpl.Status = &st
		tmpl.CanMove = true
	}
	var out []worldevent.Placed
	for range s.Amount {
		sp := tmpl
		sp.Amount, sp.noRespawn, sp.owner = 1, true, -s.ID
		sp.X, sp.Y, sp.W, sp.H = 0, 0, 0, 0
		if s.Edge {
			if x, y, ok := z.edgeCell(); ok {
				sp.X, sp.Y = x, y
			}
		}
		e := z.spawnOne(&sp)
		if e == nil {
			continue
		}
		e.monster.event = &eventTag{run: r, role: s.Role, unit: s.Unit, rewards: s.Rewards, announce: s.Announce}
		r.monsters[e] = true
		out = append(out, worldevent.Placed{X: e.x, Y: e.y, Name: e.monster.spawn.Name})
	}
	if len(out) == 0 && s.Amount > 0 {
		return nil, fmt.Errorf("no walkable cell on %s", z.Map)
	}
	return out, nil
}

// edgeCell picks a walkable cell where an invasion enters: from a random
// cell of a random side of the map, the first walkable cell toward the
// map's center.
func (z *Zone) edgeCell() (int, int, bool) {
	g := z.grid
	if g == nil || g.Width < 2 || g.Height < 2 {
		return 0, 0, false
	}
	for range 64 {
		var x, y, dx, dy int
		switch z.rand.IntN(4) {
		case 0:
			x, y, dy = z.rand.IntN(g.Width), 0, 1
		case 1:
			x, y, dy = z.rand.IntN(g.Width), g.Height-1, -1
		case 2:
			x, y, dx = 0, z.rand.IntN(g.Height), 1
		default:
			x, y, dx = g.Width-1, z.rand.IntN(g.Height), -1
		}
		for range min(edgeDepth, max(g.Width, g.Height)/2) {
			if !g.NoPass(x, y) {
				return x, y, true
			}
			x, y = x+dx, y+dy
		}
	}
	return 0, 0, false
}

func (z *Zone) eventAlive(run int64) int {
	r := z.typedRuns[run]
	if r == nil {
		return 0
	}
	n := 0
	for e := range r.monsters {
		if !e.dead && z.entities.has(e) {
			n++
		}
	}
	return n
}

// eventDamage adds a player's hit on a run's monster to the run's tally.
func (z *Zone) eventDamage(src, target *entity, logged int64) {
	t := target.monster.event
	if t == nil || src.player == nil || logged <= 0 {
		return
	}
	t.run.damage[src.player.Name] += logged
	t.run.total += logged
}

// eventDied is a run's monster's death, after mob_dead's rewards: a boss
// or a chest gives the run's rewards and tells the manager.
func (z *Zone) eventDied(e, killer *entity) {
	t := e.monster.event
	if t == nil {
		return
	}
	r := t.run
	delete(r.monsters, e)
	var killerP *Player
	if killer != nil && killer.player != nil {
		killerP = killer.player
		r.lastKiller = killerP.Name
	}
	if t.role == worldevent.RoleInvader {
		return
	}
	mvp := z.mvpOf(e)
	if t.rewards != nil {
		z.giveRewards(r, t.unit, t.rewards, z.recipients(t.rewards, killerP, e.monster.dmglog.Entries))
	} else {
		z.recordUnit(r, t.unit)
	}
	note := worldevent.Note{Kind: worldevent.NoteDied, Run: r.info.ID, Map: z.Map, Unit: t.unit, Monster: e.monster.spawn.Name,
		X: e.x, Y: e.y}
	if killerP != nil {
		note.Killer = killerP.Name
	}
	if mvp != nil {
		note.MVP = mvp.Name
	}
	if z.WorldEvents != nil {
		z.WorldEvents.Note(note)
	}
}

// mvpOf is get_mvp_player: the player on the map with the most damage
// dealt and taken, who lives.
func (z *Zone) mvpOf(e *entity) *Player {
	var best int64
	var mvp *Player
	for _, en := range e.monster.dmglog.Entries {
		p := z.players[en.CharID]
		if p == nil || p.entity.dead || !z.entities.has(p.entity) {
			continue
		}
		if sum := en.Damage + en.Tanked; sum > best {
			best, mvp = sum, p
		}
	}
	return mvp
}

// recipients are the players on the map who get a death's rewards: the
// killer, the players with at least the min share of the monster's
// damage, or every player on the map.
func (z *Zone) recipients(rw *worldevent.Rewards, killer *Player, log []progression.DamageEntry) []*Player {
	switch rw.To {
	case worldevent.ToKiller:
		if killer != nil && z.entities.has(killer.entity) {
			return []*Player{killer}
		}
		return nil
	case worldevent.ToEveryone:
		return z.playersByName()
	}
	var total int64
	for _, en := range log {
		total += en.Damage
	}
	var out []*Player
	for _, en := range log {
		p := z.players[en.CharID]
		if p == nil || !z.entities.has(p.entity) || total <= 0 {
			continue
		}
		if float64(en.Damage)*100 >= rw.MinShare*float64(total) {
			out = append(out, p)
		}
	}
	return out
}

// participants are the players on the map whose damage to the run's
// monsters here is at least the run's min share, by name.
func (z *Zone) participants(r *zoneRun) []*Player {
	var out []*Player
	for _, p := range z.playersByName() {
		d := r.damage[p.Name]
		if d > 0 && r.total > 0 && float64(d)*100 >= r.minShare*float64(r.total) {
			out = append(out, p)
		}
	}
	return out
}

// playersByName are the players on the map, by name.
func (z *Zone) playersByName() []*Player {
	out := make([]*Player, 0, len(z.players))
	for _, p := range z.players {
		if p.entity != nil && z.entities.has(p.entity) {
			out = append(out, p)
		}
	}
	slices.SortFunc(out, func(a, b *Player) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

func (z *Zone) eventReward(rr worldevent.RewardRun) {
	r := z.runOf(rr.RunInfo, rr.Rewards)
	if rr.Rewards == nil {
		z.recordUnit(r, rr.Unit)
		return
	}
	var to []*Player
	switch rr.Rewards.To {
	case worldevent.ToKiller:
		for _, p := range z.players {
			if p.Name == r.lastKiller && z.entities.has(p.entity) {
				to = append(to, p)
			}
		}
	case worldevent.ToEveryone:
		to = z.playersByName()
	default:
		to = z.participants(r)
	}
	z.giveRewards(r, rr.Unit, rr.Rewards, to)
}

// recordUnit saves a unit of progress with the next commit.
func (z *Zone) recordUnit(r *zoneRun, unit string) {
	if unit == "" || r.rewarded[unit] {
		return
	}
	r.rewarded[unit] = true
	z.txEventUnits = append(z.txEventUnits, EventUnit{Event: r.info.Event, Scheduled: r.info.Scheduled, Unit: unit})
}

// giveRewards gives each player the full reward, once for a unit.
func (z *Zone) giveRewards(r *zoneRun, unit string, rw *worldevent.Rewards, to []*Player) {
	if unit != "" && r.rewarded[unit] {
		return
	}
	z.recordUnit(r, unit)
	for _, p := range to {
		z.giveReward(p, r, rw)
	}
}

// eventOrigin is where a reward comes from, for the logs.
var eventOrigin = origin{action: transfer.ActionEvent, from: transfer.End(transfer.KindEvent)}

// giveReward gives one player a reward: items, zeny, exp and currency.
// What does not fit is mailed to the player.
func (z *Zone) giveReward(p *Player, r *zoneRun, rw *worldevent.Rewards) {
	var mail []modscript.ScriptMailItem
	for _, it := range rw.Items {
		d, ok := z.Items.Lookup(it.ID)
		if !ok {
			continue
		}
		step := it.Amount
		if !d.Stackable {
			step = 1
		}
		for given := 0; given < it.Amount; given += step {
			x := z.newExtra(d, false, rollSource{kind: sourceScript}, "")
			if z.addCopy(p, it.ID, step, 0, nil, x, eventOrigin) != item.AddSuccess {
				mail = append(mail, modscript.ScriptMailItem{Item: it.ID, Amount: it.Amount - given})
				break
			}
		}
	}
	var mailZeny int64
	if rw.Zeny > 0 {
		give := min(rw.Zeny, max(character.MaxZeny-p.record.Zeny, 0))
		z.earnZeny(p, give, transfer.ActionEvent, transfer.End(transfer.KindEvent))
		mailZeny = rw.Zeny - give
	}
	if rw.BaseExp > 0 || rw.JobExp > 0 {
		z.gainExp(p, rw.BaseExp, rw.JobExp)
	}
	for _, name := range sortedNames(rw.Currency) {
		c := z.currencies().Resolve(name, modOf(r.info.Event))
		if c == nil {
			continue
		}
		w, _, _ := z.currencyWallet(p, c)
		if w == nil {
			continue
		}
		tx := transfer.New(transfer.ActionEvent).Earn(transfer.End(transfer.KindEvent), rw.Currency[name], w, true)
		_ = z.applyWithCurrencies(p, tx, []*transfer.CurrencyWallet{w}, 0)
	}
	if len(mail) > 0 || mailZeny > 0 {
		for len(mail) > 0 || mailZeny > 0 {
			n := min(len(mail), max(z.limits().MailItems, 1))
			sm := modscript.ScriptMail{Sender: cut(r.info.Display, 23), Title: cut("Reward: "+r.info.Display, 39),
				Body: "Your reward did not fit in your inventory.", Zeny: mailZeny, Items: mail[:n]}
			mail, mailZeny = mail[n:], 0
			if err := z.mailFrom(p, sm, transfer.ActionEvent, transfer.End(transfer.KindEvent), false); err != nil {
				z.warn("a world event's reward was not mailed", "player", p.Name, "err", err)
			}
		}
	}
	p.pending = append(p.pending, systemMessage("You got the reward of "+r.info.Display+"."))
}

// cut shortens a text to at most n bytes, on a rune boundary.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

// modOf is the mod of a full name, such as example.a of example.a:Boss.
func modOf(full string) string {
	for i := range len(full) {
		if full[i] == ':' {
			return full[:i]
		}
	}
	return ""
}

func (z *Zone) eventHints(run int64, hints []worldevent.Hint) {
	r := z.typedRuns[run]
	if r == nil {
		r = z.runOf(worldevent.RunInfo{ID: run}, nil)
	}
	if z.npcHandles == nil {
		z.npcHandles = map[int64]*entity{}
	}
	for i, h := range hints {
		if h.Map != z.Map {
			continue
		}
		z.nextHint++
		n := &npc{NPC: NPC{Name: fmt.Sprintf("%s#hint%d", r.info.Event, i+1), DisplayName: h.DisplayName, X: h.X, Y: h.Y, Sprite: h.SpriteID},
			handle: hintBase + z.nextHint, hint: run}
		e := &entity{npc: n, x: h.X, y: h.Y}
		z.entities.add(e)
		z.npcHandles[n.handle] = e
		r.npcs = append(r.npcs, e)
	}
}

func (z *Zone) clearEvent(run int64) {
	r := z.typedRuns[run]
	if r == nil {
		return
	}
	delete(z.typedRuns, run)
	for e := range r.monsters {
		if e.dead || !z.entities.has(e) {
			continue
		}
		z.entities.remove(e)
		z.forget(e)
		z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT})
		z.endMonsterScript(e)
		if e.monster.handle != 0 {
			delete(z.monsterHandles, e.monster.handle)
		}
	}
	for _, e := range r.npcs {
		for _, p := range z.players {
			if p.talk != nil && p.talk.npc == e {
				z.closeTalk(p)
			}
		}
		z.entities.remove(e)
		z.forget(e)
		z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT})
		delete(z.npcHandles, e.npc.handle)
	}
}

// hintTalk shows what a hint NPC says, with a Close button.
func (z *Zone) hintTalk(p *Player, e *entity) {
	var lines []string
	if z.WorldEvents != nil {
		lines = z.WorldEvents.Hint(e.npc.hint, z.Map, e.x, e.y)
	}
	if len(lines) == 0 {
		lines = []string{"Every chest has been found."}
	}
	lines = append([]string{"[" + e.npc.DisplayName + "]"}, lines...)
	z.closeShop(p, false)
	p.entity.target, p.entity.pickItem = nil, nil
	p.dialogSeq++
	t := &talk{npc: e, dialog: p.dialogSeq, ended: true, shown: true, deadline: z.now + timeoutNext}
	p.talk = t
	z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Lines = lines })
	z.dialog(p, t, func(b *wathenav1.NpcDialog_builder) { b.Close = new(true) })
}

// invaderMarch walks an invader with no target toward the nearest
// player on the map. It reports whether a walk started.
func (z *Zone) invaderMarch(e *entity) bool {
	m := e.monster
	if m.event == nil || m.event.role != worldevent.RoleInvader || len(e.path) > 0 || z.now < m.nextWalk ||
		e.status == nil || e.status.Mode&mob.ModeCanMove == 0 || !z.unitCanMove(e) {
		return false
	}
	var goal *entity
	best := -1
	for _, p := range z.players {
		pe := p.entity
		if pe == nil || pe.dead || !z.entities.has(pe) {
			continue
		}
		if d := chebyshev(e, pe); best < 0 || d < best {
			best, goal = d, pe
		}
	}
	if goal == nil || best <= 1 {
		return false
	}
	dx, dy := goal.x-e.x, goal.y-e.y
	step := min(best, marchReach)
	for try := range 8 {
		tx := e.x + dx*step/best
		ty := e.y + dy*step/best
		if try > 0 {
			tx += z.rand.IntN(7) - 3
			ty += z.rand.IntN(7) - 3
		}
		if z.grid.NoPass(tx, ty) {
			continue
		}
		steps, ok := path.Search(z.grid, e.x, e.y, tx, ty, false)
		if !ok || len(steps) == 0 {
			continue
		}
		e.path, e.stepStart, e.walkChanged = steps, z.now, true
		m.walked = 0
		m.ai.SetState(mob.StateWalk, e.status.Mode)
		m.nextWalk = z.now + int64(path.PathTime(steps, e.speed))
		return true
	}
	m.nextWalk = z.now + 1000
	return false
}

// worldNotice is a run that started or ended: world_event_start or
// world_event_end runs, and every player is told.
func (z *Zone) worldNotice(n worldevent.Notice) {
	if r := z.itemScripts(); r != nil {
		name := "world_event_end"
		if n.Started {
			name = "world_event_start"
		}
		r.FireWorldEvent(name, modscript.WorldEvent{Name: n.Name, Type: noticeType(n.Type), Result: n.Result})
	}
	ev := wathenav1.Event_builder{WorldEventNotice: wathenav1.WorldEventNotice_builder{
		Event: worldEventInfo(n), Started: n.Started, Result: n.Result,
	}.Build()}.Build()
	for _, p := range z.players {
		p.pending = append(p.pending, ev)
	}
}

// noticeType is a run's type for scripts: "script" for an event without
// a type.
func noticeType(t string) string { return cmp.Or(t, "script") }

func worldEventInfo(n worldevent.Notice) *wathenav1.WorldEventInfo {
	b := wathenav1.WorldEventInfo_builder{Name: n.Name, DisplayName: n.DisplayName, Type: noticeType(n.Type), Map: n.Map}
	if !n.Ends.IsZero() {
		b.EndsAtMs = uint64(max(n.Ends.UnixMilli(), 0)) // #nosec G115 -- not negative.
	}
	return b.Build()
}

// sendWorldEvents tells a player who logs in the runs that last now.
func (z *Zone) sendWorldEvents(p *Player) {
	if z.WorldEvents == nil {
		return
	}
	running := z.WorldEvents.Running()
	if len(running) == 0 {
		return
	}
	var list []*wathenav1.WorldEventInfo
	for _, n := range running {
		list = append(list, worldEventInfo(n))
	}
	p.pending = append(p.pending, wathenav1.Event_builder{WorldEventList: wathenav1.WorldEventList_builder{Events: list}.Build()}.Build())
}

// The run handle of context:get_event() (modscript.EventHost).

// EventMonsters returns the handles of a run's monsters that live here.
func (h scriptHost) EventMonsters(run int64) []int64 {
	z := h.z
	r := z.typedRuns[run]
	if r == nil {
		return nil
	}
	var list []*entity
	for e := range r.monsters {
		if !e.dead && z.entities.has(e) {
			list = append(list, e)
		}
	}
	slices.SortFunc(list, func(a, b *entity) int { return cmp.Compare(a.uid, b.uid) })
	out := make([]int64, len(list))
	for i, e := range list {
		out[i] = z.monsterHandle(e)
	}
	return out
}

// EventParticipants returns the players here who did at least the run's
// min share of the damage to its monsters here, by name.
func (h scriptHost) EventParticipants(run int64) []int64 {
	r := h.z.typedRuns[run]
	if r == nil {
		return nil
	}
	var out []int64
	for _, p := range h.z.participants(r) {
		out = append(out, int64(p.charID))
	}
	return out
}

// FinishEvent ends a run with a result, from run:finish.
func (h scriptHost) FinishEvent(run int64, result string) error {
	if h.z.WorldEvents == nil {
		return errors.New("run:finish: world events do not run here")
	}
	h.z.WorldEvents.Note(worldevent.Note{Kind: worldevent.NoteFinish, Run: run, Map: h.z.Map, Result: result})
	return nil
}

// RequestWorldEvent is world_events.start and world_events.stop.
func (h scriptHost) RequestWorldEvent(name, mod string, start bool) (bool, error) {
	if h.z.WorldEvents == nil {
		return false, errors.New("world events do not run here")
	}
	return h.z.WorldEvents.Request(name, mod, start)
}
