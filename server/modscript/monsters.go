package modscript

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Monster AI (proposal 0002, review decision 22): the methods of monster
// handles, the monster_target hook, and the AI scripts of monster
// records.

// MonsterHost runs the monster methods for scripts. A Host without it
// raises an error on them.
type MonsterHost interface {
	// MonsterSetTarget makes a monster go for a target, as unitattack.
	// It reports whether the monster took it.
	MonsterSetTarget(monster int64, target Entity) (bool, error)
	// MonsterClearTarget makes a monster drop its target, as
	// mob_unlocktarget.
	MonsterClearTarget(monster int64) error
	// MonsterWalkTo walks a monster to a cell, as unitwalk. Its AI waits
	// until it gets there. It reports whether the walk started.
	MonsterWalkTo(monster int64, x, y int) (bool, error)
	// MonsterUseSkill makes a monster use a skill on a target, or on the
	// target's cell for a ground skill, as unitskilluseid. It reports
	// whether the cast started.
	MonsterUseSkill(monster int64, skill, level int64, target Entity) (bool, error)
	// MonsterTalk makes a monster say a line to the players near it, as
	// unittalk.
	MonsterTalk(monster int64, text string) error
	// MonsterStop stops a monster's walk and attack, and drops its
	// target, as unitstop.
	MonsterStop(monster int64) error
}

// MonsterTarget is the object of the monster_target hook: a monster
// chooses a target among candidates. Current is rAthena's choice, which
// the chain returns without hooks, or nil. Reason is "aggressive",
// "assist", "hit" or "change".
type MonsterTarget struct {
	Monster    int64
	Candidates []Entity
	Current    *Entity
	Reason     string
}

// RunMonsterTarget runs the monster_target hooks and returns the target
// they choose, or nil for none. A choice that is not a candidate counts
// as a failure of the hook, which the chain leaves out. Without hooks,
// inside a script's call, or when the chain fails, it is m.Current.
func (r *Runtime) RunMonsterTarget(m MonsterTarget) *Entity {
	if r.s.InHost() {
		return m.Current
	}
	list := make([]script.Value, len(m.Candidates))
	for i, c := range m.Candidates {
		list[i] = script.String(c.Kind + ":" + fmt.Sprint(c.ID))
	}
	fields := []script.Field{
		script.KV("monster", script.Int(m.Monster)),
		script.KV("candidates", script.List(list...)),
		script.KV("reason", script.String(m.Reason)),
	}
	if m.Current != nil {
		fields = append(fields, script.KV("current", script.String(m.Current.Kind+":"+fmt.Sprint(m.Current.ID))))
	}
	vals, ok := r.runHooks("monster_target", fields)
	if !ok {
		return m.Current
	}
	if vals[0].IsNil() {
		return nil
	}
	e, ok := parseEntityKey(vals[0])
	if !ok {
		return m.Current
	}
	return &e
}

// monsterHost returns the host's monster calls.
func (r *Runtime) monsterHost(what string) (MonsterHost, error) {
	h, ok := r.host.(MonsterHost)
	if !ok {
		return nil, fmt.Errorf("%s: monsters cannot act here", what)
	}
	return h, nil
}

// monsterArg reads a monster handle's ID.
func monsterArg(c *script.Call) (int64, error) {
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return 0, errors.New("not a monster: call the method with a colon, such as monster:stop()")
	}
	return id, nil
}

// monsterCall is the start of every monster method: the handle, the
// check that the game may change, and the host.
func (r *Runtime) monsterCall(c *script.Call, what string) (int64, MonsterHost, error) {
	id, err := monsterArg(c)
	if err != nil {
		return 0, nil, err
	}
	if err := r.mutating(what); err != nil {
		return 0, nil, err
	}
	h, err := r.monsterHost(what)
	if err != nil {
		return 0, nil, err
	}
	return id, h, nil
}

func (r *Runtime) monsterSetTarget(c *script.Call) error {
	id, h, err := r.monsterCall(c, "monster:set_target")
	if err != nil {
		return err
	}
	t, ok := parseEntityKey(c.Arg(1))
	if !ok {
		return errors.New("monster:set_target: the target must be a player, monster or NPC handle")
	}
	took, err := h.MonsterSetTarget(id, t)
	if err != nil {
		return fmt.Errorf("monster:set_target: %w", err)
	}
	c.Return(script.Bool(took))
	return nil
}

func (r *Runtime) monsterClearTarget(c *script.Call) error {
	id, h, err := r.monsterCall(c, "monster:clear_target")
	if err != nil {
		return err
	}
	if err := h.MonsterClearTarget(id); err != nil {
		return fmt.Errorf("monster:clear_target: %w", err)
	}
	return nil
}

func (r *Runtime) monsterWalkTo(c *script.Call) error {
	id, h, err := r.monsterCall(c, "monster:walk_to")
	if err != nil {
		return err
	}
	x, okx := c.Arg(1).AsInt()
	y, oky := c.Arg(2).AsInt()
	if !okx || !oky || x < 0 || y < 0 || x > 1<<16 || y > 1<<16 {
		return errors.New("monster:walk_to: want the cell's x and y")
	}
	ok, err := h.MonsterWalkTo(id, int(x), int(y))
	if err != nil {
		return fmt.Errorf("monster:walk_to: %w", err)
	}
	c.Return(script.Bool(ok))
	return nil
}

func (r *Runtime) monsterUseSkill(c *script.Call) error {
	id, h, err := r.monsterCall(c, "monster:use_skill")
	if err != nil {
		return err
	}
	skill, ok1 := c.Arg(1).AsInt()
	level, ok2 := c.Arg(2).AsInt()
	if !ok1 || !ok2 || skill <= 0 || level < 1 || level > MaxMonsterSkillLevel {
		return fmt.Errorf("monster:use_skill: want a skill from skills.id, a level from 1 to %d, and a target", MaxMonsterSkillLevel)
	}
	t, ok := parseEntityKey(c.Arg(3))
	if !ok {
		return errors.New("monster:use_skill: the target must be a player, monster or NPC handle; use the monster itself for a skill on itself")
	}
	started, err := h.MonsterUseSkill(id, skill, level, t)
	if err != nil {
		return fmt.Errorf("monster:use_skill: %w", err)
	}
	c.Return(script.Bool(started))
	return nil
}

// MaxMonsterSkillLevel is MAX_MOBSKILL_LEVEL.
const MaxMonsterSkillLevel = 100

func (r *Runtime) monsterTalk(c *script.Call) error {
	id, h, err := r.monsterCall(c, "monster:talk")
	if err != nil {
		return err
	}
	text := c.String(1)
	if text == "" || len(text) > MaxMessage {
		return fmt.Errorf("monster:talk: the text must be 1 to %d bytes", MaxMessage)
	}
	if err := h.MonsterTalk(id, text); err != nil {
		return fmt.Errorf("monster:talk: %w", err)
	}
	return nil
}

func (r *Runtime) monsterStop(c *script.Call) error {
	id, h, err := r.monsterCall(c, "monster:stop")
	if err != nil {
		return err
	}
	if err := h.MonsterStop(id); err != nil {
		return fmt.Errorf("monster:stop: %w", err)
	}
	return nil
}

// StartMonster runs a monster's AI script: entry's setup function in a
// new context of kind "monster", whose instance is the monster. The
// context lasts until EndMonster.
func (r *Runtime) StartMonster(monster int64, entry string) error {
	if old := r.monsterCtxs[monster]; old != nil {
		r.end(old)
	}
	setup, mod, err := r.setup(entry)
	if err != nil {
		return err
	}
	sc := r.scripts[mod]
	c := &Context{ID: r.newID(), Kind: KindMonster, Mod: mod, grants: sc.Grants, timers: map[int64]*timer{}}
	c.Instance = script.NewTable(script.KV("monster", script.Int(monster)))
	r.monsterCtxs[monster] = c
	if err := r.runSetup(c, setup); err != nil {
		r.end(c)
		delete(r.monsterCtxs, monster)
		return fmt.Errorf("%s: setup: %w", entry, err)
	}
	return nil
}

// EndMonster ends the context of a monster's AI script, when the monster
// dies or leaves the map.
func (r *Runtime) EndMonster(monster int64) {
	if c := r.monsterCtxs[monster]; c != nil {
		delete(r.monsterCtxs, monster)
		r.end(c)
	}
}

// reloadMonsters ends the monster contexts of a reloaded mod. The
// monsters spawned from then on run the new script.
func (r *Runtime) reloadMonsters(modID string) {
	for id, c := range r.monsterCtxs {
		if c.Mod == modID {
			delete(r.monsterCtxs, id)
			r.end(c)
		}
	}
}
