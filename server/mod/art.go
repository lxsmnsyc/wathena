package mod

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/assets"
)

// Art fields (proposal 0002, review decision 38, decision 0033). A
// record's art field names an asset: a mod's art, such as
// mod/example.lantern/icon/item/Paper_Lantern, or core's, such as
// icon/item/501. Values with a "/" are asset names, so fields that named
// other things before, such as a monster's sprite, keep working.

// IsArtName reports whether a field's value is an asset name.
func IsArtName(s string) bool { return strings.Contains(s, "/") }

// CheckArtName checks that a field names an asset of a type, such as
// "sprite" or "texture".
func CheckArtName(name, typ string) error {
	if err := assets.ValidName(name); err != nil {
		return fmt.Errorf("%q is not an asset name", name)
	}
	if got := assets.TypeOf(name); got != typ {
		return fmt.Errorf("%q is not a %s name: its namespace holds %ss", name, typ, cmpOr(got, "placeholders"))
	}
	return nil
}

func cmpOr(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

// ArtName reads a field that names one asset of a type.
func ArtName(v any, typ string) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("want an asset name, such as mod/example.town/%s/x", typ)
	}
	return s, CheckArtName(s, typ)
}

// ArtTable reads a field that maps keys to asset names of a type, such
// as a monster's sounds.
func ArtTable(v any, keys []string, typ string) (map[string]string, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("want a table with %s", strings.Join(keys, ", "))
	}
	out := map[string]string{}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if !slices.Contains(keys, k) {
			return nil, fmt.Errorf("%q is not a key; the keys are %s", k, strings.Join(keys, ", "))
		}
		name, err := ArtName(m[k], typ)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		out[k] = name
	}
	return out, nil
}

// ArtList reads a field that lists asset names of a type.
func ArtList(v any, typ string) ([]string, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("want a list of %s names", typ)
	}
	var out []string
	for i, e := range list {
		name, err := ArtName(e, typ)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		out = append(out, name)
	}
	return out, nil
}

// ConventionName is the art name a mod's record has without an art
// field: mod/<mod id>/<namespace>/<local name>, such as
// mod/example.lantern/icon/item/Paper_Lantern. ok is false for a core
// record, or a name that cannot be an asset name.
func ConventionName(record, namespace string) (string, bool) {
	modID, local, ok := splitName(record)
	if !ok || modID == Core {
		return "", false
	}
	local, _, _ = strings.Cut(local, "#")
	n := assets.ModPrefix(modID) + namespace + "/" + local
	if assets.ValidName(n) != nil || strings.Count(local, "/") > 0 {
		return "", false
	}
	return n, true
}
