package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Statuses the tests read, which the zone does not name.
const (
	buffSCArmor       = 104 // SC_ARMOR
	buffSCArmorchange = 252 // SC_ARMORCHANGE
	buffSCPowerup     = 951 // SC_POWERUP
	buffSCAgiup       = 952 // SC_AGIUP
	buffBfLong        = 0x0040
)

// npcSelf casts a monster self skill on the monster, as the cast end of
// mobskill_use does.
func npcSelf(z *Zone, m *entity, id, lv int) {
	z.castendNoDamage(m, m, z.Skills.Get(id), lv, 0)
}

// TestNpcSelfDestruction checks SkillSuicideBombing and
// battle_calc_misc_attack's NPC_SELFDESTRUCTION: every character in the
// splash, monsters too (BCT_ALL), takes the caster's HP as fire damage
// that ignores DEF, and the caster dies.
func TestNpcSelfDestruction(t *testing.T) {
	z, p := aiZone(t)
	z.rand = rng.New(forcedMinSource{})
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	near := aiMob(t, z, 1113, 24, 20, mob.AITypes[1], nil)
	far := aiMob(t, z, 1113, 30, 20, mob.AITypes[1], nil)
	sturdy(near)
	sturdy(p.entity)
	m.hp = 40
	pHP, nearHP, farHP := p.entity.hp, near.hp, far.hp
	npcSelf(z, m, idNpcSelfDestruction, 1)
	if lost := pHP - p.entity.hp; lost != 40 {
		t.Errorf("the player lost %d HP, want the caster's 40", lost)
	}
	if nearHP-near.hp <= 0 {
		t.Error("the monster in the splash took no damage")
	}
	if far.hp != farHP {
		t.Error("the monster outside the splash took damage")
	}
	if !m.dead || z.entities.has(m) {
		t.Error("the caster lives")
	}
}

// TestNpcSuicideGivesNothing checks SkillNpcSuicide and status_kill: the
// monster dies, and mob_dead's type 3 gives no exp.
func TestNpcSuicideGivesNothing(t *testing.T) {
	z, p := aiZone(t)
	z.Progress = testTables()
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	m.monster.spawn.Reward = &progression.Mob{ID: 1002, Level: 1, MaxHP: m.status.MaxHP, BaseExp: 15, JobExp: 25}
	m.monster.dmglog.Add(p.charID, 10, 0)
	before := p.progress.BaseExp
	npcSelf(z, m, idNpcSuicide, 1)
	if !m.dead {
		t.Fatal("the monster lives")
	}
	if p.progress.BaseExp != before {
		t.Error("the suicide gave exp")
	}
}

// TestNpcKeepingAndBarrier checks status_change_start's and
// status_change_end's case for SC_KEEPING and SC_BARRIER, status_calc_def's
// SC_KEEPING, and battle_calc_damage's SC_BARRIER.
func TestNpcKeepingAndBarrier(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 21, 20, mob.AITypes[1], nil)
	sturdy(m)
	npcSelf(z, m, idNpcKeeping, 1)
	if !m.sc.Has(sc.SCKeeping) {
		t.Fatal("no SC_KEEPING")
	}
	end := z.now + 30000
	if m.canAct != end || m.canMove != end || m.nextAttack != end || m.endure != end {
		t.Errorf("act %d move %d attack %d endure %d, want %d", m.canAct, m.canMove, m.nextAttack, m.endure, end)
	}
	if m.status.Def != 90 {
		t.Errorf("DEF %d under SC_KEEPING, want 90", m.status.Def)
	}
	z.scEnd(m, sc.SCKeeping)
	if m.canAct != z.now || m.canMove != z.now {
		t.Error("the unit still cannot act after SC_KEEPING")
	}
	npcSelf(z, m, idNpcBarrier, 1)
	bash := z.Skills.Get(skill.IDBash)
	if dmg := z.skillAttack(bash.Type, p.entity, p.entity, m, bash, 10, 0); dmg != 1 {
		t.Errorf("Bash dealt %d through SC_BARRIER, want 1", dmg)
	}
}

// TestNpcArmorChange checks status_change_start's SC_ARMORCHANGE: 20% per
// level, levels 6 to 10 as 1 to 5, and the sign by skill.
func TestNpcArmorChange(t *testing.T) {
	z, _ := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	npcSelf(z, m, idNpcStoneSkin, 7)
	if e := m.sc.Get(buffSCArmorchange); e == nil || e.Val1 != 2 || e.Val2 != 40 || e.Val3 != -40 {
		t.Fatalf("NPC_STONESKIN 7: %+v", e)
	}
	if c := m.status.Changes; c == nil || c.ArmorChangeWeapon != 40 || c.ArmorChangeMagic != -40 {
		t.Errorf("battle changes %+v", c)
	}
	z.scEnd(m, buffSCArmorchange)
	npcSelf(z, m, idNpcAntiMagic, 3)
	if e := m.sc.Get(buffSCArmorchange); e == nil || e.Val2 != -60 || e.Val3 != 60 {
		t.Fatalf("NPC_ANTIMAGIC 3: %+v", e)
	}
}

// TestNpcDefender checks status_change_start's SC_ARMOR, which divides
// ranged weapon and misc hits by 8, and status_calc_speed's SC_ARMOR.
func TestNpcDefender(t *testing.T) {
	z, _ := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	npcSelf(z, m, idNpcDefender, 1)
	e := m.sc.Get(buffSCArmor)
	if e == nil || e.Val2 != 8 || e.Val3 != buffBfLong || e.Val4 != battle.FlagWeapon|battle.FlagMisc {
		t.Fatalf("SC_ARMOR %+v", e)
	}
	if m.status.Speed < 200 {
		t.Errorf("speed %d, want 200 or slower", m.status.Speed)
	}
}

// TestNpcPowerUpAndAgiUp checks SkillPowerUp's and SkillAgilityUp's values,
// and their parts of status_calc_hit, status_calc_flee and
// battle_get_atkpercent.
func TestNpcPowerUpAndAgiUp(t *testing.T) {
	z, _ := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	hit, flee := m.status.Hit, m.status.Flee
	npcSelf(z, m, idNpcPowerUp, 1)
	if e := m.sc.Get(buffSCPowerup); e == nil || e.Val1 != 200 || e.Val2 != 100 {
		t.Fatalf("SC_POWERUP %+v", e)
	}
	if m.status.Hit != hit*2 {
		t.Errorf("hit %d, want %d", m.status.Hit, hit*2)
	}
	if c := m.status.Changes; c == nil || c.AtkPercent != 300 {
		t.Errorf("ATK percent %+v, want 300", c)
	}
	npcSelf(z, m, idNpcAgiUp, 1)
	if e := m.sc.Get(buffSCAgiup); e == nil || e.Val1 != 50 || e.Val2 != 100 {
		t.Fatalf("SC_AGIUP %+v", e)
	}
	if m.status.Flee != flee*2 {
		t.Errorf("flee %d, want %d", m.status.Flee, flee*2)
	}
}

// TestNpcInvincible checks status_change_start's SC_INVINCIBLE and
// is_infinite_defense: every hit deals 1.
func TestNpcInvincible(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 21, 20, mob.AITypes[1], nil)
	sturdy(m)
	npcSelf(z, m, idNpcInvincible, 1)
	if e := m.sc.Get(sc.SCInvincible); e == nil || e.Val2 != 100 || e.Val3 != 50 || e.Val4 != 700 {
		t.Fatalf("SC_INVINCIBLE %+v", e)
	}
	bash := z.Skills.Get(skill.IDBash)
	z.rand = rng.New(forcedMinSource{})
	if dmg := z.skillAttack(bash.Type, p.entity, p.entity, m, bash, 10, 0); dmg != 1 {
		t.Errorf("Bash dealt %d, want 1", dmg)
	}
}

// TestNpcMillenniumShield checks status_change_start's
// SC_MILLENNIUMSHIELD and battle_status_block_damage: shields take hits,
// and a broken shield stuns.
func TestNpcMillenniumShield(t *testing.T) {
	z, _ := aiZone(t)
	z.rand = rng.New(forcedMinSource{})
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	npcSelf(z, m, idNpcMillenniumShield, 1)
	e := m.sc.Get(scMillenniumshield)
	if e == nil || e.Val2 != 4 || e.Val3 != 1000 {
		t.Fatalf("SC_MILLENNIUMSHIELD %+v, want 4 shields of 1000", e)
	}
	if !z.blockDamage(nil, m, battle.FlagWeapon, 0, 0, 600) || e.Val3 != 400 {
		t.Fatalf("shield HP %d after 600, want 400", e.Val3)
	}
	if !z.blockDamage(nil, m, battle.FlagWeapon, 0, 0, 600) || e.Val2 != 3 || e.Val3 != 1000 {
		t.Errorf("%d shields of %d after a break, want 3 of 1000", e.Val2, e.Val3)
	}
	if !m.sc.Has(sc.SCStun) {
		t.Error("a broken shield did not stun")
	}
}

// TestNpcDamageHeal checks status_change_start's SC_DAMAGE_HEAL and
// battle_status_block_damage: hits of the level's type heal.
func TestNpcDamageHeal(t *testing.T) {
	z, _ := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	npcSelf(z, m, idNpcDamageHeal, 2)
	if e := m.sc.Get(scDamageHeal); e == nil || e.Val2 != battle.FlagMagic {
		t.Fatalf("SC_DAMAGE_HEAL 2: %+v", e)
	}
	m.hp = 10
	if z.blockDamage(nil, m, battle.FlagWeapon, 0, 0, 5) {
		t.Error("a weapon hit healed at level 2")
	}
	if !z.blockDamage(nil, m, battle.FlagMagic, 0, 0, 5) || m.hp != 15 {
		t.Errorf("HP %d after a magic hit of 5, want 15", m.hp)
	}
}

// TestNpcHeals checks SkillFullHeal, which also clears the damage log,
// and SkillNpcColuceoHeal, which heals the caster's side in its splash.
func TestNpcHeals(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	ally := aiMob(t, z, 1113, 23, 20, mob.AITypes[1], nil)
	m.hp = 1
	m.monster.dmglog.Add(p.charID, 10, 0)
	npcSelf(z, m, idNpcAllHeal, 1)
	if m.hp != int64(m.status.MaxHP) || len(m.monster.dmglog.Entries) != 0 {
		t.Errorf("HP %d of %d, %d log entries", m.hp, m.status.MaxHP, len(m.monster.dmglog.Entries))
	}
	m.hp, ally.hp = 1, 1
	p.entity.hp = 1
	npcSelf(z, m, idNpcCHeal, 5)
	if m.hp <= 1 || ally.hp <= 1 {
		t.Errorf("caster HP %d, ally HP %d after NPC_CHEAL", m.hp, ally.hp)
	}
	if p.entity.hp != 1 {
		t.Error("NPC_CHEAL healed the player")
	}
}

// TestNpcImmuneProperty checks SkillPropertyImmune's statuses by level.
func TestNpcImmuneProperty(t *testing.T) {
	z, _ := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	npcSelf(z, m, idNpcImmuneProperty, 6)
	if !m.sc.Has(924) {
		t.Error("level 6 did not give SC_IMMUNE_PROPERTY_DARKNESS")
	}
}

// TestNpcMoveCoordinate checks SkillChangeLocation: the caster and its
// target swap cells, and a boss stays.
func TestNpcMoveCoordinate(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	px, py := p.entity.x, p.entity.y
	z.castendNoDamage(m, p.entity, z.Skills.Get(idNpcMoveCoordinate), 1, 0)
	if p.entity.x != 22 || p.entity.y != 20 || m.x != px || m.y != py {
		t.Fatalf("player at %d,%d and monster at %d,%d", p.entity.x, p.entity.y, m.x, m.y)
	}
	m.status.Class = battle.ClassBoss
	z.castendNoDamage(m, p.entity, z.Skills.Get(idNpcMoveCoordinate), 1, 0)
	if p.entity.x != px || m.x != px {
		t.Errorf("player at %d and boss at %d, want both at %d", p.entity.x, m.x, px)
	}
}

// TestNpcRun checks SkillNpcRun: the monster drops its target, walks away
// from it, and does not think until it arrives.
func TestNpcRun(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	m.monster.target = p.entity
	npcSelf(z, m, idNpcRun, 1)
	if m.monster.target != nil || len(m.path) == 0 {
		t.Fatalf("target %v, %d steps", m.monster.target, len(m.path))
	}
	if m.monster.nextThink <= z.now {
		t.Error("the monster thinks while it runs")
	}
	x, y := m.x, m.y
	for _, d := range m.path {
		dx, dy := d.Step()
		x, y = x+dx, y+dy
	}
	if chebyshev(p.entity, &entity{x: x, y: y}) <= 2 {
		t.Errorf("it runs to %d,%d, near the player", x, y)
	}
}

// TestNpcVampireGift checks SkillVampireGift: the caster heals by the
// damage of each hit of its splash.
func TestNpcVampireGift(t *testing.T) {
	z, p := aiZone(t)
	z.rand = rng.New(forcedMinSource{})
	m := aiMob(t, z, 1002, 21, 20, mob.AITypes[1], nil)
	sturdy(p.entity)
	m.hp = 1
	pHP := p.entity.hp
	npcSelf(z, m, idNpcVampireGift, 1)
	lost := pHP - p.entity.hp
	if lost <= 0 || m.hp != min(1+lost, int64(m.status.MaxHP)) {
		t.Errorf("the player lost %d and the monster has %d HP", lost, m.hp)
	}
}

// TestNpcProvocationAndExpulsion checks SkillProvocation, which drops the
// monster's target, and SkillExpulsion, which warps the target to a
// random cell (unit_warp).
func TestNpcProvocationAndExpulsion(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	m.monster.target = p.entity
	npcSelf(z, m, idNpcProvocation, 1)
	if m.monster.target != nil {
		t.Error("the monster keeps its target")
	}
	other := aiMob(t, z, 1113, 23, 20, mob.AITypes[1], nil)
	z.castendNoDamage(m, other, z.Skills.Get(idNpcExpulsion), 1, 0)
	if other.x == 23 && other.y == 20 {
		t.Error("the target did not move")
	}
}

// TestNpcDarkBlessingAndHellPower checks SkillDarkBlessing's SC_COMA,
// which leaves 1 HP, and SkillHellPower's SC_HELLPOWER for time2.
func TestNpcDarkBlessingAndHellPower(t *testing.T) {
	z, p := aiZone(t)
	z.rand = rng.New(forcedMinSource{})
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	z.castendNoDamage(m, p.entity, z.Skills.Get(idNpcDarkBlessing), 1, 0)
	if p.entity.hp != 1 {
		t.Errorf("HP %d after SC_COMA, want 1", p.entity.hp)
	}
	z.castendNoDamage(m, p.entity, z.Skills.Get(idNpcHellPower), 5, 0)
	e := p.entity.sc.Get(294)
	if e == nil || e.Remaining(z.now) != 180000 {
		t.Errorf("SC_HELLPOWER %+v, want 180 s", e)
	}
}
