package convert

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/lxsmnsyc/wathena/server/constants"
)

// Input is what the item converter reads: the extracts of one mode.
type Input struct {
	// Scripts is item_scripts.json.
	Scripts io.Reader
	// Constants is script_constants.json.
	Constants io.Reader
	// Groups is item_groups.json.
	Groups io.Reader
	// ClientTables is client_tables.json, for monster names.
	ClientTables io.Reader
}

// ItemScript is one item of item_scripts.json.
type ItemScript struct {
	ID            uint32 `json:"id"`
	AegisName     string `json:"aegis_name"`
	Type          string `json:"type"`
	Script        string `json:"script"`
	EquipScript   string `json:"equip_script"`
	UnequipScript string `json:"unequip_script"`
}

// Kind is how an item's scripts run.
type Kind string

const (
	// KindUsable items run their script when used.
	KindUsable Kind = "usable"
	// KindDelayed items are DelayConsume: used, but spent when their
	// effect happens.
	KindDelayed Kind = "delayed"
	// KindEquip items and cards run their scripts while equipped.
	KindEquip Kind = "equip"
)

// Status is how much of an item's scripts converted.
type Status string

const (
	Full    Status = "full"
	Partial Status = "partial"
	None    Status = "none"
)

// Item is one converted item.
type Item struct {
	ID        uint32
	AegisName string
	Kind      Kind
	Status    Status
	// Lua is the entry script, or "" when the item has none: its
	// bonuses are static data, or nothing converted.
	Lua string
	// Bonuses are the static bonuses of an equipment item whose scripts
	// are only constant bonuses, as item data entries.
	Bonuses []string
	// Unsupported lists what did not convert, with the reason.
	Unsupported []string
	// Features are the commands and bonus types the scripts use, each
	// with whether it converted.
	Features map[string]bool
}

// Result is a converted item database.
type Result struct {
	Mode  string
	Items []*Item

	conv *converter
}

type converter struct {
	consts       *constants.Data
	modules      map[string]*constants.Module
	items        map[uint32]string
	itemByName   map[string]string
	itemIDs      map[string]uint32
	monsters     map[int64]string
	groups       map[int64]string
	bonusByValue map[int64]string
	bonusCanon   map[string]string
	jobs         map[string]string
	skillNames   map[string]string
}

// ConvertItems converts every item script of a mode.
func ConvertItems(in Input) (*Result, error) {
	var scripts struct {
		Mode  string       `json:"mode"`
		Items []ItemScript `json:"items"`
	}
	if err := json.NewDecoder(in.Scripts).Decode(&scripts); err != nil {
		return nil, fmt.Errorf("item_scripts.json: %w", err)
	}
	consts, err := constants.Read(in.Constants)
	if err != nil {
		return nil, err
	}
	var groups struct {
		Groups []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"groups"`
	}
	if err := json.NewDecoder(in.Groups).Decode(&groups); err != nil {
		return nil, fmt.Errorf("item_groups.json: %w", err)
	}
	var tables struct {
		Monsters []struct {
			ID     int64  `json:"id"`
			Sprite string `json:"sprite"`
		} `json:"monsters"`
		Items []struct {
			ID        uint32 `json:"id"`
			AegisName string `json:"aegis_name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(in.ClientTables).Decode(&tables); err != nil {
		return nil, fmt.Errorf("client_tables.json: %w", err)
	}
	c := &converter{
		consts: consts, modules: consts.Modules(),
		items: map[uint32]string{}, itemByName: map[string]string{}, itemIDs: map[string]uint32{},
		monsters: map[int64]string{}, groups: map[int64]string{}, bonusByValue: map[int64]string{}, bonusCanon: map[string]string{},
		jobs: map[string]string{}, skillNames: map[string]string{},
	}
	for _, it := range tables.Items {
		c.items[it.ID] = it.AegisName
		c.itemByName[strings.ToLower(it.AegisName)] = it.AegisName
		c.itemIDs[strings.ToLower(it.AegisName)] = it.ID
	}
	for _, m := range tables.Monsters {
		c.monsters[m.ID] = m.Sprite
	}
	for _, g := range groups.Groups {
		c.groups[g.ID] = g.Name
	}
	var bonusNames []string
	for n := range consts.Constants {
		if len(n) > 1 && n[0] == 'b' && n[1] >= 'A' && n[1] <= 'Z' {
			bonusNames = append(bonusNames, n)
		}
	}
	sort.Strings(bonusNames)
	for _, n := range bonusNames {
		c.bonusCanon[strings.ToLower(n)] = n
		if _, ok := c.bonusByValue[consts.Constants[n]]; !ok {
			c.bonusByValue[consts.Constants[n]] = strings.ToLower(n)
		}
	}
	for name, v := range consts.Constants {
		if strings.HasPrefix(name, "JOB_") {
			if r, ok := c.modules["jobs"].Names[v]; ok {
				c.jobs[strings.TrimPrefix(name, "JOB_")] = r
			}
		}
	}
	for name := range consts.Skills {
		c.skillNames[strings.ToLower(name)] = name
	}
	out := &Result{Mode: scripts.Mode, conv: c}
	for i := range scripts.Items {
		out.Items = append(out.Items, c.convertItem(&scripts.Items[i]))
	}
	return out, nil
}

func (c *converter) itemID(name string) (uint32, bool) {
	id, ok := c.itemIDs[strings.ToLower(name)]
	return id, ok
}

// bonusCase is a bonus type's name as rAthena exports it, such as
// bAddRace for baddrace.
func (c *converter) bonusCase(lower string) string {
	if n, ok := c.bonusCanon[lower]; ok {
		return n
	}
	return lower
}

func (c *converter) skillName(name string) string {
	if n, ok := c.skillNames[strings.ToLower(name)]; ok {
		return n
	}
	return name
}

// jobByName finds the readable job name for an EAJ_ name without its
// prefix, such as KNIGHT.
func (c *converter) jobByName(name string) (string, bool) {
	r, ok := c.jobs[name]
	return r, ok
}

// kindOf is the kind of an item type.
func kindOf(typ string) Kind {
	switch strings.ToLower(typ) {
	case "healing", "usable", "cash":
		return KindUsable
	case "delayconsume":
		return KindDelayed
	}
	return KindEquip
}

// itemConv converts one item.
type itemConv struct {
	conv    *converter
	file    *luaFile
	helpers map[string]bool
	item    *Item
	// switches and autobonuses number the locals they declare.
	switches, autobonuses int
	// unequipping is set while an unequip script converts.
	unequipping bool
}

func (c *converter) convertItem(it *ItemScript) *Item {
	out := &Item{ID: it.ID, AegisName: it.AegisName, Kind: kindOf(it.Type), Features: map[string]bool{}}
	ic := &itemConv{conv: c, file: newLuaFile(), helpers: map[string]bool{}, item: out}
	if out.Kind == KindEquip {
		ic.equipItem(it)
	} else {
		ic.useItem(it)
	}
	return out
}

// fail records what did not convert.
func (c *itemConv) fail(feature string, err error) {
	c.item.Features[feature] = false
	c.item.Unsupported = append(c.item.Unsupported, describeError(feature, err))
}

// ok records a feature that converted, unless it failed elsewhere in the
// item.
func (c *itemConv) ok(feature string) {
	if _, seen := c.item.Features[feature]; !seen {
		c.item.Features[feature] = true
	}
}

func describeError(feature string, err error) string {
	if ue, ok := err.(*unsupportedError); ok {
		if ue.Feature == feature || strings.HasPrefix(ue.Feature, feature+" ") {
			if ue.Detail == "" {
				return ue.Feature
			}
			return ue.Feature + ": " + ue.Detail
		}
		return feature + ": " + ue.Error()
	}
	return feature + ": " + err.Error()
}

// header is the comment at the top of a generated file: the item, and its
// rAthena scripts.
func header(it *ItemScript) []string {
	out := []string{fmt.Sprintf("%s (%d). Generated by wactl convert items; do not edit.", it.AegisName, it.ID)}
	for _, s := range []struct{ name, text string }{{"Script", it.Script}, {"EquipScript", it.EquipScript}, {"UnEquipScript", it.UnequipScript}} {
		if s.text == "" {
			continue
		}
		out = append(out, "rAthena "+s.name+":")
		for _, l := range strings.Split(s.text, "\n") {
			out = append(out, "  "+l)
		}
	}
	return out
}
