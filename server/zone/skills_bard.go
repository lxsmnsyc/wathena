package zone

import (
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// The skills of the Bard, the Dancer, the Clown and the Gypsy, as the
// SkillImpl classes in rAthena's src/map/skills/archer. The two modes
// differ:
//   - Renewal's songs are statuses that skill_castend_song starts on the
//     caster and its party, or on the enemies around for the songs that
//     hurt. Parties do not exist yet, so a buff reaches the caster alone,
//     as rAthena does for a player without a party.
//   - Pre-renewal's songs and dances are ground units around the
//     performer, who gets SC_DANCING. The units follow the performer, act
//     on whoever stands in them, and cost SP every few seconds.
//
// Ensembles need a partner of the other performer job next to the
// caster, in the same party (skill_check_pc_partner). Parties do not
// exist yet, so no partner is ever found and the ensembles fail before
// they start, as rAthena does for a player without a party. Their
// effects are ported for when parties come.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDAdaptation:      adaptation{},
		skill.IDEncore:          encore{},
		skill.IDLullaby:         ensemble{lullaby{}},
		skill.IDRichmanKim:      ensemble{},
		skill.IDEternalChaos:    ensemble{},
		skill.IDDrumBattlefield: ensemble{},
		skill.IDRingNibelungen:  ensemble{},
		skill.IDRokisWeil:       ensemble{},
		skill.IDIntoAbyss:       ensemble{},
		skill.IDSiegfried:       ensemble{},
		skill.IDMusicalStrike:   attackSkill{},
		skill.IDDissonance:      song{},
		skill.IDFrostJoker:      frostJoker{},
		skill.IDWhistle:         song{},
		skill.IDAssassinCross:   song{},
		skill.IDPoemBragi:       song{},
		skill.IDAppleIdun:       song{},
		skill.IDThrowArrow:      attackSkill{},
		skill.IDUglyDance:       uglyDance{},
		skill.IDScream:          frostJoker{},
		skill.IDHumming:         song{},
		skill.IDDontForgetMe:    song{},
		skill.IDFortuneKiss:     song{},
		skill.IDServiceForYou:   song{},
		skill.IDArrowVulcan:     attackSkill{},
		skill.IDMoonlit:         moonlit{},
		skill.IDMarionette:      marionette{},
		skill.IDLongingFreedom:  longingFreedom{},
		skill.IDHermode:         hermode{},
		skill.IDTarotCard:       tarotCard{},
		skill.IDPangVoice:       pangVoice{},
		skill.IDWinkCharm:       winkCharm{},
		skill.IDSpecialSinger:   specialSinger{},
	})
}

// performState is what map_session_data keeps of a performer:
// skill_id_dance and skill_lv_dance, the last song or dance, which
// Encore repeats, and skill_id_old, which Encore sets so the repeat costs
// half the SP.
type performState struct {
	danceSkill, danceLv int
	skillOld            int
}

// Values that the performance skills read: BCT_WOS, and EFST_MOON, the
// icon of Moonlit Water Mill's performer.
const (
	bctWOS   = 0x400000
	efstMoon = 123
)

// UF_ flags of a group, as group->state.song_dance.
const (
	songDanceSong     = 1
	songDanceEnsemble = 2
)

// songDanceOf is group->state.song_dance for a skill's unit: 1 for a
// song or a dance, 2 for an ensemble.
func songDanceOf(d *skill.Def) int {
	n := 0
	if unitFlag(d, "Dance") || unitFlag(d, "Song") {
		n |= songDanceSong
	}
	if unitFlag(d, "Ensemble") {
		n |= songDanceEnsemble
	}
	return n
}

// song is the songs and dances, as SkillPerfectTablature and its kin:
// renewal's skill_castend_song, or pre-renewal's unit. flag 1 keeps the
// ammunition, which the group would use up.
type song struct{}

func (song) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, _ *int) {
	if z.rules().EnsembleSongs {
		z.castendSong(src, d, lv)
	}
}

func (song) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	if z.rules().EnsembleSongs {
		return
	}
	*flag |= 1
	z.unitSetting(src, d, lv, x, y)
	z.songCastEnd(src, d)
}

// songCastEnd is the end of skill_castend_id for a song in pre-renewal:
// singing blocks Adaptation for 3 seconds (skill_blockpc_start).
func (z *Zone) songCastEnd(src *entity, d *skill.Def) {
	p := src.player
	if p == nil || !d.Has("IsSong") || !src.sc.Has(sc.SCDancing) {
		return
	}
	if p.cooldowns == nil {
		p.cooldowns = map[int]int64{}
	}
	p.cooldowns[skill.IDAdaptation] = z.now + 3000
	p.pending = append(p.pending, cooldownEvent(skill.IDAdaptation, 3000))
}

// ensemble is the Bard and Dancer ensembles, as SkillLullaby and its kin:
// songs that need a partner. song is the ensemble's own song, for those
// with more than the common behavior.
type ensemble struct{ song songer }

// songer is the part of a song type an ensemble may override.
type songer interface {
	additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int)
}

func (ensemble) castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	return z.ensembleCheck(src, d, lv)
}

func (ensemble) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	song{}.castendNoDamage(z, src, target, d, lv, flag)
}

func (ensemble) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	song{}.castendPos(z, src, x, y, d, lv, flag)
}

func (e ensemble) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if e.song != nil {
		e.song.additionalEffect(z, src, target, d, lv)
	}
}

// ensembleCheck is skill_check_condition_castbegin for an ensemble
// (INF2_ISENSEMBLE): it needs a partner next to the caster.
func (z *Zone) ensembleCheck(src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	if src.player == nil {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	if len(z.ensemblePartners(src, d, &lv, 1)) < 1 {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

// ensemblePartners is skill_check_pc_partner's search for an ensemble
// (skill_check_condition_char_sub's default case): players within range
// of the other sex and a performer job, who know the skill, hold an
// instrument or a whip, are not dancing, stand, can move, and are in the
// caster's party. Parties do not exist yet, so there is no partner. The
// level becomes the average of the caster's and the partners' levels.
func (z *Zone) ensemblePartners(src *entity, d *skill.Def, lv *int, rng int) []*entity {
	p := src.player
	if p == nil || z.Status == nil {
		return nil
	}
	var out []*entity
	sum := 0
	for _, t := range z.findEntities(&query{area: areaRadius, x: src.x, y: src.y, radius: rng, kinds: queryPlayers}) {
		tp := t.player
		if t == src || t.dead || t.cant().Move || tp.char.Male == p.char.Male {
			continue
		}
		mapID, ok := z.Status.MapID(tp.char.Job)
		if !ok || mapID&idSecondMsk != mapidBardDancer {
			continue
		}
		tlv := tp.skillLevel(d.ID)
		if tlv <= 0 || t.sc.Has(sc.SCDancing) || !checkWeaponType(t, 1<<item.WeaponMusical|1<<item.WeaponWhip) {
			continue
		}
		if !sameParty(src, t) {
			continue
		}
		out = append(out, t)
		sum += tlv
	}
	*lv = (sum + *lv) / (len(out) + 1)
	return out
}

// sameParty reports whether two players are in one party. Parties do not
// exist yet.
func sameParty(_, _ *entity) bool { return false }

// Job masks that the partner checks read, as rAthena's MAPID_ values.
const (
	idSecondMsk     = 0x0FFF // MAPID_SECONDMASK
	mapidBardDancer = 0x0203 // MAPID_BARDDANCER
)

// castendSong is skill_castend_song: renewal's songs act at once on
// every unit around the caster that the song's battle check allows.
func (z *Zone) castendSong(src *entity, d *skill.Def, lv int) int {
	p := src.player
	if p == nil || !d.Has("IsSong") && !d.Has("IsEnsemble") {
		return 0
	}
	flag := bctParty
	switch d.ID {
	case skill.IDRokisWeil:
		flag = bctEnemy | bctWOS
	case skill.IDLullaby, skill.IDEternalChaos, skill.IDDissonance, skill.IDUglyDance, skill.IDDontForgetMe:
		flag = bctEnemy
	case skill.IDHermode:
		flag |= bctGuild
	}
	z.skillEffect(src, src, d, lv, int64(lv), true)
	p.perform.danceSkill, p.perform.danceLv = d.ID, lv
	if d.Has("IsEnsemble") {
		// skill_check_pc_partner with cast_flag: the partners found when
		// the cast began. There are none without parties.
		z.ensembleCast(src, d, lv)
	}
	n := 0
	r := d.SplashFor(lv)
	for _, t := range z.findEntities(&query{area: areaRadius, x: src.x, y: src.y, radius: r, kinds: queryPlayers | queryMonsters}) {
		if !checkTarget(src, t, flag) {
			continue
		}
		n++
		z.applySong(src, t, d, lv)
	}
	return n
}

// applySong is skill_apply_songs for one target.
func (z *Zone) applySong(src, t *entity, d *skill.Def, lv int) {
	switch d.ID {
	case skill.IDDissonance:
		z.skillAttack(battle.TypeMagic, src, src, t, d, lv, 0)
	case skill.IDUglyDance, skill.IDLullaby:
		z.skillAdditionalEffect(src, t, d, lv, battle.Result{Flag: battle.FlagLong | battle.FlagSkill | battle.FlagMisc}, true)
	default:
		if d.ID == skill.IDHermode && src != t {
			// Hermode dispels allies only.
			if en := z.statusEngine(); en != nil {
				en.ClearBuffs(z.unit(t), sc.RemoveOnHermode)
			}
		}
		z.scStart(src, t, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	}
}

// ensembleCast is skill_check_pc_partner with cast_flag for an ensemble:
// the first partner dances too, or in renewal both get Ensemble Fatigue.
// Without parties there is no partner.
func (z *Zone) ensembleCast(src *entity, d *skill.Def, lv int) {
	partners := z.ensemblePartners(src, d, &lv, 1)
	if len(partners) == 0 {
		return
	}
	t := partners[0]
	dancing := src.sc.Get(sc.SCDancing)
	if dancing != nil && (!z.rules().EnsembleSongs || d.ID == skill.IDMoonlit) {
		dancing.Val4 = int(z.unit(t).Ref().ID)
		z.scStart4(src, t, sc.SCDancing, 100, d.ID, dancing.Val2, lv, int(z.unit(src).Ref().ID),
			int64(d.Duration1.At(lv))+1000)
		z.skillEffect(t, src, d, lv, int64(lv), true)
		t.player.perform.danceSkill, t.player.perform.danceLv = d.ID, lv
		return
	}
	if z.rules().EnsembleSongs {
		dur := int64(z.Skills.Get(skill.IDSpecialSinger).Duration1.At(lv))
		z.scStart(src, src, sc.SCEnsemblefatigue, 100, 1, dur)
		z.scStart(src, t, sc.SCEnsemblefatigue, 100, 1, dur)
	}
}

// songValues are the group values of a song in skill_unitsetting: what
// the song's status gets as val2 and val3.
func (z *Zone) songValues(src *entity, id, lv int) (int, int) {
	st := src.status
	if st == nil {
		st = &npcStatus
	}
	lesson := func(id int) int {
		if p := src.player; p != nil {
			return p.skillLevel(id)
		}
		return 0
	}
	var val1, val2 int
	switch id {
	case skill.IDWhistle:
		val1 = lv + st.Agi/10       // FLEE.
		val2 = (lv+1)/2 + st.Luk/30 // Perfect dodge.
		val1 += lesson(skill.IDMusicalLesson) / 2
		val2 += lesson(skill.IDMusicalLesson) / 5
	case skill.IDHumming:
		val1 = 1 + 2*lv + st.Dex/10 // HIT.
		val1 += lesson(skill.IDDancingLesson)
	case skill.IDPoemBragi:
		val1 = 3*lv + st.Dex/10 // Cast time reduction.
		// Level 10's base delay reduction is 50%.
		if lv < 10 {
			val2 = 3*lv + st.Int/5
		} else {
			val2 = 50 + st.Int/5
		}
		val1 += lesson(skill.IDMusicalLesson)
		val2 += 2 * lesson(skill.IDMusicalLesson)
	case skill.IDDontForgetMe:
		if z.rules().EnsembleSongs {
			val1 = 3*lv + st.Dex/15 // ASPD decrease.
			val2 = 2*lv + st.Agi/20 // Speed decrease.
			val1 += lesson(skill.IDDancingLesson)
			val2 += lesson(skill.IDDancingLesson) / 2
		} else {
			val1 = 5 + 3*lv + st.Dex/10
			val2 = 5 + 3*lv + st.Agi/10
			val1 += lesson(skill.IDDancingLesson)
			val2 += lesson(skill.IDDancingLesson)
		}
		// 10 is 1% of ASPD.
		val1 *= 10
	case skill.IDServiceForYou:
		val1 = 15 + lv + st.Int/10   // Max SP percent.
		val2 = 20 + 3*lv + st.Int/10 // SP cost reduction.
		val1 += lesson(skill.IDDancingLesson) / 2
		val2 += lesson(skill.IDDancingLesson) / 2
	case skill.IDAssassinCross:
		val1 = lesson(skill.IDMusicalLesson) / 2
		val1 += 5 + lv + st.Agi/20
		// ASPD works with 1000 as 100%.
		val1 *= 10
	case skill.IDFortuneKiss:
		val1 = 10 + lv + st.Luk/10 // Critical.
		// Every 10 is one critical point.
		val1 *= 10
		val1 += 5 * lesson(skill.IDDancingLesson)
	case skill.IDDrumBattlefield:
		val1 = (lv + 1) * 25 // ATK.
		val2 = (lv + 1) * 2  // DEF.
	case skill.IDRingNibelungen:
		val1 = (lv + 2) * 25 // ATK.
	case skill.IDRichmanKim:
		val1 = 25 + 11*lv // Exp bonus.
	case skill.IDSiegfried:
		val1 = 55 + lv*5 // Element resistance.
		val2 = lv * 10   // Status resistance.
	}
	return val1, val2
}

// danceStart is the dance part of skill_unitsetting: the performer
// remembers the dance and gets SC_DANCING, and an ensemble's partner
// dances too. Wand of Hermode has no partner.
func (z *Zone) danceStart(src *entity, g *unitGroup) {
	if g.songDance == 0 {
		return
	}
	if p := src.player; p != nil {
		p.perform.danceSkill, p.perform.danceLv = g.def.ID, g.lv
	}
	val4 := 0
	if g.songDance&songDanceEnsemble != 0 {
		val4 = bctSelf
	}
	if z.scStart4(src, src, sc.SCDancing, 100, g.def.ID, int(g.id), g.lv, val4, g.limit+1000) &&
		src.player != nil && g.songDance&songDanceEnsemble != 0 && g.def.ID != skill.IDHermode {
		z.ensembleCast(src, g.def, g.lv)
	}
}

// danceGroupDeleted is skill_delunitgroup's dance part: the performer
// stops dancing when its group ends. val2 is cleared first, so the end
// does not delete the group again.
func (z *Zone) danceGroupDeleted(g *unitGroup) {
	if g.songDance == 0 || g.src == nil {
		return
	}
	if e := g.src.sc.Get(sc.SCDancing); e != nil {
		e.Val2 = 0
		z.scEnd(g.src, sc.SCDancing)
	}
}

// performanceEnded is status_change_end's part for the performance
// statuses, once the status is gone:
//   - SC_DANCING ends the partner's dance too, and the dance's group.
//   - SC_MARIONETTE and SC_MARIONETTE2 end the partner's.
//   - Pre-renewal SC_HERMODE with val3 BCT_SELF clears the unit's groups.
func (z *Zone) performanceEnded(e *entity, en *sc.Entry) {
	switch en.Type {
	case sc.SCDancing:
		if en.Val4 != 0 && en.Val4 != bctSelf {
			if o := z.unitByID(en.Val4); o != nil && o.player != nil {
				if d := o.sc.Get(sc.SCDancing); d != nil {
					d.Val2 = 0
					d.Val4 = bctSelf
					z.scEnd(o, sc.SCDancing)
				}
			}
		}
		if en.Val2 != 0 {
			if g := z.groups[int64(en.Val2)]; g != nil {
				z.deleteGroup(g)
			}
		}
		if en.Val1&0xFFFF == skill.IDMoonlit {
			scEnv{z}.Icon(z.unit(e), efstMoon, false, 0, 0, 0, 0)
		}
	case sc.SCMarionette, sc.SCMarionette2:
		if en.Val1 != 0 {
			other := sc.SCMarionette2
			if en.Type == sc.SCMarionette2 {
				other = sc.SCMarionette
			}
			if o := z.unitByID(en.Val1); o != nil {
				if s := o.sc.Get(other); s != nil {
					s.Val1 = 0
					z.scEnd(o, other)
				}
			}
		}
	case sc.SCHermode:
		if !z.rules().EnsembleSongs && en.Val3 == bctSelf {
			z.clearUnitGroups(e)
		}
	}
}

// unitByID is map_id2bl for the IDs that statuses keep: a player's
// character ID or a monster's handle.
func (z *Zone) unitByID(id int) *entity {
	if id == 0 {
		return nil
	}
	var mob *entity
	for e := range z.entities.all() {
		if e.player == nil && e.monster == nil {
			continue
		}
		if z.unit(e).Ref().ID == int64(id) {
			if e.player != nil {
				return e
			}
			mob = e
		}
	}
	return mob
}

// lullaby is SkillLullaby's applyAdditionalEffects: sleep. In
// pre-renewal the chance comes from the INT of both performers, so it
// needs the partner. In renewal it always works, for the song's
// duration.
type lullaby struct{}

func (lullaby) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if z.rules().EnsembleSongs {
		z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
		return
	}
	dancing := src.sc.Get(sc.SCDancing)
	if dancing == nil {
		return
	}
	partner := z.unitByID(dancing.Val4)
	if partner == nil || partner.status == nil || src.status == nil {
		return
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	// rnd_value(100, 300).
	rate := (src.status.Int + partner.status.Int + 100 + z.rand.IntN(201)) * 10
	su := z.unit(src)
	en.Start(su, z.unit(target), sc.Start{Type: d.Status, Rate: rate, Val1: lv, Duration: int64(d.Duration2.At(lv)),
		Reason: sc.ReasonSkill, Source: su.Ref()})
}

// uglyDance is SkillHipShaker: a dance whose hits drain SP.
type uglyDance struct{ song }

func (uglyDance) additionalEffect(z *Zone, src, target *entity, _ *skill.Def, lv int) {
	var sp int
	if z.rules().EnsembleSongs {
		sp = 2*lv + 10
	} else {
		sp = 5 + 5*lv
		if p := src.player; p != nil {
			sp += lv * p.skillLevel(skill.IDDancingLesson)
		}
	}
	// status_zap.
	scEnv{z}.Damage(z.unit(target), 0, int64(sp), true, false)
}

// adaptation is SkillAmp: renewal's status, or pre-renewal's end of the
// performer's dance.
type adaptation struct{}

// castBegin is skill_check_condition_castbegin for pre-renewal's
// Adaptation: the caster must dance, and the dance must last longer than
// Adaptation's Duration2. rAthena reads the time left as val3 >> 16,
// which is 0, so only the dance's own duration counts.
func (adaptation) castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	if z.rules().EnsembleSongs {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	dancing := src.sc.Get(sc.SCDancing)
	if dancing == nil {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	elapsed := 1000 * (dancing.Val3 >> 16)
	dance := z.Skills.Get(dancing.Val1 & 0xFFFF)
	if dance == nil || dance.Duration1.At(dancing.Val1>>16)-elapsed < d.Duration2.At(lv) {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (adaptation) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if z.rules().EnsembleSongs {
		statusSkill{}.castendNoDamage(z, src, target, d, lv, flag)
		return
	}
	if target.sc.Has(sc.SCDancing) {
		z.skillEffect(src, target, d, lv, int64(lv), true)
		z.scEnd(target, sc.SCDancing)
	}
}

// encore is SkillEncore: the last song or dance again, at half its SP.
type encore struct{}

// castBegin is unit_skilluse_id2's case BD_ENCORE: the player must have
// a dance it still knows, and the next cast of it costs half.
func (encore) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	p := src.player
	if p.perform.danceSkill == 0 || p.skillLevel(p.perform.danceSkill) <= 0 {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	p.perform.skillOld = skill.IDEncore
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (encore) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	p := src.player
	if p == nil {
		return
	}
	if dance := z.Skills.Get(p.perform.danceSkill); dance != nil {
		z.skillUseID(src, src, dance, p.perform.danceLv)
	}
}

// frostJoker is SkillUnbarringOctave and SkillDazzler: 3 seconds after
// the cast, every unit around may freeze (Frost Joker) or be stunned
// (Scream), the caster's allies too.
type frostJoker struct{}

func (frostJoker) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	z.addTimerSkill(src, z.now+3000, target, src.x, src.y, d, lv, 0, *flag)
}

// timerSkill is skill_timerskill's case BA_FROSTJOKER and DC_SCREAM:
// skill_frostjoke_scream on every character in the square around where
// the caster stood.
func (frostJoker) timerSkill(z *Zone, t *timerSkill) {
	src := t.src
	r := t.def.SplashFor(t.lv)
	for _, bl := range z.findEntities(&query{area: areaRect, x0: t.x - r, y0: t.y - r, x1: t.x + r, y1: t.y + r,
		kinds: queryPlayers | queryMonsters}) {
		if bl == src || bl.dead {
			continue
		}
		// It has been reported to work the same in every map type.
		if checkTarget(src, bl, bctEnemy|bctParty) {
			z.skillAdditionalEffect(src, bl, t.def, t.lv, battle.Result{Flag: battle.FlagMisc}, true)
		}
	}
}

// additionalEffect is SkillUnbarringOctave's and SkillDazzler's
// applyAdditionalEffects. The chance is per 1,000, as Aegis's: 150 + 50
// per level, 100 more for Scream. Party members get a quarter of the
// chance for Duration1.
func (frostJoker) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	rate := 150 + 50*lv
	if d.ID == skill.IDScream {
		rate += 100
	}
	duration := int64(d.Duration2.At(lv))
	if checkTarget(src, target, bctParty) {
		rate /= 4
		duration = int64(d.Duration1.At(lv))
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	su := z.unit(src)
	en.Start(su, z.unit(target), sc.Start{Type: d.Status, Rate: rate * 10, Val1: lv, Duration: duration,
		Reason: sc.ReasonSkill, Source: su.Ref()})
}

// pangVoice is SkillPangVoice: confusion, and in renewal bleeding.
type pangVoice struct{}

func (pangVoice) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if z.skillRenewal(skill.IDPangVoice) {
		z.scStart(src, target, sc.SCConfusion, 100, lv, int64(d.Duration1.At(lv)))
		z.scStart(src, target, sc.SCBleeding, 100, lv, int64(d.Duration2.At(lv)))
	} else {
		z.scStart(src, target, sc.SCConfusion, 70, lv, int64(d.Duration1.At(lv)))
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// winkCharm is SkillWinkofCharm. On a player: renewal's confusion and
// hallucination, or pre-renewal's charm after a confusion that worked.
// On a monster: the charm by the level difference, and a hit of no
// damage that may turn the monster on the caster.
type winkCharm struct{}

func (winkCharm) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	switch {
	case target.player != nil:
		if z.skillRenewal(skill.IDWinkCharm) {
			z.scStart(src, target, sc.SCConfusion, 100, lv, int64(d.Duration1.At(lv)))
			z.scStart(src, target, sc.SCHallucination, 100, lv, int64(d.Duration2.At(lv)))
		} else if z.scStart(src, target, sc.SCConfusion, 10, lv, int64(d.Duration1.At(lv))) {
			z.scStart(src, target, d.Status, 100, lv, int64(d.Duration2.At(lv)))
		}
	case target.monster != nil:
		rate := z.unit(src).Level() - z.unit(target).Level() + 40
		if z.scStart4(src, target, d.Status, rate, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration2.At(lv))) {
			// battle_damage of 0 with BF_WEAPON|BF_LONG|BF_NORMAL: the
			// caster's cause_status bonuses, then mob_attacked.
			flag := battle.FlagWeapon | battle.FlagLong | battle.FlagNormal
			z.additionalEffectFlags(src, target, flag, d.ID)
			z.mobAttacked(src, target)
		}
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// moonlit is SkillShelteringBliss: an ensemble unit that pushes out
// whoever is not performing it.
type moonlit struct{}

// castBegin is skill_check_condition_castbegin for Moonlit Water Mill:
// the partner, then no wall within the splash and one cell more.
func (moonlit) castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	if r := z.ensembleCheck(src, d, lv); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		return r
	}
	r := d.SplashFor(lv) + 1
	for y := src.y - r; y <= src.y+r; y++ {
		for x := src.x - r; x <= src.x+r; x++ {
			if z.wallCell(x, y) {
				return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
			}
		}
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

// wallCell is map_getcell with CELL_CHKWALL: a cell that neither walks
// nor shoots. Cells outside the map are not walls.
func (z *Zone) wallCell(x, y int) bool {
	if x < 0 || y < 0 || x >= z.grid.Width || y >= z.grid.Height {
		return false
	}
	return z.grid.Cells[x+y*z.grid.Width]&(path.Walkable|path.Shootable) == 0
}

func (moonlit) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	*flag |= 1
	z.unitSetting(src, d, lv, x, y)
}

// marionette is SkillMarionetteControl: half the caster's stats go to
// the target. Casting it again on the same target ends it. A performer
// of the caster's sex, or a target under Curse or Quagmire, cannot be
// controlled. Marionette Control is PartyOnly, so without parties it
// only reaches the caster, which fails here.
type marionette struct{}

func (marionette) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	fail := func() {
		if src.player != nil {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		}
		*flag |= skillNoConsumeReq
	}
	if p, tp := src.player, target.player; p != nil && tp != nil && z.Status != nil {
		if id, ok := z.Status.MapID(tp.char.Job); ok && id&idSecondMsk == mapidBardDancer && tp.char.Male == p.char.Male {
			fail()
			return
		}
	}
	if target.sc.Has(sc.SCCurse) || target.sc.Has(sc.SCQuagmire) {
		fail()
		return
	}
	mine, theirs := src.sc.Get(sc.SCMarionette), target.sc.Get(sc.SCMarionette2)
	srcID, targetID := int(z.unit(src).Ref().ID), int(z.unit(target).Ref().ID)
	switch {
	case mine == nil && theirs == nil:
		dur := int64(d.Duration1.At(lv))
		z.scStart(src, src, sc.SCMarionette, 100, targetID, dur)
		z.scStart(src, target, sc.SCMarionette2, 100, srcID, dur)
		z.skillEffect(src, target, d, lv, int64(lv), true)
	case mine != nil && mine.Val1 == targetID && theirs != nil && theirs.Val1 == srcID:
		z.scEnd(src, sc.SCMarionette)
		z.scEnd(target, sc.SCMarionette2)
	default:
		fail()
	}
}

// longingFreedom is SkillLongingForFreedom: pre-renewal's freedom to
// walk and attack while performing an ensemble, but Moonlit Water Mill.
type longingFreedom struct{}

func (longingFreedom) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if z.rules().EnsembleSongs || target.sc.Has(d.Status) {
		return
	}
	dancing := target.sc.Get(sc.SCDancing)
	if dancing == nil || dancing.Val4 == 0 || dancing.Val1&0xFFFF == skill.IDMoonlit {
		return
	}
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// hermode is SkillWandOfHermode: renewal's song, or pre-renewal's unit
// near a warp portal, which the performer keeps dancing.
type hermode struct{}

// castBegin is pre-renewal's check that a warp portal is near.
func (hermode) castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	if !z.rules().EnsembleSongs && !z.warpNear(src.x, src.y, d.SplashFor(lv)) {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (hermode) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, _ *int) {
	if z.rules().EnsembleSongs {
		z.castendSong(src, d, lv)
	}
}

func (hermode) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	if z.rules().EnsembleSongs {
		return
	}
	z.clearUnitGroups(src)
	if g := z.unitSetting(src, d, lv, x, y); g != nil {
		z.scStart4(src, src, sc.SCDancing, 100, d.ID, 0, lv, int(g.id), int64(d.Duration1.At(lv)))
	}
	*flag |= 1
}

// specialSinger is SkillSkilledSpecialSinger: it ends Ensemble Fatigue.
type specialSinger struct{}

func (specialSinger) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if target.sc.Has(sc.SCEnsemblefatigue) {
		z.skillEffect(src, target, d, lv, int64(lv), true)
		z.scEnd(target, sc.SCEnsemblefatigue)
	}
}

// tarotCard is SkillTarotCardOfFate: one of 14 cards at random, when the
// roll of 8% per level succeeds. The SP is only paid on success.
type tarotCard struct{}

func (tarotCard) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	// The Sun's card keeps every other card away.
	if target.sc.Has(sc.SCTarotcard) {
		*flag |= skillNoConsumeReq
		return
	}
	// rAthena also fails on the Emperium and battleground monsters,
	// which do not exist yet.
	if z.rand.IntN(100) > lv*8 || !z.skillRenewal(skill.IDTarotCard) && target.sc.Has(sc.SCBasilica) {
		if src.player != nil {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		}
		*flag |= skillNoConsumeReq
		return
	}
	// status_zap of the skill's SP, which skill_consume_requirement
	// leaves out.
	scEnv{z}.Damage(z.unit(src), 0, int64(d.Requires.SP.At(lv)), true, false)
	z.tarotCard(src, target, d, lv)
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// Tarot cards, as skill_tarotcard's numbers.
const (
	tarotFool = iota + 1
	tarotMagician
	tarotHighPriestess
	tarotChariot
	tarotStrength
	tarotLovers
	tarotWheel
	tarotHangedMan
	tarotDeath
	tarotTemperance
	tarotDevil
	tarotTower
	tarotStar
	tarotSun
)

// tarotChances are the official chances of each card, in percent of a
// roll of 100, as upper bounds: card i comes out when the roll is below
// tarotChances[i-1] and not below the one before.
var tarotChances = [...]int{10, 20, 30, 37, 47, 62, 63, 69, 74, 82, 83, 85, 90, 100}

// tarotCard is skill_tarotcard: it draws a card with the official
// chances (tarotcard_equal_chance is off) and applies it. It returns the
// card.
func (z *Zone) tarotCard(src, target *entity, d *skill.Def, lv int) int {
	roll := z.rand.IntN(100)
	card := tarotSun
	for i, c := range tarotChances {
		if roll < c {
			card = i + 1
			break
		}
	}
	time2 := int64(d.Duration2.At(lv))
	// statusTime2 is skill_get_time2 of a status's DurationLookup skill
	// at level 1.
	statusTime := func(typ int, second bool) int64 {
		if z.Statuses == nil {
			return 0
		}
		def := z.Statuses.Get(typ)
		if def == nil {
			return 0
		}
		if second {
			return int64(def.Duration2(1))
		}
		return int64(def.Duration(1))
	}
	srcID := int(z.unit(src).Ref().ID)
	switch card {
	case tarotFool:
		// status_percent_damage: all the target's SP.
		if p := target.player; p != nil {
			scEnv{z}.Damage(z.unit(target), 0, p.sp, false, false)
		}
	case tarotMagician:
		z.scStart(src, target, sc.SCIncmatkrate, 100, -50, time2)
	case tarotHighPriestess:
		if en := z.statusEngine(); en != nil {
			en.ClearBuffs(z.unit(target), sc.RemoveBuffs|sc.RemoveChemProtect)
		}
	case tarotChariot:
		z.fixSkillDamage(src, target, d, 1000)
		if !target.dead {
			// skill_break_equip of the armor, the shield or the helm.
			// break_mob_equip is off, so monsters keep theirs, and
			// players are not tarot targets outside PvP.
			z.rand.IntN(3)
		}
	case tarotStrength:
		z.scStart(src, target, sc.SCIncatkrate, 100, -50, time2)
	case tarotLovers:
		z.healHP(target, 2000)
		// unit_warp to a random cell, as maps are not PvP.
		if x, y, ok := z.randomCell(); ok {
			if p := target.player; p != nil {
				p.warpTo = &scriptWarp{to: z, x: x, y: y}
				z.finishWarp(p)
			} else {
				z.moveTo(target, x, y)
			}
		}
	case tarotWheel:
		z.tarotCard(src, target, d, lv)
		z.tarotCard(src, target, d, lv)
	case tarotHangedMan:
		switch z.rand.IntN(3) {
		case 0:
			z.scStart(src, target, sc.SCAnkle, 100, lv, time2)
		case 1:
			z.scStart(src, target, sc.SCFreeze, 100, lv, statusTime(sc.SCFreeze, true))
		default:
			if en := z.statusEngine(); en != nil {
				su := z.unit(src)
				en.Start(su, z.unit(target), sc.Start{Type: sc.SCStonewait, Rate: 10000, Val1: lv, Val2: srcID,
					Duration: statusTime(sc.SCStonewait, true), Delay: statusTime(sc.SCStonewait, false),
					Reason: sc.ReasonSkill, Source: su.Ref()})
			}
		}
	case tarotDeath:
		if en := z.statusEngine(); en != nil {
			su := z.unit(src)
			en.Start(su, z.unit(target), sc.Start{Type: sc.SCComa, Rate: 10000, Val1: lv, Val3: srcID,
				Reason: sc.ReasonSkill, Source: su.Ref()})
		}
		z.scStart(src, target, sc.SCCurse, 100, lv, statusTime(sc.SCCurse, true))
		z.scStart4(src, target, sc.SCPoison, 100, lv, srcID, 0, 0, statusTime(sc.SCPoison, true))
	case tarotTemperance:
		z.scStart(src, target, sc.SCConfusion, 100, lv, time2)
	case tarotDevil:
		z.fixSkillDamage(src, target, d, 6666)
		z.scStart(src, target, sc.SCIncatkrate, 100, -50, time2)
		z.scStart(src, target, sc.SCIncmatkrate, 100, -50, time2)
		// rAthena passes the level as the chance and 100 as val1.
		z.scStart(src, target, sc.SCCurse, lv, 100, statusTime(sc.SCCurse, true))
	case tarotTower:
		z.fixSkillDamage(src, target, d, 4444)
	case tarotStar:
		z.scStart(src, target, sc.SCStun, 100, lv, statusTime(sc.SCStun, true))
	default:
		// The Sun: in renewal, SC_TAROTCARD keeps other cards away.
		if z.skillRenewal(skill.IDTarotCard) {
			z.scStart(src, target, sc.SCTarotcard, 100, lv, time2)
		}
		for _, typ := range []int{sc.SCIncatkrate, sc.SCIncmatkrate, sc.SCInchitrate, sc.SCIncfleerate, sc.SCIncdefrate} {
			z.scStart(src, target, typ, 100, -20, time2)
		}
		return tarotSun
	}
	return card
}

// fixSkillDamage is battle_fix_damage for a skill: damage that no defense changes, shown
// as a normal hit, without additional effects.
func (z *Zone) fixSkillDamage(src, target *entity, d *skill.Def, damage int64) {
	if target.dead || target.status == nil {
		return
	}
	res := battle.Result{Kind: battle.Hit, Damage: damage, Hits: 1, Flag: battle.FlagMisc}
	if src.player != nil || target.player != nil {
		z.logDamage(src, target, damage)
	}
	target.hp -= damage
	target.hpChanged = true
	z.hits = append(z.hits, hit{source: src, caster: src, target: target, result: res, hpPermille: target.hpPermille(),
		skill: d})
	z.damaged(target, damage, src.monster != nil)
	z.castCancel(target, true)
	if target.hp <= 0 {
		z.die(target, src)
		return
	}
	z.mobAttacked(src, target)
}

// Songs whose status lasts while the unit stands in them, and lingers
// after a player leaves (skill_unit_onleft).
var lingeringSongs = []int{skill.IDPoemBragi, skill.IDWhistle, skill.IDAssassinCross, skill.IDAppleIdun,
	skill.IDHumming, skill.IDDontForgetMe, skill.IDFortuneKiss, skill.IDServiceForYou}

// Ensembles whose status ends when the unit leaves them. Lullaby's sleep
// stays.
var endingEnsembles = []int{skill.IDRichmanKim, skill.IDEternalChaos, skill.IDDrumBattlefield,
	skill.IDRingNibelungen, skill.IDRokisWeil, skill.IDIntoAbyss, skill.IDSiegfried, skill.IDHermode}

// unitTargets is the target check of skill_unit_effect: the group's
// skill_get_unit_bl_target (NoPc and NoMob) and battle_check_target.
func unitTargets(g *unitGroup, t *entity) bool {
	if t.player != nil && unitFlag(g.def, "NoPc") || t.monster != nil && unitFlag(g.def, "NoMob") {
		return false
	}
	return checkTarget(g.src, t, g.targetFlag)
}

// songRefresh is skill_unit_onplace, skill_unit_onout and
// skill_unit_onleft for the song, dance and ensemble units, as a unit
// moves: each group it stands in acts on it (onplace), and the statuses
// of the songs it left end or linger (onleft).
func (z *Zone) songRefresh(t *entity) {
	if t.dead || (t.player == nil && t.monster == nil) {
		return
	}
	var groups []*unitGroup
	for _, g := range z.groups {
		if g.songDance != 0 {
			groups = append(groups, g)
		}
	}
	if len(groups) == 0 && t.sc.Empty() {
		return
	}
	slices.SortFunc(groups, func(a, b *unitGroup) int { return int(a.id - b.id) })
	covered := map[int]bool{}
	for _, g := range groups {
		// An overlapped cell acts as Dissonance or Ugly Dance, which
		// have intervals and no onplace of their own.
		if !z.songCovers(g, t) {
			continue
		}
		covered[g.def.ID] = true
		if unitTargets(g, t) {
			z.songOnPlace(g, t)
		}
	}
	z.songLeft(t, covered)
}

// songCovers reports whether a cell of a group that is not overlapped
// holds the unit.
func (z *Zone) songCovers(g *unitGroup, t *entity) bool {
	for _, u := range g.units {
		if u.x == t.x && u.y == t.y && !u.unit.overlap {
			return true
		}
	}
	return false
}

// songOnPlace is skill_unit_onplace for the song units.
func (z *Zone) songOnPlace(g *unitGroup, t *entity) {
	typ := g.def.Status
	switch g.def.ID {
	case skill.IDHermode, skill.IDRichmanKim, skill.IDEternalChaos, skill.IDDrumBattlefield, skill.IDRingNibelungen,
		skill.IDRokisWeil, skill.IDIntoAbyss, skill.IDSiegfried:
		if g.def.ID == skill.IDHermode && g.src != t && checkTarget(g.src, t, bctParty|bctGuild) {
			// Hermode dispels allies only.
			if en := z.statusEngine(); en != nil {
				en.ClearBuffs(z.unit(t), sc.RemoveOnHermode)
			}
		}
		// The performers stand in their own ensemble without its effect.
		if g.src == t {
			return
		}
		if !t.sc.Has(typ) {
			z.scStart4(g.src, t, typ, 100, g.lv, g.val1, g.val2, 0, g.limit)
		}
	case skill.IDWhistle, skill.IDAssassinCross, skill.IDPoemBragi, skill.IDAppleIdun, skill.IDHumming,
		skill.IDDontForgetMe, skill.IDFortuneKiss, skill.IDServiceForYou:
		if g.src == t {
			return
		}
		// One interval longer than the unit lasts, so the unit ends first.
		// refresh_song is off, so a status already there stays as it is.
		if !t.sc.Has(typ) {
			z.scStart4(g.src, t, typ, 100, g.lv, g.val1, g.val2, 0, g.limit+unitTimerInterval)
		}
	case skill.IDMoonlit:
		// Whoever does not perform it is pushed out.
		if d := t.sc.Get(sc.SCDancing); d != nil && d.Val1&0xFFFF == skill.IDMoonlit || g.src == t {
			return
		}
		z.skillBlown(g.src, t, g.def.Knockback.At(g.lv), rDir(t), false)
	}
}

// songLeft is skill_unit_onleft for the pre-renewal songs a unit no
// longer stands in. Renewal's songs have no units.
func (z *Zone) songLeft(t *entity, covered map[int]bool) {
	if z.rules().EnsembleSongs || t.sc.Empty() || z.Skills == nil {
		return
	}
	// A performer who stepped out of its ensemble stops.
	if d := t.sc.Get(sc.SCDancing); d != nil {
		id := d.Val1 & 0xFFFF
		if def := z.Skills.Get(id); def != nil && def.Has("IsEnsemble") && id != skill.IDMoonlit && !covered[id] {
			z.scEnd(t, sc.SCDancing)
		}
	}
	for _, id := range endingEnsembles {
		if def := z.Skills.Get(id); def != nil && !covered[id] {
			z.scEnd(t, def.Status)
		}
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	for _, id := range lingeringSongs {
		def := z.Skills.Get(id)
		if def == nil || covered[id] || !t.sc.Has(def.Status) {
			continue
		}
		if t.player == nil {
			// Only players keep a song after they leave.
			z.scEnd(t, def.Status)
			continue
		}
		en.Linger(z.unit(t), def.Status, int64(def.Duration2.At(1)))
	}
}

// songOnPlaceTimer is skill_unit_onplace_timer for the song units with
// an interval: Lullaby's sleep, Ugly Dance's SP drain, Dissonance's hit
// and Apple of Idun's heal.
func (z *Zone) songOnPlaceTimer(e, t *entity) {
	g := e.unit.group
	if !unitTargets(g, t) {
		return
	}
	switch g.def.ID {
	case skill.IDLullaby, skill.IDUglyDance:
		if g.src == t {
			return
		}
		z.skillAdditionalEffect(g.src, t, g.def, g.lv, battle.Result{Flag: battle.FlagLong | battle.FlagSkill | battle.FlagMisc}, true)
	case skill.IDDissonance:
		z.skillAttack(battle.TypeMisc, g.src, e, t, g.def, g.lv, 0)
	case skill.IDAppleIdun:
		// The performer is not healed.
		if g.src == t || g.src.status == nil || t.status == nil {
			return
		}
		heal := skill.CalcHeal(g.src.status, t.status, g.def.ID, g.lv, true, z.rules().HealFormula, z.rand)
		z.skillEffect(e, t, z.Skills.Get(skill.IDHeal), int(heal), heal, true)
		z.healHP(t, heal)
	}
}

// danceMoved is the dance part of map_moveblock: a performer's song or
// dance follows it. Ensembles stay.
func (z *Zone) danceMoved(e *entity) {
	d := e.sc.Get(sc.SCDancing)
	if d == nil || d.Val2 == 0 {
		return
	}
	g := z.groups[int64(d.Val2)]
	if g == nil || g.src != e || g.songDance&songDanceEnsemble != 0 {
		return
	}
	if dx, dy := e.x-g.x, e.y-g.y; dx != 0 || dy != 0 {
		z.moveUnitGroup(g, dx, dy)
	}
}

// moveUnitGroup is skill_unit_move_unit_group: every unit of a group
// moves by an offset. Viewers see the units leave and appear again, and
// the units that stood or now stand in the group's cells are checked.
func (z *Zone) moveUnitGroup(g *unitGroup, dx, dy int) {
	x0, y0, x1, y1 := g.x, g.y, g.x, g.y
	for _, u := range g.units {
		x0, y0, x1, y1 = min(x0, u.x), min(y0, u.y), max(x1, u.x), max(y1, u.y)
		nx, ny := u.x+dx, u.y+dy
		if nx <= 0 || ny <= 0 || nx >= z.grid.Width || ny >= z.grid.Height {
			continue
		}
		// The cell's overlap ends where it was and may start where it
		// goes.
		z.danceOverlapRemove(u)
		u.x, u.y = nx, ny
		z.danceOverlapSet(u)
		x0, y0, x1, y1 = min(x0, nx), min(y0, ny), max(x1, nx), max(y1, ny)
		for o := range z.entities.all() {
			if o.player == nil {
				continue
			}
			if id, ok := o.player.netIDs.get(u); ok {
				o.player.netIDs.del(u)
				z.queue(o.player, frame(z.tick, o.player.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT)))
			}
		}
	}
	g.x += dx
	g.y += dy
	for _, o := range z.findEntities(&query{area: areaRect, x0: x0, y0: y0, x1: x1, y1: y1, kinds: queryPlayers | queryMonsters}) {
		z.unitStatusRefresh(o)
	}
}

// performanceSkillUse is status_check_skilluse's part for performers. flag
// is statusCheckSkillUse's.
func (z *Zone) performanceSkillUse(src *entity, def *skill.Def, flag int) bool {
	l := src.sc
	if l.Empty() {
		return true
	}
	renewal := z.rules().EnsembleSongs
	if d := l.Get(sc.SCDancing); d != nil && flag != 2 {
		switch {
		case !renewal && l.Has(sc.SCLonging):
			// Everything but dancing again.
			if def.ID == skill.IDEncore || def.Has("IsSong") || def.Has("IsEnsemble") {
				return false
			}
		case !def.Has("AllowWhenPerforming"):
			return false
		}
		// Adaptation cannot stop Wand of Hermode.
		if !renewal && d.Val1&0xFFFF == skill.IDHermode && def.ID == skill.IDAdaptation {
			return false
		}
	}
	if flag == 0 && (l.Has(sc.SCMarionette) && def.ID != skill.IDMarionette ||
		l.Has(sc.SCMarionette2) && def.ID == skill.IDMarionette ||
		renewal && l.Has(sc.SCEnsemblefatigue) && def.ID != skill.IDSpecialSinger) {
		return false
	}
	if !renewal && l.Has(sc.SCRokisweil) && def.ID != skill.IDAdaptation {
		return false
	}
	if l.Has(sc.SCHermode) && def.Target == skill.TargetSupport {
		return false
	}
	return true
}

// SongUnit is statuschange.Performer's: the song's unit on the unit's
// cell, which its timer reads while refresh_song is off.
func (v scEnv) SongUnit(u sc.Unit, typ int) (int64, bool) {
	z, t := v.z, u.(scUnit).e
	if z.Statuses == nil {
		return 0, false
	}
	def := z.Statuses.Get(typ)
	if def == nil {
		return 0, false
	}
	e := z.unitOnCell(t, t.x, t.y, def.DurationSkill, true)
	// The Dissonance mark, or the performer, which its song leaves out.
	if e == nil || e.unit.overlap || e.unit.group.src == t {
		return 0, false
	}
	return e.unit.limit, true
}

// UnitByID is statuschange.Performer's map_id2bl.
func (v scEnv) UnitByID(id int) sc.Unit {
	if e := v.z.unitByID(id); e != nil {
		return v.z.unit(e)
	}
	return nil
}

// Near is statuschange.Performer's check_distance_bl.
func (v scEnv) Near(a, b sc.Unit, cells int) bool {
	return chebyshev(a.(scUnit).e, b.(scUnit).e) <= cells
}

// RawStats is statuschange.Performer's sd->status stats.
func (v scEnv) RawStats(u sc.Unit) [6]int {
	p := u.(scUnit).e.player
	if p == nil {
		return [6]int{}
	}
	s := p.char.Stats
	return [6]int{s.Str, s.Agi, s.Vit, s.Int, s.Dex, s.Luk}
}

// richmanBonus is mob_dead's exp bonus from Richman Kim: the monster's
// SC_RICHMANKIM in pre-renewal, the killer's in renewal.
func (z *Zone) richmanBonus(mob, killer *entity) int {
	if !z.rules().EnsembleSongs {
		return mob.sc.Val2(sc.SCRichmankim)
	}
	if killer != nil && killer.player != nil {
		return killer.sc.Val2(sc.SCRichmankim)
	}
	return 0
}

// appleItemHeal is renewal's SC_APPLEIDUN in pc_itemheal: val3 / 100
// more HP from items. rAthena divides the rate by 100, so it adds 0 for
// every level.
func (z *Zone) appleItemHeal(e *entity) int {
	if !z.rules().HealFormula {
		return 0
	}
	return e.sc.Val3(sc.SCAppleidun) / 100
}

// SC_NIBELUNGEN's regen effects in renewal, as RINGNBL_HPREGEN and
// RINGNBL_SPREGEN. Pre-renewal's val2 is an ATK of 75 or more.
const (
	ringHPRegen = 10
	ringSPRegen = 11
)

// danceKind is a unit's skill_unit val2 for songs and dances: 1 for a
// song (UF_SONG), 2 for a dance (UF_DANCE), 0 for neither. Two cells of
// one kind on a cell overlap.
func danceKind(g *unitGroup) int {
	if g.songDance&songDanceSong == 0 {
		return 0
	}
	if unitFlag(g.def, "Dance") {
		return 2
	}
	if unitFlag(g.def, "Song") {
		return 1
	}
	return 0
}

// danceOverlapping are the other cells on a unit's cell that overlap it,
// as skill_dance_overlap_sub's checks.
func (z *Zone) danceOverlapping(e *entity) []*entity {
	kind := danceKind(e.unit.group)
	if kind == 0 {
		return nil
	}
	var out []*entity
	for _, o := range z.unitsAt(e.x, e.y) {
		if o != e && o.unit.group != e.unit.group && danceKind(o.unit.group) == kind {
			out = append(out, o)
		}
	}
	return out
}

// danceOverlapSet is skill_dance_overlap with OVERLAP_SET: a song or
// dance cell placed on another of its kind turns both into Dissonance or
// Ugly Dance cells. Whoever stands there leaves the songs.
func (z *Zone) danceOverlapSet(e *entity) {
	for _, o := range z.danceOverlapping(e) {
		for _, x := range []*entity{o, e} {
			if !x.unit.overlap {
				x.unit.overlap = true
				z.unitReappear(x)
				z.refreshCell(x.x, x.y)
			}
		}
	}
}

// danceOverlapRemove is skill_dance_overlap with OVERLAP_REMOVE, for a
// cell that leaves: with more than one other cell, only this one turns
// back; otherwise the other turns back too and acts again.
func (z *Zone) danceOverlapRemove(e *entity) {
	if !e.unit.overlap {
		return
	}
	others := z.danceOverlapping(e)
	if len(others) <= 1 {
		for _, o := range others {
			if o.unit.overlap {
				o.unit.overlap = false
				z.unitReappear(o)
				z.refreshCell(o.x, o.y)
			}
		}
	}
	e.unit.overlap = false
	z.unitReappear(e)
}

// danceSwitch is skill_dance_switch: an overlapped cell acts as
// Dissonance (songs) or Ugly Dance (dances), at its group's level. It
// returns what turns the group back.
func (z *Zone) danceSwitch(e *entity) func() {
	u := e.unit
	if u == nil || !u.overlap || z.Skills == nil {
		return func() {}
	}
	g := u.group
	id := skill.IDDissonance
	if danceKind(g) == 2 {
		id = skill.IDUglyDance
	}
	d := z.Skills.Get(id)
	if d == nil || d.Unit == nil {
		return func() {}
	}
	def, unitID, target, interval := g.def, g.unitID, g.targetFlag, g.interval
	g.def, g.unitID, g.targetFlag, g.interval = d, d.Unit.ID, d.Unit.Target, d.Unit.Interval
	return func() { g.def, g.unitID, g.targetFlag, g.interval = def, unitID, target, interval }
}

// unitReappear shows a cell again to its viewers, as the client cannot
// change a unit's UNT_ ID (clif_skill_delunit, then
// skill_getareachar_skillunit_visibilty).
func (z *Zone) unitReappear(e *entity) {
	for o := range z.entities.all() {
		if o.player == nil {
			continue
		}
		if id, ok := o.player.netIDs.get(e); ok {
			o.player.netIDs.del(e)
			z.queue(o.player, frame(z.tick, o.player.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT)))
		}
	}
}

// refreshCell checks the characters on a cell against the units there.
func (z *Zone) refreshCell(x, y int) {
	for _, o := range z.findEntities(&query{area: areaRect, x0: x, y0: y, x1: x, y1: y, kinds: queryPlayers | queryMonsters}) {
		z.unitStatusRefresh(o)
	}
}

// shownUnitID is the UNT_ ID viewers see for a cell: Dissonance's or
// Ugly Dance's for an overlapped song or dance cell.
func (z *Zone) shownUnitID(e *entity) int {
	g := e.unit.group
	if !e.unit.overlap || z.Skills == nil {
		return g.unitID
	}
	id := skill.IDDissonance
	if danceKind(g) == 2 {
		id = skill.IDUglyDance
	}
	if d := z.Skills.Get(id); d != nil && d.Unit != nil {
		return d.Unit.ID
	}
	return g.unitID
}
