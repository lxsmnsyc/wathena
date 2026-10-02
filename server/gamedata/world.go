package gamedata

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// Monster overlays (data/monsters.yml) and world events
// (data/events.yml). mods/README.md, "Monsters", lists the fields.
//
// The server has no monster status calculation yet (phase 4), so the
// values that combat reads are set directly: hp, attack, defense, hit
// and flee, not the base stats they come from.

// monsterFields are the fields of a monster record.
var monsterFields = []string{
	"display_name", "sprite", "level", "hp", "sp", "attack", "magic_attack", "defense", "magic_defense", "hit", "flee",
	"speed", "attack_range", "attack_delay", "attack_motion", "damage_motion", "element", "element_level",
	"race", "size", "class", "modes", "base_exp", "job_exp", "drops",
}

// Monster classes of class, as rAthena's e_aegis_monsterclass, and the
// modes each adds, as mob_db's loading does.
var monsterClasses = map[string]struct {
	class int
	modes uint32
}{
	"normal":      {0, 0},
	"boss":        {1, modeDetector | modeStatusImmune | modeKnockbackImmune},
	"guardian":    {2, modeStatusImmune},
	"battlefield": {4, modeStatusImmune | modeSkillImmune},
	"event":       {5, modeFixedItemDrop},
}

// Mode bits of rAthena's e_mode.
const (
	modeKnockbackImmune = 0x0200000
	modeFixedItemDrop   = 0x1000000
	modeDetector        = 0x2000000
	modeStatusImmune    = 0x4000000
	modeSkillImmune     = 0x8000000
)

// MonsterModes are the names of modes, from rAthena's MD_ names.
var MonsterModes = map[string]uint32{
	"can_move": 0x1, "looter": 0x2, "aggressive": 0x4, "assist": 0x8, "cast_sensor_idle": 0x10,
	"no_random_walk": 0x20, "no_cast": 0x40, "can_attack": 0x80, "cast_sensor_chase": 0x200,
	"change_chase": 0x400, "angry": 0x800, "change_target_melee": 0x1000, "change_target_chase": 0x2000,
	"target_weak": 0x4000, "random_target": 0x8000, "ignore_melee": 0x10000, "ignore_magic": 0x20000,
	"ignore_ranged": 0x40000, "mvp": 0x80000, "ignore_misc": 0x100000, "knockback_immune": 0x200000,
	"teleport_block": 0x400000, "fixed_item_drop": 0x1000000, "detector": 0x2000000,
	"status_immune": 0x4000000, "skill_immune": 0x8000000,
}

// monsterOverlay applies the mods' monster overlays over the core
// monsters. It returns every monster by ID, and every name, in upper
// case, by ID. items finds drops by item name. tables adjusts drops and
// scales exp, and may be nil.
func monsterOverlay(ctx context.Context, set *mod.Set, base map[uint32]Monster, consts *constants.Data,
	items map[string]uint32, tables *progression.Tables, ids *mod.IDs) (map[uint32]Monster, map[string]int64, *mod.Overlay, error) {
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
	for _, r := range o.Records {
		var m Monster
		var id uint32
		if core, ok := byName[r.Name]; ok {
			id, m = core, out[core].clone()
		} else {
			spriteName, _ := r.Fields["sprite"].(string)
			from, ok := byName[spriteName]
			if !ok {
				errs = append(errs, fmt.Errorf("%s: monsters %s: a new monster needs sprite: the AegisName of a monster whose sprite it shows, such as PORING", r.Where, r.Name))
				continue
			}
			if id, err = ids.ID(ctx, mod.MonsterIDs, r.Name, r.Mod); err != nil {
				return nil, nil, nil, err
			}
			m = out[from].clone()
			m.AegisName, m.Name = r.Name, mod.Local(r.Name)
			if m.Reward != nil {
				m.Reward.BaseExp, m.Reward.JobExp, m.Reward.Drops = 0, 0, nil
			}
		}
		if err := applyMonster(&m, id, r.Fields, byName, consts, items, tables); err != nil {
			errs = append(errs, fmt.Errorf("%s: monsters %s: %w", r.Where, r.Name, err))
			continue
		}
		byName[r.Name] = id
		out[id] = m
	}
	if len(errs) > 0 {
		return nil, nil, nil, errors.Join(errs...)
	}
	names := make(map[string]int64, len(byName))
	for n, id := range byName {
		names[strings.ToUpper(n)] = int64(id)
	}
	return out, names, o, nil
}

// clone copies a monster, so an overlay does not change the original.
func (m Monster) clone() Monster {
	if m.Status != nil {
		st := *m.Status
		m.Status = &st
	}
	if m.Reward != nil {
		r := *m.Reward
		r.Drops = slices.Clone(r.Drops)
		m.Reward = &r
	}
	return m
}

// applyMonster sets a record's fields on a monster.
func applyMonster(m *Monster, id uint32, f map[string]any, byName map[string]uint32, consts *constants.Data,
	items map[string]uint32, tables *progression.Tables) error {
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	for k := range f {
		if !slices.Contains(monsterFields, k) {
			bad("%q is not a field; monsters have %s", k, strings.Join(monsterFields, ", "))
		}
	}
	st := m.Status
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
	pair := func(k string, lo, hi int, a, b *int) {
		v, ok := f[k]
		if !ok {
			return
		}
		list, _ := v.([]any)
		if len(list) != 2 {
			bad("%s must be [minimum, maximum]", k)
			return
		}
		x, ok1 := list[0].(int)
		y, ok2 := list[1].(int)
		if !ok1 || !ok2 || x < lo || y > hi || x > y {
			bad("%s must be [minimum, maximum], from %d to %d", k, lo, hi)
			return
		}
		*a, *b = x, y
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
		from, ok := byName[s]
		if !ok {
			bad("sprite: there is no monster %q", s)
		} else {
			m.Sprite = from
		}
	}
	num("level", 1, 999, &st.Level)
	num("hp", 1, 1<<31-1, &st.MaxHP)
	num("sp", 0, 1<<31-1, &st.MaxSP)
	pair("attack", 0, 1<<20, &st.RHW.Atk, &st.RHW.Atk2)
	pair("magic_attack", 0, 1<<20, &st.MAtkMin, &st.MAtkMax)
	num("defense", -100, 1<<15, &st.Def)
	num("magic_defense", -100, 1<<15, &st.MDef)
	num("hit", 0, 1<<15, &st.Hit)
	num("flee", 0, 1<<15, &st.Flee)
	num("speed", 20, 1000, &st.Speed)
	num("attack_range", 1, 14, &st.RHW.Range)
	num("attack_delay", 20, 10000, &st.ADelay)
	num("attack_motion", 20, 10000, &st.AMotion)
	num("damage_motion", 0, 10000, &st.DMotion)
	constant("element", "elements", &st.DefEle)
	num("element_level", 1, 4, &st.EleLv)
	constant("race", "races", &st.Race)
	constant("size", "sizes", &st.Size)
	if v, ok := f["modes"]; ok {
		list, ok := v.([]any)
		if !ok {
			bad("modes must be a list, such as [can_move, can_attack, aggressive]")
		}
		var mode uint32
		for _, e := range list {
			s, _ := e.(string)
			bit, ok := MonsterModes[s]
			if !ok {
				bad("modes: %q is not a mode; the modes are %s", s, strings.Join(slices.Sorted(maps.Keys(MonsterModes)), ", "))
				continue
			}
			mode |= bit
		}
		st.Mode = mode
	}
	if v, ok := f["class"]; ok {
		s, _ := v.(string)
		c, ok := monsterClasses[s]
		if !ok {
			bad("class: %q is not normal, boss, guardian, battlefield or event", s)
		}
		st.Class = c.class
		st.Mode |= c.modes
	}
	if r := m.Reward; r != nil {
		r.ID, r.Level, r.MaxHP, r.Mode, r.Class = id, st.Level, st.MaxHP, st.Mode, st.Class
		var exp int
		if _, ok := f["base_exp"]; ok {
			num("base_exp", 0, 1<<31-1, &exp)
			r.BaseExp = progression.MobExp(int64(exp), tables.BaseExpRate)
		}
		if _, ok := f["job_exp"]; ok {
			num("job_exp", 0, 1<<31-1, &exp)
			r.JobExp = progression.MobExp(int64(exp), tables.JobExpRate)
		}
		if v, ok := f["drops"]; ok {
			drops, err := readDrops(v, items)
			if err != nil {
				bad("drops: %v", err)
			}
			r.Drops = tables.AdjustDrops(r.Boss(), drops)
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// readDrops reads a drops list: { item, rate }, with the rate per
// 10,000, as mob_db.yml.
func readDrops(v any, items map[string]uint32) ([]progression.Drop, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, errors.New("want a list of { item: Name, rate: n }, with rates per 10,000")
	}
	var out []progression.Drop
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok || len(m) != 2 {
			return nil, fmt.Errorf("drop %d: want { item: Name, rate: n }", i+1)
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
		out = append(out, progression.Drop{Item: id, Rate: rate})
	}
	if len(out) > 20 {
		return nil, errors.New("a monster has at most 20 drops")
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
// script exists.
func worldEvents(set *mod.Set) ([]worldevent.Def, *mod.Overlay, error) {
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
		owner, path, _ := strings.Cut(d.Script, ":")
		if !set.HasFile(owner, path) {
			errs = append(errs, fmt.Errorf("%s: events %s: script %s does not exist", r.Where, r.Name, d.Script))
			continue
		}
		out = append(out, d)
	}
	return out, o, errors.Join(errs...)
}
