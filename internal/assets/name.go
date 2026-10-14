// Package assets holds the asset contract from docs/specs/assets-v1.md.
package assets

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Contract is the asset contract version.
const Contract = "assets-v1"

// MaxNameBytes is the longest allowed name.
const MaxNameBytes = 255

// ModNamespace is the first segment of a mod's art names, as in
// mod/example.lantern/icon/item/Paper_Lantern (assets-v1, "Mod art").
const ModNamespace = "mod"

// ErrName is returned for names that break the naming rules.
var ErrName = errors.New("assets: invalid name")

// modIDPattern is a mod ID, as server/mod checks it: lowercase words of
// letters, digits, "_" and "-", separated by dots.
var modIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)*$`)

// ValidModID reports whether s has the form of a mod ID.
func ValidModID(s string) bool {
	return len(s) <= 64 && modIDPattern.MatchString(s)
}

// ValidName checks a name against the rules in assets-v1, "Names".
// Names come from packs and importers, so this runs before any name is used.
// A mod's art name keeps the case of its record names after the mod ID
// and the namespace.
func ValidName(name string) error {
	if name == "" || len(name) > MaxNameBytes || !utf8.ValidString(name) {
		return ErrName
	}
	if !norm.NFC.IsNormalString(name) {
		return ErrName
	}
	segs := strings.Split(name, "/")
	for _, seg := range segs {
		if seg == "" || seg == "." || seg == ".." {
			return ErrName
		}
	}
	// Upper case is allowed after mod/<mod id>/<namespace>/.
	upper := len(name)
	if segs[0] == ModNamespace {
		if len(segs) < 4 || !ValidModID(segs[1]) {
			return ErrName
		}
		upper = len(segs[0]) + len(segs[1]) + len(segs[2]) + 3
	}
	for i, r := range name {
		switch {
		case r == '\\' || r == ':':
			return ErrName
		case unicode.IsControl(r):
			return ErrName
		case 'A' <= r && r <= 'Z' && i < upper:
			return ErrName
		}
	}
	return nil
}

// Namespace returns the first segment of a name, such as "sprite". For a
// mod's art, it is the first segment after the mod ID, so
// mod/example.lantern/icon/item/Paper_Lantern is in "icon".
func Namespace(name string) string {
	if _, rest, ok := SplitMod(name); ok {
		name = rest
	}
	ns, _, _ := strings.Cut(name, "/")
	return ns
}

// SplitMod splits a mod's art name into the mod ID and the rest, such as
// example.lantern and icon/item/Paper_Lantern. ok is false for names
// outside mod/.
func SplitMod(name string) (modID, rest string, ok bool) {
	after, found := strings.CutPrefix(name, ModNamespace+"/")
	if !found {
		return "", "", false
	}
	modID, rest, found = strings.Cut(after, "/")
	if !found || modID == "" || rest == "" {
		return "", "", false
	}
	return modID, rest, true
}

// ModPrefix is the prefix of every art name of a mod, such as
// mod/example.lantern/.
func ModPrefix(modID string) string {
	return ModNamespace + "/" + modID + "/"
}

// Types maps each namespace to the asset type its names have.
// placeholder/ names any type.
var Types = map[string]string{
	"sprite":   "sprite",
	"palette":  "palette",
	"texture":  "texture",
	"ui":       "texture",
	"icon":     "texture",
	"image":    "texture",
	"lightmap": "texture",
	"model":    "model",
	"map":      "map",
	"effect":   "effect",
	"sound":    "sound",
	"music":    "music",
	"table":    "table",
}

// TypeOf returns the asset type a name's namespace gives it, or "" for a
// namespace with no fixed type, such as placeholder/.
func TypeOf(name string) string {
	return Types[Namespace(name)]
}
