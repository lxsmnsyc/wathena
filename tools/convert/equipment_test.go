package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/status"
)

// The parity tests of equipment: rAthena's item_bonuses.json and
// refine.json against server/status and server/battle, with every
// equipment item's converted bonuses. The recorder equips one item on a
// male sample player with the melee build and job level 50 (lowered to
// the job's maximum), as pc_equipitem with the item's full equip point.
// The counts and every mismatch are golden files in testdata, so a
// change shows in review.

// meleeBuild is the recorder's melee build.
var meleeBuild = status.Stats{Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1}

// forcedMin is rAthena's rnd_force_min: every roll is lowest and every
// chance succeeds.
type forcedMin struct{}

func (forcedMin) IntN(int) int { return 0 }

// measure is one player measurement of the recorder: its status, max
// weight, and damage dealt and taken by monster ID.
type measure struct {
	Status      map[string]json.RawMessage `json:"status"`
	MaxWeight   int                        `json:"max_weight"`
	DamageDealt map[string]int64           `json:"damage_dealt"`
	DamageTaken map[string]int64           `json:"damage_taken"`
}

// equipFixture holds what the parity tests of one mode share.
type equipFixture struct {
	mode    reference.Mode
	r       *modscript.Runtime
	h       *testHost
	core    *Core
	items   *item.Table
	byID    map[uint32]*Item
	paths   map[uint32]string
	tables  *status.Tables
	mobs    map[int]*battle.Status
	renewal bool
	next    int64
}

func loadEquipFixture(t *testing.T, mode reference.Mode) *equipFixture {
	t.Helper()
	r, h, core := testRuntime(t, mode)
	f := &equipFixture{mode: mode, r: r, h: h, core: core, renewal: mode == reference.Renewal, next: 1 << 20}
	var d item.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/items.json", &d); err != nil {
		t.Fatal(err)
	}
	var e item.EquipData
	if err := reference.LoadFile("extracts/"+string(mode)+"/equipment.json", &e); err != nil {
		t.Fatal(err)
	}
	d.MergeEquipment(&e)
	f.byID = map[uint32]*Item{}
	for _, it := range core.Result.Items {
		f.byID[it.ID] = it
	}
	names := core.Names(h.items)
	for i := range d.Items {
		def := &d.Items[i]
		it := f.byID[def.ID]
		if it == nil {
			continue
		}
		for _, entry := range it.Bonuses {
			var v any
			if err := yamlUnmarshal(entry, &v); err != nil {
				t.Fatal(err)
			}
			b, err := bonus.FromData(v, names)
			if err != nil {
				t.Fatalf("%s: %v", it.AegisName, err)
			}
			def.Static = append(def.Static, b)
		}
	}
	f.items = item.New(&d)
	for i := range d.Items {
		h.defs[d.Items[i].ID] = &d.Items[i]
	}
	f.paths = scriptPaths(core.Result)

	var jd status.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &jd); err != nil {
		t.Fatal(err)
	}
	tables, err := status.New(&jd)
	if err != nil {
		t.Fatal(err)
	}
	var rd status.RefineData
	if err := reference.LoadFile("extracts/"+string(mode)+"/refine_db.json", &rd); err != nil {
		t.Fatal(err)
	}
	if err := tables.SetRefine(&rd); err != nil {
		t.Fatal(err)
	}
	f.tables = tables
	f.mobs = loadParityMobs(t, mode)
	return f
}

// loadParityMobs reads every monster's battle status, with its level, ID,
// race groups and DamageTaken.
func loadParityMobs(t *testing.T, mode reference.Mode) map[int]*battle.Status {
	t.Helper()
	var ref struct {
		Mobs []struct {
			ID     int           `json:"id"`
			Level  int           `json:"level"`
			Status battle.Status `json:"status"`
		} `json:"mobs"`
	}
	if err := reference.Load(mode, "mob_status", &ref); err != nil {
		t.Fatal(err)
	}
	var groups struct {
		Monsters []struct {
			ID     int   `json:"id"`
			Groups []int `json:"groups"`
		} `json:"monsters"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/monster_groups.json", &groups); err != nil {
		t.Fatal(err)
	}
	var prog struct {
		Mobs []struct {
			ID          int `json:"id"`
			DamageTaken int `json:"damage_taken"`
		} `json:"mobs"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/progression.json", &prog); err != nil {
		t.Fatal(err)
	}
	race2 := map[int][]int{}
	for _, m := range groups.Monsters {
		race2[m.ID] = m.Groups
	}
	taken := map[int]int{}
	for _, m := range prog.Mobs {
		taken[m.ID] = m.DamageTaken
	}
	out := map[int]*battle.Status{}
	for _, m := range ref.Mobs {
		st := m.Status
		st.Level, st.ID, st.Race2, st.DamageTaken = m.Level, m.ID, race2[m.ID], taken[m.ID]
		out[m.ID] = &st
	}
	return out
}

// player is one sample player with equipment, and the item contexts to
// end afterwards.
type player struct {
	res      status.Result
	contexts []*modscript.Context
}

// equip makes the recorder's sample player with an item in inventory
// slot 0, with a card and a refine, equipped at its full equip point.
func (f *equipFixture) equip(t *testing.T, job, level int, id, card uint32, refine int) (*player, error) {
	t.Helper()
	mapID, ok := f.tables.MapID(job)
	if !ok {
		return nil, fmt.Errorf("job %d is not in the job tables", job)
	}
	jobLevel := min(50, f.tables.MaxJobLevel(job))
	f.h.job, f.h.baseLevel, f.h.jobLevel, f.h.stats = job, level, jobLevel, meleeBuild
	inv := item.NewInventory()
	if id != 0 {
		inv.Slots[0] = item.Slot{ID: id, Amount: 1, Refine: refine}
		if card != 0 {
			inv.Slots[0].Cards = []uint32{card}
		}
		d, ok := f.items.Lookup(id)
		if !ok {
			return nil, fmt.Errorf("item %d is not in the item table", id)
		}
		w := &item.Wearer{Job: job, MapID: mapID, Male: true, BaseLevel: level, Renewal: f.renewal}
		if _, r := inv.Equip(f.items, 0, item.EquipPoint(d, w), w); r != item.EquipOK {
			return nil, fmt.Errorf("cannot equip (result %d)", r)
		}
	}
	p := &player{}
	var failed error
	// Each equipped slot is one equipped handle, which scripts find by
	// equipment slot name. Card scripts read the item that holds them.
	clear(f.h.slots)
	handles := map[int]int64{}
	for i := range inv.Slots {
		s := &inv.Slots[i]
		if s.ID == 0 || s.Equip == 0 {
			continue
		}
		f.next++
		handles[i] = f.next
		f.h.equipped[f.next] = s.ID
		f.h.refine[f.next] = s.Refine
		for name, bit := range slotBits {
			if s.Equip&bit != 0 {
				f.h.slots[name] = f.next
			}
		}
	}
	src := func(slot, c int, d *item.Def) []status.Bonus {
		out := status.StaticBonuses(d, nil)
		path := f.paths[d.ID]
		if path == "" {
			return out
		}
		handle := handles[slot]
		sink := &recorder{}
		ctx, err := f.r.Equip(path, handle, 1, sink)
		if err != nil {
			failed = fmt.Errorf("%s: %w", d.AegisName, err)
			return out
		}
		p.contexts = append(p.contexts, ctx)
		for _, b := range sink.active {
			args, chance, active, err := f.r.Values(b)
			if err != nil {
				failed = fmt.Errorf("%s: %s: %w", d.AegisName, b.Name, err)
				continue
			}
			if active {
				out = append(out, status.Bonus{Name: b.Name, Args: args, Chance: chance, Options: &b.Options})
			}
		}
		return out
	}
	eq := status.Wear(inv, f.items, src)
	res, err := f.tables.Calc(status.Character{Job: job, Male: true, BaseLevel: level, JobLevel: jobLevel, Stats: meleeBuild, Equipment: eq})
	f.unequip(p)
	if err != nil {
		return nil, err
	}
	if failed != nil {
		return nil, failed
	}
	p.res = res
	return p, nil
}

// slotBits are the equipment slot names of player:get_equipped.
var slotBits = map[string]uint32{
	"head_top": item.EqpHeadTop, "head_middle": item.EqpHeadMid, "head_low": item.EqpHeadLow, "armor": item.EqpArmor,
	"left_hand": item.EqpHandL, "right_hand": item.EqpHandR, "garment": item.EqpGarment, "shoes": item.EqpShoes,
	"left_accessory": item.EqpAccL, "right_accessory": item.EqpAccR, "ammo": item.EqpAmmo,
}

func (f *equipFixture) unequip(p *player) {
	for _, c := range p.contexts {
		_ = f.r.Unequip(c)
	}
	p.contexts = nil
}

// measure takes the recorder's measurement of a player. Damage is only
// measured in renewal, since server/battle has renewal's formula.
func (f *equipFixture) measure(t *testing.T, p *player, dealt, taken []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	b, err := json.Marshal(p.res.Battle)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		out["status."+k] = string(v)
	}
	out["max_weight"] = strconv.Itoa(p.res.MaxWeight)
	if !f.renewal {
		return out
	}
	for _, id := range dealt {
		mob := f.mobs[atoi(id)]
		if mob == nil {
			continue
		}
		src := p.res.Battle
		res := battle.Attack(&src, mob, forcedMin{})
		// The recorder keeps rAthena's damage field: the right hand's.
		out["damage_dealt."+id] = strconv.FormatInt(res.Damage-res.Left, 10)
	}
	for _, id := range taken {
		mob := f.mobs[atoi(id)]
		if mob == nil {
			continue
		}
		dst := p.res.Battle
		// The recorder turns perfect dodge off.
		dst.Flee2 = 0
		res := battle.Attack(mob, &dst, forcedMin{})
		out["damage_taken."+id] = strconv.FormatInt(res.Damage-res.Left, 10)
	}
	return out
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// flatten turns a recorded measurement into the keys measure uses, with
// each value as compact JSON.
func flatten(m *measure, renewal bool) map[string]string {
	out := map[string]string{}
	for k, v := range m.Status {
		out["status."+k] = compact(v)
	}
	out["max_weight"] = strconv.Itoa(m.MaxWeight)
	if renewal {
		for k, v := range m.DamageDealt {
			out["damage_dealt."+k] = strconv.FormatInt(v, 10)
		}
		for k, v := range m.DamageTaken {
			out["damage_taken."+k] = strconv.FormatInt(v, 10)
		}
	}
	return out
}

func compact(v []byte) string {
	var b bytes.Buffer
	if err := json.Compact(&b, v); err != nil {
		return string(v)
	}
	return b.String()
}

// sameValue compares a value of ours with a recorded one. Objects compare
// only the keys the record has, since pre-renewal's weapon has fewer.
func sameValue(ours, rec string) bool {
	if ours == rec {
		return true
	}
	var o, r map[string]any
	if json.Unmarshal([]byte(ours), &o) != nil || json.Unmarshal([]byte(rec), &r) != nil {
		return false
	}
	for k, v := range r {
		if fmt.Sprint(o[k]) != fmt.Sprint(v) {
			return false
		}
	}
	return true
}

// itemRecord is one record of item_bonuses.json.
type itemRecord struct {
	Item     uint32                     `json:"item"`
	Card     uint32                     `json:"card"`
	BaseItem uint32                     `json:"base_item"`
	Job      int                        `json:"job"`
	Level    int                        `json:"level"`
	Changes  map[string]json.RawMessage `json:"changes"`
}

// recordedChange splits a recorded [before, after] pair.
func recordedChange(raw json.RawMessage) (string, string) {
	var pair []json.RawMessage
	if json.Unmarshal(raw, &pair) != nil || len(pair) != 2 {
		return "", ""
	}
	return compact(pair[0]), compact(pair[1])
}

// TestEquipmentParity compares every equipment item and card whose
// scripts convert fully with item_bonuses.json, and every refine level
// of refine.json, in both modes.
func TestEquipmentParity(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			f := loadEquipFixture(t, mode)
			var b strings.Builder
			fmt.Fprintf(&b, "# Equipment against rAthena's item_bonuses.json and refine.json (%s)\n\n", mode)
			b.WriteString("Generated by `go test ./tools/convert -run TestEquipmentParity -update`; do not edit.\n\n")
			f.itemParity(t, &b)
			f.refineParity(t, &b)
			got := b.String()
			path := "testdata/equipment_parity_" + string(mode) + ".md"
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != got {
				t.Errorf("%s changed; run go test ./tools/convert -run TestEquipmentParity -update and review the diff:\n%s", path, got[:min(len(got), 1500)])
			}
		})
	}
}

// itemParity is the item_bonuses.json part.
func (f *equipFixture) itemParity(t *testing.T, out *strings.Builder) {
	t.Helper()
	var data struct {
		Baselines map[string]measure `json:"baselines"`
		Items     []itemRecord       `json:"items"`
		Cards     []itemRecord       `json:"cards"`
	}
	if err := reference.Load(f.mode, "item_bonuses", &data); err != nil {
		t.Fatal(err)
	}
	// The baselines first: players with nothing equipped.
	baseOK := 0
	for _, key := range slices.Sorted(maps.Keys(data.Baselines)) {
		job, level, _ := strings.Cut(key, ":")
		p, err := f.equip(t, atoi(job), atoi(level), 0, 0, 0)
		if err != nil {
			t.Errorf("baseline %s: %v", key, err)
			continue
		}
		m := data.Baselines[key]
		want := flatten(&m, f.renewal)
		got := f.measure(t, p, slices.Collect(maps.Keys(m.DamageDealt)), slices.Collect(maps.Keys(m.DamageTaken)))
		if diffs := compareAll(got, want); len(diffs) > 0 {
			t.Errorf("baseline %s: %v", key, diffs)
			continue
		}
		baseOK++
	}

	type stats struct {
		total, notFull, compared, statusOK, damageOK, allOK, notEquipped int
		mismatch                                                         map[string][]string
	}
	run := func(kind string, records []itemRecord) stats {
		s := stats{mismatch: map[string][]string{}}
		for _, rec := range records {
			s.total++
			id, card := rec.Item, uint32(0)
			if kind == "cards" {
				id, card = rec.BaseItem, rec.Card
			}
			if !f.fullyConverted(id) || card != 0 && !f.fullyConverted(card) {
				s.notFull++
				continue
			}
			key := strconv.Itoa(rec.Job) + ":" + strconv.Itoa(rec.Level)
			base, ok := data.Baselines[key]
			if !ok {
				t.Fatalf("no baseline %s", key)
			}
			dealt, taken := slices.Collect(maps.Keys(base.DamageDealt)), slices.Collect(maps.Keys(base.DamageTaken))
			name := f.name(rec.Item, rec.Card)
			p, err := f.equip(t, rec.Job, rec.Level, id, card, 0)
			if err != nil {
				s.notEquipped++
				s.mismatch["not equipped: "+err.Error()] = append(s.mismatch["not equipped: "+err.Error()], name)
				continue
			}
			s.compared++
			got := f.measure(t, p, dealt, taken)
			var want map[string]string
			if kind == "items" {
				want = flatten(&base, f.renewal)
				for k, raw := range rec.Changes {
					if _, after := recordedChange(raw); after != "" {
						if _, known := want[k]; known {
							want[k] = after
						}
					}
				}
			} else {
				// A card is compared by its change to the base item.
				bp, err := f.equip(t, rec.Job, rec.Level, id, 0, 0)
				if err != nil {
					t.Fatalf("%s: base item: %v", name, err)
				}
				before := f.measure(t, bp, dealt, taken)
				// Only the fields the recorder measures.
				want = map[string]string{}
				for k := range flatten(&base, f.renewal) {
					want[k] = before[k]
				}
				for k, raw := range rec.Changes {
					recBefore, recAfter := recordedChange(raw)
					if _, known := want[k]; !known || recAfter == "" {
						continue
					}
					// The base item's own value can differ; the change
					// must match.
					if sameValue(before[k], recBefore) {
						want[k] = recAfter
					} else {
						want[k] = shift(before[k], recBefore, recAfter)
					}
				}
			}
			diffs := compareAll(got, want)
			statusBad, damageBad := false, false
			var fields []string
			for _, d := range diffs {
				field, _, _ := strings.Cut(d, " ")
				if strings.HasPrefix(field, "damage_") {
					damageBad = true
					field, _, _ = strings.Cut(field, ".")
				} else {
					statusBad = true
				}
				if !slices.Contains(fields, field) {
					fields = append(fields, field)
				}
			}
			if !statusBad {
				s.statusOK++
			}
			if !damageBad {
				s.damageOK++
			}
			if len(diffs) == 0 {
				s.allOK++
				continue
			}
			sort.Strings(fields)
			cat := strings.Join(fields, ", ")
			s.mismatch[cat] = append(s.mismatch[cat], name+": "+strings.Join(diffs, "; "))
		}
		return s
	}
	items, cards := run("items", data.Items), run("cards", data.Cards)

	fmt.Fprintf(out, "## item_bonuses.json\n\n")
	fmt.Fprintf(out, "Baselines with nothing equipped: %d of %d match.\n\n", baseOK, len(data.Baselines))
	if !f.renewal {
		out.WriteString("Damage is not compared in pre-renewal: server/battle has renewal's formula only.\n\n")
	}
	out.WriteString("| | Records | Not fully converted | Not equipped | Compared | Stats match | Damage matches | All match |\n|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range []struct {
		name string
		s    stats
	}{{"Equipment", items}, {"Cards", cards}} {
		damage := strconv.Itoa(r.s.damageOK)
		if !f.renewal {
			damage = "not compared"
		}
		fmt.Fprintf(out, "| %s | %d | %d | %d | %d | %d | %s | %d |\n", r.name, r.s.total, r.s.notFull, r.s.notEquipped, r.s.compared, r.s.statusOK, damage, r.s.allOK)
	}
	out.WriteString("\nNot fully converted: the item or card has a script part the converter cannot convert yet (docs/generated/item-scripts-" + string(f.mode) + ".md). Not equipped: the sample player cannot equip it in wAthena.\n\n")
	out.WriteString("Mismatches by the fields that differ, with up to 10 records each:\n\n")
	all := map[string][]string{}
	for _, s := range []stats{items, cards} {
		for k, v := range s.mismatch {
			all[k] = append(all[k], v...)
		}
	}
	cats := slices.Collect(maps.Keys(all))
	sort.Slice(cats, func(i, j int) bool {
		if len(all[cats[i]]) != len(all[cats[j]]) {
			return len(all[cats[i]]) > len(all[cats[j]])
		}
		return cats[i] < cats[j]
	})
	for _, c := range cats {
		list := all[c]
		sort.Strings(list)
		fmt.Fprintf(out, "### %s (%d)\n\n", c, len(list))
		for _, l := range list[:min(len(list), 10)] {
			fmt.Fprintf(out, "- %s\n", l)
		}
		out.WriteString("\n")
	}
}

// shift moves a recorded after value by the difference between our
// before value and the recorded one, for numbers.
func shift(ours, recBefore, recAfter string) string {
	o, err1 := strconv.ParseInt(ours, 10, 64)
	b, err2 := strconv.ParseInt(recBefore, 10, 64)
	a, err3 := strconv.ParseInt(recAfter, 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return recAfter
	}
	return strconv.FormatInt(o+a-b, 10)
}

// compareAll lists the keys of want whose values differ in got.
func compareAll(got, want map[string]string) []string {
	var diffs []string
	for _, k := range slices.Sorted(maps.Keys(want)) {
		if !sameValue(got[k], want[k]) {
			diffs = append(diffs, fmt.Sprintf("%s %s, rAthena %s", k, got[k], want[k]))
		}
	}
	return diffs
}

// fullyConverted reports whether an item's scripts convert fully. An
// item without scripts has nothing to convert.
func (f *equipFixture) fullyConverted(id uint32) bool {
	it := f.byID[id]
	return it == nil || it.Status == Full
}

// name is an item's AegisName and ID, and the card's for a card record.
func (f *equipFixture) name(id, card uint32) string {
	n := func(id uint32) string {
		if d, ok := f.items.Lookup(id); ok {
			return fmt.Sprintf("%s (%d)", d.AegisName, id)
		}
		return strconv.FormatUint(uint64(id), 10)
	}
	if card != 0 {
		return n(card)
	}
	return n(id)
}

// refineParity is the refine.json part: every refine level of each
// recorded item.
func (f *equipFixture) refineParity(t *testing.T, out *strings.Builder) {
	t.Helper()
	var data struct {
		MaxRefine int `json:"max_refine"`
		Items     []struct {
			Item     uint32    `json:"item"`
			Job      int       `json:"job"`
			Level    int       `json:"level"`
			ByRefine []measure `json:"by_refine"`
		} `json:"items"`
	}
	if err := reference.Load(f.mode, "refine", &data); err != nil {
		t.Fatal(err)
	}
	total, ok := 0, 0
	var lines []string
	for _, it := range data.Items {
		for refine, m := range it.ByRefine {
			total++
			name := fmt.Sprintf("%s +%d", f.name(it.Item, 0), refine)
			p, err := f.equip(t, it.Job, it.Level, it.Item, 0, refine)
			if err != nil {
				lines = append(lines, fmt.Sprintf("- %s: %v", name, err))
				continue
			}
			want := flatten(&m, f.renewal)
			got := f.measure(t, p, slices.Collect(maps.Keys(m.DamageDealt)), slices.Collect(maps.Keys(m.DamageTaken)))
			if diffs := compareAll(got, want); len(diffs) > 0 {
				lines = append(lines, fmt.Sprintf("- %s: %s", name, strings.Join(diffs, "; ")))
				continue
			}
			ok++
		}
	}
	fmt.Fprintf(out, "## refine.json\n\n")
	fmt.Fprintf(out, "Refine levels 0 to %d of %d items: %d of %d match", data.MaxRefine, len(data.Items), ok, total)
	if f.renewal {
		out.WriteString(", stats and damage.\n\n")
	} else {
		out.WriteString(", stats.\n\n")
	}
	if len(lines) > 0 {
		out.WriteString("Different:\n\n" + strings.Join(lines, "\n") + "\n")
	}
}
