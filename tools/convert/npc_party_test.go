package convert

import (
	"strings"
	"testing"
)

// TestPartyCommandsConvert checks the party commands and party reads
// (proposal 0002, review decision 27). wa.parties is not in the runtime
// yet, so the test checks the converted text only.
func TestPartyCommandsConvert(t *testing.T) {
	res := eventResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ party_create "Team"; end; }`, []string{`parties.create(player, "Team")`, `local parties = require "wa.parties"`}},
		{`{ party_create "  Team  ",getcharid(0); end; }`, []string{`parties.create(player, "Team")`}},
		{`{ party_create "Team",getcharid(0),1,1; end; }`, []string{
			`party_1 = parties.create(player, "Team")`, "if party_1 then",
			`party_1:set_options { item_pickup = "shared", item_share = "even" }`,
		}},
		{`{ party_create "Team",getcharid(0),0,.@x; end; }`, []string{`set_options { item_share = ((x ~= 0) and "even" or "each") }`}},
		{`{ party_addmember .@p,getcharid(0); end; }`, []string{
			"party_1 = parties.get(p)", "if party_1 and player.party == nil then", "party_1:add(player)",
		}},
		{`{ party_addmember getcharid(1),getcharid(0); end; }`, []string{"-- party_addmember to the player's own party does nothing in rAthena."}},
		{`{ party_delmember; end; }`, []string{"if player.party then", "player.party:remove(player)"}},
		{`{ party_delmember getcharid(0); end; }`, []string{"player.party:remove(player)"}},
		{`{ party_delmember getcharid(0),.@p; end; }`, []string{"player.party:remove(player)"}},
		{`{ party_changeleader getcharid(1),getcharid(0); end; }`, []string{"if player.party then", "player.party:set_leader(player)"}},
		{`{ party_changeleader .@p,getcharid(0); end; }`, []string{"if player.party and player.party.id == p then", "player.party:set_leader(player)"}},
		{`{ party_changeoption getcharid(1),0,1; end; }`, []string{"if player.party then", `player.party:set_options { exp_share = "even" }`}},
		{`{ party_changeoption .@p,1,0; end; }`, []string{"party_1 = parties.get(p)", `party_1:set_options { item_pickup = "each" }`}},
		{`{ party_changeoption .@p,2,.@f; end; }`, []string{`party_1:set_options { item_share = ((f ~= 0) and "even" or "each") }`}},
		{`{ party_destroy getcharid(1); end; }`, []string{"if player.party then", "player.party:disband()"}},
		{`{ party_destroy .@p; end; }`, []string{"party_1 = parties.get(p)", "party_1:disband()"}},
		{`{ .@p = getcharid(1); end; }`, []string{"p = (player.party and player.party.id or 0)"}},
		{`{ mes strcharinfo(1); end; }`, []string{`conversation:say((player.party and player.party.name or ""))`}},
		{`{ mes getpartyname(getcharid(1)); end; }`, []string{`conversation:say((player.party and player.party.name or "null"))`}},
		{`{ mes getpartyname(.@p); end; }`, []string{`conversation:say(R.party_field(parties.get(p), "name", "null"))`}},
		{`{ mes getpartyleader(getcharid(1)); end; }`, []string{`(player.party and player.party.leader_name or "null")`}},
		{`{ mes getpartyleader(.@p,0); end; }`, []string{`R.party_field(parties.get(p), "leader_name", "null")`}},
		{`{ if (is_party_leader()) end; end; }`, []string{"(player.party ~= nil and player.party.leader_name == player.name)"}},
		{`{ if (is_party_leader(getcharid(1))) end; end; }`, []string{"(player.party ~= nil and player.party.leader_name == player.name)"}},
		{`{ if (is_party_leader(.@p)) end; end; }`, []string{`(R.party_field(parties.get(p), "leader_name", "") == player.name)`}},
		{`{ getpartymember getcharid(1); .@n = $@partymembercount; mes $@partymembername$[1]; end; }`, []string{
			"party_member_names_2, party_member_count_1 = R.party_members(player.party)",
			"n = party_member_count_1", `conversation:say((party_member_names_2[1] or ""))`,
		}},
		{`{ getpartymember(.@p, 0); .@n = getarraysize($@partymembername$); end; }`, []string{
			"R.party_members(parties.get(p))", "n = party_member_count_1",
		}},
		{`{ .@n = getpartymember(getcharid(1)); end; }`, []string{"n = party_member_count_1"}},
		// Types 1 and 2 read party:get_member_list (review decision 58).
		{`{ getpartymember getcharid(1),1; mes "" + $@partymembercid[0]; end; }`, []string{
			`party_member_cids_2, party_member_count_1 = R.party_members(player.party, "character_id")`, "(party_member_cids_2[0] or 0)",
		}},
		{`{ getpartymember getcharid(1),2; .@n = getarraysize($@partymemberaid); end; }`, []string{
			`R.party_members(player.party, "account_id")`, "n = party_member_count_1",
		}},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		if err != nil {
			t.Errorf("%s: %v", c.script, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(lua, w) {
				t.Errorf("%s: got\n%s\nwant %q", c.script, lua, w)
			}
		}
		if strings.Contains(lua, "parties.") != strings.Contains(lua, `local parties = require "wa.parties"`) {
			t.Errorf("%s: wa.parties is required only where it is used:\n%s", c.script, lua)
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ party_create "Team",150001; end; }`, "party_create: for another character"},
		{`{ party_create "   "; end; }`, "party_create: an empty name"},
		{`{ .@r = party_create("Team"); end; }`, "party_create: its result code"},
		{`{ party_addmember .@p,150001; end; }`, "party_addmember: another character"},
		{`{ party_delmember 150001,.@p; end; }`, "party_delmember: another character"},
		{`{ party_changeleader .@p,150001; end; }`, "party_changeleader: another character"},
		{`{ party_changeoption .@p,.@o,1; end; }`, "party_changeoption: a computed option"},
		{`{ party_changeoption .@p,3,1; end; }`, "party_changeoption: an option other than 0, 1 and 2"},
		{`{ getpartymember getcharid(1),0,.@names$; end; }`, "getpartymember: into an array of the script's own"},
		{`{ mes $@partymembername$[0]; end; }`, "outside a function that calls getpartymember"},
		{`{ mes getpartyleader(getcharid(1),1); end; }`, "getpartyleader: the leader's account ID (type 1), which the party API has only while the leader is online"},
		{`{ mes getpartyleader(getcharid(1),3); end; }`, "getpartyleader: the leader's job (type 3)"},
		{`{ mes getpartyleader(getcharid(1),4); end; }`, "getpartyleader: the leader's map (type 4)"},
		{`{ .@p = getcharid(1,"Bob"); end; }`, "getcharid: of another character, other than its character and account IDs"},
		{`{ .@g = getcharid(4); end; }`, "getcharid: battlegrounds, clans and other IDs"},
		{`{ warpparty "Leader",150,150,getcharid(1); end; }`, `warpparty: to "Leader", which party:warp does not take`},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}
