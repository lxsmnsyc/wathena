package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Tests of companions, mounts and carts (proposal 0002, review decision
// 25).

// testCompanionNames resolve core's skill names for test records.
var testCompanionNames = companion.Names{
	Skill: func(name string) (int, bool) {
		id, ok := map[string]int{"HT_FALCON": skill.IDFalcon, "KN_RIDING": skill.IDRiding, "SM_BASH": 5}[name]
		return id, ok
	},
	Bonus: &bonus.Tables{Constants: map[string]map[string]int64{}},
}

// withModCompanions gives a zone core's records and a mod's: an owl that
// acts as a falcon with STR +2, and a horse that acts as a Peco Peco
// with a speed of 10.
func withModCompanions(t *testing.T, z *Zone) (owl, horse *companion.Record) {
	t.Helper()
	core := companion.Core()
	set := &companion.Set{}
	for _, n := range core.Companions.Names() {
		set.Add(companion.Companions, core.Companions.ByName(n))
	}
	for _, n := range core.Mounts.Names() {
		set.Add(companion.Mounts, core.Mounts.ByName(n))
	}
	var err error
	owl, err = companion.Companions.Build("example.test:Owl", "example.test", 10000, map[string]any{
		"look": "falcon", "acts_as": "falcon", "requires": []any{map[string]any{"skill": "HT_FALCON", "level": 1}},
		"bonuses": []any{[]any{"str", 2}},
	}, testCompanionNames)
	if err != nil {
		t.Fatal(err)
	}
	horse, err = companion.Mounts.Build("example.test:Horse", "example.test", 10000, map[string]any{
		"look": "peco_peco", "acts_as": "peco_peco", "speed": 10,
		"requires": []any{map[string]any{"skill": "KN_RIDING"}},
	}, testCompanionNames)
	if err != nil {
		t.Fatal(err)
	}
	set.Add(companion.Companions, owl)
	set.Add(companion.Mounts, horse)
	z.Mods = &Mods{Companions: set}
	return owl, horse
}

// A mod companion that acts as a falcon casts the falcon's skills, shows
// the falcon and gives its bonuses while present.
func TestModCompanionActsAsFalcon(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	owl, _ := withModCompanions(t, z)
	str := p.entity.status.Str
	delete(p.char.Skills, skill.IDFalcon)
	z.recalc(p)
	if z.setCompanion(p, owl) || p.companion != nil {
		t.Fatal("an owl without Falconry Mastery")
	}
	p.char.Skills[skill.IDFalcon] = 1
	z.recalc(p)
	if !z.setCompanion(p, owl) || !isFalcon(p) || p.record.Options&companion.OptionFalcon == 0 {
		t.Fatalf("owl: companion %v, options %#x", p.companion, p.record.Options)
	}
	z.flushStats()
	if p.entity.status.Str != str+2 {
		t.Errorf("STR %d with the owl, want %d", p.entity.status.Str, str+2)
	}
	if p.record.Companion != "example.test:Owl" {
		t.Errorf("saved companion %q", p.record.Companion)
	}
	m := monsterOf(z, mobLunatic)
	cast(z, p, skill.IDBlitzBeat, 5, m)
	ev := run(z, p, z.now+50, z.now+3000)
	if hits := skillDamage(ev, skill.IDBlitzBeat); len(hits) != 1 {
		t.Fatalf("Blitz Beat with the owl: hits %d, failures %v", len(hits), skillFailures(ev))
	}
	if !z.setCompanion(p, nil) || p.record.Options&companion.OptionFalcon != 0 || p.record.Companion != "" {
		t.Errorf("after removal: options %#x, companion %q", p.record.Options, p.record.Companion)
	}
	z.flushStats()
	if p.entity.status.Str != str {
		t.Errorf("STR %d after the owl left, want %d", p.entity.status.Str, str)
	}
}

// A skill's requirements may name a companion, and a companion that acts
// as it passes.
func TestSkillRequiresCompanion(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	owl, _ := withModCompanions(t, z)
	db := z.Skills.Clone()
	d := db.Get(skill.IDDoubleStrafe).Copy()
	d.Requires.Companion = "falcon"
	db.Put(d)
	z.Skills = db
	m := monsterOf(z, mobLunatic)
	cast(z, p, skill.IDDoubleStrafe, 1, m)
	ev := run(z, p, z.now+50, z.now+500)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Fatalf("without a companion: failures %v", f)
	}
	z.setCompanion(p, owl)
	cast(z, p, skill.IDDoubleStrafe, 1, m)
	ev = run(z, p, z.now+50, z.now+3000)
	if f := skillFailures(ev); len(f) != 0 {
		t.Fatalf("with the owl: failures %v", f)
	}
}

// A mod mount that acts as a Peco Peco follows the riding rules, with
// its own speed.
func TestModMountActsAsPeco(t *testing.T) {
	z, p, _ := knightZone(t, reference.Renewal, javelin, 0)
	_, horse := withModCompanions(t, z)
	walk, weight := p.entity.status.Speed, p.maxWeight
	if !z.setRiding(p, true) {
		t.Fatal("no Peco Peco")
	}
	z.flushStats()
	peco := p.entity.status.Speed
	if !z.setMount(p, horse) || !p.riding() || p.record.Options&companion.OptionRiding == 0 {
		t.Fatal("no horse")
	}
	z.flushStats()
	if p.maxWeight != weight+10000 {
		t.Errorf("max weight %d on the horse, want %d", p.maxWeight, weight+10000)
	}
	if want := walk * 90 / 100; p.entity.status.Speed != want || peco != walk*75/100 {
		t.Errorf("speed: walking %d, Peco Peco %d, horse %d, want %d", walk, peco, p.entity.status.Speed, want)
	}
	if p.Look.Job != 4014 {
		t.Errorf("a Lord Knight on a horse shows job %d", p.Look.Job)
	}
	if !z.setMount(p, nil) || p.riding() || p.Look.Job != 4008 {
		t.Error("still mounted")
	}
}

// Looks carry the record IDs of the companion and the mount, so the
// client draws a record's art. A mount of art keeps the body on foot,
// which the client draws over the mount (decision 0036).
func TestLooksCarryCompanionsAndMounts(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 11, mobLunatic)
	owl, _ := withModCompanions(t, z)
	p.char.Skills[skill.IDFalcon] = 1
	z.recalc(p)
	p.entity.lookChanged = false
	if !z.setCompanion(p, owl) || p.Look.Companion != 10000 || !p.entity.lookChanged || p.Look.message().GetCompanion() != 10000 {
		t.Fatalf("look %+v with the owl", p.Look)
	}
	z.setCompanion(p, nil)
	if p.Look.Companion != 0 {
		t.Errorf("look %+v without a companion", p.Look)
	}

	k, knight, _ := knightZone(t, reference.Renewal, javelin, 0)
	_, horse := withModCompanions(t, k)
	art, err := companion.Mounts.Build("example.test:Pony", "example.test", 10001, map[string]any{
		"look": "mod/example.test/sprite/mount/Pony", "acts_as": "peco_peco",
		"requires": []any{map[string]any{"skill": "KN_RIDING"}},
	}, testCompanionNames)
	if err != nil {
		t.Fatal(err)
	}
	k.Mods.Companions.Add(companion.Mounts, art)
	if !k.setMount(knight, horse) || knight.Look.Mount != 10000 || knight.Look.Job != 4014 {
		t.Errorf("look %+v on a horse without art", knight.Look)
	}
	if !k.setMount(knight, art) || knight.Look.Mount != 10001 || knight.Look.Job != 4008 || !knight.riding() {
		t.Errorf("look %+v on a pony of art: want the mount and the body on foot", knight.Look)
	}
	if knight.record.Options&companion.OptionRiding == 0 {
		t.Error("the pony of art does not set the riding bit")
	}
	k.setMount(knight, nil)
	if knight.Look.Mount != 0 || knight.Look.Job != 4008 {
		t.Errorf("look %+v after the ride", knight.Look)
	}
}

// pc_jobchange removes the companion and mount the new job does not meet
// requires for, and always the Madogear.
func TestJobChangeDropsCompanions(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 11, 99, 50)
	p.char.Skills[skill.IDFalcon] = 1
	z.setFalcon(p, true)
	z.setCart(p, 0)
	if !isFalcon(p) {
		t.Fatal("no falcon")
	}
	if !z.changeJob(p, 4012, true) || !isFalcon(p) {
		t.Fatal("a Sniper with Falconry Mastery lost its falcon")
	}
	delete(p.char.Skills, skill.IDFalcon)
	if !z.changeJob(p, 7, true) || isFalcon(p) || p.record.Options&companion.OptionFalcon != 0 {
		t.Error("a Knight kept the falcon")
	}
	p.char.Skills[2255] = 1 // NC_MADOLICENCE
	mado := companion.Core().Mounts.ByName("madogear")
	if !z.setMount(p, mado) || p.mountRule() != companion.RuleMadogear {
		t.Fatal("no Madogear")
	}
	if !z.changeJob(p, 14, true) || p.mount != nil {
		t.Error("the Madogear stayed after a job change")
	}
}

// A character saved before migration 00014 has look bits only: the
// server finds core's records from them. A name a removed mod took
// away is dropped.
func TestRestoreCompanions(t *testing.T) {
	z := saveZone()
	c := savedCharacter()
	c.Options = companion.OptionFalcon | companion.OptionRiding | 0x0008
	p := addCharacter(z, c, nil)
	if p.companion == nil || p.companion.Name != "falcon" || p.mount == nil || p.mount.Name != "peco_peco" {
		t.Fatalf("companion %v, mount %v", p.companion, p.mount)
	}
	if companion.CartLook(p.record.Options) != 1 || p.record.Companion != "falcon" || p.record.Mount != "peco_peco" {
		t.Errorf("options %#x, record %q %q", p.record.Options, p.record.Companion, p.record.Mount)
	}
	if s := z.snapshot(p); s.Companion != "falcon" || s.Mount != "peco_peco" {
		t.Errorf("saved %q %q", s.Companion, s.Mount)
	}
	z2 := saveZone()
	c = savedCharacter()
	c.Companion, c.Mount = "example.gone:Owl", "grand_peco"
	c.Options = companion.OptionFalcon | companion.OptionRiding
	p = addCharacter(z2, c, nil)
	if p.companion != nil || p.record.Options&companion.OptionFalcon != 0 || p.record.Companion != "" {
		t.Errorf("a removed companion stayed: %v, options %#x", p.companion, p.record.Options)
	}
	if p.mount == nil || p.mount.Name != "grand_peco" {
		t.Errorf("mount %v", p.mount)
	}
}

// A character whose mod job was removed loads as a Novice.
func TestRemovedModJobLoadsAsNovice(t *testing.T) {
	z := goldenZone(t, reference.Renewal)
	c := &character.Character{Info: character.Info{
		Name: "Desperado", Job: 10000, Male: true, BaseLevel: 60, JobLevel: 30, JobExp: 500,
		Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}, HP: 100, SP: 10,
	}}
	p := addCharacter(z, c, nil)
	if p.char.Job != 0 || p.progress.Job != 0 || p.progress.JobLevel != 10 || p.progress.JobExp != 0 {
		t.Fatalf("job %d, progress %+v", p.char.Job, p.progress)
	}
	if p.entity.status == nil || p.entity.status.MaxHP <= 0 {
		t.Fatal("no stats")
	}
	if s := z.snapshot(p); s.Job != 0 || s.JobLevel != 10 {
		t.Errorf("saved job %d at job level %d", s.Job, s.JobLevel)
	}
	// A job the world has keeps its levels.
	c = &character.Character{Info: character.Info{Name: "Knight", Job: 7, Male: true, BaseLevel: 60, JobLevel: 30,
		Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}, HP: 100}}
	if p := addCharacter(z, c, nil); p.char.Job != 7 || p.progress.JobLevel != 30 {
		t.Errorf("a Knight loaded as job %d at job level %d", p.char.Job, p.progress.JobLevel)
	}
}
