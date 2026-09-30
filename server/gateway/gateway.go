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
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// HelloTimeout is how long a new connection has to send Hello.
const HelloTimeout = 10 * time.Second

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

// Gateway serves the protocol over WebSocket.
type Gateway struct {
	Zone     *zone.Zone
	Sessions Sessions
	// Origins lists hosts pages may connect from, as coder/websocket's
	// OriginPatterns. Empty allows only the server's own origin.
	Origins []string
	Log     *slog.Logger
}

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
	p, netID, tick, err := g.Zone.Join(ctx, name, look)
	if err != nil {
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
		case m.GetCommand().HasPickUp():
			queued = g.Zone.PickUp(p, seq, m.GetCommand().GetPickUp().GetItem())
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
