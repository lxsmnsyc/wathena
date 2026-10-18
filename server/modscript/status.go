package modscript

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/bonus"
)

// Status effects (proposal 0002, review decision 19): the status methods
// of every entity handle, the events status_start, status_end and
// status_tick, the hooks status_chance and status_duration,
// statuses.define, and player:add_timed_bonuses.
//
// A host function may call back into Lua only to compute stats again
// (internal/script, "Nested calls"). So when a script starts a status,
// the prelude runs the other Lua first: the bonuses function of a mod
// status, and the hooks, with the chance and duration that
// PreviewStatus gives. StartStatus then uses their results.

// StatusHost runs statuses for scripts. A Host without it raises an
// error on the status calls.
type StatusHost interface {
	// StartStatus starts a status on an entity, and reports whether it
	// started.
	StartStatus(target Entity, s StatusStart) (bool, error)
	// PreviewStatus is what the hooks of a start see: the chance and the
	// duration after resistance. ok is false when the start would fail
	// before its roll, so no hook runs.
	PreviewStatus(target Entity, s StatusStart) (p StatusPreview, ok bool, err error)
	// EndStatus ends a status, and reports whether it was active.
	EndStatus(target Entity, status int64) (bool, error)
	// Statuses lists an entity's active statuses, or only one when
	// status is above -1.
	Statuses(target Entity, status int64) ([]StatusInfo, error)
	// DefineStatus adds a mod's status and returns its ID.
	DefineStatus(d StatusDefinition) (int64, error)
	// AddTimedBonuses gives a player timed bonuses. It reports whether
	// they were added or extended, and not ignored by on_repeat.
	AddTimedBonuses(player int64, t TimedBonuses) (bool, error)
}

// Entity names an entity handle: its kind, "player", "monster" or
// "npc", and the handle ID.
type Entity struct {
	Kind string
	ID   int64
}

// Forever is the milliseconds of a status without an end, as rAthena's
// INFINITE_TICK.
const Forever = -1

// StatusStart is what entity:start_status asks for.
type StatusStart struct {
	Status int64
	// Milliseconds is the duration, Forever, or 0 when the script gave
	// none: the duration rAthena's sc_start looks up for the level.
	Milliseconds int64
	// Level is val1, and Values val2 to val4.
	Level  int64
	Values [3]int64
	// Chance is per 10,000.
	Chance int64
	// Source is who caused the status, or nil.
	Source *Entity
	// The flags SCSTART_NOAVOID, SCSTART_NOTICKDEF, SCSTART_NORATEDEF
	// and SCSTART_NOICON.
	CannotBeAvoided, KeepDuration, KeepChance, HideIcon bool
	// Reason is "item", "skill" or "script", from the running context.
	Reason string
	// Bonuses are the bonuses of a mod status, from its definition.
	Bonuses []bonus.Bonus
	// HookChance and HookDuration are what the hooks gave, or -1 when
	// none ran.
	HookChance, HookDuration int64
}

// StatusPreview is what the status_chance and status_duration hooks of
// a start see.
type StatusPreview struct {
	Chance       int64
	Milliseconds int64
}

// StatusInfo is an active status, as entity:get_status returns it.
type StatusInfo struct {
	Status, Level int64
	Values        [3]int64
	// Remaining is the time left, or Forever.
	Remaining int64
	Source    *Entity
}

// StatusDefinition is what statuses.define gives the host.
type StatusDefinition struct {
	Mod, Name string
	Icon      int64
	Debuff    bool
	EndsOn    []string
	TickMs    int64
}

// TimedBonuses is what player:add_timed_bonuses gives (0002a,
// "bonus_script").
type TimedBonuses struct {
	Key          string
	Milliseconds int64
	// Bonuses are the entries, checked, and Data the same entries as
	// plain data, for saving.
	Bonuses []bonus.Bonus
	Data    []any
	EndsOn  []string
	// Repeat is what happens when the player has timed bonuses with the
	// same key: RepeatKeep, RepeatExtend, RepeatAdd or RepeatReplace.
	Repeat string
	// Icon is the EFST_ icon, or -1. Kind is "buff", "debuff" or "".
	Icon int64
	Kind string
}

// The modes of the option on_repeat of add_timed_bonuses, for a key the player already
// has. The first three are pc_bonus_script_add's.
const (
	// RepeatKeep keeps the first copy and ignores the new one. It is
	// rAthena's default.
	RepeatKeep = "keep"
	// RepeatExtend keeps the first copy's values and takes the new
	// duration when it ends later: BSF_FORCE_REPLACE.
	RepeatExtend = "extend"
	// RepeatAdd adds a second copy: BSF_FORCE_DUPLICATE.
	RepeatAdd = "add"
	// RepeatReplace ends every copy and adds the new one, with its values
	// and duration. rAthena has no such flag.
	RepeatReplace = "replace"
)

// Limits of the status calls.
const (
	// MaxStatusName is the longest name of statuses.define.
	MaxStatusName = 48
	// MaxTimedBonuses is the most entries of one add_timed_bonuses.
	MaxTimedBonuses = 64
	// MaxStatusMs is the longest duration a script gives, about 24 days,
	// as rAthena keeps durations in an int32.
	MaxStatusMs = math.MaxInt32
)

// statusEvents are the events of decision 19.
var statusEvents = []string{"status_start", "status_end", "status_tick"}

// StatusEvent is the object of status_start, status_end and status_tick.
type StatusEvent struct {
	Target        Entity
	Status, Level int64
	Source        *Entity
	// Milliseconds is the duration at the start, and the time left after
	// it, or Forever.
	Milliseconds int64
	// Reason is why the status started or ended. A tick has none.
	Reason string
	// Map is the zone's map.
	Map string
}

// statusFilter is the options of a status event handler.
type statusFilter struct {
	status  int64
	kind    string
	mapName string
}

func readStatusFilter(event string, v script.Value) (*statusFilter, error) {
	t, ok := v.AsTable()
	if !ok {
		return nil, fmt.Errorf("events.on(%q): the options must be a table", event)
	}
	f := &statusFilter{status: -1}
	for _, field := range t.Fields {
		k, _ := field.Key.AsString()
		switch k {
		case "status":
			n, ok := field.Value.AsInt()
			if !ok || n < 0 {
				return nil, fmt.Errorf("events.on(%q): status must be a status from wa.statuses", event)
			}
			f.status = n
		case "target_kind":
			s, _ := field.Value.AsString()
			if s != "player" && s != "monster" && s != "npc" {
				return nil, fmt.Errorf(`events.on(%q): target_kind is "player", "monster" or "npc"`, event)
			}
			f.kind = s
		case "map":
			s, ok := field.Value.AsString()
			if !ok || s == "" {
				return nil, fmt.Errorf("events.on(%q): map must be a map name, such as prontera", event)
			}
			f.mapName = s
		default:
			return nil, fmt.Errorf("events.on(%q): %q is not an option; the options are status, target_kind and map", event, k)
		}
	}
	return f, nil
}

// FireStatus runs the handlers of a status event whose options match.
func (r *Runtime) FireStatus(event string, ev StatusEvent) {
	list := r.handlers[event]
	if len(list) == 0 {
		return
	}
	obj := statusObject(ev)
	for _, h := range slices.Clone(list) {
		if h.removed || h.ctx.ended || r.off(h.ctx) {
			continue
		}
		if f := h.status; f != nil {
			if f.status >= 0 && f.status != ev.Status || f.kind != "" && f.kind != ev.Target.Kind ||
				f.mapName != "" && f.mapName != ev.Map {
				continue
			}
		}
		if err := r.runHandler(h.ctx, r.fns.invoke.Value(), h.fn.Value(), script.String("status"), obj); err != nil {
			r.failed(h.ctx, "event "+event, err)
		}
	}
}

// statusObject is the fields of a status event, which the prelude turns
// into a read-only table with handles.
func statusObject(ev StatusEvent) script.Value {
	fields := []script.Field{
		script.KV("target", script.Int(ev.Target.ID)), script.KV("target_kind", script.String(ev.Target.Kind)),
		script.KV("status", script.Int(ev.Status)), script.KV("level", script.Int(ev.Level)),
		script.KV("milliseconds", millis(ev.Milliseconds)),
	}
	if ev.Reason != "" {
		fields = append(fields, script.KV("reason", script.String(ev.Reason)))
	}
	if ev.Source != nil {
		fields = append(fields, script.KV("source", script.Int(ev.Source.ID)), script.KV("source_kind", script.String(ev.Source.Kind)))
	}
	return script.NewTable(fields...)
}

// millis is a duration for Lua: a number, or "forever".
func millis(ms int64) script.Value {
	if ms < 0 {
		return script.String("forever")
	}
	return script.Int(ms)
}

// StatusHook is the object of the status_chance and status_duration
// hooks: the target, the status, its level, and the source.
type StatusHook struct {
	Target        Entity
	Status, Level int64
	Source        *Entity
}

func (h StatusHook) fields() []script.Field {
	f := []script.Field{
		script.KV("target", script.Int(h.Target.ID)), script.KV("target_kind", script.String(h.Target.Kind)),
		script.KV("status", script.Int(h.Status)), script.KV("level", script.Int(h.Level)),
	}
	if h.Source != nil {
		f = append(f, script.KV("source", script.Int(h.Source.ID)), script.KV("source_kind", script.String(h.Source.Kind)))
	}
	return f
}

// RunStatusChance runs the status_chance hooks and returns the chance
// they give, per 10,000. Without hooks, or when the chain fails, it is
// chance.
func (r *Runtime) RunStatusChance(h StatusHook, chance int64) int64 {
	vals, ok := r.runHooks("status_chance", append(h.fields(), script.KV("chance", script.Int(chance))))
	if !ok {
		return chance
	}
	n, _ := vals[0].AsInt()
	return n
}

// RunStatusDuration runs the status_duration hooks and returns the
// duration they give, in milliseconds. Without hooks, or when the chain
// fails, it is ms.
func (r *Runtime) RunStatusDuration(h StatusHook, ms int64) int64 {
	vals, ok := r.runHooks("status_duration", append(h.fields(), script.KV("milliseconds", script.Int(ms))))
	if !ok {
		return ms
	}
	n, _ := vals[0].AsInt()
	return n
}

// statusHost returns the host's status calls.
func (r *Runtime) statusHost(what string) (StatusHost, error) {
	h, ok := r.host.(StatusHost)
	if !ok {
		return nil, fmt.Errorf("%s: this zone has no status effects", what)
	}
	return h, nil
}

// entityArg reads an entity handle passed as a kind and an ID.
func entityArg(c *script.Call, i int) (Entity, error) {
	kind, _ := c.Arg(i).AsString()
	id, ok := c.Arg(i + 1).AsInt()
	if !ok || (kind != "player" && kind != "monster" && kind != "npc") {
		return Entity{}, errors.New("not an entity: call the method with a colon, such as player:has_status(status)")
	}
	return Entity{Kind: kind, ID: id}, nil
}

// entityStatus is the status methods of entity handles. Arguments: the
// operation, the entity's kind and ID, the status, then for "start" and
// "preview" the plain options, the bonus entries of a mod status, and
// the hooks' chance and duration.
func (r *Runtime) entityStatus(c *script.Call) error {
	op := c.String(0)
	what := map[string]string{"start": "start_status", "preview": "start_status", "end": "end_status",
		"has": "has_status", "get": "get_status", "list": "get_statuses"}[op]
	if what == "" {
		return fmt.Errorf("entity status: unknown operation %q", op)
	}
	target, err := entityArg(c, 1)
	if err != nil {
		return err
	}
	what = target.Kind + ":" + what
	h, err := r.statusHost(what)
	if err != nil {
		return err
	}
	status := int64(-1)
	if op != "list" {
		n, ok := c.Arg(3).AsInt()
		if !ok || n < 0 {
			return fmt.Errorf("%s: want a status from wa.statuses", what)
		}
		status = n
	}
	// Starting and ending a player's status needs player.status (review
	// decision 52). A preview only computes.
	if (op == "start" || op == "end") && target.Kind == "player" {
		if err := r.need(what, "player.status"); err != nil {
			return err
		}
	}
	switch op {
	case "start", "preview":
		if err := r.mutating(what); err != nil {
			return err
		}
		s, err := r.readStatusStart(what, status, c.Arg(4))
		if err != nil {
			return err
		}
		if op == "preview" {
			p, ok, err := h.PreviewStatus(target, s)
			if err != nil || !ok {
				c.Return(script.Nil)
				return err
			}
			c.Return(script.NewTable(script.KV("chance", script.Int(p.Chance)), script.KV("milliseconds", script.Int(p.Milliseconds)),
				script.KV("level", script.Int(s.Level))))
			return nil
		}
		if entries := c.Arg(5); !entries.IsNil() {
			if s.Bonuses, _, err = r.readBonusEntries(what, entries); err != nil {
				return err
			}
		}
		s.HookChance, s.HookDuration = -1, -1
		if n, ok := c.Arg(6).AsInt(); ok {
			s.HookChance = n
		}
		if n, ok := c.Arg(7).AsInt(); ok {
			s.HookDuration = n
		}
		ok, err := h.StartStatus(target, s)
		if err != nil {
			return err
		}
		c.Return(script.Bool(ok))
	case "end":
		if err := r.mutating(what); err != nil {
			return err
		}
		ok, err := h.EndStatus(target, status)
		if err != nil {
			return err
		}
		c.Return(script.Bool(ok))
	default:
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		list, err := h.Statuses(target, status)
		if err != nil {
			return err
		}
		if op == "has" {
			c.Return(script.Bool(len(list) > 0))
			return nil
		}
		out := make([]script.Value, len(list))
		for i, s := range list {
			fields := []script.Field{
				script.KV("status", script.Int(s.Status)), script.KV("level", script.Int(s.Level)),
				script.KV("values", script.List(script.Int(s.Values[0]), script.Int(s.Values[1]), script.Int(s.Values[2]))),
				script.KV("remaining_milliseconds", millis(s.Remaining)),
			}
			if s.Source != nil {
				fields = append(fields, script.KV("source", script.Int(s.Source.ID)), script.KV("source_kind", script.String(s.Source.Kind)))
			}
			out[i] = script.NewTable(fields...)
		}
		c.Return(script.List(out...))
	}
	return nil
}

// readStatusStart reads the options of start_status, which the prelude
// passes as plain data: the source handle is "<kind>:<id>".
func (r *Runtime) readStatusStart(what string, status int64, v script.Value) (StatusStart, error) {
	s := StatusStart{Status: status, Level: 1, Chance: 10000, Reason: "script"}
	if r.current != nil {
		switch r.current.Kind {
		case KindItem:
			s.Reason = "item"
		case KindSkill:
			s.Reason = "skill"
		}
	}
	if v.IsNil() {
		return s, nil
	}
	t, ok := v.AsTable()
	if !ok {
		return s, fmt.Errorf("%s: the options must be a table, such as { milliseconds = 60000, level = 10 }", what)
	}
	for _, f := range t.Fields {
		k, _ := f.Key.AsString()
		switch k {
		case "milliseconds":
			if str, ok := f.Value.AsString(); ok && str == "forever" {
				s.Milliseconds = Forever
				continue
			}
			n, ok := f.Value.AsInt()
			if !ok || n < 1 || n > MaxStatusMs {
				return s, fmt.Errorf("%s: milliseconds must be from 1 to %d, or \"forever\"", what, MaxStatusMs)
			}
			s.Milliseconds = n
		case "level":
			n, ok := f.Value.AsInt()
			if !ok || n < math.MinInt32 || n > math.MaxInt32 {
				return s, fmt.Errorf("%s: level must be an integer", what)
			}
			s.Level = n
		case "values":
			list, ok := f.Value.AsTable()
			if !ok || len(list.Fields) > 3 {
				return s, fmt.Errorf("%s: values must be a list of up to 3 integers: val2 to val4", what)
			}
			for i := range 3 {
				x := list.Index(int64(i + 1))
				if x.IsNil() {
					continue
				}
				n, ok := x.AsInt()
				if !ok || n < math.MinInt32 || n > math.MaxInt32 {
					return s, fmt.Errorf("%s: values must be a list of up to 3 integers: val2 to val4", what)
				}
				s.Values[i] = n
			}
		case "chance":
			n, ok := f.Value.AsInt()
			if !ok || n < 0 || n > 10000 {
				return s, fmt.Errorf("%s: chance must be from 0 to 10000, per 10,000", what)
			}
			s.Chance = n
		case "source":
			key, ok := f.Value.AsString()
			kind, id, found := strings.Cut(key, ":")
			var n int64
			if found {
				_, err := fmt.Sscan(id, &n)
				found = err == nil
			}
			if !ok || !found {
				return s, fmt.Errorf("%s: source must be a player, monster or NPC handle", what)
			}
			s.Source = &Entity{Kind: kind, ID: n}
		case "cannot_be_avoided", "keep_duration", "keep_chance", "hide_icon":
			b, ok := f.Value.AsBool()
			if !ok {
				return s, fmt.Errorf("%s: %s must be true or false", what, k)
			}
			switch k {
			case "cannot_be_avoided":
				s.CannotBeAvoided = b
			case "keep_duration":
				s.KeepDuration = b
			case "keep_chance":
				s.KeepChance = b
			default:
				s.HideIcon = b
			}
		default:
			return s, fmt.Errorf("%s: %q is not an option; the options are milliseconds, level, values, chance, source, cannot_be_avoided, keep_duration, keep_chance and hide_icon", what, k)
		}
	}
	return s, nil
}

// readBonusEntries reads bonus entries: a list of { name, arguments...,
// options }, as add_timed_bonuses and the bonuses function of a mod
// status give them. It returns them checked, and as plain data.
func (r *Runtime) readBonusEntries(what string, v script.Value) ([]bonus.Bonus, []any, error) {
	data, err := plainData(v, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: the bonuses: %w", what, err)
	}
	list, ok := data.([]any)
	if !ok {
		if m, isMap := data.(map[string]any); isMap && len(m) == 0 {
			return nil, []any{}, nil
		}
		return nil, nil, fmt.Errorf("%s: the bonuses must be a list of entries, such as { { \"str\", 5 } }", what)
	}
	if len(list) > MaxTimedBonuses {
		return nil, nil, fmt.Errorf("%s: at most %d bonuses", what, MaxTimedBonuses)
	}
	out := make([]bonus.Bonus, 0, len(list))
	for i, e := range list {
		b, err := bonus.FromData(e, r.bonusNames())
		if err != nil {
			return nil, nil, fmt.Errorf("%s: bonus %d: %w", what, i+1, err)
		}
		out = append(out, *b)
	}
	return out, list, nil
}

// plainData turns a Lua value of plain data into Go: a list is []any, a
// table with string keys map[string]any, and numbers int64.
func plainData(v script.Value, depth int) (any, error) {
	if depth > 8 {
		return nil, errors.New("tables nest too deeply")
	}
	switch v.Kind() {
	case script.KindInt:
		n, _ := v.AsInt()
		return n, nil
	case script.KindBool:
		b, _ := v.AsBool()
		return b, nil
	case script.KindString:
		s, _ := v.AsString()
		return s, nil
	case script.KindTable:
		t, _ := v.AsTable()
		if len(t.Fields) == 0 {
			return map[string]any{}, nil
		}
		if _, isList := t.Fields[0].Key.AsInt(); isList {
			out := make([]any, len(t.Fields))
			for _, f := range t.Fields {
				i, ok := f.Key.AsInt()
				if !ok || i < 1 || i > int64(len(t.Fields)) {
					return nil, errors.New("a list must have keys 1 to n")
				}
				x, err := plainData(f.Value, depth+1)
				if err != nil {
					return nil, err
				}
				out[i-1] = x
			}
			return out, nil
		}
		out := map[string]any{}
		for _, f := range t.Fields {
			k, ok := f.Key.AsString()
			if !ok {
				return nil, errors.New("a table must have string keys, or be a list")
			}
			x, err := plainData(f.Value, depth+1)
			if err != nil {
				return nil, err
			}
			out[k] = x
		}
		return out, nil
	}
	return nil, fmt.Errorf("a %s is not plain data", v.Kind())
}

// bonusNames finds the names that bonus entries may write: constants of
// the constant modules.
func (r *Runtime) bonusNames() bonus.Names {
	t := &bonus.Tables{Constants: map[string]map[string]int64{}}
	if r.opts.Constants != nil {
		for name, m := range r.opts.Constants.Modules() {
			t.Constants[name] = m.Values
		}
	}
	return t
}

// statusesDefine is statuses.define: the name, the icon, the kind, the
// ends_on names and tick_milliseconds. The prelude keeps the bonuses
// function and adds the constant.
func (r *Runtime) statusesDefine(c *script.Call) error {
	ctx, err := r.needContext("statuses.define")
	if err != nil {
		return err
	}
	if err := r.mutating("statuses.define"); err != nil {
		return err
	}
	if ctx.Kind != KindMod {
		return fmt.Errorf("statuses.define: define statuses in a mod script, and this is a %s context", ctx.Kind)
	}
	h, err := r.statusHost("statuses.define")
	if err != nil {
		return err
	}
	name := c.String(0)
	if !validStatusName(name) {
		return fmt.Errorf("statuses.define: the name must be 1 to %d lower case letters, digits and _, starting with a letter", MaxStatusName)
	}
	d := StatusDefinition{Mod: ctx.Mod, Name: name, Icon: -1}
	if n, ok := c.Arg(1).AsInt(); ok {
		if n < 0 || n > 1<<16 {
			return errors.New("statuses.define: icon must be a status icon ID (EFST_)")
		}
		d.Icon = n
	}
	switch kind, _ := c.Arg(2).AsString(); kind {
	case "", "buff":
	case "debuff":
		d.Debuff = true
	default:
		return errors.New(`statuses.define: kind is "buff" or "debuff"`)
	}
	if t, ok := c.Arg(3).AsTable(); ok {
		for i := range len(t.Fields) {
			s, ok := t.Index(int64(i + 1)).AsString()
			if !ok {
				return errors.New("statuses.define: ends_on must be a list of names, such as { \"death\" }")
			}
			d.EndsOn = append(d.EndsOn, s)
		}
	}
	if n, ok := c.Arg(4).AsInt(); ok {
		if n < 100 || n > MaxStatusMs {
			return fmt.Errorf("statuses.define: tick_milliseconds must be from 100 to %d", MaxStatusMs)
		}
		d.TickMs = n
	}
	id, err := h.DefineStatus(d)
	if err != nil {
		return err
	}
	c.Return(script.Int(id))
	return nil
}

func validStatusName(s string) bool {
	if s == "" || len(s) > MaxStatusName || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, ch := range s {
		if ch != '_' && (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

// ModStatusBonuses runs the bonuses function of a mod status for a
// level, as a status of it starts outside a script, such as at login. It
// returns nil for a status without one.
func (r *Runtime) ModStatusBonuses(status, level int64) ([]bonus.Bonus, error) {
	if r.fns.statusBonuses == nil {
		return nil, nil
	}
	prev := r.computing
	r.computing = true
	vals, err := r.s.Call(r.fns.statusBonuses.Value(), script.Int(status), script.Int(level))
	r.computing = prev
	if err != nil {
		r.afterError()
		return nil, err
	}
	if len(vals) == 0 || vals[0].IsNil() {
		return nil, nil
	}
	b, _, err := r.readBonusEntries("statuses.define bonuses", vals[0])
	return b, err
}

// playerAddTimedBonuses is player:add_timed_bonuses(key, milliseconds,
// bonuses, options).
func (r *Runtime) playerAddTimedBonuses(c *script.Call) error {
	const what = "player:add_timed_bonuses"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.statusHost(what)
	if err != nil {
		return err
	}
	t := TimedBonuses{Key: c.String(1), Icon: -1, Repeat: RepeatKeep}
	if t.Key == "" || len(t.Key) > 64 {
		return fmt.Errorf("%s: the key must be 1 to 64 bytes", what)
	}
	ms, ok := c.Arg(2).AsInt()
	if !ok || ms < 1 || ms > MaxStatusMs {
		return fmt.Errorf("%s: milliseconds must be from 1 to %d", what, MaxStatusMs)
	}
	t.Milliseconds = ms
	if t.Bonuses, t.Data, err = r.readBonusEntries(what, c.Arg(3)); err != nil {
		return err
	}
	if opts := c.Arg(4); !opts.IsNil() {
		o, ok := opts.AsTable()
		if !ok {
			return fmt.Errorf("%s: the options must be a table, such as { ends_on = { \"death\" } }", what)
		}
		for _, f := range o.Fields {
			k, _ := f.Key.AsString()
			switch k {
			case "ends_on":
				list, ok := f.Value.AsTable()
				if !ok {
					return fmt.Errorf("%s: ends_on must be a list of names", what)
				}
				for i := range len(list.Fields) {
					s, ok := list.Index(int64(i + 1)).AsString()
					if !ok {
						return fmt.Errorf("%s: ends_on must be a list of names", what)
					}
					t.EndsOn = append(t.EndsOn, s)
				}
			case "on_repeat":
				s, _ := f.Value.AsString()
				if s != RepeatKeep && s != RepeatExtend && s != RepeatAdd && s != RepeatReplace {
					return fmt.Errorf(`%s: on_repeat is "keep", "extend", "add" or "replace"`, what)
				}
				t.Repeat = s
			case "stack", "repeat":
				return fmt.Errorf("%s: the option %s is gone; on_repeat replaces it", what, k)
			case "icon":
				n, ok := f.Value.AsInt()
				if !ok || n < 0 || n > 1<<16 {
					return fmt.Errorf("%s: icon must be a status icon ID (EFST_)", what)
				}
				t.Icon = n
			case "kind":
				s, _ := f.Value.AsString()
				if s != "buff" && s != "debuff" {
					return fmt.Errorf(`%s: kind is "buff" or "debuff"`, what)
				}
				t.Kind = s
			default:
				return fmt.Errorf("%s: %q is not an option; the options are ends_on, on_repeat, icon and kind", what, k)
			}
		}
	}
	added, err := h.AddTimedBonuses(id, t)
	if err != nil {
		return err
	}
	c.Return(script.Bool(added))
	return nil
}

// hookChainBegin starts a hook chain that the prelude runs itself, for a
// status that a script starts: the chain of a hook name runs in its
// hooks' contexts, and computes only. It returns the number of hooks and
// their functions. hookChainEnd puts back the running context.
func (r *Runtime) hookChainBegin(c *script.Call) error {
	name := c.String(0)
	if name != "status_chance" && name != "status_duration" && name != "skill_damage" {
		return fmt.Errorf("no hook chain %q", name)
	}
	var chain []*hook
	for _, h := range r.hooks[name] {
		if !h.removed && !h.ctx.ended && !h.ctx.retiring && !r.off(h.ctx) && !r.overBudget(h.ctx) {
			chain = append(chain, h)
		}
	}
	r.saved = append(r.saved, r.beginChain(chain))
	out := []script.Value{script.Int(int64(len(chain))), script.Int(r.chainToken)}
	for _, h := range chain {
		out = append(out, h.fn.Value())
	}
	c.Return(out...)
	return nil
}

func (r *Runtime) hookChainEnd(*script.Call) error {
	if n := len(r.saved); n > 0 {
		s := r.saved[n-1]
		r.saved = r.saved[:n-1]
		r.endChain(s)
	}
	return nil
}

// savedChain is the state a hook chain that the prelude runs replaces.
type savedChain struct {
	current   *Context
	computing bool
	chain     []*hook
	token     int64
	depth     int
}

// computeBegin and computeEnd mark Lua code that only computes, such as
// the bonuses function of a mod status that a script starts: calls that
// change the game raise an error in it.
func (r *Runtime) computeBegin(*script.Call) error {
	r.computeStack = append(r.computeStack, r.computing)
	r.computing = true
	return nil
}

func (r *Runtime) computeEnd(*script.Call) error {
	if n := len(r.computeStack); n > 0 {
		r.computing = r.computeStack[n-1]
		r.computeStack = r.computeStack[:n-1]
	}
	return nil
}
