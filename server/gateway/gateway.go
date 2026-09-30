// Package gateway accepts web client connections, as docs/proposals/
// 0001-protocol.md describes: one WebSocket per session, binary frames
// of one ClientMessage or ServerMessage each, and Hello first.
package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// HelloTimeout is how long a new connection has to send Hello.
const HelloTimeout = 10 * time.Second

// MaxRaise is the most a RaiseStat may ask for at once. rAthena's
// clif_parse_StatusUp reads the amount as one byte.
const MaxRaise = 255

// Sessions checks session tokens. It returns the player's name and look.
type Sessions interface {
	Check(token []byte) (name string, look zone.Look, ok bool)
}

// DevSessions accepts any token that is a valid name, for development
// before the account service exists. Never use it on a public server.
type DevSessions struct{}

// Check takes the token as the name and derives a look from it.
func (DevSessions) Check(token []byte) (string, zone.Look, bool) {
	name := string(token)
	if !ValidName(name) {
		return "", zone.Look{}, false
	}
	h := uint32(2166136261)
	for _, b := range token {
		h = (h ^ uint32(b)) * 16777619
	}
	return name, zone.Look{Job: 0, Sex: h & 1, Hair: 1 + h>>1%20, HairColor: h >> 6 % 9}, true
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
	// and its saver. The saver is nil for a character that only lives
	// in memory.
	Load(ctx context.Context, id int64) (*character.Character, zone.Saver, error)
}

// Gateway serves the protocol over WebSocket.
//
// One character has at most one session. A login for a character that
// is already playing kicks the older session, as rAthena does, and waits
// until the older session's last save is queued before loading the
// character. The older connection gets DISCONNECT_REASON_LOGGED_IN_ELSEWHERE.
type Gateway struct {
	Zone     *zone.Zone
	Sessions Sessions
	// Characters loads characters. When nil, every session plays a new
	// character that only lives in memory.
	Characters Characters
	// Origins lists hosts pages may connect from, as coder/websocket's
	// OriginPatterns. Empty allows only the server's own origin.
	Origins []string
	Log     *slog.Logger

	mu     sync.Mutex
	online map[int64]*online
	memory *memoryCharacters
}

// online is a character's live session.
type online struct {
	// kick closes to end the session. done closes once the session has
	// left the zone.
	kick     chan struct{}
	kickOnce sync.Once
	done     chan struct{}
}

func (o *online) end() { o.kickOnce.Do(func() { close(o.kick) }) }

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: g.Origins,
		// Compression off: compressing chat alongside private data lets
		// an observer guess the data from message sizes.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	c.SetReadLimit(schema.MaxClientMessageBytes)
	ctx := r.Context()
	if err := g.session(ctx, c); err != nil && g.Log != nil {
		g.Log.Debug("session ended", "err", err)
	}
}

// errClosed ends a session that the server closed on purpose.
var errClosed = errors.New("closed")

func (g *Gateway) session(ctx context.Context, c *websocket.Conn) error {
	defer func() { _ = c.CloseNow() }()
	hello, seq, err := readHello(ctx, c)
	if err != nil {
		return err
	}
	if v := hello.GetProtocolVersion(); v < schema.MinVersion || v > schema.Version {
		return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_VERSION_UNSUPPORTED, "This client is too old or too new for the server.")
	}
	name, look, ok := g.Sessions.Check(hello.GetSessionToken())
	if !ok {
		return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_SESSION_INVALID, "Your session has ended. Log in again.")
	}
	chars := g.characters()
	id, err := chars.Find(ctx, name, look)
	if err != nil {
		return g.loadFailed(ctx, c, err)
	}
	me := g.claim(id)
	defer g.release(id, me)
	select {
	case <-me.kick:
		return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_IN_ELSEWHERE, "Your character logged in somewhere else.")
	default:
	}
	char, saver, err := chars.Load(ctx, id)
	if err != nil {
		return g.loadFailed(ctx, c, err)
	}
	p, netID, tick, err := g.Zone.JoinCharacter(ctx, char, saver)
	if err != nil {
		if saver != nil {
			// The session ends before it started. Its character is
			// unchanged, so the save only closes the session.
			saver.Save(char, true)
		}
		return err
	}
	defer g.Zone.Leave(p)
	welcome := wathenav1.ServerMessage_builder{Welcome: wathenav1.Welcome_builder{
		ProtocolVersion: schema.Version,
		ServerTimeMs:    uint64(time.Now().UnixMilli()), // #nosec G115 -- the clock is after 1970.
		Tick:            tick,
		NetId:           netID,
		Map:             g.Zone.Map,
	}.Build()}.Build()
	if err := write(ctx, c, welcome); err != nil {
		return err
	}
	g.Zone.Ack(p, seq)

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
				Reason:  wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_IN_ELSEWHERE,
				Message: "Your character logged in somewhere else.",
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
		// The zone dropped the player, such as for falling behind.
		_ = c.Close(websocket.StatusNormalClosure, "")
	}()
	return g.read(ctx, c, p, seq)
}

func (g *Gateway) read(ctx context.Context, c *websocket.Conn, p *zone.Player, seq uint32) error {
	chat := newChatLimit(time.Now())
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
			queued = g.Zone.Ack(p, seq)
		case m.GetCommand().HasAttack():
			a := m.GetCommand().GetAttack()
			queued = g.Zone.Attack(p, seq, a.GetTarget(), a.GetRepeat())
		case m.GetCommand().HasChat():
			ok, over := chat.allow(time.Now())
			if over {
				return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_RATE_LIMITED, "Too many chat messages.")
			}
			ch := m.GetCommand().GetChat()
			text, valid := CleanChat(ch.GetText())
			// Lines over the rate are dropped without a reply, as
			// rAthena's min_chat_delay does. Only local chat exists so
			// far, so other channels are ignored.
			if !ok || !valid || ch.GetChannel() != wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL {
				queued = g.Zone.Ack(p, seq)
				break
			}
			queued = g.Zone.Chat(p, seq, text)
		case m.GetCommand().HasPickUp():
			queued = g.Zone.PickUp(p, seq, m.GetCommand().GetPickUp().GetItem())
		case m.GetCommand().HasRaiseStat():
			r := m.GetCommand().GetRaiseStat()
			st, amount := r.GetStat(), r.GetAmount()
			if st < wathenav1.BaseStat_BASE_STAT_STR || st > wathenav1.BaseStat_BASE_STAT_LUK || amount < 1 || amount > MaxRaise {
				// A stat or amount out of range raises nothing, as
				// pc_statusup refuses it.
				queued = g.Zone.Ack(p, seq)
				break
			}
			queued = g.Zone.RaiseStat(p, seq, status.Stat(st-wathenav1.BaseStat_BASE_STAT_STR), int(amount))
		case m.GetCommand().HasUseItem():
			slot := m.GetCommand().GetUseItem().GetSlot()
			if slot >= item.MaxInventory {
				// clif_parse_UseItem ignores an index past MAX_INVENTORY.
				queued = g.Zone.Ack(p, seq)
				break
			}
			queued = g.Zone.UseItem(p, seq, int(slot))
		case m.GetCommand().HasDropItem():
			d := m.GetCommand().GetDropItem()
			if d.GetSlot() >= item.MaxInventory || d.GetAmount() > item.MaxAmount {
				// No slot or stack is that large, so nothing drops.
				queued = g.Zone.Ack(p, seq)
				break
			}
			queued = g.Zone.DropItem(p, seq, int(d.GetSlot()), int(d.GetAmount()))
		case m.GetCommand().HasWalkTo():
			w := m.GetCommand().GetWalkTo()
			queued = g.Zone.WalkTo(p, seq, int(w.GetX()), int(w.GetY()))
		default:
			// Unknown commands are ignored, so newer clients still work.
			queued = g.Zone.Ack(p, seq)
		}
		if !queued {
			return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_RATE_LIMITED, "Too many actions at once.")
		}
	}
}

var errBadMessage = errors.New("bad message")

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
	old.end()
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
		g.memory = &memoryCharacters{zone: g.Zone, ids: map[string]int64{}, looks: map[int64]memoryCharacter{}}
	}
	return g.memory
}

func (g *Gateway) loadFailed(ctx context.Context, c *websocket.Conn, err error) error {
	if g.Log != nil {
		g.Log.Error("cannot load a character", "err", err)
	}
	return disconnect(ctx, c, wathenav1.DisconnectReason_DISCONNECT_REASON_UNSPECIFIED, "The server cannot load your character. Try again later.")
}

// memoryCharacters gives every session a new character that only lives
// in memory. Names that differ only in case are the same character, as
// in the database.
type memoryCharacters struct {
	zone  *zone.Zone
	mu    sync.Mutex
	ids   map[string]int64
	looks map[int64]memoryCharacter
}

type memoryCharacter struct {
	name string
	look zone.Look
}

func (m *memoryCharacters) Find(_ context.Context, name string, look zone.Look) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(name)
	id, ok := m.ids[key]
	if !ok {
		id = int64(len(m.ids) + 1)
		m.ids[key] = id
	}
	m.looks[id] = memoryCharacter{name, look}
	return id, nil
}

func (m *memoryCharacters) Load(_ context.Context, id int64) (*character.Character, zone.Saver, error) {
	m.mu.Lock()
	mc := m.looks[id]
	m.mu.Unlock()
	c := m.zone.NewCharacter(mc.name, mc.look)
	return &c, nil, nil
}

func readMessage(ctx context.Context, c *websocket.Conn) (*wathenav1.ClientMessage, error) {
	typ, data, err := c.Read(ctx)
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

func readHello(ctx context.Context, c *websocket.Conn) (*wathenav1.Hello, uint32, error) {
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

func write(ctx context.Context, c *websocket.Conn, m *wathenav1.ServerMessage) error {
	data, err := proto.Marshal(m)
	if err != nil {
		return err
	}
	if len(data) > schema.MaxMessageBytes {
		return errors.New("gateway: message over the size limit")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.Write(ctx, websocket.MessageBinary, data)
}

// disconnect sends the reason and closes the connection.
func disconnect(ctx context.Context, c *websocket.Conn, reason wathenav1.DisconnectReason, text string) error {
	_ = write(ctx, c, wathenav1.ServerMessage_builder{Disconnect: wathenav1.Disconnect_builder{
		Reason: reason, Message: text,
	}.Build()}.Build())
	_ = c.Close(websocket.StatusPolicyViolation, reason.String())
	return errClosed
}
