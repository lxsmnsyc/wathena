package world

import (
	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// WarpData is warps.json, which the rAthena fork's tools/wathena/
// extract.py writes to reference/rathena/extracts/<mode>/.
type WarpData struct {
	Warps []struct {
		Map   string `json:"map"`
		X     int    `json:"x"`
		Y     int    `json:"y"`
		Name  string `json:"name"`
		XS    int    `json:"xs"`
		YS    int    `json:"ys"`
		ToMap string `json:"to_map"`
		ToX   int    `json:"to_x"`
		ToY   int    `json:"to_y"`
		Kind  string `json:"kind"`
		State string `json:"state"`
	} `json:"warps"`
}

// LoadWarps reads the warps rAthena loads in a mode, by map asset name.
func LoadWarps(mode reference.Mode) (map[string][]zone.Warp, error) {
	var d WarpData
	if err := reference.LoadFile("extracts/"+string(mode)+"/warps.json", &d); err != nil {
		return nil, err
	}
	return d.ByMap(), nil
}

// ByMap turns the warps into each map's zone.Warp list, in load order,
// with maps named by asset name, such as map/prontera. Warps that are not
// enabled are left out, since rAthena neither shows nor runs disabled,
// hidden or cloaked warps (npc_touch_areanpc skips them). Warps with a
// negative span have no touch area, as npc_setcells, and are left out
// too.
//
// warp2 works like warp for now: it differs only for hidden players, who
// do not exist yet.
func (d *WarpData) ByMap() map[string][]zone.Warp {
	out := map[string][]zone.Warp{}
	for _, w := range d.Warps {
		if w.State != "enabled" || w.XS < 0 || w.YS < 0 {
			continue
		}
		name := "map/" + w.Map
		out[name] = append(out[name], zone.Warp{
			Name: w.Name, X: w.X, Y: w.Y, XS: w.XS, YS: w.YS,
			To: "map/" + w.ToMap, ToX: w.ToX, ToY: w.ToY,
		})
	}
	return out
}
