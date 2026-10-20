package convert

import (
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/modscript"
)

// TestGroup3Convert checks the converted text of the commands of review
// decision 58, areas B7, B9 and B10, and the reasons of the forms that
// do not convert.
func TestGroup3Convert(t *testing.T) {
	res := questResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		// B7: player reads and actions.
		{`{ getexp 100,50; getexp2 -10,0; end; }`, []string{"player:give_exp(100, 50)", "player:give_exp(-10, 0, { raw = true })"}},
		{`{ getexp 1,1,150000; end; }`, []string{"!getexp: arguments, or for another character"}},
		{`{ heal 100,-5; Hp = 1; Hp -= 10; end; }`, []string{"player:heal(100, -5)", "player:set_hp(1)", "player:set_hp((player.hp - 10))"}},
		{`{ mes "" + Weight + "/" + MaxWeight + " " + SkillPoint; end; }`, []string{"player.weight", "player.max_weight", "player.skill_points"}},
		{`{ setpcblock PCBLOCK_MOVE|PCBLOCK_ATTACK,true; setpcblock .@f,0; mes "" + getpcblock(); end; }`, []string{
			`player:set_blocked("move", true)`, `player:set_blocked("attack", true)`, "R.set_pc_block(player, f, false)", "R.pc_block(player)",
		}},
		{`{ disable_items; enable_items; nude; pushpc DIR_WEST,2; end; }`, []string{
			"R.lock_items(conversation, true)", "R.lock_items(conversation, false)", "player:unequip_all()", "R.pushpc(player, 2, 2)",
		}},
		{`{ transform 1002,10000; readbook 7001,1; end; }`, []string{"player:transform(PORING, 10000)", "player:open_book(7001, 1)"}},
		{`{ transform 1002,10000,SC_INCSTR,5; end; }`, []string{"!transform: a status with the transform"}},
		{`{ active_transform 1002,10000; end; }`, []string{"!active_transform: SC_ACTIVE_MONSTER_TRANSFORM"}},
		{`{ mes getsavepoint(0) + " " + getsavepoint(1); end; }`, []string{"(select(1, player:get_save_point()))", "(select(2, player:get_save_point()))"}},
		{`{ setlook LOOK_HAIR,5; setlook LOOK_CLOTHES_COLOR,2; mes "" + getlook(LOOK_HAIR_COLOR); end; }`, []string{
			"player:set_look { hair_style = 5 }", "player:set_look { cloth_color = 2 }", "player.look.hair_color",
		}},
		{`{ setlook LOOK_WEAPON,1; end; }`, []string{"!setlook: the look type 2"}},
		{`{ if (checkoption(0x30)) end; if (checkdragon()) end; if (checkwug()) end; end; }`, []string{
			`(player:has_option("falcon") or player:has_option("riding"))`, `player:has_option("dragon")`,
			`(player:has_option("warg") or player:has_option("warg_rider"))`,
		}},
		{`{ if (ismounting()) end; setdragon; setmadogear false; end; }`, []string{
			"player:has_status(statuses.all_riding)", "R.setdragon(player)", "R.setmadogear(player, false)",
		}},
		{`{ setdragon 3; end; }`, []string{"!setdragon: a dragon's color other than green"}},
		{`{ if (getgmlevel() < 99) end; mes "" + eaclass() + " " + eaclass(Job_Knight) + " " + roclass(EAJ_SWORDMAN); end; }`, []string{
			"player.staff_level", "player.job_class", "jobs.class_flags(jobs.knight)", "jobs.from_class(1, player.sex)",
		}},
		{`{ mes "" + readparam(BaseLevel); end; }`, []string{"player.base_level"}},
		{`{ getinventorylist; mes "" + @inventorylist_count + " " + @inventorylist_attribute[0]; end; }`, []string{
			"R.inventory_list(player, { attribute = TEMP_INVENTORYLIST_ATTRIBUTE, count = TEMP_INVENTORYLIST_COUNT })",
		}},
		{`{ getinventorylist; mes "" + @inventorylist_option_id1[0]; end; }`, []string{"!getinventorylist: @inventorylist_option_id1"}},
		// B9: items and equipment.
		{`{ getitem2 1201,1,1,7,0,4001,0,0,0; getitem2 1201,1,0,0,1,.@c,0,0,0; end; }`, []string{
			"R.give_item(player, KNIFE, 1, { refine = 7, cards = { 4001 } })",
			"R.give_item(player, KNIFE, 1, { cards = R.cards(c, 0, 0, 0), identified = false, broken = true })",
		}},
		{`{ getitem2 1201,1,1,0,0,255,0,0,0; end; }`, []string{"!getitem2: a forged, named or pet item's mark in card 1"}},
		{`{ if (getnameditem(1201, strcharinfo(0))) end; getnameditem 1201,"Bob"; end; }`, []string{
			"R.named_item(player, KNIFE, player)", `R.named_item(player, KNIFE, "Bob")`,
		}},
		{`{ mes "" + countitem2(1201,1,7,0,4001,0,0,0) + " " + rentalcountitem(909); end; }`, []string{
			"player:count_item(KNIFE, { refine = 7, cards = { 4001 } })", "player:count_item(JELLOPY, { rentals = true })",
		}},
		{`{ mes "" + countitem2(1201,0,7,0,0,0,0,0); end; }`, []string{"!countitem2: unidentified or broken copies"}},
		{`{ rentitem 1201,3600; makeitem 909,3,"this",10,12; makeitem2 1201,1,"prontera",5,5,1,4,0,0,0,0,0; end; }`, []string{
			"R.rentitem(player, KNIFE, 3600)", "R.makeitem(player.map, 10, 12, JELLOPY, 3)", `R.makeitem("prontera", 5, 5, KNIFE, 1, { refine = 4 })`,
		}},
		{`{ mes "" + getiteminfo(909, ITEMINFO_WEIGHT) + getiteminfo(909, ITEMINFO_AEGISNAME) + getitemslots(1201); end; }`, []string{
			"R.item_info(JELLOPY, 6)", "R.item_info(JELLOPY, 18)", "R.item_info(KNIFE, 10)",
		}},
		{`{ if (getequipisenableref(EQI_HAND_R)) end; mes "" + getequippercentrefinery(EQI_HAND_R) + getequiprefinecost(EQI_HAND_R, REFINE_COST_NORMAL, REFINE_ZENY_COST); end; }`, []string{
			"R.equip_can_refine(player, 9)", "R.equip_refine_chance(player, 9)", "R.equip_refine_cost(player, 9, 1)",
		}},
		{`{ mes "" + getequippercentrefinery(EQI_HAND_R, true); end; }`, []string{"!getequippercentrefinery: the enriched chance"}},
		{`{ mes "" + getequiprefinecost(EQI_HAND_R, REFINE_COST_HD, REFINE_ZENY_COST); end; }`, []string{"!getequiprefinecost: a cost other than REFINE_COST_NORMAL"}},
		{`{ successrefitem EQI_HAND_R; downrefitem EQI_HAND_R,2; failedrefitem EQI_HAND_R; delequip EQI_ARMOR; end; }`, []string{
			"R.refitem(player, 9, 1)", "R.refitem(player, 9, -(2))", "R.failedrefitem(player, 9)", "R.delequip(player, 7)",
		}},
		{`{ if (getbrokenid(1)) repair 1; repairall; consumeitem 909; mergeitem; end; }`, []string{
			"R.broken_id(player, 1)", "player:repair_item(1)", "player:repair_all()", "player:run_item_script(JELLOPY)", "player:merge_items()",
		}},
		// B10: clock and server settings.
		{`{ mes "" + gettimetick(2) + gettimetick(0) + gettimetick(1) + gettime(DT_HOUR) + gettime(DT_DAYOFWEEK) + gettime(20); end; }`, []string{
			"clock.now()", "clock.milliseconds()", "R.gettimetick(1)", `clock.get("hour")`, `clock.get("day_of_week")`, "-1",
		}},
		{`{ mes gettimestr("%Y-%m-%d", 21); end; }`, []string{`R.gettimestr("%Y-%m-%d", 21, nil)`}},
		{`{ mes "" + getbattleflag("base_exp_rate") + getbattleflag("feature.bgqueue") + getbattleflag("feature.barter"); end; }`, []string{
			`rules.get("base_exp_rate")`, "0", "1",
		}},
		{`{ mes "" + getbattleflag("pvp_exp"); end; }`, []string{"!getbattleflag: the flag pvp_exp"}},
		{`{ if (jobcanentermap("te_prtcas01")) end; end; }`, []string{"!jobcanentermap: the job and map zone restrictions"}},
		{`{ skill "AL_HEAL",1,SKILL_PERM_GRANT; jobchange Job_Knight,Upper; end; }`, []string{
			`player:give_skill(AL_HEAL, 1, "permanent_grant")`, "player:change_job(jobs.knight, { force = true, keep_line = true })",
		}},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		for _, want := range c.want {
			if reason, ok := strings.CutPrefix(want, "!"); ok {
				if err == nil || !strings.Contains(err.Error(), reason) {
					t.Errorf("%s: got %v, want %q", c.script, err, reason)
				}
				continue
			}
			if err != nil || !strings.Contains(lua, want) {
				t.Errorf("%s: %v, want %q in\n%s", c.script, err, want, lua)
			}
		}
	}
}

// TestDuplicateVariablesConvert checks that a script that duplicates run
// keeps its .x variables as NPC variables of its source, which every
// duplicate shares (npc_parse_duplicate, npc.cpp 4596), and that
// getvariableofnpc of a duplicate reaches the source's.
func TestDuplicateVariablesConvert(t *testing.T) {
	res := questResult(t)
	c := res.conv
	c.duplicated["Src#t"] = true
	c.dupOf["Dup#t"] = "Src#t"
	n := c.newNPCConv(&npcLine{Kind: "script", Name: "Src#t", Display: "Src#t", Map: "prontera", Script: `{ .count += 1; mes "" + .count; end; }`})
	lua, err := n.npcScript()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`local NPC_COUNT = variables.npc_integer "count"`, `local NpcSource = world.get_npc("Src#t") or npc`,
		"NPC_COUNT:add(NpcSource, 1)", "NPC_COUNT:get(NpcSource)"} {
		if !strings.Contains(lua, want) {
			t.Errorf("want %q in\n%s", want, lua)
		}
	}
	o := c.newNPCConv(&npcLine{Kind: "script", Name: "Reader#t", Map: "prontera", Script: `{ mes "" + getvariableofnpc(.count, "Dup#t"); end; }`})
	lua, err = o.npcScript()
	if err != nil || !strings.Contains(lua, `R.npc_of("Src#t")`) {
		t.Errorf("%v\n%s", err, lua)
	}
}

// TestScriptDisplayName checks that a "::" name keeps its # part in
// display_name, for npc.full_name.
func TestScriptDisplayName(t *testing.T) {
	for _, c := range []struct{ name, display, want string }{
		{"G1", "Guard#east::G1", "Guard#east"},
		{"Guard#east", "Guard#east", "Guard"},
		{"G2", "Guard::G2", "Guard"},
	} {
		if got := scriptDisplayName(&npcLine{Name: c.name, Display: strings.TrimSuffix(c.display, "::"+c.name)}); got != c.want {
			t.Errorf("%s: %q, want %q", c.display, got, c.want)
		}
	}
}

// TestRunNPCTimerTick runs a converted NPC timer that reads its tick and
// resumes after a stop, as npc_gettimerevent_tick and
// npc_timerevent_start (npc.cpp 1542, 1704): the tick follows the server
// clock, and startnpctimer goes on from the stopped tick.
func TestRunNPCTimerTick(t *testing.T) {
	res := builtinResult(t, map[string]string{"Tick#t": `{
	end;
OnInit:
	initnpctimer;
	end;
OnTimer1000:
	stopnpctimer;
	announce "" + getnpctimer(0), bc_all;
	startnpctimer;
	end;
OnTimer2000:
	announce "two " + getnpctimer(0), bc_all;
	end;
}`})
	n := res.NPCs[len(res.NPCs)-1]
	if n.Status != Full {
		t.Fatalf("%s: %s %v", n.Source, n.Status, n.Unsupported)
	}
	_, base, _ := testRuntime(t, "renewal")
	h := &eventHost{npcHost: &npcHost{testHost: base, npcs: map[int64]*NPC{1: n}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}}}
	logs := &logRecorder{}
	rt := builtinRuntime(t, res, h, logs)
	if _, err := rt.StartNPC(1, "core:"+n.Script); err != nil {
		t.Fatal(err)
	}
	for _, now := range []int64{999, 1000, 1999, 2000} {
		h.now = now
		rt.Advance(now)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	if got, want := strings.Join(h.said, "|"), "1000|two 2000"; got != want {
		t.Errorf("said %q, want %q", got, want)
	}
}
