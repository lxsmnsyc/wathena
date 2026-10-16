//go:build procedural

package layout_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

var update = flag.Bool("update", false, "rewrite testdata/golden-layout.json")

// goldenCase is one golden layout: a secret, a config, and whether a
// home map joins it.
type goldenCase struct {
	Secret   string `json:"secret"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Towns    int    `json:"towns"`
	Dungeons int    `json:"dungeons"`
	Home     bool   `json:"home"`
	// The results.
	Checksum string   `json:"checksum"`
	Attempt  int      `json:"attempt"`
	Maps     int      `json:"maps"`
	Links    int      `json:"links"`
	Names    []string `json:"names"`
	Bands    []string `json:"bands"`
}

var goldenCases = []goldenCase{
	{Secret: "golden-layout-1", Width: 6, Height: 4, Towns: 5, Dungeons: 6},
	{Secret: "golden-layout-2", Width: 6, Height: 4, Towns: 5, Dungeons: 6},
	{Secret: "golden-layout-3", Width: 6, Height: 4, Towns: 5, Dungeons: 6, Home: true},
	{Secret: "golden-layout-4", Width: 2, Height: 2, Towns: 2, Dungeons: 1},
	{Secret: "golden-layout-5", Width: 10, Height: 8, Towns: 12, Dungeons: 14},
	{Secret: "golden-layout-6", Width: 1, Height: 3, Towns: 1, Dungeons: 2, Home: true},
}

// testHome is a home map's record for layouts with one.
var testHome = &layout.HomeMap{Folder: "village", Record: "example.home:village", Name: "Willow Village"}

func config(c goldenCase) layout.Config {
	cfg := layout.DefaultConfig()
	cfg.Width, cfg.Height, cfg.Towns, cfg.Dungeons, cfg.FieldSize = c.Width, c.Height, c.Towns, c.Dungeons, 64
	return cfg
}

func build(t testing.TB, c goldenCase) *layout.Layout {
	t.Helper()
	in := coreInputs(t, c.Secret)
	if c.Home {
		in.Home = testHome
	}
	l, err := layout.Build(config(c), in)
	if err != nil {
		t.Fatalf("%s: %v", c.Secret, err)
	}
	return l
}

func outcome(c goldenCase, l *layout.Layout) goldenCase {
	sum := l.Checksum()
	c.Checksum, c.Attempt, c.Maps, c.Links = hex.EncodeToString(sum[:]), l.Attempt, len(l.Maps), len(l.Links)
	c.Names, c.Bands = nil, nil
	for _, m := range l.Maps {
		if m.Kind == gen.KindTown || m.Kind == layout.KindHome {
			c.Names = append(c.Names, m.Name)
		}
		if m.Kind == gen.KindTown {
			c.Bands = append(c.Bands, fmt.Sprintf("%s %d-%d", gen.TownID(m.Town), m.Levels[0], m.Levels[1]))
		}
	}
	return c
}

// TestGoldenLayout: the golden layouts' checksums, town names and bands
// match testdata/golden-layout.json on this platform. scripts/check.sh
// gen runs it on arm64 and on amd64. go test -tags procedural -run
// TestGoldenLayout ./server/layout -update rewrites the file.
func TestGoldenLayout(t *testing.T) {
	file := filepath.Join("testdata", "golden-layout.json")
	if *update {
		var out []goldenCase
		for _, c := range goldenCases {
			out = append(out, outcome(c, build(t, c)))
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var want []goldenCase
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(goldenCases) {
		t.Fatalf("%d golden layouts, want %d", len(want), len(goldenCases))
	}
	for _, w := range want {
		got := outcome(w, build(t, w))
		if got.Checksum != w.Checksum || got.Attempt != w.Attempt || got.Maps != w.Maps || got.Links != w.Links ||
			!slices.Equal(got.Names, w.Names) || !slices.Equal(got.Bands, w.Bands) {
			t.Errorf("%s: %+v, want %+v", w.Secret, got, w)
		}
	}
	t.Logf("%d golden layouts match on %s/%s", len(want), runtime.GOOS, runtime.GOARCH)
}

// TestLayoutRules: the default layout of several worlds follows review
// decision 43.
func TestLayoutRules(t *testing.T) {
	for i := range 20 {
		c := goldenCase{Secret: fmt.Sprintf("rules-%d", i), Width: 6, Height: 4, Towns: 5, Dungeons: 6, Home: i%4 == 0}
		l := build(t, c)
		checkRules(t, c, l)
	}
}

// checkRules checks a layout's shape:
//   - the grid's fields, the towns, the dungeons with their style's
//     floors, and the home map when there is one;
//   - links that join fields only on shared borders, through the portal
//     of each side, and each field portal used once;
//   - each town's gates link to distinct fields, 2 to 4 of them, or one
//     fewer for the first town when a home map takes its first gate;
//   - each dungeon's first floor opens from a field, and floors link in
//     order;
//   - every map reached from the home area;
//   - unique town names of 4 to 12 letters;
//   - bands that rise with distance, start at level 1 at home, and
//     floors that build on their entrance's band.
func checkRules(t *testing.T, c goldenCase, l *layout.Layout) {
	t.Helper()
	count := map[string]int{}
	for _, m := range l.Maps {
		count[m.Kind]++
	}
	floors := 0
	for _, m := range l.Maps {
		if m.Kind == gen.KindDungeon && m.Floor == 1 {
			floors += m.Floors
		}
	}
	home := 0
	if c.Home {
		home = 1
	}
	if count[gen.KindField] != c.Width*c.Height || count[gen.KindTown] != c.Towns || count[gen.KindDungeon] != floors || count[layout.KindHome] != home {
		t.Fatalf("%s: maps %v", c.Secret, count)
	}
	used := map[string]bool{}
	gates := map[string][]string{}
	for _, k := range l.Links {
		for _, e := range []layout.End{k.A, k.B} {
			key := e.String()
			if used[key] {
				t.Errorf("%s: the end %s links twice", c.Secret, key)
			}
			used[key] = true
		}
		ax, ay, aField := gen.ParseFieldID(k.A.Map)
		bx, by, bField := gen.ParseFieldID(k.B.Map)
		switch {
		case aField && bField:
			dx, dy := bx-ax, by-ay
			want := map[[2]int64][2]string{{1, 0}: {"east", "west"}, {0, 1}: {"north", "south"}}[[2]int64{dx, dy}]
			if want[0] == "" || k.A.Tag != "portal:"+want[0] || k.B.Tag != "portal:"+want[1] {
				t.Errorf("%s: %s to %s is not a shared border", c.Secret, k.A, k.B)
			}
		case aField && strings.HasPrefix(k.B.Map, "gen:town:"):
			gates[k.B.Map] = append(gates[k.B.Map], k.A.Map)
		case aField && k.B.Tag == layout.TagEntrance:
			if _, f, _ := gen.ParseDungeonMapID(k.B.Map); f != 1 {
				t.Errorf("%s: a field opens to %s", c.Secret, k.B)
			}
		case k.A.Tag == layout.TagExit && k.B.Tag == layout.TagEntrance:
			ia, fa, _ := gen.ParseDungeonMapID(k.A.Map)
			ib, fb, _ := gen.ParseDungeonMapID(k.B.Map)
			if ia != ib || fb != fa+1 {
				t.Errorf("%s: %s to %s", c.Secret, k.A, k.B)
			}
		case k.A.Map == layout.HomeMapID("village") && k.B.Map == "gen:town:t1" && k.B.Index == 0:
		default:
			t.Errorf("%s: an unexpected link %s to %s", c.Secret, k.A, k.B)
		}
	}
	for _, t1 := range l.Towns() {
		g := gates[t1.ID]
		lo := 2
		if c.Home && t1.Town == 1 {
			lo = 1
		}
		if len(g) < lo || len(g) > 4 || len(slices.Compact(slices.Sorted(slices.Values(g)))) != len(g) {
			t.Errorf("%s: %s links to fields %v", c.Secret, t1.ID, g)
		}
	}
	names := map[string]bool{}
	for _, m := range l.Maps {
		if m.Kind == gen.KindTown {
			if !layout.ValidTownName(m.Name) || names[strings.ToLower(m.Name)] {
				t.Errorf("%s: town name %q", c.Secret, m.Name)
			}
			names[strings.ToLower(m.Name)] = true
		}
		if m.Name == "" {
			t.Errorf("%s: %s has no name", c.Secret, m.ID)
		}
	}
	at := map[string]layout.Map{}
	for _, m := range l.Maps {
		at[m.ID] = m
	}
	if h := at[l.Home]; h.Levels[0] != 1 || h.Distance != 0 {
		t.Errorf("%s: the home area %s has %v at %d", c.Secret, h.ID, h.Levels, h.Distance)
	}
	for _, a := range l.Maps {
		for _, b := range l.Maps {
			if a.Kind == gen.KindDungeon || b.Kind == gen.KindDungeon {
				continue
			}
			if a.Distance < b.Distance && a.Levels[0] > b.Levels[0] {
				t.Errorf("%s: %s at %d has %v, above %s at %d with %v", c.Secret, a.ID, a.Distance, a.Levels, b.ID, b.Distance, b.Levels)
			}
		}
	}
	for _, k := range l.Links {
		if k.B.Tag == layout.TagEntrance && k.A.Tag != layout.TagExit {
			f, e := at[k.A.Map], at[k.B.Map]
			if e.Levels[0] != min(f.Levels[0], layout.DefaultMaxLevel-layout.DefaultBandWidth+1) {
				t.Errorf("%s: %s starts at %v, its entrance field %s at %v", c.Secret, e.ID, e.Levels, f.ID, f.Levels)
			}
		}
	}
}

// TestLayoutDeterministic: a layout is the same each time, and a
// different secret gives a different one.
func TestLayoutDeterministic(t *testing.T) {
	c := goldenCases[0]
	a, b := build(t, c), build(t, c)
	if a.Checksum() != b.Checksum() {
		t.Error("two builds differ")
	}
	c.Secret = "another"
	if build(t, c).Checksum() == a.Checksum() {
		t.Error("two secrets gave one layout")
	}
}

// TestLayoutRetries: a layout that fails tries attempt(1) and on, and
// fails with each attempt's problem after 8. Some world of a crowded
// grid needs a retry and passes.
func TestLayoutRetries(t *testing.T) {
	in := coreInputs(t, "retries")
	cfg := layout.DefaultConfig()
	cfg.Width, cfg.Height, cfg.Towns, cfg.Dungeons = 1, 1, 3, 0
	_, err := layout.Build(cfg, in)
	if !errors.Is(err, layout.ErrNoLayout) || !strings.Contains(err.Error(), fmt.Sprintf("attempt %d:", layout.Attempts-1)) ||
		strings.Contains(err.Error(), fmt.Sprintf("attempt %d:", layout.Attempts)) {
		t.Fatalf("a town too many: %v", err)
	}
	cfg.Width, cfg.Height, cfg.Towns, cfg.Dungeons = 3, 2, 3, 3
	retried := 0
	for i := range 200 {
		l, err := layout.Build(cfg, coreInputs(t, fmt.Sprintf("crowded-%d", i)))
		if err != nil {
			continue
		}
		if l.Attempt > 0 {
			retried++
		}
	}
	if retried == 0 {
		t.Error("no crowded world needed a retry")
	}
}

// TestLayoutChecks: bad configs and inputs are refused.
func TestLayoutChecks(t *testing.T) {
	in := coreInputs(t, "checks")
	for _, cfg := range []layout.Config{
		{Width: 0, Height: 4, Towns: 5, BandWidth: 10, MaxLevel: 99, FieldSize: 64},
		{Width: 6, Height: 4, Towns: 0, BandWidth: 10, MaxLevel: 99, FieldSize: 64},
		{Width: 6, Height: 4, Towns: 5, Dungeons: -1, BandWidth: 10, MaxLevel: 99, FieldSize: 64},
		{Width: 6, Height: 4, Towns: 5, BandWidth: 100, MaxLevel: 99, FieldSize: 64},
		{Width: 6, Height: 4, Towns: 5, BandWidth: 10, MaxLevel: 99, FieldSize: 16},
	} {
		if _, err := layout.Build(cfg, in); err == nil {
			t.Errorf("%+v was taken", cfg)
		}
	}
	noTowns := in
	noTowns.Towns = nil
	if _, err := layout.Build(layout.DefaultConfig(), noTowns); err == nil {
		t.Error("no templates was taken")
	}
	v2, _ := gen.Lookup(2)
	old := in
	old.Generator = v2
	if _, err := layout.Build(layout.DefaultConfig(), old); err == nil || !strings.Contains(err.Error(), "generator 3") {
		t.Errorf("generator 2: %v", err)
	}
}

// FuzzLayout: any secret and a small config give a layout that follows
// the rules, or ErrNoLayout.
func FuzzLayout(f *testing.F) {
	f.Add("seed", uint8(6), uint8(4), uint8(5), uint8(6), false)
	f.Add("x", uint8(1), uint8(1), uint8(1), uint8(0), true)
	f.Fuzz(func(t *testing.T, secret string, w, h, towns, dungeons uint8, home bool) {
		c := goldenCase{Secret: secret, Width: int(w%8) + 1, Height: int(h%8) + 1, Towns: int(towns%8) + 1, Dungeons: int(dungeons % 10), Home: home}
		in := coreInputs(t, "fuzz:"+secret)
		if c.Home {
			in.Home = testHome
		}
		l, err := layout.Build(config(c), in)
		if errors.Is(err, layout.ErrNoLayout) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		checkRules(t, c, l)
	})
}
