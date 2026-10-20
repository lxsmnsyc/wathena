package store_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

func timeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func newCharacter(name string) *character.Character {
	return &character.Character{
		Info: character.Info{
			Name: name, Job: 0, Male: true, Hair: 3, HairColor: 1,
			BaseLevel: 1, JobLevel: 1, StatusPoints: 48,
			Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1},
			HP:    40, SP: 11, Map: "map/prontera", X: 150, Y: 180, InventorySize: 100,
		},
		Items: []character.Item{
			{Slot: 0, ID: 501, Amount: 5, Serial: item.NewSerial()},
			{Slot: 1, ID: 512, Amount: 10, Serial: item.NewSerial()},
			// An equipped, refined sword with two cards.
			{Slot: 2, ID: 1101, Amount: 1, Serial: item.NewSerial(), Equip: item.EqpHandR, Refine: 7, Cards: []uint32{4001, 4002}},
		},
	}
}

func TestMigrationsFromEmpty(t *testing.T) {
	admin := emptyDatabase(t)
	ctx := timeout(t)
	if err := store.CheckMigrations(ctx, admin); !errors.Is(err, store.ErrPending) {
		t.Fatalf("empty database: %v, want ErrPending", err)
	}
	m, err := store.NewMigrator(admin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	st, err := m.Status(ctx)
	if err != nil || len(st) == 0 || st[0].Applied {
		t.Fatalf("status before: %+v, %v", st, err)
	}
	applied, err := m.Up(ctx)
	if err != nil || len(applied) != len(st) {
		t.Fatalf("up applied %v, %v", applied, err)
	}
	if err := store.CheckMigrations(ctx, admin); err != nil {
		t.Fatalf("after up: %v", err)
	}
	if again, err := m.Up(ctx); err != nil || len(again) != 0 {
		t.Errorf("second up applied %v, %v", again, err)
	}
	st, _ = m.Status(ctx)
	for _, s := range st {
		if !s.Applied {
			t.Errorf("%d is not applied", s.Version)
		}
	}
}

func TestGameRoleCannotChangeSchema(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	if err := store.CheckMigrations(ctx, game); err != nil {
		t.Fatalf("the game role cannot check migrations: %v", err)
	}
	for _, ddl := range []string{
		"DROP TABLE inventory",
		"ALTER TABLE characters ADD COLUMN x2 integer",
		"CREATE TABLE extra (id bigint PRIMARY KEY)",
		"DELETE FROM characters",
		"TRUNCATE inventory",
	} {
		if _, err := game.Exec(ctx, ddl); !isCode(err, "42501") {
			t.Errorf("%s: %v, want permission denied", ddl, err)
		}
	}
}

func isCode(err error, code string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == code
}

// check_violation and unique_violation.
const (
	checkViolation  = "23514"
	uniqueViolation = "23505"
)

func TestConstraints(t *testing.T) {
	admin, _ := migrated(t)
	ctx := timeout(t)
	q := db.New(admin)
	account, err := q.CreateAccount(ctx, db.CreateAccountParams{Name: "Owner"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateAccount(ctx, db.CreateAccountParams{Name: "OWNER"}); err == nil {
		t.Error("an account name that differs only in case was accepted")
	}
	good := db.CreateCharacterParams{
		AccountID: account, Name: "Good", BaseLevel: 1, JobLevel: 1,
		Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1, Map: "map/x", InventorySize: 100,
	}
	id, err := q.CreateCharacter(ctx, good)
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(p *db.CreateCharacterParams){
		"negative zeny":       func(p *db.CreateCharacterParams) { p.Zeny = -1 },
		"zeny over MAX_ZENY":  func(p *db.CreateCharacterParams) { p.Zeny = 2147483648 },
		"negative base exp":   func(p *db.CreateCharacterParams) { p.BaseExp = -1 },
		"negative job exp":    func(p *db.CreateCharacterParams) { p.JobExp = -5 },
		"level 0":             func(p *db.CreateCharacterParams) { p.BaseLevel = 0 },
		"level 276":           func(p *db.CreateCharacterParams) { p.JobLevel = 276 },
		"negative points":     func(p *db.CreateCharacterParams) { p.StatusPoints = -1 },
		"stat 0":              func(p *db.CreateCharacterParams) { p.Str = 0 },
		"negative HP":         func(p *db.CreateCharacterParams) { p.Hp = -1 },
		"cell off the map":    func(p *db.CreateCharacterParams) { p.X = 1024 },
		"slot 15":             func(p *db.CreateCharacterParams) { p.Slot = 15 },
		"sex 2":               func(p *db.CreateCharacterParams) { p.Sex = 2 },
		"inventory over 1000": func(p *db.CreateCharacterParams) { p.InventorySize = 1001 },
		"empty name":          func(p *db.CreateCharacterParams) { p.Name = "" },
	}
	for name, change := range bad {
		p := good
		p.Name, p.Slot = "Other", 1
		change(&p)
		if _, err := q.CreateCharacter(ctx, p); !isCode(err, checkViolation) {
			t.Errorf("%s: %v, want a check violation", name, err)
		}
	}
	// A name taken without regard to case, and a taken slot, make no
	// character.
	for _, p := range []db.CreateCharacterParams{
		func() db.CreateCharacterParams { p := good; p.Name, p.Slot = "GOOD", 2; return p }(),
		func() db.CreateCharacterParams { p := good; p.Name = "Another"; return p }(),
	} {
		if _, err := q.CreateCharacter(ctx, p); err == nil {
			t.Errorf("%q in slot %d was made", p.Name, p.Slot)
		}
	}

	serial := pgtype.UUID{Bytes: item.NewSerial(), Valid: true}
	insertRolled := func(slot int, s pgtype.UUID, cards, rolls, cardRolls, options string) error {
		return q.InsertInventory(ctx, db.InsertInventoryParams{
			CharacterID: id, Slots: []int{slot}, ItemIds: []int{1101}, Amounts: []int{1}, Serials: []pgtype.UUID{s},
			Equips: []int{0}, Refines: []int{0}, Cards: []string{cards},
			Rolls: []string{rolls}, CardRolls: []string{cardRolls}, RandomOptions: []string{options}, Identified: []bool{true}, Bounds: []int{0},
			Brokens: []bool{false}, Expires: []int64{0}, SignedBys: []int{0}, SignedNames: []string{""},
		})
	}
	bound, expires := 0, int64(0)
	insertItem := func(slot, itemID, amount int, s pgtype.UUID, equip, refine int, cards string) error {
		return q.InsertInventory(ctx, db.InsertInventoryParams{
			CharacterID: id, Slots: []int{slot}, ItemIds: []int{itemID}, Amounts: []int{amount}, Serials: []pgtype.UUID{s},
			Equips: []int{equip}, Refines: []int{refine}, Cards: []string{cards},
			Rolls: []string{"{}"}, CardRolls: []string{"[]"}, RandomOptions: []string{"[]"}, Identified: []bool{true}, Bounds: []int{bound},
			Brokens: []bool{false}, Expires: []int64{expires}, SignedBys: []int{0}, SignedNames: []string{""},
		})
	}
	insert := func(slot, itemID, amount int, s pgtype.UUID) error {
		return insertItem(slot, itemID, amount, s, 0, 0, "{}")
	}
	fresh := func() pgtype.UUID { return pgtype.UUID{Bytes: item.NewSerial(), Valid: true} }
	if err := insert(0, 501, 30000, serial); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"amount 0":     insert(1, 501, 0, fresh()),
		"amount 30001": insert(1, 501, 30001, fresh()),
		"slot 1000":    insert(1000, 501, 1, fresh()),
		"bound 5":      func() error { bound = 5; defer func() { bound = 0 }(); return insert(1, 501, 1, fresh()) }(),
		"expires -1":   func() error { expires = -1; defer func() { expires = 0 }(); return insert(1, 501, 1, fresh()) }(),
		"item 0":       insert(1, 0, 1, fresh()),
		"refine -1":    insertItem(1, 1101, 1, fresh(), 0, -1, "{}"),
		"refine 256":   insertItem(1, 1101, 1, fresh(), 0, 256, "{}"),
		"equip -1":     insertItem(1, 1101, 1, fresh(), -1, 0, "{}"),
		"card 0":       insertItem(1, 1101, 1, fresh(), 0, 0, "{4001,0}"),
		"17 cards":     insertItem(1, 1101, 1, fresh(), 0, 0, "{1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17}"),
		"null card":    insertItem(1, 1101, 1, fresh(), 0, 0, "{4001,NULL}"),
		// Rolls are an object, and card rolls and options lists.
		"rolls a list":       insertRolled(1, fresh(), "{}", "[1]", "[]", "[]"),
		"card rolls object":  insertRolled(1, fresh(), "{}", "{}", "{}", "[]"),
		"options a number":   insertRolled(1, fresh(), "{}", "{}", "[]", "5"),
		"17 card roll lists": insertRolled(1, fresh(), "{}", "{}", "["+strings.Repeat("{},", 16)+"{}]", "[]"),
	} {
		if !isCode(err, checkViolation) {
			t.Errorf("%s: %v, want a check violation", name, err)
		}
	}
	// The same serial twice is a duplicated item, and two stacks cannot
	// share a slot.
	if err := insert(1, 501, 1, serial); !isCode(err, uniqueViolation) {
		t.Errorf("repeated serial: %v, want a unique violation", err)
	}
	if err := insert(0, 502, 1, fresh()); !isCode(err, uniqueViolation) {
		t.Errorf("taken slot: %v, want a unique violation", err)
	}
	// Statuses and timed bonuses.
	status := func(name string, tick, end int64) error {
		return q.InsertStatuses(ctx, db.InsertStatusesParams{CharacterID: id, Names: []string{name}, Val1s: []int{1}, Val2s: []int{0},
			Val3s: []int{0}, Val4s: []int{0}, Ticks: []int64{tick}, Ends: []int64{end}})
	}
	timed := func(pos int, key string, ms int64, ends []string, icon int, kind string) error {
		return q.InsertTimedBonus(ctx, db.InsertTimedBonusParams{CharacterID: id, Position: pos, Key: key, RemainingMs: ms,
			Bonuses: []byte("[]"), EndsOn: ends, Icon: icon, Kind: kind})
	}
	for name, err := range map[string]error{
		"empty status name":   status("", 1, 0),
		"status tick -2":      status("blessing", -2, 0),
		"negative status end": status("blessing", 1, -1),
		"empty timed key":     timed(0, "", 1, []string{}, -1, ""),
		"timed key over 64":   timed(0, strings.Repeat("k", 65), 1, []string{}, -1, ""),
		"negative remaining":  timed(0, "k", -1, []string{}, -1, ""),
		"icon -2":             timed(0, "k", 1, []string{}, -2, ""),
		"unknown kind":        timed(0, "k", 1, []string{}, -1, "curse"),
		"timed position 1024": timed(1024, "k", 1, []string{}, -1, ""),
	} {
		if !isCode(err, checkViolation) {
			t.Errorf("%s: %v, want a check violation", name, err)
		}
	}
	if err := status("blessing", 1, 0); err != nil {
		t.Fatal(err)
	}
	if err := status("blessing", 1, 0); !isCode(err, uniqueViolation) {
		t.Errorf("repeated status: %v, want a unique violation", err)
	}

	// pgx refuses a Go int that does not fit the column before it reaches
	// the database.
	if err := insert(1, 501, 1<<40, fresh()); err == nil {
		t.Error("an amount of 2^40 was accepted")
	}
}

func TestSaveAndLoadEveryField(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	c := newCharacter("Alice")
	id, err := s.DevCharacter(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	// The same name in another case is the same character.
	if again, err := s.DevCharacter(ctx, newCharacter("ALICE")); err != nil || again != id {
		t.Fatalf("second DevCharacter: %d, %v, want %d", again, err, id)
	}
	got, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	want := *c
	want.ID, want.AccountID = id, got.AccountID
	// The database sets the creation time.
	if time.Since(got.CreatedAt) > time.Minute {
		t.Errorf("created at %v", got.CreatedAt)
	}
	want.CreatedAt = got.CreatedAt
	if got.AccountID == 0 || !got.Equal(&want) {
		t.Fatalf("loaded %+v\nwant %+v", got, &want)
	}

	// Every field changes, and the inventory gains, changes and loses
	// slots.
	start := time.Now().Truncate(time.Second)
	next := &character.Character{
		Info: character.Info{
			ID: id, AccountID: got.AccountID, Slot: got.Slot, Name: "Alice",
			Job: 4001, Male: false, Hair: 9, HairColor: 5, ClothColor: 2,
			BaseLevel: 99, JobLevel: 70, BaseExp: 123456789012, JobExp: 98765,
			StatusPoints: 17, SkillPoints: 3,
			Stats: status.Stats{Str: 99, Agi: 80, Vit: 70, Int: 60, Dex: 50, Luk: 40},
			HP:    9000, SP: 800, Map: "map/prt_fild08", X: 1023, Y: 0, Zeny: 2147483647,
			InventorySize: 200, CreatedAt: got.CreatedAt,
		},
		Items: []character.Item{
			{Slot: 0, ID: 501, Amount: 30000, Serial: c.Items[0].Serial},
			// Rolls, a card's rolls, random options and an unidentified
			// copy (strategy section 5.4).
			{Slot: 7, ID: 1201, Amount: 1, Serial: item.NewSerial(), Cards: []uint32{4001, 4140}, Extra: item.Extra{
				Rolls:     item.RollsOf(map[string]int{"boss_damage": 31, "luck": -2}),
				CardRolls: []item.Rolls{nil, item.RollsOf(map[string]int{"boss_damage": 29})},
				Options:   []item.Option{{Name: "max_hp", Value: 100}, {Name: "example.a:glow", Value: -5, Param: 3}},
			}},
			{Slot: 8, ID: 4140, Amount: 2, Serial: item.NewSerial(), Extra: item.Extra{
				Rolls: item.RollsOf(map[string]int{"boss_damage": 15}), Unidentified: true,
			}},
			// A broken copy named for a character, and a rental (review
			// decision 58, migration 00042).
			{Slot: 9, ID: 1202, Amount: 1, Serial: item.NewSerial(), Refine: 4, Extra: item.Extra{
				Broken: true, SignedBy: 150000, SignedName: "Alice",
			}},
			{Slot: 10, ID: 2301, Amount: 1, Serial: item.NewSerial(), Extra: item.Extra{Expires: 1893456000}},
			{Slot: 199, ID: 909, Amount: 3, Serial: item.NewSerial()},
		},
		// Statuses come back by name, and timed bonuses in their order,
		// with two copies of one key, as the on_repeat mode "add" gives.
		Statuses: []character.Status{
			{Name: "blessing", Values: [4]int{10, 0, 0, 0}, Tick: 59000},
			{Name: "example.glow:glow", Values: [4]int{1, -2, 2147483647, -2147483648}, Tick: 1000, End: 30000},
			{Name: "strfood", Values: [4]int{10}, Tick: -1},
		},
		TimedBonuses: []character.TimedBonus{
			{Key: "b", Remaining: 5000, Bonuses: `[["str", 5]]`, EndsOn: []string{"death", "dispell"}, Icon: 925, Kind: "buff"},
			{Key: "a", Remaining: 1, Bonuses: `[]`, EndsOn: []string{}, Icon: -1},
			{Key: "b", Remaining: 3000, Bonuses: `[["str", 7]]`, EndsOn: []string{"death", "dispell"}, Icon: 925, Kind: "buff"},
		},
		// Learned skills, with a mod skill's ID.
		Skills: map[int]int{1: 9, 28: 10, 20000: 5},
		// A skill of SKILL_PERM_GRANT, which a skill reset keeps.
		Granted: []int{28},
		// A block list in its order, and a mute and a jail (decision 0018).
		Blocks: []string{"Zed", "bob", "Carol Ann"},
		Sanctions: []character.Sanction{
			{Kind: character.SanctionMute, Reason: "spam", Start: start, End: start.Add(time.Hour), By: "example.a"},
			{Kind: character.SanctionJail, Reason: "botting", Start: start, End: start.Add(48 * time.Hour), By: "staff",
				ReturnMap: "map/prontera", ReturnX: 150, ReturnY: 180},
		},
	}
	sess.Save(next, true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, sess, err = s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(next) {
		t.Errorf("after save %+v\nwant %+v", got, next)
	}

	// Statuses that end and timed bonuses that run out leave no rows. A
	// mute lifted early is kept with its lift time.
	next.Statuses, next.TimedBonuses = next.Statuses[2:], nil
	next.Statuses[0].Tick = -1
	next.Blocks = []string{"bob"}
	next.Sanctions = next.Sanctions[1:]
	sess.Save(next, true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err = s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(next) || len(got.TimedBonuses) != 0 {
		t.Errorf("after second save %+v\nwant %+v", got, next)
	}
	var lifted, total int
	if err := game.QueryRow(ctx, `SELECT count(*) FILTER (WHERE lifted_at IS NOT NULL), count(*) FROM sanctions WHERE character_id = $1`, id).Scan(&lifted, &total); err != nil {
		t.Fatal(err)
	}
	if lifted != 1 || total != 2 {
		t.Errorf("sanctions: %d lifted of %d", lifted, total)
	}
}

func TestStaleSaveIsRejected(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	id, err := s.DevCharacter(ctx, newCharacter("Bob"))
	if err != nil {
		t.Fatal(err)
	}
	c, old, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	// A newer session loads the character, as a login elsewhere would.
	_, cur, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	stale := *c
	stale.Zeny = 1000000
	old.Save(&stale, false)
	if err := old.Flush(ctx); !errors.Is(err, store.ErrStale) {
		t.Fatalf("stale save: %v, want ErrStale", err)
	}
	// The old session reports it, once, even to a function set late.
	called := make(chan struct{}, 2)
	old.OnStale(func() { called <- struct{}{} })
	select {
	case <-called:
	case <-ctx.Done():
		t.Fatal("OnStale was not called")
	}
	// Later saves of the old session are ignored.
	old.Save(&stale, true)
	if err := old.Flush(ctx); !errors.Is(err, store.ErrStale) {
		t.Fatalf("after stale: %v", err)
	}
	newer := *c
	newer.BaseExp = 50
	cur.Save(&newer, true)
	if err := cur.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Zeny != 0 || got.BaseExp != 50 {
		t.Errorf("zeny %d and base exp %d, want the newer session's 0 and 50", got.Zeny, got.BaseExp)
	}
}

func TestConcurrentSaves(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{Workers: 4})
	const characters, saves = 8, 200
	var wg sync.WaitGroup
	for i := range characters {
		c := newCharacter("Racer" + string(rune('A'+i)))
		id, err := s.DevCharacter(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		loaded, sess, err := s.Load(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		wg.Go(func() {
			// Each save is a complete copy with more exp and one more
			// potion, as the zone would send.
			for n := 1; n <= saves; n++ {
				next := *loaded
				next.BaseExp = int64(n)
				next.Items = append([]character.Item(nil), loaded.Items...)
				next.Items[0].Amount = 5 + n
				sess.Save(&next, n == saves)
			}
		})
	}
	wg.Wait()
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	for i := range characters {
		name := "Racer" + string(rune('A'+i))
		id, err := s.DevCharacter(ctx, newCharacter(name))
		if err != nil {
			t.Fatal(err)
		}
		got, _, err := s.Load(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.BaseExp != saves || got.Items[0].Amount != 5+saves {
			t.Errorf("%s: exp %d and %d potions, want the last save", name, got.BaseExp, got.Items[0].Amount)
		}
	}
}

func TestLoadWaitsForQueuedSaves(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{Workers: 1})
	id, err := s.DevCharacter(ctx, newCharacter("Carol"))
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	last := *c
	last.JobExp = 77
	sess.Save(&last, true)
	// No Flush: Load itself waits for the queued save.
	got, _, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.JobExp != 77 {
		t.Errorf("job exp %d, want the logout save's 77", got.JobExp)
	}
}

func TestFailedSaveIsTriedAgain(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{RetryDelay: 50 * time.Millisecond})
	id, err := s.DevCharacter(ctx, newCharacter("Dave"))
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	// Saves fail while the game role may not update characters.
	if _, err := admin.Exec(ctx, "REVOKE UPDATE ON characters FROM wathena_game"); err != nil {
		t.Fatal(err)
	}
	next := *c
	next.Zeny = 42
	sess.Save(&next, true)
	time.Sleep(200 * time.Millisecond)
	if _, err := admin.Exec(ctx, "GRANT UPDATE ON characters TO wathena_game"); err != nil {
		t.Fatal(err)
	}
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Load(ctx, id)
	if err != nil || got.Zeny != 42 {
		t.Fatalf("zeny %d, %v, want 42", got.Zeny, err)
	}
}
