package metrics

import (
	"sync/atomic"
	"time"
)

// Tick work budgets from strategy section 3.8.
const (
	// TickBudget is the p99 target for a tick's work.
	TickBudget = 10 * time.Millisecond
	// TickMax is the most a tick's work may take.
	TickMax = 25 * time.Millisecond
	// OverBudgetStreak is how many ticks in a row over TickBudget make a
	// zone call OnOverBudget.
	OverBudgetStreak = 10
)

// Phase is a part of a zone tick.
type Phase int

// The parts of a zone tick.
const (
	// PhaseMove advances walks and warps.
	PhaseMove Phase = iota
	// PhaseAct runs monsters, combat, items, regen and saves.
	PhaseAct
	// PhaseSend builds each player's frame.
	PhaseSend
	numPhases
)

var phaseNames = [numPhases]string{"move", "act", "send"}

func (p Phase) String() string { return phaseNames[p] }

// Zone holds one zone's tick measurements. Only the zone's goroutine
// calls Handled, Phase and Tick. Any goroutine may read a Snapshot.
type Zone struct {
	Name string
	// Work is the zone goroutine's busy time per tick: the tick itself,
	// and the commands, joins and leaves it handled since the tick
	// before.
	Work Histogram
	// Step is the tick alone.
	Step Histogram
	// OnOverBudget is called when a tick's work was over TickBudget for
	// OverBudgetStreak ticks in a row, with those ticks' work. It runs on
	// the zone's goroutine, so it must not block. Set it before the zone
	// runs.
	OnOverBudget func(z *Zone, recent []time.Duration)

	phases     [numPhases]atomic.Int64
	ticks      atomic.Uint64
	overBudget atomic.Uint64
	overMax    atomic.Uint64
	players    atomic.Int64

	// Used by the zone goroutine only.
	pending time.Duration
	streak  int
	recent  [OverBudgetStreak]time.Duration
}

// Handled records time spent between ticks, such as on a command.
func (z *Zone) Handled(d time.Duration) { z.pending += d }

// Phase records time spent in a part of the current tick.
func (z *Zone) Phase(p Phase, d time.Duration) { z.phases[p].Add(int64(d)) }

// Tick records one tick that took step, with the number of players in
// the zone after it.
func (z *Zone) Tick(step time.Duration, players int) {
	work := step + z.pending
	z.pending = 0
	z.Step.Record(step)
	z.Work.Record(work)
	z.ticks.Add(1)
	z.players.Store(int64(players))
	if work > TickMax {
		z.overMax.Add(1)
	}
	if work <= TickBudget {
		z.streak = 0
		return
	}
	z.overBudget.Add(1)
	z.recent[z.streak] = work
	z.streak++
	if z.streak == OverBudgetStreak {
		z.streak = 0
		if z.OnOverBudget != nil {
			z.OnOverBudget(z, z.recent[:])
		}
	}
}

// Asleep records that the zone stopped ticking because it has no
// players.
func (z *Zone) Asleep() {
	z.players.Store(0)
	z.pending = 0
	z.streak = 0
}

// Reset clears the counts, for a measurement that starts now.
func (z *Zone) Reset() {
	z.Work.Reset()
	z.Step.Reset()
	for i := range z.phases {
		z.phases[i].Store(0)
	}
	z.ticks.Store(0)
	z.overBudget.Store(0)
	z.overMax.Store(0)
}

// ZoneSnapshot is a zone's measurements.
type ZoneSnapshot struct {
	Name       string  `json:"name"`
	Players    int64   `json:"players"`
	Ticks      uint64  `json:"ticks"`
	Work       Summary `json:"work"`
	Step       Summary `json:"step"`
	OverBudget uint64  `json:"over_budget"`
	OverMax    uint64  `json:"over_max"`
	// PhaseMean is the mean time per tick of each phase.
	PhaseMean map[string]time.Duration `json:"phase_mean_ns"`
}

// Snapshot returns the zone's measurements.
func (z *Zone) Snapshot() ZoneSnapshot {
	s := ZoneSnapshot{
		Name: z.Name, Players: z.players.Load(), Ticks: z.ticks.Load(),
		Work: z.Work.Summary(), Step: z.Step.Summary(),
		OverBudget: z.overBudget.Load(), OverMax: z.overMax.Load(),
		PhaseMean: map[string]time.Duration{},
	}
	for p := range numPhases {
		if s.Ticks > 0 {
			s.PhaseMean[p.String()] = time.Duration(z.phases[p].Load() / int64(s.Ticks)) // #nosec G115 -- a count of ticks.
		}
	}
	return s
}
