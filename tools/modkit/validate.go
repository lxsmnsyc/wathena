package modkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript/luaapi"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// Report is what wactl mod validate found.
type Report struct {
	Manifest *mod.Manifest
	// Problems are errors, each with its file and line or field when
	// known. A mod with problems does not load.
	Problems []string
	// Notes are warnings that do not stop the mod.
	Notes []string
	// Rulesets are the core rulesets the mod loads on.
	Rulesets []string
	// Permissions are the requested permissions.
	Permissions []mod.Permission
	// Files counts the Lua files checked, and Cases the test cases.
	Files, Cases int
	// Records are the records the mod adds or changes, merged.
	Records []Record
	// Loaded is the mod merged with core, when it loads.
	Loaded *Loaded
}

// Record is a record a mod touches, with its final value.
type Record struct {
	Kind, Name string
	// AddedBy is the mod that added the record, or core.
	AddedBy string
	Where   string
	Fields  []RecordField
}

// RecordField is one field of a record and the mod that set it last:
// a mod ID, core for core's data, or "" for a default.
type RecordField struct {
	Name  string
	Value any
	SetBy string
}

// Validate checks a mod (strategy section 6.9): the manifest, the data
// files and their IDs, the rulesets it loads on, its permissions, the
// Lua syntax of every file, that entry scripts return a function, that
// required modules exist, and that test files register cases.
func (e *Env) Validate(ctx context.Context, p, depsDir string) *Report {
	rep := &Report{}
	bad := func(format string, args ...any) { rep.Problems = append(rep.Problems, fmt.Sprintf(format, args...)) }
	m, err := mod.Read(p)
	if err != nil {
		addErrors(rep, err)
		return rep
	}
	rep.Manifest = m.Manifest
	for _, name := range m.Manifest.Permissions {
		perm, _ := mod.LookupPermission(name)
		rep.Permissions = append(rep.Permissions, perm)
	}

	files, err := luaFiles(m)
	if err != nil {
		bad("%s: %v", p, err)
		return rep
	}
	rep.Files = len(files)
	problems, err := CheckSyntax(files)
	if err != nil {
		bad("%v", err)
	}
	rep.Problems = append(rep.Problems, problems...)

	// The rulesets it loads on, and the base to check it with. A ruleset
	// mod is the world's ruleset itself, on its own base.
	base := ""
	if m.Manifest.Provides.Ruleset != nil {
		s, err := mod.Resolve([]*mod.Mod{m}, mod.Config{Ruleset: m.ID(), Approve: map[string][]string{m.ID(): m.Manifest.Permissions}})
		if err == nil {
			base = s.Ruleset.Base
			rep.Rulesets = append(rep.Rulesets, m.ID()+" (base "+base+")")
		}
	}
	for _, b := range []string{mod.Renewal, mod.PreRenewal} {
		if m.Manifest.Provides.Ruleset != nil {
			break
		}
		// Other problems, such as a dependency in another folder, are
		// reported below.
		_, err := mod.Resolve([]*mod.Mod{m}, mod.Config{Base: b, Approve: map[string][]string{m.ID(): m.Manifest.Permissions}})
		if err == nil || !strings.Contains(err.Error(), "is refused on the ruleset") {
			rep.Rulesets = append(rep.Rulesets, "core.ruleset."+b)
			if base == "" {
				base = b
			}
		}
	}
	if base == "" {
		base = mod.Renewal
	}
	logs := &capture{}
	l, err := e.Load(ctx, p, depsDir, base, slog.New(logs))
	rep.Notes = append(rep.Notes, logs.records...)
	if err != nil {
		addErrors(rep, err)
		return rep
	}
	rep.Loaded = l
	checkArt(l, rep)
	checkRequires(l, files, rep)
	e.checkEntries(l, rep)
	rep.Records = records(l)
	return rep
}

// addErrors adds each line of a joined error as a problem.
func addErrors(rep *Report, err error) {
	for line := range strings.SplitSeq(err.Error(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			rep.Problems = append(rep.Problems, line)
		}
	}
}

// luaFiles reads every .lua file of a mod, by path.
func luaFiles(m *mod.Mod) (map[string]string, error) {
	fsys, closeFS, err := m.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeFS() }()
	out := map[string]string{}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != "." && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if d.IsDir() || path.Ext(p) != ".lua" {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		out[p] = string(b)
		return nil
	})
	return out, err
}

// CheckSyntax compiles Lua files, by path, without running them, and
// returns their syntax errors with file and line.
func CheckSyntax(files map[string]string) ([]string, error) {
	s, err := script.New(script.Options{})
	if err != nil {
		return nil, err
	}
	defer s.Close()
	res, err := s.Load("=syntax", "return function(src, name) local _, err = load(src, name, 't') return err end")
	if err != nil {
		return nil, err
	}
	check, err := s.Keep(res[0])
	if err != nil {
		return nil, err
	}
	defer check.Release()
	var out []string
	for _, p := range slices.Sorted(maps.Keys(files)) {
		v, err := s.Call1(check.Value(), script.String(files[p]), script.String("@"+p))
		if err != nil {
			return nil, err
		}
		if msg, ok := v.AsString(); ok {
			out = append(out, msg)
		}
	}
	return out, nil
}

var requirePattern = regexp.MustCompile(`\brequire\s*\(?\s*["']([^"'\n]+)["']`)

// checkRequires checks that every module a file requires by a constant
// name exists: an API module, or a file of the mod or of a mod in its
// depends.
func checkRequires(l *Loaded, files map[string]string, rep *Report) {
	id := l.Mod.ID()
	visible := l.Set.Visible(id)
	var all []string
	for _, m := range l.Set.Mods {
		all = append(all, m.ID())
	}
	scripts := map[string]*mod.Scripts{}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		src := files[p]
		for _, loc := range requirePattern.FindAllStringSubmatchIndex(src, -1) {
			name := src[loc[2]:loc[3]]
			line := 1 + strings.Count(src[:loc[0]], "\n")
			where := fmt.Sprintf("%s:%d", p, line)
			if strings.HasPrefix(name, "wa.") {
				m, ok := luaapi.ModuleByName(name)
				switch {
				case !ok:
					rep.Problems = append(rep.Problems, fmt.Sprintf("%s: there is no API module %s", where, name))
				case m.Testing && !strings.HasPrefix(p, "tests/"):
					rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %s exists only in tests under tests/", where, name))
				}
				continue
			}
			owner := longest(name, visible)
			if owner == "" {
				if other := longest(name, all); other != "" {
					rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %s cannot require %s: %s is not in its depends", where, id, name, other))
				} else {
					rep.Problems = append(rep.Problems, fmt.Sprintf("%s: no mod has the module %s; mod files are named <mod ID>.<path>, such as %s.lib.rewards for scripts/lib/rewards.lua", where, name, id))
				}
				continue
			}
			file, ok := mod.ModulePath(strings.TrimPrefix(name, owner+"."))
			if !ok {
				rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %s is not a module name", where, name))
				continue
			}
			sc := scripts[owner]
			if sc == nil {
				var err error
				if sc, err = l.Set.Scripts(owner); err != nil {
					rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %v", where, err))
					continue
				}
				scripts[owner] = sc
			}
			if _, ok := sc.Files[file]; !ok {
				rep.Problems = append(rep.Problems, fmt.Sprintf("%s: module %s not found: %s has no %s", where, name, owner, file))
			}
		}
	}
}

func longest(name string, ids []string) string {
	best := ""
	for _, id := range ids {
		if strings.HasPrefix(name, id+".") && len(id) > len(best) {
			best = id
		}
	}
	return best
}

// checkEntries runs the mod's entry scripts in a headless world, and
// loads its NPC and item scripts and its test files.
func (e *Env) checkEntries(l *Loaded, rep *Report) {
	id := l.Mod.ID()
	logs := &capture{}
	w, err := e.newWorld(l, slog.New(logs))
	if err != nil {
		rep.Problems = append(rep.Problems, err.Error())
		return
	}
	defer w.h.Close()
	// The map of the mod's first NPC, so its setup sees a map it uses.
	where := DefaultMap
	for _, asset := range slices.Sorted(maps.Keys(l.NPCs)) {
		if slices.ContainsFunc(l.NPCs[asset], func(n zone.NPC) bool { return strings.HasPrefix(n.Name, id+":") }) {
			where = strings.TrimPrefix(asset, "map/")
			break
		}
	}
	if err := w.bind(where); err != nil {
		addErrors(rep, err)
	}
	r := w.h.Runtime()
	var entries []string
	for _, list := range l.NPCs {
		for _, n := range list {
			if strings.HasPrefix(n.Script, id+":") && !slices.Contains(entries, n.Script) {
				entries = append(entries, n.Script)
			}
		}
	}
	for _, d := range l.Items.Data.Items {
		if strings.HasPrefix(d.Script, id+":") && !slices.Contains(entries, d.Script) {
			entries = append(entries, d.Script)
		}
	}
	if l.Skills != nil {
		for _, d := range l.Skills.All() {
			if strings.HasPrefix(d.Script, id+":") && !slices.Contains(entries, d.Script) {
				entries = append(entries, d.Script)
			}
		}
	}
	slices.Sort(entries)
	for _, entry := range entries {
		if err := r.CheckEntry(entry); err != nil {
			rep.Problems = append(rep.Problems, err.Error())
		}
	}
	files, names, err := TestFiles(l.Mod)
	if err != nil {
		rep.Problems = append(rep.Problems, err.Error())
		return
	}
	for _, f := range names {
		cases, err := r.LoadTests(id, f, files[f])
		if err != nil {
			rep.Problems = append(rep.Problems, err.Error())
			continue
		}
		rep.Cases += len(cases)
	}
	rep.Problems = append(rep.Problems, logs.about(id)...)
}

// records lists the records the mod adds or changes, with their final
// values and the mod that set each field.
func records(l *Loaded) []Record {
	id := l.Mod.ID()
	var out []Record
	byName := map[string]*item.Def{}
	for i := range l.Items.Data.Items {
		byName[l.Items.Data.Items[i].AegisName] = &l.Items.Data.Items[i]
	}
	for _, r := range l.Items.Overlay.Records {
		if !touches(r, id) {
			continue
		}
		rec := Record{Kind: "items", Name: r.Name, AddedBy: r.Mod, Where: r.Where}
		var fields map[string]any
		if d := byName[r.Name]; d != nil {
			b, _ := json.Marshal(d)
			dec := json.NewDecoder(bytes.NewReader(b))
			dec.UseNumber()
			_ = dec.Decode(&fields)
		}
		for _, k := range slices.Sorted(maps.Keys(fields)) {
			by, set := r.SetBy[k]
			if k == "name" && !set {
				// display_name in the file sets the item's name.
				by, set = r.SetBy["display_name"]
			}
			if !set {
				// Empty fields that no mod set are left out. Other
				// fields are core's, or a new record's defaults.
				if isZero(fields[k]) {
					continue
				}
				switch {
				case r.Mod == mod.Core:
					by = mod.Core
				case k == "id" || k == "aegis_name":
					by = "server"
				}
			}
			rec.Fields = append(rec.Fields, RecordField{Name: k, Value: fields[k], SetBy: by})
		}
		out = append(out, rec)
	}
	var jobOverlay *mod.Overlay
	if l.Jobs != nil {
		jobOverlay = l.Jobs.Overlay
	}
	overlays := append([]*mod.Overlay{jobOverlay, l.NPCOverlay, l.MonsterOverlay, l.EventOverlay, l.SkillOverlay}, l.CompanionOverlays...)
	overlays = append(overlays, l.ChatOverlays...)
	for _, o := range overlays {
		if o == nil {
			continue
		}
		for _, r := range o.Records {
			if !touches(r, id) {
				continue
			}
			rec := Record{Kind: o.Kind.Name, Name: r.Name, AddedBy: r.Mod, Where: r.Where}
			if o.Kind.Name == mod.MonsterKind.Name {
				if mid, ok := l.Zone.MonsterIDs[strings.ToUpper(r.Name)]; ok {
					rec.Fields = append(rec.Fields, RecordField{Name: "id", Value: json.Number(fmt.Sprint(mid)), SetBy: "server"})
				}
			}
			if o.Kind.Name == mod.JobKind.Name && r.Mod != mod.Core {
				if id, ok := l.Jobs.ID(r.Name); ok {
					rec.Fields = append(rec.Fields, RecordField{Name: "id", Value: json.Number(fmt.Sprint(id)), SetBy: "server"})
				}
			}
			if o.Kind.Name == mod.SkillKind.Name && r.Mod != mod.Core {
				if d := l.Skills.ByName(r.Name); d != nil {
					rec.Fields = append(rec.Fields, RecordField{Name: "id", Value: json.Number(fmt.Sprint(d.ID)), SetBy: "server"})
				}
			}
			if (o.Kind.Name == mod.CompanionKind.Name || o.Kind.Name == mod.MountKind.Name) && r.Mod != mod.Core {
				t := &l.Companions.Companions
				if o.Kind.Name == mod.MountKind.Name {
					t = &l.Companions.Mounts
				}
				if c := t.ByName(r.Name); c != nil {
					rec.Fields = append(rec.Fields, RecordField{Name: "id", Value: json.Number(fmt.Sprint(c.ID)), SetBy: "server"})
				}
			}
			for _, k := range slices.Sorted(maps.Keys(r.Fields)) {
				rec.Fields = append(rec.Fields, RecordField{Name: k, Value: r.Fields[k], SetBy: r.SetBy[k]})
			}
			out = append(out, rec)
		}
	}
	return out
}

func touches(r *mod.Record, id string) bool {
	if r.Mod == id {
		return true
	}
	for _, by := range r.SetBy {
		if by == id {
			return true
		}
	}
	return false
}

func isZero(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case bool:
		return !v
	case json.Number:
		return v.String() == "0"
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

// Err returns the problems as one error, or nil.
func (r *Report) Err() error {
	if len(r.Problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(r.Problems, "\n"))
}
