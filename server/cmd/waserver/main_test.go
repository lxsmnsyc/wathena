package main

import (
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/zone"
)

func TestLoadSpawnsWithRewards(t *testing.T) {
	for _, mode := range reference.Modes {
		tables, data, err := loadProgression(mode)
		if err != nil {
			t.Fatal(err)
		}
		all, err := gamedata.LoadSpawns(mode, tables, data)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, s := range all["prt_fild08"] {
			if s.Mob != 1002 {
				continue
			}
			found = true
			r := s.Reward
			if r == nil || r.Level == 0 || r.BaseExp == 0 || r.JobExp == 0 || len(r.Drops) == 0 {
				t.Fatalf("%s: poring reward %+v", mode, r)
			}
			if s.Status.DamageTaken != 100 {
				t.Errorf("%s: poring DamageTaken %d", mode, s.Status.DamageTaken)
			}
		}
		if !found {
			t.Errorf("%s: no poring on prt_fild08", mode)
		}
	}
}

func TestSelectMaps(t *testing.T) {
	if got, err := selectMaps("", "map/prontera", ""); err != nil || len(got) != 1 || got[0] != "map/prontera" {
		t.Errorf("no list: %v, %v", got, err)
	}
	got, err := selectMaps("", "map/prontera", "prt_fild08, prontera,prt_in,prontera")
	if err != nil || strings.Join(got, " ") != "map/prontera map/prt_fild08 map/prt_in" {
		t.Errorf("list: %v, %v", got, err)
	}
	if _, err := selectMaps("", "map/prontera", "prt_in"); err == nil {
		t.Error("a list without the start map is accepted")
	}
}

func TestStartCharacter(t *testing.T) {
	for _, mode := range reference.Modes {
		for build := range gamedata.Builds {
			tables, c, err := startCharacter(mode, build)
			if err != nil {
				t.Fatalf("%s %s: %v", mode, build, err)
			}
			r, err := tables.Calc(c)
			if err != nil || r.Battle.MaxHP == 0 || r.Battle.Player == nil {
				t.Errorf("%s %s: %+v, %v", mode, build, r.Battle, err)
			}
		}
	}
	if _, _, err := startCharacter(reference.Renewal, "nope"); err == nil {
		t.Error("an unknown build loads")
	}
}

func TestLoadItems(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		data, err := loadItemData(mode)
		if err != nil {
			t.Fatal(err)
		}
		items := item.New(data)
		// Red Potion: 7 weight. Its script comes from core's converted
		// item data, which loadMods applies.
		d := items.Get(501)
		if d.Weight != 70 || d.AegisName != "Red_Potion" {
			t.Errorf("%s: Red Potion %+v", mode, d)
		}
		if want := map[reference.Mode]int{reference.Renewal: 70, reference.PreRenewal: 50}[mode]; items.NaturalHealWeightRate != want {
			t.Errorf("%s: natural_heal_weight_rate %d", mode, items.NaturalHealWeightRate)
		}
	}
}

func TestParseStartItems(t *testing.T) {
	got, err := parseStartItems("501:10,512:2")
	if err != nil || len(got) != 2 || got[0] != (zone.StartItem{ID: 501, Amount: 10}) || got[1] != (zone.StartItem{ID: 512, Amount: 2}) {
		t.Errorf("%v %v", got, err)
	}
	for _, bad := range []string{"501", "0:1", "501:0", "501:30001", "x:1"} {
		if _, err := parseStartItems(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:8300": true,
		"[::1]:8300":     true,
		"localhost:8300": true,
		"0.0.0.0:8300":   false,
		":8300":          false,
		"10.0.0.5:8300":  false,
		"example.com:80": false,
		"127.0.0.1":      false,
	} {
		if got := loopback(addr); got != want {
			t.Errorf("loopback(%q) = %v", addr, got)
		}
	}
}

func TestParseProxies(t *testing.T) {
	got, err := parseProxies(splitList("10.0.0.0/8, 192.0.2.7,2001:db8::/32"))
	if err != nil || len(got) != 3 || got[1].String() != "192.0.2.7/32" {
		t.Errorf("%v %v", got, err)
	}
	if _, err := parseProxies([]string{"nope"}); err == nil {
		t.Error("nope parsed")
	}
}

func TestLoadSealer(t *testing.T) {
	const key = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
	if s, err := loadSealer(secrets{}); s != nil || err != nil {
		t.Errorf("no key: %v %v", s, err)
	}
	if s, err := loadSealer(secrets{SecretKey: key}); s == nil || err != nil {
		t.Errorf("a key: %v", err)
	}
	if s, err := loadSealer(secrets{SecretKey: key, OldSecretKeys: key + ", " + key}); s == nil || err != nil {
		t.Errorf("old keys: %v", err)
	}
	_, err := loadSealer(secrets{SecretKey: "too short"})
	if err == nil || strings.Contains(err.Error(), "too short") {
		t.Errorf("a bad key: %v", err)
	}
}

// TestPreRenewalClassicWarning checks that only a classic world in
// pre-renewal mode warns at startup (review decision 55).
func TestPreRenewalClassicWarning(t *testing.T) {
	for _, c := range []struct {
		mode    string
		classic bool
		warns   bool
	}{
		{"pre-renewal", true, true},
		{"renewal", true, false},
		{"none", true, false},
		{"pre-renewal", false, false},
	} {
		got := preRenewalClassicWarning(c.mode, c.classic)
		if (got != "") != c.warns {
			t.Errorf("%s, classic %v: warning %q", c.mode, c.classic, got)
		}
		if c.warns && !strings.Contains(got, "experimental") {
			t.Errorf("warning %q does not say experimental", got)
		}
	}
}
