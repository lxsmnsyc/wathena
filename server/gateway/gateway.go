// Package gateway accepts web client connections, as docs/proposals/
// 0001-protocol.md describes: one WebSocket per session, binary frames
// of one ClientMessage or ServerMessage each, and Hello first.
package gateway

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	chatpkg "github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// HelloTimeout is how long a new connection has to send Hello.
const HelloTimeout = 10 * time.Second

// MaxRaise is the most a RaiseStat may ask for at once. rAthena's
// clif_parse_StatusUp reads the amount as one byte.
const MaxRaise = 255

// Sessions checks the session token of Hello.
type Sessions interface {
	// Check returns who a token names. It returns ErrSessionInvalid for
	// a token that is not valid, and other errors when it cannot tell,
	// such as when the database is down.
	Check(ctx context.Context, token []byte) (Login, error)
}

// Login is who a session token names.
type Login struct {
	// CharacterID is the character's database ID when the token names
	// one, as a game ticket does. It is 0 for a dev token, whose
	// character Characters.Find finds by name.
	CharacterID int64
	// SessionID is the account service's login session that made the
	// token, or 0 for a dev token. EndLoginSessions closes the
	// connections of a session.
	SessionID int64
	Name      string
	Look      zone.Look
	// Staff is how the login logged in, for its staff role. A dev token
	// has no second factor, so it has no role.
	Staff staff.Login
}

// Bans says whether a character's account is banned.
type Bans interface {
	// Banned returns the text a banned player sees, with the reason
	// and the end time, and whether the account is banned now.
	Banned(ctx context.Context, character int64) (text string, banned bool, err error)
}

// ErrSessionInvalid is a session token that is not valid, used, or
// expired. The client gets DISCONNECT_REASON_SESSION_INVALID.
var ErrSessionInvalid = errors.New("gateway: the session token is not valid")

// DevSessions accepts any token that is a valid name, for development.
// Never use it on a public server: anyone can play any character.
type DevSessions struct{}

// Check takes the token as the name and derives a look from it.
func (DevSessions) Check(_ context.Context, token []byte) (Login, error) {
	name := string(token)
	if !ValidName(name) {
		return Login{}, ErrSessionInvalid
	}
	h := uint32(2166136261)
	for _, b := range token {
		h = (h ^ uint32(b)) * 16777619
	}
	return Login{Name: name, Look: zone.Look{Job: 0, Sex: h & 1, Hair: 1 + h>>1%20, HairColor: h >> 6 % 9}}, nil
}

// ValidName reports whether a character name is 1 to 24 printable
// characters without leading or trailing spaces.
func ValidName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > 24 || !utf8.ValidString(name) || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// KickWait is how long a login waits for the session it replaces to end
// and hand over its last save.
const KickWait = 10 * time.Second

// Characters finds and loads the character a session plays.
type Characters interface {
	// Find returns the ID of the character for a session's name and
	// look. A source may make the character first.
	Find(ctx context.Context, name string, look zone.Look) (int64, error)
	// Load starts a session for a character and returns the character
	// and its saver. The saver may be nil, when nothing keeps the
	// character.
	Load(ctx context.Context, id int64) (*character.Character, zone.Saver, error)
}

// World is where sessions play. A single *zone.Zone is one, and so is a
// world of several zones, which puts each character in the zone of its
// map.
type World interface {
	// JoinCharacter adds a loaded character to the world and returns its
	// session. saver, when not nil, saves the character.
	JoinCharacter(ctx context.Context, c *character.Character, saver zone.Saver) (*zone.Session, error)
	// NewCharacter is a new character, on the map where new characters
	// start.
	NewCharacter(name string, look zone.Look) character.Character
}

// Gateway serves the protocol over WebSocket.
//
// A session keeps its connection when the player changes maps: commands
// go to the zone the player is in, through its zone.Session.
//
// One character has at most one session. A login for a character that
// is already playing kicks the older session, as rAthena does, and waits
// until the older session's last save is queued before loading the
// character. The older connection gets DISCONNECT_REASON_LOGGED_IN_ELSEWHERE.
type Gateway struct {
	World    World
	Sessions Sessions
	// Characters loads characters. When nil, characters only live in
	// memory, for as long as the gateway runs.
	Characters Characters
	// Origins lists the web client origins pages may connect from, as
	// scheme://host[:port], such as https://play.example.com. Pages on
	// the server's own host may always connect. A request without an
	// Origin header does not come from a browser page, and is accepted:
	// the session token is what lets it in.
	Origins []string
	Log     *slog.Logger
	// Metrics counts each connection's bytes and messages. It may be
	// nil.
	Metrics *metrics.Conns
	// Chat are the chat rate limits. chat.DefaultLimits when zero.
	Chat chatpkg.Limits
	// Limits are the world ruleset's limits, which Welcome sends.
	// rAthena's when zero.
	Limits rules.Limits
	// CheckName checks the name of a --dev session's character against
	// the chat filter and the reserved names (strategy section 7.10). It
	// may be nil.
	CheckName func(name string) error
	// Bans refuses the logins of banned accounts (decision 0030). It may
	// be nil, as without a database.
	Bans Bans
	// Closed refuses new logins, with the text the player sees, such as
	// in the last minute before a planned restart (decision 0031). It
	// may be nil.
	Closed func() (text string, closed bool)

	mu     sync.Mutex
	online map[int64]*online
	memory *memoryCharacters
	// logins holds the connections of each login session, and ended the
	// login sessions that ended recently, by when.
	logins map[int64]map[*online]struct{}
	ended  map[int64]time.Time
}

// online is a character's live session.
type online struct {
	// kick closes to end the session. done closes once the session has
	// left the zone. reason and text are set before kick closes, and
	// say why.
	kick     chan struct{}
	kickOnce sync.Once
	done     chan struct{}
	reason   wathenav1.DisconnectReason
	text     string
}

// Why a connection is kicked.
const (
	elsewhereText = "Your character logged in somewhere else."
	loggedOutText = "You logged out. Log in again to play."
	shutdownText  = "The server is restarting. Try again in a minute."
)

func (o *online) end(reason wathenav1.DisconnectReason, text string) {
	o.kickOnce.Do(func() {
		o.reason, o.text = reason, text
		close(o.kick)
	})
}

// endedFor is how long EndLoginSessions remembers a login session. A
// connection whose ticket was used just before its session ended, and
// that registers just after, is still closed.
const endedFor = 5 * time.Minute

// EndLoginSessions closes the game connections that the given login
// sessions opened. The account service calls it when it revokes a
// session, such as at logout. The client gets
// DISCONNECT_REASON_LOGGED_OUT.
//
// In one process the account service calls it directly. With several
// processes, a PostgreSQL NOTIFY from the revoking transaction can call
// it in each gateway process.
func (g *Gateway) EndLoginSessions(ids ...int64) {
	now := time.Now()
	var kick []*online
	g.mu.Lock()
	if g.ended == nil {
		g.ended = map[int64]time.Time{}
	}
	for id, at := range g.ended {
		if now.Sub(at) > endedFor {
			delete(g.ended, id)
		}
	}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		g.ended[id] = now
		for o := range g.logins[id] {
			kick = append(kick, o)
		}
	}
	g.mu.Unlock()
	for _, o := range kick {
		o.end(wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_OUT, loggedOutText)
	}
}

// watch records that a connection belongs to a login session. It kicks
// the connection at once when the session already ended.
func (g *Gateway) watch(session int64, me *online) {
	if session == 0 {
		return
	}
	g.mu.Lock()
	if g.logins == nil {
		g.logins = map[int64]map[*online]struct{}{}
	}
	if g.logins[session] == nil {
		g.logins[session] = map[*online]struct{}{}
	}
	g.logins[session][me] = struct{}{}
	_, ended := g.ended[session]
	g.mu.Unlock()
	if ended {
		me.end(wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_OUT, loggedOutText)
	}
}

func (g *Gateway) unwatch(session int64, me *online) {
	if session == 0 {
		return
	}
	g.mu.Lock()
	delete(g.logins[session], me)
	if len(g.logins[session]) == 0 {
		delete(g.logins, session)
	}
	g.mu.Unlock()
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !g.originAllowed(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// originAllowed checked the origin, with the scheme, which
		// OriginPatterns does not look at.
		InsecureSkipVerify: true,
		// Compression off: compressing chat alongside private data lets
		// an observer guess the data from message sizes.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	c := &conn{Conn: ws, stats: g.Metrics.Open()}
	defer g.Metrics.Close(c.stats)
	c.SetReadLimit(schema.MaxClientMessageBytes)
	ctx := r.Context()
	if err := g.session(ctx, c); err != nil && g.Log != nil {
		g.Log.Debug("session ended", "err", err)
	}
}

// originAllowed checks the Origin header against Origins.
func (g *Gateway) originAllowed(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	if slices.Contains(g.Origins, o) {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

// errClosed ends a session that the server closed on purpose.
var errClosed = errors.New("closed")

func (g *Gateway) session(ctx context.Context, c *conn) error {
	defer func() { _ = c.CloseNow() }()
	hello, seq, err := readHello(ctx, c)
	if err != nil {
		return err
	}
	if v := hello.GetProtocolVersion(); v < schema.MinVersion || v > schema.Version {
		return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_VERSION_UNSUPPORTED, "This client is too old or too new for the server.")
	}
	// In the last minute before a planned restart, no one new joins. The
	// ticket is not used, so it works after the restart while it lasts.
	if g.Closed != nil {
		if text, closed := g.Closed(); closed {
			return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_RESTART, text)
		}
	}
	login, err := g.Sessions.Check(ctx, hello.GetSessionToken())
	if errors.Is(err, ErrSessionInvalid) {
		return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_SESSION_INVALID, "Your session has ended. Log in again.")
	}
	if err != nil {
		return g.loadFailed(ctx, c, err)
	}
	chars := g.characters()
	id := login.CharacterID
	if id == 0 && g.CheckName != nil {
		if err := g.CheckName(login.Name); err != nil {
			return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_SESSION_INVALID, "That character name is not allowed: "+err.Error()+".")
		}
	}
	if id == 0 {
		if id, err = chars.Find(ctx, login.Name, login.Look); err != nil {
			return g.loadFailed(ctx, c, err)
		}
	} else if g.Characters == nil {
		return g.loadFailed(ctx, c, errors.New("gateway: a session names a saved character, and there is no database"))
	}
	me := g.claim(id)
	defer g.release(id, me)
	g.watch(login.SessionID, me)
	defer g.unwatch(login.SessionID, me)
	select {
	case <-me.kick:
		return disconnect(ctx, c, me.reason, me.text)
	default:
	}
	// A banned account cannot play. The check comes after the claim, so
	// a ban that lands during the login kicks this session.
	if g.Bans != nil {
		text, banned, err := g.Bans.Banned(ctx, id)
		if err != nil {
			return g.loadFailed(ctx, c, err)
		}
		if banned {
			return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_BANNED, text)
		}
	}
	// An autotrader of the character leaves first, so the load sees its
	// last save (review decision 30).
	if a, ok := g.World.(interface {
		EndAutotrade(ctx context.Context, id int64)
	}); ok {
		a.EndAutotrade(ctx, id)
	}
	char, saver, err := chars.Load(ctx, id)
	if err != nil {
		return g.loadFailed(ctx, c, err)
	}
	char.Staff = login.Staff
	p, err := g.World.JoinCharacter(ctx, char, saver)
	if err != nil {
		if saver != nil {
			// The session ends before it started. Its character is
			// unchanged, so the save only closes the session.
			saver.Save(char, true)
		}
		return err
	}
	defer p.Leave()
	welcome := wathenav1.ServerMessage_builder{Welcome: wathenav1.Welcome_builder{
		ProtocolVersion: schema.Version,
		ServerTimeMs:    uint64(time.Now().UnixMilli()), // #nosec G115 -- the clock is after 1970.
		Tick:            p.Tick,
		NetId:           p.NetID,
		Map:             p.Map,
		Limits:          limitsMessage(g.Limits),
	}.Build()}.Build()
	if err := write(ctx, c, welcome); err != nil {
		return err
	}
	p.Ack(seq)

	// Messages from the zone go out on their own goroutine, so a client
	// that stops reading cannot block the reader.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-me.kick:
			// The reason goes out, then the connection closes without
			// waiting for the client's close frame, so the new login
			// does not wait on this client.
			_ = write(ctx, c, wathenav1.ServerMessage_builder{Disconnect: wathenav1.Disconnect_builder{
				Reason: me.reason, Message: me.text,
			}.Build()}.Build())
			cancel()
		case <-ctx.Done():
		}
	}()
	go func() {
		defer cancel()
		for m := range p.Out {
			if write(ctx, c, m) != nil {
				return
			}
		}
		// The zone dropped the player for falling behind, or the player
		// left the world.
		if p.FellBehind() {
			if g.Log != nil {
				g.Log.Warn("a connection fell behind, so the zone dropped it", "character", char.Name)
			}
			_ = c.Close(websocket.StatusTryAgainLater, "fell behind")
			return
		}
		_ = c.Close(websocket.StatusNormalClosure, "")
	}()
	return g.read(ctx, c, p, seq, char.CreatedAt)
}

func (g *Gateway) read(ctx context.Context, c *conn, p *zone.Session, seq uint32, created time.Time) error {
	limits := g.Chat
	if limits == (chatpkg.Limits{}) {
		limits = chatpkg.DefaultLimits()
	}
	chat := newChatLimitFor(time.Now(), func(now time.Time) (int, time.Duration) { return limits.For(created, now) })
	for {
		m, err := readMessage(ctx, c)
		if err != nil {
			if errors.Is(err, errBadMessage) {
				return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_BAD_MESSAGE, "The client sent a message the server cannot read.")
			}
			return err
		}
		// Sequence numbers increase by one per message.
		if m.GetSeq() != seq+1 {
			return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_BAD_MESSAGE, "The client sent messages out of order.")
		}
		seq = m.GetSeq()
		var queued bool
		switch {
		case m.HasPing():
			pong := wathenav1.ServerMessage_builder{Pong: wathenav1.Pong_builder{
				Nonce:        m.GetPing().GetNonce(),
				ServerTimeMs: uint64(time.Now().UnixMilli()), // #nosec G115 -- the clock is after 1970.
			}.Build()}.Build()
			if err := write(ctx, c, pong); err != nil {
				return err
			}
			queued = p.Ack(seq)
		case m.GetCommand().HasAttack():
			a := m.GetCommand().GetAttack()
			queued = p.Attack(seq, a.GetTarget(), a.GetRepeat())
		case m.GetCommand().HasChat():
			ok, over := chat.allow(time.Now())
			if over {
				return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_RATE_LIMITED, "Too many chat messages.")
			}
			ch := m.GetCommand().GetChat()
			text, valid := CleanChat(ch.GetText())
			// Lines over the rate are dropped, as rAthena's
			// min_chat_delay does, and the player is told, since the
			// client does not know a new character's lower limits.
			if !ok {
				queued = p.ChatTooFast(seq, ch.GetChannel(), ch.GetTo()+ch.GetChannelName())
				break
			}
			if !valid {
				queued = p.Ack(seq)
				break
			}
			switch ch.GetChannel() {
			case wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL:
				queued = p.Chat(seq, text)
			case wathenav1.ChatChannel_CHAT_CHANNEL_WHISPER:
				to := ch.GetTo()
				if !ValidName(to) {
					queued = p.Ack(seq)
					break
				}
				queued = p.Whisper(seq, to, text)
			case wathenav1.ChatChannel_CHAT_CHANNEL_NAMED:
				queued = p.ChannelChat(seq, ch.GetChannelName(), text)
			case wathenav1.ChatChannel_CHAT_CHANNEL_PARTY:
				queued = p.PartyChat(seq, text)
			case wathenav1.ChatChannel_CHAT_CHANNEL_GUILD:
				queued = p.GuildChat(seq, text)
			default:
				queued = p.Ack(seq)
			}
		case m.GetCommand().HasChannel():
			cc := m.GetCommand().GetChannel()
			if cc.GetAction() < wathenav1.ChannelAction_CHANNEL_ACTION_JOIN || cc.GetAction() > wathenav1.ChannelAction_CHANNEL_ACTION_MEMBERS || len(cc.GetName()) > 64 {
				queued = p.Ack(seq)
				break
			}
			queued = p.Channel(seq, cc.GetAction(), cc.GetName())
		case m.GetCommand().HasBlock():
			b := m.GetCommand().GetBlock()
			if !ValidName(b.GetName()) {
				queued = p.Ack(seq)
				break
			}
			queued = p.Block(seq, b.GetName(), b.GetUnblock())
		case m.GetCommand().HasReport():
			r := m.GetCommand().GetReport()
			reason := ""
			if r.GetReason() != "" {
				var ok bool
				if reason, ok = CleanChat(r.GetReason()); !ok {
					queued = p.Ack(seq)
					break
				}
			}
			if g := r.GetGuildId(); g != 0 {
				if g > math.MaxInt64 {
					queued = p.Ack(seq)
					break
				}
				queued = p.ReportGuild(seq, int64(g), r.GetEmblem(), reason) // #nosec G115 -- checked above.
				break
			}
			line, isLine := chatpkg.LineIDFrom(r.GetLineId())
			if !ValidName(r.GetName()) || len(r.GetLineId()) > 0 && !isLine {
				// The zone answers an empty name with REPORT_OUTCOME_BAD_NAME.
				queued = p.Report(seq, "", chatpkg.LineID{}, "")
				break
			}
			queued = p.Report(seq, r.GetName(), line, reason)
		case m.GetCommand().HasParty():
			queued = partyCommand(p, seq, m.GetCommand().GetParty())
		case m.GetCommand().HasFriend():
			queued = friendCommand(p, seq, m.GetCommand().GetFriend())
		case m.GetCommand().HasPartyBooking():
			queued = bookingCommand(p, seq, m.GetCommand().GetPartyBooking())
		case m.GetCommand().HasGuild():
			queued = guildCommand(p, seq, m.GetCommand().GetGuild())
		case m.GetCommand().HasStorage():
			queued = storageCommand(p, seq, m.GetCommand().GetStorage())
		case m.GetCommand().HasTrade():
			queued = tradeCommand(p, seq, m.GetCommand().GetTrade())
		case m.GetCommand().HasMail():
			queued = p.Mail(seq, m.GetCommand().GetMail())
		case m.GetCommand().HasVending():
			queued = p.Vending(seq, m.GetCommand().GetVending())
		case m.GetCommand().HasBuyingStore():
			queued = p.BuyingStore(seq, m.GetCommand().GetBuyingStore())
		case m.GetCommand().HasStoreSearch():
			queued = p.StoreSearch(seq, m.GetCommand().GetStoreSearch())
		case m.GetCommand().HasPickUp():
			queued = p.PickUp(seq, m.GetCommand().GetPickUp().GetItem())
		case m.GetCommand().HasRaiseStat():
			r := m.GetCommand().GetRaiseStat()
			st, amount := r.GetStat(), r.GetAmount()
			if st < wathenav1.BaseStat_BASE_STAT_STR || st > wathenav1.BaseStat_BASE_STAT_LUK || amount < 1 || amount > MaxRaise {
				// A stat or amount out of range raises nothing, as
				// pc_statusup refuses it.
				queued = p.Ack(seq)
				break
			}
			queued = p.RaiseStat(seq, status.Stat(st-wathenav1.BaseStat_BASE_STAT_STR), int(amount))
		case m.GetCommand().HasUseItem():
			slot := m.GetCommand().GetUseItem().GetSlot()
			if slot >= item.MaxInventory {
				// clif_parse_UseItem ignores an index past MAX_INVENTORY.
				queued = p.Ack(seq)
				break
			}
			queued = p.UseItem(seq, int(slot))
		case m.GetCommand().HasDropItem():
			d := m.GetCommand().GetDropItem()
			if d.GetSlot() >= item.MaxInventory || d.GetAmount() > item.MaxAmount {
				// No slot or stack is that large, so nothing drops.
				queued = p.Ack(seq)
				break
			}
			queued = p.DropItem(seq, int(d.GetSlot()), int(d.GetAmount()))
		case m.GetCommand().HasWalkTo():
			w := m.GetCommand().GetWalkTo()
			queued = p.WalkTo(seq, int(w.GetX()), int(w.GetY()))
		case m.GetCommand().HasNpcClick():
			queued = p.NpcClick(seq, m.GetCommand().GetNpcClick().GetNpc())
		case m.GetCommand().HasNpcReply():
			r := m.GetCommand().GetNpcReply()
			if !validReply(r) {
				queued = p.Ack(seq)
				break
			}
			queued = p.NpcReply(seq, r)
		case m.GetCommand().HasShopBuy():
			b := m.GetCommand().GetShopBuy()
			lines, ok := buyLines(b)
			if !ok {
				queued = p.Ack(seq)
				break
			}
			queued = p.ShopBuy(seq, b.GetNpc(), lines)
		case m.GetCommand().HasShopSell():
			b := m.GetCommand().GetShopSell()
			lines, ok := sellLines(b)
			if !ok {
				queued = p.Ack(seq)
				break
			}
			queued = p.ShopSell(seq, b.GetNpc(), lines)
		case m.GetCommand().HasShopClose():
			queued = p.ShopClose(seq, m.GetCommand().GetShopClose().GetNpc())
		case m.GetCommand().HasEquipItem():
			e := m.GetCommand().GetEquipItem()
			if e.GetSlot() >= item.MaxInventory {
				// clif_parse_EquipItem ignores an index past MAX_INVENTORY.
				queued = p.Ack(seq)
				break
			}
			queued = p.EquipItem(seq, int(e.GetSlot()), e.GetPosition())
		case m.GetCommand().HasUnequipItem():
			slot := m.GetCommand().GetUnequipItem().GetSlot()
			if slot >= item.MaxInventory {
				queued = p.Ack(seq)
				break
			}
			queued = p.UnequipItem(seq, int(slot))
		case m.GetCommand().HasInsertCard():
			c := m.GetCommand().GetInsertCard()
			if c.GetCardSlot() >= item.MaxInventory || c.GetEquipSlot() >= item.MaxInventory {
				queued = p.Ack(seq)
				break
			}
			queued = p.InsertCard(seq, int(c.GetCardSlot()), int(c.GetEquipSlot()))
		case m.GetCommand().HasUseSkill():
			u := m.GetCommand().GetUseSkill()
			if u.GetSkillId() == 0 || u.GetSkillId() > math.MaxInt32 || u.GetLevel() > math.MaxInt32 {
				queued = p.Ack(seq)
				break
			}
			target := zone.SkillTarget{Entity: u.GetEntity(), HasEntity: u.HasEntity()}
			if u.HasCell() {
				cell := u.GetCell()
				if cell.GetX() > math.MaxInt16 || cell.GetY() > math.MaxInt16 {
					queued = p.Ack(seq)
					break
				}
				target.HasCell, target.X, target.Y = true, int(cell.GetX()), int(cell.GetY())
			}
			queued = p.UseSkill(seq, int(u.GetSkillId()), int(u.GetLevel()), target)
		case m.GetCommand().HasLearnSkill():
			id := m.GetCommand().GetLearnSkill().GetSkillId()
			if id == 0 || id > math.MaxInt32 {
				queued = p.Ack(seq)
				break
			}
			queued = p.LearnSkill(seq, int(id))
		default:
			// Unknown commands are ignored, so newer clients still work.
			queued = p.Ack(seq)
		}
		if !queued {
			return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_RATE_LIMITED, "Too many actions at once.")
		}
	}
}

var errBadMessage = errors.New("bad message")

// conn is a WebSocket connection and its counts.
type conn struct {
	*websocket.Conn
	stats *metrics.Conn
}

// Shutdown ends every session with DISCONNECT_REASON_SERVER_SHUTDOWN,
// so players see why the game stopped. Each session then leaves its
// zone, which saves its character. waserver calls it on SIGTERM, before
// it stops the zones.
func (g *Gateway) Shutdown() {
	g.ShutdownWith(wathenav1.DisconnectReason_DISCONNECT_REASON_SERVER_SHUTDOWN, shutdownText)
}

// ShutdownWith ends every session as Shutdown does, with another reason
// and text, such as DISCONNECT_REASON_RESTART for a planned restart
// (decision 0031).
func (g *Gateway) ShutdownWith(reason wathenav1.DisconnectReason, text string) {
	g.mu.Lock()
	all := make([]*online, 0, len(g.online))
	for _, o := range g.online {
		all = append(all, o)
	}
	g.mu.Unlock()
	for _, o := range all {
		o.end(reason, text)
	}
}

// claim makes a session the character's live one. It kicks the session
// it replaces and waits, up to KickWait, until that session left the
// zone, so its last save is queued before the character loads.
func (g *Gateway) claim(id int64) *online {
	me := &online{kick: make(chan struct{}), done: make(chan struct{})}
	g.mu.Lock()
	if g.online == nil {
		g.online = map[int64]*online{}
	}
	old := g.online[id]
	g.online[id] = me
	g.mu.Unlock()
	if old == nil {
		return me
	}
	old.end(wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_IN_ELSEWHERE, elsewhereText)
	t := time.NewTimer(KickWait)
	defer t.Stop()
	select {
	case <-old.done:
	case <-me.kick:
		// A newer login replaced this one while it waited.
	case <-t.C:
		// The older session's saves fail from here on, since this one
		// raises the save version when it loads.
		if g.Log != nil {
			g.Log.Warn("an older session did not end in time", "character", id)
		}
	}
	return me
}

// Kick closes the connections of characters, such as for a staff kick
// or a ban (decision 0030). The client gets the reason and the text. It
// returns the characters it closed.
func (g *Gateway) Kick(ids []int64, reason wathenav1.DisconnectReason, text string) []int64 {
	var kicked []int64
	var ends []*online
	g.mu.Lock()
	for _, id := range ids {
		if o := g.online[id]; o != nil {
			kicked = append(kicked, id)
			ends = append(ends, o)
		}
	}
	g.mu.Unlock()
	for _, o := range ends {
		o.end(reason, text)
	}
	return kicked
}

// release ends a session's claim on its character.
func (g *Gateway) release(id int64, me *online) {
	g.mu.Lock()
	if g.online[id] == me {
		delete(g.online, id)
	}
	g.mu.Unlock()
	close(me.done)
}

func (g *Gateway) characters() Characters {
	if g.Characters != nil {
		return g.Characters
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.memory == nil {
		g.memory = &memoryCharacters{world: g.World, ids: map[string]int64{}, looks: map[int64]memoryCharacter{}}
	}
	return g.memory
}

func (g *Gateway) loadFailed(ctx context.Context, c *conn, err error) error {
	if g.Log != nil {
		g.Log.Error("cannot load a character", "err", err)
	}
	return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_UNSPECIFIED, "The server cannot load your character. Try again later.")
}

// memoryCharacters keeps characters in memory while the server runs. A
// name's first session makes a new character. Its saves are what the
// next session of the name loads, so items a player held when its
// connection closed come back when it logs in again. Names that differ
// only in case are the same character, as in the database.
type memoryCharacters struct {
	world World
	mu    sync.Mutex
	ids   map[string]int64
	looks map[int64]memoryCharacter
}

type memoryCharacter struct {
	name string
	look zone.Look
	// saved is the last save, or nil before the first.
	saved *character.Character
	// session counts the loads. Only the latest session's saves count,
	// as a database's save version does.
	session int
}

func (m *memoryCharacters) Find(_ context.Context, name string, look zone.Look) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(name)
	id, ok := m.ids[key]
	if !ok {
		id = int64(len(m.ids) + 1)
		m.ids[key] = id
		m.looks[id] = memoryCharacter{name: name, look: look}
	}
	return id, nil
}

func (m *memoryCharacters) Load(_ context.Context, id int64) (*character.Character, zone.Saver, error) {
	m.mu.Lock()
	mc := m.looks[id]
	mc.session++
	m.looks[id] = mc
	m.mu.Unlock()
	if mc.saved != nil {
		return mc.saved.Clone(), memorySaver{m, id, mc.session}, nil
	}
	c := m.world.NewCharacter(mc.name, mc.look)
	return &c, memorySaver{m, id, mc.session}, nil
}

// memorySaver keeps a session's saves of a memory character.
type memorySaver struct {
	m       *memoryCharacters
	id      int64
	session int
}

// Save keeps c, unless a newer session loaded the character. The zone
// hands over a copy it no longer changes.
func (s memorySaver) Save(c *character.Character, _ bool) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	mc := s.m.looks[s.id]
	if mc.session != s.session {
		return
	}
	mc.saved = c
	s.m.looks[s.id] = mc
}

func readMessage(ctx context.Context, c *conn) (*wathenav1.ClientMessage, error) {
	typ, data, err := c.Read(ctx)
	if err == nil {
		c.stats.Received(len(data))
	}
	if err != nil {
		var ce websocket.CloseError
		if errors.As(err, &ce) && ce.Code == websocket.StatusMessageTooBig {
			return nil, errBadMessage
		}
		return nil, err
	}
	m := &wathenav1.ClientMessage{}
	if typ != websocket.MessageBinary || proto.Unmarshal(data, m) != nil {
		return nil, errBadMessage
	}
	return m, nil
}

func readHello(ctx context.Context, c *conn) (*wathenav1.Hello, uint32, error) {
	ctx, cancel := context.WithTimeout(ctx, HelloTimeout)
	defer cancel()
	m, err := readMessage(ctx, c)
	if err != nil {
		if errors.Is(err, errBadMessage) {
			return nil, 0, disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_BAD_MESSAGE, "The client sent a message the server cannot read.")
		}
		return nil, 0, err
	}
	if !m.HasHello() || m.GetSeq() != 1 {
		return nil, 0, disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_BAD_MESSAGE, "The client must say hello first.")
	}
	return m.GetHello(), m.GetSeq(), nil
}

func write(ctx context.Context, c *conn, m *wathenav1.ServerMessage) error {
	data, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	if len(data) > schema.MaxMessageBytes {
		return errors.New("gateway: message over the size limit")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageBinary, data); err != nil {
		return err
	}
	c.stats.Sent(len(data))
	return nil
}

// disconnect sends the reason and closes the connection.
func disconnect(ctx context.Context, c *conn, reason wathenav1.DisconnectReason, text string) error {
	_ = write(ctx, c, wathenav1.ServerMessage_builder{Disconnect: wathenav1.Disconnect_builder{
		Reason: reason, Message: text,
	}.Build()}.Build())
	_ = c.Close(websocket.StatusPolicyViolation, reason.String())
	return errClosed
}
