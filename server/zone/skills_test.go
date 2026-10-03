package zone

import (
	"log/slog"
	"os"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// testSkills is a mode's skill data and trees.
func testSkills(t *testing.T, mode reference.Mode) (*skill.DB, *skill.Trees) {
	t.Helper()
	var d skill.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/skills.json", &d); err != nil {
		t.Fatal(err)
	}
	db, err := skill.Load(&d)
	if err != nil {
		t.Fatal(err)
	}
	var td skill.TreeData
	if err := reference.LoadFile("extracts/"+string(mode)+"/skill_trees.json", &td); err != nil {
		t.Fatal(err)
	}
	return db, skill.LoadTrees(&td)
}

// skillZone is statusZone with renewal's skills, and the player as a
// job with skills learned. It stands two cells from the Poring.
func skillZone(t *testing.T, job int, skills map[int]int) (*Zone, *Player, *entity) {
	t.Helper()
	z, p, m := statusZone(t)
	z.Skills, z.SkillTrees = testSkills(t, reference.Renewal)
	p.char.Job = job
	p.char.Skills = skills
	p.progress.BaseLevel, p.progress.JobLevel = 50, 40
	p.progress.Job = job
	z.recalc(p)
	p.sp = int64(p.entity.status.MaxSP)
	p.entity.hp = int64(p.entity.status.MaxHP)
	events(p)
	return z, p, m
}

// cast sends a skill command on an entity.
func cast(z *Zone, p *Player, id, lv int, target *entity) {
	c := &useSkillCommand{skill: id, level: lv}
	if target != nil {
		c.target = SkillTarget{Entity: p.netID(target), HasEntity: true}
	}
	z.handle(command{player: p, useSkill: c})
}

// castAt sends a ground skill command on a cell.
func castAt(z *Zone, p *Player, id, lv, x, y int) {
	z.handle(command{player: p, useSkill: &useSkillCommand{skill: id, level: lv,
		target: SkillTarget{X: x, Y: y, HasCell: true}}})
}

// run steps the zone from one time to another and collects the player's
// events.
func run(z *Zone, p *Player, from, to int64) []*wathenav1.Event {
	var out []*wathenav1.Event
	for now := from; now <= to; now += 50 {
		z.stepAt(now)
		out = append(out, events(p)...)
	}
	return out
}

func skillDamage(ev []*wathenav1.Event, id int) []*wathenav1.Damage {
	var out []*wathenav1.Damage
	for _, e := range ev {
		if d := e.GetDamage(); d != nil && d.GetSkillId() == uint32(id) {
			out = append(out, d)
		}
	}
	return out
}

func skillFailures(ev []*wathenav1.Event) []wathenav1.SkillFailure {
	var out []wathenav1.SkillFailure
	for _, e := range ev {
		if f := e.GetSkillFailed(); f != nil {
			out = append(out, f.GetReason())
		}
	}
	return out
}

func TestBashCostsSPAndHits(t *testing.T) {
	z, p, m := skillZone(t, 1, map[int]int{skill.IDBasic: 9, skill.IDBash: 5})
	m.status = &battle.Status{MaxHP: 100000, Level: 1, Vit: 1, Luk: 1, Int: 1, Flee: 1, RHW: battle.Weapon{Range: 1}}
	m.hp = 100000
	sp := p.sp
	cast(z, p, skill.IDBash, 5, m)
	ev := run(z, p, 100, 2000)
	hits := skillDamage(ev, skill.IDBash)
	if len(hits) != 1 {
		t.Fatalf("%d Bash hits, want 1; events %v", len(hits), ev)
	}
	if p.sp != sp-8 {
		t.Errorf("SP %d, want %d", p.sp, sp-8)
	}
	if m.hp != 100000-hits[0].GetAmount() {
		t.Errorf("monster HP %d after %d damage", m.hp, hits[0].GetAmount())
	}
}

func TestSkillNeedsSP(t *testing.T) {
	z, p, m := skillZone(t, 1, map[int]int{skill.IDBasic: 9, skill.IDBash: 10})
	p.sp = 5
	cast(z, p, skill.IDBash, 10, m)
	ev := run(z, p, 100, 200)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_SP_INSUFFICIENT {
		t.Fatalf("failures %v", f)
	}
}

func TestCastTimeAndInterruption(t *testing.T) {
	z, p, m := skillZone(t, 2, map[int]int{skill.IDBasic: 9, skill.IDFireBolt: 5})
	m.status = &battle.Status{MaxHP: 100000, Level: 1, Vit: 1, Luk: 1, Int: 1, Flee: 1, RHW: battle.Weapon{Range: 1}}
	m.hp = 100000
	cast(z, p, skill.IDFireBolt, 5, m)
	ev := run(z, p, 100, 100)
	var castMs uint32
	for _, e := range ev {
		if c := e.GetSkillCast(); c != nil {
			castMs = c.GetCastMs()
		}
	}
	if castMs == 0 || p.entity.cast == nil {
		t.Fatalf("no cast bar: %v", ev)
	}
	// A hit with damage stops the cast.
	z.skillDamage(m, m, p.entity, z.Skills.Get(skill.IDBash), 1, battle.Result{Kind: battle.Hit, Damage: 1, Hits: 1}, 0)
	if p.entity.cast != nil {
		t.Fatal("the hit did not stop the cast")
	}
	ev = run(z, p, 150, 3000)
	if len(skillDamage(ev, skill.IDFireBolt)) != 0 {
		t.Error("the stopped cast hit")
	}
	cancelled := false
	for _, e := range ev {
		if e.GetSkillCastCancel() != nil {
			cancelled = true
		}
	}
	if !cancelled {
		t.Error("no SkillCastCancel")
	}

	// With Phen's bonus, hits do not stop it.
	p.entity.status.Player.Cast.NoCastCancel = true
	cast(z, p, skill.IDFireBolt, 5, m)
	z.stepAt(3050)
	z.skillDamage(m, m, p.entity, z.Skills.Get(skill.IDBash), 1, battle.Result{Kind: battle.Hit, Damage: 1, Hits: 1}, 0)
	if p.entity.cast == nil {
		t.Fatal("the hit stopped an uninterruptible cast")
	}
	ev = run(z, p, 3100, 8000)
	if len(skillDamage(ev, skill.IDFireBolt)) != 1 {
		t.Errorf("Fire Bolt did not land after its cast: %v", ev)
	}
}

func TestOutOfRangeWalksThenCasts(t *testing.T) {
	z, p, m := skillZone(t, 1, map[int]int{skill.IDBasic: 9, skill.IDBash: 1})
	m.status = &battle.Status{MaxHP: 100000, Level: 1, Vit: 1, Luk: 1, Int: 1, Flee: 1, RHW: battle.Weapon{Range: 1}}
	m.hp = 100000
	p.entity.x = m.x + 5
	cast(z, p, skill.IDBash, 1, m)
	ev := run(z, p, 100, 3000)
	if len(skillDamage(ev, skill.IDBash)) != 1 {
		t.Fatalf("no Bash after the walk: %v", ev)
	}
	if chebyshev(p.entity, m) > 1 {
		t.Errorf("player %d cells away", chebyshev(p.entity, m))
	}
}

func TestHealSelfAndCooldown(t *testing.T) {
	z, p, _ := skillZone(t, 4, map[int]int{skill.IDBasic: 9, skill.IDHeal: 5, skill.IDAngelus: 1})
	p.entity.hp = 10
	cast(z, p, skill.IDHeal, 5, p.entity)
	ev := run(z, p, 100, 1000)
	var healed int64
	for _, e := range ev {
		if s := e.GetSkillEffect(); s != nil && s.GetSkillId() == skill.IDHeal {
			healed = s.GetValue()
		}
	}
	if healed <= 0 || p.entity.hp != min(10+healed, int64(p.entity.status.MaxHP)) {
		t.Fatalf("healed %d, HP %d", healed, p.entity.hp)
	}
	// Angelus has a 30 second cooldown in renewal.
	cast(z, p, skill.IDAngelus, 1, p.entity)
	run(z, p, 1050, 3000)
	if !p.entity.sc.Has(sc.SCAngelus) {
		t.Fatal("no Angelus")
	}
	cast(z, p, skill.IDAngelus, 1, p.entity)
	ev = run(z, p, 3050, 3100)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_SKILLINTERVAL {
		t.Errorf("failures %v", f)
	}
}

func TestLearnSkill(t *testing.T) {
	z, p, _ := skillZone(t, 1, map[int]int{skill.IDBasic: 9})
	p.progress.SkillPoints = 2
	z.handle(command{player: p, learnSkill: new(skill.IDBash)})
	z.handle(command{player: p, learnSkill: new(skill.IDProvoke)})
	if p.char.Skills[skill.IDBash] != 1 || p.char.Skills[skill.IDProvoke] != 1 || p.progress.SkillPoints != 0 {
		t.Fatalf("skills %v, points %d", p.char.Skills, p.progress.SkillPoints)
	}
	// No points left.
	z.handle(command{player: p, learnSkill: new(skill.IDBash)})
	if p.char.Skills[skill.IDBash] != 1 {
		t.Error("learned without a point")
	}
	ev := run(z, p, 100, 100)
	found := false
	for _, e := range ev {
		if e.GetSkillList() != nil {
			found = true
		}
	}
	if !found {
		t.Error("no SkillList after learning")
	}
}

func TestSafetyWallBlocksMelee(t *testing.T) {
	z, p, m := skillZone(t, 2, map[int]int{skill.IDBasic: 9, skill.IDSafetyWall: 1})
	give(t, z, p, itemBlueGemstone, 1)
	castAt(z, p, skill.IDSafetyWall, 1, p.entity.x, p.entity.y)
	run(z, p, 100, 6000)
	if !p.entity.sc.Has(sc.SCSafetywall) {
		t.Fatal("no Safety Wall status on the caster's cell")
	}
	if z.countItem(p, itemBlueGemstone) != 0 {
		t.Error("the gem was not used")
	}
	// A melee hit is blocked and uses one of its 2 hits.
	before := p.entity.hp
	m.target, m.repeat = p.entity, true
	m.status = &battle.Status{MaxHP: 1000, Hit: 500, ADelay: 1000, AMotion: 500, DMotion: 300, Level: 1,
		RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	m.x, m.y = p.entity.x+1, p.entity.y
	run(z, p, 6050, 6100)
	if p.entity.hp != before {
		t.Errorf("HP %d -> %d through Safety Wall", before, p.entity.hp)
	}
}

func TestFireWallHitsAndPushes(t *testing.T) {
	z, p, m := skillZone(t, 2, map[int]int{skill.IDBasic: 9, skill.IDFireWall: 10})
	m.status = &battle.Status{MaxHP: 100000, Level: 1, Vit: 1, Luk: 1, Int: 1, Flee: 1, DefEle: 0, EleLv: 1, RHW: battle.Weapon{Range: 1}}
	m.hp = 100000
	m.monster.spawn.CanMove = false
	if m.status != nil {
		// The monster AI does not move a monster without can_move.
		m.status.Mode &^= 1
	}
	castAt(z, p, skill.IDFireWall, 10, m.x, m.y)
	ev := run(z, p, 100, 6000)
	if len(z.groups) == 0 && len(skillDamage(ev, skill.IDFireWall)) == 0 {
		t.Fatal("no Fire Wall")
	}
	if len(skillDamage(ev, skill.IDFireWall)) == 0 {
		t.Errorf("Fire Wall did not hit: %v", ev)
	}
}

func TestEntityQueries(t *testing.T) {
	z := New("map/test", open(40), nil, nil)
	mk := func(x, y int, hp int64) *entity {
		e := &entity{monster: &monster{spawn: &Spawn{}}, x: x, y: y, status: &battle.Status{MaxHP: 100}, hp: hp}
		z.entities.add(e)
		return e
	}
	a, b, c := mk(10, 10, 50), mk(12, 10, 20), mk(12, 12, 90)
	got := z.findEntities(&query{area: areaRadius, x: 10, y: 10, radius: 2, sort: "nearest"})
	if len(got) != 3 || got[0] != a || got[1] != b || got[2] != c {
		t.Fatalf("square: %v", got)
	}
	// The circle leaves out the corner (2, 2): 8 > 4 + 1.
	got = z.findEntities(&query{area: areaRadius, x: 10, y: 10, radius: 2, circle: true})
	if len(got) != 2 {
		t.Errorf("circle: %d", len(got))
	}
	got = z.findEntities(&query{area: areaRadius, x: 10, y: 10, radius: 5, sort: "lowest_hp", limit: 1})
	if len(got) != 1 || got[0] != b {
		t.Errorf("lowest_hp: %v", got)
	}
	got = z.findEntities(&query{area: areaRadius, x: 10, y: 10, radius: 5, exclude: map[*entity]bool{a: true}, sort: "farthest"})
	if len(got) != 2 || got[0] != b || got[1] != c {
		// b and c are both 2 away on the square: ties go by entity ID.
		t.Errorf("farthest: %v", got)
	}
	got = z.findEntities(&query{area: areaLine, x0: 10, y0: 10, x1: 14, y1: 10, width: 0})
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("line: %v", got)
	}
}

// dummy makes the Poring a target with plenty of HP that never dodges.
func dummy(m *entity) {
	m.status = &battle.Status{MaxHP: 100000, Level: 1, Vit: 1, Luk: 1, Int: 1, Flee: 1, EleLv: 1, RHW: battle.Weapon{Range: 1}}
	m.hp = 100000
}

func TestMagnumBreakSplashAndFireBonus(t *testing.T) {
	z, p, m := skillZone(t, 1, map[int]int{skill.IDBasic: 9, skill.IDBash: 5, skill.IDMagnumBreak: 10})
	dummy(m)
	p.entity.x, p.entity.y = m.x+1, m.y
	cast(z, p, skill.IDMagnumBreak, 10, nil)
	ev := run(z, p, 100, 500)
	if len(skillDamage(ev, skill.IDMagnumBreak)) != 1 {
		t.Fatalf("Magnum Break hits: %v", ev)
	}
	if !p.entity.sc.Has(sc.SCSubWeaponPro) {
		t.Error("no fire bonus after Magnum Break in renewal")
	}
	// The monster was pushed 2 cells away.
	if d := chebyshev(p.entity, m); d != 3 {
		t.Errorf("monster %d cells away after the knockback", d)
	}
}

func TestProvokeAndTrickDead(t *testing.T) {
	z, p, m := skillZone(t, 1, map[int]int{skill.IDBasic: 9, skill.IDProvoke: 10, skill.IDTrickDead: 1})
	dummy(m)
	m.monster.spawn.Status.Mode |= modeCanAttack
	m.status.Mode |= modeCanAttack
	cast(z, p, skill.IDProvoke, 10, m)
	run(z, p, 100, 200)
	if !m.sc.Has(sc.SCProvoke) {
		t.Fatal("no Provoke")
	}
	// mob_target: the AI takes the caster as its target.
	if m.monster.target != p.entity && m.target != p.entity {
		t.Error("the provoked monster did not turn on the caster")
	}
	run(z, p, 250, 2000)
	cast(z, p, skill.IDTrickDead, 1, p.entity)
	run(z, p, 2050, 2100)
	if !p.entity.sc.Has(sc.SCTrickdead) {
		t.Fatal("no Trick Dead")
	}
	run(z, p, 2150, 3000)
	if m.target == p.entity || m.monster.target == p.entity {
		t.Error("the monster still attacks a player playing dead")
	}
	// Toggling it off costs nothing.
	sp := p.sp
	cast(z, p, skill.IDTrickDead, 1, p.entity)
	run(z, p, 3050, 3100)
	if p.entity.sc.Has(sc.SCTrickdead) || p.sp != sp {
		t.Errorf("Trick Dead on %v, SP %d -> %d", p.entity.sc.Has(sc.SCTrickdead), sp, p.sp)
	}
}

func TestStoneCurseUsesRedGemstone(t *testing.T) {
	z, p, m := skillZone(t, 2, map[int]int{skill.IDBasic: 9, skill.IDStoneCurse: 1})
	dummy(m)
	cast(z, p, skill.IDStoneCurse, 1, m)
	ev := run(z, p, 100, 3000)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_REDJAMSTONE {
		t.Fatalf("failures %v", f)
	}
	give(t, z, p, itemRedGemstone, 1)
	cast(z, p, skill.IDStoneCurse, 1, m)
	run(z, p, 3050, 6000)
	if z.countItem(p, itemRedGemstone) != 0 {
		t.Error("the gem was not used")
	}
}

func TestHidingToggle(t *testing.T) {
	z, p, _ := skillZone(t, 6, map[int]int{skill.IDBasic: 9, skill.IDHiding: 1})
	cast(z, p, skill.IDHiding, 1, p.entity)
	run(z, p, 100, 1200)
	if !p.entity.sc.Has(sc.SCHiding) {
		t.Fatal("not hidden")
	}
	sp := p.sp
	cast(z, p, skill.IDHiding, 1, p.entity)
	run(z, p, 1250, 1300)
	if p.entity.sc.Has(sc.SCHiding) {
		t.Error("still hidden")
	}
	if p.sp < sp-1 {
		t.Errorf("ending Hiding cost SP: %d -> %d", sp, p.sp)
	}
}

func TestAutoSpellOnSkill(t *testing.T) {
	z, p, m := skillZone(t, 1, map[int]int{skill.IDBasic: 9, skill.IDBash: 5})
	dummy(m)
	z.Status.SetAutoSpellSelf(func(int) bool { return false })
	p.bonuses.AutoSpellOnSkill = []status.AutoSpell{{Skill: skill.IDFireBolt, Level: 3, Rate: 1000, TriggerSkill: skill.IDBash}}
	cast(z, p, skill.IDBash, 5, m)
	ev := run(z, p, 100, 1000)
	if len(skillDamage(ev, skill.IDBash)) != 1 || len(skillDamage(ev, skill.IDFireBolt)) != 1 {
		t.Fatalf("Bash %d, Fire Bolt %d", len(skillDamage(ev, skill.IDBash)), len(skillDamage(ev, skill.IDFireBolt)))
	}
}

func TestChainLightningMod(t *testing.T) {
	z, p, m := skillZone(t, 2, map[int]int{skill.IDBasic: 9})
	src, err := os.ReadFile("../../mods/examples/chain-lightning/scripts/skills/chain_lightning.lua")
	if err != nil {
		t.Fatal(err)
	}
	const modID = "example.chain-lightning"
	sc := npcMod("")
	sc.Mod, sc.Visible = modID, []string{modID}
	sc.Files = map[string]string{"scripts/skills/chain_lightning.lua": string(src)}
	z.Mods.Scripts = append(z.Mods.Scripts, sc)
	z.Mods.IDs = append(z.Mods.IDs, modID)
	d := skill.NewModDef(20000, modID+":CHAIN_LIGHTNING", modID)
	d.MaxLevel, d.Type, d.Target, d.Icon = 5, battle.TypeMagic, skill.TargetEnemy, skill.IDLightningBolt
	for i := range d.Element {
		d.Element[i], d.Range[i] = 4, 9
	}
	d.Script = modID + ":scripts/skills/chain_lightning.lua"
	z.Skills.Put(d)
	p.char.Skills[d.ID] = 5
	// Three monsters in a row, 2 cells apart.
	dummy(m)
	var others []*entity
	for i := 1; i <= 2; i++ {
		e := z.spawnOne(m.monster.spawn)
		dummy(e)
		e.x, e.y = m.x-2*i, m.y
		others = append(others, e)
	}
	cast(z, p, d.ID, 5, m)
	ev := run(z, p, 100, 3000)
	hits := skillDamage(ev, skill.IDLightningBolt)
	if len(hits) != 3 {
		t.Fatalf("%d chain hits, want 3: %v", len(hits), ev)
	}
	for _, e := range append(others, m) {
		if e.hp == 100000 {
			t.Error("a monster was not hit")
		}
	}
}

func TestSkillUsedEventAndDamageHook(t *testing.T) {
	z, p, m := skillZone(t, 1, map[int]int{skill.IDBasic: 9, skill.IDBash: 5})
	dummy(m)
	src := `
local events = require "wa.events"
local hooks = require "wa.hooks"
return function()
  events.on("skill_used", function(use)
    use.caster:send_message("used " .. use.skill .. " at " .. use.level)
  end)
  hooks.on("skill_damage", function(o, next)
    return next() * 2
  end)
end`
	tm := testMod(src)
	tm.Grants["rules.override"] = true
	z.Mods.Scripts = append(z.Mods.Scripts, tm)
	z.Mods.IDs = append(z.Mods.IDs, "example.test")
	logs := &syncBuffer{}
	z.Mods.Log = slog.New(slog.NewTextHandler(logs, nil))
	z.startScripts()
	t.Cleanup(z.stopScripts)
	if z.scripts == nil || z.scripts.Context("example.test") == nil {
		t.Fatalf("the mod did not start: %s", logs)
	}
	z.runScripts()
	cast(z, p, skill.IDBash, 5, m)
	ev := run(z, p, 100, 1000)
	if len(skillDamage(ev, skill.IDBash)) != 1 {
		t.Fatalf("no Bash: %v", ev)
	}
	found := false
	for _, e := range ev {
		if c := e.GetChat(); c != nil && c.GetText() == "used 5 at 5" {
			found = true
		}
	}
	if !found {
		t.Errorf("no skill_used message: %v", ev)
	}
}
