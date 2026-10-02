package grfimport

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/grf"
)

// Status icons are named by status ID, the client's EFST_ number, as
// icon/status/<id>. Two client tables give each ID an image:
//
//	efstids.lub:          EFST_IDs = { EFST_PROVOKE = 0, ... }
//	stateiconimginfo.lub: StateIconImgList = {
//	                        [priority] = { [EFST_IDs.EFST_X] = "x.tga", ... },
//	                      }
//
// The images are in data\texture\effect\.

const (
	efstIDsPath       = `data\luafiles514\lua files\stateicon\efstids.lub`
	stateIconImgPath  = `data\luafiles514\lua files\stateicon\stateiconimginfo.lub`
	statusIconDir     = `data\texture\effect\`
	maxStatusIconID   = 1 << 16
	statusIconsPrefix = "icon/status/"
)

// StatusIconAssets lists the status icon images a client has, named by
// status ID. Icons whose files are missing are left out. A client without
// the tables has no icons.
func StatusIconAssets(c *grf.Client) ([]ItemTexture, error) {
	g := map[string]any{}
	for _, p := range []string{efstIDsPath, stateIconImgPath} {
		s, ok := c.Lookup(p)
		if !ok {
			return nil, nil
		}
		b, err := c.ReadFile(s)
		if err != nil {
			return nil, err
		}
		if err := evalLub(b, g); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	files := statusIconFiles(g["StateIconImgList"])
	ids := make([]int, 0, len(files))
	for id := range files {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var out []ItemTexture
	for _, id := range ids {
		if s, ok := lookupStatusIcon(c, files[id]); ok {
			out = append(out, ItemTexture{Name: statusIconsPrefix + strconv.Itoa(id), Source: s})
		}
	}
	return out, nil
}

// statusIconFiles reads StateIconImgList. Each priority holds a table
// from status ID to file name. An ID listed under two priorities keeps
// the file of the lower priority number.
func statusIconFiles(v any) map[int]string {
	out := map[int]string{}
	list, ok := v.(*LubTable)
	if !ok {
		return out
	}
	prios := make([]float64, 0, len(list.M))
	for k := range list.M {
		if p, ok := k.(float64); ok {
			prios = append(prios, p)
		}
	}
	sort.Float64s(prios)
	for _, p := range prios {
		tb, ok := list.M[p].(*LubTable)
		if !ok {
			continue
		}
		for k, v := range tb.M {
			id, ok1 := k.(float64)
			raw, ok2 := v.(string)
			if !ok1 || !ok2 || id < 0 || id >= maxStatusIconID || id != float64(int(id)) {
				continue
			}
			file := strings.TrimSpace(decodeCP949(raw))
			if _, seen := out[int(id)]; seen || file == "" || strings.ContainsAny(file, `\/`) || strings.Contains(file, "..") {
				continue
			}
			out[int(id)] = file
		}
	}
	return out
}

// lookupStatusIcon finds an icon file. It tries the name as given, then
// the same name with each image extension. Lookups ignore case.
func lookupStatusIcon(c *grf.Client, file string) (grf.Source, bool) {
	if s, ok := c.Lookup(statusIconDir + file); ok {
		return s, true
	}
	base := strings.TrimSuffix(file, path.Ext(file))
	for _, ext := range textureFormats {
		if s, ok := c.Lookup(statusIconDir + base + ext); ok {
			return s, true
		}
	}
	return grf.Source{}, false
}
