package grfimport

import (
	"strings"

	"github.com/lxsmnsyc/wathena/internal/assets/mapimport"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// Newer maps describe their particle emitters in a Lua table in the
// client's effect tool folder, such as
//
//	_prontera_emitterInfo = { [0] = { texture = "effect\\smoke1.bmp", ... } }
//
// in data\luafiles514\lua files\effecttool\prontera.lub. Each field but
// texture is a list of numbers.

var effectToolFolders = []string{
	`data\luafiles514\lua files\effecttool\`,
	`data\lua files\effecttool\`,
}

// maxEmitters bounds one map's emitter table.
const maxEmitters = 1 << 16

// EmitterSource finds the effect tool file for a map, such as "prontera".
func EmitterSource(c *grf.Client, mapName string) (grf.Source, bool) {
	for _, dir := range effectToolFolders {
		if s, ok := c.Lookup(dir + mapName + ".lub"); ok {
			return s, true
		}
	}
	return grf.Source{}, false
}

// MapEmitters reads the emitter table for a map, such as "prontera", from
// its effect tool file. A map without one has none.
func MapEmitters(c *grf.Client, mapName string) (map[int]mapimport.Emitter, error) {
	s, ok := EmitterSource(c, mapName)
	if !ok {
		return nil, nil
	}
	b, err := c.ReadFile(s)
	if err != nil {
		return nil, err
	}
	g := map[string]any{}
	if err := evalLub(b, g); err != nil {
		// A few maps have their table as Lua source instead of a compiled
		// chunk, such as moro_cav in kRO. Only compiled tables are read,
		// so those maps have no emitters. The rest of the map still
		// converts.
		return nil, nil //nolint:nilerr // The file is optional.
	}
	// Lua names cannot hold "@", so instance maps such as 1@tower leave it
	// out.
	tb, _ := g["_"+strings.ReplaceAll(mapName, "@", "")+"_emitterInfo"].(*LubTable)
	out := map[int]mapimport.Emitter{}
	if tb == nil {
		return out, nil
	}
	for k, v := range tb.M {
		i, ok := k.(float64)
		e, ok2 := v.(*LubTable)
		if !ok || !ok2 || i < 0 || i >= maxEmitters || i != float64(int(i)) {
			continue
		}
		out[int(i)] = readEmitter(e)
	}
	return out, nil
}

func readEmitter(t *LubTable) mapimport.Emitter {
	nums := func(key string, n int) []float32 {
		out := make([]float32, n)
		l, _ := t.M[key].(*LubTable)
		for i := range out {
			if l != nil {
				if f, ok := l.M[float64(i+1)].(float64); ok {
					out[i] = float32(f)
				}
			}
		}
		return out
	}
	one := func(key string) float32 { return nums(key, 1)[0] }
	vec3 := func(key string) [3]float32 { v := nums(key, 3); return [3]float32{v[0], v[1], v[2]} }
	vec2 := func(key string) [2]float32 { v := nums(key, 2); return [2]float32{v[0], v[1]} }
	e := mapimport.Emitter{
		SrcMode:      int(one("srcmode")),
		DestMode:     int(one("destmode")),
		Dir1:         vec3("dir1"),
		Dir2:         vec3("dir2"),
		Gravity:      vec3("gravity"),
		Radius:       vec3("radius"),
		Life:         vec2("life"),
		Rate:         vec2("rate"),
		Size:         vec2("size"),
		Speed:        one("speed"),
		MaxCount:     int(one("maxcount")),
		ZEnable:      one("zenable") != 0,
		Eternity:     one("eternity") != 0,
		BillboardOff: one("billboard_off") != 0,
		Scale:        [2]float32{1, 1},
		RotateAngle:  vec3("rotate_angle"),
	}
	// Colors have three or four values. Without a fourth, alpha is 255.
	c := nums("color", 4)
	e.Color = [4]float32{c[0], c[1], c[2], 255}
	if l, _ := t.M["color"].(*LubTable); l != nil && len(l.M) >= 4 {
		e.Color[3] = c[3]
	}
	if _, ok := t.M["scale"]; ok {
		e.Scale = vec2("scale")
	}
	if s, ok := t.M["texture"].(string); ok {
		e.Texture = strings.ReplaceAll(decodeCP949(s), `\\`, `\`)
	}
	return e
}
