package mod

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
)

// NPCKind is the NPC placement overlay: data/npcs.yml. Core has no NPCs
// yet. A name without a prefix is the mod's own NPC, so `name: Healer`
// in example.town is example.town:Healer.
var NPCKind = Kind{Name: "npcs", File: "data/npcs.yml", LocalNames: true}

// Directions are the facings an NPC can have, in the protocol's order:
// 0 is south, then clockwise.
var Directions = []string{"south", "southwest", "west", "northwest", "north", "northeast", "east", "southeast"}

// NPC is one placed NPC.
type NPC struct {
	// Name is the full record name, such as example.town:Healer#1.
	Name string
	// Mod added the NPC.
	Mod string
	// DisplayName is what players see: the display_name field, or the
	// name after the mod ID and before any "#".
	DisplayName string
	// Map is the map's asset name, such as map/prontera.
	Map  string
	X, Y int
	// Direction is 0 to 7, from Directions.
	Direction int
	// Sprite is the NPC sprite ID, as the protocol's Look.body.
	Sprite uint32
	// Script is the entry script that clicks will run, relative to the
	// folder of ScriptMod. NPC scripts run from phase 3 step 4.
	Script    string
	ScriptMod string
}

// npcFields are the fields of an NPC record.
var npcFields = []string{"map", "x", "y", "direction", "sprite", "script", "display_name"}

// NPCs merges the mods' NPC placements. sprite turns a sprite name, such
// as 4_F_KAFRA1, into its ID.
func (s *Set) NPCs(sprite func(name string) (uint32, bool)) ([]NPC, *Overlay, error) {
	o, err := s.Overlay(NPCKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var out []NPC
	var errs []error
	for _, r := range o.Records {
		n, err := s.npc(r, sprite)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: npcs %s: %w", r.Where, r.Name, err))
			continue
		}
		out = append(out, n)
	}
	return out, o, errors.Join(errs...)
}

func (s *Set) npc(r *Record, sprite func(string) (uint32, bool)) (NPC, error) {
	n := NPC{Name: r.Name, Mod: r.Mod}
	var errs []string
	for k := range r.Fields {
		if !slices.Contains(npcFields, k) {
			errs = append(errs, fmt.Sprintf("%q is not a field; NPCs have %s", k, strings.Join(npcFields, ", ")))
		}
	}
	str := func(k string) (string, bool) {
		v, ok := r.Fields[k].(string)
		return v, ok
	}
	num := func(k string) (int, bool) {
		v, ok := r.Fields[k].(int)
		return v, ok
	}
	if m, ok := str("map"); ok && m != "" && !strings.Contains(m, "/") {
		n.Map = "map/" + m
	} else if ok && strings.Contains(m, "/") {
		n.Map = m
	} else {
		errs = append(errs, "map is required, such as prontera")
	}
	var okX, okY bool
	n.X, okX = num("x")
	n.Y, okY = num("y")
	if !okX || !okY || n.X < 0 || n.Y < 0 || n.X > 1023 || n.Y > 1023 {
		errs = append(errs, "x and y are required: cells from 0 to 1023")
	}
	if v, ok := r.Fields["direction"]; ok {
		d, isText := v.(string)
		n.Direction = slices.Index(Directions, d)
		if !isText || n.Direction < 0 {
			errs = append(errs, fmt.Sprintf("direction is one of %s", strings.Join(Directions, ", ")))
			n.Direction = 0
		}
	}
	switch v := r.Fields["sprite"].(type) {
	case string:
		id, ok := sprite(v)
		if !ok {
			if num, err := strconv.ParseUint(v, 10, 32); err == nil {
				id, ok = uint32(num), true
			}
		}
		if !ok {
			errs = append(errs, fmt.Sprintf("sprite %q is not an NPC sprite name, such as 4_F_KAFRA1", v))
		}
		n.Sprite = id
	case int:
		if v <= 0 || v > 1<<31 {
			errs = append(errs, "sprite is an NPC sprite name or a positive number")
		}
		n.Sprite = uint32(v) // #nosec G115 -- checked above.
	default:
		errs = append(errs, "sprite is required: an NPC sprite name, such as 4_F_KAFRA1")
	}
	if d, ok := r.Fields["display_name"]; ok {
		name, isText := d.(string)
		if !isText || validLocalName(name) != nil {
			errs = append(errs, "display_name is 1 to 64 characters")
		}
		n.DisplayName = name
	} else {
		n.DisplayName, _, _ = strings.Cut(Local(r.Name), "#")
	}
	if v, ok := r.Fields["script"]; ok {
		p, isText := v.(string)
		if err := checkScriptPath(p); !isText || err != nil {
			errs = append(errs, fmt.Sprintf("script: %v", err))
		} else {
			n.Script, n.ScriptMod = p, r.SetBy["script"]
			owner := s.Mod(n.ScriptMod)
			if _, err := owner.readFile(p); errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Sprintf("script: %s has no %s", n.ScriptMod, p))
			}
		}
	}
	if len(errs) > 0 {
		return NPC{}, errors.New(strings.Join(errs, "; "))
	}
	return n, nil
}
