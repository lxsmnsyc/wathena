package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/store"
)

// TestModerationRows: staff sanctions, bans, lifts and the report queue
// on the game role, with their audit rows (decision 0030).
func TestModerationRows(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	var account, char, other int64
	if err := admin.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Target') RETURNING id").Scan(&account); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		to   *int64
	}{{"Rude", &char}, {"Alt", &other}} {
		if err := admin.QueryRow(ctx, `INSERT INTO characters (account_id, slot, name, job, sex, base_level, job_level,
			str, agi, vit, int, dex, luk, hp, sp, map, x, y)
			VALUES ($1, (SELECT count(*) FROM characters WHERE account_id = $1), $2, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 40, 11, 'map/prontera', 150, 160)
			RETURNING id`, account, c.name).Scan(c.to); err != nil {
			t.Fatal(err)
		}
	}
	audit := func(action string) store.AuditRow {
		return store.AuditRow{AccountID: 99, AccountName: "Admin", Role: "admin", Action: action, Source: "admin", Reason: "spam"}
	}
	now := time.Now().Truncate(time.Microsecond)
	mute := store.CharacterSanction{Character: char, Kind: store.SanctionMute, Reason: "spam", Start: now, End: now.Add(time.Hour), By: "Admin"}
	id, err := s.AddCharacterSanction(ctx, mute, audit("moderation.mute"))
	if err != nil {
		t.Fatal(err)
	}
	// The zone's save of the same mute is the same row.
	again, err := s.AddCharacterSanction(ctx, mute, audit("moderation.mute"))
	if err != nil || again != id {
		t.Errorf("the mute again: %d, %v; want %d", again, err, id)
	}
	if _, err := s.AddCharacterSanction(ctx, store.CharacterSanction{Character: 1 << 40, Kind: store.SanctionMute, Reason: "x",
		Start: now, End: now.Add(time.Minute)}, audit("moderation.mute")); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("a mute of no character: %v", err)
	}
	kick, err := s.AddCharacterSanction(ctx, store.CharacterSanction{Character: other, Kind: store.SanctionKick, Reason: "afk",
		Start: now, End: now, By: "Admin"}, audit("moderation.kick"))
	if err != nil {
		t.Fatal(err)
	}
	ban, err := s.AddBan(ctx, account, "scam", now, time.Time{}, "Admin", audit("moderation.ban"))
	if err != nil {
		t.Fatal(err)
	}
	row, err := s.Sanction(ctx, ban)
	if err != nil || !row.Permanent || !row.End.IsZero() || row.Account != "Target" || row.CharacterID != 0 || !row.Active(time.Now()) {
		t.Errorf("the ban %+v, %v", row, err)
	}
	if b, ok, err := s.ActiveBan(ctx, other); err != nil || !ok || b.ID != ban || !b.Permanent || b.Reason != "scam" {
		t.Errorf("the ban of Alt %+v %v %v", b, ok, err)
	}
	if bans, err := s.ActiveBans(ctx, []int64{account, account + 1}); err != nil || len(bans) != 1 || bans[account].ID != ban {
		t.Errorf("bans %+v %v", bans, err)
	}

	// The lists: a character's own sanctions and its account's bans, or
	// every sanction of the account.
	list := func(q store.SanctionQuery) []int64 {
		t.Helper()
		q.Limit = 10
		rows, err := s.SearchSanctions(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	if got := list(store.SanctionQuery{Character: char}); len(got) != 2 || got[0] != ban || got[1] != id {
		t.Errorf("Rude's sanctions %v", got)
	}
	if got := list(store.SanctionQuery{Account: account}); len(got) != 3 || got[1] != kick {
		t.Errorf("the account's sanctions %v", got)
	}
	if got := list(store.SanctionQuery{Kind: store.SanctionBan, Active: true}); len(got) != 1 {
		t.Errorf("active bans %v", got)
	}
	if got := list(store.SanctionQuery{Account: account, Active: true}); len(got) != 2 {
		t.Errorf("active sanctions %v (a kick is never active)", got)
	}
	if got := list(store.SanctionQuery{Account: account, BeforeID: ban}); len(got) != 2 {
		t.Errorf("a page before the ban %v", got)
	}

	// Lifts: once, not for a kick, and the ban ends.
	if ok, err := s.LiftSanction(ctx, ban, time.Now(), "Admin", "appeal", audit("moderation.lift")); err != nil || !ok {
		t.Errorf("lift: %v %v", ok, err)
	}
	if ok, err := s.LiftSanction(ctx, ban, time.Now(), "Admin", "again", audit("moderation.lift")); err != nil || ok {
		t.Errorf("a second lift: %v %v", ok, err)
	}
	if ok, err := s.LiftSanction(ctx, kick, time.Now(), "Admin", "x", audit("moderation.lift")); err != nil || ok {
		t.Errorf("a kick's lift: %v %v", ok, err)
	}
	if _, ok, err := s.ActiveBan(ctx, other); err != nil || ok {
		t.Errorf("a lifted ban still counts: %v %v", ok, err)
	}
	row, _ = s.Sanction(ctx, ban)
	if row.LiftedBy != "Admin" || row.LiftReason != "appeal" || row.Active(time.Now()) {
		t.Errorf("the lifted ban %+v", row)
	}
	// Each change wrote its audit row, and the failed ones none.
	var n int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM staff_audit").Scan(&n); err != nil || n != 5 {
		t.Errorf("%d audit rows, %v; want 5", n, err)
	}

	// The report queue: a line report with its context and the target's
	// lines around it, a guild emblem report, filters and closing.
	l := store.NewChatLog(game, store.ChatLogOptions{})
	at := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	seen := chat.Line{ID: chat.NewLineID(at), At: at, Kind: chat.LineLocal, CharacterID: char, Name: "Rude", Map: "map/prontera", Text: "rude"}
	before := chat.Line{ID: chat.NewLineID(at), At: at.Add(-2 * time.Minute), Kind: chat.LineWhisper, CharacterID: char, Name: "Rude",
		To: "Someone", Map: "map/prontera", Text: "hidden whisper"}
	far := chat.Line{ID: chat.NewLineID(at), At: at.Add(-time.Hour), Kind: chat.LineLocal, CharacterID: char, Name: "Rude",
		Map: "map/prontera", Text: "long ago"}
	for _, ln := range []chat.Line{seen, before, far} {
		l.Line(ln)
	}
	l.Report(chat.Report{At: at.Add(time.Second), ReporterID: other, Reporter: "Alt", Target: "Rude", TargetID: char, Line: seen.ID,
		Reason: "insults", Map: "map/prontera", Context: []chat.Line{seen}})
	l.Report(chat.Report{At: at.Add(2 * time.Second), Reporter: "Alt", Target: "Bad Guild", Map: "map/prontera", Guild: 3, Emblem: 2,
		EmblemPNG: []byte{0x89, 'P', 'N', 'G'}})
	closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := l.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	reports, err := s.Reports(ctx, store.ReportQuery{Status: "open", Limit: 10})
	if err != nil || len(reports) != 2 || reports[0].Kind() != "guild_emblem" || reports[1].Kind() != "line" ||
		reports[1].TargetAccount != account {
		t.Fatalf("reports %+v %v", reports, err)
	}
	for _, c := range []struct {
		q    store.ReportQuery
		want int
	}{
		{store.ReportQuery{Kind: "line"}, 1},
		{store.ReportQuery{Kind: "guild_name"}, 0},
		{store.ReportQuery{Target: "rude"}, 1},
		{store.ReportQuery{Status: "closed"}, 0},
		{store.ReportQuery{BeforeID: reports[0].ID}, 1},
	} {
		c.q.Limit = 10
		if got, err := s.Reports(ctx, c.q); err != nil || len(got) != c.want {
			t.Errorf("%+v: %d reports, %v; want %d", c.q, len(got), err, c.want)
		}
	}
	rep, lines, err := s.Report(ctx, reports[1].ID)
	if err != nil || rep.Line != seen.ID || len(lines) != 2 || lines[0].Text != "hidden whisper" || lines[0].Seen || !lines[1].Seen {
		t.Errorf("report %+v lines %+v %v", rep, lines, err)
	}
	if rep, _, err := s.Report(ctx, reports[0].ID); err != nil || string(rep.EmblemPNG) != "\x89PNG" {
		t.Errorf("the emblem report %+v %v", rep, err)
	}
	if _, _, err := s.Report(ctx, 1<<40); !errors.Is(err, store.ErrNoRow) {
		t.Errorf("no report: %v", err)
	}
	ok, err := s.CloseReport(ctx, reports[1].ID, "sanctioned", "muted", "Admin", id, audit("moderation.close_report"))
	if err != nil || !ok {
		t.Fatalf("close: %v %v", ok, err)
	}
	if ok, err := s.CloseReport(ctx, reports[1].ID, "warned", "", "Admin", 0, audit("moderation.close_report")); err != nil || ok {
		t.Errorf("a second close: %v %v", ok, err)
	}
	rep, _, _ = s.Report(ctx, reports[1].ID)
	if rep.Status != "closed" || rep.Outcome != "sanctioned" || rep.SanctionID != id || rep.By != "Admin" || rep.Note != "muted" {
		t.Errorf("the closed report %+v", rep)
	}
	// The database refuses a sanction on a report that was not
	// sanctioned.
	if _, err := admin.Exec(ctx, "UPDATE reports SET outcome = 'warned' WHERE id = $1", reports[1].ID); err == nil {
		t.Error("a warned report kept its sanction")
	}
}

// TestModerationRights: the account role reads only the columns of
// sanctions that the ban check needs.
func TestModerationRights(t *testing.T) {
	admin, _ := migrated(t)
	ctx := timeout(t)
	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE wathena_account"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT reason, ends_at FROM sanctions WHERE kind = 'ban' AND lifted_at IS NULL AND account_id = 1"); err != nil {
		t.Errorf("the account role cannot read bans: %v", err)
	}
	if _, err := tx.Exec(ctx, "SAVEPOINT s"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT return_map FROM sanctions"); err == nil {
		t.Error("the account role read return_map")
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT s"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM sanctions"); err == nil {
		t.Error("the account role deleted sanctions")
	}
}
