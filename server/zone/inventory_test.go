package zone

import (
	"io/fs"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/shared/rng"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// testItems are items as rAthena's renewal item_db has them, with core's
// converted scripts, one whose script needs status effects, and one that
// may not be dropped.
func testItems() *item.Table {
	return item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: 501, AegisName: "Red_Potion", Type: "Healing", Weight: 70, Stackable: true, Script: "core:scripts/items/Red_Potion.lua"},
		{ID: 505, AegisName: "Blue_Potion", Type: "Healing", Weight: 150, Stackable: true, Script: "core:scripts/items/Blue_Potion.lua"},
		{ID: 506, AegisName: "Green_Potion", Type: "Healing", Weight: 70, Stackable: true, Unsupported: "status effects"},
		{ID: 601, AegisName: "Wing_Of_Fly", Type: "DelayConsume", Weight: 50, Stackable: true, Script: "core:scripts/items/Wing_Of_Fly.lua"},
		{ID: 602, AegisName: "Wing_Of_Butterfly", Type: "DelayConsume", Weight: 50, Stackable: true, Script: "core:scripts/items/Wing_Of_Butterfly.lua"},
		{ID: 603, AegisName: "Old_Blue_Box", Type: "Usable", Weight: 200, Stackable: true, Script: "core:scripts/items/Old_Blue_Box.lua"},
		{ID: 607, AegisName: "Yggdrasilberry", Type: "Healing", Weight: 300, Stackable: true, DelayMs: 5000, DelayStatus: "Reuse_Limit_F", Script: "core:scripts/items/Yggdrasilberry.lua"},
		{ID: 909, AegisName: "Jellopy", Type: "Etc", Weight: 10, Stackable: true},
		{ID: 1201, AegisName: "Knife", Type: "Weapon", Weight: 400},
		{ID: 7777, AegisName: "Untradable", Type: "Etc", Weight: 1, Stackable: true, NoDrop: true},
	}})
}

// testCore is core's converted renewal item scripts, made once.
var testCore = sync.OnceValues(func() (*convert.Core, error) { return convert.LoadCore(reference.Renewal) })

// coreMods gives a zone core's item scripts, with every permission, as
// waserver does.
func coreMods(t *testing.T, z *Zone) {
	t.Helper()
	core, err := testCore()
	if err != nil {
		t.Fatal(err)
	}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	err = fs.WalkDir(core.Files, "scripts", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(core.Files, p)
		sc.Files[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	groups, err := item.NewGroups(core.Groups, func(name string) (uint32, bool) {
		for _, n := range []struct {
			name string
			id   uint32
		}{{"Red_Potion", 501}, {"Blue_Potion", 505}, {"Jellopy", 909}} {
			if n.name == name {
				return n.id, true
			}
		}
		return 999_999, true
	})
	if err != nil {
		t.Fatal(err)
	}
	z.Mods = &Mods{Scripts: []*mod.Scripts{sc}, IDs: []string{mod.Core}, Constants: core.Constants, Groups: groups, MonsterIDs: core.Monsters}
}

// maxWeight is the test players' maximum weight: 100.0.
const maxWeight = 1000

// invZone makes a zone with the test items and two players next to each
// other, with VIT 10, INT 5, 200 max HP and 50 max SP, holding the start
// items.
func invZone(t *testing.T, start ...StartItem) (*Zone, *Player, *Player) {
	t.Helper()
	z := New("map/test", open(40), rng.FromKey([32]byte{5}).Stream(), nil)
	z.Items = testItems()
	z.SaveMap = "map/test"
	coreMods(t, z)
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Vit: 10, Int: 5, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	// Without job tables, the maximum weight is set by hand before the
	// start items go in.
	a, b := add(z, "a"), add(z, "b")
	z.StartItems = start
	for _, p := range []*Player{a, b} {
		p.maxWeight = maxWeight
		z.giveStartItems(p)
	}
	b.entity.x = a.entity.x + 1
	z.stepAt(50)
	events(a)
	events(b)
	return z, a, b
}

// giveStartItems adds the zone's start items to a player that is
// already in the zone, as a new character gets them.
func (z *Zone) giveStartItems(p *Player) {
	for _, s := range z.StartItems {
		z.addItem(p, s.ID, s.Amount)
	}
	p.pending = nil
	z.updateWeight(p)
}

func use(z *Zone, p *Player, slot int) {
	z.handle(command{player: p, use: &slot})
}

func drop(z *Zone, p *Player, slot, amount int) {
	z.handle(command{player: p, drop: &dropCommand{slot, amount}})
}

func useFailures(ev []*wathenav1.Event) []wathenav1.UseItemFailure {
	var out []wathenav1.UseItemFailure
	for _, e := range ev {
		if f := e.GetUseItemFailed(); f != nil {
			out = append(out, f.GetReason())
		}
	}
	return out
}

func TestJoinSendsInventoryAndWeight(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{}).Stream(), nil)
	z.Items = testItems()
	z.StartItems = []StartItem{{501, 3}, {909, 2}, {1201, 1}}
	z.PlayerStatus = battle.Status{MaxHP: 100}
	// Without job tables, the maximum weight is set by hand.
	out := make(chan *wathenav1.ServerMessage, outBuffer)
	p := &Player{Name: "a", Out: out, out: out, fresh: true, maxWeight: maxWeight}
	z.join(p)
	z.stepAt(50)
	ev := events(p)
	var list *wathenav1.InventoryUpdate
	for _, e := range ev {
		if l := e.GetInventory(); l != nil {
			list = l
		}
	}
	if list == nil || list.GetSize() != item.BaseSlots || len(list.GetItems()) != 3 {
		t.Fatalf("inventory %v", list)
	}
	for i, want := range [][3]uint32{{0, 501, 3}, {1, 909, 2}, {2, 1201, 1}} {
		it := list.GetItems()[i]
		if it.GetSlot() != want[0] || it.GetItemId() != want[1] || it.GetAmount() != want[2] {
			t.Errorf("slot %d: %v", i, it)
		}
	}
	if w := stats(ev)[wathenav1.StatId_STAT_ID_WEIGHT]; w != 3*70+2*10+400 {
		t.Errorf("weight %d", w)
	}
}

func TestUseHealingItem(t *testing.T) {
	z, a, b := invZone(t, StartItem{501, 2}, StartItem{505, 1})
	a.entity.hp, a.sp = 50, 5
	use(z, a, 0)
	z.stepAt(z.now + 50)
	ev := events(a)
	// VIT 10 adds 20%: 45 to 65 becomes 54 to 78.
	healed := a.entity.hp - 50
	if healed < 54 || healed > 78 {
		t.Errorf("healed %d", healed)
	}
	var heal *wathenav1.Damage
	var removed *wathenav1.InventoryRemove
	for _, e := range ev {
		if d := e.GetDamage(); d.GetKind() == wathenav1.DamageKind_DAMAGE_KIND_HEAL {
			heal = d
		}
		if r := e.GetInventoryRemove(); r != nil {
			removed = r
		}
	}
	me := a.netIDs.id(a.entity)
	if heal.GetAmount() != healed || heal.GetSource() != me || heal.GetTarget() != me {
		t.Errorf("heal %v", heal)
	}
	if removed.GetSlot() != 0 || removed.GetAmount() != 1 || removed.GetReason() != wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED {
		t.Errorf("remove %v", removed)
	}
	got := stats(ev)
	if got[wathenav1.StatId_STAT_ID_HP] != a.entity.hp || got[wathenav1.StatId_STAT_ID_WEIGHT] != 70+150 {
		t.Errorf("stats %v", got)
	}
	// Both see the use. Only the user sees the heal number.
	for _, viewer := range []struct {
		p  *Player
		ev []*wathenav1.Event
	}{{a, ev}, {b, events(b)}} {
		seen := 0
		for _, e := range viewer.ev {
			if u := e.GetItemUsed(); u != nil && u.GetSource() == viewer.p.netIDs.id(a.entity) && u.GetItemId() == 501 {
				seen++
			}
			if viewer.p == b && e.GetDamage().GetKind() == wathenav1.DamageKind_DAMAGE_KIND_HEAL {
				t.Error("b sees the heal number")
			}
		}
		if seen != 1 {
			t.Errorf("%s sees %d uses", viewer.p.Name, seen)
		}
	}

	// A Blue Potion heals SP: INT 5 adds 10%, so 44 to 66, up to the
	// missing 45. item_use_interval asks for 100 ms between uses.
	z.stepAt(z.now + 50)
	use(z, a, 1)
	z.stepAt(z.now + 50)
	ev = events(a)
	if a.sp < 5+44 || a.sp > 50 {
		t.Errorf("SP %d", a.sp)
	}
	if got := stats(ev)[wathenav1.StatId_STAT_ID_SP]; got != a.sp {
		t.Errorf("SP stat %d", got)
	}

	// At full HP the potion is spent and heals nothing.
	a.entity.hp = 200
	z.stepAt(z.now + 50)
	use(z, a, 0)
	z.stepAt(z.now + 50)
	for _, e := range events(a) {
		if e.GetDamage() != nil {
			t.Errorf("heal at full HP: %v", e)
		}
	}
	if a.inventory.Count(501) != 0 || a.inventory.Weight != 0 {
		t.Errorf("potions %d, weight %d", a.inventory.Count(501), a.inventory.Weight)
	}
}

func TestUseItemRules(t *testing.T) {
	z, a, _ := invZone(t, StartItem{501, 3}, StartItem{909, 1}, StartItem{506, 1}, StartItem{607, 2})
	a.entity.hp = 10
	// Two uses in one tick: item_use_interval refuses the second.
	use(z, a, 0)
	use(z, a, 0)
	// An Etc item cannot be used. A Green Potion's script needs status
	// effects, so it is refused and not spent.
	z.stepAt(z.now + 100)
	use(z, a, 1)
	use(z, a, 2)
	// An empty slot, and one past the inventory. The last one sends
	// nothing, as clif_parse_UseItem.
	use(z, a, 50)
	use(z, a, 500)
	z.stepAt(z.now + 50)
	want := []wathenav1.UseItemFailure{
		wathenav1.UseItemFailure_USE_ITEM_FAILURE_TOO_SOON,
		wathenav1.UseItemFailure_USE_ITEM_FAILURE_NOT_USABLE,
		wathenav1.UseItemFailure_USE_ITEM_FAILURE_NOT_SUPPORTED,
		wathenav1.UseItemFailure_USE_ITEM_FAILURE_EMPTY_SLOT,
	}
	ev := events(a)
	if got := useFailures(ev); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Errorf("failures %v, want %v", got, want)
	}
	if !hasSystemLine(ev, "This item needs a feature the server does not have yet: status effects.") {
		t.Error("no message says what the Green Potion needs")
	}
	if a.inventory.Count(501) != 2 || a.inventory.Count(506) != 1 || a.inventory.Count(909) != 1 {
		t.Errorf("inventory %v", a.inventory.Slots[:4])
	}

	// Yggdrasil Berry heals fully, then waits 5 seconds.
	a.entity.hp = 10
	use(z, a, 3)
	if a.entity.hp != 200 || a.sp != 50 {
		t.Errorf("HP %d, SP %d", a.entity.hp, a.sp)
	}
	z.stepAt(z.now + 200)
	events(a)
	use(z, a, 3)
	z.stepAt(z.now + 50)
	if got := useFailures(events(a)); len(got) != 1 || got[0] != wathenav1.UseItemFailure_USE_ITEM_FAILURE_COOLDOWN {
		t.Errorf("failures %v", got)
	}
	z.stepAt(z.now + 5000)
	events(a)
	use(z, a, 3)
	if a.inventory.Count(607) != 0 {
		t.Error("the second berry was not used after its delay")
	}

	// A dead player uses nothing.
	a.entity.dead = true
	use(z, a, 0)
	if a.inventory.Count(501) != 2 {
		t.Error("a dead player used a potion")
	}
}

// hasSystemLine reports whether events hold a line of server text.
func hasSystemLine(ev []*wathenav1.Event, text string) bool {
	for _, e := range ev {
		if c := e.GetChat(); c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM && c.GetText() == text {
			return true
		}
	}
	return false
}

func TestDropAndPickUpAgain(t *testing.T) {
	z, a, b := invZone(t, StartItem{909, 5}, StartItem{7777, 1})
	ax, ay := a.entity.x, a.entity.y
	drop(z, a, 0, 2)
	z.stepAt(z.now + 50)
	ev := events(a)
	var ground *wathenav1.EntityAppear
	for _, it := range items(ev) {
		ground = it
	}
	if ground == nil {
		t.Fatal("no item on the ground")
	}
	if g := ground.GetItem(); g.GetItemId() != 909 || g.GetAmount() != 2 || !g.GetDropped() {
		t.Errorf("ground item %v", g)
	}
	// map_search_freecell_dist looks 1 or 2 cells away.
	if dx, dy := abs(int(ground.GetX())-ax), abs(int(ground.GetY())-ay); max(dx, dy) > 2 || dx+dy == 0 {
		t.Errorf("dropped at offset (%d, %d)", dx, dy)
	}
	if a.inventory.Count(909) != 3 || stats(ev)[wathenav1.StatId_STAT_ID_WEIGHT] != 3*10+1 {
		t.Errorf("jellopy %d, stats %v", a.inventory.Count(909), stats(ev))
	}

	// Dropped items have no looter, so anyone takes them at once.
	var it *entity
	for e := range z.entities.all() {
		if e.item != nil {
			it = e
		}
	}
	z.handle(command{player: b, pickUp: new(b.netIDs.id(it))})
	for start := z.now; z.entities.has(it) && z.now < start+2000; {
		z.stepAt(z.now + 50)
	}
	if b.inventory.Count(909) != 7 {
		t.Fatalf("b holds %d", b.inventory.Count(909))
	}
	// b already held Jellopy in slot 0, so they stack there.
	var added *wathenav1.InventoryAdd
	for _, e := range events(b) {
		if e.GetInventoryAdd() != nil {
			added = e.GetInventoryAdd()
		}
	}
	if added.GetSlot() != 0 || added.GetAmount() != 2 || b.inventory.Slots[0].Amount != 7 {
		t.Errorf("add %v, slot 0 %v", added, b.inventory.Slots[0])
	}
}

func TestDropFailures(t *testing.T) {
	z, a, _ := invZone(t, StartItem{909, 2}, StartItem{7777, 1})
	drop(z, a, 0, 0)
	drop(z, a, 0, 3)
	drop(z, a, 5, 1)
	drop(z, a, 1, 1)
	drop(z, a, item.MaxInventory, 1)
	z.stepAt(z.now + 50)
	var got []wathenav1.DropItemFailure
	for _, e := range events(a) {
		if f := e.GetDropItemFailed(); f != nil {
			got = append(got, f.GetReason())
		}
	}
	want := []wathenav1.DropItemFailure{
		wathenav1.DropItemFailure_DROP_ITEM_FAILURE_BAD_AMOUNT,
		wathenav1.DropItemFailure_DROP_ITEM_FAILURE_BAD_AMOUNT,
		wathenav1.DropItemFailure_DROP_ITEM_FAILURE_EMPTY_SLOT,
		wathenav1.DropItemFailure_DROP_ITEM_FAILURE_CANNOT_DROP,
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Errorf("failures %v, want %v", got, want)
	}
	for e := range z.entities.all() {
		if e.item != nil {
			t.Error("something fell")
		}
	}
}

// groundAt puts an item next to a player, and lets the player see it.
func groundAt(z *Zone, p *Player, id uint32, amount int) *entity {
	it := &entity{x: p.entity.x, y: p.entity.y + 1, item: &groundItem{id: id, amount: amount, subX: 6, subY: 6, expires: z.now + 60000}}
	z.entities.add(it)
	z.stepAt(z.now + 50)
	events(p)
	return it
}

func TestPickUpOverweightOrFull(t *testing.T) {
	// 13 potions weigh 91.0 of 100.0: 3 more would pass the maximum.
	z, a, _ := invZone(t, StartItem{501, 13})
	it := groundAt(z, a, 505, 1)
	z.handle(command{player: a, pickUp: new(a.netIDs.id(it))})
	z.stepAt(z.now + 50)
	if f := failures(a); len(f) != 1 || f[0] != wathenav1.PickUpFailure_PICK_UP_FAILURE_OVERWEIGHT {
		t.Errorf("failures %v", f)
	}
	if !z.entities.has(it) || a.inventory.Count(505) != 0 {
		t.Error("the item was taken")
	}

	// A full inventory takes no new item, but a held stack still grows.
	z, a, _ = invZone(t)
	for i := range item.BaseSlots {
		a.inventory.Slots[i] = item.Slot{ID: 1201, Amount: 1}
	}
	a.inventory.Slots[0] = item.Slot{ID: 909, Amount: 1}
	it = groundAt(z, a, 505, 1)
	z.handle(command{player: a, pickUp: new(a.netIDs.id(it))})
	z.stepAt(z.now + 50)
	if f := failures(a); len(f) != 1 || f[0] != wathenav1.PickUpFailure_PICK_UP_FAILURE_INVENTORY_FULL {
		t.Errorf("failures %v", f)
	}
	it = groundAt(z, a, 909, 3)
	z.handle(command{player: a, pickUp: new(a.netIDs.id(it))})
	z.stepAt(z.now + 50)
	if z.entities.has(it) || a.inventory.Slots[0].Amount != 4 {
		t.Errorf("stack %v", a.inventory.Slots[0])
	}
}

func TestOverweightRules(t *testing.T) {
	// 10 Red Potions and 6 Jellopy: 76.0 of 100.0, over renewal's 70%.
	z, a, b := invZone(t, StartItem{501, 10}, StartItem{909, 6})
	if a.overweight != item.Weight50 {
		t.Fatalf("level %d", a.overweight)
	}
	// No regen while overweight, however long the player waits.
	a.entity.hp, a.sp = 100, 10
	for range 400 {
		z.stepAt(z.now + 50)
	}
	if a.entity.hp != 100 || a.sp != 10 {
		t.Errorf("HP %d, SP %d while overweight", a.entity.hp, a.sp)
	}
	// Attacks still work at 76%.
	z.handle(command{player: a, attack: &attackCommand{target: a.netIDs.id(b.entity), repeat: true}})
	if a.entity.target != b.entity {
		t.Error("no attack at 76%")
	}
	// 14 more Jellopy make 90.0: the attack stops, and new ones are
	// refused.
	it := groundAt(z, a, 909, 14)
	z.handle(command{player: a, pickUp: new(a.netIDs.id(it))})
	z.stepAt(z.now + 50)
	if a.overweight != item.Weight90 || a.entity.target != nil {
		t.Fatalf("level %d, target %v", a.overweight, a.entity.target)
	}
	z.handle(command{player: a, attack: &attackCommand{target: a.netIDs.id(b.entity), repeat: true}})
	if a.entity.target != nil {
		t.Error("attacked at 90%")
	}
	// Healing items still work: pc_isUseitem only checks weight for cash
	// items and boxes.
	use(z, a, 0)
	if a.entity.hp <= 100 {
		t.Error("no heal at 90%")
	}
	// Dropping 20 Jellopy leaves 63.0, which ends both.
	drop(z, a, 1, 20)
	if a.overweight != item.WeightNormal {
		t.Errorf("level %d after the drop", a.overweight)
	}
}

func TestNaturalRegen(t *testing.T) {
	z, a, _ := invZone(t)
	// VIT 10 and 200 max HP: 2 + 1 HP every 6 seconds. INT 5 and 50 max
	// SP: 1 SP every 8 seconds.
	if a.regen.HP != 3 || a.regen.SP != 1 {
		t.Fatalf("regen %+v", a.regen)
	}
	// The waits start at login, at 0, and start again each time an
	// interval passes, even at full HP and SP. The timer runs every 500
	// ms and waits 250 ms less than each interval. So HP checks at 6000
	// and 12000, and SP at 8000.
	for z.now < 6050 {
		z.stepAt(z.now + 50)
	}
	a.entity.hp, a.sp = 100, 10
	var hpAt, spAt int64
	for z.now < 13000 {
		z.stepAt(z.now + 50)
		if hpAt == 0 && a.entity.hp > 100 {
			hpAt = z.now
		}
		if spAt == 0 && a.sp > 10 {
			spAt = z.now
		}
	}
	if hpAt != 12000 || spAt != 8000 {
		t.Errorf("HP healed at %d ms, SP at %d ms", hpAt, spAt)
	}
	if a.entity.hp != 103 || a.sp != 11 {
		t.Errorf("HP %d, SP %d", a.entity.hp, a.sp)
	}
	ev := events(a)
	if s := stats(ev); s[wathenav1.StatId_STAT_ID_HP] != 103 || s[wathenav1.StatId_STAT_ID_SP] != 11 {
		t.Errorf("stats %v", s)
	}
	for _, e := range ev {
		if e.GetDamage() != nil {
			t.Error("regen shows a heal number")
		}
	}

	// A walking player heals SP but not HP.
	hp, sp := a.entity.hp, a.sp
	for range 200 {
		if len(a.entity.path) == 0 {
			x := 5
			if a.entity.x < 20 {
				x = 35
			}
			z.handle(command{player: a, walkTo: &[2]int{x, a.entity.y}})
		}
		z.stepAt(z.now + 50)
	}
	if a.entity.hp != hp || a.sp <= sp {
		t.Errorf("walking: HP %d to %d, SP %d to %d", hp, a.entity.hp, sp, a.sp)
	}
}
