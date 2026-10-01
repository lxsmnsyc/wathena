package convert

import (
	"fmt"
	"strings"
)

// argKind says how a bonus argument is written: a number, or a record or
// constant written by name.
type argKind int

const (
	kNum argKind = iota
	kRace
	kElement
	kSize
	kClass
	kMonsterGroup
	kWeaponType
	kStatus
	kSkill
	kLevel
	kItem
	kItemGroup
	kMonster
)

// kindModule is the constant module of each kind written by name.
var kindModule = map[argKind]string{
	kRace: "races", kElement: "elements", kSize: "sizes", kClass: "monster_classes",
	kMonsterGroup: "monster_groups", kWeaponType: "weapon_types", kStatus: "statuses",
}

// targetKey is the key of against or from for a kind.
var targetKey = map[argKind]string{
	kRace: "race", kElement: "element", kSize: "size", kClass: "class", kMonsterGroup: "monster_group", kMonster: "monster",
}

type warg struct {
	kind argKind
	v    Expr
}

// wopt is one option. enum options hold a string such as "physical",
// flag options are true, target options hold a key and a value, and the
// rest hold a value of their kind.
type wopt struct {
	key  string
	enum string
	flag bool
	tkey string
	kind argKind
	v    Expr
}

// wcall is one bonuses.add call.
type wcall struct {
	name string
	args []warg
	opts []wopt
}

func call(name string, args ...warg) *wcall { return &wcall{name: name, args: args} }

func (w *wcall) with(opts ...wopt) *wcall {
	w.opts = append(w.opts, opts...)
	return w
}

func num(v Expr) warg                { return warg{kNum, v} }
func of(k argKind, v Expr) warg      { return warg{k, v} }
func enum(key, value string) wopt    { return wopt{key: key, enum: value} }
func flagOpt(key string) wopt        { return wopt{key: key, flag: true} }
func numOpt(key string, v Expr) wopt { return wopt{key: key, kind: kNum, v: v} }
func refOpt(key string, k argKind, v Expr) wopt {
	return wopt{key: key, kind: k, v: v}
}
func target(key string, k argKind, v Expr) wopt {
	return wopt{key: key, tkey: targetKey[k], kind: k, v: v}
}

var percent = flagOpt("percent")

// arithmetic on converted expressions, folded when static.
func (c *itemConv) neg(x Expr) Expr {
	e, _ := c.binary("-", &Num{Value: 0}, x)
	return e
}

func (c *itemConv) times(x Expr, k int64) Expr {
	e, _ := c.binary("*", x, &Num{Value: k})
	return e
}

// bonusName returns the rAthena bonus type of a bonus command's first
// argument, in lower case, such as bstr.
func (c *itemConv) bonusName(e Expr) (string, error) {
	switch e := e.(type) {
	case *Const:
		return strings.ToLower(e.Name), nil
	case *Num:
		if n, ok := c.conv.bonusByValue[e.Value]; ok {
			return n, nil
		}
	}
	return "", unsupported("bonus type", "not a bonus constant")
}

// mapBonus maps one rAthena bonus command to bonuses.add calls, following
// the table of proposal 0002a. n is 1 for bonus, 2 for bonus2, and so on.
func (c *itemConv) mapBonus(n int, args []Expr) ([]*wcall, error) {
	if len(args) == 0 {
		return nil, unsupported(fmt.Sprintf("bonus%d", n), "no bonus type")
	}
	typ, err := c.bonusName(args[0])
	if err != nil {
		return nil, err
	}
	a := args[1:]
	// Flags may leave out their value, as bonus bUnbreakableWeapon;.
	if name, ok := simpleFlags[typ]; ok && n == 1 && len(a) <= 1 {
		return []*wcall{call(name)}, nil
	}
	if len(a) != n {
		return nil, unsupported(fmt.Sprintf("bonus%d", n), "wrong number of arguments")
	}
	one := func(w *wcall) ([]*wcall, error) { return []*wcall{w}, nil }
	key := fmt.Sprintf("%d:%s", n, typ)
	if n == 1 {
		if name, ok := simpleStats[typ]; ok {
			return one(call(name, num(a[0])))
		}
		if name, ok := simplePercent[typ]; ok {
			return one(call(name, num(a[0])).with(percent))
		}
	}
	physical, magic, misc := enum("kind", "physical"), enum("kind", "magic"), enum("kind", "misc")
	melee, ranged := enum("range", "melee"), enum("range", "ranged")
	switch key {
	case "1:bagivit":
		return []*wcall{call("agi", num(a[0])), call("vit", num(a[0]))}, nil
	case "1:bagidexstr":
		return []*wcall{call("str", num(a[0])), call("agi", num(a[0])), call("dex", num(a[0]))}, nil
	case "1:bnoregen":
		switch v, _ := static(a[0]); v {
		case 1:
			return one(call("no_hp_recovery"))
		case 2:
			return one(call("no_sp_recovery"))
		}
		return nil, unsupported("bNoRegen", "a value other than 1 or 2")
	case "1:bcriticallong":
		return one(call("crit", num(a[0])).with(physical, ranged, enum("attack", "normal")))
	case "1:bcriticaldef":
		return one(call("crit_chance_taken", num(c.neg(a[0]))))
	case "1:bperfecthitrate":
		return one(call("perfect_hit_chance", num(c.times(a[0], 100))).with(enum("stacking", "highest")))
	case "1:bperfecthitaddrate":
		return one(call("perfect_hit_chance", num(c.times(a[0], 100))))
	case "1:bdoublerate":
		return one(call("double_attack_chance", num(c.times(a[0], 100))).with(enum("stacking", "highest")))
	case "1:bdoubleaddrate":
		return one(call("double_attack_chance", num(c.times(a[0], 100))))
	case "1:bspeedrate":
		return one(call("move_speed", num(a[0])).with(percent, enum("stacking", "highest")))
	case "1:bspeedaddrate":
		return one(call("move_speed", num(a[0])).with(percent))
	case "1:bfixedcastrate":
		return one(call("fixed_cast_time", num(a[0])).with(percent, enum("stacking", "lowest")))
	case "1:bshortatkrate":
		return one(call("damage", num(a[0])).with(physical, melee))
	case "1:blongatkrate":
		return one(call("damage", num(a[0])).with(physical, ranged))
	case "1:bignoredefrace", "1:bignoredefele", "1:bignoredefclass",
		"1:bignoremdefrace", "1:bignoremdefele",
		"1:bdefratioatkrace", "1:bdefratioatkele", "1:bdefratioatkclass":
		name := "ignore_def"
		if strings.HasPrefix(typ, "bignoremdef") {
			name = "ignore_mdef"
		} else if strings.HasPrefix(typ, "bdefratio") {
			name = "def_as_damage"
		}
		k := kRace
		switch {
		case strings.HasSuffix(typ, "ele"):
			k = kElement
		case strings.HasSuffix(typ, "class"):
			k = kClass
		}
		return one(call(name).with(target("against", k, a[0])))
	case "1:bnearatkdef":
		return one(call("damage_taken", num(c.neg(a[0]))).with(physical, melee))
	case "1:blongatkdef":
		return one(call("damage_taken", num(c.neg(a[0]))).with(physical, ranged))
	case "1:bmagicatkdef":
		return one(call("damage_taken", num(c.neg(a[0]))).with(magic))
	case "1:bmiscatkdef":
		return one(call("damage_taken", num(c.neg(a[0]))).with(misc))
	case "1:bcritdefrate":
		return one(call("crit_damage_taken", num(c.neg(a[0]))))
	case "1:bnoweapondamage":
		return one(call("immunity", num(a[0])).with(physical))
	case "1:bnomagicdamage":
		return one(call("immunity", num(a[0])).with(magic))
	case "1:bnomiscdamage":
		return one(call("immunity", num(a[0])).with(misc))
	case "1:babsorbdmgmaxhp2", "1:babsorbdmgmaxhp":
		return one(call("damage_taken_cap", num(a[0])).with(enum("stacking", "highest")))
	case "1:bshortweapondamagereturn":
		return one(call("damage_reflected", num(a[0])).with(physical, melee))
	case "1:blongweapondamagereturn":
		return one(call("damage_reflected", num(a[0])).with(physical, ranged))
	case "1:bmagicdamagereturn":
		return one(call("reflect_magic").with(numOpt("chance", c.times(a[0], 100)), enum("on", "hit_taken")))
	case "1:breducedamagereturn":
		return one(call("reflected_damage_taken", num(c.neg(a[0]))))
	case "1:batkele":
		return one(call("attack_element", of(kElement, a[0])))
	case "1:bdefele":
		return one(call("defense_element", of(kElement, a[0])))
	case "1:bbreakweaponrate":
		return one(call("break_weapon").with(numOpt("chance", a[0])))
	case "1:bbreakarmorrate":
		return one(call("break_armor").with(numOpt("chance", a[0])))
	case "1:bclasschange":
		return one(call("transform_monster").with(numOpt("chance", a[0]), physical))
	case "1:bhpgainvalue":
		return one(call("hp_per_kill", num(a[0])).with(physical, melee))
	case "1:blonghpgainvalue":
		return one(call("hp_per_kill", num(a[0])).with(physical, ranged))
	case "1:bmagichpgainvalue":
		return one(call("hp_per_kill", num(a[0])).with(magic))
	case "1:bspgainvalue":
		return one(call("sp_per_kill", num(a[0])).with(physical, melee))
	case "1:blongspgainvalue":
		return one(call("sp_per_kill", num(a[0])).with(physical, ranged))
	case "1:bmagicspgainvalue":
		return one(call("sp_per_kill", num(a[0])).with(magic))
	case "1:bunbreakable":
		return one(call("equipment_break_chance", num(c.neg(a[0]))))
	case "1:bsplashrange":
		return one(call("splash_range", num(a[0])).with(enum("stacking", "highest")))
	case "1:bfame":
		return nil, unsupported("bFame", "not a bonus")

	case "2:bhpregenrate", "2:bspregenrate":
		return one(call(map[string]string{"bhpregenrate": "hp_gain_over_time", "bspregenrate": "sp_gain_over_time"}[typ], num(a[0]), num(a[1])))
	case "2:bregenpercenthp", "2:bregenpercentsp":
		return one(call(map[string]string{"bregenpercenthp": "hp_gain_over_time", "bregenpercentsp": "sp_gain_over_time"}[typ], num(a[0]), num(a[1])).with(percent))
	case "2:bhplossrate":
		return one(call("hp_loss_over_time", num(a[0]), num(a[1])))
	case "2:bsplossrate":
		return one(call("sp_loss_over_time", num(a[0]), num(a[1])))
	case "2:bweaponatk":
		return one(call("atk", num(a[1])).with(refOpt("weapon_type", kWeaponType, a[0])))
	case "2:bcriticaladdrace":
		return one(call("crit", num(a[1])).with(target("against", kRace, a[0])))
	case "2:bskillvariablecast":
		return one(call("variable_cast_time", num(a[1])).with(refOpt("skill", kSkill, a[0])))
	case "2:bvariablecastrate", "2:bcastrate":
		return one(call("variable_cast_time", num(a[1])).with(percent, refOpt("skill", kSkill, a[0])))
	case "2:bskillfixedcast":
		return one(call("fixed_cast_time", num(a[1])).with(refOpt("skill", kSkill, a[0])))
	case "2:bfixedcastrate":
		return one(call("fixed_cast_time", num(a[1])).with(percent, refOpt("skill", kSkill, a[0])))
	case "2:bskilldelay":
		return one(call("after_cast_delay", num(a[1])).with(refOpt("skill", kSkill, a[0])))
	case "2:bskillcooldown":
		return one(call("cooldown", num(a[1])).with(refOpt("skill", kSkill, a[0])))
	case "2:bskillusesp":
		return one(call("sp_cost", num(c.neg(a[1]))).with(refOpt("skill", kSkill, a[0])))
	case "2:bskillusesprate":
		return one(call("sp_cost", num(c.neg(a[1]))).with(percent, refOpt("skill", kSkill, a[0])))
	case "2:baddrace":
		return one(call("damage", num(a[1])).with(physical, target("against", kRace, a[0])))
	case "2:bmagicaddrace":
		return one(call("damage", num(a[1])).with(magic, target("against", kRace, a[0])))
	case "2:baddclass":
		return one(call("damage", num(a[1])).with(physical, target("against", kClass, a[0])))
	case "2:bmagicaddclass":
		return one(call("damage", num(a[1])).with(magic, target("against", kClass, a[0])))
	case "2:baddsize":
		return one(call("damage", num(a[1])).with(physical, target("against", kSize, a[0])))
	case "2:bmagicaddsize":
		return one(call("damage", num(a[1])).with(magic, target("against", kSize, a[0])))
	case "2:baddele":
		return one(call("damage", num(a[1])).with(physical, target("against", kElement, a[0])))
	case "2:bmagicaddele":
		return one(call("damage", num(a[1])).with(magic, target("against", kElement, a[0])))
	case "2:baddrace2":
		return one(call("damage", num(a[1])).with(physical, target("against", kMonsterGroup, a[0])))
	case "2:bmagicaddrace2":
		return one(call("damage", num(a[1])).with(magic, target("against", kMonsterGroup, a[0])))
	case "2:badddamageclass":
		return one(call("damage", num(a[1])).with(physical, target("against", kMonster, a[0])))
	case "2:baddmagicdamageclass":
		return one(call("damage", num(a[1])).with(magic, target("against", kMonster, a[0])))
	case "2:bmagicatkele":
		return one(call("damage", num(a[1])).with(magic, refOpt("element", kElement, a[0])))
	case "2:bweapondamagerate":
		return one(call("damage", num(a[1])).with(physical, refOpt("weapon_type", kWeaponType, a[0])))
	case "2:bskillatk":
		return one(call("damage", num(a[1])).with(refOpt("skill", kSkill, a[0])))
	case "2:bignoredefracerate":
		return one(call("ignore_def", num(a[1])).with(percent, target("against", kRace, a[0])))
	case "2:bignoredefclassrate":
		return one(call("ignore_def", num(a[1])).with(percent, target("against", kClass, a[0])))
	case "2:bignoremdefracerate":
		return one(call("ignore_mdef", num(a[1])).with(percent, target("against", kRace, a[0])))
	case "2:bignoremdefclassrate":
		return one(call("ignore_mdef", num(a[1])).with(percent, target("against", kClass, a[0])))
	case "2:bignoremdefrace2rate":
		return one(call("ignore_mdef", num(a[1])).with(percent, target("against", kMonsterGroup, a[0])))
	case "2:bignoreresracerate":
		return one(call("ignore_res", num(a[1])).with(percent, target("against", kRace, a[0])))
	case "2:bignoremresracerate":
		return one(call("ignore_mres", num(a[1])).with(percent, target("against", kRace, a[0])))
	case "2:bsubrace":
		return one(call("damage_taken", num(c.neg(a[1]))).with(target("from", kRace, a[0])))
	case "2:bsubclass":
		return one(call("damage_taken", num(c.neg(a[1]))).with(target("from", kClass, a[0])))
	case "2:bsubsize":
		return one(call("damage_taken", num(c.neg(a[1]))).with(target("from", kSize, a[0])))
	case "2:bweaponsubsize":
		return one(call("damage_taken", num(c.neg(a[1]))).with(physical, target("from", kSize, a[0])))
	case "2:bmagicsubsize":
		return one(call("damage_taken", num(c.neg(a[1]))).with(magic, target("from", kSize, a[0])))
	case "2:bsubele":
		return one(call("damage_taken", num(c.neg(a[1]))).with(refOpt("element", kElement, a[0])))
	case "2:bsubdefele":
		return one(call("damage_taken", num(c.neg(a[1]))).with(physical, target("from", kElement, a[0])))
	case "2:bmagicsubdefele":
		return one(call("damage_taken", num(c.neg(a[1]))).with(magic, target("from", kElement, a[0])))
	case "2:bsubrace2":
		return one(call("damage_taken", num(c.neg(a[1]))).with(target("from", kMonsterGroup, a[0])))
	case "2:badddefmonster":
		return one(call("damage_taken", num(c.neg(a[1]))).with(physical, target("from", kMonster, a[0])))
	case "2:baddmdefmonster":
		return one(call("damage_taken", num(c.neg(a[1]))).with(magic, target("from", kMonster, a[0])))
	case "2:bsubskill":
		return one(call("damage_taken", num(c.neg(a[1]))).with(refOpt("skill", kSkill, a[0])))
	case "2:baddeff":
		return one(call("cause_status", of(kStatus, a[0])).with(numOpt("chance", a[1])))
	case "2:baddeff2":
		return one(call("cause_status", of(kStatus, a[0])).with(numOpt("chance", a[1]), enum("target", "self")))
	case "2:baddeffwhenhit":
		return one(call("cause_status", of(kStatus, a[0])).with(numOpt("chance", a[1]), enum("on", "hit_taken"), physical))
	case "2:breseff":
		return one(call("status_resistance", of(kStatus, a[0]), num(a[1])))
	case "2:bcomarace":
		return one(call("coma").with(numOpt("chance", a[1]), target("against", kRace, a[0])))
	case "2:bcomaclass":
		return one(call("coma").with(numOpt("chance", a[1]), target("against", kClass, a[0])))
	case "2:bweaponcomarace":
		return one(call("coma").with(numOpt("chance", a[1]), enum("attack", "normal"), target("against", kRace, a[0])))
	case "2:bweaponcomaclass":
		return one(call("coma").with(numOpt("chance", a[1]), enum("attack", "normal"), target("against", kClass, a[0])))
	case "2:bweaponcomaele":
		return one(call("coma").with(numOpt("chance", a[1]), enum("attack", "normal"), target("against", kElement, a[0])))
	case "2:bhpdrainrate":
		return one(call("drain_hp", num(a[1])).with(numOpt("chance", c.times(a[0], 10))))
	case "2:bspdrainrate":
		return one(call("drain_sp", num(a[1])).with(numOpt("chance", c.times(a[0], 10))))
	case "2:bhpvanishrate":
		return one(call("remove_hp", num(a[1])).with(numOpt("chance", c.times(a[0], 10)), enum("attack", "normal")))
	case "2:bspvanishrate":
		return one(call("remove_sp", num(a[1])).with(numOpt("chance", c.times(a[0], 10)), enum("attack", "normal")))
	case "2:bhpdrainvaluerace":
		return one(call("hp_per_hit", num(a[1])).with(target("against", kRace, a[0])))
	case "2:bhpdrainvalueclass":
		return one(call("hp_per_hit", num(a[1])).with(target("against", kClass, a[0])))
	case "2:bspdrainvaluerace":
		return one(call("sp_per_hit", num(a[1])).with(target("against", kRace, a[0])))
	case "2:bspdrainvalueclass":
		return one(call("sp_per_hit", num(a[1])).with(target("against", kClass, a[0])))
	case "2:bspgainrace":
		return one(call("sp_per_kill", num(a[1])).with(physical, melee, target("against", kRace, a[0])))
	case "2:bexpaddrace":
		return one(call("experience", num(a[1])).with(target("from", kRace, a[0])))
	case "2:bexpaddclass":
		return one(call("experience", num(a[1])).with(target("from", kClass, a[0])))
	case "2:bdropaddrace":
		return one(call("drop_rate", num(a[1])).with(target("from", kRace, a[0])))
	case "2:bdropaddclass":
		return one(call("drop_rate", num(a[1])).with(target("from", kClass, a[0])))
	case "2:baddmonsterdropitem":
		return one(call("extra_drop").with(refOpt("item", kItem, a[0]), numOpt("chance", a[1])))
	case "2:baddmonsterdropitemgroup":
		return one(call("extra_drop").with(refOpt("item_group", kItemGroup, a[0]), numOpt("chance", a[1])))
	case "2:bgetzenynum":
		return one(call("gain_zeny_on_kill", num(a[0])).with(numOpt("chance", c.times(a[1], 100)), enum("stacking", "highest")))
	case "2:baddgetzenynum":
		return one(call("gain_zeny_on_kill", num(a[0])).with(numOpt("chance", c.times(a[1], 100))))
	case "2:bskillheal":
		return one(call("healing_done", num(a[1])).with(refOpt("skill", kSkill, a[0])))
	case "2:bskillheal2":
		return one(call("healing_received", num(a[1])).with(refOpt("skill", kSkill, a[0])))
	case "2:badditemhealrate":
		return one(call("item_hp_recovery", num(a[1])).with(refOpt("item", kItem, a[0])))
	case "2:badditemgrouphealrate":
		return one(call("item_hp_recovery", num(a[1])).with(refOpt("item_group", kItemGroup, a[0])))
	case "2:badditemsphealrate":
		return one(call("item_sp_recovery", num(a[1])).with(refOpt("item", kItem, a[0])))
	case "2:badditemgroupsphealrate":
		return one(call("item_sp_recovery", num(a[1])).with(refOpt("item_group", kItemGroup, a[0])))
	case "2:baddskillblow":
		return one(call("knockback", num(a[1])).with(refOpt("skill", kSkill, a[0])))

	case "3:baddele":
		return c.splitBF(call("damage", num(a[1])).with(target("against", kElement, a[0])), a[2], bfDamage)
	case "3:bsubrace":
		return c.splitBF(call("damage_taken", num(c.neg(a[1]))).with(target("from", kRace, a[0])), a[2], bfDamage)
	case "3:bsubele":
		return c.splitBF(call("damage_taken", num(c.neg(a[1]))).with(refOpt("element", kElement, a[0])), a[2], bfDamage)
	case "3:baddeff":
		return c.splitATF(call("cause_status", of(kStatus, a[0])).with(numOpt("chance", a[1])), a[2], false)
	case "3:baddeffwhenhit":
		return c.splitATF(call("cause_status", of(kStatus, a[0])).with(numOpt("chance", a[1]), enum("on", "hit_taken")), a[2], true)
	case "3:baddeffonskill":
		return one(call("cause_status", of(kStatus, a[1])).with(numOpt("chance", a[2]), refOpt("skill", kSkill, a[0])))
	case "3:bstatenorecoverrace":
		return one(call("block_healing").with(numOpt("chance", a[1]), enum("attack", "normal"), target("against", kRace, a[0]), numOpt("duration", a[2])))
	case "3:bautospell":
		return one(call("cast", of(kSkill, a[0]), of(kLevel, a[1])).with(numOpt("chance", c.times(a[2], 10))))
	case "3:bautospellwhenhit":
		return one(call("cast", of(kSkill, a[0]), of(kLevel, a[1])).with(numOpt("chance", c.times(a[2], 10)), enum("on", "hit_taken")))
	case "3:bhpvanishrate":
		return c.splitBF(call("remove_hp", num(a[1])).with(numOpt("chance", c.times(a[0], 10))), a[2], bfDamage)
	case "3:bspvanishrate":
		return c.splitBF(call("remove_sp", num(a[1])).with(numOpt("chance", c.times(a[0], 10))), a[2], bfDamage)
	case "3:bhpvanishracerate":
		return one(call("deal_max_hp", num(a[2])).with(numOpt("chance", c.times(a[1], 10)), physical, target("against", kRace, a[0])))
	case "3:bspvanishracerate":
		return one(call("deal_max_sp", num(a[2])).with(numOpt("chance", c.times(a[1], 10)), physical, target("against", kRace, a[0])))
	case "3:baddmonsterdropitem":
		return one(call("extra_drop").with(refOpt("item", kItem, a[0]), numOpt("chance", a[2]), target("from", kRace, a[1])))
	case "3:baddclassdropitem":
		return one(call("extra_drop").with(refOpt("item", kItem, a[0]), numOpt("chance", a[2]), target("from", kClass, a[1])))
	case "3:baddmonsteriddropitem":
		return one(call("extra_drop").with(refOpt("item", kItem, a[0]), numOpt("chance", a[2]), target("from", kMonster, a[1])))
	case "3:baddmonsterdropitemgroup":
		return one(call("extra_drop").with(refOpt("item_group", kItemGroup, a[0]), numOpt("chance", a[2]), target("from", kRace, a[1])))
	case "3:baddclassdropitemgroup":
		return one(call("extra_drop").with(refOpt("item_group", kItemGroup, a[0]), numOpt("chance", a[2]), target("from", kClass, a[1])))

	case "4:bsetdefrace", "4:bsetmdefrace":
		name := map[string]string{"bsetdefrace": "set_def", "bsetmdefrace": "set_mdef"}[typ]
		return one(call(name, num(a[3])).with(numOpt("chance", a[1]), target("against", kRace, a[0]), enum("attack", "normal"), numOpt("duration", a[2])))
	case "4:baddeff":
		return c.splitATF(call("cause_status", of(kStatus, a[0])).with(numOpt("chance", a[1]), numOpt("duration", a[3])), a[2], false)
	case "4:baddeffwhenhit":
		return c.splitATF(call("cause_status", of(kStatus, a[0])).with(numOpt("chance", a[1]), enum("on", "hit_taken"), numOpt("duration", a[3])), a[2], true)
	case "4:baddeffonskill":
		return c.splitTarget(call("cause_status", of(kStatus, a[1])).with(numOpt("chance", a[2]), refOpt("skill", kSkill, a[0])), a[3])
	case "4:bautospell", "4:bautospellwhenhit":
		w := call("cast", of(kSkill, a[0]), of(kLevel, a[1])).with(numOpt("chance", c.times(a[2], 10)))
		if typ == "bautospellwhenhit" {
			w.with(enum("on", "hit_taken"))
		}
		return c.autospellFlag(w, a[3])
	case "4:bautospellonskill":
		return one(call("cast", of(kSkill, a[1]), of(kLevel, a[2])).with(numOpt("chance", c.times(a[3], 10)), enum("on", "skill_used"), refOpt("skill", kSkill, a[0])))

	case "5:baddeffonskill":
		return c.splitTarget(call("cause_status", of(kStatus, a[1])).with(numOpt("chance", a[2]), refOpt("skill", kSkill, a[0]), numOpt("duration", a[4])), a[3])
	case "5:bautospell", "5:bautospellwhenhit":
		w := call("cast", of(kSkill, a[0]), of(kLevel, a[1])).with(numOpt("chance", c.times(a[2], 10)))
		if typ == "bautospellwhenhit" {
			w.with(enum("on", "hit_taken"))
		}
		calls, err := c.splitBF(w, a[3], bfAutospell)
		if err != nil {
			return nil, err
		}
		var out []*wcall
		for _, w := range calls {
			more, err := c.autospellFlag(w, a[4])
			if err != nil {
				return nil, err
			}
			out = append(out, more...)
		}
		return out, nil
	case "5:bautospellonskill":
		return c.autospellFlag(call("cast", of(kSkill, a[1]), of(kLevel, a[2])).with(numOpt("chance", c.times(a[3], 10)), enum("on", "skill_used"), refOpt("skill", kSkill, a[0])), a[4])
	}
	return nil, unsupported(fmt.Sprintf("bonus%s %s", map[int]string{1: "", 2: "2", 3: "3", 4: "4", 5: "5"}[n], typ), "not in the 0002a table")
}

// simpleStats are bonus types that map to a name with a flat amount.
var simpleStats = map[string]string{
	"bstr": "str", "bagi": "agi", "bvit": "vit", "bint": "int", "bdex": "dex", "bluk": "luk", "ballstats": "all_stats",
	"bpow": "pow", "bsta": "sta", "bwis": "wis", "bspl": "spl", "bcon": "con", "bcrt": "crt", "balltraitstats": "all_trait_stats",
	"bpatk": "patk", "bsmatk": "smatk", "bres": "res", "bmres": "mres", "bhplus": "hplus", "bcrate": "crate",
	"bmaxhp": "max_hp", "bmaxsp": "max_sp", "bmaxap": "max_ap",
	"bbaseatk": "atk", "batk": "weapon_atk", "batk2": "refine_atk", "bmatk": "matk", "bmatk2": "hidden_matk",
	"bdef": "def", "bdef2": "status_def", "bmdef": "mdef", "bmdef2": "status_mdef", "batkrange": "attack_range",
	"bhit": "hit", "bflee": "flee", "bflee2": "perfect_dodge", "bcritical": "crit",
	"baspd": "aspd", "bvariablecast": "variable_cast_time", "bfixedcast": "fixed_cast_time",
	"bcritatkrate": "crit_damage", "bnoncritatkrate": "non_crit_damage", "bskillratio": "skill_ratio",
	"bhpdrainvalue": "hp_per_hit", "bspdrainvalue": "sp_per_hit", "baddstealrate": "steal_chance",
	"bhealpower": "healing_done", "bhealpower2": "healing_received",
	"badditemhealrate": "item_hp_recovery", "badditemsphealrate": "item_sp_recovery",
	"bsplashaddrange": "splash_range", "baddmaxweight": "max_weight",
}

// simplePercent are bonus types that map to a name with percent = true.
var simplePercent = map[string]string{
	"bpatkrate": "patk", "bsmatkrate": "smatk", "bresrate": "res", "bmresrate": "mres", "bhplusrate": "hplus", "bcraterate": "crate",
	"bmaxhprate": "max_hp", "bmaxsprate": "max_sp", "bmaxaprate": "max_ap",
	"bhprecovrate": "natural_hp_recovery", "bsprecovrate": "natural_sp_recovery",
	"batkrate": "atk", "bweaponatkrate": "weapon_atk", "bmatkrate": "matk", "bweaponmatkrate": "weapon_matk",
	"bdefrate": "def", "bdef2rate": "status_def", "bmdefrate": "mdef", "bmdef2rate": "status_mdef",
	"bhitrate": "hit", "bfleerate": "flee", "bflee2rate": "perfect_dodge", "bcriticalrate": "crit",
	"baspdrate": "aspd", "bvariablecastrate": "variable_cast_time", "bcastrate": "variable_cast_time",
	"bdelayrate": "after_cast_delay", "busesprate": "sp_cost",
}

// simpleFlags are bonus types that map to a flag.
var simpleFlags = map[string]string{
	"bnocastcancel": "uninterruptible_cast", "bnocastcancel2": "uninterruptible_cast_everywhere", "bnowalkdelay": "always_endure",
	"bnosizefix": "ignore_size_penalty", "bnogemstone": "no_gemstone_cost", "bnomadofuel": "no_magic_gear_fuel_cost",
	"brestartfullrecover": "full_recovery_on_revive",
	"bunbreakableweapon":  "unbreakable_weapon", "bunbreakablearmor": "unbreakable_armor", "bunbreakablehelm": "unbreakable_helm",
	"bunbreakableshield": "unbreakable_shield", "bunbreakablegarment": "unbreakable_garment", "bunbreakableshoes": "unbreakable_shoes",
	"bunstripableweapon": "unstrippable_weapon", "bunstripablearmor": "unstrippable_armor", "bunstripable": "unstrippable_armor",
	"bunstripablehelm": "unstrippable_helm", "bunstripableshield": "unstrippable_shield",
	"bnoknockback": "no_knockback", "bintravision": "see_hidden", "bperfecthide": "hidden_from_detectors",
}

// rAthena's BF_ and ATF_ flag bits.
const (
	bfWeapon = 0x0001
	bfMagic  = 0x0002
	bfMisc   = 0x0004
	bfShort  = 0x0010
	bfLong   = 0x0040
	bfSkill  = 0x0100
	bfNormal = 0x0200

	atfSelf   = 0x01
	atfTarget = 0x02
	atfShort  = 0x04
	atfLong   = 0x08
	atfWeapon = 0x10
	atfMagic  = 0x20
	atfMisc   = 0x40
)

// bfUse says what an omitted option means for a name.
type bfUse int

const (
	// bfDamage: an omitted option means every value, as on damage.
	bfDamage bfUse = iota
	// bfAutospell: an omitted option means rAthena's autospell default:
	// physical, both ranges, normal attacks.
	bfAutospell
)

// splitBF applies a BF_ flag as options, with rAthena's defaults
// (pc_bonus_addele, pc_bonus_autospell). An option has one value, so a
// flag with two kinds, or with normal attacks and skills where the
// omitted value means one of them, becomes one call per value. Each
// hit has one kind and one attack type, so the calls never both apply
// to a hit, and the result is the same as rAthena's one entry.
func (c *itemConv) splitBF(w *wcall, flagExpr Expr, use bfUse) ([]*wcall, error) {
	flag, ok := static(flagExpr)
	if !ok {
		return nil, unsupported("BF_ flag", "computed")
	}
	if flag&(bfShort|bfLong) == 0 {
		flag |= bfShort | bfLong
	}
	if flag&(bfWeapon|bfMagic|bfMisc) == 0 {
		flag |= bfWeapon
	}
	if flag&(bfSkill|bfNormal) == 0 {
		if flag&(bfMagic|bfMisc) != 0 {
			flag |= bfSkill
		}
		if flag&bfWeapon != 0 {
			flag |= bfNormal
			if use == bfDamage {
				flag |= bfSkill
			}
		}
	}
	var kinds []string
	for _, k := range []struct {
		bit  int64
		name string
	}{{bfWeapon, "physical"}, {bfMagic, "magic"}, {bfMisc, "misc"}} {
		if flag&k.bit != 0 {
			kinds = append(kinds, k.name)
		}
	}
	allKinds := len(kinds) == 3
	if allKinds && use == bfDamage {
		kinds = []string{""}
	}
	attacks := []string{""}
	switch flag & (bfNormal | bfSkill) {
	case bfNormal:
		attacks = []string{"normal"}
	case bfSkill:
		attacks = []string{"skill"}
	case bfNormal | bfSkill:
		if use == bfAutospell {
			attacks = []string{"normal", "skill"}
		}
	}
	rng := ""
	switch flag & (bfShort | bfLong) {
	case bfShort:
		rng = "melee"
	case bfLong:
		rng = "ranged"
	}
	var out []*wcall
	for _, k := range kinds {
		for _, at := range attacks {
			// Magic and misc hits are always skills.
			if (k == "magic" || k == "misc") && at == "normal" {
				continue
			}
			n := &wcall{name: w.name, args: w.args, opts: append([]wopt(nil), w.opts...)}
			if k != "" && (use != bfAutospell || k != "physical") {
				n.opts = append(n.opts, enum("kind", k))
			}
			if rng != "" {
				if k != "physical" {
					return nil, unsupported("BF_ flag", "a range on magic or misc hits")
				}
				if use == bfAutospell {
					n.opts = append(n.opts, enum("kind", "physical"))
				}
				n.opts = append(n.opts, enum("range", rng))
			}
			if at != "" && (use != bfAutospell || at != "normal") {
				n.opts = append(n.opts, enum("attack", at))
			}
			out = append(out, n)
		}
	}
	return out, nil
}

// splitATF applies an ATF_ flag of bAddEff, with rAthena's defaults
// (pc_bonus_addeff): both ranges, the target, and weapon hits.
func (c *itemConv) splitATF(w *wcall, flagExpr Expr, whenHit bool) ([]*wcall, error) {
	flag, ok := static(flagExpr)
	if !ok {
		return nil, unsupported("ATF_ flag", "computed")
	}
	if flag&(atfShort|atfLong) == 0 {
		flag |= atfShort | atfLong
	}
	if flag&(atfSelf|atfTarget) == 0 {
		flag |= atfTarget
	}
	if flag&(atfWeapon|atfMagic|atfMisc) == 0 {
		flag |= atfWeapon
	}
	var targets, kinds []string
	if flag&atfTarget != 0 {
		targets = append(targets, "")
	}
	if flag&atfSelf != 0 {
		targets = append(targets, "self")
	}
	for _, k := range []struct {
		bit  int64
		name string
	}{{atfWeapon, "physical"}, {atfMagic, "magic"}, {atfMisc, "misc"}} {
		if flag&k.bit != 0 {
			kinds = append(kinds, k.name)
		}
	}
	rng := ""
	switch flag & (atfShort | atfLong) {
	case atfShort:
		rng = "melee"
	case atfLong:
		rng = "ranged"
	}
	var out []*wcall
	for _, t := range targets {
		for _, k := range kinds {
			n := &wcall{name: w.name, args: w.args, opts: append([]wopt(nil), w.opts...)}
			if t != "" {
				n.opts = append(n.opts, enum("target", t))
			}
			if k != "physical" || whenHit || rng != "" {
				n.opts = append(n.opts, enum("kind", k))
			}
			if rng != "" {
				if k != "physical" {
					return nil, unsupported("ATF_ flag", "a range on magic or misc hits")
				}
				n.opts = append(n.opts, enum("range", rng))
			}
			out = append(out, n)
		}
	}
	return out, nil
}

// splitTarget applies the ATF_SELF and ATF_TARGET bits of bAddEffOnSkill.
func (c *itemConv) splitTarget(w *wcall, flagExpr Expr) ([]*wcall, error) {
	flag, ok := static(flagExpr)
	if !ok {
		return nil, unsupported("ATF_ flag", "computed")
	}
	if flag&(atfSelf|atfTarget) == 0 {
		flag |= atfTarget
	}
	var out []*wcall
	if flag&atfTarget != 0 {
		out = append(out, w)
	}
	if flag&atfSelf != 0 {
		n := &wcall{name: w.name, args: w.args, opts: append(append([]wopt(nil), w.opts...), enum("target", "self"))}
		out = append(out, n)
	}
	return out, nil
}

// autospellFlag applies autospell's i flag: 1 casts on the wearer, and 2
// picks a random level up to the given one.
func (c *itemConv) autospellFlag(w *wcall, flagExpr Expr) ([]*wcall, error) {
	flag, ok := static(flagExpr)
	if !ok {
		return nil, unsupported("autospell flag", "computed")
	}
	if flag&^3 != 0 {
		return nil, unsupported("autospell flag", "bits other than 1 and 2")
	}
	if flag&1 != 0 {
		w.with(enum("target", "self"))
	}
	if flag&2 != 0 {
		w.with(flagOpt("random_level"))
	}
	return []*wcall{w}, nil
}
