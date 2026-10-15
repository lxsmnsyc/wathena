package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// jailConfig puts the jail on the field map, which the test world runs.
const jailConfig = `jail:
  map: e2e_field
  cells: [[20, 20]]
`

// TestBanKicksAndRefusesLogin: a ban closes the player's connection with
// the reason, and the account cannot log in again while it lasts
// (decision 0030).
func TestBanKicksAndRefusesLogin(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	p := srv.Join(t, "Cheater")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, code, err := srv.GiveSanction(ctx, "ban", "Cheater", 60*24, "botting in the field"); err != nil || code != "" {
		t.Fatalf("ban: %q %v", code, err)
	}
	select {
	case <-p.Done():
	case <-time.After(DefaultTimeout):
		t.Fatal("the ban did not close the connection")
	}
	var d *botclient.DisconnectError
	if err := p.Err(); !errors.As(err, &d) || d.Reason != wathenav1.DisconnectReason_DISCONNECT_REASON_BANNED ||
		!strings.Contains(d.Message, "botting in the field") || !strings.Contains(d.Message, "banned until") {
		t.Errorf("the banned player got %v", err)
	}
	_, err := botclient.Dial(ctx, srv.WS, []byte("Cheater"), botclient.Options{})
	if err == nil || !strings.Contains(err.Error(), "DISCONNECT_REASON_BANNED") || !strings.Contains(err.Error(), "botting in the field") {
		t.Errorf("a login of the banned account: %v", err)
	}
	// Other players still play.
	srv.Join(t, "Honest")
	srv.CheckLogs()
}

// TestMuteBlocksChat: a staff mute refuses the player's chat, and the
// player sees the reason.
func TestMuteBlocksChat(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	p := srv.Join(t, "Spammer")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, code, err := srv.GiveSanction(ctx, "mute", "Spammer", 30, "flooding #global"); err != nil || code != "" {
		t.Fatalf("mute: %q %v", code, err)
	}
	if n := p.Sanction(false); n.GetKind() != wathenav1.SanctionKind_SANCTION_KIND_MUTE || n.GetReason() != "flooding #global" {
		t.Errorf("notice %v", n)
	}
	if r := p.SayRefused("buy my stuff"); r.GetReason() != wathenav1.ChatRefusal_CHAT_REFUSAL_MUTED {
		t.Errorf("refusal %v", r)
	}
	srv.CheckLogs()
}

// TestJailMovesPlayer: a game master jails a player with @jail. The
// player goes to the jail map at once.
func TestJailMovesPlayer(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true, Config: jailConfig})
	gm := srv.Join(t, "Warden")
	p := srv.Join(t, "Prisoner")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := srv.GiveStaffRole(ctx, "Warden", "game_master"); err != nil {
		t.Fatal(err)
	}
	if got := gm.GMCommand("@jail Prisoner 10 botting"); got != "Jailed Prisoner for 10 minutes." {
		t.Errorf("@jail: %q", got)
	}
	if n := p.Sanction(false); n.GetKind() != wathenav1.SanctionKind_SANCTION_KIND_JAIL || n.GetReason() != "botting" {
		t.Errorf("notice %v", n)
	}
	if m := p.MapChange(FieldMap); m.GetX() != 20 || m.GetY() != 20 {
		t.Errorf("jailed at %d,%d", m.GetX(), m.GetY())
	}
	// @kick ends the connection with its reason.
	if got := gm.GMCommand("@kick Prisoner afk in jail"); got != "Kicked Prisoner." {
		t.Errorf("@kick: %q", got)
	}
	<-p.Done()
	var d *botclient.DisconnectError
	if err := p.Err(); !errors.As(err, &d) || d.Reason != wathenav1.DisconnectReason_DISCONNECT_REASON_KICKED || !strings.Contains(d.Message, "afk in jail") {
		t.Errorf("the kicked player got %v", err)
	}
	srv.CheckLogs()
}

// TestCommandsNeedPermission: a player without the permission of any
// command says a line that starts with @ as chat, as in rAthena (review
// decision 40). A role without a command's permission gets "Unknown
// command." for it, which no one else hears. A game master warps, hides
// and recalls.
func TestCommandsNeedPermission(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	player := srv.Join(t, "Player")
	gm := srv.Join(t, "Gamemaster")
	admin := srv.Join(t, "Overseer")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, line := range []string{"@hide", "@warp e2e_field", "@kick Gamemaster bye"} {
		player.Say(line)
		gm.Hears("Player", line)
	}
	if err := srv.GiveStaffRole(ctx, "Gamemaster", "game_master"); err != nil {
		t.Fatal(err)
	}
	if err := srv.GiveStaffRole(ctx, "Overseer", "admin"); err != nil {
		t.Fatal(err)
	}
	if got := gm.GMCommand("@item 501 10 free potions"); got != "Unknown command." {
		t.Errorf("game master @item: %q", got)
	}
	before := admin.Count(RedPotion)
	if got := admin.GMCommand("@item 501 10 event prize"); !strings.HasPrefix(got, "Item created: 10 x") {
		t.Errorf("admin @item: %q", got)
	}
	admin.Until("ten more Red Potions", func(_ botclient.View, g botclient.Game) bool { return g.Count(RedPotion) == before+10 })

	// @hide: the others see the game master vanish.
	gmID := player.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Gamemaster").ID
	if got := gm.GMCommand("@hide"); got != "Invisible: On" {
		t.Errorf("@hide: %q", got)
	}
	player.Expect("the game master to vanish", func(e *wathenav1.Event) bool { return e.GetVanish().GetId() == gmID })
	// @warp keeps it hidden, and @recall brings the player along.
	if got := gm.GMCommand("@warp e2e_field"); got != "Warped." {
		t.Errorf("@warp: %q", got)
	}
	gm.MapChange(FieldMap)
	if got := gm.GMCommand("@recall Player"); got != "Player recalled!" {
		t.Errorf("@recall: %q", got)
	}
	player.MapChange(FieldMap)
	mark := player.Mark()
	time.Sleep(500 * time.Millisecond)
	events, _ := player.Events(mark)
	for _, e := range events {
		if a := e.GetAppear(); a != nil && a.GetName() == "Gamemaster" {
			t.Error("the player saw the hidden game master on the field")
		}
	}
	// Command lines are never chat.
	for _, p := range []*Player{player, admin} {
		if p.HeardText("free potions") || p.HeardText("event prize") || p.HeardText("Unknown command.") {
			t.Errorf("%s heard a command line", p.Name)
		}
	}
	srv.CheckLogs()
}
