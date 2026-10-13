package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// TestHold checks that a Hold keeps a character from loading, that it
// fails while the character has a session, and that Hold.Load loads it
// for the restore (decision 0029).
func TestHold(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	id, err := s.DevCharacter(ctx, newCharacter("Held"))
	if err != nil {
		t.Fatal(err)
	}
	// A session that has not ended keeps a Hold away.
	c, x, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Live(id) {
		t.Error("a loaded character is not live")
	}
	if _, err := s.Hold(id); !errors.Is(err, store.ErrLive) {
		t.Fatalf("hold of a live character: %v", err)
	}
	x.Save(c, true)
	if err := x.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if s.Live(id) {
		t.Error("an ended session is live")
	}
	h, err := s.Hold(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Hold(id); !errors.Is(err, store.ErrLive) {
		t.Errorf("a second hold: %v", err)
	}
	// A login waits for the hold.
	loaded := make(chan error, 1)
	go func() {
		_, lx, err := s.Load(ctx, id)
		if err == nil {
			lx.Save(c, true)
		}
		loaded <- err
	}()
	select {
	case err := <-loaded:
		t.Fatalf("a load went past the hold: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	// The restore loads it itself, and ends its session.
	hc, hx, err := h.Load(ctx, id)
	if err != nil || hc.ID != id {
		t.Fatalf("hold load: %v", err)
	}
	if _, _, err := h.Load(ctx, id+1); err == nil {
		t.Error("a hold loaded a character it does not keep")
	}
	hx.Save(hc, true)
	h.Release()
	if err := <-loaded; err != nil {
		t.Fatalf("load after the release: %v", err)
	}
}

// TestSettle checks that Settle waits for the commits queued before it.
func TestSettle(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	if err := s.Settle(ctx); err != nil {
		t.Fatal(err)
	}
	serial := item.NewSerial()
	s.Commit(nil, transfer.Logs{Items: []transfer.ItemLog{{At: time.Now(), Action: transfer.ActionGive, From: transfer.End(transfer.KindScript),
		To: transfer.Inventory(1), Serial: serial, ItemID: 501, Amount: 1}}})
	if err := s.Settle(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := s.LastCopy(ctx, serial); err != nil || !found {
		t.Errorf("after settle: found %v, %v", found, err)
	}
}

// logLine writes one item log line.
func logLine(t *testing.T, s *store.Store, at time.Time, serial, source item.Serial, from, to transfer.Owner, amount int) {
	t.Helper()
	s.Commit(nil, transfer.Logs{Items: []transfer.ItemLog{{At: at, Action: transfer.ActionMove, From: from, To: to, Serial: serial,
		Source: source, ItemID: 501, Amount: amount, Refine: 3, Cards: []uint32{4001},
		Extra: item.Extra{Rolls: item.Rolls{{Name: "atk", Value: 5}}}}}})
}

// TestStaffQueries checks the queries of the staff tools on the game
// role: an offline character's view, where a serial is, a gone serial's
// last line, and a character's logs by page.
func TestStaffQueries(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	c := newCharacter("Viewed")
	id, err := s.DevCharacter(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	var account int64
	if err := admin.QueryRow(ctx, "SELECT account_id FROM characters WHERE id = $1", id).Scan(&account); err != nil {
		t.Fatal(err)
	}
	cartSerial, storeSerial, mailSerial := item.NewSerial(), item.NewSerial(), item.NewSerial()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO storage_items (serial, holder, character_id, slot, item_id, amount) VALUES ($1, 'cart', $2, 0, 512, 4)",
			[]any{pgtype.UUID{Bytes: cartSerial, Valid: true}, id}},
		{"INSERT INTO storage_items (serial, holder, account_id, slot, item_id, amount) VALUES ($1, 'account', $2, 3, 909, 7)",
			[]any{pgtype.UUID{Bytes: storeSerial, Valid: true}, account}},
		{"INSERT INTO mails (id, sender_name, recipient_id, recipient_name, title, sent_at) VALUES (77, 'Staff', $1, 'Viewed', 'Hi', now())",
			[]any{id}},
		{"INSERT INTO mail_items (serial, mail_id, slot, item_id, amount) VALUES ($1, 77, 0, 607, 2)",
			[]any{pgtype.UUID{Bytes: mailSerial, Valid: true}}},
		{"INSERT INTO sanctions (character_id, account_id, kind, reason, started_at, ends_at) VALUES ($1, $2, 'mute', 'spam', now(), now() + interval '1 hour')",
			[]any{id, account}},
	} {
		if _, err := admin.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	v, err := s.ReadCharacter(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Character.Name != "Viewed" || len(v.Character.Items) != 3 || len(v.Storages) != 2 || len(v.Mails) != 1 ||
		len(v.Mails[0].Items) != 1 || len(v.Character.Sanctions) != 1 {
		t.Fatalf("view %+v", v)
	}
	if _, err := s.ReadCharacter(ctx, id+100); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a missing character: %v", err)
	}
	// ReadCharacter starts no session: the save version stays.
	var version int64
	if err := admin.QueryRow(ctx, "SELECT save_version FROM characters WHERE id = $1", id).Scan(&version); err != nil || version != 0 {
		t.Errorf("save version %d, %v", version, err)
	}
	for _, c := range []struct {
		serial item.Serial
		want   string
	}{
		{c.Items[0].Serial, "inventory"}, {cartSerial, "cart"}, {storeSerial, "account"}, {mailSerial, "mail"}, {item.NewSerial(), ""},
	} {
		hs, err := s.ItemHolders(ctx, c.serial)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		if len(hs) == 1 {
			got = hs[0].Holder
		}
		if got != c.want {
			t.Errorf("holder of %s: %q, want %q", c.serial, got, c.want)
		}
	}
	// The last line of a serial is its copy.
	gone := item.NewSerial()
	now := time.Now()
	logLine(t, s, now.Add(-time.Hour), gone, item.Serial{}, transfer.Inventory(id), transfer.Inventory(id+1), 9)
	logLine(t, s, now.Add(-time.Minute), gone, item.Serial{}, transfer.Inventory(id+1), transfer.End(transfer.KindUsed), 4)
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	cp, amount, found, err := s.LastCopy(ctx, gone)
	if err != nil || !found || amount != 4 || cp.ID != 501 || cp.Refine != 3 || len(cp.Cards) != 1 || len(cp.Rolls) != 1 {
		t.Errorf("last copy %+v %d %v %v", cp, amount, found, err)
	}
	if _, _, found, _ := s.LastCopy(ctx, item.NewSerial()); found {
		t.Error("a serial without lines has a copy")
	}
	// The character's item log by pages, newest first.
	q := s.Queries()
	far := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	page := func(before time.Time, beforeID int64) []db.CharacterItemLogRow {
		rows, err := q.CharacterItemLog(ctx, db.CharacterItemLogParams{Kinds: []string{"inventory", "cart", "vending"}, CharacterID: id,
			BeforeAt: pgtype.Timestamptz{Time: before, Valid: true}, BeforeID: beforeID,
			Since: pgtype.Timestamptz{Time: time.Unix(0, 0), Valid: true}, Until: pgtype.Timestamptz{Time: far, Valid: true}, MaxRows: 1})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	first := page(far, 1<<62)
	if len(first) != 1 || first[0].Amount != 9 {
		t.Fatalf("first page %+v", first)
	}
	if next := page(first[0].At.Time, first[0].ID); len(next) != 0 {
		t.Errorf("second page %+v", next)
	}
}

// TestItemHistoryQueries follows a serial through a split and a merge
// with the two history queries.
func TestItemHistoryQueries(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	a, b, c := item.NewSerial(), item.NewSerial(), item.NewSerial()
	now := time.Now()
	logLine(t, s, now.Add(-3*time.Minute), a, item.Serial{}, transfer.End(transfer.KindMonster), transfer.Inventory(1), 10)
	// A split of a into b, and a merge of b into c.
	logLine(t, s, now.Add(-2*time.Minute), b, a, transfer.Inventory(1), transfer.Inventory(2), 4)
	logLine(t, s, now.Add(-time.Minute), c, b, transfer.Inventory(2), transfer.Inventory(3), 4)
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	q := s.Queries()
	bySerial, err := q.ItemLogBySerials(ctx, db.ItemLogBySerialsParams{Serials: []pgtype.UUID{{Bytes: b, Valid: true}}, MaxRows: 10})
	if err != nil || len(bySerial) != 1 || bySerial[0].Source.Bytes != a {
		t.Errorf("by serial %+v %v", bySerial, err)
	}
	bySource, err := q.ItemLogBySources(ctx, db.ItemLogBySourcesParams{Serials: []pgtype.UUID{{Bytes: b, Valid: true}}, MaxRows: 10})
	if err != nil || len(bySource) != 1 || bySource[0].Serial.Bytes != c {
		t.Errorf("by source %+v %v", bySource, err)
	}
}

// explain returns a query's plan with sequential scans off, so the plan
// shows whether an index serves it at all, as on a large table.
func explain(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	ctx := timeout(t)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), "RESET enable_seqscan") }()
	rows, err := conn.Query(ctx, "EXPLAIN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, l)
	}
	return strings.Join(lines, "\n")
}

// TestStaffQueryPlans checks that indexes serve the staff tools' queries:
// the history queries on every partition of item_log, the character
// logs, the name prefix searches and the IP search.
func TestStaffQueryPlans(t *testing.T) {
	admin, _ := migrated(t)
	ctx := timeout(t)
	if _, err := admin.Exec(ctx, "SELECT ensure_log_partitions(now() - interval '1 month')"); err != nil {
		t.Fatal(err)
	}
	serials := []pgtype.UUID{{Bytes: item.NewSerial(), Valid: true}}
	far := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name, query string
		args        []any
		want        []string
	}{
		{"history by serial", "SELECT id FROM item_log WHERE serial = ANY($1::uuid[])", []any{serials}, []string{"_serial_at_idx on item_log_default"}},
		{"history by source", "SELECT id FROM item_log WHERE source = ANY($1::uuid[])", []any{serials}, []string{"_source_at_idx"}},
		{"character item log", `SELECT id FROM item_log WHERE ((from_kind = ANY($1::text[]) AND from_id = $2) OR (to_kind = ANY($1::text[]) AND to_id = $2))
			AND at <= $3 ORDER BY at DESC, id DESC LIMIT 51`, []any{[]string{"inventory", "cart"}, int64(5), far}, []string{"_from_", "_to_"}},
		{"account name prefix", "SELECT id FROM accounts WHERE lower(name) LIKE $1", []any{"ali%"}, []string{"accounts_name_prefix"}},
		{"character name prefix", "SELECT account_id FROM characters WHERE lower(name) LIKE $1", []any{"ali%"}, []string{"characters_name_prefix"}},
		{"ip range", "SELECT DISTINCT account_id FROM account_logins WHERE ip <<= $1::cidr AND at >= now() - interval '90 days'",
			[]any{"10.0.0.0/8"}, []string{"account_logins_ip"}},
		{"last item line", "SELECT id FROM item_log WHERE serial = $1 ORDER BY at DESC, id DESC LIMIT 1", []any{serials[0]},
			[]string{"_serial_at_idx"}},
	} {
		plan := explain(t, admin, c.query, c.args...)
		if strings.Contains(plan, "Seq Scan") {
			t.Errorf("%s: a sequential scan:\n%s", c.name, plan)
		}
		for _, w := range c.want {
			if !strings.Contains(plan, w) {
				t.Errorf("%s: the plan does not use %s:\n%s", c.name, w, plan)
			}
		}
	}
}

// TestRestoreAuditInCommit checks that a commit writes its audit rows in
// its transaction, as the game role.
func TestRestoreAuditInCommit(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	s.CommitAll(nil, nil, store.Writes{Audit: []store.AuditRow{{AccountID: 1, AccountName: "Admin1", Role: "admin",
		Action: "items.restore", Source: "admin", Target: "Hero", Reason: "theft", NeedsReview: true,
		Arguments: map[string]any{"mail_id": 5}}}}, transfer.Logs{})
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var action string
	var review bool
	if err := admin.QueryRow(ctx, "SELECT action, needs_review FROM staff_audit").Scan(&action, &review); err != nil ||
		action != "items.restore" || !review {
		t.Errorf("audit row %q %v %v", action, review, err)
	}

}
