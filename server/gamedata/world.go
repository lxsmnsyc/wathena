package gamedata

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// Monster overlays (data/monsters.yml) and world events
// (data/events.yml). mods/README.md, "Monsters", lists the fields.
//
// The fields set mob_db's base values. The server computes the combat
// values from them, as rAthena's status_calc_mob (proposal 0002, review
// decision 22).

// monsterFields are the fields of a monster record.
var monsterFields = []string{
	"display_name", "sprite", "level", "hp", "sp", "attack", "magic_attack", "defense", "magic_defense",
	"resistance", "magic_resistance", "str", "agi", "vit", "int", "dex", "luk", "speed", "attack_range",
	"skill_range", "chase_range", "attack_delay", "attack_motion", "damage_motion", "element", "element_level",
	"race", "size", "class", "ai", "modes", "base_exp", "job_exp", "mvp_exp", "drops", "mvp_drops",
	"damage_taken", "skills", "script", "sounds",
}

// artBaseMonster is the monster a new monster with a sprite of its own
// art starts from (decision 0033).
const artBaseMonster = "PORING"

// MonsterSounds are the keys of a monster's sounds field: its sounds,
// as the client's monster sound table has them.
var MonsterSounds = []string{"attack", "damage", "die", "move"}

// Monster classes of class, as rAthena's e_aegis_monsterclass.
var monsterClasses = map[string]int{"normal": 0, "boss": 1, "guardian": 2, "battlefield": 4, "event": 5}

// MonsterModes are the names of modes, from rAthena's MD_ names.
var MonsterModes = mob.ModeNames

// monsterNames resolves the names of a skills list for mob.ParseSkills.
type monsterNames struct {
	skills   *skill.DB
	byName   map[string]uint32
	names    map[uint32]string
	statuses *constants.Module
	emotions *constants.Module
}

func (n *monsterNames) SkillID(name string) (int, bool) {
	if n.skills == nil {
		return 0, false
	}
	d := n.skills.ByName(name)
	if d == nil {
		return 0, false
	}
	return d.ID, true
}

func (n *monsterNames) SkillName(id int) (string, bool) {
	if n.skills == nil {
		return "", false
	}
	d := n.skills.Get(id)
	if d == nil {
		return "", false
	}
	return d.Name, true
}

func (n *monsterNames) Ground(id int) bool {
	if n.skills == nil {
		return false
	}
	d := n.skills.Get(id)
	return d != nil && d.Target == skill.TargetGround
}

func (n *monsterNames) MonsterID(name string) (uint32, bool) {
	id, ok := n.byName[name]
	return id, ok
}

func (n *monsterNames) MonsterName(id uint32) (string, bool) {
	s, ok := n.names[id]
	return s, ok
}

func (n *monsterNames) StatusID(name string) (int, bool) {
	if n.statuses == nil {
		return 0, false
	}
	v, ok := n.statuses.Values[name]
	return int(v), ok
}

func (n *monsterNames) StatusName(id int) (string, bool) {
	if n.statuses == nil {
		return "", false
	}
	s, ok := n.statuses.Names[int64(id)]
	return s, ok
}

func (n *monsterNames) EmotionID(name string) (int, bool) {
	if n.emotions == nil {
		return 0, false
	}
	v, ok := n.emotions.Values[name]
	return int(v), ok
}

func (n *monsterNames) EmotionName(id int) (string, bool) {
	if n.emotions == nil {
		return "", false
	}
	s, ok := n.emotions.Names[int64(id)]
	return s, ok
}

// monsterOverlay applies the overlays of data/monsters.yml, core's and
// the mods', over the core monsters. It returns every monster by ID, and
// every name, in upper case, by ID. items finds drops by item name,
// skills the skills of skills lists, and set the AI scripts. tables
// adjusts drops and scales exp, and may be nil.
func monsterOverlay(ctx context.Context, set *mod.Set, base map[uint32]Monster, consts *constants.Data,
	items map[string]uint32, skills *skill.DB, tables *progression.Tables, ids *mod.IDs) (map[uint32]Monster, map[string]int64, *mod.Overlay, error) {
	byName := map[string]uint32{}
	for id, m := range base {
		if m.AegisName != "" {
			byName[m.AegisName] = id
		}
	}
	o, err := set.Monsters(func(name string) bool { _, ok := byName[name]; return ok })
	if err != nil {
		return nil, nil, nil, err
	}
	out := maps.Clone(base)
	var errs []error
	// The first pass gives every new monster its ID, so a skills list
	// may summon a monster that a later record adds.
	type pending struct {
		r  *mod.Record
		id uint32
		m  Monster
	}
	var todo []pending
	for _, r := range o.Records {
		if core, ok := byName[r.Name]; ok {
			todo = append(todo, pending{r, core, out[core].clone()})
			continue
		}
		spriteName, _ := r.Fields["sprite"].(string)
		if mod.IsArtName(spriteName) {
			// A sprite of the mod's art: the stats start from Poring's.
			spriteName = artBaseMonster
		}
		from, ok := byName[spriteName]
		if !ok {
			errs = append(errs, fmt.Errorf("%s: monsters %s: a new monster needs sprite: the AegisName of a monster whose sprite it shows, such as PORING, or an art name, such as mod/example.town/sprite/mob/Ghost", r.Where, r.Name))
			continue
		}
		id, err := ids.ID(ctx, mod.MonsterIDs, r.Name, r.Mod)
		if err != nil {
			return nil, nil, nil, err
		}
		m := out[from].clone()
		m.AegisName, m.Name = r.Name, mod.Local(r.Name)
		if m.Def != nil {
			m.Def.ID, m.Def.AegisName = id, r.Name
			m.Def.BaseExp, m.Def.JobExp, m.Def.MvpExp = 0, 0, 0
		}
		// A new monster has its own drops and skills, and no race groups
		// of its own.
		m.drops, m.mvpDrops, m.Skills, m.Script = nil, nil, nil, ""
		if m.Status != nil {
			m.Status.Race2 = nil
		}
		byName[r.Name] = id
		todo = append(todo, pending{r, id, m})
	}
	if len(errs) > 0 {
		return nil, nil, nil, errors.Join(errs...)
	}
	names := &monsterNames{skills: skills, byName: byName, names: map[uint32]string{}}
	for n, id := range byName {
		names.names[id] = n
	}
	if consts != nil {
		mods := consts.Modules()
		names.statuses, names.emotions = mods["statuses"], mods["emotions"]
	}
	for _, p := range todo {
		m := p.m
		if err := applyMonster(&m, p.r, set, consts, items, names, tables); err != nil {
			errs = append(errs, fmt.Errorf("%s: monsters %s: %w", p.r.Where, p.r.Name, err))
			continue
		}
		out[p.id] = m
	}
	if len(errs) > 0 {
		return nil, nil, nil, errors.Join(errs...)
	}
	upper := make(map[string]int64, len(byName))
	for n, id := range byName {
		upper[strings.ToUpper(n)] = int64(id)
	}
	return out, upper, o, nil
}

// clone copies a monster, so an overlay does not change the original.
func (m Monster) clone() Monster {
	if m.Def != nil {
		d := *m.Def
		m.Def = &d
	}
	if m.Status != nil {
		st := *m.Status
		m.Status = &st
	}
	if m.Reward != nil {
		r := *m.Reward
		r.Drops = slices.Clone(r.Drops)
		m.Reward = &r
	}
	m.Skills = slices.Clone(m.Skills)
	return m
}

// applyMonster sets a record's fields on a monster, and computes its
// battle status again.
func applyMonster(m *Monster, r *mod.Record, set *mod.Set, consts *constants.Data, items map[string]uint32,
	names *monsterNames, tables *progression.Tables) error {
	f := r.Fields
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	for k := range f {
		if !slices.Contains(monsterFields, k) {
			bad("%q is not a field; monsters have %s", k, strings.Join(monsterFields, ", "))
		}
	}
	d := m.Def
	if d == nil {
		return errors.New("the monster has no data")
	}
	num := func(k string, lo, hi int, to *int) {
		v, ok := f[k]
		if !ok {
			return
		}
		n, ok := v.(int)
		if !ok || n < lo || n > hi {
			bad("%s must be a whole number from %d to %d", k, lo, hi)
			return
		}
		*to = n
	}
	num64 := func(k string, to *int64) {
		var n int
		if _, ok := f[k]; ok {
			num(k, 0, 1<<31-1, &n)
			*to = int64(n)
		}
	}
	constant := func(k, module string, to *int) {
		v, ok := f[k]
		if !ok {
			return
		}
		s, _ := v.(string)
		var mod *constants.Module
		if consts != nil {
			mod = consts.Modules()[module]
		}
		if mod == nil {
			bad("%s: the constants of wa.%s are not loaded", k, module)
			return
		}
		n, ok := mod.Values[s]
		if !ok {
			bad("%s: %q is not in wa.%s", k, s, module)
			return
		}
		*to = int(n)
	}
	if v, ok := f["display_name"]; ok {
		s, _ := v.(string)
		if s == "" || len(s) > 64 {
			bad("display_name must be 1 to 64 bytes")
		}
		m.Name = s
	}
	if v, ok := f["sprite"]; ok {
		s, _ := v.(string)
		from, ok := names.byName[s]
		switch {
		case mod.IsArtName(s):
			if err := mod.CheckArtName(s, "sprite"); err != nil {
				bad("sprite: %v", err)
			}
			m.Art.Sprite, m.Sprite = s, 0
		case !ok:
			bad("sprite: there is no monster %q", s)
		default:
			m.Sprite, m.Art.Sprite = from, ""
		}
	}
	if v, ok := f["sounds"]; ok {
		sounds, err := mod.ArtTable(v, MonsterSounds, "sound")
		if err != nil {
			bad("sounds: %v", err)
		}
		m.Art.Sounds = sounds
	}
	num("level", 1, 999, &d.Level)
	num("hp", 1, 1<<31-1, &d.HP)
	num("sp", 1, 1<<31-1, &d.SP)
	if v, ok := f["attack"]; ok {
		// mob_db's Attack: renewal's base attack, or pre-renewal's
		// minimum. Pre-renewal also takes [minimum, maximum] (Attack and
		// Attack2).
		switch v := v.(type) {
		case int:
			num("attack", 0, 1<<16-1, &d.Attack)
		case []any:
			lo, ok1 := listInt(v, 0)
			hi, ok2 := listInt(v, 1)
			switch {
			case m.renewal:
				bad("attack must be one number in renewal, and magic_attack sets the magic attack")
			case len(v) != 2 || !ok1 || !ok2 || lo < 0 || hi > 1<<16-1 || lo > hi:
				bad("attack must be a number, or [minimum, maximum] from 0 to 65535")
			default:
				d.Attack, d.Attack2 = lo, hi
			}
		default:
			bad("attack must be a whole number from 0 to 65535")
		}
	}
	if _, ok := f["magic_attack"]; ok {
		if !m.renewal {
			bad("magic_attack is mob_db's Attack2 of renewal; pre-renewal monsters get their magic attack from int")
		}
		num("magic_attack", 0, 1<<16-1, &d.Attack2)
	}
	defLo, defHi := -128, 127
	if m.renewal {
		defLo, defHi = -32768, 32767
	}
	num("defense", defLo, defHi, &d.Defense)
	num("magic_defense", defLo, defHi, &d.MagicDefense)
	num("resistance", 0, 1<<16-1, &d.Resistance)
	num("magic_resistance", 0, 1<<16-1, &d.MagicResistance)
	for _, st := range []struct {
		k  string
		to *int
	}{{"str", &d.Str}, {"agi", &d.Agi}, {"vit", &d.Vit}, {"int", &d.Int}, {"dex", &d.Dex}, {"luk", &d.Luk}} {
		num(st.k, 0, 1<<16-1, st.to)
	}
	num("speed", 20, 1000, &d.WalkSpeed)
	num("attack_range", 0, 14, &d.AttackRange)
	num("skill_range", 0, 30, &d.SkillRange)
	num("chase_range", 0, 30, &d.ChaseRange)
	num("attack_delay", 100, 8000, &d.AttackDelay)
	num("attack_motion", 1, 8000, &d.AttackMotion)
	num("damage_motion", 0, 10000, &d.DamageMotion)
	constant("element", "elements", &d.Element)
	num("element_level", 1, 4, &d.ElementLevel)
	constant("race", "races", &d.Race)
	constant("size", "sizes", &d.Size)
	num("damage_taken", 0, 100, &d.DamageTaken)
	if v, ok := f["class"]; ok {
		s, _ := v.(string)
		c, ok := monsterClasses[s]
		if !ok {
			bad("class: %q is not normal, boss, guardian, battlefield or event", s)
		}
		d.Class = c
	}
	if v, ok := f["ai"]; ok {
		n, ok := v.(int)
		if _, exists := mob.AITypes[n]; !ok || !exists {
			bad("ai must be one of rAthena's AI types, from 1 to 27: %s", aiTypeList())
		} else {
			d.AI = fmt.Sprintf("%02d", n)
		}
	}
	if v, ok := f["modes"]; ok {
		list, ok := v.([]any)
		if !ok {
			bad("modes must be a list, such as [aggressive, assist]")
		}
		var on []uint32
		for _, e := range list {
			s, _ := e.(string)
			bit, ok := mob.ModeNames[s]
			if !ok {
				bad("modes: %q is not a mode; the modes are %s", s, strings.Join(slices.Sorted(maps.Keys(mob.ModeNames)), ", "))
				continue
			}
			on = append(on, bit)
		}
		// The list adds to the modes of the AI type, as mob_db's Modes.
		m.modesOn, m.modesOff = on, nil
	}
	ai, _ := mob.AIModes(d.AI)
	d.Mode = mob.Modes(ai, m.modesOn, m.modesOff, d.Class)
	num64("base_exp", &d.BaseExp)
	num64("job_exp", &d.JobExp)
	num64("mvp_exp", &d.MvpExp)
	if v, ok := f["drops"]; ok {
		drops, err := readDrops(v, items, 20)
		if err != nil {
			bad("drops: %v", err)
		}
		m.drops = drops
	}
	if v, ok := f["mvp_drops"]; ok {
		drops, err := readDrops(v, items, 3)
		if err != nil {
			bad("mvp_drops: %v", err)
		}
		m.mvpDrops = drops
	}
	if v, ok := f["skills"]; ok {
		list, err := mob.ParseSkills(v, names)
		if err != nil {
			bad("skills: %v", err)
		}
		m.Skills = list
	}
	if v, ok := f["script"]; ok {
		path, _ := v.(string)
		owner := r.SetBy["script"]
		if path == "" || !set.HasFile(owner, path) {
			bad("script: %s does not exist in %s", path, owner)
		} else {
			m.Script = owner + ":" + path
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	race2 := m.Status.Race2
	st := mob.Calc(d, m.renewal)
	st.Race2 = race2
	m.Status = &st
	if m.Reward != nil && tables != nil {
		m.fillReward(tables)
	}
	return nil
}

func listInt(list []any, i int) (int, bool) {
	if i >= len(list) {
		return 0, false
	}
	n, ok := list[i].(int)
	return n, ok
}

// aiTypeList lists the AI types that exist.
func aiTypeList() string {
	var out []string
	for _, n := range slices.Sorted(maps.Keys(mob.AITypes)) {
		out = append(out, fmt.Sprintf("%02d", n))
	}
	return strings.Join(out, ", ")
}

// readDrops reads a drops list: { item, rate }, with the rate per
// 10,000, as mob_db.yml, and random_option_group, the group of random
// options the dropped copy rolls. LoadMods checks that the group exists.
func readDrops(v any, items map[string]uint32, most int) ([]progression.Drop, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, errors.New("want a list of { item: Name, rate: n }, with rates per 10,000")
	}
	var out []progression.Drop
	for i, e := range list {
		m, ok := e.(map[string]any)
		group, hasGroup := m["random_option_group"].(string)
		if !ok || len(m) != 2 && (len(m) != 3 || !hasGroup || group == "") {
			return nil, fmt.Errorf("drop %d: want { item: Name, rate: n }, and random_option_group when the copy rolls random options", i+1)
		}
		name, _ := m["item"].(string)
		id, ok := items[name]
		if !ok {
			return nil, fmt.Errorf("drop %d: there is no item %q", i+1, name)
		}
		rate, ok := m["rate"].(int)
		if !ok || rate < 1 || rate > 10000 {
			return nil, fmt.Errorf("drop %d: rate must be 1 to 10000", i+1)
		}
		out = append(out, progression.Drop{Item: id, Rate: rate, RandomOptionGroup: group})
	}
	if len(out) > most {
		return nil, fmt.Errorf("a monster has at most %d of these drops", most)
	}
	return out, nil
}

// Template is a spawn of one monster with its stats, for scripts:
// Amount is 1, and the area is anywhere.
func (m Monster) Template(id uint32) zone.Spawn {
	s := m.Spawn(id, m.Name, 0, 0)
	s.Sprite = m.Sprite
	return s
}

// worldEvents reads the mods' world events, and checks that each
// script exists. Typed events (review decision 31) name monsters, items,
// currencies and NPC sprites: names finds them.
func worldEvents(set *mod.Set, names eventNames) ([]worldevent.Def, *mod.Overlay, error) {
	o, err := set.Events()
	if err != nil {
		return nil, nil, err
	}
	var out []worldevent.Def
	var errs []error
	for _, r := range o.Records {
		d, err := worldevent.Parse(r)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: events %s: %w", r.Where, r.Name, err))
			continue
		}
		if d.Script != "" {
			owner, path, _ := strings.Cut(d.Script, ":")
			if !set.HasFile(owner, path) {
				errs = append(errs, fmt.Errorf("%s: events %s: script %s does not exist", r.Where, r.Name, d.Script))
				continue
			}
		}
		if d.Typed() {
			if err := names.resolve(set, r, &d); err != nil {
				errs = append(errs, fmt.Errorf("%s: events %s: %w", r.Where, r.Name, err))
				continue
			}
		}
		out = append(out, d)
	}
	return out, o, errors.Join(errs...)
}

// eventNames finds the names that typed events use.
type eventNames struct {
	// monster finds a monster that can spawn, by its AegisName or a
	// mod's full name.
	monster func(name string) (uint32, bool)
	// item finds an item by its AegisName or a mod's full name.
	item func(name string) (uint32, bool)
	// currency finds a currency for a mod, and reports the mod that owns
	// it.
	currency func(name, mod string) (owner string, ok bool)
	// sprite finds an NPC sprite.
	sprite func(name string) (uint32, bool)
}

// resolve fills a typed event's IDs, and checks the permissions its
// rewards need: each kind of reward needs the permission a script needs
// to give it.
func (n eventNames) resolve(set *mod.Set, r *mod.Record, d *worldevent.Def) error {
	var errs []string
	owner := r.Mod
	local := func(name string) []string {
		if strings.Contains(name, ":") || owner == mod.Core {
			return []string{name}
		}
		return []string{owner + ":" + name, name}
	}
	monster := func(name string) (uint32, bool) {
		if n.monster == nil {
			// Without monster data, as --mode none, no monster spawns:
			// the names are left for a server that has the data.
			return 0, true
		}
		for _, c := range local(name) {
			if id, ok := n.monster(c); ok {
				return id, true
			}
		}
		return 0, false
	}
	switch {
	case d.Boss != nil:
		for _, m := range d.Boss.Monsters {
			id, ok := monster(m)
			if !ok {
				errs = append(errs, fmt.Sprintf("monsters: %s is not a monster with stats", m))
				continue
			}
			if id != 0 {
				d.Boss.MonsterIDs = append(d.Boss.MonsterIDs, id)
			}
		}
	case d.Invasion != nil:
		for i := range d.Invasion.Waves {
			w := &d.Invasion.Waves[i]
			for j := range w.Monsters {
				id, ok := monster(w.Monsters[j].Monster)
				if !ok {
					errs = append(errs, fmt.Sprintf("waves: wave %d: %s is not a monster with stats", i+1, w.Monsters[j].Monster))
					continue
				}
				w.Monsters[j].ID = id
			}
		}
	case d.Treasure != nil:
		id, ok := monster(d.Treasure.Chest)
		if !ok {
			errs = append(errs, fmt.Sprintf("chest: %s is not a monster with stats", d.Treasure.Chest))
		}
		d.Treasure.ChestID = id
		for i := range d.Treasure.Hints {
			h := &d.Treasure.Hints[i]
			id, ok := uint32(0), false
			if n.sprite != nil {
				id, ok = n.sprite(h.Sprite)
			}
			if !ok {
				errs = append(errs, fmt.Sprintf("hint_npcs: NPC %d: sprite %q is not an NPC sprite name, such as 4_M_MANAGER", i+1, h.Sprite))
			}
			h.SpriteID = id
		}
	}
	if rw := d.Rewards; rw != nil {
		by := set.Mod(r.SetBy["rewards"])
		need := func(perm, what string) {
			if by != nil && !by.Grants.Has(perm) {
				errs = append(errs, fmt.Sprintf("rewards: %s needs the permission %s to give %s", by.ID(), perm, what))
			}
		}
		for i := range rw.Items {
			found := false
			for _, c := range local(rw.Items[i].Item) {
				if id, ok := n.item(c); ok {
					rw.Items[i].ID, found = id, true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Sprintf("rewards: items: %s is not an item", rw.Items[i].Item))
			}
		}
		if len(rw.Items) > 0 {
			need("player.give_items", "items")
		}
		if rw.Zeny > 0 {
			need("player.give_zeny", "zeny")
		}
		if rw.BaseExp > 0 || rw.JobExp > 0 {
			need("player.give_exp", "exp")
		}
		for _, name := range slices.Sorted(maps.Keys(rw.Currency)) {
			cOwner, ok := "", false
			if n.currency != nil {
				cOwner, ok = n.currency(name, owner)
			}
			if !ok {
				errs = append(errs, fmt.Sprintf("rewards: currency: %s is not a currency of data/currencies.yml", name))
				continue
			}
			if by != nil && cOwner != by.ID() {
				need("currencies.modify", "another mod's currency "+name)
			}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// IsField reports whether a map is a field: a map whose ID has "_fild",
// as rAthena names its fields, such as prt_fild08. Typed events with a
// level band use fields only.
func IsField(id string) bool { return strings.Contains(id, "_fild") }

// FieldLevels is the level of each field with spawns, by map ID: the
// middle level of its monsters, weighted by their amounts, leaving out
// bosses (review decision 31).
func FieldLevels(spawns map[string][]zone.Spawn) map[string]int {
	out := map[string]int{}
	for id, list := range spawns {
		if !IsField(id) {
			continue
		}
		type lv struct{ level, amount int }
		var levels []lv
		total := 0
		for _, s := range list {
			if s.Reward == nil || s.Reward.Boss() != progression.BossNone || s.Amount <= 0 {
				continue
			}
			levels = append(levels, lv{s.Reward.Level, s.Amount})
			total += s.Amount
		}
		if total == 0 {
			continue
		}
		slices.SortFunc(levels, func(a, b lv) int { return a.level - b.level })
		seen := 0
		for _, l := range levels {
			seen += l.amount
			if 2*seen >= total {
				out[id] = l.level
				break
			}
		}
	}
	return out
}
