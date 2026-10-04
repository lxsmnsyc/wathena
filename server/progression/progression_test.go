package progression

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
)

// load reads a mode's progression.json and item types.
func load(t testing.TB, mode reference.Mode) (*Data, map[uint32]string) {
	t.Helper()
	var d Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/progression.json", &d); err != nil {
		t.Fatal(err)
	}
	var tables struct {
		Items []struct {
			ID   uint32 `json:"id"`
			Type string `json:"type"`
		} `json:"items"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/client_tables.json", &tables); err != nil {
		t.Fatal(err)
	}
	items := map[uint32]string{}
	for _, it := range tables.Items {
		items[it.ID] = it.Type
	}
	return &d, items
}

func tables(t testing.TB, mode reference.Mode) *Tables {
	t.Helper()
	d, items := load(t, mode)
	return New(d, items, DefaultConfig)
}

type mobStatus struct {
	ID      uint32 `json:"id"`
	Level   int    `json:"level"`
	BaseExp int64  `json:"base_exp"`
	JobExp  int64  `json:"job_exp"`
	Status  struct {
		MaxHP int    `json:"max_hp"`
		Mode  uint32 `json:"mode"`
		Class int    `json:"class"`
	} `json:"status"`
}

func loadMobs(t testing.TB, mode reference.Mode) map[uint32]mobStatus {
	t.Helper()
	var ref struct {
		Mobs []mobStatus `json:"mobs"`
	}
	if err := reference.Load(mode, "mob_status", &ref); err != nil {
		t.Fatal(err)
	}
	out := map[uint32]mobStatus{}
	for _, m := range ref.Mobs {
		out[m.ID] = m
	}
	return out
}

func TestPenaltyMatchesReference(t *testing.T) {
	tb := tables(t, reference.Renewal)
	mobs := loadMobs(t, reference.Renewal)
	var ref struct {
		Cases []struct {
			PlayerLevel int    `json:"player_level"`
			Mob         uint32 `json:"mob"`
			MobLevel    int    `json:"mob_level"`
			Exp         int    `json:"exp"`
			Drop        int    `json:"drop"`
			MVPExp      int    `json:"mvp_exp"`
			MVPDrop     int    `json:"mvp_drop"`
		} `json:"cases"`
	}
	if err := reference.Load(reference.Renewal, "level_penalty", &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Cases) != 642 {
		t.Fatalf("%d cases", len(ref.Cases))
	}
	passed := 0
	for _, c := range ref.Cases {
		m := &Mob{ID: c.Mob, Level: c.MobLevel, Mode: mobs[c.Mob].Status.Mode}
		got := [4]int{
			tb.Penalty(PenaltyExp, c.PlayerLevel, m),
			tb.Penalty(PenaltyDrop, c.PlayerLevel, m),
			tb.Penalty(PenaltyMVPExp, c.PlayerLevel, m),
			tb.Penalty(PenaltyMVPDrop, c.PlayerLevel, m),
		}
		if want := [4]int{c.Exp, c.Drop, c.MVPExp, c.MVPDrop}; got != want {
			t.Errorf("player %d, mob %d level %d: got %v, want %v", c.PlayerLevel, c.Mob, c.MobLevel, got, want)
			continue
		}
		passed++
	}
	t.Logf("%d/%d cases match", passed, len(ref.Cases))
}

func TestPenaltyPreRenewal(t *testing.T) {
	tb := tables(t, reference.PreRenewal)
	for _, typ := range []string{PenaltyExp, PenaltyDrop} {
		if got := tb.Penalty(typ, 1, &Mob{Level: 99}); got != 100 {
			t.Errorf("%s: %d", typ, got)
		}
	}
}

func TestPenaltyGaps(t *testing.T) {
	testrace.SkipUnderRace(t, "it compares recorded reference cases on one goroutine")
	tb := tables(t, reference.Renewal)
	// Differences from db/re/level_penalty.yml and the gaps between them.
	for _, c := range []struct{ diff, exp, drop int }{
		{0, 100, 100}, {2, 100, 100}, {3, 105, 100}, {4, 110, 90}, {5, 115, 90},
		{16, 40, 50}, {100, 40, 50}, {-3, 100, 100}, {-4, 100, 90}, {-5, 100, 90}, {-6, 95, 90}, {-7, 95, 80},
		{-30, 35, 50}, {-31, 10, 50}, {-274, 10, 50}, {400, 40, 50},
	} {
		m := &Mob{Level: 100 + c.diff}
		if got := tb.Penalty(PenaltyExp, 100, m); got != c.exp {
			t.Errorf("exp at %d: %d, want %d", c.diff, got, c.exp)
		}
		if got := tb.Penalty(PenaltyDrop, 100, m); got != c.drop {
			t.Errorf("drop at %d: %d, want %d", c.diff, got, c.drop)
		}
	}
	// Fixed item drop monsters have no drop penalty.
	if got := tb.Penalty(PenaltyDrop, 1, &Mob{Level: 99, Mode: ModeFixedItemDrop}); got != 100 {
		t.Errorf("fixed drop: %d", got)
	}
}

func TestBossType(t *testing.T) {
	for _, c := range []struct {
		m    Mob
		want BossType
	}{
		{Mob{}, BossNone}, {Mob{Class: ClassBoss}, BossMini}, {Mob{Mode: ModeMVP, Class: ClassBoss}, BossMVP},
	} {
		if got := c.m.Boss(); got != c.want {
			t.Errorf("%+v: %d", c.m, got)
		}
	}
}
