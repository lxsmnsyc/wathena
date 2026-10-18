package botclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/lxsmnsyc/wathena/server/metrics"
)

// Scenario is a load test: how many bots join, how fast, what they do,
// and for how long they are measured.
type Scenario struct {
	Name string `yaml:"name"`
	// URL is the server's WebSocket URL, such as ws://127.0.0.1:8300/ws.
	URL string `yaml:"url"`
	// Bots is how many bots join. Each plays the character named
	// NamePrefix and its number, which a server with development
	// sessions accepts as the session token.
	Bots       int    `yaml:"bots"`
	NamePrefix string `yaml:"name_prefix"`
	// Ramp is how many bots join per second. 0 joins them all at once.
	Ramp float64 `yaml:"ramp"`
	// Warmup is the time after the last bot joined before measuring
	// starts.
	Warmup time.Duration `yaml:"warmup"`
	// Duration is how long the measurement runs.
	Duration  time.Duration `yaml:"duration"`
	Behaviour Behaviour     `yaml:"behaviour"`
	// ChatRate, when set, is the chat lines per second of all bots
	// together. It sets Behaviour.ChatEvery to Bots / ChatRate.
	ChatRate float64 `yaml:"chat_rate"`
	// Seed makes the bots' choices repeatable.
	Seed uint64 `yaml:"seed"`
	// Place, for a procedural world, is where wabots puts the bots
	// through the database before they join: "fields" spreads them over
	// fields of several bands, and "towns" over the towns. Empty leaves
	// each bot where its character is.
	Place string `yaml:"place"`
	// MetricsURL is the server's --debug-addr as a URL, such as
	// http://127.0.0.1:8301. When set, the server's counts reset when
	// measuring starts, and the report includes them.
	MetricsURL string `yaml:"metrics_url"`
	// Progress, when set, gets a line about each stage.
	Progress func(string) `yaml:"-"`
}

// Report is what a scenario measured. Rates are per bot, over the
// measuring time.
type Report struct {
	Scenario string  `json:"scenario"`
	Bots     int     `json:"bots"`
	Joined   int     `json:"joined"`
	Seconds  float64 `json:"seconds"`
	// DialErrors are bots that could not join. Disconnects are bots
	// whose connection ended while they played, by reason.
	DialErrors  int            `json:"dial_errors"`
	Disconnects map[string]int `json:"disconnects"`
	// Errors are the first few error messages.
	Errors []string `json:"errors"`
	// Join is the time from dialing to the bot's first frame, for the
	// bots that joined.
	Join metrics.Summary `json:"join"`
	// In and Out are each bot's average bytes per second received and
	// sent. PeakIn and PeakOut are each bot's most bytes in one second.
	In      metrics.Spread `json:"in"`
	Out     metrics.Spread `json:"out"`
	PeakIn  metrics.Spread `json:"peak_in"`
	PeakOut metrics.Spread `json:"peak_out"`
	// FramesPerSec is the mean messages per second each bot received.
	FramesPerSec float64 `json:"frames_per_sec"`
	// Ack is the time from a walk or chat command to the frame that
	// acknowledges it.
	Ack metrics.Summary `json:"ack"`
	// ChatsHeard is the mean chat lines each bot heard per second.
	ChatsHeard float64 `json:"chats_heard_per_sec"`
	Kills      uint64  `json:"kills"`
	Damage     uint64  `json:"damage"`
	MapChanges int     `json:"map_changes"`
	// Maps counts the bots on each map at the end.
	Maps map[string]int `json:"maps"`
	// Server is the server's snapshot over the measuring time, when
	// MetricsURL is set.
	Server *metrics.Snapshot `json:"server,omitempty"`
}

type player struct {
	bot  *Bot
	err  error
	base Counts
	last Counts
	peak [2]uint64
}

// Names returns the bots' character names, in order.
func (s *Scenario) Names() []string {
	prefix := s.NamePrefix
	if prefix == "" {
		prefix = "bot"
	}
	out := make([]string, s.Bots)
	for i := range out {
		out[i] = fmt.Sprintf("%s%d", prefix, i+1)
	}
	return out
}

func (s *Scenario) progress(format string, args ...any) {
	if s.Progress != nil {
		s.Progress(fmt.Sprintf(format, args...))
	}
}

// Run runs the scenario: bots join at the ramp rate, play for the
// warmup, then are measured for the duration. Run returns an error only
// when the scenario cannot start. Bots that fail are counted in the
// report.
func (s *Scenario) Run(ctx context.Context) (*Report, error) {
	if s.URL == "" || s.Bots <= 0 || s.Duration <= 0 {
		return nil, errors.New("botclient: a scenario needs a URL, bots and a duration")
	}
	names := s.Names()
	if s.ChatRate > 0 {
		s.Behaviour.ChatEvery = time.Duration(float64(s.Bots) / s.ChatRate * float64(time.Second))
	}
	rep := &Report{Scenario: s.Name, Bots: s.Bots, Disconnects: map[string]int{}, Maps: map[string]int{}}
	var errMu sync.Mutex
	addErr := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		if len(rep.Errors) < 10 {
			rep.Errors = append(rep.Errors, err.Error())
		}
	}
	ack, join := &metrics.Histogram{}, &metrics.Histogram{}
	play, stop := context.WithCancel(ctx)
	defer stop()
	players := make([]*player, s.Bots)
	var joined, playing sync.WaitGroup
	start := time.Now()
	s.progress("%d bots joining at %.0f a second", s.Bots, s.Ramp)
	for i := range s.Bots {
		if s.Ramp > 0 {
			at := start.Add(time.Duration(float64(i) / s.Ramp * float64(time.Second)))
			select {
			case <-time.After(time.Until(at)):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		p := &player{}
		players[i] = p
		joined.Add(1)
		go func() {
			defer joined.Done()
			dial, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			began := time.Now()
			b, err := Dial(dial, s.URL, []byte(names[i]), Options{Ack: ack})
			if err != nil {
				p.err = err
				addErr(err)
				return
			}
			join.Record(time.Since(began))
			p.bot = b
			playing.Go(func() {
				rnd := rand.New(rand.NewPCG(s.Seed, uint64(i))) // #nosec G404 G115 -- bot choices, not secrets, from a bot number 0 or more.
				if err := b.Play(play, s.Behaviour, rnd); err != nil {
					p.err = err
					addErr(err)
				}
			})
		}()
	}
	joined.Wait()
	for _, p := range players {
		if p.bot != nil {
			rep.Joined++
		} else {
			rep.DialErrors++
		}
	}
	rep.Join = join.Summary()
	s.progress("%d bots joined in %s, %d failed; warming up for %s", rep.Joined, time.Since(start).Round(time.Millisecond), rep.DialErrors, s.Warmup)
	select {
	case <-time.After(s.Warmup):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Measuring starts.
	if s.MetricsURL != "" {
		if err := post(ctx, s.MetricsURL+"/debug/wathena/reset"); err != nil {
			return nil, err
		}
	}
	ack.Reset()
	for _, p := range players {
		if p.bot != nil {
			p.base = p.bot.Counts()
			p.last = p.base
		}
	}
	began := time.Now()
	s.progress("measuring for %s", s.Duration)
	tick := time.NewTicker(time.Second)
	end := time.After(s.Duration)
measure:
	for {
		select {
		case <-ctx.Done():
			tick.Stop()
			return nil, ctx.Err()
		case <-end:
			break measure
		case <-tick.C:
			for _, p := range players {
				if p.bot == nil {
					continue
				}
				c := p.bot.Counts()
				p.peak[0] = max(p.peak[0], c.BytesIn-p.last.BytesIn)
				p.peak[1] = max(p.peak[1], c.BytesOut-p.last.BytesOut)
				p.last = c
			}
		}
	}
	tick.Stop()
	rep.Seconds = time.Since(began).Seconds()
	if s.MetricsURL != "" {
		snap, err := fetch(ctx, s.MetricsURL+"/debug/wathena/metrics")
		if err != nil {
			return nil, err
		}
		rep.Server = snap
	}
	var in, out, peakIn, peakOut []float64
	var frames, chats float64
	for _, p := range players {
		if p.bot == nil {
			continue
		}
		c := p.bot.Counts()
		in = append(in, float64(c.BytesOut-p.base.BytesOut)/rep.Seconds)
		out = append(out, float64(c.BytesIn-p.base.BytesIn)/rep.Seconds)
		// The bot's in is the server's out.
		peakIn = append(peakIn, float64(p.peak[1]))
		peakOut = append(peakOut, float64(p.peak[0]))
		frames += float64(c.MsgsIn-p.base.MsgsIn) / rep.Seconds
		chats += float64(c.ChatsHeard-p.base.ChatsHeard) / rep.Seconds
		rep.Kills += c.Kills - p.base.Kills
		rep.Damage += c.Damage - p.base.Damage
		rep.MapChanges += c.MapChanges
		rep.Maps[p.bot.View().Map]++
	}
	if n := float64(len(in)); n > 0 {
		rep.FramesPerSec, rep.ChatsHeard = frames/n, chats/n
	}
	rep.In, rep.Out = spread(in), spread(out)
	rep.PeakIn, rep.PeakOut = spread(peakIn), spread(peakOut)
	rep.Ack = ack.Summary()

	// Bots leave.
	stop()
	playing.Wait()
	for _, p := range players {
		if p.err != nil {
			var d *DisconnectError
			switch {
			case errors.As(p.err, &d):
				rep.Disconnects[d.Reason.String()]++
			case p.bot != nil:
				rep.Disconnects["connection error"]++
			}
		}
	}
	var closing sync.WaitGroup
	for _, p := range players {
		if p.bot != nil {
			closing.Go(func() { _ = p.bot.Close() })
		}
	}
	closing.Wait()
	return rep, nil
}

func spread(v []float64) metrics.Spread {
	if len(v) == 0 {
		return metrics.Spread{}
	}
	slices.Sort(v)
	var sum float64
	for _, x := range v {
		sum += x
	}
	at := func(q float64) float64 { return v[min(len(v)-1, int(q*float64(len(v))))] }
	return metrics.Spread{Mean: sum / float64(len(v)), P50: at(0.5), P99: at(0.99), Max: v[len(v)-1]}
}

func post(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("botclient: %s: %s", url, resp.Status)
	}
	return nil
}

func fetch(ctx context.Context, url string) (*metrics.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("botclient: %s: %s", url, resp.Status)
	}
	var s metrics.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Limits are the budgets a report is checked against.
type Limits struct {
	// WorkP99 and WorkMax limit each zone's tick work.
	WorkP99, WorkMax time.Duration
	// OutAvg and OutPeak limit each bot's bytes per second from the
	// server, on average and in its busiest second. InAvg limits each
	// bot's bytes per second to the server.
	OutAvg, OutPeak, InAvg float64
	// Errors is how many bots may fail to join or lose their
	// connection.
	Errors int
}

// Budgets are the limits of strategy section 3.8 for a busy town.
var Budgets = Limits{
	WorkP99: metrics.TickBudget, WorkMax: metrics.TickMax,
	OutAvg: 8 << 10, OutPeak: 32 << 10, InAvg: 1 << 10,
}

// Check returns each limit the report breaks. Zone limits need the
// server's snapshot.
func (r *Report) Check(l Limits) []string {
	var out []string
	failed := r.DialErrors
	for _, n := range r.Disconnects {
		failed += n
	}
	if failed > l.Errors {
		out = append(out, fmt.Sprintf("%d bots failed, over %d", failed, l.Errors))
	}
	if r.Server != nil {
		for _, z := range r.Server.Zones {
			if l.WorkP99 > 0 && z.Work.P99 > l.WorkP99 {
				out = append(out, fmt.Sprintf("%s: tick work p99 %v is over %v", z.Name, z.Work.P99, l.WorkP99))
			}
			if l.WorkMax > 0 && z.Work.Max > l.WorkMax {
				out = append(out, fmt.Sprintf("%s: tick work max %v is over %v", z.Name, z.Work.Max, l.WorkMax))
			}
		}
	} else if l.WorkP99 > 0 || l.WorkMax > 0 {
		out = append(out, "no server measurements to check the tick work against")
	}
	if l.OutAvg > 0 && r.Out.Max > l.OutAvg {
		out = append(out, fmt.Sprintf("a bot received %.0f bytes/s on average, over %.0f", r.Out.Max, l.OutAvg))
	}
	if l.OutPeak > 0 && r.PeakOut.Max > l.OutPeak {
		out = append(out, fmt.Sprintf("a bot received %.0f bytes in one second, over %.0f", r.PeakOut.Max, l.OutPeak))
	}
	if l.InAvg > 0 && r.In.Max > l.InAvg {
		out = append(out, fmt.Sprintf("a bot sent %.0f bytes/s on average, over %.0f", r.In.Max, l.InAvg))
	}
	return out
}

func kb(b float64) string { return fmt.Sprintf("%.2f", b/1024) }

func ms(d time.Duration) string { return fmt.Sprintf("%.1f", float64(d)/float64(time.Millisecond)) }

// Text formats the report as tables.
func (r *Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "scenario %s: %d of %d bots joined, measured for %.0f s\n", r.Scenario, r.Joined, r.Bots, r.Seconds)
	fmt.Fprintf(&b, "failed to join: %d; lost connections: %v\n", r.DialErrors, r.Disconnects)
	fmt.Fprintf(&b, "dial to first frame: p50 %s ms, p99 %s ms, max %s ms\n", ms(r.Join.P50), ms(r.Join.P99), ms(r.Join.Max))
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "  error: %s\n", e)
	}
	maps := make([]string, 0, len(r.Maps))
	for m := range r.Maps {
		maps = append(maps, m)
	}
	sort.Slice(maps, func(i, j int) bool { return r.Maps[maps[i]] > r.Maps[maps[j]] })
	var parts []string
	for _, m := range maps {
		parts = append(parts, fmt.Sprintf("%s %d", m, r.Maps[m]))
	}
	fmt.Fprintf(&b, "bots per map at the end: %s; map changes %d\n", strings.Join(parts, ", "), r.MapChanges)
	fmt.Fprintf(&b, "per bot: %.1f frames/s, %.2f chat lines heard/s; kills %d, damage %d\n", r.FramesPerSec, r.ChatsHeard, r.Kills, r.Damage)
	fmt.Fprintf(&b, "command to acknowledging frame: p50 %s ms, p99 %s ms, max %s ms (%d commands)\n\n", ms(r.Ack.P50), ms(r.Ack.P99), ms(r.Ack.Max), r.Ack.Count)
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(w, "per bot, KB/s\tmean\tp50\tp99\tmax\tbudget\t")
	for _, row := range []struct {
		name   string
		v      metrics.Spread
		budget string
	}{
		{"server to client, average", r.Out, "8"},
		{"server to client, peak second", r.PeakOut, "32"},
		{"client to server, average", r.In, "1"},
		{"client to server, peak second", r.PeakIn, ""},
	} {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t\n", row.name, kb(row.v.Mean), kb(row.v.P50), kb(row.v.P99), kb(row.v.Max), row.budget)
	}
	_ = w.Flush()
	if r.Server != nil {
		fmt.Fprintf(&b, "\nserver:\n%s", r.Server.Text())
	}
	return b.String()
}
