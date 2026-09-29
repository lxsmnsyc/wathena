package grfimport

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/unicode/norm"

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

var (
	headPath = regexp.MustCompile(`^data/sprite/인간족/머리통/(남|여)/(\d+)_(남|여)\.(spr|act)$`)
	hairPath = regexp.MustCompile(`^data/palette/머리/머리(\d+)_(남|여)_(\d+)\.pal$`)
)

// Tables holds the client tables the importer uses to name assets.
type Tables struct {
	// headgear maps "<sex>/<accessory name>", such as "남/_고글", to view IDs.
	headgear map[string][]int
}

// clientTables lists the .lub files to evaluate, in order, since later
// files use tables from earlier ones.
var clientTables = []string{
	`data\luafiles514\lua files\datainfo\accessoryid.lub`,
	`data\luafiles514\lua files\datainfo\accname.lub`,
}

// LoadTables evaluates the client's data tables. Missing tables leave the
// assets that need them unnamed, and are not an error.
func LoadTables(c *grf.Client) (*Tables, error) {
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
	t := &Tables{headgear: map[string][]int{}}
	if acc, ok := g["AccNameTable"].(*LubTable); ok {
		for k, v := range acc.M {
			id, ok1 := k.(float64)
			raw, ok2 := v.(string)
			if !ok1 || !ok2 || id < 1 || id > 1<<20 || id != float64(int(id)) {
				continue
			}
			name := decodeCP949(raw)
			for _, sex := range []string{"남", "여"} {
				key := grf.Normalize(sex + "/" + sex + name)
				t.headgear[key] = append(t.headgear[key], int(id))
			}
		}
	}
	return t, nil
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
	if m := headPath.FindStringSubmatch(key); m != nil && m[1] == m[3] {
		add("sprite/head/" + sexes[m[1]] + "/" + trimZeros(m[2]))
	}
	if t != nil {
		if rest, ok := strings.CutPrefix(key, "data/sprite/악세사리/"); ok {
			sex, _, _ := strings.Cut(rest, "/")
			for _, id := range t.headgear[strings.TrimSuffix(rest, ext)] {
				add(fmt.Sprintf("sprite/headgear/%s/%d", sexes[sex], id))
			}
		}
	}
	return out
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

// PaletteName returns the asset name for a GRF palette path.
func PaletteName(grfPath string) (string, bool) {
	key := norm.NFC.String(grf.Normalize(grfPath))
	if m := hairPath.FindStringSubmatch(key); m != nil {
		name := "palette/hair/" + sexes[m[2]] + "/" + trimZeros(m[1]) + "/" + trimZeros(m[3])
		return name, assets.ValidName(name) == nil
	}
	return "", false
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
