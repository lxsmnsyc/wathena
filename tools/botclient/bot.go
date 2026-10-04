// Package botclient is a headless client for load and end-to-end tests
// (strategy section 12.1). A Bot speaks the web client's protocol with
// the generated message code, tracks what it sees from the server's
// events, and counts its bytes. Behaviour drives a bot: it walks, chats,
// fights monsters and takes warps. RunScenario runs many bots against a
// server and reports what they and the server measured.
package botclient

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/metrics"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// ClientName is what bots send as Hello.client_name.
const ClientName = "wathena-botclient"

// writeTimeout is how long a bot waits to send one message.
const writeTimeout = 10 * time.Second

// Entity is something a bot sees.
type Entity struct {
	ID   uint32
	Kind wathenav1.EntityKind
	// X and Y are where the entity was last seen standing or starting a
	// walk. DestX and DestY are where its last walk ends.
	X, Y, DestX, DestY int
	Name               string
	// Body is the look's body: a job, a monster or an NPC sprite.
	Body uint32
	// ItemID and Amount are a ground item's.
	ItemID uint32
	Amount int
	// Guild is a player's guild badge, or nil.
	Guild *wathenav1.GuildBadge
	// ShopTitle is the title of a player's vending shop or buying store,
	// empty without one. ShopBuying is set for a buying store.
	ShopTitle  string
	ShopBuying bool
}

// Warp reports whether the entity is a warp portal.
func (e Entity) Warp() bool {
	return e.Kind == wathenav1.EntityKind_ENTITY_KIND_NPC && e.Body == warpSprite
}

// warpSprite is the NPC sprite of warp portals, zone.WarpSprite.
const warpSprite = 45

// walk is the bot's own walk, from a Move event.
type walk struct {
	fromX, fromY int
	steps        []path.Dir
	start        time.Time
	speed        int
}

// Bot is one connection. Its methods may be called from any goroutine.
type Bot struct {
	Name string

	conn *websocket.Conn
	wmu  sync.Mutex
	seq  uint32

	mu       sync.Mutex
	mapName  string
	netID    uint32
	x, y     int
	walk     *walk
	hp, maxH int64
	seen     map[uint32]*Entity
	// hit holds the entities the bot damaged, for Kills.
	hit      map[uint32]bool
	sent     map[uint32]time.Time
	reason   wathenav1.DisconnectReason
	text     string
	mapMoves int

	bytesIn, bytesOut, msgsIn, msgsOut atomic.Uint64
	// maxIn is the size of the largest message the bot got.
	maxIn                     atomic.Uint64
	chatsHeard, damage, kills atomic.Uint64
	ack                       *metrics.Histogram

	// ready closes when the bot first sees itself.
	ready     chan struct{}
	readyOnce sync.Once

	// game is the bot's own state from its events (game.go). record
	// keeps every event, and changed closes and is replaced when events
	// come.
	game      gameState
	social    socialState
	guildSt   guildState
	storageSt storageState
	tradeSt   tradeState
	marketSt  marketState
	record    bool
	events    []*wathenav1.Event
	changed   chan struct{}

	done chan struct{}
	err  error
}

// Options are settings for Dial.
type Options struct {
	// Ack, when set, records the time from each walk or chat command to
	// the first frame that acknowledges it. It includes the wait for the
	// next tick. Many bots may share one.
	Ack *metrics.Histogram
	// Record keeps every event the bot gets, for Events and Wait, as
	// end-to-end tests need. Load tests leave it off, since the list
	// grows for as long as the bot plays.
	Record bool
}

// Dial connects to a server's WebSocket URL, such as
// ws://127.0.0.1:8300/ws, and says hello with a session token: a game
// ticket, or a character name for a server with development sessions.
// It returns once the bot sees itself in its first frame.
func Dial(ctx context.Context, url string, token []byte, opts Options) (*Bot, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(schema.MaxMessageBytes)
	b := &Bot{Name: string(token), conn: conn, seen: map[uint32]*Entity{}, hit: map[uint32]bool{}, sent: map[uint32]time.Time{}, done: make(chan struct{}), ready: make(chan struct{}), ack: opts.Ack,
		record: opts.Record, changed: make(chan struct{}), game: newGameState()}
	err = b.send(ctx, wathenav1.ClientMessage_builder{Hello: wathenav1.Hello_builder{
		ProtocolVersion: schema.Version, SessionToken: token, ClientName: ClientName, Language: "en",
	}.Build()}, false)
	if err != nil {
		_ = conn.CloseNow()
		return nil, err
	}
	m, err := b.read(ctx)
	if err != nil {
		_ = conn.CloseNow()
		return nil, err
	}
	if d := m.GetDisconnect(); d != nil {
		_ = conn.CloseNow()
		return nil, fmt.Errorf("botclient: %s: %s", d.GetReason(), d.GetMessage())
	}
	w := m.GetWelcome()
	if w == nil {
		_ = conn.CloseNow()
		return nil, errors.New("botclient: the first message is not Welcome")
	}
	b.mapName, b.netID = w.GetMap(), w.GetNetId()
	b.social.limits = limitsOf(w.GetLimits())
	go b.readLoop() // #nosec G118 -- the loop lasts as long as the connection, not the dial's context.
	select {
	case <-b.ready:
		return b, nil
	case <-b.done:
		if err := b.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("botclient: the connection closed before the first frame")
	case <-ctx.Done():
		_ = conn.CloseNow()
		return nil, ctx.Err()
	}
}

// wsHeader is the size of a WebSocket frame header, as metrics counts it.
func wsHeader(n int) uint64 {
	switch {
	case n <= 125:
		return 2
	case n <= 0xFFFF:
		return 4
	}
	return 10
}

func (b *Bot) read(ctx context.Context) (*wathenav1.ServerMessage, error) {
	typ, data, err := b.conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	b.bytesIn.Add(uint64(len(data)) + wsHeader(len(data)))
	b.msgsIn.Add(1)
	for n := uint64(len(data)); ; {
		old := b.maxIn.Load()
		if n <= old || b.maxIn.CompareAndSwap(old, n) {
			break
		}
	}
	if typ != websocket.MessageBinary {
		return nil, errors.New("botclient: a text message")
	}
	m := &wathenav1.ServerMessage{}
	if err := proto.Unmarshal(data, m); err != nil {
		return nil, err
	}
	return m, nil
}

// send numbers and sends a message. track records its time, for Ack.
//
// A context that has ended sends nothing. Once the write starts, ctx no
// longer stops it: the WebSocket library closes the connection when a
// write's context ends in the middle of the write, so a caller's
// deadline would end the session. Only writeTimeout stops a write.
// A message that was not sent gives its sequence number back, since the
// server closes a connection whose numbers skip one.
func (b *Bot) send(ctx context.Context, m wathenav1.ClientMessage_builder, track bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.wmu.Lock()
	defer b.wmu.Unlock()
	m.Seq = b.seq + 1
	data, err := proto.Marshal(m.Build())
	if err != nil {
		return err
	}
	track = track && b.ack != nil
	if track {
		b.mu.Lock()
		b.sent[m.Seq] = time.Now()
		b.mu.Unlock()
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	if err := b.conn.Write(wctx, websocket.MessageBinary, data); err != nil {
		if track {
			b.mu.Lock()
			delete(b.sent, m.Seq)
			b.mu.Unlock()
		}
		return err
	}
	b.seq++
	// Client frames carry a 4-byte mask key.
	b.bytesOut.Add(uint64(len(data)) + wsHeader(len(data)) + 4)
	b.msgsOut.Add(1)
	return nil
}

func (b *Bot) readLoop() {
	defer close(b.done)
	for {
		m, err := b.read(context.Background())
		if err != nil {
			b.mu.Lock()
			if b.err == nil {
				b.err = err
			}
			b.mu.Unlock()
			return
		}
		b.apply(m)
	}
}

// Done closes when the connection ends.
func (b *Bot) Done() <-chan struct{} { return b.done }

// Err is why the connection ended: a Disconnect from the server, or a
// read error. It is nil while the connection is open.
func (b *Bot) Err() error {
	select {
	case <-b.done:
	default:
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reason != wathenav1.DisconnectReason_DISCONNECT_REASON_UNSPECIFIED || b.text != "" {
		return &DisconnectError{Reason: b.reason, Message: b.text}
	}
	return b.err
}

// DisconnectError is a Disconnect message from the server.
type DisconnectError struct {
	Reason  wathenav1.DisconnectReason
	Message string
}

func (e *DisconnectError) Error() string {
	return fmt.Sprintf("botclient: disconnected: %s: %s", e.Reason, e.Message)
}

// Close closes the connection, as a player who closes the page.
func (b *Bot) Close() error {
	err := b.conn.Close(websocket.StatusNormalClosure, "")
	<-b.done
	return err
}

// CloseNow closes the connection at once, without the close handshake,
// as a client whose network drops.
func (b *Bot) CloseNow() error {
	err := b.conn.CloseNow()
	<-b.done
	return err
}

// WalkTo asks to walk to a cell.
func (b *Bot) WalkTo(ctx context.Context, x, y int) error {
	return b.send(ctx, wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		WalkTo: wathenav1.WalkTo_builder{X: uint32(max(x, 0)), Y: uint32(max(y, 0))}.Build(), // #nosec G115 -- map cells.
	}.Build()}, true)
}

// Chat says a line of local chat.
func (b *Bot) Chat(ctx context.Context, text string) error {
	return b.send(ctx, wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		Chat: wathenav1.Chat_builder{Channel: wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL, Text: text}.Build(),
	}.Build()}, true)
}

// Attack attacks an entity, by its ID, once or until it dies.
func (b *Bot) Attack(ctx context.Context, id uint32, repeat bool) error {
	return b.send(ctx, wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		Attack: wathenav1.Attack_builder{Target: id, Repeat: repeat}.Build(),
	}.Build()}, false)
}

// Ping sends a ping. The gateway answers it directly.
func (b *Bot) Ping(ctx context.Context) error {
	return b.send(ctx, wathenav1.ClientMessage_builder{Ping: wathenav1.Ping_builder{Nonce: 1}.Build()}, false)
}

// Counts are a bot's bytes and messages, counted as metrics.Conns
// counts them, and what it saw.
type Counts struct {
	BytesIn, BytesOut, MsgsIn, MsgsOut uint64
	ChatsHeard, Damage                 uint64
	// Kills counts monsters the bot hit that died.
	Kills uint64
	// MapChanges counts warps to other maps.
	MapChanges int
	// MaxMessage is the size in bytes of the largest message the bot
	// got.
	MaxMessage uint64
}

// Counts returns the bot's counts so far.
func (b *Bot) Counts() Counts {
	b.mu.Lock()
	moves := b.mapMoves
	b.mu.Unlock()
	return Counts{
		BytesIn: b.bytesIn.Load(), BytesOut: b.bytesOut.Load(), MsgsIn: b.msgsIn.Load(), MsgsOut: b.msgsOut.Load(),
		ChatsHeard: b.chatsHeard.Load(), Damage: b.damage.Load(), Kills: b.kills.Load(), MapChanges: moves,
		MaxMessage: b.maxIn.Load(),
	}
}

// View is what a bot knows about itself and its surroundings.
type View struct {
	Map   string
	NetID uint32
	// X and Y are the bot's cell now, from its last walk and the time.
	X, Y int
	// Walking is set while the bot's last walk has steps left.
	Walking   bool
	HP, MaxHP int64
	// Guild is the bot's own guild badge, as others see it, or nil.
	Guild    *wathenav1.GuildBadge
	Entities []Entity
}

// View returns what the bot knows now.
func (b *Bot) View() View {
	b.mu.Lock()
	defer b.mu.Unlock()
	x, y, walking := b.position(time.Now())
	v := View{Map: b.mapName, NetID: b.netID, X: x, Y: y, Walking: walking, HP: b.hp, MaxHP: b.maxH}
	v.Entities = make([]Entity, 0, len(b.seen))
	if self := b.seen[b.netID]; self != nil {
		v.Guild = self.Guild
	}
	for _, e := range b.seen {
		if e.ID != b.netID {
			v.Entities = append(v.Entities, *e)
		}
	}
	return v
}

// position is the bot's cell at a time, following its last walk. The
// caller holds mu.
func (b *Bot) position(now time.Time) (int, int, bool) {
	w := b.walk
	if w == nil {
		return b.x, b.y, false
	}
	x, y := w.fromX, w.fromY
	t := w.start
	for _, d := range w.steps {
		t = t.Add(time.Duration(path.StepTime(d, w.speed)) * time.Millisecond)
		if t.After(now) {
			return x, y, true
		}
		dx, dy := d.Step()
		x, y = x+dx, y+dy
	}
	return x, y, false
}

// dirOf turns a protocol facing (0 south, clockwise) into a walk
// direction (0 north, counterclockwise), the inverse of the zone's
// facing.
func dirOf(f byte) path.Dir { return path.Dir((4 - int(f) + 8) % 8) }

func unpackSteps(b []byte) []path.Dir {
	out := make([]path.Dir, 0, 2*len(b))
	for _, v := range b {
		for _, f := range [2]byte{v & 0x0F, v >> 4} {
			if f > 7 {
				return out
			}
			out = append(out, dirOf(f))
		}
	}
	return out
}

func (b *Bot) apply(m *wathenav1.ServerMessage) {
	if d := m.GetDisconnect(); d != nil {
		b.mu.Lock()
		b.reason, b.text = d.GetReason(), d.GetMessage()
		b.mu.Unlock()
		return
	}
	f := m.GetFrame()
	if f == nil {
		return
	}
	now := time.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	for seq, at := range b.sent {
		if seq <= f.GetAckSeq() {
			b.ack.Record(now.Sub(at))
			delete(b.sent, seq)
		}
	}
	for _, e := range f.GetEvents() {
		b.event(e, now)
		b.game.apply(e)
		b.social.apply(e)
		b.guildSt.apply(e)
		b.storageSt.apply(e)
		b.tradeSt.apply(e)
		b.marketSt.apply(e)
	}
	if b.record {
		b.events = append(b.events, f.GetEvents()...)
	}
	close(b.changed)
	b.changed = make(chan struct{})
}

// event applies one event. The caller holds mu.
func (b *Bot) event(e *wathenav1.Event, now time.Time) {
	switch {
	case e.HasMapChange():
		c := e.GetMapChange()
		b.mapName, b.netID = c.GetMap(), c.GetNetId()
		b.x, b.y, b.walk = int(c.GetX()), int(c.GetY()), nil
		clear(b.seen)
		clear(b.hit)
		b.mapMoves++
	case e.HasAppear():
		a := e.GetAppear()
		x, y := int(a.GetX()), int(a.GetY())
		b.seen[a.GetId()] = &Entity{ID: a.GetId(), Kind: a.GetKind(), X: x, Y: y, DestX: x, DestY: y, Name: a.GetName(), Body: a.GetLook().GetBody(),
			ItemID: a.GetItem().GetItemId(), Amount: int(a.GetItem().GetAmount()), Guild: a.GetGuild()}
		if a.GetId() == b.netID {
			b.x, b.y, b.walk = x, y, nil
			b.readyOnce.Do(func() { close(b.ready) })
		}
	case e.HasMove():
		mv := e.GetMove()
		steps := unpackSteps(mv.GetSteps())
		x, y := int(mv.GetFromX()), int(mv.GetFromY())
		dx, dy := x, y
		for _, d := range steps {
			sx, sy := d.Step()
			dx, dy = dx+sx, dy+sy
		}
		if en := b.seen[mv.GetId()]; en != nil {
			en.X, en.Y, en.DestX, en.DestY = x, y, dx, dy
		}
		if mv.GetId() == b.netID {
			b.walk = &walk{fromX: x, fromY: y, steps: steps, start: now.Add(time.Duration(mv.GetStartMs()) * time.Millisecond), speed: int(max(mv.GetWalkMs(), 1))}
			b.x, b.y = dx, dy
		}
	case e.HasStop():
		s := e.GetStop()
		x, y := int(s.GetX()), int(s.GetY())
		if en := b.seen[s.GetId()]; en != nil {
			en.X, en.Y, en.DestX, en.DestY = x, y, x, y
		}
		if s.GetId() == b.netID {
			b.x, b.y, b.walk = x, y, nil
		}
	case e.HasEntityGuild():
		g := e.GetEntityGuild()
		if en := b.seen[g.GetId()]; en != nil {
			en.Guild = g.GetGuild()
		}
	case e.HasEntityShop():
		sh := e.GetEntityShop()
		if en := b.seen[sh.GetId()]; en != nil {
			en.ShopTitle, en.ShopBuying = sh.GetTitle(), sh.GetBuying()
			if sh.GetRemove() {
				en.ShopTitle, en.ShopBuying = "", false
			}
		}
	case e.HasVanish():
		v := e.GetVanish()
		if v.GetReason() == wathenav1.VanishReason_VANISH_REASON_DIED && b.hit[v.GetId()] {
			b.kills.Add(1)
		}
		delete(b.seen, v.GetId())
		delete(b.hit, v.GetId())
	case e.HasChat():
		b.chatsHeard.Add(1)
	case e.HasDamage():
		if d := e.GetDamage(); d.GetSource() == b.netID {
			b.damage.Add(uint64(max(d.GetAmount(), 0)))
			b.hit[d.GetTarget()] = true
		}
	case e.HasStats():
		for _, s := range e.GetStats().GetStats() {
			switch s.GetId() {
			case wathenav1.StatId_STAT_ID_HP:
				b.hp = s.GetValue()
			case wathenav1.StatId_STAT_ID_MAX_HP:
				b.maxH = s.GetValue()
			}
		}
	}
}
