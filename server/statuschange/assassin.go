package statuschange

// The parts of the Assassin statuses that need the zone: Cloaking's wall
// check (skill_check_cloaking) and Venom Splasher's explosion when its
// timer ends it.

// WallChecker is an Env that knows the map. NearWall reports whether a
// cell next to the unit cannot be entered (CELL_CHKNOPASS), as the wall
// check of skill_check_cloaking. An Env without it has no walls.
type WallChecker interface {
	NearWall(u Unit) bool
}

// SkillCaster is an Env that runs skills. CastendDamage is
// skill_castend_damage_id: src casts a skill at a level on the target,
// with a flag.
type SkillCaster interface {
	CastendDamage(src Source, target Unit, skill, lv, flag int)
}

// sdLevel is SD_LEVEL: the flag of a skill that a status casts.
const sdLevel = 0x1000

// cloakWall is skill_check_cloaking without a status: whether a wall is
// next to the unit. Without the wall check of player_cloak_check_type or
// monster_cloak_check_type, there is always a wall.
func (en *Engine) cloakWall(u Unit) bool {
	check := monsterCloakCheckType
	if u.Kind() == KindPlayer {
		check = pcCloakCheckType
	}
	if check&1 == 0 {
		return true
	}
	if w, ok := en.env.(WallChecker); ok {
		return w.NearWall(u)
	}
	return false
}

// CheckCloaking is skill_check_cloaking with the unit's Cloaking, which a
// player's walk request runs. Away from walls, Cloaking below level 3
// ends, and higher levels lose the wall's speed bonus (val4 bit 1). Next
// to a wall, the bonus comes back.
func (en *Engine) CheckCloaking(u Unit) {
	e := u.Changes().Get(SCCloaking)
	if e == nil {
		return
	}
	if !en.cloakWall(u) {
		switch {
		case e.Val1 < 3:
			en.End(u, SCCloaking, ReasonRemoved)
		case e.Val4&1 != 0:
			e.Val4 &^= 1
			en.env.Recalc(u, en.DB.Get(SCCloaking))
		}
		return
	}
	if e.Val4&1 == 0 {
		e.Val4 |= 1
		en.env.Recalc(u, en.DB.Get(SCCloaking))
	}
}

// CloakingMoved is map_moveblock's Cloaking check: a unit that moved away
// from walls with Cloaking below level 3 loses it.
func (en *Engine) CloakingMoved(u Unit) {
	if e := u.Changes().Get(SCCloaking); e != nil && e.Val1 < 3 && !en.cloakWall(u) {
		en.End(u, SCCloaking, ReasonRemoved)
	}
}
