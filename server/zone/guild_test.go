package zone

import (
	"fmt"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// formGuild puts players in one guild of the zone's social registry, the
// first as its master, and gives each the guild view, as the registry's
// notices would. It returns the guild's ID.
func formGuild(t testing.TB, z *Zone, players ...*Player) int64 {
	t.Helper()
	reg := z.socialHub()
	for _, p := range players {
		if p.soc == nil {
			e := p.entity
			p.soc = reg.Online(social.Login{Name: p.Name, Map: z.Map, X: e.x, Y: e.y, HP: e.hp, Level: p.progress.BaseLevel, Job: p.char.Job})
		}
	}
	id, o := reg.GuildCreate(players[0].soc, fmt.Sprintf("Guild of %s", players[0].Name))
	if id == 0 {
		t.Fatalf("formGuild: %v", o)
	}
	for _, p := range players[1:] {
		if ok, err := reg.GuildAddMember(id, p.soc); !ok || err != nil {
			t.Fatalf("formGuild: %s did not join: %v", p.Name, err)
		}
	}
	refreshGuild(z, id)
	return id
}

// refreshGuild gives every player of the zone in a guild its current
// view and badge.
func refreshGuild(z *Zone, id int64) {
	reg := z.socialHub()
	v := reg.GuildViewByID(id)
	for _, p := range z.players {
		if p.soc != nil && reg.GuildIDOf(p.soc) == id {
			p.guild.view = v
			z.updateBadge(p)
		}
	}
}

// learnGuildSkills gives a guild exp for its skill points and raises the
// skills in order.
func learnGuildSkills(t *testing.T, z *Zone, master *Player, skills ...int) {
	t.Helper()
	reg := z.socialHub()
	id := reg.GuildIDOf(master.soc)
	if _, err := reg.GiveExp(id, social.MaxGuildExp); err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if o := reg.GuildSkillUp(master.soc, s); o != social.GuildSkillRaised {
			t.Fatalf("skill %d: %v", s, o)
		}
	}
	refreshGuild(z, id)
}

func guildOutcomes(evs []*wathenav1.Event) []wathenav1.GuildOutcome {
	var out []wathenav1.GuildOutcome
	for _, e := range evs {
		if e.HasGuildResult() {
			out = append(out, e.GetGuildResult().GetOutcome())
		}
	}
	return out
}

// guildItems are the test items and the Emperium.
func guildItems() *item.Table {
	defs := []item.Def{
		{ID: 501, AegisName: "Red_Potion", Type: "Healing", Weight: 70, Stackable: true},
		{ID: 714, AegisName: "Emperium", Type: "Etc", Weight: 200, Stackable: true},
		{ID: 909, AegisName: "Jellopy", Type: "Etc", Weight: 10, Stackable: true},
		{ID: 1201, AegisName: "Knife", Type: "Weapon", Weight: 400},
	}
	return item.New(&item.Data{Mode: "renewal", Items: defs})
}

func TestGuildAcrossZones(t *testing.T) {
	w := newSocialWorld(nil, "map/a", "map/b")
	for _, z := range w.zones {
		z.Items = guildItems()
	}
	alice := w.join(w.zones[0], "Alice")
	bob := w.join(w.zones[1], "Bob")
	carol := w.join(w.zones[0], "Carol")
	w.step(50)
	// No Emperium: guild_created 3.
	w.zones[0].handle(command{player: alice, seq: 1, guild: &guildCommand{create: "Knights"}})
	w.step(100)
	if got := guildOutcomes(events(alice)); !slices.Equal(got, []wathenav1.GuildOutcome{wathenav1.GuildOutcome_GUILD_OUTCOME_NO_EMPERIUM}) {
		t.Fatalf("without an Emperium %v", got)
	}
	alice.maxWeight = maxWeight
	w.zones[0].addItem(alice, itemEmperium, 2, scriptOrigin)
	w.zones[0].handle(command{player: alice, seq: 2, guild: &guildCommand{create: "Knights"}})
	w.zones[0].handle(command{player: alice, seq: 3, guild: &guildCommand{invite: "bob"}})
	w.step(150)
	if n := alice.inventory.Count(itemEmperium); n != 1 {
		t.Errorf("Emperiums left %d, want 1", n)
	}
	ev := events(alice)
	if got := guildOutcomes(ev); !slices.Contains(got, wathenav1.GuildOutcome_GUILD_OUTCOME_CREATED) {
		t.Fatalf("Alice's outcomes %v", got)
	}
	inv := eventsOf(bob, (*wathenav1.Event).GetGuildInvite, (*wathenav1.Event).HasGuildInvite)
	if len(inv) != 1 || inv[0].GetFrom() != "Alice" || inv[0].GetGuildName() != "Knights" {
		t.Fatalf("Bob's invites %v", inv)
	}
	w.zones[1].handle(command{player: bob, seq: 1, guild: &guildCommand{reply: &guildReply{guild: int64(inv[0].GetGuildId()), accept: true}}})
	w.step(200)
	var last *wathenav1.GuildInfo
	for _, e := range events(alice) {
		if e.HasGuildInfo() {
			last = e.GetGuildInfo()
		}
	}
	if last == nil || len(last.GetMembers()) != 2 || last.GetMembers()[1].GetName() != "Bob" || last.GetMembers()[1].GetMap() != "map/b" ||
		!last.GetMembers()[0].GetMaster() || last.GetMaster() != "Alice" || last.GetPosition() != 0 {
		t.Fatalf("Alice's guild %v", last)
	}
	if alice.guild.badge.GetName() != "Knights" || alice.guild.badge.GetPosition() != "GuildMaster" ||
		bob.guild.badge.GetPosition() != "Newbie" {
		t.Errorf("badges %v %v", alice.guild.badge, bob.guild.badge)
	}

	// Guild chat reaches Bob on the other map, and not Carol.
	events(bob)
	w.zones[0].handle(command{player: alice, seq: 4, chat: &chatCommand{kind: chatGuild, text: "to the castle"}})
	w.step(250)
	got := chats(bob)
	if len(got) != 1 || got[0].GetChannel() != wathenav1.ChatChannel_CHAT_CHANNEL_GUILD || got[0].GetFromName() != "Alice" {
		t.Fatalf("Bob hears %v", got)
	}
	if got := chats(carol); len(got) != 0 {
		t.Errorf("Carol hears %v", got)
	}
	if lines := w.log.Lines(); len(lines) != 1 || lines[0].Kind != "guild" || lines[0].To != "Knights" {
		t.Errorf("log %+v", lines)
	}
	w.zones[0].handle(command{player: carol, seq: 1, chat: &chatCommand{kind: chatGuild, text: "hello?"}})
	w.step(300)
	if r := eventsOf(carol, (*wathenav1.Event).GetChatRefused, (*wathenav1.Event).HasChatRefused); len(r) != 1 {
		t.Errorf("Carol's refusals %v", r)
	}

	// Carol sees Alice's badge, and its change when Alice's position is
	// renamed.
	ap := w.zones[0].appear(1, alice.entity).GetAppear()
	if ap.GetGuild().GetName() != "Knights" {
		t.Errorf("appear %v", ap.GetGuild())
	}
	events(carol)
	w.zones[0].handle(command{player: alice, seq: 5, guild: &guildCommand{positions: []social.PositionChange{{Index: 0, Position: social.Position{Name: "King", Invite: true}}}}})
	w.step(350)
	eg := eventsOf(carol, (*wathenav1.Event).GetEntityGuild, (*wathenav1.Event).HasEntityGuild)
	if len(eg) != 1 || eg[0].GetGuild().GetPosition() != "King" {
		t.Errorf("Carol sees %v", eg)
	}
	// The master hands the guild to Bob on the other map.
	w.zones[0].handle(command{player: alice, seq: 6, guild: &guildCommand{memberPos: &guildMemberPos{name: "Bob", position: 0}}})
	w.step(400)
	if !bob.isGuildMaster() || alice.isGuildMaster() {
		t.Errorf("master: Bob %v Alice %v", bob.isGuildMaster(), alice.isGuildMaster())
	}
}

func TestGuildExpTax(t *testing.T) {
	z := saveZone()
	a := addCharacter(z, savedCharacter(), &saves{})
	c := savedCharacter()
	c.ID, c.Name = 8, "Member"
	b := addCharacter(z, c, &saves{})
	z.stepAt(50)
	id := formGuild(t, z, a, b)
	reg := z.socialHub()
	reg.GuildSetPositions(a.soc, []social.PositionChange{{Index: 19, Position: social.Position{Name: "Newbie", ExpTax: 10}}})
	refreshGuild(z, id)
	before := b.progress
	z.gainExp(b, 25, 3)
	in, _ := reg.Guild(id)
	// guild_payexp: 25 * 10 / 100 = 2 to the guild, 23 to the member.
	if in.Exp != 2 {
		t.Errorf("guild exp %d, want 2", in.Exp)
	}
	gained := b.progress.BaseExp - before.BaseExp
	if b.progress.BaseLevel == before.BaseLevel && gained != 23 {
		t.Errorf("Bob gained %d base exp, want 23", gained)
	}
	if m, _ := reg.GuildViewOf(a.soc).Member("Member"); m.Exp != 2 {
		t.Errorf("Bob gave %d", m.Exp)
	}
	// The master's position has no tax.
	z.gainExp(a, 25, 0)
	if in, _ := reg.Guild(id); in.Exp != 2 {
		t.Errorf("guild exp %d after the master's gain", in.Exp)
	}
}

func TestGuildStorageMoves(t *testing.T) {
	z := saveZone()
	z.Items = guildItems()
	cm := &commits{}
	z.Committer = cm
	ac := savedCharacter()
	ac.Items = []character.Item{{Slot: 0, ID: 909, Amount: 10, Serial: item.NewSerial()}}
	a := addCharacter(z, ac, &saves{})
	bc := savedCharacter()
	bc.ID, bc.Name = 8, "Member"
	b := addCharacter(z, bc, &saves{})
	z.stepAt(50)
	a.maxWeight, b.maxWeight = maxWeight, maxWeight
	reg := z.socialHub()
	reg.SetGuildData(social.CoreGuildTables(false), rules.DefaultGuild())
	id := formGuild(t, z, a, b)
	events(a)
	if o := z.openGuildStorage(a); o != social.StorageOpened {
		t.Fatalf("open: %v", o)
	}
	z.stepAt(100)
	so := eventsOf(a, (*wathenav1.Event).GetStorageOpen, (*wathenav1.Event).HasStorageOpen)
	if len(so) != 1 || so[0].GetSize() != 600 || len(so[0].GetItems()) != 0 {
		t.Fatalf("open %v", so)
	}
	// Another member waits; the Newbie position has no storage right.
	if o := z.openGuildStorage(b); o != social.StorageNoPermission {
		t.Errorf("Newbie: %v", o)
	}
	reg.GuildSetPositions(a.soc, []social.PositionChange{{Index: 19, Position: social.Position{Name: "Newbie", Storage: true}}})
	refreshGuild(z, id)
	if o := z.openGuildStorage(b); o != social.StorageInUse {
		t.Errorf("in use: %v", o)
	}
	z.handle(command{player: a, seq: 1, storage: &storageCommand{move: &storageMove{kind: MovePut, slot: 0, amount: 4}}})
	z.commitTransfers()
	z.stepAt(150)
	ev := events(a)
	adds := eventsOf2(ev, (*wathenav1.Event).GetStorageAdd, (*wathenav1.Event).HasStorageAdd)
	if len(adds) != 1 || adds[0].GetItem().GetItemId() != 909 || adds[0].GetItem().GetAmount() != 4 || adds[0].GetAmount() != 4 {
		t.Fatalf("storage adds %v", adds)
	}
	if a.inventory.Count(909) != 6 {
		t.Errorf("inventory %d", a.inventory.Count(909))
	}
	last := cm.got[len(cm.got)-1]
	if len(last.storages) != 1 || last.storages[0].Guild != id || len(last.storages[0].Cur) != 1 || last.storages[0].Cur[0].Amount != 4 ||
		len(last.logs.Items) != 1 || last.logs.Items[0].To.Kind.String() != "guild_storage" || len(last.saves) != 1 {
		t.Fatalf("commit %+v", last)
	}
	// Taking back 1 splits the stack, in the same way.
	z.handle(command{player: a, seq: 2, storage: &storageCommand{move: &storageMove{kind: MoveTake, slot: 0, amount: 1}}})
	z.commitTransfers()
	z.stepAt(200)
	ev = events(a)
	rm := eventsOf2(ev, (*wathenav1.Event).GetStorageRemove, (*wathenav1.Event).HasStorageRemove)
	if len(rm) != 1 || rm[0].GetAmount() != 1 || a.inventory.Count(909) != 7 {
		t.Fatalf("take %v, inventory %d", rm, a.inventory.Count(909))
	}
	last = cm.got[len(cm.got)-1]
	if len(last.storages) != 1 || len(last.storages[0].Base) != 1 || last.storages[0].Base[0].Amount != 4 || last.storages[0].Cur[0].Amount != 3 {
		t.Errorf("second commit %+v", last.storages)
	}
	// The log, newest first.
	z.handle(command{player: a, seq: 3, storage: &storageCommand{log: true}})
	z.stepAt(250)
	logs := eventsOf(a, (*wathenav1.Event).GetGuildStorageLog, (*wathenav1.Event).HasGuildStorageLog)
	if len(logs) != 1 || len(logs[0].GetEntries()) != 2 || logs[0].GetEntries()[0].GetAmount() != -1 || logs[0].GetEntries()[1].GetAmount() != 4 {
		t.Fatalf("log %v", logs)
	}
	// Closing lets the other member open it.
	z.handle(command{player: a, seq: 4, storage: &storageCommand{close: true}})
	if o := z.openGuildStorage(b); o != social.StorageOpened {
		t.Errorf("after the close: %v", o)
	}
	// An expelled member's storage closes.
	reg.GuildExpel(a.soc, "Member", "")
	z.drainSocial()
	if b.guild.storage != nil {
		t.Error("the expelled member still has the storage open")
	}
}

func eventsOf2[T any](evs []*wathenav1.Event, get func(*wathenav1.Event) T, has func(*wathenav1.Event) bool) []T {
	var out []T
	for _, e := range evs {
		if has(e) {
			out = append(out, get(e))
		}
	}
	return out
}

func TestGuildBattleOrders(t *testing.T) {
	z, a, _ := skillZone(t, 0, nil)
	a.Name = "Master"
	b := add(z, "b")
	c := add(z, "c")
	far := add(z, "far")
	z.stepAt(100)
	b.entity.x, b.entity.y = a.entity.x+3, a.entity.y
	c.entity.x, c.entity.y = a.entity.x, a.entity.y+3
	far.entity.x, far.entity.y = a.entity.x-16, a.entity.y
	formGuild(t, z, a, b, far)
	learnGuildSkills(t, z, a, social.SkillApproval, social.SkillExtension, social.SkillExtension, social.SkillBattleOrder)
	if a.skillLevel(social.SkillBattleOrder) != 1 || b.skillLevel(social.SkillBattleOrder) != 0 {
		t.Fatalf("levels %d %d", a.skillLevel(social.SkillBattleOrder), b.skillLevel(social.SkillBattleOrder))
	}
	// Not a GvG map: skill_check_condition_castbegin fails.
	cast(z, a, social.SkillBattleOrder, 1, a.entity)
	ev := run(z, a, 150, 300)
	if f := eventsOf2(ev, (*wathenav1.Event).GetSkillFailed, (*wathenav1.Event).HasSkillFailed); len(f) != 1 {
		t.Fatalf("failures %v", f)
	}
	z.GvG = true
	cast(z, a, social.SkillBattleOrder, 1, a.entity)
	run(z, a, 350, 2000)
	for _, p := range []*Player{a, b} {
		if !p.entity.sc.Has(sc.SCBattleOrders) {
			t.Errorf("%s has no Battle Orders", p.Name)
		}
	}
	for _, p := range []*Player{c, far} {
		if p.entity.sc.Has(sc.SCBattleOrders) {
			t.Errorf("%s has Battle Orders", p.Name)
		}
	}
	// Renewal's own cooldown: 3 minutes.
	if until := a.cooldowns[social.SkillBattleOrder]; until < z.now+170000 {
		t.Errorf("cooldown until %d, now %d", until, z.now)
	}
}

func TestGuildEmergencyCall(t *testing.T) {
	z, a, _ := skillZone(t, 0, nil)
	a.Name = "Master"
	c := savedCharacter()
	c.ID, c.Name, c.X, c.Y = 8, "Called", 2, 2
	b := addCharacter(z, c, &saves{})
	z.stepAt(100)
	formGuild(t, z, a, b)
	learnGuildSkills(t, z, a, social.SkillApproval, social.SkillGuardResearch, social.SkillExtension, social.SkillExtension,
		social.SkillExtension, social.SkillExtension, social.SkillExtension, social.SkillBattleOrder, social.SkillRegeneration,
		social.SkillEmergencyCall)
	// emergency_call 11 needs a GvG map.
	cast(z, a, social.SkillEmergencyCall, 1, a.entity)
	ev := run(z, a, 150, 300)
	if f := eventsOf2(ev, (*wathenav1.Event).GetSkillFailed, (*wathenav1.Event).HasSkillFailed); len(f) != 1 {
		t.Fatalf("failures %v", f)
	}
	r, err := rules.GuildOf(map[string]int{"emergency_call": 15})
	if err != nil {
		t.Fatal(err)
	}
	world := *rules.Of(true)
	world.Guild = r
	z.Mods.Rules = &world
	cast(z, a, social.SkillEmergencyCall, 1, a.entity)
	ev = run(z, a, 350, 9000)
	if f := eventsOf2(ev, (*wathenav1.Event).GetSkillFailed, (*wathenav1.Event).HasSkillFailed); len(f) != 0 {
		t.Fatalf("failures with emergency_call 15: %v", f)
	}
	z.drainSocial()
	z.stepAt(9050)
	// Slot 1 takes the second offset: one cell east of the master.
	if b.entity.x != a.entity.x+1 || b.entity.y != a.entity.y {
		t.Errorf("called to %d,%d; the master is at %d,%d", b.entity.x, b.entity.y, a.entity.x, a.entity.y)
	}
}

// TestGuildOnlySkillReachesGuild checks battle_check_target's guild
// state: a GuildOnly skill reaches a guildmate who is in no party, and
// not a stranger.
func TestGuildOnlySkillReachesGuild(t *testing.T) {
	z, a, _ := skillZone(t, 0, nil)
	b := add(z, "b")
	c := add(z, "c")
	z.stepAt(100)
	formGuild(t, z, a, b)
	if !checkTarget(a.entity, b.entity, bctGuild) || checkTarget(a.entity, c.entity, bctGuild) {
		t.Error("BCT_GUILD")
	}
	if !checkTarget(a.entity, b.entity, bctSameGuild) {
		t.Error("BCT_SAMEGUILD")
	}
}

func TestGuildEmperiumCheckOff(t *testing.T) {
	w := newSocialWorld(nil, "map/a")
	z := w.zones[0]
	z.Items = guildItems()
	g, err := rules.GuildOf(map[string]int{"guild_emperium_check": 0})
	if err != nil {
		t.Fatal(err)
	}
	world := *rules.Of(true)
	world.Guild = g
	z.Mods = &Mods{Rules: &world}
	alice := w.join(z, "Alice")
	w.step(50)
	z.handle(command{player: alice, seq: 1, guild: &guildCommand{create: "Free"}})
	w.step(100)
	if got := guildOutcomes(events(alice)); !slices.Contains(got, wathenav1.GuildOutcome_GUILD_OUTCOME_CREATED) {
		t.Errorf("outcomes %v", got)
	}
}

// TestGuildSkillRelogDelay checks guild_skill_relog_delay: in
// pre-renewal (the skill_cooldown flag off) a master who logs in waits
// before using guild skills; in renewal it does not. The recording's
// member_joined_connect_new case of each mode says the same.
func TestGuildSkillRelogDelay(t *testing.T) {
	for _, renewal := range []bool{true, false} {
		mode := reference.PreRenewal
		if renewal {
			mode = reference.Renewal
		}
		var ref struct {
			Relog []struct {
				Name   string           `json:"name"`
				Blocks map[string]int64 `json:"blocks"`
			} `json:"relog_cases"`
		}
		if err := reference.Load(mode, "guild", &ref); err != nil {
			t.Fatal(err)
		}
		for _, c := range ref.Relog {
			if c.Name == "member_joined_connect_new" && (len(c.Blocks) == 4) == renewal {
				t.Fatalf("%s: the recording blocks %v", mode, c.Blocks)
			}
		}
		w := newSocialWorld(nil, "map/a")
		z := w.zones[0]
		r := *rules.Of(renewal)
		z.Mods = &Mods{Rules: &r}
		alice := w.join(z, "Alice")
		w.step(50)
		if _, o := z.socialHub().GuildCreate(alice.soc, "Relog"); o != social.GuildCreated {
			t.Fatal(o)
		}
		z.leave(alice, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
		again := w.join(z, "Alice")
		w.step(100)
		until, blocked := again.cooldowns[social.SkillEmergencyCall]
		if renewal == blocked {
			t.Errorf("renewal %v: blocked %v", renewal, blocked)
		}
		if blocked && until != z.now+300000-50 && until < z.now+299000 {
			t.Errorf("blocked until %d, now %d", until, z.now)
		}
	}
}
