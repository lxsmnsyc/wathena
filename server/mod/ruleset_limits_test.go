package mod

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/rules"
)

// TestRulesetLimitsAndRules checks a ruleset mod's limits and rules
// blocks (review decision 27): they merge down the chain, a value out of
// bounds or an unknown name fails, and unset ones are rAthena's.
func TestRulesetLimitsAndRules(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "big", map[string]string{"mod.yaml": manifest("example.big",
		"provides:\n  ruleset:\n    extends: core.ruleset.renewal\n    limits:\n      party_members: 20\n      friends: 80\n    rules:\n      party_share_level: 30\n      friend_auto_add: no")})
	writeMod(t, dir, "bigger", map[string]string{"mod.yaml": manifest("example.bigger",
		"provides:\n  ruleset:\n    extends: example.big\n    limits:\n      party_members: 64\n      block_list: 5\n    rules:\n      party_even_share_bonus: 10")})
	s, err := resolve(t, dir, Config{Ruleset: "example.bigger"})
	if err != nil {
		t.Fatal(err)
	}
	r := s.Ruleset
	want := map[string]int{"party_members": 64, "friends": 80, "block_list": 5}
	for k, v := range want {
		if r.Limits[k] != v {
			t.Errorf("limit %s = %d, want %d", k, r.Limits[k], v)
		}
	}
	if r.Rules["party_share_level"] != 30 || r.Rules["friend_auto_add"] != 0 || r.Rules["party_even_share_bonus"] != 10 {
		t.Errorf("rules %v", r.Rules)
	}
	l, err := rules.LimitsOf(r.Limits)
	wantLimits := rules.DefaultLimits()
	wantLimits.PartyMembers, wantLimits.Friends, wantLimits.BlockList = 64, 80, 5
	if err != nil || l != wantLimits {
		t.Errorf("limits %+v %v", l, err)
	}
	p, err := rules.PartyOf(r.Rules)
	if err != nil || p.ShareLevel != 30 || p.FriendAutoAdd || p.EvenShareBonus != 10 || !p.LeaderSameMap || p.ItemShareType != 0 {
		t.Errorf("party rules %+v %v", p, err)
	}

	for _, c := range []struct{ yaml, msg string }{
		{"limits:\n      party_members: 65", "party_members is 2 to 64, not 65"},
		{"limits:\n      friends: 0", "friends is 1 to 200, not 0"},
		{"limits:\n      block_list: 101", "block_list is 1 to 100, not 101"},
		{"limits:\n      card_slots: 8", `"card_slots" is not a ruleset limit`},
		{"rules:\n      party_item_share_type: 4", "party_item_share_type is 0 to 3, not 4"},
		{"rules:\n      party_share_level: high", "party_share_level is a whole number"},
		{"rules:\n      change_party_leader_samemap: maybe", "change_party_leader_samemap is yes or no"},
	} {
		_, err := ParseManifest("m/mod.yaml", []byte(manifest("example.bad", "provides:\n  ruleset:\n    extends: core.ruleset.renewal\n    "+c.yaml)))
		wantError(t, err, c.msg)
	}

	// The core rulesets have rAthena's values.
	s, err = resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := rules.LimitsOf(s.Ruleset.Limits); l != rules.DefaultLimits() || l.PartyMembers != 12 || l.Friends != 40 || l.BlockList != 20 {
		t.Errorf("core limits %+v", l)
	}
}

// boundTests names, for each limit and rule, the tests that check its
// behavior, as "<package directory>:<test name>".
var boundTests = map[string][]string{
	"party_members":               {"server/social:TestInviteRespectsBlocksAndLimits"},
	"friends":                     {"server/social:TestFriends", "server/social:TestOverFullListsKeepEntries"},
	"block_list":                  {"server/zone:TestBlockListLimit"},
	"party_share_level":           {"server/social:TestEvenShareLevelRange"},
	"party_even_share_bonus":      {"server/progression:TestPartyExpShareMatchesReference"},
	"party_item_share_type":       {"server/social:TestLootShareMatchesReference"},
	"change_party_leader_samemap": {"server/social:TestSetLeaderSameMap"},
	"block_account_in_same_party": {"server/social:TestInviteRespectsBlocksAndLimits"},
	"friend_auto_add":             {"server/social:TestFriends", "server/social:TestFriendAutoAddOff"},
	"guild_members":               {"server/social:TestGuildInviteRespectsBlocksAndLimits"},
	"guild_positions":             {"server/social:TestGuildPositionsLimit"},
	"guild_allies":                {"server/social:TestGuildRelations"},
	"guild_enemies":               {"server/social:TestGuildRelations"},
	"guild_storage":               {"server/social:TestGuildStorageOpen"},
	"guild_emperium_check":        {"server/zone:TestGuildAcrossZones", "server/zone:TestGuildEmperiumCheckOff"},
	"guild_exp_limit":             {"server/social:TestGuildPositionsAndPermissions"},
	"emergency_call":              {"server/zone:TestGuildEmergencyCall"},
	"guild_skill_relog_delay":     {"server/zone:TestGuildSkillRelogDelay"},
	"guild_leaderchange_delay":    {"server/social:TestGuildMasterChangeDelay"},
	"guild_alliance_onlygm":       {"server/social:TestGuildAllianceOnlyMaster"},
}

// TestEveryLimitAndRuleTested fails for a limit or rule without a
// behavior test, and for a named test that does not exist.
func TestEveryLimitAndRuleTested(t *testing.T) {
	for _, b := range append(slices.Clone(rules.LimitBounds), rules.RuleBounds...) {
		tests := boundTests[b.Name]
		if len(tests) == 0 {
			t.Errorf("%s has no behavior test in boundTests", b.Name)
		}
		for _, ref := range tests {
			dir, name, _ := strings.Cut(ref, ":")
			files, err := filepath.Glob(filepath.Join("..", "..", dir, "*_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range files {
				src, err := os.ReadFile(f) // #nosec G304 -- test sources of this repository.
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(src), "func "+name+"(") {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: %s does not exist", b.Name, ref)
			}
		}
	}
}
