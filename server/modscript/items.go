package modscript

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/bonus"
)

// Item contexts (proposal 0002, "Item scripts"):
//   - A use runs the item's setup function in a new item context, then
//     its "use" handlers with the player, then the context ends.
//   - An equipped copy runs the setup function when it is equipped. The
//     context lives until the copy is unequipped. Its bonuses go to a
//     BonusSink, which the zone reads when it computes the wearer's
//     stats.

// Bonus is one bonus that an item context added.
type Bonus struct {
	bonus.Bonus
	// Context owns the bonus. Timer is the with_bonuses timer it belongs
	// to, or 0 for the context itself.
	Context *Context
	Timer   int64
	// computed holds the functions of computed arguments, by index;
	// chance a computed chance; and when the condition of the when
	// option.
	computed map[int]*script.Ref
	chance   *script.Ref
	when     *script.Ref
	dropped  bool
	// id is the bonus's handle ID.
	id int64
}

// Computed reports whether the bonus has a function to run: a computed
// value or a condition.
func (b *Bonus) Computed() bool { return len(b.computed) > 0 || b.chance != nil || b.when != nil }

// BonusSink gets the bonuses of an equip context: when they are added,
// and when they end.
type BonusSink interface {
	BonusAdded(b *Bonus)
	BonusRemoved(b *Bonus)
}

// StatsHost computes stats again. The host calls that add or remove
// bonuses call FlushStats before they return, so the script reads the
// new stats after the call, as rAthena's status_calc_pc runs at once.
type StatsHost interface {
	FlushStats()
}

// flushStats computes again the stats that the call's bonuses changed.
func (r *Runtime) flushStats() {
	if h, ok := r.host.(StatsHost); ok {
		h.FlushStats()
	}
}

// UseItem runs a usable item's script for a player. script is the item's
// entry script as <mod>:<path>. It returns an error when the script
// fails, after its effects so far stay.
func (r *Runtime) UseItem(player int64, itemID uint32, entry string) error {
	setup, mod, err := r.setup(entry)
	if err != nil {
		return err
	}
	sc := r.scripts[mod]
	c := &Context{ID: r.newID(), Kind: KindItem, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}, player: player}
	c.Instance = script.NewTable(script.KV("item", script.Int(int64(itemID))))
	defer r.end(c)
	if err := r.run(c, setup); err != nil {
		return fmt.Errorf("%s: setup: %w", entry, err)
	}
	return r.fireContext(c, "use", Player(player))
}

// Equip runs an equipment item's script for an equipped copy. equipped
// is the copy's handle ID, which the Host's EquippedField reads; wearer
// is the player. The returned context ends with Unequip.
func (r *Runtime) Equip(entry string, equipped, wearer int64, sink BonusSink) (*Context, error) {
	setup, mod, err := r.setup(entry)
	if err != nil {
		return nil, err
	}
	sc := r.scripts[mod]
	c := &Context{ID: r.newID(), Kind: KindItem, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}, sink: sink, player: wearer}
	c.Instance = script.NewTable(script.KV("equipped", script.Int(equipped)))
	if err := r.run(c, setup); err != nil {
		r.end(c)
		return nil, fmt.Errorf("%s: setup: %w", entry, err)
	}
	return c, nil
}

// Unequip fires the context's unequip handlers, then ends it, which
// removes its bonuses, handlers and timers.
func (r *Runtime) Unequip(c *Context) error {
	if c.ended {
		return nil
	}
	err := r.fireContext(c, "unequip", Player(c.player))
	r.end(c)
	return err
}

// EndEquip ends an equip context without its unequip handlers, as when
// the wearer leaves the map: the copy is not unequipped, so a new context
// starts where the wearer arrives.
func (r *Runtime) EndEquip(c *Context) {
	if !c.ended {
		r.end(c)
	}
}

// Listens reports whether a context has handlers for an event, so the
// zone builds an event's object only when a handler will read it.
func (r *Runtime) Listens(c *Context, event string) bool {
	if c.ended {
		return false
	}
	for _, h := range c.handlers {
		if !h.removed && h.event == event {
			return true
		}
	}
	return false
}

// Hit is the damage object of the "damage" item event (0002a, "Item
// events"). Source and Target are player or monster handle IDs, as their
// kinds say.
type Hit struct {
	Source, Target         int64
	SourceKind, TargetKind string // "player" or "monster"
	Kind, Range            string
	Skill                  int64 // 0 for a normal attack
	Element                int64
	Critical               bool
	Amount                 int64
	WearerIsSource         bool
}

// SkillUse is the object of the "skill_used" item event.
type SkillUse struct {
	Skill, Level int64
	Target       int64 // a player handle ID, or 0 for a ground skill
	X, Y         int64
}

// FireHit runs a context's damage handlers.
func (r *Runtime) FireHit(c *Context, h Hit) error {
	fields := []script.Field{
		script.KV("source", script.Int(h.Source)), script.KV("source_kind", script.String(h.SourceKind)),
		script.KV("target", script.Int(h.Target)), script.KV("target_kind", script.String(h.TargetKind)),
		script.KV("kind", script.String(h.Kind)), script.KV("element", script.Int(h.Element)),
		script.KV("is_critical", script.Bool(h.Critical)), script.KV("amount", script.Int(h.Amount)),
		script.KV("wearer_is_source", script.Bool(h.WearerIsSource)),
	}
	if h.Range != "" {
		fields = append(fields, script.KV("range", script.String(h.Range)))
	}
	if h.Skill != 0 {
		fields = append(fields, script.KV("skill", script.Int(h.Skill)))
	}
	return r.fireContext(c, "damage", Arg{"hit", script.NewTable(fields...)})
}

// FireSkillUse runs a context's skill_used handlers.
func (r *Runtime) FireSkillUse(c *Context, u SkillUse) error {
	fields := []script.Field{
		script.KV("skill", script.Int(u.Skill)), script.KV("level", script.Int(u.Level)),
		script.KV("x", script.Int(u.X)), script.KV("y", script.Int(u.Y)),
	}
	if u.Target != 0 {
		fields = append(fields, script.KV("target", script.Int(u.Target)), script.KV("target_kind", script.String("player")))
	}
	return r.fireContext(c, "skill_used", Arg{"skill_use", script.NewTable(fields...)})
}

// setup loads an item entry script once and returns its setup function.
func (r *Runtime) setup(entry string) (script.Value, string, error) {
	mod, path, ok := strings.Cut(entry, ":")
	if !ok {
		return script.Nil, "", fmt.Errorf("item script %q is not <mod>:<path>", entry)
	}
	if ref := r.setups[entry]; ref != nil {
		return ref.Value(), mod, nil
	}
	sc := r.scripts[mod]
	if sc == nil {
		return script.Nil, "", fmt.Errorf("item script %s: %s has no scripts in this zone", entry, mod)
	}
	src, ok := sc.Files[path]
	if !ok {
		return script.Nil, "", fmt.Errorf("item script %s: %s has no %s", entry, mod, path)
	}
	r.loading = mod
	fn, err := r.s.Call1(r.fns.loadEntry.Value(), script.String(mod), script.String(path), script.String(src))
	r.loading = ""
	if err != nil {
		r.afterError()
		return script.Nil, "", err
	}
	ref, err := r.s.Keep(fn)
	if err != nil {
		return script.Nil, "", err
	}
	r.setups[entry] = ref
	r.setupOrder = append(r.setupOrder, entry)
	for len(r.setupOrder) > MaxItemScripts {
		old := r.setupOrder[0]
		r.setupOrder = r.setupOrder[1:]
		r.dropSetup(old)
	}
	return ref.Value(), mod, nil
}

// dropSetup forgets a loaded item script.
func (r *Runtime) dropSetup(entry string) {
	ref := r.setups[entry]
	if ref == nil {
		return
	}
	delete(r.setups, entry)
	ref.Release()
	mod, path, _ := strings.Cut(entry, ":")
	if _, err := r.s.Call(r.fns.dropFile.Value(), script.String(mod), script.String(path)); err != nil {
		r.log.Error("modscript: dropping an item script failed", "script", entry, "err", err)
	}
}

// fireContext runs the handlers of one context for an event, in
// registration order. Every handler runs; the first error is returned.
func (r *Runtime) fireContext(c *Context, event string, args ...Arg) error {
	var first error
	for _, h := range append([]*handler(nil), c.handlers...) {
		if h.removed || h.event != event || c.ended {
			continue
		}
		vals := []script.Value{h.fn.Value()}
		for _, a := range args {
			vals = append(vals, script.String(a.kind), a.v)
		}
		if err := r.run(c, r.fns.invoke.Value(), vals...); err != nil {
			r.failed(c, "event "+event, err)
			if first == nil {
				first = err
			}
		}
	}
	return first
}

// Values runs a bonus's computed parts: the numbers, the chance, and
// whether the when condition holds. They run in the bonus's context,
// and calls that change the game are refused while they run.
func (r *Runtime) Values(b *Bonus) (args []int64, chance int64, active bool, err error) {
	if b.dropped {
		return nil, 0, false, errors.New("the bonus has ended")
	}
	prev, prevComputing := r.current, r.computing
	r.current, r.computing = b.Context, true
	defer func() { r.current, r.computing = prev, prevComputing }()
	// Inside a script's host call, such as start_status, these are
	// nested calls (internal/script, "Nested calls").
	num := func(ref *script.Ref, what string) (int64, error) {
		v, err := r.s.Call1Nested(ref.Value())
		if err != nil {
			r.afterError()
			return 0, err
		}
		n, ok := v.AsInt()
		if !ok {
			return 0, fmt.Errorf("bonuses.add(%q): the %s function returned %s, not an integer", b.Name, what, v.Kind())
		}
		return n, nil
	}
	active = true
	if b.when != nil {
		v, err := r.s.Call1Nested(b.when.Value())
		if err != nil {
			r.afterError()
			return nil, 0, false, err
		}
		active = v.Truthy()
	}
	for i, a := range b.Args {
		if ref := b.computed[i]; ref != nil {
			n, err := num(ref, fmt.Sprintf("argument %d", i+1))
			if err != nil {
				return nil, 0, false, err
			}
			args = append(args, n)
			continue
		}
		args = append(args, a.Value)
	}
	chance = b.Options.Chance.Value
	if b.chance != nil {
		if chance, err = num(b.chance, "chance"); err != nil {
			return nil, 0, false, err
		}
	}
	return args, chance, active, nil
}

// dropBonus ends a bonus: the sink hears of it, and its functions are
// freed.
func (r *Runtime) dropBonus(b *Bonus) {
	if b.dropped {
		return
	}
	b.dropped = true
	delete(r.bonuses, b.id)
	if s := b.Context.sink; s != nil {
		s.BonusRemoved(b)
	}
	for _, ref := range b.computed {
		ref.Release()
	}
	for _, ref := range []*script.Ref{b.chance, b.when} {
		if ref != nil {
			ref.Release()
		}
	}
}

// endTimerBonuses ends the bonuses of a with_bonuses timer.
func (r *Runtime) endTimerBonuses(t *timer) {
	for _, b := range t.bonuses {
		r.dropBonus(b)
	}
	t.bonuses = nil
}

// mutating refuses a call that changes the game while a computed bonus,
// a hook, or a condition runs.
func (r *Runtime) mutating(what string) error {
	if r.computing {
		return fmt.Errorf("%s cannot run in a computed bonus, a hook or a condition: they only return a value", what)
	}
	return nil
}

// bonusesAdd is bonuses.add. Arguments: the name,
// the options without a computed chance, the computed chance, the
// when condition, the count of values, then the values.
func (r *Runtime) bonusesAdd(c *script.Call) error {
	name := c.String(0)
	what := fmt.Sprintf("bonuses.add(%q)", name)
	ctx, err := r.needContext(what)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if ctx.Kind != KindItem || ctx.sink == nil {
		return fmt.Errorf("%s: bonuses belong to an equipped item: call it in an equip script's setup, or in timers.with_bonuses", what)
	}
	b := &Bonus{Bonus: bonus.Bonus{Name: name, Options: bonus.Options{Set: map[string]bool{}}}, Context: ctx, computed: map[int]*script.Ref{}}
	release := func() {
		for _, ref := range b.computed {
			ref.Release()
		}
		for _, ref := range []*script.Ref{b.chance, b.when} {
			if ref != nil {
				ref.Release()
			}
		}
	}
	n := int(c.Int(4))
	for i := range n {
		v := c.Arg(5 + i)
		if x, ok := v.AsInt(); ok {
			b.Args = append(b.Args, bonus.Num{Value: x})
			continue
		}
		ref, err := c.State().Keep(v)
		if err != nil {
			release()
			return err
		}
		b.computed[i] = ref
		b.Args = append(b.Args, bonus.Num{Computed: true})
	}
	if opts, ok := c.Arg(1).AsTable(); ok {
		if err := readOptions(&b.Options, opts); err != nil {
			release()
			return fmt.Errorf("%s: %w", what, err)
		}
	}
	if v := c.Arg(2); !v.IsNil() {
		ref, err := c.State().Keep(v)
		if err != nil {
			release()
			return err
		}
		b.chance = ref
		b.Options.Chance = bonus.Num{Computed: true}
		b.Options.Set["chance"] = true
	}
	if v := c.Arg(3); !v.IsNil() {
		ref, err := c.State().Keep(v)
		if err != nil {
			release()
			return err
		}
		b.when = ref
		b.Options.When = true
		b.Options.Set["when"] = true
	}
	if err := bonus.Check(&b.Bonus); err != nil {
		release()
		return err
	}
	if r.group != nil {
		b.Timer = r.group.id
		r.group.bonuses = append(r.group.bonuses, b)
	} else {
		ctx.bonuses = append(ctx.bonuses, b)
	}
	b.id = r.newID()
	r.bonuses[b.id] = b
	ctx.sink.BonusAdded(b)
	r.flushStats()
	c.Return(script.Int(b.id))
	return nil
}

// bonusesRemove is the remove method of the handle bonuses.add returns.
// It removes that one bonus. A bonus that has already ended, by an
// earlier remove, its timer, or its context, is left as it is.
func (r *Runtime) bonusesRemove(c *script.Call) error {
	if err := r.mutating("bonus:remove"); err != nil {
		return err
	}
	b := r.bonuses[c.Int(0)]
	if b == nil || b.dropped {
		return nil
	}
	if t := r.timers[b.Timer]; b.Timer != 0 && t != nil {
		t.bonuses = slices.DeleteFunc(t.bonuses, func(x *Bonus) bool { return x == b })
	} else {
		b.Context.bonuses = slices.DeleteFunc(b.Context.bonuses, func(x *Bonus) bool { return x == b })
	}
	r.dropBonus(b)
	r.flushStats()
	return nil
}

// readOptions reads the options table of bonuses.add.
func readOptions(o *bonus.Options, t *script.Table) error {
	str := func(key string, v script.Value) (string, error) {
		s, ok := v.AsString()
		if !ok {
			return "", fmt.Errorf("%s must be a string", key)
		}
		return s, nil
	}
	num := func(key string, v script.Value) (int64, error) {
		n, ok := v.AsInt()
		if !ok {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return n, nil
	}
	for _, f := range t.Fields {
		key, ok := f.Key.AsString()
		if !ok {
			return errors.New("option names are strings")
		}
		o.Set[key] = true
		v := f.Value
		var err error
		switch key {
		case "percent", "random_level":
			if b, ok := v.AsBool(); !ok || !b {
				return fmt.Errorf("%s must be true", key)
			}
			if key == "percent" {
				o.Percent = true
			} else {
				o.RandomLevel = true
			}
		case "kind":
			o.Kind, err = str(key, v)
		case "range":
			o.Range, err = str(key, v)
		case "attack":
			o.Attack, err = str(key, v)
		case "on":
			o.On, err = str(key, v)
		case "target":
			o.Target, err = str(key, v)
		case "stacking":
			o.Stacking, err = str(key, v)
		case "skill":
			o.Skill, err = num(key, v)
		case "item":
			o.Item, err = num(key, v)
		case "item_group":
			o.ItemGroup, err = num(key, v)
		case "element":
			o.Element, err = num(key, v)
		case "weapon_type":
			o.WeaponType, err = num(key, v)
		case "chance":
			var n int64
			n, err = num(key, v)
			o.Chance = bonus.Num{Value: n}
		case "duration":
			o.Duration, err = num(key, v)
		case "priority":
			o.Priority, err = num(key, v)
		case "against", "from":
			tt, ok := v.AsTable()
			if !ok || len(tt.Fields) != 1 {
				return fmt.Errorf("%s is a table with one key, such as { race = races.demi_human }", key)
			}
			k, _ := tt.Fields[0].Key.AsString()
			n, ok := tt.Fields[0].Value.AsInt()
			if !ok {
				return fmt.Errorf("%s.%s must be a constant or an ID", key, k)
			}
			target := &bonus.Target{Key: k, Value: n}
			if key == "against" {
				o.Against = target
			} else {
				o.From = target
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// timersBonusStart starts a with_bonuses timer and collects the bonuses
// its function adds. Arguments: milliseconds, and the timer to restart
// or nil.
func (r *Runtime) timersBonusStart(c *script.Call) error {
	ctx, err := r.needContext("timers.with_bonuses")
	if err != nil {
		return err
	}
	if err := r.mutating("timers.with_bonuses"); err != nil {
		return err
	}
	ms := c.Int(0)
	if ms < 0 || ms > 1<<40 {
		return errors.New("timers.with_bonuses: milliseconds must be from 0 to 2^40")
	}
	if id, ok := c.Arg(1).AsInt(); ok {
		if t := r.timers[id]; t != nil && t.ctx == ctx {
			r.dropTimer(t)
			r.endTimerBonuses(t)
			r.flushStats()
		}
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	if r.group != nil {
		return errors.New("timers.with_bonuses cannot run inside another with_bonuses")
	}
	t := &timer{id: r.newID(), ctx: ctx, due: r.host.Now() + ms}
	ctx.timers[t.id] = t
	r.timers[t.id] = t
	r.group = t
	c.Return(script.Int(t.id))
	return nil
}

// timersBonusDone ends the collection. A function that failed leaves no
// bonuses.
func (r *Runtime) timersBonusDone(c *script.Call) error {
	t := r.group
	r.group = nil
	if t == nil || t.id != c.Int(0) {
		return nil
	}
	if !c.Bool(1) {
		r.dropTimer(t)
		r.endTimerBonuses(t)
		r.flushStats()
	}
	return nil
}
