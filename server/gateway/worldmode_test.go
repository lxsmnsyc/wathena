package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestWelcomeWorldMode: Welcome names the world's mode, and a procedural
// world's generator and rng versions (decision 0038).
func TestWelcomeWorldMode(t *testing.T) {
	for _, c := range []struct {
		mode WorldMode
		want wathenav1.WorldMode
		gen  uint32
		rng  string
	}{
		{WorldMode{}, wathenav1.WorldMode_WORLD_MODE_CLASSIC, 0, ""},
		{WorldMode{Procedural: true, GeneratorVersion: 1, RNGVersion: "rng-v1"}, wathenav1.WorldMode_WORLD_MODE_PROCEDURAL, 1, "rng-v1"},
	} {
		g := &path.Grid{Width: 10, Height: 10, Cells: make([]uint8, 100)}
		for i := range g.Cells {
			g.Cells[i] = path.Walkable | path.Shootable
		}
		z := zone.New("map/test", g, rng.FromKey([32]byte{}).Stream(), nil)
		ctx, cancel := context.WithCancel(context.Background())
		go z.Run(ctx)
		s := httptest.NewServer(&Gateway{World: z, Sessions: DevSessions{}, Mode: c.mode})
		a := dial(t, "ws"+strings.TrimPrefix(s.URL, "http"))
		a.hello("Alice", schema.Version)
		w := a.next((*wathenav1.ServerMessage).HasWelcome).GetWelcome().GetWorld()
		if w.GetMode() != c.want || w.GetGeneratorVersion() != c.gen || w.GetRngVersion() != c.rng {
			t.Errorf("%+v: welcome world %v", c.mode, w)
		}
		s.Close()
		cancel()
	}
}
