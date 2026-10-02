package progression

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

var testItems = map[uint32]string{
	501: "Healing", 601: "Usable", 909: "Etc", 1201: "Weapon", 4001: "Card", 9001: "PetEgg", 12000: "Cash",
}

func TestAdjustDropsDefaults(t *testing.T) {
	// Default rates keep every drop as mob_db.yml has it.
	tb := tables(t, reference.Renewal)
	d, _ := load(t, reference.Renewal)
	for _, m := range d.Mobs[:200] {
		got := tb.AdjustDrops(BossNone, m.Drops)
		if !slices.Equal(got, m.Drops) {
			t.Fatalf("mob %d: %v, want %v", m.ID, got, m.Drops)
		}
	}
}

func TestAdjustDropsRates(t *testing.T) {
	cfg := DefaultConfig
	cfg.Common = ItemRate{Rate: 300, Boss: 100, MVP: 100, Min: 1, Max: 10000}
	cfg.Card = ItemRate{Rate: 50, Boss: 200, MVP: 0, Min: 1, Max: 10000}
	cfg.Heal = ItemRate{Rate: 100, Boss: 100, MVP: 100, Min: 500, Max: 2000}
	tb := New(&Data{}, testItems, cfg)
	drops := []Drop{
		{Item: 909, Rate: 2750}, {Item: 909, Rate: 5000}, {Item: 4001, Rate: 1},
		{Item: 501, Rate: 10}, {Item: 501, Rate: 9000}, {Item: 7777, Rate: 100}, {Item: 909, Rate: 0},
	}
	// Common drops triple up to 100%, a 0.005% card rounds up to the
	// minimum, heals stay within 5% to 20%, and unknown items and rates of
	// 0 go.
	want := []Drop{{Item: 909, Rate: 8250}, {Item: 909, Rate: 10000}, {Item: 4001, Rate: 1}, {Item: 501, Rate: 500}, {Item: 501, Rate: 2000}}
	if got := tb.AdjustDrops(BossNone, drops); !slices.Equal(got, want) {
		t.Errorf("normal: %v", got)
	}
	// Bosses use the boss rates, and a rate of 0 removes the item.
	want = []Drop{{Item: 909, Rate: 2750}, {Item: 909, Rate: 5000}, {Item: 4001, Rate: 2}, {Item: 501, Rate: 500}, {Item: 501, Rate: 2000}}
	if got := tb.AdjustDrops(BossMini, drops); !slices.Equal(got, want) {
		t.Errorf("boss: %v", got)
	}
	if got := tb.AdjustDrops(BossMVP, drops[2:3]); len(got) != 0 {
		t.Errorf("mvp: %v", got)
	}

	// drop_rateincrease adds 1 below 50%.
	cfg = DefaultConfig
	cfg.DropRateIncrease = true
	if got := New(&Data{}, testItems, cfg).AdjustDrops(BossNone, drops[:2]); got[0].Rate != 2751 || got[1].Rate != 5000 {
		t.Errorf("increase: %v", got)
	}
	// Logarithmic drops: 2750 * (5 - log10(2750)) ^ (ln 2 / ln 5) + 0.5.
	cfg = DefaultConfig
	cfg.LogarithmicDrops = true
	cfg.Common.Rate = 200
	if got := New(&Data{}, testItems, cfg).AdjustDrops(BossNone, drops[:1]); got[0].Rate != 3331 {
		t.Errorf("logarithmic: %v", got)
	}
}

func TestDropRate(t *testing.T) {
	tb := New(&Data{}, testItems, DefaultConfig)
	player := &Killer{Player: true}
	for _, c := range []struct {
		name                 string
		base, modifier, want int
		killer               *Killer
	}{
		{"plain", 2750, 100, 2750, player},
		{"no killer", 2750, 100, 2750, nil},
		{"renewal penalty", 2750, 50, 1375, player},
		{"at least 0.01%", 1, 50, 1, player},
		{"at most 100%", 10000, 140, 10000, player},
		// The cap does not lower a rate that starts above it.
		{"above the cap", 9500, 100, 9500, player},
	} {
		if got := tb.DropRate(c.base, c.modifier, c.killer); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}

	// LUK can raise a rate past the cap, which then applies.
	cfg := DefaultConfig
	cfg.DropsByLuk = 100
	tb = New(&Data{}, testItems, cfg)
	if got := tb.DropRate(8990, 100, &Killer{Player: true, Luk: 100}); got != 9000 {
		t.Errorf("cap: %d", got)
	}
	if got := tb.DropRate(8990, 100, &Killer{Luk: 100}); got != 9090 {
		t.Errorf("monster killer: %d", got)
	}
	cfg = DefaultConfig
	cfg.DropRate0Item = true
	if got := New(&Data{}, testItems, cfg).DropRate(1, 50, player); got != 0 {
		t.Errorf("rate 0: %d", got)
	}
}

func TestRollDrops(t *testing.T) {
	tb := New(&Data{}, testItems, DefaultConfig)
	m := &Mob{Drops: []Drop{{Item: 909, Rate: 5000}, {Item: 9001, Rate: 100}, {Item: 7777, Rate: 10000}, {Item: 501, Rate: 10000}}}
	// One draw per known item, in order: 4999 < 5000 drops, 100 >= 100
	// does not.
	r := &scripted{vals: []int{4999, 100, 9999}}
	got := tb.RollDrops(m, 100, &Killer{Player: true}, true, r)
	if want := []Dropped{{Item: 909}, {Item: 501}}; !slices.Equal(got, want) {
		t.Errorf("%v", got)
	}
	if !slices.Equal(r.drawn, []int{10000, 10000, 10000}) {
		t.Errorf("draws %v", r.drawn)
	}
	// Pet eggs go to the first looter when there is one.
	r = &scripted{vals: []int{9999, 0, 9999}}
	if got := tb.RollDrops(m, 100, nil, true, r); !slices.Equal(got, []Dropped{{Item: 9001, Egg: true}, {Item: 501}}) {
		t.Errorf("egg: %v", got)
	}
	r = &scripted{vals: []int{9999, 0, 9999}}
	if got := tb.RollDrops(m, 100, nil, false, r); !slices.Equal(got, []Dropped{{Item: 9001}, {Item: 501}}) {
		t.Errorf("egg on the ground: %v", got)
	}
}

func TestRollDropsHook(t *testing.T) {
	tb := New(&Data{}, map[uint32]string{501: "Healing"}, DefaultConfig)
	m := &Mob{Drops: []Drop{{Item: 501, Rate: 1}}}
	count := func(h *Hooks) int {
		r := rand.New(rand.NewPCG(1, 2))
		n := 0
		for range 1000 {
			n += len(tb.RollDropsWith(m, 100, nil, false, r, h))
		}
		return n
	}
	if n := count(nil); n > 10 {
		t.Fatalf("without a hook, %d of 1000 kills dropped at 0.01%%", n)
	}
	if n := count(&Hooks{DropRate: func(item uint32, rate int) int { return 20000 }}); n != 1000 {
		t.Fatalf("a hook rate above 10,000 dropped %d of 1000, want all", n)
	}
	if n := count(&Hooks{DropRate: func(item uint32, rate int) int { return -5 }}); n != 0 {
		t.Fatalf("a hook rate below 0 dropped %d", n)
	}
}
