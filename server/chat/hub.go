package chat

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Limits of the hub.
const (
	// MaxBlocks is the most names a block list holds, as rAthena's
	// MAX_IGNORE_LIST.
	MaxBlocks = 20
	// MaxChannels is the most channels a player may be in. rAthena ends
	// the connection of a player in more than 200.
	MaxChannels = 200
	// MaxReports is the most reports a player may send in ReportWindow.
	MaxReports   = 10
	ReportWindow = time.Hour
)

// Inbox gets the lines and channel changes of one online player. The
// zone that holds the player implements it. DeliverChat must not block:
// it may run on another zone's goroutine with the hub locked.
type Inbox interface {
	DeliverChat(d Delivery)
}

// DeliveryKind is what a Delivery carries.
type DeliveryKind int

// The kinds of Delivery.
const (
	// DeliverWhisper is a whisper to the player.
	DeliverWhisper DeliveryKind = iota + 1
	// DeliverChannel is a line of a channel the player is in.
	DeliverChannel
	// DeliverJoined tells the player it is in a channel, such as after a
	// mod's channel:add.
	DeliverJoined
	// DeliverLeft tells the player it left a channel, such as when the
	// channel's mod ended it.
	DeliverLeft
)

// Delivery is one thing for one player.
type Delivery struct {
	Kind DeliveryKind
	// Line is the line, for whispers and channel lines. A channel's own
	// line, from a mod's channel:send, has no name and no ID.
	Line Line
	// Channel is the channel, for channel lines and changes.
	Channel ChannelInfo
}

// ChannelInfo is what players see of a channel.
type ChannelInfo struct {
	// Name is the name with its #, such as #trade.
	Name        string
	DisplayName string
	Color       uint32
	CanLeave    bool
	CanChat     bool
	// Members counts the members; ChannelList shows it.
	Members int
	Joined  bool
}

// Member is one online player in the hub. Its fields are the hub's: the
// zone reads them only through the hub's methods.
type Member struct {
	name        string
	key         string
	characterID int64
	inbox       Inbox
	mapName     string
	blocks      map[string]bool
	// channels holds the channels the member is in, with the time of its
	// last line in each, for their delays.
	channels map[*Channel]time.Time
	online   bool
	// reports are the times of the member's last reports.
	reports []time.Time
}

// Name is the member's character name.
func (m *Member) Name() string { return m.name }

// Channel is one running channel.
type Channel struct {
	Def ChannelDef
	// members are the channel's members, by lower case name.
	members map[string]*Member
	// owners are the contexts that made a mod's channel. It ends when the
	// last one releases it.
	owners map[any]bool
}

func (c *Channel) info(joined bool) ChannelInfo {
	return ChannelInfo{Name: c.Def.Full(), DisplayName: c.Def.DisplayName, Color: c.Def.Color, CanLeave: c.Def.CanLeave,
		CanChat: c.Def.CanChat, Members: len(c.members), Joined: joined}
}

// Hub is the chat of a world: the players online, their block lists,
// and the channels. Every zone of the world shares one. It is safe for
// use by many goroutines.
type Hub struct {
	// Log keeps the chat log and the reports. MemoryLog when nil.
	Log Log
	// Filter is the chat filter. An empty filter when nil.
	Filter *Filter
	// Limits are the chat rate limits.
	Limits Limits
	// Clock is the server clock. time.Now when nil.
	Clock func() time.Time

	mu       sync.Mutex
	online   map[string]*Member
	channels map[string]*Channel
	memory   *MemoryLog
}

// NewHub makes a hub with channels: core's records with the mods'.
func NewHub(channels []ChannelDef, filter *Filter) *Hub {
	h := &Hub{Filter: filter, Limits: DefaultLimits(), online: map[string]*Member{}, channels: map[string]*Channel{}}
	for _, d := range channels {
		h.channels[d.Name] = &Channel{Def: d, members: map[string]*Member{}}
	}
	return h
}

// ChatLog returns the hub's log: Log, or a MemoryLog it made.
func (h *Hub) ChatLog() Log {
	if h.Log != nil {
		return h.Log
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.memory == nil {
		h.memory = &MemoryLog{}
	}
	return h.memory
}

// Now is the server clock.
func (h *Hub) Now() time.Time {
	if h.Clock != nil {
		return h.Clock()
	}
	return time.Now()
}

// Key is the key of a character name: names are the same without regard
// to case, as in the database.
func Key(name string) string { return strings.ToLower(name) }

// Online adds a player that logged in, with its block list, and joins
// the channels with autojoin. A player of the same name that is still
// online, such as an older login, is replaced. It returns the joined
// channels.
func (h *Hub) Online(name string, characterID int64, inbox Inbox, mapName string, blocks []string) (*Member, []ChannelInfo) {
	m := &Member{name: name, key: Key(name), characterID: characterID, inbox: inbox, mapName: mapName,
		blocks: map[string]bool{}, channels: map[*Channel]time.Time{}, online: true}
	for _, b := range blocks {
		m.blocks[Key(b)] = true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if old := h.online[m.key]; old != nil {
		h.offline(old)
	}
	h.online[m.key] = m
	var joined []ChannelInfo
	for _, c := range h.sorted() {
		if c.Def.Autojoin && len(m.channels) < MaxChannels {
			h.join(c, m)
			joined = append(joined, c.info(true))
		}
	}
	return m, joined
}

// Offline removes a player that logged out from the hub and its
// channels. A member that a newer login replaced is left alone.
func (h *Hub) Offline(m *Member) {
	if m == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.offline(m)
}

func (h *Hub) offline(m *Member) {
	if !m.online {
		return
	}
	m.online = false
	if h.online[m.key] == m {
		delete(h.online, m.key)
	}
	for c := range m.channels {
		delete(c.members, m.key)
	}
	clear(m.channels)
}

// Move records that a player changed maps: its new inbox and map. It
// leaves the map channels, as rAthena's pc_setpos, and returns them.
func (h *Hub) Move(m *Member, inbox Inbox, mapName string) []ChannelInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	m.inbox, m.mapName = inbox, mapName
	var left []ChannelInfo
	for c := range m.channels {
		if c.Def.Kind == KindMap {
			h.leave(c, m)
			left = append(left, c.info(false))
		}
	}
	slices.SortFunc(left, func(a, b ChannelInfo) int { return cmp.Compare(a.Name, b.Name) })
	return left
}

// SetBlocks replaces a member's block list.
func (h *Hub) SetBlocks(m *Member, names []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clear(m.blocks)
	for _, n := range names {
		m.blocks[Key(n)] = true
	}
}

// Blocks reports whether a member blocks a name.
func (h *Hub) Blocks(m *Member, name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return m.blocks[Key(name)]
}

// IsOnline reports whether a character of the name is online, and its
// name as it is spelled.
func (h *Hub) IsOnline(name string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.online[Key(name)]; m != nil {
		return m.name, true
	}
	return "", false
}

// WhisperOutcome is what a whisper did.
type WhisperOutcome int

// The outcomes of Whisper.
const (
	WhisperSent WhisperOutcome = iota + 1
	WhisperNotOnline
	WhisperBlocked
)

// Whisper sends a line from a member to the player of a name, on any
// zone of the world, as clif_parse_WisMessage. It returns the target's
// name as it is spelled, and the outcome: the target is not online, or
// blocks the sender, as rAthena's ignore list.
func (h *Hub) Whisper(from *Member, line Line) (string, WhisperOutcome) {
	h.mu.Lock()
	defer h.mu.Unlock()
	to := h.online[Key(line.To)]
	if to == nil {
		return line.To, WhisperNotOnline
	}
	if to.blocks[from.key] {
		return to.name, WhisperBlocked
	}
	line.To = to.name
	to.inbox.DeliverChat(Delivery{Kind: DeliverWhisper, Line: line})
	return to.name, WhisperSent
}

// ChannelFailure is why a channel command did nothing.
type ChannelFailure int

// The failures of the channel calls. 0 is none.
const (
	ChannelUnknown ChannelFailure = iota + 1
	ChannelAlreadyJoined
	ChannelInviteOnly
	ChannelNotJoined
	ChannelCannotLeave
	ChannelTooMany
	// ChannelCannotChat, ChannelTooFast are refusals of Send.
	ChannelCannotChat
	ChannelTooFast
)

// Channel finds a running channel by name, with or without its #.
func (h *Hub) Channel(name string) *Channel {
	n, ok := NormalizeChannel(name)
	if !ok {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.channels[n]
}

// Join puts a member in a channel. force skips the join rule, for a
// mod's channel:add.
func (h *Hub) Join(m *Member, name string, force bool) (ChannelInfo, ChannelFailure) {
	n, ok := NormalizeChannel(name)
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.channels[n]
	switch {
	case !ok || c == nil || !m.online:
		return ChannelInfo{Name: "#" + n}, ChannelUnknown
	case c.members[m.key] == m:
		return c.info(true), ChannelAlreadyJoined
	case c.Def.Join == JoinInvite && !force:
		return c.info(false), ChannelInviteOnly
	case len(m.channels) >= MaxChannels:
		return c.info(false), ChannelTooMany
	}
	h.join(c, m)
	if force {
		m.inbox.DeliverChat(Delivery{Kind: DeliverJoined, Channel: c.info(true)})
	}
	return c.info(true), 0
}

func (h *Hub) join(c *Channel, m *Member) {
	c.members[m.key] = m
	m.channels[c] = time.Time{}
}

// Leave takes a member out of a channel. force skips can_leave, for a
// mod's channel:remove, and tells the player.
func (h *Hub) Leave(m *Member, name string, force bool) (ChannelInfo, ChannelFailure) {
	n, ok := NormalizeChannel(name)
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.channels[n]
	switch {
	case !ok || c == nil:
		return ChannelInfo{Name: "#" + n}, ChannelUnknown
	case c.members[m.key] != m:
		return c.info(false), ChannelNotJoined
	case !c.Def.CanLeave && !force:
		return c.info(true), ChannelCannotLeave
	}
	h.leave(c, m)
	if force {
		m.inbox.DeliverChat(Delivery{Kind: DeliverLeft, Channel: c.info(false)})
	}
	return c.info(false), 0
}

func (h *Hub) leave(c *Channel, m *Member) {
	delete(c.members, m.key)
	delete(m.channels, c)
}

// List returns the channels a member may join and those it is in, by
// name, as @channel list.
func (h *Hub) List(m *Member) []ChannelInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []ChannelInfo
	for _, c := range h.sorted() {
		_, in := m.channels[c]
		if in || c.Def.Join == JoinOpen {
			out = append(out, c.info(in))
		}
	}
	return out
}

// Members returns the names of a channel's members, sorted, for a
// member of the channel.
func (h *Hub) Members(m *Member, name string) ([]string, ChannelFailure) {
	n, ok := NormalizeChannel(name)
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.channels[n]
	switch {
	case !ok || c == nil:
		return nil, ChannelUnknown
	case c.members[m.key] != m:
		return nil, ChannelNotJoined
	}
	return memberNames(c), 0
}

func memberNames(c *Channel) []string {
	out := make([]string, 0, len(c.members))
	for _, m := range c.members {
		out = append(out, m.name)
	}
	slices.SortFunc(out, func(a, b string) int { return cmp.Compare(Key(a), Key(b)) })
	return out
}

// Send says a member's line in a channel, as channel_send: every member
// hears it, the sender too, except members who block the sender. Members
// of a map channel only hear the members on their map. The line's To is
// set to the channel's name.
func (h *Hub) Send(from *Member, name string, line Line, now time.Time) (ChannelInfo, ChannelFailure) {
	n, ok := NormalizeChannel(name)
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.channels[n]
	if !ok || c == nil {
		return ChannelInfo{Name: "#" + n}, ChannelUnknown
	}
	last, in := from.channels[c]
	switch {
	case !in:
		return c.info(false), ChannelNotJoined
	case !c.Def.CanChat:
		return c.info(true), ChannelCannotChat
	case c.Def.Delay > 0 && !last.IsZero() && now.Sub(last) < time.Duration(c.Def.Delay)*time.Millisecond:
		return c.info(true), ChannelTooFast
	}
	from.channels[c] = now
	line.To = c.Def.Full()
	h.deliver(c, from, line)
	return c.info(true), 0
}

// deliver hands a line to a channel's members. from is nil for a mod's
// line.
func (h *Hub) deliver(c *Channel, from *Member, line Line) {
	info := c.info(true)
	for _, m := range c.members {
		if from != nil && (m.blocks[from.key] || c.Def.Kind == KindMap && m.mapName != from.mapName) {
			continue
		}
		m.inbox.DeliverChat(Delivery{Kind: DeliverChannel, Line: line, Channel: info})
	}
}

// Errors of the channel calls of mods.
var (
	ErrChannelTaken = errors.New("another mod or core has a channel of that name")
	ErrChannelName  = fmt.Errorf("a channel name is 1 to %d letters, digits, _ and -", MaxChannelName)
)

// Create makes a mod's channel, owned by owner: one context of the mod
// in one zone. A channel of the same name that the same mod made is
// shared: its definition is replaced, and owner becomes one more owner.
func (h *Hub) Create(def ChannelDef, owner any) (*Channel, error) {
	n, ok := NormalizeChannel(def.Name)
	if !ok {
		return nil, ErrChannelName
	}
	def.Name = n
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.channels[n]
	switch {
	case c == nil:
		c = &Channel{Def: def, members: map[string]*Member{}, owners: map[any]bool{}}
		h.channels[n] = c
	case c.Def.Mod != def.Mod || def.Mod == "":
		return nil, ErrChannelTaken
	default:
		c.Def = def
	}
	c.owners[owner] = true
	return c, nil
}

// Release drops an owner of a mod's channel. The channel ends with its
// last owner: its members leave it.
func (h *Hub) Release(c *Channel, owner any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.owners == nil || !c.owners[owner] {
		return
	}
	delete(c.owners, owner)
	if len(c.owners) > 0 || h.channels[c.Def.Name] != c {
		return
	}
	delete(h.channels, c.Def.Name)
	info := c.info(false)
	for _, m := range c.members {
		delete(m.channels, c)
		m.inbox.DeliverChat(Delivery{Kind: DeliverLeft, Channel: info})
	}
	clear(c.members)
}

// Running reports whether a channel still runs.
func (h *Hub) Running(c *Channel) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.channels[c.Def.Name] == c
}

// Announce says a line of a channel itself, as rAthena's channelmes: its
// members hear it without a speaker.
func (h *Hub) Announce(c *Channel, text string, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.channels[c.Def.Name] != c {
		return
	}
	h.deliver(c, nil, Line{At: at, Kind: LineChannel, To: c.Def.Full(), Text: text})
}

// ChannelMembers returns the names of a channel's members, sorted.
func (h *Hub) ChannelMembers(c *Channel) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return memberNames(c)
}

// AddMember puts a member in a channel, for channel:add. It returns
// false when the member is already in it or in too many channels.
func (h *Hub) AddMember(c *Channel, m *Member) bool {
	_, f := h.Join(m, c.Def.Full(), true)
	return f == 0
}

// RemoveMember takes a member out of a channel, for channel:remove. It
// returns false when the member was not in it.
func (h *Hub) RemoveMember(c *Channel, m *Member) bool {
	_, f := h.Leave(m, c.Def.Full(), true)
	return f == 0
}

// sorted returns the channels by name. h.mu must be held.
func (h *Hub) sorted() []*Channel {
	out := make([]*Channel, 0, len(h.channels))
	for _, c := range h.channels {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b *Channel) int { return cmp.Compare(a.Def.Name, b.Def.Name) })
	return out
}

// AllowReport records a report of a member at now, and reports whether
// the member may send it: at most MaxReports in ReportWindow.
func (h *Hub) AllowReport(m *Member, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	m.reports = slices.DeleteFunc(m.reports, func(t time.Time) bool { return now.Sub(t) >= ReportWindow })
	if len(m.reports) >= MaxReports {
		return false
	}
	m.reports = append(m.reports, now)
	return true
}
