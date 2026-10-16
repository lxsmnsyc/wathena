package gamedata

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/quest"
)

// Quests (proposal 0002, review decision 45): core's data/quests.yml,
// converted from rAthena's quest_db.yml for the mode, with the mods'
// records over them. A core record's name is its ID. Records of mods get
// IDs from 1,000,000.

// modQuests merges the quest records. monsters maps monster names in
// upper case to IDs, and items item names to IDs.
func modQuests(ctx context.Context, set *mod.Set, ids *mod.IDs, monsters map[string]int64, items *mod.Items, consts *constants.Data) (*quest.Table, *mod.Overlay, error) {
	o, err := set.Overlay(mod.QuestKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	modules := consts.Modules()
	stacks := map[uint32]bool{}
	if items != nil && items.Data != nil {
		for _, d := range items.Data.Items {
			stacks[d.ID] = d.Stackable
		}
	}
	names := quest.Names{
		Monster: func(name string) (uint32, bool) {
			id, ok := monsters[strings.ToUpper(name)]
			if !ok || id <= 0 || id > int64(^uint32(0)) {
				return 0, false
			}
			return uint32(id), true // #nosec G115 -- checked above.
		},
		Item: func(name string) (uint32, bool, bool) {
			if items == nil {
				return 0, false, false
			}
			id, ok := items.IDs[name]
			return id, stacks[id], ok
		},
		Constant: func(module, name string) (int64, bool) {
			m := modules[module]
			if m == nil {
				return 0, false
			}
			v, ok := m.Values[name]
			return v, ok
		},
	}
	var recs []*quest.Record
	var errs []error
	used := map[uint32]string{}
	for _, r := range o.Records {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("%s: quests %s: %s", r.Where, r.Name, fmt.Sprintf(format, args...)))
		}
		var id uint32
		if r.Mod == mod.Core {
			n, err := strconv.ParseUint(r.Name, 10, 32)
			if err != nil || n == 0 || n >= uint64(mod.QuestIDs.Start) {
				bad("a core quest is named by its ID, from 1 to %d", mod.QuestIDs.Start-1)
				continue
			}
			id = uint32(n)
		} else {
			n, err := ids.ID(ctx, mod.QuestIDs, r.Name, r.Mod)
			if err != nil {
				return nil, nil, err
			}
			id = n
		}
		if prev, ok := used[id]; ok {
			bad("id %d is also %s", id, prev)
			continue
		}
		used[id] = r.Name
		rec, err := quest.Build(r.Name, r.Mod, id, r.Fields, names)
		if err != nil {
			bad("%v", err)
			continue
		}
		recs = append(recs, rec)
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}
	return quest.NewTable(recs), o, nil
}
