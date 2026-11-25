package modscript

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Chat rooms (proposal 0006, section 2 and 12; decision 0080): NPC rooms,
// as rAthena's waitingroom and its siblings, and the handles of any
// room. Reads need game.read. Changes need chat_rooms.manage, and
// npcs.control for an NPC of another mod. A room handle's ID is the
// room's ID in its zone, and it fails once the room is gone.

// RoomSpec is npc:open_chat_room's table.
type RoomSpec struct {
	Title string
	Limit int
	// Event is "NPC name::label", or "" for none. A name without a mod is
	// Mod's record, then core's.
	Event string
	Mod   string
	// Trigger is the number of users that run the event, or -1 for the
	// limit.
	Trigger int
	Zeny    int64
	// MinLevel and MaxLevel bound the joiner's base level. 0 is the
	// default: 1, and the level cap.
	MinLevel, MaxLevel int
}

// ChatRoomHost is a Host with chat rooms. A Host without it raises an
// error on these calls.
type ChatRoomHost interface {
	// OpenChatRoom opens the room of an NPC of this zone. It returns the
	// room's ID, or 0 when the NPC has a room already.
	OpenChatRoom(npc int64, s RoomSpec) (int64, error)
	// CloseChatRoom deletes an NPC's room and empties it. It reports
	// whether the NPC had one.
	CloseChatRoom(npc int64) (bool, error)
	// NPCChatRoom returns an NPC's room ID, or 0.
	NPCChatRoom(npc int64) (int64, error)
	// ChatRoomField reads a field of a room handle.
	ChatRoomField(room int64, field string) (script.Value, error)
	// ChatRoomMembers lists a room's players in join order.
	ChatRoomMembers(room int64) ([]int64, error)
	// ChatRoomEvent enables or disables the room's trigger. Enabling
	// runs the check at once.
	ChatRoomEvent(room int64, enable bool) error
	// ChatRoomKick makes a member leave, by name.
	ChatRoomKick(room int64, name string) (bool, error)
	// ChatRoomKickAll empties the room and returns how many left.
	ChatRoomKickAll(room int64) (int, error)
	// ChatRoomWarp warps the first n members in join order, each paying
	// the room's fee, and returns the players it warped.
	ChatRoomWarp(room int64, mapName string, x, y, n int) ([]int64, error)
	// OpenPlayerRoom opens a player room for a test and joins members.
	OpenPlayerRoom(owner int64, title, password string, limit int, public bool, members []int64) (int64, error)
}

// ChatRoomEvent is the object of the chat_room_join and chat_room_leave
// events.
type ChatRoomEvent struct {
	Player int64
	// Name is the player's name, which stays when the player has left the
	// map by the time the handler runs.
	Name string
	// Room is the room's ID, or 0 when the room is gone.
	Room  int64
	Title string
	NPC   bool
	// Users is the number of members after the change.
	Users  int
	Kicked bool
}

// ChatRoomEventArg is the argument of a chat room event, which becomes a
// wa.ChatRoomEvent.
func ChatRoomEventArg(e ChatRoomEvent) Arg {
	fields := []script.Field{script.KV("player", script.Int(e.Player)), script.KV("name", script.String(e.Name)), script.KV("title", script.String(e.Title)),
		script.KV("npc", script.Bool(e.NPC)), script.KV("users", script.Int(int64(e.Users))), script.KV("kicked", script.Bool(e.Kicked))}
	if e.Room != 0 {
		fields = append(fields, script.KV("room", script.Int(e.Room)))
	}
	return Arg{"chat_room_event", script.NewTable(fields...)}
}

func (r *Runtime) chatRoomHost(what string) (ChatRoomHost, error) {
	h, ok := r.host.(ChatRoomHost)
	if !ok {
		return nil, errors.New(what + ": chat rooms do not exist here")
	}
	return h, nil
}

// needRooms checks that a call may change rooms: not in a computed
// bonus, and with chat_rooms.manage. A test case's own code needs no
// permission.
func (r *Runtime) needRooms(what string) error {
	if err := r.mutating(what); err != nil {
		return err
	}
	if r.opts.Testing && r.current == nil {
		return nil
	}
	return r.need(what, "chat_rooms.manage")
}

// chatRoomOpen is npc:open_chat_room. Arguments: the NPC, title, limit,
// event, trigger, zeny, min level and max level.
func (r *Runtime) chatRoomOpen(c *script.Call) error {
	const what = "npc:open_chat_room"
	id, err := npcID(c)
	if err != nil {
		return err
	}
	if err := r.needRooms(what); err != nil {
		return err
	}
	h, err := r.chatRoomHost(what)
	if err != nil {
		return err
	}
	if w, ok := r.host.(NPCWorldHost); ok {
		if err := r.controls(what, w, id); err != nil {
			return err
		}
	}
	title, ok := c.Arg(1).AsString()
	if !ok || title == "" || len(title) > 144 {
		return errors.New(what + ": the title is 1 to 36 characters")
	}
	spec := RoomSpec{Title: title, Event: c.String(3), Mod: r.actor()}
	var v [5]int64
	for i, k := range []int{2, 4, 5, 6, 7} {
		v[i], _ = c.Arg(k).AsInt()
	}
	if v[0] < 0 || v[0] > 1<<20 || v[1] < -1 || v[1] > 1<<20 || v[2] < 0 || v[3] < 0 || v[4] < 0 || v[3] > 1<<20 || v[4] > 1<<20 {
		return errors.New(what + ": limit is 0 or more, trigger 0 or more, zeny and the levels are not negative")
	}
	spec.Limit, spec.Trigger, spec.Zeny, spec.MinLevel, spec.MaxLevel = int(v[0]), int(v[1]), v[2], int(v[3]), int(v[4])
	room, err := h.OpenChatRoom(id, spec)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if room == 0 {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(room))
	return nil
}

// chatRoomClose is npc:close_chat_room.
func (r *Runtime) chatRoomClose(c *script.Call) error {
	const what = "npc:close_chat_room"
	id, err := npcID(c)
	if err != nil {
		return err
	}
	if err := r.needRooms(what); err != nil {
		return err
	}
	h, err := r.chatRoomHost(what)
	if err != nil {
		return err
	}
	if w, ok := r.host.(NPCWorldHost); ok {
		if err := r.controls(what, w, id); err != nil {
			return err
		}
	}
	ok, err := h.CloseChatRoom(id)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	c.Return(script.Bool(ok))
	return nil
}

// npcChatRoom is npc:chat_room: the room's ID, or nil.
func (r *Runtime) npcChatRoom(c *script.Call) error {
	const what = "npc:chat_room"
	id, err := npcID(c)
	if err != nil {
		return err
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.chatRoomHost(what)
	if err != nil {
		return err
	}
	room, err := h.NPCChatRoom(id)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if room == 0 {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(room))
	return nil
}

func roomID(c *script.Call, what string) (int64, error) {
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return 0, errors.New(what + ": not a chat room: read fields of a wa.ChatRoom, such as room.title")
	}
	return id, nil
}

// chatRoomField is a read of a wa.ChatRoom field.
func (r *Runtime) chatRoomField(c *script.Call) error {
	const what = "reading a chat room"
	id, err := roomID(c, what)
	if err != nil {
		return err
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.chatRoomHost(what)
	if err != nil {
		return err
	}
	v, err := h.ChatRoomField(id, c.String(1))
	if err != nil {
		return err
	}
	if c.String(1) == "password" && (!r.opts.Testing || r.current != nil) && r.need(what, "chat_rooms.manage") != nil {
		// A player room's password is for a mod that manages rooms.
		v = script.String("")
	}
	c.Return(v)
	return nil
}

// chatRoomCall runs a method of wa.ChatRoom. Arguments: the room, the
// method's name, then its own: members; enable_event and disable_event;
// kick (name); kick_all; warp_members (map, x, y, n).
func (r *Runtime) chatRoomCall(c *script.Call) error {
	call := c.String(1)
	what := "chat room:" + call + "()"
	id, err := roomID(c, what)
	if err != nil {
		return err
	}
	h, err := r.chatRoomHost(what)
	if err != nil {
		return err
	}
	if call == "members" {
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		ids, err := h.ChatRoomMembers(id)
		if err != nil {
			return err
		}
		c.Return(intList(ids))
		return nil
	}
	if err := r.needRooms(what); err != nil {
		return err
	}
	switch call {
	case "enable_event", "disable_event":
		return h.ChatRoomEvent(id, call == "enable_event")
	case "kick":
		name, ok := c.Arg(2).AsString()
		if !ok {
			return errors.New(what + ": want a player's name")
		}
		ok, err := h.ChatRoomKick(id, name)
		if err != nil {
			return err
		}
		c.Return(script.Bool(ok))
	case "kick_all":
		n, err := h.ChatRoomKickAll(id)
		if err != nil {
			return err
		}
		c.Return(script.Int(int64(n)))
	case "warp_members":
		name, ok := c.Arg(2).AsString()
		if !ok || name == "" || len(name) > 64 {
			return errors.New(what + ": want a map's name, \"Random\" or \"SavePoint\"")
		}
		x, _ := c.Arg(3).AsInt()
		y, _ := c.Arg(4).AsInt()
		n, _ := c.Arg(5).AsInt()
		if x < 0 || x >= 1<<16 || y < 0 || y >= 1<<16 || n < 0 || n > 1<<10 {
			return errors.New(what + ": x and y are cells, and n is the number of players, 0 to 1024")
		}
		ids, err := h.ChatRoomWarp(id, name, int(x), int(y), int(n))
		if err != nil {
			return err
		}
		c.Return(intList(ids))
	default:
		return fmt.Errorf("chat room has no method %s", call)
	}
	return nil
}
