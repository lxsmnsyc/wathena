package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Hand tests of the Sniper's skills.

// monsters are a zone's monsters in the order the zone added them.
func monsters(z *Zone) []*entity {
	var out []*entity
	for e := range z.entities.all() {
		if e.monster != nil {
			out = append(out, e)
		}
	}
	return out
}

// Focused Arrow Strike (SkillFocusedArrowStrike): in pre-renewal it hits
// the enemies in a path of width 1 and length 14 toward the target
// (map_foreachindir), past the target too; in renewal the enemies within
// 2 cells of the target.
func TestSharpShootingArea(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p := hunterZone(t, mode, 4012, mobLunatic, mobLunatic, mobLunatic, mobLunatic)
			ms := monsters(z)
			x, y := p.entity.x, p.entity.y
			target, onLine, offLine, behind := ms[0], ms[1], ms[2], ms[3]
			target.x, target.y = x+6, y
			onLine.x, onLine.y = x+3, y+1
			offLine.x, offLine.y = x+3, y+3
			behind.x, behind.y = x+10, y
			start := int64(1 << 40)
			cast(z, p, skill.IDSharpShooting, 5, target)
			ev := run(z, p, z.now+50, z.now+3000)
			if f := skillFailures(ev); len(f) != 0 {
				t.Fatalf("failures %v", f)
			}
			hit := func(e *entity) bool { return e.hp < start }
			if !hit(target) {
				t.Fatal("the target was not hit")
			}
			if mode == reference.PreRenewal {
				if !hit(onLine) || !hit(behind) || hit(offLine) {
					t.Errorf("line: on %v, behind %v, off %v; want true, true, false", hit(onLine), hit(behind), hit(offLine))
				}
				return
			}
			if hit(onLine) || hit(behind) || hit(offLine) {
				t.Errorf("renewal splash reached units more than 2 cells from the target: %v %v %v",
					hit(onLine), hit(behind), hit(offLine))
			}
		})
	}
}

// Falcon Assault needs the falcon, and hits once.
func TestFalconAssault(t *testing.T) {
	z, p := hunterZone(t, reference.PreRenewal, 4012, mobLunatic)
	m := monsterOf(z, mobLunatic)
	m.x, m.y = p.entity.x+4, p.entity.y
	cast(z, p, skill.IDFalconAssault, 5, m)
	if f := skillFailures(run(z, p, z.now+50, z.now+500)); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Fatalf("without a falcon: failures %v", f)
	}
	z.setFalcon(p, true)
	cast(z, p, skill.IDFalconAssault, 5, m)
	ev := run(z, p, z.now+50, z.now+4000)
	if hits := skillDamage(ev, skill.IDFalconAssault); len(hits) != 1 {
		t.Fatalf("Falcon Assault hits %d, failures %v", len(hits), skillFailures(ev))
	}
}

// Wind Walker (SkillWindWalker) reaches the caster alone without a
// party.
func TestWindWalkCasterAlone(t *testing.T) {
	z, p := hunterZone(t, reference.Renewal, 4012)
	q := add(z, "friend")
	q.entity.x, q.entity.y = p.entity.x+1, p.entity.y
	z.recalc(q)
	cast(z, p, skill.IDWindWalk, 10, nil)
	ev := run(z, p, z.now+50, z.now+4000)
	if !p.entity.sc.Has(sc.SCWindwalk) {
		t.Fatalf("no Wind Walker on the caster: failures %v", skillFailures(ev))
	}
	if q.entity.sc.Has(sc.SCWindwalk) {
		t.Error("Wind Walker reached another player")
	}
}
