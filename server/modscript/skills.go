package modscript

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Skills and entity queries (proposal 0002, review decisions 18 and 20):
//   - maps.find_entities and entity:find_nearby, through EntityHost;
//   - entity:show_effect and entity:exists on every entity handle;
//   - the skill_used event for mod and event contexts;
//   - skill contexts: a skill with a script gets one context per zone,
//     whose cast handlers run when the skill takes effect (RunCast);
//   - passive skills: one context per owner and skill while the skill is
//     learned (StartPassive and StopPassive);
//   - the hooks skill_damage, cast_time, skill_cost and cast_allowed.
//
// The zone runs casting. It calls RunCast for a skill whose data names a
// script, and the cast object's methods call back into SkillHost. The
// runtime keeps no cast: the cast object holds its fields in Lua, and
// passes them to each SkillHost call, so a timer can use it after the
// event.

// EntityHost finds entities and acts on any entity handle. A Host
// without it raises an error on maps.find_entities, entity:find_nearby,
// and show_effect on monsters and NPCs. exists then reads a field.
type EntityHost interface {
	// FindEntities returns the entities of a query, filtered, sorted and
	// cut to its limit (EntityQuery says how).
	FindEntities(q EntityQuery) ([]Entity, error)
	// EntityExists reports whether an entity is still in the zone: not
	// removed, not logged out, and not moved to another map.
	EntityExists(e Entity) bool
	// ShowEntityEffect plays an effect on an entity for everyone near,
	// as specialeffect. Players go through Host.ShowEffect.
	ShowEntityEffect(e Entity, effect int64) error
}

// SkillHost runs the cast object's methods. A Host without it raises an
// error on them. Every method gets the cast as RunCast gave it.
//
// A SkillHost call runs inside a script's call, so it cannot run Lua:
// the events it causes, such as damage and death, fire after the
// script's call returns, as status events do. The skill_damage hooks of
// cast:deal_damage already ran in Lua: CastDamage.Percent is their
// result.
type SkillHost interface {
	// CastDealDamage runs rAthena's damage calculation of the cast's
	// skill on a target, with o's changes, and applies it: cards,
	// defense, the damage event. A nil result with no error means the
	// caster is gone or cannot act, as skill_addtimerskill's checks.
	CastDealDamage(c Cast, target Entity, o CastDamage) (*CastDamageResult, error)
	// CastHeal heals a target with rAthena's heal bonuses of the caster
	// and the target, as skill_calc_heal and status_heal, and returns the
	// HP healed. ok is false when the caster is gone or cannot act.
	CastHeal(c Cast, target Entity, amount int64) (healed int64, ok bool, err error)
	// CastRunDefault runs core's own behavior of the cast's skill, on
	// target, or on the cast's target when target is nil. It returns an
	// error for a skill that has none, such as a mod's skill.
	CastRunDefault(c Cast, target *Entity) error
	// SkillID finds a skill by name: skill_db's name or a mod skill's
	// full name, such as example.chain-lightning:CHAIN_LIGHTNING, for
	// skills.id.
	SkillID(name string) (int64, bool)
}

// Areas of an EntityQuery.
const (
	AreaAround    = "around"
	AreaCell      = "cell"
	AreaRectangle = "rectangle"
	AreaLine      = "line"
	AreaCells     = "cells"
)

// Orders of an EntityQuery.
const (
	SortNone     = ""
	SortNearest  = "nearest"
	SortFarthest = "farthest"
	SortLowestHP = "lowest_hp"
	SortRandom   = "random"
)

// Limits of entity queries.
const (
	// MaxQueryRadius is the largest radius, in cells.
	MaxQueryRadius = 30
	// MaxQueryLine is the longest line and the widest rectangle side, in
	// cells, and MaxLineWidth the widest line.
	MaxQueryLine = 2*MaxQueryRadius + 1
	MaxLineWidth = 15
	// MaxQueryCells is the most cells of a cells area, and of the
	// exclude list.
	MaxQueryCells = 1024
	// MaxFound is the most entities a query returns.
	MaxFound = 1000
)

// EntityQuery is what maps.find_entities and entity:find_nearby ask for.
// The zone finds the entities of the area, keeps those that pass every
// filter, sorts them, and cuts the list to Limit.
type EntityQuery struct {
	// Area is one of AreaAround, AreaCell, AreaRectangle, AreaLine and
	// AreaCells. Every area is on the zone's map.
	Area string
	// Around is the entity of AreaAround. It is in its own area, as in
	// rAthena's map_foreachinrange, unless Exclude or a filter leaves it
	// out. X and Y are the cell of AreaCell. Radius is the radius of
	// both, from 0. Circle is set for shape "circle": the cells within
	// Radius by Euclidean distance. Otherwise the area is a square, as
	// rAthena's ranges.
	Around Entity
	X, Y   int
	Radius int
	Circle bool
	// Corners are two opposite corners of AreaRectangle, each [x, y].
	Corners [2][2]int
	// From and To are the ends of AreaLine, and Width its width in
	// cells, from 1: the cells within Width/2 of the line from From to
	// To.
	From, To [2]int
	Width    int
	// Cells are the cells of AreaCells.
	Cells [][2]int
	// Kinds keeps entities of these kinds: "player", "monster" and
	// "npc". Empty keeps every kind.
	Kinds []string
	// Alive keeps living entities when true, and dead ones only when
	// false.
	Alive bool
	// EnemiesOf and AlliesOf keep the entities that one entity may
	// attack, or may help, by rAthena's battle_check_target rules
	// (BCT_ENEMY, and BCT_PARTY with the entity itself). nil keeps all.
	EnemiesOf, AlliesOf *Entity
	// LineOfSight keeps the entities that the center sees, as
	// path_search_long.
	LineOfSight bool
	// Exclude leaves these entities out.
	Exclude []Entity
	// Sort is one of the Sort constants. Distances are from the center.
	// SortNone and ties sort by kind, then handle ID. SortRandom draws
	// from the zone's secret stream.
	Sort string
	// Limit is the most entities to return, 1 to MaxFound.
	Limit int
}

// Center is the cell that LineOfSight and the distance orders measure
// from: the entity of AreaAround, the cell of AreaCell, the middle of a
// rectangle (rounded down), the From end of a line, and the first cell
// of AreaCells. ok is false for AreaAround, whose center is the entity's
// cell, which the zone knows.
func (q *EntityQuery) Center() (x, y int, ok bool) {
	switch q.Area {
	case AreaCell:
		return q.X, q.Y, true
	case AreaRectangle:
		return (q.Corners[0][0] + q.Corners[1][0]) / 2, (q.Corners[0][1] + q.Corners[1][1]) / 2, true
	case AreaLine:
		return q.From[0], q.From[1], true
	case AreaCells:
		return q.Cells[0][0], q.Cells[0][1], true
	}
	return 0, 0, false
}

// Cast is one cast of a skill whose script runs: what the cast event's
// object holds.
type Cast struct {
	// ID is the zone's own ID for the cast, passed back unchanged to the
	// SkillHost calls. The runtime does not keep casts.
	ID     int64
	Caster Entity
	Skill  int64
	Level  int64
	// Target is the target of a skill on a unit, or nil for a ground
	// skill.
	Target *Entity
	// X and Y are the cell the skill takes effect at: the target's, or
	// the ground cell.
	X, Y int64
}

// CastDamage is what cast:deal_damage asks for.
type CastDamage struct {
	// Percent is the skill's damage percentage, after the skill_damage
	// hooks.
	Percent int64
	// Hits replaces the data's hit count when above 0.
	Hits int64
	// Type replaces the data's attack type when set: "weapon", "magic"
	// or "misc".
	Type string
	// Element replaces the data's element when 0 or more, as the
	// constants of wa.elements.
	Element int64
}

// CastDamageResult is what cast:deal_damage returns.
type CastDamageResult struct {
	Amount   int64
	Critical bool
	Missed   bool
}

// SkillUsed is the object of the skill_used event of mod and event
// contexts: a skill took effect, after its cast time.
type SkillUsed struct {
	Caster       Entity
	Skill, Level int64
	// Target is nil for a ground skill.
	Target *Entity
	X, Y   int64
}

// SkillHook is the object of the skill_damage, cast_time, skill_cost and
// cast_allowed hooks.
type SkillHook struct {
	Caster       Entity
	Skill, Level int64
	// Target is nil for a ground skill, and for skill_cost.
	Target *Entity
}

// Hook limits.
const (
	// MaxSkillPercent is the highest damage percentage the skill_damage
	// hooks may give.
	MaxSkillPercent = 1_000_000
	// MaxCastPercent is the highest percent deal_damage takes.
	MaxCastPercent = MaxSkillPercent
	// MaxCastMs is the longest cast time the cast_time hooks may give.
	MaxCastMs = 3_600_000
)

// skillFilter is the options of a skill_used handler.
type skillFilter struct {
	skill   int64
	kind    string
	mapName string
}

// passiveKey names the passive context of one owner and skill.
type passiveKey struct {
	owner Entity
	skill int64
}

// passive is a running passive skill context.
type passive struct {
	ctx   *Context
	entry string
	level int64
}

func readSkillFilter(v script.Value) (*skillFilter, error) {
	const event = "skill_used"
	t, ok := v.AsTable()
	if !ok {
		return nil, fmt.Errorf("events.on(%q): the options must be a table", event)
	}
	f := &skillFilter{}
	for _, field := range t.Fields {
		k, _ := field.Key.AsString()
		switch k {
		case "skill":
			n, ok := field.Value.AsInt()
			if !ok || n <= 0 {
				return nil, fmt.Errorf("events.on(%q): skill must be a skill from skills.id", event)
			}
			f.skill = n
		case "caster_kind":
			s, _ := field.Value.AsString()
			if s != "player" && s != "monster" && s != "npc" {
				return nil, fmt.Errorf(`events.on(%q): caster_kind is "player", "monster" or "npc"`, event)
			}
			f.kind = s
		case "map":
			s, ok := field.Value.AsString()
			if !ok || s == "" {
				return nil, fmt.Errorf("events.on(%q): map must be a map name, such as prontera", event)
			}
			f.mapName = s
		default:
			return nil, fmt.Errorf("events.on(%q): %q is not an option; the options are skill, caster_kind and map", event, k)
		}
	}
	return f, nil
}

// skillUsedObject is the fields of a skill_used object, which the
// prelude turns into a read-only table with handles.
func skillUsedObject(caster Entity, skill, level int64, target *Entity, x, y int64) script.Value {
	fields := []script.Field{
		script.KV("caster", script.Int(caster.ID)), script.KV("caster_kind", script.String(caster.Kind)),
		script.KV("skill", script.Int(skill)), script.KV("level", script.Int(level)),
		script.KV("x", script.Int(x)), script.KV("y", script.Int(y)),
	}
	if target != nil {
		fields = append(fields, script.KV("target", script.Int(target.ID)), script.KV("target_kind", script.String(target.Kind)))
	}
	return script.NewTable(fields...)
}

// FireSkillUsed runs the skill_used handlers of mod and event contexts
// whose options match. The skill_used handlers of an equipped item run
// with FireSkillUse, for the wearer's skills.
func (r *Runtime) FireSkillUsed(ev SkillUsed) {
	list := r.handlers["skill_used"]
	if len(list) == 0 {
		return
	}
	obj := skillUsedObject(ev.Caster, ev.Skill, ev.Level, ev.Target, ev.X, ev.Y)
	for _, h := range slices.Clone(list) {
		if h.removed || h.ctx.ended || h.ctx.Kind == KindItem || r.off(h.ctx) {
			continue
		}
		if f := h.skill; f != nil {
			if f.skill != 0 && f.skill != ev.Skill || f.kind != "" && f.kind != ev.Caster.Kind ||
				f.mapName != "" && f.mapName != r.host.Map() {
				continue
			}
		}
		if err := r.runHandler(h.ctx, r.fns.invoke.Value(), h.fn.Value(), script.String("skill_use"), obj); err != nil {
			r.failed(h.ctx, "event skill_used", err)
		}
	}
}

// skillContext returns the context of a skill's script in this zone,
// and makes it on first use: entry's setup function runs in a new
// context of kind "skill".
func (r *Runtime) skillContext(entry string, skill int64) (*Context, error) {
	if c := r.skillCtxs[skill]; c != nil && r.skillEntries[skill] == entry {
		return c, nil
	}
	if old := r.skillCtxs[skill]; old != nil {
		r.end(old)
	}
	delete(r.skillCtxs, skill)
	setup, mod, err := r.setup(entry)
	if err != nil {
		return nil, err
	}
	sc := r.scripts[mod]
	c := &Context{ID: r.newID(), Kind: KindSkill, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}}
	c.Instance = script.NewTable(script.KV("skill", script.Int(skill)))
	if err := r.runSetup(c, setup); err != nil {
		r.end(c)
		return nil, fmt.Errorf("%s: setup: %w", entry, err)
	}
	r.skillCtxs[skill] = c
	r.skillEntries[skill] = entry
	return c, nil
}

// RunCast runs the cast handlers of a skill whose data names a script,
// as <mod>:<path>, when the skill takes effect. The script's setup
// function runs once per zone, at the skill's first cast, in the skill's
// context. ran is false when the script has no cast handler, or its
// context ended after failing too often: the zone then runs core's own
// behavior of the skill, if it has one. A handler that fails is logged,
// and its error returned, after the other handlers ran.
func (r *Runtime) RunCast(entry string, c Cast) (ran bool, err error) {
	ctx, err := r.skillContext(entry, c.Skill)
	if err != nil {
		return false, err
	}
	if !r.Listens(ctx, "cast") {
		return false, nil
	}
	return true, r.fireContext(ctx, "cast", Arg{"cast", castFields(c, r.host.Map())})
}

// castFields are the fields of a cast object. The prelude keeps them, and
// passes them back to each method.
func castFields(c Cast, mapName string) script.Value {
	fields := []script.Field{
		script.KV("id", script.Int(c.ID)),
		script.KV("caster", script.Int(c.Caster.ID)), script.KV("caster_kind", script.String(c.Caster.Kind)),
		script.KV("skill", script.Int(c.Skill)), script.KV("level", script.Int(c.Level)),
		script.KV("map", script.String(mapName)), script.KV("x", script.Int(c.X)), script.KV("y", script.Int(c.Y)),
	}
	if c.Target != nil {
		fields = append(fields, script.KV("target", script.Int(c.Target.ID)), script.KV("target_kind", script.String(c.Target.Kind)))
	}
	return script.NewTable(fields...)
}

// readCast reads the cast fields that a cast method passes back.
func readCast(v script.Value) (Cast, error) {
	t, ok := v.AsTable()
	if !ok {
		return Cast{}, errors.New("not a cast: call the method with a colon, such as cast:deal_damage(target)")
	}
	num := func(k string) int64 { n, _ := t.Get(k).AsInt(); return n }
	str := func(k string) string { s, _ := t.Get(k).AsString(); return s }
	c := Cast{ID: num("id"), Caster: Entity{Kind: str("caster_kind"), ID: num("caster")},
		Skill: num("skill"), Level: num("level"), X: num("x"), Y: num("y")}
	if k := str("target_kind"); k != "" {
		c.Target = &Entity{Kind: k, ID: num("target")}
	}
	return c, nil
}

// StartPassive runs a passive skill's script for an owner, in a context
// of kind "skill" whose instance is { skill, level, owner }. The zone
// calls it when the owner has the skill learned: when the owner enters
// the zone, and when the skill is learned. When the level changes, the
// zone calls it again: the old context ends and a new one starts. A
// call with the level and script the context has does nothing.
func (r *Runtime) StartPassive(entry string, skill int64, owner Entity, level int64) error {
	key := passiveKey{owner, skill}
	if p := r.passives[key]; p != nil {
		if p.entry == entry && p.level == level && !p.ctx.ended {
			return nil
		}
		r.end(p.ctx)
		delete(r.passives, key)
	}
	setup, mod, err := r.setup(entry)
	if err != nil {
		return err
	}
	sc := r.scripts[mod]
	c := &Context{ID: r.newID(), Kind: KindSkill, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}}
	c.Instance = script.NewTable(
		script.KV("skill", script.Int(skill)), script.KV("level", script.Int(level)),
		script.KV("owner", script.Int(owner.ID)), script.KV("owner_kind", script.String(owner.Kind)),
	)
	r.passives[key] = &passive{ctx: c, entry: entry, level: level}
	if err := r.runSetup(c, setup); err != nil {
		r.end(c)
		delete(r.passives, key)
		return fmt.Errorf("%s: setup: %w", entry, err)
	}
	return nil
}

// StopPassive ends the passive context of an owner's skill, as when the
// skill is unlearned. The zone calls StopPassives when the owner leaves
// the zone or dies, if the zone ends passives at death.
func (r *Runtime) StopPassive(skill int64, owner Entity) {
	key := passiveKey{owner, skill}
	if p := r.passives[key]; p != nil {
		delete(r.passives, key)
		r.end(p.ctx)
	}
}

// StopPassives ends every passive context of an owner, as when it logs
// out or leaves the zone.
func (r *Runtime) StopPassives(owner Entity) {
	for key, p := range r.passives {
		if key.owner == owner {
			delete(r.passives, key)
			r.end(p.ctx)
		}
	}
}

// reloadSkills ends the skill contexts of a reloaded mod. Active skills
// start again at their next cast. Passive contexts start again at once,
// with the new scripts. Close ends them all.
func (r *Runtime) reloadSkills(modID string) error {
	for skill, c := range r.skillCtxs {
		if c.Mod == modID || strings.HasPrefix(r.skillEntries[skill], modID+":") {
			r.end(c)
			delete(r.skillCtxs, skill)
			delete(r.skillEntries, skill)
		}
	}
	var errs []error
	for key, p := range r.passives {
		if p.ctx.Mod != modID {
			continue
		}
		r.end(p.ctx)
		delete(r.passives, key)
		if err := r.StartPassive(p.entry, key.skill, key.owner, p.level); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// endSkills ends every skill context, for Close.
func (r *Runtime) endSkills() {
	for skill, c := range r.skillCtxs {
		r.end(c)
		delete(r.skillCtxs, skill)
	}
	for key, p := range r.passives {
		r.end(p.ctx)
		delete(r.passives, key)
	}
}

func (h SkillHook) fields() []script.Field {
	f := []script.Field{
		script.KV("caster", script.Int(h.Caster.ID)), script.KV("caster_kind", script.String(h.Caster.Kind)),
		script.KV("skill", script.Int(h.Skill)), script.KV("level", script.Int(h.Level)),
	}
	if h.Target != nil {
		f = append(f, script.KV("target", script.Int(h.Target.ID)), script.KV("target_kind", script.String(h.Target.Kind)))
	}
	return f
}

// runSkillHooks runs a skill hook chain, unless a script's call runs: a
// hook chain cannot run inside a host call (internal/script, "Nested
// calls"), so the value stays rAthena's there.
func (r *Runtime) runSkillHooks(name string, fields []script.Field) ([]script.Value, bool) {
	if r.s.InHost() {
		return nil, false
	}
	return r.runHooks(name, fields)
}

// RunSkillDamage runs the skill_damage hooks and returns the damage
// percentage they give, from 0 to MaxSkillPercent. Without hooks, when
// the chain fails, or inside a script's call, it is percent.
// cast:deal_damage runs these hooks itself, in Lua: do not run them again
// for its damage.
func (r *Runtime) RunSkillDamage(h SkillHook, percent int64) int64 {
	vals, ok := r.runSkillHooks("skill_damage", append(h.fields(), script.KV("percent", script.Int(percent))))
	if !ok {
		return percent
	}
	n, _ := vals[0].AsInt()
	return min(max(n, 0), MaxSkillPercent)
}

// RunCastTime runs the cast_time hooks and returns the cast time they
// give, in milliseconds, from 0 to MaxCastMs. Without hooks, when the
// chain fails, or inside a script's call, it is ms.
func (r *Runtime) RunCastTime(h SkillHook, ms int64) int64 {
	vals, ok := r.runSkillHooks("cast_time", append(h.fields(), script.KV("milliseconds", script.Int(ms))))
	if !ok {
		return ms
	}
	n, _ := vals[0].AsInt()
	return min(max(n, 0), MaxCastMs)
}

// RunSkillCost runs the skill_cost hooks and returns the SP cost they
// give, from 0. Without hooks, when the chain fails, or inside a
// script's call, it is sp.
func (r *Runtime) RunSkillCost(h SkillHook, sp int64) int64 {
	vals, ok := r.runSkillHooks("skill_cost", append(h.fields(), script.KV("sp", script.Int(sp))))
	if !ok {
		return sp
	}
	n, _ := vals[0].AsInt()
	return max(n, 0)
}

// RunCastAllowed runs the cast_allowed hooks and returns whether the
// cast may start. Without hooks, when the chain fails, or inside a
// script's call, it is allowed.
func (r *Runtime) RunCastAllowed(h SkillHook, allowed bool) bool {
	vals, ok := r.runSkillHooks("cast_allowed", append(h.fields(), script.KV("allowed", script.Bool(allowed))))
	if !ok {
		return allowed
	}
	b, _ := vals[0].AsBool()
	return b
}

// entityHost returns the host's entity calls.
func (r *Runtime) entityHost(what string) (EntityHost, error) {
	h, ok := r.host.(EntityHost)
	if !ok {
		return nil, fmt.Errorf("%s: this zone cannot find entities", what)
	}
	return h, nil
}

// skillHost returns the host's cast calls.
func (r *Runtime) skillHost(what string) (SkillHost, error) {
	h, ok := r.host.(SkillHost)
	if !ok {
		return nil, fmt.Errorf("%s: this zone has no skills", what)
	}
	return h, nil
}

// parseEntityKey reads a handle key, "<kind>:<id>", that the prelude
// makes of an entity handle.
func parseEntityKey(v script.Value) (Entity, bool) {
	s, ok := v.AsString()
	if !ok {
		return Entity{}, false
	}
	kind, id, found := strings.Cut(s, ":")
	n, err := strconv.ParseInt(id, 10, 64)
	if !found || err != nil || kind != "player" && kind != "monster" && kind != "npc" {
		return Entity{}, false
	}
	return Entity{Kind: kind, ID: n}, true
}

// mapsFindEntities is maps.find_entities with the query as plain data:
// handles are "<kind>:<id>" keys.
func (r *Runtime) mapsFindEntities(c *script.Call) error {
	what := c.String(0)
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	q, err := r.readQuery(what, c.Arg(1))
	if err != nil {
		return err
	}
	h, err := r.entityHost(what)
	if err != nil {
		return err
	}
	list, err := h.FindEntities(q)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if len(list) > q.Limit {
		list = list[:q.Limit]
	}
	out := make([]script.Value, len(list))
	for i, e := range list {
		out[i] = script.NewTable(script.KV("kind", script.String(e.Kind)), script.KV("id", script.Int(e.ID)))
	}
	c.Return(script.List(out...))
	return nil
}

// queryFields are the fields of a query.
const queryFields = "around, map, x, y, radius, rectangle, line, cells, shape, kinds, alive, enemies_of, allies_of, line_of_sight, exclude, sort and limit"

// readQuery reads and checks a query.
func (r *Runtime) readQuery(what string, v script.Value) (EntityQuery, error) {
	q := EntityQuery{Alive: true, Limit: MaxFound}
	t, ok := v.AsTable()
	if !ok {
		return q, fmt.Errorf("%s: want a query table, such as { around = player, radius = 5 }", what)
	}
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%s: %s", what, fmt.Sprintf(format, args...))
	}
	var areas []string
	var hasX, hasY, hasRadius, hasShape bool
	cell := func(v script.Value) ([2]int, bool) {
		ct, ok := v.AsTable()
		if !ok || len(ct.Fields) != 2 {
			return [2]int{}, false
		}
		x, ok1 := ct.Index(1).AsInt()
		y, ok2 := ct.Index(2).AsInt()
		if !ok1 || !ok2 || x < 0 || y < 0 || x > 1<<16 || y > 1<<16 {
			return [2]int{}, false
		}
		return [2]int{int(x), int(y)}, true
	}
	for _, f := range t.Fields {
		k, _ := f.Key.AsString()
		switch k {
		case "around":
			e, ok := parseEntityKey(f.Value)
			if !ok {
				return q, bad("around must be a player, monster or NPC handle")
			}
			q.Around = e
			areas = append(areas, "around")
		case "map":
			m, ok := f.Value.AsString()
			if !ok {
				return q, bad("map must be a map name, such as prontera")
			}
			if m != r.host.Map() {
				return q, bad("this context runs on %s, and finds entities on its own map only, not on %s", r.host.Map(), m)
			}
		case "x", "y":
			n, ok := f.Value.AsInt()
			if !ok || n < 0 || n > 1<<16 {
				return q, bad("%s must be a cell coordinate", k)
			}
			if k == "x" {
				q.X, hasX = int(n), true
			} else {
				q.Y, hasY = int(n), true
			}
		case "radius":
			n, ok := f.Value.AsInt()
			if !ok || n < 0 || n > MaxQueryRadius {
				return q, bad("radius must be 0 to %d cells", MaxQueryRadius)
			}
			q.Radius, hasRadius = int(n), true
		case "rectangle":
			rt, ok := f.Value.AsTable()
			var a, b [2]int
			var okA, okB bool
			if ok && len(rt.Fields) == 2 {
				a, okA = cell(rt.Index(1))
				b, okB = cell(rt.Index(2))
			}
			if !okA || !okB {
				return q, bad("rectangle must be two corners, such as { { 100, 100 }, { 110, 105 } }")
			}
			if abs(a[0]-b[0]) >= MaxQueryLine || abs(a[1]-b[1]) >= MaxQueryLine {
				return q, bad("a rectangle's sides are at most %d cells", MaxQueryLine)
			}
			q.Corners = [2][2]int{a, b}
			areas = append(areas, "rectangle")
		case "line":
			lt, ok := f.Value.AsTable()
			if !ok {
				return q, bad("line must be a table, such as { from = { 100, 100 }, to = { 105, 100 }, width = 3 }")
			}
			q.Width = 1
			var hasFrom, hasTo bool
			for _, lf := range lt.Fields {
				lk, _ := lf.Key.AsString()
				switch lk {
				case "from", "to":
					p, ok := cell(lf.Value)
					if !ok {
						return q, bad("line.%s must be a cell, such as { 100, 100 }", lk)
					}
					if lk == "from" {
						q.From, hasFrom = p, true
					} else {
						q.To, hasTo = p, true
					}
				case "width":
					n, ok := lf.Value.AsInt()
					if !ok || n < 1 || n > MaxLineWidth {
						return q, bad("line.width must be 1 to %d cells", MaxLineWidth)
					}
					q.Width = int(n)
				default:
					return q, bad("line: %q is not a field; the fields are from, to and width", lk)
				}
			}
			if !hasFrom || !hasTo {
				return q, bad("line needs from and to")
			}
			if abs(q.From[0]-q.To[0]) >= MaxQueryLine || abs(q.From[1]-q.To[1]) >= MaxQueryLine {
				return q, bad("a line is at most %d cells long", MaxQueryLine)
			}
			areas = append(areas, "line")
		case "cells":
			ct, ok := f.Value.AsTable()
			if !ok || len(ct.Fields) == 0 || len(ct.Fields) > MaxQueryCells {
				return q, bad("cells must be a list of 1 to %d cells, such as { { 100, 100 }, { 101, 100 } }", MaxQueryCells)
			}
			for i := range len(ct.Fields) {
				p, ok := cell(ct.Index(int64(i + 1)))
				if !ok {
					return q, bad("cells: entry %d is not a cell, such as { 100, 100 }", i+1)
				}
				q.Cells = append(q.Cells, p)
			}
			areas = append(areas, "cells")
		case "shape":
			s, _ := f.Value.AsString()
			if s != "square" && s != "circle" {
				return q, bad(`shape is "square" or "circle"`)
			}
			q.Circle, hasShape = s == "circle", true
		case "kinds":
			kt, ok := f.Value.AsTable()
			if !ok || len(kt.Fields) == 0 {
				return q, bad(`kinds must be a list, such as { "monster" }`)
			}
			for i := range len(kt.Fields) {
				s, _ := kt.Index(int64(i + 1)).AsString()
				if s != "player" && s != "monster" && s != "npc" {
					return q, bad(`kinds holds "player", "monster" and "npc"`)
				}
				if !slices.Contains(q.Kinds, s) {
					q.Kinds = append(q.Kinds, s)
				}
			}
		case "alive", "line_of_sight":
			b, ok := f.Value.AsBool()
			if !ok {
				return q, bad("%s must be true or false", k)
			}
			if k == "alive" {
				q.Alive = b
			} else {
				q.LineOfSight = b
			}
		case "enemies_of", "allies_of":
			e, ok := parseEntityKey(f.Value)
			if !ok {
				return q, bad("%s must be a player, monster or NPC handle", k)
			}
			if k == "enemies_of" {
				q.EnemiesOf = &e
			} else {
				q.AlliesOf = &e
			}
		case "exclude":
			et, ok := f.Value.AsTable()
			if !ok || len(et.Fields) > MaxQueryCells {
				return q, bad("exclude must be a list of at most %d handles", MaxQueryCells)
			}
			for i := range len(et.Fields) {
				e, ok := parseEntityKey(et.Index(int64(i + 1)))
				if !ok {
					return q, bad("exclude must be a list of player, monster or NPC handles")
				}
				q.Exclude = append(q.Exclude, e)
			}
		case "sort":
			s, _ := f.Value.AsString()
			switch s {
			case SortNearest, SortFarthest, SortLowestHP, SortRandom:
				q.Sort = s
			default:
				return q, bad(`sort is "nearest", "farthest", "lowest_hp" or "random"`)
			}
		case "limit":
			n, ok := f.Value.AsInt()
			if !ok || n < 1 || n > MaxFound {
				return q, bad("limit must be 1 to %d", MaxFound)
			}
			q.Limit = int(n)
		default:
			return q, bad("%q is not a field; the fields are %s", k, queryFields)
		}
	}
	if hasX != hasY {
		return q, bad("give both x and y")
	}
	if hasX {
		areas = append(areas, "x and y")
	}
	switch {
	case len(areas) == 0:
		return q, bad("the query needs an area: around, x and y, rectangle, line or cells")
	case len(areas) > 1:
		return q, bad("the query has one area, and this one has %s", strings.Join(areas, " and "))
	}
	switch areas[0] {
	case "around":
		q.Area = AreaAround
	case "x and y":
		q.Area = AreaCell
	case "rectangle":
		q.Area = AreaRectangle
	case "line":
		q.Area = AreaLine
	default:
		q.Area = AreaCells
	}
	round := q.Area == AreaAround || q.Area == AreaCell
	if round && !hasRadius {
		return q, bad("%s needs radius", areas[0])
	}
	if !round && (hasRadius || hasShape) {
		return q, bad("radius and shape go with around, or x and y, and this area is %s", areas[0])
	}
	if q.EnemiesOf != nil && q.AlliesOf != nil {
		return q, bad("give enemies_of or allies_of, not both")
	}
	return q, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// entityExists is entity:exists. It never fails: an error is false.
func (r *Runtime) entityExists(c *script.Call) error {
	e, err := entityArg(c, 0)
	if err != nil {
		c.Return(script.Bool(false))
		return nil
	}
	if h, ok := r.host.(EntityHost); ok {
		c.Return(script.Bool(h.EntityExists(e)))
		return nil
	}
	var ferr error
	switch e.Kind {
	case "player":
		_, ferr = r.host.PlayerField(e.ID, "name")
	case "monster":
		_, ferr = r.host.MonsterField(e.ID, "name")
	default:
		h, err := r.npcHost()
		if err == nil {
			_, err = h.NPCField(e.ID, "name")
		}
		ferr = err
	}
	c.Return(script.Bool(ferr == nil))
	return nil
}

// entityShowEffect is show_effect on monster and NPC handles.
func (r *Runtime) entityShowEffect(c *script.Call) error {
	e, err := entityArg(c, 0)
	if err != nil {
		return err
	}
	what := e.Kind + ":show_effect"
	if err := r.mutating(what); err != nil {
		return err
	}
	effect, ok := c.Arg(2).AsInt()
	if !ok || effect < 0 || effect > 1<<20 {
		return fmt.Errorf("%s: want an effect from wa.effects", what)
	}
	if e.Kind == "player" {
		return r.host.ShowEffect(e.ID, effect, false)
	}
	h, err := r.entityHost(what)
	if err != nil {
		return err
	}
	return h.ShowEntityEffect(e, effect)
}

// castDealDamage is cast:deal_damage: the cast's fields, the target's
// key, then the percent after the hooks, the hits, the type and the
// element (nil for the data's).
func (r *Runtime) castDealDamage(c *script.Call) error {
	const what = "cast:deal_damage"
	cast, err := readCast(c.Arg(0))
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.skillHost(what)
	if err != nil {
		return err
	}
	target, ok := parseEntityKey(c.Arg(1))
	if !ok {
		return fmt.Errorf("%s: the target must be a player, monster or NPC handle", what)
	}
	o := CastDamage{Element: -1}
	o.Percent, _ = c.Arg(2).AsInt()
	if o.Percent < 0 || o.Percent > MaxCastPercent {
		return fmt.Errorf("%s: percent must be 0 to %d", what, MaxCastPercent)
	}
	if v := c.Arg(3); !v.IsNil() {
		n, ok := v.AsInt()
		if !ok || n < 1 || n > 30 {
			return fmt.Errorf("%s: hits must be 1 to 30", what)
		}
		o.Hits = n
	}
	if v := c.Arg(4); !v.IsNil() {
		s, _ := v.AsString()
		if s != "weapon" && s != "magic" && s != "misc" {
			return fmt.Errorf(`%s: type is "weapon", "magic" or "misc"`, what)
		}
		o.Type = s
	}
	if v := c.Arg(5); !v.IsNil() {
		n, ok := v.AsInt()
		if !ok || n < 0 || n > 9 {
			return fmt.Errorf("%s: element must be an element of wa.elements, not all", what)
		}
		o.Element = n
	}
	res, err := h.CastDealDamage(cast, target, o)
	if err != nil {
		return err
	}
	if res == nil {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.NewTable(script.KV("amount", script.Int(res.Amount)),
		script.KV("is_critical", script.Bool(res.Critical)), script.KV("missed", script.Bool(res.Missed))))
	return nil
}

// castHeal is cast:heal: the cast's fields, the target's key and the
// amount.
func (r *Runtime) castHeal(c *script.Call) error {
	const what = "cast:heal"
	cast, err := readCast(c.Arg(0))
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.skillHost(what)
	if err != nil {
		return err
	}
	target, ok := parseEntityKey(c.Arg(1))
	if !ok {
		return fmt.Errorf("%s: the target must be a player, monster or NPC handle", what)
	}
	amount, ok := c.Arg(2).AsInt()
	if !ok || amount < 0 || amount > 1<<31-1 {
		return fmt.Errorf("%s: the amount must be 0 to 2147483647", what)
	}
	healed, ok, err := h.CastHeal(cast, target, amount)
	if err != nil {
		return err
	}
	if !ok {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(healed))
	return nil
}

// castRunDefault is cast:run_default: the cast's fields, and the target's
// key or nil.
func (r *Runtime) castRunDefault(c *script.Call) error {
	const what = "cast:run_default"
	cast, err := readCast(c.Arg(0))
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.skillHost(what)
	if err != nil {
		return err
	}
	var target *Entity
	if v := c.Arg(1); !v.IsNil() {
		e, ok := parseEntityKey(v)
		if !ok {
			return fmt.Errorf("%s: target must be a player, monster or NPC handle", what)
		}
		target = &e
	}
	return h.CastRunDefault(cast, target)
}

// InHost reports whether a script's call runs now, so a host call is
// on the stack: Lua cannot run again until it returns, except to
// compute stats.
func (r *Runtime) InHost() bool { return r.s.InHost() }
