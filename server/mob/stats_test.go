package mob

import (
	"reflect"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
)

// TestStatusParity computes every monster's battle status from
// monsters.json and compares it with mob_status.json, the status rAthena
// gave each monster when it spawned, in both modes.
func TestStatusParity(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			d, err := Load(mode)
			if err != nil {
				t.Fatal(err)
			}
			var rec struct {
				Mobs []struct {
					ID      uint32        `json:"id"`
					Level   int           `json:"level"`
					BaseExp int64         `json:"base_exp"`
					JobExp  int64         `json:"job_exp"`
					Status  battle.Status `json:"status"`
				} `json:"mobs"`
			}
			if err := reference.Load(mode, "mob_status", &rec); err != nil {
				t.Fatal(err)
			}
			byID := map[uint32]*Def{}
			for i := range d.Monsters {
				byID[d.Monsters[i].ID] = &d.Monsters[i]
			}
			renewal := mode == reference.Renewal
			matched := 0
			for _, r := range rec.Mobs {
				def := byID[r.ID]
				if def == nil {
					t.Errorf("monster %d: not in monsters.json", r.ID)
					continue
				}
				got := Calc(def, renewal)
				want := r.Status
				// The recording leaves out what is not in status_data.
				got.ID, got.DamageTaken = 0, 0
				want.Level = r.Level
				if !reflect.DeepEqual(got, want) || def.BaseExp != r.BaseExp || def.JobExp != r.JobExp {
					if t.Failed() {
						continue
					}
					t.Errorf("monster %d (%s):\n got %+v\nwant %+v", r.ID, def.AegisName, got, want)
					continue
				}
				matched++
			}
			t.Logf("%s: %d of %d monsters match mob_status.json", mode, matched, len(rec.Mobs))
		})
	}
}
