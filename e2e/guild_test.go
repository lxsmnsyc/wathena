package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"golang.org/x/image/bmp"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// guildItems are the start items of the guild scenarios: an Emperium
// each, and the default items.
const guildItems = "714:1," + DefaultStartItems

// memberNames lists a GuildInfo's members in slot order.
func memberNames(info *wathenav1.GuildInfo) []string {
	var out []string
	for _, m := range info.GetMembers() {
		out = append(out, m.GetName())
	}
	return out
}

// TestGuildCreateAndInvite: a player with an Emperium makes a guild,
// which takes the Emperium, and invites a player, who accepts. Both see
// the two members. A player in sight sees the member's new badge, and a
// player who comes later sees both badges when they appear.
func TestGuildCreateAndInvite(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	if alice.Count(Emperium) != 1 {
		t.Fatalf("Alice holds %d Emperiums", alice.Count(Emperium))
	}
	bobID := carol.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Bob").ID

	info := alice.CreateGuild("Crew")
	if info.GetId() == 0 || info.GetLevel() != 1 || info.GetMaster() != "Alice" || info.GetPosition() != 0 ||
		!slices.Equal(memberNames(info), []string{"Alice"}) || len(info.GetPositions()) != 20 || info.GetMaxMembers() != 16 {
		t.Fatalf("new guild %v", info)
	}
	alice.Until("the Emperium is gone", func(_ View, g Game) bool { return g.Count(Emperium) == 0 })
	if e := alice.Seen(func(e *wathenav1.Event) bool {
		r := e.GetInventoryRemove()
		return r != nil && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_GUILD
	}); e == nil {
		t.Error("Alice saw no InventoryRemove with INVENTORY_REMOVE_REASON_GUILD")
	}
	if r := alice.InviteToGuild("Crew"); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_NOT_ONLINE {
		t.Errorf("inviting a name that is not online: %v", r)
	}
	if r := alice.InviteToGuild("Bob"); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_INVITE_SENT || r.GetName() != "Bob" {
		t.Fatalf("invite %v", r)
	}
	inv := bob.GuildInvited("Alice")
	if inv.GetGuildId() != info.GetId() || inv.GetGuildName() != "Crew" {
		t.Errorf("invite %v", inv)
	}
	joined := bob.AcceptGuild(inv)
	alice.GuildOutcome(wathenav1.GuildOutcome_GUILD_OUTCOME_ACCEPTED, "Bob")
	seen := alice.GuildInfo(func(i *wathenav1.GuildInfo) bool { return len(i.GetMembers()) == 2 })
	for who, got := range map[string]*wathenav1.GuildInfo{"Alice": seen, "Bob": joined} {
		if !slices.Equal(memberNames(got), []string{"Alice", "Bob"}) || got.GetId() != info.GetId() {
			t.Errorf("%s sees %v", who, got)
			continue
		}
		for _, m := range got.GetMembers() {
			master := m.GetName() == "Alice"
			wantPos := NewbiePosition
			if master {
				wantPos = 0
			}
			if !m.GetOnline() || m.GetMap() != StartMap || m.GetJob() != 1 || m.GetBaseLevel() != 50 || m.GetMaster() != master || m.GetPosition() != wantPos {
				t.Errorf("%s sees member %v", who, m)
			}
		}
	}
	if joined.GetPosition() != NewbiePosition {
		t.Errorf("Bob's own position is %d", joined.GetPosition())
	}

	// Carol, in sight, sees Bob's new badge.
	ev := carol.Expect("Bob's badge", func(e *wathenav1.Event) bool {
		g := e.GetEntityGuild()
		return g != nil && g.GetId() == bobID && g.GetGuild() != nil
	}).GetEntityGuild().GetGuild()
	if ev.GetGuildId() != info.GetId() || ev.GetName() != "Crew" || ev.GetPosition() != "Newbie" || ev.GetEmblemVersion() != 0 {
		t.Errorf("Carol sees Bob's badge %v", ev)
	}

	// Dave comes later and sees both badges as the players appear.
	dave := srv.Join(t, "Dave")
	for name, pos := range map[string]string{"Alice": "GuildMaster", "Bob": "Newbie"} {
		a := dave.Seen(func(e *wathenav1.Event) bool { return e.GetAppear().GetName() == name })
		if a == nil {
			a = dave.Expect(name+" to appear", func(e *wathenav1.Event) bool { return e.GetAppear().GetName() == name })
		}
		b := a.GetAppear().GetGuild()
		if b.GetGuildId() != info.GetId() || b.GetName() != "Crew" || b.GetPosition() != pos {
			t.Errorf("Dave sees %s with badge %v", name, b)
		}
	}
	if e := dave.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Carol"); e.Guild != nil {
		t.Errorf("Carol has badge %v", e.Guild)
	}
	srv.CheckLogs()
}

// TestGuildInviteBlocked: a player without an Emperium cannot make a
// guild. A player who blocks the master refuses its invite at once and
// sees nothing of it.
func TestGuildInviteBlocked(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	bob.Drop(Emperium, 1)
	bob.send("create a guild", func(ctx context.Context) error { return bob.GuildCreate(ctx, "Loners") })
	if r := bob.GuildResult(); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_NO_EMPERIUM {
		t.Errorf("creating without an Emperium: %v", r)
	}

	bob.BlockName("Alice", false)
	alice.CreateGuild("Crew")
	if r := alice.InviteToGuild("Bob"); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_REFUSED || r.GetName() != "Bob" {
		t.Fatalf("invite %v", r)
	}
	// A whisper from Bob comes after the invite was refused. Bob has seen
	// nothing of the invite by then.
	bob.Whisper("Alice", "no thanks")
	alice.HearsWhisper("Bob", "no thanks")
	if e := bob.Seen((*wathenav1.Event).HasGuildInvite); e != nil {
		t.Errorf("Bob saw %v", e)
	}
	if g := bob.Game().Guild; g != nil {
		t.Errorf("Bob is in %+v", g)
	}
	srv.CheckLogs()
}

// TestGuildPositions: a new member's position cannot invite. The master
// lets position 1 invite and moves the member there; the member then
// invites a player, who joins. A player in sight sees the member's new
// position name.
func TestGuildPositions(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	dave := srv.Join(t, "Dave")
	bobID := dave.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Bob").ID
	MakeGuild("Crew", alice, bob)

	if r := bob.InviteToGuild("Carol"); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_NO_PERMISSION {
		t.Errorf("a Newbie invites: %v", r)
	}
	bob.send("change positions", func(ctx context.Context) error {
		return bob.GuildPositions(ctx, GuildPosition(1, "Recruiter", true, false, false, 0))
	})
	if r := bob.GuildResult(); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_NOT_MASTER {
		t.Errorf("a member changes positions: %v", r)
	}

	info := alice.SetPositions(GuildPosition(1, "Recruiter", true, false, false, 0))
	if p := info.GetPositions()[0]; p.GetName() != "GuildMaster" || !p.GetCanInvite() || !p.GetCanExpel() || !p.GetCanUseStorage() {
		t.Errorf("the master's position is %v", p)
	}
	alice.SetMemberPosition("Bob", 1)
	bob.GuildInfo(func(i *wathenav1.GuildInfo) bool { return i.GetPosition() == 1 })
	if b := dave.Expect("Bob's new position", func(e *wathenav1.Event) bool {
		g := e.GetEntityGuild()
		return g.GetId() == bobID && g.GetGuild().GetPosition() == "Recruiter"
	}).GetEntityGuild().GetGuild(); b.GetName() != "Crew" {
		t.Errorf("Dave sees Bob's badge %v", b)
	}

	if r := bob.InviteToGuild("Carol"); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_INVITE_SENT {
		t.Fatalf("the Recruiter invites: %v", r)
	}
	carol.AcceptGuild(carol.GuildInvited("Bob"))
	bob.GuildOutcome(wathenav1.GuildOutcome_GUILD_OUTCOME_ACCEPTED, "Carol")
	alice.GuildOutcome(wathenav1.GuildOutcome_GUILD_OUTCOME_JOINED, "Carol")
	alice.Until("three members", func(_ View, g Game) bool { return g.Guild != nil && len(g.Guild.Members) == 3 })
	if m, ok := alice.Game().Guild.Member("Carol"); !ok || m.Position != int(NewbiePosition) {
		t.Errorf("Carol in the guild: %+v", m)
	}
	srv.CheckLogs()
}

// TestGuildChat: a guild line reaches a member on another map, and not a
// player outside the guild on the speaker's map.
func TestGuildChat(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	carol.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Alice")
	MakeGuild("Crew", alice, bob)
	bob.Talk("Guide").Pick("Go to the field")
	bob.MapChange(FieldMap)
	alice.Until("Bob on the field in the guild window", func(_ View, g Game) bool {
		m, ok := g.Guild.Member("Bob")
		return ok && m.Map == FieldMap
	})

	line := alice.SayToGuild("meet me at the field")
	if len(line.GetLineId()) != 16 {
		t.Errorf("line %v", line)
	}
	bob.HearsGuild("Alice", "meet me at the field")
	bob.SayToGuild("on my way")
	alice.HearsGuild("Bob", "on my way")
	// Carol hears Alice's next local line, and no guild line before it.
	alice.Say("hello Carol")
	carol.Hears("Alice", "hello Carol")
	if e := carol.Seen(func(e *wathenav1.Event) bool {
		return e.GetChat().GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_GUILD
	}); e != nil {
		t.Errorf("Carol heard %v", e)
	}
	carol.send("guild chat", func(ctx context.Context) error { return carol.GuildChat(ctx, "anyone?") })
	if r := carol.Expect("a refused line", (*wathenav1.Event).HasChatRefused).GetChatRefused(); r.GetReason() != wathenav1.ChatRefusal_CHAT_REFUSAL_NOT_IN_CHANNEL {
		t.Errorf("refused %v", r)
	}
	srv.CheckLogs()
}

// TestGuildExpTax: the master sets an exp tax of 50% on new members.
// A member kills a Dummy, worth 10 base exp: the guild gets the tax,
// the member's paid exp shows it, and the member keeps the rest.
func TestGuildExpTax(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	MakeGuild("Crew", alice, bob)
	alice.SetPositions(GuildPosition(NewbiePosition, "Newbie", false, false, false, 50))
	bob.GuildInfo(func(i *wathenav1.GuildInfo) bool {
		return i.GetPositions()[NewbiePosition].GetExpTax() == 50
	})

	baseExp, jobExp := wathenav1.StatId_STAT_ID_BASE_EXP, wathenav1.StatId_STAT_ID_JOB_EXP
	base, job := bob.Stat(baseExp), bob.Stat(jobExp)
	guildExp := bob.Game().Guild.Exp
	bob.Kill(bob.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy"))
	info := bob.GuildInfo(func(i *wathenav1.GuildInfo) bool { return i.GetExp() > guildExp })
	bob.Until("the job exp of the kill", func(_ View, g Game) bool { return g.Stats[jobExp] >= job+10 })
	bob.Until("the base exp of the kill", func(_ View, g Game) bool { return g.Stats[baseExp] > base })
	// Stats come once a frame; give a later one the time to come.
	time.Sleep(200 * time.Millisecond)
	paid := info.GetExp() - guildExp
	kept := bob.Stat(baseExp) - base
	var memberPaid int64
	for _, m := range info.GetMembers() {
		if m.GetName() == "Bob" {
			memberPaid = m.GetExp()
		}
	}
	t.Logf("Bob kept %d base exp and paid the guild %d", kept, paid)
	if paid != 5 || kept != 5 || memberPaid != paid || bob.Stat(jobExp)-job != 10 {
		t.Errorf("Bob kept %d base exp and %d job exp, paid %d, and the guild shows %d paid; want 5, 10, 5 and 5",
			kept, bob.Stat(jobExp)-job, paid, memberPaid)
	}
	alice.GuildInfo(func(i *wathenav1.GuildInfo) bool { return i.GetExp() == info.GetExp() })
	srv.CheckLogs()
}

// TestGuildLevelAndSkill: the fixture gives the guild the exp of a
// level. The members see LEVEL_UP and a skill point. A skill whose
// requirement is missing is refused; the master raises Approval, and the
// members see its level.
func TestGuildLevelAndSkill(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	MakeGuild("Crew", alice, bob)
	if info := alice.Game().Guild; info.Level != 1 || info.SkillPoints != 0 || info.NextExp != 100000 {
		t.Fatalf("new guild %+v", info)
	}

	alice.Say("!guild level 2")
	for _, p := range []*Player{alice, bob} {
		p.GuildOutcome(wathenav1.GuildOutcome_GUILD_OUTCOME_LEVEL_UP, "Crew")
		info := p.GuildInfo(func(i *wathenav1.GuildInfo) bool { return i.GetLevel() == 2 })
		if info.GetSkillPoints() != 1 || info.GetExp() != 0 || info.GetNextExp() != 400000 {
			t.Errorf("%s sees %v", p.Name, info)
		}
	}

	alice.send("raise Kafra Contract", func(ctx context.Context) error { return alice.GuildSkillUp(ctx, 10001) })
	if r := alice.GuildResult(); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_SKILL_REFUSED || r.GetName() != "10001" {
		t.Errorf("raising Kafra Contract without Approval: %v", r)
	}
	bob.send("raise Approval", func(ctx context.Context) error { return bob.GuildSkillUp(ctx, SkillApproval) })
	if r := bob.GuildResult(); r.GetOutcome() != wathenav1.GuildOutcome_GUILD_OUTCOME_NOT_MASTER {
		t.Errorf("a member raises a skill: %v", r)
	}
	info := alice.RaiseGuildSkill(SkillApproval)
	if info.GetSkillPoints() != 0 {
		t.Errorf("after raising Approval: %v", info)
	}
	for _, s := range info.GetSkills() {
		if s.GetCanRaise() {
			t.Errorf("skill %v can be raised without points", s)
		}
	}
	bob.GuildInfo(func(i *wathenav1.GuildInfo) bool { return guildSkill(i, SkillApproval) == 1 })
	srv.CheckLogs()
}

// TestGuildEmergencyCall: with the ruleset mod that allows Emergency
// Call outside GvG maps, the master raises it at guild level 11 and
// casts it. A member on the other map comes to a cell next to the
// master.
func TestGuildEmergencyCall(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems, Ruleset: GuildRuleset})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	MakeGuild("Crew", alice, bob)
	alice.LevelGuild(11)
	for _, s := range EmergencyCallSkills {
		alice.RaiseGuildSkill(s)
	}
	bob.Talk("Guide").Pick("Go to the field")
	bob.MapChange(FieldMap)
	alice.Skip()
	bob.Skip()

	// Emergency Call has a cast time.
	alice.Timeout = 30 * time.Second
	alice.CastOnSelf(SkillEmergencyCall, 1)
	m := bob.MapChange(StartMap)
	v := alice.View()
	if dx, dy := int(m.GetX())-v.X, int(m.GetY())-v.Y; max(abs(dx), abs(dy)) > 1 {
		t.Errorf("Bob arrived at %d,%d, Alice stands at %d,%d", m.GetX(), m.GetY(), v.X, v.Y)
	}
	alice.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Bob")
	srv.CheckLogs()
}

// TestGuildEmergencyCallRefused: with the core ruleset, Emergency Call
// works on GvG maps only, and no map is one yet.
func TestGuildEmergencyCallRefused(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	MakeGuild("Crew", alice, bob)
	alice.LevelGuild(11)
	for _, s := range EmergencyCallSkills {
		alice.RaiseGuildSkill(s)
	}
	alice.send("cast", func(ctx context.Context) error { return alice.UseSkill(ctx, SkillEmergencyCall, 1, alice.Self()) })
	f := alice.Expect("Emergency Call to fail", func(e *wathenav1.Event) bool {
		return e.GetSkillFailed().GetSkillId() == SkillEmergencyCall
	}).GetSkillFailed()
	if f.GetReason() != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Errorf("Emergency Call failed with %s", f.GetReason())
	}
	srv.CheckLogs()
}

// emblemImage is a square image of a size with a magenta corner, which
// the server keeps as transparent.
func emblemImage(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 10), G: 80, B: uint8(y * 10), A: 255}) // #nosec G115 -- small sizes.
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, B: 255, A: 255})
	return img
}

// TestGuildEmblem: an emblem uploaded as a 24 x 24 BMP reaches the
// members in GuildInfo and the players in sight as EntityGuild, with
// version 1. The game server serves it as a PNG any origin may draw. A
// 32 x 32 image is refused.
func TestGuildEmblem(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	aliceID := carol.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Alice").ID
	id := MakeGuild("Crew", alice, bob)

	var img bytes.Buffer
	if err := bmp.Encode(&img, emblemImage(24)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	// A frame shows what changed in sight before the player's own guild
	// events, so Bob's EntityGuild comes before his GuildInfo.
	marks := map[*Player]int{bob: bob.Mark(), carol: carol.Mark()}
	v, err := srv.UploadEmblem(ctx, "Crew", img.Bytes())
	if err != nil || v != 1 {
		t.Fatalf("upload: version %d, %v", v, err)
	}
	for _, p := range []*Player{alice, bob} {
		p.GuildOutcome(wathenav1.GuildOutcome_GUILD_OUTCOME_EMBLEM_CHANGED, "Crew")
		p.GuildInfo(func(i *wathenav1.GuildInfo) bool { return i.GetEmblemVersion() == 1 })
	}
	for _, p := range []*Player{bob, carol} {
		p.mark = marks[p]
		b := p.Expect("Alice's emblem", func(e *wathenav1.Event) bool {
			g := e.GetEntityGuild()
			return g.GetId() == aliceID && g.GetGuild().GetEmblemVersion() == 1
		}).GetEntityGuild().GetGuild()
		if b.GetGuildId() != id || b.GetName() != "Crew" || b.GetPosition() != "GuildMaster" {
			t.Errorf("%s sees Alice's badge %v", p.Name, b)
		}
	}
	alice.Until("her own badge", func(v View, _ Game) bool { return v.Guild.GetEmblemVersion() == 1 })

	get := func(version int) *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/emblems/%d/%d.png", srv.HTTP(), id, version), nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req) // #nosec G704 -- the test server.
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp := get(1)
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" || resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("GET the emblem: %s, headers %v", resp.Status, resp.Header)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the emblem is not a PNG: %v", err)
	}
	if b := decoded.Bounds(); b.Dx() != 24 || b.Dy() != 24 {
		t.Errorf("the emblem is %v", b)
	}
	if _, _, _, a := decoded.At(0, 0).RGBA(); a != 0 {
		t.Error("the magenta corner is not transparent")
	}
	if resp := get(2); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET version 2: %s", resp.Status)
	} else {
		_ = resp.Body.Close()
	}

	img.Reset()
	if err := png.Encode(&img, emblemImage(32)); err != nil {
		t.Fatal(err)
	}
	_, err = srv.UploadEmblem(ctx, "Crew", img.Bytes())
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != http.StatusBadRequest {
		t.Errorf("a 32 x 32 emblem: %v", err)
	}
	if g := alice.Game().Guild; g.Emblem != 1 {
		t.Errorf("after the refused upload, the guild shows emblem %d", g.Emblem)
	}
	srv.CheckLogs()
}

// TestGuildStorage: at guild level 21 with the skills up to Guild
// Storage, the master opens the storage, puts Jellopy in and takes some
// out. While it is open, a member gets STORAGE_OUTCOME_IN_USE, and a
// member whose position does not allow storage gets
// STORAGE_OUTCOME_NO_PERMISSION.
func TestGuildStorage(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	MakeGuild("Crew", alice, bob)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	if o, err := srv.OpenGuildStorage(ctx, "Alice"); err != nil || o != wathenav1.StorageOutcome_STORAGE_OUTCOME_NO_STORAGE {
		t.Errorf("opening before Guild Storage: %s, %v", o, err)
	}
	alice.LevelGuild(21)
	for _, s := range GuildStorageSkills {
		alice.RaiseGuildSkill(s)
	}
	if o, err := srv.OpenGuildStorage(ctx, "Bob"); err != nil || o != wathenav1.StorageOutcome_STORAGE_OUTCOME_NO_PERMISSION {
		t.Errorf("a Newbie opens: %s, %v", o, err)
	}

	open := alice.OpenGuildStorage(srv)
	if open.GetKind() != wathenav1.StorageKind_STORAGE_KIND_GUILD || open.GetSize() != 200 || len(open.GetItems()) != 0 {
		t.Errorf("the storage %v", open)
	}
	add := alice.StorePut(Jellopy, 8)
	if add.GetAmount() != 8 || add.GetItem().GetAmount() != 8 {
		t.Errorf("stored %v", add)
	}
	alice.StoreTake(Jellopy, 3)
	if alice.Count(Jellopy) != 15 || alice.Game().Storage.Count(Jellopy) != 5 {
		t.Errorf("Alice holds %d Jellopy and sees %d stored", alice.Count(Jellopy), alice.Game().Storage.Count(Jellopy))
	}

	alice.SetPositions(GuildPosition(NewbiePosition, "Newbie", false, false, true, 0))
	if o, err := srv.OpenGuildStorage(ctx, "Bob"); err != nil || o != wathenav1.StorageOutcome_STORAGE_OUTCOME_IN_USE {
		t.Errorf("Bob opens while Alice has it open: %s, %v", o, err)
	}
	bob.Expect("the storage in use", func(e *wathenav1.Event) bool {
		return e.GetStorageResult().GetOutcome() == wathenav1.StorageOutcome_STORAGE_OUTCOME_IN_USE
	})
	alice.CloseStorage()
	if items := bob.OpenGuildStorage(srv).GetItems(); len(items) != 1 || items[0].GetItemId() != Jellopy || items[0].GetAmount() != 5 {
		t.Errorf("Bob sees %v", items)
	}

	zones, err := srv.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	totals, twice := TotalsOf(zones)
	if len(twice) > 0 || totals.Stored[Jellopy] != 5 || totals.Items[Jellopy] != 40 {
		t.Errorf("the audit holds %v, %v stored, serials held twice %v", totals.Items, totals.Stored, twice)
	}
	srv.CheckLogs()
}

// TestGuildInTheDatabase: a guild's members, positions, notice, skills,
// emblem and storage come back after the server restarts.
func TestGuildInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: guildItems, Database: true})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	id := MakeGuild("Crew", alice, bob)
	alice.LevelGuild(21)
	for _, s := range GuildStorageSkills {
		alice.RaiseGuildSkill(s)
	}
	officer := GuildPosition(1, "Officer", true, false, true, 10)
	alice.SetPositions(officer)
	alice.SetMemberPosition("Bob", 1)
	alice.SetGuildNotice("Welcome", "Meet at the field")
	var img bytes.Buffer
	if err := png.Encode(&img, emblemImage(24)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if v, err := srv.UploadEmblem(ctx, "Crew", img.Bytes()); err != nil || v != 1 {
		t.Fatalf("upload: version %d, %v", v, err)
	}
	alice.GuildInfo(func(i *wathenav1.GuildInfo) bool { return i.GetEmblemVersion() == 1 })
	alice.OpenGuildStorage(srv)
	alice.StorePut(Jellopy, 7)
	alice.CloseStorage()
	_ = alice.Close()
	_ = bob.Close()

	srv.Restart()
	alice = srv.Join(t, "Alice")
	info := alice.GuildInfo(func(i *wathenav1.GuildInfo) bool { return i.GetId() != 0 })
	pos := info.GetPositions()
	switch {
	case info.GetId() != id || info.GetName() != "Crew" || info.GetMaster() != "Alice" || !slices.Equal(memberNames(info), []string{"Alice", "Bob"}):
		t.Errorf("after the restart: %v", info)
	case info.GetLevel() != 21 || info.GetSkillPoints() != 0 || guildSkill(info, SkillGuildStorage) != 1 || guildSkill(info, SkillExtension) != 10:
		t.Errorf("after the restart: level %d, %d points, skills %v", info.GetLevel(), info.GetSkillPoints(), info.GetSkills())
	case len(pos) != 20 || pos[1].GetName() != "Officer" || !pos[1].GetCanInvite() || !pos[1].GetCanUseStorage() || pos[1].GetExpTax() != 10:
		t.Errorf("after the restart: positions %v", pos)
	case info.GetNotice().GetTitle() != "Welcome" || info.GetNotice().GetText() != "Meet at the field" || info.GetEmblemVersion() != 1:
		t.Errorf("after the restart: notice %v, emblem %d", info.GetNotice(), info.GetEmblemVersion())
	}
	if m := info.GetMembers(); len(m) == 2 && (!m[0].GetOnline() || !m[0].GetMaster() || m[1].GetOnline() || m[1].GetPosition() != 1) {
		t.Errorf("after the restart: members %v", m)
	}
	if items := alice.OpenGuildStorage(srv).GetItems(); len(items) != 1 || items[0].GetItemId() != Jellopy || items[0].GetAmount() != 7 {
		t.Errorf("after the restart, the storage holds %v", items)
	}
	alice.CloseStorage()
	bob = srv.Join(t, "Bob")
	if info := bob.GuildInfo(func(i *wathenav1.GuildInfo) bool { return i.GetId() == id }); info.GetPosition() != 1 {
		t.Errorf("Bob's position after the restart: %d", info.GetPosition())
	}
	if b := alice.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Bob").Guild; b.GetPosition() != "Officer" || b.GetEmblemVersion() != 1 {
		t.Errorf("Alice sees Bob's badge %v", b)
	}

	// The restart can take longer than the upload's context.
	qctx, qcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer qcancel()
	var stored, members int
	for q, n := range map[string]*int{
		`SELECT coalesce(sum(amount), 0) FROM guild_storage WHERE item_id = 909`: &stored,
		`SELECT count(*) FROM guild_members`:                                     &members,
	} {
		if err := srv.DB.QueryRow(qctx, q).Scan(n); err != nil {
			t.Fatal(err)
		}
	}
	if stored != 7 || members != 2 {
		t.Errorf("%d Jellopy in guild_storage and %d guild members saved; want 7 and 2", stored, members)
	}
	srv.CheckLogs()
}
