package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// fakeCharacters keeps characters in memory as a database would: a save
// is what the next load returns.
type fakeCharacters struct {
	z   *zone.Zone
	mu  sync.Mutex
	ids map[string]int64
	db  map[int64]*character.Character
	log []string
}

func (f *fakeCharacters) Find(_ context.Context, name string, look zone.Look) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := strings.ToLower(name)
	if id, ok := f.ids[key]; ok {
		return id, nil
	}
	c := f.z.NewCharacter(name, look)
	c.ID = int64(len(f.ids) + 1)
	f.ids[key] = c.ID
	f.db[c.ID] = &c
	return c.ID, nil
}

func (f *fakeCharacters) Load(_ context.Context, id int64) (*character.Character, zone.Saver, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = append(f.log, "load")
	c := *f.db[id]
	return &c, fakeSaver{f}, nil
}

type fakeSaver struct{ f *fakeCharacters }

func (s fakeSaver) Save(c *character.Character, final bool) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	s.f.db[c.ID] = c
	if final {
		s.f.log = append(s.f.log, "final save")
	}
}

func TestSecondLoginKicksFirst(t *testing.T) {
	g := &path.Grid{Width: 40, Height: 40, Cells: make([]uint8, 1600)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New("map/test", g, rng.FromKey([32]byte{}).Stream(), nil)
	chars := &fakeCharacters{z: z, ids: map[string]int64{}, db: map[int64]*character.Character{}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go z.Run(ctx)
	s := httptest.NewServer(&Gateway{Zone: z, Sessions: DevSessions{}, Characters: chars})
	t.Cleanup(s.Close)
	url := "ws" + strings.TrimPrefix(s.URL, "http")

	a := dial(t, url)
	a.hello("Alice", schema.Version)
	a.next((*wathenav1.ServerMessage).HasWelcome)
	// Alice walks, so her character changes.
	a.send(wathenav1.ClientMessage_builder{Command: wathenav1.Command_builder{
		WalkTo: wathenav1.WalkTo_builder{X: 5, Y: 5}.Build(),
	}.Build()}.Build())
	a.next(hasEvent((*wathenav1.Event).HasMove))

	// The same character, by a name that differs in case.
	b := dial(t, url)
	b.hello("alice", schema.Version)
	d := a.next((*wathenav1.ServerMessage).HasDisconnect).GetDisconnect()
	if d.GetReason() != wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_IN_ELSEWHERE {
		t.Errorf("first session ended with %v", d.GetReason())
	}
	w := b.next((*wathenav1.ServerMessage).HasWelcome).GetWelcome()
	if w.GetNetId() == 0 {
		t.Fatalf("welcome %v", w)
	}
	chars.mu.Lock()
	defer chars.mu.Unlock()
	// The first session's last save came before the second load.
	if got := strings.Join(chars.log, ", "); got != "load, final save, load" {
		t.Errorf("order: %s", got)
	}
}

// sessionTokens gives each name its own login session: "name/42" is the
// character name with login session 42.
type sessionTokens struct{}

func (sessionTokens) Check(ctx context.Context, token []byte) (Login, error) {
	name, id, ok := strings.Cut(string(token), "/")
	if !ok {
		return Login{}, ErrSessionInvalid
	}
	l, err := DevSessions{}.Check(ctx, []byte(name))
	n := int64(0)
	for _, c := range id {
		n = n*10 + int64(c-'0')
	}
	l.SessionID = n
	return l, err
}

func TestEndLoginSessions(t *testing.T) {
	g := &path.Grid{Width: 40, Height: 40, Cells: make([]uint8, 1600)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New("map/test", g, rng.FromKey([32]byte{}).Stream(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go z.Run(ctx)
	gw := &Gateway{Zone: z, Sessions: sessionTokens{}}
	s := httptest.NewServer(gw)
	t.Cleanup(s.Close)
	url := "ws" + strings.TrimPrefix(s.URL, "http")

	a := dial(t, url)
	a.hello("Alice/1", schema.Version)
	a.next((*wathenav1.ServerMessage).HasWelcome)
	b := dial(t, url)
	b.hello("Bob/2", schema.Version)
	b.next((*wathenav1.ServerMessage).HasWelcome)

	// Ending session 1 closes Alice's connection only.
	gw.EndLoginSessions(1)
	d := a.next((*wathenav1.ServerMessage).HasDisconnect).GetDisconnect()
	if d.GetReason() != wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_OUT {
		t.Errorf("Alice ended with %v", d.GetReason())
	}
	b.send(wathenav1.ClientMessage_builder{Ping: wathenav1.Ping_builder{Nonce: 7}.Build()}.Build())
	if p := b.next((*wathenav1.ServerMessage).HasPong).GetPong(); p.GetNonce() != 7 {
		t.Errorf("Bob's pong %v", p)
	}

	// A connection from a session that already ended is closed as soon
	// as it arrives, as when its ticket was used just before the logout.
	late := dial(t, url)
	late.hello("Carol/1", schema.Version)
	d = late.next((*wathenav1.ServerMessage).HasDisconnect).GetDisconnect()
	if d.GetReason() != wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_OUT {
		t.Errorf("late connection ended with %v", d.GetReason())
	}
	// The clients hang up, so the server does not wait for their close
	// frames.
	_ = a.c.CloseNow()
	_ = late.c.CloseNow()
	// The closed connections stop being watched. Only Bob's is left.
	deadline := time.Now().Add(5 * time.Second)
	for {
		gw.mu.Lock()
		n := len(gw.logins)
		gw.mu.Unlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d login sessions still watched", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
