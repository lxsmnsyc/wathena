package rotation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/layout"
)

// fakeHost records what the manager asks of it.
type fakeHost struct {
	log     []string
	applied []Change
	fail    error
}

func (h *fakeHost) Prepare(_ context.Context, _, _ State, c Change) error {
	h.log = append(h.log, "prepare "+c.String())
	return nil
}

func (h *fakeHost) Warn(_ context.Context, c Change, left time.Duration) {
	h.log = append(h.log, fmt.Sprintf("warn %s %s", c.String(), left))
}

func (h *fakeHost) Apply(_ context.Context, from, to State, c Change) ([]Move, error) {
	if h.fail != nil {
		return nil, h.fail
	}
	h.applied = append(h.applied, c)
	h.log = append(h.log, "apply "+c.String())
	var moves []Move
	if c.Kind == KindRotation && from.Dates[c.Dungeon] != "" {
		moves = append(moves, Move{From: layout.FloorID(c.Dungeon, from.Dates[c.Dungeon], 1), To: "gen:town:t1", X: 5, Y: 6})
	}
	_ = to
	return moves, nil
}

func (h *fakeHost) Done(_ context.Context, c Change) { h.log = append(h.log, "done "+c.String()) }

// fakeStore records commits, and fails while fail is set.
type fakeStore struct {
	commits []Change
	moves   [][]Move
	started []string
	fail    bool
}

func (s *fakeStore) Commit(_ context.Context, c Change, moves []Move) error {
	if s.fail {
		return errors.New("the database is away")
	}
	s.commits, s.moves = append(s.commits, c), append(s.moves, moves)
	return nil
}

func (s *fakeStore) StartSeasons(_ context.Context, date string) error {
	s.started = append(s.started, date)
	return nil
}

var manila, _ = time.LoadLocation("Asia/Manila")

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, manila)
	if err != nil {
		panic(err)
	}
	return t
}

func crypt() Rotation {
	return Rotation{Dungeon: "crypt", Rule: layout.RotationRule{Every: layout.EveryWeekly, Weekday: "friday", At: "20:00"}}
}

func cave() Rotation {
	return Rotation{Dungeon: "cave", Rule: layout.RotationRule{Every: layout.EveryDaily, At: "04:00"}}
}

func newState() State {
	return State{State: layout.State{Dates: map[string]string{}, Seasons: map[string]int{}}}
}

// TestDue: the state a world starts in from its stored state: the first
// dates, a rotation that came while the server was stopped, and the
// season ends that came (decision 0045).
func TestDue(t *testing.T) {
	opts := Options{Rotations: []Rotation{crypt(), cave()}, Location: manila}
	now := at("2026-10-17 21:00:00") // a Saturday
	st, due, err := Due(opts, newState(), now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Dates["crypt"] != "2026-10-16" || st.Dates["cave"] != "2026-10-17" || len(due) != 2 || !due[0].First || !due[1].First {
		t.Fatalf("first start: %v, %+v", st.Dates, due)
	}
	// Up to date: nothing is due.
	if _, due, _ := Due(opts, st, now); len(due) != 0 {
		t.Errorf("up to date: %+v", due)
	}
	// Stopped over a Friday at 20:00 and two mornings.
	later := at("2026-10-24 03:59:00")
	st2, due, _ := Due(opts, st, later)
	if st2.Dates["crypt"] != "2026-10-23" || st2.Dates["cave"] != "2026-10-23" || len(due) != 2 || due[0].First || due[0].Date != "2026-10-23" {
		t.Errorf("after a stop: %v %+v", st2.Dates, due)
	}
	// Seasons every 10 days, started on the 1st, with three regions: the
	// ends of the 11th and the 21st are due, in turn.
	opts.SeasonDays, opts.Regions = 10, []string{"t1", "t2", "t3"}
	s := newState()
	s.SeasonsStarted, s.Dates = "2026-10-01", st.Dates
	st3, due, _ := Due(opts, s, at("2026-10-21 00:00:00"))
	due = slices.DeleteFunc(due, func(c Change) bool { return c.Kind != KindRegion })
	if len(due) != 2 || due[0].Region != "t1" || due[0].Season != 1 || due[0].SeasonEnd != 1 || due[1].Region != "t2" || st3.SeasonEnds != 2 ||
		st3.Seasons["t1"] != 1 || st3.Seasons["t2"] != 1 {
		t.Errorf("seasons: %+v, %+v", due, st3)
	}
	// Seasons that never ran start today.
	st4, due, _ := Due(opts, newState(), now)
	if st4.SeasonsStarted != "2026-10-17" || slices.ContainsFunc(due, func(c Change) bool { return c.Kind == KindRegion }) {
		t.Errorf("new seasons: %+v %+v", st4, due)
	}
}

// run checks a manager each second from one time to another.
func run(m *Manager, from, to time.Time) {
	for t := from; !t.After(to); t = t.Add(time.Second) {
		m.Check(context.Background(), t)
	}
}

// TestRotationTiming: on the server clock, the next rotation is built at
// once, warned at 10, 5 and 1 minutes and 30 seconds, made at its time
// and recorded with its moves (strategy section 8.10).
func TestRotationTiming(t *testing.T) {
	h, s := &fakeHost{}, &fakeStore{}
	st := newState()
	st.Dates["crypt"] = "2026-10-16"
	m, err := New(Options{Rotations: []Rotation{crypt()}, Location: manila, Host: h, Store: s}, st)
	if err != nil {
		t.Fatal(err)
	}
	m.Check(context.Background(), at("2026-10-23 19:40:00"))
	if len(h.log) != 1 || h.log[0] != "prepare the rotation of crypt to 2026-10-23" {
		t.Fatalf("built before: %q", h.log)
	}
	if n := m.Status().Rotations[0].Next; n == nil || !n.Due.Equal(at("2026-10-23 20:00:00")) || n.Date != "2026-10-23" {
		t.Errorf("status %+v", n)
	}
	run(m, at("2026-10-23 19:40:01"), at("2026-10-23 20:00:05"))
	want := []string{
		"prepare the rotation of crypt to 2026-10-23",
		"warn the rotation of crypt to 2026-10-23 10m0s",
		"warn the rotation of crypt to 2026-10-23 5m0s",
		"warn the rotation of crypt to 2026-10-23 1m0s",
		"warn the rotation of crypt to 2026-10-23 30s",
		"apply the rotation of crypt to 2026-10-23",
		"done the rotation of crypt to 2026-10-23",
		// The next week's is built at once.
		"prepare the rotation of crypt to 2026-10-30",
	}
	if !slices.Equal(h.log, want) {
		t.Errorf("log:\n%s\nwant:\n%s", strings.Join(h.log, "\n"), strings.Join(want, "\n"))
	}
	if len(s.commits) != 1 || s.commits[0].Date != "2026-10-23" || len(s.moves[0]) != 1 || s.moves[0][0].From != "gen:dungeon:crypt:2026-10-16:1" {
		t.Errorf("commits %+v %+v", s.commits, s.moves)
	}
	if m.State().Dates["crypt"] != "2026-10-23" {
		t.Errorf("state %v", m.State().Dates)
	}
}

// TestClockJump: when the clock jumps past a rotation, as with --dev's
// clock or a long pause, the rotation happens at once without warnings.
func TestClockJump(t *testing.T) {
	h, s := &fakeHost{}, &fakeStore{}
	st := newState()
	st.Dates["cave"] = "2026-10-17"
	m, _ := New(Options{Rotations: []Rotation{cave()}, Location: manila, Host: h, Store: s}, st)
	m.Check(context.Background(), at("2026-10-17 12:00:00"))
	m.Check(context.Background(), at("2026-10-19 12:00:00"))
	if len(h.applied) != 1 || h.applied[0].Date != "2026-10-18" {
		t.Fatalf("applied %+v", h.applied)
	}
	// The next check catches up to the 19th.
	m.Check(context.Background(), at("2026-10-19 12:00:01"))
	if len(h.applied) != 2 || h.applied[1].Date != "2026-10-19" || len(s.commits) != 2 {
		t.Errorf("applied %+v", h.applied)
	}
}

// TestForce: staff force a rotation and a region's change; each happens
// after the countdown, with its warnings, and an unknown one is refused.
func TestForce(t *testing.T) {
	h, s := &fakeHost{}, &fakeStore{}
	st := newState()
	st.Dates["crypt"] = "2026-10-16"
	m, _ := New(Options{Rotations: []Rotation{crypt()}, Regions: []string{"t1", "t2"}, Location: manila, Host: h, Store: s}, st)
	now := at("2026-10-18 12:00:00")
	m.Check(context.Background(), now)
	done := make(chan error, 3)
	go func() {
		c, err := m.Force(context.Background(), "crypt", "", nil)
		if err == nil && (c.Date != "2026-10-23" || !c.Forced || !c.Due.Equal(now.Add(Countdown+time.Second))) {
			err = fmt.Errorf("rotation %+v", c)
		}
		done <- err
		c, err = m.Force(context.Background(), "", "t2", nil)
		if err == nil && (c.Region != "t2" || c.Season != 1 || !c.Forced) {
			err = fmt.Errorf("region %+v", c)
		}
		done <- err
		_, err = m.Force(context.Background(), "nope", "", nil)
		if !errors.Is(err, ErrNoDungeon) {
			err = fmt.Errorf("unknown dungeon: %w", err)
		} else {
			err = nil
		}
		done <- err
	}()
	for i := 0; i < 3; {
		m.Check(context.Background(), now.Add(time.Second))
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			i++
		case <-time.After(10 * time.Millisecond):
		}
	}
	run(m, now.Add(2*time.Second), now.Add(Countdown+2*time.Second))
	if len(h.applied) != 2 || h.applied[0].Kind != KindRegion || h.applied[1].Date != "2026-10-23" {
		t.Fatalf("applied %+v", h.applied)
	}
	if m.State().Seasons["t2"] != 1 || m.State().Dates["crypt"] != "2026-10-23" {
		t.Errorf("state %+v", m.State())
	}
	// The forced rotation took the 23rd's floors, so the 23rd's own time
	// changes nothing, and the next is the 30th.
	run(m, at("2026-10-23 19:59:59"), at("2026-10-23 20:00:01"))
	if len(h.applied) != 2 {
		t.Errorf("applied again %+v", h.applied[2:])
	}
	if n := m.Status().Rotations[0].Next; n == nil || n.Date != "2026-10-30" {
		t.Errorf("next %+v", n)
	}
}

// TestSeasons: with seasons on, they start today, and each season end
// changes the next region in turn, recorded with the count.
func TestSeasons(t *testing.T) {
	h, s := &fakeHost{}, &fakeStore{}
	m, _ := New(Options{SeasonDays: 2, Regions: []string{"t1", "t2"}, Location: manila, Host: h, Store: s}, newState())
	m.Check(context.Background(), at("2026-10-17 10:00:00"))
	if len(s.started) != 1 || s.started[0] != "2026-10-17" {
		t.Fatalf("started %v", s.started)
	}
	if n := m.Status().Next; n == nil || !n.Due.Equal(at("2026-10-19 00:00:00")) || n.Region != "t1" {
		t.Fatalf("next %+v", n)
	}
	run(m, at("2026-10-18 23:59:58"), at("2026-10-19 00:00:01"))
	m.Check(context.Background(), at("2026-10-21 00:00:00"))
	m.Check(context.Background(), at("2026-10-23 00:00:00"))
	got := ""
	for _, c := range s.commits {
		got += fmt.Sprintf("%s:%d:%d ", c.Region, c.Season, c.SeasonEnd)
	}
	if got != "t1:1:1 t2:1:2 t1:2:3 " {
		t.Errorf("commits %q", got)
	}
}

// TestRetry: a change the store cannot record yet is recorded at a
// later check, and one the host cannot make is tried again.
func TestRetry(t *testing.T) {
	h, s := &fakeHost{fail: errors.New("no maps")}, &fakeStore{fail: true}
	st := newState()
	st.Dates["cave"] = "2026-10-17"
	m, _ := New(Options{Rotations: []Rotation{cave()}, Location: manila, Host: h, Store: s}, st)
	m.Check(context.Background(), at("2026-10-18 04:00:00"))
	if len(h.applied) != 0 || m.State().Dates["cave"] != "2026-10-17" {
		t.Fatalf("a failed change ran: %+v", h.applied)
	}
	h.fail = nil
	m.Check(context.Background(), at("2026-10-18 04:00:01"))
	if len(h.applied) != 1 || len(s.commits) != 0 {
		t.Fatalf("applied %+v, commits %+v", h.applied, s.commits)
	}
	s.fail = false
	m.Check(context.Background(), at("2026-10-18 04:00:02"))
	if len(s.commits) != 1 || s.commits[0].Date != "2026-10-18" {
		t.Errorf("commits %+v", s.commits)
	}
}

// TestDaylightSaving: a rotation at a time that does not exist runs at
// the next valid minute, as the scheduler's times do.
func TestDaylightSaving(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	r := Rotation{Dungeon: "cave", Rule: layout.RotationRule{Every: layout.EveryDaily, At: "02:30"}}
	h, s := &fakeHost{}, &fakeStore{}
	st := newState()
	st.Dates["cave"] = "2027-03-13"
	m, _ := New(Options{Rotations: []Rotation{r}, Location: ny, Host: h, Store: s}, st)
	m.Check(context.Background(), time.Date(2027, 3, 14, 1, 0, 0, 0, ny))
	n := m.Status().Rotations[0].Next
	if n == nil || n.Date != "2027-03-14" || n.Due.In(ny).Hour() != 3 || n.Due.In(ny).Minute() != 0 {
		t.Errorf("next %+v", n)
	}
}

// forceAt forces a change while the manager checks at now, and returns
// what Force returned.
func forceAt(m *Manager, now time.Time, dungeon, region string, audit func(Change) error) (Change, error) {
	done := make(chan result, 1)
	go func() {
		c, err := m.Force(context.Background(), dungeon, region, audit)
		done <- result{c, err}
	}()
	for {
		m.Check(context.Background(), now)
		select {
		case r := <-done:
			return r.c, r.err
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestForceAhead: a dungeon that a force moved past its schedule's date
// cannot be forced again until real time catches up, so forces cannot
// hold back its natural rotation (review finding L39).
func TestForceAhead(t *testing.T) {
	h, s := &fakeHost{}, &fakeStore{}
	st := newState()
	st.Dates["crypt"] = "2026-10-16"
	m, _ := New(Options{Rotations: []Rotation{crypt()}, Location: manila, Host: h, Store: s}, st)
	now := at("2026-10-18 12:00:00")
	m.Check(context.Background(), now)
	if c, err := forceAt(m, now, "crypt", "", nil); err != nil || c.Date != "2026-10-23" {
		t.Fatalf("first force: %+v %v", c, err)
	}
	run(m, now.Add(time.Second), now.Add(Countdown+time.Second))
	if m.State().Dates["crypt"] != "2026-10-23" {
		t.Fatalf("state %+v", m.State())
	}
	later := now.Add(Countdown + 2*time.Second)
	if c, err := forceAt(m, later, "crypt", "", nil); !errors.Is(err, ErrAhead) {
		t.Errorf("second force: %+v %v", c, err)
	}
	// Once the 23rd's time came, a force may move it on again.
	if c, err := forceAt(m, at("2026-10-23 20:00:01"), "crypt", "", nil); err != nil || c.Date != "2026-10-30" {
		t.Errorf("force on the 23rd: %+v %v", c, err)
	}
}

// TestForceAuditFirst: the audit runs before the change is planned, and
// a failed audit plans nothing (review finding L39).
func TestForceAuditFirst(t *testing.T) {
	h, s := &fakeHost{}, &fakeStore{}
	st := newState()
	st.Dates["crypt"] = "2026-10-16"
	m, _ := New(Options{Rotations: []Rotation{crypt()}, Regions: []string{"t1"}, Location: manila, Host: h, Store: s}, st)
	now := at("2026-10-18 12:00:00")
	m.Check(context.Background(), now)
	fail := errors.New("no audit")
	var seen []Change
	audit := func(c Change) error {
		seen = append(seen, c)
		if p := m.planned[c.key()]; p != nil && p.Forced {
			t.Errorf("the change %+v was planned before its audit", c)
		}
		return fail
	}
	if _, err := forceAt(m, now, "crypt", "", audit); !errors.Is(err, fail) {
		t.Errorf("rotation: %v", err)
	}
	if _, err := forceAt(m, now, "", "t1", audit); !errors.Is(err, fail) {
		t.Errorf("region: %v", err)
	}
	if len(seen) != 2 || seen[0].Date != "2026-10-23" || seen[1].Season != 1 {
		t.Errorf("audited %+v", seen)
	}
	run(m, now.Add(time.Second), now.Add(Countdown+time.Second))
	if len(h.applied) != 0 || m.State().Dates["crypt"] != "2026-10-16" {
		t.Errorf("a change without its audit happened: %+v", h.applied)
	}
}
