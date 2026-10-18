package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestClassicWorldProceduralBuild: the release binaries, built with
// -tags procedural, still run a classic world (strategy section 14.2,
// decision 0047):
//   - `wactl world new --mode classic` makes the world row with no
//     generator.
//   - waserver starts on it with the classic maps of --maps and serves no
//     generated maps.
//   - A new character joins on the start map, walks, and the fixture
//     mod's Guide sends it to the field map, which another zone runs.
func TestClassicWorldProceduralBuild(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true, Classic: true})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var mode string
	var generator int
	if err := srv.DB.QueryRow(ctx, "SELECT mode, generator_version FROM world").Scan(&mode, &generator); err != nil {
		t.Fatal(err)
	}
	if mode != "classic" || generator != 0 {
		t.Fatalf("the world row: mode %q, generator %d", mode, generator)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.HTTP()+"/gen/maps/gen:town:t1.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode == http.StatusOK {
		t.Error("a classic world serves a generated map")
	}

	p := srv.Join(t, "Classic")
	p.Until("on the classic start map", func(v View, _ Game) bool { return v.Map == StartMap })
	v := p.View()
	p.WalkTo(v.X+2, v.Y)
	p.Talk("Guide").Pick("Go to the field")
	p.MapChange(FieldMap)
	srv.CheckLogs()
}
