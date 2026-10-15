package mod

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
)

// ItemKind is the item overlay: data/items.yml. Fields are the names of
// the item extract (server/item.Def), such as weight and stackable, and
// script, bonuses and unsupported. A record's name is its AegisName.
var ItemKind = Kind{Name: "items", File: "data/items.yml", Fixed: []string{"id", "aegis_name"}}

// itemTypes are item_db's types, in lower case.
var itemTypes = []string{"healing", "usable", "etc", "armor", "weapon", "card", "petegg", "petarmor", "ammo", "delayconsume", "shadowgear", "cash"}

// Items is the item database with every mod's overlay applied.
type Items struct {
	Data *item.Data
	// IDs maps every item's name to its ID: core AegisNames and mods'
	// full names.
	IDs map[string]uint32
	// Overlay is the merge, with its notes.
	Overlay *Overlay
	// Art is the art that records name, by item ID (decision 0033).
	Art map[uint32]ItemArt
	// Descriptions are the description fields of records, by item ID, as
	// lines (review decision 40). The client shows them. The server does
	// not use them.
	Descriptions map[uint32][]string
}

// MaxItemDescription is the longest description field of an item, in
// bytes.
const MaxItemDescription = 4096

// itemDescription reads an item's description field: text of 1 to
// MaxItemDescription bytes. Lines are split at line breaks, and trailing
// space and empty lines at the end go.
func itemDescription(v any) ([]string, error) {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" || len(s) > MaxItemDescription || !utf8.ValidString(s) {
		return nil, fmt.Errorf("description is text of 1 to %d bytes", MaxItemDescription)
	}
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

// ItemArt is the art an item record names: asset names of a mod's art
// or of core's. Empty fields keep the default name, such as
// icon/item/<id>.
type ItemArt struct {
	Icon       string `json:"icon,omitempty"`
	Image      string `json:"image,omitempty"`
	DropSprite string `json:"drop_sprite,omitempty"`
	ViewSprite string `json:"view_sprite,omitempty"`
}

// itemArtFields are an item's art fields and the type each names.
var itemArtFields = map[string]string{"icon": "texture", "image": "texture", "drop_sprite": "sprite", "view_sprite": "sprite"}

// Items applies the mods' item overlays to the core items. New items get
// IDs in the mod range from ids. base is not changed. bonusNames resolves the
// names in static bonuses, which are checked when it is set.
func (s *Set) Items(ctx context.Context, base *item.Data, ids *IDs, bonusNames bonus.Names) (*Items, error) {
	byName := make(map[string]int, len(base.Items))
	for i := range base.Items {
		byName[base.Items[i].AegisName] = i
	}
	o, err := s.Overlay(ItemKind, func(name string) bool { _, ok := byName[name]; return ok })
	if err != nil {
		return nil, err
	}
	out := &item.Data{Mode: base.Mode, Items: append([]item.Def(nil), base.Items...)}
	art := map[uint32]ItemArt{}
	descriptions := map[uint32][]string{}
	var errs []error
	for _, r := range o.Records {
		var d item.Def
		if i, ok := byName[r.Name]; ok {
			d = out.Items[i]
		} else {
			id, err := ids.ID(ctx, ItemIDs, r.Name, r.Mod)
			if err != nil {
				return nil, err
			}
			d = item.Def{ID: id, AegisName: r.Name, Type: "Etc"}
		}
		fields := r.Fields
		// Art fields are not item data: they are kept apart for the
		// client.
		var a ItemArt
		hasArt := false
		for k, typ := range itemArtFields {
			v, ok := fields[k]
			if !ok {
				continue
			}
			if !hasArt {
				fields = maps.Clone(fields)
				hasArt = true
			}
			delete(fields, k)
			name, err := ArtName(v, typ)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: items %s: %s: %w", r.Where, r.Name, k, err))
				continue
			}
			switch k {
			case "icon":
				a.Icon = name
			case "image":
				a.Image = name
			case "drop_sprite":
				a.DropSprite = name
			case "view_sprite":
				a.ViewSprite = name
			}
		}
		if hasArt {
			art[d.ID] = a
		}
		if v, ok := fields["description"]; ok {
			fields = maps.Clone(fields)
			delete(fields, "description")
			lines, err := itemDescription(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: items %s: %w", r.Where, r.Name, err))
				continue
			}
			descriptions[d.ID] = lines
		}
		if v, ok := fields["display_name"]; ok {
			// The record's name is its key, so the name players see is
			// display_name, as for NPCs.
			fields = maps.Clone(fields)
			delete(fields, "display_name")
			fields["name"] = v
		}
		// jobs is rAthena's three masks, or a list of job names, which
		// server/gamedata resolves once the jobs are loaded.
		names, isNames := jobNameList(fields["jobs"])
		if isNames {
			fields = maps.Clone(fields)
			delete(fields, "jobs")
		}
		if err := applyFields(&d, fields); err != nil {
			errs = append(errs, fmt.Errorf("%s: items %s: %w", r.Where, r.Name, err))
			continue
		}
		if isNames {
			d.JobNames, d.JobIDs, d.Jobs = names, nil, &item.JobMask{}
			d.JobRestricted = true
		}
		// restrictions and the older no_drop and no_sell fields agree.
		// A record that sets restrictions sets the whole list.
		if _, ok := fields["restrictions"]; ok {
			d.SyncRestrictions(true)
		} else if _, ok := fields["no_drop"]; ok {
			d.SyncRestrictions(false)
		} else if _, ok := fields["no_sell"]; ok {
			d.SyncRestrictions(false)
		}
		if err := checkRollNames(fields["rolls"]); err != nil {
			errs = append(errs, fmt.Errorf("%s: items %s: %w", r.Where, r.Name, err))
			continue
		}
		if _, ok := r.Fields["script"]; ok {
			// A script path belongs to the mod that set it.
			if err := checkScriptPath(d.Script); err != nil {
				errs = append(errs, fmt.Errorf("%s: items %s: script: %w", r.Where, r.Name, err))
				continue
			}
			d.Script = r.SetBy["script"] + ":" + d.Script
		}
		if err := checkItem(&d); err != nil {
			errs = append(errs, fmt.Errorf("%s: items %s: %w", r.Where, r.Name, err))
			continue
		}
		if bonusNames != nil {
			d.Static = nil
			for i, entry := range d.Bonuses {
				b, err := bonus.FromData(entry, bonusNames)
				if err != nil {
					errs = append(errs, fmt.Errorf("%s: items %s: bonuses[%d]: %w", r.Where, r.Name, i, err))
					continue
				}
				for _, name := range b.Rolls() {
					if d.Rolls[name] == nil {
						errs = append(errs, fmt.Errorf("%s: items %s: bonuses[%d]: the item has no roll %q", r.Where, r.Name, i, name))
					}
				}
				d.Static = append(d.Static, b)
			}
		}
		if i, ok := byName[r.Name]; ok {
			out.Items[i] = d
		} else {
			byName[r.Name] = len(out.Items)
			out.Items = append(out.Items, d)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	names := make(map[string]uint32, len(out.Items))
	for _, d := range out.Items {
		names[d.AegisName] = d.ID
	}
	return &Items{Data: out, IDs: names, Overlay: o, Art: art, Descriptions: descriptions}, nil
}

// jobNameList reads an item's jobs field as a list of job names, such as
// [knight, example.jobs:Desperado]. Three decimal strings are rAthena's
// masks, not names.
func jobNameList(v any) ([]string, bool) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	var names []string
	numbers := 0
	for _, e := range list {
		s, ok := e.(string)
		if !ok {
			return nil, false
		}
		if _, err := strconv.ParseUint(s, 10, 64); err == nil {
			numbers++
		}
		names = append(names, s)
	}
	if numbers == len(list) {
		return nil, false
	}
	return names, true
}

// applyFields sets fields on a record through its JSON form, so the
// field names and types are the extract's. Unknown fields are errors.
func applyFields[T any](rec *T, fields map[string]any) error {
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for k, v := range fields {
		if _, ok := m[k]; !ok {
			return fmt.Errorf("%q is not a field", k)
		}
		m[k] = v
	}
	if b, err = json.Marshal(m); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var out T
	if err := dec.Decode(&out); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return fmt.Errorf("%s: want %s, got %s", te.Field, te.Type, te.Value)
		}
		return err
	}
	*rec = out
	return nil
}

// maxItemSlots is the most card slots item data may give: the most the
// database stores (strategy section 6.8). The world's MAX_SLOTS, 4 by
// default, limits how many are used.
const maxItemSlots = 16

// checkItem checks the values of an item.
func checkItem(d *item.Def) error {
	var errs []string
	if !containsFold(itemTypes, d.Type) {
		errs = append(errs, fmt.Sprintf("type %q is not one of %s", d.Type, strings.Join(itemTypes, ", ")))
	}
	if d.Weight < 0 || d.StackLimit < 0 || d.DelayMs < 0 || d.MinLevel < 0 || d.MaxLevel < 0 {
		errs = append(errs, "weight, stack_limit, delay_ms, min_level and max_level cannot be negative")
	}
	if d.Buy < 0 || d.Sell < 0 || d.Buy > item.MaxZeny || d.Sell > item.MaxZeny {
		errs = append(errs, fmt.Sprintf("buy and sell are 0 to %d zeny", item.MaxZeny))
	}
	if len(d.Name) > 255 {
		errs = append(errs, "name is at most 255 bytes")
	}
	if d.StackLimit > item.MaxAmount {
		errs = append(errs, fmt.Sprintf("stack_limit is above %d", item.MaxAmount))
	}
	switch d.Gender {
	case "", "Male", "Female":
	default:
		errs = append(errs, fmt.Sprintf("gender %q is not Male, Female, or empty", d.Gender))
	}
	if d.Atk < 0 || d.Matk < 0 || d.Defense < 0 || d.Range < 0 || d.View < 0 || d.Subtype < 0 {
		errs = append(errs, "atk, matk, def, range, view and subtype cannot be negative")
	}
	if d.Slots < 0 || d.Slots > maxItemSlots {
		errs = append(errs, fmt.Sprintf("slots is 0 to %d", maxItemSlots))
	}
	if d.WeaponLevel < 0 || d.WeaponLevel > 5 || d.ArmorLevel < 0 || d.ArmorLevel > 2 {
		errs = append(errs, "weapon_level is 0 to 5, and armor_level 0 to 2")
	}
	if uint32(d.Location) > item.EqpShadowGear|item.EqpAmmo|0x3fff {
		errs = append(errs, "location has a position that does not exist")
	}
	for _, r := range d.Restrictions {
		if _, err := item.ParseRestriction(r); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(d.Rolls) > item.MaxRolls {
		errs = append(errs, fmt.Sprintf("an item has at most %d rolls", item.MaxRolls))
	}
	for _, name := range d.RollNames() {
		r := d.Rolls[name]
		if !rollName.MatchString(name) {
			errs = append(errs, fmt.Sprintf("roll %q: a roll name is lower-case letters, digits and _, starting with a letter, at most 32 bytes", name))
			continue
		}
		if r == nil {
			errs = append(errs, fmt.Sprintf("roll %s: want min, max, default and distribution", name))
			continue
		}
		if err := r.Check(); err != nil {
			errs = append(errs, fmt.Sprintf("roll %s: %v", name, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// checkRollNames refuses an empty display_name in the rolls field of an
// item record, which the item data cannot tell from no display_name.
func checkRollNames(rolls any) error {
	m, _ := rolls.(map[string]any)
	for _, name := range slices.Sorted(maps.Keys(m)) {
		r, _ := m[name].(map[string]any)
		if v, ok := r["display_name"]; ok && v == "" {
			return fmt.Errorf("roll %s: display_name is 1 to %d bytes", name, item.MaxRollDisplayName)
		}
	}
	return nil
}

// rollName is the form of a roll's name.
var rollName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
