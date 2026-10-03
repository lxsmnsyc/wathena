package zone

import (
	"fmt"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/companion"
)

// mountCase is one case of mount_status.json: a player with every tree
// skill, a weapon, and a mount or none.
type mountCase struct {
	Job          int            `json:"job"`
	JobLevel     int            `json:"job_level"`
	Build        string         `json:"build"`
	Mount        string         `json:"mount"`
	Item         uint32         `json:"item"`
	Option       int            `json:"option"`
	MaxWeight    int            `json:"max_weight"`
	BattleStatus map[string]any `json:"battle_status"`
	Damage       *struct {
		ForcedMin goldenDamage `json:"forced_min"`
	} `json:"damage"`
	Skills [][2]int `json:"skills"`
}

// mountRecords are the core mounts of the recording's mount names.
var mountRecords = map[string]string{"riding": "peco_peco", "dragon": "dragon", "wug_rider": "warg", "madogear": "madogear"}

// TestMountParity puts each recorded player on its mount through core's
// mount records and compares its stats, its weight limit, its option
// bits and a forced-minimum normal attack on an Orc Warrior with
// rAthena's (mount_status.json): the mounted speed, the riding ASPD rule
// with Cavalier Mastery and Dragon Training, the spear against medium
// targets, and the Madogear's weight and speed.
func TestMountParity(t *testing.T) {
	const orcWarrior = 1023
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			var ref struct {
				Cases []mountCase `json:"cases"`
			}
			if err := reference.Load(mode, "mount_status", &ref); err != nil {
				t.Fatal(err)
			}
			if len(ref.Cases) == 0 {
				t.Fatal("no cases")
			}
			// Golden zones do not run item scripts, so the damage of a
			// weapon with a script, such as the Taurus Sword's bBaseAtk,
			// is left to the equipment parity tests.
			var scripts struct {
				Items []struct {
					ID     uint32 `json:"id"`
					Script string `json:"script"`
				} `json:"items"`
			}
			if err := reference.LoadFile("extracts/"+string(mode)+"/item_scripts.json", &scripts); err != nil {
				t.Fatal(err)
			}
			scripted := map[uint32]bool{}
			for _, it := range scripts.Items {
				scripted[it.ID] = it.Script != ""
			}
			matched, damage := 0, 0
			for i := range ref.Cases {
				c := &ref.Cases[i]
				name := fmt.Sprintf("job %d, %s, item %d", c.Job, c.Mount, c.Item)
				z := goldenZone(t, mode, orcWarrior)
				p := goldenPlayerIn(t, z, goldenPlayer{Job: c.Job, Build: c.Build, Sex: 1, JobLevel: c.JobLevel, Skills: c.Skills}, c.Item, 0)
				if r := mountRecords[c.Mount]; r != "" {
					if !z.setMount(p, companion.Core().Mounts.ByName(r)) {
						t.Errorf("%s: cannot mount", name)
						continue
					}
				}
				z.flushStats()
				var diffs []string
				if got := p.record.Options & companion.OptionLooks; got != c.Option {
					diffs = append(diffs, fmt.Sprintf("option %#x, want %#x", got, c.Option))
				}
				if p.maxWeight != c.MaxWeight {
					diffs = append(diffs, fmt.Sprintf("max weight %d, want %d", p.maxWeight, c.MaxWeight))
				}
				diffs = append(diffs, diffFields(statusFields(p.entity.status), c.BattleStatus)...)
				if c.Damage != nil && !scripted[c.Item] {
					damage++
					if got := goldenAttack(z, p.entity, monsterOf(z, orcWarrior)); !sameAttack(got, &c.Damage.ForcedMin) {
						diffs = append(diffs, fmt.Sprintf("attack %+v, want %+v", got, c.Damage.ForcedMin))
					}
				}
				if len(diffs) > 0 {
					t.Errorf("%s: %v", name, diffs)
					continue
				}
				matched++
			}
			t.Logf("%s: %d of %d mounted cases match, %d of them with damage", mode, matched, len(ref.Cases), damage)
		})
	}
}
