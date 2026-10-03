package zone

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// Chat and moderation (strategy section 7.10, proposal 0002 review
// decision 26, decision 0018):
//   - Local chat reaches the players in range, as before. Whispers and
//     channel lines go through the world's chat hub, which finds players
//     on any zone of the process.
//   - Every line passes the sender's mute, the server's chat filter and
//     the mods' chat hooks, in that order. The chat event runs after the
//     line went out.
//   - A player does not see the lines of the names it blocks, and they
//     cannot whisper to it.
//   - Each player keeps the lines it saw in the last 5 minutes. A report
//     takes them, and the chat log keeps them while the report is open.
//   - Mutes and jails run on the server clock. A muted player cannot
//     talk. A jailed player goes to the jail map and cannot warp until
//     the jail ends, as rAthena's SC_JAILED.

// Jail is where jailed players go, as rAthena's MAP_JAIL and pc_jail.
type Jail struct {
	// Map is the jail map's asset name. A jail map that does not run
	// leaves jailed players where they are, and they still cannot warp.
	Map string
	// Cells are the cells a jailed player is placed on, one at random.
	Cells [][2]int
}

// DefaultJail is rAthena's: sec_pri, at one of pc_jail's two cells.
var DefaultJail = Jail{Map: "map/sec_pri", Cells: [][2]int{{49, 75}, {24, 75}}}

// seenLines is the most lines a player keeps for reports.
const seenLines = 300

// sanctionCheck is how often the zone ends the sanctions that ran out,
// in zone milliseconds, as SC_JAILED's one minute timer, but finer.
const sanctionCheck = 1000

// chatKind is the kind of a chat command.
type chatKind int

const (
	chatLocal chatKind = iota
	chatWhisper
	chatChannel
)

type chatCommand struct {
	kind chatKind
	// to is the whisper's target or the channel's name.
	to, text string
	// tooFast is set for a line the gateway dropped for the rate limit:
	// the player only learns why.
	tooFast bool
	channel wathenav1.ChatChannel
}

type channelCommand struct {
	action wathenav1.ChannelAction
	name   string
}

type blockCommand struct {
	name    string
	unblock bool
}

type reportCommand struct {
	name   string
	line   chat.LineID
	reason string
}

// chatIn is a delivery from the hub for a player of this zone.
type chatIn struct {
	sess *Session
	d    chat.Delivery
}

// DeliverChat hands a delivery from the hub to the zone the session is
// in. It never blocks: a zone that is far behind drops the line. A
// delivery while the player moves between zones is dropped too.
func (s *Session) DeliverChat(d chat.Delivery) {
	z, _ := s.where()
	if z == nil {
		return
	}
	select {
	case z.chatIn <- chatIn{s, d}:
	default:
	}
}

// Whisper whispers a line to a player by name, on any map, as
// clif_parse_WisMessage. A name that starts with # names a channel.
func (s *Session) Whisper(seq uint32, to, text string) bool {
	return s.send(command{seq: seq, chat: &chatCommand{kind: chatWhisper, to: to, text: text}})
}

// ChannelChat says a line in a channel, by name.
func (s *Session) ChannelChat(seq uint32, channel, text string) bool {
	return s.send(command{seq: seq, chat: &chatCommand{kind: chatChannel, to: channel, text: text}})
}

// ChatTooFast tells the player that a line went over the chat rate
// limit and was dropped.
func (s *Session) ChatTooFast(seq uint32, channel wathenav1.ChatChannel, to string) bool {
	return s.send(command{seq: seq, chat: &chatCommand{tooFast: true, channel: channel, to: to}})
}

// Channel joins or leaves a channel, or lists channels or members.
func (s *Session) Channel(seq uint32, action wathenav1.ChannelAction, name string) bool {
	return s.send(command{seq: seq, channel: &channelCommand{action, name}})
}

// Block adds a name to the player's block list, or removes it. The
// gateway checked the name.
func (s *Session) Block(seq uint32, name string, unblock bool) bool {
	return s.send(command{seq: seq, block: &blockCommand{name, unblock}})
}

// Report reports a player, or a line by its ID. An empty name is
// answered with REPORT_OUTCOME_BAD_NAME.
func (s *Session) Report(seq uint32, name string, line chat.LineID, reason string) bool {
	return s.send(command{seq: seq, report: &reportCommand{name, line, reason}})
}

// chatHub is the world's chat hub, or one of the zone's own, made on
// first use, from the mods' channels and filter.
func (z *Zone) chatHub() *chat.Hub {
	if z.Chat == nil {
		d := chat.Core()
		if z.Mods != nil && z.Mods.Chat != nil {
			d = z.Mods.Chat
		}
		z.Chat = chat.NewHub(d.Channels, d.Filter)
		z.Chat.Clock = z.wallClock
	}
	return z.Chat
}

// chatOnline adds a player that entered the zone to the hub: at login
// it goes online and joins the autojoin channels; after a warp it moves.
// The player gets its channels, block list and sanctions.
func (z *Zone) chatOnline(p *Player, h *handoff) {
	if p.sess == nil {
		return
	}
	hub := z.chatHub()
	if h != nil && h.member != nil {
		p.member, p.seen = h.member, h.seen
		for _, c := range hub.Move(p.member, p.sess, z.Map) {
			p.pending = append(p.pending, channelLeft(c))
		}
		return
	}
	var joined []chat.ChannelInfo
	p.member, joined = hub.Online(p.Name, p.record.ID, p.sess, z.Map, p.blocks)
	for _, c := range joined {
		p.pending = append(p.pending, channelJoined(c))
	}
	p.pending = append(p.pending, blockList(p.blocks, false))
	now := z.wallClock()
	for i := range p.sanctions {
		if s := &p.sanctions[i]; s.End.After(now) {
			p.pending = append(p.pending, sanctionNotice(s, false))
			if s.Kind == character.SanctionJail {
				p.jailPending = true
			}
		}
	}
}

// chatOffline takes a player that logged out out of the hub.
func (z *Zone) chatOffline(p *Player) {
	if p.member != nil {
		z.chatHub().Offline(p.member)
		p.member = nil
	}
}

// say runs a chat command: the mute, the filter, the hooks, then the
// line goes out and the chat event runs.
func (z *Zone) say(p *Player, cmd *chatCommand) {
	if cmd.tooFast {
		p.pending = append(p.pending, wathenav1.Event_builder{ChatRefused: wathenav1.ChatRefused_builder{
			Channel: cmd.channel, To: cmd.to, Reason: wathenav1.ChatRefusal_CHAT_REFUSAL_TOO_FAST,
		}.Build()}.Build())
		return
	}
	hub := z.chatHub()
	now := hub.Now()
	ev := modscript.ChatMessage{Player: int64(p.charID), Channel: "public", To: cmd.to}
	protoChannel := wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL
	viaWhisper := false
	if cmd.kind == chatWhisper {
		ev.Channel, protoChannel = "whisper", wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER
		if strings.HasPrefix(cmd.to, "#") {
			// A whisper to #name speaks in the channel, as rAthena.
			cmd, viaWhisper = &chatCommand{kind: chatChannel, to: cmd.to, text: cmd.text}, true
		}
	}
	if cmd.kind == chatChannel {
		protoChannel, ev.To = wathenav1.ChatChannel_CHAT_CHANNEL_NAMED, ""
		n, ok := chat.NormalizeChannel(cmd.to)
		if !ok {
			p.pending = append(p.pending, channelFailed(cmd.to, chat.ChannelUnknown))
			return
		}
		ev.Channel = n
	}
	refuse := func(reason wathenav1.ChatRefusal) {
		p.pending = append(p.pending, wathenav1.Event_builder{ChatRefused: wathenav1.ChatRefused_builder{
			Channel: protoChannel, To: cmd.to, Reason: reason,
		}.Build()}.Build())
	}
	if s := p.sanction(character.SanctionMute, now); s != nil {
		// clif_process_message drops a muted player's line.
		refuse(wathenav1.ChatRefusal_CHAT_REFUSAL_MUTED)
		p.pending = append(p.pending, sanctionNotice(s, false))
		return
	}
	text, ok := hub.Filter.Check(cmd.text)
	if !ok {
		refuse(wathenav1.ChatRefusal_CHAT_REFUSAL_FILTERED)
		return
	}
	ev.Text = text
	if r := z.scripts; r != nil && r.HasHooks("chat") {
		out, deliver := r.RunChat(ev)
		if !deliver {
			return
		}
		if t, ok := cleanLine(out); ok {
			text = t
		}
		ev.Text = text
	}
	line := chat.Line{ID: chat.NewLineID(now), At: now, CharacterID: p.record.ID, Name: p.Name, Map: z.Map, Text: text}
	switch cmd.kind {
	case chatLocal:
		line.Kind = chat.LineLocal
		z.chats = append(z.chats, chatLine{speaker: p.entity, name: p.Name, text: text, line: line})
	case chatWhisper:
		line.Kind, line.To = chat.LineWhisper, cmd.to
		to, outcome := hub.Whisper(p.member, line)
		res := wathenav1.WhisperResult_builder{To: to}
		switch outcome {
		case chat.WhisperSent:
			res.Outcome, res.Text, res.LineId = wathenav1.WhisperOutcome_WHISPER_OUTCOME_SENT, text, line.ID[:]
		case chat.WhisperBlocked:
			res.Outcome = wathenav1.WhisperOutcome_WHISPER_OUTCOME_BLOCKED
		default:
			res.Outcome = wathenav1.WhisperOutcome_WHISPER_OUTCOME_NOT_ONLINE
		}
		p.pending = append(p.pending, wathenav1.Event_builder{WhisperResult: res.Build()}.Build())
		if outcome != chat.WhisperSent {
			return
		}
		line.To, ev.To = to, to
		p.see(line)
	case chatChannel:
		line.Kind = chat.LineChannel
		if p.member == nil {
			return
		}
		if viaWhisper {
			// A whisper to an open channel the player is not in joins it
			// first, as clif_parse_WisMessage.
			if info, f := hub.Join(p.member, ev.Channel, false); f == 0 {
				p.pending = append(p.pending, channelJoined(info))
			}
		}
		info, f := hub.Send(p.member, ev.Channel, line, now)
		switch f {
		case 0:
		case chat.ChannelNotJoined:
			refuse(wathenav1.ChatRefusal_CHAT_REFUSAL_NOT_IN_CHANNEL)
			return
		case chat.ChannelCannotChat:
			refuse(wathenav1.ChatRefusal_CHAT_REFUSAL_CANNOT_CHAT)
			return
		case chat.ChannelTooFast:
			refuse(wathenav1.ChatRefusal_CHAT_REFUSAL_TOO_FAST)
			return
		default:
			p.pending = append(p.pending, channelFailed(info.Name, f))
			return
		}
		line.To = info.Name
	}
	hub.ChatLog().Line(line)
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "chat", chat: &ev})
}

// cleanLine checks a hook's text as the gateway checks a client's:
// valid UTF-8, control characters as spaces, trimmed, 1 to 255
// characters.
func cleanLine(s string) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > modscript.MaxMessage {
		return "", false
	}
	return s, true
}

// see records a line the player saw, for reports, and drops the lines
// older than the report window.
func (p *Player) see(l chat.Line) {
	p.seen = append(p.seen, l)
	p.trimSeen(l.At)
}

func (p *Player) trimSeen(now time.Time) {
	i := 0
	for i < len(p.seen) && (now.Sub(p.seen[i].At) > chat.ContextWindow || len(p.seen)-i > seenLines) {
		i++
	}
	if i > 0 {
		p.seen = append(p.seen[:0], p.seen[i:]...)
	}
}

// blocking reports whether the player blocks a name.
func (p *Player) blocking(name string) bool {
	k := chat.Key(name)
	return slices.ContainsFunc(p.blocks, func(b string) bool { return chat.Key(b) == k })
}

// receiveChat shows a delivery from the hub to its player.
func (z *Zone) receiveChat(in chatIn) {
	_, p := in.sess.where()
	if p == nil || p.gone() || p.entity == nil || !z.entities.has(p.entity) || p.sess != in.sess {
		return
	}
	d := in.d
	switch d.Kind {
	case chat.DeliverWhisper:
		if p.blocking(d.Line.Name) {
			return
		}
		p.pending = append(p.pending, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
			Channel: wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER, FromName: d.Line.Name, Text: d.Line.Text, LineId: d.Line.ID[:],
		}.Build()}.Build())
		p.see(d.Line)
	case chat.DeliverChannel:
		if d.Line.Name != "" && p.blocking(d.Line.Name) {
			return
		}
		m := wathenav1.ChatMessage_builder{Channel: wathenav1.ChatChannel_CHAT_CHANNEL_NAMED, FromName: d.Line.Name,
			Text: d.Line.Text, Color: d.Channel.Color, ChannelName: d.Channel.Name}
		if !d.Line.ID.IsZero() {
			m.LineId = d.Line.ID[:]
			p.see(d.Line)
		}
		p.pending = append(p.pending, wathenav1.Event_builder{Chat: m.Build()}.Build())
	case chat.DeliverJoined:
		p.pending = append(p.pending, channelJoined(d.Channel))
	case chat.DeliverLeft:
		p.pending = append(p.pending, channelLeft(d.Channel))
	}
}

// drainChat shows every delivery that waits, for a zone without its
// goroutine.
func (z *Zone) drainChat() {
	for {
		select {
		case in := <-z.chatIn:
			z.receiveChat(in)
		default:
			return
		}
	}
}

func channelJoined(c chat.ChannelInfo) *wathenav1.Event {
	return wathenav1.Event_builder{ChannelJoined: wathenav1.ChannelJoined_builder{
		Name: c.Name, DisplayName: c.DisplayName, Color: c.Color, CanLeave: c.CanLeave, CanChat: c.CanChat,
	}.Build()}.Build()
}

func channelLeft(c chat.ChannelInfo) *wathenav1.Event {
	return wathenav1.Event_builder{ChannelLeft: wathenav1.ChannelLeft_builder{Name: c.Name}.Build()}.Build()
}

func channelFailed(name string, f chat.ChannelFailure) *wathenav1.Event {
	reason := map[chat.ChannelFailure]wathenav1.ChannelFailure{
		chat.ChannelUnknown:       wathenav1.ChannelFailure_CHANNEL_FAILURE_UNKNOWN,
		chat.ChannelAlreadyJoined: wathenav1.ChannelFailure_CHANNEL_FAILURE_ALREADY_JOINED,
		chat.ChannelInviteOnly:    wathenav1.ChannelFailure_CHANNEL_FAILURE_INVITE_ONLY,
		chat.ChannelNotJoined:     wathenav1.ChannelFailure_CHANNEL_FAILURE_NOT_JOINED,
		chat.ChannelCannotLeave:   wathenav1.ChannelFailure_CHANNEL_FAILURE_CANNOT_LEAVE,
		chat.ChannelTooMany:       wathenav1.ChannelFailure_CHANNEL_FAILURE_TOO_MANY,
	}[f]
	return wathenav1.Event_builder{ChannelFailed: wathenav1.ChannelFailed_builder{Name: name, Reason: reason}.Build()}.Build()
}

func blockList(names []string, full bool) *wathenav1.Event {
	return wathenav1.Event_builder{BlockList: wathenav1.BlockList_builder{Names: slices.Clone(names), Full: full}.Build()}.Build()
}

func sanctionNotice(s *character.Sanction, ended bool) *wathenav1.Event {
	kind := wathenav1.SanctionKind_SANCTION_KIND_MUTE
	if s.Kind == character.SanctionJail {
		kind = wathenav1.SanctionKind_SANCTION_KIND_JAIL
	}
	return wathenav1.Event_builder{Sanction: wathenav1.SanctionNotice_builder{
		Kind: kind, Reason: s.Reason, EndsAtMs: s.End.UnixMilli(), Ended: ended,
	}.Build()}.Build()
}

func reportResult(o wathenav1.ReportOutcome) *wathenav1.Event {
	return wathenav1.Event_builder{ReportResult: wathenav1.ReportResult_builder{Outcome: o}.Build()}.Build()
}

// channelCommand runs a ChannelCommand.
func (z *Zone) channelCommand(p *Player, c *channelCommand) {
	if p.member == nil {
		return
	}
	hub := z.chatHub()
	switch c.action {
	case wathenav1.ChannelAction_CHANNEL_ACTION_JOIN:
		info, f := hub.Join(p.member, c.name, false)
		if f != 0 {
			p.pending = append(p.pending, channelFailed(c.name, f))
			return
		}
		p.pending = append(p.pending, channelJoined(info))
	case wathenav1.ChannelAction_CHANNEL_ACTION_LEAVE:
		info, f := hub.Leave(p.member, c.name, false)
		if f != 0 {
			p.pending = append(p.pending, channelFailed(c.name, f))
			return
		}
		p.pending = append(p.pending, channelLeft(info))
	case wathenav1.ChannelAction_CHANNEL_ACTION_LIST:
		var list []*wathenav1.ChannelInfo
		for _, c := range hub.List(p.member) {
			list = append(list, wathenav1.ChannelInfo_builder{
				Name: c.Name, DisplayName: c.DisplayName, Color: c.Color, Members: uint32(c.Members), Joined: c.Joined, // #nosec G115 -- a count of players.
			}.Build())
		}
		p.pending = append(p.pending, wathenav1.Event_builder{ChannelList: wathenav1.ChannelList_builder{Channels: list}.Build()}.Build())
	case wathenav1.ChannelAction_CHANNEL_ACTION_MEMBERS:
		names, f := hub.Members(p.member, c.name)
		if f != 0 {
			p.pending = append(p.pending, channelFailed(c.name, f))
			return
		}
		name := c.name
		if n, ok := chat.NormalizeChannel(name); ok {
			name = "#" + n
		}
		p.pending = append(p.pending, wathenav1.Event_builder{ChannelMembers: wathenav1.ChannelMembers_builder{Name: name, Names: names}.Build()}.Build())
	}
}

// block changes the player's block list, as clif_parse_PMIgnore. A
// list that is full refuses a new name.
func (z *Zone) block(p *Player, c *blockCommand) {
	k := chat.Key(c.name)
	i := slices.IndexFunc(p.blocks, func(b string) bool { return chat.Key(b) == k })
	full := false
	switch {
	case c.unblock && i >= 0:
		p.blocks = slices.Delete(slices.Clone(p.blocks), i, i+1)
	case !c.unblock && i < 0 && len(p.blocks) >= chat.MaxBlocks:
		full = true
	case !c.unblock && i < 0:
		p.blocks = append(slices.Clone(p.blocks), c.name)
	}
	if p.member != nil {
		z.chatHub().SetBlocks(p.member, p.blocks)
	}
	p.pending = append(p.pending, blockList(p.blocks, full))
}

// report saves a player's report with the chat it saw.
func (z *Zone) report(p *Player, c *reportCommand) {
	hub := z.chatHub()
	now := hub.Now()
	if c.name == "" || p.member == nil {
		p.pending = append(p.pending, reportResult(wathenav1.ReportOutcome_REPORT_OUTCOME_BAD_NAME))
		return
	}
	p.trimSeen(now)
	r := chat.Report{At: now, ReporterID: p.record.ID, Reporter: p.Name, Target: c.name, Line: c.line, Reason: c.reason, Map: z.Map}
	if !c.line.IsZero() {
		i := slices.IndexFunc(p.seen, func(l chat.Line) bool { return l.ID == c.line })
		if i < 0 || chat.Key(p.seen[i].Name) != chat.Key(c.name) {
			p.pending = append(p.pending, reportResult(wathenav1.ReportOutcome_REPORT_OUTCOME_LINE_NOT_FOUND))
			return
		}
		r.Target, r.TargetID = p.seen[i].Name, p.seen[i].CharacterID
	}
	if !hub.AllowReport(p.member, now) {
		p.pending = append(p.pending, reportResult(wathenav1.ReportOutcome_REPORT_OUTCOME_TOO_MANY))
		return
	}
	r.Context = slices.Clone(p.seen)
	hub.ChatLog().Report(r)
	p.pending = append(p.pending, reportResult(wathenav1.ReportOutcome_REPORT_OUTCOME_SENT))
}

// sanction returns the player's active sanction of a kind, or nil.
func (p *Player) sanction(kind string, now time.Time) *character.Sanction {
	for i := range p.sanctions {
		if s := &p.sanctions[i]; s.Kind == kind && s.End.After(now) {
			return s
		}
	}
	return nil
}

// jailed reports whether the player is in jail now.
func (z *Zone) jailed(p *Player) bool {
	return len(p.sanctions) > 0 && p.sanction(character.SanctionJail, z.wallClock()) != nil
}

// jail is the zone's jail settings, or DefaultJail.
func (z *Zone) jail() Jail {
	if z.Jail.Map == "" {
		return DefaultJail
	}
	return z.Jail
}

// jailCell picks the jail zone and a jail cell, or nil when the jail map
// does not run.
func (z *Zone) jailCell() (*Zone, int, int) {
	j := z.jail()
	to := z.zoneOf(j.Map)
	if to == nil {
		return nil, 0, 0
	}
	if len(j.Cells) == 0 {
		x, y := to.StartCell()
		return to, x, y
	}
	c := j.Cells[z.rand.IntN(len(j.Cells))]
	return to, c[0], c[1]
}

// addSanction mutes or jails a player, as SC_NOCHAT and pc_jail. A new
// sanction replaces one of its kind that has not ended. A jail keeps the
// place to go back to of the jail it replaces.
func (z *Zone) addSanction(p *Player, kind string, minutes int64, reason, by string) {
	now := z.wallClock()
	s := character.Sanction{Kind: kind, Reason: reason, Start: now, End: now.Add(time.Duration(minutes) * time.Minute), By: by}
	if kind == character.SanctionJail {
		if old := p.sanction(kind, now); old != nil {
			s.ReturnMap, s.ReturnX, s.ReturnY = old.ReturnMap, old.ReturnX, old.ReturnY
		} else {
			s.ReturnMap, s.ReturnX, s.ReturnY = z.Map, p.entity.x, p.entity.y
		}
		p.jailPending = true
	}
	p.sanctions = slices.DeleteFunc(slices.Clone(p.sanctions), func(o character.Sanction) bool { return o.Kind == kind })
	p.sanctions = append(p.sanctions, s)
	p.pending = append(p.pending, sanctionNotice(&s, false))
	z.nextSanctionCheck = 0
}

// unmute ends a player's mute. It returns false when it was not muted.
func (z *Zone) unmute(p *Player) bool {
	now := z.wallClock()
	s := p.sanction(character.SanctionMute, now)
	if s == nil {
		return false
	}
	ended := *s
	p.sanctions = slices.DeleteFunc(slices.Clone(p.sanctions), func(o character.Sanction) bool { return o.Kind == character.SanctionMute })
	p.pending = append(p.pending, sanctionNotice(&ended, true))
	return true
}

// checkSanctions ends the sanctions that ran out, and moves jailed
// players to the jail and freed ones back, as SC_JAILED's start and
// end. It runs once a second, or at once after a new sanction.
func (z *Zone) checkSanctions() {
	if z.now < z.nextSanctionCheck {
		return
	}
	z.nextSanctionCheck = z.now + sanctionCheck
	now := z.wallClock()
	var moves []func()
	for _, p := range z.players {
		if len(p.sanctions) == 0 || p.gone() {
			continue
		}
		var freed *character.Sanction
		kept := p.sanctions[:0:0]
		for i := range p.sanctions {
			s := p.sanctions[i]
			if s.End.After(now) {
				kept = append(kept, s)
				continue
			}
			p.pending = append(p.pending, sanctionNotice(&s, true))
			if s.Kind == character.SanctionJail {
				freed = &s
			}
		}
		if len(kept) != len(p.sanctions) {
			p.sanctions = kept
		}
		switch {
		case freed != nil:
			p.jailPending = false
			if z.Map == z.jail().Map {
				back := freed
				moves = append(moves, func() { z.unjail(p, back) })
			}
		case p.jailPending && !p.entity.dead:
			p.jailPending = false
			if to, x, y := z.jailCell(); to != nil && to != z {
				moves = append(moves, func() { z.depart(p, to, x, y) })
			} else if to == z {
				moves = append(moves, func() { z.teleport(p, x, y) })
			}
		}
	}
	for _, m := range moves {
		m()
	}
}

// unjail sends a freed player back to where it was before the jail, or
// to its save point when that map does not run.
func (z *Zone) unjail(p *Player, s *character.Sanction) {
	if p.gone() || p.entity.dead {
		return
	}
	to, x, y := z.zoneOf(s.ReturnMap), s.ReturnX, s.ReturnY
	if to == nil {
		to, x, y = z.savePoint(p)
	}
	switch {
	case to == nil:
	case to == z:
		z.teleport(p, x, y)
	case p.sess != nil:
		z.depart(p, to, x, y)
	}
}

// The script calls of chat (modscript.ChatHost).

// Whisper shows a whisper from a script to a player.
func (h scriptHost) Whisper(id int64, from, text string) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	p.pending = append(p.pending, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER, FromName: from, Text: text,
	}.Build()}.Build())
	return nil
}

// IsBlocking reports whether a player blocks another.
func (h scriptHost) IsBlocking(id, other int64) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	o, err := h.player(other)
	if err != nil {
		return false, err
	}
	return p.blocking(o.Name), nil
}

// Sanction mutes or jails a player.
func (h scriptHost) Sanction(id int64, kind string, minutes int64, reason, by string) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	h.z.addSanction(p, kind, minutes, reason, by)
	return nil
}

// Unmute ends a player's mute.
func (h scriptHost) Unmute(id int64) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	return h.z.unmute(p), nil
}

// channelOwner is the owner key of a mod's channel: one context of one
// zone.
type channelOwner struct {
	z   *Zone
	ctx int64
}

// channelHandle returns the script handle of a channel, made on first
// use.
func (z *Zone) channelHandle(c *chat.Channel) int64 {
	for id, ch := range z.channelHandles {
		if ch == c {
			return id
		}
	}
	if z.channelHandles == nil {
		z.channelHandles = map[int64]*chat.Channel{}
	}
	z.nextChannel++
	z.channelHandles[z.nextChannel] = c
	return z.nextChannel
}

func (h scriptHost) channel(id int64) (*chat.Channel, error) {
	c := h.z.channelHandles[id]
	if c == nil || !h.z.chatHub().Running(c) {
		return nil, errors.New("the channel has ended")
	}
	return c, nil
}

// CreateChannel makes a mod's channel.
func (h scriptHost) CreateChannel(owner int64, def chat.ChannelDef) (int64, error) {
	c, err := h.z.chatHub().Create(def, channelOwner{h.z, owner})
	if err != nil {
		return 0, err
	}
	if h.z.channelOwners == nil {
		h.z.channelOwners = map[int64][]*chat.Channel{}
	}
	if !slices.Contains(h.z.channelOwners[owner], c) {
		h.z.channelOwners[owner] = append(h.z.channelOwners[owner], c)
	}
	return h.z.channelHandle(c), nil
}

// FindChannel finds a running channel.
func (h scriptHost) FindChannel(name string) (int64, bool) {
	c := h.z.chatHub().Channel(name)
	if c == nil {
		return 0, false
	}
	return h.z.channelHandle(c), true
}

// ChannelSend says a line of the channel itself.
func (h scriptHost) ChannelSend(id int64, text string) error {
	c, err := h.channel(id)
	if err != nil {
		return err
	}
	h.z.chatHub().Announce(c, text, h.z.wallClock())
	return nil
}

// ChannelAdd puts a player in a channel.
func (h scriptHost) ChannelAdd(id, player int64) (bool, error) {
	return h.channelMember(id, player, true)
}

// ChannelRemove takes a player out of a channel.
func (h scriptHost) ChannelRemove(id, player int64) (bool, error) {
	return h.channelMember(id, player, false)
}

func (h scriptHost) channelMember(id, player int64, add bool) (bool, error) {
	c, err := h.channel(id)
	if err != nil {
		return false, err
	}
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	if p.member == nil {
		return false, nil
	}
	if add {
		return h.z.chatHub().AddMember(c, p.member), nil
	}
	return h.z.chatHub().RemoveMember(c, p.member), nil
}

// ChannelMembers are the names of a channel's members.
func (h scriptHost) ChannelMembers(id int64) ([]string, error) {
	c, err := h.channel(id)
	if err != nil {
		return nil, err
	}
	return h.z.chatHub().ChannelMembers(c), nil
}

// ReleaseChannels drops the channels a context made.
func (h scriptHost) ReleaseChannels(owner int64) {
	for _, c := range h.z.channelOwners[owner] {
		h.z.chatHub().Release(c, channelOwner{h.z, owner})
	}
	delete(h.z.channelOwners, owner)
}
