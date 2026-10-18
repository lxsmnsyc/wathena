// Package rotation runs a procedural world's rotating dungeons and world
// changes (strategy sections 8.8 and 8.10, proposal 0002 review decision
// 47, decision 0045):
//
//   - A rotating dungeon gets new floors at its style's time, daily or
//     weekly, in the server's time zone, as the world event scheduler
//     reads clock times (package schedule). Its new floors carry the
//     date in their map IDs.
//   - With procedural.seasons.days set, a season ends at 00:00 every
//     that many days from the day seasons started, and the next region,
//     by town number in turn, generates again with a new biome.
//   - Staff force a rotation or a region's change. It happens 10
//     minutes later, after the countdown.
//   - Before a change, the players on the maps it removes or replaces
//     get warnings at 10, 5 and 1 minutes and 30 seconds. At the time,
//     the Host makes the change in the running world: the new maps, the
//     portals, and the players and points moved to the nearest town by
//     the map graph. The Store records it with the moves of the offline
//     characters' points in one transaction.
//   - The next change is built before it starts, so players do not wait.
//   - Restarts: Due turns the stored state into the state a world runs
//     at its start, with the changes that were due while it was stopped,
//     or that a stop cut short before the Store recorded them. A forced
//     change that waits for its countdown does not last across a
//     restart.
//
// Manager does the timing. Only one goroutine calls Check.
package rotation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/schedule"
)

// Kinds of change.
const (
	KindRotation = "rotation"
	KindRegion   = "region"
)

// Countdown is how long before a change staff force it, and Warnings
// when players on its maps hear of it (strategy section 8.10).
var (
	Countdown = 10 * time.Minute
	Warnings  = []time.Duration{10 * time.Minute, 5 * time.Minute, time.Minute, 30 * time.Second}
)

// Rotation is a rotating dungeon: its ID and its schedule.
type Rotation struct {
	Dungeon string
	Rule    layout.RotationRule
}

// State is what runs: the active dates and the regions' seasons, and
// the seasons' start and count of season ends.
type State struct {
	layout.State
	// SeasonsStarted is the date seasons started, or "" while they never
	// ran, and SeasonEnds how many season ends changed a region.
	SeasonsStarted string
	SeasonEnds     int
}

// Clone returns a copy of the state.
func (s State) Clone() State {
	out := s
	out.State = s.State.Clone()
	if out.Dates == nil {
		out.Dates = map[string]string{}
	}
	if out.Seasons == nil {
		out.Seasons = map[string]int{}
	}
	return out
}

// Change is one change of the world: a rotation or a region's change.
type Change struct {
	Kind string
	// Dungeon and Date are a rotation's dungeon and its new date.
	Dungeon, Date string
	// Region is a region's town ID, such as t2, and Season its new
	// season. SeasonEnd is the count of season ends after a scheduled
	// season end, or 0 for a change staff forced.
	Region    string
	Season    int
	SeasonEnd int
	// Due is when it happens. Forced is set for a change staff forced.
	Due    time.Time
	Forced bool
	// First is set for a rotating dungeon's first date, at a world's
	// first start: no floors go.
	First bool
}

// key names the change's slot: one change waits per dungeon, one per
// region that staff force, and one season end.
func (c Change) key() string {
	switch {
	case c.Kind == KindRotation:
		return "rotation:" + c.Dungeon
	case c.Forced:
		return "region:" + c.Region
	}
	return "season"
}

// Next returns the state after the change.
func (c Change) Next(s State) State {
	out := s.Clone()
	switch c.Kind {
	case KindRotation:
		out.Dates[c.Dungeon] = c.Date
	case KindRegion:
		out.Seasons[c.Region] = c.Season
		if c.SeasonEnd > 0 {
			out.SeasonEnds = c.SeasonEnd
		}
	}
	return out
}

// String describes the change for the log.
func (c Change) String() string {
	if c.Kind == KindRotation {
		return "the rotation of " + c.Dungeon + " to " + c.Date
	}
	return fmt.Sprintf("season %d of the region %s", c.Season, c.Region)
}

// Move is where the points on a removed or replaced map go: the nearest
// town's save spot.
type Move struct {
	From, To string
	X, Y     int
}

// Host makes changes in the running world.
type Host interface {
	// Prepare builds the maps of the state a change leads to, before it
	// happens. A second call for the same change does nothing.
	Prepare(ctx context.Context, from, to State, c Change) error
	// Warn tells the players on the maps the change removes or replaces
	// how long is left.
	Warn(ctx context.Context, c Change, left time.Duration)
	// Apply makes the change in the running world: the new maps, the
	// portals to them, and the players and the points of the players
	// online moved off the old maps. It returns the moves of the points
	// for the Store. An error leaves the world as it was.
	Apply(ctx context.Context, from, to State, c Change) ([]Move, error)
	// Done runs once the Store recorded the change, or failed to: the
	// dungeon_rotated and region_changed handlers, and the old maps'
	// zones stop.
	Done(ctx context.Context, c Change)
}

// Store records changes.
type Store interface {
	// Commit records a change and moves the points on the maps it
	// removed or replaced, in one transaction.
	Commit(ctx context.Context, c Change, moves []Move) error
	// StartSeasons records the date seasons started.
	StartSeasons(ctx context.Context, date string) error
}

// Options configure a Manager.
type Options struct {
	Rotations []Rotation
	// SeasonDays is procedural.seasons.days, 0 when seasons are off.
	SeasonDays int
	// Regions are the regions' town IDs by town number, which season
	// ends change in turn.
	Regions []string
	// Location is the server's time zone. It is UTC when nil.
	Location *time.Location
	Host     Host
	Store    Store
	Log      *slog.Logger
}

// Errors of Force.
var (
	ErrNoDungeon = errors.New("there is no such rotating dungeon")
	ErrNoRegion  = errors.New("there is no such region")
	// ErrAhead refuses a forced rotation of a dungeon whose date is
	// already after the schedule's date for now, as an earlier force
	// left it. Each force would move it one more date ahead and hold
	// back its natural rotation.
	ErrAhead = errors.New("the dungeon is already ahead of its schedule")
)

// Manager decides when changes happen, warns, and makes them through its
// Host. Force, Status and Do may be called from any goroutine.
type Manager struct {
	opts    Options
	specs   map[string]schedule.Spec
	state   State
	planned map[string]*Change
	// prepared and warned are by the planned change's key: whether its
	// maps were built, and the warnings sent.
	prepared map[string]bool
	warned   map[string]int
	// unsaved are the changes the world made that the Store did not
	// record yet, with their moves.
	unsaved []unsaved

	mu     sync.Mutex
	queue  []request
	status Status
}

type unsaved struct {
	c     Change
	moves []Move
}

type request struct {
	dungeon, region string
	audit           func(Change) error
	done            chan result
}

type result struct {
	c   Change
	err error
}

// New makes a manager that runs from a state: the state the world runs
// at its start, from Due.
func New(opts Options, state State) (*Manager, error) {
	if opts.Location == nil {
		opts.Location = time.UTC
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	m := &Manager{opts: opts, specs: map[string]schedule.Spec{}, state: state.Clone(), planned: map[string]*Change{},
		prepared: map[string]bool{}, warned: map[string]int{}}
	for _, r := range opts.Rotations {
		s, err := r.Rule.Spec()
		if err != nil {
			return nil, fmt.Errorf("the rotating dungeon %s: %w", r.Dungeon, err)
		}
		m.specs[r.Dungeon] = s
	}
	m.publish(time.Time{})
	return m, nil
}

// State returns the state the world runs. Call it on the goroutine that
// calls Check.
func (m *Manager) State() State { return m.state.Clone() }

// lastDate is the date of a rotation's latest time at or before now.
func lastDate(spec schedule.Spec, now time.Time, loc *time.Location) string {
	// Rotations come at least once a week, so the latest is within 8
	// days.
	var latest time.Time
	for t := schedule.Next(spec, now.AddDate(0, 0, -8), loc); !t.After(now); t = schedule.Next(spec, t, loc) {
		latest = t
	}
	if latest.IsZero() {
		return ""
	}
	return layout.DateOf(latest, loc)
}

// nextAfter is a rotation's first time after a date's time and after
// now: the time of the next date to rotate to.
func nextAfter(spec schedule.Spec, date string, now time.Time, loc *time.Location) time.Time {
	after := now
	if d, err := time.ParseInLocation("2006-01-02", date, loc); err == nil {
		// The time of day on the date itself, so its own time is not
		// next.
		at := schedule.Resolve(d.Year(), d.Month(), d.Day(), spec.Hour, spec.Minute, loc)
		if at.After(after) {
			after = at
		}
	}
	return schedule.Next(spec, after, loc)
}

// seasonEnd is the time of season end n, from 1: 00:00 of the start
// date plus n times days, in the server's time zone.
func seasonEnd(started string, days, n int, loc *time.Location) time.Time {
	d, err := time.ParseInLocation("2006-01-02", started, loc)
	if err != nil {
		return time.Time{}
	}
	d = d.AddDate(0, 0, n*days)
	return schedule.Resolve(d.Year(), d.Month(), d.Day(), 0, 0, loc)
}

// endsBy counts the season ends at or before now.
func endsBy(started string, days int, now time.Time, loc *time.Location) int {
	if started == "" || days <= 0 {
		return 0
	}
	n := 0
	for !seasonEnd(started, days, n+1, loc).After(now) {
		n++
	}
	return n
}

// regionChange is season end n's change: the next region in turn.
func (m *Manager) regionChange(st State, n int, due time.Time) (Change, bool) {
	if len(m.opts.Regions) == 0 {
		return Change{}, false
	}
	r := m.opts.Regions[(n-1)%len(m.opts.Regions)]
	return Change{Kind: KindRegion, Region: r, Season: st.Seasons[r] + 1, SeasonEnd: n, Due: due}, true
}

// Due returns the state a world runs when it starts at now, from its
// stored state, and the changes that lead there, in order: each rotating
// dungeon's first date, or its date when a rotation came while the
// server was stopped, and each season end that came. Seasons that are on
// and never started start today; the caller records that with
// StartSeasons.
func Due(opts Options, stored State, now time.Time) (State, []Change, error) {
	m, err := New(opts, stored)
	if err != nil {
		return State{}, nil, err
	}
	st := m.state.Clone()
	var out []Change
	for _, r := range opts.Rotations {
		d := lastDate(m.specs[r.Dungeon], now, m.opts.Location)
		active := st.Dates[r.Dungeon]
		if d == "" || active != "" && active >= d {
			continue
		}
		c := Change{Kind: KindRotation, Dungeon: r.Dungeon, Date: d, Due: now, First: active == ""}
		out = append(out, c)
		st = c.Next(st)
	}
	if opts.SeasonDays > 0 {
		if st.SeasonsStarted == "" {
			st.SeasonsStarted = layout.DateOf(now, m.opts.Location)
		}
		for n := st.SeasonEnds + 1; n <= endsBy(st.SeasonsStarted, opts.SeasonDays, now, m.opts.Location); n++ {
			c, ok := m.regionChange(st, n, now)
			if !ok {
				break
			}
			out = append(out, c)
			st = c.Next(st)
		}
	}
	return st, out, nil
}

// Force plans a rotation of a dungeon, or a change of a region by its
// town ID, Countdown from now, and returns it. A change of it that
// comes sooner stays. The change waits on the goroutine that calls
// Check.
//
// Rules:
//   - audit, when not nil, runs with the change before it is planned.
//     When it fails, nothing is planned and Force returns its error.
//   - A dungeon whose date is already after the schedule's date for now
//     is refused with ErrAhead.
func (m *Manager) Force(ctx context.Context, dungeon, region string, audit func(Change) error) (Change, error) {
	done := make(chan result, 1)
	m.mu.Lock()
	m.queue = append(m.queue, request{dungeon: dungeon, region: region, audit: audit, done: done})
	m.mu.Unlock()
	select {
	case r := <-done:
		return r.c, r.err
	case <-ctx.Done():
		return Change{}, ctx.Err()
	}
}

// applyQueue plans the forced changes.
func (m *Manager) applyQueue(now time.Time) {
	m.mu.Lock()
	list := m.queue
	m.queue = nil
	m.mu.Unlock()
	for _, q := range list {
		c, err := m.force(q.dungeon, q.region, q.audit, now)
		q.done <- result{c, err}
	}
}

func (m *Manager) force(dungeon, region string, audit func(Change) error, now time.Time) (Change, error) {
	if audit == nil {
		audit = func(Change) error { return nil }
	}
	due := now.Add(Countdown)
	if dungeon != "" {
		spec, ok := m.specs[dungeon]
		if !ok {
			return Change{}, ErrNoDungeon
		}
		k := "rotation:" + dungeon
		if p := m.planned[k]; p != nil && !p.Due.After(due) {
			return *p, audit(*p)
		}
		active := m.state.Dates[dungeon]
		if d := lastDate(spec, now, m.opts.Location); d != "" && active > d {
			return Change{}, ErrAhead
		}
		date := layout.DateOf(nextAfter(spec, active, now, m.opts.Location), m.opts.Location)
		if p := m.planned[k]; p != nil && p.Date == date {
			// The same floors, sooner: what was built stays.
			c := *p
			c.Due, c.Forced = due, true
			if err := audit(c); err != nil {
				return Change{}, err
			}
			p.Due, p.Forced = due, true
			return *p, nil
		}
		c := Change{Kind: KindRotation, Dungeon: dungeon, Date: date, Due: due, Forced: true}
		if err := audit(c); err != nil {
			return Change{}, err
		}
		m.plan(&c)
		return c, nil
	}
	if !slices.Contains(m.opts.Regions, region) {
		return Change{}, ErrNoRegion
	}
	k := "region:" + region
	if p := m.planned[k]; p != nil {
		return *p, audit(*p)
	}
	c := Change{Kind: KindRegion, Region: region, Season: m.state.Seasons[region] + 1, Due: due, Forced: true}
	if err := audit(c); err != nil {
		return Change{}, err
	}
	m.plan(&c)
	return c, nil
}

// plan makes a change the next of its slot.
func (m *Manager) plan(c *Change) {
	k := c.key()
	m.planned[k] = c
	delete(m.prepared, k)
	delete(m.warned, k)
}

// Check plans, builds, warns and makes the changes that are due at now.
func (m *Manager) Check(ctx context.Context, now time.Time) {
	defer m.publish(now)
	m.applyQueue(now)
	m.save(ctx)
	for _, r := range m.opts.Rotations {
		k := "rotation:" + r.Dungeon
		if m.planned[k] != nil {
			continue
		}
		spec := m.specs[r.Dungeon]
		active := m.state.Dates[r.Dungeon]
		if d := lastDate(spec, now, m.opts.Location); d != "" && active < d {
			// A rotation came, such as when the clock jumped.
			m.plan(&Change{Kind: KindRotation, Dungeon: r.Dungeon, Date: d, Due: now})
			continue
		}
		at := nextAfter(spec, active, now, m.opts.Location)
		m.plan(&Change{Kind: KindRotation, Dungeon: r.Dungeon, Date: layout.DateOf(at, m.opts.Location), Due: at})
	}
	if m.opts.SeasonDays > 0 && m.planned["season"] == nil {
		if m.state.SeasonsStarted == "" {
			today := layout.DateOf(now, m.opts.Location)
			if err := m.opts.Store.StartSeasons(ctx, today); err != nil {
				m.opts.Log.Error("seasons did not start: the date could not be recorded", "err", err)
			} else {
				m.state.SeasonsStarted = today
			}
		}
		if m.state.SeasonsStarted != "" {
			n := m.state.SeasonEnds + 1
			due := seasonEnd(m.state.SeasonsStarted, m.opts.SeasonDays, n, m.opts.Location)
			if due.Before(now) {
				due = now
			}
			if c, ok := m.regionChange(m.state, n, due); ok {
				m.plan(&c)
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(m.planned)) {
		c := m.planned[k]
		left := c.Due.Sub(now)
		if !m.prepared[k] {
			// The season is the region's when the change happens.
			if c.Kind == KindRegion {
				c.Season = m.state.Seasons[c.Region] + 1
			}
			if err := m.opts.Host.Prepare(ctx, m.state, c.Next(m.state), *c); err != nil {
				m.opts.Log.Error("the next world change could not be built; it is tried again", "change", c.String(), "err", err)
				continue
			}
			m.prepared[k] = true
		}
		if left > 0 {
			m.warn(ctx, k, c, left)
			continue
		}
		m.perform(ctx, k, c)
	}
}

// warn sends the warning of the last threshold that left passed, once.
func (m *Manager) warn(ctx context.Context, k string, c *Change, left time.Duration) {
	i := m.warned[k]
	sent := false
	for i < len(Warnings) && left <= Warnings[i] {
		i++
		sent = true
	}
	if !sent {
		return
	}
	m.warned[k] = i
	m.opts.Host.Warn(ctx, *c, left)
}

// perform makes a change, then records it.
func (m *Manager) perform(ctx context.Context, k string, c *Change) {
	if c.Kind == KindRegion {
		c.Season = m.state.Seasons[c.Region] + 1
	}
	from := m.state
	to := c.Next(from)
	moves, err := m.opts.Host.Apply(ctx, from, to, *c)
	if err != nil {
		m.opts.Log.Error("a world change failed; it is tried again", "change", c.String(), "err", err)
		delete(m.prepared, k)
		return
	}
	delete(m.planned, k)
	delete(m.prepared, k)
	delete(m.warned, k)
	m.state = to
	m.opts.Log.Info("world changed", "change", c.String(), "forced", c.Forced, "moves", len(moves))
	m.unsaved = append(m.unsaved, unsaved{c: *c, moves: moves})
	m.save(ctx)
	m.opts.Host.Done(ctx, *c)
}

// save records the changes the world made, in order. One that fails is
// tried again at the next Check.
func (m *Manager) save(ctx context.Context) {
	for len(m.unsaved) > 0 {
		u := m.unsaved[0]
		if err := m.opts.Store.Commit(ctx, u.c, u.moves); err != nil {
			m.opts.Log.Error("a world change is not recorded yet; it is tried again", "change", u.c.String(), "err", err)
			return
		}
		m.unsaved = m.unsaved[1:]
	}
}

// Run calls Check every second on clock until ctx ends.
func (m *Manager) Run(ctx context.Context, clock func() time.Time) {
	if clock == nil {
		clock = time.Now
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		m.Check(ctx, clock())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RotationStatus is a rotating dungeon's state.
type RotationStatus struct {
	Dungeon string
	Rule    layout.RotationRule
	// Date is the active date, and Next the change that waits.
	Date string
	Next *Change
}

// RegionStatus is a region's season, and the change staff forced on it
// that waits, or nil.
type RegionStatus struct {
	Region string
	Season int
	Forced *Change
}

// Status is the state for the admin screens.
type Status struct {
	Rotations []RotationStatus
	// SeasonDays is procedural.seasons.days, Started the date seasons
	// started, Ends the count of season ends, and Next the next season
	// end's change.
	SeasonDays int
	Started    string
	Ends       int
	Next       *Change
	Regions    []RegionStatus
	// Checked is when Check last ran.
	Checked time.Time
}

// Status returns the state after the last Check. Any goroutine may call
// it.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	s.Rotations = slices.Clone(s.Rotations)
	s.Regions = slices.Clone(s.Regions)
	return s
}

func (m *Manager) publish(now time.Time) {
	s := Status{SeasonDays: m.opts.SeasonDays, Started: m.state.SeasonsStarted, Ends: m.state.SeasonEnds, Checked: now}
	copyOf := func(c *Change) *Change {
		if c == nil {
			return nil
		}
		out := *c
		return &out
	}
	for _, r := range m.opts.Rotations {
		s.Rotations = append(s.Rotations, RotationStatus{Dungeon: r.Dungeon, Rule: r.Rule, Date: m.state.Dates[r.Dungeon],
			Next: copyOf(m.planned["rotation:"+r.Dungeon])})
	}
	s.Next = copyOf(m.planned["season"])
	for _, r := range m.opts.Regions {
		s.Regions = append(s.Regions, RegionStatus{Region: r, Season: m.state.Seasons[r], Forced: copyOf(m.planned["region:"+r])})
	}
	m.mu.Lock()
	m.status = s
	m.mu.Unlock()
}
