package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// distanceRecord is distance.json: rAthena's distance functions for
// every offset up to 20 cells, and the smallest range at which each
// range check passes.
type distanceRecord struct {
	CircularArea bool `json:"circular_area"`
	Cells        []struct {
		DX                  int `json:"dx"`
		DY                  int `json:"dy"`
		Distance            int `json:"distance"`
		DistanceClient      int `json:"distance_client"`
		CheckMinRange       int `json:"check_min_range"`
		CheckClientMinRange int `json:"check_client_min_range"`
	} `json:"cells"`
}

// TestDistanceMatchesReference compares distance, distance_client,
// check_distance and check_distance_client with distance.json in both
// modes, and battle_check_range on an open map for a player, which uses
// the client's distance, and for a monster, which uses the square one.
func TestDistanceMatchesReference(t *testing.T) {
	z := New("map/test", open(60), rng.FromKey([32]byte{3}).Stream(), nil)
	const ox, oy = 30, 30
	player := &entity{x: ox, y: oy, player: &Player{}}
	monster := &entity{x: ox, y: oy}
	for _, mode := range reference.Modes {
		var ref distanceRecord
		if err := reference.Load(mode, "distance", &ref); err != nil {
			t.Fatal(err)
		}
		if ref.CircularArea {
			t.Fatalf("%s: recorded with CIRCULAR_AREA, which wAthena's checks do not use", mode)
		}
		matched, ranges := 0, 0
		for _, c := range ref.Cells {
			to := &entity{x: ox + c.DX, y: oy + c.DY}
			minRange := 0
			for !checkDistance(c.DX, c.DY, minRange, false) {
				minRange++
			}
			got := [4]int{chebyshev(monster, to), clientDistance(c.DX, c.DY), minRange, clientDistance(c.DX, c.DY)}
			want := [4]int{c.Distance, c.DistanceClient, c.CheckMinRange, c.CheckClientMinRange}
			if got != want {
				t.Errorf("%s: offset %d,%d: distance, client distance, check ranges %v, want %v", mode, c.DX, c.DY, got, want)
				continue
			}
			matched++
			// battle_check_range refuses anything beyond the view range.
			if max(abs(c.DX), abs(c.DY)) > ViewRange {
				continue
			}
			for _, r := range []int{c.CheckClientMinRange - 1, c.CheckClientMinRange} {
				if r >= 0 && z.checkRange(player, to.x, to.y, r) != (r >= c.CheckClientMinRange) {
					t.Errorf("%s: player range %d to offset %d,%d: in range %v", mode, r, c.DX, c.DY, r < c.CheckClientMinRange)
				}
			}
			for _, r := range []int{c.CheckMinRange - 1, c.CheckMinRange} {
				if r >= 0 && z.checkRange(monster, to.x, to.y, r) != (r >= c.CheckMinRange) {
					t.Errorf("%s: monster range %d to offset %d,%d: in range %v", mode, r, c.DX, c.DY, r < c.CheckMinRange)
				}
			}
			ranges++
		}
		t.Logf("%s: %d of %d offsets match distance.json; range checks at %d offsets within the view range", mode, matched, len(ref.Cells), ranges)
		if matched != len(ref.Cells) || len(ref.Cells) != 41*41 {
			t.Errorf("%s: %d of %d offsets match", mode, matched, len(ref.Cells))
		}
	}
}
