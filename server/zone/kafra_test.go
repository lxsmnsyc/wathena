package zone

import (
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// kafraZone is prontera with core's converted Kafra kaf_prontera, as
// waserver places it, and a player next to her with 5000 zeny.
func kafraZone(t *testing.T) (*Zone, *Player, *entity, *syncBuffer) {
	t.Helper()
	z := New("map/prontera", open(400), rng.FromKey([32]byte{9}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	coreMods(t, z)
	logs := &syncBuffer{}
	z.Mods.Log = slog.New(slog.NewTextHandler(logs, nil))
	z.Mods.Server = variables.NewServer(nil, nil)
	z.Mods.ItemIDs = map[string]uint32{"Warp_Free_Ticket": 7060, "Red_Potion": 501}
	core, err := testCore()
	if err != nil {
		t.Fatal(err)
	}
	var kafra NPC
	for _, n := range core.NPCs.NPCs {
		if n.Name == "kaf_prontera" {
			kafra = NPC{Name: "core:" + n.Name, DisplayName: "Kafra Employee", X: 152, Y: 326, Sprite: 112, Script: "core:" + n.Script}
		}
	}
	if kafra.Script == "core:" {
		t.Fatal("kaf_prontera did not convert")
	}
	z.SetNPCs([]NPC{kafra})
	t.Cleanup(z.stopScripts)
	c := savedCharacter()
	c.Map, c.X, c.Y, c.Zeny = "map/prontera", 153, 326, 5000
	p := addCharacter(z, c, &saves{})
	z.stepAt(50)
	events(p)
	var n *entity
	for e := range z.entities.all() {
		if e.npc != nil {
			n = e
		}
	}
	return z, p, n, logs
}

func choose(z *Zone, p *Player, ev []*wathenav1.Event, option string) []*wathenav1.Event {
	d := lastDialog(ev)
	for i, o := range d.GetMenu().GetOptions() {
		if o == option {
			return answer(z, p, reply(d.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Menu = new(uint32(i + 1)) }))
		}
	}
	return nil
}

func next(z *Zone, p *Player, ev []*wathenav1.Event) []*wathenav1.Event {
	return answer(z, p, reply(lastDialog(ev).GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Next = new(true) }))
}

func TestKafraSaves(t *testing.T) {
	z, p, n, logs := kafraZone(t)
	ev := click(z, p, n)
	var img *wathenav1.NpcImage
	for _, e := range ev {
		if e.GetNpcImage() != nil {
			img = e.GetNpcImage()
		}
	}
	if img == nil || img.GetName() != "kafra_06" || !strings.HasPrefix(lines(ev), "[Kafra Employee]|Welcome to the") {
		t.Fatalf("image %v, lines %q, logs %s", img, lines(ev), logs)
	}
	ev = next(z, p, ev)
	menu := lastDialog(ev).GetMenu().GetOptions()
	if strings.Join(menu, ",") != "Save,Use Storage,Use Teleport Service,Rent a Pushcart,Check Other Information,Cancel" {
		t.Fatalf("menu %q", menu)
	}
	// Storage is not available yet, and the menu comes back.
	ev = choose(z, p, ev, "Use Storage")
	if lines(ev) != "Storage is not available yet." {
		t.Fatalf("storage: %q", lines(ev))
	}
	ev = next(z, p, ev)
	ev = choose(z, p, ev, "Save")
	if p.record.SaveMap != "map/prontera" || p.record.SaveX != 157 || p.record.SaveY != 327 {
		t.Fatalf("save point %s %d,%d, logs %s", p.record.SaveMap, p.record.SaveX, p.record.SaveY, logs)
	}
	if got := lines(ev); !strings.Contains(got, "has been saved here|in the city of Prontera.") {
		t.Fatalf("after saving: %q", got)
	}
	if d := lastDialog(ev); d == nil || !d.HasClose() {
		t.Fatalf("no Close button: %v", d)
	}
}

func TestKafraTeleportCharges(t *testing.T) {
	z, p, n, logs := kafraZone(t)
	ev := next(z, p, click(z, p, n))
	ev = choose(z, p, ev, "Use Teleport Service")
	ev = next(z, p, ev)
	menu := lastDialog(ev).GetMenu().GetOptions()
	if len(menu) == 0 || menu[0] != "Izlude -> 600 z" || menu[len(menu)-1] != "Cancel" {
		t.Fatalf("destinations %q, logs %s", menu, logs)
	}
	choose(z, p, ev, "Izlude -> 600 z")
	// Izlude does not run in this world, so the warp does not happen,
	// but the fee is paid, and Special Reserve Points go up by 600 / 16.
	if p.record.Zeny != 4400 {
		t.Fatalf("zeny %d, logs %s", p.record.Zeny, logs)
	}
	if v := p.vars[character.Variable{Mod: "core", Name: "resrvpts"}]; v != "37" {
		t.Errorf("reserve points %q", v)
	}
	// The script ended: only the Close button is left.
	if p.talk != nil && !p.talk.ended {
		t.Errorf("the conversation goes on after the teleport: %+v", p.talk)
	}
}
