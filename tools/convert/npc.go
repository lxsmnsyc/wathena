package convert

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/currency"
)

// NPC conversion: rAthena's NPC lines (npcs.json) become core's NPC data
// and scripts:
//   - data/npcs.yml places every NPC that converts, and holds every shop
//     that converts (npcshop.go).
//   - scripts/npcs/<name>.lua is the entry script of a script NPC. Its
//     duplicates use the same file.
//   - scripts/functions/<name>.lua is a function (callfunc), loaded when
//     a script first calls it.
//   - scripts/lib/rathena.lua has the helpers they share.
//
// A script NPC is placed when its own script converts whole. Functions
// may hold parts that do not convert: they fail only when a script
// reaches them, and the NPCs that call them count as partly converted.

//go:embed rathena.lua
var rathenaLua string

// RathenaLib is the path of the shared helpers in core.
const RathenaLib = "scripts/lib/rathena.lua"

// NPCInput is what the NPC converter reads: the extracts of one mode.
type NPCInput struct {
	// NPCs is npcs.json.
	NPCs io.Reader
	// Constants is script_constants.json.
	Constants io.Reader
	// ClientTables is client_tables.json, for item names and NPC
	// sprites.
	ClientTables io.Reader
	// Shops is shops.json, for barter shops. It may be nil.
	Shops io.Reader
}

// npcLine is one line of npcs.json.
type npcLine struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Display     string `json:"display"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Map         string `json:"map"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Dir         int    `json:"dir"`
	Sprite      string `json:"sprite"`
	XS          int    `json:"xs"`
	YS          int    `json:"ys"`
	State       string `json:"state"`
	Script      string `json:"script"`
	DuplicateOf string `json:"duplicate_of"`
	Shop        *struct {
		Items []struct {
			Item  string `json:"item"`
			ID    uint32 `json:"id"`
			Price int    `json:"price"`
			// Stock is a marketshop item's stock, -1 for no limit.
			Stock *int `json:"stock,omitempty"`
		} `json:"items"`
		Discount bool `json:"discount"`
		// Currency is an itemshop's item or a pointshop's variable.
		Currency *struct {
			Item     string `json:"item,omitempty"`
			Variable string `json:"variable,omitempty"`
		} `json:"currency,omitempty"`
	} `json:"shop"`
}

// NPC is one converted NPC line.
type NPC struct {
	// Name is the NPC's record name in core, and Source its unique name
	// in rAthena.
	Name, Source string
	Kind         string
	File         string
	Line         int
	Map          string
	// DuplicateOf is the source of a duplicate.
	DuplicateOf string
	Status      Status
	// Placed is set when data/npcs.yml places the NPC on its map.
	Placed bool
	// Shop is set for a shop that converted, placed or not.
	Shop bool
	// Script is the NPC's Lua file, or "".
	Script string
	// Unsupported lists what did not convert, with the reason.
	Unsupported []string
	// Features are the commands the script uses, and whether each
	// converted. Functions it calls count too.
	Features map[string]bool
	// Functions are the functions the script calls, directly or not.
	Functions []string
	// Lua is the entry script, for a script NPC that converted.
	Lua string
	// record is the npcs.yml record.
	record string
	// converted is how the script converted, before the checks that
	// place it. Duplicates use their source's script, so a source that is
	// not placed, such as one without a map, still serves them.
	converted Status
}

// Function is one converted rAthena function.
type Function struct {
	Name, File string
	Line       int
	// Path is the Lua file, and Lua its content. A function that does
	// not parse has none.
	Path, Lua string
	// Stubs are the parts that did not convert, which fail when a
	// script reaches them.
	Stubs []string
	// Features are the commands it uses.
	Features map[string]bool
	// Calls are the functions it calls.
	Calls []string
	// Err is set when the function did not convert at all.
	Err string
}

// NPCResult is a converted NPC set.
type NPCResult struct {
	Mode      string
	NPCs      []*NPC
	Functions map[string]*Function
	// ScriptFolders and LeftOut are the extract's: the folders whose
	// script NPCs it kept, and the counts of the others.
	ScriptFolders []string
	LeftOut       map[string]int

	conv *npcConverter
}

// npcConverter holds what conversions of one mode share.
type npcConverter struct {
	mode     string
	consts   *constants.Data
	modules  map[string]*constants.Module
	items    map[uint32]string
	itemIDs  map[string]uint32
	sprites  map[string]uint32
	spriteID map[uint32]bool
	jobs     map[string]string
	// monsters are the monsters by ID, and monsterIDs their IDs by
	// AegisName in upper case.
	monsters   map[int64]monsterInfo
	monsterIDs map[string]int64
	// functions are the rAthena functions by lower-case name, and
	// converted those converted so far.
	functions map[string]*npcLine
	converted map[string]*Function
	paths     map[string]bool
	// zeroAmounts and checkedAmounts are the lines with a getitem or
	// delitem of a fixed amount of 0 or less, which convert to nothing,
	// and of a computed amount, which R checks.
	zeroAmounts    map[string]bool
	checkedAmounts map[string]bool
	// shops are the shops and barters by rAthena's unique name, for
	// callshop and the npcshop commands.
	shops map[string]*shopRef
	// currencyVars are the currencies of variables by lower-case name:
	// #CASHPOINTS, #KAFRAPOINTS and pointshop variables. currencyErr
	// says why a pointshop's variable is not one, and newCurrencies are
	// the records added to core's data/currencies.yml.
	currencyVars  map[string]string
	currencyErr   map[string]string
	newCurrencies []currencyRecord
}

// monsterInfo is a monster of client_tables.json: its AegisName, and
// the name players see.
type monsterInfo struct {
	sprite, name string
}

// unavailableFunctions are rAthena functions for services wAthena does
// not have yet. A call says so instead of running them.
var unavailableFunctions = map[string]string{
	"f_kafcart": "Pushcart rental",
}

// ignoredFunctions are rAthena functions that do nothing in wAthena.
// F_ClearGarbage clears variables of old rAthena versions.
var ignoredFunctions = map[string]bool{"f_cleargarbage": true}

// ConvertNPCs converts the NPC lines of a mode.
func ConvertNPCs(in NPCInput) (*NPCResult, error) {
	var extract struct {
		Mode          string         `json:"mode"`
		ScriptFolders []string       `json:"script_folders"`
		LeftOut       map[string]int `json:"left_out"`
		NPCs          []npcLine      `json:"npcs"`
	}
	if err := json.NewDecoder(in.NPCs).Decode(&extract); err != nil {
		return nil, fmt.Errorf("npcs.json: %w", err)
	}
	consts, err := constants.Read(in.Constants)
	if err != nil {
		return nil, err
	}
	var tables struct {
		Items []struct {
			ID        uint32 `json:"id"`
			AegisName string `json:"aegis_name"`
		} `json:"items"`
		NPCSprites []struct {
			ID     uint32 `json:"id"`
			Sprite string `json:"sprite"`
		} `json:"npc_sprites"`
		Monsters []struct {
			ID     int64  `json:"id"`
			Sprite string `json:"sprite"`
			Name   string `json:"name"`
		} `json:"monsters"`
	}
	if err := json.NewDecoder(in.ClientTables).Decode(&tables); err != nil {
		return nil, fmt.Errorf("client_tables.json: %w", err)
	}
	c := &npcConverter{
		mode: extract.Mode, consts: consts, modules: consts.Modules(),
		items: map[uint32]string{}, itemIDs: map[string]uint32{}, sprites: map[string]uint32{}, spriteID: map[uint32]bool{},
		jobs: map[string]string{}, functions: map[string]*npcLine{}, converted: map[string]*Function{}, paths: map[string]bool{},
		zeroAmounts: map[string]bool{}, checkedAmounts: map[string]bool{},
		monsters: map[int64]monsterInfo{}, monsterIDs: map[string]int64{}, shops: map[string]*shopRef{},
	}
	shopsIn, err := readShops(in.Shops)
	if err != nil {
		return nil, err
	}
	for _, m := range tables.Monsters {
		c.monsters[m.ID] = monsterInfo{sprite: m.Sprite, name: m.Name}
		c.monsterIDs[strings.ToUpper(m.Sprite)] = m.ID
	}
	for _, it := range tables.Items {
		c.items[it.ID] = it.AegisName
		c.itemIDs[strings.ToLower(it.AegisName)] = it.ID
	}
	for _, s := range tables.NPCSprites {
		c.sprites[strings.ToUpper(s.Sprite)] = s.ID
		c.spriteID[s.ID] = true
	}
	for i := range extract.NPCs {
		if l := &extract.NPCs[i]; l.Kind == "function" {
			// A later function of the same name replaces the earlier
			// one, as npc_parse_function does.
			c.functions[strings.ToLower(l.Name)] = l
		}
	}
	out := &NPCResult{Mode: extract.Mode, Functions: c.converted, ScriptFolders: extract.ScriptFolders, LeftOut: extract.LeftOut, conv: c}
	c.prepareCurrencies(extract.NPCs)
	// Record names first, in the extract's order, then the barters'.
	// Shops convert before scripts, so callshop and the npcshop commands
	// find them wherever they are.
	names := map[string]bool{}
	recordNames := make([]string, len(extract.NPCs))
	shopNPCs := map[int]*NPC{}
	for i := range extract.NPCs {
		if l := &extract.NPCs[i]; l.Kind != "function" {
			recordNames[i] = recordName(l.Name, names)
		}
	}
	for i := range extract.NPCs {
		l := &extract.NPCs[i]
		if !isShopKind(l.Kind) {
			continue
		}
		n := &NPC{Name: recordNames[i], Source: l.Name, Kind: l.Kind, File: l.File, Line: l.Line, Map: l.Map, DuplicateOf: l.DuplicateOf, Features: map[string]bool{}}
		shopNPCs[i] = c.convertShop(n, l)
		ref := &shopRef{npc: shopNPCs[i], line: l, items: map[uint32]bool{}}
		if l.Shop != nil {
			for _, it := range l.Shop.Items {
				ref.items[it.ID] = true
			}
		}
		if c.shops[l.Name] == nil {
			c.shops[l.Name] = ref
		}
	}
	var barters []*NPC
	for i := range shopsIn.Barters {
		bl := &shopsIn.Barters[i]
		n := c.convertBarter(bl, recordName(bl.Name, names))
		barters = append(barters, n)
		if c.shops[bl.Name] == nil {
			ref := &shopRef{npc: n, line: &npcLine{Kind: "barter", Name: bl.Name}, items: map[uint32]bool{}}
			for _, it := range bl.Items {
				ref.items[it.ID] = true
			}
			c.shops[bl.Name] = ref
		}
	}
	bySource := map[string]*NPC{}
	for i := range extract.NPCs {
		l := &extract.NPCs[i]
		if l.Kind == "function" {
			continue
		}
		n := shopNPCs[i]
		if n == nil {
			n = c.convertNPC(l, recordNames[i], bySource)
		}
		bySource[l.Name] = n
		out.NPCs = append(out.NPCs, n)
	}
	out.NPCs = append(out.NPCs, barters...)
	return out, nil
}

// recordName makes a unique rAthena name a valid record name: no colons,
// no control characters, no spaces at the ends, and unique.
func recordName(name string, used map[string]bool) string {
	s := strings.Map(func(r rune) rune {
		if r == ':' || r < 32 || r == 127 {
			return '_'
		}
		return r
	}, name)
	s = strings.TrimSpace(s)
	if s == "" {
		s = "npc"
	}
	if len(s) > 60 {
		s = s[:60]
	}
	base := s
	for i := 2; used[strings.ToLower(s)]; i++ {
		s = base + "_" + strconv.Itoa(i)
	}
	used[strings.ToLower(s)] = true
	return s
}

// displayName is what players see: the name up to '#'.
func displayName(display string) string {
	d, _, _ := strings.Cut(display, "#")
	return strings.TrimSpace(d)
}

// sprite resolves a line's view to what npcs.yml takes: an NPC sprite
// name or number. ok is false for views that show nothing (-1) or that
// are not NPC sprites, such as monsters.
func (c *npcConverter) sprite(view string) (string, bool) {
	if n, err := strconv.Atoi(view); err == nil {
		if n < 0 || !c.spriteID[uint32(n)] { // #nosec G115 -- checked above.
			return "", false
		}
		return view, true
	}
	if _, ok := c.sprites[strings.ToUpper(view)]; ok {
		return view, true
	}
	return "", false
}

// direction is a named direction from rAthena's facing: 0 is north, then
// counterclockwise. The protocol's facing is 0 south, clockwise.
func direction(dir int) string {
	return []string{"south", "southwest", "west", "northwest", "north", "northeast", "east", "southeast"}[((4-dir)%8+8)%8]
}

func (c *npcConverter) convertNPC(l *npcLine, name string, bySource map[string]*NPC) *NPC {
	n := &NPC{Name: name, Source: l.Name, Kind: l.Kind, File: l.File, Line: l.Line, Map: l.Map, DuplicateOf: l.DuplicateOf, Features: map[string]bool{}}
	fail := func(feature, why string) *NPC {
		n.Status = None
		n.Features[feature] = false
		if why == "" {
			n.Unsupported = append(n.Unsupported, feature)
		} else {
			n.Unsupported = append(n.Unsupported, feature+": "+why)
		}
		return n
	}
	if l.Kind != "script" {
		return fail(l.Kind, "not an NPC kind the converter knows")
	}
	var src *NPC
	if l.DuplicateOf != "" {
		src = bySource[l.DuplicateOf]
		if src == nil {
			return fail("duplicate", "the source "+l.DuplicateOf+" is not in the extract")
		}
		n.Features, n.Functions = src.Features, src.Functions
		if src.converted == "" || src.converted == None {
			n.Status = None
			n.Unsupported = append(n.Unsupported, "duplicate: the source "+l.DuplicateOf+" did not convert")
			return n
		}
		n.Status, n.Script = src.converted, src.Script
	} else {
		nc := c.newNPCConv(l)
		lua, err := nc.npcScript()
		n.Features = nc.features
		n.Functions = nc.functions()
		if err != nil {
			n.Status = None
			n.Unsupported = append(n.Unsupported, describeError(errFeature(err), err))
			if ue, ok := err.(*unsupportedError); ok {
				n.Features[ue.Feature] = false
			}
			return n
		}
		n.Status = Full
		for _, f := range n.Functions {
			if fn := c.converted[f]; fn != nil && (len(fn.Stubs) > 0 || fn.Err != "") {
				n.Status = Partial
			}
		}
		if lua != "" {
			n.Lua = lua
			n.Script = ScriptPathIn("scripts/npcs/", l.Name, c.paths)
		}
		n.converted = n.Status
	}
	if l.Map == "" {
		n.Unsupported = append(n.Unsupported, "floating: a script NPC without a map runs only through events")
		n.Status = None
		return n
	}
	if strings.Contains(l.Map, "@") {
		// rAthena copies the NPCs of an instance's maps, such as 1@tower,
		// into each instance. wAthena has no instances yet.
		n.Unsupported = append(n.Unsupported, "instance: "+l.Map+" is an instance map, which wAthena does not have yet")
		n.Status = None
		return n
	}
	if l.State != "" && l.State != "enabled" {
		n.Unsupported = append(n.Unsupported, "state: the NPC starts "+l.State+", and needs events to show")
		n.Status = None
		return n
	}
	view, ok := c.sprite(l.Sprite)
	if !ok {
		n.Unsupported = append(n.Unsupported, "sprite: "+l.Sprite+" is not an NPC sprite")
		n.Status = None
		return n
	}
	n.Placed = true
	fields := []string{
		"name: " + yamlString(n.Name), "map: " + yamlString(l.Map),
		fmt.Sprintf("x: %d", l.X), fmt.Sprintf("y: %d", l.Y),
		"direction: " + direction(l.Dir), "sprite: " + yamlString(view),
		"display_name: " + yamlString(displayName(l.Display)),
	}
	if n.Script != "" {
		fields = append(fields, "script: "+yamlString(n.Script))
	}
	n.record = "- {" + strings.Join(fields, ", ") + "}"
	return n
}

// ScriptPathIn is the file of a converted script in a folder: the name
// with characters other than letters, digits, _ and - made _, such as
// Shuger_pront for Shuger#pront, made unique without case by a number.
// A name with nothing left is npc_<n>.
func ScriptPathIn(folder, name string, used map[string]bool) string {
	var b strings.Builder
	for _, c := range name {
		if c == '_' || c == '-' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_-")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "npc"
	}
	base := s
	for i := 2; used[strings.ToLower(folder+s)]; i++ {
		s = fmt.Sprintf("%s_%d", base, i)
	}
	used[strings.ToLower(folder+s)] = true
	return folder + s + ".lua"
}

// errFeature is the feature an error names.
func errFeature(err error) string {
	if ue, ok := err.(*unsupportedError); ok {
		return strings.SplitN(ue.Feature, " ", 2)[0]
	}
	return "script"
}

// Files are the converted files, by path relative to core's folder.
func (r *NPCResult) Files() map[string]string {
	out := map[string]string{RathenaLib: rathenaLua}
	var data strings.Builder
	data.WriteString("# NPCs and shops converted from rAthena's npc/ by wactl convert npcs.\n")
	data.WriteString("# Generated; do not edit. See mods/README.md, \"Core's NPCs\".\n")
	// A script is written when a placed NPC uses it. A source that is
	// not placed can serve its duplicates.
	used := map[string]bool{}
	for _, n := range r.NPCs {
		if n.record != "" && (n.Placed || n.Shop) && n.Status != None {
			data.WriteString(n.record + "\n")
			if n.Script != "" {
				used[n.Script] = true
			}
		}
	}
	for _, n := range r.NPCs {
		if n.Lua != "" && used[n.Script] {
			out[n.Script] = n.Lua
		}
	}
	out["data/npcs.yml"] = data.String()
	written := map[string]bool{}
	for _, f := range r.sortedFunctions() {
		if f.Lua != "" && !written[f.Path] {
			out[f.Path] = f.Lua
			written[f.Path] = true
		}
	}
	addCoreFiles(out)
	out[currency.File] = r.currencyFile(out[currency.File])
	return out
}

func (r *NPCResult) sortedFunctions() []*Function {
	out := make([]*Function, 0, len(r.Functions))
	for _, f := range r.Functions {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// functionsOf lists the functions a script calls, directly or through
// other functions, sorted.
func (c *npcConverter) functionsOf(direct []string) []string {
	seen := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if f := c.converted[name]; f != nil {
			for _, g := range f.Calls {
				walk(g)
			}
		}
	}
	for _, d := range direct {
		walk(d)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
