package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/schedule"
)

// rotationConfig is a small world layout with one dungeon, which core
// marks as rotating, in UTC.
const rotationConfig = `time_zone: UTC
procedural:
  field_size: 64
  layout: {grid: [2, 2], towns: 2, dungeons: 1}
`

// mapStatus returns the HTTP status of a generated map's info.
func mapStatus(t *testing.T, srv *Server, id string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.HTTP()+"/gen/maps/"+url.PathEscape(id)+".json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	return res.StatusCode
}

// rotationRow is the world's rotating dungeon.
type rotationRow struct {
	dungeon string
	rule    layout.RotationRule
	date    string
}

func readRotation(ctx context.Context, t *testing.T, srv *Server) rotationRow {
	t.Helper()
	var r rotationRow
	var date *time.Time
	err := srv.DB.QueryRow(ctx, `SELECT dungeon, every, at, weekday, active_date FROM world_rotations`).
		Scan(&r.dungeon, &r.rule.Every, &r.rule.At, &r.rule.Weekday, &date)
	if err != nil {
		t.Fatal(err)
	}
	if date != nil {
		r.date = date.Format("2006-01-02")
	}
	return r
}

// points reads a character's map, save point and memo points.
func points(ctx context.Context, t *testing.T, srv *Server, name string) (pos [3]any, save [3]any, memos []string) {
	t.Helper()
	var id int64
	var m, sm string
	var x, y, sx, sy int
	err := srv.DB.QueryRow(ctx, `SELECT id, map, x, y, save_map, save_x, save_y FROM characters WHERE name = $1`, name).
		Scan(&id, &m, &x, &y, &sm, &sx, &sy)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := srv.DB.Query(ctx, `SELECT map_id, x, y FROM character_memos WHERE character_id = $1 ORDER BY slot`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var mm string
		var mx, my int
		if err := rows.Scan(&mm, &mx, &my); err != nil {
			t.Fatal(err)
		}
		memos = append(memos, fmt.Sprintf("%s %d,%d", mm, mx, my))
	}
	return [3]any{m, x, y}, [3]any{sm, sx, sy}, memos
}

// offlineOn puts an offline character's save point and a memo point on
// a map, as the database keeps them.
func offlineOn(ctx context.Context, t *testing.T, srv *Server, name, mapID string) {
	t.Helper()
	if _, err := srv.DB.Exec(ctx, `UPDATE characters SET save_map = $2, save_x = 5, save_y = 6 WHERE name = $1`, name, mapID); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(ctx, `DELETE FROM character_memos WHERE character_id = (SELECT id FROM characters WHERE name = $1)`, name); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(ctx, `INSERT INTO character_memos (character_id, slot, map_id, x, y)
		SELECT id, 0, $2, 7, 8 FROM characters WHERE name = $1`, name, mapID); err != nil {
		t.Fatal(err)
	}
}

// TestRotation proves the rotation exit criteria of phase 7 with
// PostgreSQL (strategy section 15, decision 0045):
//   - A rotating dungeon changes on schedule without staff action: the
//     server clock moves to 9 minutes before the dungeon's next time, a
//     player inside gets the warning, and at the time new floors with the
//     next date run and the old ones are gone.
//   - The player inside moves to the nearest town, and an offline
//     player's save point and memo point on the old floor move to that
//     town's save spot, in the database.
//   - Restarts: a rotation that came while the server was stopped happens
//     at the next start, with the moves of its old floors.
func TestRotation(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true, Procedural: true, Config: rotationConfig})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	rot := readRotation(ctx, t, srv)
	spec, err := rot.rule.Spec()
	if err != nil || rot.date == "" {
		t.Fatalf("the rotating dungeon %+v: %v", rot, err)
	}
	d := rot.dungeon
	old1 := layout.FloorID(d, rot.date, 1)
	if s := mapStatus(t, srv, old1); s != http.StatusOK {
		t.Fatalf("the floor %s: %d", old1, s)
	}
	if s := mapStatus(t, srv, gen1(d)); s != http.StatusNotFound {
		t.Errorf("the stored floor runs: %d", s)
	}
	info := readMap(t, srv, old1)
	if info.Kind != "dungeon" || !strings.HasSuffix(info.DisplayName, " F1") {
		t.Errorf("the floor %+v", info)
	}

	// An offline character with its points on the first floor.
	away := srv.Join(t, "Away")
	away.Until("in the world", func(v View, _ Game) bool { return v.Map != "" })
	_ = away.Close()
	for range 100 {
		var n int
		if err := srv.DB.QueryRow(ctx, `SELECT count(*) FROM characters WHERE name = 'Away'`).Scan(&n); err == nil && n == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	offlineOn(ctx, t, srv, "Away", old1)

	// A hidden player inside the first floor.
	p := srv.Join(t, "Delver")
	if err := srv.GiveStaffRole(ctx, "Delver", "admin"); err != nil {
		t.Fatal(err)
	}
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, "@hide") })
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, "@warp "+old1) })
	p.MapChange(old1)

	// The server clock goes to 9 minutes before the next rotation.
	now := time.Now().UTC()
	next := schedule.Next(spec, now, time.UTC)
	nextDate := layout.DateOf(next, time.UTC)
	if lead := next.Sub(now) - 9*time.Minute; lead > 0 {
		if err := srv.AdvanceClock(ctx, lead); err != nil {
			t.Fatal(err)
		}
	}
	warning := p.Expect("the rotation's warning", func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_ANNOUNCEMENT && strings.Contains(c.GetText(), "closes for new floors in")
	}).GetChat().GetText()
	townName := strings.TrimSuffix(warning[strings.Index(warning, "goes to ")+len("goes to "):], ".")
	if !strings.HasPrefix(warning, info.DisplayName) {
		t.Errorf("the warning %q", warning)
	}
	// At the time, without staff action.
	if err := srv.AdvanceClock(ctx, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	var arrived *wathenav1.MapChange
	p.Expect("the move to the nearest town", func(e *wathenav1.Event) bool {
		if m := e.GetMapChange(); m != nil && strings.HasPrefix(m.GetMap(), "gen:town:") {
			arrived = m
			return true
		}
		return false
	})
	town := readMap(t, srv, arrived.GetMap())
	if town.DisplayName != townName {
		t.Errorf("moved to %s (%s), the warning named %s", town.ID, town.DisplayName, townName)
	}
	new1 := layout.FloorID(d, nextDate, 1)
	for range 100 {
		if readRotation(ctx, t, srv).date == nextDate {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if r := readRotation(ctx, t, srv); r.date != nextDate {
		t.Fatalf("the stored date %s, want %s", r.date, nextDate)
	}
	if s := mapStatus(t, srv, new1); s != http.StatusOK {
		t.Errorf("the new floor %s: %d", new1, s)
	}
	if s := mapStatus(t, srv, old1); s != http.StatusNotFound {
		t.Errorf("the old floor %s: %d", old1, s)
	}
	if fresh := readMap(t, srv, new1); fresh.DisplayName != info.DisplayName {
		t.Errorf("the new floor's name %q", fresh.DisplayName)
	}
	// The offline character's points are at the town's save spot.
	want := [3]any{town.ID, int(arrived.GetX()), int(arrived.GetY())}
	wantMemo := fmt.Sprintf("%s %d,%d", town.ID, arrived.GetX(), arrived.GetY())
	if _, save, memos := points(ctx, t, srv, "Away"); save != want || len(memos) != 1 || memos[0] != wantMemo {
		t.Errorf("the offline points: save %v, memos %v; want %v", save, memos, want)
	}
	// The old floor no longer runs; the new one does.
	if got := p.GMCommand("@warp " + old1); got == "Warped." {
		t.Error("a warp to the old floor")
	}
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, "@warp "+new1) })
	p.MapChange(new1)
	_ = p.Close()

	// Restarts: the server stops over a rotation. At the next start the
	// rotation happens, and the points on the old floors move.
	srv.Stop()
	older := "2020-01-03"
	if _, err := srv.DB.Exec(ctx, `UPDATE world_rotations SET active_date = $1`, older); err != nil {
		t.Fatal(err)
	}
	offlineOn(ctx, t, srv, "Away", layout.FloorID(d, older, 1))
	srv.Restart()
	if r := readRotation(ctx, t, srv); r.date <= older {
		t.Errorf("after the restart the date is %s", r.date)
	}
	if _, save, memos := points(ctx, t, srv, "Away"); !strings.HasPrefix(save[0].(string), "gen:town:") || len(memos) != 1 || !strings.HasPrefix(memos[0], "gen:town:") {
		t.Errorf("after the restart: save %v, memos %v", save, memos)
	}
	found := false
	for _, l := range srv.logs.lines() {
		if strings.Contains(l, "world changed while the server was stopped") && strings.Contains(l, "first=false") && strings.Contains(l, d) {
			found = true
		}
	}
	if !found {
		t.Error("no rotation at the start in the log")
	}
}

func gen1(d string) string { return "gen:dungeon:" + d + ":1" }
