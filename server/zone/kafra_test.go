package zone

import (
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// kafraItems are the items the Kafra tests use: the free tickets of
// storage, teleport and cart rental.
func kafraItems() *item.Table {
	return item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: 501, AegisName: "Red_Potion", Type: "Healing", Weight: 70, Stackable: true, Script: "core:scripts/items/Red_Potion.lua"},
		{ID: 7059, AegisName: "Cargo_Free_Ticket", Type: "Etc", Weight: 1, Stackable: true},
		{ID: 7060, AegisName: "Warp_Free_Ticket", Type: "Etc", Weight: 1, Stackable: true},
		{ID: 7061, AegisName: "Cart_Free_Ticket", Type: "Etc", Weight: 1, Stackable: true},
	}})
}

// kafraZone is prontera with core's converted Kafra kaf_prontera, as
// waserver places it, and a player next to her with 5000 zeny. setup
// changes the player's character before it joins.
func kafraZone(t *testing.T, setup ...func(*character.Character)) (*Zone, *Player, *entity, *syncBuffer) {
	t.Helper()
	return kafraZoneWith(t, nil, setup...)
}

// kafraZoneWith is kafraZone with a world's ruleset, or the core one for
// nil.
func kafraZoneWith(t *testing.T, rl *rules.Rules, setup ...func(*character.Character)) (*Zone, *Player, *entity, *syncBuffer) {
	t.Helper()
	z := New("map/prontera", open(400), rng.FromKey([32]byte{9}).Stream(), nil)
	z.Items = kafraItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	coreMods(t, z)
	if rl != nil {
		z.Mods.Rules = rl
	}
	logs := &syncBuffer{}
	z.Mods.Log = slog.New(slog.NewTextHandler(logs, nil))
	z.Mods.Server = variables.NewServer(nil, nil)
	z.Mods.ItemIDs = map[string]uint32{"Cargo_Free_Ticket": 7059, "Warp_Free_Ticket": 7060, "Cart_Free_Ticket": 7061, "Red_Potion": 501}
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
	for _, f := range setup {
		f(c)
	}
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
	// A Novice without Basic Skill level 6 cannot use storage, as
	// F_CanOpenStorage, and the menu comes back.
	ev = choose(z, p, ev, "Use Storage")
	if lines(ev) != "[Kafra Employee]|I'm sorry, but you|need the Novice's|Basic Skill Level 6 to|use the Storage Service." {
		t.Fatalf("storage: %q, logs %s", lines(ev), logs)
	}
	if p.storage != nil || p.record.Zeny != 5000 {
		t.Fatalf("storage %v, zeny %d", p.storage, p.record.Zeny)
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

// TestKafraBasicSkillRule runs the converted F_CanOpenStorage with the
// rule basic_skill_check off: its basicskillcheck() reads the rule with
// rules.get (review decision 32), so a Novice without Basic Skill level
// 6 opens storage.
func TestKafraBasicSkillRule(t *testing.T) {
	rl := rules.Base(true)
	var err error
	if rl.Values, err = rules.RuleValuesOf(map[string]int{"basic_skill_check": 0}); err != nil {
		t.Fatal(err)
	}
	if rl.Items, err = rules.ItemsOf(map[string]int{"basic_skill_check": 0}); err != nil {
		t.Fatal(err)
	}
	z, p, n, logs := kafraZoneWith(t, rl)
	personalStorageOpened(t, p, kafraStorage(t, z, p, n, logs))
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

// kafraStorage chooses Use Storage at the Kafra, and returns the events
// up to the storage window.
func kafraStorage(t *testing.T, z *Zone, p *Player, n *entity, logs *syncBuffer) []*wathenav1.Event {
	t.Helper()
	ev := next(z, p, click(z, p, n))
	ev = choose(z, p, ev, "Use Storage")
	if !strings.Contains(lines(ev), "Here, let me open|your Storage for you.") {
		t.Fatalf("storage: %q, logs %s", lines(ev), logs)
	}
	// close2 waits for the Close button, and then the storage opens.
	d := lastDialog(ev)
	if d == nil || !d.HasClose() {
		t.Fatalf("no Close button: %v", d)
	}
	return answer(z, p, reply(d.GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
}

// personalStorageOpened checks that the personal storage opened.
func personalStorageOpened(t *testing.T, p *Player, ev []*wathenav1.Event) {
	t.Helper()
	so := eventsOf2(ev, (*wathenav1.Event).GetStorageOpen, (*wathenav1.Event).HasStorageOpen)
	if len(so) != 1 || so[0].GetKind() != wathenav1.StorageKind_STORAGE_KIND_PERSONAL || p.storage == nil {
		t.Fatalf("storage open %v, open %v", so, p.storage)
	}
}

func TestKafraStorageCharges(t *testing.T) {
	z, p, n, logs := kafraZone(t, func(c *character.Character) { c.Skills = map[int]int{skillBasic: 6} })
	ev := kafraStorage(t, z, p, n, logs)
	personalStorageOpened(t, p, ev)
	// Prontera's fee is 40 zeny, and Special Reserve Points go up by
	// 40 / 5.
	if p.record.Zeny != 4960 {
		t.Errorf("zeny %d, logs %s", p.record.Zeny, logs)
	}
	if v := p.vars[character.Variable{Mod: "core", Name: "resrvpts"}]; v != "8" {
		t.Errorf("reserve points %q", v)
	}
	if p.talk != nil && !p.talk.ended {
		t.Errorf("the conversation goes on after the storage opened: %+v", p.talk)
	}
}

func TestKafraStorageTicket(t *testing.T) {
	z, p, n, logs := kafraZone(t, func(c *character.Character) {
		c.Skills = map[int]int{skillBasic: 9}
		c.Items = []character.Item{{Slot: 0, ID: 7059, Amount: 2, Serial: item.NewSerial()}}
	})
	ev := kafraStorage(t, z, p, n, logs)
	personalStorageOpened(t, p, ev)
	// The Free Ticket for Kafra Storage pays instead of zeny.
	if p.record.Zeny != 5000 || p.inventory.Count(7059) != 1 {
		t.Errorf("zeny %d, tickets %d, logs %s", p.record.Zeny, p.inventory.Count(7059), logs)
	}
}

// kafraCart chooses Rent a Pushcart at the Kafra, and returns the events.
func kafraCart(z *Zone, p *Player, n *entity) []*wathenav1.Event {
	return choose(z, p, next(z, p, click(z, p, n)), "Rent a Pushcart")
}

// merchant makes the Kafra tests' character a Merchant with Pushcart.
func merchant(c *character.Character) {
	c.Job, c.Skills = jobMerchant, map[int]int{skill.IDPushcart: 1}
}

// jobMerchant is Job_Merchant.
const jobMerchant = 5

// TestKafraCartRental runs the converted F_KafCart (review decision 55):
// the Pushcart rental of Prontera's Kafra costs 800 zeny, gives 48
// Special Reserve Points, and gives the cart through player:set_cart.
func TestKafraCartRental(t *testing.T) {
	z, p, n, logs := kafraZone(t, merchant)
	z.Status = jobTables(t)
	ev := kafraCart(z, p, n)
	if got := lines(ev); got != "[Kafra Employee]|The Pushcart rental|fee is 800 zeny. Would|you like to rent a Pushcart?" {
		t.Fatalf("cart: %q, logs %s", got, logs)
	}
	ev = next(z, p, ev)
	ev = choose(z, p, ev, "Rent a Pushcart.")
	if p.record.Zeny != 4200 || companion.CartLook(p.record.Options) != 1 {
		t.Fatalf("zeny %d, cart %d, logs %s", p.record.Zeny, companion.CartLook(p.record.Options), logs)
	}
	if v := p.vars[character.Variable{Mod: "core", Name: "resrvpts"}]; v != "48" {
		t.Errorf("reserve points %q", v)
	}
	// F_KafCart returns 1, so F_Kafra waits for Next and shows its menu.
	ev = next(z, p, ev)
	if d := lastDialog(ev); d == nil || len(d.GetMenu().GetOptions()) == 0 {
		t.Fatalf("no menu after the rental: %q, logs %s", lines(ev), logs)
	}
	// A second rental is refused: the player has a cart.
	ev = choose(z, p, ev, "Rent a Pushcart")
	if got := lines(ev); !strings.Contains(got, "You already have|a Pushcart equipped.") || p.record.Zeny != 4200 {
		t.Fatalf("second rental: %q, zeny %d", got, p.record.Zeny)
	}
}

// TestKafraCartTicket checks that a Free Ticket for the Cart Service pays
// for the cart instead of zeny, without a question.
func TestKafraCartTicket(t *testing.T) {
	z, p, n, logs := kafraZone(t, merchant, func(c *character.Character) {
		c.Items = []character.Item{{Slot: 0, ID: 7061, Amount: 1, Serial: item.NewSerial()}}
	})
	z.Status = jobTables(t)
	kafraCart(z, p, n)
	if p.record.Zeny != 5000 || p.inventory.Count(7061) != 0 || companion.CartLook(p.record.Options) != 1 {
		t.Fatalf("zeny %d, tickets %d, cart %d, logs %s", p.record.Zeny, p.inventory.Count(7061), companion.CartLook(p.record.Options), logs)
	}
}

// TestKafraCartRefusals checks who cannot rent a cart: a class other
// than a Merchant's, and a Merchant without Pushcart.
func TestKafraCartRefusals(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(*character.Character)
		want  string
	}{
		{"novice", func(*character.Character) {}, "is only available to Merchant classes."},
		{"no skill", func(c *character.Character) { c.Job = jobMerchant }, "You can only rent a cart after learning the \"Push Cart\" skill."},
	} {
		t.Run(c.name, func(t *testing.T) {
			z, p, n, logs := kafraZone(t, c.setup)
			z.Status = jobTables(t)
			ev := kafraCart(z, p, n)
			if got := lines(ev); !strings.Contains(got, c.want) {
				t.Fatalf("lines %q, logs %s", got, logs)
			}
			if p.record.Zeny != 5000 || companion.CartLook(p.record.Options) != 0 {
				t.Fatalf("zeny %d, cart %d", p.record.Zeny, companion.CartLook(p.record.Options))
			}
		})
	}
}
