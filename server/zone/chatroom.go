package zone

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chatroom"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Chat rooms (proposal 0006, section 2; decision 0080), as rAthena's
// chat.cpp:
//   - A room lives in one zone and is not saved. server/chatroom holds
//     its state: members in join order, the kick list, the limits and
//     the trigger. This file wires it to players, NPCs, events and the
//     client.
//   - An NPC room (waitingroom) belongs to an NPC and goes with it. It
//     takes limit-1 players. A player room belongs to its first member.
//   - A member who walks, warps, dies or logs out leaves. While in a
//     room a player cannot use items or skills, trade, buy, sell, click
//     NPCs or attack, and speaks to the room on CHAT_CHANNEL_ROOM.
//   - Players near a room see its ChatRoomShow, which is sent again when
//     the room changes (roomEvents). Members get their own events at
//     once.
//   - The trigger runs the room's event on its NPC with no player, as
//     npc_event_do, after the join that reached it.

// room is a chat room of the zone.
type room struct {
	*chatroom.Room
	// npc is the NPC of an NPC room, or nil.
	npc *entity
	// mod is the mod whose script opened an NPC room: a plain event name
	// is that mod's record, then core's.
	mod string
	// ver changes whenever the room's bubble changes, so each viewer is
	// shown the new one once.
	ver uint32
}

// roomMessage texts, as rAthena's msg_txt 281 and 665.
const (
	msgNoChatMap  = "You can't create a chat room on this map."
	msgNoChatCell = "You can't create a chat room on this cell."
)

// roomRules are the chat room rules: the most users and the longest
// title.
func (z *Zone) roomRules() (maxUsers, maxTitle int) {
	r := z.worldRules()
	maxUsers, _ = r.Rule("chat_room_max_users")
	maxTitle, _ = r.Rule("chat_room_title")
	return maxUsers, maxTitle
}

// roomOwner returns the entity a room sits on: its NPC, or the first
// member's player. It is nil for a room without one.
func (z *Zone) roomOwner(r *room) *entity {
	if r.npc != nil {
		return r.npc
	}
	m, ok := r.Owner()
	if !ok {
		return nil
	}
	if p := z.players[uint32(m.ID)]; p != nil { // #nosec G115 -- IDs of this zone are uint32.
		return p.entity
	}
	return nil
}

// roomPlayers are a room's members in join order.
func (z *Zone) roomPlayers(r *room) []*Player {
	var out []*Player
	for _, m := range r.Members() {
		if p := z.players[uint32(m.ID)]; p != nil { // #nosec G115 -- IDs of this zone are uint32.
			out = append(out, p)
		}
	}
	return out
}

// roomSend adds an event to each member's next frame, except one.
func (z *Zone) roomSend(r *room, ev *wathenav1.Event, except *Player) {
	for _, m := range z.roomPlayers(r) {
		if m != except {
			m.pending = append(m.pending, ev)
		}
	}
}

func roomResult(o wathenav1.ChatRoomOutcome) *wathenav1.Event {
	return wathenav1.Event_builder{ChatRoomResult: wathenav1.ChatRoomResult_builder{Outcome: o}.Build()}.Build()
}

// roomJoined is the member's own view of the room.
func (z *Zone) roomJoined(r *room) *wathenav1.Event {
	b := wathenav1.ChatRoomJoined_builder{Room: r.ID, Title: r.Title, Limit: uint32(r.Limit), Public: r.Public} // #nosec G115 -- 1 to 20.
	for _, m := range r.Members() {
		b.Members = append(b.Members, m.Name)
	}
	if o, ok := r.Owner(); ok {
		b.Owner = o.Name
	}
	return wathenav1.Event_builder{ChatRoomJoined: b.Build()}.Build()
}

func roomMember(r *room, name string, change wathenav1.ChatRoomChange) *wathenav1.Event {
	return wathenav1.Event_builder{ChatRoomMember: wathenav1.ChatRoomMember_builder{
		Room: r.ID, Name: name, Change: change, Title: r.Title, Limit: uint32(r.Limit), Public: r.Public, // #nosec G115 -- 1 to 20.
	}.Build()}.Build()
}

// roomEvents are the bubbles a player sees this tick: a ChatRoomShow for
// each room in view that the player was not shown in its current state,
// and a ChatRoomClear for each one it was shown that is gone or out of
// view.
func (z *Zone) roomEvents(p *Player) []*wathenav1.Event {
	if len(z.rooms) == 0 && len(p.roomSeen) == 0 {
		return nil
	}
	if p.fresh {
		// The client forgot every entity: the bubbles too.
		clear(p.roomSeen)
	}
	var events []*wathenav1.Event
	for _, id := range z.roomIDs {
		r := z.rooms[id]
		net, ok := z.roomViewed(p, r)
		if !ok || p.roomSeen[id] == r.ver+1 {
			continue
		}
		if p.roomSeen == nil {
			p.roomSeen = map[uint32]uint32{}
		}
		p.roomSeen[id] = r.ver + 1
		// #nosec G115 -- limits are 1 to 20, levels and zeny are bounded by the rules.
		events = append(events, wathenav1.Event_builder{ChatRoomShow: wathenav1.ChatRoomShow_builder{
			Room: r.ID, Owner: net, Title: r.Title, Users: uint32(r.Users()), Limit: uint32(r.Limit), Public: r.Public, Npc: r.NPC,
			MinLevel: uint32(min(r.MinLevel, 1<<20)), MaxLevel: uint32(min(r.MaxLevel, 1<<20)), Zeny: uint32(r.Zeny),
		}.Build()}.Build())
	}
	if len(p.roomSeen) == 0 {
		return events
	}
	var gone []uint32
	for id := range p.roomSeen {
		if r := z.rooms[id]; r == nil {
			gone = append(gone, id)
		} else if _, ok := z.roomViewed(p, r); !ok {
			gone = append(gone, id)
		}
	}
	slices.Sort(gone)
	for _, id := range gone {
		delete(p.roomSeen, id)
		events = append(events, wathenav1.Event_builder{ChatRoomClear: wathenav1.ChatRoomClear_builder{Room: id}.Build()}.Build())
	}
	return events
}

// roomViewed reports whether a player sees a room's owner, and the
// owner's ID for that player.
func (z *Zone) roomViewed(p *Player, r *room) (uint32, bool) {
	owner := z.roomOwner(r)
	me := p.entity
	if owner == nil || abs(owner.x-me.x) > ViewRange || abs(owner.y-me.y) > ViewRange {
		return 0, false
	}
	return p.netIDs.get(owner)
}

// newRoom adds a room to the zone.
func (z *Zone) newRoom(npc bool) *room {
	if z.rooms == nil {
		z.rooms = map[uint32]*room{}
	}
	z.roomLast = chatroom.NextID(roomsOf(z.rooms), z.roomLast)
	r := &room{Room: chatroom.New(z.roomLast, npc)}
	z.rooms[r.ID] = r
	k, _ := slices.BinarySearch(z.roomIDs, r.ID)
	z.roomIDs = slices.Insert(z.roomIDs, k, r.ID)
	return r
}

// roomsOf is the rooms' state by ID, for chatroom.NextID.
func roomsOf(m map[uint32]*room) map[uint32]*chatroom.Room {
	out := make(map[uint32]*chatroom.Room, len(m))
	for id, r := range m {
		out[id] = r.Room
	}
	return out
}

// roomEvent queues a chat_room_join or chat_room_leave event.
func (z *Zone) roomEvent(name string, r *room, p *Player, kicked, gone bool) {
	e := modscript.ChatRoomEvent{Player: int64(p.charID), Name: p.Name, Title: r.Title, NPC: r.NPC, Users: r.Users(), Kicked: kicked}
	if !gone {
		e.Room = int64(r.ID)
	}
	z.scriptEventFor(name, modscript.ChatRoomEventArg(e))
}

// joinPlayer puts a player that passed the checks into a room: it stops,
// gets the room and the others hear of it, the bubble changes and the
// trigger runs.
func (z *Zone) joinPlayer(r *room, p *Player) {
	z.stopForRoom(p)
	p.room = r
	r.ver++
	p.pending = append(p.pending, roomResult(wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_OK), z.roomJoined(r))
	z.roomSend(r, roomMember(r, p.Name, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_JOIN), p)
	z.roomEvent("chat_room_join", r, p, false, false)
	z.roomTrigger(r)
}

// stopForRoom is unit_stop_walking and unit_stop_attack.
func (z *Zone) stopForRoom(p *Player) {
	e := p.entity
	e.path = e.path[:0]
	e.target, e.pickItem, e.pending = nil, nil, nil
}

// leaveRoom makes a player leave its room, as chat_leavechat: a change of
// LEAVE or KICKED. The first remaining member of a player room becomes
// its owner, and an empty player room is deleted. An NPC room stays.
func (z *Zone) leaveRoom(p *Player, change wathenav1.ChatRoomChange) {
	r := p.room
	if r == nil {
		return
	}
	p.room = nil
	owner, empty, ok := r.Leave(int64(p.charID))
	if !ok {
		return
	}
	r.ver++
	kicked := change == wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_KICKED
	// The leaver is told too (clif_chat_leave), then the others.
	p.pending = append(p.pending, roomMember(r, p.Name, change))
	z.roomSend(r, roomMember(r, p.Name, change), nil)
	if empty {
		z.dropRoom(r)
		z.roomEvent("chat_room_leave", r, p, kicked, true)
		return
	}
	if owner != nil {
		z.roomSend(r, roomMember(r, owner.Name, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_OWNER), nil)
	}
	z.roomEvent("chat_room_leave", r, p, kicked, false)
}

// kickAll is chat_npckickall: the last member leaves first.
func (z *Zone) kickAll(r *room) int {
	n := 0
	for {
		ps := z.roomPlayers(r)
		if len(ps) == 0 {
			return n
		}
		z.leaveRoom(ps[len(ps)-1], wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_LEAVE)
		n++
	}
}

// deleteRoom removes a room: every member leaves.
func (z *Zone) deleteRoom(r *room) {
	z.kickAll(r)
	z.dropRoom(r)
}

// dropRoom takes a room out of the zone's lists.
func (z *Zone) dropRoom(r *room) {
	delete(z.rooms, r.ID)
	if k, ok := slices.BinarySearch(z.roomIDs, r.ID); ok {
		z.roomIDs = slices.Delete(z.roomIDs, k, k+1)
	}
}

// roomOfNPC is an NPC's room, or nil.
func (z *Zone) roomOfNPC(e *entity) *room {
	for _, r := range z.rooms {
		if r.npc == e {
			return r
		}
	}
	return nil
}

// closeNPCRoom deletes the room of an NPC that goes, as npc_unload does.
func (z *Zone) closeNPCRoom(e *entity) {
	if r := z.roomOfNPC(e); r != nil {
		z.deleteRoom(r)
	}
}

// roomTrigger is chat_triggerevent: when users reach the trigger, the
// room's event runs on its NPC with no player.
func (z *Zone) roomTrigger(r *room) {
	if !r.ShouldRun() {
		return
	}
	name, label, ok := cutLast(r.Event, "::")
	if !ok || label == "" {
		return
	}
	label = strings.ToLower(label)
	if r.npc != nil && (name == "" || slices.Contains(fullNames(name, r.mod), r.npc.npc.Name)) {
		// The room's own NPC runs it in this zone.
		z.postNPC(z, npcMessage{kind: npcEventMessage, handle: r.npc.npc.handle, label: label})
		return
	}
	d := z.npcDir()
	if d == nil {
		return
	}
	if en := d.find(name, r.mod); en != nil {
		z.postNPC(en.zone, npcMessage{kind: npcEventMessage, handle: en.handle, label: label})
	}
}

func cutLast(s, sep string) (before, after string, ok bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

// roomGuard takes from a command of a member what a member cannot do:
// walking leaves the room first, and items, skills, trades, shops, NPC
// clicks and attacks are refused.
func (z *Zone) roomGuard(p *Player, c *command) {
	if c.walkTo != nil {
		z.leaveRoom(p, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_LEAVE)
		return
	}
	c.use, c.useSkill, c.attack, c.npcClick, c.pickUp = nil, nil, nil, nil, nil
	c.trade, c.shopBuy, c.shopSell, c.vending, c.buying = nil, nil, nil, nil, nil
}

// roomSay sends a line to the room of the speaker. A player that blocks
// the speaker does not get it.
func (z *Zone) roomSay(p *Player, text string) {
	r := p.room
	if r == nil {
		return
	}
	ev := wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_ROOM, FromName: p.Name, Text: text,
	}.Build()}.Build()
	for _, m := range z.roomPlayers(r) {
		if m != p && m.blocking(p.Name) {
			continue
		}
		m.pending = append(m.pending, ev)
	}
}

// chatRoomCommand is a ChatRoomCommand of the client.
func (z *Zone) chatRoomCommand(p *Player, c *wathenav1.ChatRoomCommand) {
	switch c.GetAction() {
	case wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_CREATE:
		z.createPlayerRoom(p, c)
	case wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_JOIN:
		z.joinRoom(p, c.GetRoom(), c.GetPassword())
	case wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_LEAVE:
		z.leaveRoom(p, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_LEAVE)
	case wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_CHANGE:
		z.changeRoom(p, c)
	case wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_OWNER:
		z.handOverRoom(p, c.GetName())
	case wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_KICK:
		z.kickFromRoom(p, c.GetName())
	}
}

// roomValues checks the title, password and limit a client asked for.
func (z *Zone) roomValues(c *wathenav1.ChatRoomCommand) bool {
	maxUsers, maxTitle := z.roomRules()
	title := strings.TrimSpace(c.GetTitle())
	n := utf8.RuneCountInString(title)
	return n >= 1 && n <= maxTitle && utf8.RuneCountInString(c.GetPassword()) <= chatroom.MaxPassword &&
		c.GetLimit() >= 1 && int(c.GetLimit()) <= maxUsers && utf8.ValidString(title) && utf8.ValidString(c.GetPassword())
}

// createPlayerRoom is chat_createpcchat with the checks of proposal 0006:
// no room yet, no store, Basic Skill level 4, no mute, a map without
// nochat, a cell without no_chat and no NPC within
// min_npc_vendchat_distance.
func (z *Zone) createPlayerRoom(p *Player, c *wathenav1.ChatRoomCommand) {
	refuse := func(text string) {
		if text != "" {
			p.pending = append(p.pending, systemMessage(text))
		}
		p.pending = append(p.pending, roomResult(wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_NOT_ALLOWED))
	}
	e := p.entity
	switch {
	case p.room != nil || p.hasStore():
		refuse("")
		return
	case z.mapFlag("nochat"):
		refuse(msgNoChatMap)
		return
	case z.cellMark(e.x, e.y, markNoChat):
		refuse(msgNoChatCell)
		return
	case rules.ItemsOrDefault(z.worldRules()).BasicSkillCheck && p.skillLevel(skillBasic) < 4,
		p.sanction(character.SanctionMute, z.chatHub().Now()) != nil,
		z.npcNear(e), !z.roomValues(c):
		refuse("")
		return
	}
	r := z.newRoom(false)
	r.Title, r.Password, r.Public = strings.TrimSpace(c.GetTitle()), c.GetPassword(), c.GetPublic()
	r.SetLimit(int(c.GetLimit()))
	r.Add(chatroom.Member{ID: int64(p.charID), Name: p.Name})
	z.stopForRoom(p)
	p.room = r
	r.ver++
	p.pending = append(p.pending, roomResult(wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_OK), z.roomJoined(r))
	z.roomEvent("chat_room_join", r, p, false, false)
}

var outcomes = map[chatroom.Outcome]wathenav1.ChatRoomOutcome{
	chatroom.Full:          wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_FULL,
	chatroom.WrongPassword: wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_WRONG_PASSWORD,
	chatroom.LevelLow:      wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_LEVEL_LOW,
	chatroom.LevelHigh:     wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_LEVEL_HIGH,
	chatroom.NoZeny:        wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_NO_ZENY,
	chatroom.Kicked:        wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_KICKED,
}

// joinOutcome is chat_joinchat's checks and the join itself.
func (z *Zone) joinOutcome(p *Player, r *room, password string) chatroom.Outcome {
	if r == nil {
		return chatroom.Full
	}
	return r.Join(chatroom.Joiner{
		Member: chatroom.Member{ID: int64(p.charID), Name: p.Name},
		Level:  p.progress.BaseLevel, Zeny: p.record.Zeny, Busy: p.hasStore(), InRoom: p.room != nil,
		Any: z.hasStaffRight(p, "chat.join_all"),
	}, password)
}

// joinRoom is chat_joinchat.
func (z *Zone) joinRoom(p *Player, id uint32, password string) {
	r := z.rooms[id]
	if o := z.joinOutcome(p, r, password); o != chatroom.OK {
		p.pending = append(p.pending, roomResult(outcomes[o]))
		return
	}
	z.joinPlayer(r, p)
}

// changeRoom is chat_changechatstatus.
func (z *Zone) changeRoom(p *Player, c *wathenav1.ChatRoomCommand) {
	r := p.room
	if r == nil || !z.roomValues(c) {
		return
	}
	if !r.Change(int64(p.charID), strings.TrimSpace(c.GetTitle()), c.GetPassword(), int(c.GetLimit()), c.GetPublic()) {
		return
	}
	r.ver++
	z.roomSend(r, roomMember(r, p.Name, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_STATUS), nil)
}

// handOverRoom is chat_changechatowner.
func (z *Zone) handOverRoom(p *Player, name string) {
	r := p.room
	if r == nil || !r.Transfer(int64(p.charID), name) {
		return
	}
	r.ver++
	z.roomSend(r, roomMember(r, name, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_OWNER), nil)
}

// kickFromRoom is chat_kickchat. A member with the staff right
// chat.no_kick stays.
func (z *Zone) kickFromRoom(p *Player, name string) {
	r := p.room
	if r == nil {
		return
	}
	m, ok := r.Kick(int64(p.charID), name, func(m chatroom.Member) bool {
		t := z.players[uint32(m.ID)] // #nosec G115 -- IDs of this zone are uint32.
		return t != nil && z.hasStaffRight(t, "chat.no_kick")
	})
	if !ok {
		return
	}
	if t := z.players[uint32(m.ID)]; t != nil { // #nosec G115 -- IDs of this zone are uint32.
		z.leaveRoom(t, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_KICKED)
	}
}

// hasStaffRight reports whether a player's staff role has a permission.
func (z *Zone) hasStaffRight(p *Player, permission string) bool {
	role := z.staffRole(p)
	return role != "" && z.Staff.Roles().Has(role, permission)
}

// RoomChat says a line to the room the player is in.
func (s *Session) RoomChat(seq uint32, text string) bool {
	return s.send(command{seq: seq, chat: &chatCommand{kind: chatRoom, text: text}})
}

// ChatRoom is a room action of the client: the gateway checked the sizes.
func (s *Session) ChatRoom(seq uint32, c *wathenav1.ChatRoomCommand) bool {
	return s.send(command{seq: seq, chatRoom: c})
}

// The scripts' side: waitingroom and its siblings (proposal 0006,
// section 12).

// npcRoom reads a local NPC's room.
func (h scriptHost) npcRoom(id int64) (*entity, *room, error) {
	e, err := h.npc(id)
	if err != nil {
		return nil, nil, err
	}
	return e, h.z.roomOfNPC(e), nil
}

// OpenChatRoom is npc:open_chat_room, as waitingroom: an NPC that has a
// room already keeps it, and the call returns 0.
func (h scriptHost) OpenChatRoom(npc int64, s modscript.RoomSpec) (int64, error) {
	e, r, err := h.npcRoom(npc)
	if err != nil {
		return 0, err
	}
	z := h.z
	if r != nil {
		if z.Mods != nil && z.Mods.Log != nil {
			z.Mods.Log.Warn("npc:open_chat_room: the NPC has a chat room already", "npc", e.npc.Name, "map", z.Map)
		}
		return 0, nil
	}
	maxUsers, maxTitle := z.roomRules()
	if s.MinLevel == 0 {
		s.MinLevel = 1
	}
	if s.MaxLevel == 0 {
		s.MaxLevel = progression.MaxLevel
	}
	if s.Trigger < 0 {
		s.Trigger = s.Limit
	}
	if s.Zeny > character.MaxZeny || s.MaxLevel > progression.MaxLevel {
		// chat_createnpcchat logs this and makes no room.
		return 0, errors.New("the zeny fee or the level range is over the game's cap")
	}
	// safestrncpy cuts a title to the title size.
	if rs := []rune(s.Title); len(rs) > maxTitle {
		s.Title = string(rs[:maxTitle])
	}
	r = z.newRoom(true)
	r.npc, r.mod = e, s.Mod
	r.Title, r.Public, r.Event = s.Title, true, s.Event
	r.SetLimit(min(s.Limit, maxUsers))
	r.SetTrigger(s.Trigger)
	r.Zeny, r.MinLevel, r.MaxLevel = s.Zeny, s.MinLevel, s.MaxLevel
	return int64(r.ID), nil
}

// CloseChatRoom is npc:close_chat_room, as delwaitingroom. An NPC of
// another map closes its room in its zone after the current step.
func (h scriptHost) CloseChatRoom(npc int64) (bool, error) {
	t, err := h.npcTarget(npc)
	if err != nil {
		return false, err
	}
	if t.local == nil {
		h.z.postNPC(t.entry.zone, npcMessage{kind: npcPresentMessage, handle: t.entry.handle, run: func(z *Zone, e *entity) {
			z.closeNPCRoom(e)
		}})
		return true, nil
	}
	r := h.z.roomOfNPC(t.local)
	if r == nil {
		return false, nil
	}
	h.z.deleteRoom(r)
	return true, nil
}

// NPCChatRoom is npc:chat_room: the room's ID, or 0. An NPC of another
// map has no room to this zone.
func (h scriptHost) NPCChatRoom(npc int64) (int64, error) {
	t, err := h.npcTarget(npc)
	if err != nil {
		return 0, err
	}
	if t.local == nil {
		return 0, nil
	}
	if r := h.z.roomOfNPC(t.local); r != nil {
		return int64(r.ID), nil
	}
	return 0, nil
}

// room finds a room by the script's handle.
func (h scriptHost) room(id int64) (*room, error) {
	r := h.z.rooms[uint32(id)] // #nosec G115 -- handles hold room IDs.
	if id <= 0 || r == nil {
		return nil, errors.New("the chat room is gone")
	}
	return r, nil
}

// roomFields are the fields of wa.ChatRoom.
var roomFields = map[string]func(z *Zone, r *room) script.Value{
	"title":            func(_ *Zone, r *room) script.Value { return script.String(r.Title) },
	"limit":            func(_ *Zone, r *room) script.Value { return intValue(r.Limit) },
	"trigger":          func(_ *Zone, r *room) script.Value { return intValue(r.Trigger) },
	"trigger_disabled": func(_ *Zone, r *room) script.Value { return script.Bool(r.Disabled) },
	"event":            func(_ *Zone, r *room) script.Value { return script.String(r.Event) },
	"users":            func(_ *Zone, r *room) script.Value { return intValue(r.Users()) },
	"password":         func(_ *Zone, r *room) script.Value { return script.String(r.Password) },
	"public":           func(_ *Zone, r *room) script.Value { return script.Bool(r.Public) },
	"full":             func(_ *Zone, r *room) script.Value { return script.Bool(r.Full()) },
	"triggered":        func(_ *Zone, r *room) script.Value { return script.Bool(r.Triggered()) },
	"owner": func(z *Zone, r *room) script.Value {
		if m, ok := r.Owner(); ok {
			return script.Int(int64(m.ID))
		}
		return script.Nil
	},
	"npc": func(_ *Zone, r *room) script.Value { return script.Bool(r.NPC) },
}

// ChatRoomField reads a field of a wa.ChatRoom.
func (h scriptHost) ChatRoomField(id int64, field string) (script.Value, error) {
	r, err := h.room(id)
	if err != nil {
		return script.Nil, err
	}
	read, ok := roomFields[field]
	if !ok {
		return script.Nil, errors.New("chat room has no field " + field)
	}
	return read(h.z, r), nil
}

// ChatRoomMembers lists the members' player IDs in join order.
func (h scriptHost) ChatRoomMembers(id int64) ([]int64, error) {
	r, err := h.room(id)
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, m := range r.Members() {
		out = append(out, m.ID)
	}
	return out, nil
}

// ChatRoomEvent is enablewaitingroomevent and disablewaitingroomevent.
// Enabling runs the trigger check at once.
func (h scriptHost) ChatRoomEvent(id int64, enable bool) error {
	r, err := h.room(id)
	if err != nil {
		return err
	}
	r.Disabled = !enable
	if enable {
		h.z.roomTrigger(r)
	}
	return nil
}

// ChatRoomKick is waitingroomkick: the member leaves.
func (h scriptHost) ChatRoomKick(id int64, name string) (bool, error) {
	r, err := h.room(id)
	if err != nil {
		return false, err
	}
	m, ok := r.Find(name)
	if !ok {
		return false, nil
	}
	if t := h.z.players[uint32(m.ID)]; t != nil { // #nosec G115 -- IDs of this zone are uint32.
		h.z.leaveRoom(t, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_KICKED)
	}
	return true, nil
}

// ChatRoomKickAll is kickwaitingroomall.
func (h scriptHost) ChatRoomKickAll(id int64) (int, error) {
	r, err := h.room(id)
	if err != nil {
		return 0, err
	}
	return h.z.kickAll(r), nil
}

// ChatRoomWarp is warpwaitingpc: the first n members in join order are
// warped, each paying the room's fee, and the loop stops at the first
// that cannot pay or cannot be warped. It returns the players that were
// warped. map is "Random" for a random cell of the room's map, and
// "SavePoint" for each player's save point.
func (h scriptHost) ChatRoomWarp(id int64, mapName string, x, y, n int) ([]int64, error) {
	r, err := h.room(id)
	if err != nil {
		return nil, err
	}
	z := h.z
	var warped []int64
	if where := strings.TrimPrefix(mapName, "map/"); mapName != "Random" && mapName != "SavePoint" && z.zoneOf("map/"+where) == nil {
		// A map that does not run stops the loop before anyone pays.
		return nil, nil
	}
	for i := 0; i < n && r.Users() > 0; i++ {
		ps := z.roomPlayers(r)
		if len(ps) == 0 {
			break
		}
		p := ps[0]
		if r.Zeny > 0 {
			if p.record.Zeny < r.Zeny {
				break
			}
			z.payZeny(p, r.Zeny, transfer.ActionTake, transfer.End(transfer.KindScript))
		}
		warped = append(warped, int64(p.charID))
		// pc_setpos leaves the room first.
		z.leaveRoom(p, wathenav1.ChatRoomChange_CHAT_ROOM_CHANGE_LEAVE)
		w := z.roomDestination(p, mapName, x, y)
		if w == nil {
			// A map that does not run leaves the player where it is.
			continue
		}
		p.warpTo = w
		z.later(func() { z.afterTalk(p) })
	}
	return warped, nil
}

// roomDestination is where warpwaitingpc sends a player: a cell of a map,
// "Random" for a random cell of the player's map, or "SavePoint" for its
// save point. The map flag noteleport, which rAthena checks for the save
// point, does not exist yet.
func (z *Zone) roomDestination(p *Player, mapName string, x, y int) *scriptWarp {
	switch mapName {
	case "Random":
		if cx, cy, ok := z.randomCell(); ok {
			return &scriptWarp{to: z, x: cx, y: cy, dead: true}
		}
	case "SavePoint":
		if to, sx, sy := z.savePoint(p); to != nil {
			return &scriptWarp{to: to, x: sx, y: sy, dead: true}
		}
	default:
		if to := z.zoneOf("map/" + strings.TrimPrefix(mapName, "map/")); to != nil {
			return &scriptWarp{to: to, x: x, y: y, dead: true}
		}
	}
	return nil
}

// OpenPlayerRoom opens a player room for a test, as the client's create
// without its checks of the map and the skill, and joins the members
// through the join checks. It returns the room's ID.
func (h scriptHost) OpenPlayerRoom(owner int64, title, password string, limit int, public bool, members []int64) (int64, error) {
	z := h.z
	p, err := h.player(owner)
	if err != nil {
		return 0, err
	}
	if p.room != nil {
		return 0, errors.New("the player is in a chat room already")
	}
	c := wathenav1.ChatRoomCommand_builder{Title: title, Password: password, Limit: uint32(max(limit, 0)), Public: public}.Build() // #nosec G115 -- clamped.
	if !z.roomValues(c) {
		return 0, errors.New("a chat room has a title of 1 to 36 characters, a password of at most 8 and a limit of 1 to 20")
	}
	r := z.newRoom(false)
	r.Title, r.Password, r.Public = title, password, public
	r.SetLimit(limit)
	r.Add(chatroom.Member{ID: int64(p.charID), Name: p.Name})
	z.stopForRoom(p)
	p.room = r
	r.ver++
	p.pending = append(p.pending, roomResult(wathenav1.ChatRoomOutcome_CHAT_ROOM_OUTCOME_OK), z.roomJoined(r))
	z.roomEvent("chat_room_join", r, p, false, false)
	for _, id := range members {
		m, err := h.player(id)
		if err != nil {
			return 0, err
		}
		if o := z.joinOutcome(m, r, password); o != chatroom.OK {
			return 0, errors.New("a member could not join: " + outcomes[o].String())
		}
		z.joinPlayer(r, m)
	}
	return int64(r.ID), nil
}

var _ modscript.ChatRoomHost = scriptHost{}
