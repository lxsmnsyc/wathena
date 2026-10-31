// Package social keeps a world's parties, friend lists and party booking
// ads (phase 5 step 4, proposal 0002 review decision 27, decision 0020).
//
// One Registry serves every zone of a process, as the chat hub does. It
// knows the players online, by name, and hands each one what changed
// through its Inbox. Zones call it from their own goroutines, so it
// locks itself, and Inbox deliveries never block.
//
// Rules:
//   - Parties follow rAthena's party.cpp and the char server's
//     int_party.cpp: members sit in slots, the leader invites, expels,
//     hands over the lead and sets the options, and the party breaks up
//     when its leader leaves.
//   - Friend lists follow clif.cpp's friend packets: a request needs the
//     other player online and its answer, and a removal takes each name
//     off the other's list.
//   - Every write goes to the Store in order. Without a Store, parties
//     and friend lists live while the process runs.
//   - Names are keys without regard to case, as character names in the
//     database.
package social

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// Key is the key of a character name: chat.Key.
func Key(name string) string { return chat.Key(name) }

// Inbox gets what changed for one online player. The zone that holds the
// player implements it. DeliverSocial must not block: it may run on
// another zone's goroutine with the registry locked.
type Inbox interface {
	DeliverSocial(n Notice)
}

// Player is a character the registry knows: one that is online, or one
// that is offline and in a party or on a friend list. Its fields are the
// registry's: zones read them through the registry's methods.
type Player struct {
	key, name   string
	charID      int64
	accountID   int64
	online      bool
	inbox       Inbox
	mapName     string
	x, y        int
	hp, maxHP   int64
	level, job  int
	party       *Party
	friends     []Friend
	friendsSeen bool
	// friendReq is the key of the player this one asked to be friends
	// with, or that asked it, as sd->friend_req.
	friendReq string
	// nextFriendAsk is when the player may send its next friend
	// request.
	nextFriendAsk time.Time
	// invite is the party invite the player has open, as
	// sd->party_invite.
	invite *invite
	// guild is the player's guild, guildInvite its open guild invite, as
	// sd->guild_invite, and allyInvite its open alliance request, as
	// sd->guild_alliance.
	guild       *Guild
	guildInvite *guildInvite
	allyInvite  *allyInvite
}

// Name is the player's character name.
func (p *Player) Name() string { return p.name }

// Key is the player's key.
func (p *Player) Key() string { return p.key }

type invite struct {
	party int64
	from  string
}

// Friend is one name on a friend list.
type Friend struct {
	Name string
	// CharID is the friend's database ID, 0 for a character that only
	// lives in memory.
	CharID int64
}

// Registry holds a world's parties, friend lists and booking ads.
type Registry struct {
	// Store saves parties and friend lists. Without one, they last while
	// the process runs.
	Store Store
	// Blocks reports whether the online player target blocks the name
	// from. Party invites and friend requests check it. Without it, no
	// one blocks anyone.
	Blocks func(target, from string) bool
	// Clock is the server clock. time.Now when nil.
	Clock func() time.Time
	// Castles hears guild breaks and badge changes (proposal 0004,
	// section 2): a broken guild's castles run OnGuildBreak and lose
	// their owner, and NPCs that show a guild's badge follow its emblem.
	// The registry calls it with its lock held. Without it, nothing
	// hears them.
	Castles Castles

	limits      rules.Limits
	rules       rules.Party
	guildRules  rules.Guild
	guildTables *GuildTables

	mu       sync.Mutex
	players  map[string]*Player
	parties  map[int64]*Party
	names    map[string]*Party
	ended    map[int64]*Party
	endedIDs []int64
	nextID   int64
	booking  booking

	guilds        map[int64]*Guild
	guildNames    map[string]*Guild
	endedGuilds   map[int64]*Guild
	endedGuildIDs []int64
	nextGuild     int64
}

// Castles is what the registry tells the castle registry.
type Castles interface {
	GuildBroken(guild int64)
	BadgeChanged(guild int64)
}

// maxEnded is how many broken up parties stay readable, so a handle in
// a party_leave event can still read the party's name.
const maxEnded = 1024

// New makes a registry with a world's limits and party rules.
func New(limits rules.Limits, party rules.Party) *Registry {
	if limits == (rules.Limits{}) {
		limits = rules.DefaultLimits()
	}
	return &Registry{limits: limits, rules: party, guildRules: rules.DefaultGuild(), players: map[string]*Player{},
		parties: map[int64]*Party{}, names: map[string]*Party{}, ended: map[int64]*Party{}, nextID: 1,
		guilds: map[int64]*Guild{}, guildNames: map[string]*Guild{}, endedGuilds: map[int64]*Guild{}, nextGuild: 1}
}

// Limits are the registry's limits.
func (r *Registry) Limits() rules.Limits { return r.limits }

// Rules are the registry's party rules.
func (r *Registry) Rules() rules.Party { return r.rules }

func (r *Registry) now() time.Time {
	if r.Clock != nil {
		return r.Clock()
	}
	return time.Now()
}

// player returns the player of a key, made when missing. r.mu must be
// held.
func (r *Registry) player(name string, charID int64) *Player {
	k := Key(name)
	p := r.players[k]
	if p == nil {
		p = &Player{key: k, name: name, charID: charID}
		r.players[k] = p
	}
	if charID != 0 {
		p.charID = charID
	}
	return p
}

// online returns the online player of a name, or nil. r.mu must be held.
func (r *Registry) online(name string) *Player {
	if p := r.players[Key(name)]; p != nil && p.online {
		return p
	}
	return nil
}

// blocks reports whether target blocks from. r.mu must be held; Blocks
// must not call the registry.
func (r *Registry) blocks(target, from *Player) bool {
	return r.Blocks != nil && r.Blocks(target.name, from.name)
}

// deliver hands a notice to an online player. r.mu must be held.
func deliver(p *Player, n Notice) {
	if p != nil && p.online && p.inbox != nil {
		p.inbox.DeliverSocial(n)
	}
}

// Login is what a zone knows of a player that logged in.
type Login struct {
	Name      string
	CharID    int64
	AccountID int64
	Inbox     Inbox
	Map       string
	X, Y      int
	HP, MaxHP int64
	Level     int
	Job       int
	// Friends is the friend list the character loaded with. The registry
	// keeps its own list of a character it saw since the process
	// started, which is newer.
	Friends []Friend
}

// Online adds a player that logged in. A player of the same name that is
// still online, such as an older login, is replaced. The player gets its
// party and friend list, its friends learn that it is online, and its
// party sees it online. It returns the player's handle.
func (r *Registry) Online(l Login) *Player {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.player(l.Name, l.CharID)
	p.name, p.accountID = l.Name, l.AccountID
	was := p.online
	p.online, p.inbox, p.mapName, p.x, p.y = true, l.Inbox, l.Map, l.X, l.Y
	p.hp, p.maxHP, p.level, p.job = l.HP, l.MaxHP, l.Level, l.Job
	if !p.friendsSeen {
		p.friends, p.friendsSeen = slices.Clone(l.Friends), true
	}
	deliver(p, Notice{Kind: NoticeFriends, Friends: r.friendViews(p)})
	if !was {
		r.friendToggle(p, true)
	}
	if pt := p.party; pt != nil {
		deliver(p, Notice{Kind: NoticeParty, Party: pt.view()})
		r.memberChanged(pt, p)
		r.checkLevels(pt, p)
	} else {
		deliver(p, Notice{Kind: NoticeParty})
	}
	r.guildOnline(p, !was)
	return p
}

// Offline takes a player that logged out out of the registry: its open
// invite, friend request and booking ad end, and its friends and party
// see it offline. A player that a newer login replaced is left alone.
func (r *Registry) Offline(p *Player, inbox Inbox) {
	if p == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !p.online || p.inbox != inbox {
		return
	}
	p.online, p.inbox = false, nil
	p.invite, p.friendReq = nil, ""
	p.guildInvite, p.allyInvite = nil, nil
	r.booking.remove(p.key)
	r.friendToggle(p, false)
	if pt := p.party; pt != nil {
		r.memberChanged(pt, p)
		r.checkLevels(pt, p)
	}
	if g := p.guild; g != nil {
		r.closeStorageOf(g, p)
		if m := g.member(p); m != nil {
			r.guildMemberChanged(g, m)
		}
	}
}

// Move records that a player changed maps: its new inbox, map and cell.
// Its party sees the new map.
func (r *Registry) Move(p *Player, inbox Inbox, mapName string, x, y int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.inbox, p.mapName, p.x, p.y = inbox, mapName, x, y
	if pt := p.party; pt != nil {
		// Notices sent while the player moved between zones were dropped,
		// so it gets its party again.
		deliver(p, Notice{Kind: NoticeParty, Party: pt.view()})
		r.memberChanged(pt, p)
	}
	if g := p.guild; g != nil {
		deliver(p, Notice{Kind: NoticeGuild, Guild: r.guildView(g)})
		if m := g.member(p); m != nil {
			r.guildMemberChanged(g, m)
		}
	}
}

// Update records a player's HP, cell, level and job, for the party
// window of members on other maps. Members on the same map get HP and
// cells from their zone. A level or job change goes to the party at
// once, and a level change checks the even exp share range, as the char
// server does.
func (r *Registry) Update(p *Player, hp, maxHP int64, x, y, level, job int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := p.level != level || p.job != job
	levelChanged := p.level != level
	p.hp, p.maxHP, p.x, p.y, p.level, p.job = hp, maxHP, x, y, level, job
	if pt := p.party; pt != nil && changed {
		r.memberChanged(pt, p)
		if levelChanged {
			r.checkLevels(pt, p)
		}
	}
	if g := p.guild; g != nil && changed {
		if m := g.member(p); m != nil {
			r.guildMemberChanged(g, m)
		}
	}
}

// IsOnline reports whether a character of the name is online.
func (r *Registry) IsOnline(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.online(name) != nil
}

// trimName trims a party name as party_create does.
func trimName(name string) string { return strings.TrimSpace(name) }
