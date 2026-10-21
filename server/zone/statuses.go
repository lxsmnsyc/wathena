package zone

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Status changes: every entity can have statuses (server/statuschange).
// The zone runs the engine on its clock, as rAthena's timers run
// status_change_timer, and does what the engine asks of the world:
//   - Stats are computed again when a status changes them: players
//     through status.Calc, monsters through status.CalcMonster.
//   - The player sees its status icons, and everyone who sees an entity
//     sees its options, such as Stun's stars.
//   - Mod scripts get the events status_start, status_end and
//     status_tick, and their hooks status_chance and status_duration
//     change chances and durations.
//   - Players keep the statuses rAthena saves across logout and map
//     changes, and their timed bonuses (rAthena's bonus_script).

// npcStatus is the status of entities without one, such as NPCs: level
// 1 and 1 HP, as status_calc_npc leaves an NPC.
var npcStatus = battle.Status{Level: 1, MaxHP: 1, MaxSP: 1, Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1, EleLv: 1}

// statusEngine returns the zone's status engine, made on first use, or
// nil when the zone has no status data.
func (z *Zone) statusEngine() *sc.Engine {
	if z.statusChanges == nil && z.Statuses != nil {
		z.statusChanges = sc.New(z.Statuses, scEnv{z})
	}
	if z.statusChanges != nil {
		// The world's ruleset: status_resist, ensemble_songs and the
		// skills' flags.
		z.statusChanges.Rules = z.rules()
	}
	return z.statusChanges
}

// scUnit is an entity as the engine sees it.
type scUnit struct {
	z *Zone
	e *entity
}

func (z *Zone) unit(e *entity) scUnit { return scUnit{z, e} }

func (u scUnit) Changes() *sc.List {
	if u.e.sc == nil {
		u.e.sc = sc.NewList()
	}
	return u.e.sc
}

func (u scUnit) Kind() sc.Kind {
	switch {
	case u.e.player != nil:
		return sc.KindPlayer
	case u.e.monster != nil:
		return sc.KindMonster
	case u.e.npc != nil:
		return sc.KindNPC
	}
	return sc.KindNone
}

func (u scUnit) Status() *battle.Status {
	if u.e.status != nil {
		return u.e.status
	}
	return &npcStatus
}

func (u scUnit) Base() *battle.Status {
	switch {
	case u.e.player != nil:
		return &u.e.player.base
	case u.e.monster != nil && u.e.monster.spawn.Status != nil:
		return u.e.monster.spawn.Status
	}
	return u.Status()
}

func (u scUnit) Level() int {
	switch {
	case u.e.player != nil:
		if u.z.Progress != nil {
			return u.e.player.progress.BaseLevel
		}
		return 1
	case u.e.status != nil:
		return u.e.status.Level
	}
	return 1
}

func (u scUnit) JobLevel() int {
	if p := u.e.player; p != nil && u.z.Progress != nil {
		return p.progress.JobLevel
	}
	return 1
}

func (u scUnit) HP() int64 {
	if u.e.status == nil {
		return 1
	}
	return u.e.hp
}

func (u scUnit) SP() int64 {
	if p := u.e.player; p != nil {
		return p.sp
	}
	return int64(u.Status().MaxSP)
}

func (u scUnit) Dead() bool { return u.e.dead }

func (u scUnit) ResEff(typ int) int {
	if p := u.e.player; p != nil {
		return p.bonuses.ResEff[typ]
	}
	return 0
}

func (u scUnit) Skill(id int) int {
	if p := u.e.player; p != nil {
		if st := u.e.status; st != nil && st.Player != nil {
			return st.Player.Skills[id]
		}
		return p.char.Skills[id]
	}
	return 0
}

func (u scUnit) AlwaysEndure() bool {
	if p := u.e.player; p != nil {
		return p.bonuses.AlwaysEndure
	}
	return false
}

func (u scUnit) Ref() sc.Source {
	switch {
	case u.e.player != nil:
		return sc.Source{Kind: sc.KindPlayer, ID: int64(u.e.player.charID)}
	case u.e.monster != nil:
		return sc.Source{Kind: sc.KindMonster, ID: u.z.monsterHandle(u.e)}
	case u.e.npc != nil:
		return sc.Source{Kind: sc.KindNPC, ID: u.e.npc.handle}
	}
	return sc.Source{}
}

// scEnv is the zone as the engine sees it.
type scEnv struct{ z *Zone }

func (v scEnv) Now() int64 { return v.z.now }

func (v scEnv) RandN(n int) int {
	if n <= 1 {
		return 0
	}
	return v.z.rand.IntN(n)
}

// Recalc is status_calc_bl: stats come again from the statuses, at
// once, as rAthena computes them inside status_change_start and
// status_change_end. A script that starts or ends a status reads the new
// stats after the call.
func (v scEnv) Recalc(u sc.Unit, def *sc.Def) {
	z, e := v.z, u.(scUnit).e
	switch {
	case e.player != nil:
		p := e.player
		pending := p.statsDirty
		z.markStats(p)
		if !pending && !recalcsAspd(def) {
			// status_calc_bl only computes what the status's flags name.
			// Of what this server computes, only ASPD can differ from a
			// full status_calc_pc: renewal's ASPD reads Musical Lesson
			// and Plagiarism only while a status is active.
			p.keepAspd = true
		}
		if !pending && def != nil && !def.Calcs("base") {
			// Only SCB_BASE's status_calc_pc recalculates skills, which
			// ends player:give_skill's temporary levels.
			p.keepTempSkills = true
		}
		z.flushStats()
	case e.monster != nil && e.monster.spawn.Status != nil:
		z.recalcMonster(e)
	}
}

// recalcsAspd reports whether status_calc_bl computes ASPD for a
// status's flags: SCB_ASPD, or SCB_AGI and SCB_DEX, which lead to it,
// or SCB_BASE, which is a full status_calc_pc. A nil status computes
// everything.
func recalcsAspd(def *sc.Def) bool {
	return def == nil || def.Calcs("aspd") || def.Calcs("agi") || def.Calcs("dex") || def.Calcs("base")
}

// recalcMonster is status_calc_bl_main for a monster: its battle status
// from its base status and its statuses. A monster without statuses
// shares its spawn's status.
func (z *Zone) recalcMonster(e *entity) {
	base := e.monster.spawn.Status
	speed := e.speed
	if e.sc.Empty() {
		e.status = base
	} else {
		st := status.CalcMonsterRules(base, e.sc, z.rules())
		e.status = &st
	}
	if e.hp > int64(e.status.MaxHP) {
		e.hp, e.hpChanged = int64(e.status.MaxHP), true
	}
	if e.status.Speed > 0 {
		e.speed = e.status.Speed
	}
	z.speedChanged(e, speed)
}

// speedChanged sends a walking entity's path again when its speed
// changed, so viewers walk it at the new speed, as rAthena's
// change_walk_target.
func (z *Zone) speedChanged(e *entity, before int) {
	if e.speed != before && len(e.path) > 0 {
		e.walkChanged = true
	}
}

func (v scEnv) Icon(u sc.Unit, icon int, on bool, tick int64, v1, v2, v3 int) {
	p := u.(scUnit).e.player
	if p == nil || icon < 0 {
		return
	}
	if !on {
		tick = 0
	}
	p.pending = append(p.pending, wathenav1.Event_builder{StatusIcon: wathenav1.StatusIcon_builder{
		Icon:        uint32(icon), // #nosec G115 -- checked above.
		On:          on,
		RemainingMs: tick,
		Values:      []int32{int32(v1), int32(v2), int32(v3)}, // #nosec G115 -- rAthena's values are int32.
	}.Build()}.Build())
}

func (v scEnv) OptionChanged(u sc.Unit) { u.(scUnit).e.optionChanged = true }

func (v scEnv) StopWalking(u sc.Unit) {
	e := u.(scUnit).e
	if len(e.path) > 0 {
		e.path = e.path[:0]
		e.stopped = true
	}
}

func (v scEnv) StopAttacking(u sc.Unit) { u.(scUnit).e.target = nil }

// Damage is status_damage without a source. A player can die of it; a
// monster's statuses never kill it.
func (v scEnv) Damage(u sc.Unit, hp, sp int64, passive, noKill bool) int64 {
	z, e := v.z, u.(scUnit).e
	if e.status == nil || e.dead {
		return 0
	}
	if noKill && hp > 0 && hp >= e.hp {
		return 0
	}
	hp = min(max(hp, 0), e.hp)
	if hp > 0 {
		e.hp -= hp
		e.hpChanged = true
	}
	if p := e.player; p != nil && sp > 0 {
		p.sp = max(p.sp-sp, 0)
		if s := z.battleStats(p); s != nil {
			p.pending = append(p.pending, s)
		}
	}
	if !passive {
		z.damaged(e, hp, false)
	}
	if e.hp <= 0 {
		z.die(e, nil)
		return hp
	}
	z.hpLowered(e)
	return hp
}

// SetHP is status_set_hp: HP becomes the value, by damage or a heal
// that statuses do not block.
func (v scEnv) SetHP(u sc.Unit, hp int64) {
	e := u.(scUnit).e
	if e.status == nil || e.dead {
		return
	}
	hp = min(max(hp, 1), int64(e.status.MaxHP))
	if hp != e.hp {
		e.hp, e.hpChanged = hp, true
	}
}

// SetSP is status_set_sp.
func (v scEnv) SetSP(u sc.Unit, sp int64) {
	z, e := v.z, u.(scUnit).e
	p := e.player
	if p == nil || e.status == nil {
		return
	}
	p.sp = min(max(sp, 0), int64(e.status.MaxSP))
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
}

// ForceHeal is status_heal with flag 1: a heal that Frenzy does not
// block.
func (v scEnv) ForceHeal(u sc.Unit, hp int64) {
	e := u.(scUnit).e
	if e.status == nil || e.dead || hp <= 0 {
		return
	}
	e.hp = min(e.hp+hp, int64(e.status.MaxHP))
	e.hpChanged = true
}

// refreshChanges keeps what attacks read of a unit's statuses current.
// rAthena's battle code reads the statuses at each attack, so a status
// without calc flags, such as SC_WATK_ELEMENT or SC_AUTOCOUNTER, changes
// attacks without a stat calculation. A monster that shares its spawn's
// status gets its own copy first.
func (z *Zone) refreshChanges(e *entity) {
	if e.status == nil {
		return
	}
	if m := e.monster; m != nil && e.status == m.spawn.Status {
		if e.sc.Empty() {
			return
		}
		st := *e.status
		e.status = &st
	}
	e.status.Changes = status.BattleChanges(e.sc)
}

// hpLowered is the end of status_damage: Frenzy ends at 100 HP or less.
func (z *Zone) hpLowered(e *entity) {
	if e.hp <= 100 && e.sc.Has(sc.SCBerserk) {
		z.scEnd(e, sc.SCBerserk)
	}
}

func (v scEnv) Heal(u sc.Unit, hp, sp int64) {
	z, e := v.z, u.(scUnit).e
	if p := e.player; p != nil {
		_, gotSP := z.statusHeal(p, hp, sp)
		if gotSP > 0 {
			if s := z.battleStats(p); s != nil {
				p.pending = append(p.pending, s)
			}
		}
		return
	}
	if e.status != nil && !e.dead && hp > 0 {
		e.hp = min(e.hp+hp, int64(e.status.MaxHP))
		e.hpChanged = true
	}
}

func (v scEnv) Started(u sc.Unit, e *sc.Entry) {
	v.z.keepingStarted(u.(scUnit).e, e)
	v.z.refreshChanges(u.(scUnit).e)
	if v.z.quietStatuses {
		return
	}
	v.z.statusEvent("status_start", u.(scUnit), e, e.Remaining(v.z.now), string(e.Reason))
}

func (v scEnv) Ended(u sc.Unit, e *sc.Entry, reason sc.Reason) {
	v.z.statusEndEffects(u.(scUnit).e, e)
	v.z.performanceEnded(u.(scUnit).e, e)
	v.z.refreshChanges(u.(scUnit).e)
	v.z.monkStatusEnded(u.(scUnit).e, e)
	v.z.keepingEnded(u.(scUnit).e, e)
	v.z.transformEnded(u.(scUnit).e, e.Type)
	if v.z.quietStatuses {
		return
	}
	v.z.statusEvent("status_end", u.(scUnit), e, e.Remaining(v.z.now), string(reason))
}

func (v scEnv) Ticked(u sc.Unit, e *sc.Entry) {
	v.z.statusEvent("status_tick", u.(scUnit), e, e.Remaining(v.z.now), "")
}

// Strip is the strip part of status_change_start, as stripEquip.
func (v scEnv) Strip(u sc.Unit, typ int) bool { return v.z.stripEquip(u.(scUnit).e, typ) }

// UnitOf is map_id2bl: the player or monster a source names, while it
// is in the zone.
func (v scEnv) UnitOf(s sc.Source) sc.Unit {
	z := v.z
	var e *entity
	switch s.Kind {
	case sc.KindPlayer:
		if p := z.players[uint32(s.ID)]; p != nil { // #nosec G115 -- a player's source ID is its char ID.
			e = p.entity
		}
	case sc.KindMonster:
		e = z.monsterHandles[s.ID]
	}
	if e == nil || !z.entities.has(e) {
		return nil
	}
	return z.unit(e)
}

// AreaChars is map_foreachinallarea over BL_CHAR: the players and
// monsters in the square around a unit, the dead included.
func (v scEnv) AreaChars(u sc.Unit, r int) []sc.Unit {
	z, e := v.z, u.(scUnit).e
	var out []sc.Unit
	for _, t := range z.findEntities(&query{area: areaRadius, x: e.x, y: e.y, radius: r,
		kinds: queryPlayers | queryMonsters, dead: true}) {
		out = append(out, z.unit(t))
	}
	return out
}

// statusEvent queues a status event for mod scripts. Events run after
// the script call that started or ended the status returns, not inside
// it:
//   - A handler that starts a status from status_start cannot loop
//     inside one call.
//   - rAthena has no such events, so the order matches no rAthena rule.
func (z *Zone) statusEvent(name string, u scUnit, e *sc.Entry, ms int64, reason string) {
	if z.scripts == nil {
		return
	}
	ev := &modscript.StatusEvent{Target: entityRef(u.Ref()), Status: int64(e.Type), Level: int64(e.Val1),
		Milliseconds: ms, Reason: reason, Map: scriptHost{z}.Map()}
	if e.Source.Kind != sc.KindNone {
		src := entityRef(e.Source)
		ev.Source = &src
	}
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: name, status: ev})
}

func entityRef(s sc.Source) modscript.Entity {
	return modscript.Entity{Kind: s.Kind.String(), ID: s.ID}
}

// Chance is the status_chance hook. A start that a script asked for has
// its hooks run already: their result comes through hookOverride.
func (v scEnv) Chance(t sc.Unit, src *sc.Source, typ, level, chance int) int {
	z := v.z
	if o := z.hookOverride; o.overrides(typ, t) && !o.chanceUsed {
		o.chanceUsed = true
		if o.chance >= 0 {
			return int(o.chance)
		}
		return chance
	}
	if z.scripts == nil || z.inScriptCall || !z.scripts.HasHooks("status_chance") {
		return chance
	}
	return int(min(max(z.scripts.RunStatusChance(z.statusHook(t, src, typ, level), int64(chance)), 0), 10000))
}

// Duration is the status_duration hook.
func (v scEnv) Duration(t sc.Unit, src *sc.Source, typ, level int, d int64) int64 {
	z := v.z
	if o := z.hookOverride; o.overrides(typ, t) && !o.durationUsed {
		o.durationUsed = true
		if o.duration >= 0 {
			return max(o.duration, 1)
		}
		return d
	}
	if z.scripts == nil || z.inScriptCall || !z.scripts.HasHooks("status_duration") {
		return d
	}
	return min(max(z.scripts.RunStatusDuration(z.statusHook(t, src, typ, level), d), 1), math.MaxInt32)
}

func (z *Zone) statusHook(t sc.Unit, src *sc.Source, typ, level int) modscript.StatusHook {
	h := modscript.StatusHook{Target: entityRef(t.Ref()), Status: int64(typ), Level: int64(level)}
	if src != nil {
		s := entityRef(*src)
		h.Source = &s
	}
	return h
}

// hookOverride holds what a script's own run of the status hooks gave,
// for the start it asked for.
type hookOverride struct {
	typ                      int
	target                   *entity
	chance, duration         int64
	chanceUsed, durationUsed bool
}

// overrides reports whether the override is for a start of a status on
// a target.
func (o *hookOverride) overrides(typ int, t sc.Unit) bool {
	u, ok := t.(scUnit)
	return o != nil && ok && o.typ == typ && o.target == u.e
}

// cant is what an entity's statuses keep it from doing.
func (e *entity) cant() sc.Cant {
	if e.sc == nil {
		return sc.Cant{}
	}
	c := e.sc.Cant
	// status_calc_state_sub's NoMoveCond: the caster of pre-renewal
	// Basilica cannot move.
	if !c.Move && e.basilicaCaster() {
		c.Move = true
	}
	// SC_HIDING's NoMoveCond in status_calc_state_sub: a hidden unit
	// cannot move, except a player with Tunnel Drive.
	if !c.Move && e.sc.Has(sc.SCHiding) && (e.player == nil || e.player.skillLevel(skill.IDTunnelDrive) <= 0) {
		c.Move = true
	}
	// The caster of Battle Chant cannot move.
	if !c.Move && e.gospelCaster() {
		c.Move = true
	}
	return c
}

// body is an entity's opt1: stun, freeze, stone or sleep, which stop it
// from acting.
func (e *entity) body() int {
	if e.sc == nil {
		return sc.Opt1None
	}
	return e.sc.Opt1
}

// actsBlocked reports whether an opt1 status stops the entity, as the
// checks of rAthena's mob AI and pc_useitem: every opt1 but Stone
// Curse's wait and Burning.
func (e *entity) actsBlocked() bool {
	b := e.body()
	return b != sc.Opt1None && b != sc.Opt1StoneWait && b != sc.Opt1Burning
}

// options are an entity's status options for the protocol, or nil for
// none.
func (e *entity) options() *wathenav1.Options {
	l := e.sc
	// A player's saved options (cart, falcon, Peco Peco) join the
	// options of its statuses, as rAthena keeps both in sc.option.
	saved := 0
	if e.player != nil {
		saved = e.player.record.Options
	}
	if (l == nil || (l.Opt1 == 0 && l.Opt2 == 0 && l.Opt3 == 0 && l.Option == 0)) && saved == 0 {
		return nil
	}
	var o1, o2, o3, opt int
	if l != nil {
		o1, o2, o3, opt = int(l.Opt1), int(l.Opt2), int(l.Opt3), int(l.Option)
	}
	return wathenav1.Options_builder{
		Body:   uint32(o1),          // #nosec G115 -- OPT1_ values are small.
		Health: uint32(o2),          // #nosec G115 -- bits.
		Effect: uint32(o3),          // #nosec G115 -- bits.
		Option: uint32(opt | saved), // #nosec G115 -- bits.
	}.Build()
}

// optionChange is the event that tells a viewer an entity's options
// changed, as clif_changeoption.
func optionChange(id uint32, e *entity) *wathenav1.Event {
	o := e.options()
	if o == nil {
		o = wathenav1.Options_builder{}.Build()
	}
	return wathenav1.Event_builder{OptionChange: wathenav1.OptionChange_builder{Id: id, Options: o}.Build()}.Build()
}

// damaged is the status part of status_damage for damage that is not
// passive: statuses that damage ends do, and timed bonuses that end on
// damage.
func (z *Zone) damaged(e *entity, hp int64, fromMonster bool) {
	if en := z.statusEngine(); en != nil && e.sc != nil {
		en.Damaged(z.unit(e), hp, fromMonster)
		z.gravitationDamaged(e)
	}
	if hp > 0 && !e.dead && e.hp > 0 {
		z.hpLowered(e)
	}
	if p := e.player; p != nil {
		z.endTimedBonuses(p, "damage")
	}
}

// statusesDied is status_change_clear for a unit that died: statuses
// without NoRemoveOnDead end, and the player's timed bonuses that end on
// death.
func (z *Zone) statusesDied(e *entity) {
	if en := z.statusEngine(); en != nil && e.sc != nil {
		en.Clear(z.unit(e), sc.ClearDeath, sc.ReasonDeath)
	}
	if p := e.player; p != nil {
		z.endTimedBonuses(p, "death")
	}
}

// dropStatuses forgets an entity's statuses without ending them, as
// when it leaves the zone: they go on in the save or the next zone.
func (z *Zone) dropStatuses(e *entity) {
	if e.sc == nil {
		return
	}
	e.sc = nil
	if p := e.player; p != nil {
		p.timed = nil
	}
}

// statusNames turns saved statuses into the engine's, and back.
func (z *Zone) savedStatuses(p *Player) []character.Status {
	en := z.statusEngine()
	if en == nil || p.entity.sc == nil {
		return nil
	}
	var out []character.Status
	for _, s := range en.Save(z.unit(p.entity)) {
		def := z.Statuses.Get(s.Type)
		if def == nil || def.Name == "" {
			continue
		}
		out = append(out, character.Status{Name: def.Name, Values: [4]int{s.Val1, s.Val2, s.Val3, s.Val4}, Tick: s.Tick, End: s.End})
	}
	return out
}

// restoreStatuses starts a player's saved statuses again, as
// chrif_load_scdata. A login fires status_start with the reason login;
// a map change starts them quietly, since they never ended.
func (z *Zone) restoreStatuses(p *Player, saved []character.Status, login bool) {
	en := z.statusEngine()
	if en == nil {
		return
	}
	z.quietStatuses = !login
	defer func() { z.quietStatuses = false }()
	u := z.unit(p.entity)
	for _, s := range saved {
		def := z.Statuses.ByName(s.Name)
		if def == nil {
			if z.Mods != nil && z.Mods.Log != nil {
				z.Mods.Log.Warn("saved status left out: no status of that name", "map", z.Map, "status", s.Name)
			}
			continue
		}
		bonuses := z.modStatusBonuses(def, int64(s.Values[0]))
		en.Load(u, sc.Saved{Type: def.ID, Val1: s.Values[0], Val2: s.Values[1], Val3: s.Values[2], Val4: s.Values[3], Tick: s.Tick, End: s.End}, bonuses)
	}
}

// modStatusBonuses runs the bonuses function of a mod status, for a
// start outside a script.
func (z *Zone) modStatusBonuses(def *sc.Def, level int64) []sc.Bonus {
	if def.Mod == "" || z.scripts == nil || z.inScriptCall {
		return nil
	}
	list, err := z.scripts.ModStatusBonuses(int64(def.ID), level)
	if err != nil {
		if z.Mods != nil && z.Mods.Log != nil {
			z.Mods.Log.Warn("status bonuses failed", "map", z.Map, "status", def.Name, "err", err)
		}
		return nil
	}
	return toStatusBonuses(list)
}

func toStatusBonuses(list []bonus.Bonus) []sc.Bonus {
	out := make([]sc.Bonus, 0, len(list))
	for i := range list {
		b := &list[i]
		x := sc.Bonus{Name: b.Name, Options: &b.Options}
		for _, a := range b.Args {
			x.Args = append(x.Args, a.Value)
		}
		out = append(out, x)
	}
	return out
}

// statusBonuses are a player's status bonuses for status.Calc: the
// scripts of its statuses and the bonuses of mod statuses.
func (z *Zone) statusBonuses(p *Player) []status.Bonus {
	l := p.entity.sc
	if l.Empty() || z.Statuses == nil {
		return nil
	}
	skill := func(id int) int { return p.char.Skills[id] }
	var out []status.Bonus
	for _, b := range l.ScriptBonuses(z.Statuses, skill) {
		out = append(out, status.Bonus{Name: b.Name, Args: b.Args, Options: b.Options})
	}
	return out
}

// advanceStatuses runs the status timers and timed bonuses that are due.
func (z *Zone) advanceStatuses() {
	if en := z.statusChanges; en != nil {
		en.Advance(z.now)
	}
	for _, p := range z.players {
		if len(p.timed) > 0 {
			z.expireTimedBonuses(p)
		}
	}
}

// timedBonus is one of a player's timed bonuses: rAthena's
// s_bonus_script_entry.
type timedBonus struct {
	key     string
	end     int64
	bonuses []status.Bonus
	// data is the entries as JSON, for saving.
	data   string
	endsOn map[string]bool
	icon   int
	kind   string
}

// addTimedBonuses is pc_bonus_script_add and pc_bonus_script: the
// bonuses start, with their icon, and stats are computed again at once.
// When the player has timed bonuses with the same key, t.Repeat decides,
// as pc_bonus_script_add's flags do for the same script text:
//   - RepeatKeep, rAthena's default: the new ones are ignored.
//   - RepeatExtend, BSF_FORCE_REPLACE: the first copy keeps its values
//     and takes the new end when it is later. Otherwise the new ones are
//     ignored.
//   - RepeatAdd, BSF_FORCE_DUPLICATE: a second copy starts.
//   - RepeatReplace: every copy ends, and the new one starts.
//
// It reports whether the bonuses were added or extended.
func (z *Zone) addTimedBonuses(p *Player, t modscript.TimedBonuses) (bool, error) {
	ends := map[string]bool{}
	for _, e := range t.EndsOn {
		if !slices.Contains(sc.EndsOnNames, e) {
			return false, fmt.Errorf("player:add_timed_bonuses: ends_on has no %q", e)
		}
		ends[e] = true
	}
	data, err := json.Marshal(t.Data)
	if err != nil {
		return false, err
	}
	end := z.now + t.Milliseconds
	// pc_bonus_script_add looks at the first copy only.
	if i := slices.IndexFunc(p.timed, func(x *timedBonus) bool { return x.key == t.Key }); i >= 0 {
		switch t.Repeat {
		case modscript.RepeatAdd:
		case modscript.RepeatReplace:
			// Stats are computed once, after the new copy starts.
			z.holdStats++
			z.removeTimedBonuses(p, func(x *timedBonus) bool { return x.key == t.Key })
			z.holdStats--
		case modscript.RepeatExtend:
			x := p.timed[i]
			if x.end >= end {
				return false, nil
			}
			x.end = end
			if x.icon >= 0 {
				scEnv{z}.Icon(z.unit(p.entity), x.icon, true, t.Milliseconds, 1, 0, 0)
			}
			return true, nil
		default:
			return false, nil
		}
	}
	tb := &timedBonus{key: t.Key, end: end, data: string(data), endsOn: ends, icon: int(t.Icon), kind: t.Kind}
	for i := range t.Bonuses {
		tb.bonuses = append(tb.bonuses, status.FromBonus(&t.Bonuses[i]))
	}
	p.timed = append(p.timed, tb)
	if tb.icon >= 0 {
		scEnv{z}.Icon(z.unit(p.entity), tb.icon, true, t.Milliseconds, 1, 0, 0)
	}
	z.statsChanged(p)
	return true, nil
}

// removeTimedBonuses ends the timed bonuses that match, with their
// icons, and computes stats again. An icon stays while another timed
// bonus shows it. It reports whether any ended.
func (z *Zone) removeTimedBonuses(p *Player, match func(*timedBonus) bool) bool {
	kept := p.timed[:0]
	var icons []int
	for _, t := range p.timed {
		if !match(t) {
			kept = append(kept, t)
			continue
		}
		icons = append(icons, t.icon)
	}
	clear(p.timed[len(kept):])
	p.timed = kept
	if len(icons) == 0 {
		return false
	}
	slices.Sort(icons)
	for _, icon := range slices.Compact(icons) {
		if icon >= 0 && !slices.ContainsFunc(kept, func(x *timedBonus) bool { return x.icon == icon }) {
			scEnv{z}.Icon(z.unit(p.entity), icon, false, 0, 0, 0, 0)
		}
	}
	z.statsChanged(p)
	return true
}

// endTimedBonuses is pc_bonus_script_clear for one removal: the timed
// bonuses whose ends_on names it end.
func (z *Zone) endTimedBonuses(p *Player, what string) {
	if len(p.timed) == 0 {
		return
	}
	z.removeTimedBonuses(p, func(t *timedBonus) bool { return t.endsOn[what] })
}

// expireTimedBonuses is pc_bonus_script_timer: timed bonuses whose time
// ran out end.
func (z *Zone) expireTimedBonuses(p *Player) {
	z.removeTimedBonuses(p, func(t *timedBonus) bool { return t.end <= z.now })
}

// timedBonusList is the bonuses of a player's timed bonuses, in the
// order they started, for status.Calc.
func (p *Player) timedBonusList() []status.Bonus {
	var out []status.Bonus
	for _, t := range p.timed {
		out = append(out, t.bonuses...)
	}
	return out
}

// savedTimedBonuses are a player's timed bonuses to save. Those that end
// on logout are not saved.
func (z *Zone) savedTimedBonuses(p *Player) []character.TimedBonus {
	var out []character.TimedBonus
	for _, t := range p.timed {
		if t.endsOn["logout"] {
			continue
		}
		var ends []string
		for _, n := range sc.EndsOnNames {
			if t.endsOn[n] {
				ends = append(ends, n)
			}
		}
		out = append(out, character.TimedBonus{Key: t.key, Remaining: max(t.end-z.now, 0), Bonuses: t.data,
			EndsOn: ends, Icon: t.icon, Kind: t.kind})
	}
	return out
}

// restoreTimedBonuses gives back a player's saved timed bonuses. One
// whose bonuses no longer read, such as a bonus a mod removed, is left
// out.
func (z *Zone) restoreTimedBonuses(p *Player, saved []character.TimedBonus) {
	if len(saved) == 0 {
		// bonusNames copies every constant module, which a login without
		// timed bonuses does not need.
		return
	}
	names := z.bonusNames()
	for _, s := range saved {
		var entries []any
		if err := json.Unmarshal([]byte(s.Bonuses), &entries); err != nil {
			continue
		}
		// Each saved copy comes back, even several with one key.
		t := modscript.TimedBonuses{Key: s.Key, Milliseconds: max(s.Remaining, 1), Data: entries, EndsOn: s.EndsOn,
			Repeat: modscript.RepeatAdd, Icon: int64(s.Icon), Kind: s.Kind}
		ok := true
		for _, e := range entries {
			b, err := bonus.FromData(e, names)
			if err != nil {
				ok = false
				break
			}
			t.Bonuses = append(t.Bonuses, *b)
		}
		if !ok {
			if z.Mods != nil && z.Mods.Log != nil {
				z.Mods.Log.Warn("saved timed bonuses left out: a bonus no longer reads", "map", z.Map, "key", s.Key)
			}
			continue
		}
		_, _ = z.addTimedBonuses(p, t)
	}
}

// bonusNames finds the constants that saved bonus entries may name.
func (z *Zone) bonusNames() bonus.Names {
	t := &bonus.Tables{Constants: map[string]map[string]int64{}}
	if z.Mods != nil && z.Mods.Constants != nil {
		for name, m := range z.Mods.Constants.Modules() {
			t.Constants[name] = m.Values
		}
	}
	return t
}

// mapWarpStatuses is the status part of pc_setpos for a change of map:
// statuses with RemoveOnMapWarp or RemoveOnChangeMap end.
func (z *Zone) mapWarpStatuses(p *Player) {
	en := z.statusEngine()
	e := p.entity
	if en == nil || e.sc.Empty() {
		return
	}
	for _, x := range e.sc.Sorted() {
		if def := z.Statuses.Get(x.Type); def != nil && (def.Flags.Has(sc.FlagRemoveOnMapWarp) || def.Flags.Has(sc.FlagRemoveOnChangeMap)) {
			en.End(z.unit(e), x.Type, sc.ReasonRemoved)
		}
	}
}

// dirX and dirY are rAthena's dirx and diry: the cell offsets of the 8
// directions, from north, counterclockwise.
var (
	dirX = [8]int{0, -1, -1, -1, 0, 1, 1, 1}
	dirY = [8]int{1, 1, 0, -1, -1, -1, 0, 1}
)

// randomDir is map_random_dir: Confusion sends a walk to a random cell
// at the same distance, along a diagonal. It keeps the cell when 100
// tries find no reachable one.
func (z *Zone) randomDir(e *entity, x, y int) (int, int) {
	xi, yi := x-e.x, y-e.y
	dist2 := xi*xi + yi*yi
	dist := max(int(math.Sqrt(float64(float32(dist2)))), 1)
	for range 100 {
		j := 1 + 2*z.rand.IntN(4)
		segment := 1 + z.rand.IntN(dist)
		nx := e.x + segment*dirX[j]
		segment = int(math.Sqrt(float64(float32(dist2 - segment*segment))))
		ny := e.y + segment*dirY[j]
		if !z.grid.NoPass(nx, ny) {
			if _, ok := path.Search(z.grid, e.x, e.y, nx, ny, true); ok {
				return nx, ny
			}
		}
	}
	return x, y
}

// addEffDelay is the delay of statuses that equipment causes, as
// skill_additional_effect starts them.
const addEffDelay = 100

// additionalEffect is skill_additional_effect for a normal attack that
// landed: the attacker's cause_status bonuses roll on the target, or on
// the attacker with target = "self". Normal attacks are physical.
func (z *Zone) additionalEffect(src, target *entity, long bool) {
	p := src.player
	en := z.statusEngine()
	if p == nil || target.dead {
		return
	}
	if en == nil {
		z.snatcher(src, target)
		return
	}
	su, tu := z.unit(src), z.unit(target)
	for _, a := range p.bonuses.AddEff {
		rate := a.Rate
		if long {
			rate += a.ArrowRate
		}
		if rate == 0 || !addEffMatches(a.Flag, long) {
			continue
		}
		s := sc.Start{Type: a.Status, Rate: rate, Val1: 7, Duration: int64(a.Duration), Delay: addEffDelay,
			Reason: sc.ReasonBonus, Source: su.Ref()}
		if a.Flag&status.ATFTarget != 0 {
			en.Start(su, tu, s)
		}
		if a.Flag&status.ATFSelf != 0 {
			en.Start(su, su, s)
		}
	}
	// The normal attack's own effects (case 0 of the skill switch).
	if !target.dead && !src.dead {
		z.autoBlitzBeat(src, target)
	}
	// The normal attack's own effects: Snatcher's steal.
	z.snatcher(src, target)
}

// counterEffect is skill_counter_additional_effect for a normal attack
// that landed on a player: the player's cause_status bonuses with on =
// "hit_taken" roll on the attacker, or on the player with target =
// "self".
func (z *Zone) counterEffect(src, target *entity, long bool) {
	p := target.player
	en := z.statusEngine()
	if p == nil || en == nil {
		return
	}
	su, tu := z.unit(src), z.unit(target)
	for _, a := range p.bonuses.AddEffWhenHit {
		rate := a.Rate
		if long {
			rate += a.ArrowRate
		}
		if rate == 0 || !addEffMatches(a.Flag, long) {
			continue
		}
		s := sc.Start{Type: a.Status, Rate: rate, Val1: 7, Duration: int64(a.Duration), Delay: addEffDelay,
			Reason: sc.ReasonBonus, Source: tu.Ref()}
		if a.Flag&status.ATFTarget != 0 && src != target {
			en.Start(su, su, s)
		}
		if a.Flag&status.ATFSelf != 0 && !target.dead {
			en.Start(su, tu, s)
		}
	}
}

// addEffMatches checks an entry's ATF_ flags against a normal attack: a
// weapon attack, ranged or not.
func addEffMatches(flag int, long bool) bool {
	const kinds = status.ATFWeapon | status.ATFMagic | status.ATFMisc
	if flag&kinds != kinds && flag&status.ATFWeapon == 0 {
		return false
	}
	if flag&(status.ATFLong|status.ATFShort) != status.ATFLong|status.ATFShort {
		if flag&status.ATFLong != 0 && !long || flag&status.ATFShort != 0 && long {
			return false
		}
	}
	return true
}

// The status calls of scripts (proposal 0002, review decision 19).

// entityOf finds the entity of a script handle.
func (h scriptHost) entityOf(ref modscript.Entity) (*entity, error) {
	z := h.z
	switch ref.Kind {
	case "player":
		p, err := h.player(ref.ID)
		if err != nil {
			return nil, err
		}
		return p.entity, nil
	case "monster":
		if e := z.monsterHandles[ref.ID]; e != nil && z.entities.has(e) && !e.dead {
			return e, nil
		}
		if isRemoteMonster(ref.ID) {
			return nil, errors.New("the monster is on another map")
		}
		return nil, fmt.Errorf("the monster is gone from this map")
	case "npc":
		if e := z.npcHandles[ref.ID]; e != nil && z.entities.has(e) {
			return e, nil
		}
		return nil, fmt.Errorf("the NPC is gone from this map")
	}
	return nil, fmt.Errorf("not an entity: %s", ref.Kind)
}

// statusStart turns a script's start into the engine's: the target is
// the source when the script names none, as rAthena's sc_start.
func (h scriptHost) statusStart(target modscript.Entity, s modscript.StatusStart) (*sc.Engine, scUnit, scUnit, sc.Start, error) {
	z := h.z
	en := z.statusEngine()
	if en == nil {
		return nil, scUnit{}, scUnit{}, sc.Start{}, fmt.Errorf("%s:start_status: the server has no status data", target.Kind)
	}
	e, err := h.entityOf(target)
	if err != nil {
		return nil, scUnit{}, scUnit{}, sc.Start{}, err
	}
	def := z.Statuses.Get(int(s.Status))
	if def == nil {
		return nil, scUnit{}, scUnit{}, sc.Start{}, fmt.Errorf("%s:start_status: there is no status %d", target.Kind, s.Status)
	}
	tu := z.unit(e)
	su := tu
	start := sc.Start{Type: def.ID, Rate: int(s.Chance), Val1: int(s.Level), Val2: int(s.Values[0]), Val3: int(s.Values[1]), Val4: int(s.Values[2]),
		Duration: s.Milliseconds, Reason: sc.Reason(s.Reason)}
	if s.Source != nil {
		src, err := h.entityOf(*s.Source)
		if err != nil {
			return nil, scUnit{}, scUnit{}, sc.Start{}, fmt.Errorf("%s:start_status: source: %w", target.Kind, err)
		}
		su = z.unit(src)
		start.Source = su.Ref()
	}
	switch s.Milliseconds {
	case modscript.Forever:
		start.Duration = sc.InfiniteTick
	case 0:
		// sc_start with no duration takes the skill's for the level.
		if start.Val1 > 0 {
			start.Duration = int64(def.Duration(start.Val1))
		}
	}
	// buildin_sc_start with potion_flag and potion_target: a thrown
	// potion's status goes on the target, from the target, for half its
	// time, with val4 1.
	if t := z.potionStatusTarget(); t != nil {
		tu, su = z.unit(t), z.unit(t)
		start.Source = sc.Source{}
		if start.Duration != sc.InfiniteTick {
			start.Duration /= 2
		}
		start.Val4 = 1
	}
	if s.CannotBeAvoided {
		start.Flag |= sc.NoAvoid
	}
	if s.KeepDuration {
		start.Flag |= sc.NoTickDef
	}
	if s.KeepChance {
		start.Flag |= sc.NoRateDef
	}
	if s.HideIcon {
		start.Flag |= sc.NoIcon
	}
	if def.Mod != "" {
		start.Bonuses = toStatusBonuses(s.Bonuses)
	}
	return en, su, tu, start, nil
}

// StartStatus is entity:start_status.
func (h scriptHost) StartStatus(target modscript.Entity, s modscript.StatusStart) (bool, error) {
	z := h.z
	en, su, tu, start, err := h.statusStart(target, s)
	if err != nil {
		return false, err
	}
	z.inScriptCall = true
	z.hookOverride = &hookOverride{typ: start.Type, target: tu.e, chance: s.HookChance, duration: s.HookDuration}
	defer func() { z.inScriptCall, z.hookOverride = false, nil }()
	return en.Start(su, tu, start), nil
}

// PreviewStatus gives the chance and duration that the hooks of a
// script's start see.
func (h scriptHost) PreviewStatus(target modscript.Entity, s modscript.StatusStart) (modscript.StatusPreview, bool, error) {
	en, su, tu, start, err := h.statusStart(target, s)
	if err != nil {
		return modscript.StatusPreview{}, false, err
	}
	rate, d, ok := en.Preview(su, tu, start)
	return modscript.StatusPreview{Chance: int64(rate), Milliseconds: d}, ok, nil
}

// EndStatus is entity:end_status, as sc_end.
func (h scriptHost) EndStatus(target modscript.Entity, status int64) (bool, error) {
	z := h.z
	en := z.statusEngine()
	if en == nil {
		return false, nil
	}
	e, err := h.entityOf(target)
	if err != nil {
		return false, err
	}
	// buildin_sc_end with potion_flag ends the status on potion_target.
	if t := z.potionStatusTarget(); t != nil {
		e = t
	}
	// buildin_sc_end refuses a status that neither clearing nor a
	// forced end may remove.
	if def := z.Statuses.Get(int(status)); def != nil && def.Flags.Has(sc.FlagNoClearBuff) && def.Flags.Has(sc.FlagNoForcedEnd) {
		return false, nil
	}
	z.inScriptCall = true
	defer func() { z.inScriptCall = false }()
	return en.End(z.unit(e), int(status), sc.ReasonRemoved), nil
}

// Statuses lists an entity's active statuses, for has_status, get_status
// and get_statuses.
func (h scriptHost) Statuses(target modscript.Entity, status int64) ([]modscript.StatusInfo, error) {
	z := h.z
	e, err := h.entityOf(target)
	if err != nil {
		return nil, err
	}
	var out []modscript.StatusInfo
	for _, x := range e.sc.Sorted() {
		if status >= 0 && int64(x.Type) != status {
			continue
		}
		info := modscript.StatusInfo{Status: int64(x.Type), Level: int64(x.Val1),
			Values: [3]int64{int64(x.Val2), int64(x.Val3), int64(x.Val4)}, Remaining: x.Remaining(z.now)}
		if x.Source.Kind != sc.KindNone {
			src := entityRef(x.Source)
			info.Source = &src
		}
		out = append(out, info)
	}
	return out, nil
}

// DefineStatus is statuses.define.
func (h scriptHost) DefineStatus(d modscript.StatusDefinition) (int64, error) {
	if h.z.Statuses == nil {
		return 0, fmt.Errorf("statuses.define: the server has no status data")
	}
	def, err := h.z.Statuses.Define(sc.ModSpec{Mod: d.Mod, Name: d.Name, Icon: int(d.Icon), Debuff: d.Debuff, EndsOn: d.EndsOn, TickMs: d.TickMs})
	if err != nil {
		return 0, err
	}
	return int64(def.ID), nil
}

// AddTimedBonuses is player:add_timed_bonuses.
func (h scriptHost) AddTimedBonuses(id int64, t modscript.TimedBonuses) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.addTimedBonuses(p, t)
}
