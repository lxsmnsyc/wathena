package modscript

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Waits in handlers without a conversation (review decision 60, A4).
//
//   - The handlers of an NPC context that run without a conversation run
//     in a thread: the npc_event handlers sent without a player, those
//     sent with a player of another map, and the callbacks of timers.
//     timers.wait pauses the thread, and a timer resumes it.
//   - The wait follows the clock the handler started on: the zone clock,
//     or the server clock for the callback of a timer with
//     { clock = "server" }.
//   - A handler with a conversation waits with conversation:wait. Other
//     contexts' handlers run in no thread, and timers.wait raises an
//     error there.
//   - A context that ends closes the threads that wait in it.

// waiter is a handler thread that may wait.
type waiter struct {
	ctx    *Context
	thread *script.Thread
	// server is set when the handler follows the server clock.
	server bool
}

// runWaitable calls fn like run does. In an NPC context it runs fn in a
// thread, so that timers.wait can pause it. server is the clock of its
// waits.
func (r *Runtime) runWaitable(c *Context, server bool, fn script.Value, args ...script.Value) error {
	if c.Kind != KindNPC {
		return r.run(c, fn, args...)
	}
	th, err := r.s.NewThread(fn)
	if err != nil {
		return err
	}
	return r.resumeWaiter(&waiter{ctx: c, thread: th, server: server}, args...)
}

// resumeWaiter goes on with a handler thread, until it waits or ends.
func (r *Runtime) resumeWaiter(w *waiter, args ...script.Value) error {
	prev, prevWaiting, prevTalk := r.current, r.waiting, r.talking
	r.setCurrent(w.ctx)
	r.waiting, r.talking = w, nil
	_, _, err := w.thread.Resume(args...)
	r.waiting, r.talking = prevWaiting, prevTalk
	r.setCurrent(prev)
	if err != nil {
		r.afterError()
	}
	return err
}

func (r *Runtime) timersWait(c *script.Call) error {
	const what = "timers.wait"
	w := r.waiting
	switch {
	case r.talking != nil:
		return errors.New(what + " waits in a handler without a conversation: use conversation:wait")
	case w == nil || r.current != w.ctx || !c.CanYield():
		return errors.New(what + " runs only in a handler of an NPC context that has no conversation: an npc_event handler or a timer callback, not inside table.sort")
	case w.ctx.ended || w.ctx.retiring:
		return errors.New(what + ": the NPC's script ended")
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	ms, ok := c.Arg(0).AsInt()
	if !ok || ms < 0 || ms > MaxWait {
		return fmt.Errorf("%s: milliseconds are 0 to %d", what, MaxWait)
	}
	due := r.host.Now() + ms
	if w.server {
		now, err := r.clockNow(what)
		if err != nil {
			return err
		}
		due = now.UnixMilli() + ms
	}
	t := &timer{id: r.newID(), ctx: w.ctx, due: due, server: w.server, wait: w}
	w.ctx.timers[t.id] = t
	r.timers[t.id] = t
	return c.Yield()
}

// PlayerTimerHost is what player:add_timer and player:remove_timer reach
// in the zone (review decision 60, C2). The zone keeps the timers with
// the player.
type PlayerTimerHost interface {
	// AddPlayerTimer starts a timer of a player on the NPC named name, or
	// on the NPC with the handle npc when name is empty. modID is the
	// mod that gave the name.
	AddPlayerTimer(player, ms, npc int64, name, modID, label string) error
	// RemovePlayerTimer ends the first such timer of the player. It
	// reports whether one ended.
	RemovePlayerTimer(player, npc int64, name, modID, label string) (bool, error)
}

// MaxPlayerTimer is the longest time of player:add_timer, in
// milliseconds.
const MaxPlayerTimer = 1 << 40

// timerTarget reads the label of add_timer and remove_timer: the NPC's
// name before ::, or the context's NPC.
func (r *Runtime) timerTarget(what string, v script.Value) (npc int64, name, label string, err error) {
	s, ok := v.AsString()
	bad := fmt.Errorf("%s: want an event label, such as OnTimeout or Guard::OnTimeout", what)
	if !ok || len(s) > 255 {
		return 0, "", "", bad
	}
	if n, l, found := strings.Cut(s, "::"); found {
		if n == "" || !labelName.MatchString(l) {
			return 0, "", "", bad
		}
		// Another mod's NPC needs npcs.control, as npc:fire_event.
		if m, _, hasMod := strings.Cut(n, ":"); hasMod && m != r.actor() {
			if err := r.need(what, "npcs.control"); err != nil {
				return 0, "", "", err
			}
		}
		return 0, n, l, nil
	}
	if !labelName.MatchString(s) {
		return 0, "", "", bad
	}
	c, err := r.needContext(what)
	if err != nil {
		return 0, "", "", err
	}
	if c.Kind != KindNPC {
		return 0, "", "", fmt.Errorf("%s: outside an NPC's script, name the NPC, such as Guard::OnTimeout", what)
	}
	return c.npc, "", s, nil
}

func (r *Runtime) playerAddTimer(c *script.Call) error {
	const what = "player:add_timer"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, ok := r.host.(PlayerTimerHost)
	if !ok {
		return errors.New(what + ": player timers do not run here")
	}
	ms, ok := c.Arg(1).AsInt()
	if !ok || ms < 0 || ms > MaxPlayerTimer {
		return errors.New(what + ": milliseconds are 0 or more")
	}
	npc, name, label, err := r.timerTarget(what, c.Arg(2))
	if err != nil {
		return err
	}
	return wrap(what, h.AddPlayerTimer(id, ms, npc, name, r.actor(), label))
}

func (r *Runtime) playerRemoveTimer(c *script.Call) error {
	const what = "player:remove_timer"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, ok := r.host.(PlayerTimerHost)
	if !ok {
		return errors.New(what + ": player timers do not run here")
	}
	npc, name, label, err := r.timerTarget(what, c.Arg(1))
	if err != nil {
		return err
	}
	removed, err := h.RemovePlayerTimer(id, npc, name, r.actor(), label)
	if err != nil {
		return wrap(what, err)
	}
	c.Return(script.Bool(removed))
	return nil
}
