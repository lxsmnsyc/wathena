package convert

import (
	"strings"
	"testing"
)

// TestMapsConvert checks the converted text of the map, area and monster
// commands of review decision 58, area B8, and the reasons of the forms
// that do not convert.
func TestMapsConvert(t *testing.T) {
	res := questResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ monster "geffen",100,110,"Boss Jelly",1002,3,"X#t::OnDead",2,1; end; OnDead: end; }`, []string{
			`R.monster("geffen", 100, 110, "Boss Jelly", PORING, 3, { label = "OnDead", size = "big", ai = "attack" })`,
		}},
		{`{ monster "prontera",0,0,"--ja--",1002,1,"Other#t::OnX"; end; }`, []string{`{ label = "Other#t::OnX" }`}},
		{`{ areamonster "geffen",10,10,20,20,"",1002,5,strnpcinfo(3)+"::OnDead"; end; OnDead: end; }`, []string{
			`R.areamonster("geffen", 10, 10, 20, 20, "", PORING, 5, { label = "OnDead" })`,
		}},
		{`{ killmonster "geffen","X#t::OnDead"; killmonster "geffen","All",1; killmonsterall "geffen"; killmonsterall "geffen",1; end; OnDead: end; }`, []string{
			`R.on_map("geffen", maps.remove_monsters, { label = "OnDead" })`,
			`R.on_map("geffen", maps.remove_monsters, { scripted = true, run_labels = true })`,
			`R.on_map("geffen", maps.remove_monsters, nil)`,
			`R.on_map("geffen", maps.remove_monsters, { run_labels = true })`,
		}},
		{`{ mes "" + mobcount("geffen","X#t::OnDead") + mobcount("geffen","all") + mobcount("geffen","Zeith#frozen02::OnMyMobDead"); end; OnDead: end; }`, []string{
			`(maps.count_monsters("geffen", { label = "OnDead" }) or -1)`, `(maps.count_monsters("geffen") or -1)`,
			`(maps.count_monsters("geffen", { label = "Zeith#frozen02::OnMyMobDead" }) or -1)`,
		}},
		{`{ killmonster "geffen","#f_boss_c::OnMobDead" + .@i; areamonster "geffen",1,1,5,5,"",1002,1,"X#t::OnMobDead" + .@i; end; }`, []string{
			`{ label = "#f_boss_c::OnMobDead" .. i }`, `{ label = "OnMobDead" .. i }`,
		}},
		{`{ .@l$ = "A::OnB"; monster "geffen",0,0,"",1002,1,.@l$ + "::OnMyMobDead"; killmonster "geffen",.@l$,1; mes "" + mobcount("geffen",.@l$); end; }`, []string{
			`{ label = R.label(l .. "::OnMyMobDead") }`, `R.killmonster("geffen", l, true)`, `R.mobcount("geffen", l)`,
		}},
		{`{ mes "" + getmapusers("geffen") + getareausers("geffen",1,2,3,4); end; }`, []string{
			`(maps.count_players("geffen") or -1)`, `(maps.count_players("geffen", { rectangle = { { 1, 2 }, { 3, 4 } } }) or -1)`,
		}},
		{`{ mapwarp "geffen","prontera",150,150; mapwarp "geffen","prontera",150,150,1,getcharid(2); end; }`, []string{
			`R.warp_players("geffen", "prontera", 150, 150, nil)`, `R.warp_players("geffen", "prontera", 150, 150, { guild = `,
		}},
		{`{ areawarp "geffen",1,1,9,9,"prontera",150,150; areawarp "geffen",1,1,9,9,"prontera",150,150,155,155; end; }`, []string{
			`R.warp_players("geffen", "prontera", 150, 150, { rectangle = { { 1, 1 }, { 9, 9 } } })`,
			`R.areawarp("geffen", "prontera", 150, 150, 155, 155, { rectangle = { { 1, 1 }, { 9, 9 } } })`,
		}},
		{`{ warpparty "prontera",150,150,getcharid(1); warpparty "Leader",0,0,getcharid(1),"geffen",2,2; end; }`, []string{
			`R.warpparty(`, `"prontera", 150, 150, player, nil, 0, 0)`, `"Leader", 0, 0, player, "geffen", 2, 2)`}},
		{`{ areawarp "geffen",1,1,9,9,"Random",0,0; mapwarp "geffen","Random",0,0; end; }`, []string{
			`R.on_map("geffen", maps.warp_players, "random", 0, 0, { rectangle = { { 1, 1 }, { 9, 9 } } })`, `mapwarp to "Random" moves nobody`}},
		{`{ monster "this",0,0,"--ja--",1002,1; areamonster "this",1,1,5,5,"--ja--",1002,1; if (mobcount("this","all")) end; end; }`, []string{
			`R.monster(R.this_map(player), 0, 0`, `R.areamonster(R.this_map(player), 1, 1`, `maps.count_monsters(R.this_map(player))`}},
		{`{ areapercentheal "geffen",1,1,9,9,100,50; end; }`, []string{`R.areapercentheal("geffen", 1, 1, 9, 9, 100, 50)`}},
		{`{ setcell "geffen",1,1,5,5,cell_walkable,0; if (checkcell("geffen",1,1,cell_chknopass)) end; end; }`, []string{
			`R.on_map("geffen", maps.set_cells, 1, 1, 5, 5, "walkable", false)`, `maps.check_cell("geffen", 1, 1, "no_pass")`,
		}},
		{`{ setwall "geffen",10,10,5,DIR_EAST,0,"w1"; if (checkwall("w1")) delwall "w1"; end; }`, []string{
			`R.setwall("geffen", 10, 10, 5, 6, false, "w1")`, `R.checkwall("w1")`, `R.delwall("w1")`,
		}},
		{`{ setmapflag "geffen",mf_nomemo; removemapflag "geffen",mf_nomemo; mes "" + getmapflag("geffen",mf_nomemo) + getmapflag("geffen",mf_pvp); end; }`, []string{
			`R.on_map("geffen", maps.set_flag, "nomemo", true)`, `R.on_map("geffen", maps.set_flag, "nomemo", false)`,
			`maps.get_flag("geffen", "nomemo")`,
		}},
		{`{ getfreecell "geffen",.@x,.@y,100,100,5,5,3; end; }`, []string{`= R.getfreecell("geffen", 100, 100, 5, 5, 3, player)`}},
		{`{ areaannounce "geffen",1,1,9,9,"Hi",bc_yellow; end; }`, []string{`R.areaannounce("geffen", 1, 1, 9, 9, "Hi")`}},
		{`{ mes getmonsterinfo(1002,MOB_NAME) + getmonsterinfo("PORING",MOB_LV); end; }`, []string{
			`R.monster_info(1002, 1)`, `R.monster_info("PORING", 2)`,
		}},
		{`{ mes getmonsterinfo(1002,99); end; }`, []string{"!getmonsterinfo: the type 99"}},
		{`{ if (checkcell("geffen",1,1,cell_chkwater)) end; end; }`, []string{"!checkcell: water cells"}},
		{`{ removemapflag "geffen",mf_gvg; end; }`, []string{"!removemapflag: the map flag gvg: sieges do not exist"}},
		{`{ warpparty "prontera",150,150,getcharid(1),"geffen",2,2,4; end; }`, []string{"!warpparty: arguments"}},
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
