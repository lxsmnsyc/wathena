// Package restart plans server restarts that staff set from the admin
// screens (proposal 0002 review decision 36, decision 0031):
//   - One plan at a time, with a time, a reason and a message.
//   - Players get announcements 30, 15, 10, 5 and 1 minute before, and
//     30 seconds before. A plan made later than a mark skips it.
//   - New logins are refused in the last minute.
//   - At the time, Due runs once. waserver then saves everyone,
//     disconnects the players with the reason, and exits with
//     ExitCode, which the process manager restarts.
//   - The plan lives in memory. A restart before the time, such as a
//     crash or a stop, drops it.
//
// The planner follows the server clock, so --dev's clock moves it.
package restart

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ExitCode is waserver's exit status after a planned restart. It is
// EX_TEMPFAIL of sysexits.h: the process ended on purpose and should
// start again. deploy/README.md, "Planned restarts", lists the process
// managers that restart on it.
const ExitCode = 75

// Marks are the times before a restart when players get an
// announcement.
var Marks = []time.Duration{30 * time.Minute, 15 * time.Minute, 10 * time.Minute, 5 * time.Minute, time.Minute, 30 * time.Second}

// Limits of a plan's time.
const (
	// LoginsClose is how long before the restart new logins are
	// refused.
	LoginsClose = time.Minute
	// MinLead is the least time from now to a planned restart, and
	// MaxLead the most.
	MinLead = time.Minute
	MaxLead = 7 * 24 * time.Hour
)

// Errors of the planner.
var (
	ErrPlanned  = errors.New("a restart is planned already; cancel it first")
	ErrNotTime  = errors.New("a restart is planned from 1 minute to 7 days ahead")
	ErrNoPlan   = errors.New("no restart is planned")
	ErrTooLate  = errors.New("the restart has begun")
	ErrNoReason = errors.New("a restart needs a reason")
)

// Plan is a planned restart.
type Plan struct {
	At time.Time
	// Reason says why, for the log, the disconnect message and
	// restart_planned. Message is what the announcements say, or the
	// reason when empty.
	Reason, Message string
	// By and ByID are the staff account that planned it, and Planned
	// when.
	By      string
	ByID    int64
	Planned time.Time
}

// Options configure a Planner.
type Options struct {
	// Clock is the server clock. It is time.Now when nil.
	Clock func() time.Time
	// Announce shows a line to every player of the world.
	Announce func(text string)
	// Notify tells the world of a new plan, or of a cancelled one with
	// nil, for restart_planned. It may be nil.
	Notify func(p *Plan)
	// Due runs once, at the restart's time, on the planner's goroutine.
	Due func(p Plan)
	Log *slog.Logger
}

// Planner holds the planned restart.
type Planner struct {
	opts Options

	mu   sync.Mutex
	plan *Plan
	// mark is the index of the next mark to announce, and due is set
	// once Due ran.
	mark int
	due  bool
}

// New makes a planner.
func New(opts Options) *Planner {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	return &Planner{opts: opts}
}

// Now is the server clock.
func (p *Planner) Now() time.Time { return p.opts.Clock() }

// Plan plans a restart. The time must be MinLead to MaxLead from now.
func (p *Planner) Plan(pl Plan) (Plan, error) {
	if pl.Reason == "" {
		return Plan{}, ErrNoReason
	}
	now := p.opts.Clock()
	lead := pl.At.Sub(now)
	if lead < MinLead || lead > MaxLead {
		return Plan{}, ErrNotTime
	}
	p.mu.Lock()
	if p.plan != nil {
		p.mu.Unlock()
		return Plan{}, ErrPlanned
	}
	pl.Planned = now
	p.plan, p.due = &pl, false
	// Only the marks still ahead are announced.
	p.mark = 0
	for p.mark < len(Marks) && Marks[p.mark] > lead {
		p.mark++
	}
	p.mu.Unlock()
	p.opts.Log.Info("restart planned", "at", pl.At.UTC().Format(time.RFC3339), "reason", pl.Reason, "by", pl.By)
	if p.opts.Notify != nil {
		p.opts.Notify(&pl)
	}
	return pl, nil
}

// Cancel drops the planned restart, and returns it.
func (p *Planner) Cancel() (Plan, error) {
	p.mu.Lock()
	if p.plan == nil {
		p.mu.Unlock()
		return Plan{}, ErrNoPlan
	}
	if p.due {
		p.mu.Unlock()
		return Plan{}, ErrTooLate
	}
	pl := *p.plan
	p.plan = nil
	p.mu.Unlock()
	p.opts.Log.Info("planned restart cancelled", "at", pl.At.UTC().Format(time.RFC3339))
	if p.opts.Announce != nil {
		p.opts.Announce("The planned server restart is cancelled.")
	}
	if p.opts.Notify != nil {
		p.opts.Notify(nil)
	}
	return pl, nil
}

// Current returns the planned restart.
func (p *Planner) Current() (Plan, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.plan == nil {
		return Plan{}, false
	}
	return *p.plan, true
}

// Closed reports whether new logins are refused, with the text a
// refused player sees: in the last minute before a planned restart.
func (p *Planner) Closed() (string, bool) {
	pl, ok := p.Current()
	if !ok || p.opts.Clock().Before(pl.At.Add(-LoginsClose)) {
		return "", false
	}
	return "The server restarts in less than a minute: " + pl.Reason + ". Come back in a few minutes.", true
}

// DisconnectText is what players see when the planned restart
// disconnects them.
func DisconnectText(pl Plan) string {
	return "The server is restarting: " + pl.Reason + ". Come back in a few minutes."
}

// Check announces the marks that came, and runs Due at the restart's
// time. When the clock passed several marks at once, only the last one
// is announced.
func (p *Planner) Check(now time.Time) {
	p.mu.Lock()
	if p.plan == nil || p.due {
		p.mu.Unlock()
		return
	}
	pl := *p.plan
	say := -1
	for p.mark < len(Marks) && !now.Before(pl.At.Add(-Marks[p.mark])) {
		say = p.mark
		p.mark++
	}
	due := !now.Before(pl.At)
	if due {
		p.due = true
		say = -1
	}
	p.mu.Unlock()
	if say >= 0 && p.opts.Announce != nil {
		p.opts.Announce(Announcement(pl, Marks[say]))
	}
	if due {
		p.opts.Log.Info("restarting as planned", "reason", pl.Reason)
		if p.opts.Due != nil {
			p.opts.Due(pl)
		}
	}
}

// Announcement is the line players see at a mark.
func Announcement(pl Plan, left time.Duration) string {
	text := pl.Message
	if text == "" {
		text = pl.Reason
	}
	return fmt.Sprintf("The server restarts in %s. %s", Left(left), text)
}

// Left says a time left, such as "5 minutes" or "30 seconds".
func Left(d time.Duration) string {
	if d >= time.Minute {
		n := int(d / time.Minute)
		if n == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", n)
	}
	n := int(d / time.Second)
	if n == 1 {
		return "1 second"
	}
	return fmt.Sprintf("%d seconds", n)
}

// Run checks the plan four times a second on the server clock until ctx
// ends.
func (p *Planner) Run(ctx context.Context) {
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.Check(p.opts.Clock())
		}
	}
}
