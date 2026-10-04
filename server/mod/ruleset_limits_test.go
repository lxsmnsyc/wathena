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
		if r.Limits[k].Base != v || !r.Limits[k].HasBase || r.Limits[k].HasMax {
			t.Errorf("limit %s = %+v, want %d", k, r.Limits[k], v)
		}
	}
	if r.Rules["party_share_level"] != 30 || r.Rules["friend_auto_add"] != 0 || r.Rules["party_even_share_bonus"] != 10 {
		t.Errorf("rules %v", r.Rules)
	}
	l, err := rules.LimitsOf(r.Limits)
	wantLimits := rules.DefaultLimits()
	wantLimits.PartyMembers, wantLimits.Friends, wantLimits.BlockList = 64, 80, 5
	// A base above the default max raises the max with it.
	wantLimits.Max[0], wantLimits.Max[1] = 64, 80
	if err != nil || l != wantLimits {
		t.Errorf("limits %+v %v", l, err)
	}
	p, err := rules.PartyOf(r.Rules)
	if err != nil || p.ShareLevel != 30 || p.FriendAutoAdd || p.EvenShareBonus != 10 || !p.LeaderSameMap || p.ItemShareType != 0 {
		t.Errorf("party rules %+v %v", p, err)
	}

	for _, c := range []struct{ yaml, msg string }{
		{"limits:\n      party_members: 257", "party_members is 2 to 256, not 257"},
		{"limits:\n      friends: 0", "friends is 1 to 1000, not 0"},
		{"limits:\n      block_list: 1001", "block_list is 1 to 1000, not 1001"},
		{"limits:\n      inventory_slots: { base: 100, max: 1001 }", "inventory_slots: the max is at most 1000, the engine's ceiling, not 1001"},
		{"limits:\n      storage_slots: { size: 5 }", "a limit has base and max, not size"},
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
	if l, _ := rules.LimitsOf(s.Ruleset.Limits); l != rules.DefaultLimits() || l.PartyMembers != 12 || l.Friends != 40 || l.BlockList != 20 ||
		l.InventorySlots != 100 || l.MaxOf("inventory_slots") != 200 || l.StorageSlots != 600 || l.CartSlots != 100 ||
		l.GuildStorage != 600 || l.MaxOf("guild_members") != 76 {
		t.Errorf("core limits %+v", l)
	}
}

// TestRulesetLimitRanges checks the base and max of limits (review
// decision 29): a range merges down the chain part by part, a base above
// its max fails the load, and a max above the engine's ceiling fails.
func TestRulesetLimitRanges(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "wide", map[string]string{"mod.yaml": manifest("example.wide",
		"provides:\n  ruleset:\n    extends: core.ruleset.renewal\n    limits:\n      inventory_slots: { base: 150, max: 400 }\n      storage_slots: 1200")})
	writeMod(t, dir, "wider", map[string]string{"mod.yaml": manifest("example.wider",
		"provides:\n  ruleset:\n    extends: example.wide\n    limits:\n      inventory_slots: { max: 600 }\n      cart_slots: { base: 50, max: 300 }")})
	writeMod(t, dir, "bad", map[string]string{"mod.yaml": manifest("example.badrange",
		"provides:\n  ruleset:\n    extends: example.wide\n    limits:\n      inventory_slots: { max: 120 }")})
	s, err := resolve(t, dir, Config{Ruleset: "example.wider"})
	if err != nil {
		t.Fatal(err)
	}
	l, err := rules.LimitsOf(s.Ruleset.Limits)
	if err != nil {
		t.Fatal(err)
	}
	if l.InventorySlots != 150 || l.MaxOf("inventory_slots") != 600 || l.StorageSlots != 1200 || l.MaxOf("storage_slots") != 1200 ||
		l.CartSlots != 50 || l.MaxOf("cart_slots") != 300 {
		t.Errorf("limits %+v", l)
	}
	s, err = resolve(t, dir, Config{Ruleset: "example.badrange"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rules.LimitsOf(s.Ruleset.Limits); err == nil || !strings.Contains(err.Error(), "inventory_slots: the base, 150, is above the max, 120") {
		t.Errorf("a base above the max: %v", err)
	}
	for _, b := range rules.LimitBounds {
		if b.Default > b.DefaultMax || b.DefaultMax > b.Max || b.Min > b.Default {
			t.Errorf("%s: the default range %d to %d does not fit %d to %d", b.Name, b.Default, b.DefaultMax, b.Min, b.Max)
		}
	}
	if len(rules.LimitBounds) != rules.NumLimits {
		t.Errorf("NumLimits is %d, and there are %d limits", rules.NumLimits, len(rules.LimitBounds))
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
	"inventory_slots":             {"server/zone:TestInventorySlotsLimit"},
	"storage_slots":               {"server/zone:TestStorageSlotsLimit"},
	"cart_slots":                  {"server/zone:TestCartSlotsLimit"},
	"guild_emperium_check":        {"server/zone:TestGuildAcrossZones", "server/zone:TestGuildEmperiumCheckOff"},
	"guild_exp_limit":             {"server/social:TestGuildPositionsAndPermissions"},
	"emergency_call":              {"server/zone:TestGuildEmergencyCall"},
	"guild_skill_relog_delay":     {"server/zone:TestGuildSkillRelogDelay"},
	"guild_leaderchange_delay":    {"server/social:TestGuildMasterChangeDelay"},
	"guild_alliance_onlygm":       {"server/social:TestGuildAllianceOnlyMaster"},
	"max_cart_weight":             {"server/zone:TestCartWeight"},
	"basic_skill_check":           {"server/zone:TestTradeBasicSkill"},
	"mail_items":                  {"server/zone:TestMailSendRules"},
	"mailbox":                     {"server/zone:TestMailboxLimit"},
	"vending_tax":                 {"server/zone:TestVendingTax"},
	"vending_tax_min":             {"server/zone:TestVendingTax"},
	"vending_max_value":           {"server/zone:TestVendingOpenRules"},
	"vending_over_max":            {"server/zone:TestVendingOpenRules"},
	"min_npc_vendchat_distance":   {"server/zone:TestVendingOpenRules"},
	"autotrade_restore":           {"server/cmd/waserver:TestAutotradeRestoreRule"},
	"searchstore_querydelay":      {"server/zone:TestStoreSearchRules"},
	"searchstore_maxresults":      {"server/zone:TestStoreSearchRules"},
	"mail_zeny_fee":               {"server/zone:TestMailSendRules"},
	"mail_attachment_price":       {"server/zone:TestMailSendRules"},
	"mail_attachment_weight":      {"server/zone:TestMailSendRules"},
	"mail_daily_count":            {"server/zone:TestMailSendRules"},
	"mail_delay":                  {"server/zone:TestMailSendRules"},
	"mail_return_days":            {"server/mail:TestExpiry"},
	"mail_delete_days":            {"server/mail:TestExpiry"},
	"mail_return_empty":           {"server/mail:TestExpiry"},
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
