package zone

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// The full-recalculation check of strategy section 5.3. A player's
// stats come from the equip contexts that its worn items started when
// they were equipped (docs/decisions/0009-stats-as-totals-of-modifiers.md).
// After each change, the check saves the player and loads the saved
// character as a new player, which starts every context again at once,
// as a login does. Both must have the same stats. A mismatch names the
// item and the change.
//
// It runs over every equipment item and card of core in both modes, on
// a zone with core's converted items and scripts:
//   - an item is equipped with refine +4 (when it can be refined) at the
//     lowest level that wears it, the level then rises by up to 20, as a
//     level up computes stats again, and the item comes off;
//   - a card goes into a slotted item that takes it, with the insert
//     command, then the item is equipped, the level rises and the item
//     comes off.
//
// The race detector makes the Lua contexts slow, so the check runs
// without it only.

// recalcFixture is a zone with one mode's core items and scripts.
type recalcFixture struct {
	z       *Zone
	defs    []*item.Def
	weapons []*item.Def
	jobs    []int
	// computes caches whether the job tables compute a job at a level.
	computes map[[2]int]bool
	// held caches the first weapon of each type that a job, level and
	// sex can hold.
	held map[[3]int][]*item.Def
	// scratch is the inventory of canWear.
	scratch *item.Inventory
	prog    *progression.Tables
	tb      *status.Tables
}

func loadRecalcFixture(t *testing.T, mode reference.Mode) *recalcFixture {
	t.Helper()
	core, err := testCores[mode]()
	if err != nil {
		t.Fatal(err)
	}
	var d item.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/items.json", &d); err != nil {
		t.Fatal(err)
	}
	var e item.EquipData
	if err := reference.LoadFile("extracts/"+string(mode)+"/equipment.json", &e); err != nil {
		t.Fatal(err)
	}
	d.MergeEquipment(&e)
	base, _ := mod.ModeBase(string(mode))
	set, err := mod.Resolve(nil, mod.Config{Core: core.Files, Base: base})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]uint32, len(d.Items))
	for _, x := range d.Items {
		byName[x.AegisName] = x.ID
	}
	merged, err := set.Items(context.Background(), &d, ids, core.Names(byName))
	if err != nil {
		t.Fatal(err)
	}
	scripts, err := set.AllScripts()
	if err != nil {
		t.Fatal(err)
	}
	var jd status.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &jd); err != nil {
		t.Fatal(err)
	}
	tb, err := status.New(&jd)
	if err != nil {
		t.Fatal(err)
	}
	var rd status.RefineData
	if err := reference.LoadFile("extracts/"+string(mode)+"/refine_db.json", &rd); err != nil {
		t.Fatal(err)
	}
	if err := tb.SetRefine(&rd); err != nil {
		t.Fatal(err)
	}
	var pd progression.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/progression.json", &pd); err != nil {
		t.Fatal(err)
	}
	types := map[uint32]string{}
	for _, x := range merged.Data.Items {
		types[x.ID] = x.Type
	}
	prog := progression.New(&pd, types, progression.DefaultConfig)
	z := New("map/test", open(40), rng.FromKey([32]byte{11}).Stream(), nil)
	z.Items = item.New(merged.Data)
	z.Status = tb
	z.Progress = prog
	z.Start = status.Character{Male: true, Stats: status.Stats{Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1}}
	z.Mods = &Mods{Limits: testLimits, Scripts: scripts, IDs: []string{mod.Core}, Log: slog.New(slog.DiscardHandler), Constants: core.Constants, ItemIDs: merged.IDs}
	z.startScripts()
	t.Cleanup(z.stopScripts)
	f := &recalcFixture{z: z, prog: prog, tb: tb}
	for _, x := range merged.Data.Items {
		f.defs = append(f.defs, z.Items.Get(x.ID))
	}
	slices.SortFunc(f.defs, func(a, b *item.Def) int { return int(a.ID) - int(b.ID) })
	for _, x := range f.defs {
		if x.Type == "Weapon" {
			f.weapons = append(f.weapons, x)
		}
	}
	f.computes = map[[2]int]bool{}
	f.held = map[[3]int][]*item.Def{}
	f.scratch = item.NewInventory()
	for _, j := range jd.Jobs {
		if _, ok := tb.MapID(j.ID); ok && prog.MaxBaseLevel(j.ID) > 0 {
			f.jobs = append(f.jobs, j.ID)
		}
	}
	slices.Sort(f.jobs)
	return f
}

// wearing is who wears an item in the check: a job and sex, the lowest
// base level that wears it, and for ammunition the weapon it needs.
type wearing struct {
	job    int
	male   bool
	level  int
	weapon *item.Def
}

// canWear reports whether a wearer can put on an item, after a weapon
// when weapon is not nil.
func (f *recalcFixture) canWear(d *item.Def, w *item.Wearer, weapon *item.Def) bool {
	// Only slots 0 and 1 are used, so clearing them resets the scratch
	// inventory.
	inv := f.scratch
	inv.Slots[0], inv.Slots[1], inv.Weight = item.Slot{}, item.Slot{}, 0
	if weapon != nil {
		inv.Slots[1] = item.Slot{ID: weapon.ID, Amount: 1}
		if _, r := inv.Equip(f.z.Items, 1, item.EquipPoint(weapon, w), w); r != item.EquipOK {
			return false
		}
	}
	pos := item.EquipPoint(d, w)
	if pos == 0 {
		return false
	}
	inv.Slots[0] = item.Slot{ID: d.ID, Amount: 1}
	_, r := inv.Equip(f.z.Items, 0, pos, w)
	return r == item.EquipOK
}

// weaponFor finds the first weapon, by ID, that a wearer can hold and
// that ammunition fits. It tries the first weapon of each type the
// wearer can hold, which it finds once per job, sex and level.
func (f *recalcFixture) weaponFor(ammo *item.Def, w *item.Wearer) *item.Def {
	key := [3]int{w.Job, w.BaseLevel, 0}
	if w.Male {
		key[2] = 1
	}
	held, ok := f.held[key]
	if !ok {
		seen := map[int]bool{}
		for _, x := range f.weapons {
			if !seen[x.Subtype] && f.canWear(x, w, nil) {
				seen[x.Subtype] = true
				held = append(held, x)
			}
		}
		f.held[key] = held
	}
	for _, x := range held {
		if item.AmmoFits(ammo, w, x.Subtype) && f.canWear(ammo, w, x) {
			return x
		}
	}
	return nil
}

// wearerFor finds the first job and sex, by job ID, that the job tables
// compute and that wears an item, with ammunition after a weapon it
// fits.
func (f *recalcFixture) wearerFor(d *item.Def) (wearing, bool) {
	for _, j := range f.jobs {
		lv := max(d.MinLevel, 1)
		if lv > f.prog.MaxBaseLevel(j) {
			continue
		}
		ok, known := f.computes[[2]int{j, lv}]
		if !known {
			_, err := f.tb.Calc(status.Character{Job: j, Male: true, BaseLevel: lv, JobLevel: min(50, f.prog.MaxJobLevel(j)), Stats: f.z.Start.Stats})
			ok = err == nil
			f.computes[[2]int{j, lv}] = ok
		}
		if !ok {
			continue
		}
		mapID, _ := f.tb.MapID(j)
		for _, m := range []bool{true, false} {
			w := &item.Wearer{Job: j, MapID: mapID, Male: m, BaseLevel: lv, Renewal: f.tb.Renewal()}
			if f.canWear(d, w, nil) {
				return wearing{job: j, male: m, level: lv}, true
			}
			if d.Type == "Ammo" {
				if weapon := f.weaponFor(d, w); weapon != nil {
					return wearing{job: j, male: m, level: lv, weapon: weapon}, true
				}
			}
		}
	}
	return wearing{}, false
}

// player joins a player with a job, sex and levels.
func (f *recalcFixture) player(job int, male bool, level int) *Player {
	p := add(f.z, "check")
	p.char.Job, p.char.Male = job, male
	p.progress.Job = job
	p.progress.BaseLevel = level
	p.progress.JobLevel = min(50, f.prog.MaxJobLevel(job))
	f.z.recalc(p)
	f.z.stepAt(f.z.now + 50)
	events(p)
	return p
}

// check saves p, loads the saved character as a new player, and compares
// their stats. It returns "" when they match.
func (f *recalcFixture) check(p *Player) string {
	z := f.z
	c := z.snapshot(p)
	out := make(chan *wathenav1.ServerMessage, outBuffer)
	fresh := &Player{Name: "fresh", Out: out, out: out, load: c}
	z.join(fresh)
	defer z.leave(fresh, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	if !reflect.DeepEqual(p.entity.status, fresh.entity.status) || p.maxWeight != fresh.maxWeight {
		return fmt.Sprintf("event-driven %+v, full %+v", *p.entity.status, *fresh.entity.status)
	}
	return ""
}

// levelUp raises a player's level by up to 20 within the item's level
// range, as a level up computes stats again.
func (f *recalcFixture) levelUp(p *Player, d *item.Def) bool {
	top := f.prog.MaxBaseLevel(p.char.Job)
	if d.MaxLevel > 0 {
		top = min(top, d.MaxLevel)
	}
	lv := min(p.progress.BaseLevel+20, top)
	if lv <= p.progress.BaseLevel {
		return false
	}
	p.progress.BaseLevel = lv
	f.z.recalc(p)
	return true
}

// recalcResult counts one mode's checks.
type recalcResult struct {
	checked, steps int
	mismatches     []string
	skipped        map[string][]uint32
}

func (r *recalcResult) skip(reason string, id uint32) {
	r.skipped[reason] = append(r.skipped[reason], id)
}

// runSteps equips slot, raises the level, and unequips, checking after
// each step. It returns the step that mismatched, or "".
func (f *recalcFixture) runSteps(p *Player, slot int, d *item.Def, r *recalcResult) string {
	z := f.z
	equip(z, p, slot)
	z.stepAt(z.now + 50)
	events(p)
	if p.inventory.Slots[slot].Equip == 0 {
		return "not equipped"
	}
	r.steps++
	if m := f.check(p); m != "" {
		return "equip: " + m
	}
	if f.levelUp(p, d) {
		r.steps++
		if m := f.check(p); m != "" {
			return "level up: " + m
		}
	}
	unequip(z, p, slot)
	z.stepAt(z.now + 50)
	events(p)
	r.steps++
	if m := f.check(p); m != "" {
		return "unequip: " + m
	}
	return ""
}

func isEquipment(d *item.Def) bool {
	switch d.Type {
	case "Weapon", "Armor", "Ammo", "ShadowGear":
		return d.Location != 0
	}
	return false
}

func TestFullRecalculationEveryItem(t *testing.T) {
	testrace.SkipUnderRace(t, "it equips every item on one goroutine")
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			f := loadRecalcFixture(t, mode)
			z := f.z
			res := &recalcResult{skipped: map[string][]uint32{}}
			var equipment, cards []*item.Def
			for _, d := range f.defs {
				switch {
				case d.Type == "Card":
					cards = append(cards, d)
				case isEquipment(d):
					equipment = append(equipment, d)
				}
			}
			for _, d := range equipment {
				who, ok := f.wearerFor(d)
				if !ok {
					res.skip("no job and sex that the job tables compute can wear it", d.ID)
					continue
				}
				p := f.player(who.job, who.male, who.level)
				if who.weapon != nil {
					if z.addCopy(p, who.weapon.ID, 1, 0, nil, item.Extra{}, scriptOrigin) != item.AddSuccess {
						res.skip("cannot be added to the inventory", d.ID)
						z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
						continue
					}
					equip(z, p, slotOf(t, p, who.weapon.ID))
					z.stepAt(z.now + 50)
					events(p)
				}
				refine := 0
				if d.Refineable {
					refine = 4
				}
				amount := 1
				if d.Type == "Ammo" {
					amount = 10
				}
				if r := z.addCopy(p, d.ID, amount, refine, nil, item.Extra{}, scriptOrigin); r != item.AddSuccess {
					res.skip("cannot be added to the inventory", d.ID)
					z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
					continue
				}
				slot := slotOf(t, p, d.ID)
				if m := f.runSteps(p, slot, d, res); m == "not equipped" {
					res.skip("the zone refused to equip it", d.ID)
				} else {
					res.checked++
					if m != "" {
						res.mismatches = append(res.mismatches, fmt.Sprintf("%d %s: %s", d.ID, d.AegisName, m))
					}
				}
				z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
			}
			nItems := res.checked
			// Slotted items by the positions they take, for cards.
			var slotted []*item.Def
			for _, d := range equipment {
				if d.Slots > 0 && d.Type != "Ammo" {
					slotted = append(slotted, d)
				}
			}
			for _, c := range cards {
				var host *item.Def
				var who wearing
				for _, d := range slotted {
					if uint32(d.Location)&uint32(c.Location) == 0 {
						continue
					}
					inv := item.NewInventory()
					inv.Slots[0], inv.Slots[1] = item.Slot{ID: d.ID, Amount: 1}, item.Slot{ID: c.ID, Amount: 1}
					if !inv.InsertCard(z.Items, 1, 0, z.maxCardSlots()) {
						continue
					}
					if w, ok := f.wearerFor(d); ok && w.weapon == nil {
						host, who = d, w
						break
					}
				}
				if host == nil {
					if c.Location == 0 {
						res.skip("an enchant with no position, which the insert command never takes", c.ID)
					} else {
						res.skip("no slotted item takes it", c.ID)
					}
					continue
				}
				p := f.player(who.job, who.male, who.level)
				refine := 0
				if host.Refineable {
					refine = 4
				}
				if z.addCopy(p, host.ID, 1, refine, nil, item.Extra{}, scriptOrigin) != item.AddSuccess || z.addCopy(p, c.ID, 1, 0, nil, item.Extra{}, scriptOrigin) != item.AddSuccess {
					res.skip("cannot be added to the inventory", c.ID)
					z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
					continue
				}
				slot, cardSlot := slotOf(t, p, host.ID), slotOf(t, p, c.ID)
				z.handle(command{player: p, insertCard: &insertCardCommand{cardSlot, slot}})
				z.stepAt(z.now + 50)
				events(p)
				if !slices.Contains(p.inventory.Slots[slot].Cards, c.ID) {
					res.skip("the insert command refused it", c.ID)
					z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
					continue
				}
				if m := f.runSteps(p, slot, host, res); m == "not equipped" {
					res.skip("the zone refused to equip its host", c.ID)
				} else {
					res.checked++
					if m != "" {
						res.mismatches = append(res.mismatches, fmt.Sprintf("card %d %s in %d %s: %s", c.ID, c.AegisName, host.ID, host.AegisName, m))
					}
				}
				z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
			}
			var skipped []string
			for _, reason := range slices.Sorted(maps.Keys(res.skipped)) {
				skipped = append(skipped, fmt.Sprintf("%s: %d", reason, len(res.skipped[reason])))
			}
			t.Logf("%s: %d equipment items and %d cards; checked %d items and %d cards in %d steps; %d mismatches; skipped %s",
				mode, len(equipment), len(cards), nItems, res.checked-nItems, res.steps, len(res.mismatches), strings.Join(skipped, ", "))
			for _, m := range res.mismatches[:min(len(res.mismatches), 20)] {
				t.Errorf("mismatch: %s", m)
			}
			for reason, list := range res.skipped {
				t.Logf("skipped (%s): %v", reason, list[:min(len(list), 30)])
			}
		})
	}
}
