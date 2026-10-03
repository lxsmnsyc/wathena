package e2e

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// Duplication tests (strategy section 12.1, decision 0017): many bots
// move items at the same time through the server's transfers, while the
// test asks the server for every item it holds. After every audit, and
// at the end:
//   - the amount of each item, players' and ground items together, is
//     what it was at the start,
//   - all zeny together is what it was at the start,
//   - no serial is held twice,
//   - each bot's inventory, as the server told it, is what the server
//     holds for it.
//
// With a database, the test also stops the server and checks the saved
// inventories and the item log: the same totals, one log line for each
// move the bots saw, and a known history for every serial.

// Move is one thing a bot may do in a duplication test. Do returns
// whether the move happened. A move another bot won, such as a ground
// item it picked up first, is not an error.
type Move struct {
	Name string
	Do   func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error)
}

// DupTest is a duplication test.
type DupTest struct {
	Server *Server
	// Bots is how many bots play, named Prefix plus a number. Each joins
	// with the server's start items.
	Bots   int
	Prefix string
	// Duration is how long the bots make moves.
	Duration time.Duration
	// Seed fixes each bot's choices.
	Seed uint64
	// Moves are what the bots do: each step, a bot picks one at random.
	Moves []Move
	// Settle runs on every bot after the moves, until it returns false,
	// so the world ends quiet, such as with every ground item picked up.
	Settle func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error)
	// AuditEvery is the time between audits while the bots move. It is
	// 200 ms when 0.
	AuditEvery time.Duration
}

// DupReport is what a duplication test did.
type DupReport struct {
	// Moves counts the moves that happened, by name.
	Moves map[string]int
	// Audits is how many audits passed.
	Audits int
	// Dropped and Added count the drops and the items entering an
	// inventory that the bots' messages showed, over the whole test.
	Dropped, Added int
	// Start and End are the totals at the start and at the end.
	Start, End Totals
}

// Totals are what a world holds.
type Totals struct {
	// Items are the amounts of each item, by item ID.
	Items map[uint32]int
	Zeny  int64
	// Serials are the serials players hold, with who holds them.
	Serials map[string]string
	// Ground counts the ground items.
	Ground int
}

// TotalsOf adds up an audit. It returns the serials held more than
// once.
func TotalsOf(zones []AuditZone) (Totals, []string) {
	t := Totals{Items: map[uint32]int{}, Serials: map[string]string{}}
	var twice []string
	for _, z := range zones {
		for _, p := range z.Players {
			t.Zeny += p.Zeny
			for _, it := range p.Items {
				t.Items[it.Item] += it.Amount
				if who, ok := t.Serials[it.Serial]; ok {
					twice = append(twice, fmt.Sprintf("serial %s is held by %s and %s", it.Serial, who, p.Name))
				}
				t.Serials[it.Serial] = p.Name
			}
		}
		for _, g := range z.Ground {
			t.Items[g.Item] += g.Amount
			t.Ground++
		}
	}
	return t, twice
}

// compare lists how b differs from a in items and zeny.
func compare(a, b Totals) []string {
	var out []string
	for _, id := range slices.Sorted(maps.Keys(mergeKeys(a.Items, b.Items))) {
		if a.Items[id] != b.Items[id] {
			out = append(out, fmt.Sprintf("item %d: %d, was %d", id, b.Items[id], a.Items[id]))
		}
	}
	if a.Zeny != b.Zeny {
		out = append(out, fmt.Sprintf("zeny: %d, was %d", b.Zeny, a.Zeny))
	}
	return out
}

func mergeKeys(a, b map[uint32]int) map[uint32]int {
	m := maps.Clone(a)
	for k := range b {
		m[k] = 0
	}
	return m
}

// Run runs the test. It fails t for every check that does not hold.
func (d DupTest) Run(t *testing.T) DupReport {
	t.Helper()
	if d.Prefix == "" {
		d.Prefix = "Dup"
	}
	if d.AuditEvery <= 0 {
		d.AuditEvery = 200 * time.Millisecond
	}
	srv := d.Server
	ctx, cancel := context.WithTimeout(context.Background(), d.Duration+2*time.Minute)
	defer cancel()
	bots := make([]*botclient.Bot, d.Bots)
	for i := range bots {
		b, err := botclient.Dial(ctx, srv.WS, fmt.Appendf(nil, "%s%d", d.Prefix, i+1), botclient.Options{Record: true})
		if err != nil {
			t.Fatalf("bot %d: %v", i+1, err)
		}
		bots[i] = b
		t.Cleanup(func() { _ = b.Close() })
	}
	start, err := d.audit(ctx, d.Bots)
	if err != nil {
		t.Fatal(err)
	}
	rep := DupReport{Moves: map[string]int{}, Start: start}
	var mu sync.Mutex
	var failed atomic.Bool
	fail := func(format string, args ...any) {
		failed.Store(true)
		t.Errorf(format, args...)
	}

	// The bots move until the time is up, while the test audits.
	moving, stop := context.WithTimeout(ctx, d.Duration)
	defer stop()
	var wg sync.WaitGroup
	for i, b := range bots {
		rnd := rand.New(rand.NewPCG(d.Seed, uint64(i)+1)) // #nosec G404 -- test choices, not secrets.
		wg.Go(func() {
			for moving.Err() == nil {
				m := d.Moves[rnd.IntN(len(d.Moves))]
				done, err := m.Do(moving, b, rnd)
				if err != nil && moving.Err() == nil {
					fail("%s: %s: %v", b.Name, m.Name, err)
					return
				}
				if done {
					mu.Lock()
					rep.Moves[m.Name]++
					mu.Unlock()
				}
			}
		})
	}
	audits := make(chan struct{})
	go func() {
		defer close(audits)
		tick := time.NewTicker(d.AuditEvery)
		defer tick.Stop()
		for {
			select {
			case <-moving.Done():
				return
			case <-tick.C:
			}
			zones, err := srv.Audit(ctx)
			if err != nil {
				if moving.Err() == nil {
					fail("audit: %v", err)
				}
				return
			}
			if !d.check(t, start, zones, "during the moves") {
				failed.Store(true)
				return
			}
			rep.Audits++
		}
	}()
	wg.Wait()
	<-audits
	if failed.Load() {
		t.FailNow()
	}

	// Every bot settles, until the world is quiet. A move sent just
	// before the time was up can land after a bot settled, so settling
	// runs again while the audit shows ground items.
	for round := 0; d.Settle != nil && round < 5; round++ {
		for i, b := range bots {
			rnd := rand.New(rand.NewPCG(d.Seed, uint64(d.Bots*(round+1)+i)+1)) // #nosec G404 G115 -- test choices, not secrets, from small counts.
			wg.Go(func() {
				for ctx.Err() == nil {
					again, err := d.Settle(ctx, b, rnd)
					if err != nil {
						fail("%s: settling: %v", b.Name, err)
						return
					}
					if !again {
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
	zones, err := srv.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !d.check(t, start, zones, "at the end") {
		t.FailNow()
	}
	rep.Audits++
	rep.End, _ = TotalsOf(zones)
	d.checkBots(ctx, t, bots)
	for _, b := range bots {
		events, _ := b.Events(0)
		for _, e := range events {
			if r := e.GetInventoryRemove(); r != nil && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_DROPPED {
				rep.Dropped++
			}
			if e.HasInventoryAdd() {
				rep.Added++
			}
		}
	}
	return rep
}

// audit waits until the server holds the given number of players, and
// returns its totals.
func (d DupTest) audit(ctx context.Context, players int) (Totals, error) {
	for {
		zones, err := d.Server.Audit(ctx)
		if err != nil {
			return Totals{}, err
		}
		n := 0
		for _, z := range zones {
			n += len(z.Players)
		}
		if n >= players {
			t, twice := TotalsOf(zones)
			if len(twice) > 0 {
				return t, errors.New(strings.Join(twice, "; "))
			}
			return t, nil
		}
		select {
		case <-ctx.Done():
			return Totals{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// check compares an audit with the start.
func (d DupTest) check(t *testing.T, start Totals, zones []AuditZone, when string) bool {
	t.Helper()
	now, twice := TotalsOf(zones)
	problems := append(twice, compare(start, now)...)
	for _, p := range problems {
		t.Errorf("%s: %s", when, p)
	}
	return len(problems) == 0
}

// checkBots checks that each bot's inventory, from the server's
// messages, is what the server holds for it.
func (d DupTest) checkBots(ctx context.Context, t *testing.T, bots []*botclient.Bot) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		zones, err := d.Server.Audit(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held := map[string]map[int]botclient.Item{}
		for _, z := range zones {
			for _, p := range z.Players {
				m := map[int]botclient.Item{}
				for _, it := range p.Items {
					m[it.Slot] = botclient.Item{Slot: it.Slot, ID: it.Item, Amount: it.Amount}
				}
				held[p.Name] = m
			}
		}
		var diffs []string
		for _, b := range bots {
			seen := map[int]botclient.Item{}
			for slot, it := range b.Game().Items {
				seen[slot] = botclient.Item{Slot: it.Slot, ID: it.ID, Amount: it.Amount}
			}
			if !maps.Equal(seen, held[b.Name]) {
				diffs = append(diffs, fmt.Sprintf("%s sees %v, the server holds %v", b.Name, seen, held[b.Name]))
			}
		}
		if len(diffs) == 0 {
			return
		}
		if time.Now().After(deadline) {
			sort.Strings(diffs)
			for _, s := range diffs {
				t.Error(s)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// DropMove drops part or all of a random stack of an item.
func DropMove(id uint32) Move {
	return Move{Name: "drop", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		var slots []botclient.Item
		for _, it := range b.Game().Items {
			if it.ID == id {
				slots = append(slots, it)
			}
		}
		if len(slots) == 0 {
			return false, pause(ctx)
		}
		slices.SortFunc(slots, func(a, b botclient.Item) int { return a.Slot - b.Slot })
		it := slots[rnd.IntN(len(slots))]
		amount := 1 + rnd.IntN(it.Amount)
		mark := b.Mark()
		if err := b.DropItem(ctx, it.Slot, amount); err != nil {
			return false, err
		}
		e, _, err := b.Wait(ctx, mark, func(e *wathenav1.Event) bool {
			r := e.GetInventoryRemove()
			return e.HasDropItemFailed() || r != nil && int(r.GetSlot()) == it.Slot && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_DROPPED
		})
		if err != nil {
			return false, ignoreDone(ctx, err)
		}
		if f := e.GetDropItemFailed(); f != nil {
			return false, fmt.Errorf("drop of %d from slot %d failed: %s", amount, it.Slot, f.GetReason())
		}
		return true, nil
	}}
}

// PickUpMove picks up a random ground item in view. Another bot may get
// it first.
func PickUpMove() Move {
	return Move{Name: "pick_up", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		return pickUp(ctx, b, rnd)
	}}
}

// SettlePickUps picks up ground items until none is in view from a
// cell that sees the whole map, such as its center.
func SettlePickUps(x, y int) func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
	return func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		if len(groundItems(b)) > 0 {
			_, err := pickUp(ctx, b, rnd)
			return true, err
		}
		v := b.View()
		if v.X == x && v.Y == y && !v.Walking {
			return false, nil
		}
		if !v.Walking {
			if err := b.WalkTo(ctx, x, y); err != nil {
				return false, err
			}
		}
		return true, pause(ctx)
	}
}

func groundItems(b *botclient.Bot) []botclient.Entity {
	var out []botclient.Entity
	for _, e := range b.View().Entities {
		if e.Kind == wathenav1.EntityKind_ENTITY_KIND_ITEM {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b botclient.Entity) int { return int(a.ID) - int(b.ID) })
	return out
}

// pickUp picks up a random ground item in view. It returns true when
// the bot got it.
func pickUp(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
	items := groundItems(b)
	if len(items) == 0 {
		return false, pause(ctx)
	}
	it := items[rnd.IntN(len(items))]
	mark := b.Mark()
	if err := b.PickUp(ctx, it.ID); err != nil {
		return false, err
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	e, _, err := b.Wait(wait, mark, func(e *wathenav1.Event) bool {
		if f := e.GetPickUpFailed(); f != nil && f.GetItem() == it.ID {
			return true
		}
		if a := e.GetInventoryAdd(); a != nil && a.GetItemId() == it.ItemID {
			return true
		}
		// Another bot took it.
		v := e.GetVanish()
		return v != nil && v.GetId() == it.ID
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			// The walk to the item did not finish, such as when the
			// item went first without the bot seeing it.
			return false, nil
		}
		return false, ignoreDone(ctx, err)
	}
	if f := e.GetPickUpFailed(); f != nil {
		if f.GetReason() == wathenav1.PickUpFailure_PICK_UP_FAILURE_GONE {
			return false, nil
		}
		return false, fmt.Errorf("picking up item %d failed: %s", it.ItemID, f.GetReason())
	}
	if e.HasInventoryAdd() {
		return true, nil
	}
	// The item left view. The bot got it when its InventoryAdd comes in
	// the same frame or soon after.
	soon, stop := context.WithTimeout(ctx, 300*time.Millisecond)
	defer stop()
	_, _, err = b.Wait(soon, mark, func(e *wathenav1.Event) bool {
		a := e.GetInventoryAdd()
		return a != nil && a.GetItemId() == it.ItemID
	})
	return err == nil, nil
}

// pause waits a little when a bot has nothing to do.
func pause(ctx context.Context) error {
	select {
	case <-ctx.Done():
	case <-time.After(20 * time.Millisecond):
	}
	return nil
}

func ignoreDone(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// CheckDatabase stops the server, then checks what it saved for a test
// of drops and pick-ups: the totals of the start in the bots'
// inventories, an item log line for each drop and each pick-up the bots
// saw, and a history for every serial: each serial in the inventories
// was there at the start or is in the item log, and each pick-up names
// a serial that was dropped.
func (d DupTest) CheckDatabase(t *testing.T, rep DupReport) {
	t.Helper()
	srv := d.Server
	if srv.DB == nil {
		return
	}
	srv.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db := srv.DB
	pattern := d.Prefix + "%"
	got := Totals{Items: map[uint32]int{}, Serials: map[string]string{}}
	rows, err := db.Query(ctx, `SELECT c.name, i.item_id, i.amount, i.serial::text FROM inventory i JOIN characters c ON c.id = i.character_id
		WHERE c.name LIKE $1`, pattern)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name, serial string
		var id uint32
		var amount int
		if err := rows.Scan(&name, &id, &amount, &serial); err != nil {
			t.Fatal(err)
		}
		got.Items[id] += amount
		got.Serials[serial] = name
	}
	rows.Close()
	if err := db.QueryRow(ctx, `SELECT coalesce(sum(zeny), 0) FROM characters WHERE name LIKE $1`, pattern).Scan(&got.Zeny); err != nil {
		t.Fatal(err)
	}
	for _, p := range compare(rep.Start, got) {
		t.Errorf("database: %s", p)
	}
	for action, want := range map[string]int{"drop": rep.Dropped, "pick_up": rep.Added} {
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM item_log WHERE action = $1`, action).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("database: %d item log lines for %s, the bots saw %d", n, action, want)
		}
	}
	logged := map[string]bool{}
	dropped := map[string]bool{}
	type pick struct{ serial, source string }
	var picks []pick
	rows, err = db.Query(ctx, `SELECT action, serial::text, source FROM item_log`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var action, serial string
		var source pgtype.UUID
		if err := rows.Scan(&action, &serial, &source); err != nil {
			t.Fatal(err)
		}
		logged[serial] = true
		switch action {
		case "drop":
			dropped[serial] = true
		case "pick_up":
			picks = append(picks, pick{serial, uuidText(source)})
		}
	}
	rows.Close()
	for serial, who := range got.Serials {
		if _, ok := rep.Start.Serials[serial]; !ok && !logged[serial] {
			t.Errorf("database: %s holds serial %s, which has no history", who, serial)
		}
	}
	for _, p := range picks {
		if !dropped[p.source] {
			t.Errorf("database: a pick-up into %s names %q, which no drop wrote", p.serial, p.source)
		}
	}
}

func uuidText(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
