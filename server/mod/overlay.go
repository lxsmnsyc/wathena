package mod

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
)

// Kind is one kind of game data that mods overlay (strategy section
// 6.4), such as items.
type Kind struct {
	// Name is the kind in messages, such as "items".
	Name string
	// File is the overlay file in each mod, such as data/items.yml.
	File string
	// LocalNames lets a mod add a record with a name that has no prefix:
	// it becomes <mod ID>:<name>. Without it, a name without a prefix
	// must be a core record, so a typo in a core name is an error and
	// not a new record.
	LocalNames bool
	// Fixed are fields a mod cannot set on records it did not add, such
	// as the ID.
	Fixed []string
	// CoreNames lets core's own file add records with plain names, such
	// as max_hp, which mods then change by that name.
	CoreNames bool
	// Economy are fields that change the game's economy, such as an
	// item's prices or a monster's drops. Setting one on a record another
	// mod added, core's too, needs the permission data.core_override
	// (proposal 0002, review decision 52).
	Economy []string
}

// CoreOverride is the permission that changes the Economy fields of
// records that other mods added.
const CoreOverride = "data.core_override"

// Record is one record that mods added or changed. Core records that no
// mod touched are not in an Overlay.
type Record struct {
	// Name is the record's full name: a core name such as Red_Potion, or
	// a mod's name such as example.halloween:Pumpkin_Hat.
	Name string
	// Mod is the mod that added the record, or Core for a core record.
	Mod string
	// Fields are the values mods set, merged in load order.
	Fields map[string]any
	// SetBy names the mod that set each field last.
	SetBy map[string]string
	// Where is the file and line that added or first changed the record.
	Where string
}

// Overlay is the merged result of every mod's file of one kind.
type Overlay struct {
	Kind Kind
	// Records are in the order mods first touched them.
	Records []*Record
	// Notes are the warnings of a merge that is not strict, such as two
	// mods setting the same field.
	Notes []string

	byName map[string]*Record
}

// Record returns a record by full name, or nil when no mod touched it.
func (o *Overlay) Record(name string) *Record { return o.byName[name] }

// Overrides lists the Economy fields that a mod set on records other
// mods added, as "<kind> <record>: <field>, <field>", for wactl mod
// install. Such a mod needs data.core_override.
func (o *Overlay) Overrides(modID string) []string {
	if o == nil {
		return nil
	}
	var out []string
	for _, r := range o.Records {
		if r.Mod == modID {
			continue
		}
		var keys []string
		for _, k := range o.Kind.Economy {
			if r.SetBy[k] == modID {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			out = append(out, fmt.Sprintf("%s %s: %s", o.Kind.Name, r.Name, strings.Join(keys, ", ")))
		}
	}
	return out
}

// Overlay merges every loaded mod's file of a kind, in load order. core
// reports whether core has a record of a name.
//
// Rules:
//   - A mod adds records named <its own ID>:<name>, or changes fields of
//     records that exist: core's, and those of mods that loaded before.
//   - A mod never deletes a record. There is no way to write a deletion.
//   - When a mod sets a field that an earlier mod set, the later mod
//     wins and a note is added. In strict mode it is an error.
func (s *Set) Overlay(k Kind, core func(name string) bool) (*Overlay, error) {
	o := &Overlay{Kind: k, byName: map[string]*Record{}}
	var errs []error
	for _, m := range s.Mods {
		data, err := m.readFile(k.File)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.file(k.File), err)
		}
		if err := o.merge(m, data, core, s.Strict); err != nil {
			errs = append(errs, err)
		}
	}
	return o, errors.Join(errs...)
}

// merge applies one mod's file.
func (o *Overlay) merge(m *Mod, data []byte, core func(string) bool, strict bool) error {
	file := m.file(o.Kind.File)
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	if root.Kind == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s: line %d: want a list of records, each with a name", file, doc.Line)
	}
	var errs []error
	bad := func(line int, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: line %d: %s", file, line, fmt.Sprintf(format, args...)))
	}
	mine := map[string]bool{}
	for _, n := range doc.Content {
		if n.Kind != yaml.MappingNode {
			bad(n.Line, "want a record: a mapping with a name")
			continue
		}
		var name string
		fields := map[string]*yaml.Node{}
		var order []string
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i].Value, n.Content[i+1]
			if key == "name" {
				name = val.Value
				continue
			}
			if _, dup := fields[key]; dup {
				bad(n.Content[i].Line, "%s is set twice", key)
			}
			fields[key] = val
			order = append(order, key)
		}
		if name == "" {
			bad(n.Line, "the record has no name")
			continue
		}
		full, err := o.resolve(m.ID(), name, core)
		if err != nil {
			bad(n.Line, "%v", err)
			continue
		}
		if mine[full] {
			bad(n.Line, "%s is listed twice in this file", full)
			continue
		}
		mine[full] = true
		r := o.byName[full]
		if r == nil {
			owner := Core
			if p, _, ok := splitName(full); ok {
				owner = p
			}
			r = &Record{Name: full, Mod: owner, Fields: map[string]any{}, SetBy: map[string]string{}, Where: fmt.Sprintf("%s: line %d", file, n.Line)}
			o.byName[full] = r
			o.Records = append(o.Records, r)
		}
		for _, key := range order {
			if r.Mod != m.ID() && slices.Contains(o.Kind.Fixed, key) {
				bad(fields[key].Line, "%s: %s cannot be changed on a record the mod did not add", full, key)
				continue
			}
			if r.Mod != m.ID() && slices.Contains(o.Kind.Economy, key) && !m.Grants.Has(CoreOverride) {
				bad(fields[key].Line, "%s: changing %s of a record that %s added needs the permission %s", full, key, r.Mod, CoreOverride)
				continue
			}
			var v any
			if err := fields[key].Decode(&v); err != nil {
				bad(fields[key].Line, "%s: %v", key, err)
				continue
			}
			// Changing what core set is what mods are for, so only a
			// field another mod set gets a note.
			if prev, ok := r.SetBy[key]; ok && prev != m.ID() && prev != Core {
				msg := fmt.Sprintf("%s %s: %s sets %s, which %s set before; %s wins, as it loads later", o.Kind.Name, full, m.ID(), key, prev, m.ID())
				if strict {
					bad(fields[key].Line, "%s (strict mode makes this an error)", msg)
					continue
				}
				o.Notes = append(o.Notes, msg)
			}
			r.Fields[key] = v
			r.SetBy[key] = m.ID()
		}
	}
	return errors.Join(errs...)
}

// resolve turns a name in a mod's file into a record's full name.
func (o *Overlay) resolve(modID, name string, core func(string) bool) (string, error) {
	if owner, local, ok := splitName(name); ok {
		if err := validLocalName(local); err != nil {
			return "", err
		}
		if owner == modID {
			return name, nil
		}
		if o.byName[name] == nil || o.byName[name].Mod != owner {
			return "", fmt.Errorf("%s: %s has not added this record, or loads after %s; list it in depends or load_after", name, owner, modID)
		}
		return name, nil
	}
	if core(name) {
		return name, nil
	}
	if o.Kind.CoreNames {
		if modID == Core {
			if err := validLocalName(name); err != nil {
				return "", err
			}
			return name, nil
		}
		if r := o.byName[name]; r != nil && r.Mod == Core {
			return name, nil
		}
	}
	if !o.Kind.LocalNames {
		return "", fmt.Errorf("%s is not a core record of %s; a new record is named with the mod's ID, such as %s:%s", name, o.Kind.Name, modID, name)
	}
	if err := validLocalName(name); err != nil {
		return "", err
	}
	return modID + ":" + name, nil
}

// validLocalName checks the part of a record name after the mod ID.
func validLocalName(s string) error {
	if s == "" || len(s) > 64 || strings.TrimSpace(s) != s {
		return fmt.Errorf("%q is not a record name: 1 to 64 characters, without spaces at the ends", s)
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == ':' {
			return fmt.Errorf("%q is not a record name: it has a control character or a colon", s)
		}
	}
	return nil
}

// Local returns the part of a full record name after the mod ID.
func Local(name string) string {
	_, local, _ := splitName(name)
	return local
}
