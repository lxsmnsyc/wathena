package zone

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
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

// SpawnMonsters places monsters that a script context owns
// (spawning.monsters).
func (h scriptHost) SpawnMonsters(owner int64, s modscript.SpawnSpec) ([]int64, error) {
	z := h.z
	if z.Mods == nil || z.Mods.Monsters == nil {
		return nil, errors.New("there are no monster records")
	}
	tmpl, ok := z.Mods.Monsters[s.Monster]
	if !ok {
		return nil, fmt.Errorf("there is no monster %d", s.Monster)
	}
	sp := tmpl
	sp.Amount, sp.owner = s.Amount, owner
	sp.X, sp.Y, sp.W, sp.H = 0, 0, 0, 0
	if s.HasCell {
		if s.X < 0 || s.Y < 0 || s.X >= z.grid.Width || s.Y >= z.grid.Height {
			return nil, fmt.Errorf("%d,%d is outside the map", s.X, s.Y)
		}
		if s.Area == 0 && z.grid.NoPass(s.X, s.Y) {
			return nil, fmt.Errorf("%d,%d is not a walkable cell", s.X, s.Y)
		}
		sp.X, sp.Y, sp.W, sp.H = s.X, s.Y, s.Area, s.Area
		// spawnCell reads a center of 0,0 as anywhere.
		if sp.X == 0 && sp.Y == 0 {
			sp.X = 1
		}
	}
	if s.Respawn {
		// As a spawn line: its delay, or rAthena's default of 5 seconds.
		// mob_setdelayspawn's spawn delay rates are 100% by default, and
		// die keeps the 1 second floor.
		sp.RespawnMs = cmp.Or(s.RespawnMs, modscript.DefaultRespawnMs)
	} else {
		sp.noRespawn = true
	}
	if z.owned == nil {
		z.owned = map[int64][]*Spawn{}
	}
	z.owned[owner] = append(z.owned[owner], &sp)
	ids := make([]int64, 0, s.Amount)
	for range s.Amount {
		if e := z.spawnOne(&sp); e != nil {
			ids = append(ids, z.monsterHandle(e))
		}
	}
	return ids, nil
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

// do runs f on the zone's goroutine and waits for it.
func (z *Zone) do(ctx context.Context, f func() error) error {
	done := make(chan error, 1)
	select {
	case z.control <- func() { done <- f() }:
	case <-ctx.Done():
		return ctx.Err()
	case <-z.stopped:
		return ErrStopped
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-z.stopped:
		return ErrStopped
	}
}

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
	c, err := r.StartEvent(run.Name, run.Entry)
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
