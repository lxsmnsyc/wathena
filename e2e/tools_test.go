package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// restartExitCode is waserver's exit status after a planned restart
// (server/restart.ExitCode).
const restartExitCode = 75

// TestAnnouncementsAndCancel: an announcement reaches the world and one
// map's players. A planned restart reaches the mods' restart_planned
// handlers, and a cancel announces itself and reaches them too
// (decision 0031).
func TestAnnouncementsAndCancel(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	p := srv.Join(t, "Listener")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := srv.Announce(ctx, "Welcome to the test world!", ""); err != nil {
		t.Fatal(err)
	}
	p.Announcement("Welcome to the test world!")
	// A line to another map does not reach the player. One to its own
	// map does.
	if err := srv.Announce(ctx, "Field news", "e2e_field"); err != nil {
		t.Fatal(err)
	}
	if err := srv.Announce(ctx, "Town news", StartMap); err != nil {
		t.Fatal(err)
	}
	p.Announcement("Town news")
	if p.HeardText("Field news") {
		t.Error("a line to another map reached the player")
	}
	if err := srv.Announce(ctx, "x", "nowhere"); err == nil {
		t.Error("an announcement to a map that does not run went out")
	}

	plan, err := srv.PlanRestart(ctx, 10*time.Minute, "update", "")
	if err != nil || plan.Error != "" {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	if got := p.SystemLine("e2e restart:"); got != fmt.Sprintf("e2e restart: at %d for update", plan.At.Unix()) {
		t.Errorf("restart_planned: %q", got)
	}
	if again, err := srv.PlanRestart(ctx, 20*time.Minute, "again", ""); err != nil || again.Error != "conflict" {
		t.Errorf("a second plan: %+v %v", again, err)
	}
	if _, err := srv.CancelRestart(ctx); err != nil {
		t.Fatal(err)
	}
	p.Announcement("The planned server restart is cancelled.")
	if got := p.SystemLine("e2e restart:"); got != "e2e restart: cancelled" {
		t.Errorf("restart_planned after the cancel: %q", got)
	}
	// The old time passes without a restart.
	if err := srv.AdvanceClock(ctx, 11*time.Minute); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if _, exited := srv.WaitExit(0); exited {
		t.Fatal("the server stopped after the restart was cancelled")
	}
	srv.Join(t, "Latecomer")
	srv.CheckLogs()
}

// TestPlannedRestart: a planned restart announces itself, refuses new
// logins in its last minute, disconnects the players with its reason,
// saves them and exits with the status that the process manager
// restarts on. The world then starts again.
func TestPlannedRestart(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	p := srv.Join(t, "Waiting")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	plan, err := srv.PlanRestart(ctx, 2*time.Minute, "update", "New maps arrive!")
	if err != nil || plan.Error != "" {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	p.SystemLine("e2e restart: at")
	// 59 seconds left: the 1 minute mark, and logins close.
	if err := srv.AdvanceClock(ctx, 61*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := p.Announcement("restarts in").GetText(); got != "The server restarts in 1 minute. New maps arrive!" {
		t.Errorf("at 1 minute: %q", got)
	}
	_, err = botclient.Dial(ctx, srv.WS, []byte("Latecomer"), botclient.Options{})
	if err == nil || !strings.Contains(err.Error(), "DISCONNECT_REASON_RESTART") || !strings.Contains(err.Error(), "less than a minute: update") {
		t.Errorf("a login in the last minute: %v", err)
	}
	if err := srv.AdvanceClock(ctx, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if got := p.Announcement("restarts in").GetText(); got != "The server restarts in 30 seconds. New maps arrive!" {
		t.Errorf("at 30 seconds: %q", got)
	}
	if err := srv.AdvanceClock(ctx, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	var d *botclient.DisconnectError
	select {
	case <-p.Done():
	case <-time.After(DefaultTimeout):
		t.Fatal("the restart did not disconnect the player")
	}
	if err := p.Err(); !errors.As(err, &d) || d.Reason != wathenav1.DisconnectReason_DISCONNECT_REASON_RESTART ||
		d.Message != "The server is restarting: update. Come back in a few minutes." {
		t.Errorf("the player got %v", err)
	}
	code, exited := srv.WaitExit(30 * time.Second)
	if !exited || code != restartExitCode {
		t.Fatalf("the server exited %v with %d", exited, code)
	}
	srv.CheckLogs()
	// The process manager starts it again. The character comes back as it
	// was saved.
	srv.Restart()
	again := srv.Join(t, "Waiting")
	if again.Count(RedPotion) != 10 {
		t.Errorf("after the restart the character has %d Red Potions", again.Count(RedPotion))
	}
	srv.CheckLogs()
}
