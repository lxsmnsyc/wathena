package zone

import (
	"strconv"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// The skills of the Rogue (job 17) and the Stalker (job 4018), as the
// SkillImpl classes of rAthena's src/map/skills/thief, and the parts of
// skill.cpp, pc.cpp and unit.cpp they need:
//   - Snatcher steals on normal attacks, from skill_additional_effect.
//   - Plagiarism copies the last copyable skill that hit the player, as
//     skill_do_copy, and saves it in the character variables
//     CLONE_SKILL and CLONE_SKILL_LV, as rAthena's global registers.
//   - Gangster's Paradise needs sitting, which the zone does not have:
//     skillSit is ported, and nothing calls it yet.
//   - Flag Graffiti has no SkillImpl in rAthena: it shows its effect and
//     does nothing else, as rAthena without a guild.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDSnatcher:     rogueSkill{},
		skill.IDStealCoin:    stealCoin{},
		skill.IDBackStab:     backStab{},
		skill.IDTunnelDrive:  rogueSkill{},
		skill.IDRaid:         raid{},
		skill.IDStripWeapon:  divest{},
		skill.IDStripShield:  divest{},
		skill.IDStripArmor:   divest{},
		skill.IDStripHelm:    divest{},
		skill.IDIntimidate:   intimidate{},
		skill.IDGraffiti:     graffiti{},
		skill.IDFlagGraffiti: flagGraffiti{},
		skill.IDCleaner:      cleaner{},
		skill.IDGangster:     rogueSkill{},
		skill.IDCompulsion:   rogueSkill{},
		skill.IDPlagiarism:   rogueSkill{},
		skill.IDCloseConfine: closeConfine{},

		skill.IDChaseWalk:   chaseWalk{},
		skill.IDRejectSword: statusSkill{},
		skill.IDPreserve:    statusSkill{},
		skill.IDFullStrip:   divestAll{},
	})
}

// rogueState is what a player keeps for the Rogue's skills.
type rogueState struct {
	// cloneID and cloneLv are the skill Plagiarism copied and its level,
	// as cloneskill_idx and its skill entry with SKILL_FLAG_PLAGIARIZED.
	cloneID, cloneLv int
	// sitting is pc_issit, and gangster is state.gangsterparadise.
	// Sitting does not exist yet, so both stay false.
	sitting, gangster bool
}

// rogueSkill is a passive skill whose effect lives where rAthena reads
// it: Snatcher in the normal attack's effects, Tunnel Drive in
// status_calc_speed and the hiding move check, Gangster's Paradise in
// monster targeting, Compulsion Discount in pc_modifybuyvalue, and
// Plagiarism in skill_attack.
type rogueSkill struct{}

// snatcher is the Gank part of skill_additional_effect for a player's
// normal attack: a chance to steal from a monster, unless the player
// holds a bow.
func (z *Zone) snatcher(src, target *entity) {
	p := src.player
	if p == nil || target.monster == nil || src.status == nil || src.status.Player == nil ||
		src.status.Player.Weapon == battle.WeaponBow {
		return
	}
	lv := p.skillLevel(skill.IDSnatcher)
	if lv <= 0 {
		return
	}
	steal := p.skillLevel(skill.IDSteal)
	if lv*15+55+steal*10 <= z.rand.IntN(1000) {
		return
	}
	if z.stealItem(src, target, steal) {
		z.skillEffect(src, target, z.Skills.Get(skill.IDSteal), lv, int64(lv), true)
		return
	}
	z.skillFail(src, skill.IDSnatcher, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
}

// stealCoin is SkillMug.
type stealCoin struct{}

func (stealCoin) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p, m := src.player, target.monster
	if p == nil || m == nil || src.status == nil {
		return
	}
	targetLv := z.unit(target).Level()
	rate := 10 * p.skillLevel(skill.IDStealCoin)
	rate += src.status.Dex / 2
	rate += src.status.Luk / 2
	rate += 2 * (z.unit(src).Level() - targetLv)
	// rnd_chance_official: a roll of 0 to 20000, modulo the base.
	if z.rand.IntN(20001)%1000 >= rate {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	m.stoleCoin = true
	// rnd_value(8 * level, 10 * level).
	amount := 8*targetLv + z.rand.IntN(2*targetLv+1)
	amount += lv * targetLv / 10
	z.setZeny(p, min(p.record.Zeny+int64(amount), character.MaxZeny))
	// battle_damage with 0 damage: the effects of a ranged normal attack,
	// and the monster may turn on the caster.
	z.additionalEffect(src, target, true)
	z.counterEffect(src, target, true)
	z.mobAttacked(src, target)
	// The client shows the zeny stolen as the level.
	z.skillEffect(src, target, d, amount, int64(amount), true)
}

// mobAttacked is the mob_attacked timer of a skill that hits a monster
// without damage, such as Steal Coin: the monster learns who did it.
func (z *Zone) mobAttacked(src, target *entity) {
	if target.monster == nil || target.dead || target.status == nil || src.dead {
		return
	}
	z.mobSetAttacked(target, src, false)
}

// backStab is SkillBackStab.
type backStab struct{}

func (backStab) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	dir := mapCalcDir(src, target.x, target.y)
	if z.renewal() {
		// The caster steps to the cell behind the target.
		x, y := 0, 0
		switch {
		case dir > 0 && dir < 4:
			x = -1
		case dir > 4:
			x = 1
		}
		switch {
		case dir > 2 && dir < 6:
			y = -1
		case dir == 7 || dir < 2:
			y = 1
		}
		if !isEnemy(src, target) || !z.unitMovePos(src, target.x+x, target.y+y) {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return
		}
	} else {
		if chebyshev(src, target) == 0 {
			return
		}
		// Only from behind: the target faces the way the caster looks.
		if mapCheckDir(dir, rDir(target)) && target.unit == nil {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return
		}
	}
	z.scEnd(src, sc.SCHiding)
	// The target turns to face the caster.
	if dir < 4 {
		dir += 4
	} else {
		dir -= 4
	}
	target.dir = protocolDir(dir)
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
}

// additionalEffect is SkillBackStab::applyAdditionalEffects: renewal's
// stun.
func (backStab) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if z.renewal() {
		z.scStart(src, target, sc.SCStun, 5+2*lv, lv, int64(d.Duration1.At(lv)))
	}
}

// mapCheckDir is map_check_dir: false when two directions are the same
// or next to each other.
func mapCheckDir(sDir, tDir int) bool {
	d := (sDir - tDir + 8) % 8
	return d != 0 && d != 1 && d != 7
}

// protocolDir turns one of rAthena's directions (0 north,
// counterclockwise) into the protocol's facing (0 south, clockwise).
func protocolDir(dir int) int { return (4 - dir + 8) % 8 }

// unitMovePos is unit_movepos with a path check: the unit moves at once
// to a cell it can walk to, facing it, as a blown unit is seen.
func (z *Zone) unitMovePos(e *entity, x, y int) bool {
	if len(e.path) > 0 {
		e.path = e.path[:0]
		e.stopped = true
	}
	e.target = nil
	if x < 0 || y < 0 || x >= z.grid.Width || y >= z.grid.Height || z.grid.NoPass(x, y) {
		return false
	}
	if _, ok := path.Search(z.grid, e.x, e.y, x, y, false); !ok && (e.x != x || e.y != y) {
		return false
	}
	e.dir = protocolDir(mapCalcDir(e, x, y))
	e.x, e.y = x, y
	e.stopped = true
	z.moveBlock(e)
	return true
}

// moveBlock is the part of map_moveblock that a unit moved at once
// does: the ground units under it act, and a Close Confine hold ends.
func (z *Zone) moveBlock(e *entity) {
	z.skillUnitMove(e)
	if !e.sc.Empty() {
		z.scEnd(e, sc.SCCloseconfine)
		z.scEnd(e, sc.SCCloseconfine2)
	}
}

// raid is SkillSightlessMind: from hiding, a hit on every enemy around
// the caster.
type raid struct{}

func (raid) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.area = areaTemp{}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	// skill_area_temp[1] is 0, so every target gets the splash animation.
	for _, t := range z.areaTargets(src, target.x, target.y, d.SplashFor(lv), true) {
		z.skillAttack(d.Type, src, src, t, d, lv, *flag|sdAnimation)
	}
	z.scEnd(src, sc.SCHiding)
}

// additionalEffect is SkillSightlessMind::applyAdditionalEffects: stun,
// blindness, and in renewal SC_RAID for 10 seconds.
func (raid) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCStun, 10+3*lv, lv, int64(d.Duration1.At(lv)))
	z.scStart(src, target, sc.SCBlind, 10+3*lv, lv, int64(d.Duration2.At(lv)))
	if z.renewal() {
		z.scStart(src, target, sc.SCRaid, 100, lv, 10000)
	}
}

// divest is SkillDivestWeapon, SkillDivestShield, SkillDivestArmor and
// SkillDivestHelm.
type divest struct{}

func (divest) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.stripSkillEquip(src, target, d, lv)
	z.skillEffect(src, target, d, lv, int64(lv), ok)
	if !ok {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	}
}

// divestAll is SkillDivestAll.
type divestAll struct{}

func (divestAll) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	l := target.sc
	if src.player != nil && l.Has(scCPWeapon) && l.Has(scCPHelm) && l.Has(scCPArmor) && l.Has(scCPShield) {
		// rAthena shows a message that the coating stopped it
		// (clif_gospel_info 0x28), which the protocol does not have.
		return
	}
	ok := z.stripSkillEquip(src, target, d, lv)
	if ok {
		z.skillEffect(src, target, d, lv, int64(lv), true)
	}
	if !ok {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	}
}

// stripPositions are skill_strip_equip's pos, sc_atk and sc_def for the
// four strips: the equipment, the status that strips it, and the
// Chemical Protection that keeps it on.
var stripPositions = [4]struct {
	pos       uint32
	atk, prot int
}{
	{item.EqpWeapon, sc.SCStripweapon, scCPWeapon},
	{item.EqpShield, sc.SCStripshield, scCPShield},
	{item.EqpArmor, sc.SCStriparmor, scCPArmor},
	{item.EqpHelm, sc.SCStriphelm, scCPHelm},
}

// stripSkillEquip is skill_strip_equip for the Rogue's and the
// Stalker's strips: the chance, the duration, then the strip statuses
// on what Chemical Protection does not cover.
func (z *Zone) stripSkillEquip(src, target *entity, d *skill.Def, lv int) bool {
	sst, tst := src.status, target.status
	if sst == nil {
		sst = &npcStatus
	}
	if tst == nil {
		return false
	}
	var rate int
	mod := 1000
	var pos uint32
	switch d.ID {
	case skill.IDStripWeapon, skill.IDStripArmor, skill.IDStripShield, skill.IDStripHelm:
		rate = 50*(lv+1) + 2*(sst.Dex-tst.Dex)
	case skill.IDFullStrip:
		minRate := 50 + 20*lv
		rate = max(minRate, minRate+2*(sst.Dex-tst.Dex))
	default:
		return false
	}
	if z.rand.IntN(mod) >= rate {
		return false
	}
	time := d.Duration1.At(lv)
	if target.player == nil {
		time += 15000
	}
	time += max(1, lv+500*(sst.Dex-tst.Dex))
	switch d.ID {
	case skill.IDStripWeapon:
		pos = item.EqpWeapon
	case skill.IDStripArmor:
		pos = item.EqpArmor
	case skill.IDStripShield:
		pos = item.EqpShield
	case skill.IDStripHelm:
		pos = item.EqpHelm
	case skill.IDFullStrip:
		pos = item.EqpWeapon | item.EqpShield | item.EqpArmor | item.EqpHelm
	}
	for _, s := range stripPositions {
		if pos&s.pos != 0 && target.sc.Has(s.prot) {
			pos &^= s.pos
		}
	}
	if pos == 0 {
		return false
	}
	for _, s := range stripPositions {
		if pos&s.pos != 0 && !z.scStart(src, target, s.atk, 100, lv, int64(time)) {
			pos &^= s.pos
		}
	}
	return pos != 0
}

// stripEquip is the part of status_change_start for the strip statuses
// on a player: the equipment comes off (pc_unequipitem), unless the
// player wears none or a bonus keeps it on.
func (z *Zone) stripEquip(e *entity, typ int) bool {
	p := e.player
	if p == nil || p.inventory == nil || z.Items == nil {
		return false
	}
	off := func(eqi int, ok func(d *item.Def) bool) bool {
		slot := p.inventory.Worn(z.Items).Index[eqi]
		if slot < 0 {
			return false
		}
		d, found := z.Items.Lookup(p.inventory.Slots[slot].ID)
		if !found || !ok(d) {
			return false
		}
		z.unequipItem(p, slot)
		return true
	}
	any := func(*item.Def) bool { return true }
	unstrip := p.bonuses.Unstripable
	switch typ {
	case sc.SCStripweapon:
		if unstrip&item.EqpWeapon != 0 {
			return false
		}
		// A left-hand weapon comes off too.
		left := off(item.EqiHandL, (*item.Def).IsWeapon)
		right := off(item.EqiHandR, (*item.Def).IsWeapon)
		return left || right
	case sc.SCStripshield:
		if unstrip&item.EqpShield != 0 {
			return false
		}
		return off(item.EqiHandL, (*item.Def).IsArmor)
	case sc.SCStriparmor:
		if unstrip&item.EqpArmor != 0 {
			return false
		}
		return off(item.EqiArmor, any)
	case sc.SCStriphelm:
		if unstrip&item.EqpHelm != 0 {
			return false
		}
		return off(item.EqiHeadTop, any)
	}
	return false
}

// stripBlocksEquip is the part of pc_isequip for the strip statuses: a
// stripped player cannot put on what was stripped.
func stripBlocksEquip(l *sc.List, d *item.Def) bool {
	if l.Empty() {
		return false
	}
	loc := uint32(d.Location)
	switch {
	case loc&item.EqpArms != 0 && d.IsWeapon() && l.Has(sc.SCStripweapon):
		return true
	case loc&item.EqpShield != 0 && d.IsArmor() && l.Has(sc.SCStripshield):
		return true
	case loc&item.EqpArmor != 0 && l.Has(sc.SCStriparmor):
		return true
	case loc&item.EqpHeadTop != 0 && l.Has(sc.SCStriphelm):
		return true
	}
	return false
}

// intimidate is SkillSnatch: a weapon hit that may take the caster and
// the target to a random cell.
type intimidate struct{}

func (intimidate) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	damage := z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
	// The part of skill_attack after the damage.
	if damage <= 0 || target.status == nil || target.status.Mode&modeStatusImmune != 0 {
		return
	}
	rate := 50 + lv*5
	rate += z.unit(src).Level() - z.unit(target).Level()
	if z.rand.IntN(100) < rate {
		z.addTimerSkill(src, z.now+800, target, 0, 0, d, lv, 0, *flag)
	}
}

// timerSkill is skill_timerskill for Intimidate: the caster warps to a
// random cell, and the target to a cell next to it. A target that left
// or is out of sight warps nobody but the caster.
func (intimidate) timerSkill(z *Zone, t *timerSkill) {
	src, target := t.src, t.target
	if target == nil || !z.entities.has(target) || chebyshev(src, target) > ViewRange {
		target = src
	}
	if src.dead {
		return
	}
	if !z.unitWarp(src, -1, -1) {
		return
	}
	x, y, _ := z.searchFreeCell(src.x, src.y, 1, 1)
	if target != src && !target.dead {
		z.unitWarp(target, x, y)
	}
}

// mapEdgeSize is map_edge_size: map-wide searches keep this far from
// the edges.
const mapEdgeSize = 15

// rndValue is rnd_value: a number from lo to hi.
func (z *Zone) rndValue(lo, hi int) int {
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo + z.rand.IntN(hi-lo+1)
}

// searchFreeCell is map_search_freecell with flag 1 and 50 tries: a
// random walkable cell within rx and ry of a cell, or anywhere on the map
// away from the edges for a negative range. It never picks the cell
// itself, and gives it back when no try finds one.
func (z *Zone) searchFreeCell(bx, by, rx, ry int) (int, int, bool) {
	w, h := z.grid.Width, z.grid.Height
	edgeValid := min(mapEdgeSize, 5)
	for range 50 {
		var x, y int
		if rx >= 0 {
			x = z.rndValue(bx-rx, bx+rx)
		} else {
			x = z.rndValue(mapEdgeSize, w-mapEdgeSize-1)
		}
		if ry >= 0 {
			y = z.rndValue(by-ry, by+ry)
		} else {
			y = z.rndValue(mapEdgeSize, h-mapEdgeSize-1)
		}
		if x == bx && y == by {
			continue
		}
		if x < edgeValid || x > w-edgeValid || y < edgeValid || y > h-edgeValid {
			continue
		}
		if x < w && y < h && !z.grid.NoPass(x, y) {
			return x, y, true
		}
	}
	return bx, by, false
}

// unitWarp is unit_warp within the map: a negative cell picks a random
// one, and a cell that cannot be walked on one within 4 cells of it.
func (z *Zone) unitWarp(e *entity, x, y int) bool {
	if x < 0 || y < 0 {
		var ok bool
		if x, y, ok = z.searchFreeCell(-1, -1, -1, -1); !ok {
			return false
		}
	} else if x >= z.grid.Width || y >= z.grid.Height || z.grid.NoPass(x, y) {
		var ok bool
		if x, y, ok = z.searchFreeCell(x, y, 4, 4); !ok {
			return false
		}
	}
	if p := e.player; p != nil {
		z.teleport(p, x, y)
		return true
	}
	z.moveTo(e, x, y)
	return true
}

// graffiti is SkillScribble: a unit with text on the ground. Only one
// Graffiti may be on the map.
type graffiti struct{}

func (graffiti) castBegin(z *Zone, _ *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	if z.graffitiRemover(nil, false) > 0 {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (graffiti) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	z.unitSetting(src, d, lv, x, y)
	*flag |= 1
}

// unitGraffiti is UNT_GRAFFITI.
const unitGraffiti = 0xb0

// graffitiRemover is skill_graffitiremover over the map's skill units:
// it counts the Graffiti units, in the square around a cell when area is
// set, and deletes them when remove is set.
func (z *Zone) graffitiRemover(area *[4]int, remove bool) int {
	var found []*entity
	for e := range z.entities.all() {
		u := e.unit
		if u == nil || u.group.unitID != unitGraffiti {
			continue
		}
		if area != nil && (e.x < area[0] || e.y < area[1] || e.x > area[2] || e.y > area[3]) {
			continue
		}
		found = append(found, e)
	}
	if remove {
		for _, e := range found {
			z.deleteUnit(e)
		}
	}
	return len(found)
}

// flagGraffiti is RG_FLAGGRAFFITI. rAthena has no SkillImpl for it:
// skill_castend_pos2 shows the effect and stops.
type flagGraffiti struct{}

func (flagGraffiti) castendPos(*Zone, *entity, int, int, *skill.Def, int, *int) {}

// cleaner is SkillRemover: it removes every Graffiti in its area.
type cleaner struct{}

func (cleaner) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

func (cleaner) castendPos(z *Zone, _ *entity, x, y int, d *skill.Def, lv int, _ *int) {
	i := d.SplashFor(lv)
	z.graffitiRemover(&[4]int{x - i, y - i, x + i, y + i}, true)
}

// closeConfine is SkillCloseConfine: the target and the caster cannot
// move.
type closeConfine struct{}

func (closeConfine) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart4(src, target, d.Status, 100, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// chaseWalk is SkillStealth: a toggle that keeps its requirements when
// it ends. The client shows no level.
type chaseWalk struct{}

func (chaseWalk) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if target.sc.Has(d.Status) {
		z.skillEffect(src, target, d, -1, -1, z.scEnd(target, d.Status))
		*flag |= skillNoConsumeReq
		return
	}
	z.skillEffect(src, target, d, -1, -1, z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv))))
}

// skillSit is skill_sit for Gangster's Paradise: when a player with the
// skill sits, and another player with it sits within its area, they all
// get its protection. When one stands up, the others keep it only while
// two still sit together. Sitting does not exist yet, so nothing calls
// it.
func (z *Zone) skillSit(p *Player, sitting bool) {
	lv := p.skillLevel(skill.IDGangster)
	if lv <= 0 || z.Skills == nil {
		return
	}
	r := z.Skills.Get(skill.IDGangster).SplashFor(lv)
	sitters := func(e *entity) []*Player {
		var out []*Player
		for _, t := range z.findEntities(&query{area: areaRadius, x: e.x, y: e.y, radius: r, kinds: queryPlayers,
			dead: true}) {
			if t.player.rogue.sitting && t.player.skillLevel(skill.IDGangster) > 0 {
				out = append(out, t.player)
			}
		}
		return out
	}
	if sitting {
		if s := sitters(p.entity); len(s) > 1 {
			for _, o := range s {
				o.rogue.gangster = true
			}
		}
		return
	}
	for _, t := range z.findEntities(&query{area: areaRadius, x: p.entity.x, y: p.entity.y, radius: r,
		kinds: queryPlayers, dead: true}) {
		o := t.player
		if !o.rogue.sitting || len(sitters(t)) > 1 {
			continue
		}
		o.rogue.gangster = false
	}
}

// gangsterProtects is the Gangster's Paradise check of mob_ai: a monster
// that is not a boss does not attack a player under its protection.
func gangsterProtects(m, t *entity) bool {
	return t.player != nil && t.player.rogue.gangster && m.monster != nil && m.status != nil &&
		m.status.Mode&modeStatusImmune == 0
}

// The character variables that keep the skill Plagiarism copied, as
// rAthena's SKILL_VAR_PLAGIARISM and SKILL_VAR_PLAGIARISM_LV.
var (
	varPlagiarism   = character.Variable{Mod: "core", Name: "CLONE_SKILL"}
	varPlagiarismLv = character.Variable{Mod: "core", Name: "CLONE_SKILL_LV"}
)

// skillIsCopyable is skill_isCopyable for Plagiarism: a skill the player
// does not have, or the copy it holds, whose CopyFlags name Plagiarism,
// while the player has Plagiarism and no Preserve.
func (z *Zone) skillIsCopyable(p *Player, id int) bool {
	if p.rogue.cloneID != id && z.hasSkillEntry(p, id) {
		return false
	}
	return skill.CopyableByPlagiarism(id) && p.skillLevel(skill.IDPlagiarism) > 0 && !p.entity.sc.Has(sc.SCPreserve)
}

// hasSkillEntry reports whether a skill is in the player's skill list,
// learned or not, as a status.skill entry that pc_calc_skilltree filled.
func (z *Zone) hasSkillEntry(p *Player, id int) bool {
	if p.char.Skills[id] > 0 {
		return true
	}
	if !z.skillsReady() {
		return false
	}
	entries, err := z.SkillTrees.List(z.Skills, z.Status, z.learner(p), z.SkillSettings)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Skill == id {
			return true
		}
	}
	return false
}

// skillDoCopy is skill_do_copy for Plagiarism: the player copies a skill
// that hit it, at most at its Plagiarism level, in place of the last
// one.
func (z *Zone) skillDoCopy(target *entity, d *skill.Def, lv int) {
	p := target.player
	if p == nil || p.skillLevel(skill.IDPlagiarism) <= 0 || target.sc.Has(sc.SCPreserve) {
		return
	}
	if !z.skillIsCopyable(p, d.ID) {
		return
	}
	z.plagiarismReset(p)
	lv = min(lv, p.skillLevel(skill.IDPlagiarism))
	z.setPlagiarism(p, d.ID, lv)
}

// setPlagiarism holds a copied skill and saves it, as the end of
// skill_do_copy and pc_skill_plagiarism.
func (z *Zone) setPlagiarism(p *Player, id, lv int) {
	p.rogue.cloneID, p.rogue.cloneLv = id, lv
	m := p.varMap("character")
	m[varPlagiarism] = strconv.Itoa(id)
	m[varPlagiarismLv] = strconv.Itoa(lv)
	z.sendSkillList(p)
}

// plagiarismReset is pc_skill_plagiarism_reset for Plagiarism: the
// copied skill goes, and its variables with it.
func (z *Zone) plagiarismReset(p *Player) {
	if p.rogue.cloneID == 0 {
		return
	}
	p.rogue.cloneID, p.rogue.cloneLv = 0, 0
	delete(p.varMap("character"), varPlagiarism)
	delete(p.varMap("character"), varPlagiarismLv)
	z.sendSkillList(p)
}

// loadPlagiarism is the Plagiarism part of pc_reg_received: a player
// with Plagiarism gets back the skill it copied, at most at its
// Plagiarism level.
func (z *Zone) loadPlagiarism(p *Player, vars map[character.Variable]string) {
	plv := p.skillLevel(skill.IDPlagiarism)
	if plv <= 0 {
		return
	}
	id, err := strconv.Atoi(vars[varPlagiarism])
	if err != nil || id <= 0 || z.Skills == nil || z.Skills.Get(id) == nil {
		return
	}
	lv, _ := strconv.Atoi(vars[varPlagiarismLv])
	p.rogue.cloneID, p.rogue.cloneLv = id, min(lv, plv)
}

// copiedSkillInfo is the copied skill's entry of the skill list, as
// clif_addskill shows it, or nil without one.
func (z *Zone) copiedSkillInfo(p *Player) *wathenav1.SkillInfo {
	id := p.rogue.cloneID
	if id == 0 || z.Skills == nil {
		return nil
	}
	d := z.Skills.Get(id)
	if d == nil {
		return nil
	}
	lv := max(p.rogue.cloneLv, 1)
	return wathenav1.SkillInfo_builder{
		SkillId:     uint32(d.ID),            // #nosec G115 -- skill IDs are positive.
		Level:       uint32(p.rogue.cloneLv), // #nosec G115 -- a small level.
		MaxLevel:    uint32(p.rogue.cloneLv), // #nosec G115 -- as above.
		Name:        d.Name,
		DisplayName: d.DisplayName,
		Target:      skillTarget(d.Target),
		Sp:          uint32(max(d.Requires.SP.At(lv), 0)),            // #nosec G115 -- checked.
		Range:       uint32(max(d.RangeFor(p.entity.status, lv), 0)), // #nosec G115 -- checked.
		Icon:        uint32(d.ID),                                    // #nosec G115 -- as above.
	}.Build()
}

// rejectSword is the zone's part of Reject Sword in
// battle_calc_weapon_final_atk_modifiers: the target shows the skill,
// the attacker takes the halved damage, and the status counts one
// reflection.
func (z *Zone) rejectSword(src, target *entity, res battle.Result) {
	if res.Rejected <= 0 || z.Skills == nil {
		return
	}
	e := target.sc.Get(sc.SCRejectsword)
	if e == nil {
		return
	}
	z.skillEffect(target, target, z.Skills.Get(skill.IDRejectSword), e.Val1, int64(e.Val1), true)
	z.fixDamage(target, src, res.Rejected, res.Hits)
	if target.dead {
		return
	}
	e.Val3--
	if e.Val3 <= 0 {
		z.scEnd(target, sc.SCRejectsword)
	}
}

// fixDamage is battle_fix_damage: damage from src that no defense
// lowers, shown as a normal hit.
func (z *Zone) fixDamage(src, target *entity, damage int64, hits int) {
	if target.dead || target.status == nil || damage <= 0 {
		return
	}
	z.logDamage(src, target, damage)
	target.hp -= damage
	target.hpChanged = true
	z.hits = append(z.hits, hit{source: src, target: target, result: battle.Result{Kind: battle.Hit, Damage: damage,
		Hits: max(hits, 1)}, hpPermille: target.hpPermille()})
	z.damaged(target, damage, src.monster != nil)
	z.castCancel(target, true)
	if target.hp <= 0 {
		z.die(target, src)
	}
}
