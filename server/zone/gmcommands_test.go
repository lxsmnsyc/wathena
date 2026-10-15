package zone

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// fakeModerator records the sanctions of GM commands.
type fakeModerator struct {
	mu     sync.Mutex
	orders []StaffOrder
}

func (m *fakeModerator) Command(_ context.Context, o StaffOrder) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders = append(m.orders, o)
	if o.Target == "Nobody" {
		return "", &CommandError{Text: "Character not found."}
	}
	return "Done: " + o.Kind + " " + o.Target + ".", nil
}

func (m *fakeModerator) got() []StaffOrder {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.orders)
}

// gmWorld is map/a and map/b with a chat hub, the default staff roles,
// and a fake moderator. Account 1 is an admin, 2 a game master and 3 a
// moderator.
type gmWorld struct {
	a, b *Zone
	mod  *fakeModerator
}

func newGMWorld() *gmWorld {
	a, b := twoMaps()
	hub := chat.NewHub(nil, chat.NewFilter(nil))
	reg := staff.NewRegistry(staff.MustDefaultRoles(), nil)
	reg.Load(map[int64]string{1: "admin", 2: "game_master", 3: "moderator"})
	mod := &fakeModerator{}
	for _, z := range []*Zone{a, b} {
		z.Chat, z.Staff, z.Moderator = hub, reg, mod
	}
	return &gmWorld{a: a, b: b, mod: mod}
}

// player joins a character of an account at a cell. twoFactor is how it
// logged in.
func (w *gmWorld) player(z *Zone, id, account int64, name string, x, y int, twoFactor bool) *Player {
	c := savedCharacter()
	c.ID, c.AccountID, c.Name, c.Map, c.X, c.Y = id, account, name, z.Map, x, y
	c.Staff = staff.Login{AccountName: name + "Account", TwoFactor: twoFactor}
	sess := newSession(&saves{})
	sess.staff = c.Staff
	p := newPlayer(sess, c)
	sess.zone, sess.player = z, p
	z.join(p)
	return p
}

// pump runs the commands and arrivals that wait in both zones until
// done holds, for the work that commands hand to other goroutines.
func (w *gmWorld) pump(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("the command did not finish")
		}
		for _, z := range []*Zone{w.a, w.b} {
			select {
			case c := <-z.commands:
				z.handle(c)
			case h := <-z.arrivals:
				z.arrive(h)
			default:
			}
		}
		time.Sleep(time.Millisecond)
	}
}

// say types a line in local chat.
func say(z *Zone, p *Player, text string) {
	z.handle(command{player: p, chat: &chatCommand{kind: chatLocal, text: text}})
}

// answers are the system chat lines of the player's pending events.
func answers(p *Player) []string {
	var out []string
	for _, e := range p.pending {
		if c := e.GetChat(); c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM {
			out = append(out, c.GetText())
		}
	}
	return out
}

// sent are the system chat lines that went out to the player.
func sent(p *Player) []string {
	var out []string
	for _, e := range events(p) {
		if c := e.GetChat(); c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM {
			out = append(out, c.GetText())
		}
	}
	return out
}

func lastSystem(p *Player) string {
	l := answers(p)
	if len(l) == 0 {
		return ""
	}
	return l[len(l)-1]
}

// TestGMCommandsNeedRoles: a staff member's command without the
// permission, or that does not exist, answers "Unknown command.", and no
// command line is chat or reaches the chat log. A player without the
// permission of any command, or a staff member who logged in without a
// second factor, says a line that starts with @ as normal chat, as in
// rAthena (review decision 40).
func TestGMCommandsNeedRoles(t *testing.T) {
	w := newGMWorld()
	z := w.a
	log := &chat.MemoryLog{}
	z.Chat.Log = log
	admin := w.player(z, 10, 1, "Admin", 10, 10, true)
	admin.maxWeight = 1 << 30
	gm := w.player(z, 11, 2, "Gamemaster", 11, 10, true)
	mod := w.player(z, 12, 3, "Moddy", 12, 10, true)
	lazy := w.player(z, 13, 1, "Lazy", 13, 10, false)
	plain := w.player(z, 14, 9, "Plain", 14, 10, true)
	for _, c := range []struct {
		p    *Player
		line string
		want string
	}{
		{admin, "@nonsense", UnknownCommand},
		{admin, "@", UnknownCommand},
		{gm, "@item 501 1 testing", UnknownCommand},
		{mod, "@warp map/b", UnknownCommand},
		{mod, "@recall Plain", UnknownCommand},
		{mod, "@nonsense", UnknownCommand},
		{gm, "@hide", "Invisible: On"},
		{admin, "@ITEM 501 1 testing", "Item created: 1 x Red_Potion."},
	} {
		c.p.pending = nil
		say(z, c.p, c.line)
		if got := lastSystem(c.p); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.p.Name, c.line, got, c.want)
		}
	}
	if len(z.chats) != 0 || len(log.Lines()) != 0 {
		t.Errorf("command lines went out as chat: %d local, %d logged", len(z.chats), len(log.Lines()))
	}
	// Players without a command go out as chat, and get no answer.
	for i, c := range []struct {
		p    *Player
		line string
	}{
		{plain, "@item 501 1 testing"},
		{plain, "@hide"},
		{lazy, "@hide"},
		{plain, "hello @there"},
	} {
		c.p.pending = nil
		say(z, c.p, c.line)
		if got := lastSystem(c.p); got != "" {
			t.Errorf("%s %q: answered %q", c.p.Name, c.line, got)
		}
		if len(z.chats) != i+1 || z.chats[i].text != c.line {
			t.Errorf("%s %q is not chat: %d lines", c.p.Name, c.line, len(z.chats))
		}
	}
	if n := len(log.Lines()); n != 4 {
		t.Errorf("%d chat lines logged, want 4", n)
	}
	if lazy.gmHidden || plain.gmHidden {
		t.Error("an @hide of a player without the permission hid them")
	}
}

// TestGMWarp: @warp to another map, to a cell of the same map, and to a
// cell that is not walkable, with the audit row.
func TestGMWarp(t *testing.T) {
	w := newGMWorld()
	gm := w.player(w.a, 10, 2, "Gamemaster", 10, 10, true)
	say(w.a, gm, "@warp nowhere")
	if got := lastSystem(gm); got != "Map not found." {
		t.Errorf("warp to nowhere: %q", got)
	}
	say(w.a, gm, "@warp a 30 31")
	if e := gm.entity; e.x != 30 || e.y != 31 || lastSystem(gm) != "Warped." {
		t.Errorf("warp on the map: %d,%d %q", e.x, e.y, lastSystem(gm))
	}
	say(w.a, gm, "@warp a 999 5")
	if !slices.Contains(answers(gm), "Invalid coordinates, using a random cell.") {
		t.Errorf("a cell off the map: %v", answers(gm))
	}
	say(w.a, gm, "@warp a x")
	if got := lastSystem(gm); !strings.HasPrefix(got, "Usage: @warp") {
		t.Errorf("bad arguments: %q", got)
	}
	if n := len(w.a.txAudit); n != 2 || w.a.txAudit[0].Action != "commands.warp" || w.a.txAudit[0].Arguments["map"] != "a" ||
		w.a.txAudit[0].AccountName != "GamemasterAccount" || w.a.txAudit[0].Role != "game_master" || w.a.txAudit[0].Source != "game" {
		t.Errorf("audit rows %+v", w.a.txAudit)
	}
	say(w.a, gm, "@warp map/b 7 8")
	q := arrive(t, w.b)
	if q.entity.x != 7 || q.entity.y != 8 {
		t.Errorf("warped to %d,%d", q.entity.x, q.entity.y)
	}
}

// TestGMJumpToAndRecall: @jumpto goes to a player on another map, and
// @recall brings one from another map.
func TestGMJumpToAndRecall(t *testing.T) {
	w := newGMWorld()
	gm := w.player(w.a, 10, 2, "Gamemaster", 10, 10, true)
	target := w.player(w.b, 11, 9, "Target Name", 25, 26, true)
	say(w.a, gm, "@jumpto Nobody")
	if got := lastSystem(gm); got != "Character not found." {
		t.Errorf("jump to nobody: %q", got)
	}
	say(w.a, gm, "@jumpto target name")
	var q *Player
	w.pump(t, func() bool { _, q = gm.sess.where(); return q != nil && q != gm })
	if !slices.Contains(sent(q), "Jumped to Target Name.") {
		t.Error("no answer to @jumpto")
	}
	if z, _ := q.sess.where(); z != w.b || q.entity.x != 25 || q.entity.y != 26 {
		t.Fatalf("jumped to %v %d,%d", z.Map, q.entity.x, q.entity.y)
	}
	if len(w.a.txAudit) != 1 || w.a.txAudit[0].Action != "commands.jumpto" || w.a.txAudit[0].Target != "Target Name" {
		t.Errorf("audit %+v", w.a.txAudit)
	}

	// The game master goes back to map/a and recalls the target there.
	say(w.b, q, "@warp a 12 13")
	gm = arrive(t, w.a)
	say(w.a, gm, "@recall Target Name")
	var r *Player
	w.pump(t, func() bool { z, p := target.sess.where(); r = p; return z == w.a })
	w.pump(t, func() bool { return slices.Contains(answers(gm), "Target Name recalled!") })
	if r.entity.x != 12 || r.entity.y != 13 {
		t.Errorf("recalled to %d,%d", r.entity.x, r.entity.y)
	}
	say(w.a, gm, "@recall Gamemaster")
	if got := lastSystem(gm); got != "You cannot recall yourself." {
		t.Errorf("recall self: %q", got)
	}
	// A jailed player is not recalled.
	r.sanctions = []character.Sanction{{Kind: character.SanctionJail, Reason: "x", Start: w.a.wallClock(), End: w.a.wallClock().Add(time.Hour)}}
	r.entity.x, r.entity.y = 30, 30
	say(w.a, gm, "@recall Target Name")
	w.pump(t, func() bool { return slices.Contains(answers(gm), "Target Name cannot warp while in jail.") })
	if r.entity.x != 30 {
		t.Error("a jailed player was recalled")
	}
}

// TestGMHide: a hidden staff member vanishes for other players, staff
// too, monsters do not see or target it, it stays hidden across a map
// change, and appears again with the next @hide.
func TestGMHide(t *testing.T) {
	w := newGMWorld()
	gm := w.player(w.a, 10, 2, "Gamemaster", 10, 10, true)
	other := w.player(w.a, 11, 3, "Moddy", 12, 10, true)
	w.a.stepAt(50)
	events(other)
	if _, ok := other.netIDs.get(gm.entity); !ok {
		t.Fatal("the other player does not see the game master")
	}
	say(w.a, gm, "@hide")
	w.a.stepAt(100)
	vanished := false
	for _, e := range events(other) {
		if e.GetVanish() != nil {
			vanished = true
		}
	}
	if !vanished {
		t.Error("the hidden game master did not vanish")
	}
	if _, ok := other.netIDs.get(gm.entity); ok {
		t.Error("the other player still knows the hidden game master")
	}
	if _, ok := gm.netIDs.get(gm.entity); !ok {
		t.Error("the hidden game master does not see itself")
	}
	if w.a.visible(other.entity, gm.entity, false) || w.a.canTarget(other.entity, gm.entity) {
		t.Error("a hidden game master can be seen or targeted")
	}
	if !w.a.canTarget(gm.entity, other.entity) {
		t.Error("the hidden game master cannot target others")
	}
	if len(w.a.txAudit) != 1 || w.a.txAudit[0].Action != "commands.hide" || w.a.txAudit[0].Arguments["hidden"] != true {
		t.Errorf("audit %+v", w.a.txAudit)
	}
	// The hidden game master's local chat shows to others in range, as
	// rAthena sends it to the area. They do not know its entity, so the
	// line has no speaker ID (decision 0036).
	far := w.player(w.a, 13, 4, "Faraway", 10, 25, true)
	w.a.stepAt(150)
	events(far)
	say(w.a, gm, "nothing to see here")
	w.a.stepAt(200)
	heard := false
	for _, e := range events(other) {
		if c := e.GetChat(); c != nil && c.GetText() == "nothing to see here" {
			heard = c.GetFrom() == 0 && c.GetFromName() == "Gamemaster" && len(c.GetLineId()) > 0
		}
	}
	if !heard {
		t.Error("the other player did not hear the hidden game master by name")
	}
	for _, e := range events(far) {
		if c := e.GetChat(); c != nil && c.GetText() == "nothing to see here" {
			t.Error("a player out of chat range heard the hidden game master")
		}
	}
	// Across a map change.
	watcher := w.player(w.b, 12, 9, "Watcher", 7, 8, true)
	w.b.stepAt(50)
	events(watcher)
	say(w.a, gm, "@warp b 7 9")
	q := arrive(t, w.b)
	w.b.stepAt(100)
	if !q.gmHidden {
		t.Fatal("@hide did not last across the map change")
	}
	for _, e := range events(watcher) {
		if a := e.GetAppear(); a != nil && a.GetName() == "Gamemaster" {
			t.Error("the watcher saw the hidden game master arrive")
		}
	}
	say(w.b, q, "@hide")
	if got := lastSystem(q); got != "Invisible: Off" {
		t.Errorf("the second @hide: %q", got)
	}
	w.b.stepAt(150)
	seen := false
	for _, e := range events(watcher) {
		if a := e.GetAppear(); a != nil && a.GetName() == "Gamemaster" {
			seen = true
		}
	}
	if !seen {
		t.Error("the watcher did not see the game master after the second @hide")
	}
}

// TestGMHideLastsAcrossLogins: the character saves @hide with the option
// bit OPTION_INVISIBLE, as pc_makesavestatus, and the next login starts
// hidden while its role may still use @hide, as pc_authok (decision
// 0036). Looks never carry the bit.
func TestGMHideLastsAcrossLogins(t *testing.T) {
	w := newGMWorld()
	gm := w.player(w.a, 10, 2, "Gamemaster", 10, 10, true)
	say(w.a, gm, "@hide")
	saved := w.a.snapshot(gm)
	if saved.Options&companion.OptionInvisible == 0 {
		t.Fatalf("the save has options %#x, without OPTION_INVISIBLE", saved.Options)
	}
	if gm.record.Options&companion.OptionInvisible != 0 {
		t.Error("the playing record carries OPTION_INVISIBLE")
	}
	w.a.leave(gm, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)

	login := func(id int64, account int64, twoFactor bool) *Player {
		c := *saved
		c.ID, c.AccountID = id, account
		c.Staff = staff.Login{AccountName: "GamemasterAccount", TwoFactor: twoFactor}
		sess := newSession(&saves{})
		sess.staff = c.Staff
		p := newPlayer(sess, &c)
		sess.zone, sess.player = w.a, p
		w.a.join(p)
		return p
	}
	back := login(10, 2, true)
	other := w.player(w.a, 11, 3, "Moddy", 12, 10, true)
	w.a.stepAt(50)
	events(other)
	if !back.gmHidden {
		t.Fatal("the game master is not hidden after a new login")
	}
	if _, ok := other.netIDs.get(back.entity); ok {
		t.Error("another player sees the game master who logged in hidden")
	}
	if back.record.Options&companion.OptionInvisible != 0 {
		t.Error("the restored record carries OPTION_INVISIBLE")
	}
	if s := w.a.snapshot(back); s.Options&companion.OptionInvisible == 0 {
		t.Error("the next save lost OPTION_INVISIBLE")
	}
	say(w.a, back, "@hide")
	if back.gmHidden || w.a.snapshot(back).Options&companion.OptionInvisible != 0 {
		t.Error("the second @hide did not end the saved hide")
	}
	w.a.leave(back, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)

	// A login without a second factor has no role, so the hide ends, as
	// pc_authok clears the bit without the hide command.
	plain := login(10, 2, false)
	if plain.gmHidden || w.a.snapshot(plain).Options&companion.OptionInvisible != 0 {
		t.Error("a login that may not use @hide stayed hidden")
	}
	w.a.leave(plain, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	// So does an account whose role lacks commands.hide.
	c := *saved
	c.ID, c.AccountID, c.Name = 14, 3, "Moderator"
	c.Staff = staff.Login{AccountName: "ModeratorAccount", TwoFactor: true}
	sess := newSession(&saves{})
	sess.staff = c.Staff
	p := newPlayer(sess, &c)
	sess.zone, sess.player = w.a, p
	w.a.join(p)
	if p.gmHidden {
		t.Error("a moderator without commands.hide logged in hidden")
	}
}

// TestGMModerationCommands: @mute, @jail and @kick read names with
// spaces, in quotes or not, and hand the sanction to the Moderator,
// whose answer comes back.
func TestGMModerationCommands(t *testing.T) {
	w := newGMWorld()
	mod := w.player(w.a, 10, 3, "Moddy", 10, 10, true)
	w.player(w.b, 11, 9, "Big Bob", 10, 10, true)
	for _, c := range []struct {
		line string
		want StaffOrder
	}{
		{"@mute Big Bob 10 spam in chat", StaffOrder{Kind: "mute", Target: "Big Bob", Minutes: 10, Reason: "spam in chat"}},
		{`@jail "Big Bob" 60 botting`, StaffOrder{Kind: "jail", Target: "Big Bob", Minutes: 60, Reason: "botting"}},
		{"@kick big bob afk in the way", StaffOrder{Kind: "kick", Target: "big bob", Reason: "afk in the way"}},
		{"@kick Nobody rude", StaffOrder{Kind: "kick", Target: "Nobody", Reason: "rude"}},
	} {
		before := len(w.mod.got())
		mod.pending = nil
		say(w.a, mod, c.line)
		w.pump(t, func() bool { return len(answers(mod)) > 0 })
		got := w.mod.got()
		if len(got) != before+1 {
			t.Fatalf("%q: no order", c.line)
		}
		o := got[before]
		if o.Kind != c.want.Kind || o.Target != c.want.Target || o.Minutes != c.want.Minutes || o.Reason != c.want.Reason {
			t.Errorf("%q: %+v, want %+v", c.line, o, c.want)
		}
		if got[before].Actor.Role != "moderator" || got[before].Actor.AccountID != 3 || got[before].Actor.AccountName != "ModdyAccount" {
			t.Errorf("actor %+v", got[before].Actor)
		}
	}
	if got := lastSystem(mod); got != "Character not found." {
		t.Errorf("the moderator's error: %q", got)
	}
	for _, line := range []string{"@mute Big Bob 10", "@mute Big Bob spam", "@jail Big Bob 0 x", `@kick "Big Bob"`, "@kick"} {
		mod.pending = nil
		say(w.a, mod, line)
		if got := lastSystem(mod); !strings.HasPrefix(got, "Usage: ") {
			t.Errorf("%q: %q", line, got)
		}
	}
	say(w.a, mod, "@mute Big Bob 525601 forever")
	if got := lastSystem(mod); !strings.HasPrefix(got, "A mute lasts at most") {
		t.Errorf("a long mute: %q", got)
	}
	w.a.Moderator = nil
	say(w.a, mod, "@mute Big Bob 5 x")
	if got := lastSystem(mod); got != "Moderation is not available on this server." {
		t.Errorf("without a moderator: %q", got)
	}
}

// TestGMItem: @item makes items from the staff end, with an audit row
// marked for review.
func TestGMItem(t *testing.T) {
	w := newGMWorld()
	z := w.a
	admin := w.player(z, 10, 1, "Admin", 10, 10, true)
	admin.maxWeight = 1 << 30
	before := admin.inventory.Count(909)
	say(z, admin, "@item Jellopy 25 event prize")
	if got := admin.inventory.Count(909) - before; got != 25 {
		t.Errorf("%d Jellopies made", got)
	}
	say(z, admin, "@item 1201 2 test knives")
	if lastSystem(admin) != "Item created: 2 x Knife." {
		t.Errorf("knives: %q", lastSystem(admin))
	}
	if len(z.txAudit) != 2 || !z.txAudit[0].NeedsReview || z.txAudit[0].Reason != "event prize" || z.txAudit[0].Action != "commands.item" ||
		z.txAudit[0].Arguments["amount"] != 25 {
		t.Errorf("audit %+v", z.txAudit)
	}
	var made int
	for _, l := range z.txLogs.Items {
		if l.Action == transfer.ActionCommand && l.From.Kind == transfer.KindStaff && l.From.ID == 1 {
			made += l.Amount
		}
	}
	if made != 27 {
		t.Errorf("the item log has %d made by the command", made)
	}
	for line, want := range map[string]string{
		"@item 501 5":            "Usage: @item <item> <amount> <reason>",
		"@item Nothing 5 x":      "Invalid item ID or name.",
		"@item 501 0 x":          "The amount is 1 to 30000.",
		"@item 1201 101 x":       "At most 100 of an item that does not stack.",
		"@item 99999999999 1 x":  "Invalid item ID or name.",
		"@item red_potion 1 why": "Item created: 1 x Red_Potion.",
	} {
		say(z, admin, line)
		if got := lastSystem(admin); got != want {
			t.Errorf("%q: %q, want %q", line, got, want)
		}
	}
}

// TestLiftSanctionInZone: a lifted mute ends at once, and a lifted jail
// sends the player back.
func TestLiftSanctionInZone(t *testing.T) {
	w := newSocialWorld(nil, "map/a")
	z := w.zones[0]
	z.Jail = Jail{Map: "map/a", Cells: [][2]int{{5, 5}}}
	alice := w.join(z, "Alice")
	w.step(50)
	alice.entity.x, alice.entity.y = 20, 21
	z.addSanction(alice, character.SanctionJail, 60, "botting", "Staff")
	z.addSanction(alice, character.SanctionMute, 60, "spam", "Staff")
	w.step(100)
	if e := alice.entity; e.x != 5 || e.y != 5 {
		t.Fatalf("jailed at %d,%d", e.x, e.y)
	}
	mute := *alice.sanction(character.SanctionMute, w.now)
	jail := *alice.sanction(character.SanctionJail, w.now)
	z.liftSanction(alice, character.SanctionMute, mute.Start.Add(time.Nanosecond))
	if alice.sanction(character.SanctionMute, w.now) != nil {
		t.Error("the mute was not lifted")
	}
	z.liftSanction(alice, character.SanctionJail, jail.Start)
	if e := alice.entity; e.x != 20 || e.y != 21 || z.jailed(alice) {
		t.Errorf("after the lift at %d,%d, jailed %v", e.x, e.y, z.jailed(alice))
	}
}
