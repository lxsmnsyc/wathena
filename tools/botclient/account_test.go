package botclient

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

type storeCharacters struct{ store *store.Store }

func (storeCharacters) Find(context.Context, string, zone.Look) (int64, error) {
	panic("tickets name their character")
}

func (s storeCharacters) Load(ctx context.Context, id int64) (*character.Character, zone.Saver, error) {
	return s.store.Load(ctx, id)
}

// TestAccountFlow signs a bot up on the account pages, makes a
// character, gets a game ticket, plays with it, and checks that the
// character was saved where it walked. It needs WATHENA_TEST_DATABASE_URL.
func TestAccountFlow(t *testing.T) {
	admin := storetest.Migrated(t)
	st := store.New(storetest.As(t, admin, "wathena_game"), store.Options{Log: slog.New(slog.DiscardHandler)})
	g := open(40)
	z := zone.New("map/test", g, rng.FromKey([32]byte{6}).Stream(), nil)
	const origin = "http://client.test"
	svc, err := account.New(account.Config{
		Pool: storetest.As(t, admin, "wathena_account"), ClientOrigins: []string{origin}, PlayURL: origin + "/play",
		NewCharacter: func(name string, look account.Look) character.Character {
			c := z.NewCharacter(name, zone.Look{Hair: uint32(look.Hair), HairColor: uint32(look.HairColor)}) // #nosec G115 -- small test values.
			c.Stats = status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}
			return c
		},
		Hasher: account.NewHasher(account.Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}, 2),
		Log:    slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	mux := http.NewServeMux()
	mux.Handle(account.Prefix, svc)
	mux.Handle("/ws", &gateway.Gateway{World: z, Sessions: account.GameTickets{Store: st}, Characters: storeCharacters{st}, Origins: []string{origin}})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	a := NewAccount(srv.URL, origin)
	steps := []struct {
		name string
		f    func() error
	}{
		{"sign up", func() error { return a.SignUp(ctx, "botaccount", "a fine long password") }},
		{"make a character", func() error { return a.MakeCharacter(ctx, "Bot Hero", 0) }},
		{"play", func() error { return a.Play(ctx, 0) }},
	}
	for _, s := range steps {
		if err := s.f(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
	ticket, err := a.Ticket(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dial, dcancel := context.WithTimeout(ctx, 10*time.Second)
	defer dcancel()
	b, err := Dial(dial, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", ticket, Options{})
	if err != nil {
		t.Fatal(err)
	}
	v := b.View()
	if err := b.WalkTo(ctx, v.X+2, v.Y); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the walk", func() bool { w := b.View(); return w.X == v.X+2 && !w.Walking })
	// The bot follows its walk by the clock. The zone takes the last step
	// at its next tick.
	time.Sleep(3 * zone.Tick)
	// The ticket is used up.
	if _, err := Dial(dial, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", ticket, Options{}); err == nil || !strings.Contains(err.Error(), "SESSION_INVALID") {
		t.Errorf("a second use of the ticket: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	// Leaving saves the character where it stands.
	eventually(t, "the save", func() bool {
		var x int
		err := admin.QueryRow(ctx, "SELECT x FROM characters WHERE name = 'Bot Hero'").Scan(&x)
		return err == nil && x == v.X+2
	})
	cancel()
	<-z.Stopped()
	drain, dc := context.WithTimeout(context.Background(), 10*time.Second)
	defer dc()
	if err := st.Close(drain); err != nil {
		t.Fatal(err)
	}
}
