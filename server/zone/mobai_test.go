package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// aiZone makes a renewal zone with the full data and a level 99 Knight
// at (20, 20), and no monsters. Rolls are seeded.
func aiZone(t *testing.T) (*Zone, *Player) {
	t.Helper()
	z := goldenZone(t, reference.Renewal)
	z.rand = rng.FromKey([32]byte{11}).Stream()
	p := goldenPlayerIn(t, z, goldenPlayer{Job: 7, Build: "melee", Sex: 1, JobLevel: 50}, 0, 0)
	z.Mods = &Mods{Monsters: map[uint32]Spawn{}}
	return z, p
}

// aiMob places a monster of the golden data on a cell, with an AI type's
// modes and a skills list. Its kind also becomes a template, for summons.
func aiMob(t *testing.T, z *Zone, id uint32, x, y int, mode uint32, skills []mob.Skill) *entity {
	t.Helper()
	g := loadGoldenData(t, reference.Renewal)
	st := *g.mobs[id]
	st.Mode = mode
	sp := Spawn{Mob: id, Name: "m", Amount: 1, X: x, Y: y, Speed: st.Speed, CanMove: mode&mob.ModeCanMove != 0,
		ViewRange: 10, ChaseRange: 12, Status: &st, Skills: skills, RespawnMs: 60000}
	z.Mods.Monsters[id] = sp
	sp2 := sp
	e := z.spawnOne(&sp2)
	if e == nil {
		t.Fatal("no cell")
	}
	e.x, e.y = x, y
	return e
}

// steps runs the zone in 50 ms ticks from z.now to the time, and stops
// early when done reports true.
func steps(z *Zone, p *Player, until int64, done func() bool) {
	for now := z.now + 50; now <= until; now += 50 {
		z.stepAt(now)
		events(p)
		if done != nil && done() {
			return
		}
	}
}

// TestAggressiveMonsterChasesAndAttacks checks mob_ai_sub_hard for an
// aggressive monster (AI 04): it sees the player within its view range,
// walks next to it and attacks.
func TestAggressiveMonsterChasesAndAttacks(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 27, 20, mob.AITypes[4], nil)
	hp := p.entity.hp
	steps(z, p, 10000, func() bool { return p.entity.hp < hp })
	if m.monster.target != p.entity {
		t.Fatalf("the monster did not take the player as its target: %v", m.monster.target)
	}
	if p.entity.hp >= hp {
		t.Fatal("the monster did not attack")
	}
	if d := chebyshev(m, p.entity); d > 1 {
		t.Errorf("it attacks from %d cells", d)
	}
	if s := m.monster.ai.State; s != mob.StateAngry && s != mob.StateBerserk {
		t.Errorf("state %s, want angry or attack", s)
	}
}

// TestPassiveMonsterIgnoresThenRetaliates checks that a passive monster
// (AI 01) does not attack first, and hits back once hit.
func TestPassiveMonsterIgnoresThenRetaliates(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	steps(z, p, 3000, nil)
	if m.monster.target != nil {
		t.Fatal("a passive monster took a target")
	}
	p.entity.target, p.entity.repeat = m, false
	sturdy(m)
	// The AI learns of the hit after its damage shows and the walk delay,
	// then attacks. The Knight dodges most of a Poring's attacks.
	steps(z, p, 5000, func() bool { return m.target == p.entity })
	if m.monster.target != p.entity || m.target != p.entity || m.nextAttack <= 0 {
		t.Fatalf("the monster did not hit back: AI target %v, attack target %v", m.monster.target, m.target)
	}
	if s := m.monster.ai.State; s != mob.StateBerserk {
		t.Errorf("state %s, want attack", s)
	}
}

// TestChaseGivesUp checks that a chasing monster drops a target past its
// chase range once it stops (status_check_visibility with range3).
func TestChaseGivesUp(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[4], nil)
	m.status.Mode &^= mob.ModeCanMove
	steps(z, p, 500, func() bool { return m.monster.target != nil })
	if m.monster.target != p.entity {
		t.Fatal("no target")
	}
	// The player steps away past the chase range of 12.
	p.entity.x = m.x + 13
	steps(z, p, z.now+3000, func() bool { return m.monster.target == nil })
	if m.monster.target != nil {
		t.Fatal("the monster kept a target out of its chase range")
	}
	if s := m.monster.ai.State; s != mob.StateIdle {
		t.Errorf("state %s, want idle", s)
	}
}

// TestAssist checks mob_linksearch: a monster that assists calls the
// monsters of its kind when it attacks.
func TestAssist(t *testing.T) {
	z, p := aiZone(t)
	// AI 04 with assist.
	mode := mob.AITypes[4] | mob.ModeAssist
	a := aiMob(t, z, 1002, 21, 20, mode, nil)
	b := aiMob(t, z, 1002, 21, 26, mode&^mob.ModeAggressive, nil)
	sturdy(a)
	steps(z, p, 5000, func() bool { return b.monster.target != nil })
	if a.monster.target != p.entity {
		t.Fatal("the first monster did not attack")
	}
	if b.monster.target != p.entity {
		t.Fatal("the second monster did not answer the call")
	}
}

// TestLooter checks the loot part of mob_ai_sub_hard: a looter walks to
// an item, takes it, and drops it when it dies.
func TestLooter(t *testing.T) {
	z, p := aiZone(t)
	p.entity.x, p.entity.y = 20, 20
	m := aiMob(t, z, 1002, 30, 30, mob.AITypes[2], nil)
	it := &entity{x: 33, y: 30, item: &groundItem{id: 909, amount: 1, expires: 1 << 40}}
	z.entities.add(it)
	steps(z, p, 15000, func() bool { return !z.entities.has(it) })
	if z.entities.has(it) {
		t.Fatal("the looter did not take the item")
	}
	if len(m.monster.loot) != 1 || m.monster.loot[0].id != 909 {
		t.Fatalf("loot %v", m.monster.loot)
	}
	z.Progress = testTables()
	m.monster.spawn.Reward = &progression.Mob{ID: 1002, Level: 1, MaxHP: m.status.MaxHP}
	m.hp = 0
	z.die(m, p.entity)
	if len(z.falling) != 1 || len(z.falling[0].items) != 1 || z.falling[0].items[0].id != 909 {
		t.Fatalf("drops %v", z.falling)
	}
}

// TestCastSensor checks the cast sensor of unit_skilluse_id2: an idle
// monster with cast_sensor_idle takes a player that casts on it.
func TestCastSensor(t *testing.T) {
	z, p := aiZone(t)
	p.char.Skills[skill.IDFireBolt] = 10
	m := aiMob(t, z, 1002, 28, 20, mob.ModeCanMove|mob.ModeCanAttack|mob.ModeCastSensorIdle, nil)
	sturdy(m)
	z.stepAt(z.now + 50)
	if !z.skillUseID(p.entity, m, z.Skills.Get(skill.IDFireBolt), 10) {
		t.Fatal("no cast")
	}
	if m.monster.target != p.entity {
		t.Fatal("the monster did not sense the cast")
	}
}

// TestMonsterSummonsSlaves checks NPC_SUMMONSLAVE through mobskill_use:
// the slaves come next to their master, follow it, and die with it.
func TestMonsterSummonsSlaves(t *testing.T) {
	z, p := aiZone(t)
	p.entity.x, p.entity.y = 30, 30
	summon := mob.Skill{ID: mob.IDSummonSlave, Level: 2, State: mob.StateIdle, Rate: 10000, Delay: 10000,
		Target: mob.TargetSelf, Condition: mob.CondSlavesBelow, Value: 1, Summons: []uint32{1113}, Emotion: -1}
	aiMob(t, z, 1113, 5, 5, mob.AITypes[1], nil)
	master := aiMob(t, z, 1002, 25, 25, mob.AITypes[1]&^mob.ModeCanMove, []mob.Skill{summon})
	steps(z, p, 3000, func() bool { return z.countSlaves(master) > 0 })
	if n := z.countSlaves(master); n != 2 {
		t.Fatalf("%d slaves, want 2", n)
	}
	for o := range z.entities.all() {
		if o.monster != nil && o.monster.master == master && chebyshev(o, master) > slaveDistance {
			t.Errorf("a slave is %d cells away", chebyshev(o, master))
		}
	}
	// Its delay keeps it from summoning again.
	steps(z, p, z.now+3000, nil)
	if n := z.countSlaves(master); n != 2 {
		t.Errorf("%d slaves after the delay check", n)
	}
	master.hp = 0
	z.die(master, p.entity)
	if n := z.countSlaves(master); n != 0 {
		t.Errorf("%d slaves after the master died", n)
	}
}

// TestMonsterHealsBelowHP checks hp_below: a monster heals itself when
// its HP falls to the condition's percent.
func TestMonsterHealsBelowHP(t *testing.T) {
	z, p := aiZone(t)
	p.entity.x, p.entity.y = 30, 30
	heal := mob.Skill{ID: skill.IDHeal, Level: 9, State: mob.StateAny, Rate: 10000, Delay: 5000,
		Target: mob.TargetSelf, Condition: mob.CondHPBelow, Value: 50, Emotion: -1}
	m := aiMob(t, z, 1002, 25, 25, mob.AITypes[1], []mob.Skill{heal})
	sturdy(m)
	steps(z, p, 2000, nil)
	if m.hp != int64(m.status.MaxHP) {
		t.Fatal("it healed at full HP")
	}
	m.hp = int64(m.status.MaxHP) / 3
	low := m.hp
	steps(z, p, z.now+2000, func() bool { return m.hp > low })
	if m.hp <= low {
		t.Fatal("no heal below 50%")
	}
}

// TestMVPReward checks the MVP part of mob_dead: the player with the most
// damage gets the MVP exp and the first MVP drop that rolls.
func TestMVPReward(t *testing.T) {
	z, p := aiZone(t)
	z.Progress = testTables()
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1]|mob.ModeMvp, nil)
	m.monster.spawn.Reward = &progression.Mob{ID: 1002, Level: 1, MaxHP: m.status.MaxHP, Mode: m.status.Mode,
		MvpExp: 30, MvpDrops: []progression.Drop{{Item: 909, Rate: 10000}}}
	m.monster.dmglog.Add(p.charID, 100, 0)
	before := p.progress.BaseExp
	m.hp = 0
	z.die(m, p.entity)
	if p.progress.BaseExp <= before {
		t.Error("no MVP exp")
	}
	if z.countItem(p, 909) != 1 {
		t.Error("no MVP prize")
	}
	if len(z.effects) == 0 || z.effects[len(z.effects)-1].id != effectMVP {
		t.Error("no MVP effect")
	}
}

// TestMonsterRebirth checks NPC_REBIRTH: SC_REBIRTH brings the monster back
// once, with 20% HP per level.
func TestMonsterRebirth(t *testing.T) {
	z, p := aiZone(t)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	z.castendNoDamage(m, m, z.Skills.Get(idRebirth), 2, 0)
	if !m.sc.Has(sc.SCRebirth) {
		t.Fatal("no SC_REBIRTH")
	}
	m.hp = 0
	z.die(m, p.entity)
	if m.dead || !z.entities.has(m) || m.hp != int64(m.status.MaxHP)*40/100 {
		t.Fatalf("after the first death: dead %v, HP %d of %d", m.dead, m.hp, m.status.MaxHP)
	}
	m.hp = 0
	z.die(m, p.entity)
	if !m.dead || z.entities.has(m) {
		t.Fatal("it came back twice")
	}
}

// TestClassChange checks NPC_METAMORPHOSIS: the monster becomes its
// summon, with full HP, and comes back as its own kind.
func TestClassChange(t *testing.T) {
	z, p := aiZone(t)
	aiMob(t, z, 1113, 5, 5, mob.AITypes[1], nil)
	m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
	m.hp = int64(m.status.MaxHP) / 2
	before := hpPercent(m)
	orig := m.monster.spawn
	m.monster.spawn.Skills = []mob.Skill{{ID: mob.IDMetamorphosis, Level: 1, Summons: []uint32{1113}, Emotion: -1}}
	m.monster.skillIdx = 0
	z.castendNoDamage(m, m, z.Skills.Get(mob.IDMetamorphosis), 1, 0)
	if m.monster.spawn.Mob != 1113 {
		t.Fatalf("class %d", m.monster.spawn.Mob)
	}
	// monster_class_change_full_recover: the new kind has full HP.
	if m.hp != int64(m.status.MaxHP) || before >= 100 {
		t.Errorf("HP %d of %d", m.hp, m.status.MaxHP)
	}
	m.hp = 0
	z.die(m, p.entity)
	if len(z.respawns) != 1 || z.respawns[0].spawn != orig {
		t.Error("it does not come back as its own kind")
	}
}
