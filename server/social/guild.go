package social

import (
	"errors"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// Guilds (phase 5 step 5, proposal 0002 review decision 28, decision
// 0021). They follow rAthena's guild.cpp and the char server's
// int_guild.cpp:
//   - Members sit in slots, as guild.member[]. The master is in slot 0.
//     A new member takes the first free slot. The member list, Emergency
//     Call and the exp tax go in slot order.
//   - A guild has positions, as guild.position[]. Position 0 is the
//     master's, and a new member gets the last one. A position names who
//     may invite, expel and use the guild storage, and the exp tax.
//   - Only the master changes positions, the notice, the skills, the
//     alliances and the master, and breaks the guild up, as clif.cpp's
//     gmaster_flag checks.
//   - The exp a member pays goes to the guild, which levels up by
//     exp_guild.yml and gets a skill point per level, as guild_calcinfo.

// MaxGuildName is the longest guild name, in characters.
const MaxGuildName = 24

// MaxNoticeTitle and MaxNoticeText are MAX_GUILDMES1 and MAX_GUILDMES2
// less their terminators, in characters.
const (
	MaxNoticeTitle = 59
	MaxNoticeText  = 119
)

// MaxGuildReason is the longest leave or expel reason, as the 40 bytes
// of guild_expulsion.mes less the terminator.
const MaxGuildReason = 39

// MaxExpulsions is MAX_GUILDEXPULSION: the expulsion list keeps the last
// ones.
const MaxExpulsions = 32

// extensionMembers is what each level of Guild Extension adds to
// guild_members, as guild_calcinfo.
const extensionMembers = 6

// Position is a guild position, as rAthena's guild_position.
type Position struct {
	Name string
	// Invite, Expel and Storage are GUILD_PERM_INVITE, GUILD_PERM_EXPEL
	// and GUILD_PERM_STORAGE.
	Invite, Expel, Storage bool
	// ExpTax is exp_mode: the percent of base exp a member pays the
	// guild.
	ExpTax int
}

// GuildNotice is the guild notice, as mes1 and mes2.
type GuildNotice struct {
	Title, Text string
}

// Relation is an allied or enemy guild, as guild_alliance.
type Relation struct {
	Guild int64
	Name  string
	// Enemy is opposition 1.
	Enemy bool
}

// Expulsion is one entry of the expulsion list.
type Expulsion struct {
	CharID int64
	Name   string
	Reason string
}

type guildMember struct {
	p        *Player
	position int
	// exp is what the member paid through the exp tax.
	exp int64
}

// Guild is one guild.
type Guild struct {
	ID          int64
	Name        string
	level       int
	exp         int64
	skillPoints int
	skills      map[int]int
	positions   []Position
	// members are the slots. A member who left leaves a hole, which the
	// next member to join takes. Slot 0 is the master.
	members    []*guildMember
	notice     GuildNotice
	relations  []Relation
	expulsions []Expulsion
	emblem     Emblem
	// lastMasterChange is guild.last_leader_change.
	lastMasterChange time.Time
	storage          *GuildStorage
	ended            bool
}

// Emblem is a guild's emblem: a re-encoded PNG and its version.
type Emblem struct {
	Version int
	PNG     []byte
}

type guildInvite struct {
	guild int64
	from  string
}

func (g *Guild) master() *guildMember {
	if len(g.members) == 0 {
		return nil
	}
	return g.members[0]
}

func (g *Guild) size() int {
	n := 0
	for _, m := range g.members {
		if m != nil {
			n++
		}
	}
	return n
}

func (g *Guild) list() []*guildMember {
	out := make([]*guildMember, 0, len(g.members))
	for _, m := range g.members {
		if m != nil {
			out = append(out, m)
		}
	}
	return out
}

func (g *Guild) member(p *Player) *guildMember {
	for _, m := range g.members {
		if m != nil && m.p == p {
			return m
		}
	}
	return nil
}

func (g *Guild) memberNamed(name string) *guildMember {
	k := Key(name)
	for _, m := range g.members {
		if m != nil && m.p.key == k {
			return m
		}
	}
	return nil
}

// isMaster reports whether p is the master.
func (g *Guild) isMaster(p *Player) bool {
	m := g.master()
	return m != nil && m.p == p
}

// skill is guild_checkskill.
func (g *Guild) skill(id int) int { return g.skills[id] }

// maxMembers is guild_calcinfo's max_member: guild_members, and 6 more
// for each level of Guild Extension.
func (r *Registry) maxMembers(g *Guild) int {
	return r.limits.GuildMembers + g.skill(SkillExtension)*extensionMembers
}

// position returns a member's position, or a zero one.
func (g *Guild) position(m *guildMember) Position {
	if m == nil || m.position < 0 || m.position >= len(g.positions) {
		return Position{}
	}
	return g.positions[m.position]
}

// defaultPositions are mapif_parse_CreateGuild's: GuildMaster with every
// permission, "Position 2" to the one before the last, and Newbie last.
func defaultPositions(n int) []Position {
	out := make([]Position, n)
	out[0] = Position{Name: "GuildMaster", Invite: true, Expel: true, Storage: true}
	for i := 1; i < n-1; i++ {
		out[i].Name = "Position " + strconv.Itoa(i+1)
	}
	out[n-1].Name = "Newbie"
	return out
}

// GuildRecord is a saved guild, as the Store reads and writes it.
type GuildRecord struct {
	ID          int64
	Name        string
	Level       int
	Exp         int64
	SkillPoints int
	Skills      map[int]int
	Positions   []Position
	Notice      GuildNotice
	Relations   []Relation
	Expulsions  []Expulsion
	Emblem      Emblem
	// LastMasterChange is when the master last changed, or zero.
	LastMasterChange time.Time
	Members          []GuildMemberRecord
	// Storage is the guild storage's slots, by slot, for Load.
	Storage []StorageSlot
}

// GuildMemberRecord is a saved guild member.
type GuildMemberRecord struct {
	Slot      int
	CharID    int64
	AccountID int64
	Name      string
	Position  int
	Exp       int64
	Level     int
	Job       int
	Map       string
}

func (g *Guild) record() GuildRecord {
	rec := GuildRecord{ID: g.ID, Name: g.Name, Level: g.level, Exp: g.exp, SkillPoints: g.skillPoints, Skills: g.skillsCopy(),
		Positions: slices.Clone(g.positions), Notice: g.notice, Relations: slices.Clone(g.relations),
		Expulsions: slices.Clone(g.expulsions), Emblem: g.emblem, LastMasterChange: g.lastMasterChange}
	for i, m := range g.members {
		if m != nil {
			rec.Members = append(rec.Members, guildMemberRecord(i, m))
		}
	}
	return rec
}

func (g *Guild) skillsCopy() map[int]int {
	out := make(map[int]int, len(g.skills))
	for k, v := range g.skills {
		out[k] = v
	}
	return out
}

func guildMemberRecord(slot int, m *guildMember) GuildMemberRecord {
	return GuildMemberRecord{Slot: slot, CharID: m.p.charID, AccountID: m.p.accountID, Name: m.p.name, Position: m.position,
		Exp: m.exp, Level: m.p.level, Job: m.p.job, Map: m.p.mapName}
}

// SetGuildData sets the world's guild tables and rules. Call it before
// the registry serves players. Without it the registry uses renewal's
// tables and rAthena's rules.
func (r *Registry) SetGuildData(t *GuildTables, g rules.Guild) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.guildTables, r.guildRules = t, g
}

// GuildTables are the world's guild tables.
func (r *Registry) GuildTables() *GuildTables {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tables()
}

// GuildRules are the world's guild rules.
func (r *Registry) GuildRules() rules.Guild {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.guildRules
}

func (r *Registry) tables() *GuildTables {
	if r.guildTables == nil {
		return CoreGuildTables(true)
	}
	return r.guildTables
}

// LoadGuilds puts saved guilds in the registry, at start. A guild
// without members is dropped and deleted from the Store, as
// guild_check_empty. Members past a lowered limit stay.
func (r *Registry) LoadGuilds(guilds []GuildRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range guilds {
		r.nextGuild = max(r.nextGuild, rec.ID+1)
		g := &Guild{ID: rec.ID, Name: rec.Name, level: max(rec.Level, 1), exp: rec.Exp, skillPoints: rec.SkillPoints,
			skills: rec.Skills, positions: rec.Positions, notice: rec.Notice, relations: rec.Relations, expulsions: rec.Expulsions,
			emblem: rec.Emblem, lastMasterChange: rec.LastMasterChange}
		if g.skills == nil {
			g.skills = map[int]int{}
		}
		if len(g.positions) < r.limits.GuildPositions {
			def := defaultPositions(r.limits.GuildPositions)
			g.positions = append(g.positions, def[len(g.positions):]...)
		}
		for _, m := range rec.Members {
			p := r.player(m.Name, m.CharID)
			if p.guild != nil {
				continue
			}
			p.accountID, p.level, p.job, p.mapName = m.AccountID, m.Level, m.Job, m.Map
			for len(g.members) <= m.Slot {
				g.members = append(g.members, nil)
			}
			slot := m.Slot
			if g.members[slot] != nil {
				g.members = append(g.members, nil)
				slot = len(g.members) - 1
			}
			pos := m.Position
			if pos < 0 || pos >= len(g.positions) {
				pos = len(g.positions) - 1
			}
			g.members[slot] = &guildMember{p: p, position: pos, exp: m.Exp}
			p.guild = g
		}
		if g.size() == 0 {
			r.save(func(s Store) { s.DeleteGuild(g.ID) })
			continue
		}
		if g.members[0] == nil {
			// The master's slot is empty, as after a master whose
			// character was deleted: the first member takes it.
			i := slices.IndexFunc(g.members, func(m *guildMember) bool { return m != nil })
			g.members[0], g.members[i] = g.members[i], nil
			g.members[0].position = 0
		}
		g.storage = newGuildStorage(g.ID, rec.Storage)
		r.calcInfo(g)
		r.guilds[g.ID] = g
		r.guildNames[Key(g.Name)] = g
	}
	// Relations to guilds that are gone are dropped, as
	// mapif_parse_GuildDeleteAlliance.
	for _, g := range r.guilds {
		g.relations = slices.DeleteFunc(g.relations, func(rel Relation) bool { return r.guilds[rel.Guild] == nil })
	}
}

// SetGuildIDFloor makes new guilds take IDs above floor, the highest ID
// that a guild that broke up left in the logs.
func (r *Registry) SetGuildIDFloor(floor int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextGuild = max(r.nextGuild, floor+1)
}

// calcInfo is guild_calcinfo: exp past the next level's need levels the
// guild up, each level giving a skill point. It returns the levels
// gained. r.mu must be held.
func (r *Registry) calcInfo(g *Guild) int {
	t := r.tables()
	levels := 0
	for g.level < MaxGuildLevel {
		next := t.Next(g.level)
		if next <= 0 || g.exp < next {
			break
		}
		g.exp -= next
		g.level++
		g.skillPoints++
		levels++
	}
	return levels
}

// guildBroadcast hands a notice to every online member. r.mu must be
// held.
func (g *Guild) broadcast(n Notice) {
	for _, m := range g.members {
		if m != nil {
			deliver(m.p, n)
		}
	}
}

// guildChanged sends every online member the whole guild. r.mu must be
// held.
func (r *Registry) guildChanged(g *Guild) {
	for _, m := range g.members {
		if m != nil && m.p.online {
			deliver(m.p, Notice{Kind: NoticeGuild, Guild: r.guildView(g)})
		}
	}
}

// guildMemberChanged sends the other online members one member's
// values. r.mu must be held.
func (r *Registry) guildMemberChanged(g *Guild, m *guildMember) {
	v := r.guildMemberView(g, m)
	for _, o := range g.members {
		if o != nil && o != m {
			deliver(o.p, Notice{Kind: NoticeGuildMember, GuildMember: v})
		}
	}
}

func guildResult(p *Player, o GuildOutcome, name string) {
	deliver(p, Notice{Kind: NoticeGuildResult, GuildOutcome: o, Name: name})
}

func guildEvent(p *Player, kind EventKind, guild int64, reason string) {
	deliver(p, Notice{Kind: NoticeGuildEvent, Event: Event{Kind: kind, Party: guild, Reason: reason}})
}

// ValidGuildName reports whether a guild name has 1 to MaxGuildName
// characters after trimming.
func ValidGuildName(name string) bool {
	name = trimName(name)
	return name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= MaxGuildName
}

// GuildCreate makes a guild with a master, as guild_create and the char
// server's mapif_parse_CreateGuild. The caller checks and takes the
// Emperium. It returns the guild's ID, or 0 and why not.
func (r *Registry) GuildCreate(master *Player, name string) (int64, GuildOutcome) {
	name = trimName(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	id, o := r.guildCreate(master, name)
	if o != GuildCreated {
		guildResult(master, o, name)
	}
	return id, o
}

func (r *Registry) guildCreate(master *Player, name string) (int64, GuildOutcome) {
	switch {
	case !ValidGuildName(name):
		return 0, GuildBadName
	case master.guild != nil:
		return 0, GuildAlreadyInGuild
	case r.guildNames[Key(name)] != nil:
		return 0, GuildNameTaken
	}
	g := &Guild{ID: r.nextGuild, Name: name, level: 1, skills: map[int]int{}, positions: defaultPositions(r.limits.GuildPositions),
		members: []*guildMember{{p: master, position: 0}}}
	g.storage = newGuildStorage(g.ID, nil)
	r.nextGuild++
	r.guilds[g.ID] = g
	r.guildNames[Key(name)] = g
	master.guild, master.guildInvite = g, nil
	r.save(func(s Store) { s.CreateGuild(g.record()) })
	guildResult(master, GuildCreated, name)
	r.guildChanged(g)
	deliver(master, Notice{Kind: NoticeGuildBadge})
	return g.ID, GuildCreated
}

// GuildInvite asks an online player to join from's guild, as
// guild_invite. from's position must allow invites. A player who blocks
// from refuses at once, without seeing the invite (decision 0018).
func (r *Registry) GuildInvite(from *Player, name string) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, target := r.guildInvite(from, name)
	guildResult(from, o, target)
	return o
}

func (r *Registry) guildInvite(from *Player, name string) (GuildOutcome, string) {
	g := from.guild
	if g == nil {
		return GuildNoGuild, ""
	}
	if !g.position(g.member(from)).Invite {
		return GuildNoPermission, ""
	}
	to := r.online(name)
	switch {
	case to == nil:
		return GuildNotOnline, name
	case to.guild == g:
		return GuildTargetInGuild, to.name
	case r.blocks(to, from):
		return GuildRefused, to.name
	case to.guild != nil || to.guildInvite != nil || to.invite != nil:
		// rAthena with invite_request_check: an open party invite
		// counts too.
		return GuildTargetInGuild, to.name
	case g.size() >= r.maxMembers(g):
		return GuildFull, to.name
	}
	to.guildInvite = &guildInvite{guild: g.ID, from: from.key}
	deliver(to, Notice{Kind: NoticeGuildInvite, Invite: InviteView{Party: g.ID, PartyName: g.Name, From: from.name}})
	return GuildInviteSent, to.name
}

// GuildReply answers a player's open guild invite, as
// guild_reply_invite and guild_member_added. An answer to an invite the
// player does not have is ignored.
func (r *Registry) GuildReply(p *Player, guild int64, accept bool) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv := p.guildInvite
	if inv == nil || inv.guild != guild {
		return 0
	}
	p.guildInvite = nil
	from := r.players[inv.from]
	g := r.guilds[guild]
	if !accept || p.guild != nil || g == nil {
		if from != nil {
			guildResult(from, GuildRefused, p.name)
		}
		return GuildRefused
	}
	if g.size() >= r.maxMembers(g) {
		if from != nil {
			guildResult(from, GuildFull, p.name)
		}
		return GuildFull
	}
	r.guildJoin(g, p)
	if from != nil {
		guildResult(from, GuildAccepted, p.name)
	}
	for _, m := range g.members {
		if m != nil && m.p != from && m.p != p {
			guildResult(m.p, GuildJoined, p.name)
		}
	}
	guildEvent(p, EventJoin, g.ID, ReasonInvited)
	r.guildChanged(g)
	return GuildAccepted
}

// guildJoin puts a player in the first free slot with the last position,
// as guild_makemember. r.mu must be held.
func (r *Registry) guildJoin(g *Guild, p *Player) {
	m := &guildMember{p: p, position: len(g.positions) - 1}
	slot := slices.Index(g.members, nil)
	if slot < 0 {
		g.members = append(g.members, m)
		slot = len(g.members) - 1
	} else {
		g.members[slot] = m
	}
	p.guild, p.guildInvite = g, nil
	r.save(func(s Store) { s.AddGuildMember(g.ID, guildMemberRecord(slot, m)) })
	deliver(p, Notice{Kind: NoticeGuildBadge})
}

// guildWithdraw takes a member out, as guild_member_withdraw: the player
// gets no guild, its open guild storage closes, and it gets its own
// outcome. r.mu must be held.
func (r *Registry) guildWithdraw(g *Guild, m *guildMember, own GuildOutcome, reason string) {
	i := slices.Index(g.members, m)
	if i < 0 {
		return
	}
	g.members[i] = nil
	p := m.p
	p.guild = nil
	r.closeStorageOf(g, p)
	r.save(func(s Store) { s.RemoveGuildMember(g.ID, p.charID, p.name) })
	guildResult(p, own, g.Name)
	deliver(p, Notice{Kind: NoticeGuild})
	deliver(p, Notice{Kind: NoticeGuildBadge})
	guildEvent(p, EventLeave, g.ID, reason)
}

// GuildLeave takes a player out of its guild, as guild_leave. The master
// cannot leave: rAthena's client hides the button, and a guild without
// its master in slot 0 would break the master checks.
func (r *Registry) GuildLeave(p *Player, reason string) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := p.guild
	switch {
	case g == nil:
		guildResult(p, GuildNoGuild, "")
		return GuildNoGuild
	case g.isMaster(p):
		guildResult(p, GuildMasterCannotLeave, "")
		return GuildMasterCannotLeave
	}
	r.guildRemove(g, g.member(p), GuildLeft, ReasonLeft, false, reason)
	return GuildLeft
}

// guildRemove takes a member out and tells the others. expel adds the
// member to the expulsion list. r.mu must be held.
func (r *Registry) guildRemove(g *Guild, m *guildMember, o GuildOutcome, reason string, expel bool, why string) {
	if expel {
		if len(g.expulsions) >= MaxExpulsions {
			g.expulsions = g.expulsions[1:]
		}
		g.expulsions = append(g.expulsions, Expulsion{CharID: m.p.charID, Name: m.p.name, Reason: why})
		r.save(func(s Store) { s.SaveGuild(g.record()) })
	}
	r.guildWithdraw(g, m, o, reason)
	for _, o2 := range g.members {
		if o2 != nil {
			guildResult(o2.p, o, m.p.name)
		}
	}
	if g.size() == 0 {
		r.guildBreak(g)
		return
	}
	r.guildChanged(g)
}

// GuildExpel removes a member by name, as guild_expulsion. by's position
// must allow expelling, and the master cannot be expelled.
func (r *Registry) GuildExpel(by *Player, name, reason string) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := by.guild
	switch {
	case g == nil:
		guildResult(by, GuildNoGuild, "")
		return GuildNoGuild
	case !g.position(g.member(by)).Expel:
		guildResult(by, GuildNoPermission, "")
		return GuildNoPermission
	}
	m := g.memberNamed(name)
	switch {
	case m == nil:
		guildResult(by, GuildNotMember, name)
		return GuildNotMember
	case g.members[0] == m:
		guildResult(by, GuildNoPermission, name)
		return GuildNoPermission
	}
	r.guildRemove(g, m, GuildExpelled, ReasonExpelled, true, reason)
	return GuildExpelled
}

// guildBreak ends a guild, as guild_broken: every member leaves it, its
// relations end, and its storage goes. r.mu must be held.
func (r *Registry) guildBreak(g *Guild) {
	for _, m := range g.list() {
		r.guildWithdraw(g, m, GuildDisbanded, ReasonDisbanded)
	}
	for _, rel := range g.relations {
		if o := r.guilds[rel.Guild]; o != nil {
			o.relations = slices.DeleteFunc(o.relations, func(x Relation) bool { return x.Guild == g.ID })
			r.save(func(s Store) { s.SaveGuild(o.record()) })
			r.guildChanged(o)
		}
	}
	g.ended = true
	delete(r.guilds, g.ID)
	if r.guildNames[Key(g.Name)] == g {
		delete(r.guildNames, Key(g.Name))
	}
	r.endedGuilds[g.ID] = g
	r.endedGuildIDs = append(r.endedGuildIDs, g.ID)
	if len(r.endedGuildIDs) > maxEnded {
		delete(r.endedGuilds, r.endedGuildIDs[0])
		r.endedGuildIDs = r.endedGuildIDs[1:]
	}
	r.save(func(s Store) { s.DeleteGuild(g.ID) })
}

// GuildDisband breaks a guild up, as guild_break: the master names the
// guild and is its only member.
func (r *Registry) GuildDisband(master *Player, name string) (GuildOutcome, []StorageSlot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := master.guild
	switch {
	case g == nil:
		guildResult(master, GuildNoGuild, "")
		return GuildNoGuild, nil
	case !g.isMaster(master):
		guildResult(master, GuildNotMaster, "")
		return GuildNotMaster, nil
	case g.Name != name:
		// guild_break compares the name exactly.
		guildResult(master, GuildNotMember, name)
		return GuildNotMember, nil
	case g.size() > 1:
		guildResult(master, GuildNotEmpty, g.Name)
		return GuildNotEmpty, nil
	}
	left := g.storage.takeAll()
	r.guildBreak(g)
	return GuildDisbanded, left
}

// GuildChange is a guild_change event, which the zone that made the
// change runs.
type GuildChange struct {
	Guild int64
	// What is "master", "level", "notice" or "emblem".
	What string
}

// GuildSetMemberPosition gives a member a position, as
// guild_change_memberposition. Position 0 hands the guild to the member,
// as guild_gm_change, after guild_leaderchange_delay. Only the master
// may do it.
func (r *Registry) GuildSetMemberPosition(master *Player, name string, position int) (GuildOutcome, *GuildChange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := master.guild
	switch {
	case g == nil:
		guildResult(master, GuildNoGuild, "")
		return GuildNoGuild, nil
	case !g.isMaster(master):
		guildResult(master, GuildNotMaster, "")
		return GuildNotMaster, nil
	}
	m := g.memberNamed(name)
	if m == nil {
		guildResult(master, GuildNotMember, name)
		return GuildNotMember, nil
	}
	if position == 0 {
		// rAthena compares the delay with seconds, though guild.conf
		// says minutes.
		if d := r.guildRules.LeaderChangeDelay; d > 0 && r.now().Sub(g.lastMasterChange) < time.Duration(d)*time.Second {
			guildResult(master, GuildMasterChangeTooSoon, "")
			return GuildMasterChangeTooSoon, nil
		}
		if m.p == master {
			return 0, nil
		}
		return GuildMasterChanged, r.setMaster(g, m)
	}
	if position < 0 || position >= len(g.positions) || m == g.members[0] {
		return 0, nil
	}
	m.position = position
	r.save(func(s Store) { s.SaveGuildMember(g.ID, guildMemberRecord(slices.Index(g.members, m), m)) })
	guildResult(master, GuildPositionChanged, m.p.name)
	r.guildChanged(g)
	deliver(m.p, Notice{Kind: NoticeGuildBadge})
	return GuildPositionChanged, nil
}

// setMaster is guild_gm_changed: the new master swaps slots with the old
// one and takes position 0, and the old master takes the new one's
// position. r.mu must be held.
func (r *Registry) setMaster(g *Guild, m *guildMember) *GuildChange {
	i := slices.Index(g.members, m)
	old := g.members[0]
	g.members[0], g.members[i] = m, old
	old.position, m.position = m.position, 0
	g.lastMasterChange = r.now()
	r.save(func(s Store) { s.SaveGuild(g.record()) })
	r.save(func(s Store) {
		s.SaveGuildMember(g.ID, guildMemberRecord(0, m))
		s.SaveGuildMember(g.ID, guildMemberRecord(i, old))
	})
	g.broadcast(Notice{Kind: NoticeGuildResult, GuildOutcome: GuildMasterChanged, Name: m.p.name})
	r.guildChanged(g)
	deliver(m.p, Notice{Kind: NoticeGuildBadge})
	deliver(old.p, Notice{Kind: NoticeGuildBadge})
	// A new master's guild skills wait, as guild_gm_changed's
	// guild_block_skill in pre-renewal.
	deliver(m.p, Notice{Kind: NoticeGuildSkillBlock})
	return &GuildChange{Guild: g.ID, What: ChangeMaster}
}

// The kinds of guild_change.
const (
	ChangeMaster = "master"
	ChangeLevel  = "level"
	ChangeNotice = "notice"
	ChangeEmblem = "emblem"
)

// PositionChange is one position a master changes.
type PositionChange struct {
	Index int
	Position
}

// GuildSetPositions changes positions, as guild_change_position. The exp
// tax is capped at guild_exp_limit. Position 0 keeps every permission.
// Only the master may do it.
func (r *Registry) GuildSetPositions(master *Player, changes []PositionChange) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := master.guild
	switch {
	case g == nil:
		guildResult(master, GuildNoGuild, "")
		return GuildNoGuild
	case !g.isMaster(master):
		guildResult(master, GuildNotMaster, "")
		return GuildNotMaster
	}
	changed := false
	for _, c := range changes {
		if c.Index < 0 || c.Index >= len(g.positions) {
			continue
		}
		name := trimName(c.Name)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxGuildName {
			continue
		}
		pos := c.Position
		pos.Name = name
		pos.ExpTax = min(max(pos.ExpTax, 0), r.guildRules.ExpLimit)
		g.positions[c.Index] = pos
		changed = true
	}
	if !changed {
		return 0
	}
	r.save(func(s Store) { s.SaveGuild(g.record()) })
	g.broadcast(Notice{Kind: NoticeGuildResult, GuildOutcome: GuildPositionsChanged, Name: g.Name})
	r.guildChanged(g)
	g.broadcast(Notice{Kind: NoticeGuildBadge})
	return GuildPositionsChanged
}

// ValidNotice reports whether a notice fits MaxNoticeTitle and
// MaxNoticeText.
func ValidNotice(n GuildNotice) bool {
	return utf8.ValidString(n.Title) && utf8.ValidString(n.Text) && utf8.RuneCountInString(n.Title) <= MaxNoticeTitle &&
		utf8.RuneCountInString(n.Text) <= MaxNoticeText
}

// GuildSetNotice changes the notice, as guild_change_notice. Only the
// master may do it.
func (r *Registry) GuildSetNotice(master *Player, n GuildNotice) (GuildOutcome, *GuildChange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := master.guild
	switch {
	case g == nil:
		guildResult(master, GuildNoGuild, "")
		return GuildNoGuild, nil
	case !g.isMaster(master):
		guildResult(master, GuildNotMaster, "")
		return GuildNotMaster, nil
	case !ValidNotice(n):
		return 0, nil
	}
	return GuildNoticeChanged, r.setNotice(g, n)
}

func (r *Registry) setNotice(g *Guild, n GuildNotice) *GuildChange {
	g.notice = n
	r.save(func(s Store) { s.SaveGuild(g.record()) })
	g.broadcast(Notice{Kind: NoticeGuildResult, GuildOutcome: GuildNoticeChanged, Name: g.Name})
	r.guildChanged(g)
	return &GuildChange{Guild: g.ID, What: ChangeNotice}
}

// canRaise reports whether the master may raise a skill now: a point,
// below the tree's maximum, and every requirement met, as
// guild_check_skill_require. rAthena leaves the requirements to the
// client. r.mu must be held.
func (r *Registry) canRaise(g *Guild, id int) bool {
	d := r.tables().Skill(id)
	if d == nil || g.skillPoints <= 0 || g.skill(id) >= d.Max {
		return false
	}
	for _, n := range d.Needs {
		if g.skill(n.ID) < n.Level {
			return false
		}
	}
	return true
}

// GuildSkillUp raises a guild skill, as guild_skillup and the char
// server's mapif_parse_GuildSkillUp. Only the master may do it.
func (r *Registry) GuildSkillUp(master *Player, id int) GuildOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := master.guild
	switch {
	case g == nil:
		guildResult(master, GuildNoGuild, "")
		return GuildNoGuild
	case !g.isMaster(master):
		guildResult(master, GuildNotMaster, "")
		return GuildNotMaster
	case !r.canRaise(g, id):
		guildResult(master, GuildSkillRefused, strconv.Itoa(id))
		return GuildSkillRefused
	}
	g.skills[id]++
	g.skillPoints--
	r.save(func(s Store) { s.SaveGuild(g.record()); s.SaveGuildExp(g.ID, g.level, g.exp, g.skillPoints) })
	guildResult(master, GuildSkillRaised, strconv.Itoa(id))
	r.guildChanged(g)
	return GuildSkillRaised
}

// GuildChat sends a guild chat line to every online member, the speaker
// too, as guild_send_message. It returns false when the speaker has no
// guild. Each zone hides the line from members who block the speaker.
func (r *Registry) GuildChat(from *Player, line chat.Line) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := from.guild
	if g == nil {
		return false
	}
	line.To = g.Name
	g.broadcast(Notice{Kind: NoticeGuildChat, Line: line})
	return true
}

// GuildTax returns the exp tax of a player's position in percent, and its
// guild's ID, or 0 and 0 without a guild, as guild_payexp's exp_mode.
func (r *Registry) GuildTax(p *Player) (int64, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil || p.guild == nil {
		return 0, 0
	}
	g := p.guild
	return g.ID, g.position(g.member(p)).ExpTax
}

// TaxOf is guild_payexp's share: base exp times the tax in percent,
// rounded down, or all of it at 100.
func TaxOf(base int64, tax int) int64 {
	if base <= 0 || tax < 1 {
		return 0
	}
	if tax < 100 {
		return base * int64(tax) / 100
	}
	return base
}

// PayExp adds a member's taxed exp to its guild and to the member's own
// count, as guild_payexp and the char server's GMI_EXP. The char server
// adds the cache every 10 seconds; the registry adds it at once. It
// returns the guild_change of a level up, or nil.
func (r *Registry) PayExp(p *Player, exp int64) *GuildChange {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil || p.guild == nil || exp <= 0 {
		return nil
	}
	g := p.guild
	if m := g.member(p); m != nil {
		m.exp = min(m.exp+exp, MaxGuildExp)
		r.save(func(s Store) { s.SaveGuildMemberExp(g.ID, p.charID, m.exp) })
	}
	return r.addExp(g, exp)
}

// addExp adds exp to a guild and levels it up. r.mu must be held.
func (r *Registry) addExp(g *Guild, exp int64) *GuildChange {
	g.exp = min(g.exp+exp, MaxGuildExp)
	levels := r.calcInfo(g)
	r.save(func(s Store) { s.SaveGuildExp(g.ID, g.level, g.exp, g.skillPoints) })
	if levels == 0 {
		v := r.guildView(g)
		for _, m := range g.members {
			if m != nil {
				deliver(m.p, Notice{Kind: NoticeGuildExp, Guild: v})
			}
		}
		return nil
	}
	g.broadcast(Notice{Kind: NoticeGuildResult, GuildOutcome: GuildLevelUp, Name: g.Name})
	r.guildChanged(g)
	return &GuildChange{Guild: g.ID, What: ChangeLevel}
}

// GuildIDOf returns a player's guild ID, or 0.
func (r *Registry) GuildIDOf(p *Player) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil || p.guild == nil {
		return 0
	}
	return p.guild.ID
}

// Allied reports whether two guilds are allies, as guild_isallied.
func (r *Registry) Allied(a, b int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.related(a, b, false)
}

// Enemies reports whether guild a has b as an enemy.
func (r *Registry) Enemies(a, b int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.related(a, b, true)
}

func (r *Registry) related(a, b int64, enemy bool) bool {
	g := r.guilds[a]
	if g == nil {
		return false
	}
	return slices.ContainsFunc(g.relations, func(rel Relation) bool { return rel.Guild == b && rel.Enemy == enemy })
}

// Errors of the guild script calls (wa.guilds).
var (
	ErrNoGuild = errors.New("the guild has disbanded")
)

// FindGuild finds a guild by name.
func (r *Registry) FindGuild(name string) (int64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g := r.guildNames[Key(trimName(name))]; g != nil {
		return g.ID, true
	}
	return 0, false
}

// GuildRuns reports whether a guild runs.
func (r *Registry) GuildRuns(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.guilds[id] != nil
}

// GuildInfo is what scripts read of a guild.
type GuildInfo struct {
	ID     int64
	Name   string
	Level  int
	Exp    int64
	Notice GuildNotice
	// Master is the master's name, and MasterOnline whether it is
	// online.
	Master       string
	MasterOnline bool
	// Members are every member's name in slot order, and Online the
	// online ones.
	Members []string
	Online  []string
	Ended   bool
}

// Guild returns what scripts read of a guild, also for a guild that
// broke up a short time ago. ok is false for an unknown ID.
func (r *Registry) Guild(id int64) (GuildInfo, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		g = r.endedGuilds[id]
	}
	if g == nil {
		return GuildInfo{}, false
	}
	in := GuildInfo{ID: g.ID, Name: g.Name, Level: g.level, Exp: g.exp, Notice: g.notice, Ended: g.ended}
	if m := g.master(); m != nil {
		in.Master, in.MasterOnline = m.p.name, m.p.online && !g.ended
	}
	for _, m := range g.list() {
		in.Members = append(in.Members, m.p.name)
		if m.p.online {
			in.Online = append(in.Online, m.p.name)
		}
	}
	return in, true
}

// MemberPosition returns a member's position, and false for a player who
// is not a member.
func (r *Registry) MemberPosition(id int64, p *Player) (Position, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return Position{}, false
	}
	m := g.member(p)
	if m == nil {
		return Position{}, false
	}
	return g.position(m), true
}

// GuildSkillLevel returns a guild skill's level, as guild_checkskill.
func (r *Registry) GuildSkillLevel(id int64, skill int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return 0
	}
	return g.skill(skill)
}

// Script calls of wa.guilds (review decision 28). They skip the
// position permissions and the invite, as rAthena's guild script
// commands.

// ScriptGuildCreate makes a guild for a master without an Emperium. It
// returns 0 when the name is taken or the master has a guild.
func (r *Registry) ScriptGuildCreate(master *Player, name string) int64 {
	name = trimName(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !ValidGuildName(name) || master.guild != nil || r.guildNames[Key(name)] != nil {
		return 0
	}
	id, _ := r.guildCreate(master, name)
	if id != 0 {
		guildEvent(master, EventJoin, id, ReasonScript)
	}
	return id
}

// GuildAddMember puts a player in a guild. It returns false when the
// player has a guild or the guild is full.
func (r *Registry) GuildAddMember(id int64, p *Player) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return false, ErrNoGuild
	}
	if p.guild != nil || g.size() >= r.maxMembers(g) {
		return false, nil
	}
	r.guildJoin(g, p)
	for _, m := range g.members {
		if m != nil && m.p != p {
			guildResult(m.p, GuildJoined, p.name)
		}
	}
	guildEvent(p, EventJoin, g.ID, ReasonScript)
	r.guildChanged(g)
	return true, nil
}

// GuildRemoveMember takes a member out of a guild. It returns false for a
// player who is not a member, and for the master.
func (r *Registry) GuildRemoveMember(id int64, p *Player) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return false, ErrNoGuild
	}
	m := g.member(p)
	if m == nil || g.members[0] == m {
		return false, nil
	}
	r.guildRemove(g, m, GuildExpelled, ReasonScript, false, "")
	return true, nil
}

// ScriptSetMaster makes a member the master, as guildchangegm: without
// the delay, and the member may be offline.
func (r *Registry) ScriptSetMaster(id int64, p *Player) (bool, *GuildChange, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return false, nil, ErrNoGuild
	}
	m := g.member(p)
	if m == nil || g.members[0] == m {
		return false, nil, nil
	}
	return true, r.setMaster(g, m), nil
}

// ScriptSetNotice sets the notice. It returns false for a notice that is
// too long.
func (r *Registry) ScriptSetNotice(id int64, n GuildNotice) (bool, *GuildChange, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return false, nil, ErrNoGuild
	}
	if !ValidNotice(n) {
		return false, nil, nil
	}
	return true, r.setNotice(g, n), nil
}

// GiveExp adds exp to a guild, as guildgetexp, which rAthena pays
// through the attached player.
func (r *Registry) GiveExp(id, exp int64) (*GuildChange, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return nil, ErrNoGuild
	}
	if exp <= 0 {
		return nil, nil
	}
	return r.addExp(g, exp), nil
}

// DisbandGuild breaks a guild up from a script, with its members. It
// returns what the guild storage held, which is lost.
func (r *Registry) DisbandGuild(id int64) ([]StorageSlot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return nil, ErrNoGuild
	}
	left := g.storage.takeAll()
	r.guildBreak(g)
	return left, nil
}

// SetEmblem gives a guild a new emblem, as guild_emblem_changed: its
// version goes up by one. It returns the new version and the
// guild_change, or an error for a guild that does not run.
func (r *Registry) SetEmblem(id int64, png []byte) (int, *GuildChange, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil {
		return 0, nil, ErrNoGuild
	}
	g.emblem = Emblem{Version: g.emblem.Version + 1, PNG: slices.Clone(png)}
	r.save(func(s Store) { s.SaveGuildEmblem(g.ID, g.emblem) })
	g.broadcast(Notice{Kind: NoticeGuildResult, GuildOutcome: GuildEmblemChanged, Name: g.Name})
	r.guildChanged(g)
	g.broadcast(Notice{Kind: NoticeGuildBadge})
	ch := &GuildChange{Guild: g.ID, What: ChangeEmblem}
	// A change made outside a zone runs its event in the zone of the
	// master, or of the first online member.
	for _, m := range g.list() {
		if m.p.online {
			deliver(m.p, Notice{Kind: NoticeGuildChange, Change: *ch})
			break
		}
	}
	return g.emblem.Version, ch, nil
}

// EmblemOf returns a guild's emblem, or false for a guild without one.
func (r *Registry) EmblemOf(id int64) (Emblem, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[id]
	if g == nil || g.emblem.Version == 0 {
		return Emblem{}, false
	}
	return g.emblem, true
}

// GuildMasterOf reports whether a character is the master of a guild,
// for the account pages. It returns the guild's ID.
func (r *Registry) GuildMasterOf(charID int64) (int64, string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.guilds {
		if m := g.master(); m != nil && m.p.charID == charID && charID != 0 {
			return g.ID, g.Name, true
		}
	}
	return 0, "", false
}

// EmergencyCall is GD_EMERGENCYCALL: every online member but the caster
// gets a notice to come to the caster's cell, at the offsets of
// skill_castend_nodamage_id's dx and dy tables in slot order. reach
// reports whether a cell of the caster's map can be stood on; a cell that
// cannot moves the member to the caster's cell.
func (r *Registry) EmergencyCall(caster *Player, mapName string, x, y int, reach func(x, y int) bool) {
	dx := [9]int{-1, 1, 0, 0, -1, 1, -1, 1, 0}
	dy := [9]int{0, 0, 1, -1, 1, -1, -1, 1, 0}
	r.mu.Lock()
	defer r.mu.Unlock()
	g := caster.guild
	if g == nil {
		return
	}
	j := 0
	for i := 0; i < r.maxMembers(g); i++ {
		if j > 8 {
			j = 0
		}
		var m *guildMember
		if i < len(g.members) {
			m = g.members[i]
		}
		if m != nil && m.p != caster && m.p.online {
			cx, cy := x+dx[j], y+dy[j]
			if !reach(cx, cy) {
				dx[j], dy[j] = 0, 0
				cx, cy = x, y
			}
			deliver(m.p, Notice{Kind: NoticeEmergencyCall, Call: Call{Map: mapName, X: cx, Y: cy}})
		}
		j++
	}
}

// Call is where Emergency Call sends a member.
type Call struct {
	Map  string
	X, Y int
}
