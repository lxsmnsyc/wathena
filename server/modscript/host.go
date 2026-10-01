package modscript

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// The host functions behind the API modules. prelude.lua wraps each one,
// so errors point at the script's line.

// needContext returns the running context, or an error that says the
// call needs one.
func (r *Runtime) needContext(what string) (*Context, error) {
	if r.current == nil || r.current.ended {
		return nil, fmt.Errorf("%s needs a context: call it inside a setup function or a handler, not while the file loads", what)
	}
	return r.current, nil
}

// actor is the mod whose code runs now: the context's mod, or the mod
// whose file loads.
func (r *Runtime) actor() string {
	if r.current != nil {
		return r.current.Mod
	}
	return r.loading
}

// need checks that the running mod holds a permission.
func (r *Runtime) need(what, permission string) error {
	id := r.actor()
	sc := r.scripts[id]
	if sc == nil || !sc.Grants.Has(permission) {
		return fmt.Errorf("%s needs the permission %s, which %s does not have; request it in mod.yaml, and the server owner approves it", what, permission, id)
	}
	return nil
}

// resolve finds the file a module name names, for require. A name is
// <mod ID>.<path>: example.halloween.lib.rewards is scripts/lib/rewards.lua
// of example.halloween. The longest mod ID that matches wins. A mod may
// only require its own files and those of the mods in its depends.
func (r *Runtime) resolve(c *script.Call) error {
	from, name := c.String(0), c.String(1)
	sc := r.scripts[from]
	if sc == nil {
		return fmt.Errorf("require: %s has no scripts", from)
	}
	owner := longestPrefix(name, sc.Visible)
	if owner == "" {
		if other := longestPrefix(name, r.opts.Mods); other != "" {
			return fmt.Errorf("require: %s cannot load %s: %s is not in its depends", from, name, other)
		}
		return fmt.Errorf("require: no mod has the module %s; mod files are named <mod ID>.<path>, such as %s.lib.rewards for scripts/lib/rewards.lua", name, from)
	}
	path, ok := mod.ModulePath(strings.TrimPrefix(name, owner+"."))
	if !ok {
		return fmt.Errorf("require: %s is not a module name: after the mod ID come names of letters, digits, _ and -, separated by dots", name)
	}
	osc := r.scripts[owner]
	if osc == nil {
		return fmt.Errorf("require: module %s not found: %s has no scripts", name, owner)
	}
	src, ok := osc.Files[path]
	if !ok {
		return fmt.Errorf("require: module %s not found: %s has no %s", name, owner, path)
	}
	c.Return(script.String(owner), script.String(path), script.String(src))
	return nil
}

func longestPrefix(name string, ids []string) string {
	best := ""
	for _, id := range ids {
		if strings.HasPrefix(name, id+".") && len(id) > len(best) {
			best = id
		}
	}
	return best
}

func (r *Runtime) contextInstance(c *script.Call) error {
	ctx, err := r.needContext("context:get_instance")
	if err != nil {
		return err
	}
	c.Return(ctx.Instance)
	return nil
}

func (r *Runtime) contextKind(c *script.Call) error {
	ctx, err := r.needContext("context:get_kind")
	if err != nil {
		return err
	}
	c.Return(script.String(ctx.Kind))
	return nil
}

func (r *Runtime) contextMod(c *script.Call) error {
	ctx, err := r.needContext("context:get_mod")
	if err != nil {
		return err
	}
	c.Return(script.String(ctx.Mod))
	return nil
}

func (r *Runtime) eventsOn(c *script.Call) error {
	ctx, err := r.needContext("events.on")
	if err != nil {
		return err
	}
	if err := r.need("events.on", "event.listen"); err != nil {
		return err
	}
	name := c.String(0)
	if err := checkEvent(name, ctx.Kind); err != nil {
		return err
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	ref, err := c.State().Keep(c.Arg(1))
	if err != nil {
		return err
	}
	h := &handler{id: r.newID(), event: name, ctx: ctx, fn: ref}
	ctx.handlers = append(ctx.handlers, h)
	r.handlers[name] = append(r.handlers[name], h)
	r.byID[h.id] = h
	c.Return(script.Int(h.id))
	return nil
}

// room checks that a context can own one more registration.
func (r *Runtime) room(ctx *Context) error {
	if n := len(ctx.handlers) + len(ctx.timers); n >= MaxRegistrations {
		return fmt.Errorf("the context has %d handlers and timers, the most it may have", n)
	}
	return nil
}

func (r *Runtime) eventsRemove(c *script.Call) error {
	h := r.byID[c.Int(0)]
	if h == nil {
		// Removing twice, or after the context ended, does nothing.
		return nil
	}
	r.removeHandler(h)
	h.ctx.handlers = removeFrom(h.ctx.handlers, h)
	return nil
}

func removeFrom(list []*handler, h *handler) []*handler {
	for i, x := range list {
		if x == h {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

func (r *Runtime) timersStart(c *script.Call) error {
	repeat := c.Bool(2)
	what := "timers.after"
	if repeat {
		what = "timers.every"
	}
	ctx, err := r.needContext(what)
	if err != nil {
		return err
	}
	ms := c.Int(0)
	switch {
	case repeat && ms <= 0:
		return fmt.Errorf("%s: milliseconds must be above 0", what)
	case ms < 0:
		return fmt.Errorf("%s: milliseconds cannot be negative", what)
	case ms > 1<<40:
		return fmt.Errorf("%s: milliseconds is too large", what)
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	ref, err := c.State().Keep(c.Arg(1))
	if err != nil {
		return err
	}
	t := &timer{id: r.newID(), ctx: ctx, due: r.host.Now() + ms, fn: ref}
	if repeat {
		t.every = ms
	}
	ctx.timers[t.id] = t
	r.timers[t.id] = t
	c.Return(script.Int(t.id))
	return nil
}

func (r *Runtime) timersCancel(c *script.Call) error {
	t := r.timers[c.Int(0)]
	if t == nil {
		return nil
	}
	r.dropTimer(t)
	t.fn.Release()
	return nil
}

func (r *Runtime) itemsID(c *script.Call) error {
	if err := r.need("items.id", "game.read"); err != nil {
		return err
	}
	name := c.String(0)
	id, ok := r.host.ItemID(name)
	if !ok {
		return fmt.Errorf("items.id: there is no item %q", name)
	}
	c.Return(script.Int(int64(id)))
	return nil
}

// playerID reads a player handle's ID.
func playerID(c *script.Call) (int64, error) {
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return 0, errors.New("not a player: call the method with a colon, such as player:send_message(text)")
	}
	return id, nil
}

func (r *Runtime) playerField(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need("reading a player", "game.read"); err != nil {
		return err
	}
	v, err := r.host.PlayerField(id, c.String(1))
	if err != nil {
		return err
	}
	c.Return(v)
	return nil
}

func (r *Runtime) playerSendMessage(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	text := c.String(1)
	if text == "" || len(text) > MaxMessage {
		return fmt.Errorf("player:send_message: the text must be 1 to %d bytes", MaxMessage)
	}
	return r.host.SendMessage(id, text)
}

// MaxMessage is the longest text send_message accepts, in bytes. It is
// the chat limit of the web client.
const MaxMessage = 255

func (r *Runtime) playerGiveItem(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need("player:give_item", "player.give_items"); err != nil {
		return err
	}
	item, amount := c.Int(1), int64(1)
	if c.NArgs() > 2 && !c.Arg(2).IsNil() {
		amount = c.Int(2)
	}
	if item <= 0 || item > 1<<32-1 || amount < 1 || amount > 30000 {
		return errors.New("player:give_item: want an item ID from items.id and an amount from 1 to 30000")
	}
	ok, err := r.host.GiveItem(id, uint32(item), int(amount)) // #nosec G115 -- checked above.
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

func (r *Runtime) playerCountItem(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need("player:count_item", "game.read"); err != nil {
		return err
	}
	item := c.Int(1)
	if item <= 0 || item > 1<<32-1 {
		return errors.New("player:count_item: want an item ID from items.id")
	}
	n, err := r.host.CountItem(id, uint32(item)) // #nosec G115 -- checked above.
	if err != nil {
		return err
	}
	c.Return(script.Int(int64(n)))
	return nil
}
