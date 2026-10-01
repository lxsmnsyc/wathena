package convert

import (
	"fmt"
	"sort"
	"strings"
)

// Files are the generated files of a converted item database, by path
// relative to the mode's folder in mods/core:
//   - data/items.yml sets the script, bonuses or unsupported field of
//     every item with a script.
//   - scripts/items/<AegisName>.lua is the entry script of each item
//     that needs Lua.
func (r *Result) Files() map[string]string {
	out := map[string]string{}
	var data strings.Builder
	data.WriteString("# Item scripts converted from rAthena's item_db by wactl convert items.\n")
	data.WriteString("# Generated; do not edit. See mods/README.md, \"Item scripts\".\n")
	items := append([]*Item(nil), r.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	used := map[string]bool{}
	for _, it := range items {
		var fields []string
		if it.Lua != "" {
			path := ScriptPath(it.AegisName, it.ID, used)
			out[path] = it.Lua
			fields = append(fields, "script: "+yamlString(path))
		}
		if it.Bonuses != nil {
			fields = append(fields, "bonuses: ["+strings.Join(it.Bonuses, ", ")+"]")
		}
		if len(it.Unsupported) > 0 {
			fields = append(fields, "unsupported: "+yamlString(unsupportedSummary(it)))
		}
		if len(fields) == 0 {
			continue
		}
		fmt.Fprintf(&data, "- {name: %s, %s}\n", yamlString(it.AegisName), strings.Join(fields, ", "))
	}
	out["data/items.yml"] = data.String()
	return out
}

// ScriptPath is the generated script of an item. Names that are not
// valid module names, or that two items share without case, use the
// item ID.
func ScriptPath(aegis string, id uint32, used map[string]bool) string {
	name := aegis
	valid := name != ""
	for _, c := range name {
		if c != '_' && c != '-' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			valid = false
		}
	}
	if !valid || used[strings.ToLower(name)] {
		name = fmt.Sprintf("item_%d", id)
	}
	used[strings.ToLower(name)] = true
	return "scripts/items/" + name + ".lua"
}

// unsupportedSummary says in a few words what an item needs that the
// server cannot do yet, for the item data and the message players see.
// The coverage report has the details.
func unsupportedSummary(it *Item) string {
	seen := map[string]bool{}
	var out []string
	var features []string
	for f, ok := range it.Features {
		if !ok {
			features = append(features, f)
		}
	}
	sort.Strings(features)
	if len(features) == 0 && len(it.Unsupported) > 0 {
		features = []string{"script"}
	}
	for _, f := range features {
		w := friendly(f)
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return strings.Join(out, ", ")
}

// friendly names a feature in words a player understands.
func friendly(feature string) string {
	name := strings.SplitN(feature, " ", 2)[0]
	switch {
	case strings.HasPrefix(name, "sc_") || feature == "status effects":
		return "status effects"
	case name == "pet" || name == "bpet" || name == "getpetinfo":
		return "pets"
	case strings.HasPrefix(name, "mercenary_"):
		return "mercenaries"
	case strings.HasPrefix(name, "laphine_"):
		return "the Laphine windows"
	case name == "item_reform":
		return "item reform"
	case name == "item_enchant":
		return "item enchanting"
	case name == "itemskill" || name == "unitskilluseid" || name == "skilleffect":
		return "skills used by items"
	case name == "callfunc":
		return "NPC functions"
	case name == "bonus_script":
		return "timed bonuses"
	case name == "transform" || name == "active_transform":
		return "transformations"
	case name == "getexp" || name == "getexp2" || name == "guildgetexp":
		return "experience from items"
	case name == "getitembound":
		return "bound items"
	case name == "showscript" || name == "setfont":
		return "text effects"
	case name == "heal":
		return "direct HP and SP changes"
	case name == "vip_status":
		return "VIP accounts"
	case strings.HasPrefix(name, "bonus") || name == "autobonus" || name == "autobonus2" || name == "autobonus3" || name == "skill":
		return "item bonuses"
	}
	return "script parts that do not convert yet"
}
