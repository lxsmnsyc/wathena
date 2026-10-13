package zone

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Skills and mod scripts (proposal 0002, review decisions 18 and 20):
// the cast event of skills with a script, passive skill contexts, the
// skill_used event, the skill hooks, and the entity calls of scripts.
//
// A cast method such as cast:deal_damage runs inside a script's call,
// where Lua cannot run again. What a hit makes Lua do, such as a death
// event, drop hooks or an item's damage handlers, waits until the call
// returns (flushDeferred).

// passiveSkill is a learned passive skill whose script runs.
type passiveSkill struct {
	level int
}

// ref is an entity's script handle.
func (z *Zone) ref(e *entity) modscript.Entity { return entityRef(z.unit(e).Ref()) }

// refPtr is ref for an entity that may be nil.
func (z *Zone) refPtr(e *entity) *modscript.Entity {
	if e == nil {
		return nil
	}
	r := z.ref(e)
	return &r
}

// inHost reports whether a script's call runs now.
func (z *Zone) inHost() bool { return z.scripts != nil && z.scripts.InHost() }

// later runs f once no script's call runs: at once, or after the call.
func (z *Zone) later(f func()) {
	if z.inHost() {
		z.deferred = append(z.deferred, f)
		return
	}
	f()
}

// flushDeferred runs what waited for a script's call to return.
func (z *Zone) flushDeferred() {
	for len(z.deferred) > 0 && !z.inHost() {
		f := z.deferred[0]
		z.deferred = z.deferred[1:]
		f()
	}
}

// runSkillScript runs the cast event of a skill with a script, in place
// of its Go behavior. It reports whether a script ran. A cast inside a
// script's call, such as an autospell from cast:deal_damage, runs the
// Go behavior only.
func (z *Zone) runSkillScript(src, target *entity, d *skill.Def, lv, x, y int, ground bool) bool {
	if d.Script == "" || z.inHost() || z.itemScripts() == nil {
		return false
	}
	if !ground && target != nil {
		x, y = target.x, target.y
	}
	z.nextCast++
	c := modscript.Cast{ID: z.nextCast, Caster: z.ref(src), Skill: int64(d.ID), Level: int64(lv), X: int64(x), Y: int64(y)}
	if !ground {
		c.Target = z.refPtr(target)
	}
	ran, err := z.scripts.RunCast(d.Script, c)
	if err != nil && !quietOff(err) && z.Mods != nil && z.Mods.Log != nil {
		z.Mods.Log.Warn("skill script failed", "map", z.Map, "skill", d.Name, "err", err)
	}
	z.flushDeferred()
	return ran
}

// skillUsed fires the skill_used event once a skill took effect: the
// handlers of mods and events, and those of the caster's equipped items.
// x and y are the target's cell, or the ground cell.
func (z *Zone) skillUsed(src, target *entity, d *skill.Def, lv, x, y int) {
	if z.scripts == nil {
		return
	}
	if target != nil {
		x, y = target.x, target.y
	}
	ev := modscript.SkillUsed{Caster: z.ref(src), Skill: int64(d.ID), Level: int64(lv), Target: z.refPtr(target),
		X: int64(x), Y: int64(y)}
	z.later(func() {
		z.scripts.FireSkillUsed(ev)
		p := src.player
		if p == nil || p.gone() || len(p.worn) == 0 {
			return
		}
		u := modscript.SkillUse{Skill: ev.Skill, Level: ev.Level, X: ev.X, Y: ev.Y}
		if t := ev.Target; t != nil {
			u.Target, u.TargetKind = t.ID, t.Kind
		}
		for _, slot := range wornSlots(p) {
			ws := p.worn[slot]
			for _, es := range append([]*equipScript{ws.item}, ws.cards...) {
				if es != nil && z.scripts.Listens(es.ctx, "skill_used") {
					if err := z.scripts.FireSkillUse(es.ctx, u); err != nil && z.Mods != nil && z.Mods.Log != nil {
						z.Mods.Log.Warn("skill_used handler failed", "map", z.Map, "err", err)
					}
				}
			}
		}
	})
}

// skillHook is the object of the skill hooks.
func (z *Zone) skillHook(src, target *entity, d *skill.Def, lv int) modscript.SkillHook {
	return modscript.SkillHook{Caster: z.ref(src), Skill: int64(d.ID), Level: int64(lv), Target: z.refPtr(target)}
}

// hookedRatio is a skill's formulas with the skill_damage hooks run on
// the damage percent it gives.
type hookedRatio struct {
	inner battle.SkillImpl
	hook  func(int) int
}

func (h hookedRatio) SkillRatio(a *battle.SkillContext, ratio *int) {
	if h.inner != nil {
		h.inner.SkillRatio(a, ratio)
	}
	*ratio = h.hook(*ratio)
}

func (h hookedRatio) HitRate(a *battle.SkillContext, hit *int) {
	if h.inner != nil {
		h.inner.HitRate(a, hit)
	}
}

// hookedBase is hookedRatio for a skill with its own base damage, as
// Heal on undead.
type hookedBase struct {
	hookedRatio
	base battle.BaseDamager
}

func (h hookedBase) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	return h.base.BaseDamage(a, r)
}

// hookSkillRatio wraps a skill's formulas so the skill_damage hooks
// change its damage percent. Without hooks it changes nothing.
func (z *Zone) hookSkillRatio(src, target *entity, d *skill.Def, lv int, impl battle.SkillImpl) battle.SkillImpl {
	if z.scripts == nil || z.inHost() || !z.scripts.HasHooks("skill_damage") {
		return impl
	}
	h := z.skillHook(src, target, d, lv)
	hr := hookedRatio{inner: impl, hook: func(ratio int) int {
		return int(min(max(z.scripts.RunSkillDamage(h, int64(ratio)), 0), modscript.MaxSkillPercent))
	}}
	if b, ok := impl.(battle.BaseDamager); ok {
		return hookedBase{hr, b}
	}
	return hr
}

// hookCastTime runs the cast_time hooks.
func (z *Zone) hookCastTime(src, target *entity, d *skill.Def, lv, ms int) int {
	if z.scripts == nil || z.inHost() || !z.scripts.HasHooks("cast_time") {
		return ms
	}
	return int(min(max(z.scripts.RunCastTime(z.skillHook(src, target, d, lv), int64(ms)), 0), modscript.MaxCastMs))
}

// hookSkillCost runs the skill_cost hooks on the SP a skill costs.
func (z *Zone) hookSkillCost(src *entity, d *skill.Def, lv, sp int) int {
	if z.scripts == nil || z.inHost() || !z.scripts.HasHooks("skill_cost") {
		return sp
	}
	return int(min(max(z.scripts.RunSkillCost(z.skillHook(src, nil, d, lv), int64(sp)), 0), 1<<15-1))
}

// hookCastAllowed runs the cast_allowed hooks when a cast would start.
func (z *Zone) hookCastAllowed(src, target *entity, d *skill.Def, lv int) bool {
	if z.scripts == nil || z.inHost() || !z.scripts.HasHooks("cast_allowed") {
		return true
	}
	return z.scripts.RunCastAllowed(z.skillHook(src, target, d, lv), true)
}

// startPassives starts the scripts of the passive skills a player
// learned, when it enters the zone.
func (z *Zone) startPassives(p *Player) {
	if z.Skills == nil || z.Mods == nil {
		return
	}
	for _, id := range sortedSkills(p.char.Skills) {
		z.passiveChanged(p, id)
	}
}

// passiveChanged starts, restarts or ends the script of a passive skill
// whose level changed.
func (z *Zone) passiveChanged(p *Player, id int) {
	if z.Skills == nil {
		return
	}
	d := z.Skills.Get(id)
	if d == nil || !d.Passive() || d.Script == "" || z.itemScripts() == nil {
		return
	}
	lv := p.char.Skills[id]
	owner := z.ref(p.entity)
	if lv <= 0 {
		z.scripts.StopPassive(int64(id), owner)
		delete(p.passives, id)
		return
	}
	if err := z.scripts.StartPassive(d.Script, int64(id), owner, int64(lv)); err != nil {
		if !quietOff(err) && z.Mods != nil && z.Mods.Log != nil {
			z.Mods.Log.Warn("passive skill script failed", "map", z.Map, "skill", d.Name, "err", err)
		}
		return
	}
	if p.passives == nil {
		p.passives = map[int]*passiveSkill{}
	}
	p.passives[id] = &passiveSkill{level: lv}
}

// stopPassives ends a player's passive skill scripts, when it leaves the
// zone.
func (z *Zone) stopPassives(p *Player) {
	if z.scripts == nil || len(p.passives) == 0 {
		return
	}
	z.scripts.StopPassives(z.ref(p.entity))
	p.passives = nil
}

// The EntityHost and SkillHost calls of scripts.

var errNoSkills = errors.New("the server has no skill data")

// FindEntities is maps.find_entities and entity:find_nearby.
func (h scriptHost) FindEntities(q modscript.EntityQuery) ([]modscript.Entity, error) {
	z := h.z
	zq := &query{sort: q.Sort, limit: q.Limit, circle: q.Circle, lineOfSight: q.LineOfSight}
	switch q.Area {
	case modscript.AreaAround:
		e, err := h.entityOf(q.Around)
		if err != nil {
			return nil, err
		}
		zq.area, zq.x, zq.y, zq.radius = areaRadius, e.x, e.y, q.Radius
	case modscript.AreaCell:
		zq.area, zq.x, zq.y, zq.radius = areaRadius, q.X, q.Y, q.Radius
	case modscript.AreaRectangle:
		zq.area = areaRect
		zq.x0, zq.y0, zq.x1, zq.y1 = q.Corners[0][0], q.Corners[0][1], q.Corners[1][0], q.Corners[1][1]
	case modscript.AreaLine:
		zq.area = areaLine
		zq.x0, zq.y0, zq.x1, zq.y1 = q.From[0], q.From[1], q.To[0], q.To[1]
		zq.width = q.Width / 2
	case modscript.AreaCells:
		zq.area, zq.cells = areaCells, q.Cells
	default:
		return nil, fmt.Errorf("unknown area %q", q.Area)
	}
	for _, k := range q.Kinds {
		switch k {
		case "player":
			zq.kinds |= queryPlayers
		case "monster":
			zq.kinds |= queryMonsters
		case "npc":
			zq.kinds |= queryNPCs
		}
	}
	if !q.Alive {
		zq.dead, zq.onlyDead = true, true
	}
	if q.EnemiesOf != nil {
		e, err := h.entityOf(*q.EnemiesOf)
		if err != nil {
			return nil, fmt.Errorf("enemies_of: %w", err)
		}
		zq.enemiesOf = e
	}
	if q.AlliesOf != nil {
		e, err := h.entityOf(*q.AlliesOf)
		if err != nil {
			return nil, fmt.Errorf("allies_of: %w", err)
		}
		zq.alliesOf = e
	}
	for _, x := range q.Exclude {
		if e, err := h.entityOf(x); err == nil {
			if zq.exclude == nil {
				zq.exclude = map[*entity]bool{}
			}
			zq.exclude[e] = true
		}
	}
	found := z.findEntities(zq)
	out := make([]modscript.Entity, 0, len(found))
	for _, e := range found {
		out = append(out, z.ref(e))
	}
	return out, nil
}

// EntityExists is entity:exists.
func (h scriptHost) EntityExists(e modscript.Entity) bool {
	x, err := h.entityOf(e)
	return err == nil && h.z.entities.has(x)
}

// ShowEntityEffect is show_effect on a monster or an NPC.
func (h scriptHost) ShowEntityEffect(ref modscript.Entity, id int64) error {
	e, err := h.entityOf(ref)
	if err != nil {
		return err
	}
	h.z.effects = append(h.z.effects, effect{e, uint32(id)}) // #nosec G115 -- checked by the runtime.
	return nil
}

// castCaster is the caster of a cast, or nil when it is gone or cannot
// act, as skill_addtimerskill's checks.
func (h scriptHost) castCaster(c modscript.Cast) *entity {
	e, err := h.entityOf(c.Caster)
	if err != nil || e.dead || !h.z.entities.has(e) {
		return nil
	}
	if e.actsBlocked() || e.sc.Has(sc.SCTrickdead) {
		return nil
	}
	return e
}

// CastDealDamage is cast:deal_damage: one skill_attack of the cast's
// skill, with the script's percent as the skill's ratio.
func (h scriptHost) CastDealDamage(c modscript.Cast, target modscript.Entity, o modscript.CastDamage) (*modscript.CastDamageResult, error) {
	z := h.z
	if z.Skills == nil {
		return nil, errNoSkills
	}
	src := h.castCaster(c)
	if src == nil {
		return nil, nil
	}
	t, err := h.entityOf(target)
	if err != nil || t.dead || t.status == nil {
		return &modscript.CastDamageResult{Missed: true}, nil
	}
	d := z.Skills.Get(int(c.Skill))
	if d == nil {
		return nil, fmt.Errorf("there is no skill %d", c.Skill)
	}
	dd := *d
	if o.Element >= 0 {
		for i := range dd.Element {
			dd.Element[i] = int(o.Element)
		}
	}
	if o.Hits > 0 {
		for i := range dd.HitCount {
			dd.HitCount[i] = int(o.Hits)
		}
	}
	typ := dd.Type
	switch o.Type {
	case "weapon":
		typ = battle.TypeWeapon
	case "magic":
		typ = battle.TypeMagic
	case "misc":
		typ = battle.TypeMisc
	}
	if typ == 0 {
		typ = battle.TypeMagic
	}
	lv := max(int(c.Level), 1)
	if t.sc.Has(sc.SCTrickdead) || skillImmune(t) {
		return &modscript.CastDamageResult{Missed: true}, nil
	}
	res := z.calcSkillAttack(typ, src, src, t, &dd, lv, 0, skill.Percent{Value: int(o.Percent)})
	z.applySkillAttack(src, src, t, &dd, lv, 0, res)
	return &modscript.CastDamageResult{Amount: res.Damage, Critical: res.Kind == battle.Critical,
		Missed: res.Kind == battle.Miss || res.Kind == battle.LuckyDodge}, nil
}

// CastHeal is cast:heal: status_heal with the heal bonuses of the caster
// and the target for the cast's skill, as skill_calc_heal adds them.
func (h scriptHost) CastHeal(c modscript.Cast, target modscript.Entity, amount int64) (int64, bool, error) {
	z := h.z
	src := h.castCaster(c)
	if src == nil {
		return 0, false, nil
	}
	t, err := h.entityOf(target)
	if err != nil || t.dead || t.status == nil {
		return 0, true, nil
	}
	bonus := int64(0)
	if st := src.status; st != nil && st.Player != nil {
		bonus += int64(st.Player.AddHealRate + st.Player.SkillHeal[int(c.Skill)])
	}
	if st := t.status; st.Player != nil {
		bonus += int64(st.Player.AddHeal2Rate + st.Player.SkillHeal2[int(c.Skill)])
	}
	amount += amount * bonus / 100
	before := t.hp
	z.healHP(t, amount)
	if z.Skills != nil {
		if d := z.Skills.Get(int(c.Skill)); d != nil {
			z.skillEffect(src, t, d, int(c.Level), amount, true)
		}
	}
	return max(t.hp-before, 0), true, nil
}

// CastRunDefault is cast:run_default: core's behavior of the cast's
// skill. It runs once the script's call returns.
func (h scriptHost) CastRunDefault(c modscript.Cast, target *modscript.Entity) error {
	z := h.z
	if z.Skills == nil {
		return errNoSkills
	}
	d := z.Skills.Get(int(c.Skill))
	if d == nil || d.Mod != "" || skillImpls[d.ID] == nil {
		return errors.New("the skill has no core behavior")
	}
	impl := skillImpls[d.ID]
	if _, ok := impl.(notPorted); ok {
		return errors.New("the skill's core behavior is not ported yet")
	}
	src := h.castCaster(c)
	if src == nil {
		return nil
	}
	ref := c.Target
	if target != nil {
		ref = target
	}
	var t *entity
	if ref != nil {
		e, err := h.entityOf(*ref)
		if err != nil {
			return err
		}
		t = e
	}
	lv := max(int(c.Level), 1)
	x, y := int(c.X), int(c.Y)
	z.deferred = append(z.deferred, func() {
		if src.dead || !z.entities.has(src) || t != nil && (!z.entities.has(t) || t.dead) {
			return
		}
		flag := 0
		switch {
		case d.CastType() == skill.CastGround:
			if i, ok := impl.(castendPoser); ok {
				i.castendPos(z, src, x, y, d, lv, &flag)
			}
		case t == nil:
		case d.CastType() == skill.CastNoDamage:
			if i, ok := impl.(castendNoDamager); ok {
				i.castendNoDamage(z, src, t, d, lv, &flag)
			} else if i, ok := impl.(castendDamager); ok {
				i.castendDamage(z, src, t, d, lv, &flag)
			}
		default:
			if i, ok := impl.(castendDamager); ok {
				i.castendDamage(z, src, t, d, lv, &flag)
			}
		}
	})
	return nil
}

// SkillID is skills.id.
func (h scriptHost) SkillID(name string) (int64, bool) {
	if h.z.Skills == nil {
		return 0, false
	}
	if d := h.z.Skills.ByName(name); d != nil {
		return int64(d.ID), true
	}
	return 0, false
}

var (
	_ modscript.EntityHost = scriptHost{}
	_ modscript.SkillHost  = scriptHost{}
)
