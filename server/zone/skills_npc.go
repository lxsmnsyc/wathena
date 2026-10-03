package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Monster skills that only monsters use: the NPC_ skills of rAthena's
// src/map/skills/npc. This file holds the ones that summon, change or
// move the monster. Their entries' values (summons, emotion) come from
// the monster's skills list.

func init() {
	registerSkillImpls(map[int]any{
		mob.IDEmotion:        npcEmotionSkill{},
		mob.IDEmotionOn:      npcEmotionSkill{},
		mob.IDSummonSlave:    npcSummon{},
		mob.IDSummonMonster:  npcSummon{},
		mob.IDDeathSummon:    npcSummon{},
		mob.IDCallSlave:      npcCallSlave{},
		mob.IDMetamorphosis:  npcClassChange{},
		mob.IDTransformation: npcClassChange{},
		mob.IDRandomMove:     npcRandomMove{},
		mob.IDSpeedUp:        npcSpeedUp{},
		idRebirth:            npcRebirth{},
	})
}

// idRebirth is NPC_REBIRTH.
const idRebirth = 208

// mobEntry is the skills entry a monster is using, as md->db->skill
// [md->skill_idx], or nil.
func mobEntry(e *entity) *mob.Skill {
	m := e.monster
	if m == nil || m.skillIdx < 0 || m.skillIdx >= len(m.spawn.Skills) {
		return nil
	}
	return &m.spawn.Skills[m.skillIdx]
}

// npcEmotionSkill is SkillEmotion and SkillEmotionOn: the entry's
// emotions, which mobCastEnd shows, and its mode change through
// SC_MODECHANGE. npc_emotion_behavior is 0.
type npcEmotionSkill struct{}

func (npcEmotionSkill) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, _ *int) {
	s := mobEntry(src)
	if s == nil {
		return
	}
	set, add, del, end := s.ModeChange()
	if end {
		z.scEnd(src, d.Status)
	}
	m := src.monster
	// A monster whose mode NPC_EMOTION sets drops its target.
	if s.ID == mob.IDEmotion && m.ai.State != mob.StateIdle && m.ai.State != mob.StateWalk && set != 0 {
		z.mobUnlockTarget(src)
	}
	if set != 0 || add != 0 {
		z.scStart4(src, src, d.Status, 100, lv, int(set), int(add), int(del), int64(d.Duration1.At(lv)))
	}
	if src.status != nil {
		m.ai.Aggressive = src.status.Mode&mob.ModeAngry != 0
	}
}

// npcSummon is SkillFollowerSummons, SkillMonsterSummons and
// SkillDeathSummon: mob_summonslave with the entry's summons.
type npcSummon struct{}

func (npcSummon) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, _ *int) {
	if s := mobEntry(src); s != nil {
		z.mobSummonSlave(src, s.Summons, lv, d.ID)
	}
}

// mobSummonSlave is mob_summonslave: amount monsters of the list, in
// turn, near the summoner. A random start in the list keeps the total
// the same. NPC_SUMMONSLAVE makes them slaves that follow the summoner.
func (z *Zone) mobSummonSlave(src *entity, list []uint32, amount, skillID int) {
	if len(list) == 0 || z.Mods == nil {
		return
	}
	count := len(list)
	k := 0
	if amount > 0 && amount < count {
		k = z.rand.IntN(count)
		amount += k
	}
	for ; k < amount; k++ {
		id := list[k%count]
		tmpl, ok := z.Mods.Monsters[id]
		if !ok {
			continue
		}
		x, y := src.x, src.y
		if skillID != mob.IDDeathSummon {
			if fx, fy, ok := z.searchFreeCell(src.x, src.y, slaveDistance, slaveDistance); ok {
				x, y = fx, fy
			}
		}
		sp := tmpl
		sp.Amount, sp.noRespawn, sp.owner = 1, true, 0
		sp.X, sp.Y, sp.W, sp.H = x, y, 0, 0
		e := z.spawnSummon(&sp, x, y, func(e *entity) {
			if skillID == mob.IDSummonSlave {
				e.monster.master = src
				e.monster.specialAI = src.monster.specialAI
			}
		})
		if e == nil {
			continue
		}
		if skillID == mob.IDSummonSlave {
			// status_calc_slave_mode (slaves_inherit_mode 4): a slave
			// attacks first when its master does.
			z.slaveMode(e, src)
		}
		if d := z.skillsGet(skillID); d != nil {
			z.skillEffect(e, e, d, amount, int64(amount), true)
		}
	}
}

// spawnSummon places one summoned monster on a cell, with setup run on
// the entity before its AI starts.
func (z *Zone) spawnSummon(sp *Spawn, x, y int, setup func(*entity)) *entity {
	speed := sp.Speed
	if speed <= 0 {
		speed = WalkSpeed
	}
	e := &entity{monster: &monster{spawn: sp, nextWalk: -1}, x: x, y: y, dir: z.rand.IntN(8), speed: speed}
	if sp.Status != nil {
		st := *sp.Status
		e.status, e.hp = &st, int64(st.MaxHP)
	}
	if setup != nil {
		setup(e)
	}
	z.entities.add(e)
	z.monsterSpawned(e)
	return e
}

// slaveMode is status_calc_slave_mode with slaves_inherit_mode 4: a slave
// of an aggressive master is aggressive, and a slave of a passive master
// is passive, unless the slave was aggressive by its own mode.
func (z *Zone) slaveMode(e, master *entity) {
	if master.status == nil || e.status == nil || e.monster.spawn.Status == nil {
		return
	}
	own := e.monster.spawn.Status.Mode
	if own&mob.ModeAggressive != 0 {
		return
	}
	if master.status.Mode&mob.ModeAggressive != 0 {
		e.status.Mode |= mob.ModeAggressive
	} else {
		e.status.Mode &^= mob.ModeAggressive
	}
}

// npcCallSlave is SkillRecallSlaves: mob_warpslave brings the slaves next
// to their master.
type npcCallSlave struct{}

func (npcCallSlave) castendNoDamage(z *Zone, src, _ *entity, _ *skill.Def, _ int, _ *int) {
	for o := range z.entities.all() {
		if m := o.monster; m != nil && m.master == src && !o.dead {
			x, y, _ := z.searchFreeCell(src.x, src.y, slaveDistance, slaveDistance)
			z.moveTo(o, x, y)
		}
	}
}

// npcClassChange is SkillMetamorphosis and SkillTransformation: the
// monster becomes one of its summons, and summons the rest of the level.
type npcClassChange struct{}

func (npcClassChange) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, _ *int) {
	s := mobEntry(src)
	if s == nil || len(s.Summons) == 0 {
		return
	}
	// mob_random_class.
	class := s.Summons[z.rand.IntN(len(s.Summons))]
	if lv > 1 {
		z.mobSummonSlave(src, s.Summons, lv-1, d.ID)
	}
	z.mobClassChange(src, class)
}

// mobClassChange is mob_class_change: the monster becomes another kind,
// with full HP and a new damage log (monster_class_change_full_recover),
// and comes back as its own kind.
func (z *Zone) mobClassChange(e *entity, class uint32) {
	m := e.monster
	if m == nil || z.Mods == nil || m.spawn.Mob == class || m.specialAI > aiAttack {
		return
	}
	tmpl, ok := z.Mods.Monsters[class]
	if !ok || tmpl.Status == nil {
		return
	}
	for _, g := range e.status.Race2 {
		if g == race2Treasure {
			return
		}
	}
	sp := tmpl
	old := m.spawn
	sp.Amount, sp.RespawnMs, sp.noRespawn, sp.owner = 1, old.RespawnMs, old.noRespawn, old.owner
	sp.X, sp.Y, sp.W, sp.H = old.X, old.Y, old.W, old.H
	if m.respawnAs == nil {
		m.respawnAs = old
	}
	m.spawn = &sp
	z.scEnd(e, sc.SCKeeping)
	z.scEnd(e, sc.SCBarrier)
	e.target = nil
	z.stopWalking(e)
	z.castCancel(e, false)
	st := *sp.Status
	e.status = &st
	z.refreshChanges(e)
	e.speed = st.Speed
	e.hp = int64(st.MaxHP)
	e.hpChanged = true
	m.dmglog.Entries = nil
	m.skillDelay = make([]int64, len(sp.Skills))
	if st.Mode&mob.ModeLooter != 0 {
		m.canLoot = true
	}
	// clif_class_change: viewers see the new kind. The web client keeps
	// a monster's sprite on LookChange, so the monster appears again.
	for o := range z.entities.all() {
		if o.player == nil {
			continue
		}
		if id, ok := o.player.netIDs.get(e); ok {
			o.player.netIDs.del(e)
			z.queue(o.player, frame(z.tick, o.player.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT)))
		}
	}
	m.target, m.attacked, m.normAttacked = nil, nil, nil
}

// race2Treasure is RC2_TREASURE: treasure boxes do not change.
const race2Treasure = 9

// npcRandomMove is SkillRandomMove: a fake cast bar while the monster
// runs up to 8 cells from its target.
type npcRandomMove struct{}

func (npcRandomMove) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	m := src.monster
	if m == nil {
		return
	}
	const trick = mobSkillInterval * 3
	m.trickcasting = z.now + trick
	z.skillEvents = append(z.skillEvents, skillEvent{kind: skillEventCast, src: src, target: src, skill: d, lv: lv,
		castMs: trick + mobSkillInterval/2})
	z.unitEscape(src, target, 8)
}

// unitEscape is unit_escape with flag 3: the unit runs up to rng cells
// away from another, on a cell it can walk to.
func (z *Zone) unitEscape(e, from *entity, rng int) {
	dir := mapCalcDir(from, e.x, e.y)
	if from == e {
		dir = z.rand.IntN(8)
	}
	for ; rng > 0; rng-- {
		x, y := e.x+dirX[dir]*rng, e.y+dirY[dir]*rng
		if z.grid.NoPass(x, y) {
			continue
		}
		if steps, ok := path.Search(z.grid, e.x, e.y, x, y, false); ok && len(steps) > 0 {
			e.target = nil
			e.path = steps
			e.stepStart = z.now
			e.walkChanged = true
			if e.monster != nil {
				e.monster.walked = 0
			}
			return
		}
	}
}

// npcSpeedUp is SkillSpeedUp: while the fake cast lasts, the monster
// gets faster, and at its end it stops.
type npcSpeedUp struct{}

func (npcSpeedUp) castendNoDamage(z *Zone, src, _ *entity, _ *skill.Def, _ int, _ *int) {
	m := src.monster
	if m == nil {
		return
	}
	const stop = mobSkillInterval * 7 / 10
	if m.trickcasting-z.now >= stop {
		src.speed = max(src.speed-250, 20)
		src.status.Speed = src.speed
		if m.trickcasting-z.now < stop+mobSkillInterval {
			m.lastSkillCheck = z.now + 100
		}
		return
	}
	m.lastSkillCheck = m.trickcasting
	m.trickcasting = 0
}

// npcRebirth is SkillRebirth: the monster gets SC_REBIRTH, which brings
// it back once when it dies.
type npcRebirth struct{}

func (npcRebirth) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if m := src.monster; m != nil && m.reborn {
		return
	}
	z.scStart(src, target, d.Status, 100, lv, sc.InfiniteTick)
}

// skillsGet is a skill of the zone's database, or nil without one.
func (z *Zone) skillsGet(id int) *skill.Def {
	if z.Skills == nil {
		return nil
	}
	return z.Skills.Get(id)
}
