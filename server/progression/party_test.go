package progression

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// partyShare is the exp part of party_share.json (rAthena fork commit
// 50df331ca): kills with damage logs of players with and without
// parties, and every pc_gainexp call they made.
type partyShare struct {
	ExpBonusAttacker    int `json:"exp_bonus_attacker"`
	ExpBonusMaxAttacker int `json:"exp_bonus_max_attacker"`
	ExpCalcType         int `json:"exp_calc_type"`
	ExpCases            []struct {
		Name           string         `json:"name"`
		Monster        uint32         `json:"monster"`
		MonsterLevel   int            `json:"monster_level"`
		MonsterBaseExp int64          `json:"monster_base_exp"`
		MonsterJobExp  int64          `json:"monster_job_exp"`
		PartyExp       map[string]int `json:"party_exp"`
		EvenShareBonus int            `json:"even_share_bonus"`
		Players        []struct {
			BaseLevel int    `json:"base_level"`
			Party     int    `json:"party"`
			Map       string `json:"map"`
			Dead      bool   `json:"dead"`
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
}

// TestPartyExpShareMatchesReference replays every recorded kill through
// KillWith and ShareExp, in both modes. A party member who gets 0 and 0
// is left out of both sides: rAthena calls pc_gainexp with them, which
// changes nothing.
func TestPartyExpShareMatchesReference(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		t.Run(string(mode), func(t *testing.T) { checkPartyExp(t, mode, tables(t, mode)) })
	}
}

// TestPartyExpShareFlag runs each recording on the other base with
// level_penalty_exp set to the recording's mode (RENEWAL_EXP): the level
// penalty inside party_exp_share follows the flag, not the base.
func TestPartyExpShareFlag(t *testing.T) {
	re, _ := load(t, reference.Renewal)
	pre, err := tables(t, reference.PreRenewal).WithLevelPenalty(true, true, re)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("renewal exp on a pre-renewal base", func(t *testing.T) { checkPartyExp(t, reference.Renewal, pre) })
	off, err := tables(t, reference.Renewal).WithLevelPenalty(false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("pre-renewal exp on a renewal base", func(t *testing.T) { checkPartyExp(t, reference.PreRenewal, off) })
}

// checkPartyExp replays a mode's exp cases on tables.
func checkPartyExp(t *testing.T, mode reference.Mode, tb *Tables) {
	t.Helper()
	mobs := loadMobs(t, mode)
	var ref partyShare
	if err := reference.Load(mode, "party_share", &ref); err != nil {
		t.Fatal(err)
	}
	if ref.ExpCalcType != tb.ExpCalcType || ref.ExpBonusAttacker != tb.ExpBonusAttacker || ref.ExpBonusMaxAttacker != tb.ExpBonusMaxAttacker {
		t.Fatalf("the recording's exp settings differ from the tables'")
	}
	passed := 0
	for _, c := range ref.ExpCases {
		m := &Mob{ID: c.Monster, Level: c.MonsterLevel, BaseExp: c.MonsterBaseExp, JobExp: c.MonsterJobExp,
			MaxHP: mobs[c.Monster].Status.MaxHP, Mode: mobs[c.Monster].Status.Mode}
		var log DamageLog
		for _, d := range c.Damage {
			log.Add(uint32(d.Player+1), d.Damage, 0) // #nosec G115 -- small indexes.
		}
		who := func(id uint32) Attacker {
			p := c.Players[id-1]
			a := Attacker{Online: p.Map == "same", Dead: p.Dead, BaseLevel: p.BaseLevel}
			if p.Party != 0 {
				a.Party, a.EvenShare = int64(p.Party), c.PartyExp[strconv.Itoa(p.Party)] == 1
			}
			return a
		}
		killer := uint32(0)
		if c.Killer >= 0 {
			killer = uint32(c.Killer + 1) // #nosec G115 -- small indexes.
		}
		k := tb.KillWith(m, &log, killer, who, nil)
		got := slices.Clone(k.Rewards)
		for _, pot := range k.Pots {
			var members []ShareMember
			for i, p := range c.Players {
				if int64(p.Party) == pot.Party && p.Map == "same" && !p.Dead {
					members = append(members, ShareMember{CharID: uint32(i + 1), BaseLevel: p.BaseLevel}) // #nosec G115 -- small indexes.
				}
			}
			got = append(got, tb.ShareExp(m, pot, members, c.EvenShareBonus, nil)...)
		}
		var gotS, wantS []string
		for _, r := range got {
			gotS = append(gotS, fmt.Sprintf("%d:%d/%d", r.CharID-1, r.BaseExp, r.JobExp))
		}
		for _, g := range c.Gained {
			if g.Base != 0 || g.Job != 0 {
				wantS = append(wantS, fmt.Sprintf("%d:%d/%d", g.Player, g.Base, g.Job))
			}
		}
		if !slices.Equal(gotS, wantS) {
			t.Errorf("%s (mob %d): got %v, want %v", c.Name, c.Monster, gotS, wantS)
			continue
		}
		passed++
	}
	t.Logf("%s: %d/%d exp cases match", mode, passed, len(ref.ExpCases))
	if passed != len(ref.ExpCases) || passed == 0 {
		t.Fail()
	}
}
