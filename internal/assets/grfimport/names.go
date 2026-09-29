package grfimport

import (
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// spriteFolders maps GRF sprite folders to asset namespaces. Characters,
// headgear, and weapons need the look tables and are added separately.
var spriteFolders = map[string]string{
	"data/sprite/몬스터/": "sprite/mob/",
	"data/sprite/npc/": "sprite/npc/",
}

// SpriteName returns the asset name for a GRF sprite path, such as
// "data\sprite\몬스터\poring.spr", and false for paths it does not map.
func SpriteName(grfPath string) (string, bool) {
	key := norm.NFC.String(grf.Normalize(grfPath))
	ext := path.Ext(key)
	if ext != ".spr" && ext != ".act" {
		return "", false
	}
	for folder, ns := range spriteFolders {
		rest, ok := strings.CutPrefix(key, folder)
		if !ok || strings.Contains(rest, "/") {
			continue
		}
		name := ns + strings.TrimSuffix(rest, ext)
		if assets.ValidName(name) != nil {
			return "", false
		}
		return name, true
	}
	return "", false
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
