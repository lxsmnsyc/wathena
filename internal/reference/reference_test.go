package reference

import (
	"regexp"
	"testing"
)

var gitHash = regexp.MustCompile(`^[0-9a-f]{40}$`)

func TestFormulaHeaders(t *testing.T) {
	hashes := map[string]bool{}
	for _, mode := range Modes {
		for _, system := range Systems {
			var h Header
			if err := Load(mode, system, &h); err != nil {
				t.Fatal(err)
			}
			if h.System != system {
				t.Errorf("%s/%s: system is %q", mode, system, h.System)
			}
			if h.Mode != mode {
				t.Errorf("%s/%s: mode is %q", mode, system, h.Mode)
			}
			if h.DataPath != mode.DataPath() {
				t.Errorf("%s/%s: data_path is %q", mode, system, h.DataPath)
			}
			if !gitHash.MatchString(h.RathenaGitHash) {
				t.Errorf("%s/%s: bad rathena_git_hash %q", mode, system, h.RathenaGitHash)
			}
			hashes[h.RathenaGitHash] = true
		}
	}
	if len(hashes) != 1 {
		t.Errorf("formula files come from %d rAthena commits, want 1", len(hashes))
	}
}

// The Neutral attack against a level 1 Ghost defender is the value the
// recorder uses to check the mode.
func TestElementsMode(t *testing.T) {
	const neutral, ghost = 0, 8
	want := map[Mode]int{Renewal: 90, PreRenewal: 25}
	for _, mode := range Modes {
		var e Elements
		if err := Load(mode, "elements", &e); err != nil {
			t.Fatal(err)
		}
		if len(e.Levels) != 4 {
			t.Fatalf("%s: %d element levels, want 4", mode, len(e.Levels))
		}
		if got := e.Levels[0].Table[neutral][ghost]; got != want[mode] {
			t.Errorf("%s: Neutral against Ghost 1 is %d, want %d", mode, got, want[mode])
		}
	}
}

func TestLevelPenalty(t *testing.T) {
	var p struct {
		Cases []any `json:"cases"`
	}
	if err := Load(Renewal, "level_penalty", &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Cases) == 0 {
		t.Error("renewal level_penalty has no cases")
	}
	if err := Load(PreRenewal, "level_penalty", &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Cases) != 0 {
		t.Errorf("pre-renewal level_penalty has %d cases, want 0", len(p.Cases))
	}
}

func TestPartyShare(t *testing.T) {
	for _, mode := range Modes {
		var p struct {
			ExpCalcType         int `json:"exp_calc_type"`
			ExpBonusAttacker    int `json:"exp_bonus_attacker"`
			ExpBonusMaxAttacker int `json:"exp_bonus_max_attacker"`
			ExpCases            []struct {
				Name    string `json:"name"`
				Monster int    `json:"monster"`
				Players []struct {
					Party int    `json:"party"`
					Map   string `json:"map"`
					Dead  bool   `json:"dead"`
				} `json:"players"`
				Damage []struct {
					Player int   `json:"player"`
					Damage int64 `json:"damage"`
				} `json:"damage"`
				Killer int `json:"killer"`
				Gained []struct {
					Player int   `json:"player"`
					Base   int64 `json:"base"`
					Job    int64 `json:"job"`
				} `json:"gained"`
			} `json:"exp_cases"`
			LootCases []struct {
				Name       string `json:"name"`
				PartyItem  int    `json:"party_item"`
				ShareType  int    `json:"share_type"`
				Picker     int    `json:"picker"`
				Taken      bool   `json:"taken"`
				Receiver   int    `json:"receiver"`
				Candidates []int  `json:"candidates"`
				Drawn      []int  `json:"drawn"`
			} `json:"loot_cases"`
		}
		if err := Load(mode, "party_share", &p); err != nil {
			t.Fatal(err)
		}
		if p.ExpCalcType != 0 || p.ExpBonusAttacker != 25 || p.ExpBonusMaxAttacker != 12 {
			t.Errorf("%s: exp settings are %d, %d, %d, want rAthena's defaults 0, 25, 12", mode, p.ExpCalcType, p.ExpBonusAttacker, p.ExpBonusMaxAttacker)
		}
		if len(p.ExpCases) == 0 {
			t.Errorf("%s: party_share has no exp cases", mode)
		}
		if len(p.LootCases) == 0 {
			t.Errorf("%s: party_share has no loot cases", mode)
		}
		for _, c := range p.ExpCases {
			if len(c.Players) == 0 || len(c.Damage) == 0 || len(c.Gained) == 0 {
				t.Errorf("%s: exp case %s has no players, damage or gains", mode, c.Name)
			}
		}
		draws := 0
		for _, c := range p.LootCases {
			draws += len(c.Drawn)
		}
		if draws == 0 {
			t.Errorf("%s: no loot case has a random pick", mode)
		}
	}
}

func TestExtracts(t *testing.T) {
	for _, mode := range Modes {
		for _, name := range []string{"spawns", "client_tables", "progression", "job_stats", "items", "equipment", "refine_db", "monster_groups", "status", "skills", "skill_trees", "random_options"} {
			var h struct {
				Mode Mode `json:"mode"`
			}
			if err := LoadFile("extracts/"+string(mode)+"/"+name+".json", &h); err != nil {
				t.Fatal(err)
			}
			if h.Mode != mode {
				t.Errorf("extracts/%s/%s: mode is %q", mode, name, h.Mode)
			}
		}
	}
}

func TestInventory(t *testing.T) {
	var features struct {
		MissingSources []string `json:"missing_sources"`
		MissingData    []string `json:"missing_data"`
	}
	if err := LoadFile("inventory/features.json", &features); err != nil {
		t.Fatal(err)
	}
	if n := len(features.MissingSources) + len(features.MissingData); n != 0 {
		t.Errorf("feature inventory has %d gaps", n)
	}
	for _, name := range []string{"skills", "script_commands", "renewal_sites"} {
		var v map[string]any
		if err := LoadFile("inventory/"+name+".json", &v); err != nil {
			t.Fatal(err)
		}
	}
}
