package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// The phase 5 exit check (strategy, "Phase 5" exit criteria): groups of
// bots trade, store, mail, vend, sell to buying stores, buy at the
// Exchange and drop items, all at the same time and on both maps, while
// the test audits. In the middle, the server restarts under load. The
// totals of DupTest hold the whole time, and the bots' tokens hold too:
//   - Only TokenMove takes tokens, and it gives 1 Red Potion for each
//     TokensPerPotion tokens. So the audits count each Red Potion as that
//     many tokens, and the tokens plus the Red Potions stay constant.
//   - Ground items are not saved, so before the restart the groups that
//     drop items stop and pick up every ground item. The other groups
//     keep moving while the server stops.
//   - The saved totals at the restart are the totals of the start. After
//     the restart, the bots log in again and wait until an audit with
//     every bot equals what the database saved. Then they go on.
//   - At the end, CheckDatabase checks what the server saved against the
//     last audit, and the zeny log checks the tokens: the tokens given,
//     less the tokens the shop took, are the tokens the characters hold.

// ExitGroup is a group of bots of the exit check that play together on
// one map.
type ExitGroup struct {
	// Prefix names the bots: Prefix plus a number from 1. It starts with
	// the ExitCheck's Prefix.
	Prefix string
	Bots   int
	// Map is where the bots play: StartMap, or FieldMap, where the bots
	// go with the fixture's Guide.
	Map string
	// Moves, Settle and Setup are as DupTest's, for this group's bots.
	Moves  []Move
	Settle func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error)
	Setup  func(ctx context.Context, bots []*botclient.Bot) error
	// Ground marks a group that drops items. Before the restart its bots
	// stop and run Settle until no item is on the ground.
	Ground bool
}

// ExitCheck is the phase 5 exit check.
type ExitCheck struct {
	Server *Server
	// Prefix starts every group's prefix. The database checks read the
	// characters whose names start with it.
	Prefix string
	Groups []ExitGroup
	// Duration is how long the bots make moves, the restart left out.
	// The restart comes after half of it.
	Duration time.Duration
	Seed     uint64
	// AuditEvery is the time between audits. It is 500 ms when 0.
	AuditEvery time.Duration
}

// ExitReport is what the exit check did.
type ExitReport struct {
	// Moves counts the moves that happened, by name.
	Moves map[string]int
	// Audits counts the audits with every bot online, and Partial the
	// ones with a bot away.
	Audits, Partial int
	// Reconnects counts the logins again after a connection closed, by a
	// move or by the server, and Restarted the logins again after the
	// restart. Closed lists the connections the server closed outside
	// the restart, with why.
	Reconnects, Restarted int
	Closed                []string
	// Ground counts the drops and pick-ups that the ground groups' bots
	// saw, as DupReport's Dropped and Added.
	Ground DupReport
	// RestartTook is how long the restart took, from the stop until
	// every bot was back.
	RestartTook time.Duration
	// Start, Saved, Restored and End are the totals at the start, in the
	// database at the restart, in the first audit with every bot after
	// it, and at the end.
	Start, Saved, Restored, End Totals
}

// bots is how many bots the check runs.
func (x ExitCheck) bots() int {
	n := 0
	for _, g := range x.Groups {
		n += g.Bots
	}
	return n
}

// exitBot is one bot of the check: its group and its name.
type exitBot struct {
	group int
	name  string
}

// gate holds the bots after a restart until the test lets them go on.
// waiting counts the bots that wait.
type gate struct {
	mu      sync.Mutex
	held    chan struct{}
	waiting atomic.Int64
}

func (g *gate) hold() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.held = make(chan struct{})
}

func (g *gate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.held != nil {
		close(g.held)
		g.held = nil
	}
}

// wait waits until the gate opens, ctx ends or the bot's connection
// closes.
func (g *gate) wait(ctx context.Context, b *botclient.Bot) {
	g.mu.Lock()
	c := g.held
	g.mu.Unlock()
	if c == nil {
		return
	}
	g.waiting.Add(1)
	defer g.waiting.Add(-1)
	select {
	case <-c:
	case <-ctx.Done():
	case <-b.Done():
	}
}

// FoldPotions returns a copy of an audit in which each Red Potion counts
// as TokensPerPotion tokens of its holder, and the Red Potions count as
// 0. The serials stay, so a serial held twice still shows.
func FoldPotions(zones []AuditZone) []AuditZone {
	out := make([]AuditZone, len(zones))
	for i, z := range zones {
		out[i] = z
		out[i].Players = make([]AuditPlayer, len(z.Players))
		for j, p := range z.Players {
			p.Items = append([]AuditItem(nil), p.Items...)
			cur := map[string]int64{}
			for name, n := range p.Currencies {
				cur[name] = n
			}
			for k, it := range p.Items {
				if it.Item == RedPotion {
					cur[Tokens] += int64(it.Amount) * TokensPerPotion
					p.Items[k].Amount = 0
				}
			}
			p.Currencies = cur
			out[i].Players[j] = p
		}
	}
	return out
}

// foldTotals is FoldPotions for totals.
func foldTotals(t Totals) Totals {
	f := t
	f.Items = map[uint32]int{}
	for id, n := range t.Items {
		f.Items[id] = n
	}
	f.Currencies = map[string]int64{}
	for name, n := range t.Currencies {
		f.Currencies[name] = n
	}
	f.Currencies[Tokens] += int64(f.Items[RedPotion]) * TokensPerPotion
	f.Items[RedPotion] = 0
	return f
}

// Run runs the check. It fails t for every check that does not hold.
func (x ExitCheck) Run(t *testing.T) ExitReport {
	t.Helper()
	srv := x.Server
	if srv.DB == nil {
		t.Fatal("the exit check needs a server with a database")
	}
	if x.AuditEvery <= 0 {
		x.AuditEvery = 500 * time.Millisecond
	}
	var all []exitBot
	for gi, g := range x.Groups {
		if !strings.HasPrefix(g.Prefix, x.Prefix) {
			t.Fatalf("group %s does not start with %s", g.Prefix, x.Prefix)
		}
		for i := range g.Bots {
			all = append(all, exitBot{gi, fmt.Sprintf("%s%d", g.Prefix, i+1)})
		}
	}
	n := len(all)
	d := DupTest{Server: srv, Bots: n, Prefix: x.Prefix}
	ctx, cancel := context.WithTimeout(context.Background(), x.Duration+15*time.Minute)
	defer cancel()
	bots := &dupBots{now: make([]*botclient.Bot, n)}
	t.Cleanup(bots.close)
	// serving is held for writing while the server restarts, so no
	// audit or login runs then.
	var serving sync.RWMutex
	dial := func(i int) (*botclient.Bot, error) {
		serving.RLock()
		defer serving.RUnlock()
		b, err := botclient.Dial(ctx, srv.WS, []byte(all[i].name), botclient.Options{Record: true})
		if err != nil {
			return nil, err
		}
		if err := place(ctx, b, x.Groups[all[i].group].Map); err != nil {
			_ = b.Close()
			return nil, err
		}
		return b, nil
	}
	audit := func() ([]AuditZone, error) {
		serving.RLock()
		defer serving.RUnlock()
		return srv.Audit(ctx)
	}

	// Each group logs in, goes to its map and runs its setup, beside the
	// other groups.
	var wg sync.WaitGroup
	setupErrs := make([]error, len(x.Groups))
	for gi, g := range x.Groups {
		wg.Go(func() {
			var group []*botclient.Bot
			for i, eb := range all {
				if eb.group != gi {
					continue
				}
				b, err := dial(i)
				if err != nil {
					setupErrs[gi] = fmt.Errorf("%s: %w", eb.name, err)
					return
				}
				bots.replace(i, b)
				group = append(group, b)
			}
			if g.Setup != nil {
				if err := g.Setup(ctx, group); err != nil {
					setupErrs[gi] = fmt.Errorf("group %s: setup: %w", g.Prefix, err)
				}
			}
		})
	}
	wg.Wait()
	// The first logins are not logins again.
	bots.mu.Lock()
	bots.gone = nil
	bots.mu.Unlock()
	if err := errors.Join(setupErrs...); err != nil {
		t.Fatal(err)
	}
	start, err := d.audit(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	startValue := foldTotals(start)
	rep := ExitReport{Moves: map[string]int{}, Start: start}

	var mu sync.Mutex
	var failed atomic.Bool
	// The moves stop after Duration, or at the first failure.
	moving, stop := context.WithCancel(ctx)
	defer stop()
	fail := func(format string, args ...any) {
		failed.Store(true)
		t.Errorf(format, args...)
		stop()
	}
	// restarting is set from just before the stop until the bots go on,
	// and quiet while the ground groups pick up before the restart.
	var restarting, quiet atomic.Bool
	var after gate
	// again logs bot i in again when its connection closed. left is set
	// when the bot closed it. During a restart, the bot waits at the gate
	// once it is back, and logs in again if its connection closes there.
	again := func(i int, b *botclient.Bot, left bool) (*botclient.Bot, error) {
		for {
			select {
			case <-b.Done():
			default:
				return b, nil
			}
			why := b.Err()
			var nb *botclient.Bot
			var err error
			for try := 0; try < 5 && ctx.Err() == nil; try++ {
				if nb, err = dial(i); err == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if err != nil {
				return nil, fmt.Errorf("logging in again after %v: %w", why, err)
			}
			bots.replace(i, nb)
			mu.Lock()
			if restarting.Load() {
				rep.Restarted++
			} else {
				rep.Reconnects++
				if !left {
					rep.Closed = append(rep.Closed, fmt.Sprintf("%s: %v", b.Name, why))
				}
			}
			mu.Unlock()
			b, left = nb, false
			after.wait(ctx, b)
		}
	}

	for i, eb := range all {
		g := x.Groups[eb.group]
		rnd := rand.New(rand.NewPCG(x.Seed, uint64(i)+1)) // #nosec G404 G115 -- test choices, not secrets, from small counts.
		wg.Go(func() {
			b := bots.get(i)
			for moving.Err() == nil {
				name := "settle"
				var done bool
				var err error
				if g.Ground && quiet.Load() {
					var more bool
					more, err = g.Settle(moving, b, rnd)
					if err == nil && !more {
						err = pause(moving)
					}
				} else {
					m := pick(g.Moves, rnd)
					name = m.Name
					done, err = m.Do(moving, b, rnd)
				}
				left := errors.Is(err, errLeft)
				if left {
					err = nil
				}
				if err != nil && moving.Err() == nil && !closed(b) && !restarting.Load() {
					fail("%s: %s: %v", b.Name, name, closedErr(b, err))
					return
				}
				if done {
					mu.Lock()
					rep.Moves[name]++
					mu.Unlock()
				}
				if b, err = again(i, b, left); err != nil {
					fail("%s: %v", eb.name, err)
					return
				}
			}
		})
	}
	audits := make(chan struct{})
	go func() {
		defer close(audits)
		tick := time.NewTicker(x.AuditEvery)
		defer tick.Stop()
		for {
			select {
			case <-moving.Done():
				return
			case <-tick.C:
			}
			zones, err := audit()
			if err != nil {
				if moving.Err() == nil {
					fail("audit: %v", err)
				}
				return
			}
			full, ok := d.check(t, startValue, FoldPotions(zones), "during the moves")
			if !ok {
				failed.Store(true)
				stop()
				return
			}
			mu.Lock()
			if full {
				rep.Audits++
			} else {
				rep.Partial++
			}
			mu.Unlock()
		}
	}()
	// The test's goroutine stops the moves after Duration, with the
	// restart in the middle.
	if sleep(moving, x.Duration/2) {
		if err := x.restart(moving, t, &rep, restartHooks{
			audit: audit, quiet: &quiet, restarting: &restarting, gate: &after, serving: &serving, value: startValue, bots: n,
		}); err != nil {
			fail("restart: %v", err)
		}
		sleep(moving, x.Duration-x.Duration/2)
	}
	stop()
	wg.Wait()
	<-audits
	if failed.Load() {
		t.FailNow()
	}

	// Every bot settles, as in DupTest.
	for round := 0; round < 5; round++ {
		for i, eb := range all {
			g := x.Groups[eb.group]
			if g.Settle == nil {
				continue
			}
			rnd := rand.New(rand.NewPCG(x.Seed, uint64(n*(round+1)+i)+1)) // #nosec G404 G115 -- test choices, not secrets, from small counts.
			wg.Go(func() {
				b := bots.get(i)
				for ctx.Err() == nil {
					more, err := g.Settle(ctx, b, rnd)
					if err != nil && !closed(b) {
						fail("%s: settling: %v", b.Name, closedErr(b, err))
						return
					}
					if b, err = again(i, b, false); err != nil {
						fail("%s: settling: %v", eb.name, err)
						return
					}
					if !more {
						return
					}
				}
			})
		}
		wg.Wait()
		zones, err := srv.Audit(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if now, _ := TotalsOf(zones); now.Ground == 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if failed.Load() {
		t.FailNow()
	}
	zones, err := d.online(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if full, ok := d.check(t, startValue, FoldPotions(zones), "at the end"); !ok || !full {
		t.FailNow()
	}
	rep.Audits++
	rep.End, _ = TotalsOf(zones)
	d.checkBots(ctx, t, bots.all())
	for _, b := range bots.every() {
		for _, g := range x.Groups {
			if g.Ground && strings.HasPrefix(b.Name, g.Prefix) {
				rep.Ground.count(b)
			}
		}
	}
	for _, c := range rep.Closed {
		t.Logf("the server closed a connection: %s", c)
	}
	return rep
}

// restartHooks are what the restart shares with Run.
type restartHooks struct {
	audit             func() ([]AuditZone, error)
	quiet, restarting *atomic.Bool
	gate              *gate
	serving           *sync.RWMutex
	value             Totals
	bots              int
}

// restart stops the server and starts it again while the bots move:
//  1. The ground groups stop and pick up every ground item, which the
//     server does not save.
//  2. The server stops, as Ctrl+C does, while the other groups move.
//     What it saved must have the totals of the start.
//  3. The server starts again. The bots log in again and wait. The
//     first audit with every bot must equal what the database saved.
//  4. The bots go on.
func (x ExitCheck) restart(ctx context.Context, t *testing.T, rep *ExitReport, h restartHooks) error {
	srv := x.Server
	h.quiet.Store(true)
	defer h.quiet.Store(false)
	// A drop sent just before the ground groups stopped lands within a
	// few ticks.
	if !sleep(ctx, time.Second) {
		return nil
	}
	deadline := time.Now().Add(2 * time.Minute)
	for empty := 0; empty < 2; {
		zones, err := h.audit()
		if err != nil {
			return err
		}
		now, _ := TotalsOf(zones)
		switch {
		case now.Ground == 0:
			empty++
		case time.Now().After(deadline):
			return fmt.Errorf("%d items stay on the ground", now.Ground)
		default:
			empty = 0
		}
		if !sleep(ctx, 300*time.Millisecond) {
			return nil
		}
	}

	h.gate.hold()
	h.restarting.Store(true)
	defer func() {
		h.restarting.Store(false)
		h.gate.release()
	}()
	h.serving.Lock()
	began := time.Now()
	srv.Stop()
	saveCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	saved, twice, err := SavedTotals(saveCtx, srv.DB, x.Prefix+"%")
	cancel()
	if err != nil {
		h.serving.Unlock()
		return err
	}
	rep.Saved = saved
	for _, p := range twice {
		t.Errorf("at the restart: database: %s", p)
	}
	for _, p := range compare(h.value, foldTotals(saved)) {
		t.Errorf("at the restart: database: %s", p)
	}
	srv.Restart()
	h.serving.Unlock()

	// Every bot logs in again and waits at the gate.
	var zones []AuditZone
	for {
		if h.gate.waiting.Load() >= int64(h.bots) {
			if zones, err = h.audit(); err != nil {
				return err
			}
			if playersIn(zones) >= h.bots {
				break
			}
		}
		if !sleep(ctx, 100*time.Millisecond) {
			return nil
		}
	}
	restored, twice := TotalsOf(zones)
	rep.Restored = restored
	for _, p := range twice {
		t.Errorf("after the restart: %s", p)
	}
	for _, p := range compare(saved, restored) {
		t.Errorf("after the restart, against the database: %s", p)
	}
	rep.RestartTook = time.Since(began)
	return nil
}

// CheckTokens checks the tokens in the logs of a stopped server: the
// tokens that scripts gave, less the tokens the shop took, are the
// tokens the characters hold at the end, and the shop took
// TokensPerPotion tokens for each Red Potion it sold, which the item log
// shows. Only the setup gives tokens and only TokenMove spends them.
func (x ExitCheck) CheckTokens(t *testing.T, rep ExitReport) (given, spent int64) {
	t.Helper()
	db := x.Server.DB
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var other, bought int64
	err := db.QueryRow(ctx, `SELECT
		coalesce(sum(l.amount) FILTER (WHERE l.action = 'give' AND l.to_kind = 'inventory'), 0),
		coalesce(sum(l.amount) FILTER (WHERE l.action = 'buy' AND l.from_kind = 'inventory' AND l.to_kind = 'shop'), 0),
		coalesce(sum(l.amount) FILTER (WHERE NOT (l.action = 'give' AND l.to_kind = 'inventory') AND NOT (l.action = 'buy' AND l.from_kind = 'inventory' AND l.to_kind = 'shop')), 0)
		FROM zeny_log l JOIN mod_ids m ON m.kind = 'currency' AND m.id = l.currency WHERE m.name = $1`, Tokens).Scan(&given, &spent, &other)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT coalesce(sum(amount), 0) FROM item_log WHERE action = 'buy' AND item_id = $1`, RedPotion).Scan(&bought); err != nil {
		t.Fatal(err)
	}
	held := rep.End.Currencies[Tokens]
	if given-spent != held {
		t.Errorf("tokens: %d given and %d spent in the zeny log, the characters hold %d", given, spent, held)
	}
	if given != rep.Start.Currencies[Tokens] {
		t.Errorf("tokens: %d given in the zeny log, the characters held %d at the start", given, rep.Start.Currencies[Tokens])
	}
	if other != 0 {
		t.Errorf("tokens: %d moved in the zeny log other than a give or a purchase", other)
	}
	potions := int64(rep.End.Items[RedPotion] - rep.Start.Items[RedPotion])
	if spent != potions*TokensPerPotion || bought != potions {
		t.Errorf("tokens: %d spent and %d Red Potions bought in the logs, the characters hold %d more Red Potions", spent, bought, potions)
	}
	return given, spent
}

// place takes a bot to a map with the fixture's Guide, when it is not
// there. The Guide stands on the start map and warps to the middle of
// FieldMap.
func place(ctx context.Context, b *botclient.Bot, to string) error {
	if to == "" || b.View().Map == to {
		return nil
	}
	if to != FieldMap || b.View().Map != StartMap {
		return fmt.Errorf("%s is on %s and cannot go to %s", b.Name, b.View().Map, to)
	}
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var guide uint32
	if err := b.WaitState(wait, func(v botclient.View, _ botclient.Game) bool {
		for _, e := range v.Entities {
			if e.Kind == wathenav1.EntityKind_ENTITY_KIND_NPC && e.Name == "Guide" {
				guide = e.ID
				return true
			}
		}
		return false
	}); err != nil {
		return fmt.Errorf("%s: no Guide in view: %w", b.Name, err)
	}
	mark := b.Mark()
	if err := b.Click(wait, guide); err != nil {
		return err
	}
	e, _, err := b.Wait(wait, mark, func(e *wathenav1.Event) bool { return e.GetNpcDialog().HasMenu() })
	if err != nil {
		return fmt.Errorf("%s: the Guide did not ask: %w", b.Name, err)
	}
	if err := b.Choose(wait, e.GetNpcDialog().GetDialog(), 1); err != nil {
		return err
	}
	if err := b.WaitState(wait, func(v botclient.View, _ botclient.Game) bool { return v.Map == to }); err != nil {
		return fmt.Errorf("%s: did not arrive on %s: %w", b.Name, to, err)
	}
	return nil
}

// TokenSetup gives every bot tokens with the fixture's "!currency".
func TokenSetup(tokens int64) func(ctx context.Context, bots []*botclient.Bot) error {
	return func(ctx context.Context, bots []*botclient.Bot) error {
		for _, b := range bots {
			if err := b.Chat(ctx, fmt.Sprintf("!currency tokens %d", tokens)); err != nil {
				return err
			}
			if err := b.WaitState(ctx, func(_ botclient.View, g botclient.Game) bool { return g.Currencies[Tokens] >= tokens }); err != nil {
				return fmt.Errorf("%s: no tokens: %w", b.Name, err)
			}
		}
		return nil
	}
}

// Setups runs setups one after another.
func Setups(setups ...func(ctx context.Context, bots []*botclient.Bot) error) func(ctx context.Context, bots []*botclient.Bot) error {
	return func(ctx context.Context, bots []*botclient.Bot) error {
		for _, s := range setups {
			if err := s(ctx, bots); err != nil {
				return err
			}
		}
		return nil
	}
}

// sleep waits for d. It returns false when ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
