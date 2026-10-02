package convert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// goldenItems are sample items whose converted output is kept in
// testdata/golden, so a change to the converter shows in review.
var goldenItems = []string{
	"Red_Potion", "Yggdrasilberry", "Old_Blue_Box", "Wing_Of_Fly", "Wing_Of_Butterfly", "Women's_Bundle",
	"Passion_FB_Hat_Box", "A_Drop_Of_Heal_", "Poring_Card", "Knight_Of_Abyss_Card", "Krieger_Katar2",
	"Sword_Guardian_Card", "Tae_Goo_Lyeon", "Shadow_Staff", "BF_Spear1", "C_Flutter_Butterfly", "Ice_Falchon",
	"Book_Of_The_Apocalypse", "Erhu", "Time_Gap_SS_Dagger1", "Altas_Weapon", "Veteran_Sword", "Green_Potion",
	"Str_Dish01", "RWC_Cele_Fire", "Center_Potion",
}

func TestGolden(t *testing.T) {
	res := loadCore(t, reference.Renewal).Result
	byName := map[string]*Item{}
	for _, it := range res.Items {
		byName[it.AegisName] = it
	}
	for _, name := range goldenItems {
		it := byName[name]
		if it == nil {
			t.Errorf("no item %s", name)
			continue
		}
		got := fmt.Sprintf("-- status: %s\n", it.Status)
		if it.Bonuses != nil {
			got += "-- bonuses: [" + strings.Join(it.Bonuses, ", ") + "]\n"
		}
		if len(it.Unsupported) > 0 {
			got += "-- unsupported: " + unsupportedSummary(it) + "\n-- details: " + strings.Join(it.Unsupported, "; ") + "\n"
		}
		got += it.Lua
		path := filepath.Join("testdata", "golden", strings.ReplaceAll(name, "'", "")+".lua")
		if *update {
			if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != got {
			t.Errorf("%s changed; run go test ./tools/convert -run TestGolden -update and review the diff:\n%s", path, got)
		}
	}
}

// TestHealParity checks every item whose script the items extract read
// as a plain heal: its converted use handler is one item_heal or
// heal_percent call with the same amounts. The Go heal path that these
// scripts replace read the same amounts, so the server heals as before.
func TestHealParity(t *testing.T) {
	for _, mode := range reference.Modes {
		var d struct {
			Items []struct {
				ID   uint32 `json:"id"`
				Heal *struct {
					Command string `json:"command"`
					HP      [2]int `json:"hp"`
					SP      [2]int `json:"sp"`
				} `json:"heal"`
			} `json:"items"`
		}
		if err := reference.LoadFile("extracts/"+string(mode)+"/items.json", &d); err != nil {
			t.Fatal(err)
		}
		res := loadCore(t, mode).Result
		byID := map[uint32]*Item{}
		for _, it := range res.Items {
			byID[it.ID] = it
		}
		n := 0
		for _, it := range d.Items {
			if it.Heal == nil {
				continue
			}
			n++
			amount := func(key string, a [2]int) string {
				switch {
				case a[0] == a[1] && a[0] == 0:
					return ""
				case a[0] == a[1]:
					return fmt.Sprintf("%s = %d", key, a[0])
				}
				return fmt.Sprintf("%s = { %d, %d }", key, a[0], a[1])
			}
			var parts []string
			for _, p := range []string{amount("hp", it.Heal.HP), amount("sp", it.Heal.SP)} {
				if p != "" {
					parts = append(parts, p)
				}
			}
			method := map[string]string{"itemheal": "item_heal", "percentheal": "heal_percent"}[it.Heal.Command]
			want := fmt.Sprintf("  events.on(\"use\", function(player)\n    player:%s { %s }\n  end)\n", method, strings.Join(parts, ", "))
			c := byID[it.ID]
			if c == nil || c.Status != Full || !strings.Contains(c.Lua, want) {
				t.Errorf("%s item %d: want\n%s\ngot\n%v", mode, it.ID, want, c)
			}
		}
		if n < 100 {
			t.Errorf("%s: only %d heal items", mode, n)
		}
	}
}

// convertOne converts one script as an item of a type.
func convertOne(t *testing.T, typ, script string) *Item {
	t.Helper()
	core := loadCore(t, reference.Renewal)
	c := &converter{}
	*c = *core.Result.conv
	return c.convertItem(&ItemScript{ID: 1, AegisName: "Test_Item", Type: typ, Script: script})
}

func TestConvertBonuses(t *testing.T) {
	for _, tc := range []struct{ script, want string }{
		{"bonus bStr,5;", "[str, 5]"},
		{"bonus2 bSubRace,RC_DemiHuman,15;", "[damage_taken, -15, {from: {race: demi_human}}]"},
		{"bonus2 bAddEff,Eff_Stun,300;", "[cause_status, stun, {chance: 300}]"},
		{`bonus3 bAutoSpell,"MG_FIREBOLT",3,50;`, "[cast, MG_FIREBOLT, 3, {chance: 500}]"},
		{`bonus3 bAutoSpellWhenHit,"MG_FIREBOLT",3,50;`, "[cast, MG_FIREBOLT, 3, {chance: 500, on: hit_taken}]"},
		{"bonus2 bHPDrainRate,10,5;", "[drain_hp, 5, {chance: 100}]"},
		{"bonus bSpeedRate,25;", "[move_speed, 25, {percent: true, stacking: highest}]"},
		{`bonus2 bSkillUseSP,"SM_BASH",5;`, "[sp_cost, -5, {skill: SM_BASH}]"},
		{"bonus bMagicDamageReturn,5;", "[reflect_magic, {chance: 500, on: hit_taken}]"},
		{"bonus3 bAddEle,Ele_Fire,10,BF_MAGIC;", "[damage, 10, {against: {element: fire}, kind: magic, attack: skill}]"},
		{`bonus5 bAutoSpell,"MG_COLDBOLT",3,100,BF_WEAPON|BF_MAGIC,0;`,
			"[cast, MG_COLDBOLT, 3, {chance: 1000}], [cast, MG_COLDBOLT, 3, {chance: 1000, attack: skill}], [cast, MG_COLDBOLT, 3, {chance: 1000, kind: magic, attack: skill}]"},
		{"bonus3 bAddEff,Eff_Stun,500,ATF_SELF|ATF_SHORT;", "[cause_status, stun, {chance: 500, target: self, kind: physical, range: melee}]"},
		{"bonus2 bAddEffWhenHit,Eff_Curse,100;", "[cause_status, curse, {chance: 100, on: hit_taken, kind: physical}]"},
		{"bonus2 bResEff,Eff_Freeze,2000;", "[status_resistance, freeze, 2000]"},
		{"bonus bDefEle,Ele_Water;", "[defense_element, water]"},
		{"bonus bAgiVit,3;", "[agi, 3], [vit, 3]"},
		{"bonus bUnbreakableWeapon;", "[unbreakable_weapon]"},
		{"for (.@i = 0; .@i < 3; .@i++) bonus bStr,1;", "[str, 1], [str, 1], [str, 1]"},
		{".@a = 2; switch (.@a) { case 1: bonus bStr,1; break; case 2: bonus bAgi,2; default: bonus bVit,3; }", "[agi, 2], [vit, 3]"},
	} {
		it := convertOne(t, "Armor", tc.script)
		if got := strings.Join(it.Bonuses, ", "); got != tc.want || it.Status != Full {
			t.Errorf("%s:\n got %s (%s, %v)\nwant %s", tc.script, got, it.Status, it.Unsupported, tc.want)
		}
	}
}

func TestConvertComputed(t *testing.T) {
	for _, tc := range []struct{ script, want string }{
		{"bonus bAtk,getrefine()*5;", `bonuses.add("weapon_atk", function() return equipped.refine * 5 end)`},
		{"if (getrefine() >= 9) bonus bStr,2;", "bonuses.add(\"str\", 2, {\n    when = function() return equipped.refine >= 9 end,\n  })"},
		{"if (getrefine() >= 9) bonus bUnbreakableArmor;", "bonuses.add(\"unbreakable_armor\", {\n    when = function() return equipped.refine >= 9 end,\n  })"},
		{"bonus bMaxHP,BaseLevel/10*50;", `return (player.base_level // 10) * 50`},
		{"bonus bMaxHP,-getrefine()/3;", `integers.divide_truncated(-equipped.refine, 3)`},
		{"if (BaseJob == Job_Knight) bonus bStr,1;", "jobs.base_job(player.job) == jobs.knight"},
		{"if (eaclass()&EAJL_THIRD) bonus bStr,1;", "when = function() return jobs.is_third(player.job) end"},
		{"if (readparam(bStr) >= 90) bonus bAtk,10;", "player.base_str >= 90"},
		{`bonus bMatk,getskilllv("MG_FIREBOLT")*3;`, `player:get_skill_level(MG_FIREBOLT) * 3`},
		{"if (getenchantgrade() >= ENCHANTGRADE_B) bonus bStr,1;", "equipped.enchant_grade >= 3 --[[ ENCHANTGRADE_B ]]"},
		{`autobonus "{ bonus bStr,10; }",30,5000,BF_WEAPON,"{ specialeffect2 EF_HEAL; }";`,
			"if hit.wearer_is_source and hit.kind == \"physical\" and random.chance(30, 1000) then"},
	} {
		it := convertOne(t, "Weapon", tc.script)
		if !strings.Contains(it.Lua, tc.want) || it.Status != Full {
			t.Errorf("%s: want %q in\n%s\n%v", tc.script, tc.want, it.Lua, it.Unsupported)
		}
	}
}

func TestConvertUse(t *testing.T) {
	for _, tc := range []struct{ script, want string }{
		{"itemheal rand(45,65),0;", "player:item_heal { hp = { 45, 65 } }"},
		{"percentheal 50,10;", "player:heal_percent { hp = 50, sp = 10 }"},
		{"getitem 501,3;", "player:give_item(RED_POTION, 3)"},
		{`getitem "Red_Potion",1;`, "player:give_item(RED_POTION, 1)"},
		{"rentitem 1201,3600;", "player:give_item(KNIFE, 1, { rent_seconds = 3600 })"},
		{"getgroupitem(IG_BlueBox);", "player:give_item_from_group(BLUEBOX)"},
		{"getgroupitem IG_BlueBox,1;", "player:give_item_from_group(BLUEBOX)"},
		{`warp "SavePoint",0,0;`, "player:warp_to_save_point()"},
		{`warp "Random",0,0;`, "player:warp_to_random_cell()"},
		{`warp "prontera",156,191;`, `player:warp("prontera", 156, 191)`},
		{"specialeffect2 EF_HEAL;", "player:show_effect(effects.heal)"},
		{"sc_start SC_BLESSING,60000,10;", "player:start_status(statuses.blessing, { milliseconds = 60000, level = 10, keep_duration = true })"},
		{"sc_start SC_STONE,0,1,5000,0;", "player:start_status(statuses.stone, { chance = 5000 })"},
		{"sc_start4 SC_STRFOOD,1200000,10,0,0,0;", "player:start_status(statuses.strfood, { milliseconds = 1200000, level = 10, keep_duration = true })"},
		{"sc_start2 SC_POISON,-1,1,30;", `player:start_status(statuses.poison, { milliseconds = "forever", values = { 30 }, keep_duration = true })`},
		{"sc_end SC_POISON;", "player:end_status(statuses.poison)"},
		{`bonus_script "{ bonus bStr,5; }",60,1025,0,EFST_ATTACK_PROPERTY_FIRE;`, "{ \"str\", 5 },\n    }, { ends_on = { \"death\" }, on_repeat = \"extend\", icon = "},
		{`bonus_script "{ bonus bStr,5; }",60,2049;`, "{ \"str\", 5 },\n    }, { ends_on = { \"death\" }, on_repeat = \"add\" })"},
		{`bonus_script "{ bonus bStr,5; }",60,3072;`, "{ \"str\", 5 },\n    }, { on_repeat = \"extend\" })"},
		{`bonus_script "{ bonus bStr,5; }",60,1;`, "{ \"str\", 5 },\n    }, { ends_on = { \"death\" } })"},
		{".@r = rand(1,3); if (.@r == 1) getitem 501,1; else if (.@r == 2) getitem 502,1; else getitem 503,1;",
			"r = random.integer(1, 3)\n    if r == 1 then"},
	} {
		it := convertOne(t, "Usable", tc.script)
		if !strings.Contains(it.Lua, tc.want) || it.Status != Full {
			t.Errorf("%s: want %q in\n%s\n%v", tc.script, tc.want, it.Lua, it.Unsupported)
		}
	}
	it := convertOne(t, "Usable", "sc_end SC_ALL;")
	if it.Status != None || it.Lua != "" || unsupportedSummary(it) != "status effects" {
		t.Errorf("sc_end SC_ALL: %s %q %v", it.Status, it.Lua, it.Unsupported)
	}
}

func TestParse(t *testing.T) {
	for _, src := range []string{
		"bonus bStr,1; bonus2 bAddRace,RC_All,5;",
		"if (getrefine() > 5) { bonus bStr,1; } else bonus bAgi,1;",
		"set .@a, 5; .@a += 2; .@a++; ++.@a;",
		"for (set .@i,0; .@i < 5; set .@i,.@i+1) {}",
		"switch (rand(3)) { case 0: case 1: getitem 501,1; break; default: end; }",
		`autobonus "{ bonus bStr,10; }",10,1000,BF_WEAPON,"{ specialeffect2 EF_HEAL; }";`,
		"do { .@i++; } while (.@i < 3);",
		"/* comment */ bonus bStr, (1 + 2) * 3; // trailing",
		"getgroupitem(IG_BlueBox);",
		"Zeny += 100;",
	} {
		if _, err := Parse(src); err != nil {
			t.Errorf("%s: %v", src, err)
		}
	}
	for _, src := range []string{"bonus bStr,;", "if (1 { }", `"unclosed`} {
		if _, err := Parse(src); err == nil {
			t.Errorf("%s parsed", src)
		}
	}
}
