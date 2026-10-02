package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Hand tests of the Rogue's and the Stalker's skills, for what the
// recordings do not cover. The zones are golden zones: rolls are lowest,
// so every chance succeeds.

const (
	jobRogue   = 17
	jobStalker = 4018
	itemKnife  = 1201
	itemSword  = 1101
	itemGuard  = 2101
	itemShirt  = 2302
	itemHat    = 2220
	mobGolem   = 1040
	mobOrc     = 1023
)

// rogueZone is a golden zone of a mode with a level 99 player of a job
// with skills and a weapon, and monsters from (20, 22) east.
func rogueZone(t *testing.T, mode reference.Mode, job int, skills map[int]int, weapon uint32, mobs ...uint32) (*Zone, *Player) {
	t.Helper()
	z := goldenZone(t, mode, mobs...)
	sp := goldenPlayer{Job: job, Build: "melee", Sex: 1, JobLevel: 50}
	for id, lv := range skills {
		sp.Skills = append(sp.Skills, [2]int{id, lv})
	}
	return z, goldenPlayerIn(t, z, sp, weapon, 0)
}

// worn reports whether a player wears an item.
func worn(p *Player, id uint32) bool {
	for _, s := range p.inventory.Slots {
		if s.ID == id && s.Equip != 0 {
			return true
		}
	}
	return false
}

// TestSnatcherStealsOnNormalAttacks checks the Gank part of
// skill_additional_effect: a normal attack on a monster steals, and a
// bow never does.
func TestSnatcherStealsOnNormalAttacks(t *testing.T) {
	for _, c := range []struct {
		weapon uint32
		steals bool
	}{{itemKnife, true}, {itemBow, false}} {
		z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDSnatcher: 10, skill.IDSteal: 10}, c.weapon, mobGolem)
		m := monsterOf(z, mobGolem)
		m.monster.spawn.Reward = &progression.Mob{Drops: []progression.Drop{{Item: 501, Rate: 5000}}}
		z.strike(p.entity, m)
		if got := z.countItem(p, 501) > 0; got != c.steals || m.monster.stolen != c.steals {
			t.Errorf("weapon %d: stole %v (flag %v), want %v", c.weapon, got, m.monster.stolen, c.steals)
		}
	}
}

// TestMugStealsZenyOnce checks SkillMug and skill_castend_id_check: the
// zeny stolen, then a failure on the same monster.
func TestMugStealsZenyOnce(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDStealCoin: 10}, itemKnife, mobPoring)
	m := monsterOf(z, mobPoring)
	p.entity.x, p.entity.y = m.x+1, m.y
	cast(z, p, skill.IDStealCoin, 10, m)
	run(z, p, 100, 1000)
	// rnd_value(8, 10) at its lowest is 8, plus 10 * 1 / 10.
	if p.record.Zeny != 9 || !m.monster.stoleCoin {
		t.Fatalf("zeny %d, mugged %v; want 9 and true", p.record.Zeny, m.monster.stoleCoin)
	}
	cast(z, p, skill.IDStealCoin, 10, m)
	ev := run(z, p, 1100, 2000)
	if f := skillFailures(ev); len(f) != 1 || p.record.Zeny != 9 {
		t.Errorf("second Mug: failures %v, zeny %d", f, p.record.Zeny)
	}
}

// TestBackStabRenewalStepsBehind checks renewal's
// SkillBackStab::castendDamageId: the caster stops hiding, steps behind
// the target, and the target turns to it.
func TestBackStabRenewalStepsBehind(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDBackStab: 10}, itemKnife, mobGolem)
	m := monsterOf(z, mobGolem)
	z.scStart(p.entity, p.entity, sc.SCHiding, 100, 10, 30000)
	hp := m.hp
	z.castendDamage(p.entity, m, z.Skills.Get(skill.IDBackStab), 10, 0)
	if p.entity.x != m.x || p.entity.y != m.y+1 {
		t.Errorf("caster at (%d, %d), want behind the target at (%d, %d)", p.entity.x, p.entity.y, m.x, m.y+1)
	}
	if p.entity.sc.Has(sc.SCHiding) || m.hp >= hp {
		t.Errorf("hiding %v, target HP %d of %d", p.entity.sc.Has(sc.SCHiding), m.hp, hp)
	}
	if rDir(m) != 4 {
		t.Errorf("target faces %d, want 4 (south, to the caster)", rDir(m))
	}
}

// TestBackStabPreRenewalFromBehind checks pre-renewal's
// skill_castend_id_check and SkillBackStab: only a target that faces
// away can be stabbed.
func TestBackStabPreRenewalFromBehind(t *testing.T) {
	z, p := rogueZone(t, reference.PreRenewal, jobRogue, map[int]int{skill.IDBackStab: 10}, itemKnife, mobGolem)
	m := monsterOf(z, mobGolem)
	d := z.Skills.Get(skill.IDBackStab)
	// The caster looks north at the target. A target facing south sees
	// it.
	m.dir = protocolDir(4)
	if r := z.castEndIDCheck(p.entity, m, d); r != skillFailSilent {
		t.Errorf("facing the caster: check %v, want a silent failure", r)
	}
	m.dir = protocolDir(0)
	if r := z.castEndIDCheck(p.entity, m, d); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Errorf("facing away: check %v", r)
	}
	hp := m.hp
	z.castendDamage(p.entity, m, d, 10, 0)
	if m.hp >= hp || p.entity.x != 20 || p.entity.y != 20 {
		t.Errorf("target HP %d of %d, caster at (%d, %d)", m.hp, hp, p.entity.x, p.entity.y)
	}
}

// TestRaidFromHiding checks SkillSightlessMind: it needs hiding, hits
// the enemies around the caster with stun, blindness and renewal's
// SC_RAID, and ends hiding.
func TestRaidFromHiding(t *testing.T) {
	for _, mode := range reference.Modes {
		z, p := rogueZone(t, mode, jobRogue, map[int]int{skill.IDRaid: 5}, itemKnife, mobGolem, mobOrc)
		golem, orc := monsterOf(z, mobGolem), monsterOf(z, mobOrc)
		// Pre-renewal's area is 1 cell around the caster.
		golem.x, golem.y = 20, 21
		orc.x, orc.y = 30, 30
		cast(z, p, skill.IDRaid, 5, nil)
		if ev := run(z, p, 100, 200); len(skillFailures(ev)) != 1 {
			t.Fatalf("%s: Sightless Mind without hiding did not fail", mode)
		}
		z.scStart(p.entity, p.entity, sc.SCHiding, 100, 10, 30000)
		cast(z, p, skill.IDRaid, 5, nil)
		run(z, p, 1000, 1200)
		if p.entity.sc.Has(sc.SCHiding) {
			t.Errorf("%s: still hidden", mode)
		}
		if !golem.sc.Has(sc.SCStun) || !golem.sc.Has(sc.SCBlind) || golem.sc.Has(sc.SCRaid) != (mode == reference.Renewal) {
			t.Errorf("%s: Golem stun %v, blind %v, raid %v", mode, golem.sc.Has(sc.SCStun), golem.sc.Has(sc.SCBlind),
				golem.sc.Has(sc.SCRaid))
		}
		if !orc.sc.Empty() || orc.hp < int64(orc.status.MaxHP) {
			t.Errorf("%s: the Orc out of the area was hit", mode)
		}
	}
}

// TestRaidDamageBonus checks renewal's SC_RAID in battle_calc_damage:
// 30% more damage, 15% for a boss.
func TestRaidDamageBonus(t *testing.T) {
	for _, c := range []struct {
		boss bool
		pct  int64
	}{{false, 130}, {true, 115}} {
		z, p := rogueZone(t, reference.Renewal, jobRogue, nil, itemKnife, mobGolem)
		m := monsterOf(z, mobGolem)
		if c.boss {
			m.status.Class = battle.ClassBoss
		}
		before := goldenAttack(z, p.entity, m).Damage
		z.scStart(p.entity, m, sc.SCRaid, 100, 1, 10000)
		after := goldenAttack(z, p.entity, m).Damage
		if after != before*c.pct/100 {
			t.Errorf("boss %v: %d after SC_RAID, want %d%% of %d", c.boss, after, c.pct, before)
		}
	}
}

// TestIntimidateWarpsBoth checks skill_attack and skill_timerskill for
// RG_INTIMIDATE: 800 ms after the hit, the caster warps to a random cell
// and the target to a cell next to it.
func TestIntimidateWarpsBoth(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDIntimidate: 5}, itemKnife, mobGolem)
	m := monsterOf(z, mobGolem)
	z.castendDamage(p.entity, m, z.Skills.Get(skill.IDIntimidate), 5, 0)
	if p.entity.x != 20 || p.entity.y != 20 {
		t.Fatal("warped before 800 ms")
	}
	start := z.now
	run(z, p, start+800, start+800)
	// map_search_freecell at its lowest roll: the map's edge size, then
	// the cell up and left of the caster.
	if p.entity.x != mapEdgeSize || p.entity.y != mapEdgeSize {
		t.Errorf("caster at (%d, %d), want (15, 15)", p.entity.x, p.entity.y)
	}
	if m.x != mapEdgeSize-1 || m.y != mapEdgeSize-1 {
		t.Errorf("target at (%d, %d), want (14, 14)", m.x, m.y)
	}
}

// TestTunnelDriveMovesHidden checks SC_HIDING's NoMoveCond and
// status_calc_speed: a hidden player moves only with Tunnel Drive, and
// at its speed.
func TestTunnelDriveMovesHidden(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDHiding: 10}, itemKnife)
	speed := p.entity.status.Speed
	z.scStart(p.entity, p.entity, sc.SCHiding, 100, 10, 30000)
	if !p.entity.cant().Move {
		t.Error("hidden without Tunnel Drive, the player can move")
	}
	z, p = rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDHiding: 10, skill.IDTunnelDrive: 5}, itemKnife)
	z.scStart(p.entity, p.entity, sc.SCHiding, 100, 10, 30000)
	if p.entity.cant().Move {
		t.Error("hidden with Tunnel Drive, the player cannot move")
	}
	// 120 - 6 * 5 = 90% slower.
	if want := speed * 190 / 100; p.entity.status.Speed != want {
		t.Errorf("speed %d, want %d", p.entity.status.Speed, want)
	}
}

// strippee adds a second player who wears a sword, a shield, an armor
// and a hat.
func strippee(t *testing.T, z *Zone) *Player {
	t.Helper()
	q := add(z, "target")
	q.char.Job = 1
	q.progress.Job, q.progress.BaseLevel, q.progress.JobLevel = 1, 99, 50
	q.maxWeight = 1 << 30
	q.entity.x, q.entity.y = 21, 20
	z.recalc(q)
	for _, id := range []uint32{itemSword, itemGuard, itemShirt, itemHat} {
		equip(z, q, give(t, z, q, id, 1))
		if !worn(q, id) {
			t.Fatalf("item %d did not go on", id)
		}
	}
	return q
}

// TestDivestWeaponOnPlayer checks skill_strip_equip and the strip part
// of status_change_start and pc_isequip on a player: the weapon comes
// off and cannot go back on while the status lasts, and Chemical
// Protection keeps it on.
func TestDivestWeaponOnPlayer(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDStripWeapon: 5}, itemKnife)
	q := strippee(t, z)
	d := z.Skills.Get(skill.IDStripWeapon)
	z.scStart(q.entity, q.entity, scCPWeapon, 100, 5, 60000)
	z.castendNoDamage(p.entity, q.entity, d, 5, 0)
	if !worn(q, itemSword) || q.entity.sc.Has(sc.SCStripweapon) {
		t.Fatal("Chemical Protection did not keep the weapon on")
	}
	z.scEnd(q.entity, scCPWeapon)
	z.castendNoDamage(p.entity, q.entity, d, 5, 0)
	if worn(q, itemSword) || !q.entity.sc.Has(sc.SCStripweapon) {
		t.Fatalf("sword worn %v, status %v", worn(q, itemSword), q.entity.sc.Has(sc.SCStripweapon))
	}
	if !worn(q, itemGuard) {
		t.Error("the shield came off too")
	}
	for i, s := range q.inventory.Slots {
		if s.ID == itemSword {
			equip(z, q, i)
		}
	}
	if worn(q, itemSword) {
		t.Error("the sword went back on while stripped")
	}
}

// TestDivestAllOnPlayer checks SkillDivestAll: the four pieces come off
// with their four statuses.
func TestDivestAllOnPlayer(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobStalker, map[int]int{skill.IDFullStrip: 5}, itemKnife)
	q := strippee(t, z)
	z.castendNoDamage(p.entity, q.entity, z.Skills.Get(skill.IDFullStrip), 5, 0)
	for _, id := range []uint32{itemSword, itemGuard, itemShirt, itemHat} {
		if worn(q, id) {
			t.Errorf("item %d still worn", id)
		}
	}
	for _, typ := range []int{sc.SCStripweapon, sc.SCStripshield, sc.SCStriparmor, sc.SCStriphelm} {
		if !q.entity.sc.Has(typ) {
			t.Errorf("status %d missing", typ)
		}
	}
}

// TestDivestOnBoss checks that a boss resists the strips (BossResist),
// so the skill fails.
func TestDivestOnBoss(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDStripArmor: 5}, itemKnife, 1039)
	m := monsterOf(z, 1039)
	z.castendNoDamage(p.entity, m, z.Skills.Get(skill.IDStripArmor), 5, 0)
	if m.sc.Has(sc.SCStriparmor) {
		t.Error("the boss lost its armor")
	}
	if f := skillFailures(run(z, p, 100, 100)); len(f) != 1 {
		t.Errorf("failures %v", f)
	}
}

// TestCloseConfineHoldsBoth checks SkillCloseConfine and the
// SC_CLOSECONFINE2 setup: the target and the caster cannot move, the
// caster gains FLEE, and a knockback ends both holds.
func TestCloseConfineHoldsBoth(t *testing.T) {
	for _, mode := range reference.Modes {
		z, p := rogueZone(t, mode, jobRogue, map[int]int{skill.IDCloseConfine: 1}, itemKnife, mobGolem)
		m := monsterOf(z, mobGolem)
		flee := p.entity.status.Flee
		z.castendNoDamage(p.entity, m, z.Skills.Get(skill.IDCloseConfine), 1, 0)
		bonus := 50
		if mode == reference.PreRenewal {
			bonus = 10
		}
		if !m.sc.Has(sc.SCCloseconfine2) || !p.entity.sc.Has(sc.SCCloseconfine) {
			t.Fatalf("%s: target %v, caster %v", mode, m.sc.Has(sc.SCCloseconfine2), p.entity.sc.Has(sc.SCCloseconfine))
		}
		if !m.cant().Move || !p.entity.cant().Move || p.entity.status.Flee != flee+bonus {
			t.Errorf("%s: target held %v, caster held %v, FLEE %d of %d", mode, m.cant().Move, p.entity.cant().Move,
				p.entity.status.Flee, flee)
		}
		z.skillBlown(p.entity, m, 2, -1, true)
		if m.sc.Has(sc.SCCloseconfine2) || p.entity.sc.Has(sc.SCCloseconfine) {
			t.Errorf("%s: knockback left target %v, caster %v", mode, m.sc.Has(sc.SCCloseconfine2), p.entity.sc.Has(sc.SCCloseconfine))
		}
	}
}

// TestCloseConfineCasterLetsGo checks the end of SC_CLOSECONFINE: the
// holder's end frees the targets near it.
func TestCloseConfineCasterLetsGo(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDCloseConfine: 1}, itemKnife, mobGolem)
	m := monsterOf(z, mobGolem)
	z.castendNoDamage(p.entity, m, z.Skills.Get(skill.IDCloseConfine), 1, 0)
	z.scEnd(p.entity, sc.SCCloseconfine)
	if m.sc.Has(sc.SCCloseconfine2) {
		t.Error("the target is still held")
	}
}

// TestGraffitiAndRemover checks SkillScribble, its castbegin check, and
// SkillRemover: one Graffiti on the map, which Remover takes away.
func TestGraffitiAndRemover(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDGraffiti: 1, skill.IDCleaner: 1}, itemKnife)
	g := z.Skills.Get(skill.IDGraffiti)
	z.castendPos(p.entity, 22, 22, g, 1, 0)
	if z.graffitiRemover(nil, false) != 1 {
		t.Fatal("no Graffiti unit")
	}
	for _, grp := range z.groups {
		if grp.unitID != unitGraffiti || grp.text != "" {
			t.Errorf("unit %#x with text %q", grp.unitID, grp.text)
		}
	}
	if graffiti.castBegin(graffiti{}, z, p.entity, g, 1) == wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Error("a second Graffiti may start")
	}
	z.castendPos(p.entity, 25, 25, z.Skills.Get(skill.IDCleaner), 1, 0)
	if z.graffitiRemover(nil, false) != 0 {
		t.Error("Remover left the Graffiti")
	}
	z.castendPos(p.entity, 22, 22, z.Skills.Get(skill.IDFlagGraffiti), 1, 0)
	if len(z.groups) != 0 {
		t.Error("Flag Graffiti placed a unit")
	}
}

// TestGangsterParadise checks skill_sit and the target check of mob_ai:
// two sitting players with the skill are safe from monsters that are not
// bosses, until one stands up.
func TestGangsterParadise(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDGangster: 1}, itemKnife, mobGolem)
	q := add(z, "b")
	q.char.Skills = map[int]int{skill.IDGangster: 1}
	q.entity.x, q.entity.y = 21, 20
	p.rogue.sitting = true
	z.skillSit(p, true)
	if p.rogue.gangster {
		t.Fatal("one sitter is protected")
	}
	q.rogue.sitting = true
	z.skillSit(q, true)
	if !p.rogue.gangster || !q.rogue.gangster {
		t.Fatal("two sitters are not protected")
	}
	m := monsterOf(z, mobGolem)
	m.target = p.entity
	z.fight()
	if m.target != nil {
		t.Error("the monster kept its target")
	}
	q.rogue.sitting = false
	z.skillSit(q, false)
	if p.rogue.gangster {
		t.Error("the protection stayed after one stood up")
	}
}

// TestCompulsionDiscount checks pc_modifybuyvalue: Compulsion Discount's
// 5% + 4% per level, when it beats Discount.
func TestCompulsionDiscount(t *testing.T) {
	if got := buyPrice(map[int]int{skill.IDCompulsion: 5}, 1000); got != 750 {
		t.Errorf("Compulsion Discount 5: %d, want 750", got)
	}
	if got := buyPrice(map[int]int{skill.IDCompulsion: 1, skill.IDDiscount: 10}, 1000); got != 760 {
		t.Errorf("Discount 10 over Compulsion Discount 1: %d, want 760", got)
	}
}

// TestPlagiarism checks skill_do_copy, pc_skill_plagiarism_reset and
// the load of pc_reg_received: a skill that hits the player is copied
// at most at Plagiarism's level, shows in the skill list, is saved, is
// kept from change by Preserve, and goes with a job change.
func TestPlagiarism(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobStalker, map[int]int{skill.IDPlagiarism: 5, skill.IDPreserve: 1}, itemKnife, mobGolem)
	m := monsterOf(z, mobGolem)
	p.entity.hp = 1 << 40
	bash := z.Skills.Get(skill.IDBash)
	z.skillAttack(battle.TypeWeapon, m, m, p.entity, bash, 10, 0)
	if p.rogue.cloneID != skill.IDBash || p.skillLevel(skill.IDBash) != 5 {
		t.Fatalf("copied %d at level %d, want Bash at 5", p.rogue.cloneID, p.skillLevel(skill.IDBash))
	}
	if p.vars[varPlagiarism] != "5" || p.vars[varPlagiarismLv] != "5" {
		t.Errorf("variables %v", p.vars)
	}
	listed := false
	for _, s := range z.skillList(p).GetSkillList().GetSkills() {
		if s.GetSkillId() == skill.IDBash && s.GetLevel() == 5 {
			listed = true
		}
	}
	if !listed {
		t.Error("Bash is not in the skill list")
	}
	z.scStart(p.entity, p.entity, sc.SCPreserve, 100, 1, 600000)
	z.skillAttack(battle.TypeWeapon, m, m, p.entity, z.Skills.Get(skill.IDMammonite), 5, 0)
	if p.rogue.cloneID != skill.IDBash {
		t.Error("Preserve did not keep Bash")
	}
	z.scEnd(p.entity, sc.SCPreserve)
	z.skillAttack(battle.TypeWeapon, m, m, p.entity, z.Skills.Get(skill.IDMammonite), 3, 0)
	if p.rogue.cloneID != skill.IDMammonite || p.skillLevel(skill.IDBash) != 0 || p.skillLevel(skill.IDMammonite) != 3 {
		t.Errorf("copied %d at %d; Bash %d", p.rogue.cloneID, p.rogue.cloneLv, p.skillLevel(skill.IDBash))
	}
	// pc_reg_received gives it back, at most at Plagiarism's level.
	saved := map[character.Variable]string{varPlagiarism: "5", varPlagiarismLv: "9"}
	p.rogue = rogueState{}
	z.loadPlagiarism(p, saved)
	if p.rogue.cloneID != skill.IDBash || p.rogue.cloneLv != 5 {
		t.Errorf("loaded %d at %d", p.rogue.cloneID, p.rogue.cloneLv)
	}
	z.jobChange(p, jobRogue, 0, 0)
	if p.rogue.cloneID != 0 || p.vars[varPlagiarism] != "" {
		t.Error("the job change kept the copy")
	}
}

// TestPlagiarismSkipsOwnSkills checks skill_isCopyable: a skill the
// player has is not copied.
func TestPlagiarismSkipsOwnSkills(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobRogue, map[int]int{skill.IDPlagiarism: 10, skill.IDBackStab: 1}, itemKnife, mobGolem)
	m := monsterOf(z, mobGolem)
	p.entity.hp = 1 << 40
	z.skillAttack(battle.TypeWeapon, m, m, p.entity, z.Skills.Get(skill.IDBackStab), 5, 0)
	if p.rogue.cloneID != 0 {
		t.Errorf("copied %d", p.rogue.cloneID)
	}
}

// TestChaseWalk checks SkillStealth and SC_CHASEWALK: the option, the
// block on other skills and on attacks, the SP drain and STR bonus of
// status_change_timer, and the toggle that ends it at no cost.
func TestChaseWalk(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobStalker, map[int]int{skill.IDChaseWalk: 5, skill.IDRejectSword: 5}, itemKnife, mobGolem)
	cast(z, p, skill.IDChaseWalk, 5, nil)
	run(z, p, 100, 2000)
	e := p.entity
	if !e.sc.Has(sc.SCChasewalk) || e.sc.Option&optionChasewalk == 0 {
		t.Fatalf("Chase Walk %v, option %#x", e.sc.Has(sc.SCChasewalk), e.sc.Option)
	}
	if z.statusCheckSkillUse(e, e, z.Skills.Get(skill.IDRejectSword), 0) {
		t.Error("another skill may start in Chase Walk")
	}
	if z.canTarget(e, monsterOf(z, mobGolem)) {
		t.Error("the player may attack in Chase Walk")
	}
	str, sp := e.status.Str, p.sp
	run(z, p, 2050, 12000)
	if p.sp != sp-20 || e.status.Str != str+16 || !e.sc.Has(sc.SCChasewalk2) {
		t.Errorf("SP %d of %d, STR %d of %d", p.sp, sp, e.status.Str, str)
	}
	sp = p.sp
	cast(z, p, skill.IDChaseWalk, 5, nil)
	run(z, p, 12050, 14000)
	if e.sc.Has(sc.SCChasewalk) || p.sp != sp {
		t.Errorf("toggle: still on %v, SP %d of %d", e.sc.Has(sc.SCChasewalk), p.sp, sp)
	}
}

// TestChaseWalkEndsAtOncePreRenewal checks unit_skilluse_id2 without
// RENEWAL_CAST: ending Chase Walk has no cast time.
func TestChaseWalkEndsAtOncePreRenewal(t *testing.T) {
	z, p := rogueZone(t, reference.PreRenewal, jobStalker, map[int]int{skill.IDChaseWalk: 5}, itemKnife)
	z.scStart(p.entity, p.entity, sc.SCChasewalk, 100, 5, 10000)
	cast(z, p, skill.IDChaseWalk, 5, nil)
	if p.entity.sc.Has(sc.SCChasewalk) {
		t.Error("ending Chase Walk waited for a cast time")
	}
}

// TestRejectSwordReflects checks battle_calc_weapon_final_atk_modifiers
// for Reject Sword: a monster's hit is halved, the attacker takes the
// half, and three reflections end the status.
func TestRejectSwordReflects(t *testing.T) {
	z, p := rogueZone(t, reference.Renewal, jobStalker, map[int]int{skill.IDRejectSword: 5}, itemKnife, mobGolem)
	m := monsterOf(z, mobGolem)
	e := p.entity
	e.hp = 1 << 40
	// The lowest roll is always a lucky dodge with perfect dodge.
	e.status.Flee2 = 0
	full := battle.Attack(m.status, z.defender(e), z.rand).Damage
	if full < 2 {
		t.Fatalf("the Golem deals %d", full)
	}
	z.scStart(e, e, sc.SCRejectsword, 100, 5, 300000)
	for i := 1; i <= 3; i++ {
		hp, mhp := e.hp, m.hp
		z.strike(m, e)
		if e.hp != hp-full/2 || m.hp != mhp-full/2 {
			t.Fatalf("hit %d: player took %d, monster took %d, want %d each", i, hp-e.hp, mhp-m.hp, full/2)
		}
	}
	if e.sc.Has(sc.SCRejectsword) {
		t.Error("Reject Sword lasted past three reflections")
	}
}
