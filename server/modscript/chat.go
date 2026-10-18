package modscript

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/chat"
)

// Chat and moderation (proposal 0002, review decision 26): the chat
// event and hook, player:whisper, player:is_blocking, the sanctions, and
// wa.channels.

// ChatHost is a Host with chat. A Host without it raises an error on
// these calls.
type ChatHost interface {
	// Whisper shows a whisper to a player from a name it gives. mod is
	// the mod that sends it, which the player sees with the line, or ""
	// for core (review decision 52).
	Whisper(player int64, mod, from, text string) error
	// IsBlocking reports whether a player blocks another.
	IsBlocking(player, other int64) (bool, error)
	// Sanction mutes or jails a player for minutes, with a reason. by
	// names the mod.
	Sanction(player int64, kind string, minutes int64, reason, by string) error
	// Unmute ends a player's mute. It returns false when the player was
	// not muted.
	Unmute(player int64) (bool, error)
	// CreateChannel makes a mod's channel, owned by a context, and
	// returns its handle.
	CreateChannel(owner int64, def chat.ChannelDef) (int64, error)
	// FindChannel finds a running channel by name, with or without its #.
	FindChannel(name string) (int64, bool)
	// ChannelSend says a line of the channel itself, as channelmes.
	ChannelSend(channel int64, text string) error
	// ChannelAdd and ChannelRemove put a player in a channel and take it
	// out. They return false when nothing changed.
	ChannelAdd(channel, player int64) (bool, error)
	ChannelRemove(channel, player int64) (bool, error)
	// ChannelMembers are the names of a channel's members, sorted.
	ChannelMembers(channel int64) ([]string, error)
	// ReleaseChannels drops a context's channels when it ends. A channel
	// ends with its last owner.
	ReleaseChannels(owner int64)
}

// Limits of the chat calls.
const (
	// MaxSanctionMinutes is the longest mute or jail: ten years.
	MaxSanctionMinutes = 10 * 365 * 24 * 60
	// MaxReason is the longest reason of a sanction, in characters.
	MaxReason = 255
	// MaxWhisperName is the longest from of player:whisper, in
	// characters, as a character name.
	MaxWhisperName = 24
)

// Chat channels of the chat event that are not channel names.
var chatKinds = []string{"public", "whisper", "party", "guild"}

// PrivateChatPermission is the permission that hears private lines in
// the chat event. The chat hook needs it too, and sees every line.
const PrivateChatPermission = "rules.override"

// IsPrivateChat reports whether a line of a channel of the chat event is
// private: a whisper, or a party or guild line.
func IsPrivateChat(channel string) bool {
	return channel == "whisper" || channel == "party" || channel == "guild"
}

// IsChatKind reports whether a channel of the chat event is one of
// public, whisper, party and guild, which no chat channel may be named.
func IsChatKind(name string) bool { return slices.Contains(chatKinds, name) }

// ChatMessage is the object of the chat event and the chat hook: a line
// a player says (review decision 26).
type ChatMessage struct {
	// Player is the speaker's player ID in the zone.
	Player int64
	// Channel is "public", "whisper", "party", "guild" or a channel's
	// name without its #.
	Channel string
	Text    string
	// To is a whisper's target name, or "".
	To string
}

func (r *Runtime) chatFields(m ChatMessage) []script.Field {
	fields := []script.Field{
		script.KV("player", script.Int(m.Player)), script.KV("channel", script.String(m.Channel)),
		script.KV("text", script.String(m.Text)), script.KV("map", script.String(r.host.Map())),
	}
	if m.To != "" {
		fields = append(fields, script.KV("to", script.String(m.To)))
	}
	return fields
}

// chatFilter is the options of a chat handler.
type chatFilter struct {
	channel, mapName string
	player           int64
}

func (f *chatFilter) match(m ChatMessage, mapName string) bool {
	return (f.channel == "" || f.channel == m.Channel) && (f.mapName == "" || f.mapName == mapName) &&
		(f.player == 0 || f.player == m.Player)
}

// readChatFilter reads the options of events.on("chat", ...). prelude
// turns a player handle into its ID.
func readChatFilter(v script.Value) (*chatFilter, error) {
	t, ok := v.AsTable()
	if !ok {
		return nil, errors.New(`events.on("chat"): the options must be a table`)
	}
	f := &chatFilter{}
	for _, field := range t.Fields {
		k, _ := field.Key.AsString()
		switch k {
		case "channel":
			s, _ := field.Value.AsString()
			if IsChatKind(s) {
				f.channel = s
				break
			}
			n, ok := chat.NormalizeChannel(s)
			if !ok {
				return nil, errors.New(`events.on("chat"): channel is "public", "whisper", "party", "guild" or a channel's name`)
			}
			f.channel = n
		case "map":
			s, ok := field.Value.AsString()
			if !ok || s == "" {
				return nil, errors.New(`events.on("chat"): map must be a map name, such as prontera`)
			}
			f.mapName = s
		case "player":
			n, ok := field.Value.AsInt()
			if !ok || n <= 0 {
				return nil, errors.New(`events.on("chat"): player must be a player handle`)
			}
			f.player = n
		default:
			return nil, fmt.Errorf(`events.on("chat"): %q is not an option; the options are channel, map and player`, k)
		}
	}
	return f, nil
}

// FireChat runs the chat handlers whose options match. It runs after
// the line went out.
func (r *Runtime) FireChat(m ChatMessage) {
	list := r.handlers["chat"]
	if len(list) == 0 {
		return
	}
	obj := script.NewTable(r.chatFields(m)...)
	for _, h := range slices.Clone(list) {
		if h.removed || h.ctx.ended || r.off(h.ctx) {
			continue
		}
		if h.chat != nil && !h.chat.match(m, r.host.Map()) {
			continue
		}
		// Whispers, party and guild lines are private. Only a mod with
		// rules.override, which the chat hook needs and which sees every
		// line there, hears them (review decision 52).
		if IsPrivateChat(m.Channel) && !h.ctx.grants.Has(PrivateChatPermission) {
			continue
		}
		if err := r.runHandler(h.ctx, r.fns.invoke.Value(), h.fn.Value(), script.String("chat"), obj); err != nil {
			r.failed(h.ctx, "event chat", err)
		}
	}
}

// RunChat runs the chat hooks, after the server's chat filter. It
// returns the text to send, and false when a hook blocked the line.
// Without hooks, or when the chain fails, the text is the line's.
func (r *Runtime) RunChat(m ChatMessage) (string, bool) {
	vals, ok := r.runHooks("chat", r.chatFields(m))
	if !ok {
		return m.Text, true
	}
	if vals[0].IsNil() {
		return "", false
	}
	s, isString := vals[0].AsString()
	if !isString {
		return m.Text, true
	}
	return s, true
}

func (r *Runtime) chatHost(what string) (ChatHost, error) {
	h, ok := r.host.(ChatHost)
	if !ok {
		return nil, errors.New(what + ": chat does not exist here")
	}
	return h, nil
}

// textArg checks a text argument of 1 to max bytes.
func textArg(what, v string, maxBytes int) error {
	if v == "" || len(v) > maxBytes || !utf8.ValidString(v) {
		return fmt.Errorf("%s: the text must be 1 to %d bytes of UTF-8", what, maxBytes)
	}
	return nil
}

// playerWhisper is player:whisper(text, { from }).
func (r *Runtime) playerWhisper(c *script.Call) error {
	const what = "player:whisper"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.chatHost(what)
	if err != nil {
		return err
	}
	text := c.String(1)
	if err := textArg(what, text, MaxMessage); err != nil {
		return err
	}
	from, _ := c.Arg(2).AsString()
	if from == "" {
		from = r.whisperName()
	}
	if utf8.RuneCountInString(from) > MaxWhisperName || strings.TrimSpace(from) == "" {
		return fmt.Errorf("%s: from must be 1 to %d characters", what, MaxWhisperName)
	}
	return h.Whisper(id, r.sender(), from, text)
}

// whisperName is the default from of player:whisper: the NPC's name in
// an NPC context, else the mod's name, cut to MaxWhisperName.
func (r *Runtime) whisperName() string {
	name := ""
	if c := r.current; c != nil && c.npc != 0 {
		if h, ok := r.host.(NPCHost); ok {
			if v, err := h.NPCField(c.npc, "name"); err == nil {
				name, _ = v.AsString()
			}
		}
	}
	if name == "" {
		if sc := r.scripts[r.actor()]; sc != nil {
			name = sc.Name
		}
	}
	if name == "" {
		name = r.actor()
	}
	if utf8.RuneCountInString(name) > MaxWhisperName {
		name = string([]rune(name)[:MaxWhisperName])
	}
	return name
}

func (r *Runtime) playerIsBlocking(c *script.Call) error {
	const what = "player:is_blocking"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	other, ok := c.Arg(1).AsInt()
	if !ok {
		return errors.New(what + ": want a player handle")
	}
	h, err := r.chatHost(what)
	if err != nil {
		return err
	}
	b, err := h.IsBlocking(id, other)
	if err != nil {
		return err
	}
	c.Return(script.Bool(b))
	return nil
}

// playerSanction is player:mute, player:jail and player:unmute.
func (r *Runtime) playerSanction(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	kind := c.String(1)
	what := "player:" + kind
	if kind != "mute" && kind != "jail" && kind != "unmute" {
		return fmt.Errorf("player sanction: no kind %q", kind)
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "player.sanction"); err != nil {
		return err
	}
	h, err := r.chatHost(what)
	if err != nil {
		return err
	}
	if kind == "unmute" {
		ok, err := h.Unmute(id)
		if err != nil {
			return err
		}
		c.Return(script.Bool(ok))
		return nil
	}
	minutes, ok := c.Arg(2).AsInt()
	if !ok || minutes < 1 || minutes > MaxSanctionMinutes {
		return fmt.Errorf("%s: minutes must be an integer from 1 to %d", what, MaxSanctionMinutes)
	}
	reason := c.String(3)
	if strings.TrimSpace(reason) == "" || utf8.RuneCountInString(reason) > MaxReason || !utf8.ValidString(reason) {
		return fmt.Errorf("%s: the reason must be 1 to %d characters", what, MaxReason)
	}
	return h.Sanction(id, kind, minutes, reason, r.actor())
}

// channelsCreate is channels.create { name, display_name, color, join }.
func (r *Runtime) channelsCreate(c *script.Call) error {
	const what = "channels.create"
	ctx, err := r.needContext(what)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.chatHost(what)
	if err != nil {
		return err
	}
	if err := r.room(ctx); err != nil {
		return err
	}
	t, ok := c.Arg(0).AsTable()
	if !ok {
		return errors.New(what + `: want a table such as { name = "lounge", join = "open" }`)
	}
	def := chat.ChannelDef{Color: 0xFFFFFF, Join: chat.JoinOpen, CanLeave: true, CanChat: true, Delay: 1000, Kind: chat.KindWorld, Mod: ctx.Mod}
	for _, f := range t.Fields {
		k, _ := f.Key.AsString()
		switch k {
		case "name":
			s, _ := f.Value.AsString()
			n, ok := chat.NormalizeChannel(s)
			if !ok || IsChatKind(n) {
				return fmt.Errorf("%s: name must be 1 to %d letters, digits, _ and -, and not public, whisper, party or guild", what, chat.MaxChannelName)
			}
			def.Name = n
		case "display_name":
			s, _ := f.Value.AsString()
			if s == "" || utf8.RuneCountInString(s) > 24 {
				return fmt.Errorf("%s: display_name must be 1 to 24 characters", what)
			}
			def.DisplayName = s
		case "color":
			n, ok := f.Value.AsInt()
			if !ok || n < 0 || n > 0xFFFFFF {
				return fmt.Errorf("%s: color must be 0xRRGGBB", what)
			}
			def.Color = uint32(n) // #nosec G115 -- checked above.
		case "join":
			s, _ := f.Value.AsString()
			if s != chat.JoinOpen && s != chat.JoinInvite {
				return fmt.Errorf(`%s: join is "open" or "invite"`, what)
			}
			def.Join = s
		default:
			return fmt.Errorf("%s: %q is not a field; the fields are name, display_name, color and join", what, k)
		}
	}
	if def.Name == "" {
		return errors.New(what + ": name is required")
	}
	if def.DisplayName == "" {
		def.DisplayName = "[" + def.Name + "]"
	}
	id, err := h.CreateChannel(ctx.ID, def)
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	ctx.channels = true
	c.Return(script.Int(id))
	return nil
}

func (r *Runtime) channelsGet(c *script.Call) error {
	const what = "channels.get"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	h, err := r.chatHost(what)
	if err != nil {
		return err
	}
	id, ok := h.FindChannel(c.String(0))
	if !ok {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(id))
	return nil
}

// channelCall is a method of a channel handle: send, add, remove or
// get_members.
func (r *Runtime) channelCall(c *script.Call) error {
	op, id := c.String(0), c.Int(1)
	what := "channel:" + op
	h, err := r.chatHost(what)
	if err != nil {
		return err
	}
	switch op {
	case "send":
		if err := r.mutating(what); err != nil {
			return err
		}
		text := c.String(2)
		if err := textArg(what, text, MaxMessage); err != nil {
			return err
		}
		return h.ChannelSend(id, text)
	case "add", "remove":
		if err := r.mutating(what); err != nil {
			return err
		}
		player, ok := c.Arg(2).AsInt()
		if !ok {
			return errors.New(what + ": want a player handle")
		}
		f := h.ChannelAdd
		if op == "remove" {
			f = h.ChannelRemove
		}
		changed, err := f(id, player)
		if err != nil {
			return err
		}
		c.Return(script.Bool(changed))
		return nil
	case "get_members":
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
		names, err := h.ChannelMembers(id)
		if err != nil {
			return err
		}
		vals := make([]script.Value, len(names))
		for i, n := range names {
			vals[i] = script.String(n)
		}
		c.Return(script.List(vals...))
		return nil
	}
	return fmt.Errorf("channel: no method %q", op)
}
