package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// spawnConfig is a small world: 2 × 2 fields of 64 cells, two towns
// and a dungeon, with crowded fields.
const spawnConfig = `procedural:
  field_size: 64
  layout: {grid: [2, 2], towns: 2, dungeons: 1}
spawns:
  density: {field: 400}
`

// TestWorldSpawns plays a procedural world that wactl world new made,
// with PostgreSQL (decision 0042):
//   - The database keeps a spawn list for every field and dungeon floor,
//     and the Mossy Fields sample's hook put 12 Moss Porings on each
//     field below level 20.
//   - A player who looks at each quarter of such a field sees its stored
//     monsters, Moss Porings among them, and no other monster.
func TestWorldSpawns(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true, Procedural: true, Config: spawnConfig})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var maps, listed int
	if err := srv.DB.QueryRow(ctx, "SELECT count(*) FROM world_maps WHERE kind IN ('field', 'dungeon')").Scan(&maps); err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.QueryRow(ctx, "SELECT count(DISTINCT map_id) FROM world_spawns").Scan(&listed); err != nil {
		t.Fatal(err)
	}
	if maps == 0 || listed != maps {
		t.Fatalf("%d of %d fields and floors have spawns", listed, maps)
	}
	var field string
	if err := srv.DB.QueryRow(ctx, "SELECT id FROM world_maps WHERE kind = 'field' AND min_level < 20 ORDER BY ord LIMIT 1").Scan(&field); err != nil {
		t.Fatal(err)
	}
	rows, err := srv.DB.Query(ctx, "SELECT monster, count FROM world_spawns WHERE map_id = $1 ORDER BY ord", field)
	if err != nil {
		t.Fatal(err)
	}
	stored := map[string]int{}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			t.Fatal(err)
		}
		stored[name] += count
	}
	if stored["example.mossy-fields:MOSS_PORING"] != 12 || len(stored) < 5 {
		t.Fatalf("%s's spawns %v", field, stored)
	}
	// The names players see of the stored monsters.
	monsters, err := gamedata.LoadMonsters(reference.Renewal, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{"Moss Poring": true, "Moss Spore": true}
	for _, m := range monsters {
		if stored[m.AegisName] > 0 {
			names[m.Name] = true
		}
	}
	p := srv.Join(t, "Spotter")
	if err := srv.GiveStaffRole(ctx, "Spotter", "admin"); err != nil {
		t.Fatal(err)
	}
	// Hidden, so no monster attacks it, the player looks at each quarter
	// of the field in turn.
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, "@hide") })
	seen := map[string]int{}
	for i, cell := range [][2]int{{16, 16}, {48, 16}, {16, 48}, {48, 48}} {
		from := p.View()
		p.send("chat", func(ctx context.Context) error {
			return p.Chat(ctx, fmt.Sprintf("@warp %s %d %d", field, cell[0], cell[1]))
		})
		if i == 0 {
			p.MapChange(field)
		}
		p.Until("monsters in sight near "+fmt.Sprint(cell), func(v botclient.View, _ botclient.Game) bool {
			if v.Map != field || (v.X == from.X && v.Y == from.Y && from.Map == field) {
				return false
			}
			found := false
			for _, e := range v.Entities {
				if e.Kind == wathenav1.EntityKind_ENTITY_KIND_MONSTER {
					seen[e.Name]++
					found = true
				}
			}
			return found
		})
	}
	if seen["Moss Poring"] == 0 || len(seen) < 3 {
		t.Errorf("seen %v on %s", seen, field)
	}
	for name := range seen {
		if !names[name] {
			t.Errorf("a %s on %s, which stores %v", name, field, stored)
		}
	}
	t.Logf("%s: seen %v", field, seen)
	if got := p.View().Map; got != field {
		t.Errorf("the player is on %s, not %s", got, field)
	}
	srv.CheckLogs()
}
