package gamedata

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/quest"
)

// Procedural content (review decision 46, decision 0044): what content
// reads of the loaded game data when `wactl world new` makes a world, and
// the quest templates.

// referenceJob is the job whose exp table scales quest rewards: a first
// job, Swordman, whose table runs to the transcendent level cap.
const referenceJob = 1

// ContentData returns what content reads of the loaded monsters, items
// and exp tables.
func ContentData(m *Mods) content.Data {
	itemNames := map[uint32]string{}
	type info struct {
		display     string
		collectable bool
	}
	items := map[string]info{}
	if m.Items != nil && m.Items.Data != nil {
		for _, d := range m.Items.Data.Items {
			itemNames[d.ID] = d.AegisName
			display := d.Name
			if display == "" {
				display = strings.ReplaceAll(d.AegisName, "_", " ")
			}
			items[d.AegisName] = info{display: display, collectable: strings.EqualFold(d.Type, "Etc") && d.Stackable}
		}
		for name, id := range m.Items.IDs {
			if _, ok := items[name]; !ok {
				if n, ok := itemNames[id]; ok {
					items[name] = items[n]
				}
			}
		}
	}
	monsters := map[string]content.MonsterInfo{}
	for _, mon := range m.Monsters {
		if mon.Def == nil || mon.AegisName == "" {
			continue
		}
		mi := content.MonsterInfo{Name: mon.AegisName, DisplayName: mon.Name, Level: mon.Def.Level, Boss: bossMonster(mon.Def)}
		if mon.Reward != nil {
			for _, d := range mon.Reward.Drops {
				if n, ok := itemNames[d.Item]; ok {
					mi.Drops = append(mi.Drops, content.Drop{Item: n, Rate: d.Rate})
				}
			}
		}
		monsters[mon.AegisName] = mi
	}
	d := content.Data{
		Monster: func(name string) (content.MonsterInfo, bool) {
			mi, ok := monsters[name]
			return mi, ok
		},
		Item: func(name string) (content.ItemInfo, bool) {
			it, ok := items[name]
			return content.ItemInfo{DisplayName: it.display, Collectable: it.collectable}, ok
		},
	}
	if m.Tables != nil {
		tables := m.Tables
		d.BaseExp = func(level int) int64 { return tables.BaseExpAt(referenceJob, level) }
	}
	return d
}

// QuestTemplates reads the quest templates of the loaded mods: core's
// with the mods' data/quest_templates.yml, with their items checked.
func QuestTemplates(m *Mods) ([]content.Template, *mod.Overlay, error) {
	var exists func(string) bool
	if m.Items != nil {
		exists = func(name string) bool {
			_, ok := m.Items.IDs[name]
			return ok
		}
	}
	return content.LoadTemplates(m.Set, exists)
}

// WorldQuests turns a world's stored quests into quest records and the
// quests core's boards and guilds read (decision 0044). A record is
// named by its ID as text, as core's converted quests are, and belongs to
// core:
//   - a hunt asks for Count kills of its monster;
//   - a collect quest has no objective: while it is in the log, each kill
//     of its monster gives its item at its drop rate (decision 0043,
//     "Kills"), and the board takes Count of them;
//   - a delivery has none: the board of its town ends it.
//
// A quest whose monster or item no longer loads, such as one of a removed
// mod, is left out, and missing names it.
func WorldQuests(w *content.World, monsters map[uint32]Monster, items *mod.Items) (recs []*quest.Record, out []modscript.CoreQuest, missing []string) {
	if w == nil {
		return nil, nil, nil
	}
	byName := map[string]uint32{}
	for id, m := range monsters {
		if m.AegisName != "" {
			byName[m.AegisName] = id
		}
	}
	for _, q := range w.Quests {
		var mon, it uint32
		ok := true
		if q.Monster != "" {
			mon, ok = byName[q.Monster]
		}
		if ok && q.Item != "" {
			if items == nil {
				ok = false
			} else {
				it, ok = items.IDs[q.Item]
			}
		}
		if !ok {
			missing = append(missing, fmt.Sprintf("%d (%s)", q.ID, q.Title))
			continue
		}
		r := &quest.Record{ID: q.ID, Name: strconv.FormatUint(uint64(q.ID), 10), Mod: mod.Core, Title: q.Title}
		switch q.Kind {
		case content.KindHunt:
			r.Targets = []quest.Target{{Monster: mon, Count: q.Count, Race: -1, Size: -1, Element: -1}}
		case content.KindCollect:
			r.Drops = []quest.Drop{{Monster: mon, Item: it, Count: 1, Rate: q.DropRate}}
		}
		recs = append(recs, r)
		out = append(out, modscript.CoreQuest{
			ID: int64(q.ID), Town: q.Town, Purpose: q.Purpose, Guild: q.Guild, Stage: q.Stage, Step: q.Step,
			Kind: q.Kind, Title: q.Title, Monster: int64(mon), Item: int64(it), Count: q.Count, To: q.To,
			BaseExp: q.BaseExp, JobExp: q.JobExp, Zeny: q.Zeny,
		})
	}
	return recs, out, missing
}

// WithWorldQuests returns a quest table with a world's quest records
// added. A record whose ID the table has already is an error.
func WithWorldQuests(t *quest.Table, recs []*quest.Record) (*quest.Table, error) {
	var all []*quest.Record
	if t != nil {
		all = slices.Clone(t.All())
	}
	for _, r := range recs {
		if t != nil && t.ByID(r.ID) != nil {
			return nil, fmt.Errorf("world quest %d: a loaded quest has the ID", r.ID)
		}
		all = append(all, r)
	}
	return quest.NewTable(all), nil
}
