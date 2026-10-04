package zone

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// The benchmarks of the engine ceilings (decision 0023): each limit at
// rAthena's value and at the starting value of the ceiling. They report
// message sizes, the cost of the work a limit scales, and the zone tick.
//
//	go test ./server/zone -run '^$' -bench Ceiling -benchtime 2s

// ceilingRules are rules with every limit's max at the values given.
func ceilingRules(b testing.TB, set map[string]int) *rules.Rules {
	b.Helper()
	r := rules.Base(true)
	m := map[string]rules.RangeSet{}
	for k, v := range set {
		m[k] = rules.RangeSet{Base: v, Max: v, HasBase: true, HasMax: true}
	}
	var err error
	if r.Limits, err = rules.LimitsOf(m); err != nil {
		b.Fatal(err)
	}
	return r
}

// benchItems has many equipment items with 16 card slots, so each slot
// is its own stack, and a card.
func benchItems(n int) *item.Table {
	defs := []item.Def{{ID: 4001, AegisName: "Card", Type: "Card", Weight: 1, Stackable: true},
		{ID: 909, AegisName: "Jellopy", Type: "Etc", Weight: 1, Stackable: true}}
	for i := range n {
		defs = append(defs, item.Def{ID: uint32(10000 + i), AegisName: fmt.Sprintf("Gear%d", i), Type: "Armor", Weight: 1, Slots: 16}) // #nosec G115 -- small.
	}
	return item.New(&item.Data{Mode: "renewal", Items: defs})
}

// gear is a slot of item i with cards cards, refine 10 and an option,
// as a large stack in a list.
func gear(i, cards int) item.Slot {
	s := item.Slot{ID: uint32(10000 + i), Amount: 1, Refine: 10, Serial: item.NewSerial()} // #nosec G115 -- small.
	for range cards {
		s.Cards = append(s.Cards, 4001)
	}
	s.Options = []item.Option{{Name: "var_maxhp", Value: 500}}
	return s
}

// BenchmarkCeilingLists builds the paged list of a full inventory,
// storage and cart, and reports the pages, the largest frame and the
// total bytes.
func BenchmarkCeilingLists(b *testing.B) {
	for _, c := range []struct {
		name  string
		slots int
		cards int
	}{
		{"inventory_200", 200, 4}, {"inventory_1000", 1000, 4}, {"storage_600", 600, 4}, {"storage_10000", 10000, 4},
		{"cart_100", 100, 4}, {"cart_1000", 1000, 4}, {"storage_10000_cards_16", 10000, 16},
	} {
		b.Run(c.name, func(b *testing.B) {
			z := saveZone()
			z.Items = benchItems(c.slots)
			box := &transfer.ListBox{Size: c.slots, Items: z.Items}
			for i := range c.slots {
				s := gear(i, c.cards)
				box.Slots = append(box.Slots, s)
			}
			var pages [][]*wathenav1.InventoryItem
			for b.Loop() {
				pages = pageItemsOf(z.storageItems(box))
			}
			largest, total := 0, 0
			for i, p := range pages {
				var e *wathenav1.Event
				if i == 0 {
					e = wathenav1.Event_builder{StorageOpen: wathenav1.StorageOpen_builder{Items: p, Pages: uint32(len(pages) - 1)}.Build()}.Build() // #nosec G115 -- small.
				} else {
					e = wathenav1.Event_builder{StorageItems: wathenav1.StorageItems_builder{Items: p, Page: uint32(i)}.Build()}.Build() // #nosec G115 -- small.
				}
				n := proto.Size(wathenav1.ServerMessage_builder{Frame: wathenav1.Frame_builder{Tick: 1 << 30, Events: []*wathenav1.Event{e}}.Build()}.Build())
				largest, total = max(largest, n), total+n
			}
			b.ReportMetric(float64(len(pages)), "pages")
			b.ReportMetric(float64(largest), "max-frame-bytes")
			b.ReportMetric(float64(total), "total-bytes")
		})
	}
}

// BenchmarkCeilingItemMoves moves one item into a nearly full inventory
// or storage and back, as a pick-up or a storage move does. The transfer
// checks on a copy of the box, so its cost grows with the slots.
func BenchmarkCeilingItemMoves(b *testing.B) {
	for _, c := range []struct {
		name    string
		slots   int
		storage bool
	}{{"inventory_200", 200, false}, {"inventory_1000", 1000, false}, {"storage_600", 600, true}, {"storage_10000", 10000, true},
		{"cart_100", 100, true}, {"cart_1000", 1000, true}} {
		b.Run(c.name, func(b *testing.B) {
			tb := benchItems(c.slots)
			// A small source, so the cost is the target's.
			inv := item.NewInventoryOf(10, 10)
			inv.Slots[9] = item.Slot{ID: 909, Amount: 1, Serial: item.NewSerial()}
			src := &transfer.InventoryBox{Inv: inv, Items: tb, MaxWeight: 1 << 30, Who: transfer.Inventory(1)}
			var dst transfer.Box
			if c.storage {
				lb := &transfer.ListBox{Size: c.slots, Items: tb, Who: transfer.Owner{Kind: transfer.KindStorage, ID: 1}}
				for i := range c.slots - 1 {
					lb.Slots = append(lb.Slots, gear(i, 4))
				}
				dst = lb
			} else {
				full := item.NewInventoryOf(c.slots, c.slots)
				for i := range c.slots - 1 {
					full.Slots[i] = gear(i, 4)
				}
				dst = &transfer.InventoryBox{Inv: full, Items: tb, MaxWeight: 1 << 30, Who: transfer.Inventory(2)}
			}
			for b.Loop() {
				from := 0
				for i := range inv.Slots {
					if inv.Slots[i].ID == 909 {
						from = i
					}
				}
				s := src.Slot(from)
				if _, err := transfer.New(transfer.ActionStorage).Move(src, from, s.Serial, 1, dst).Apply(nil); err != nil {
					b.Fatal(err)
				}
				var at int
				for i := c.slots - 1; i >= 0; i-- {
					if d := dst.Slot(i); d != nil && d.ID == 909 {
						at = i
						break
					}
				}
				d := dst.Slot(at)
				if _, err := transfer.New(transfer.ActionStorage).Move(dst, at, d.Serial, 1, src).Apply(nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCeilingSnapshot copies a character with a full inventory, as
// each autosave and commit does.
func BenchmarkCeilingSnapshot(b *testing.B) {
	for _, n := range []int{200, 1000} {
		b.Run(fmt.Sprintf("inventory_%d", n), func(b *testing.B) {
			z := saveZone()
			z.Items = benchItems(n)
			z.Mods = &Mods{Rules: ceilingRules(b, map[string]int{"inventory_slots": n})}
			c := savedCharacter()
			c.Items = nil
			for i := range n {
				s := gear(i, 4)
				c.Items = append(c.Items, character.Item{Slot: i, ID: s.ID, Amount: 1, Serial: s.Serial, Refine: s.Refine, Cards: s.Cards, Extra: s.Extra})
			}
			p := addCharacter(z, c, &saves{})
			for b.Loop() {
				_ = z.snapshot(p)
			}
		})
	}
}

// BenchmarkCeilingPartyTick runs zone ticks with every member of one
// party on the map, walking, so each tick sends members' cells and HP.
// It reports the bytes of the party's info message.
func BenchmarkCeilingPartyTick(b *testing.B) {
	for _, n := range []int{12, 256, -256} {
		name := fmt.Sprintf("members_%d", n)
		if n < 0 {
			// The same players without a party, for the tick's base cost.
			n, name = -n, fmt.Sprintf("no_party_%d", -n)
		}
		b.Run(name, func(b *testing.B) {
			w := newSocialWorld(nil, "map/a")
			w.social = social.New(ceilingRules(b, map[string]int{"party_members": n}).Limits, rules.DefaultParty())
			z := w.zones[0]
			z.Social = w.social
			var players []*Player
			for i := range n {
				players = append(players, w.join(z, fmt.Sprintf("P%03d", i)))
			}
			info := 0
			if name != fmt.Sprintf("no_party_%d", n) {
				formParty(b, z, social.Options{}, players...)
				info = proto.Size(partyInfo(z.socialHub().View(z.socialHub().PartyOf(players[0].soc))))
			}
			at := int64(50)
			for b.Loop() {
				for i, p := range players {
					if i%4 == int(at/50)%4 {
						p.entity.x, p.entity.y = 5+(int(at/50)+i)%50, 5+i%50
						p.entity.hp--
						p.entity.hpChanged = true
					}
				}
				w.step(at)
				at += 50
				for _, p := range players {
					for len(p.Out) > 0 {
						<-p.Out
					}
				}
			}
			b.ReportMetric(float64(info), "party-info-bytes")
		})
	}
}

// BenchmarkCeilingGuildTick is BenchmarkCeilingPartyTick for one guild.
func BenchmarkCeilingGuildTick(b *testing.B) {
	for _, n := range []int{76, 512, -512} {
		name := fmt.Sprintf("members_%d", n)
		if n < 0 {
			n, name = -n, fmt.Sprintf("no_guild_%d", -n)
		}
		b.Run(name, func(b *testing.B) {
			w := newSocialWorld(nil, "map/a")
			w.social = social.New(ceilingRules(b, map[string]int{"guild_members": n}).Limits, rules.DefaultParty())
			w.social.SetGuildData(social.CoreGuildTables(true), rules.DefaultGuild())
			z := w.zones[0]
			z.Social = w.social
			var players []*Player
			for i := range n {
				players = append(players, w.join(z, fmt.Sprintf("G%03d", i)))
			}
			info := 0
			if name != fmt.Sprintf("no_guild_%d", n) {
				formGuild(b, z, players...)
				info = proto.Size(guildInfo(z.socialHub().GuildViewOf(players[0].soc), players[0].Name))
			}
			at := int64(50)
			for b.Loop() {
				for i, p := range players {
					if i%4 == int(at/50)%4 {
						p.entity.x, p.entity.y = 5+(int(at/50)+i)%50, 5+i%50
					}
				}
				w.step(at)
				at += 50
				for _, p := range players {
					for len(p.Out) > 0 {
						<-p.Out
					}
				}
			}
			b.ReportMetric(float64(info), "guild-info-bytes")
		})
	}
}

// TestCeilingMessageSizes logs the size of the messages that carry a
// whole list: the friend list, the block list and an item with 16 cards.
// Each must fit the 64 KiB message limit at the ceiling.
func TestCeilingMessageSizes(t *testing.T) {
	var friends []*wathenav1.Friend
	var names []string
	for i := range 1000 {
		name := fmt.Sprintf("%-24s", fmt.Sprintf("Friend%04d", i))
		friends = append(friends, wathenav1.Friend_builder{Name: name, Online: true}.Build())
		names = append(names, name)
	}
	z := saveZone()
	z.Items = benchItems(1)
	s := gear(0, 16)
	for _, c := range []struct {
		name string
		e    *wathenav1.Event
	}{
		{"friend list of 1000", wathenav1.Event_builder{FriendList: wathenav1.FriendList_builder{Friends: friends}.Build()}.Build()},
		{"block list of 1000", blockList(names, true)},
		{"an item with 16 cards", wathenav1.Event_builder{StorageAdd: wathenav1.StorageAdd_builder{Item: z.itemMessage(0, &s, 1)}.Build()}.Build()},
	} {
		n := proto.Size(c.e)
		t.Logf("%s: %d bytes", c.name, n)
		if n > 60<<10 {
			t.Errorf("%s: %d bytes, over the message limit with room for a frame", c.name, n)
		}
	}
}
