package gamedata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// Mods is what the loaded mods bring to a world.
type Mods struct {
	Set   *mod.Set
	Items *mod.Items
	// NPCs are the placed NPCs, by map asset name, such as map/prontera.
	NPCs map[string][]zone.NPC
	// NPCOverlay is the merge of every mod's data/npcs.yml.
	NPCOverlay *mod.Overlay
	// Monsters are every monster with the mods' overlays, by ID, and
	// MonsterOverlay is the merge of every mod's data/monsters.yml.
	Monsters       map[uint32]Monster
	MonsterOverlay *mod.Overlay
	// Tables are the exp and drop tables, which also know mod items.
	Tables *progression.Tables
	// Events are the world events of data/events.yml, and EventOverlay
	// their merge.
	Events       []worldevent.Def
	EventOverlay *mod.Overlay
	// Skills and SkillTrees are core's skills and trees with the mods'
	// data/skills.yml and data/skill_trees.yml. SkillOverlay is the merge
	// of data/skills.yml.
	Skills       *skill.DB
	SkillTrees   *skill.Trees
	SkillOverlay *mod.Overlay
	// RandomOptions are the random options and groups with the mods'
	// overlays.
	RandomOptions *mod.RandomOptions
	// Zone is what every zone gets.
	Zone *zone.Mods
}

// LoadMods checks the found mods against each other and the config, and
// merges their data over the core data. ids keeps the IDs of the records
// mods add. core is the mode's item and NPC scripts, converted from
// rAthena (docs/decisions/0007-converted-item-scripts.md). They are
// core's files: cfg.Core is set to them.
//
// monsters are the core monsters, and tables the exp and drop tables,
// which may be nil. Mods' monster overlays apply over them.
func LoadMods(ctx context.Context, found []*mod.Mod, cfg mod.Config, mode reference.Mode, items *item.Data, monsters map[uint32]Monster,
	tables *progression.Tables, ids mod.IDStore, core *convert.Core, log *slog.Logger) (*Mods, error) {
	cfg.Core = core.Files
	set, err := mod.Resolve(found, cfg)
	if err != nil {
		return nil, err
	}
	for _, n := range set.Notes {
		log.Warn("mods: " + n)
	}
	var list []string
	for _, m := range set.Mods {
		if !m.Builtin {
			list = append(list, m.ID()+" "+m.Manifest.Version)
		}
	}
	log.Info("mods loaded", "ruleset", set.Ruleset.String(), "mods", strings.Join(list, ", "))

	assigned, err := mod.LoadIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]uint32, len(items.Items))
	for _, d := range items.Items {
		byName[d.AegisName] = d.ID
	}
	merged, err := set.Items(ctx, items, assigned, core.Names(byName))
	if err != nil {
		return nil, err
	}
	for _, a := range assigned.Assigned {
		log.Info("mods: new ID", "kind", a.Kind, "name", a.Name, "id", a.ID, "mod", a.Mod)
	}
	sprites, err := NPCSprites(mode)
	if err != nil {
		return nil, err
	}
	placed, npcOverlay, err := set.NPCs(func(name string) (uint32, bool) {
		id, ok := sprites[strings.ToUpper(name)]
		return id, ok
	})
	if err != nil {
		return nil, err
	}
	for _, n := range append(merged.Overlay.Notes, npcOverlay.Notes...) {
		log.Warn("mods: " + n)
	}
	out := &Mods{Set: set, Items: merged, NPCs: map[string][]zone.NPC{}, NPCOverlay: npcOverlay}
	shops, err := npcShops(placed, merged)
	if err != nil {
		return nil, err
	}
	for _, n := range placed {
		if !n.Placed {
			continue
		}
		zn := zone.NPC{
			Name: n.Name, DisplayName: n.DisplayName, X: n.X, Y: n.Y, Direction: n.Direction, Sprite: n.Sprite,
			Shop: shops[n.Name],
		}
		if n.Script != "" {
			zn.Script = n.ScriptMod + ":" + n.Script
		}
		out.NPCs[n.Map] = append(out.NPCs[n.Map], zn)
	}
	scripts, err := set.AllScripts()
	if err != nil {
		return nil, err
	}
	if err := checkItemScripts(merged.Data, scripts); err != nil {
		return nil, err
	}
	if out.Skills, out.SkillTrees, out.SkillOverlay, err = modSkills(ctx, set, mode, assigned, core.Constants, merged.IDs, scripts); err != nil {
		return nil, err
	}
	for _, n := range out.SkillOverlay.Notes {
		log.Warn("mods: " + n)
	}
	for _, a := range assigned.Assigned {
		if a.Kind == mod.SkillIDs.Kind {
			log.Info("mods: new ID", "kind", a.Kind, "name", a.Name, "id", a.ID, "mod", a.Mod)
		}
	}
	groups, err := item.NewGroups(core.Groups, func(name string) (uint32, bool) {
		id, ok := merged.IDs[name]
		return id, ok
	})
	if err != nil {
		return nil, err
	}
	var all []string
	for _, m := range set.Mods {
		all = append(all, m.ID())
	}
	if tables != nil {
		types := map[uint32]string{}
		for _, d := range merged.Data.Items {
			types[d.ID] = d.Type
		}
		tables = tables.WithItems(types)
	}
	out.Tables = tables
	monsterIDs := core.Monsters
	if monsters != nil {
		if out.Monsters, monsterIDs, out.MonsterOverlay, err = monsterOverlay(ctx, set, monsters, core.Constants, merged.IDs, out.Skills, tables, assigned); err != nil {
			return nil, err
		}
		for _, n := range out.MonsterOverlay.Notes {
			log.Warn("mods: " + n)
		}
	}
	if out.Events, out.EventOverlay, err = worldEvents(set); err != nil {
		return nil, err
	}
	for _, n := range out.EventOverlay.Notes {
		log.Warn("mods: " + n)
	}
	templates := make(map[uint32]zone.Spawn, len(out.Monsters))
	for id, m := range out.Monsters {
		if m.Status != nil {
			templates[id] = m.Template(id)
		}
	}
	opts, err := set.RandomOptions(core.Names(merged.IDs))
	if err != nil {
		return nil, err
	}
	for _, n := range append(opts.OptionOverlay.Notes, opts.GroupOverlay.Notes...) {
		log.Warn("mods: " + n)
	}
	if err := checkOptionGroups(out.Monsters, opts); err != nil {
		return nil, err
	}
	out.RandomOptions = opts
	labelRolls(merged.Data, opts, core.Constants)
	out.Zone = &zone.Mods{Scripts: scripts, IDs: all, ItemIDs: merged.IDs, Log: log,
		Constants: core.Constants, Groups: groups, MonsterIDs: monsterIDs, Shops: shops, Monsters: templates,
		RandomOptions: opts, ItemRolls: set.Ruleset.Flag("item_rolls") == "on"}
	return out, nil
}

// checkOptionGroups checks that every drop's random option group exists.
func checkOptionGroups(monsters map[uint32]Monster, opts *mod.RandomOptions) error {
	var errs []error
	for _, id := range slices.Sorted(maps.Keys(monsters)) {
		m := monsters[id]
		if m.Reward == nil {
			continue
		}
		for _, d := range append(slices.Clone(m.Reward.Drops), m.Reward.MvpDrops...) {
			if d.RandomOptionGroup != "" && opts.Group(d.RandomOptionGroup) == nil {
				errs = append(errs, fmt.Errorf("monsters %s: drop of item %d: there is no random option group %q", m.AegisName, d.Item, d.RandomOptionGroup))
			}
		}
	}
	return errors.Join(errs...)
}

// labelRolls sets how the client shows each roll and random option: the
// words of the first static bonus that reads it, as bonus.Describe.
func labelRolls(items *item.Data, opts *mod.RandomOptions, consts *constants.Data) {
	var modules map[string]*constants.Module
	if consts != nil {
		modules = consts.Modules()
	}
	constName := func(module string, v int64) string {
		if m := modules[module]; m != nil {
			return m.Names[v]
		}
		return ""
	}
	for i := range items.Items {
		d := &items.Items[i]
		for name, r := range d.Rolls {
			r.Label, r.Unit = "", ""
			for _, b := range d.Static {
				if slices.Contains(b.Rolls(), name) {
					r.Label, r.Unit = bonus.Describe(b, constName)
					break
				}
			}
			if r.Label == "" {
				r.Label = strings.ReplaceAll(name, "_", " ")
				r.Label = strings.ToUpper(r.Label[:1]) + r.Label[1:]
			}
		}
	}
	for _, o := range opts.Options {
		if len(o.Static) == 0 {
			continue
		}
		label, unit := bonus.Describe(o.Static[0], constName)
		if o.DisplayName == "" {
			o.DisplayName = label
		}
		o.Unit = unit
		if s, ok := bonus.Lookup(o.Static[0].Name); ok {
			o.Flag = !bonus.HasAmount(s, o.Static[0])
		}
	}
}

// npcShops makes the shops of NPC records, by record name. A price of -1
// is the item's buy price. An item that does not exist is an error.
func npcShops(npcs []mod.NPC, items *mod.Items) (map[string]*zone.Shop, error) {
	byID := make(map[uint32]*item.Def, len(items.Data.Items))
	for i := range items.Data.Items {
		byID[items.Data.Items[i].ID] = &items.Data.Items[i]
	}
	out := map[string]*zone.Shop{}
	var errs []error
	for _, n := range npcs {
		if n.Shop == nil {
			continue
		}
		s := &zone.Shop{Name: n.Name, Discount: n.Discount}
		for _, it := range n.Shop {
			id, ok := items.IDs[it.Item]
			if !ok {
				errs = append(errs, fmt.Errorf("npcs %s: shop: there is no item %q", n.Name, it.Item))
				continue
			}
			price := it.Price
			if price < 0 {
				price = byID[id].Buy
			}
			s.Items = append(s.Items, zone.ShopItem{ID: id, Price: price})
		}
		out[n.Name] = s
	}
	return out, errors.Join(errs...)
}

// checkItemScripts checks that every item's script exists, so a typo in
// a mod's script field fails at start and not at the first use.
func checkItemScripts(items *item.Data, scripts []*mod.Scripts) error {
	files := map[string]bool{}
	for _, sc := range scripts {
		for p := range sc.Files {
			files[sc.Mod+":"+p] = true
		}
	}
	var errs []error
	for _, d := range items.Items {
		if d.Script != "" && !files[d.Script] {
			errs = append(errs, fmt.Errorf("items %s: script %s does not exist", d.AegisName, d.Script))
		}
	}
	return errors.Join(errs...)
}

// NPCSprites reads the NPC sprite IDs of the client tables that
// reference/rathena extracted, by upper-case name, such as 4_F_KAFRA1.
func NPCSprites(mode reference.Mode) (map[string]uint32, error) {
	var d struct {
		NPCSprites []struct {
			ID     uint32 `json:"id"`
			Sprite string `json:"sprite"`
		} `json:"npc_sprites"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/client_tables.json", &d); err != nil {
		return nil, err
	}
	out := make(map[string]uint32, len(d.NPCSprites))
	for _, s := range d.NPCSprites {
		out[strings.ToUpper(s.Sprite)] = s.ID
	}
	return out, nil
}
