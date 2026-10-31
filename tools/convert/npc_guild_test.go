package convert

import (
	"strings"
	"testing"
)

// TestGuildCommandsConvert checks the guild commands and guild reads
// (proposal 0002, review decision 28). It checks the converted text.
// server/zone's guildwarehouse_test.go runs a converted NPC.
func TestGuildCommandsConvert(t *testing.T) {
	res := eventResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ .@g = getcharid(2); end; }`, []string{"g = (player.guild and player.guild.id or 0)"}},
		{`{ mes strcharinfo(2); end; }`, []string{`conversation:say((player.guild and player.guild.name or ""))`}},
		{`{ mes getguildname(getcharid(2)); end; }`, []string{`conversation:say((player.guild and player.guild.name or "null"))`}},
		{`{ mes getguildname(.@g); end; }`, []string{`conversation:say(R.guild_field(guilds.get(g), "name", "null"))`, `local guilds = require "wa.guilds"`}},
		{`{ mes getguildmaster(getcharid(2)); end; }`, []string{`(player.guild and player.guild.master_name or "null")`}},
		{`{ mes getguildmaster(.@g); end; }`, []string{`R.guild_field(guilds.get(g), "master_name", "null")`}},
		{`{ .@l = getgdskilllv(getcharid(2),"GD_GUILD_STORAGE"); end; }`, []string{
			`local GD_GUILD_STORAGE = skills.id "GD_GUILD_STORAGE"`,
			"l = (player.guild and player.guild:get_skill_level(GD_GUILD_STORAGE) or -1)",
		}},
		{`{ .@l = getgdskilllv(.@g,10016); end; }`, []string{"l = R.guild_skill(guilds.get(g), GD_GUILD_STORAGE)"}},
		{`{ if (is_guild_leader()) end; end; }`, []string{"(player.guild ~= nil and player.guild.master_name == player.name)"}},
		{`{ if (is_guild_leader(getcharid(2))) end; end; }`, []string{"(player.guild ~= nil and player.guild.master_name == player.name)"}},
		{`{ if (is_guild_leader(.@g)) end; end; }`, []string{`(R.guild_field(guilds.get(g), "master_name", "") == player.name)`}},
		{`{ if (guild_has_permission(GUILD_PERM_STORAGE)) end; end; }`, []string{`R.guild_permission(player, "can_use_storage")`}},
		{`{ if (guild_has_permission(GUILD_PERM_INVITE|GUILD_PERM_EXPEL,getcharid(0))) end; end; }`, []string{`R.guild_permission(player, "can_invite", "can_expel")`}},
		{`{ if (guild_has_permission(GUILD_PERM_ALL)) end; end; }`, []string{`R.guild_permission(player, "can_invite", "can_expel", "can_use_storage")`}},
		{`{ getguildmember getcharid(2); .@n = $@guildmembercount; mes $@guildmembername$[1]; end; }`, []string{
			"guild_member_names_1, guild_member_count_2 = R.guild_members(player.guild)",
			"n = guild_member_count_2", `conversation:say((guild_member_names_1[1] or ""))`,
		}},
		{`{ getguildmember(.@g, 0); .@n = getarraysize($@guildmembername$); end; }`, []string{
			"R.guild_members(guilds.get(g))", "n = guild_member_count_2",
		}},
		{`{ .@n = getguildmember(getcharid(2)); end; }`, []string{"n = guild_member_count_2"}},
		{`{ guildgetexp 100; end; }`, []string{"if player.guild then", "player.guild:give_exp(100)"}},
		{`{ if (guildopenstorage() == GSTORAGE_OPEN) end; end; }`, []string{"(R.guild_storage(conversation, player) == 0)"}},
		{`{ guildopenstorage; end; }`, []string{"R.guild_storage(conversation, player)"}},
		{`{ guildopenstorage_log(); end; }`, []string{"R.guild_storage_log(conversation, player)"}},
		{`{ guildopenstorage_log getcharid(0); end; }`, []string{"R.guild_storage_log(conversation, player)"}},
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
		if strings.Contains(lua, "guilds.") != strings.Contains(lua, `local guilds = require "wa.guilds"`) {
			t.Errorf("%s: wa.guilds is required only where it is used:\n%s", c.script, lua)
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ .@x = getcharid(4); end; }`, "getcharid: battlegrounds, clans and other IDs"},
		{`{ .@l = getgdskilllv(getcharid(2),.@s); end; }`, "getgdskilllv: a computed skill"},
		{`{ if (guild_has_permission(.@p)) end; end; }`, "guild_has_permission: a computed permission"},
		{`{ if (guild_has_permission(0)) end; end; }`, "guild_has_permission: permission 0"},
		{`{ if (guild_has_permission(2)) end; end; }`, "guild_has_permission: permission 2"},
		{`{ if (guild_has_permission(GUILD_PERM_STORAGE,150001)) end; end; }`, "guild_has_permission: for another character"},
		{`{ getguildmember getcharid(2),1; end; }`, "getguildmember: type 1, character IDs, which the guild API does not give"},
		{`{ getguildmember getcharid(2),2; end; }`, "getguildmember: type 2, account IDs, which the guild API does not give"},
		{`{ getguildmember getcharid(2),0,.@names$; end; }`, "getguildmember: into an array of the script's own"},
		{`{ mes $@guildmembername$[0]; end; }`, "outside a function that calls getguildmember"},
		{`{ guildgetexp 0; end; }`, "guildgetexp: an amount of 0 or less"},
		{`{ guildopenstorage_log 150001; end; }`, "guildopenstorage_log: another character"},
		{`{ .@r = guildopenstorage_log(); end; }`, "guildopenstorage_log: its result"},
		{`{ guildskill "GD_EXTENSION",1; end; }`, "guildskill: the guild API cannot raise a guild skill"},
		{`{ guildchangegm getcharid(2),"Bob"; end; }`, "guildchangegm: it names the new master by name"},
		{`{ .@c = getguildmasterid(getcharid(2)); end; }`, "getguildmasterid: the guild API does not give character IDs"},
		{`{ .@a = getguildalliance(1,2); end; }`, "getguildalliance: rAthena's relation codes"},
		{`{ mes getguildinfo(getcharid(2),0); end; }`, "getguildinfo: the guild API does not give"},
		{`{ warpguild "prontera",150,150,getcharid(2); end; }`, "warpguild: the guild API cannot warp a guild"},
		{`{ requestguildinfo 1; end; }`, "requestguildinfo: the guild API has no call to load a guild"},
		{`{ .@n = getmapguildusers("prontera",1); end; }`, "getmapguildusers: the guild API cannot count"},
		{`{ maprespawnguildid "prontera",1,3; end; }`, "maprespawnguildid: the guild API cannot warp"},
		{`{ agitstart; end; }`, "agitstart: castles, sieges and guardians are not in the API yet"},
		{`{ .@n = getcastledata("prtg_cas01"); end; }`, "getcastledata: arguments"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}

// TestCastleCommandsConvert checks the castle rows of proposal 0004's
// section 9.
func TestCastleCommandsConvert(t *testing.T) {
	res := eventResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ .@g = getcastledata("prtg_cas01",CD_GUILD_ID); end; }`, []string{`g = R.castle_data("prtg_cas01", 1)`}},
		{`{ .@e = getcastledata(strnpcinfo(2),2) - 5; end; }`, []string{`R.castle_data(`, `, 2) - 5`}},
		{`{ setcastledata "prtg_cas01",CD_GUILD_ID,getcharid(2); end; }`, []string{`R.set_castle_data("prtg_cas01", 1, (player.guild and player.guild.id or 0))`}},
		{`{ setcastledata .@map$,CD_ENABLED_GUARDIAN00+.@i,1; end; }`, []string{`R.set_castle_data(map, (10 + i), 1)`}},
		{`{ mes getcastlename("prtg_cas01"); end; }`, []string{`conversation:say(R.castle_name("prtg_cas01"))`}},
		{`{ flagemblem getcastledata("prtg_cas01",1); end; }`, []string{`R.flag_emblem(npc, R.castle_data("prtg_cas01", 1))`}},
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
	}
}
