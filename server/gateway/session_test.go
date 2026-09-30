package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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
