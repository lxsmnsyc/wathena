package modscript

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
	coremod "github.com/lxsmnsyc/wathena/server/mod"
)

// Mods turned off in a zone, and the error counts of the admin screens
// (proposal 0002 review decision 36, decision 0031):
//   - A mod that is off in a zone runs no code there: its event
//     handlers, timers, hooks, schedules, NPC clicks, item scripts,
//     skill scripts and monster AI scripts are skipped. Its contexts
//     stay, so its handlers run again once it is on.
//   - Its data stays: variables, server variables and the database are
//     not touched. Monsters it spawned stay on the map.
//   - Conversations with its NPCs close at once, without running more of
//     their scripts.
//   - A timer that comes due while the mod is off is skipped: a timer
//     that runs once is dropped, and a repeating one keeps its rhythm.
//   - A mod that is off when the zone starts does not run its entry
//     scripts or its NPCs' setup functions there. They run when it is
//     turned on.
//   - Item scripts of its items do not start while it is off.
//   - Core cannot be turned off.
//   - ErrorLog counts each mod's failures in a zone: failed handlers,
//     timers, hooks, schedules and clicks, and setup functions that
//     failed. The zone owns the log, so the counts outlive its Lua
//     state.

// ErrModOff is the error of a script that does not start because its mod
// is off in the zone.
var ErrModOff = errors.New("the mod is turned off in this zone")

// ModErrors are one mod's errors in one zone.
type ModErrors struct {
	Count int64
	// Last is the last error's text, at most MaxErrorText bytes, and At
	// when it happened.
	Last string
	At   time.Time
}

// MaxErrorText is the longest error text an ErrorLog keeps.
const MaxErrorText = 500

// ErrorLog counts the errors of mods' scripts in a zone. Only the zone's
// goroutine adds. Any goroutine may read.
type ErrorLog struct {
	mu   sync.Mutex
	mods map[string]*ModErrors
}

func (l *ErrorLog) add(mod string, err error, at time.Time) {
	if l == nil || err == nil || errors.Is(err, ErrModOff) {
		return
	}
	text := err.Error()
	if len(text) > MaxErrorText {
		cut := MaxErrorText
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.mods == nil {
		l.mods = map[string]*ModErrors{}
	}
	e := l.mods[mod]
	if e == nil {
		e = &ModErrors{}
		l.mods[mod] = e
	}
	e.Count++
	e.Last, e.At = text, at
}

// Snapshot returns the counts by mod.
func (l *ErrorLog) Snapshot() map[string]ModErrors {
	out := map[string]ModErrors{}
	if l == nil {
		return out
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for id, e := range l.mods {
		out[id] = *e
	}
	return out
}

// countError adds an error of a mod to the zone's log.
func (r *Runtime) countError(mod string, err error) {
	now := time.Now()
	if r.opts.Clock != nil {
		now = r.opts.Clock()
	}
	r.opts.Errors.add(mod, err, now)
}

// off reports whether a context's mod is off in this zone.
func (r *Runtime) off(c *Context) bool { return len(r.offMods) > 0 && r.offMods[c.Mod] }

// Off reports whether a mod is off in this zone.
func (r *Runtime) Off(mod string) bool { return r.offMods[mod] }

// OffEntry reports whether an entry script, as <mod>:<path>, belongs to
// a mod that is off in this zone.
func (r *Runtime) OffEntry(entry string) bool {
	mod, _, _ := strings.Cut(entry, ":")
	return r.offMods[mod]
}

// SetOff turns a mod off or on in this zone. Turning it on runs the entry
// scripts and NPC setups that did not run while it was off. It returns
// the errors of those scripts.
func (r *Runtime) SetOff(mod string, off bool) error {
	if mod == "" || mod == coremod.Core {
		return errors.New("core cannot be turned off")
	}
	if off {
		if r.offMods[mod] {
			return nil
		}
		if r.offMods == nil {
			r.offMods = map[string]bool{}
		}
		r.offMods[mod] = true
		for _, conv := range r.conversations {
			if conv.ctx.Mod == mod {
				conv.Closed, conv.Cancelled = true, true
				_ = conv.thread.Close()
				r.finishConversation(conv)
			}
		}
		r.log.Info("mod turned off in this zone", "map", r.host.Map(), "mod", mod)
		return nil
	}
	if !r.offMods[mod] {
		return nil
	}
	delete(r.offMods, mod)
	r.log.Info("mod turned on in this zone", "map", r.host.Map(), "mod", mod)
	var errs []error
	if r.notStarted[mod] {
		delete(r.notStarted, mod)
		if sc := r.scripts[mod]; sc != nil {
			if err := r.start(sc); err != nil {
				r.log.Error("mod scripts failed to load; the mod is off in this zone", "map", r.host.Map(), "mod", mod, "err", err)
				errs = append(errs, err)
			}
		}
	}
	if r.reloadedOff[mod] {
		delete(r.reloadedOff, mod)
		r.reloadMonsters(mod)
		if err := r.reloadSkills(mod); err != nil {
			errs = append(errs, err)
		}
	}
	for _, n := range r.npcs {
		m, _, _ := strings.Cut(n.Entry, ":")
		if m != mod || n.ctx != nil {
			continue
		}
		if err := r.startNPC(n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// runSetup runs a setup function in a new context, and counts its
// failure.
func (r *Runtime) runSetup(c *Context, setup script.Value) error {
	err := r.run(c, setup)
	if err != nil {
		r.countError(c.Mod, fmt.Errorf("setup: %w", err))
	}
	return err
}
