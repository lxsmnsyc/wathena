package zone

import (
	"math"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Skill behaviors of the Priest and the High Priest, as the SkillImpl
// classes in rAthena's src/map/skills/acolyte. Parties do not exist yet:
// the party skills reach the caster alone, as rAthena does for a player
// without a party, and Redemptio fails, as it does without a party. Mace
// Mastery, Meditatio and Mana Recharge are passive: their effects are in
// server/status, server/battle and server/skill.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDResurrection:   resurrection{},
		skill.IDImpositio:      renewalPartyStatus{},
		skill.IDSuffragium:     renewalPartyStatus{},
		skill.IDAspersio:       aspersio{},
		skill.IDBenedictio:     benedictio{},
		skill.IDSanctuary:      unitSkillImpl{},
		skill.IDSlowPoison:     statusSkill{},
		skill.IDStatusRecovery: statusRecovery{},
		skill.IDKyrie:          kyrie{},
		skill.IDMagnificat:     partyStatus{},
		skill.IDGloria:         partyStatus{},
		skill.IDLexDivina:      lexDivina{},
		skill.IDTurnUndead:     turnUndead{},
		skill.IDLexAeterna:     statusSkill{},
		skill.IDMagnus:         unitSkillImpl{},
		skill.IDRedemptio:      redemptio{},
		skill.IDAssumptio:      assumptio{},
		skill.IDBasilica:       basilica{},
	})
}

// Statuses that the Priest skills read, which server/statuschange has
// no constants for.
const (
	scHermode   = 158 // SC_HERMODE
	scHellpower = 294 // SC_HELLPOWER
)

// Skill IDs that the Priest skills read.
const (
	idPressure = 367  // PA_PRESSURE
	idSoulBurn = 375  // PF_SOULBURN
	idBreaker  = 379  // ASC_BREAKER
	mapidAcoly = 0x4  // MAPID_ACOLYTE
	idFirstMsk = 0xff // MAPID_FIRSTMASK
)

// isUndeadUnit is battle_check_undead on a unit's status.
func isUndeadUnit(e *entity) bool {
	st := e.status
	return st != nil && battle.IsUndead(st.Race, st.DefEle)
}

// isUndeadOrDemon reports whether a unit is undead or a demon, which
// the holy skills hit instead of helping.
func isUndeadOrDemon(e *entity) bool {
	return isUndeadUnit(e) || e.status != nil && e.status.Race == battle.RaceDemon
}

// statusIsImmune is status_isimmune: a unit under Hermode's Rod takes no
// magic. Golden Thief Bug Card's bNoMagicDamage is not ported.
func statusIsImmune(e *entity) bool { return e.sc.Has(scHermode) }

// resurrection is SkillResurrection: it revives a dead unit, and hits
// the undead, as Turn Undead.
type resurrection struct{}

func (resurrection) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if !isUndeadUnit(target) {
		return
	}
	z.skillAttack(battle.TypeMagic, src, src, target, d, lv, *flag)
}

func (resurrection) castendNoDamage(z *Zone, src, target *entity, _ *skill.Def, lv int, _ *int) {
	// GvG maps and battlegrounds, where no one revives, do not exist yet.
	z.resurrect(src, target, lv)
}

// resurrect is the revive part of SkillResurrection::castendNoDamageId,
// which Redemptio shares: the target comes back with 10%, 30%, 50% or
// 80% of its HP by level. resurrection_exp is 0, so the caster gains no
// experience. PvP maps do not exist yet.
func (z *Zone) resurrect(src, target *entity, lv int) {
	if !target.dead {
		return
	}
	if target.sc.Has(scHellpower) {
		z.skillEffect(src, target, z.Skills.Get(skill.IDResurrection), lv, int64(lv), true)
		return
	}
	per := 0
	switch lv {
	case 1:
		per = 10
	case 2:
		per = 30
	case 3:
		per = 50
	case 4:
		per = 80
	}
	// bRestartFullRecover is not ported.
	if z.statusRevive(target, per, 0) {
		// Both Redemptio and Resurrection show Resurrection's effect.
		z.skillEffect(src, target, z.Skills.Get(skill.IDResurrection), lv, int64(lv), true)
	}
}

// statusRevive is status_revive and pc_revive: a dead unit stands with
// a percent of its HP and SP, at least 1 for a percent above 0. Players
// who saw it lie dead see it appear again, as clif_resurrection, which
// the protocol does not carry yet. pc_setinvincibletimer is not ported.
func (z *Zone) statusRevive(e *entity, perHP, perSP int) bool {
	if !e.dead || e.status == nil {
		return false
	}
	st := e.status
	e.hp = max(e.hp, 0)
	hp := int64(st.MaxHP) * int64(perHP) / 100
	if room := int64(st.MaxHP) - e.hp; hp > room {
		hp = room
	} else if perHP > 0 && hp == 0 {
		hp = 1
	}
	e.dead = false
	e.hp += hp
	e.hpChanged = true
	if p := e.player; p != nil {
		sp := int64(st.MaxSP) * int64(perSP) / 100
		if room := int64(st.MaxSP) - p.sp; sp > room {
			sp = room
		} else if perSP > 0 && sp == 0 {
			sp = 1
		}
		p.sp += sp
		if s := z.battleStats(p); s != nil {
			p.pending = append(p.pending, s)
		}
		p.fresh = true
	}
	for o := range z.entities.all() {
		if o.player == nil || o == e {
			continue
		}
		if id, ok := o.player.netIDs.get(e); ok {
			o.player.netIDs.del(e)
			z.queue(o.player, frame(z.tick, o.player.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_TELEPORTED)))
		}
	}
	return true
}

// renewalPartyStatus is Impositio Manus and Suffragium: a party buff in
// renewal (SkillImpositioManus, SkillSuffragium), which reaches the
// caster alone without a party, and StatusSkillImpl in pre-renewal.
type renewalPartyStatus struct{}

func (renewalPartyStatus) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if z.renewal() {
		partyStatus{}.castendNoDamage(z, src, target, d, lv, flag)
		return
	}
	statusSkill{}.castendNoDamage(z, src, target, d, lv, flag)
}

// aspersio is SkillAspersio: a holy weapon, which a player cannot give a
// monster, and a hit on the undead.
type aspersio struct{}

func (aspersio) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if src.player != nil && target.monster != nil {
		z.skillEffect(src, target, d, lv, int64(lv), false)
		return
	}
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

func (aspersio) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillAttack(battle.TypeMagic, src, src, target, d, lv, *flag)
}

// benedictio is SkillBenedictioSanctissimiSacramenti: on a 3x3 area,
// holy armor for players that are neither undead nor demons, and a hit on
// the undead and demons among the enemies. It needs two Acolyte class
// partners beside the caster.
type benedictio struct{}

func (benedictio) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	if src.player != nil && len(z.findPartners(src)) < 2 {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (benedictio) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	z.area.main = src
	i := d.SplashFor(lv)
	rect := func(kinds int, enemies bool) []*entity {
		q := &query{area: areaRect, x0: x - i, y0: y - i, x1: x + i, y1: y + i, kinds: kinds}
		if enemies {
			q.enemiesOf = src
		}
		return z.findEntities(q)
	}
	for _, t := range rect(queryPlayers, false) {
		z.castendNoDamage(src, t, d, lv, *flag|1)
	}
	for _, t := range rect(queryPlayers|queryMonsters, true) {
		z.castendDamage(src, t, d, lv, *flag|1)
	}
}

func (benedictio) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if target.status == nil || isUndeadOrDemon(target) {
		return
	}
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

func (benedictio) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if isUndeadOrDemon(target) {
		z.skillAttack(battle.TypeMagic, src, src, target, d, lv, *flag)
	}
}

// findPartners is skill_check_pc_partner's search for Benedictio, with
// skill_check_condition_char_sub: up to two living Acolyte class players
// next to the caster, to its left or right, who can cast. The caster
// needs 10 SP. The zone keeps them for the cast end, as rAthena's static
// list.
func (z *Zone) findPartners(src *entity) []*entity {
	z.partners = z.partners[:0]
	p := src.player
	if p == nil || z.Status == nil {
		return z.partners
	}
	for _, t := range z.findEntities(&query{area: areaRadius, x: src.x, y: src.y, radius: 1, kinds: queryPlayers}) {
		if len(z.partners) >= 2 {
			break
		}
		if t == src || t.dead || t.cant().Cast {
			continue
		}
		mapID, ok := z.Status.MapID(t.player.char.Job)
		if !ok || mapID&idFirstMsk != mapidAcoly {
			continue
		}
		dir := (rDir(src) + mapCalcDir(src, t.x, t.y)) % 8
		if (dir == 2 || dir == 6) && p.sp >= 10 {
			z.partners = append(z.partners, t)
		}
	}
	return z.partners
}

// chargePartners is skill_check_pc_partner with cast_flag for
// Benedictio: each partner found at the cast start pays 10 SP, when it
// has them (status_charge).
func (z *Zone) chargePartners() {
	for _, t := range z.partners {
		p := t.player
		if p == nil || !z.entities.has(t) || p.sp < 10 {
			continue
		}
		p.sp -= 10
		if s := z.battleStats(p); s != nil {
			p.pending = append(p.pending, s)
		}
	}
}

// statusRecovery is SkillStatusRecovery: it ends the body states of a
// living unit, and blinds the undead a second later.
type statusRecovery struct{}

func (statusRecovery) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if statusIsImmune(target) {
		z.skillEffect(src, target, d, lv, int64(lv), false)
		return
	}
	if isUndeadUnit(target) {
		z.addTimerSkill(src, z.now+1000, target, 0, 0, d, lv, 100, *flag)
	} else {
		for _, typ := range []int{sc.SCStone, sc.SCFreeze, sc.SCStun, sc.SCSleep, sc.SCStonewait, sc.SCBurning,
			sc.SCWhiteimprison} {
			z.scEnd(target, typ)
		}
		if target.monster != nil {
			// mob_unlocktarget: the monster goes back to idle.
			target.target, target.repeat = nil, false
		}
	}
	z.scEnd(target, sc.SCNetherworld)
	z.scEnd(target, sc.SCNorecoverState)
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// timerSkill is Status Recovery's part of skill_timerskill: the blind
// of the undead, with the chance in typ.
func (statusRecovery) timerSkill(z *Zone, t *timerSkill) {
	z.scStart(t.src, t.target, sc.SCBlind, t.typ, t.lv, int64(t.def.Duration2.At(t.lv)))
}

// kyrie is SkillKyrieEleison: the barrier on the target.
type kyrie struct{}

func (kyrie) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(target, target, d, lv, int64(lv), ok)
}

// kyrieBlock is Kyrie Eleison's part of battle_status_block_damage. The
// barrier loses the damage of every hit and counts the hit. A weapon hit,
// or Throw Stone, deals only the damage beyond the barrier. The barrier
// ends with its last hit or its last HP, and at once on Holy Light and
// Pressure.
func (z *Zone) kyrieBlock(t *entity, flag, skillID int, dmg int64) int64 {
	e := t.sc.Get(sc.SCKyrie)
	if e == nil {
		return dmg
	}
	e.Val2 -= int(min(max(dmg, math.MinInt32), math.MaxInt32))
	if flag&battle.FlagWeapon != 0 || skillID == skill.IDThrowStone {
		if e.Val2 >= 0 {
			dmg = 0
		} else {
			dmg = int64(-e.Val2)
		}
	}
	e.Val3--
	if e.Val3 <= 0 || e.Val2 <= 0 || skillID == skill.IDHolyLight || skillID == idPressure {
		z.scEnd(t, sc.SCKyrie)
	}
	return dmg
}

// lexAeterna is Lex Aeterna's part of battle_calc_damage: it doubles a
// hit and ends. Soul Burn is not doubled. In pre-renewal, the weapon part
// of Breaker leaves it on.
func (z *Zone) lexAeterna(t *entity, flag, skillID int) bool {
	if skillID == idSoulBurn {
		return false
	}
	if z.renewal() || skillID != idBreaker || flag&battle.FlagWeapon == 0 {
		z.scEnd(t, sc.SCAeterna)
	}
	return true
}

// lexDivina is SkillLexDivina: it cures a silenced target, and silences
// another one a second later.
type lexDivina struct{}

func (lexDivina) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if target.sc.Has(d.Status) {
		z.scEnd(target, d.Status)
	} else {
		z.addTimerSkill(src, z.now+1000, target, 0, 0, d, lv, 100, *flag)
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// timerSkill is Lex Divina's part of skill_timerskill: the silence, with
// the chance in typ. Monsters use Silence's own duration at level 1.
func (lexDivina) timerSkill(z *Zone, t *timerSkill) {
	duration := int64(t.def.Duration2.At(t.lv))
	if t.src.monster != nil && z.Statuses != nil {
		if d := z.Statuses.Get(sc.SCSilence); d != nil {
			duration = int64(d.Durations2[0])
		}
	}
	z.scStart(t.src, t.target, sc.SCSilence, t.typ, t.lv, duration)
}

// turnUndead is SkillTurnUndead: a magic hit on the undead, which may
// kill at once.
type turnUndead struct{}

func (turnUndead) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if !isUndeadUnit(target) {
		return
	}
	z.skillAttack(battle.TypeMagic, src, src, target, d, lv, *flag)
}

// redemptio is SkillRedemptio. It revives the dead members of the
// caster's party at the cost of the caster's HP, SP and, in pre-renewal,
// experience. A player without a party fails, as in rAthena, and
// parties do not exist yet.
type redemptio struct{}

// castBegin is Redemptio's part of skill_check_condition_castbegin: in
// pre-renewal the caster needs exp_cost_redemptio (1%) of the experience
// to the next base level.
func (redemptio) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	p := src.player
	if z.renewal() || p == nil || z.Progress == nil {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	const expCostRedemptio = 1
	next := int64(0)
	if !z.Progress.IsMaxBaseLevel(&p.progress) {
		next = z.Progress.NextBaseExp(&p.progress)
	}
	if next > 0 && int64(math.Floor(100*(float64(p.progress.BaseExp)/float64(next)))) < expCostRedemptio {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (redemptio) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if src.player != nil && *flag&1 == 0 {
		// The caster has no party.
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	if !target.dead || *flag&1 == 0 {
		return
	}
	// A party member: Resurrection at level 3.
	z.resurrect(src, target, 3)
}

// assumptio is SkillAssumptio: StatusSkillImpl, which a player cannot
// cast on a monster.
type assumptio struct{}

func (assumptio) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if src.player != nil && target.monster != nil {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	statusSkill{}.castendNoDamage(z, src, target, d, lv, flag)
}

// basilica is SkillBasilica. In renewal it is a status on the caster.
// In pre-renewal it places a 5x5 area around the caster: units inside
// cannot attack or be hit, enemies are pushed out, and the caster cannot
// move. Casting it again cancels it.
type basilica struct{}

// castBegin is Basilica's part of skill_check_condition_castbegin in
// pre-renewal: the 7x7 area around the caster has no walls and no other
// unit, unless the cast cancels a Basilica.
func (basilica) castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	if z.renewal() || src.sc.Has(sc.SCBasilica) || d.Unit == nil {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	r := d.Unit.Layout.At(lv) + 1
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if isWall(z.grid, src.x+dx, src.y+dy) {
				return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
			}
		}
	}
	// skill_count_wos: any other character in range, dead ones too.
	for _, t := range z.findEntities(&query{area: areaRadius, x: src.x, y: src.y, radius: r,
		kinds: queryPlayers | queryMonsters, dead: true}) {
		if t != src {
			return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
		}
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (basilica) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if z.renewal() {
		statusSkill{}.castendNoDamage(z, src, target, d, lv, flag)
	}
}

func (basilica) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	if z.renewal() {
		return
	}
	if src.sc.Has(sc.SCBasilica) {
		// Cancel Basilica, without paying the items again.
		z.scEnd(src, sc.SCBasilica)
		*flag |= skillNoConsumeReq
		return
	}
	// Land Protector is not ported, so no cell refuses Basilica.
	z.clearUnitGroups(src)
	z.unitSetting(src, d, lv, x, y)
	*flag |= 1
}

// isWall is map_getcell with CELL_CHKWALL: a cell that is neither
// walkable nor shootable. Cells outside the map are not walls.
func isWall(g *path.Grid, x, y int) bool {
	if x < 0 || x >= g.Width-1 || y < 0 || y >= g.Height-1 {
		return false
	}
	return g.Cells[x+y*g.Width]&(path.Walkable|path.Shootable) == 0
}

// basilicaCaster reports whether a player is the caster of a pre-renewal
// Basilica: its SC_BASILICA names itself in val4.
func (e *entity) basilicaCaster() bool {
	b := e.sc.Get(sc.SCBasilica)
	return b != nil && e.player != nil && int64(b.Val4) == int64(e.player.charID)
}

// basilicaUnitAt is the unit of a pre-renewal Basilica on a cell, or nil.
// The oldest group wins.
func (z *Zone) basilicaUnitAt(x, y int) *entity {
	var best *entity
	var bestID int64
	for id, g := range z.groups {
		if g.deleted || g.def.ID != skill.IDBasilica || best != nil && id > bestID {
			continue
		}
		for _, u := range g.units {
			if u.x == x && u.y == y {
				best, bestID = u, id
				break
			}
		}
	}
	return best
}

// basilicaOnPlace is UNT_BASILICA in skill_unit_onplace and
// skill_unit_onplace_timer: an enemy of the caster on the area is pushed
// back the way it faces. Any other unit gets SC_BASILICA, with the group
// in val3 and the caster in val4, for the group's whole time.
func (z *Zone) basilicaOnPlace(unit, t *entity) {
	g := unit.unit.group
	if t.dead || g.src == nil {
		return
	}
	if isEnemy(g.src, t) {
		z.skillBlown(g.src, t, g.def.Knockback.At(g.lv), rDir(t), false)
		return
	}
	if t.sc.Has(sc.SCBasilica) {
		return
	}
	z.scStart4(g.src, t, sc.SCBasilica, 100, 0, 0, int(g.id), int(z.unit(g.src).Ref().ID), g.limit)
}

// basilicaRefresh runs pre-renewal Basilica's cell effects for a unit
// that moved (moved set) or whose cell gained or lost a unit:
//   - skill_unit_onplace on the Basilica unit of its cell;
//   - skill_unit_onout off the area: SC_BASILICA ends, but the caster
//     keeps its own;
//   - pc_cell_basilica for a player that moved: it has SC_BASILICA on the
//     area's cells, with no values and no end, and loses it off them.
func (z *Zone) basilicaRefresh(t *entity, moved bool) {
	if z.renewal() || t.dead || t.player == nil && t.monster == nil {
		return
	}
	x, y := t.x, t.y
	on := z.basilicaUnitAt(x, y)
	if on != nil {
		z.basilicaOnPlace(on, t)
		if t.x != x || t.y != y {
			// The push moved it, and the move ran the cells again.
			return
		}
	}
	cur := t.sc.Get(sc.SCBasilica)
	switch {
	case on == nil && cur != nil:
		if moved && t.player != nil || int64(cur.Val4) != z.unit(t).Ref().ID {
			z.scEnd(t, sc.SCBasilica)
		}
	case on != nil && cur == nil && moved && t.player != nil:
		z.scStart(t, t, sc.SCBasilica, 100, 0, sc.InfiniteTick)
	}
}

// statusEndEffects is the part of status_change_end that runs for a
// status that ends: the caster's Battle Chant and pre-renewal Basilica
// clear their areas.
func (z *Zone) statusEndEffects(e *entity, en *sc.Entry) {
	// The caster's Battle Chant clears its group.
	if en.Type == sc.SCGospel && en.Val3 != 0 {
		if g := z.groups[int64(en.Val3)]; g != nil {
			z.deleteGroup(g)
		}
	}
	if en.Type == sc.SCBasilica && !z.renewal() && en.Val3 != 0 && int64(en.Val4) == z.unit(e).Ref().ID {
		if g := z.groups[int64(en.Val3)]; g != nil {
			z.deleteGroup(g)
		}
	}
}

// sanctuaryOnPlace is UNT_SANCTUARY in skill_unit_onplace_timer: it
// heals each unit that is neither undead nor a demon, and hits the undead
// and demons among the caster's enemies, counting the hits.
func (z *Zone) sanctuaryOnPlace(unit, t *entity) {
	g := unit.unit.group
	if isUndeadOrDemon(t) {
		if isEnemy(g.src, t) && z.skillAttack(battle.TypeMagic, g.src, unit, t, g.def, g.lv, 0) != 0 {
			g.val1--
		}
		return
	}
	srcSt, st := g.src.status, t.status
	if srcSt == nil {
		srcSt = &npcStatus
	}
	if st == nil {
		return
	}
	heal := skill.CalcHeal(srcSt, st, g.def.ID, g.lv, true, z.renewal(), z.rand)
	// The Emperium and battleground monsters do not exist yet.
	if t.hp >= int64(st.MaxHP) {
		return
	}
	if statusIsImmune(t) {
		heal = 0
	}
	z.skillEffect(unit, t, z.Skills.Get(skill.IDHeal), int(heal), heal, true)
	z.healHP(t, heal)
}

// sanctuaryLayout and magnusLayout are the cells of Sanctuary and Magnus
// Exorcismus, from skill_init_unit_layout: a 5x5 square without its
// corners, and a cross of 7x7 with 3 wide arms.
var (
	sanctuaryLayout = layoutOf(
		[]int{-1, 0, 1, -2, -1, 0, 1, 2, -2, -1, 0, 1, 2, -2, -1, 0, 1, 2, -1, 0, 1},
		[]int{-2, -2, -2, -1, -1, -1, -1, -1, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 2, 2, 2})
	magnusLayout = layoutOf(
		[]int{-1, 0, 1, -1, 0, 1, -3, -2, -1, 0, 1, 2, 3, -3, -2, -1, 0, 1, 2, 3, -3, -2, -1, 0, 1, 2, 3, -1, 0, 1, -1, 0, 1},
		[]int{-3, -3, -3, -2, -2, -2, -1, -1, -1, -1, -1, -1, -1, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 2, 2, 2, 3, 3, 3})
)

// layoutOf pairs a layout's offsets.
func layoutOf(dx, dy []int) [][2]int {
	out := make([][2]int, len(dx))
	for i := range dx {
		out[i] = [2]int{dx[i], dy[i]}
	}
	return out
}
