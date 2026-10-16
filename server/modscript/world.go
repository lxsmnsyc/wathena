package modscript

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/schedule"
)

// World calls of proposal 0002, review decision 16: wa.schedules,
// wa.hooks, wa.spawning, the death event, and event contexts.
//   - Schedules run on the server clock, in the server's time zone
//     (Options.Clock and Options.Location). Conditions of schedules.when
//     run once a minute.
//   - Hooks change a result in a chain ordered by priority. They only
//     compute: calls that change the game raise an error in them.
//   - Spawned monsters belong to the context that spawned them, and go
//     when it ends.
//   - An event context is one run of a world event (data/events.yml).
//     Everything it owns goes when the run ends.

// SpawnHost places monsters for scripts. A Host without it raises an
// error on spawning.monsters.
type SpawnHost interface {
	// SpawnMonsters places monsters that a context owns, and returns
	// their monster handles.
	SpawnMonsters(owner int64, s SpawnSpec) ([]int64, error)
	// Despawn removes every monster a context owns, alive or waiting to
	// come back.
	Despawn(owner int64)
}

// SpawnSpec is what spawning.monsters asks for.
type SpawnSpec struct {
	Monster uint32
	Amount  int
	// HasCell is set when the script gave x and y. Area spreads the
	// monsters within that many cells of it.
	HasCell bool
	X, Y    int
	Area    int
	// Respawn makes dead monsters come back while the context lives,
	// after RespawnMs, or DefaultRespawnMs when it is 0.
	Respawn   bool
	RespawnMs int
}

// Limits of the world calls.
const (
	// MaxSpawn is the most monsters one spawning.monsters call places.
	MaxSpawn = 1000
	// MaxSpawnArea is the largest area of spawning.monsters, in cells.
	MaxSpawnArea = 512
	// MaxLasting is the longest lasting_minutes of schedules.when.
	MaxLasting = 7 * 24 * 60
	// DefaultRespawnMs is the respawn time of a spawn line that gives
	// none in rAthena (npc_parse_mob), and of spawning.monsters without
	// respawn_milliseconds. Monster data has no respawn time of its own.
	DefaultRespawnMs = 5000
	// MinRespawnMs is mob_respawn_time: rAthena refuses a shorter delay,
	// and a monster never comes back faster (mob_setdelayspawn).
	MinRespawnMs = 1000
	// MaxRespawnMs is the longest delay a spawn line may have.
	MaxRespawnMs = 0xfffffff
)

type hook struct {
	id       int64
	name     string
	ctx      *Context
	fn       *script.Ref
	priority int64
	removed  bool
}

// Hooks lists every hook name and the context kinds that can add it.
var Hooks = map[string][]string{
	"drop_rate":       {KindMod, KindNPC, KindEvent},
	"experience":      {KindMod, KindNPC, KindEvent},
	"status_chance":   {KindMod, KindNPC, KindEvent},
	"status_duration": {KindMod, KindNPC, KindEvent},
	// Skills (review decision 20).
	"skill_damage": {KindMod, KindNPC, KindEvent},
	"cast_time":    {KindMod, KindNPC, KindEvent},
	"skill_cost":   {KindMod, KindNPC, KindEvent},
	"cast_allowed": {KindMod, KindNPC, KindEvent},
	// Monster AI (review decision 22).
	"monster_target": {KindMod, KindNPC, KindEvent},
	// Rolled item values (review decision 23).
	"item_roll": {KindMod, KindNPC, KindEvent},
	// Chat (review decision 26). It runs after the server's chat filter.
	"chat": {KindMod, KindNPC, KindEvent},
	// Trade (review decision 29). It runs before a trade completes.
	"trade": {KindMod, KindNPC, KindEvent},
	// Vending, buying stores and mail (review decision 30). They run
	// before a sale or a mail.
	"vend":              {KindMod, KindNPC, KindEvent},
	"buying_store_sale": {KindMod, KindNPC, KindEvent},
	"mail":              {KindMod, KindNPC, KindEvent},
	// Spawns (review decision 44). It runs when `wactl world new` makes
	// a procedural world, and in the test worlds of wactl mod test.
	"spawn_table": {KindMod},
}

// hookPermissions are the permissions of the hooks that do not need
// rules.override.
var hookPermissions = map[string]string{"spawn_table": "spawns.modify"}

type scheduled struct {
	id   int64
	ctx  *Context
	fn   *script.Ref
	spec schedule.Spec
	next time.Time
	// cond is set for schedules.when. heldSince is when it first held,
	// fired is set once the handler ran for this holding, and polled is
	// the minute it last ran.
	cond      *script.Ref
	lasting   time.Duration
	heldSince time.Time
	fired     bool
	polled    time.Time
	canceled  bool
}

// deathFilter is the options of a death handler.
type deathFilter struct {
	victims map[string]bool
	kind    string
	monster int64
	mapName string
}

// Death is a unit's death, for the death event. Victim and Killer are
// handle IDs of their kinds: "player" or "monster". Killer is 0 when
// nothing killed the unit.
type Death struct {
	Victim     int64
	VictimKind string
	// Monster is the victim's monster ID, for a monster.
	Monster    int64
	Killer     int64
	KillerKind string
	X, Y       int
	// Skill is the skill of the last hit, or 0 for a normal attack.
	Skill int64
}

// FireDeath runs the death handlers whose options match.
func (r *Runtime) FireDeath(d Death) {
	list := r.handlers["death"]
	if len(list) == 0 {
		return
	}
	key := d.VictimKind + ":" + fmt.Sprint(d.Victim)
	fields := []script.Field{
		script.KV("victim", script.Int(d.Victim)), script.KV("victim_kind", script.String(d.VictimKind)),
		script.KV("map", script.String(r.host.Map())), script.KV("x", script.Int(int64(d.X))), script.KV("y", script.Int(int64(d.Y))),
	}
	if d.Killer != 0 {
		fields = append(fields, script.KV("killer", script.Int(d.Killer)), script.KV("killer_kind", script.String(d.KillerKind)))
	}
	if d.Skill != 0 {
		fields = append(fields, script.KV("skill", script.Int(d.Skill)))
	}
	obj := script.NewTable(fields...)
	for _, h := range slices.Clone(list) {
		if h.removed || h.ctx.ended || r.off(h.ctx) {
			continue
		}
		if f := h.death; f != nil {
			if f.victims != nil && !f.victims[key] || f.kind != "" && f.kind != d.VictimKind ||
				f.monster != 0 && (d.VictimKind != "monster" || f.monster != d.Monster) ||
				f.mapName != "" && f.mapName != r.host.Map() {
				continue
			}
		}
		if err := r.run(h.ctx, r.fns.invoke.Value(), h.fn.Value(), script.String("death"), obj); err != nil {
			r.failed(h.ctx, "event death", err)
		}
	}
}

// readDeathFilter reads the options of events.on("death", ...). prelude
// turns handles into "<kind>:<id>" keys.
func readDeathFilter(v script.Value) (*deathFilter, error) {
	t, ok := v.AsTable()
	if !ok {
		return nil, errors.New(`events.on("death"): the options must be a table`)
	}
	f := &deathFilter{}
	for _, field := range t.Fields {
		k, _ := field.Key.AsString()
		switch k {
		case "victims":
			list, ok := field.Value.AsTable()
			if !ok {
				return nil, errors.New(`events.on("death"): victims must be a list of handles`)
			}
			f.victims = map[string]bool{}
			for _, e := range list.Fields {
				s, ok := e.Value.AsString()
				if !ok {
					return nil, errors.New(`events.on("death"): victims must be a list of player or monster handles`)
				}
				f.victims[s] = true
			}
		case "victim_kind":
			s, _ := field.Value.AsString()
			if s != "player" && s != "monster" {
				return nil, errors.New(`events.on("death"): victim_kind is "player" or "monster"`)
			}
			f.kind = s
		case "monster":
			n, ok := field.Value.AsInt()
			if !ok || n <= 0 {
				return nil, errors.New(`events.on("death"): monster must be a monster from monsters.id`)
			}
			f.monster = n
		case "map":
			s, ok := field.Value.AsString()
			if !ok || s == "" {
				return nil, errors.New(`events.on("death"): map must be a map name, such as prontera`)
			}
			f.mapName = s
		default:
			return nil, fmt.Errorf(`events.on("death"): %q is not an option; the options are victim, victims, victim_kind, monster and map`, k)
		}
	}
	return f, nil
}

// hooksOn is hooks.on: the name, the function, and the priority.
func (r *Runtime) hooksOn(c *script.Call) error {
	ctx, err := r.needContext("hooks.on")
	if err != nil {
		return err
	}
	if err := r.mutating("hooks.on"); err != nil {
		return err
	}
	name := c.String(0)
	what, permission := "hooks.on", "rules.override"
	if p, ok := hookPermissions[name]; ok {
		what, permission = `hooks.on("`+name+`")`, p
	}
	if err := r.need(what, permission); err != nil {
		return err
	}
	kinds, ok := Hooks[name]
	if !ok {
		return fmt.Errorf("hooks.on: there is no hook %q; the hooks are %s", name, strings.Join(sortedKeys(Hooks), ", "))
	}
	if !slices.Contains(kinds, ctx.Kind) {
		return fmt.Errorf("hooks.on: %q is a hook of %s contexts, and this is a %s context", name, strings.Join(kinds, ", "), ctx.Kind)
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	ref, err := c.State().Keep(c.Arg(1))
	if err != nil {
		return err
	}
	h := &hook{id: r.newID(), name: name, ctx: ctx, fn: ref, priority: c.Int(2)}
	ctx.hooks = append(ctx.hooks, h)
	list := append(r.hooks[name], h)
	// Higher priority runs first. Equal priorities keep the order of
	// registration.
	sort.SliceStable(list, func(i, j int) bool { return list[i].priority > list[j].priority })
	r.hooks[name] = list
	r.hookByID[h.id] = h
	c.Return(script.Int(h.id))
	return nil
}

func (r *Runtime) hooksRemove(c *script.Call) error {
	h := r.hookByID[c.Int(0)]
	if h == nil {
		return nil
	}
	r.removeHook(h)
	return nil
}

func (r *Runtime) removeHook(h *hook) {
	if h.removed {
		return
	}
	h.removed = true
	h.fn.Release()
	delete(r.hookByID, h.id)
	if i := slices.Index(r.hooks[h.name], h); i >= 0 {
		r.hooks[h.name] = slices.Delete(r.hooks[h.name], i, i+1)
	}
	if i := slices.Index(h.ctx.hooks, h); i >= 0 {
		h.ctx.hooks = slices.Delete(h.ctx.hooks, i, i+1)
	}
}

// HasHooks reports whether any context added a hook of a name.
func (r *Runtime) HasHooks(name string) bool { return len(r.hooks[name]) > 0 }

// DropRate is the object of the drop_rate hook. Player is the killer's
// player handle, or 0. Monster is the dead monster's handle. Rate is
// rAthena's rate, per 10,000.
type DropRate struct {
	Player, Monster int64
	Item            uint32
	Rate            int
}

// Experience is the object of the experience hook: the exp one player
// gets for a kill, after rAthena's shares and penalty.
type Experience struct {
	Player, Monster int64
	Base, Job       int64
	// Party is the player's party ID, or 0. ShareBase and ShareJob are
	// the player's own share before rAthena's party split.
	Party               int64
	ShareBase, ShareJob int64
	// Guild is the player's guild ID, or 0, and GuildTax the share of
	// Base that goes to the guild (review decision 28).
	Guild, GuildTax int64
}

// RunDropRate runs the drop_rate hooks and returns the rate they give,
// from 0 to 10,000. Without hooks, or when the chain fails, it is d.Rate.
func (r *Runtime) RunDropRate(d DropRate) int {
	fields := []script.Field{
		script.KV("monster", script.Int(d.Monster)), script.KV("item", script.Int(int64(d.Item))),
		script.KV("map", script.String(r.host.Map())), script.KV("rate", script.Int(int64(d.Rate))),
	}
	if d.Player != 0 {
		fields = append(fields, script.KV("player", script.Int(d.Player)))
	}
	vals, ok := r.runHooks("drop_rate", fields)
	if !ok {
		return d.Rate
	}
	n, _ := vals[0].AsInt()
	return int(min(max(n, 0), 10000))
}

// RunExperience runs the experience hooks and returns the base and job
// exp they give, from 0. Without hooks, or when the chain fails, it is
// e.Base and e.Job.
func (r *Runtime) RunExperience(e Experience) (int64, int64) {
	fields := []script.Field{
		script.KV("player", script.Int(e.Player)), script.KV("monster", script.Int(e.Monster)),
		script.KV("base", script.Int(e.Base)), script.KV("job", script.Int(e.Job)),
		script.KV("share", script.NewTable(script.KV("base", script.Int(e.ShareBase)), script.KV("job", script.Int(e.ShareJob)))),
	}
	if e.Party != 0 {
		fields = append(fields, script.KV("party", script.Int(e.Party)))
	}
	if e.Guild != 0 {
		fields = append(fields, script.KV("guild", script.Int(e.Guild)))
	}
	fields = append(fields, script.KV("guild_tax", script.Int(e.GuildTax)))
	vals, ok := r.runHooks("experience", fields)
	if !ok || len(vals) < 2 {
		return e.Base, e.Job
	}
	b, _ := vals[0].AsInt()
	j, _ := vals[1].AsInt()
	return max(b, 0), max(j, 0)
}

// runHooks runs a hook chain. ok is false when there are no hooks, or
// when the chain stopped, such as at a CPU limit.
func (r *Runtime) runHooks(name string, fields []script.Field) ([]script.Value, bool) {
	var chain []*hook
	for _, h := range r.hooks[name] {
		if !h.removed && !h.ctx.ended && !h.ctx.retiring && !r.off(h.ctx) {
			chain = append(chain, h)
		}
	}
	if len(chain) == 0 {
		return nil, false
	}
	args := []script.Value{script.String(name), script.NewTable(fields...), script.Int(int64(len(chain)))}
	for _, h := range chain {
		args = append(args, h.fn.Value())
	}
	prev, prevComputing, prevChain := r.current, r.computing, r.chain
	r.chain, r.computing = chain, true
	vals, err := r.s.Call(r.fns.runHooks.Value(), args...)
	r.current, r.computing, r.chain = prev, prevComputing, prevChain
	if err != nil {
		r.afterError()
		r.failed(chain[0].ctx, "hook "+name, err)
		return nil, false
	}
	return vals, len(vals) > 0
}

// runHookNested runs a hook chain of one result as a nested call when a
// script's call runs (internal/script, "Nested calls"), and as a plain
// call otherwise. The item_roll hooks need it: a script's give_item or
// reroll makes a copy inside its own call.
func (r *Runtime) runHookNested(name string, fields []script.Field) (script.Value, bool) {
	var chain []*hook
	for _, h := range r.hooks[name] {
		if !h.removed && !h.ctx.ended && !h.ctx.retiring && !r.off(h.ctx) {
			chain = append(chain, h)
		}
	}
	if len(chain) == 0 {
		return script.Nil, false
	}
	args := []script.Value{script.String(name), script.NewTable(fields...), script.Int(int64(len(chain)))}
	for _, h := range chain {
		args = append(args, h.fn.Value())
	}
	prev, prevComputing, prevChain := r.current, r.computing, r.chain
	r.chain, r.computing = chain, true
	v, err := r.s.Call1Nested(r.fns.runHooks.Value(), args...)
	r.current, r.computing, r.chain = prev, prevComputing, prevChain
	if err != nil {
		r.afterError()
		r.failed(chain[0].ctx, "hook "+name, err)
		return script.Nil, false
	}
	return v, true
}

// hookEnter sets the running context to the context of hook i, from 1,
// of the running chain.
func (r *Runtime) hookEnter(c *script.Call) error {
	i := c.Int(0)
	if i < 1 || int(i) > len(r.chain) {
		return errors.New("no hook chain runs")
	}
	r.current = r.chain[i-1].ctx
	return nil
}

// hookFailed logs a hook that failed. The chain goes on without its
// result.
func (r *Runtime) hookFailed(c *script.Call) error {
	i := c.Int(0)
	if i < 1 || int(i) > len(r.chain) {
		return nil
	}
	h := r.chain[i-1]
	msg, _ := c.Arg(1).AsString()
	r.failed(h.ctx, "hook "+h.name, errors.New(msg))
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// schedulesAdd is schedules.daily, weekly and hourly: the kind, the
// handler, then the kind's arguments.
func (r *Runtime) schedulesAdd(c *script.Call) error {
	kind := c.String(0)
	what := "schedules." + kind
	ctx, err := r.needContext(what)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if r.opts.Clock == nil {
		return fmt.Errorf("%s: schedules do not run here", what)
	}
	var s schedule.Spec
	switch kind {
	case "daily":
		s.Kind = schedule.Daily
		h, m, err := schedule.ParseClock(c.String(2))
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		s.Hour, s.Minute = h, m
	case "weekly":
		s.Kind = schedule.Weekly
		d, err := schedule.ParseWeekday(c.String(2))
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		h, m, err := schedule.ParseClock(c.String(3))
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		s.Weekday, s.Hour, s.Minute = d, h, m
	case "hourly":
		s.Kind = schedule.Hourly
		m, ok := c.Arg(2).AsInt()
		if !ok || m < 0 || m > 59 {
			return fmt.Errorf("%s: the minute must be 0 to 59", what)
		}
		s.Minute = int(m)
	default:
		return fmt.Errorf("schedules: no kind %q", kind)
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	ref, err := c.State().Keep(c.Arg(1))
	if err != nil {
		return err
	}
	sc := &scheduled{id: r.newID(), ctx: ctx, fn: ref, spec: s, next: schedule.Next(s, r.opts.Clock(), r.location())}
	r.addSchedule(sc)
	c.Return(script.Int(sc.id))
	return nil
}

// schedulesWhen is schedules.when: the condition, the handler, and the
// minutes it must hold.
func (r *Runtime) schedulesWhen(c *script.Call) error {
	const what = "schedules.when"
	ctx, err := r.needContext(what)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if r.opts.Clock == nil {
		return fmt.Errorf("%s: schedules do not run here", what)
	}
	lasting := c.Int(2)
	if lasting < 0 || lasting > MaxLasting {
		return fmt.Errorf("%s: lasting_minutes is 0 to %d", what, MaxLasting)
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	cond, err := c.State().Keep(c.Arg(0))
	if err != nil {
		return err
	}
	fn, err := c.State().Keep(c.Arg(1))
	if err != nil {
		cond.Release()
		return err
	}
	sc := &scheduled{id: r.newID(), ctx: ctx, fn: fn, cond: cond, lasting: time.Duration(lasting) * time.Minute}
	r.addSchedule(sc)
	c.Return(script.Int(sc.id))
	return nil
}

func (r *Runtime) addSchedule(sc *scheduled) {
	if sc.ctx.schedules == nil {
		sc.ctx.schedules = map[int64]*scheduled{}
	}
	sc.ctx.schedules[sc.id] = sc
	r.schedules[sc.id] = sc
}

func (r *Runtime) schedulesCancel(c *script.Call) error {
	sc := r.schedules[c.Int(0)]
	if sc == nil {
		return nil
	}
	if err := r.mutating("schedule:cancel"); err != nil {
		return err
	}
	r.dropSchedule(sc)
	return nil
}

func (r *Runtime) dropSchedule(sc *scheduled) {
	if sc.canceled {
		return
	}
	sc.canceled = true
	delete(r.schedules, sc.id)
	delete(sc.ctx.schedules, sc.id)
	sc.fn.Release()
	if sc.cond != nil {
		sc.cond.Release()
	}
}

// SetLocation changes the server's time zone, before any schedule is
// added. Test worlds use it.
func (r *Runtime) SetLocation(loc *time.Location) { r.opts.Location = loc }

func (r *Runtime) location() *time.Location {
	if r.opts.Location == nil {
		return time.UTC
	}
	return r.opts.Location
}

// RunSchedules runs the schedules that are due at now, on the server
// clock: fixed times in order, then the conditions of schedules.when,
// once per minute. A fixed time that passed more than once, such as
// while the server was stopped, runs once.
func (r *Runtime) RunSchedules(now time.Time) {
	if len(r.schedules) == 0 {
		return
	}
	var due []*scheduled
	minute := schedule.MinuteStart(now)
	for _, sc := range r.schedules {
		if sc.cond == nil && !sc.next.After(now) || sc.cond != nil && sc.polled.Before(minute) {
			due = append(due, sc)
		}
	}
	sort.Slice(due, func(i, j int) bool {
		if !due[i].next.Equal(due[j].next) {
			return due[i].next.Before(due[j].next)
		}
		return due[i].id < due[j].id
	})
	for _, sc := range due {
		if sc.canceled || sc.ctx.ended || sc.ctx.retiring {
			continue
		}
		if r.off(sc.ctx) {
			// A schedule of a mod that is off skips its times.
			if sc.cond == nil {
				sc.next = schedule.Next(sc.spec, now, r.location())
			} else {
				sc.polled, sc.heldSince, sc.fired = minute, time.Time{}, false
			}
			continue
		}
		if sc.cond == nil {
			sc.next = schedule.Next(sc.spec, now, r.location())
			if err := r.run(sc.ctx, sc.fn.Value()); err != nil {
				r.failed(sc.ctx, "schedule", err)
			}
			continue
		}
		sc.polled = minute
		held, err := r.condition(sc.ctx, sc.cond.Value())
		if err != nil {
			r.failed(sc.ctx, "schedules.when condition", err)
			continue
		}
		if !held {
			sc.heldSince, sc.fired = time.Time{}, false
			continue
		}
		if sc.heldSince.IsZero() {
			sc.heldSince = minute
		}
		if !sc.fired && now.Sub(sc.heldSince) >= sc.lasting {
			sc.fired = true
			if err := r.run(sc.ctx, sc.fn.Value()); err != nil {
				r.failed(sc.ctx, "schedule", err)
			}
		}
	}
}

// condition runs a condition function in a context. Calls that change
// the game are refused while it runs.
func (r *Runtime) condition(ctx *Context, fn script.Value) (bool, error) {
	prev := r.computing
	r.computing = true
	defer func() { r.computing = prev }()
	var held bool
	err := r.runFn(ctx, func() error {
		v, err := r.s.Call1(fn)
		held = v.Truthy()
		return err
	})
	return held, err
}

// runFn runs f with a context set.
func (r *Runtime) runFn(c *Context, f func() error) error {
	prev := r.current
	r.current = c
	err := f()
	r.current = prev
	if err != nil {
		r.afterError()
	}
	return err
}

// NextSchedule returns when RunSchedules next has work: the first fixed
// time, or the next minute when a condition waits. ok is false without
// schedules.
func (r *Runtime) NextSchedule() (time.Time, bool) {
	var next time.Time
	for _, sc := range r.schedules {
		at := sc.next
		if sc.cond != nil {
			at = sc.polled.Add(time.Minute)
		}
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	return next, !next.IsZero()
}

// spawningMonsters is spawning.monsters with its spec table.
func (r *Runtime) spawningMonsters(c *script.Call) error {
	const what = "spawning.monsters"
	ctx, err := r.needContext(what)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "npc.create"); err != nil {
		return err
	}
	h, ok := r.host.(SpawnHost)
	if !ok {
		return errors.New(what + ": monsters cannot be spawned here")
	}
	if ctx.retiring {
		return errors.New(what + ": the NPC's script was reloaded: a conversation of the old version cannot spawn monsters")
	}
	t, ok := c.Arg(0).AsTable()
	if !ok {
		return errors.New(what + ": want a table such as { monster = PORING, amount = 5 }")
	}
	s := SpawnSpec{Amount: 1}
	var hasX, hasY bool
	for _, f := range t.Fields {
		k, _ := f.Key.AsString()
		num := func() (int64, error) {
			n, ok := f.Value.AsInt()
			if !ok {
				return 0, fmt.Errorf("%s: %s must be an integer", what, k)
			}
			return n, nil
		}
		switch k {
		case "map":
			m, ok := f.Value.AsString()
			if !ok {
				return fmt.Errorf("%s: map must be a map name, such as prontera", what)
			}
			if m != r.host.Map() {
				return fmt.Errorf("%s: this context runs on %s, and spawns monsters on its own map only, not on %s", what, r.host.Map(), m)
			}
		case "monster":
			n, err := num()
			if err != nil {
				return err
			}
			if n <= 0 || n > 1<<32-1 {
				return fmt.Errorf("%s: want a monster from monsters.id", what)
			}
			s.Monster = uint32(n) // #nosec G115 -- checked above.
		case "amount":
			n, err := num()
			if err != nil {
				return err
			}
			if n < 1 || n > MaxSpawn {
				return fmt.Errorf("%s: amount is 1 to %d", what, MaxSpawn)
			}
			s.Amount = int(n)
		case "x", "y":
			n, err := num()
			if err != nil {
				return err
			}
			if n < 0 || n > 1<<16 {
				return fmt.Errorf("%s: %s is not a cell", what, k)
			}
			if k == "x" {
				s.X, hasX = int(n), true
			} else {
				s.Y, hasY = int(n), true
			}
		case "area":
			n, err := num()
			if err != nil {
				return err
			}
			if n < 0 || n > MaxSpawnArea {
				return fmt.Errorf("%s: area is 0 to %d cells", what, MaxSpawnArea)
			}
			s.Area = int(n)
		case "respawn":
			b, ok := f.Value.AsBool()
			if !ok {
				return fmt.Errorf("%s: respawn must be true or false", what)
			}
			s.Respawn = b
		case "respawn_milliseconds":
			n, err := num()
			if err != nil {
				return err
			}
			if n < MinRespawnMs || n > MaxRespawnMs {
				return fmt.Errorf("%s: respawn_milliseconds is %d to %d", what, MinRespawnMs, MaxRespawnMs)
			}
			s.RespawnMs = int(n)
		default:
			return fmt.Errorf("%s: %q is not a field; the fields are map, monster, amount, x, y, area, respawn and respawn_milliseconds", what, k)
		}
	}
	if s.Monster == 0 {
		return fmt.Errorf("%s: monster is required", what)
	}
	if s.RespawnMs != 0 && !s.Respawn {
		return fmt.Errorf("%s: respawn_milliseconds needs respawn = true", what)
	}
	if hasX != hasY {
		return fmt.Errorf("%s: give both x and y, or neither for a random cell", what)
	}
	s.HasCell = hasX
	ids, err := h.SpawnMonsters(ctx.ID, s)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	ctx.spawned = true
	vals := make([]script.Value, len(ids))
	for i, id := range ids {
		vals[i] = script.Int(id)
	}
	c.Return(script.List(vals...))
	return nil
}

// StartEvent starts one run of a world event: its entry script, as
// <mod>:<path>, runs its setup function in a new event context. name
// is the event's full name. The run lasts until EndContext.
func (r *Runtime) StartEvent(name, entry string) (*Context, error) {
	setup, mod, err := r.setup(entry)
	if err != nil {
		return nil, err
	}
	sc := r.scripts[mod]
	c := &Context{ID: r.newID(), Kind: KindEvent, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}}
	c.Instance = script.NewTable(script.KV("name", script.String(name)), script.KV("map", script.String(r.host.Map())))
	r.events[c.ID] = c
	if err := r.runSetup(c, setup); err != nil {
		r.EndContext(c)
		return nil, fmt.Errorf("%s: setup: %w", entry, err)
	}
	return c, nil
}

// EndContext ends a context, such as an event run: its handlers,
// timers, schedules, hooks and monsters go.
func (r *Runtime) EndContext(c *Context) {
	delete(r.events, c.ID)
	r.end(c)
}

// CheckCondition runs the condition of a world event: function fn of
// the module that require name module finds, as mod. It only reads the
// game: calls that change it raise an error.
func (r *Runtime) CheckCondition(mod, module, fn string) (bool, error) {
	sc := r.scripts[mod]
	if sc == nil {
		return false, fmt.Errorf("%s has no scripts in this zone", mod)
	}
	if r.offMods[mod] {
		return false, ErrModOff
	}
	c := &Context{ID: r.newID(), Kind: KindEvent, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}}
	c.Instance = script.NewTable(script.KV("map", script.String(r.host.Map())))
	defer func() { c.ended = true }()
	prev := r.computing
	r.computing = true
	defer func() { r.computing = prev }()
	var held bool
	err := r.runFn(c, func() error {
		v, err := r.s.Call1(r.fns.condition.Value(), script.String(mod), script.String(module), script.String(fn))
		held = v.Truthy()
		return err
	})
	return held, err
}
