package metrics

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	httppprof "net/http/pprof" // #nosec G108 -- the handlers go on Registry.Handler's own mux, which waserver serves only on a loopback address.
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

// Registry holds the measurements of one server process.
type Registry struct {
	Conns   Conns
	Process Process
	// Log gets the over-budget warnings. Without it, they go nowhere.
	Log *slog.Logger
	// ProfileDir is where a CPU profile is written when a zone is over
	// budget. Empty means no profiles.
	ProfileDir string
	// ProfileFor is how long a profile records. It is 10 seconds when 0.
	ProfileFor time.Duration
	// ProfileEvery is the least time between two profiles. It is 5
	// minutes when 0.
	ProfileEvery time.Duration
	// WarnEvery is the least time between two warnings for one zone.
	// It is 1 minute when 0. Warnings in between are counted.
	WarnEvery time.Duration

	mu          sync.Mutex
	zones       []*Zone
	since       time.Time
	lastProfile time.Time
	warned      map[string]time.Time
	quiet       map[string]int
	profiles    int
}

// NewRegistry returns a registry whose counts start now.
func NewRegistry() *Registry {
	r := &Registry{since: time.Now()}
	r.Process.Reset()
	return r
}

// AddZone adds a zone's measurements and sets its OnOverBudget to log a
// warning and write a profile. Call it before the zone runs.
func (r *Registry) AddZone(z *Zone) {
	z.OnOverBudget = r.overBudget
	r.mu.Lock()
	r.zones = append(r.zones, z)
	r.mu.Unlock()
}

// Reset starts every count again from now.
func (r *Registry) Reset() {
	r.mu.Lock()
	zones := r.zones
	r.since = time.Now()
	r.mu.Unlock()
	for _, z := range zones {
		z.Reset()
	}
	r.Conns.Reset()
	r.Process.Reset()
}

func durationOr(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// overBudget runs on a zone's goroutine, so it only logs and starts a
// profile on another goroutine.
func (r *Registry) overBudget(z *Zone, recent []time.Duration) {
	now := time.Now()
	r.mu.Lock()
	if r.warned == nil {
		r.warned, r.quiet = map[string]time.Time{}, map[string]int{}
	}
	if now.Sub(r.warned[z.Name]) < durationOr(r.WarnEvery, time.Minute) {
		r.quiet[z.Name]++
		r.mu.Unlock()
		return
	}
	quiet := r.quiet[z.Name]
	r.warned[z.Name], r.quiet[z.Name] = now, 0
	var file string
	if r.ProfileDir != "" && now.Sub(r.lastProfile) >= durationOr(r.ProfileEvery, 5*time.Minute) {
		r.lastProfile = now
		r.profiles++
		name := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(z.Name)
		file = filepath.Join(r.ProfileDir, fmt.Sprintf("cpu-%s-%s-%d.pprof", name, now.Format("20060102T150405"), r.profiles))
	}
	r.mu.Unlock()
	works := make([]string, len(recent))
	for i, d := range recent {
		works[i] = d.Round(10 * time.Microsecond).String()
	}
	snap := z.Snapshot()
	args := []any{
		"zone", z.Name, "players", snap.Players, "work", strings.Join(works, " "),
		"budget", TickBudget, "phase_mean", fmt.Sprint(snap.PhaseMean),
		"earlier_warnings_skipped", quiet,
	}
	if file != "" {
		args = append(args, "profile", file)
		go r.profile(file)
	}
	if r.Log != nil {
		r.Log.Warn(fmt.Sprintf("zone over the tick budget for %d ticks in a row", len(recent)), args...)
	}
}

// profile records a CPU profile into file. It does nothing when another
// CPU profile is running, such as one from /debug/pprof/profile.
func (r *Registry) profile(file string) {
	if err := os.MkdirAll(r.ProfileDir, 0o750); err != nil {
		r.logError("cannot make the profile folder", err)
		return
	}
	f, err := os.Create(file) // #nosec G304 -- the operator's folder and a name made here.
	if err != nil {
		r.logError("cannot write a profile", err)
		return
	}
	defer func() { _ = f.Close() }()
	if err := pprof.StartCPUProfile(f); err != nil {
		r.logError("cannot start a profile", err)
		return
	}
	time.Sleep(durationOr(r.ProfileFor, 10*time.Second))
	pprof.StopCPUProfile()
}

func (r *Registry) logError(msg string, err error) {
	if r.Log != nil {
		r.Log.Error(msg, "err", err)
	}
}

// Snapshot is every measurement since the last reset.
type Snapshot struct {
	Since   time.Time       `json:"since"`
	Seconds float64         `json:"seconds"`
	Budget  Budgets         `json:"budget"`
	Zones   []ZoneSnapshot  `json:"zones"`
	Conns   ConnSnapshot    `json:"connections"`
	Process ProcessSnapshot `json:"process"`
}

// Budgets are the tick budgets the counts compare against.
type Budgets struct {
	Tick   time.Duration `json:"tick_ns"`
	Work   time.Duration `json:"work_p99_ns"`
	Max    time.Duration `json:"work_max_ns"`
	Streak int           `json:"streak"`
}

// Snapshot returns every measurement. Zones that never ticked since the
// last reset and have no players are left out.
func (r *Registry) Snapshot() Snapshot {
	r.mu.Lock()
	zones, since := r.zones, r.since
	r.mu.Unlock()
	s := Snapshot{
		Since: since, Seconds: time.Since(since).Seconds(),
		Budget: Budgets{Tick: 50 * time.Millisecond, Work: TickBudget, Max: TickMax, Streak: OverBudgetStreak},
		Conns:  r.Conns.Snapshot(), Process: r.Process.Snapshot(),
	}
	for _, z := range zones {
		if zs := z.Snapshot(); zs.Ticks > 0 || zs.Players > 0 {
			s.Zones = append(s.Zones, zs)
		}
	}
	return s
}

func ms(d time.Duration) string { return fmt.Sprintf("%.2f", float64(d)/float64(time.Millisecond)) }

func kb(b float64) string { return fmt.Sprintf("%.2f", b/1024) }

// Text formats the snapshot as tables.
func (s Snapshot) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "measured for %.0f s; tick work budget: p99 %s ms, max %s ms\n\n", s.Seconds, ms(s.Budget.Work), ms(s.Budget.Max))
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(w, "zone\tplayers\tticks\twork p50 ms\tp99 ms\tp99.9 ms\tmax ms\tover 10 ms\tover 25 ms\tstep p99 ms\tmove/act/send mean ms\t")
	for _, z := range s.Zones {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s/%s/%s\t\n", z.Name, z.Players, z.Ticks,
			ms(z.Work.P50), ms(z.Work.P99), ms(z.Work.P999), ms(z.Work.Max), z.OverBudget, z.OverMax, ms(z.Step.P99),
			ms(z.PhaseMean["move"]), ms(z.PhaseMean["act"]), ms(z.PhaseMean["send"]))
	}
	_ = w.Flush()
	p := s.Process
	fmt.Fprintf(&b, "\nprocess: CPU %.2f cores (GC %.2f), %d GC cycles, GC pause p50 %s ms p99 %s ms max %s ms, scheduler latency p99 %s ms, heap %d MB, total %d MB, %d goroutines, %.1f MB/s allocated\n",
		p.CPU, p.GCCPU, p.GCCycles, ms(p.GCPauses.P50), ms(p.GCPauses.P99), ms(p.GCPauses.Max), ms(p.SchedDelay.P99),
		p.HeapBytes>>20, p.TotalBytes>>20, p.Goroutines, p.AllocRate/(1<<20))
	c := s.Conns
	fmt.Fprintf(&b, "connections: %d open (%d opened, %d closed), %d messages out, %d in\n", c.Live, c.Opened, c.Closed, c.MsgsOut, c.MsgsIn)
	w = tabwriter.NewWriter(&b, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(w, "per connection, KB/s\tmean\tp50\tp99\tmax\t")
	for _, row := range []struct {
		name string
		v    Spread
	}{{"server to client, average", c.OutRate}, {"server to client, peak second", c.PeakOut}, {"client to server, average", c.InRate}, {"client to server, peak second", c.PeakIn}} {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t\n", row.name, kb(row.v.Mean), kb(row.v.P50), kb(row.v.P99), kb(row.v.Max))
	}
	_ = w.Flush()
	return b.String()
}

// Handler serves the measurements and Go's profiles:
//
//   - GET /debug/wathena/metrics returns the Snapshot as JSON, or as
//     text with ?format=text.
//   - POST /debug/wathena/reset starts every count again.
//   - /debug/pprof/ serves Go's profiles.
//
// It has no access control. Serve it only on a loopback address.
func (r *Registry) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/wathena/metrics", func(w http.ResponseWriter, req *http.Request) {
		s := r.Snapshot()
		if req.URL.Query().Get("format") == "text" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(s.Text()))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(s)
	})
	mux.HandleFunc("POST /debug/wathena/reset", func(w http.ResponseWriter, _ *http.Request) {
		r.Reset()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/debug/pprof/", httppprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", httppprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", httppprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", httppprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", httppprof.Trace)
	return mux
}
