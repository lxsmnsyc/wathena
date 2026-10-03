package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/store"
)

// TestChatLogAndReports writes lines and a report, and checks that the
// retention keeps the lines of an open report only.
func TestChatLogAndReports(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	l := store.NewChatLog(game, store.ChatLogOptions{})
	old := time.Now().Add(-40 * 24 * time.Hour).Truncate(time.Millisecond)
	var lines []chat.Line
	for i, text := range []string{"hello", "rude", "bye"} {
		ln := chat.Line{ID: chat.NewLineID(old), At: old.Add(time.Duration(i) * time.Second), Kind: chat.LineLocal,
			CharacterID: 7, Name: "Alice", Map: "map/prontera", Text: text}
		l.Line(ln)
		lines = append(lines, ln)
	}
	other := chat.Line{ID: chat.NewLineID(old), At: old, Kind: chat.LineWhisper, Name: "Bob", To: "Carol", Map: "map/prontera", Text: "psst"}
	l.Line(other)
	l.Report(chat.Report{At: old, Reporter: "Dave", Target: "Alice", TargetID: 7, Line: lines[1].ID, Reason: "insults",
		Map: "map/prontera", Context: lines[:2]})
	// Close writes what waits.
	closeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := l.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	l = store.NewChatLog(game, store.ChatLogOptions{Retention: time.Hour})
	defer func() { _ = l.Close(context.Background()) }()
	reports, err := l.Reports(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("reports %+v", reports)
	}
	r := reports[0]
	if r.Status != "open" || r.Target != "Alice" || r.Line != lines[1].ID || len(r.Context) != 2 || r.Context[1].Text != "rude" {
		t.Errorf("report %+v", r)
	}
	// Old lines go, except those of the open report. The new log's
	// own retention run may have deleted them already.
	left := func() int {
		var n int
		if err := game.QueryRow(ctx, `SELECT count(*) FROM chat_log`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := l.DeleteOld(ctx, time.Now().Add(-30*24*time.Hour)); err != nil || left() != 2 {
		t.Errorf("%d lines left, %v; want the report's 2", left(), err)
	}
	if ok, err := l.CloseReport(ctx, r.ID); err != nil || !ok {
		t.Fatalf("close: %v %v", ok, err)
	}
	if n, err := l.DeleteOld(ctx, time.Now().Add(-30*24*time.Hour)); err != nil || n != 2 || left() != 0 {
		t.Errorf("deleted %d, %v after closing; want 2", n, err)
	}
	reports, _ = l.Reports(ctx, 10)
	if len(reports) != 1 || reports[0].Status != "closed" || len(reports[0].Context) != 0 {
		t.Errorf("closed report %+v", reports)
	}
}
