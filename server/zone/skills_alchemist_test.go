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
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Hand tests of the Alchemist and Creator skills: what the recordings do
// not cover. The zones are golden zones, whose rolls are all lowest, so
// every chance succeeds.

// alchemistIn adds a level 99 melee Alchemist or Creator with skills and
// a weapon to a golden zone.
func alchemistIn(t *testing.T, z *Zone, job int, weapon uint32, skills map[int]int) *Player {
	t.Helper()
	var learned [][2]int
	for id, lv := range skills {
		learned = append(learned, [2]int{id, lv})
	}
	return goldenPlayerIn(t, z, goldenPlayer{Job: job, Build: "melee", Sex: 1, JobLevel: 50, Skills: learned}, weapon, 0)
}

// potionZone is a renewal golden zone whose potions run core's converted
// item scripts.
func potionZone(t *testing.T, mobs ...uint32) *Zone {
	t.Helper()
	z := goldenZone(t, reference.Renewal, mobs...)
	coreMods(t, z)
	var defs []item.Def
	for _, id := range []uint32{501, 504, 547, 657, 7139, 1301} {
		d := *z.Items.Get(id)
		if d.Type == "Healing" || d.Type == "Usable" {
			d.Script = "core:scripts/items/" + d.AegisName + ".lua"
		}
		defs = append(defs, d)
	}
	z.Items = item.New(&item.Data{Mode: "renewal", Items: defs})
	return z
}

// summonTemplates gives a zone the monster records of the summons, as
// waserver's Mods.Monsters.
func summonTemplates(z *Zone) {
	if z.Mods == nil {
		z.Mods = &Mods{}
	}
	z.Mods.Monsters = map[uint32]Spawn{}
	for _, id := range []uint32{1589, 1579, 1575, 1555, 1590, 1142, 1084, 1085, 1078, 1079, 1080, 1081, 1082, 1083} {
		z.Mods.Monsters[id] = Spawn{Mob: id, Name: "plant", Amount: 1, Status: &battle.Status{MaxHP: 1000, Level: 10, Vit: 10},
			Reward: &progression.Mob{ID: id, Level: 10, MaxHP: 1000, BaseExp: 100, JobExp: 100,
				Drops: []progression.Drop{{Item: 909, Rate: 10000}}}}
	}
}

// summonsOf are a zone's monsters of a class.
func summonsOf(z *Zone, class uint32) []*entity {
	var out []*entity
	for e := range z.entities.all() {
		if e.monster != nil && e.monster.spawn.Mob == class {
			out = append(out, e)
		}
	}
	return out
}

// TestBombBurnsWhoStepsIn checks SkillBomb: skill_unitsetting's group of
// one cell (UNT_DEMONSTRATION), whose skill_unit_onplace_timer hits the
// enemies around it with a weapon hit at each interval, and that the
// Bottle Grenade goes once and no ammunition goes (flag 1). A monster's
// weapon does not break (break_mob_equip is off).
func TestBombBurnsWhoStepsIn(t *testing.T) {
	z := goldenZone(t, reference.Renewal, 1002)
	m := assassinMobs(z)[0]
	sturdy(m)
	p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDBomb: 5})
	give(t, z, p, 7135, 2)
	m.x, m.y = 30, 30
	castAt(z, p, skill.IDBomb, 5, 22, 20)
	run(z, p, 50, 1500)
	var g *unitGroup
	for _, x := range z.groups {
		g = x
	}
	if g == nil || len(g.units) != 1 || g.def.ID != skill.IDBomb {
		t.Fatalf("Bomb group %v", g)
	}
	if n := z.countItem(p, 7135); n != 1 {
		t.Fatalf("Bottle Grenades left %d, want 1", n)
	}
	// The monster steps next to the unit: it burns every 500 ms.
	m.x, m.y = 23, 20
	ev := run(z, p, 1550, 3550)
	if hits := skillDamage(ev, skill.IDBomb); len(hits) < 3 || len(hits) > 5 {
		t.Errorf("hits in 2 seconds: %d", len(hits))
	}
	if m.hp >= 1000000 {
		t.Error("the monster took no damage")
	}
	if m.sc.Has(scStripWeapon) {
		t.Error("the monster's weapon broke")
	}
	// The unit ends after Duration1 (60 seconds at level 5).
	run(z, p, 3600, 62000)
	if len(z.groups) != 0 {
		t.Error("the Bomb unit outlived its time")
	}
}

// TestAcidTerrorBleedsThroughPneuma checks SkillAcidTerror's bleeding
// (applyAdditionalEffects), and skill_additional_effect's ATK_BLOCK case:
// a hit Pneuma blocks still causes bleeding.
func TestAcidTerrorBleedsThroughPneuma(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		z := goldenZone(t, reference.PreRenewal, 1002)
		m := assassinMobs(z)[0]
		sturdy(m)
		p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDAcidTerror: 5})
		if blocked {
			z.scStart(m, m, sc.SCPneuma, 100, 1, 60000)
		}
		z.castendDamage(p.entity, m, z.Skills.Get(skill.IDAcidTerror), 5, 0)
		if !m.sc.Has(sc.SCBleeding) {
			t.Errorf("blocked %v: no bleeding", blocked)
		}
		if took := m.hp < 1000000; took == blocked {
			t.Errorf("blocked %v: damage taken %v", blocked, took)
		}
	}
}

// TestBreakEquip checks skill_break_equip: monsters' equipment does not
// break (break_mob_equip), a player's armor breaks at rate 10000 unless
// SC_CP_ARMOR covers it, and an axe never breaks.
func TestBreakEquip(t *testing.T) {
	z := goldenZone(t, reference.Renewal, 1002)
	m := assassinMobs(z)[0]
	a := alchemistIn(t, z, 18, 1301, map[int]int{})
	b := alchemistIn(t, z, 18, 0, map[int]int{})
	if got := z.breakEquip(a.entity, m, item.EqpWeapon|item.EqpArmor, 10000, bctEnemy); got != 0 {
		t.Errorf("monster: broke %#x", got)
	}
	if got := z.breakEquip(b.entity, a.entity, item.EqpWeapon|item.EqpArmor, 10000, bctEnemy); got != item.EqpArmor {
		t.Errorf("player with an axe: broke %#x, want the armor only", got)
	}
	z.scStart(a.entity, a.entity, scCPArmor, 100, 1, 60000)
	if got := z.breakEquip(b.entity, a.entity, item.EqpArmor, 10000, bctEnemy); got != 0 {
		t.Errorf("Chemical Protection: broke %#x", got)
	}
}

// TestChemicalProtection checks SkillAlchemicalWeapon and
// SkillSynthesizedShield (a player wearing an item at the position gets
// the status; without one the skill fails and keeps its Glistening Coat)
// and SkillFullProtection (one status per worn position).
func TestChemicalProtection(t *testing.T) {
	z := goldenZone(t, reference.PreRenewal)
	p := alchemistIn(t, z, 4019, 1301, map[int]int{skill.IDCPWeapon: 5, skill.IDCPShield: 5, skill.IDFullProtection: 5})
	me := p.entity
	give(t, z, p, 7139, 3)
	cast(z, p, skill.IDCPWeapon, 5, me)
	run(z, p, 50, 2500)
	if e := me.sc.Get(scCPWeapon); e == nil || e.Val1 != 5 {
		t.Fatalf("SC_CP_WEAPON %v", e)
	}
	if n := z.countItem(p, 7139); n != 2 {
		t.Errorf("coats after Alchemical Weapon %d, want 2", n)
	}
	cast(z, p, skill.IDCPShield, 5, me)
	ev := run(z, p, 2550, 5000)
	if f := skillFailures(ev); len(f) != 1 || me.sc.Has(scCPShield) {
		t.Errorf("no shield: failures %v, status %v", f, me.sc.Has(scCPShield))
	}
	if n := z.countItem(p, 7139); n != 2 {
		t.Errorf("coats after the failure %d, want 2", n)
	}
	z.scEnd(me, scCPWeapon)
	cast(z, p, skill.IDFullProtection, 5, me)
	run(z, p, 5050, 7500)
	if !me.sc.Has(scCPWeapon) || me.sc.Has(scCPShield) || me.sc.Has(scCPArmor) || me.sc.Has(scCPHelm) {
		t.Errorf("Full Protection with a weapon only: weapon %v shield %v armor %v helm %v", me.sc.Has(scCPWeapon),
			me.sc.Has(scCPShield), me.sc.Has(scCPArmor), me.sc.Has(scCPHelm))
	}
	if n := z.countItem(p, 7139); n != 1 {
		t.Errorf("coats after Full Protection %d, want 1", n)
	}
}

// TestPotionPitcherHeals checks SkillAidPotion: the Red Potion of level 1
// runs its script with potion_flag, and heals rand(45, 65) at its lowest
// with the caster's Aid Potion and Potion Research and the target's VIT.
// The potion is used.
func TestPotionPitcherHeals(t *testing.T) {
	z := potionZone(t)
	p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDPotionPitcher: 5, skill.IDLearningPotion: 10})
	me := p.entity
	give(t, z, p, 501, 2)
	me.hp = 1
	cast(z, p, skill.IDPotionPitcher, 1, me)
	run(z, p, 50, 1000)
	// 45 * (100 + 50 + 50) / 100, then VIT.
	want := int64(45*200*100/10000*(100+me.status.Vit*2)/100) + 1
	if me.hp != want {
		t.Errorf("HP %d, want %d", me.hp, want)
	}
	if n := z.countItem(p, 501); n != 1 {
		t.Errorf("Red Potions left %d, want 1", n)
	}
}

// TestBerserkPitcherHalvesTheStatus checks SkillAidBerserkPotion and
// buildin_sc_start with potion_target: the Berserk Potion's
// SC_ASPDPOTION2 goes on the target for half its time, with val4 1, and
// both potions are used.
func TestBerserkPitcherHalvesTheStatus(t *testing.T) {
	z := potionZone(t)
	p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDBerserkPitcher: 1})
	q := alchemistIn(t, z, 18, 0, map[int]int{})
	give(t, z, p, 657, 2)
	z.castendNoDamage(p.entity, q.entity, z.Skills.Get(skill.IDBerserkPitcher), 1, 0)
	e := q.entity.sc.Get(sc.SCAspdpotion2)
	if e == nil || e.Val4 != 1 || e.Timer()-z.now != 900000 {
		t.Fatalf("SC_ASPDPOTION2 %+v", e)
	}
	if p.entity.sc.Has(sc.SCAspdpotion2) {
		t.Error("the thrower drank it")
	}
	if n := z.countItem(p, 657); n != 0 {
		t.Errorf("Berserk Potions left %d", n)
	}
}

// TestAidCondensedPotionArea checks SkillAidCondensedPotion: the White
// Slim Potion of level 10 heals the caster's side in the 7x7 square
// around the cell (BCT_PARTY and BCT_GUILD): the caster and its own
// summon, not another player without a party, nor a monster, nor anyone
// outside the square.
func TestAidCondensedPotionArea(t *testing.T) {
	z := potionZone(t, 1002)
	summonTemplates(z)
	m := assassinMobs(z)[0]
	p := alchemistIn(t, z, 4019, 0, map[int]int{skill.IDSlimPitcher: 10, skill.IDPotionPitcher: 5, skill.IDLearningPotion: 10})
	other := alchemistIn(t, z, 18, 0, map[int]int{})
	far := alchemistIn(t, z, 18, 0, map[int]int{})
	me := p.entity
	me.x, me.y = 20, 20
	other.entity.x, other.entity.y = 22, 22
	far.entity.x, far.entity.y = 25, 20
	m.x, m.y = 21, 22
	m.hp = 1
	flora := z.summonMonster(me, 1589, 20, 23, p.Name, aiFlora, 60000)
	flora.status.MaxHP, flora.hp = 100000, 1
	for _, e := range []*entity{me, other.entity, far.entity} {
		e.hp = 1
	}
	give(t, z, p, 547, 1)
	castAt(z, p, skill.IDSlimPitcher, 10, 21, 21)
	run(z, p, 50, 3000)
	// 325 * (100 + 100 + 50 + 50) / 100, then the target's VIT.
	base := 325 * 300 / 100
	if want := int64(base*(100+me.status.Vit*2)/100) + 1; me.hp != want {
		t.Errorf("caster HP %d, want %d", me.hp, want)
	}
	if want := int64(base*(100+10*2)/100) + 1; flora.hp != want {
		t.Errorf("own flora HP %d, want %d", flora.hp, want)
	}
	if other.entity.hp != 1 || far.entity.hp != 1 || m.hp != 1 {
		t.Errorf("healed others: player %d, far player %d, monster %d", other.entity.hp, far.entity.hp, m.hp)
	}
	if n := z.countItem(p, 547); n != 0 {
		t.Errorf("White Slim Potions left %d", n)
	}
}

// TestSummonFlora checks SkillSummonFlora and skill_check_condition_castbegin
// for AM_CANNIBALIZE: the level's plant appears with its master's name,
// players may hit it in pre-renewal but not in renewal
// (alchemist_summon_setting), at most 6 less the level are out, it
// leaves after Duration1, and it dies with its master gone.
func TestSummonFlora(t *testing.T) {
	for _, mode := range reference.Modes {
		z := goldenZone(t, mode)
		summonTemplates(z)
		p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDCannibalize: 5})
		other := alchemistIn(t, z, 18, 0, map[int]int{})
		give(t, z, p, 7137, 3)
		castAt(z, p, skill.IDCannibalize, 5, 22, 20)
		run(z, p, 50, 3000)
		got := summonsOf(z, 1590)
		if len(got) != 1 {
			t.Fatalf("%s: Geographers %d", mode, len(got))
		}
		g := got[0]
		if g.monster.master != p.entity || g.monster.spawn.Name != p.Name || g.x != 22 || g.y != 20 {
			t.Errorf("%s: master %v, name %q, cell %d,%d", mode, g.monster.master == p.entity, g.monster.spawn.Name, g.x, g.y)
		}
		if hit := isEnemy(other.entity, g); hit != (mode == reference.PreRenewal) {
			t.Errorf("%s: players may hit it: %v", mode, hit)
		}
		z.handle(command{player: other, attack: &attackCommand{target: other.netID(g)}})
		if attacks := other.entity.target == g; attacks != (mode == reference.PreRenewal) {
			t.Errorf("%s: another player attacks it: %v", mode, attacks)
		}
		other.entity.target = nil
		// Level 5 allows one.
		castAt(z, p, skill.IDCannibalize, 5, 23, 20)
		ev := run(z, p, 3050, 6000)
		if f := skillFailures(ev); len(f) != 1 || len(summonsOf(z, 1590)) != 1 {
			t.Errorf("%s: second Geographer: failures %v, out %d", mode, f, len(summonsOf(z, 1590)))
		}
		// It leaves after 60 seconds at level 5.
		run(z, p, 6050, 63100)
		if len(summonsOf(z, 1590)) != 0 {
			t.Errorf("%s: the Geographer outlived its time", mode)
		}
		// A summon whose master left dies.
		castAt(z, p, skill.IDCannibalize, 5, 22, 20)
		run(z, p, 63150, 66000)
		if len(summonsOf(z, 1590)) != 1 {
			t.Fatalf("%s: no second Geographer", mode)
		}
		z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
		z.stepAt(66100)
		if len(summonsOf(z, 1590)) != 0 {
			t.Errorf("%s: the Geographer outlived its master", mode)
		}
	}
}

// TestMarineSphereLifetime checks SkillSummonMarineSphere: a Marine Sphere
// of the caster's for 30 seconds, at most ActiveInstance (3) at once.
func TestMarineSphereLifetime(t *testing.T) {
	z := goldenZone(t, reference.Renewal)
	summonTemplates(z)
	p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDSphereMine: 5})
	give(t, z, p, 7138, 4)
	now := int64(50)
	for i := range 4 {
		castAt(z, p, skill.IDSphereMine, 5, 21+i, 20)
		run(z, p, now, now+2500)
		now += 2550
	}
	if n := len(summonsOf(z, mobMarineSphere)); n != 3 {
		t.Errorf("Marine Spheres %d, want 3", n)
	}
	if n := z.countItem(p, 7138); n != 1 {
		t.Errorf("Marine Sphere Bottles left %d, want 1", n)
	}
	// The first came at about 2 seconds, the others 2.5 seconds apart.
	run(z, p, now, 33000)
	if n := len(summonsOf(z, mobMarineSphere)); n != 2 {
		t.Errorf("Marine Spheres after 33 seconds %d, want 2", n)
	}
	run(z, p, 33050, 40000)
	if n := len(summonsOf(z, mobMarineSphere)); n != 0 {
		t.Errorf("Marine Spheres after 40 seconds %d", n)
	}
}

// maxSource makes every roll its highest, so rnd()%100 is 99.
type maxSource struct{}

func (maxSource) Uint64() uint64 { return ^uint64(0) }

// TestPlantCultivation checks SkillPlantCultivation (pre-renewal): with
// the lowest roll the plant fails and the item goes; with the highest,
// level 1 grows a Red Mushroom and level 2 a Shining Plant, which give
// their usual exp (no master). A cell with a character fails and keeps
// the item.
func TestPlantCultivation(t *testing.T) {
	z := goldenZone(t, reference.PreRenewal)
	summonTemplates(z)
	p := alchemistIn(t, z, 4019, 0, map[int]int{skill.IDCultivation: 2})
	give(t, z, p, 921, 2)
	give(t, z, p, 905, 1)
	castAt(z, p, skill.IDCultivation, 1, 21, 20)
	run(z, p, 50, 1000)
	if z.countItem(p, 921) != 1 || len(summonsOf(z, 1084))+len(summonsOf(z, 1085)) != 0 {
		t.Fatalf("lowest roll: mushrooms %d, items left %d", len(summonsOf(z, 1084))+len(summonsOf(z, 1085)), z.countItem(p, 921))
	}
	z.rand = rng.New(maxSource{})
	castAt(z, p, skill.IDCultivation, 1, 21, 20)
	run(z, p, 1050, 2000)
	got := summonsOf(z, 1085)
	if len(got) != 1 || got[0].monster.master != nil || got[0].monster.spawn.Reward.BaseExp != 100 {
		t.Fatalf("Red Mushrooms %d", len(got))
	}
	// The mushroom's cell is taken.
	castAt(z, p, skill.IDCultivation, 2, 21, 20)
	ev := run(z, p, 2050, 3000)
	if f := skillFailures(ev); len(f) != 1 || z.countItem(p, 905) != 1 {
		t.Errorf("taken cell: failures %v, stems %d", f, z.countItem(p, 905))
	}
	castAt(z, p, skill.IDCultivation, 2, 22, 21)
	run(z, p, 3050, 4000)
	if len(summonsOf(z, 1083)) != 1 || z.countItem(p, 905) != 0 {
		t.Errorf("Shining Plants %d", len(summonsOf(z, 1083)))
	}
	// They leave after 5 minutes.
	run(z, p, 4050, 4050)
	z.stepAt(305000)
	if len(summonsOf(z, 1085))+len(summonsOf(z, 1083)) != 0 {
		t.Error("the plants outlived their time")
	}
}

// TestTwilightAlchemy checks SkillTwilightAlchemy1 through
// skill_produce_mix: 200 White Potions from produce_db's entry 121, which
// keeps its guide; and AM_TWILIGHT2's party_skill_check, which fails
// without a party and costs nothing.
func TestTwilightAlchemy(t *testing.T) {
	z := goldenZone(t, reference.Renewal)
	p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDPharmacy: 10, skill.IDLearningPotion: 10, skill.IDTwilight1: 1,
		skill.IDTwilight2: 1})
	me := p.entity
	for _, it := range [][2]int{{7134, 400}, {7144, 1}, {509, 200}, {1093, 200}} {
		give(t, z, p, uint32(it[0]), it[1])
	}
	cast(z, p, skill.IDTwilight1, 1, me)
	run(z, p, 50, 4000)
	if n := z.countItem(p, itemWhitePotion); n != 200 {
		t.Errorf("White Potions %d, want 200", n)
	}
	if z.countItem(p, 7134) != 200 || z.countItem(p, 7144) != 1 || z.countItem(p, 509) != 0 || z.countItem(p, 1093) != 0 {
		t.Errorf("left: bowls %d, guide %d, herbs %d, bottles %d", z.countItem(p, 7134), z.countItem(p, 7144),
			z.countItem(p, 509), z.countItem(p, 1093))
	}
	sp := p.sp
	cast(z, p, skill.IDTwilight2, 1, me)
	ev := run(z, p, 14050, 20000)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL || p.sp < sp ||
		z.countItem(p, 7134) != 200 {
		t.Errorf("Twilight Alchemy 2: failures %v, SP %d of %d, bowls %d", f, p.sp, sp, z.countItem(p, 7134))
	}
}

// TestHomunculusSkillsWait checks that the skills that need a homunculus
// or the recipe window fail before they start and cost nothing.
func TestHomunculusSkillsWait(t *testing.T) {
	z := goldenZone(t, reference.Renewal)
	p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDCallHomunculus: 1, skill.IDPharmacy: 10})
	give(t, z, p, 7142, 1)
	give(t, z, p, 7134, 1)
	sp := p.sp
	cast(z, p, skill.IDCallHomunculus, 1, p.entity)
	cast(z, p, skill.IDPharmacy, 10, p.entity)
	ev := run(z, p, 50, 1000)
	if f := skillFailures(ev); len(f) != 2 || p.sp != sp || z.countItem(p, 7142) != 1 || z.countItem(p, 7134) != 1 {
		t.Errorf("failures %v, SP %d of %d", f, p.sp, sp)
	}
}
