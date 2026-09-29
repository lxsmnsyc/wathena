package grfimport

import (
	"path"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// uiFolders maps folders below data/texture/유저인터페이스/ to namespaces
// other than ui/.
var uiFolders = map[string]string{
	"item":       "icon/item/",
	"collection": "image/item/",
	"illust":     "image/illust/",
	"cardbmp":    "image/card/",
}

// textureFormats lists image extensions in order of preference. When a
// folder has the same image name in several formats, the first gets the
// plain asset name. Maps and models refer to BMP files.
var textureFormats = []string{".bmp", ".tga", ".png", ".jpg", ".jpeg"}

// textureBase returns the asset name for a GRF image path, without the
// suffix that tells formats apart, and the image's extension.
func textureBase(grfPath string) (name, ext string, ok bool) {
	key := norm.NFC.String(grf.Normalize(grfPath))
	ext = path.Ext(key)
	known := false
	for _, e := range textureFormats {
		known = known || e == ext
	}
	rest, ok := strings.CutPrefix(strings.TrimSuffix(key, ext), "data/texture/")
	if !known || !ok {
		return "", "", false
	}
	name = "texture/" + rest
	if ui, ok := strings.CutPrefix(rest, "유저인터페이스/"); ok {
		name = "ui/" + ui
		folder, file, _ := strings.Cut(ui, "/")
		if ns, ok := uiFolders[folder]; ok && file != "" {
			name = ns + file
		}
	}
	if assets.ValidName(name) != nil {
		return "", "", false
	}
	return name, ext, true
}

// TextureNames returns the asset name for each image in paths, a list of
// GRF paths. An image whose name another format also has gets the format
// appended, as in texture/effect/fire~tga, unless it is the preferred
// format. Paths that are not images in data\texture have no name.
func TextureNames(paths []string) map[string]string {
	type file struct {
		path, ext string
	}
	byName := map[string][]file{}
	for _, p := range paths {
		if name, ext, ok := textureBase(p); ok {
			byName[name] = append(byName[name], file{p, ext})
		}
	}
	out := map[string]string{}
	for name, files := range byName {
		best := files[0]
		for _, f := range files[1:] {
			if formatRank(f.ext) < formatRank(best.ext) {
				best = f
			}
		}
		for _, f := range files {
			n := name
			if f != best {
				n += "~" + strings.TrimPrefix(f.ext, ".")
			}
			if assets.ValidName(n) == nil {
				out[f.path] = n
			}
		}
	}
	return out
}

func formatRank(ext string) int {
	for i, e := range textureFormats {
		if e == ext {
			return i
		}
	}
	return len(textureFormats)
}
