package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Hand tests of the Blacksmith and Whitesmith skills: the areas, the
// cart, item pick-ups, drops and breaking that the recordings do not
// cover.

// Weapons of both modes: a one-handed sword, which breaks, and an axe,
// which does not.
const (
	smithSword = 1101 // Sword
	smithAxe   = 1301 // Axe
)

// smithSkills are every Merchant, Blacksmith and Whitesmith skill at its
// top level, Pushcart at level 5.
var smithSkills = [][2]int{
	{skill.IDBasic, 9}, {skill.IDIncCarry, 10}, {skill.IDDiscount, 10}, {skillPushCart, 5},
	{skill.IDMammonite, 10}, {skill.IDIron, 5}, {skill.IDSteel, 5}, {skill.IDEnchantedStone, 5},
	{skill.IDOrideocon, 5}, {skill.IDSmithDagger, 3}, {skill.IDSmithSword, 3}, {skill.IDSmithTwoHand, 3},
	{skill.IDSmithAxe, 3}, {skill.IDSmithMace, 3}, {skill.IDSmithKnuckle, 3}, {skill.IDSmithSpear, 3},
	{skill.IDHiltBinding, 1}, {skill.IDFindingOre, 1}, {skill.IDWeaponResearch, 10}, {skill.IDRepairWeapon, 1},
	{skill.IDSkinTemper, 5}, {skill.IDHammerFall, 5}, {skill.IDAdrenaline, 5}, {skill.IDWeaponPerfect, 5},
	{skill.IDOverThrust, 5}, {skill.IDMaximize, 5}, {skill.IDMeltdown, 10}, {skill.IDCartBoost, 1},
	{skill.IDWeaponRefine, 10}, {skill.IDCartTermination, 10}, {skill.IDOverThrustMax, 5}, {skill.IDGreed, 1},
}

// smithZone is a golden zone of a mode with a level 99 Whitesmith at
// (20, 20) holding a weapon, and Golems with a lot of HP that stay
// still. Rolls are forced to their lowest value, so every chance
// succeeds.
func smithZone(t *testing.T, mode reference.Mode, weapon uint32, golems int) (*Zone, *Player, []*entity) {
	t.Helper()
	mobs := make([]uint32, golems)
	for i := range mobs {
		mobs[i] = 1040
	}
	z := goldenZone(t, mode, mobs...)
	p := goldenPlayerIn(t, z, goldenPlayer{Job: 4011, Build: "melee", Sex: 1, JobLevel: 50, Skills: smithSkills}, weapon, 0)
	var out []*entity
	for e := range z.entities.all() {
		if e.monster == nil {
			continue
		}
		st := *e.status
		st.MaxHP = 1000000
		e.status = &st
		e.monster.spawn.Status = &st
		e.hp = 1000000
		e.monster.spawn.CanMove = false
		if e.status != nil {
			// The monster AI does not move a monster without can_move.
			e.status.Mode &^= 1
		}
		out = append(out, e)
	}
	return z, p, out
}

// TestHammerFallStunsLater checks SkillHammerFall: castendPos2 reaches
// the enemies in the square of the splash (2 cells) around the cell,
// and the stun of skill_timerskill comes a second later.
func TestHammerFallStunsLater(t *testing.T) {
	z, p, m := smithZone(t, reference.Renewal, smithAxe, 3)
	m[0].x, m[0].y = 20, 22 // on the cell
	m[1].x, m[1].y = 22, 24 // at the corner of the square
	m[2].x, m[2].y = 20, 25 // outside
	d := z.Skills.Get(skill.IDHammerFall)
	z.castendPos(p.entity, 20, 22, d, 5, 0)
	for i, e := range m {
		if e.sc.Has(sc.SCStun) {
			t.Errorf("monster %d stunned at once", i)
		}
	}
	z.stepAt(z.now + 1000)
	for i, want := range []bool{true, true, false} {
		if got := m[i].sc.Has(sc.SCStun); got != want {
			t.Errorf("monster %d at (%d, %d): stunned %v, want %v", i, m[i].x, m[i].y, got, want)
		}
	}
	if p.entity.sc.Has(sc.SCStun) {
		t.Error("the caster stunned itself")
	}
}

// TestCartRequirement checks pc_setcart, pc_iscarton with the saved
// option bits, ST_CART in skill_check_condition_castbegin, the cart's
// speed penalty in status_calc_speed and SC_CARTBOOST's haste.
func TestCartRequirement(t *testing.T) {
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	cast(z, p, skill.IDCartBoost, 1, p.entity)
	ev := run(z, p, 50, 300)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_CART {
		t.Fatalf("without a cart: failures %v", f)
	}
	walk := p.entity.status.Speed
	if !z.setCart(p, 1) || !z.hasCart(p) {
		t.Fatal("could not get a cart")
	}
	z.flushStats()
	// Pushcart 5 slows by 50% - 25%.
	if got, want := p.entity.status.Speed, walk+walk*25/100; got != want {
		t.Errorf("speed with a cart %d, want %d", got, want)
	}
	cast(z, p, skill.IDCartBoost, 1, p.entity)
	run(z, p, 350, 1500)
	if !p.entity.sc.Has(sc.SCCartboost) {
		t.Fatal("Cart Boost did not start with a cart")
	}
	z.flushStats()
	if got := p.entity.status.Speed; got >= walk+walk*25/100 {
		t.Errorf("Cart Boost did not speed up: %d", got)
	}
	// Type 0 removes the cart. Without Pushcart, no cart.
	if !z.setCart(p, 0) || z.hasCart(p) {
		t.Error("the cart stayed")
	}
	delete(p.char.Skills, skillPushCart)
	if z.setCart(p, 1) || z.hasCart(p) {
		t.Error("got a cart without Pushcart")
	}
	if z.setCart(p, len(cartOptions)) {
		t.Error("an unknown cart type was accepted")
	}
}

// TestCartTerminationNeedsCartBoost checks skill_db's Requires.Status of
// Cart Termination: it fails without SC_CARTBOOST and hits with it. The
// cart is empty, so the ratio is 100% (SkillCartTermination).
func TestCartTerminationNeedsCartBoost(t *testing.T) {
	z, p, m := smithZone(t, reference.Renewal, smithAxe, 1)
	m[0].x, m[0].y = 20, 21
	p.record.Zeny = 100000
	z.setCart(p, 1)
	cast(z, p, skill.IDCartTermination, 10, m[0])
	ev := run(z, p, 50, 300)
	if f := skillFailures(ev); len(f) != 1 {
		t.Fatalf("without Cart Boost: failures %v", f)
	}
	z.scStart(p.entity, p.entity, sc.SCCartboost, 100, 1, 60000)
	cast(z, p, skill.IDCartTermination, 10, m[0])
	ev = run(z, p, 350, 2000)
	if len(skillDamage(ev, skill.IDCartTermination)) != 1 {
		t.Fatal("Cart Termination did not hit")
	}
	if p.record.Zeny != 100000-1500 {
		t.Errorf("zeny %d after a level 10 cast", p.record.Zeny)
	}
	if !m[0].sc.Has(sc.SCStun) {
		t.Error("no stun (applyAdditionalEffects)")
	}
}

// TestGreed checks SkillGreed and pc_takeitem: the caster takes the items
// around it within the splash (2 cells) at any distance, but not an item
// it has no right to yet, nor one farther away.
func TestGreed(t *testing.T) {
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	drop := func(x, y int, first uint32) *entity {
		e := &entity{x: x, y: y, item: &groundItem{id: 909, amount: 1, expires: z.now + 60000,
			rights: z.Progress.NewRights(first, 0, 0, z.now, progression.BossNone)}}
		z.entities.add(e)
		return e
	}
	near := drop(22, 22, 0)
	other := drop(21, 20, p.charID+1)
	far := drop(23, 20, 0)
	d := z.Skills.Get(skill.IDGreed)
	z.castendNoDamage(p.entity, p.entity, d, 1, 0)
	if z.entities.has(near) || p.inventory.Count(909) != 1 {
		t.Errorf("the item 2 cells away was not taken: %d in the inventory", p.inventory.Count(909))
	}
	if !z.entities.has(other) {
		t.Error("took an item of another player")
	}
	if !z.entities.has(far) {
		t.Error("took an item 3 cells away")
	}
}

// TestFindingOre checks mob_dead's Ore Discovery: a first looter with
// Finding Ore drops an ore of IG_ORE besides the monster's drops, by
// get_random_entry with GROUP_ALGORITHM_DROP.
func TestFindingOre(t *testing.T) {
	for _, learned := range []bool{false, true} {
		z, p, m := lootZone(t, &progression.Mob{ID: 1002, Level: 1, MaxHP: 30})
		groups, err := item.NewGroups(&item.GroupData{Groups: []item.Group{{ID: 17, Name: "ORE",
			Subgroups: []item.Subgroup{{Subgroup: 1, Algorithm: item.AlgorithmSharedPool,
				Entries: []item.GroupEntry{{Item: "Iron_Ore", Rate: 10000}}}}}}},
			func(string) (uint32, bool) { return 1002, true })
		if err != nil {
			t.Fatal(err)
		}
		z.Mods = &Mods{Groups: groups}
		p.char.Skills = map[int]int{}
		if learned {
			p.char.Skills[skill.IDFindingOre] = 1
		}
		z.stepAt(50)
		events(p)
		ores := 0
		for _, a := range items(kill(t, z, p, m)) {
			if a.GetItem().GetItemId() == 1002 {
				ores++
			}
		}
		if want := map[bool]int{false: 0, true: 1}[learned]; ores != want {
			t.Errorf("Finding Ore learned %v: %d ores", learned, ores)
		}
	}
}

// TestOverThrustBreaksWeapon checks skill_additional_effect's self
// weapon break: in pre-renewal Power Thrust adds 0.1% per hit, and a
// broken weapon comes off and cannot be equipped (skill_break_equip,
// pc_isequip). Renewal has no such chance, and axes do not break.
func TestOverThrustBreaksWeapon(t *testing.T) {
	for _, c := range []struct {
		mode   reference.Mode
		weapon uint32
		breaks bool
	}{
		{reference.PreRenewal, smithSword, true},
		{reference.Renewal, smithSword, false},
		{reference.PreRenewal, smithAxe, false},
	} {
		z, p, m := smithZone(t, c.mode, c.weapon, 1)
		m[0].x, m[0].y = 20, 21
		z.scStart(p.entity, p.entity, sc.SCOverthrust, 100, 5, 60000)
		slot := -1
		for i, s := range p.inventory.Slots {
			if s.ID == c.weapon {
				slot = i
			}
		}
		z.skillAttack(battle.TypeWeapon, p.entity, p.entity, m[0], z.Skills.Get(skill.IDMammonite), 1, 0)
		s := p.inventory.Slots[slot]
		if s.Broken != c.breaks || (s.Equip == 0) != c.breaks {
			t.Errorf("%s, weapon %d: broken %v, equipped %v", c.mode, c.weapon, s.Broken, s.Equip != 0)
		}
		if !c.breaks {
			continue
		}
		equip(z, p, slot)
		if p.inventory.Slots[slot].Equip != 0 {
			t.Errorf("%s: equipped a broken weapon", c.mode)
		}
	}
}

// TestMeltdownStripsMonsters checks skill_additional_effect's target
// breaking under Melt Down: a monster gets SC_STRIPWEAPON and
// SC_STRIPARMOR (skill_break_equip on a unit that is not a player), but
// not from Cart Termination.
func TestMeltdownStripsMonsters(t *testing.T) {
	z, p, m := smithZone(t, reference.Renewal, smithAxe, 2)
	m[0].x, m[0].y = 20, 21
	m[1].x, m[1].y = 21, 21
	z.scStart(p.entity, p.entity, sc.SCMeltdown, 100, 10, 60000)
	z.skillAttack(battle.TypeWeapon, p.entity, p.entity, m[0], z.Skills.Get(skill.IDMammonite), 1, 0)
	if !m[0].sc.Has(scStripWeapon) || !m[0].sc.Has(scStripArmor) {
		t.Error("Melt Down stripped nothing")
	}
	z.skillAttack(battle.TypeWeapon, p.entity, p.entity, m[1], z.Skills.Get(skill.IDCartTermination), 1, 0)
	if !m[1].sc.Empty() && (m[1].sc.Has(scStripWeapon) || m[1].sc.Has(scStripArmor)) {
		t.Error("Cart Termination stripped")
	}
}

// TestMaximizeDrainsSP checks SC_MAXIMIZEPOWER: a player's status has no
// end and drains 1 SP every val2 ms (status_change_timer), the skill is a
// toggle (StatusSkillImpl with end_if_running), and no SP is left ends
// it.
func TestMaximizeDrainsSP(t *testing.T) {
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	cast(z, p, skill.IDMaximize, 1, p.entity)
	run(z, p, 50, 100)
	e := p.entity.sc.Get(sc.SCMaximizepower)
	if e == nil {
		t.Fatal("Maximize Power did not start")
	}
	if e.Val2 != 1000 {
		t.Errorf("val2 %d, want 1000", e.Val2)
	}
	sp := p.sp
	run(z, p, 150, 3100)
	if got := sp - p.sp; got != 3 {
		t.Errorf("drained %d SP in 3 seconds, want 3", got)
	}
	p.sp = 0
	run(z, p, 3150, 4200)
	if p.entity.sc.Has(sc.SCMaximizepower) {
		t.Error("Maximize Power kept going without SP")
	}
	// The toggle: a second cast ends it.
	p.sp = 100
	cast(z, p, skill.IDMaximize, 1, p.entity)
	run(z, p, 4250, 4400)
	if !p.entity.sc.Has(sc.SCMaximizepower) {
		t.Fatal("Maximize Power did not start again")
	}
	// After the cast's delay.
	run(z, p, 4450, 5500)
	cast(z, p, skill.IDMaximize, 1, p.entity)
	ev := run(z, p, 5550, 5700)
	if p.entity.sc.Has(sc.SCMaximizepower) {
		t.Errorf("the second cast did not end it: failures %v", skillFailures(ev))
	}
}

// TestSmithWindowsNotPorted checks that Weapon Repair and Upgrade Weapon,
// which need item list windows, fail before they start.
func TestSmithWindowsNotPorted(t *testing.T) {
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	sp := p.sp
	cast(z, p, skill.IDWeaponRefine, 10, p.entity)
	cast(z, p, skill.IDRepairWeapon, 1, p.entity)
	ev := run(z, p, 50, 300)
	if f := skillFailures(ev); len(f) != 2 || p.sp != sp {
		t.Errorf("failures %v, SP %d -> %d", f, sp, p.sp)
	}
}
