package grfimport

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/unicode/norm"

	clientdata "github.com/lxsmnsyc/wathena/client/data"
	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// spriteFolders maps GRF sprite folders to asset namespaces, for folders
// where the file name is the asset name.
var spriteFolders = map[string]string{
	"data/sprite/몬스터/": "sprite/mob/",
	"data/sprite/npc/": "sprite/npc/",
}

// sexes maps RO's Korean sex markers to assets-v1 names.
var sexes = map[string]string{"남": "m", "여": "f"}

// races maps RO's sprite race folders to assets-v1 names.
var races = map[string]string{"인간족": "human", "도람족": "doram"}

var (
	headPath    = regexp.MustCompile(`^data/sprite/(인간족|도람족)/머리통/(남|여)/(\d+)_(남|여)\.(spr|act)$`)
	hairPath    = regexp.MustCompile(`^data/palette/(도람족/)?머리/머리(\d+)_(남|여)_(\d+)\.pal$`)
	bodyPalPath = regexp.MustCompile(`^data/palette/(몸|도람족/body)/(.+)_(남|여)_(\d+)\.pal$`)
)

// Tables holds the client tables the importer uses to name assets.
type Tables struct {
	// headgear maps "<sex>/<accessory name>", such as "남/_고글", to view IDs.
	headgear map[string][]int
	// bodies maps a lowercase GRF body sprite name to its look names.
	bodies map[string][]string
	// races maps a lowercase GRF body sprite name to its race folder.
	races map[string]string
	// weapons maps a lowercase weapon suffix, such as "_검", to view IDs.
	weapons map[string][]int
	// garments maps a lowercase garment folder name, such as "천사날개",
	// to view IDs.
	garments map[string][]int
}

// shieldViews is the client's built-in table of named shield sprites.
// Other shields use their view ID in the file name, as "_<view>_방패".
var shieldViews = map[string]int{
	"_가드":   1,
	"_버클러":  2,
	"_쉴드":   3,
	"_미러쉴드": 4,
	"_매드바니": 6,
}

var shieldIDPath = regexp.MustCompile(`^_([1-9]\d{0,6})_방패$`)

// raceFolders maps jobs.yml races to GRF sprite folders.
var raceFolders = map[string]string{"human": "인간족", "doram": "도람족"}

// clientTables lists the .lub files to evaluate, in order, since later
// files use tables from earlier ones.
var clientTables = []string{
	`data\luafiles514\lua files\datainfo\accessoryid.lub`,
	`data\luafiles514\lua files\datainfo\accname.lub`,
	`data\luafiles514\lua files\datainfo\weapontable.lub`,
	`data\luafiles514\lua files\datainfo\spriterobeid.lub`,
	`data\luafiles514\lua files\datainfo\spriterobename.lub`,
}

// LoadTables evaluates the client's data tables. Missing tables leave the
// assets that need them unnamed, and are not an error.
func LoadTables(c *grf.Client, jobs *clientdata.Jobs) (*Tables, error) {
	g := map[string]any{}
	for _, name := range clientTables {
		s, ok := c.Lookup(name)
		if !ok {
			continue
		}
		b, err := c.ReadFile(s)
		if err != nil {
			return nil, err
		}
		if err := evalLub(b, g); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	t := &Tables{headgear: map[string][]int{}, bodies: map[string][]string{}, races: map[string]string{}, weapons: map[string][]int{}, garments: map[string][]int{}}
	forEachName(g["AccNameTable"], func(id int, name string) {
		for _, sex := range []string{"남", "여"} {
			key := grf.Normalize(sex + "/" + sex + name)
			t.headgear[key] = append(t.headgear[key], id)
		}
	})
	forEachName(g["WeaponNameTable"], func(id int, name string) {
		key := grf.Normalize(name)
		t.weapons[key] = append(t.weapons[key], id)
	})
	forEachName(g["RobeNameTable"], func(id int, name string) {
		key := grf.Normalize(name)
		t.garments[key] = append(t.garments[key], id)
	})
	if jobs != nil {
		for _, l := range jobs.Looks {
			key := norm.NFC.String(grf.Normalize(l.GRFSprite))
			t.bodies[key] = append(t.bodies[key], l.Look)
			t.races[key] = raceFolders[l.Race]
		}
	}
	return t, nil
}

// forEachName calls f for each entry of a table from numeric IDs to
// CP949 names.
func forEachName(v any, f func(id int, name string)) {
	tb, ok := v.(*LubTable)
	if !ok {
		return
	}
	for k, v := range tb.M {
		id, ok1 := k.(float64)
		raw, ok2 := v.(string)
		if !ok1 || !ok2 || id < 0 || id > 1<<20 || id != float64(int(id)) {
			continue
		}
		f(int(id), decodeCP949(raw))
	}
}

func decodeCP949(s string) string {
	d, err := korean.EUCKR.NewDecoder().String(s)
	if err != nil {
		return strings.ToValidUTF8(s, "")
	}
	return norm.NFC.String(d)
}

// SpriteNames returns the asset names for a GRF sprite or act path. A file
// can have several names, such as a headgear shared by several view IDs.
// tables may be nil, which leaves out names that need client tables.
func (t *Tables) SpriteNames(grfPath string) []string {
	key := norm.NFC.String(grf.Normalize(grfPath))
	ext := path.Ext(key)
	if ext != ".spr" && ext != ".act" {
		return nil
	}
	var out []string
	add := func(name string) {
		if assets.ValidName(name) == nil {
			out = append(out, name)
		}
	}
	for folder, ns := range spriteFolders {
		if rest, ok := strings.CutPrefix(key, folder); ok && !strings.Contains(rest, "/") {
			add(ns + strings.TrimSuffix(rest, ext))
		}
	}
	if m := headPath.FindStringSubmatch(key); m != nil && m[2] == m[4] {
		add("sprite/head/" + races[m[1]] + "/" + sexes[m[2]] + "/" + trimZeros(m[3]))
	}
	if t != nil {
		t.characterNames(strings.TrimSuffix(key, ext), add)
		if rest, ok := strings.CutPrefix(key, "data/sprite/악세사리/"); ok {
			sex, _, _ := strings.Cut(rest, "/")
			for _, id := range t.headgear[strings.TrimSuffix(rest, ext)] {
				add(fmt.Sprintf("sprite/headgear/%s/%d", sexes[sex], id))
			}
		}
	}
	return out
}

// characterNames adds body, weapon, shield, and garment names for a path
// without extension:
//
//	bodies    data/sprite/<race>/몸통/<sex>/<sprite>_<sex>
//	weapons   data/sprite/<race>/<sprite>/<sprite>_<sex><weapon suffix>
//	shields   data/sprite/방패/<sprite>/<sprite>_<sex><shield suffix>
//	garments  data/sprite/로브/<garment>/<sex>/<sprite>_<sex>
func (t *Tables) characterNames(key string, add func(string)) {
	rest, ok := strings.CutPrefix(key, "data/sprite/")
	if !ok {
		return
	}
	race, rest, _ := strings.Cut(rest, "/")
	switch race {
	case "방패":
		t.shieldNames(rest, add)
		return
	case "로브":
		t.garmentNames(rest, add)
		return
	}
	if race != "인간족" && race != "도람족" {
		return
	}
	folder, file, ok := strings.Cut(rest, "/")
	if !ok {
		return
	}
	if folder == "몸통" {
		sex, file, ok := strings.Cut(file, "/")
		sprite, ok2 := strings.CutSuffix(file, "_"+sex)
		if !ok || !ok2 || sexes[sex] == "" {
			return
		}
		if t.races[sprite] != race {
			return
		}
		for _, look := range t.bodies[sprite] {
			add("sprite/body/" + look + "/" + sexes[sex])
		}
		return
	}
	looks := t.bodies[folder]
	if len(looks) == 0 || t.races[folder] != race {
		return
	}
	for _, sex := range []string{"남", "여"} {
		suffix, ok := strings.CutPrefix(file, folder+"_"+sex)
		if !ok {
			continue
		}
		for _, id := range t.weapons[suffix] {
			for _, look := range looks {
				add(fmt.Sprintf("sprite/weapon/%s/%s/%d", look, sexes[sex], id))
			}
		}
	}
}

// shieldNames adds names for rest, a path below data/sprite/방패/.
// Shields are not split by race, so any look with the sprite name matches.
func (t *Tables) shieldNames(rest string, add func(string)) {
	folder, file, ok := strings.Cut(rest, "/")
	if !ok || strings.Contains(file, "/") {
		return
	}
	for _, sex := range []string{"남", "여"} {
		suffix, ok := strings.CutPrefix(file, folder+"_"+sex)
		if !ok {
			continue
		}
		view, ok := shieldViews[suffix]
		if m := shieldIDPath.FindStringSubmatch(suffix); m != nil {
			view, _ = strconv.Atoi(m[1])
			ok = true
		}
		if !ok {
			continue
		}
		for _, look := range t.bodies[folder] {
			add(fmt.Sprintf("sprite/shield/%s/%s/%d", look, sexes[sex], view))
		}
	}
}

// garmentNames adds names for rest, a path below data/sprite/로브/.
func (t *Tables) garmentNames(rest string, add func(string)) {
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || sexes[parts[1]] == "" {
		return
	}
	sprite, ok := strings.CutSuffix(parts[2], "_"+parts[1])
	if !ok {
		return
	}
	for _, id := range t.garments[parts[0]] {
		for _, look := range t.bodies[sprite] {
			add(fmt.Sprintf("sprite/garment/%s/%s/%d", look, sexes[parts[1]], id))
		}
	}
}

// SpriteName returns the first asset name for a path, for callers that
// only need one.
func SpriteName(grfPath string) (string, bool) {
	names := (*Tables)(nil).SpriteNames(grfPath)
	if len(names) == 0 {
		return "", false
	}
	return names[0], true
}

// PaletteNames returns the asset names for a GRF palette path. tables may
// be nil, which leaves out body palettes.
func (t *Tables) PaletteNames(grfPath string) []string {
	key := norm.NFC.String(grf.Normalize(grfPath))
	var out []string
	add := func(name string) {
		if assets.ValidName(name) == nil {
			out = append(out, name)
		}
	}
	if m := hairPath.FindStringSubmatch(key); m != nil {
		race := "human"
		if m[1] != "" {
			race = "doram"
		}
		add("palette/hair/" + race + "/" + sexes[m[3]] + "/" + trimZeros(m[2]) + "/" + trimZeros(m[4]))
		return out
	}
	if t == nil {
		return out
	}
	if m := bodyPalPath.FindStringSubmatch(key); m != nil {
		race := "인간족"
		if m[1] != "몸" {
			race = "도람족"
		}
		if t.races[m[2]] == race {
			for _, look := range t.bodies[m[2]] {
				add("palette/body/" + look + "/" + sexes[m[3]] + "/" + trimZeros(m[4]))
			}
		}
	}
	return out
}

// PaletteName returns the first asset name for a GRF palette path.
func PaletteName(grfPath string) (string, bool) {
	names := (*Tables)(nil).PaletteNames(grfPath)
	if len(names) == 0 {
		return "", false
	}
	return names[0], true
}

func trimZeros(s string) string {
	n, err := strconv.Atoi(s)
	if err != nil {
		return s
	}
	return strconv.Itoa(n)
}

// EventName maps an act event to the name stored in WSPR. Sound files,
// relative to data\wav, become sound names. Other events, such as the
// "atk" hit marker, are kept in lowercase.
func EventName(event string) string {
	e := norm.NFC.String(grf.Normalize(event))
	if base, ok := strings.CutSuffix(e, ".wav"); ok {
		name := "sound/" + base
		if assets.ValidName(name) == nil {
			return name
		}
	}
	return e
}
