package mod

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/rules"
	"go.yaml.in/yaml/v3"
)

// ManifestFile is the name of a mod's manifest, at the root of its folder.
const ManifestFile = "mod.yaml"

// API is the scripting API version this server offers (proposal 0002,
// "Version", and strategy section 6.12). A mod declares `api: 1.0`, or a
// range that includes it, such as ">=1.0 <2.0".
const API = "1.0"

// apiVersion is API as a version, which a manifest's range must include.
var apiVersion = Version{Major: 1}

// Manifest is a mod's mod.yaml (strategy section 6.3).
type Manifest struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Version     string       `yaml:"version"`
	API         string       `yaml:"api"`
	Description string       `yaml:"description"`
	Authors     []Author     `yaml:"authors"`
	License     string       `yaml:"license"`
	Depends     []Dependency `yaml:"depends"`
	LoadAfter   []string     `yaml:"load_after"`
	Supports    struct {
		Rulesets *RulesetSupport `yaml:"rulesets"`
	} `yaml:"supports"`
	Provides struct {
		Ruleset *RulesetSpec `yaml:"ruleset"`
	} `yaml:"provides"`
	Permissions []string `yaml:"permissions"`
	// Scripts are the mod's own entry scripts, which run once per zone
	// with a mod context. Paths are relative to the mod's folder, under
	// scripts/.
	Scripts []string `yaml:"scripts"`

	version Version
}

// Author is one author of a mod.
type Author struct {
	Name  string `yaml:"name"`
	Email string `yaml:"email"`
	URL   string `yaml:"url"`
}

// Dependency is a mod that must load first, with the versions that work.
type Dependency struct {
	ID      string `yaml:"id"`
	Version string `yaml:"version"`

	versions Range
}

// RulesetSupport lists the rulesets a mod works with: the bases, and the
// values each named flag may have.
type RulesetSupport struct {
	Base  []string              `yaml:"base"`
	Flags map[string]flagChoice `yaml:"flags"`
}

// RulesetSpec is what a ruleset mod provides: the ruleset it starts from,
// and the flags it changes.
type RulesetSpec struct {
	Extends string               `yaml:"extends"`
	Flags   map[string]flagValue `yaml:"flags"`
	// Limits and Rules set the limits that rAthena compiles in and the
	// values of its conf files (proposal 0002, review decision 27). A
	// ruleset mod that does not set one keeps what it extends.
	Limits map[string]flagValue `yaml:"limits"`
	Rules  map[string]flagValue `yaml:"rules"`
}

// flagValue is a flag's value. YAML 1.1 readers turn on and off into
// booleans, so true and false are read as on and off.
type flagValue string

func (f *flagValue) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: a flag value must be a single word", n.Line)
	}
	*f = flagValue(scalarFlag(n))
	return nil
}

// flagChoice is one flag value or a list of them.
type flagChoice []string

func (f *flagChoice) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*f = flagChoice{scalarFlag(n)}
		return nil
	case yaml.SequenceNode:
		*f = nil
		for _, c := range n.Content {
			if c.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: a flag value must be a single word", c.Line)
			}
			*f = append(*f, scalarFlag(c))
		}
		return nil
	}
	return fmt.Errorf("line %d: want a flag value or a list of them", n.Line)
}

func scalarFlag(n *yaml.Node) string {
	if n.Tag == "!!bool" {
		switch strings.ToLower(n.Value) {
		case "true", "on", "yes":
			return "on"
		default:
			return "off"
		}
	}
	return n.Value
}

// idPattern is a mod ID: lowercase words of letters, digits, "_" and
// "-", separated by dots, such as example.halloween-event.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)*$`)

// validID checks a mod ID's form.
func validID(id string) error {
	switch {
	case id == "":
		return errors.New("is required")
	case len(id) > 64:
		return fmt.Errorf("%q is longer than 64 characters", id)
	case !idPattern.MatchString(id):
		return fmt.Errorf("%q is not a mod ID: lowercase words of letters, digits, _ and -, separated by dots, such as example.halloween-event", id)
	}
	return nil
}

// ParseManifest reads and checks a mod.yaml. file names it in errors.
// Every problem is listed, each with its field.
func ParseManifest(file string, data []byte) (*Manifest, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: the manifest is empty", file)
		}
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	var errs []error
	bad := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s: %s", file, field, fmt.Sprintf(format, args...)))
	}
	if err := validID(m.ID); err != nil {
		bad("id", "%v", err)
	} else if m.ID == "core" || strings.HasPrefix(m.ID, "core.") || m.ID == "wa" || strings.HasPrefix(m.ID, "wa.") {
		bad("id", "%q is reserved for the server", m.ID)
	}
	if strings.TrimSpace(m.Name) == "" {
		bad("name", "is required")
	}
	if m.Version == "" {
		bad("version", "is required, such as 1.0.0")
	} else if v, err := ParseVersion(m.Version); err != nil {
		bad("version", "%v", err)
	} else {
		m.version = v
	}
	switch m.API {
	case "":
		bad("api", "is required; this server's API is %s, so use api: %s", API, API)
	case "experimental":
		bad("api", "experimental ended with API %s; use api: %s", API, API)
	default:
		if _, err := ParseRange(m.API); err != nil {
			bad("api", "%v", err)
		}
	}
	for i, a := range m.Authors {
		if strings.TrimSpace(a.Name) == "" {
			bad(fmt.Sprintf("authors[%d].name", i), "is required")
		}
	}
	seen := map[string]bool{}
	for i := range m.Depends {
		d := &m.Depends[i]
		field := fmt.Sprintf("depends[%d]", i)
		if err := validID(d.ID); err != nil {
			bad(field+".id", "%v", err)
			continue
		}
		if seen[d.ID] {
			bad(field+".id", "%s is listed twice", d.ID)
		}
		seen[d.ID] = true
		if d.ID == m.ID {
			bad(field+".id", "a mod cannot depend on itself")
		}
		v := d.Version
		if v == "" {
			v = ">=0.0.0-0"
		}
		r, err := ParseRange(v)
		if err != nil {
			bad(field+".version", "%v", err)
		}
		d.versions = r
	}
	for i, id := range m.LoadAfter {
		if err := validID(id); err != nil {
			bad(fmt.Sprintf("load_after[%d]", i), "%v", err)
		}
	}
	if s := m.Supports.Rulesets; s != nil {
		for i, b := range s.Base {
			if b != Renewal && b != PreRenewal {
				bad(fmt.Sprintf("supports.rulesets.base[%d]", i), "%q is not %s or %s", b, Renewal, PreRenewal)
			}
		}
		for name, values := range s.Flags {
			if len(values) == 0 {
				bad("supports.rulesets.flags."+name, "lists no values")
			}
			for _, v := range values {
				if err := checkFlag(name, v); err != nil {
					bad("supports.rulesets.flags", "%v", err)
				}
			}
		}
	}
	if p := m.Provides.Ruleset; p != nil {
		if err := validID(p.Extends); err != nil {
			bad("provides.ruleset.extends", "%v; name the ruleset mod this one starts from, such as %s", err, CoreRenewal)
		}
		for name, v := range p.Flags {
			if err := checkFlag(name, string(v)); err != nil {
				bad("provides.ruleset.flags", "%v", err)
			}
		}
		for name, v := range p.Limits {
			if _, err := checkBound(rules.LimitBounds, "limit", name, string(v)); err != nil {
				bad("provides.ruleset.limits", "%v", err)
			}
		}
		for name, v := range p.Rules {
			if _, err := checkBound(rules.RuleBounds, "rule", name, string(v)); err != nil {
				bad("provides.ruleset.rules", "%v", err)
			}
		}
		if m.Supports.Rulesets != nil {
			bad("supports.rulesets", "a ruleset mod uses provides.ruleset instead")
		}
	}
	for i, name := range m.Permissions {
		if _, ok := LookupPermission(name); !ok {
			bad(fmt.Sprintf("permissions[%d]", i), "%q is not a permission; see strategy section 6.6", name)
		}
	}
	for i, s := range m.Scripts {
		if err := checkScriptPath(s); err != nil {
			bad(fmt.Sprintf("scripts[%d]", i), "%v", err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &m, nil
}

// checkScriptPath checks a script path from a manifest or a data file:
// a .lua file under scripts/, written with forward slashes.
func checkScriptPath(p string) error {
	if p == "" || p != path.Clean(p) || strings.Contains(p, `\`) || !strings.HasPrefix(p, "scripts/") || !strings.HasSuffix(p, ".lua") {
		return fmt.Errorf("%q is not a .lua file under scripts/, such as scripts/main.lua", p)
	}
	for _, seg := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(p, "scripts/"), ".lua"), "/") {
		if !scriptName.MatchString(seg) {
			return fmt.Errorf("%q: script folder and file names use letters, digits, _ and -", p)
		}
	}
	return nil
}

var scriptName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// SupportsAPI reports whether the manifest's api includes this server's
// API.
func (m *Manifest) SupportsAPI() bool {
	r, err := ParseRange(m.API)
	return err == nil && r.Match(apiVersion)
}

// SemVer returns the parsed version.
func (m *Manifest) SemVer() Version { return m.version }

// requests reports whether the manifest requests a permission.
func (m *Manifest) requests(p string) bool { return slices.Contains(m.Permissions, p) }
