package modscript

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/modscript/luaapi"
)

// Players across zones (review decision 59): a handle of a player of
// another zone reads the fields the world keeps of the player, and its
// methods that act go to the player's zone as messages, as rAthena's
// attachrid and the commands that take a character or account ID act on
// a player of any map. Methods that read raise an error there, since a
// zone cannot wait on another (decision 0062).

// RemotePlayerHost is a host whose player handles may name players of
// other zones.
type RemotePlayerHost interface {
	// RemotePlayerCall sends a player method to the zone of a player of
	// another map, where it runs after the current step in the calling
	// mod's name. local is true when the player is in this zone, so the
	// runtime runs the method here. A player who went offline gets
	// nothing.
	RemotePlayerCall(player int64, mod, method string, args []script.Value) (local bool, err error)
	// RemotePlayerExists reports whether the player of a handle is
	// online, on any map.
	RemotePlayerExists(player int64) bool
}

// remotePlayerMethods are the permissions of wa.Player's methods, from
// the API definitions: a method of game.read reads the player.
var remotePlayerMethods = sync.OnceValue(func() map[string]string {
	out := map[string]string{}
	for _, c := range luaapi.Classes {
		if c.Name != "wa.Player" {
			continue
		}
		for _, m := range c.Methods {
			out[m.Name] = m.Permission
		}
	}
	return out
})

// playerRemote is the prelude's check of a player method on a handle of
// a player of another zone: Arg(0) is the handle, Arg(1) the method, and
// the rest its arguments, plain values only. It returns true when the
// player is in this zone, so the prelude runs the method itself, and
// nothing when the call went to the player's zone. exists returns its
// answer.
func (r *Runtime) playerRemote(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	method := c.String(1)
	what := "player:" + method
	h, ok := r.host.(RemotePlayerHost)
	if !ok {
		return errors.New(what + ": the player is on another map")
	}
	if method == "exists" {
		c.Return(script.Bool(h.RemotePlayerExists(id)))
		return nil
	}
	perm, known := remotePlayerMethods()[method]
	if !known {
		return fmt.Errorf("player has no method %q", method)
	}
	args := make([]script.Value, 0, c.NArgs()-2)
	for i := 2; i < c.NArgs(); i++ {
		args = append(args, c.Arg(i))
	}
	if perm == "game.read" {
		local, err := h.RemotePlayerCall(id, r.actor(), "", nil)
		if err != nil {
			return wrap(what, err)
		}
		if !local {
			return fmt.Errorf("%s: the player is on another map: a read runs on the player's own map", what)
		}
		c.Return(script.Bool(true))
		return nil
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if perm != "" {
		if err := r.need(what, perm); err != nil {
			return err
		}
	}
	local, err := h.RemotePlayerCall(id, r.actor(), method, args)
	if err != nil {
		return wrap(what, err)
	}
	if local {
		c.Return(script.Bool(true))
	}
	return nil
}

// RunPlayerCall runs a player method that another zone sent, in the
// name of mod: in its mod context, or in a context of its own for a mod
// without entry scripts. player is the zone's handle of the player.
func (r *Runtime) RunPlayerCall(mod string, player int64, method string, args []script.Value) error {
	sc := r.scripts[mod]
	if sc == nil {
		return fmt.Errorf("the mod %s has no scripts in this zone", mod)
	}
	ctx := r.contexts[mod]
	if ctx == nil || ctx.ended {
		if r.callCtx == nil {
			r.callCtx = map[string]*Context{}
		}
		ctx = r.callCtx[mod]
		if ctx == nil || ctx.ended {
			ctx = &Context{ID: r.newID(), Kind: KindMod, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}}
			ctx.Instance = script.NewTable(script.KV("id", script.String(mod)), script.KV("map", script.String(r.host.Map())))
			r.callCtx[mod] = ctx
		}
	}
	if r.off(ctx) {
		return nil
	}
	vals := append([]script.Value{script.Int(player), script.String(method)}, args...)
	return r.runHandler(ctx, r.fns.playerCall.Value(), vals...)
}

// FireNPCs runs the handlers of an event in NPC contexts alone: a player
// event of another zone, which mod contexts of this zone do not hear,
// since they hear their own map's players.
func (r *Runtime) FireNPCs(event string, args ...Arg) {
	list := slices.Clone(r.handlers[event])
	vals := make([]script.Value, 0, 1+2*len(args))
	for _, h := range list {
		if h.removed || h.ctx.ended || h.ctx.Kind != KindNPC || r.off(h.ctx) || skipsDisabled(h, event) {
			continue
		}
		vals = append(vals[:0], h.fn.Value())
		for _, a := range args {
			vals = append(vals, script.String(a.kind), a.v)
		}
		if err := r.runHandler(h.ctx, r.fns.invoke.Value(), vals...); err != nil {
			r.failed(h.ctx, "event "+event, err)
		}
	}
}

// PlayerEvents are the events of one player that reach the NPC handlers
// of every map (review decision 59).
var PlayerEvents = []string{"login", "logout", "map_enter", "base_level_up", "job_level_up", "death"}

// eventListened tells the host that an NPC context gained or lost a
// handler of a player event.
func (r *Runtime) eventListened(h *handler, add bool) {
	if h.ctx.Kind != KindNPC || r.opts.NPCEvents == nil || !slices.Contains(PlayerEvents, h.event) {
		return
	}
	r.opts.NPCEvents(h.event, add)
}
