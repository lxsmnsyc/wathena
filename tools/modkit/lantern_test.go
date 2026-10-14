package modkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modassets"
)

// TestLantern validates the lantern sample, runs its tests, builds its
// art into a copy, and checks the client data the server would send: the
// icon by the item's name, the icon field of another item, the ghost's
// sprite, and the Lamplighter's sprite under a sprite ID of its own.
func TestLantern(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	e := env(t)
	ctx := context.Background()
	rep := e.Validate(ctx, "../../mods/examples/lantern", "")
	if len(rep.Problems) > 0 {
		t.Fatalf("validate: %v", rep.Problems)
	}
	dir := filepath.Join(t.TempDir(), "lantern")
	if err := os.CopyFS(dir, os.DirFS("../../mods/examples/lantern")); err != nil {
		t.Fatal(err)
	}
	b, err := BuildArt(dir, "", 1)
	if err != nil || b.Assets != 4 {
		t.Fatalf("art: %v, %+v", err, b)
	}
	l, err := e.Load(ctx, dir, "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.RunTests(ctx, l, "", func(CaseResult) {})
	if err != nil || len(results) != 2 {
		t.Fatalf("tests: %v, %d cases", err, len(results))
	}
	for _, r := range results {
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}
	a, err := modassets.Load(l.Mods, modassets.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	var doc modassets.ClientData
	if err := json.Unmarshal(a.ClientData, &doc); err != nil {
		t.Fatal(err)
	}
	id := func(name string) string { return strconv.FormatUint(uint64(l.Items.IDs[name]), 10) }
	lantern := doc.Items[id("example.lantern:Paper_Lantern")]
	if lantern.Icon != "mod/example.lantern/icon/item/Paper_Lantern" || lantern.DropSprite != "mod/example.lantern/sprite/item/Paper_Lantern" {
		t.Errorf("lantern art %+v", lantern)
	}
	if oil := doc.Items[id("example.lantern:Lantern_Oil")]; oil.Icon != "mod/example.lantern/icon/item/Paper_Lantern" {
		t.Errorf("oil art %+v", oil)
	}
	if unlit, ok := doc.Items[id("example.lantern:Unlit_Lantern")]; ok {
		t.Errorf("the unlit lantern has art %+v", unlit)
	}
	ghost := ""
	for monsterID, m := range doc.Monsters {
		if m.Sprite == "mod/example.lantern/sprite/mob/Lantern_Ghost" {
			ghost = monsterID
		}
	}
	if n, _ := strconv.Atoi(ghost); n < int(mod.MonsterIDs.Start) {
		t.Errorf("ghost sprite under monster %q", ghost)
	}
	if doc.NPCSprites["1000000"] != "mod/example.lantern/sprite/npc/Lamplighter" {
		t.Errorf("npc sprites %v", doc.NPCSprites)
	}
	if !strings.Contains(string(a.ClientData), `"hit":[1,17]`) {
		t.Error("the client data lacks the mod's skills.yml")
	}
}
