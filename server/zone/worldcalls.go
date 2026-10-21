package zone

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/social"
)

// The world calls of mod scripts in a zone: monsters that scripts
// spawn, the death event, the drop_rate and experience hooks, schedules
// on the server clock, and runs of world events.

// wallClock is the server clock: Clock, or the system clock.
func (z *Zone) wallClock() time.Time {
	if z.Clock != nil {
		return z.Clock()
	}
	return time.Now()
}

// monsterHandle returns the script handle of a monster, made on first
// use. A dead monster's handle ends after its death event.
func (z *Zone) monsterHandle(e *entity) int64 {
	if e.monster.handle != 0 {
		return e.monster.handle
	}
	if z.monsterHandles == nil {
		z.monsterHandles = map[int64]*entity{}
	}
	z.nextMonsterHandle++
	e.monster.handle = z.nextMonsterHandle
	z.monsterHandles[e.monster.handle] = e
	return e.monster.handle
}

// queueDeath queues the death event for a unit that died. A monster's
// handle stays readable until the event has run.
func (z *Zone) queueDeath(e, killer *entity) {
	if p := e.player; p != nil {
		// A player's death reaches the NPCs of every map, as rAthena's
		// OnPCDieEvent (status.cpp 1786, review decision 59).
		var k *Player
		if killer != nil {
			k = killer.player
		}
		z.sendPlayerEvent("death", p, &modscript.Death{X: e.x, Y: e.y, Map: strings.TrimPrefix(z.Map, "map/")}, k, false)
	}
	if z.scripts == nil {
		return
	}
	d := &modscript.Death{X: e.x, Y: e.y}
	ev := scriptEvent{death: d}
	if e.player != nil {
		d.Victim, d.VictimKind = int64(e.player.charID), "player"
	} else {
		d.Victim, d.VictimKind, d.Monster = z.monsterHandle(e), "monster", int64(e.monster.spawn.Mob)
		ev.release = []int64{d.Victim}
	}
	if killer != nil {
		switch {
		case killer.player != nil:
			d.Killer, d.KillerKind = int64(killer.player.charID), "player"
		case killer.monster != nil && z.entities.has(killer):
			d.Killer, d.KillerKind = z.monsterHandle(killer), "monster"
		}
	}
	z.scriptEvents = append(z.scriptEvents, ev)
}

// killHooks returns the hooks of mods for a kill, or nil when no mod
// added one.
func (z *Zone) killHooks(e, killer *entity) *progression.Hooks {
	r := z.scripts
	if r == nil || !r.HasHooks("drop_rate") && !r.HasHooks("experience") {
		return nil
	}
	monster := z.monsterHandle(e)
	var player int64
	if killer != nil && killer.player != nil {
		player = int64(killer.player.charID)
	}
	h := &progression.Hooks{}
	if r.HasHooks("drop_rate") {
		h.DropRate = func(item uint32, rate int) int {
			return r.RunDropRate(modscript.DropRate{Player: player, Monster: monster, Item: item, Rate: rate})
		}
	}
	if r.HasHooks("experience") {
		h.Experience = func(g progression.ExpGain) (int64, int64) {
			ex := modscript.Experience{Player: int64(g.CharID), Monster: monster, Base: g.Base, Job: g.Job,
				Party: g.Party, ShareBase: g.ShareBase, ShareJob: g.ShareJob}
			if p := z.players[g.CharID]; p != nil {
				var tax int
				ex.Guild, tax = z.guildTax(p)
				ex.GuildTax = social.TaxOf(g.Base, tax)
			}
			return r.RunExperience(ex)
		}
	}
	return h
}

// Despawn removes the monsters a context owns, alive or waiting to come
// back.
func (h scriptHost) Despawn(owner int64) {
	z := h.z
	if len(z.owned[owner]) == 0 {
		return
	}
	delete(z.owned, owner)
	for e := range z.entities.all() {
		if e.monster == nil || e.monster.spawn.owner != owner {
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
	z.respawns = slices.DeleteFunc(z.respawns, func(r respawn) bool { return r.spawn.owner == owner })
}

// runSchedules runs the scripts' schedules that are due on the server
// clock.
func (z *Zone) runSchedules() {
	if z.scripts != nil {
		z.scripts.RunSchedules(z.wallClock())
	}
}

// scheduleWait returns a channel that fires when the scripts' next
// schedule is due, for a zone that sleeps. It is nil without schedules.
func (z *Zone) scheduleWait(t **time.Timer) <-chan time.Time {
	if z.scripts == nil {
		return nil
	}
	next, ok := z.scripts.NextSchedule()
	if !ok {
		return nil
	}
	d := max(next.Sub(z.wallClock()), 0)
	if *t == nil {
		*t = time.NewTimer(d)
	} else {
		(*t).Reset(d)
	}
	return (*t).C
}

// EventRun is one run of a world event in a zone (data/events.yml).
type EventRun struct {
	// ID names the run, for EndEvent.
	ID int64
	// Name is the event's full name, and Entry its script, as
	// <mod>:<path>.
	Name, Entry string
	// Typed is set for a typed event's script, whose context:get_event()
	// returns the run.
	Typed bool
}

// StartEvent starts a run of a world event: its script's setup function
// runs in an event context. It waits until the zone has done it.
func (z *Zone) StartEvent(ctx context.Context, run EventRun) error {
	return z.do(ctx, func() error { return z.startEvent(run) })
}

// EndEvent ends a run: what its context owns goes.
func (z *Zone) EndEvent(ctx context.Context, id int64) error {
	return z.do(ctx, func() error { z.endEvent(id); return nil })
}

// EventCondition runs the condition of a world event: function fn of
// the module that the require name module finds, as the mod.
func (z *Zone) EventCondition(ctx context.Context, mod, module, fn string) (bool, error) {
	var held bool
	err := z.do(ctx, func() error {
		var err error
		held, err = z.eventCondition(mod, module, fn)
		return err
	})
	return held, err
}

// do runs f on the zone's goroutine and waits for it. It is all or
// nothing for the caller (review 2026-10):
//   - When ctx ends or the zone stops before f starts, f never runs, and
//     do returns the error.
//   - Once f starts, do waits for it and returns its result, even when
//     ctx ends meanwhile. f may change the zone, such as a restore that
//     moves items, and the caller must not act as if it did not.
//
// Before, do returned ctx's error while f still ran later. A restore
// whose request timed out then let go of its offline characters and
// storages, and the zone moved their items afterwards: the moved items
// were in two places.
func (z *Zone) do(ctx context.Context, f func() error) error {
	var err error
	if cerr := z.call(ctx, func() { err = f() }); cerr != nil {
		return cerr
	}
	return err
}

// call runs f on the zone's goroutine and waits for it, as do does. It
// returns ctx's error or ErrStopped when f did not run.
func (z *Zone) call(ctx context.Context, f func()) error {
	// state is callWaiting until the zone starts f (callStarted) or the
	// caller gives up first (callAbandoned).
	var state atomic.Int32
	done := make(chan struct{})
	run := func() {
		if !state.CompareAndSwap(callWaiting, callStarted) {
			return
		}
		f()
		// Not deferred: after a panic in f, done stays open.
		close(done)
	}
	select {
	case z.control <- run:
	case <-ctx.Done():
		return ctx.Err()
	case <-z.stopped:
		return ErrStopped
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		if state.CompareAndSwap(callWaiting, callAbandoned) {
			return ctx.Err()
		}
	case <-z.stopped:
		if state.CompareAndSwap(callWaiting, callAbandoned) {
			return ErrStopped
		}
	}
	// f started. A panic in it stops the zone, which closes stopped.
	select {
	case <-done:
		return nil
	case <-z.stopped:
		select {
		case <-done:
			return nil
		default:
			return ErrStopped
		}
	}
}

// The states of a call.
const (
	callWaiting int32 = iota
	callStarted
	callAbandoned
)

func (z *Zone) startEvent(run EventRun) error {
	r := z.itemScripts()
	if r == nil {
		return errors.New("mod scripts do not run in this zone")
	}
	if z.eventRuns == nil {
		z.eventRuns = map[int64]*modscript.Context{}
	}
	if z.eventRuns[run.ID] != nil {
		return fmt.Errorf("run %d of %s already started", run.ID, run.Name)
	}
	var typed int64
	if run.Typed {
		typed = run.ID
	}
	c, err := r.StartEventRun(run.Name, run.Entry, typed)
	if err != nil {
		return err
	}
	z.eventRuns[run.ID] = c
	return nil
}

func (z *Zone) endEvent(id int64) {
	c := z.eventRuns[id]
	if c == nil {
		return
	}
	delete(z.eventRuns, id)
	if z.scripts != nil {
		z.scripts.EndContext(c)
	}
}

func (z *Zone) eventCondition(mod, module, fn string) (bool, error) {
	r := z.itemScripts()
	if r == nil {
		return false, errors.New("mod scripts do not run in this zone")
	}
	return r.CheckCondition(mod, module, fn)
}

// StallNextCall makes the zone stall for d after it takes its next call
// from another goroutine, and before it runs it, as a machine that
// pauses the zone in the middle of serving a call. The caller may give
// up meanwhile. It is for the duplication tests of waserver --dev only
// (POST /debug/wathena/stall?next=1).
func (z *Zone) StallNextCall(d time.Duration) {
	z.stallNext.Store(int64(d))
}

// Stall holds the zone's goroutine for d, as a machine that stalls
// does: the zone takes no command, tick or call meanwhile. It is for the
// duplication tests of waserver --dev only (POST /debug/wathena/stall).
func (z *Zone) Stall(ctx context.Context, d time.Duration) error {
	return z.do(ctx, func() error {
		time.Sleep(d)
		return nil
	})
}
