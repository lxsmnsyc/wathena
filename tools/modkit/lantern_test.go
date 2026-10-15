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
	"github.com/lxsmnsyc/wathena/server/gamedata"
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
	if err != nil || b.Assets != 17 {
		t.Fatalf("art: %v, %+v", err, b)
	}
	l, err := e.Load(ctx, dir, "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.RunTests(ctx, l, "", func(CaseResult) {})
	if err != nil || len(results) != 7 {
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
	if unlit := doc.Items[id("example.lantern:Unlit_Lantern")]; unlit.ItemArt != (modassets.ItemArt{}) || unlit.Name != "Unlit Lantern" {
		t.Errorf("the unlit lantern has art %+v", unlit)
	}
	// Version 2: the names of the mod's records and the item text.
	if lantern.Name != "Paper Lantern" || len(lantern.Description) != 2 || lantern.Description[1] != "^777777Weight:^000000 1" {
		t.Errorf("lantern text %+v", lantern)
	}
	hat := doc.Items[id("example.lantern:Lantern_Hat")]
	if hat.Name != "Lantern Hat" || hat.ViewSprite != "mod/example.lantern/sprite/headgear/Lantern_Hat" ||
		hat.View != gamedata.ItemArtViews || hat.Icon != "mod/example.lantern/icon/item/Lantern_Hat" {
		t.Errorf("hat %+v", hat)
	}
	for _, d := range l.Items.Data.Items {
		if d.AegisName == "example.lantern:Lantern_Hat" && d.View != gamedata.ItemArtViews {
			t.Errorf("the hat's view is %d", d.View)
		}
	}
	// The shield and the wisp of decision 0036: a shield's worn sprite
	// under the next view, and the wisp's art under its record ID.
	shield := doc.Items[id("example.lantern:Lantern_Shield")]
	if shield.ViewSprite != "mod/example.lantern/sprite/shield/Lantern_Shield" || shield.View != gamedata.ItemArtViews+1 ||
		shield.Icon != "mod/example.lantern/icon/item/Lantern_Shield" || shield.Slots != 1 {
		t.Errorf("shield %+v", shield)
	}
	wisp := l.Zone.Companions.Companions.ByName("example.lantern:Lantern_Wisp")
	if wisp == nil || doc.Companions[strconv.Itoa(wisp.ID)] != "mod/example.lantern/sprite/companion/Lantern_Wisp" {
		t.Errorf("wisp %+v, companions %v", wisp, doc.Companions)
	}
	if _, ok := doc.Items["501"]; ok {
		t.Error("a core item without art has an entry")
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
	if g := doc.Monsters[ghost]; g.Name != "Lantern Ghost" || g.Sounds["die"] != "mod/example.lantern/sound/Lantern_Ghost/die" {
		t.Errorf("ghost %+v", g)
	}
	keeper, ok := l.Jobs.ID("example.lantern:Lantern_Keeper")
	if j := doc.Jobs[strconv.Itoa(keeper)]; !ok || j.Name != "Lantern Keeper" || j.Body["m"] != "mod/example.lantern/sprite/body/Lantern_Keeper/m" ||
		j.Body["f"] != "mod/example.lantern/sprite/body/Lantern_Keeper/f" || j.Look != 5 || !l.Zone.OwnLookJobs[keeper] {
		t.Errorf("keeper %d: %+v", keeper, j)
	}
	flare := l.Skills.ByName("example.lantern:LANTERN_FLARE")
	if s := doc.Skills[strconv.Itoa(flare.ID)]; s.Name != "Lantern Flare" || s.Icon != "mod/example.lantern/icon/skill/LANTERN_FLARE" ||
		len(s.Effects["hit"]) != 1 || s.Effects["hit"][0] != "mod/example.lantern/effect/flare" {
		t.Errorf("flare %+v", s)
	}
	if doc.NPCSprites["1000000"] != "mod/example.lantern/sprite/npc/Lamplighter" {
		t.Errorf("npc sprites %v", doc.NPCSprites)
	}
	if !strings.Contains(string(a.ClientData), `"hit":[1,17]`) {
		t.Error("the client data lacks the mod's skills.yml")
	}
}
