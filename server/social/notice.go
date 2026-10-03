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
