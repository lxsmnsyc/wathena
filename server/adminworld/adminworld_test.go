package adminworld_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/adminworld"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/quest"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// committer hands the zones' commits to the store, as waserver's does.
type committer struct{ s *store.Store }

func (c committer) Commit(saves []zone.CommitSave, storages []zone.StorageCommit, logs transfer.Logs) {
	c.CommitWrites(saves, storages, zone.Writes{}, logs)
}

func (c committer) CommitWrites(saves []zone.CommitSave, storages []zone.StorageCommit, w zone.Writes, logs transfer.Logs) {
	var entries []store.Entry
	for _, s := range saves {
		if x, ok := s.Saver.(*store.Session); ok {
			entries = append(entries, store.Entry{Session: x, Char: s.Character})
		}
	}
	var writes []store.StorageWrite
	for _, s := range storages {
		writes = append(writes, store.StorageWrite{Guild: s.Guild, Key: s.Key, Base: s.Base, Cur: s.Cur, CurrencyBase: s.CurrencyBase,
			Currencies: s.Currencies, HasCurrencies: s.HasCurrencies, SizeBase: s.SizeBase, Size: s.Size})
	}
	c.s.CommitAll(entries, writes, store.Writes{Mails: w.Mails, Stores: w.Stores, Audit: w.Audit}, logs)
}

type env struct {
	t     *testing.T
	ctx   context.Context
	admin *pgxpool.Pool
	st    *store.Store
	w     *world.World
	aw    *adminworld.World
	// cfg is aw's config, for tests that need another clock.
	cfg   adminworld.Config
	items *item.Table
	// kicker records the kicks of the moderation tests.
	kicker *kicker
}

func grid() *path.Grid {
	g := &path.Grid{Width: 40, Height: 40, Cells: make([]uint8, 1600)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	return g
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	adminPool := storetest.Migrated(t)
	game := storetest.As(t, adminPool, "wathena_game")
	st := store.New(game, store.Options{Log: slog.New(slog.DiscardHandler)})
	items := item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: 501, AegisName: "Red_Potion", Name: "Red Potion", Type: "Healing", Weight: 70, Stackable: true},
		{ID: 909, AegisName: "Jellopy", Name: "Jellopy", Type: "Etc", Weight: 10, Stackable: true},
	}})
	hub := chat.NewHub(nil, nil)
	storages := storage.NewRegistry(st)
	mails := mail.NewRegistry(st, items)
	floor, err := st.MailIDFloor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mails.SetIDFloor(floor)
	var zones []*zone.Zone
	for i, name := range []string{"map/one", "map/two"} {
		z := zone.New(name, grid(), rng.FromKey([32]byte{byte(i + 1)}).Stream(), nil)
		z.Items, z.Chat, z.Storages, z.Mail = items, hub, storages, mails
		z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300,
			RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
		z.Committer = committer{st}
		zones = append(zones, z)
	}
	w, err := world.New("map/one", zones...)
	if err != nil {
		t.Fatal(err)
	}
	wctx, stop := context.WithCancel(context.Background())
	go w.Run(wctx)
	t.Cleanup(func() {
		stop()
		<-w.Stopped()
		dctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := st.Close(dctx); err != nil {
			t.Error(err)
		}
	})
	k := &kicker{st: st}
	cfg := adminworld.Config{World: w, Store: st, Chat: hub, Items: items, Storages: storages, Mail: mails,
		Social: zones[0].Social, Kicker: k, Log: slog.New(slog.DiscardHandler),
		Quests: quest.NewTable([]*quest.Record{{ID: 1100, Name: "1100", Title: "Porings", Targets: []quest.Target{{Monster: 1002, Count: 3}}}})}
	return &env{t: t, ctx: ctx, admin: adminPool, st: st, w: w, aw: adminworld.New(cfg), cfg: cfg, items: items, kicker: k}
}

// character makes a saved character with items.
func (e *env) character(name string, items ...character.Item) int64 {
	e.t.Helper()
	c := &character.Character{Info: character.Info{Name: name, Male: true, BaseLevel: 1, JobLevel: 1,
		Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}, HP: 40, SP: 11, Map: "map/one", X: 10, Y: 10, InventorySize: 100},
		Items: items}
	id, err := e.st.DevCharacter(e.ctx, c)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

// login loads a character and joins it, as the gateway does, and reads
// its messages until it leaves.
func (e *env) login(id int64) *zone.Session {
	e.t.Helper()
	c, x, err := e.st.Load(e.ctx, id)
	if err != nil {
		e.t.Fatal(err)
	}
	s, err := e.w.JoinCharacter(e.ctx, c, x)
	if err != nil {
		e.t.Fatal(err)
	}
	go func() {
		for range s.Out {
		}
	}()
	return s
}

func stack(slot int, id uint32, amount int) character.Item {
	return character.Item{Slot: slot, ID: id, Amount: amount, Serial: item.NewSerial()}
}

// holders counts the saved places of a serial.
func (e *env) holders(s item.Serial) []store.Holder {
	e.t.Helper()
	if err := e.st.Drain(e.ctx); err != nil {
		e.t.Fatal(err)
	}
	h, err := e.st.ItemHolders(e.ctx, s)
	if err != nil {
		e.t.Fatal(err)
	}
	return h
}

// total adds up an item over every saved place.
func (e *env) total(id uint32) int {
	e.t.Helper()
	if err := e.st.Drain(e.ctx); err != nil {
		e.t.Fatal(err)
	}
	var n int
	err := e.admin.QueryRow(e.ctx, `SELECT coalesce(sum(amount), 0)::int FROM (
		SELECT amount FROM inventory WHERE item_id = $1 UNION ALL SELECT amount FROM storage_items WHERE item_id = $1
		UNION ALL SELECT amount FROM mail_items WHERE item_id = $1 UNION ALL SELECT amount FROM guild_storage WHERE item_id = $1) x`, id).Scan(&n)
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

func restore(to int64, mayCreate bool, items ...admin.RestoreItem) admin.Restore {
	return admin.Restore{AccountID: 99, AccountName: "Admin1", Role: "admin", Character: to, Items: items, Reason: "theft",
		MayCreate: mayCreate}
}

// TestRestoreFromEveryPlace restores serials from an online inventory,
// an offline inventory, an account storage, a mail, and a gone serial,
// in one mail, and checks the rows, the logs, the audit row and the
// history.
func TestRestoreFromEveryPlace(t *testing.T) {
	e := setup(t)
	online := stack(0, 909, 10)
	away := stack(0, 501, 4)
	thief := e.character("Thief", online)
	fence := e.character("Fence", away)
	victim := e.character("Victim")
	s := e.login(thief)
	defer s.Leave()
	// A stack in the fence's account storage and one in a mail to it.
	stored, mailed := item.NewSerial(), item.NewSerial()
	var account int64
	_ = e.admin.QueryRow(e.ctx, "SELECT account_id FROM characters WHERE id = $1", fence).Scan(&account)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO storage_items (serial, holder, account_id, slot, item_id, amount) VALUES ($1, 'account', $2, 0, 909, 6)",
			[]any{pgtype.UUID{Bytes: stored, Valid: true}, account}},
		{"INSERT INTO mails (id, sender_name, recipient_id, recipient_name, title, sent_at) VALUES (500, 'Thief', $1, 'Fence', 'Loot', now())",
			[]any{fence}},
		{"INSERT INTO mail_items (serial, mail_id, slot, item_id, amount) VALUES ($1, 500, 0, 909, 3)",
			[]any{pgtype.UUID{Bytes: mailed, Valid: true}}},
	} {
		if _, err := e.admin.Exec(e.ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	// A serial that was used up: only its log lines are left.
	gone := item.NewSerial()
	e.st.Commit(nil, transfer.Logs{Items: []transfer.ItemLog{{At: time.Now().Add(-time.Minute), Action: transfer.ActionUse,
		From: transfer.Inventory(thief), To: transfer.End(transfer.KindUsed), Serial: gone, ItemID: 501, Amount: 2}}})
	before := e.total(909)
	all := []admin.RestoreItem{{Serial: online.Serial}, {Serial: away.Serial, Amount: 1}, {Serial: stored}, {Serial: mailed},
		{Serial: gone}}
	// Without items.create the gone serial is refused, and nothing moves.
	var we *admin.WorldError
	if _, err := e.aw.Restore(e.ctx, restore(victim, false, all...)); !errors.As(err, &we) || we.Code != "forbidden" {
		t.Fatalf("without items.create: %v", err)
	}
	res, err := e.aw.Restore(e.ctx, restore(victim, true, all...))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetItems()) != 5 || res.GetMailId() == 0 || !res.GetItems()[4].GetCopy() || res.GetItems()[0].GetFrom().GetName() != "Thief" ||
		res.GetItems()[3].GetFrom().GetKind() != "mail" {
		t.Errorf("result %v", res)
	}
	// Every serial that moved is in the victim's mail, once.
	for _, sr := range []item.Serial{online.Serial, stored, mailed} {
		h := e.holders(sr)
		if len(h) != 1 || h[0].Holder != "mail" || h[0].ID != res.GetMailId() {
			t.Errorf("holders of %s: %+v", sr, h)
		}
	}
	if h := e.holders(away.Serial); len(h) != 1 || h[0].Holder != "inventory" || h[0].Amount != 3 {
		t.Errorf("the fence keeps the rest: %+v", h)
	}
	if after := e.total(909); after != before {
		t.Errorf("jellopy %d, was %d", after, before)
	}
	// One audit row, marked for review, with the mail.
	var n int
	var args string
	if err := e.admin.QueryRow(e.ctx, "SELECT count(*), max(arguments::text) FROM staff_audit WHERE action = 'items.restore' AND needs_review").
		Scan(&n, &args); err != nil || n != 1 {
		t.Errorf("audit rows %d %v", n, err)
	}
	// The history of the moved stack shows the restore.
	hist, err := e.aw.ItemHistory(e.ctx, online.Serial)
	if err != nil {
		t.Fatal(err)
	}
	last := hist.GetMoves()[len(hist.GetMoves())-1]
	if last.GetAction() != "restore" || last.GetTo().GetName() == "" || hist.GetHolder().GetKind() != "mail" {
		t.Errorf("history %v", hist)
	}
	// The split of the fence's stack is a descendant of it.
	hist, err = e.aw.ItemHistory(e.ctx, away.Serial)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist.GetSerials()) != 2 || hist.GetMoves()[len(hist.GetMoves())-1].GetRelation() != "descendant" {
		t.Errorf("split history %v", hist)
	}
	// The victim finds the mail at its next login.
	if d, err := e.aw.Character(e.ctx, victim); err != nil || len(d.GetMails()) != 1 || len(d.GetMails()[0].GetItems()) != 5 {
		t.Errorf("victim's mail %v %v", d, err)
	}
	// The thief online lost the stack in its zone.
	if d, err := e.aw.Character(e.ctx, thief); err != nil || !d.GetOnline() || len(d.GetInventory()) != 0 {
		t.Errorf("thief %v %v", d, err)
	}
	// The logs of the thief show the restore.
	logs, err := e.aw.CharacterLogs(e.ctx, thief, admin.LogQuery{Limit: 1})
	if err != nil || len(logs.GetEntries()) != 1 || logs.GetNextCursor() == "" {
		t.Fatalf("logs %v %v", logs, err)
	}
	more, err := e.aw.CharacterLogs(e.ctx, thief, admin.LogQuery{Limit: 10, Cursor: logs.GetNextCursor()})
	if err != nil || len(more.GetEntries()) != 1 || more.GetNextCursor() != "" {
		t.Errorf("second page %v %v", more, err)
	}
}

// TestRestoreWhileLoggingInAndOut restores a stack while its holder logs
// in and out over and over. Whatever the order, the serial ends in one
// place, and the item's total stays.
func TestRestoreWhileLoggingInAndOut(t *testing.T) {
	e := setup(t)
	held := stack(0, 909, 7)
	thief := e.character("Runner", held, stack(1, 909, 5))
	victim := e.character("Victim")
	before := e.total(909)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			s := e.login(thief)
			time.Sleep(5 * time.Millisecond)
			s.Leave()
		}
	})
	time.Sleep(20 * time.Millisecond)
	var res error
	for range 5 {
		_, res = e.aw.Restore(e.ctx, restore(victim, false, admin.RestoreItem{Serial: held.Serial}))
		if res == nil {
			break
		}
	}
	close(stop)
	wg.Wait()
	if res != nil {
		t.Fatalf("restore: %v", res)
	}
	if h := e.holders(held.Serial); len(h) != 1 || h[0].Holder != "mail" {
		t.Errorf("holders %+v", h)
	}
	if after := e.total(909); after != before {
		t.Errorf("jellopy %d, was %d", after, before)
	}
	// The thief logs in once more with what is left.
	s := e.login(thief)
	defer s.Leave()
	if d, err := e.aw.Character(e.ctx, thief); err != nil || len(d.GetInventory()) != 1 || d.GetInventory()[0].GetAmount() != 5 {
		t.Errorf("thief after %v %v", d, err)
	}
}

// TestSearchStates checks Characters and FindSerial.
func TestSearchStates(t *testing.T) {
	e := setup(t)
	it := stack(0, 909, 1)
	a := e.character("Online", it)
	b := e.character("Offline")
	s := e.login(a)
	defer s.Leave()
	states, err := e.aw.Characters(e.ctx, []int64{a, b})
	if err != nil || !states[a].Online || states[a].Position.GetMap() != "one" || states[b].Online {
		t.Errorf("states %+v %v", states, err)
	}
	o, err := e.aw.FindSerial(e.ctx, it.Serial)
	if err != nil || o.GetKind() != "inventory" || o.GetName() != "Online" || o.GetCharacterId() != a || o.GetAccountId() == 0 {
		t.Errorf("holder %v %v", o, err)
	}
	if o, err := e.aw.FindSerial(e.ctx, item.NewSerial()); err != nil || o != nil {
		t.Errorf("unknown serial %v %v", o, err)
	}
	if _, err := e.aw.ItemHistory(e.ctx, item.NewSerial()); !errors.Is(err, admin.ErrNotFound) {
		t.Errorf("history of an unknown serial: %v", err)
	}
	if _, err := e.aw.Character(e.ctx, 999999); !errors.Is(err, admin.ErrNotFound) {
		t.Errorf("unknown character: %v", err)
	}
}
