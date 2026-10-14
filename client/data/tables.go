package clientdata

import (
	_ "embed"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/assets"
)

//go:embed weapons.yml
var weaponsYAML []byte

//go:embed effects.yml
var effectsYAML []byte

// Weapon is one rAthena weapon type.
type Weapon struct {
	ID       int    `yaml:"id"`
	Constant string `yaml:"constant"`
	Name     string `yaml:"name"`
	// View is the weapon sprite for items without their own view. 0 draws
	// no weapon.
	View   int      `yaml:"view"`
	Attack []string `yaml:"attack"`
	Hit    []string `yaml:"hit"`
}

// Weapons is weapons.yml.
type Weapons struct {
	Weapons []Weapon `yaml:"weapons"`
}

// EffectPart is one thing an effect shows. Exactly one field is set.
type EffectPart struct {
	// Effect is a baked STR effect.
	Effect string `yaml:"effect"`
	// Variants are STR effects, one chosen at random each time.
	Variants []string `yaml:"variants"`
	// Sound is a sound played when the effect starts.
	Sound string `yaml:"sound"`
	// Cylinder is a textured cone the client draws.
	Cylinder *Cylinder `yaml:"cylinder"`
}

// Cylinder is roBrowser's CYLINDER effect type. effects.yml explains the
// fields.
type Cylinder struct {
	Texture    string     `yaml:"texture"`
	Bottom     float64    `yaml:"bottom"`
	Top        float64    `yaml:"top"`
	Height     float64    `yaml:"height"`
	Color      [3]float64 `yaml:"color"`
	Alpha      float64    `yaml:"alpha"`
	DurationMs int        `yaml:"duration_ms"`
	Copies     int        `yaml:"copies"`
	CopyMs     int        `yaml:"copy_ms"`
	Animation  int        `yaml:"animation"`
	Fade       bool       `yaml:"fade"`
	Repeat     bool       `yaml:"repeat"`
}

// check checks a cylinder's values.
func (c *Cylinder) check() error {
	if c.Bottom < 0 || c.Top < 0 || c.Height < 0 || c.Alpha < 0 || c.Alpha > 1 || c.DurationMs <= 0 ||
		c.Copies < 1 || c.CopyMs < 0 || c.Animation < 0 || c.Animation > 5 {
		return fmt.Errorf("effects.yml: cylinder %s has values out of range", c.Texture)
	}
	for _, v := range c.Color {
		if v < 0 || v > 1 {
			return fmt.Errorf("effects.yml: cylinder %s has a color out of range", c.Texture)
		}
	}
	return checkName("effects.yml", c.Texture, "texture/")
}

// names returns the asset names a part refers to.
func (p EffectPart) names() []string {
	var out []string
	if p.Cylinder != nil {
		out = append(out, p.Cylinder.Texture)
	}
	for _, n := range append([]string{p.Effect, p.Sound}, p.Variants...) {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// Effect is one RO effect ID.
type Effect struct {
	ID       int          `yaml:"id"`
	Constant string       `yaml:"constant"`
	Parts    []EffectPart `yaml:"parts"`
}

// Effects is effects.yml.
type Effects struct {
	Effects []Effect `yaml:"effects"`
}

// LoadWeapons parses and checks the embedded weapons.yml.
func LoadWeapons() (*Weapons, error) { return ParseWeapons(weaponsYAML) }

// LoadEffects parses and checks the embedded effects.yml.
func LoadEffects() (*Effects, error) { return ParseEffects(effectsYAML) }

// checkName checks an asset name in a table and its namespace. A mod's
// table may name its own art, such as mod/example.lantern/effect/glow.
func checkName(table, name, prefix string) error {
	if assets.Namespace(name) != strings.TrimSuffix(prefix, "/") || assets.ValidName(name) != nil {
		return fmt.Errorf("%s: %q is not a %s name", table, name, strings.TrimSuffix(prefix, "/"))
	}
	return nil
}

// ParseWeapons parses and checks a weapons table.
func ParseWeapons(data []byte) (*Weapons, error) {
	var w Weapons
	if err := strictUnmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("weapons.yml: %w", err)
	}
	ids := map[int]bool{}
	for _, wp := range w.Weapons {
		if ids[wp.ID] || wp.ID < 0 || wp.View < 0 || wp.Constant == "" {
			return nil, fmt.Errorf("weapons.yml: weapon %d is invalid or listed twice", wp.ID)
		}
		ids[wp.ID] = true
		for _, s := range append(append([]string(nil), wp.Attack...), wp.Hit...) {
			if err := checkName("weapons.yml", s, "sound/"); err != nil {
				return nil, err
			}
		}
	}
	return &w, nil
}

// ParseEffects parses and checks an effects table.
func ParseEffects(data []byte) (*Effects, error) {
	var e Effects
	if err := strictUnmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("effects.yml: %w", err)
	}
	ids := map[int]bool{}
	for _, ef := range e.Effects {
		if ids[ef.ID] || ef.ID < 0 || len(ef.Parts) == 0 {
			return nil, fmt.Errorf("effects.yml: effect %d is invalid or listed twice", ef.ID)
		}
		ids[ef.ID] = true
		for _, p := range ef.Parts {
			set := 0
			for _, ok := range []bool{p.Effect != "", len(p.Variants) > 0, p.Sound != "", p.Cylinder != nil} {
				if ok {
					set++
				}
			}
			if set != 1 {
				return nil, fmt.Errorf("effects.yml: effect %d: a part needs exactly one of effect, variants, sound, and cylinder", ef.ID)
			}
			if p.Cylinder != nil {
				if err := p.Cylinder.check(); err != nil {
					return nil, err
				}
			}
			for _, n := range append([]string{p.Effect}, p.Variants...) {
				if n == "" {
					continue
				}
				if err := checkName("effects.yml", n, "effect/"); err != nil {
					return nil, err
				}
			}
			if p.Sound != "" {
				if err := checkName("effects.yml", p.Sound, "sound/"); err != nil {
					return nil, err
				}
			}
		}
	}
	return &e, nil
}

// strictUnmarshal rejects fields the table types do not have, so a typo in
// a key is an error instead of a missing value.
func strictUnmarshal(data []byte, v any) error {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	return dec.Decode(v)
}

// Reference is an asset name a table refers to.
type Reference struct {
	Table string
	Name  string
}

// References lists every asset name the embedded tables refer to, for
// `wactl assets check`. Every look needs a body sprite for each sex it
// exists for.
func References() ([]Reference, error) {
	jobs, err := LoadJobs()
	if err != nil {
		return nil, err
	}
	weapons, err := LoadWeapons()
	if err != nil {
		return nil, err
	}
	effects, err := LoadEffects()
	if err != nil {
		return nil, err
	}
	var refs []Reference
	for _, l := range jobs.Looks {
		for _, sex := range l.Sexes() {
			refs = append(refs, Reference{"client/data/jobs.yml", "sprite/body/" + l.Look + "/" + sex})
		}
	}
	for _, w := range weapons.Weapons {
		for _, s := range append(append([]string(nil), w.Attack...), w.Hit...) {
			refs = append(refs, Reference{"client/data/weapons.yml", s})
		}
	}
	for _, e := range effects.Effects {
		for _, p := range e.Parts {
			for _, n := range p.names() {
				refs = append(refs, Reference{"client/data/effects.yml", n})
			}
		}
	}
	return refs, nil
}
