package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/social"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// partyOf adds players of a build to a zone at cells near (20, 20), with
// the given names, and puts the leader and them in one party. It returns
// the new players.
func partyOf(t *testing.T, z *Zone, leader *Player, build goldenPlayer, names ...string) []*Player {
	t.Helper()
	return armedPartyOf(t, z, leader, build, 0, names...)
}

// armedPartyOf is partyOf with the new players holding a weapon.
func armedPartyOf(t *testing.T, z *Zone, leader *Player, build goldenPlayer, weapon uint32, names ...string) []*Player {
	t.Helper()
	var out []*Player
	for i, n := range names {
		m := goldenPlayerIn(t, z, build, weapon, 0)
		m.Name = n
		m.entity.x, m.entity.y = 20+i+1, 21
		out = append(out, m)
	}
	leader.Name = "Leader"
	formParty(t, z, social.Options{}, append([]*Player{leader}, out...)...)
	return out
}

// outsider adds a player of a build who is in no party, at a cell next to
// (20, 20).
func outsider(t *testing.T, z *Zone, build goldenPlayer) *Player {
	t.Helper()
	return armedOutsider(t, z, build, 0)
}

// armedOutsider is outsider holding a weapon.
func armedOutsider(t *testing.T, z *Zone, build goldenPlayer, weapon uint32) *Player {
	t.Helper()
	o := goldenPlayerIn(t, z, build, weapon, 0)
	o.Name = "Outsider"
	o.entity.x, o.entity.y = 19, 21
	return o
}

// The party buffs reach the members in the splash on the caster's map
// (party_foreachsamemap with skill_area_sub in SkillAngelus, SkillGloria,
// SkillMagnificat, and renewal's SkillImpositioManus and
// SkillSuffragium). A player who is not in the party and a member out of
// the splash do not get them.
func TestPartyStatusReachesMembers(t *testing.T) {
	cases := []struct {
		id, status int
		renewal    bool
	}{
		{skill.IDAngelus, sc.SCAngelus, false},
		{skill.IDGloria, sc.SCGloria, false},
		{skill.IDMagnificat, sc.SCMagnificat, false},
		{skill.IDImpositio, sc.SCImpositio, true},
		{skill.IDSuffragium, sc.SCSuffragium, true},
	}
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			for _, c := range cases {
				z, p, _ := priestZone(t, mode)
				ms := partyOf(t, z, p, highPriest, "Near", "Far")
				far := ms[1]
				d := z.Skills.Get(c.id)
				far.entity.x = p.entity.x + d.SplashFor(5) + 1
				o := outsider(t, z, highPriest)
				z.castendNoDamage(p.entity, p.entity, d, 5, 0)
				party := !c.renewal || mode == reference.Renewal
				if !p.entity.sc.Has(c.status) {
					t.Errorf("skill %d: the caster has no status", c.id)
				}
				if got := ms[0].entity.sc.Has(c.status); got != party {
					t.Errorf("skill %d: member has the status %v, want %v", c.id, got, party)
				}
				if far.entity.sc.Has(c.status) || o.entity.sc.Has(c.status) {
					t.Errorf("skill %d: a far member or an outsider has the status", c.id)
				}
			}
		})
	}
}

// Without a party the caster alone gets a party buff.
func TestPartyStatusWithoutParty(t *testing.T) {
	z, p, _ := priestZone(t, reference.Renewal)
	o := outsider(t, z, highPriest)
	z.castendNoDamage(p.entity, p.entity, z.Skills.Get(skill.IDAngelus), 5, 0)
	if !p.entity.sc.Has(sc.SCAngelus) || o.entity.sc.Has(sc.SCAngelus) {
		t.Error("Angelus without a party did not reach the caster alone")
	}
}

// Redemptio revives the dead members in its splash with half their HP,
// and leaves the caster at 1 HP. In pre-renewal the caster also loses
// its SP and 1% of the next level's experience, less a fifth for each
// member revived (SkillRedemptio). A dead outsider stays dead.
func TestRedemptioRevivesParty(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, _ := priestZone(t, mode)
			ms := partyOf(t, z, p, highPriest, "Fallen", "Living")
			o := outsider(t, z, highPriest)
			for _, e := range []*entity{ms[0].entity, o.entity} {
				e.hp = 0
				z.die(e, nil)
			}
			me := p.entity
			next := z.Progress.NextBaseExp(&p.progress)
			p.progress.BaseExp = next / 2
			exp := p.progress.BaseExp
			z.castendNoDamage(me, me, z.Skills.Get(skill.IDRedemptio), 1, 0)
			f := ms[0].entity
			if f.dead || f.hp != int64(f.status.MaxHP)*50/100 {
				t.Errorf("member dead %v, HP %d of %d", f.dead, f.hp, f.status.MaxHP)
			}
			if !o.entity.dead {
				t.Error("the outsider was revived")
			}
			if me.hp != 1 {
				t.Errorf("caster HP %d, want 1", me.hp)
			}
			if mode == reference.PreRenewal {
				if want := exp - next*4/5/100; p.progress.BaseExp != want || p.sp != 0 {
					t.Errorf("caster exp %d and SP %d, want %d and 0", p.progress.BaseExp, p.sp, want)
				}
			} else if p.progress.BaseExp != exp || p.sp == 0 {
				t.Errorf("caster exp %d and SP %d, want %d and some SP", p.progress.BaseExp, p.sp, exp)
			}
		})
	}
}

// Redemptio fails when no member in its splash is dead.
func TestRedemptioNeedsDeadMember(t *testing.T) {
	z, p, _ := priestZone(t, reference.Renewal)
	partyOf(t, z, p, highPriest, "Living")
	events(p)
	hp := p.entity.hp
	z.castendNoDamage(p.entity, p.entity, z.Skills.Get(skill.IDRedemptio), 1, 0)
	z.stepAt(z.now + 50)
	if f := skillFailures(events(p)); len(f) != 1 || p.entity.hp != hp {
		t.Errorf("failures %v and HP %d, want one failure and HP %d", f, p.entity.hp, hp)
	}
}

// Weapon Perfection, Power Thrust and Wind Walker reach the members in
// the splash on the caster's map (SkillWeaponPerfection,
// SkillPowerThrust, SkillWindWalker). A member's Power Thrust has val2
// 0, the caster's 1. Adrenaline Rush skips a member without an axe or a
// mace (pc_check_weapontype). An outsider gets nothing. Everyone holds a
// sword.
func TestSmithAndWindWalkReachParty(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			for _, c := range []struct{ id, status int }{
				{skill.IDWeaponPerfect, sc.SCWeaponperfection},
				{skill.IDOverThrust, sc.SCOverthrust},
				{skill.IDWindWalk, sc.SCWindwalk},
				{skill.IDAdrenaline, sc.SCAdrenaline},
			} {
				z, p, _ := smithZone(t, mode, smithAxe, 0)
				m := armedPartyOf(t, z, p, knightPlayer, smithSword, "Member")[0]
				o := armedOutsider(t, z, knightPlayer, smithSword)
				z.castendNoDamage(p.entity, p.entity, z.Skills.Get(c.id), 5, 0)
				if !p.entity.sc.Has(c.status) {
					t.Errorf("skill %d: the caster has no status", c.id)
				}
				member := c.id != skill.IDAdrenaline
				if got := m.entity.sc.Has(c.status); got != member {
					t.Errorf("skill %d: member has the status %v, want %v", c.id, got, member)
				}
				if o.entity.sc.Has(c.status) {
					t.Errorf("skill %d: the outsider has the status", c.id)
				}
				if c.id == skill.IDOverThrust {
					if self, other := p.entity.sc.Get(c.status), m.entity.sc.Get(c.status); self == nil || self.Val2 != 1 || other.Val2 != 0 {
						t.Errorf("Power Thrust val2 %v on the caster and %v on the member", self, other)
					}
				}
			}
		})
	}
}

// Twilight Pharmacy 2 and 3 start only with an online Super Novice or
// Taekwon class in the caster's party (party_skill_check through
// skill_check_condition_castbegin).
func TestTwilightNeedsPartyClass(t *testing.T) {
	for _, c := range []struct{ id, job int }{{skill.IDTwilight2, 23}, {skill.IDTwilight3, 4046}} {
		z := goldenZone(t, reference.Renewal)
		p := alchemistIn(t, z, 18, 0, map[int]int{c.id: 1})
		d := z.Skills.Get(c.id)
		if r := (twilightAlchemy{}).castBegin(z, p.entity, d, 1); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
			t.Errorf("skill %d without a party: %v", c.id, r)
		}
		partyOf(t, z, p, knightPlayer, "Knight")
		if r := (twilightAlchemy{}).castBegin(z, p.entity, d, 1); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
			t.Errorf("skill %d with a Knight: %v", c.id, r)
		}
		m := goldenPlayerIn(t, z, goldenPlayer{Job: c.job, Build: "melee", Sex: 1, JobLevel: 50}, 0, 0)
		m.Name = "Helper"
		formPartyWith(t, z, p, m)
		if r := (twilightAlchemy{}).castBegin(z, p.entity, d, 1); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
			t.Errorf("skill %d with job %d in the party: %v", c.id, c.job, r)
		}
	}
}

// formPartyWith adds a player to the party of another, as the registry
// would on an accepted invite.
func formPartyWith(t *testing.T, z *Zone, member, p *Player) {
	t.Helper()
	reg := z.socialHub()
	if p.soc == nil {
		e := p.entity
		p.soc = reg.Online(social.Login{Name: p.Name, Map: z.Map, X: e.x, Y: e.y, HP: e.hp, Level: p.progress.BaseLevel, Job: p.char.Job})
	}
	id := member.partyID()
	if ok, err := reg.AddMember(id, p.soc); !ok || err != nil {
		t.Fatalf("%s did not join: %v", p.Name, err)
	}
	refreshParty(z, id)
}

// Counter Kick (TK_COUNTER) by a player with Union of the Sun, Moon and
// Stars raises the Triple Attack rate of the party's Monks on the map,
// and Combo Finish raises the Counter Kick rate of the party's Star
// Gladiators in the Counter Kick stance (party_skill_check). A Monk out
// of the party gets nothing.
func TestSGFriendPartyBonus(t *testing.T) {
	z, monk, m := monkZone(t)
	sg := goldenPlayerIn(t, z, goldenPlayer{Job: 4047, Build: "melee", Sex: 1, JobLevel: 50,
		Skills: [][2]int{{idSGFriend, 3}}}, 0, 0)
	sg.Name = "Gladiator"
	sg.entity.x, sg.entity.y = m.x-1, m.y
	monk.Name = "Monk"
	formParty(t, z, social.Options{}, sg, monk)
	o := goldenPlayerIn(t, z, goldenPlayer{Job: jobChampion, Build: "melee", Sex: 1, JobLevel: 50,
		Skills: [][2]int{{skill.IDTripleAttack, 10}}}, 0, 0)
	o.Name = "Other"
	z.skillAttack(battle.TypeWeapon, sg.entity, sg.entity, m, z.Skills.Get(idTKCounter), 1, 0)
	e := monk.entity.sc.Get(sc.SCSkillrateUp)
	if e == nil || e.Val1 != skill.IDTripleAttack || e.Val2 != 200 {
		t.Errorf("Monk's skill rate %+v, want Triple Attack at 200", e)
	}
	if o.entity.sc.Has(sc.SCSkillrateUp) {
		t.Error("a Monk out of the party got the bonus")
	}
	z.scStart(sg.entity, sg.entity, sc.SCReadycounter, 100, 1, sc.InfiniteTick)
	z.skillCombo(monk.entity, monk.entity, m, z.Skills.Get(skill.IDComboFinish), 5)
	e = sg.entity.sc.Get(sc.SCSkillrateUp)
	if e == nil || e.Val1 != idTKCounter || e.Val2 != 200 {
		t.Errorf("Star Gladiator's skill rate %+v, want Counter Kick at 200", e)
	}
}

// Outside PvP a player's Dispell ends the statuses of the players in its
// party, the caster included, and does nothing to other players
// (SkillDispell).
func TestDispellOnPartyOnly(t *testing.T) {
	z, p, _ := sageZone(t, 16, map[int]int{skill.IDDispell: 5})
	o := outsider(t, z, knightPlayer)
	d := z.Skills.Get(skill.IDDispell)
	for _, e := range []*entity{p.entity, o.entity} {
		z.scStart(e, e, sc.SCBlessing, 100, 10, 60000)
	}
	z.castendNoDamage(p.entity, p.entity, d, 5, 0)
	if !p.entity.sc.Has(sc.SCBlessing) {
		t.Error("Dispell without a party ended the caster's Blessing")
	}
	m := partyOf(t, z, p, knightPlayer, "Member")[0]
	z.scStart(m.entity, m.entity, sc.SCBlessing, 100, 10, 60000)
	for _, e := range []*entity{p.entity, m.entity, o.entity} {
		z.castendNoDamage(p.entity, e, d, 5, 0)
	}
	if p.entity.sc.Has(sc.SCBlessing) || m.entity.sc.Has(sc.SCBlessing) {
		t.Error("Dispell did not end the party's Blessing")
	}
	if !o.entity.sc.Has(sc.SCBlessing) {
		t.Error("Dispell ended an outsider's Blessing")
	}
}

// A hidden trap shows to its caster and the caster's party, and to no
// one else (skill_getareachar_skillunit_visibilty_single).
func TestHiddenTrapShowsToParty(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11)
	g := placeTrap(t, z, p, skill.IDSandman, 5)
	m := partyOf(t, z, p, knightPlayer, "Member")[0]
	o := outsider(t, z, knightPlayer)
	if !g.trap.hidden {
		t.Fatal("the trap is not hidden")
	}
	u := g.units[0]
	if trapHiddenFrom(u, p.entity) || trapHiddenFrom(u, m.entity) {
		t.Error("the trap is hidden from its caster or the party")
	}
	if !trapHiddenFrom(u, o.entity) {
		t.Error("the trap shows to an outsider")
	}
}

// A player's battle status counts the party members on its map, which
// pre-renewal's Taekwon Mission reads.
func TestPartyOnMapCount(t *testing.T) {
	z, p, _ := priestZone(t, reference.PreRenewal)
	z.syncBattlePlayer(p.entity)
	if n := p.entity.status.Player.PartyOnMap; n != 0 {
		t.Errorf("%d members without a party, want 0", n)
	}
	partyOf(t, z, p, knightPlayer, "A", "B")
	outsider(t, z, knightPlayer)
	z.syncBattlePlayer(p.entity)
	if n := p.entity.status.Player.PartyOnMap; n != 3 {
		t.Errorf("%d members on the map, want 3", n)
	}
}

// Renewal's songs reach the caster's party around it, and no outsider
// (skill_castend_song with BCT_PARTY). Wand of Hermode also ends the
// buffs of the members it reaches.
func TestRenewalSongsReachParty(t *testing.T) {
	z, p, _ := bardZone(t, reference.Renewal, clown)
	m := partyOf(t, z, p, knightPlayer, "Member")[0]
	o := outsider(t, z, knightPlayer)
	me := p.entity
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDWhistle), 10, 0)
	if !m.entity.sc.Has(sc.SCWhistle) || o.entity.sc.Has(sc.SCWhistle) {
		t.Errorf("Whistle on the member %v and the outsider %v", m.entity.sc.Has(sc.SCWhistle), o.entity.sc.Has(sc.SCWhistle))
	}
	z.scStart(m.entity, m.entity, sc.SCBlessing, 100, 10, 60000)
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDHermode), 5, 0)
	if !m.entity.sc.Has(sc.SCHermode) || o.entity.sc.Has(sc.SCHermode) {
		t.Errorf("Hermode on the member %v and the outsider %v", m.entity.sc.Has(sc.SCHermode), o.entity.sc.Has(sc.SCHermode))
	}
	if m.entity.sc.Has(sc.SCBlessing) {
		t.Error("Hermode did not end the member's Blessing")
	}
}

// An ensemble starts with a performer of the other sex next to the
// caster in its party (skill_check_pc_partner). In renewal both then get
// Ensemble Fatigue; in pre-renewal the partner dances too. A partner out
// of the party does not count.
func TestEnsembleWithPartyPartner(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, _ := bardZone(t, mode, clown)
			partner := goldenPlayerIn(t, z, dancer, itemRope, 0)
			partner.Name = "Partner"
			partner.entity.x = 21
			me := p.entity
			d := z.Skills.Get(skill.IDRichmanKim)
			if z.skillUseID(me, me, d, 1) {
				t.Fatal("the ensemble started with a partner out of the party")
			}
			p.Name = "Leader"
			formParty(t, z, social.Options{}, p, partner)
			if !z.skillUseID(me, me, d, 1) {
				t.Fatalf("the ensemble did not start: failures %v", skillFailures(p.pending))
			}
			step(z, z.now+50, z.now+3000)
			if mode == reference.Renewal {
				if !me.sc.Has(sc.SCEnsemblefatigue) || !partner.entity.sc.Has(sc.SCEnsemblefatigue) {
					t.Error("no Ensemble Fatigue on both performers")
				}
				if !partner.entity.sc.Has(sc.SCRichmankim) {
					t.Error("the partner did not get the song")
				}
				return
			}
			pd := partner.entity.sc.Get(sc.SCDancing)
			if pd == nil || pd.Val1&0xFFFF != skill.IDRichmanKim || !me.sc.Has(sc.SCDancing) {
				t.Errorf("partner SC_DANCING %+v", pd)
			}
		})
	}
}

// Marionette Control (PartyOnly) can target a member of the caster's
// party, and no other player (skill_castend_id_check with BCT_PARTY).
func TestMarionetteTargetsParty(t *testing.T) {
	z, p, _ := bardZone(t, reference.Renewal, clown)
	d := z.Skills.Get(skill.IDMarionette)
	m := partyOf(t, z, p, knightPlayer, "Member")[0]
	o := outsider(t, z, knightPlayer)
	if r := z.castEndIDCheck(p.entity, m.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Errorf("on a member: %v", r)
	}
	if r := z.castEndIDCheck(p.entity, o.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Errorf("on an outsider: %v", r)
	}
}

// Devotion protects a member of the Crusader's party: SC_DEVOTION names
// the Crusader, the slot and the skill's range, and a skill hit on the
// member goes to the Crusader. Out of range, the hit ends SC_DEVOTION
// and the member takes it (SkillSacrifice, skill_attack). An outsider
// cannot be the target (PartyOnly).
func TestDevotionProtectsMember(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, golems := crusaderZone(t, mode, false, 1)
			g := golems[0]
			m := partyOf(t, z, p, knightPlayer, "Member")[0]
			o := outsider(t, z, knightPlayer)
			d := z.Skills.Get(skill.IDDevotion)
			if r := z.castEndIDCheck(p.entity, o.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
				t.Errorf("Devotion on an outsider: %v", r)
			}
			cast(z, p, skill.IDDevotion, 5, m.entity)
			run(z, p, z.now+100, z.now+3000)
			e := m.entity.sc.Get(sc.SCDevotion)
			if e == nil || e.Val1 != int(p.charID) || e.Val2 != 0 || e.Val3 != d.RangeFor(p.entity.status, 5) {
				t.Fatalf("SC_DEVOTION %+v", e)
			}
			me := p.entity
			hp, mhp := me.hp, m.entity.hp
			bash := z.Skills.Get(skill.IDBash)
			dmg := z.skillAttack(battle.TypeWeapon, g, g, m.entity, bash, 1, 0)
			if dmg <= 0 {
				t.Fatal("the hit dealt nothing")
			}
			if m.entity.hp != mhp || me.hp != hp-dmg {
				t.Errorf("member HP %d of %d, Crusader HP %d, want %d", m.entity.hp, mhp, me.hp, hp-dmg)
			}
			// A normal attack on the member goes to the Crusader too
			// (battle_weapon_attack).
			// The golden zone rolls the lowest value, so the Golem must not
			// miss and the Knight must not dodge by luck.
			g.status.Hit = 10000
			ms := *m.entity.status
			ms.Flee2 = 0
			m.entity.status = &ms
			var normal int64
			for i := 0; i < 50 && normal == 0; i++ {
				hp = me.hp
				z.hits = z.hits[:0]
				z.strike(g, m.entity)
				for _, h := range z.hits {
					if h.target == m.entity {
						normal = h.result.Damage
					}
				}
			}
			if normal <= 0 {
				t.Fatal("no normal attack hit the member")
			}
			if m.entity.hp != mhp || me.hp != hp-normal {
				t.Errorf("normal attack: member HP %d of %d, Crusader HP %d, want %d", m.entity.hp, mhp, me.hp, hp-normal)
			}
			// Out of range, the link breaks on the next hit.
			m.entity.x = me.x + e.Val3 + 1
			hp = me.hp
			dmg = z.skillAttack(battle.TypeWeapon, g, g, m.entity, bash, 1, 0)
			if m.entity.sc.Has(sc.SCDevotion) || m.entity.hp != mhp-dmg || me.hp != hp {
				t.Errorf("out of range: devoted %v, member HP %d, want %d, Crusader HP %d of %d",
					m.entity.sc.Has(sc.SCDevotion), m.entity.hp, mhp-dmg, me.hp, hp)
			}
		})
	}
}

// Devotion takes one slot per level: at level 1 a second member finds
// none, at the cast start (unit_skilluse_id2) and at the cast end
// (SkillSacrifice).
func TestDevotionSlots(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.Renewal, false, 0)
	ms := partyOf(t, z, p, knightPlayer, "First", "Second")
	d := z.Skills.Get(skill.IDDevotion)
	z.castendNoDamage(p.entity, ms[0].entity, d, 1, 0)
	if !ms[0].entity.sc.Has(sc.SCDevotion) {
		t.Fatal("the first member is not devoted")
	}
	if z.skillUseID(p.entity, ms[1].entity, d, 1) {
		t.Error("the cast started without a free slot")
	}
	z.castendNoDamage(p.entity, ms[1].entity, d, 1, 0)
	if ms[1].entity.sc.Has(sc.SCDevotion) {
		t.Error("the second member is devoted without a free slot")
	}
	// The same member keeps its slot on a new cast.
	if _, ok := z.devotionSlot(p.entity, ms[0].entity, 1); !ok {
		t.Error("the devoted member has no slot")
	}
}

// The Crusader's Guard, Defending Aura, Shield Reflect and Endure pass
// on to a devoted member, and end on it with the Crusader's
// (status_change_start, status_change_end). A member devoted later
// inherits them, with val4 1 on Shield Reflect (SC_DEVOTION's start). The
// Crusader's death ends SC_DEVOTION (pc_dead).
func TestDevotionPassesGuards(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.Renewal, true, 0)
	ms := partyOf(t, z, p, knightPlayer, "First", "Second")
	me := p.entity
	d := z.Skills.Get(skill.IDDevotion)
	z.castendNoDamage(me, ms[0].entity, d, 5, 0)
	first := ms[0].entity
	z.scStart(me, me, sc.SCAutoguard, 100, 10, 60000)
	if g := first.sc.Get(sc.SCAutoguard); g == nil || g.Val2 != me.sc.Get(sc.SCAutoguard).Val2 {
		t.Fatalf("member's Guard %+v", g)
	}
	z.scStart(me, me, sc.SCReflectshield, 100, 10, 60000)
	if r := first.sc.Get(sc.SCReflectshield); r == nil || r.Val4 != 1 {
		t.Errorf("member's Shield Reflect %+v", r)
	}
	z.scEnd(me, sc.SCAutoguard)
	if first.sc.Has(sc.SCAutoguard) {
		t.Error("the member kept Guard after the Crusader's ended")
	}
	z.castendNoDamage(me, ms[1].entity, d, 5, 0)
	if r := ms[1].entity.sc.Get(sc.SCReflectshield); r == nil || r.Val4 != 1 {
		t.Errorf("inherited Shield Reflect %+v", r)
	}
	me.hp = 0
	z.die(me, nil)
	for _, m := range ms {
		if m.entity.sc.Has(sc.SCDevotion) {
			t.Errorf("%s stays devoted after the Crusader died", m.Name)
		}
	}
}

// Battle Chant gives a member of the caster's party on the cross one of
// its support effects; with the lowest rolls, a heal of 1000 HP
// (UNT_GOSPEL in skill_unit_onplace_timer). An outsider gets nothing.
func TestGospelSupportsParty(t *testing.T) {
	z, p, _ := crusaderZone(t, reference.PreRenewal, false, 0)
	m := partyOf(t, z, p, knightPlayer, "Member")[0]
	m.entity.x, m.entity.y = 21, 20
	o := outsider(t, z, knightPlayer)
	o.entity.x, o.entity.y = 19, 20
	m.entity.hp = 1
	z.castendNoDamage(p.entity, p.entity, z.Skills.Get(skill.IDGospel), 10, 0)
	z.unitTimer()
	if m.entity.hp != 1001 {
		t.Errorf("member HP %d, want 1001", m.entity.hp)
	}
	if !o.entity.sc.Empty() {
		t.Errorf("the outsider got %d statuses", o.entity.sc.Len())
	}
}

// Aid Condensed Potion heals the members of the caster's party in its
// square too (BCT_PARTY in SkillAidCondensedPotion::castendPos2), but not
// an outsider. Aid Potion (PartyOnly and GuildOnly) can target a member
// and no outsider.
func TestPotionsReachParty(t *testing.T) {
	z := potionZone(t)
	p := alchemistIn(t, z, 4019, 0, map[int]int{skill.IDSlimPitcher: 10, skill.IDPotionPitcher: 5, skill.IDLearningPotion: 10})
	me := p.entity
	me.x, me.y = 20, 20
	m := alchemistIn(t, z, 18, 0, map[int]int{})
	m.Name = "Member"
	m.entity.x, m.entity.y = 22, 22
	o := alchemistIn(t, z, 18, 0, map[int]int{})
	o.Name = "Outsider"
	o.entity.x, o.entity.y = 20, 22
	p.Name = "Leader"
	formParty(t, z, social.Options{}, p, m)
	if r := z.castEndIDCheck(me, m.entity, z.Skills.Get(skill.IDPotionPitcher)); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Errorf("Aid Potion on a member: %v", r)
	}
	if r := z.castEndIDCheck(me, o.entity, z.Skills.Get(skill.IDPotionPitcher)); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Errorf("Aid Potion on an outsider: %v", r)
	}
	for _, e := range []*entity{me, m.entity, o.entity} {
		e.hp = 1
	}
	give(t, z, p, 547, 1)
	castAt(z, p, skill.IDSlimPitcher, 10, 21, 21)
	run(z, p, 50, 3000)
	if me.hp == 1 || m.entity.hp == 1 {
		t.Errorf("caster HP %d and member HP %d, want both healed", me.hp, m.entity.hp)
	}
	if o.entity.hp != 1 {
		t.Errorf("outsider HP %d, want 1", o.entity.hp)
	}
}

// Ki Translation (PartyOnly) passes the caster's spheres to a member of
// its party, and cannot target an outsider (SkillKiTranslation,
// skill_castend_id_check).
func TestKiTranslationToParty(t *testing.T) {
	z, p, _ := monkZone(t)
	m := partyOf(t, z, p, knightPlayer, "Member")[0]
	o := outsider(t, z, knightPlayer)
	d := z.Skills.Get(skill.IDKiTranslation)
	if r := z.castEndIDCheck(p.entity, o.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Errorf("on an outsider: %v", r)
	}
	if r := z.castEndIDCheck(p.entity, m.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Fatalf("on a member: %v", r)
	}
	giveSpheres(z, p, 5)
	z.castendNoDamage(p.entity, m.entity, d, 1, 0)
	if n := m.spiritBalls(); n != 1 {
		t.Errorf("member spheres %d, want 1", n)
	}
}

// skill_castend_id_check lets a PartyOnly skill target a member of the
// caster's party and no outsider, for support skills (Endow Blaze,
// Enchant Poison, Aid Berserk Potion) and for skills on enemies that
// also take the party (Dispell, Soul Change).
func TestPartyOnlyTargets(t *testing.T) {
	z, p, _ := priestZone(t, reference.Renewal)
	m := partyOf(t, z, p, knightPlayer, "Member")[0]
	o := outsider(t, z, knightPlayer)
	for _, id := range []int{skill.IDFlameLauncher, skill.IDEnchantPoison, skill.IDBerserkPitcher, skill.IDDispell,
		skill.IDSoulChange} {
		d := z.Skills.Get(id)
		if r := z.castEndIDCheck(p.entity, m.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
			t.Errorf("skill %d on a member: %v", id, r)
		}
		if r := z.castEndIDCheck(p.entity, o.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
			t.Errorf("skill %d on an outsider: %v", id, r)
		}
	}
}

// Renewal's Loud Exclamation reaches the members in its splash on the
// caster's map (SkillCrazyUproar). In pre-renewal it is the caster's
// alone (StatusSkillImpl).
func TestLoudReachesPartyInRenewal(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, _ := smithZone(t, mode, smithAxe, 0)
			m := partyOf(t, z, p, knightPlayer, "Member")[0]
			o := outsider(t, z, knightPlayer)
			z.castendNoDamage(p.entity, p.entity, z.Skills.Get(skill.IDLoud), 1, 0)
			if !p.entity.sc.Has(sc.SCLoud) {
				t.Error("the caster has no SC_LOUD")
			}
			if got := m.entity.sc.Has(sc.SCLoud); got != (mode == reference.Renewal) {
				t.Errorf("member has SC_LOUD %v", got)
			}
			if o.entity.sc.Has(sc.SCLoud) {
				t.Error("the outsider has SC_LOUD")
			}
		})
	}
}
