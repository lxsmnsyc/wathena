package social

import (
	"slices"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

func (b *box) guildResults() []GuildOutcome {
	var out []GuildOutcome
	for _, n := range b.got {
		if n.Kind == NoticeGuildResult {
			out = append(out, n.GuildOutcome)
		}
	}
	return out
}

func (b *box) lastGuildResult() GuildOutcome {
	r := b.guildResults()
	if len(r) == 0 {
		return 0
	}
	return r[len(r)-1]
}

// guild makes a guild of the first name with the others as members.
func (w *world) guild(t *testing.T, names ...string) int64 {
	t.Helper()
	id, o := w.r.GuildCreate(w.ps[names[0]], "Guild "+names[0])
	if id == 0 {
		t.Fatalf("create: %v", o)
	}
	for _, n := range names[1:] {
		if o := w.r.GuildInvite(w.ps[names[0]], n); o != GuildInviteSent {
			t.Fatalf("invite %s: %v", n, o)
		}
		if o := w.r.GuildReply(w.ps[n], id, true); o != GuildAccepted {
			t.Fatalf("reply %s: %v", n, o)
		}
	}
	return id
}

func guildWorld(t *testing.T, names ...string) *world {
	t.Helper()
	return newWorld(t, rules.DefaultLimits(), rules.DefaultParty(), names...)
}

func TestGuildCreateInviteAccept(t *testing.T) {
	w := guildWorld(t, "Ann", "Bob", "Cid")
	id := w.guild(t, "Ann", "Bob")
	if got := w.boxes["Ann"].guildResults(); !slices.Equal(got, []GuildOutcome{GuildCreated, GuildInviteSent, GuildAccepted}) {
		t.Errorf("Ann's outcomes %v", got)
	}
	n, _ := w.boxes["Bob"].last(NoticeGuild)
	v := n.Guild
	if v == nil || v.ID != id || len(v.Members) != 2 || !v.Members[0].Master || v.Members[1].Position != 19 || v.Master != "Ann" {
		t.Fatalf("Bob's guild %+v", v)
	}
	if v.Positions[0].Name != "GuildMaster" || !v.Positions[0].Invite || v.Positions[19].Name != "Newbie" || v.Positions[1].Name != "Position 2" {
		t.Errorf("default positions %+v", v.Positions)
	}
	if v.Level != 1 || v.NextExp != 100000 || v.MaxMembers != 16 {
		t.Errorf("level %d next %d max %d", v.Level, v.NextExp, v.MaxMembers)
	}
	ev, _ := w.boxes["Bob"].last(NoticeGuildEvent)
	if ev.Event != (Event{Kind: EventJoin, Party: id, Reason: ReasonInvited}) {
		t.Errorf("Bob's event %+v", ev.Event)
	}
	// The master who made the guild gets guild_join with "create" (review
	// decision 32).
	ev, _ = w.boxes["Ann"].last(NoticeGuildEvent)
	if ev.Event != (Event{Kind: EventJoin, Party: id, Reason: ReasonCreate}) {
		t.Errorf("Ann's event %+v", ev.Event)
	}
	// Newbie has no invite permission.
	if o := w.r.GuildInvite(w.ps["Bob"], "Cid"); o != GuildNoPermission {
		t.Errorf("member invite: %v", o)
	}
	// The name is taken without regard to case; the master is in a guild.
	if id2, o := w.r.GuildCreate(w.ps["Cid"], "guild ann"); id2 != 0 || o != GuildNameTaken {
		t.Errorf("taken name: %d %v", id2, o)
	}
	if _, o := w.r.GuildCreate(w.ps["Ann"], "Another"); o != GuildAlreadyInGuild {
		t.Errorf("second guild: %v", o)
	}
	if _, o := w.r.GuildCreate(w.ps["Cid"], "  "); o != GuildBadName {
		t.Errorf("empty name: %v", o)
	}
	// A refusal tells the inviter.
	w.r.GuildInvite(w.ps["Ann"], "Cid")
	w.r.GuildReply(w.ps["Cid"], id, false)
	if o := w.boxes["Ann"].lastGuildResult(); o != GuildRefused {
		t.Errorf("refusal: %v", o)
	}
	// An answer without an open invite is ignored.
	if o := w.r.GuildReply(w.ps["Cid"], id, true); o != 0 {
		t.Errorf("stale reply: %v", o)
	}
}

func TestGuildInviteRespectsBlocksAndLimits(t *testing.T) {
	limits := rules.DefaultLimits()
	limits.GuildMembers = 2
	w := newWorld(t, limits, rules.DefaultParty(), "Ann", "Bob", "Cid", "Dee")
	blocks := map[[2]string]bool{{"Cid", "Ann"}: true}
	w.r.Blocks = func(target, from string) bool { return blocks[[2]string{target, from}] }
	id := w.guild(t, "Ann", "Bob")
	if o := w.r.GuildInvite(w.ps["Ann"], "Cid"); o != GuildRefused {
		t.Errorf("blocked invite: %v", o)
	}
	if _, ok := w.boxes["Cid"].last(NoticeGuildInvite); ok {
		t.Error("Cid saw the invite of a player it blocks")
	}
	if o := w.r.GuildInvite(w.ps["Ann"], "Dee"); o != GuildFull {
		t.Errorf("full guild: %v", o)
	}
	if o := w.r.GuildInvite(w.ps["Ann"], "Nobody"); o != GuildNotOnline {
		t.Errorf("offline: %v", o)
	}
	// Guild Extension adds 6 members per level.
	w.r.mu.Lock()
	w.r.guilds[id].skills[SkillExtension] = 1
	w.r.mu.Unlock()
	if o := w.r.GuildInvite(w.ps["Ann"], "Dee"); o != GuildInviteSent {
		t.Errorf("extension: %v", o)
	}
	// An open invite makes another one wait.
	other := w.login("Eve", 9, 10)
	w.r.GuildCreate(other, "Other")
	if o := w.r.GuildInvite(other, "Dee"); o != GuildTargetInGuild {
		t.Errorf("open invite: %v", o)
	}
}

func TestGuildPositionsAndPermissions(t *testing.T) {
	w := guildWorld(t, "Ann", "Bob", "Cid", "Dee")
	id := w.guild(t, "Ann", "Bob", "Cid")
	// Only the master changes positions.
	if o := w.r.GuildSetPositions(w.ps["Bob"], []PositionChange{{Index: 1}}); o != GuildNotMaster {
		t.Errorf("member positions: %v", o)
	}
	w.r.GuildSetPositions(w.ps["Ann"], []PositionChange{{Index: 1, Position: Position{Name: "Officer", Invite: true, Expel: true, ExpTax: 90}}})
	v := w.r.GuildViewOf(w.ps["Ann"])
	if p := v.Positions[1]; p.Name != "Officer" || !p.Invite || !p.Expel || p.Storage || p.ExpTax != 50 {
		t.Errorf("position 1 %+v; the tax is capped at guild_exp_limit 50", p)
	}
	if o, _ := w.r.GuildSetMemberPosition(w.ps["Ann"], "Bob", 1); o != GuildPositionChanged {
		t.Errorf("member position: %v", o)
	}
	if guild, tax := w.r.GuildTax(w.ps["Bob"]); guild != id || tax != 50 {
		t.Errorf("Bob's tax %d %d", guild, tax)
	}
	// The officer invites and expels; the master cannot be expelled.
	if o := w.r.GuildInvite(w.ps["Bob"], "Dee"); o != GuildInviteSent {
		t.Errorf("officer invite: %v", o)
	}
	if o := w.r.GuildExpel(w.ps["Bob"], "Ann", "no"); o != GuildNoPermission {
		t.Errorf("expel master: %v", o)
	}
	if o := w.r.GuildExpel(w.ps["Cid"], "Bob", "no"); o != GuildNoPermission {
		t.Errorf("newbie expel: %v", o)
	}
	if o := w.r.GuildExpel(w.ps["Bob"], "Cid", "rude"); o != GuildExpelled {
		t.Errorf("expel: %v", o)
	}
	ev, _ := w.boxes["Cid"].last(NoticeGuildEvent)
	if ev.Event != (Event{Kind: EventLeave, Party: id, Reason: ReasonExpelled}) {
		t.Errorf("Cid's event %+v", ev.Event)
	}
	w.r.mu.Lock()
	ex := w.r.guilds[id].expulsions
	w.r.mu.Unlock()
	if len(ex) != 1 || ex[0].Name != "Cid" || ex[0].Reason != "rude" {
		t.Errorf("expulsions %+v", ex)
	}
	// The master cannot leave; a member can.
	if o := w.r.GuildLeave(w.ps["Ann"], ""); o != GuildMasterCannotLeave {
		t.Errorf("master leave: %v", o)
	}
	if o := w.r.GuildLeave(w.ps["Bob"], "bye"); o != GuildLeft {
		t.Errorf("leave: %v", o)
	}
	// Notices: master only, and length limits.
	if o, _ := w.r.GuildSetNotice(w.ps["Ann"], GuildNotice{Title: "Hi", Text: "Welcome"}); o != GuildNoticeChanged {
		t.Errorf("notice: %v", o)
	}
	long := GuildNotice{Title: string(make([]byte, 60))}
	if o, _ := w.r.GuildSetNotice(w.ps["Ann"], long); o != 0 {
		t.Errorf("long notice: %v", o)
	}
}

func TestGuildMasterChangeDelay(t *testing.T) {
	w := guildWorld(t, "Ann", "Bob")
	now := time.Unix(1_000_000, 0)
	w.r.Clock = func() time.Time { return now }
	id := w.guild(t, "Ann", "Bob")
	o, ch := w.r.GuildSetMemberPosition(w.ps["Ann"], "Bob", 0)
	if o != GuildMasterChanged || ch == nil || ch.What != ChangeMaster || ch.Guild != id {
		t.Fatalf("master change: %v %+v", o, ch)
	}
	v := w.r.GuildViewOf(w.ps["Ann"])
	if v.Master != "Bob" || v.Members[0].Name != "Bob" || v.Members[1].Name != "Ann" || v.Members[1].Position != 19 {
		t.Errorf("after the change %+v", v.Members)
	}
	if _, ok := w.boxes["Bob"].last(NoticeGuildSkillBlock); !ok {
		t.Error("the new master's skills were not blocked")
	}
	// guild_leaderchange_delay 1440, compared with seconds as rAthena.
	now = now.Add(1439 * time.Second)
	if o, _ := w.r.GuildSetMemberPosition(w.ps["Bob"], "Ann", 0); o != GuildMasterChangeTooSoon {
		t.Errorf("too soon: %v", o)
	}
	now = now.Add(time.Second)
	if o, _ := w.r.GuildSetMemberPosition(w.ps["Bob"], "Ann", 0); o != GuildMasterChanged {
		t.Errorf("after the delay: %v", o)
	}
}

func TestGuildExpLevelsAndSkills(t *testing.T) {
	w := guildWorld(t, "Ann", "Bob")
	id := w.guild(t, "Ann", "Bob")
	if TaxOf(101, 10) != 10 || TaxOf(7, 50) != 3 || TaxOf(5, 100) != 5 || TaxOf(5, 0) != 0 || TaxOf(99, 99) != 98 {
		t.Error("TaxOf does not round as guild_payexp")
	}
	if ch := w.r.PayExp(w.ps["Bob"], 99999); ch != nil {
		t.Errorf("no level up yet: %+v", ch)
	}
	ch := w.r.PayExp(w.ps["Bob"], 400001)
	if ch == nil || ch.What != ChangeLevel {
		t.Fatalf("level up: %+v", ch)
	}
	v := w.r.GuildViewOf(w.ps["Ann"])
	// 500000 exp: level 2 at 100000, level 3 at 400000 more, 0 left.
	if v.Level != 3 || v.Exp != 0 || v.SkillPoints != 2 || v.NextExp != 900000 {
		t.Errorf("level %d exp %d points %d next %d", v.Level, v.Exp, v.SkillPoints, v.NextExp)
	}
	if m, _ := v.Member("Bob"); m.Exp != 500000 {
		t.Errorf("Bob gave %d", m.Exp)
	}
	// Requirements: Kafra Contract needs Official Guild Approval.
	if o := w.r.GuildSkillUp(w.ps["Ann"], SkillKafraContract); o != GuildSkillRefused {
		t.Errorf("missing requirement: %v", o)
	}
	if o := w.r.GuildSkillUp(w.ps["Bob"], SkillApproval); o != GuildNotMaster {
		t.Errorf("member skill up: %v", o)
	}
	w.r.GuildSkillUp(w.ps["Ann"], SkillApproval)
	w.r.GuildSkillUp(w.ps["Ann"], SkillKafraContract)
	if w.r.GuildSkillLevel(id, SkillKafraContract) != 1 {
		t.Error("Kafra Contract not raised")
	}
	if o := w.r.GuildSkillUp(w.ps["Ann"], SkillExtension); o != GuildSkillRefused {
		t.Errorf("no points left: %v", o)
	}
	// Glory of Guild has level 0 in the tree; Guild Storage is renewal's.
	if CoreGuildTables(true).MaxLevel(SkillGloryGuild) != 0 || CoreGuildTables(false).Skill(SkillGuildStorage) != nil {
		t.Error("tree")
	}
	// The cap: MAX_GUILDLEVEL. Each gift stops at MAX_GUILD_EXP.
	for range 3 {
		_, _ = w.r.GiveExp(id, MaxGuildExp)
	}
	v = w.r.GuildViewOf(w.ps["Ann"])
	if v.Level != 50 || v.NextExp != 0 {
		t.Errorf("top level %d next %d", v.Level, v.NextExp)
	}
}

func TestGuildPreRenewalExp(t *testing.T) {
	w := guildWorld(t, "Ann")
	w.r.SetGuildData(CoreGuildTables(false), rules.DefaultGuild())
	w.guild(t, "Ann")
	w.r.PayExp(w.ps["Ann"], 6000000)
	v := w.r.GuildViewOf(w.ps["Ann"])
	if v.Level != 3 || v.Exp != 0 || v.NextExp != 8000000 {
		t.Errorf("pre-renewal level %d exp %d next %d", v.Level, v.Exp, v.NextExp)
	}
}

func TestGuildRelations(t *testing.T) {
	limits := rules.DefaultLimits()
	limits.GuildAllies, limits.GuildEnemies = 1, 1
	w := newWorld(t, limits, rules.DefaultParty(), "Ann", "Bob", "Cid", "Dee")
	a := w.guild(t, "Ann")
	b := w.guild(t, "Bob")
	c := w.guild(t, "Cid", "Dee")
	if o := w.r.GuildEnemy(w.ps["Ann"], "Bob"); o != GuildEnemyAdded || !w.r.Enemies(a, b) || w.r.Enemies(b, a) {
		t.Errorf("enemy: %v", o)
	}
	if o := w.r.GuildEnemy(w.ps["Ann"], "Cid"); o != GuildEnemyLimit {
		t.Errorf("enemy limit: %v", o)
	}
	// An alliance ends the enemy relation.
	if o := w.r.GuildAlly(w.ps["Ann"], "Bob"); o != GuildAllyRequestSent {
		t.Fatalf("ally: %v", o)
	}
	if o := w.r.GuildAllyReply(w.ps["Bob"], a, true); o != GuildAllied || !w.r.Allied(a, b) || !w.r.Allied(b, a) || w.r.Enemies(a, b) {
		t.Errorf("allied: %v", o)
	}
	if o := w.r.GuildAlly(w.ps["Ann"], "Dee"); o != GuildAllyLimit {
		t.Errorf("ally limit: %v", o)
	}
	if o := w.r.GuildAlly(w.ps["Cid"], "Ann"); o != GuildTheirAllyLimit {
		t.Errorf("their ally limit: %v", o)
	}
	if o := w.r.GuildAlly(w.ps["Dee"], "Ann"); o != GuildNotMaster {
		t.Errorf("member ally: %v", o)
	}
	if o := w.r.GuildEndRelation(w.ps["Bob"], a, false); o != GuildRelationEnded || w.r.Allied(a, b) || w.r.Allied(b, a) {
		t.Errorf("end: %v", o)
	}
	_ = c
}

func TestGuildAllianceOnlyMaster(t *testing.T) {
	w := guildWorld(t, "Ann", "Bob", "Cid")
	g := rules.DefaultGuild()
	g.AllianceOnlyMaster = true
	w.r.SetGuildData(nil, g)
	w.guild(t, "Ann")
	w.guild(t, "Bob", "Cid")
	if o := w.r.GuildAlly(w.ps["Ann"], "Cid"); o != GuildTargetNotMaster {
		t.Errorf("only master: %v", o)
	}
	if o := w.r.GuildAlly(w.ps["Ann"], "Bob"); o != GuildAllyRequestSent {
		t.Errorf("master: %v", o)
	}
}

func TestGuildDisband(t *testing.T) {
	w := guildWorld(t, "Ann", "Bob")
	id := w.guild(t, "Ann", "Bob")
	if o, _ := w.r.GuildDisband(w.ps["Ann"], "Guild Ann"); o != GuildNotEmpty {
		t.Errorf("not empty: %v", o)
	}
	w.r.GuildLeave(w.ps["Bob"], "")
	if o, _ := w.r.GuildDisband(w.ps["Ann"], "guild ann"); o != GuildNotMember {
		t.Errorf("name must match exactly: %v", o)
	}
	if o, _ := w.r.GuildDisband(w.ps["Ann"], "Guild Ann"); o != GuildDisbanded {
		t.Errorf("disband: %v", o)
	}
	if w.r.GuildRuns(id) || w.r.GuildIDOf(w.ps["Ann"]) != 0 {
		t.Error("the guild still runs")
	}
	in, ok := w.r.Guild(id)
	if !ok || !in.Ended || in.Name != "Guild Ann" {
		t.Errorf("ended guild %+v %v", in, ok)
	}
	if _, ok := w.r.FindGuild("Guild Ann"); ok {
		t.Error("the name is still taken")
	}
}

func TestGuildEmergencyCall(t *testing.T) {
	w := guildWorld(t, "Ann", "Bob", "Cid", "Dee")
	w.guild(t, "Ann", "Bob", "Cid", "Dee")
	w.r.Offline(w.ps["Dee"], w.boxes["Dee"])
	blocked := map[[2]int]bool{{100, 99}: true}
	w.r.EmergencyCall(w.ps["Ann"], "map/prontera", 100, 100, func(x, y int) bool { return !blocked[[2]int{x, y}] })
	// Slot order: Ann 0 (the caster, j 0), Bob 1 (j 1: +1, 0), Cid 2 (j 2:
	// 0, +1). Dee is offline.
	b, _ := w.boxes["Bob"].last(NoticeEmergencyCall)
	c, _ := w.boxes["Cid"].last(NoticeEmergencyCall)
	if b.Call != (Call{Map: "map/prontera", X: 101, Y: 100}) || c.Call != (Call{Map: "map/prontera", X: 100, Y: 101}) {
		t.Errorf("calls %+v %+v", b.Call, c.Call)
	}
	if _, ok := w.boxes["Ann"].last(NoticeEmergencyCall); ok {
		t.Error("the caster was called")
	}
}

func TestGuildStorageOpen(t *testing.T) {
	w := guildWorld(t, "Ann", "Bob")
	id := w.guild(t, "Ann", "Bob")
	if _, _, o := w.r.OpenGuildStorage(w.ps["Ann"]); o != StorageNoStorage {
		t.Errorf("renewal without the skill: %v", o)
	}
	w.r.mu.Lock()
	w.r.guilds[id].skills[SkillGuildStorage] = 5
	w.r.mu.Unlock()
	st, size, o := w.r.OpenGuildStorage(w.ps["Ann"])
	if o != StorageOpened || size != 600 {
		t.Errorf("open: %v %d; level 5 is 600, as rAthena's MAX_GUILD_STORAGE", o, size)
	}
	if _, _, o := w.r.OpenGuildStorage(w.ps["Bob"]); o != StorageNoPermission {
		t.Errorf("newbie: %v", o)
	}
	w.r.GuildSetPositions(w.ps["Ann"], []PositionChange{{Index: 19, Position: Position{Name: "Newbie", Storage: true}}})
	if _, _, o := w.r.OpenGuildStorage(w.ps["Bob"]); o != StorageInUse {
		t.Errorf("in use: %v", o)
	}
	if _, _, o := w.r.OpenGuildStorage(w.ps["Ann"]); o != StorageAlreadyOpen {
		t.Errorf("twice: %v", o)
	}
	// Ann logs out: the storage closes.
	w.r.Offline(w.ps["Ann"], w.boxes["Ann"])
	st2, _, o := w.r.OpenGuildStorage(w.ps["Bob"])
	if o != StorageOpened || st2 != st {
		t.Errorf("Bob opens: %v", o)
	}
	// Bob is expelled while it has the storage open.
	ann := w.login("Ann", 1, 50)
	w.r.GuildExpel(ann, "Bob", "")
	if _, ok := w.boxes["Bob"].last(NoticeStorageClose); !ok {
		t.Error("no close notice at expel")
	}
	st.Lock()
	open := st.OpenFor(w.ps["Bob"])
	st.Unlock()
	if open {
		t.Error("the storage is still open for Bob")
	}
	// Pre-renewal has no Guild Storage skill: the limit is the size.
	w.r.SetGuildData(CoreGuildTables(false), rules.DefaultGuild())
	if _, size, o := w.r.OpenGuildStorage(ann); o != StorageOpened || size != 600 {
		t.Errorf("pre-renewal: %v %d", o, size)
	}
}

func TestGuildStorageCommitAndDisband(t *testing.T) {
	w := guildWorld(t, "Ann")
	w.r.SetGuildData(CoreGuildTables(false), rules.DefaultGuild())
	id := w.guild(t, "Ann")
	st, _, _ := w.r.OpenGuildStorage(w.ps["Ann"])
	items := item.New(&item.Data{Items: []item.Def{{ID: 909, Weight: 10, Stackable: true}}})
	st.Lock()
	b := st.Box()
	b.Items = items
	if _, err := transfer.New(transfer.ActionMove).Create(transfer.End(transfer.KindScript), item.Slot{ID: 909}, 5, b, item.Serial{}).Apply(nil); err != nil {
		t.Fatal(err)
	}
	base, cur := st.Commit()
	st.Unlock()
	if len(base) != 0 || len(cur) != 1 || cur[0].Amount != 5 {
		t.Errorf("commit %v %v", base, cur)
	}
	w.r.LogStorage(id, []StorageLogEntry{{Name: "Ann", ItemID: 909, Amount: 5}, {Name: "Ann", ItemID: 909, Amount: -1}})
	log, o := w.r.StorageLog(w.ps["Ann"])
	if o != StorageOpened || len(log) != 2 || log[0].Amount != -1 {
		t.Errorf("log %v %+v", o, log)
	}
	if a := w.r.AuditStorages(); len(a) != 1 || a[0].Guild != id || a[0].Items[0].Amount != 5 {
		t.Errorf("audit %+v", a)
	}
	o2, left := w.r.GuildDisband(w.ps["Ann"], "Guild Ann")
	if o2 != GuildDisbanded || len(left) != 1 || left[0].Amount != 5 {
		t.Errorf("disband %v %+v", o2, left)
	}
}

func TestGuildLoad(t *testing.T) {
	r := New(rules.DefaultLimits(), rules.DefaultParty())
	r.LoadGuilds([]GuildRecord{
		{ID: 7, Name: "Seven", Level: 2, Exp: 5, SkillPoints: 1, Skills: map[int]int{SkillExtension: 2},
			Positions: defaultPositions(20)[:3], Relations: []Relation{{Guild: 99, Name: "Gone"}, {Guild: 8, Name: "Eight"}},
			Members: []GuildMemberRecord{{Slot: 2, CharID: 11, Name: "Bob", Position: 2}, {Slot: 0, CharID: 10, Name: "Ann"}},
			Storage: []StorageSlot{{Slot: 3, ID: 909, Amount: 4}}},
		{ID: 8, Name: "Eight", Level: 1, Members: []GuildMemberRecord{{Slot: 0, CharID: 12, Name: "Cid"}},
			Relations: []Relation{{Guild: 7, Name: "Seven"}}},
		{ID: 9, Name: "Empty", Level: 1},
	})
	b := &box{}
	p := r.Online(Login{Name: "Bob", CharID: 11, Inbox: b, Map: "map/prontera", Level: 30})
	n, _ := b.last(NoticeGuild)
	v := n.Guild
	if v == nil || v.ID != 7 || v.Master != "Ann" || len(v.Positions) != 20 || v.MaxMembers != 28 || len(v.Relations) != 1 {
		t.Fatalf("loaded %+v", v)
	}
	if !r.Allied(7, 8) || r.GuildRuns(9) {
		t.Error("relations or empty guild")
	}
	if id, _ := r.GuildCreate(p, "x"); id != 0 {
		t.Error("Bob made a second guild")
	}
	q := r.Online(Login{Name: "Dee", Inbox: &box{}})
	if id, _ := r.GuildCreate(q, "New"); id != 10 {
		t.Errorf("new guild ID %d, want 10", id)
	}
	if a := r.AuditStorages(); len(a) != 3 || a[0].Items[0].Slot != 3 {
		t.Errorf("storage %+v", a)
	}
}

func TestGuildPositionsLimit(t *testing.T) {
	limits := rules.DefaultLimits()
	limits.GuildPositions = 4
	w := newWorld(t, limits, rules.DefaultParty(), "Ann", "Bob")
	w.guild(t, "Ann", "Bob")
	v := w.r.GuildViewOf(w.ps["Ann"])
	if len(v.Positions) != 4 || v.Positions[3].Name != "Newbie" || v.Positions[2].Name != "Position 3" {
		t.Errorf("positions %+v", v.Positions)
	}
	if m, _ := v.Member("Bob"); m.Position != 3 {
		t.Errorf("Bob's position %d", m.Position)
	}
	// A position past the limit is ignored.
	if o := w.r.GuildSetPositions(w.ps["Ann"], []PositionChange{{Index: 4, Position: Position{Name: "Extra"}}}); o != 0 {
		t.Errorf("past the limit: %v", o)
	}
}
