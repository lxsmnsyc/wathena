package social

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// guildExtract is extracts/<mode>/guilds.json: exp_guild.yml,
// guild_skill_tree.yml, and the guild values rAthena compiles in.
type guildExtract struct {
	Exp []struct {
		Level   int   `json:"level"`
		NextExp int64 `json:"next_exp"`
	} `json:"exp"`
	Skills []struct {
		ID       int `json:"id"`
		MaxLevel int `json:"max_level"`
		Required []struct {
			ID    int `json:"id"`
			Level int `json:"level"`
		} `json:"required"`
	} `json:"skills"`
	Constants      map[string]int64 `json:"constants"`
	GuildMaxMember struct {
		Base              int `json:"base"`
		PerExtensionLevel int `json:"per_extension_level"`
	} `json:"guild_max_member"`
	Storage struct {
		SizeByLevel []int `json:"size_by_level"`
	} `json:"storage"`
	BattleConfig map[string]struct {
		Value int `json:"value"`
	} `json:"battle_config"`
}

// TestCoreGuildTablesMatchExtracts checks the guild tables and limits
// against rAthena's files of each mode.
func TestCoreGuildTablesMatchExtracts(t *testing.T) {
	for _, mode := range reference.Modes {
		var x guildExtract
		if err := reference.LoadFile("extracts/"+string(mode)+"/guilds.json", &x); err != nil {
			t.Fatal(err)
		}
		tb := CoreGuildTables(mode == reference.Renewal)
		if len(tb.NextExp) != len(x.Exp) {
			t.Errorf("%s: %d levels, want %d", mode, len(tb.NextExp), len(x.Exp))
		}
		for _, e := range x.Exp {
			if got := tb.Next(e.Level); got != e.NextExp {
				t.Errorf("%s: level %d needs %d, want %d", mode, e.Level, got, e.NextExp)
			}
		}
		if len(tb.Skills) != len(x.Skills) {
			t.Errorf("%s: %d skills, want %d", mode, len(tb.Skills), len(x.Skills))
		}
		for _, s := range x.Skills {
			d := tb.Skill(s.ID)
			if d == nil || d.Max != s.MaxLevel || len(d.Needs) != len(s.Required) {
				t.Errorf("%s: skill %d: %+v, want %+v", mode, s.ID, d, s)
				continue
			}
			// rAthena keeps the requirements in an unordered map.
			for _, r := range s.Required {
				if !slices.Contains(d.Needs, GuildSkillNeed{r.ID, r.Level}) {
					t.Errorf("%s: skill %d does not need %+v", mode, s.ID, r)
				}
			}
		}
		if x.Constants["MAX_GUILDLEVEL"] != MaxGuildLevel || x.Constants["MAX_GUILD_EXP"] != MaxGuildExp {
			t.Errorf("%s: constants %v", mode, x.Constants)
		}
		l, g := rules.DefaultLimits(), rules.DefaultGuild()
		if l.GuildMembers != x.GuildMaxMember.Base || extensionMembers != x.GuildMaxMember.PerExtensionLevel ||
			int64(l.GuildPositions) != x.Constants["MAX_GUILDPOSITION"] || l.GuildAllies != x.BattleConfig["max_guild_alliance"].Value ||
			l.GuildEnemies != x.BattleConfig["max_guild_alliance"].Value {
			t.Errorf("%s: limits %+v", mode, l)
		}
		if g.ExpLimit != x.BattleConfig["guild_exp_limit"].Value || g.EmergencyCall != x.BattleConfig["emergency_call"].Value ||
			g.SkillRelogDelay != x.BattleConfig["guild_skill_relog_delay"].Value || g.LeaderChangeDelay != x.BattleConfig["guild_leaderchange_delay"].Value ||
			g.EmperiumCheck != (x.BattleConfig["guild_emperium_check"].Value == 1) || g.AllianceOnlyMaster != (x.BattleConfig["guild_alliance_onlygm"].Value == 1) {
			t.Errorf("%s: rules %+v", mode, g)
		}
		// Renewal's storage sizes by Guild Storage level; the decided
		// guild_storage limit of 500 caps level 5's 600.
		for i, want := range x.Storage.SizeByLevel {
			if got := 100 + 100*(i+1); got != want {
				t.Errorf("%s: storage level %d: %d, want %d", mode, i+1, got, want)
			}
		}
	}
}

// guildTaxRef is the tax part of guild.json.
type guildTaxRef struct {
	TaxCases []struct {
		Rate     int   `json:"rate"`
		Base     int64 `json:"base"`
		Job      int64 `json:"job"`
		ExpFlag  int   `json:"exp_flag"`
		ExpBoost int   `json:"expboost"`
		TaxTaken int64 `json:"tax_taken"`
		Added    *struct {
			Base, Job int64
		} `json:"added"`
	} `json:"tax_cases"`
}

// TestGuildTaxMatchesReference checks TaxOf against every recorded
// pc_gainexp call: exp_flag 0 and 1 are taxed, 2 is not, and the tax
// comes before pc_calcexp's bonuses. A case with a Battle Manual checks
// the tax only, since the zone's exp bonuses are checked elsewhere.
func TestGuildTaxMatchesReference(t *testing.T) {
	for _, mode := range reference.Modes {
		var ref guildTaxRef
		if err := reference.Load(mode, "guild", &ref); err != nil {
			t.Fatal(err)
		}
		passed := 0
		for _, c := range ref.TaxCases {
			tax := TaxOf(c.Base, c.Rate)
			if c.ExpFlag&2 != 0 {
				tax = 0
			}
			ok := tax == c.TaxTaken
			if c.ExpBoost == 0 && c.Added != nil {
				ok = ok && c.Added.Base == c.Base-tax && c.Added.Job == c.Job
			}
			if !ok {
				t.Errorf("%s: rate %d base %d flag %d: tax %d, want %d (added %+v)", mode, c.Rate, c.Base, c.ExpFlag, tax, c.TaxTaken, c.Added)
				continue
			}
			passed++
		}
		t.Logf("%s: %d/%d tax cases match", mode, passed, len(ref.TaxCases))
	}
}
