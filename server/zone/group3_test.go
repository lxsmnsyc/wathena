package zone

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/status"
)

// The calls of review decision 58, areas B7, B9 and B10 (decision 0064).

// clickScript is an NPC script whose click runs body with player,
// conversation and npc, and the modules it names.
func clickScript(body string) string {
	return `
local events = require "wa.events"
local context = require "wa.context"
local items = require "wa.items"
local maps = require "wa.maps"
local clock = require "wa.clock"
local rules = require "wa.rules"
return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
` + body + `
  end)
end`
}

// messages are the system lines of events.
func messages(ev []*wathenav1.Event) []string {
	var out []string
	for _, e := range ev {
		if c := e.GetChat(); c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM {
			out = append(out, c.GetText())
		}
	}
	return out
}

// TestGiveExpRules checks player:give_exp: getexp gives the exp with
// quest_exp_rate (script.cpp 11168), getexp2 takes exp with a negative
// amount, down to 0 (pc_lostexp), and 0 and 0, or a negative getexp,
// raise an error.
func TestGiveExpRules(t *testing.T) {
	z, p, n, logs := npcZone(t, clickScript(`
    player:give_exp(3, 2)
    player:send_message(player.base_level .. " " .. player.job_level)
    player:give_exp(-100, -100, { raw = true })
    local ok, err = pcall(player.give_exp, player, 0, 0)
    player:send_message(tostring(ok) .. " " .. tostring(err):match("both 0") .. "")
    ok = pcall(player.give_exp, player, -1, 0)
    player:send_message(tostring(ok))
`))
	z.Progress.QuestExpRate = 200
	before := p.progress
	got := messages(click(z, p, n))
	// The rate doubles 3 and 2 to 6 and 4: one base level (10) is not
	// reached, and job exp 4 is below the first level's 5.
	if !slices.Equal(got, []string{"1 1", "false both 0", "false"}) {
		t.Fatalf("got %q; logs %s", got, logs)
	}
	if p.progress.BaseExp != 0 || p.progress.JobExp != 0 || before.BaseLevel != p.progress.BaseLevel {
		t.Errorf("exp after getexp2 -100: %d %d", p.progress.BaseExp, p.progress.JobExp)
	}
}

// TestHealAndSetHP checks player:heal and player:set_hp: heal caps at the
// maximum and kills with enough negative HP (status_heal's
// status_damage), set_hp caps to 1 and the maximum, and a dead player is
// not healed.
func TestHealAndSetHP(t *testing.T) {
	z, p, n, _ := npcZone(t, clickScript(`
    player:set_hp(0)
    player:send_message(tostring(player.hp))
    player:set_hp(50)
    player:heal(1000, 0)
    player:send_message(tostring(player.hp))
    player:heal(-100, 0)
    player:send_message(tostring(player.hp))
    player:heal(-1000, 0)
`))
	got := messages(click(z, p, n))
	if !slices.Equal(got, []string{"1", "200", "100"}) {
		t.Fatalf("got %q", got)
	}
	if !p.entity.dead {
		t.Fatalf("heal -1000 did not kill: hp %d", p.entity.hp)
	}
	if err := (scriptHost{z}).HealFixed(int64(p.charID), 50, 0); err != nil || p.entity.hp != 0 {
		t.Errorf("a dead player healed: %d %v", p.entity.hp, err)
	}
}

// TestBlockedActions checks player:set_blocked: the PCBLOCK_ flags stop
// a walk, item use and NPC clicks where rAthena checks them.
func TestBlockedActions(t *testing.T) {
	z, p, n, _ := npcZone(t, clickScript(`
    player:set_blocked("move", true)
    player:set_blocked("use_item", true)
    player:send_message(tostring(player:is_blocked("move")) .. tostring(player:is_blocked("npc")))
`))
	if got := messages(click(z, p, n)); !slices.Equal(got, []string{"truefalse"}) {
		t.Fatalf("got %q", got)
	}
	z.handle(command{player: p, walkTo: &[2]int{25, 20}})
	z.stepAt(z.now + 1000)
	if p.entity.x != 21 {
		t.Errorf("a blocked player walked to %d", p.entity.x)
	}
	p.maxWeight = 1 << 30
	z.giveItem(p, 501, 1, true)
	potion := slices.IndexFunc(p.inventory.Slots, func(s item.Slot) bool { return s.ID == 501 })
	z.handle(command{player: p, use: &potion})
	if p.inventory.Count(501) != 1 {
		t.Error("a blocked player used an item")
	}
	if err := (scriptHost{z}).SetBlocked(int64(p.charID), blockNPCClick, true); err != nil {
		t.Fatal(err)
	}
	if got := click(z, p, n); len(dialogs(got)) != 0 || len(messages(got)) != 0 {
		t.Errorf("a blocked player clicked the NPC: %v", got)
	}
}

// TestItemsDuringTalk checks conversation:lock_items and unlock_items:
// a renewal conversation starts locked, as item_enabled_npc, so an item
// is refused while the player talks; unlock_items lets it through.
func TestItemsDuringTalk(t *testing.T) {
	z, p, n, _ := npcZone(t, clickScript(`
    conversation:say("wait")
    conversation:wait_for_next()
    conversation:unlock_items()
    conversation:say("now")
    conversation:wait_for_next()
`))
	p.maxWeight = 1 << 30
	z.giveItem(p, 909, 1, true)
	slot := slices.IndexFunc(p.inventory.Slots, func(s item.Slot) bool { return s.ID == 909 })
	d := dialogs(click(z, p, n))
	if len(d) == 0 {
		t.Fatal("no dialog")
	}
	// used tells a use refused for the conversation (busy) from one the
	// item refused (UseItemFailed: a Jellopy is not used).
	used := func() string {
		z.handle(command{player: p, use: &slot})
		z.stepAt(z.now + 50)
		for _, e := range events(p) {
			if e.HasUseItemFailed() {
				return "item"
			}
		}
		return "busy"
	}
	if got := used(); got != "busy" {
		t.Fatalf("a locked conversation let the item through: %s", got)
	}
	answer(z, p, reply(d[len(d)-1].GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Next = new(true) }))
	if got := used(); got != "item" {
		t.Errorf("an unlocked conversation refused the item: %s", got)
	}
}

// TestRentalsExpire checks rentals: player:give_item's rent_seconds
// gives a copy that does not stack, cannot be dropped, and goes when its
// time ends by the server clock, as pc_inventory_rentals; count_item
// leaves it out, as countitem, and rentals = true counts it.
func TestRentalsExpire(t *testing.T) {
	z, p, n, logs := npcZone(t, clickScript(`
    player:give_item(items.id("Jellopy"), 1)
    player:give_item(items.id("Jellopy"), 2, { rent_seconds = 60 })
    player:send_message(player:count_item(items.id("Jellopy")) .. " " .. player:count_item(items.id("Jellopy"), { rentals = true }))
`))
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	z.Clock = func() time.Time { return now }
	p.maxWeight = 1 << 30
	if got := messages(click(z, p, n)); !slices.Equal(got, []string{"1 2"}) {
		t.Fatalf("got %q; logs %s", got, logs)
	}
	var rentals []int
	for i, s := range p.inventory.Slots {
		if s.ID == 909 && s.Expires != 0 {
			rentals = append(rentals, i)
			if s.Expires != now.Unix()+60 || s.Amount != 1 {
				t.Errorf("rental %+v", s)
			}
		}
	}
	if len(rentals) != 2 {
		t.Fatalf("rentals in slots %v", rentals)
	}
	z.dropItem(p, rentals[0], 1)
	if p.inventory.Slots[rentals[0]].ID != 909 {
		t.Error("a rental was dropped")
	}
	events(p)
	now = now.Add(61 * time.Second)
	z.stepAt(z.now + rentalCheckEvery)
	expired := 0
	for _, e := range events(p) {
		if r := e.GetInventoryRemove(); r != nil && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_EXPIRED {
			expired++
		}
	}
	if expired != 2 || p.inventory.Count(909) != 1 {
		t.Errorf("%d expired, %d left", expired, p.inventory.Count(909))
	}
}

// TestGiveItemCopies checks give_item's getitem2 options on equipment,
// and that an item that is not equipment stays plain, as script.cpp 7853.
func TestGiveItemCopies(t *testing.T) {
	z, p, n, logs := npcZone(t, clickScript(`
    player:give_item(items.id("Knife"), 1, { refine = 7, cards = { 4001 }, broken = true, identified = false, signed_by = player })
    player:give_item(items.id("Jellopy"), 1, { refine = 7, broken = true })
    local inv = player:get_inventory()
    for _, e in ipairs(inv) do
      player:send_message(e.item .. " " .. e.refine .. " " .. tostring(e.broken) .. " " .. tostring(e.identified) .. " " .. #e.cards)
    end
    player:send_message(table.concat(player:get_broken_items(), ","))
    player:send_message(tostring(player:repair_all()))
`))
	p.maxWeight = 1 << 30
	z.Items = item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: 909, AegisName: "Jellopy", Type: "Etc", Weight: 10, Stackable: true},
		{ID: 1201, AegisName: "Knife", Type: "Weapon", Weight: 400, Refineable: true},
		{ID: 4001, AegisName: "Poring_Card", Type: "Card", Weight: 10},
	}})
	got := messages(click(z, p, n))
	want := []string{"1201 7 true false 1", "909 0 false true 0", "1201", "1"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q; logs %s", got, want, logs)
	}
	for _, s := range p.inventory.Slots {
		if s.ID == 1201 && s.SignedName != "a" {
			t.Errorf("signed by %q", s.SignedName)
		}
	}
}

// TestClockCalls checks wa.clock against the server clock in the
// server's time zone.
func TestClockCalls(t *testing.T) {
	z, p, n, logs := npcZone(t, clickScript(`
    player:send_message(clock.now() .. " " .. clock.get("day_of_week") .. " " .. clock.get("date") .. " " .. clock.get("day_of_year"))
    player:send_message(clock.format("%Y-%m-%d %H:%M:%S %a %b %j"))
    player:send_message(rules.get("quest_exp_rate") .. " " .. rules.get("max_hair_style"))
`))
	at := time.Date(2026, 10, 21, 9, 5, 7, 0, time.UTC)
	z.Clock = func() time.Time { return at }
	got := messages(click(z, p, n))
	want := []string{"1792573507 3 20261021 293", "2026-10-21 09:05:07 Wed Oct 294", "100 42"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q; logs %s", got, want, logs)
	}
}

// TestQuestInfoNewNPC checks rAthena's rule for questinfo at run time
// (pc.cpp 15396): a player already on the map does not see the icons of
// an NPC new to the map's list until it enters the map again.
func TestQuestInfoNewNPC(t *testing.T) {
	z, p, n, _ := npcZone(t, clickScript(`
    npc:add_quest_icon("quest")
`))
	click(z, p, n)
	z.stepAt(z.now + 50)
	for _, e := range events(p) {
		if e.HasQuestIcon() {
			t.Fatalf("a player on the map saw a new NPC's icon: %v", e)
		}
	}
	b := joinAt(z, "b", 21, 21)
	shownIcon := false
	for _, e := range events(b) {
		shownIcon = shownIcon || e.HasQuestIcon() && e.GetQuestIcon().GetIcon() == "quest"
	}
	if !shownIcon {
		t.Error("a player who entered after the icon did not see it")
	}
}

// TestFullNameHiddenPart checks npc.full_name of a record whose
// display_name keeps the # part of a "::" name.
func TestFullNameHiddenPart(t *testing.T) {
	z, _ := presentZone(t, "test", clickScript(`player:send_message(npc.full_name .. "|" .. npc.hidden_name .. "|" .. npc.unique_name .. "|" .. npc.name)`),
		NPC{Name: "example.test:G1", DisplayName: "Guard", Hidden: "east", X: 20, Y: 20, Sprite: 112})
	a := joinAt(z, "a", 21, 20)
	events(a)
	clickNPC(t, z, a, npcNamed(z, "G1"))
	z.stepAt(z.now + 50)
	if got := messages(events(a)); !slices.Equal(got, []string{"Guard#east|east|G1|Guard"}) {
		t.Errorf("got %q", got)
	}
	if !strings.Contains(npcNamed(z, "G1").npc.DisplayName, "Guard") {
		t.Error("display name")
	}
}

// TestRefineCalls checks the refine reads of refine.yml, as
// getequippercentrefinery and getequiprefinecost read them, and
// equipped:add_refine and destroy, as successrefitem, downrefitem and
// delequip.
func TestRefineCalls(t *testing.T) {
	z, a, _ := equipZone(t)
	var rd status.RefineData
	if err := reference.LoadFile("extracts/renewal/refine_db.json", &rd); err != nil {
		t.Fatal(err)
	}
	if err := z.Status.SetRefine(&rd); err != nil {
		t.Fatal(err)
	}
	h := scriptHost{z}
	sword := modscript.ItemRef{Item: 1101}
	for _, c := range []struct{ refine, chance int }{{0, 100}, {6, 100}, {7, 60}, {8, 40}, {20, 0}} {
		if got, err := h.RefineChance(sword, c.refine, false); err != nil || got != c.chance {
			t.Errorf("refine_chance at +%d: %d %v, want %d", c.refine, got, err, c.chance)
		}
	}
	if _, err := h.RefineChance(sword, 0, true); err == nil {
		t.Error("the enriched chance did not raise an error")
	}
	if zeny, _, ok, err := h.RefineCost(sword, 7, "normal"); err != nil || !ok || zeny != 50 {
		t.Errorf("refine_cost: %d %v %v", zeny, ok, err)
	}
	if ok, err := h.CanRefine(sword); err != nil || !ok {
		t.Errorf("can_refine: %v %v", ok, err)
	}
	slot := give(t, z, a, 1101, 1)
	equip(z, a, slot)
	handle, ok, err := h.Equipped(int64(a.charID), "right_hand")
	if err != nil || !ok {
		t.Fatalf("not worn: %v", err)
	}
	if got, err := h.EquippedRefine(handle, 3); err != nil || got != 3 {
		t.Errorf("+3: %d %v", got, err)
	}
	handle, _, _ = h.Equipped(int64(a.charID), "right_hand")
	if got, err := h.RefineChance(modscript.ItemRef{Equipped: handle}, -1, false); err != nil || got != 100 {
		t.Errorf("chance of the worn copy at +3: %d %v", got, err)
	}
	if got, err := h.EquippedRefine(handle, -5); err != nil || got != 0 {
		t.Errorf("-5 from +3: %d %v", got, err)
	}
	a.inventory.Slots[slot].Refine = z.maxRefine()
	handle, _, _ = h.Equipped(int64(a.charID), "right_hand")
	if got, _ := h.EquippedRefine(handle, 1); got != z.maxRefine() {
		t.Errorf("successrefitem at MAX_REFINE: %d", got)
	}
	handle, _, _ = h.Equipped(int64(a.charID), "right_hand")
	if err := h.EquippedDestroy(handle); err != nil || a.inventory.Slots[slot].ID != 0 {
		t.Errorf("destroy: %v %+v", err, a.inventory.Slots[slot])
	}
}

// TestMovingTouchArea checks a moving NPC's touch area, as rAthena moves
// its CELL_NPC cells with it (map_moveblock, map.cpp 555 and 632): a
// walking NPC touches the players its new area holds on each step
// (unit_walktoxy_ontouch, unit.cpp 700), and movenpc fires no touch
// until the player steps (npc_movenpc, npc.cpp 5046).
func TestMovingTouchArea(t *testing.T) {
	z, w := touchZone(t, toucher)
	a := joinAt(z, "a", 26, 20)
	heard(w)
	gate := npcNamed(z, "Gate")
	h := scriptHost{z}
	if err := h.MoveNPC(gate.npc.handle, 27, 20); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if got := heard(w); len(got) != 0 {
		t.Errorf("movenpc onto a player touched it: %q", got)
	}
	walkTo(t, z, a, 26, 21)
	if got := heard(w); len(got) != 1 || got[0] != "touch a" {
		t.Errorf("a step inside the moved area: %q", got)
	}
	walkTo(t, z, a, 20, 25)
	heard(w)
	if ok, err := h.WalkNPC(gate.npc.handle, 20, 24); err != nil || !ok {
		t.Fatalf("walk: %v %v", ok, err)
	}
	for end, now := z.now+5000, z.now+50; now <= end; now += 50 {
		z.stepAt(now)
	}
	if got := heard(w); len(got) == 0 || got[0] != "touch a" {
		t.Errorf("a walking NPC's area reached the player: %q", got)
	}
}

// TestTransformLookAndPush checks player:transform, which shows the
// player as the monster until SC_MONSTER_TRANSFORM ends, player:set_look
// with the look limits, and player:push.
func TestTransformLookAndPush(t *testing.T) {
	z, p, n, logs := npcZone(t, clickScript(`
    player:send_message(tostring(player:transform(1002, 1000)))
    player:set_look { hair_style = 99, cloth_color = 3, head_top = 17 }
    player:send_message(player.look.hair_style .. " " .. player.look.cloth_color .. " " .. player.look.head_top)
    player:push(2, 3)
`))
	z.Statuses = testStatuses(t)
	z.Mods.Monsters = map[uint32]Spawn{1002: {Mob: 1002, Name: "Poring"}}
	x := p.entity.x
	if got := messages(click(z, p, n)); !slices.Equal(got, []string{"true", "42 3 17"}) {
		t.Fatalf("got %q; logs %s", got, logs)
	}
	if m := p.Look.message(); m.GetBody() != 1002 || m.GetNpcView() != wathenav1.NpcView_NPC_VIEW_MONSTER {
		t.Errorf("transformed look %v", m)
	}
	// DIR_WEST is 2: three cells to the west.
	if p.entity.x != x-3 {
		t.Errorf("pushed from %d to %d", x, p.entity.x)
	}
	for end, now := z.now+1500, z.now+50; now <= end; now += 50 {
		z.stepAt(now)
	}
	if p.transform != 0 || p.Look.message().GetBody() == 1002 {
		t.Errorf("the transform did not end: %d", p.transform)
	}
	if p.record.Hair != 42 || p.record.ClothColor != 3 {
		t.Errorf("saved look %d %d", p.record.Hair, p.record.ClothColor)
	}
}

// TestPlayerReadsAndItemCalls checks the reads of area B7 and the item
// calls of area B9 that the other tests leave out: weight, skill points,
// the look, the staff level, options, the save point, item info,
// count_item's countitem2 form, broken items, unequip_all, drops,
// merges, books and items run without being used.
func TestPlayerReadsAndItemCalls(t *testing.T) {
	z, p, n, logs := npcZone(t, clickScript(`
    player:send_message(player.weight .. " " .. player.skill_points .. " " .. player.staff_level .. " " .. player.look.hair_style)
    player:send_message(tostring(player:has_option("cart")) .. " " .. tostring(player:has_option("riding")))
    local m, x, y = player:get_save_point()
    player:send_message(m .. " " .. x .. " " .. y)
    local info = items.get(items.id("Knife"))
    player:send_message(info.type .. " " .. info.weight .. " " .. tostring(items.get(424242)))
    player:give_item(items.id("Knife"), 1, { refine = 3 })
    player:send_message(player:count_item(items.id("Knife")) .. " " .. player:count_item(items.id("Knife"), { refine = 3 }) .. " " .. player:count_item(items.id("Knife"), { refine = 2 }))
    player:give_item(items.id("Knife"), 1, { broken = true })
    player:send_message(tostring(player:repair_item(1)) .. " " .. tostring(player:repair_item(1)))
    maps.drop_item("test", 10, 10, items.id("Jellopy"), 3)
    player:open_book(items.id("Jellopy"), 2)
    player:unequip_all()
    player:send_message(tostring(player:merge_items()))
`))
	p.maxWeight = 1 << 30
	p.record.SaveMap, p.record.SaveX, p.record.SaveY = "map/prontera", 150, 180
	got := messages(click(z, p, n))
	want := []string{"0 0 0 0", "false false", "prontera 150 180", "weapon 400 nil", "1 1 0", "true false", "0"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q; logs %s", got, want, logs)
	}
	dropped := 0
	for e := range z.entities.all() {
		if e.item != nil && e.item.id == 909 && e.item.amount == 3 {
			dropped++
		}
	}
	if dropped != 1 {
		t.Errorf("%d piles dropped", dropped)
	}
}
