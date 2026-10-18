package botclient

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Behaviour is what a bot does while it plays.
type Behaviour struct {
	// WalkEvery is the mean time between walks. Each wait is 0.5 to 1.5
	// times it. 0 means no walking.
	WalkEvery time.Duration `yaml:"walk_every"`
	// WalkRadius is how far a walk goes, in cells on each axis. It is 8
	// when 0. Paths are at most 32 steps (path.MaxWalkPath).
	WalkRadius int `yaml:"walk_radius"`
	// Stay keeps walks within WalkRadius of the cell where the bot
	// entered its map, instead of around its position. Bots that enter
	// at the same cell then stay in view of each other, as a crowd at an
	// event does.
	Stay bool `yaml:"stay"`
	// ChatEvery is the mean time between chat lines, which come at
	// random times (a Poisson process). 0 means no chat.
	ChatEvery time.Duration `yaml:"chat_every"`
	// ChatLength is the length of each line in characters. It is 40 when
	// 0, as in the bandwidth estimate in schema/protocol_test.go.
	ChatLength int `yaml:"chat_length"`
	// Fight attacks the nearest monster in view until it dies, before
	// walking on.
	Fight bool `yaml:"fight"`
	// WarpChance is the chance that a walk goes to the nearest warp
	// portal in view, which takes the bot to another map.
	WarpChance float64 `yaml:"warp_chance"`
	// Travel walks each bot along a route to a portal of its map, other
	// than the one it arrived at, in legs of up to 24 cells. Each leg
	// starts when the last one ended and WalkEvery has passed. It needs
	// Grids and Portals; on a map without them, the bot walks as without
	// Travel.
	Travel bool `yaml:"travel"`
	// Portals returns the cells of a map's warps by map name, for
	// Travel.
	Portals func(mapName string) [][2]int `yaml:"-"`
	// PingEvery is the time between pings. 0 means none, as the web
	// client sends none.
	PingEvery time.Duration `yaml:"ping_every"`
	// Grids returns a map's cells by asset name, such as map/prontera,
	// so walks only go to cells a path reaches. When it is nil or returns
	// nil, bots pick cells blindly and the server ignores bad ones.
	Grids func(mapName string) *path.Grid `yaml:"-"`
}

// think is how often a playing bot decides what to do.
const think = 100 * time.Millisecond

// giveUp is how long a bot attacks one monster before it picks another.
const giveUp = 15 * time.Second

// Play runs a behaviour until ctx ends or the connection closes. It
// returns nil when ctx ended, and the connection's error otherwise.
func (b *Bot) Play(ctx context.Context, beh Behaviour, rnd *rand.Rand) error {
	now := time.Now()
	jitter := func(d time.Duration) time.Duration {
		return time.Duration(float64(d) * (0.5 + rnd.Float64()))
	}
	// The first walk and line come at a random time, so bots that joined
	// together do not act together.
	var nextWalk, nextChat, nextPing time.Time
	if beh.WalkEvery > 0 {
		nextWalk = now.Add(time.Duration(rnd.Float64() * float64(beh.WalkEvery)))
	}
	if beh.ChatEvery > 0 {
		nextChat = now.Add(time.Duration(rnd.ExpFloat64() * float64(beh.ChatEvery)))
	}
	if beh.PingEvery > 0 {
		nextPing = now.Add(jitter(beh.PingEvery))
	}
	var target uint32
	var targetSince time.Time
	// home is where the bot entered its map, for Stay.
	home := b.View()
	var trip travel
	lines := 0
	// Bots that joined together start thinking at different times.
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(time.Duration(rnd.Int64N(int64(think)))):
	}
	t := time.NewTicker(think)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-b.done:
			return b.Err()
		case now = <-t.C:
		}
		v := b.View()
		if v.Map != home.Map {
			home = v
		}
		var err error
		if beh.Fight && v.HP > 0 {
			if target != 0 && (!sees(v, target) || now.Sub(targetSince) > giveUp) {
				target = 0
			}
			if target == 0 {
				if m, ok := nearestMonster(v); ok {
					target, targetSince = m, now
					err = b.Attack(ctx, m, true)
				}
			}
		}
		if err == nil && target == 0 && beh.WalkEvery > 0 && !now.Before(nextWalk) && (!beh.Travel || !v.Walking) {
			nextWalk = now.Add(jitter(beh.WalkEvery))
			x, y, ok := 0, 0, false
			if beh.Travel {
				x, y, ok = beh.travelPick(&trip, v, rnd)
			}
			if !ok {
				x, y, ok = beh.pick(v, home, rnd)
			}
			if ok {
				err = b.WalkTo(ctx, x, y)
			}
		}
		if err == nil && beh.ChatEvery > 0 && !now.Before(nextChat) {
			nextChat = now.Add(time.Duration(rnd.ExpFloat64() * float64(beh.ChatEvery)))
			lines++
			err = b.Chat(ctx, chatLine(b.Name, lines, beh.ChatLength))
		}
		if err == nil && beh.PingEvery > 0 && !now.Before(nextPing) {
			nextPing = now.Add(jitter(beh.PingEvery))
			err = b.Ping(ctx)
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			select {
			case <-b.done:
				return b.Err()
			default:
			}
			return err
		}
	}
}

func sees(v View, id uint32) bool {
	for _, e := range v.Entities {
		if e.ID == id {
			return true
		}
	}
	return false
}

func chebyshev(x0, y0, x1, y1 int) int { return max(abs(x1-x0), abs(y1-y0)) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// nearestMonster is the closest monster in view, by where its walk ends.
func nearestMonster(v View) (uint32, bool) {
	best, dist := uint32(0), 1<<30
	for _, e := range v.Entities {
		if e.Kind != wathenav1.EntityKind_ENTITY_KIND_MONSTER {
			continue
		}
		if d := chebyshev(v.X, v.Y, e.DestX, e.DestY); d < dist {
			best, dist = e.ID, d
		}
	}
	return best, best != 0
}

// pick chooses where to walk: a warp portal in view at WarpChance, else
// a cell that a path reaches, around the bot or around home with Stay.
func (beh Behaviour) pick(v, home View, rnd *rand.Rand) (int, int, bool) {
	if beh.WarpChance > 0 && rnd.Float64() < beh.WarpChance {
		best, dist := Entity{}, 1<<30
		for _, e := range v.Entities {
			if d := chebyshev(v.X, v.Y, e.X, e.Y); e.Warp() && d < dist {
				best, dist = e, d
			}
		}
		if dist < 1<<30 {
			return best.X, best.Y, true
		}
	}
	var g *path.Grid
	if beh.Grids != nil {
		g = beh.Grids(v.Map)
	}
	cx, cy, r := v.X, v.Y, beh.WalkRadius
	if r <= 0 {
		r = 8
	}
	if beh.Stay {
		cx, cy = home.X, home.Y
	}
	for range 20 {
		x := cx + rnd.IntN(2*r+1) - r
		y := cy + rnd.IntN(2*r+1) - r
		if x == v.X && y == v.Y {
			continue
		}
		if g == nil {
			return x, y, true
		}
		if g.NoPass(x, y) {
			continue
		}
		if steps, ok := path.Search(g, v.X, v.Y, x, y, false); ok && len(steps) > 0 {
			return x, y, true
		}
	}
	return 0, 0, false
}

// chatLine is a line of chat of a given length, with the bot's name and
// a count so lines differ.
func chatLine(name string, n, length int) string {
	if length <= 0 {
		length = 40
	}
	s := fmt.Sprintf("%s line %d ", name, n)
	if len(s) < length {
		s += strings.Repeat("abcdefghij", length/10+1)
	}
	return s[:length]
}
