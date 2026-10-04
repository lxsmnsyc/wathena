package progression

import (
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/social"
)

// guildKills is the kill part of guild.json (rAthena fork commit
// f30328d04): kills by players in guilds whose positions take an exp
// tax, with and without parties.
type guildKills struct {
	KillCases []struct {
		Name           string         `json:"name"`
		Monster        uint32         `json:"monster"`
		MonsterLevel   int            `json:"monster_level"`
		MonsterBaseExp int64          `json:"monster_base_exp"`
		MonsterJobExp  int64          `json:"monster_job_exp"`
		PartyExp       map[string]int `json:"party_exp"`
		Players        []struct {
			BaseLevel int    `json:"base_level"`
			Party     int    `json:"party"`
			Map       string `json:"map"`
			Dead      bool   `json:"dead"`
			Position  int    `json:"guild_position"`
			TaxRate   int    `json:"tax_rate"`
		} `json:"players"`
		Damage []struct {
			Player int   `json:"player"`
			Damage int64 `json:"damage"`
		} `json:"damage"`
		Killer int `json:"killer"`
		Taxed  []struct {
			Player int   `json:"player"`
			Exp    int64 `json:"exp"`
			Tax    int64 `json:"tax"`
		} `json:"taxed"`
		Added []struct {
			Player int   `json:"player"`
			Base   int64 `json:"base"`
			Job    int64 `json:"job"`
		} `json:"added"`
	} `json:"kill_cases"`
}

// TestGuildTaxOnKillsMatchesReference replays each recorded kill through
// KillWith and ShareExp, then takes each member's tax as pc_gainexp's
// guild_payexp: after the party split and the level penalty. It checks
// every tax rAthena took and the exp each player kept.
func TestGuildTaxOnKillsMatchesReference(t *testing.T) {
	testrace.SkipUnderRace(t, "it compares recorded reference cases on one goroutine")
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		t.Run(string(mode), func(t *testing.T) {
			tb := tables(t, mode)
			mobs := loadMobs(t, mode)
			var ref guildKills
			if err := reference.Load(mode, "guild", &ref); err != nil {
				t.Fatal(err)
			}
			passed := 0
			for _, c := range ref.KillCases {
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
					got = append(got, tb.ShareExp(m, pot, members, 0, nil)...)
				}
				var gotTax, wantTax, gotAdded, wantAdded []string
				for _, r := range got {
					p := c.Players[r.CharID-1]
					base := r.BaseExp
					if p.Position >= 0 && p.TaxRate >= 1 && base > 0 {
						tax := social.TaxOf(base, p.TaxRate)
						gotTax = append(gotTax, fmt.Sprintf("%d:%d/%d", r.CharID-1, base, tax))
						base -= tax
					}
					gotAdded = append(gotAdded, fmt.Sprintf("%d:%d/%d", r.CharID-1, base, r.JobExp))
				}
				for _, x := range c.Taxed {
					wantTax = append(wantTax, fmt.Sprintf("%d:%d/%d", x.Player, x.Exp, x.Tax))
				}
				for _, a := range c.Added {
					wantAdded = append(wantAdded, fmt.Sprintf("%d:%d/%d", a.Player, a.Base, a.Job))
				}
				if !slices.Equal(gotTax, wantTax) || !slices.Equal(gotAdded, wantAdded) {
					t.Errorf("%s (mob %d): tax %v, want %v; added %v, want %v", c.Name, c.Monster, gotTax, wantTax, gotAdded, wantAdded)
					continue
				}
				passed++
			}
			t.Logf("%s: %d/%d kill cases match", mode, passed, len(ref.KillCases))
		})
	}
}
