package convert

import (
	"fmt"
	"regexp"
	"strings"
)

// The player, item and clock commands of review decision 58, areas B7,
// B9 and B10 of proposal 0002b, onto the calls of
// server/modscript/playerapi.go, itemapi.go and clock.go (decision
// 0064). Each takes the attached player only: a character ID argument
// does not convert, as the calls act on a player handle.

// rAthena's constants that these commands read.
const (
	refineCostNormal = 0 // REFINE_COST_NORMAL
	refineMaterialID = 0 // REFINE_MATERIAL_ID
	refineZenyCost   = 1 // REFINE_ZENY_COST
	madoRobot        = 0 // MADO_ROBOT
	card0Forge       = 0x00FF
	card0Create      = 0x00FE
	card0Pet         = -0x0100
)

// pcblockNames are player:set_blocked's actions by rAthena's PCBLOCK_
// bit (script.hpp 2221).
var pcblockNames = []string{"move", "attack", "skill", "use_item", "chat", "immune", "sit_stand", "commands", "npc_click", "emotion", "equip"}

// lookFieldsByType are player:set_look's and player.look's fields by
// rAthena's LOOK_ types (map.hpp 595). getlook reads two more.
var lookFieldsByType = map[int64]string{1: "hair_style", 3: "head_bottom", 4: "head_top", 5: "head_middle", 6: "hair_color", 7: "cloth_color"}

// getlookFields are getlook's types that player.look reads.
var getlookFields = map[int64]string{1: "hair_style", 2: "weapon", 3: "head_bottom", 4: "head_top", 5: "head_middle", 6: "hair_color", 7: "cloth_color", 8: "shield"}

// optionNames are player:has_option's names by rAthena's OPTION_ bits,
// with the bits of cart and dragon, which are several.
var optionNames = []struct {
	name string
	bits int64
}{
	{"sight", 0x1}, {"hide", 0x2}, {"cloak", 0x4}, {"cart", 0x8 | 0x80 | 0x100 | 0x200 | 0x400}, {"falcon", 0x10}, {"riding", 0x20},
	{"invisible", 0x40}, {"orcish", 0x800}, {"wedding", 0x1000}, {"ruwach", 0x2000}, {"chasewalk", 0x4000}, {"flying", 0x8000},
	{"xmas", 0x10000}, {"transform", 0x20000}, {"summer", 0x40000}, {"dragon", 0x80000 | 0x800000 | 0x1000000 | 0x2000000 | 0x4000000},
	{"warg", 0x100000}, {"warg_rider", 0x200000}, {"madogear", 0x400000}, {"hanbok", 0x8000000}, {"oktoberfest", 0x10000000},
	{"summer2", 0x20000000},
}

// gettimeFields are clock.get's fields by gettime's DT_ type (date.hpp).
var gettimeFields = map[int64]string{1: "second", 2: "minute", 3: "hour", 4: "day_of_week", 5: "day_of_month", 6: "month", 7: "year", 8: "day_of_year", 9: "date"}

// battleFlags are the battle flags getbattleflag converts: rules.get's
// settings, and feature flags that wAthena's systems fix.
var battleFlags = map[string]string{
	"base_exp_rate": "", "job_exp_rate": "", "quest_exp_rate": "", "max_hair_style": "", "max_hair_color": "", "max_cloth_color": "",
	// feature.barter is on: barter shops exist. The battleground queue,
	// the refine window, achievements and the stylist do not exist yet.
	"feature.barter": "1", "feature.bgqueue": "0", "feature.refineui": "0", "feature.achievement": "0", "feature.stylist": "0",
}

// inventoryListFields are getinventorylist's arrays (script.cpp 15029)
// that R.inventory_list fills, by the name after @inventorylist_.
var inventoryListFields = []string{"id", "idx", "amount", "equip", "refine", "identify", "attribute", "card1", "card2", "card3", "card4",
	"expire", "bound", "enchantgrade", "tradable", "favorite", "count"}

// onlyPlayer checks that a command has at most n arguments of its own,
// so no character ID follows them.
func onlyPlayer(name string, args []Expr, n int) error {
	if len(args) > n {
		return unsupported(name, "for another character")
	}
	return nil
}

// group3Read converts the commands of areas B7, B9 and B10 that give a
// value. ok is false for another command.
func (b *body) group3Read(name string, args []Expr) (v val, ok bool, err error) {
	f := b.n.file
	switch name {
	case "getgmlevel":
		if err := onlyPlayer(name, args, 0); err != nil {
			return val{}, true, err
		}
		return val{code: "player.staff_level", t: tInt}, true, nil
	case "eaclass":
		if len(args) == 0 {
			return val{code: "player.job_class", t: tInt}, true, nil
		}
		if err := onlyPlayer(name, args, 1); err != nil {
			return val{}, true, err
		}
		j, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		f.need("jobs")
		return val{code: "jobs.class_flags(" + j.asInt() + ")", t: tInt}, true, nil
	case "roclass":
		if len(args) < 1 || len(args) > 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		f.need("jobs")
		sex := "player.sex"
		if len(args) == 2 {
			s, err := b.expr(args[1])
			if err != nil {
				return val{}, true, err
			}
			sex = `(` + s.asInt() + ` == 1 and "male" or "female")`
		}
		return val{code: "jobs.from_class(" + m.asInt() + ", " + sex + ")", t: tInt}, true, nil
	case "getlook":
		if err := onlyPlayer(name, args, 1); err != nil || len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		t, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		if t.static == nil {
			return val{}, true, unsupported(name, "a computed type")
		}
		field, ok := getlookFields[*t.static]
		if !ok {
			if *t.static == 12 || *t.static == 13 {
				return val{}, true, unsupported(name, "the garment's or the second body's view")
			}
			// buildin_getlook gives -1 for another type.
			return intVal(-1), true, nil
		}
		return val{code: "player.look." + field, t: tInt}, true, nil
	case "checkoption":
		if err := onlyPlayer(name, args, 1); err != nil || len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		o, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		if o.static == nil {
			return val{}, true, unsupported(name, "a computed option")
		}
		code, err := hasOptionCode(*o.static)
		if err != nil {
			return val{}, true, err
		}
		return val{code: code, t: tBool}, true, nil
	case "checkdragon", "checkmadogear", "checkwug":
		if err := onlyPlayer(name, args, 0); err != nil {
			return val{}, true, err
		}
		code := map[string]string{"checkdragon": `player:has_option("dragon")`, "checkmadogear": `player:has_option("madogear")`,
			"checkwug": `(player:has_option("warg") or player:has_option("warg_rider"))`}[name]
		return val{code: code, t: tBool}, true, nil
	case "ismounting":
		if err := onlyPlayer(name, args, 0); err != nil {
			return val{}, true, err
		}
		// SC_ALL_RIDING (script.cpp 22857).
		return val{code: "player:has_status(" + f.need("statuses") + ".all_riding)", t: tBool}, true, nil
	case "readparam":
		if err := onlyPlayer(name, args, 1); err != nil || len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		id, isIdent := args[0].(*Ident)
		if !isIdent {
			return val{}, true, unsupported(name, "a parameter by number")
		}
		if _, param, ok := b.n.c.consts.Lookup(id.Name); !ok || !param {
			return val{}, true, unsupported(name, "a parameter by number")
		}
		v, err := b.param(id.Name)
		return v, true, err
	case "getsavepoint":
		if err := onlyPlayer(name, args, 1); err != nil || len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		t, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		if t.static == nil {
			return val{}, true, unsupported(name, "a computed type")
		}
		switch *t.static {
		case 0:
			return val{code: "(select(1, player:get_save_point()))", t: tStr}, true, nil
		case 1, 2:
			return val{code: fmt.Sprintf("(select(%d, player:get_save_point()))", *t.static+1), t: tInt}, true, nil
		}
		return intVal(0), true, nil
	case "getpcblock":
		if err := onlyPlayer(name, args, 0); err != nil {
			return val{}, true, err
		}
		return val{code: "R.pc_block(player)", t: tInt}, true, nil
	case "getiteminfo", "getitemslots":
		if name == "getitemslots" {
			if len(args) != 1 {
				return val{}, true, unsupported(name, "arguments")
			}
			args = []Expr{args[0], &Num{Value: 10}}
		}
		if len(args) != 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		it, err := b.item(args[0])
		if err != nil {
			return val{}, true, err
		}
		t, err := b.expr(args[1])
		if err != nil {
			return val{}, true, err
		}
		typ := tInt
		if t.static != nil && *t.static == 18 {
			typ = tStr
		}
		return val{code: "R.item_info(" + it + ", " + t.asInt() + ")", t: typ}, true, nil
	case "countitem2":
		return b.countitem2(args)
	case "rentalcountitem":
		if err := onlyPlayer(name, args, 1); err != nil || len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		it, err := b.item(args[0])
		if err != nil {
			return val{}, true, err
		}
		return val{code: "player:count_item(" + it + ", { rentals = true })", t: tInt}, true, nil
	case "getbrokenid":
		if err := onlyPlayer(name, args, 1); err != nil || len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		n, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		return val{code: "R.broken_id(player, " + n.asInt() + ")", t: tInt}, true, nil
	case "getequipisenableref":
		slot, err := b.equipSlotArg(name, args, 1)
		if err != nil {
			return val{}, true, err
		}
		return val{code: "R.equip_can_refine(player, " + slot + ")", t: tInt}, true, nil
	case "getequippercentrefinery":
		if len(args) < 1 || len(args) > 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		if len(args) == 2 {
			e, err := b.expr(args[1])
			if err != nil {
				return val{}, true, err
			}
			if e.static == nil || *e.static != 0 {
				return val{}, true, unsupported(name, "the enriched chance, which the refine data lacks")
			}
		}
		slot, err := b.equipSlotArg(name, args[:1], 1)
		if err != nil {
			return val{}, true, err
		}
		return val{code: "R.equip_refine_chance(player, " + slot + ")", t: tInt}, true, nil
	case "getequiprefinecost":
		if len(args) != 3 {
			return val{}, true, unsupported(name, "arguments, or for another character")
		}
		vals, err := b.args(args)
		if err != nil {
			return val{}, true, err
		}
		if vals[1].static == nil || *vals[1].static != refineCostNormal {
			return val{}, true, unsupported(name, "a cost other than REFINE_COST_NORMAL, which the refine data lacks")
		}
		if vals[2].static == nil || *vals[2].static != refineMaterialID && *vals[2].static != refineZenyCost {
			return val{}, true, unsupported(name, "an information other than REFINE_MATERIAL_ID and REFINE_ZENY_COST")
		}
		return val{code: fmt.Sprintf("R.equip_refine_cost(player, %s, %d)", vals[0].asInt(), *vals[2].static), t: tInt}, true, nil
	case "successrefitem", "downrefitem":
		v, err := b.refitem(name, args)
		return v, true, err
	case "failedrefitem":
		slot, err := b.equipSlotArg(name, args, 1)
		if err != nil {
			return val{}, true, err
		}
		return val{code: "R.failedrefitem(player, " + slot + ")", t: tInt}, true, nil
	case "delequip":
		slot, err := b.equipSlotArg(name, args, 1)
		if err != nil {
			return val{}, true, err
		}
		return val{code: "R.delequip(player, " + slot + ")", t: tInt}, true, nil
	case "getnameditem":
		v, err := b.getnameditem(args)
		return v, true, err
	case "gettimetick":
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		t, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		if t.static == nil {
			return val{code: "R.gettimetick(" + t.asInt() + ")", t: tInt}, true, nil
		}
		f.need("clock")
		switch *t.static {
		case 2:
			return val{code: "clock.now()", t: tInt}, true, nil
		case 1:
			return val{code: "R.gettimetick(1)", t: tInt}, true, nil
		}
		return val{code: "clock.milliseconds()", t: tInt}, true, nil
	case "gettime":
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		t, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		if t.static == nil {
			return val{code: "R.gettime(" + t.asInt() + ")", t: tInt}, true, nil
		}
		field, ok := gettimeFields[*t.static]
		if !ok {
			// buildin_gettime: -1 for a type outside DT_ (script.cpp
			// 10908).
			return intVal(-1), true, nil
		}
		return val{code: f.need("clock") + ".get(" + luaString(field) + ")", t: tInt}, true, nil
	case "gettimestr":
		if len(args) < 2 || len(args) > 3 {
			return val{}, true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return val{}, true, err
		}
		at := "nil"
		if len(vals) == 3 {
			at = vals[2].asInt()
		}
		return val{code: fmt.Sprintf("R.gettimestr(%s, %s, %s)", vals[0].asText(), vals[1].asInt(), at), t: tStr}, true, nil
	case "getbattleflag":
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		s, isStr := args[0].(*Str)
		if !isStr {
			return val{}, true, unsupported(name, "a computed flag")
		}
		flag := strings.ToLower(s.Value)
		fixed, ok := battleFlags[flag]
		if !ok {
			return val{}, true, unsupported(name, "the flag %s, which wAthena has no setting for", s.Value)
		}
		if fixed != "" {
			return val{code: fixed, t: tInt}, true, nil
		}
		return val{code: f.need("rules") + ".get(" + luaString(flag) + ")", t: tInt}, true, nil
	case "jobcanentermap":
		return val{}, true, unsupported(name, "the job and map zone restrictions of job_noenter_map.txt, which need WoE:TE map flags")
	case "getexp", "getexp2", "nude", "pushpc", "readbook", "heal", "consumeitem", "getinventorylist":
		return val{}, true, unsupported(name, "as a value")
	}
	return val{}, false, nil
}

// hasOptionCode is checkoption's player:has_option: true when any bit of
// the mask is set.
func hasOptionCode(mask int64) (string, error) {
	var parts []string
	left := mask
	for _, o := range optionNames {
		if mask&o.bits == o.bits || (o.name == "cart" || o.name == "dragon") && mask&o.bits != 0 {
			if mask&o.bits != o.bits {
				// One cart or dragon look of several: any of them counts
				// only as rAthena's bit does, which the API cannot tell.
				return "", unsupported("checkoption", "one look of the carts or the dragons")
			}
			parts = append(parts, `player:has_option("`+o.name+`")`)
			left &^= o.bits
		}
	}
	if left != 0 || len(parts) == 0 {
		return "", unsupported("checkoption", "the option 0x%x", mask)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return "(" + strings.Join(parts, " or ") + ")", nil
}

// equipSlotArg reads the EQI_ slot of an equipment command with n
// arguments of its own.
func (b *body) equipSlotArg(name string, args []Expr, n int) (string, error) {
	if len(args) != n {
		return "", unsupported(name, "arguments, or for another character")
	}
	slot, err := b.expr(args[0])
	if err != nil {
		return "", err
	}
	if slot.static != nil && *slot.static == -1 {
		return "", unsupported(name, "the item whose script runs (EQI_COMPOUND_ON)")
	}
	return slot.asInt(), nil
}

// refitem converts successrefitem and downrefitem: <slot>{, <count>}.
func (b *body) refitem(name string, args []Expr) (val, error) {
	if len(args) < 1 || len(args) > 2 {
		return val{}, unsupported(name, "arguments, or for another character")
	}
	slot, err := b.equipSlotArg(name, args[:1], 1)
	if err != nil {
		return val{}, err
	}
	count := "1"
	if len(args) == 2 {
		c, err := b.expr(args[1])
		if err != nil {
			return val{}, err
		}
		count = c.asInt()
	}
	if name == "downrefitem" {
		count = "-(" + count + ")"
	}
	return val{code: fmt.Sprintf("R.refitem(player, %s, %s)", slot, count), t: tInt}, nil
}

// countitem2 converts countitem2(<item>, <identify>, <refine>,
// <attribute>, <card1> to <card4>): identified copies that are not
// broken convert, as count_item's refine and cards count only those.
func (b *body) countitem2(args []Expr) (val, bool, error) {
	const name = "countitem2"
	if len(args) != 8 {
		return val{}, true, unsupported(name, "arguments, or for another character")
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return val{}, true, err
	}
	if vals[0].static == nil || *vals[0].static == 0 || vals[2].static == nil || *vals[2].static != 0 {
		return val{}, true, unsupported(name, "unidentified or broken copies")
	}
	it, err := b.item(args[0])
	if err != nil {
		return val{}, true, err
	}
	cards, err := cardList(name, vals[3:])
	if err != nil {
		return val{}, true, err
	}
	return val{code: fmt.Sprintf("player:count_item(%s, { refine = %s, cards = %s })", it, vals[1].asInt(), cards), t: tInt}, true, nil
}

// cardList writes getitem2's four card arguments as a list of the
// filled slots. card[0] of a forged, named or pet item is not a card.
func cardList(name string, cards []val) (string, error) {
	var parts []string
	computed := false
	for i, c := range cards {
		if c.static == nil {
			computed = true
			continue
		}
		if i == 0 && (*c.static == card0Forge || *c.static == card0Create || *c.static == card0Pet) {
			return "", unsupported(name, "a forged, named or pet item's mark in card 1")
		}
		if *c.static != 0 {
			parts = append(parts, c.code)
		}
	}
	if computed {
		// R.cards leaves out the empty slots when the script runs.
		all := make([]string, len(cards))
		for i, c := range cards {
			all[i] = c.asInt()
		}
		return "R.cards(" + strings.Join(all, ", ") + ")", nil
	}
	// rAthena's slots keep their places; wAthena lists the filled ones.
	return "{ " + strings.Join(parts, ", ") + " }", nil
}

// getnameditem converts getnameditem(<item>, <character>): the item named
// for the attached player, or for a player of the map by name or
// character ID. It gives 1, or 0 when the player is not on the map.
func (b *body) getnameditem(args []Expr) (val, error) {
	const name = "getnameditem"
	if len(args) != 2 {
		return val{}, unsupported(name, "arguments")
	}
	it, err := b.item(args[0])
	if err != nil {
		return val{}, err
	}
	who, err := b.expr(args[1])
	if err != nil {
		return val{}, err
	}
	if who.code == "player.name" || who.code == "player.character_id" {
		return val{code: "R.named_item(player, " + it + ", player)", t: tInt}, nil
	}
	b.n.file.need("maps")
	return val{code: "R.named_item(player, " + it + ", " + who.code + ")", t: tInt}, nil
}

// group3Command converts the commands of areas B7, B9 and B10 that are
// statements. ok is false for another command.
func (b *body) group3Command(name string, args []Expr) (ok bool, err error) {
	switch name {
	case "getexp", "getexp2":
		if err := onlyPlayer(name, args, 2); err != nil || len(args) != 2 {
			return true, unsupported(name, "arguments, or for another character")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		raw := ""
		if name == "getexp2" {
			raw = ", { raw = true }"
		}
		b.line("player:give_exp(%s, %s%s)", vals[0].asInt(), vals[1].asInt(), raw)
	case "heal":
		if err := onlyPlayer(name, args, 2); err != nil || len(args) != 2 {
			return true, unsupported(name, "arguments, or for another character")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		b.line("player:heal(%s, %s)", vals[0].asInt(), vals[1].asInt())
	case "setpcblock":
		if err := onlyPlayer(name, args, 2); err != nil || len(args) != 2 {
			return true, unsupported(name, "arguments, or for another character")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		on := vals[1].asInt() + " > 0"
		if vals[1].static != nil {
			on = map[bool]string{true: "true", false: "false"}[*vals[1].static > 0]
		}
		if m := vals[0].static; m != nil && *m > 0 && *m <= 0x7FF {
			for i, n := range pcblockNames {
				if *m&(1<<i) != 0 {
					b.line("player:set_blocked(%s, %s)", luaString(n), on)
				}
			}
			break
		}
		b.line("R.set_pc_block(player, %s, %s)", vals[0].asInt(), on)
	case "disable_items", "enable_items":
		if len(args) != 0 {
			return true, unsupported(name, "arguments")
		}
		b.line("R.lock_items(conversation, %t)", name == "disable_items")
	case "nude":
		if err := onlyPlayer(name, args, 0); err != nil {
			return true, err
		}
		b.line("player:unequip_all()")
	case "pushpc":
		if len(args) != 2 {
			return true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		b.line("R.pushpc(player, %s, %s)", vals[0].asInt(), vals[1].asInt())
	case "transform":
		if len(args) != 2 {
			return true, unsupported(name, "a status with the transform")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return true, err
		}
		code := m.asInt()
		if m.str != nil {
			id, ok := b.n.c.monsterIDs[strings.ToUpper(*m.str)]
			if !ok {
				return true, unsupported(name, "%q is not a monster", *m.str)
			}
			code = b.n.file.record("monsters.id", b.n.c.monsters[id].sprite)
		} else if m.static != nil {
			info, ok := b.n.c.monsters[*m.static]
			if !ok {
				return true, unsupported(name, "%d is not a monster", *m.static)
			}
			code = b.n.file.record("monsters.id", info.sprite)
		}
		d, err := b.expr(args[1])
		if err != nil {
			return true, err
		}
		b.line("player:transform(%s, %s)", code, d.asInt())
	case "active_transform":
		return true, unsupported(name, "SC_ACTIVE_MONSTER_TRANSFORM, which player:transform does not start")
	case "readbook":
		if len(args) != 2 {
			return true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		b.line("player:open_book(%s, %s)", vals[0].asInt(), vals[1].asInt())
	case "setlook":
		if err := onlyPlayer(name, args, 2); err != nil || len(args) != 2 {
			return true, unsupported(name, "arguments, or for another character")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		if vals[0].static == nil {
			return true, unsupported(name, "a computed type")
		}
		field, ok := lookFieldsByType[*vals[0].static]
		if !ok {
			return true, unsupported(name, "the look type %d", *vals[0].static)
		}
		b.line("player:set_look { %s = %s }", field, vals[1].asInt())
	case "getinventorylist":
		if err := onlyPlayer(name, args, 0); err != nil {
			return true, err
		}
		return true, b.getinventorylist()
	case "setdragon":
		if len(args) > 1 {
			return true, unsupported(name, "for another character")
		}
		if len(args) == 1 {
			c, err := b.expr(args[0])
			if err != nil {
				return true, err
			}
			if c.static == nil || *c.static > 1 {
				return true, unsupported(name, "a dragon's color other than green, which core's mounts lack")
			}
		}
		b.line("R.setdragon(player)")
	case "setmadogear":
		if len(args) > 2 {
			return true, unsupported(name, "for another character")
		}
		on := "true"
		if len(args) >= 1 {
			v, err := b.expr(args[0])
			if err != nil {
				return true, err
			}
			on = v.asBool()
		}
		if len(args) == 2 {
			t, err := b.expr(args[1])
			if err != nil {
				return true, err
			}
			if t.static == nil || *t.static != madoRobot {
				return true, unsupported(name, "a Madogear other than MADO_ROBOT, which core's mounts lack")
			}
		}
		b.line("R.setmadogear(player, %s)", on)
	case "getitem2":
		return true, b.getitem2(name, args)
	case "rentitem", "rentitem2":
		return true, b.rentitem(name, args)
	case "makeitem", "makeitem2":
		return true, b.makeitem(name, args)
	case "repair":
		if err := onlyPlayer(name, args, 1); err != nil || len(args) != 1 {
			return true, unsupported(name, "arguments")
		}
		n, err := b.expr(args[0])
		if err != nil {
			return true, err
		}
		b.line("player:repair_item(%s)", n.asInt())
	case "repairall":
		if err := onlyPlayer(name, args, 0); err != nil {
			return true, err
		}
		b.line("player:repair_all()")
	case "consumeitem":
		if err := onlyPlayer(name, args, 1); err != nil || len(args) != 1 {
			return true, unsupported(name, "arguments, or for another character")
		}
		it, err := b.item(args[0])
		if err != nil {
			return true, err
		}
		b.line("player:run_item_script(%s)", it)
	case "mergeitem":
		if err := onlyPlayer(name, args, 0); err != nil {
			return true, err
		}
		b.line("player:merge_items()")
	case "successrefitem", "downrefitem", "failedrefitem", "delequip", "getnameditem":
		v, _, err := b.group3Read(name, args)
		if err != nil {
			return true, err
		}
		b.line("%s", v.code)
	default:
		return false, nil
	}
	return true, nil
}

// inventoryListRef matches the @inventorylist_ variables a script reads.
var inventoryListRef = regexp.MustCompile(`(?i)@inventorylist_([a-z0-9_]+)`)

// getinventorylist fills the @inventorylist_ arrays that the script
// reads, through R.inventory_list.
func (b *body) getinventorylist() error {
	used := map[string]bool{}
	for _, m := range inventoryListRef.FindAllStringSubmatch(b.n.line.Script, -1) {
		used[strings.ToLower(m[1])] = true
	}
	var parts []string
	for _, field := range inventoryListFields {
		if !used[field] {
			continue
		}
		v, err := b.variable("@inventorylist_" + field)
		if err != nil {
			return err
		}
		parts = append(parts, field+" = "+v.local)
		delete(used, field)
	}
	for field := range used {
		return unsupported("getinventorylist", "@inventorylist_%s", field)
	}
	b.n.file.need("variables")
	b.line("R.inventory_list(player, { %s })", strings.Join(parts, ", "))
	return nil
}

// getitem2 converts getitem2 and getitembound2: <item>, <amount>,
// <identify>, <refine>, <attribute>, <card1> to <card4>{, <bound>}.
func (b *body) getitem2(name string, args []Expr) error {
	n := 9
	if name == "getitembound2" {
		n = 10
	}
	if len(args) != n {
		return unsupported(name, "arguments, or for another character")
	}
	it, err := b.item(args[0])
	if err != nil {
		return err
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return err
	}
	cards, err := cardList(name, vals[4:8])
	if err != nil {
		return err
	}
	opts := copyOptions(vals[1], vals[2], vals[3], cards)
	if name == "getitembound2" {
		bnd := vals[8]
		if bnd.static == nil || *bnd.static < 0 || *bnd.static >= int64(len(boundNames)) {
			return unsupported(name, "a computed bound")
		}
		if *bnd.static > 0 {
			opts = append(opts, "bound = "+luaString(boundNames[*bnd.static]))
		}
	}
	b.line("R.give_item(player, %s, %s%s)", it, vals[0].asInt(), optionsArg(opts))
	return nil
}

// copyOptions writes give_item's and drop_item's copy options from
// getitem2's identify, refine and attribute and its card list, leaving
// out what is the default.
func copyOptions(identify, refine, attribute val, cards string) []string {
	var out []string
	if refine.static == nil || *refine.static != 0 {
		out = append(out, "refine = "+refine.asInt())
	}
	if cards != "{  }" {
		out = append(out, "cards = "+cards)
	}
	switch {
	case identify.static == nil:
		out = append(out, "identified = "+identify.asInt()+" ~= 0")
	case *identify.static == 0:
		out = append(out, "identified = false")
	}
	switch {
	case attribute.static == nil:
		out = append(out, "broken = "+attribute.asInt()+" == 1")
	case *attribute.static == 1:
		out = append(out, "broken = true")
	}
	return out
}

// optionsArg is ", { options }", or "" for none.
func optionsArg(opts []string) string {
	if len(opts) == 0 {
		return ""
	}
	return ", { " + strings.Join(opts, ", ") + " }"
}

// boundNames are wa.Bound's names by rAthena's BOUND_ values.
var boundNames = []string{"", "account", "guild", "party", "character"}

// rentitem converts rentitem <item>, <seconds> and rentitem2 with
// getitem2's copy arguments after them.
func (b *body) rentitem(name string, args []Expr) error {
	n := 2
	if name == "rentitem2" {
		n = 9
	}
	if len(args) != n {
		return unsupported(name, "arguments, or for another character")
	}
	it, err := b.item(args[0])
	if err != nil {
		return err
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return err
	}
	opts := ""
	if name == "rentitem2" {
		cards, err := cardList(name, vals[4:8])
		if err != nil {
			return err
		}
		opts = optionsArg(copyOptions(vals[1], vals[2], vals[3], cards))
	}
	b.line("R.rentitem(player, %s, %s%s)", it, vals[0].asInt(), opts)
	return nil
}

// makeitem converts makeitem <item>, <amount>, <map>, <x>, <y> and
// makeitem2 with getitem2's copy arguments after them. The map "this" is
// the attached player's.
func (b *body) makeitem(name string, args []Expr) error {
	if name == "makeitem" && (len(args) < 5 || len(args) > 6) || name == "makeitem2" && (len(args) < 12 || len(args) > 13) {
		return unsupported(name, "arguments")
	}
	id, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if id.static != nil && *id.static < 0 {
		return unsupported(name, "a negative item ID, which identifies by the item data")
	}
	it, err := b.item(args[0])
	if err != nil {
		return err
	}
	vals, err := b.args(args[1:5])
	if err != nil {
		return err
	}
	m := vals[1].asText()
	if vals[1].str != nil && strings.EqualFold(*vals[1].str, "this") {
		m = "player.map"
	}
	opts := ""
	if name == "makeitem2" {
		more, err := b.args(args[5:12])
		if err != nil {
			return err
		}
		cards, err := cardList(name, more[3:7])
		if err != nil {
			return err
		}
		opts = optionsArg(copyOptions(more[0], more[1], more[2], cards))
	}
	b.n.file.need("maps")
	b.line("R.makeitem(%s, %s, %s, %s, %s%s)", m, vals[2].asInt(), vals[3].asInt(), it, vals[0].asInt(), opts)
	return nil
}
