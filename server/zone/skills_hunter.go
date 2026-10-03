package zone

import (
	"cmp"
	"maps"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The Hunter's skills: the SkillImpl classes of rAthena's
// src/map/skills/archer, the trap units of skill.cpp (skill_unitsetting,
// skill_unit_onplace_timer, skill_trap_splash and skill_unit_timer_sub),
// the falcon (pc_setfalcon) and the automatic Blitz Beat of normal
// attacks (skill_additional_effect).
//
// Talkie Box has no message: the client cannot send one with the cast,
// and cannot show one when the box is set off (clif_talkiebox).
// A trap's change of look (clif_changetraplook) is not shown either: new
// viewers see the used trap, viewers that knew it keep the old look.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDSkidTrap:     trapSkill{},
		skill.IDLandMine:     landMine{},
		skill.IDAnkleSnare:   trapSkill{},
		skill.IDShockwave:    shockwaveTrap{},
		skill.IDSandman:      sandman{},
		skill.IDFlasher:      flasher{},
		skill.IDFreezingTrap: freezingTrap{},
		skill.IDBlastMine:    trapSkill{},
		skill.IDClaymoreTrap: trapSkill{},
		skill.IDRemoveTrap:   removeTrap{},
		skill.IDTalkieBox:    trapSkill{},
		skill.IDBlitzBeat:    splashSkill{},
		skill.IDDetecting:    detect{},
		skill.IDSpringTrap:   springTrap{},
		skill.IDBeastStrafe:  beastStrafing{},
		skill.IDPhantasmic:   attackSkill{},
	})
}

// Values of the traps.
const (
	// itemTrap is ITEMID_TRAP (Booby Trap), and itemTrapAlloy
	// ITEMID_TRAP_ALLOY (Special Alloy Trap).
	itemTrap      = 1065
	itemTrapAlloy = 7940
	// trapsSettingRenewal is battle_config.traps_setting in renewal; it
	// is 0 in pre-renewal. With 2, traps of UF_HIDDENTRAP are hidden on
	// every map.
	trapsSettingRenewal = 2
	// usedTrapTime is how long a trap that went off stays.
	usedTrapTime = 1500
	idVenomDust  = 140 // AS_VENOMDUST
)

// trapState is what a trap's skill_unit_group and its unit hold besides
// the values every group has.
type trapState struct {
	// casterX and casterY are Skid Trap's val1: the caster's cell when
	// it set the trap.
	casterX, casterY int
	// caught is Ankle Snare's val2: the unit it holds, or nil.
	caught *entity
	// item is item_id: the trap item that set the trap.
	item uint32
	// hidden is the unit's hidden: only the caster sees the trap.
	hidden bool
	// spoken is Talkie Box's val2 of -1: the box spoke its message. The
	// message itself (valstr) is not kept: the client cannot send one.
	spoken bool
}

// trapSetting is skill_unitsetting's part for traps: Skid Trap keeps the
// caster's cell, the trap item that set the trap is kept, and with
// traps_setting 2 a trap of UF_HIDDENTRAP is hidden.
func (z *Zone) trapSetting(src *entity, d *skill.Def, lv int) *trapState {
	t := &trapState{}
	if d.ID == skill.IDSkidTrap {
		t.casterX, t.casterY = src.x, src.y
	}
	if src.player != nil {
		for _, it := range z.requirement(src, d, lv).Items {
			if it[0] == itemTrap || it[0] == itemTrapAlloy {
				t.item = uint32(it[0]) // #nosec G115 -- item IDs are positive.
				break
			}
		}
	}
	setting := 0
	if z.renewal() {
		setting = trapsSettingRenewal
	}
	t.hidden = unitFlag(d, "HiddenTrap") && setting == 2
	return t
}

// trapHiddenFrom reports whether a hidden trap is hidden from a viewer,
// as skill_getareachar_skillunit_visibilty_single: only its caster and
// the caster's party see it.
func trapHiddenFrom(e, viewer *entity) bool {
	g := e.unit.group
	return g.trap != nil && g.trap.hidden && g.src != viewer && !sameParty(battleMaster(g.src), viewer)
}

// trapUsed reports whether a trap went off (UNT_USED_TRAPS).
func trapUsed(g *unitGroup) bool { return g.unitID == untUsedTraps }

// useTrap turns a trap into a used trap that ends after ms
// (clif_changetraplook with UNT_USED_TRAPS and the new limit).
func (z *Zone) useTrap(e *entity, ms int64) {
	g := e.unit.group
	g.unitID = untUsedTraps
	g.limit = z.now - g.start + ms
}

// trapCatches is skill_unit_timer_sub_onplace's check for traps: in
// renewal, Ankle Snare and Skid Trap no longer catch bosses.
func (z *Zone) trapCatches(g *unitGroup, t *entity) bool {
	if !z.rules().BossProtection || trapUsed(g) || (g.def.ID != skill.IDAnkleSnare && g.def.ID != skill.IDSkidTrap) {
		return true
	}
	return t.status == nil || t.status.Mode&modeStatusImmune == 0
}

// trapOnPlaceTimer is skill_unit_onplace_timer for the traps.
func (z *Zone) trapOnPlaceTimer(e, t *entity) {
	g := e.unit.group
	tr := g.trap
	if trapUsed(g) || tr == nil {
		return
	}
	ss := g.src
	switch g.def.ID {
	case skill.IDSkidTrap:
		// Knockback away from the caster's cell when it set the trap.
		z.skillBlown(e, t, g.def.Knockback.At(g.lv), mapCalcDir(t, tr.casterX, tr.casterY), false)
		z.useTrap(e, usedTrapTime)
		if z.skillRenewal(skill.IDSkidTrap) {
			// The target stops for Duration2.
			z.scStart(ss, t, sc.SCStop, 100, 0, int64(g.def.Duration2.At(g.lv)))
		} else if t.monster != nil {
			// mob_unlocktarget: the monster stops and idles.
			t.target = nil
		}
	case skill.IDAnkleSnare:
		z.ankleSnare(e, t)
	case skill.IDTalkieBox:
		if ss == t || tr.spoken {
			return
		}
		// clif_talkiebox: the client cannot show the message yet.
		z.useTrap(e, 5000)
		tr.spoken = true
	default:
		z.trapGoOff(e, t)
	}
}

// ankleSnare is UNT_ANKLESNARE of skill_unit_onplace_timer: the trap
// holds the target for Duration2, shortened by its AGI down to a minimum
// by the caster's level, and stays as long as it holds.
func (z *Zone) ankleSnare(e, t *entity) {
	u := e.unit
	g := u.group
	tr := g.trap
	if tr.caught != nil {
		return
	}
	en := z.statusEngine()
	if en == nil || t.status == nil {
		return
	}
	ss := g.src
	sec := int64(g.def.Duration2.At(g.lv))
	mintime := int64(30 * (z.unit(ss).Level() + 100))
	if !z.rules().BossProtection && t.status.Mode&modeStatusImmune != 0 {
		// Bosses cannot set it off in renewal.
		sec /= 5
	}
	sec = max(sec*int64(t.status.Agi)/-200+sec, mintime)
	su := z.unit(ss)
	if en.Start(su, z.unit(t), sc.Start{Type: sc.SCAnkle, Rate: 10000, Val1: g.lv, Val2: int(g.id), Duration: sec,
		Flag: sc.NoRateDef, Reason: sc.ReasonSkill, Source: su.Ref()}) {
		if s := t.sc.Get(sc.SCAnkle); s != nil {
			sec = s.Remaining(z.now)
		}
		if !blownImmune(t) {
			// unit_movepos: the target moves onto the trap.
			t.path = t.path[:0]
			t.x, t.y = e.x, e.y
			t.stopped = true
			z.skillUnitMove(t)
		}
		tr.caught = t
	} else {
		// It could not hold the target.
		sec = 3000
	}
	// A hidden trap shows once it holds a unit.
	tr.hidden = false
	g.limit = z.now - g.start + sec
	g.interval = -1
	u.rng = 0
}

// blownImmune is unit_blown_immune with the offensive flag (0x1): a
// monster immune to knockback, or a player with bNoKnockback.
func blownImmune(t *entity) bool {
	if t.monster != nil {
		return t.status != nil && t.status.Mode&modeKnockbackImm != 0
	}
	return t.player != nil && t.player.bonuses.NoKnockback
}

// trapGoOff is the damage and status traps of skill_unit_onplace_timer:
// Land Mine, Blast Mine, Shockwave Trap, Sandman, Flasher, Freezing Trap
// and Claymore Trap. A trap without splash acts on the unit that set it
// off; the others act on the units around the trap, or around that unit
// for Sandman and Freezing Trap. multi_trigger_trap is off: the trap is
// used at once.
func (z *Zone) trapGoOff(e, t *entity) {
	g := e.unit.group
	d, ss := g.def, g.src
	if d.ID == skill.IDFlasher && t.monster != nil && t.status != nil && t.status.Class == battle.ClassBoss &&
		t.status.Race == battle.RacePlant {
		// Flasher does not go off on plant bosses.
		return
	}
	center := e
	if d.ID == skill.IDSandman || d.ID == skill.IDFreezingTrap {
		center = t
	}
	if splash := d.SplashFor(g.lv); splash == 0 {
		if d.NK.Has(battle.NKNoDamage) {
			z.trapEffect(ss, t, d, g.lv)
		} else {
			z.skillAttack(d.Type, ss, e, t, d, g.lv, 0)
		}
	} else {
		z.trapSplash(e, center, splash)
	}
	if z.entities.has(e) {
		z.useTrap(e, usedTrapTime)
	}
}

// trapEffect is skill_additional_effect of a trap on a unit: the trap's
// status, as a misc hit that connected.
func (z *Zone) trapEffect(ss, t *entity, d *skill.Def, lv int) {
	if t.dead {
		return
	}
	z.skillAdditionalEffect(ss, t, d, lv, battle.Result{Kind: battle.Hit, Flag: battle.FlagMisc}, true)
}

// trapSplash is map_foreachinrange with skill_trap_splash: every enemy
// of the caster around center. Claymore Trap also sets off the traps
// around it. Damage that splits counts the enemies around the trap.
func (z *Zone) trapSplash(e, center *entity, splash int) {
	g := e.unit.group
	d, ss := g.def, g.src
	if d.ID == skill.IDClaymoreTrap {
		// Traps it hits are used up. Units take no damage here.
		for _, og := range sortedGroups(z.groups) {
			if og == g || trapUsed(og) || !og.def.Has("IsTrap") {
				continue
			}
			for _, ou := range og.units {
				if abs(ou.x-center.x) <= splash && abs(ou.y-center.y) <= splash {
					switch og.def.ID {
					case skill.IDClaymoreTrap, skill.IDLandMine, skill.IDBlastMine, skill.IDShockwave, skill.IDSandman,
						skill.IDFlasher, skill.IDFreezingTrap:
						z.useTrap(ou, usedTrapTime)
					}
					break
				}
			}
		}
	}
	targets := z.findEntities(&query{area: areaRadius, x: center.x, y: center.y, radius: splash,
		kinds: queryPlayers | queryMonsters})
	for _, t := range targets {
		if !checkTarget(ss, t, g.targetFlag) {
			continue
		}
		switch d.ID {
		case skill.IDSandman:
			z.trapEffect(ss, t, d, g.lv)
		default:
			split := 0
			if d.NK.Has(battle.NKSplashSplit) {
				// map_foreachinallrange around the trap with BCT_ENEMY.
				n := 0
				for _, o := range z.findEntities(&query{area: areaRadius, x: e.x, y: e.y, radius: splash,
					kinds: queryPlayers | queryMonsters}) {
					if checkTarget(ss, o, bctEnemy) {
						n++
					}
				}
				split = max(1, n)
			}
			z.skillAttack(d.Type, ss, e, t, d, g.lv, split)
		}
	}
}

// sortedGroups are the zone's unit groups in the order they were made.
func sortedGroups(groups map[int64]*unitGroup) []*unitGroup {
	out := slices.Collect(maps.Values(groups))
	slices.SortFunc(out, func(a, b *unitGroup) int { return cmp.Compare(a.id, b.id) })
	return out
}

// trapExpired is skill_unit_timer_sub for a trap whose time ran out. It
// reports whether it handled the unit:
//   - Blast Mine, and Claymore Trap in renewal, turn into used traps for
//     1.5 seconds.
//   - An Ankle Snare that held a unit ends without its item.
//   - Other traps that never went off leave their trap item on the cell,
//     for a player's trap, and end.
func (z *Zone) trapExpired(e *entity) bool {
	u := e.unit
	g := u.group
	if trapUsed(g) {
		return false
	}
	switch g.def.ID {
	case skill.IDBlastMine, skill.IDClaymoreTrap:
		if g.def.ID == skill.IDBlastMine || z.skillRenewal(skill.IDClaymoreTrap) {
			g.unitID = untUsedTraps
			g.limit = z.now + usedTrapTime - g.start
			u.limit = z.now + usedTrapTime - g.start
			return true
		}
	case skill.IDAnkleSnare:
		if g.trap.caught != nil {
			z.deleteUnit(e)
			return true
		}
	}
	switch g.def.ID {
	case skill.IDAnkleSnare, skill.IDSkidTrap, skill.IDLandMine, skill.IDShockwave, skill.IDSandman, skill.IDFlasher,
		skill.IDFreezingTrap, skill.IDClaymoreTrap, skill.IDTalkieBox:
		if g.src != nil && g.src.player != nil {
			// The trap turns back into its item.
			z.addFloorItem(trapItem(g), 1, e.x, e.y)
		}
		z.deleteUnit(e)
		return true
	}
	return false
}

// trapItem is the item a trap gives back: the one that set it, or a
// Booby Trap.
func trapItem(g *unitGroup) uint32 {
	if g.trap != nil && g.trap.item != 0 {
		return g.trap.item
	}
	return itemTrap
}

// addFloorItem is map_addflooritem with no looters near a cell.
func (z *Zone) addFloorItem(id uint32, amount, x, y int) {
	if z.Items == nil {
		return
	}
	if _, ok := z.Items.Lookup(id); !ok {
		return
	}
	lifetime := floorItemLifetime
	if z.Progress != nil {
		lifetime = z.Progress.FloorItemLifetime
	}
	pl := progression.PlaceDrop(z.cellFree, x, y, z.rand)
	z.entities.add(&entity{x: pl.X, y: pl.Y, item: &groundItem{id: id, amount: amount, subX: pl.SubX, subY: pl.SubY,
		expires: z.now + lifetime, fresh: true}})
}

// trapSkill is the traps' castendPos2: flag 1 keeps the ammunition, and
// the trap's unit is set.
type trapSkill struct{}

func (trapSkill) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	*flag |= 1
	z.unitSetting(src, d, lv, x, y)
}

// landMine is SkillLandMine: a misc hit that may stun.
type landMine struct{ trapSkill }

func (landMine) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStartDelayed(src, target, sc.SCStun, 10, lv, int64(d.Duration2.At(lv)), 1000)
}

// shockwaveTrap is SkillShockwaveTrap: it drains 20% + 15% per level of
// the target's maximum SP (status_percent_damage).
type shockwaveTrap struct{ trapSkill }

func (shockwaveTrap) additionalEffect(z *Zone, _, target *entity, _ *skill.Def, lv int) {
	p := target.player
	if p == nil || target.status == nil {
		// Monsters have no SP to lose.
		return
	}
	rate := int64(15*lv + 5)
	sp := int64(target.status.MaxSP) * rate / 100
	if rate >= 100 {
		sp = int64(target.status.MaxSP)
	}
	if sp == 0 {
		sp = 1
	}
	// status_damage of SP alone ends no status that damage ends.
	scEnv{z}.Damage(z.unit(target), 0, sp, true, false)
}

// sandman is SkillSandman: sleep at 40% + 10% per level.
type sandman struct{ trapSkill }

func (sandman) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStartDelayed(src, target, sc.SCSleep, 10*lv+40, lv, int64(d.Duration2.At(lv)), 1000)
}

// flasher is SkillFlasher: blindness.
type flasher struct{ trapSkill }

func (flasher) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStartDelayed(src, target, sc.SCBlind, 100, lv, int64(d.Duration2.At(lv)), 1000)
}

// freezingTrap is SkillFreezingTrap: a weapon hit that freezes after the
// caster's attack motion.
type freezingTrap struct{ trapSkill }

func (freezingTrap) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	delay := int64(100)
	if src.status != nil {
		delay += int64(src.status.AMotion)
	}
	z.scStartDelayed(src, target, sc.SCFreeze, 100, lv, int64(d.Duration2.At(lv)), delay)
}

// scStartDelayed is sc_start with its delay: the status starts that long
// after the resistance check.
func (z *Zone) scStartDelayed(src, target *entity, typ, rate, val1 int, duration, delay int64) bool {
	en := z.statusEngine()
	if en == nil {
		return false
	}
	su := z.unit(src)
	return en.Start(su, z.unit(target), sc.Start{Type: typ, Rate: rate * 100, Val1: val1, Duration: duration,
		Delay: delay, Reason: sc.ReasonSkill, Source: su.Ref()})
}

// removeTrap is SkillRemoveTrap: a player takes back its own trap. A
// trap that did not go off gives back one trap item
// (skill_removetrap_type 0); skill_drop_items_full is off, so an item
// that does not fit is lost.
type removeTrap struct{}

func (removeTrap) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	u := target.unit
	if u == nil || u.group.src != src || !u.group.def.Has("IsTrap") {
		// Only on Versus maps may a player take others' traps.
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	g := u.group
	z.skillEffect(src, target, d, lv, int64(lv), true)
	held := g.def.ID == skill.IDAnkleSnare && g.trap != nil && g.trap.caught != nil
	if !trapUsed(g) && !held {
		z.addItem(p, trapItem(g), 1, skillOrigin(d.ID))
	}
	z.deleteUnit(target)
}

// springTrap is SkillSpringTrap: the falcon sets off a trap, which ends
// 1.5 seconds later. An Ankle Snare that holds a unit is left alone.
type springTrap struct{}

func (springTrap) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	u := target.unit
	if u == nil || trapUsed(u.group) {
		return
	}
	g := u.group
	switch g.def.ID {
	case skill.IDAnkleSnare:
		if g.trap != nil && g.trap.caught != nil {
			return
		}
	case skill.IDBlastMine, skill.IDSkidTrap, skill.IDLandMine, skill.IDShockwave, skill.IDSandman, skill.IDFlasher,
		skill.IDFreezingTrap, skill.IDClaymoreTrap, skill.IDTalkieBox:
	default:
		return
	}
	g.unitID = untUsedTraps
	g.limit = z.now + usedTrapTime - g.start
	u.limit = z.now + usedTrapTime - g.start
}

// detect is SkillDetect: hidden characters around the cell stop hiding,
// as Sight does, and hidden traps show (skill_reveal_trap_inarea, with
// traps_setting on, which is renewal's).
type detect struct{}

func (detect) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, _ *int) {
	r := d.SplashFor(lv)
	z.revealArea(&entity{x: x, y: y}, r)
	if !z.renewal() {
		return
	}
	for _, g := range z.groups {
		if g.trap == nil || !g.trap.hidden || !g.def.Has("IsTrap") {
			continue
		}
		for _, ue := range g.units {
			if abs(ue.x-x) <= r && abs(ue.y-y) <= r {
				g.trap.hidden = false
				break
			}
		}
	}
}

// beastStrafing is SkillBeastStrafing: Double Strafe's combo, a weapon
// hit on brutes, insects and Doram players only.
type beastStrafing struct{}

// castBegin is skill_check_condition_castbegin for HT_POWER: it needs the
// combo window of Double Strafe, and fails silently without it.
func (beastStrafing) castBegin(_ *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	if c := src.sc.Get(sc.SCCombo); c == nil || c.Val1 != skill.IDDoubleStrafe {
		return skillFailSilent
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (beastStrafing) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	st := target.status
	if st == nil {
		return
	}
	if st.Race == battle.RaceBrute || st.Race == battle.RacePlayerDoram || st.Race == battle.RaceInsect {
		z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
	}
}

// rangerJobs are the jobs of MAPID_RANGER under MAPID_THIRDMASK: Ranger,
// transcendent and baby Ranger, and Windhawk.
var rangerJobs = map[int]bool{4056: true, 4062: true, 4098: true, 4257: true}

// autoBlitzBeat is the automatic Blitz Beat of skill_additional_effect: a
// player with a falcon and a bow may cast Blitz Beat on the target of a
// normal attack, by LUK, at a level its job level allows.
func (z *Zone) autoBlitzBeat(src, target *entity) {
	p := src.player
	if p == nil || !isFalcon(p) || src.status == nil || src.status.Player == nil ||
		src.status.Player.Weapon != item.WeaponBow || z.Skills == nil {
		return
	}
	lv := p.skillLevel(skill.IDBlitzBeat)
	if lv <= 0 || z.rand.IntN(1000) > src.status.Luk*10/3+1 {
		return
	}
	rate := (p.progress.JobLevel + 9) / 10
	if rangerJobs[p.char.Job] {
		rate = 5
	}
	d := z.Skills.Get(skill.IDBlitzBeat)
	if d == nil {
		return
	}
	z.castendDamage(src, target, d, min(lv, rate), sdLevel)
}
