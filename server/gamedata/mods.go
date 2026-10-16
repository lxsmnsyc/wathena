package gamedata

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/currency"
	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/quest"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/storage"
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
	// PlaceNPCs are the NPCs whose records give a place in place of a
	// map: each stands at every place of its scope with its tag, which
	// the server finds once the maps are generated (review decision 42).
	PlaceNPCs []PlaceNPC
	// DungeonStyles and TownTemplates are core's dungeon styles and town
	// templates with the mods' data/dungeon_styles.yml and
	// data/town_templates.yml, in record order. GeneratedOverlays are
	// the two merges.
	DungeonStyles     []fields.Style
	TownTemplates     []fields.Template
	GeneratedOverlays []*mod.Overlay
	// Naming is core's naming styles with the mods' data/
	// naming_styles.yml (review decision 43), as town names use them. Its
	// overlay joins GeneratedOverlays.
	Naming *layout.Naming
	// NPCOverlay is the merge of every mod's data/npcs.yml.
	NPCOverlay *mod.Overlay
	// Monsters are every monster with the mods' overlays, by ID, and
	// MonsterOverlay is the merge of every mod's data/monsters.yml.
	Monsters       map[uint32]Monster
	MonsterOverlay *mod.Overlay
	// SpawnMonsters are what spawn selection reads of every monster, by
	// name, and SpawnThemes the spawn themes: core's with the mods'
	// data/spawn_themes.yml (review decision 44). SpawnThemeOverlay is
	// their merge, which joins GeneratedOverlays.
	SpawnMonsters     []spawns.Monster
	SpawnThemes       []spawns.Theme
	SpawnThemeOverlay *mod.Overlay
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
	// Jobs are the job records: core's with the mods' data/jobs.yml.
	// JobTables are the job tables built from them, or nil when LoadMods
	// had none to start from.
	Jobs      *Jobs
	JobTables *status.Tables
	// Companions are the companions and mounts: core's with the mods'
	// data/companions.yml and data/mounts.yml. CompanionOverlays are the
	// two merges.
	Companions        *companion.Set
	CompanionOverlays []*mod.Overlay
	// Storages are the extra storages of data/storages.yml, by ID, and
	// StorageOverlay their merge.
	Storages       []*storage.Record
	StorageOverlay *mod.Overlay
	// Currencies are the currencies: core's with the mods'
	// data/currencies.yml. CurrencyOverlay is their merge.
	Currencies      *currency.Table
	CurrencyOverlay *mod.Overlay
	// Quests are the quests: core's data/quests.yml, converted from
	// rAthena's quest_db.yml, with the mods' records (review decision
	// 45). QuestOverlay is their merge.
	Quests       *quest.Table
	QuestOverlay *mod.Overlay
	// Shops are the shops of the NPC records, which every zone shares.
	Shops *shop.Registry
	// Chat are the chat channels and filter: core's with the mods'
	// data/channels.yml and data/chat_filter.yml. ChatOverlays are the two
	// merges.
	Chat         *chat.Data
	ChatOverlays []*mod.Overlay
	// Rules are the ruleset's flags as the game code reads them
	// (decision 0019). JobTables, Tables, Skills and Zone already follow
	// them.
	Rules *rules.Rules
	// Zone is what every zone gets.
	Zone *zone.Mods
	// NPCArt maps the sprite IDs this start gave to NPC sprites of art to
	// their asset names (decision 0033).
	NPCArt map[uint32]string
}

// PlaceNPC is an NPC that stands at places: NPC has every field but the
// cell, which each place gives.
type PlaceNPC struct {
	NPC   zone.NPC
	Place mod.NPCPlace
}

// NPCArtSprites is the first sprite ID given to NPC sprites of art. The
// IDs are given at each start, in the order of the names, and the client
// learns them from the client data.
const NPCArtSprites = 1_000_000

// ItemArtViews is the first view ID given to items with a view_sprite.
// The IDs are given at each start, in the order of the item IDs, over
// the item's view field. A look sends them as the views of headgear and
// garments, and the client learns them from the client data (review
// decision 40).
const ItemArtViews = item.ArtViews

// giveArtViews gives each item whose record names a view_sprite a view ID
// of its own from ItemArtViews.
func giveArtViews(items *mod.Items) {
	if items == nil || items.Data == nil {
		return
	}
	index := make(map[uint32]int, len(items.Data.Items))
	for i := range items.Data.Items {
		index[items.Data.Items[i].ID] = i
	}
	next := ItemArtViews
	for _, id := range slices.Sorted(maps.Keys(items.Art)) {
		i, ok := index[id]
		if !ok || items.Art[id].ViewSprite == "" {
			continue
		}
		items.Data.Items[i].View = next
		next++
	}
}

// LoadMods checks the found mods against each other and the config, and
// merges their data over the core data. ids keeps the IDs of the records
// mods add. core is the mode's item and NPC scripts, converted from
// rAthena (docs/decisions/0007-converted-item-scripts.md). They are
// core's files: cfg.Core is set to them.
//
// monsters are the core monsters, and tables the exp and drop tables,
// which may be nil. Mods' monster overlays apply over them. jobs are the
// job tables, which may be nil: the result's JobTables are built from the
// job records over them, and the exp tables get the records' exp.
func LoadMods(ctx context.Context, found []*mod.Mod, cfg mod.Config, mode reference.Mode, items *item.Data, monsters map[uint32]Monster,
	tables *progression.Tables, jobs *status.Tables, ids mod.IDStore, core *convert.Core, log *slog.Logger) (*Mods, error) {
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
	jobRecs, err := modJobs(ctx, set, mode, assigned, core.Constants)
	if err != nil {
		return nil, err
	}
	for _, n := range jobRecs.Overlay.Notes {
		log.Warn("mods: " + n)
	}
	byName := make(map[string]uint32, len(items.Items))
	for _, d := range items.Items {
		byName[d.AegisName] = d.ID
	}
	merged, err := set.Items(ctx, items, assigned, core.Names(byName))
	if err != nil {
		return nil, err
	}
	if err := itemJobs(merged, jobRecs); err != nil {
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
	giveArtViews(merged)
	out := &Mods{Set: set, Items: merged, NPCs: map[string][]zone.NPC{}, NPCOverlay: npcOverlay, Jobs: jobRecs}
	if jobs != nil {
		out.JobTables = jobs.WithJobs(jobRecs.StatusData())
	}
	if out.Currencies, out.CurrencyOverlay, err = modCurrencies(ctx, set, assigned); err != nil {
		return nil, err
	}
	for _, n := range out.CurrencyOverlay.Notes {
		log.Warn("mods: " + n)
	}
	shops, err := npcShops(placed, merged, out.Currencies)
	if err != nil {
		return nil, err
	}
	out.Shops = shops
	out.NPCArt = map[uint32]string{}
	artIDs := map[string]uint32{}
	var artNames []string
	for _, n := range placed {
		if n.Art != "" && artIDs[n.Art] == 0 {
			artIDs[n.Art] = 1
			artNames = append(artNames, n.Art)
		}
	}
	slices.Sort(artNames)
	for i, name := range artNames {
		id := NPCArtSprites + uint32(i) // #nosec G115 -- one per NPC record.
		artIDs[name], out.NPCArt[id] = id, name
	}
	for _, n := range placed {
		if !n.Placed {
			continue
		}
		if n.Art != "" {
			n.Sprite = artIDs[n.Art]
		}
		zn := zone.NPC{
			Name: n.Name, DisplayName: n.DisplayName, X: n.X, Y: n.Y, Direction: n.Direction, Sprite: n.Sprite,
			Shop: shops.Get(n.Name),
		}
		if n.Script != "" {
			zn.Script = n.ScriptMod + ":" + n.Script
		}
		if n.Place != nil {
			out.PlaceNPCs = append(out.PlaceNPCs, PlaceNPC{NPC: zn, Place: *n.Place})
			continue
		}
		out.NPCs[n.Map] = append(out.NPCs[n.Map], zn)
	}
	if out.DungeonStyles, out.TownTemplates, out.GeneratedOverlays, err = fields.LoadRecords(set); err != nil {
		return nil, err
	}
	naming, namingOverlay, err := layout.LoadNaming(set)
	if err != nil {
		return nil, err
	}
	out.Naming = layout.NewNaming(naming)
	out.GeneratedOverlays = append(out.GeneratedOverlays, namingOverlay)
	for _, o := range out.GeneratedOverlays {
		for _, n := range o.Notes {
			log.Warn("mods: " + n)
		}
	}
	scripts, err := set.AllScripts()
	if err != nil {
		return nil, err
	}
	if err := checkItemScripts(merged.Data, scripts); err != nil {
		return nil, err
	}
	if out.Skills, out.SkillTrees, out.SkillOverlay, err = modSkills(ctx, set, mode, assigned, core.Constants, merged.IDs, scripts, jobRecs); err != nil {
		return nil, err
	}
	for _, n := range out.SkillOverlay.Notes {
		log.Warn("mods: " + n)
	}
	rl, notes, err := RulesOf(set.Ruleset, out.Skills)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		log.Warn("ruleset: " + n)
	}
	out.Rules = rl
	if out.JobTables != nil {
		if err := ApplyRuleset(out.JobTables, rl); err != nil {
			return nil, fmt.Errorf("ruleset: %w", err)
		}
	}
	if out.Companions, out.CompanionOverlays, err = modCompanions(ctx, set, assigned, out.Skills, core.Names(merged.IDs)); err != nil {
		return nil, err
	}
	for _, o := range out.CompanionOverlays {
		for _, n := range o.Notes {
			log.Warn("mods: " + n)
		}
	}
	if out.Storages, out.StorageOverlay, err = modStorages(ctx, set, assigned); err != nil {
		return nil, err
	}
	for _, n := range out.StorageOverlay.Notes {
		log.Warn("mods: " + n)
	}
	for _, a := range assigned.Assigned {
		if a.Kind == mod.SkillIDs.Kind || a.Kind == mod.CompanionIDs.Kind || a.Kind == mod.MountIDs.Kind || a.Kind == mod.StorageIDs.Kind ||
			a.Kind == mod.CurrencyIDs.Kind {
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
		tables = tables.WithJobs(jobRecs.ExpTables())
		types := map[uint32]string{}
		for _, d := range merged.Data.Items {
			types[d.ID] = d.Type
		}
		tables = tables.WithItems(types)
		if tables, err = levelPenalty(tables, mode, rl); err != nil {
			return nil, fmt.Errorf("ruleset: %w", err)
		}
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
	if out.SpawnThemes, out.SpawnThemeOverlay, err = spawnThemes(set, core.Constants); err != nil {
		return nil, err
	}
	for _, n := range out.SpawnThemeOverlay.Notes {
		log.Warn("mods: " + n)
	}
	out.GeneratedOverlays = append(out.GeneratedOverlays, out.SpawnThemeOverlay)
	out.SpawnMonsters = SpawnMonsters(out.Monsters, core.Constants)
	names := eventNames{
		item: func(name string) (uint32, bool) {
			id, ok := merged.IDs[name]
			return id, ok
		},
		currency: func(name, m string) (string, bool) {
			r := out.Currencies.Resolve(name, m)
			if r == nil {
				return "", false
			}
			return r.Mod, true
		},
		sprite: func(name string) (uint32, bool) {
			id, ok := sprites[strings.ToUpper(name)]
			return id, ok
		},
	}
	if monsters != nil {
		names.monster = func(name string) (uint32, bool) {
			id, ok := monsterIDs[strings.ToUpper(name)]
			if !ok || id <= 0 || id > math.MaxUint32 {
				return 0, false
			}
			m, ok := out.Monsters[uint32(id)]
			return uint32(id), ok && m.Status != nil
		}
	}
	if out.Events, out.EventOverlay, err = worldEvents(set, names); err != nil {
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
	if out.Chat, out.ChatOverlays, err = modChat(set); err != nil {
		return nil, err
	}
	for _, o := range out.ChatOverlays {
		for _, n := range o.Notes {
			log.Warn("mods: " + n)
		}
	}
	if out.Quests, out.QuestOverlay, err = modQuests(ctx, set, assigned, monsterIDs, merged, core.Constants); err != nil {
		return nil, err
	}
	for _, n := range out.QuestOverlay.Notes {
		log.Warn("mods: " + n)
	}
	for _, a := range assigned.Assigned {
		if a.Kind == mod.QuestIDs.Kind {
			log.Info("mods: new ID", "kind", a.Kind, "name", a.Name, "id", a.ID, "mod", a.Mod)
		}
	}
	labelRolls(merged.Data, opts, core.Constants)
	out.Zone = &zone.Mods{Quests: out.Quests, Scripts: scripts, IDs: all, ItemIDs: merged.IDs, Log: log,
		Constants: core.Constants, Groups: groups, MonsterIDs: monsterIDs, Shops: shops, Monsters: templates,
		RandomOptions: opts, ItemRolls: rl.ItemRolls, Rules: rl, Companions: out.Companions, Chat: out.Chat,
		Storages: out.Storages, Currencies: out.Currencies, OwnLookJobs: ownLookJobs(jobRecs)}
	return out, nil
}

// ownLookJobs are the jobs a look shows by their own ID: mods' jobs, and
// jobs whose records name body sprites.
func ownLookJobs(j *Jobs) map[int]bool {
	out := map[int]bool{}
	if j == nil {
		return out
	}
	for id, r := range j.Records {
		if (r.Data.Info.Mod != "" && r.Data.Info.Mod != mod.Core) || len(r.Art) > 0 {
			out[id] = true
		}
	}
	return out
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
// words of the first static bonus that reads it, as bonus.Describe. A
// roll's display_name replaces those words, and its unit stays.
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
			switch {
			case r.DisplayName != "":
				r.Label = r.DisplayName
			case r.Label == "":
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

// npcShops makes the shops of NPC records. A price of -1 is the item's
// buy price. An item or a currency that does not exist is an error.
func npcShops(npcs []mod.NPC, items *mod.Items, currencies *currency.Table) (*shop.Registry, error) {
	byID := make(map[uint32]*item.Def, len(items.Data.Items))
	for i := range items.Data.Items {
		byID[items.Data.Items[i].ID] = &items.Data.Items[i]
	}
	var out []*shop.Shop
	var errs []error
	for _, n := range npcs {
		if n.Shop == nil {
			continue
		}
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("npcs %s: shop: %s", n.Name, fmt.Sprintf(format, args...)))
		}
		var list []shop.Item
		for _, it := range n.Shop.Items {
			id, ok := items.IDs[it.Item]
			if !ok {
				bad("there is no item %q", it.Item)
				continue
			}
			si := shop.Item{ID: id, Zeny: it.Price, Stock: -1, Initial: -1}
			if it.Cost != nil {
				si.Zeny = it.Cost.Zeny
				for _, c := range it.Cost.Items {
					cid, ok := items.IDs[c.Item]
					if !ok {
						bad("%s: there is no item %q", it.Item, c.Item)
						continue
					}
					si.Items = append(si.Items, shop.CostItem{ID: cid, Amount: c.Amount})
				}
				for _, name := range slices.Sorted(maps.Keys(it.Cost.Currencies)) {
					r := currencies.Resolve(name, n.Mod)
					if r == nil {
						bad("%s: there is no currency %q in data/currencies.yml", it.Item, name)
						continue
					}
					si.Currencies = append(si.Currencies, shop.CostCurrency{ID: r.ID, Amount: int64(it.Cost.Currencies[name])})
				}
			} else if si.Zeny < 0 {
				si.Zeny = byID[id].Buy
			}
			if it.Stock > 0 {
				si.Stock, si.Initial = it.Stock, it.Stock
				si.Restock, _ = mod.ParseRestock(it.Restock)
			}
			list = append(list, si)
		}
		display := n.DisplayName
		if display == "" {
			display, _, _ = strings.Cut(mod.Local(n.Name), "#")
		}
		out = append(out, shop.New(n.Name, display, n.Shop.Discount, n.Shop.BuysFromPlayers, n.Mod, list))
	}
	return shop.NewRegistry(out), errors.Join(errs...)
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

// itemJobs resolves the job names of items' jobs lists: a core job sets
// the bits of rAthena's Jobs for its class, as item_db's Jobs, and a mod
// job is listed by ID.
func itemJobs(items *mod.Items, jobs *Jobs) error {
	var errs []error
	for i := range items.Data.Items {
		d := &items.Data.Items[i]
		if len(d.JobNames) == 0 {
			continue
		}
		mask := item.JobMask{}
		d.JobIDs = nil
		for _, name := range d.JobNames {
			id, ok := jobs.ID(name)
			if !ok {
				where := ""
				if r := items.Overlay.Record(d.AegisName); r != nil {
					where = r.Where + ": "
				}
				errs = append(errs, fmt.Errorf("%sitems %s: jobs: %q is not a job: want a name of wa.jobs, such as knight, or a mod job's full name", where, d.AegisName, name))
				continue
			}
			r := jobs.Records[id]
			if r.Data.Info.Mod != "" {
				d.JobIDs = append(d.JobIDs, id)
				continue
			}
			// itemdb_jobid2mapid: the bits of rAthena's Jobs for the
			// job's class.
			c := item.ClassMask(uint64(r.Data.MapID)) // #nosec G115 -- map job IDs are positive.
			for k := range mask {
				mask[k] |= c[k]
			}
		}
		d.Jobs = &mask
	}
	return errors.Join(errs...)
}
