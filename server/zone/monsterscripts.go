package zone

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// The monster side of the Lua API (proposal 0002, review decision 22):
// monster handles' fields and methods, the monster_target hook, and the
// AI scripts of monster records.

// handleKey is a handle key, "<kind>:<id>", of an entity, or nil for one
// scripts cannot hold, such as an item on the ground.
func (z *Zone) handleKey(e *entity) script.Value {
	if e == nil || (e.player == nil && e.monster == nil && e.npc == nil) {
		return script.Nil
	}
	r := z.ref(e)
	return script.String(fmt.Sprintf("%s:%d", r.Kind, r.ID))
}

func init() {
	monsterFields["target"] = func(h scriptHost, e *entity) script.Value {
		t := e.monster.target
		if t == nil || t.item != nil || !h.z.entities.has(t) {
			return script.Nil
		}
		return h.z.handleKey(t)
	}
	monsterFields["state"] = func(_ scriptHost, e *entity) script.Value {
		return script.String(e.monster.ai.State.String())
	}
	monsterFields["master"] = func(h scriptHost, e *entity) script.Value {
		ms := e.monster.master
		if ms == nil || !h.z.entities.has(ms) {
			return script.Nil
		}
		return h.z.handleKey(ms)
	}
}

// liveMonster finds a living monster by its handle.
func (h scriptHost) liveMonster(id int64) (*entity, error) {
	e := h.z.monsterHandles[id]
	if e == nil || e.dead || !h.z.entities.has(e) {
		return nil, errors.New("the monster is gone from this map")
	}
	if e.status == nil {
		return nil, errors.New("the monster has no stats, so it has no AI")
	}
	return e, nil
}

// MonsterSetTarget is unitattack for a monster: it takes the target and
// chases it to attack.
func (h scriptHost) MonsterSetTarget(id int64, ref modscript.Entity) (bool, error) {
	e, err := h.liveMonster(id)
	if err != nil {
		return false, err
	}
	t, err := h.entityOf(ref)
	if err != nil {
		return false, err
	}
	if t == e || t.dead || !h.z.canTargetAI(e, t) {
		return false, nil
	}
	m := e.monster
	m.forceWalk = false
	m.target = t
	if h.z.inRange(e, t, e.attackRange()) {
		h.z.unitAttack(e, t)
		return true, nil
	}
	return h.z.walkToBL(e, t, e.attackRange(), true), nil
}

// MonsterClearTarget is mob_unlocktarget.
func (h scriptHost) MonsterClearTarget(id int64) error {
	e, err := h.liveMonster(id)
	if err != nil {
		return err
	}
	h.z.mobUnlockTarget(e)
	return nil
}

// MonsterWalkTo is unitwalk for a monster: it walks to the cell, and its
// AI waits until it gets there (ud.state.force_walk).
func (h scriptHost) MonsterWalkTo(id int64, x, y int) (bool, error) {
	z := h.z
	e, err := h.liveMonster(id)
	if err != nil {
		return false, err
	}
	if x >= z.grid.Width || y >= z.grid.Height || z.grid.NoPass(x, y) {
		return false, nil
	}
	if e.status.Mode&mob.ModeCanMove == 0 || !z.unitCanMove(e) {
		return false, nil
	}
	if x == e.x && y == e.y {
		return true, nil
	}
	z.walk(e, x, y)
	if len(e.path) == 0 {
		return false, nil
	}
	m := e.monster
	e.target = nil
	m.chaseTo, m.attackContinue, m.walked = nil, false, 0
	m.forceWalk = true
	return true, nil
}

// MonsterUseSkill is unitskilluseid for a monster: the skill with its own
// cast time, on the target, or on its cell for a ground skill. No delay
// of the monster's skills list applies.
func (h scriptHost) MonsterUseSkill(id int64, skillID, level int64, ref modscript.Entity) (bool, error) {
	z := h.z
	e, err := h.liveMonster(id)
	if err != nil {
		return false, err
	}
	t, err := h.entityOf(ref)
	if err != nil {
		return false, err
	}
	if z.Skills == nil {
		return false, errors.New("this zone has no skills")
	}
	d := z.Skills.Get(int(skillID))
	if d == nil {
		return false, fmt.Errorf("there is no skill %d", skillID)
	}
	if d.Passive() {
		return false, fmt.Errorf("%s is a passive skill", d.Name)
	}
	lv := int(level)
	e.monster.skillIdx = -1
	o := &castOverride{ms: d.CastTime.At(lv), cancel: d.CastCancel}
	if d.Target == skill.TargetGround {
		return z.skillUsePosWith(e, d, lv, t.x, t.y, o), nil
	}
	if t != e && !z.inRange(e, t, d.RangeFor(e.status, lv)) {
		return false, nil
	}
	return z.skillUseIDWith(e, t, d, lv, o), nil
}

// MonsterTalk is unittalk for a monster: the line goes to the players
// near it, after its name.
func (h scriptHost) MonsterTalk(id int64, text string) error {
	e, err := h.liveMonster(id)
	if err != nil {
		return err
	}
	h.z.mobChat(e, text, 0)
	return nil
}

// MonsterStop is unitstop for a monster: it stops attacking and walking,
// and drops its target.
func (h scriptHost) MonsterStop(id int64) error {
	e, err := h.liveMonster(id)
	if err != nil {
		return err
	}
	m := e.monster
	e.target = nil
	h.z.stopWalking(e)
	m.target, m.chaseTo, m.attackContinue, m.forceWalk = nil, nil, false, false
	return nil
}

// runMonsterTargetHook runs the monster_target hooks: the result is one
// of the candidates, or nil.
func (z *Zone) runMonsterTargetHook(e *entity, candidates []*entity, pick *entity, reason string) *entity {
	refs := make([]modscript.Entity, 0, len(candidates))
	byRef := map[modscript.Entity]*entity{}
	for _, c := range candidates {
		if c.player == nil && c.monster == nil && c.npc == nil {
			continue
		}
		r := z.ref(c)
		refs = append(refs, r)
		byRef[r] = c
	}
	if len(refs) == 0 {
		return pick
	}
	m := modscript.MonsterTarget{Monster: z.monsterHandle(e), Candidates: refs, Reason: reason}
	if pick != nil {
		r := z.ref(pick)
		m.Current = &r
	}
	got := z.scripts.RunMonsterTarget(m)
	if got == nil {
		return nil
	}
	if c, ok := byRef[*got]; ok {
		return c
	}
	return pick
}

// startMonsterScript queues the setup of a monster's AI script, which
// runs where scripts may run.
func (z *Zone) startMonsterScript(e *entity) {
	if z.scripts == nil || e.monster.spawn.Script == "" {
		return
	}
	z.scriptEvents = append(z.scriptEvents, scriptEvent{monster: e})
}

// runMonsterScript runs the setup of a monster's AI script, in a new
// context of kind "monster" whose instance is the monster.
func (z *Zone) runMonsterScript(e *entity) {
	if e.dead || !z.entities.has(e) {
		return
	}
	id := z.monsterHandle(e)
	e.monster.ctx = id
	if err := z.scripts.StartMonster(id, e.monster.spawn.Script); err != nil && !quietOff(err) && z.Mods != nil && z.Mods.Log != nil {
		z.Mods.Log.Warn("monster script failed", "map", z.Map, "monster", e.monster.spawn.Mob, "err", err)
	}
}

// endMonsterScript ends the context of a monster's AI script.
func (z *Zone) endMonsterScript(e *entity) {
	if z.scripts == nil || e.monster == nil || e.monster.ctx == 0 {
		return
	}
	z.scripts.EndMonster(e.monster.ctx)
	e.monster.ctx = 0
}
