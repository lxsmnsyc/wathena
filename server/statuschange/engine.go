package statuschange

import (
	"container/heap"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// StartFlag holds rAthena's e_status_change_start_flags.
type StartFlag uint8

const (
	// NoAvoid is SCSTART_NOAVOID: no resistance, and no boss immunity.
	NoAvoid StartFlag = 0x01
	// NoTickDef is SCSTART_NOTICKDEF: resistance does not shorten the
	// duration.
	NoTickDef StartFlag = 0x02
	// Loaded is SCSTART_LOADED: a status restored from a save keeps its
	// values.
	Loaded StartFlag = 0x04
	// NoRateDef is SCSTART_NORATEDEF: resistance does not lower the
	// rate.
	NoRateDef StartFlag = 0x08
	// NoIcon is SCSTART_NOICON: no status icon.
	NoIcon StartFlag = 0x10
)

// Unit is one entity that can have statuses: rAthena's block_list with
// its status_change.
type Unit interface {
	// Changes is the unit's status_change, never nil.
	Changes() *List
	Kind() Kind
	// Status is status_get_status_data: the battle status. Base is
	// status_get_base_status.
	Status() *battle.Status
	Base() *battle.Status
	// Level is status_get_lv. JobLevel is a player's job level, or 1.
	Level() int
	JobLevel() int
	HP() int64
	SP() int64
	Dead() bool
	// ResEff is the player's resistance to a status from bResEff, per
	// 10,000. Other units have none.
	ResEff(typ int) int
	// Skill is pc_checkskill: a player's level of a skill, or 0.
	Skill(id int) int
	// AlwaysEndure is a player's bNoWalkDelay.
	AlwaysEndure() bool
	// Ref is the unit as a source of a status.
	Ref() Source
}

// Env is what the engine does to the zone: rAthena's calls to clif,
// unit, status_calc_bl, and the script events and hooks.
type Env interface {
	Now() int64
	// RandN is rnd() % n from the zone's secret stream.
	RandN(n int) int
	// Recalc is status_calc_bl: the unit's stats are computed again.
	Recalc(u Unit)
	// Icon is clif_status_change: a status icon shows or hides, with the
	// time left and up to three values.
	Icon(u Unit, icon int, on bool, tick int64, v1, v2, v3 int)
	// OptionChanged is clif_changeoption: the unit's opt1, opt2, opt3 or
	// option changed.
	OptionChanged(u Unit)
	// StopWalking and StopAttacking are unit_stop_walking and
	// unit_stop_attack.
	StopWalking(u Unit)
	StopAttacking(u Unit)
	// Damage is status_damage without a source: passive damage when
	// passive is set (status_zap), damage that cannot kill when noKill is
	// set. It returns the HP taken.
	Damage(u Unit, hp, sp int64, passive, noKill bool) int64
	// Heal is status_heal.
	Heal(u Unit, hp, sp int64)
	// Started, Ended and Ticked are the script events status_start,
	// status_end and status_tick.
	Started(u Unit, e *Entry)
	Ended(u Unit, e *Entry, reason Reason)
	Ticked(u Unit, e *Entry)
	// Chance and Duration are the hooks status_chance and
	// status_duration. Each returns the value it was given when no hook
	// changes it.
	Chance(target Unit, src *Source, typ, level, chance int) int
	Duration(target Unit, src *Source, typ, level int, duration int64) int64
}

// Engine runs the statuses of one zone. Only the zone's goroutine uses
// it.
type Engine struct {
	DB  *DB
	env Env
	// timers are the statuses' timers and the delayed starts, by due
	// time.
	timers timerHeap
	seq    uint64
}

// New makes a zone's engine.
func New(db *DB, env Env) *Engine { return &Engine{DB: db, env: env} }

// timer is one due time: a status's status_change_timer, or a delayed
// start (status_change_start_timer).
type timer struct {
	due  int64
	seq  uint64
	unit Unit
	// entry is the status the timer runs for. For a delayed start it is
	// nil, and start holds the start.
	entry *Entry
	start *Start
	src   Unit
}

type timerHeap []*timer

func (h timerHeap) Len() int { return len(h) }
func (h timerHeap) Less(i, j int) bool {
	if h[i].due != h[j].due {
		return h[i].due < h[j].due
	}
	return h[i].seq < h[j].seq
}
func (h timerHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *timerHeap) Push(x any)   { *h = append(*h, x.(*timer)) }
func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	t := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return t
}

// setTimer is add_timer for a status: the entry's timer runs at due.
func (en *Engine) setTimer(u Unit, e *Entry, due int64) {
	en.seq++
	e.timer, e.seq = due, en.seq
	heap.Push(&en.timers, &timer{due: due, seq: en.seq, unit: u, entry: e})
}

// clearTimer is delete_timer: the entry's timer no longer runs.
func (e *Entry) clearTimer() { e.timer, e.seq = -1, 0 }

// Advance runs every timer due at now, in order of due time, as rAthena's
// timer loop.
func (en *Engine) Advance(now int64) {
	for en.timers.Len() > 0 && en.timers[0].due <= now {
		t := heap.Pop(&en.timers).(*timer)
		if t.start != nil {
			// status_change_start_timer: the target must still be there and
			// alive.
			if !t.unit.Dead() {
				en.startPostDelay(t.src, t.unit, t.start)
			}
			continue
		}
		e := t.entry
		if e.seq != t.seq || e.timer != t.due || t.unit.Changes().Get(e.Type) != e {
			// A timer that was cleared or replaced.
			continue
		}
		en.timer(t.unit, e, t.due)
	}
}

// Shift moves every timer and status end later, for time the zone spent
// asleep, as mod timers shift.
func (en *Engine) Shift(ms int64) {
	for _, t := range en.timers {
		t.due += ms
		if t.entry != nil && t.entry.seq == t.seq {
			t.entry.timer += ms
			if t.entry.End >= 0 {
				t.entry.End += ms
			}
		}
	}
}

// Next is the time of the next timer, or -1.
func (en *Engine) Next() int64 {
	if en.timers.Len() == 0 {
		return -1
	}
	return en.timers[0].due
}

// Start is one status_change_start: the status and its values, the rate
// per 10,000, the duration in milliseconds (InfiniteTick for none), and
// the flags.
type Start struct {
	Type                   int
	Rate                   int
	Val1, Val2, Val3, Val4 int
	Duration               int64
	Flag                   StartFlag
	// Delay is how long after the resistance check the status starts.
	Delay int64
	// Reason is why it starts, and Source who caused it, as the script
	// events report. Source may be the zero Source.
	Reason Reason
	Source Source
	// Bonuses are the bonuses of a mod status.
	Bonuses []Bonus
	// NextTick is, for a mod status loaded from a save, the time to its
	// next status_tick. Duration is then the time to its end.
	NextTick int64
}
