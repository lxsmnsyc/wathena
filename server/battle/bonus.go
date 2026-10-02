package battle

// Bonuses from equipment that attacks read, as rAthena's weapon_data and
// the s_indexed_bonus and s_bonus fields of map_session_data. server/status
// fills them from item bonuses (pc_bonus). Maps are by race, element,
// size, class, monster group, monster ID or weapon type. A nil map reads
// as zeros.

// Hand is one hand's weapon bonuses, as rAthena's weapon_data.
type Hand struct {
	// AddRace, AddEle, AddSize, AddClass and AddRace2 are bAddRace,
	// bAddEle, bAddSize, bAddClass and bAddRace2: percent more damage
	// against a target by race, element, size, class and monster group.
	AddRace, AddEle, AddSize, AddClass, AddRace2 map[int]int
	// AddEle2 is bonus3 bAddEle: by the target's element, for hits that
	// match a battle flag.
	AddEle2 []FlaggedRate
	// AddDamage is bAddDamageClass: by monster ID, or job ID for a player
	// target.
	AddDamage map[int]int
	// IgnoreDefEle, IgnoreDefRace and IgnoreDefClass are bits by element,
	// race and class: bIgnoreDefEle and its kin, which make attacks
	// ignore DEF.
	IgnoreDefEle, IgnoreDefRace, IgnoreDefClass uint32
	// DefRatioEle, DefRatioRace and DefRatioClass are bDefRatioAtkEle and
	// its kin: bits by element, race and class.
	DefRatioEle, DefRatioRace, DefRatioClass uint32
	// Overrefine is the over-refine bonus maximum, randombonus_max / 100
	// of the refine table.
	Overrefine int
}

// FlaggedRate is an entry of a bonus list with battle flags, as
// rAthena's s_addele2 and s_addrace2: a key (element or race), a rate in
// percent, and the BF_ flags of the hits it applies to.
type FlaggedRate struct {
	Key, Rate, Flag int
}

// Arrow holds the bonuses of the equipped ammunition, as
// indexed_bonus.arrow_add* and bonus.arrow_hit and arrow_cri.
type Arrow struct {
	AddRace, AddEle, AddSize, AddClass map[int]int
	Hit, Cri                           int
}

// add adds n to m[k], making the map when it is nil.
func add(m *map[int]int, k, n int) {
	if *m == nil {
		*m = map[int]int{}
	}
	(*m)[k] += n
}

// Add adds a value to a bonus map.
func Add(m *map[int]int, k, n int) { add(m, k, n) }

// matchesFlag is rAthena's check of a flagged bonus against a hit's
// flags: the hit must match a weapon bit, a range bit and a skill bit.
func matchesFlag(entry, hit int) bool {
	f := entry & hit
	return f&(bfWeapon|bfMagic|bfMisc) != 0 && f&(bfShort|bfLong) != 0 && f&(bfSkill|bfNormal) != 0
}

// sumFlagged sums the rates of the entries for a key, or for every key
// (all), that match the hit's flags.
func sumFlagged(list []FlaggedRate, key, all, hit int) int {
	n := 0
	for _, e := range list {
		if e.Key != all && e.Key != key {
			continue
		}
		if !matchesFlag(e.Flag, hit) {
			continue
		}
		n += e.Rate
	}
	return n
}
