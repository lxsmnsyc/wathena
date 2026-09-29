// Package assets holds the asset contract from docs/specs/assets-v1.md.
package assets

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Contract is the asset contract version.
const Contract = "assets-v1"

// MaxNameBytes is the longest allowed name.
const MaxNameBytes = 255

// ErrName is returned for names that break the naming rules.
var ErrName = errors.New("assets: invalid name")

// ValidName checks a name against the rules in assets-v1, "Names".
// Names come from packs and importers, so this runs before any name is used.
func ValidName(name string) error {
	if name == "" || len(name) > MaxNameBytes || !utf8.ValidString(name) {
		return ErrName
	}
	if !norm.NFC.IsNormalString(name) {
		return ErrName
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return ErrName
		}
	}
	for _, r := range name {
		switch {
		case r == '\\' || r == ':':
			return ErrName
		case unicode.IsControl(r):
			return ErrName
		case 'A' <= r && r <= 'Z':
			return ErrName
		}
	}
	return nil
}

// Namespace returns the first segment of a name, such as "sprite".
func Namespace(name string) string {
	ns, _, _ := strings.Cut(name, "/")
	return ns
}
