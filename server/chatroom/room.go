// Package chatroom holds the state of chat rooms (proposal 0006, section
// 2; rAthena's chat.cpp): members in join order, the kick list, the
// limits, the join checks in rAthena's order, the owner of a player room
// and the trigger of an NPC room. It knows no players or NPCs: the zone
// wires it to them. Nothing here is saved.
package chatroom

// The limits of rAthena's chat rooms.
const (
	// MaxUsers is MAX_CHAT_USERS.
	MaxUsers = 20
	// MaxTitle is the longest title, CHATROOM_TITLE_SIZE - 1.
	MaxTitle = 36
	// MaxPassword is the longest password, CHATROOM_PASS_SIZE - 1.
	MaxPassword = 8
	// MaxTrigger is the most users that trigger a room's event: the
	// trigger is seven bits, and the eighth is the disabled flag.
	MaxTrigger = 127
)

// Outcome is the result of a join, in the order rAthena checks them.
type Outcome int

// The outcomes of Room.Join.
const (
	OK Outcome = iota
	// Full is also the answer for a joiner that cannot join at all: it
	// is vending, runs a buying store or is in a room already.
	Full
	WrongPassword
	LevelLow
	LevelHigh
	NoZeny
	Kicked
)

// Member is a player in a room.
type Member struct {
	// ID is the character's ID.
	ID   int64
	Name string
}

// Joiner is what Join checks of a player.
type Joiner struct {
	Member
	Level int
	Zeny  int64
	// Busy is set for a player that is vending or runs a buying store.
	Busy bool
	// InRoom is set for a player that is in a room already.
	InRoom bool
	// Any is the permission join_all_chat: no password is needed.
	Any bool
}

// Room is one chat room. An NPC room belongs to an NPC and takes one
// player fewer than its limit: the NPC counts. A player room belongs to
// its first member.
type Room struct {
	ID       uint32
	Title    string
	Password string
	Public   bool
	Limit    int
	// Trigger is the number of users that runs Event, 0 to MaxTrigger.
	Trigger  int
	Disabled bool
	// Event is the NPC event the trigger runs, or "".
	Event string
	Zeny  int64
	// MinLevel and MaxLevel bound the joiner's base level.
	MinLevel, MaxLevel int
	// NPC is set for a room of an NPC.
	NPC bool

	members []Member
	kicked  map[int64]bool
}

// New makes an empty room. The limit is cut to MaxUsers, as rAthena's
// array, and the trigger to the seven bits.
func New(id uint32, npc bool) *Room {
	r := &Room{ID: id, NPC: npc, Limit: 1, MinLevel: 1, MaxLevel: 1 << 30}
	if !npc {
		r.kicked = map[int64]bool{}
	}
	return r
}

// SetLimit sets the limit, cut to MaxUsers.
func (r *Room) SetLimit(n int) { r.Limit = min(n, MaxUsers) }

// SetTrigger sets the number of users that trigger the event, cut to
// the seven bits. It keeps the disabled flag.
func (r *Room) SetTrigger(n int) { r.Trigger = max(min(n, MaxTrigger), 0) }

// Users is the number of members. An NPC is not one.
func (r *Room) Users() int { return len(r.members) }

// Members returns the members in join order. The owner of a player room
// is first.
func (r *Room) Members() []Member { return append([]Member(nil), r.members...) }

// Has reports whether a character is a member.
func (r *Room) Has(id int64) bool { return r.index(id) >= 0 }

func (r *Room) index(id int64) int {
	for i, m := range r.members {
		if m.ID == id {
			return i
		}
	}
	return -1
}

// Owner returns the owner of a player room, the first member. ok is
// false for an NPC room and for an empty room.
func (r *Room) Owner() (Member, bool) {
	if r.NPC || len(r.members) == 0 {
		return Member{}, false
	}
	return r.members[0], true
}

// Full is getwaitingroomstate(32): users reached the limit. For an NPC
// room it is not the join test, which counts the NPC.
func (r *Room) Full() bool { return len(r.members) >= r.Limit }

// Triggered is getwaitingroomstate(33): enough users to run the event.
// A disabled trigger is the flag 0x80, which no user count reaches.
func (r *Room) Triggered() bool { return !r.Disabled && len(r.members) >= r.Trigger }

// ShouldRun reports whether chat_triggerevent runs the event now: the
// room has an event and enough users.
func (r *Room) ShouldRun() bool { return r.Event != "" && r.Triggered() }

// Join checks a player against the room in rAthena's order (chat_joinchat)
// and adds it when every check passes. The fee is only checked here; the
// zone does not take it.
func (r *Room) Join(j Joiner, password string) Outcome {
	users := len(r.members)
	if r.NPC {
		users++
	}
	if j.Busy || j.InRoom || users >= r.Limit {
		return Full
	}
	if !r.Public && password != r.Password && !j.Any {
		return WrongPassword
	}
	if j.Level < r.MinLevel {
		return LevelLow
	}
	if j.Level > r.MaxLevel {
		return LevelHigh
	}
	if j.Zeny < r.Zeny {
		return NoZeny
	}
	if !r.NPC && r.kicked[j.ID] {
		return Kicked
	}
	r.members = append(r.members, j.Member)
	return OK
}

// Add puts the first member into a new player room, the creator.
func (r *Room) Add(m Member) { r.members = append(r.members, m) }

// Leave removes a member. owner is the new owner of a player room when
// the first member left and others stay. empty is set for a player room
// that has no member left, which the zone deletes; an NPC room stays.
// ok is false for a character that is not a member.
func (r *Room) Leave(id int64) (owner *Member, empty, ok bool) {
	i := r.index(id)
	if i < 0 {
		return nil, false, false
	}
	r.members = append(r.members[:i], r.members[i+1:]...)
	if r.NPC {
		return nil, false, true
	}
	if len(r.members) == 0 {
		return nil, true, true
	}
	if i == 0 {
		m := r.members[0]
		return &m, false, true
	}
	return nil, false, true
}

// Kick removes a member of a player room and bars it from coming back
// (chat_kickchat). The owner is not a member to kick: by is the owner's
// ID, which must be the first member. protected is set for a member
// that holds the staff right no_chat_kick: nothing happens to it. ok is
// false when by is not the owner or the name is not a member.
func (r *Room) Kick(by int64, name string, protected func(Member) bool) (m Member, ok bool) {
	if r.NPC || len(r.members) == 0 || r.members[0].ID != by {
		return Member{}, false
	}
	i := r.named(name)
	if i < 0 {
		return Member{}, false
	}
	m = r.members[i]
	if protected != nil && protected(m) {
		return Member{}, false
	}
	r.kicked[m.ID] = true
	return m, true
}

// named finds a member by name.
func (r *Room) named(name string) int {
	for i, m := range r.members {
		if m.Name == name {
			return i
		}
	}
	return -1
}

// Find returns the member with a name.
func (r *Room) Find(name string) (Member, bool) {
	if i := r.named(name); i >= 0 {
		return r.members[i], true
	}
	return Member{}, false
}

// Transfer hands a player room to another member (chat_changechatowner):
// the two swap places. by must be the owner, and the new owner a member
// other than the first.
func (r *Room) Transfer(by int64, name string) bool {
	if r.NPC || len(r.members) < 2 || r.members[0].ID != by {
		return false
	}
	i := r.named(name)
	if i < 1 {
		return false
	}
	r.members[0], r.members[i] = r.members[i], r.members[0]
	return true
}

// Change is a player room's chat_changechatstatus: title, password,
// limit and the public flag. Only the owner may.
func (r *Room) Change(by int64, title, password string, limit int, public bool) bool {
	if r.NPC || len(r.members) == 0 || r.members[0].ID != by {
		return false
	}
	r.Title, r.Password, r.Public = title, password, public
	r.SetLimit(limit)
	return true
}

// NextID gives the ID of a room: the next free one after the highest.
func NextID(rooms map[uint32]*Room, last uint32) uint32 {
	for {
		last++
		if last == 0 {
			continue
		}
		if _, used := rooms[last]; !used {
			return last
		}
	}
}
