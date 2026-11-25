package botclient

import (
	"context"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Chat rooms (proposal 0006, section 2): the commands a bot sends to join
// a room, leave it and speak in it.

// ChatRoomJoin joins a room of a ChatRoomShow.
func (b *Bot) ChatRoomJoin(ctx context.Context, room uint32, password string) error {
	return b.chatRoom(ctx, wathenav1.ChatRoomCommand_builder{
		Action: wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_JOIN, Room: room, Password: password,
	})
}

// ChatRoomCreate makes a room.
func (b *Bot) ChatRoomCreate(ctx context.Context, title, password string, limit uint32, public bool) error {
	return b.chatRoom(ctx, wathenav1.ChatRoomCommand_builder{
		Action: wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_CREATE, Title: title, Password: password, Limit: limit, Public: public,
	})
}

// ChatRoomLeave leaves the room.
func (b *Bot) ChatRoomLeave(ctx context.Context) error {
	return b.chatRoom(ctx, wathenav1.ChatRoomCommand_builder{Action: wathenav1.ChatRoomAction_CHAT_ROOM_ACTION_LEAVE})
}

func (b *Bot) chatRoom(ctx context.Context, c wathenav1.ChatRoomCommand_builder) error {
	return b.send(ctx, wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{ChatRoom: c.Build()}.Build()}, true)
}

// RoomChat says a line to the room the bot is in.
func (b *Bot) RoomChat(ctx context.Context, text string) error {
	return b.send(ctx, wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		Chat: wathenav1.Chat_builder{Channel: wathenav1.ChatChannel_CHAT_CHANNEL_ROOM, Text: text}.Build(),
	}.Build()}, true)
}
