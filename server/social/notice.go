package social

import "github.com/lxsmnsyc/wathena/server/chat"

// NoticeKind is what a Notice carries.
type NoticeKind int

// The kinds of Notice.
const (
	// NoticeParty is the player's whole party, after a change of its
	// members, leader or options. Party is nil when the player has no
	// party.
	NoticeParty NoticeKind = iota + 1
	// NoticeMember is one member's values: HP, cell, map, level or
	// online state.
	NoticeMember
	// NoticeInvite is a party invite for the player.
	NoticeInvite
	// NoticePartyResult tells how a party action went, or what happened
	// to the party.
	NoticePartyResult
	// NoticeChat is a party chat line.
	NoticeChat
	// NoticeFriends is the player's whole friend list.
	NoticeFriends
	// NoticeFriendStatus is a friend that logged in or out.
	NoticeFriendStatus
	// NoticeFriendRequest asks the player to be friends.
	NoticeFriendRequest
	// NoticeFriendResult tells how a friend action went.
	NoticeFriendResult
	// NoticeEvent is a party_join or party_leave event about the player,
	// which the zone that holds it runs.
	NoticeEvent
	// NoticeItemShared tells the party who got a shared item.
	NoticeItemShared
	// NoticeBooking answers a party booking command.
	NoticeBooking
	// NoticeGuild is the player's whole guild, after a change. Guild is
	// nil when the player has no guild.
	NoticeGuild
	// NoticeGuildMember is one guild member's values.
	NoticeGuildMember
	// NoticeGuildExp is the guild after exp that did not level it up.
	NoticeGuildExp
	// NoticeGuildInvite is a guild invite for the player.
	NoticeGuildInvite
	// NoticeGuildAllyRequest asks the player's guild to be allies.
	NoticeGuildAllyRequest
	// NoticeGuildResult tells how a guild action went, or what happened
	// to the guild.
	NoticeGuildResult
	// NoticeGuildChat is a guild chat line.
	NoticeGuildChat
	// NoticeGuildEvent is a guild_join or guild_leave event about the
	// player, which the zone that holds it runs.
	NoticeGuildEvent
	// NoticeGuildChange is a guild_change event made outside a zone, which
	// the zone that holds the player runs.
	NoticeGuildChange
	// NoticeGuildBadge tells the zone the player's guild badge may have
	// changed, so players in sight see it again.
	NoticeGuildBadge
	// NoticeGuildSkillBlock blocks a new master's guild skills, as
	// guild_block_skill in pre-renewal.
	NoticeGuildSkillBlock
	// NoticeEmergencyCall sends the player to its master's cell.
	NoticeEmergencyCall
	// NoticeStorageClose closes the player's guild storage window.
	NoticeStorageClose
)

// Notice is one thing for one player. Its values are copies, so a zone
// may keep them.
type Notice struct {
	Kind          NoticeKind
	Party         *PartyView
	Member        MemberView
	Invite        InviteView
	PartyOutcome  PartyOutcome
	FriendOutcome FriendOutcome
	// Name is the player or party an outcome is about, a friend's name,
	// or who asks to be friends.
	Name    string
	Line    chat.Line
	Friends []FriendView
	Online  bool
	Event   Event
	Shared  ItemShared
	Booking BookingView
	// Guild, GuildMember, GuildOutcome, Change and Call are the guild
	// notices' values.
	Guild        *GuildView
	GuildMember  GuildMemberView
	GuildOutcome GuildOutcome
	Change       GuildChange
	Call         Call
}

// GuildView is what members see of a guild.
type GuildView struct {
	ID           int64
	Name         string
	Level        int
	Exp, NextExp int64
	SkillPoints  int
	MaxMembers   int
	Master       string
	Notice       GuildNotice
	Emblem       int
	AverageLevel int
	// Members are in slot order, as clif_guild_memberlist.
	Members   []GuildMemberView
	Positions []Position
	Relations []Relation
	Skills    []GuildSkillView
}

// Member returns a member's view by name.
func (v *GuildView) Member(name string) (GuildMemberView, bool) {
	k := Key(name)
	for _, m := range v.Members {
		if Key(m.Name) == k {
			return m, true
		}
	}
	return GuildMemberView{}, false
}

// GuildMemberView is what members see of a member.
type GuildMemberView struct {
	Name     string
	Position int
	Online   bool
	Map      string
	Job      int
	Level    int
	Exp      int64
	X, Y     int
	Master   bool
}

// GuildSkillView is one guild skill of the tree.
type GuildSkillView struct {
	ID, Level, Max int
	CanRaise       bool
}

// PartyView is what members see of a party.
type PartyView struct {
	ID      int64
	Name    string
	Options Options
	// Members are the members in slot order, as clif_party_info.
	Members []MemberView
}

// MemberView is what members see of a member.
type MemberView struct {
	Name   string
	Leader bool
	Online bool
	Map    string
	Job    int
	Level  int
	HP     int64
	MaxHP  int64
	X, Y   int
}

// InviteView is a party invite.
type InviteView struct {
	Party     int64
	PartyName string
	From      string
}

// FriendView is one name on a friend list.
type FriendView struct {
	Name   string
	Online bool
}

// EventKind is a party event of the Lua API.
type EventKind int

// The party events.
const (
	EventJoin EventKind = iota + 1
	EventLeave
)

// Reasons of party_join and party_leave (review decision 27).
const (
	ReasonInvited   = "invited"
	ReasonLeft      = "left"
	ReasonExpelled  = "expelled"
	ReasonDisbanded = "disbanded"
	ReasonScript    = "script"
	// ReasonCreate is a player who made the party or guild from the
	// client (review decision 32).
	ReasonCreate = "create"
)

// Event is a party_join or party_leave event.
type Event struct {
	Kind   EventKind
	Party  int64
	Reason string
}

// Change is a party_change event, which the zone that made the change
// runs.
type Change struct {
	Party int64
	// Leader is the leader's name.
	Leader  string
	Options Options
}

// ItemShared is an item that even item share gave, as
// clif_party_show_picker.
type ItemShared struct {
	Name   string
	Item   uint32
	Amount int
}

// PartyOutcome is the outcome of a party action, as the protocol's
// PartyOutcome.
type PartyOutcome int

// The party outcomes.
const (
	PartyCreated PartyOutcome = iota + 1
	PartyNameTaken
	PartyAlreadyInParty
	PartyBadName
	PartyNotLeader
	PartyNotOnline
	PartyTargetInParty
	PartyFull
	PartyInviteSent
	PartyRefused
	PartyAccepted
	PartySameAccount
	PartyJoined
	PartyLeft
	PartyExpelled
	PartyDisbanded
	PartyNotMember
	PartyLeaderChanged
	PartyLeaderNotHere
	PartyExpShareRefused
	PartyExpShareOff
	PartyOptionsChanged
	PartyNoParty
)

// GuildOutcome is the outcome of a guild action, as the protocol's
// GuildOutcome.
type GuildOutcome int

// The guild outcomes.
const (
	GuildCreated GuildOutcome = iota + 1
	GuildAlreadyInGuild
	GuildNameTaken
	GuildNoEmperium
	GuildBadName
	GuildNoGuild
	GuildNoPermission
	GuildNotMaster
	GuildNotOnline
	GuildTargetInGuild
	GuildFull
	GuildInviteSent
	GuildRefused
	GuildAccepted
	GuildJoined
	GuildLeft
	GuildExpelled
	GuildDisbanded
	GuildNotMember
	GuildMasterCannotLeave
	GuildMasterChanged
	GuildMasterChangeTooSoon
	GuildPositionsChanged
	GuildNoticeChanged
	GuildSkillRaised
	GuildSkillRefused
	GuildLevelUp
	GuildAllyRequestSent
	GuildAllied
	GuildAllyRefused
	GuildAllyLimit
	GuildTheirAllyLimit
	GuildAlreadyAllied
	GuildTargetBusy
	GuildTargetNotMaster
	GuildEnemyAdded
	GuildEnemyLimit
	GuildAlreadyEnemy
	GuildRelationEnded
	GuildSameGuild
	GuildNotEmpty
	GuildEmblemChanged
	GuildPositionChanged
)

// FriendOutcome is the outcome of a friend action, as the protocol's
// FriendOutcome.
type FriendOutcome int

// The friend outcomes.
const (
	FriendAdded FriendOutcome = iota + 1
	FriendRefused
	FriendListFull
	FriendTheirListFull
	FriendNotOnline
	FriendAlready
	FriendRequestSent
	FriendRemoved
	FriendNotFound
)
