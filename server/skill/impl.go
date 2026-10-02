package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// Skills are Go types that follow rAthena's SkillImpl: each one has the
// methods of the parts it changes, and the zone calls them where rAthena
// calls the SkillImpl methods. The common shapes are base types to embed,
// as rAthena's WeaponSkillImpl and StatusSkillImpl.

// Base is SkillImpl's no-op methods. Every skill type embeds it, or a
// shape that embeds it.
type Base struct{}

// SkillRatio is SkillImpl::calculateSkillRatio. It changes nothing.
func (Base) SkillRatio(*battle.SkillContext, *int) {}

// HitRate is SkillImpl::modifyHitRate. It changes nothing.
func (Base) HitRate(*battle.SkillContext, *int) {}

// registry holds the skills that have Go behavior, by ID.
var registry = map[int]battle.SkillImpl{}

// register adds skills to the registry, once each.
func register(impls map[int]battle.SkillImpl) {
	for id, impl := range impls {
		if _, dup := registry[id]; dup {
			panic("skill: registered twice")
		}
		registry[id] = impl
	}
}

// implOf is a skill's Go behavior, or nil for a skill without one.
func implOf(d *Def) battle.SkillImpl {
	if d.Mod != "" {
		return nil
	}
	return registry[d.ID]
}

// Ported reports whether a skill has Go behavior.
func Ported(id int) bool { _, ok := registry[id]; return ok }

// Percent is the SkillImpl of a skill whose ratio a script gives:
// cast:deal_damage's percent replaces the base 100%.
type Percent struct {
	Base
	Value int
}

// SkillRatio sets the ratio to the percent, on top of the ATK percent
// that renewal starts from.
func (p Percent) SkillRatio(_ *battle.SkillContext, ratio *int) { *ratio += p.Value - 100 }
