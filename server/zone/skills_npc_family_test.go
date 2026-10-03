package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// TestNpcElementChanges checks SkillAttributeChange and the element
// changes of one element: SC_ELEMENTALCHANGE with the skill level as
// val1 and the skill's element as val2, which status_calc_element and
// status_calc_element_lv give the monster as its defense element and
// level. NPC_ATTRICHANGE has no element of its own, so the status picks
// one.
func TestNpcElementChanges(t *testing.T) {
	z, _ := aiZone(t)
	for _, c := range []struct {
		name    string
		id, ele int
	}{
		{"NPC_CHANGEWATER", idNpcChangeWater, 1},
		{"NPC_CHANGEGROUND", idNpcChangeGround, 2},
		{"NPC_CHANGEFIRE", idNpcChangeFire, 3},
		{"NPC_CHANGEWIND", idNpcChangeWind, 4},
		{"NPC_CHANGEPOISON", idNpcChangePoison, 5},
		{"NPC_CHANGEHOLY", idNpcChangeHoly, 6},
		{"NPC_CHANGEDARKNESS", idNpcChangeDarkness, 7},
		{"NPC_CHANGETELEKINESIS", idNpcChangeTelekinesis, 8},
		{"NPC_ATTRICHANGE", idNpcAttriChange, -1},
	} {
		m := aiMob(t, z, 1002, 22, 20, mob.AITypes[1], nil)
		const lv = 3
		npcSelf(z, m, c.id, lv)
		e := m.sc.Get(sc.SCElementalchange)
		if e == nil || e.Val1 != lv || c.ele >= 0 && e.Val2 != c.ele {
			t.Errorf("%s: SC_ELEMENTALCHANGE %+v", c.name, e)
			continue
		}
		if m.status.DefEle != e.Val2 || m.status.EleLv != lv {
			t.Errorf("%s: element %d level %d, want %d level %d", c.name, m.status.DefEle, m.status.EleLv, e.Val2, lv)
		}
	}
}

// TestNPCWideStatusFamily checks every wide status skill that shares
// SkillWideCurse's code (npcWideStatus without a delay), at level 1 in
// both modes: the player 2 cells away gets the skill's status at 100%,
// with the level as val1 and the monster's ID as val2 (SC_SLOWCAST's
// own val2 for NPC_SLOWCAST), and the monster does not. A skill that a mode's skill_db lacks is left out.
func TestNPCWideStatusFamily(t *testing.T) {
	ids := []int{idNPCSlowCast, idNPCWideSilence, idNPCWideSilence2, idNPCWideFreeze, idNPCWideFreeze2, idNPCWideCurse2,
		idNPCWideConfuse, idNPCWideConfuse2, idNPCWideBleeding, idNPCWideBleeding2, idNPCWideStun, idNPCWideSleep,
		idNPCWideHellDignity, idNPCDeadlyCurse2}
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			checked := 0
			for _, id := range ids {
				z, m, p := npcStatusZone(t, mode)
				d := z.Skills.Get(id)
				if d == nil || d.Target == skill.TargetPassive {
					continue
				}
				z.castendNoDamage(m, m, d, 1, 0)
				e := p.entity.sc.Get(d.Status)
				if e == nil {
					t.Errorf("%s: the player 2 cells away has no status %d", d.Name, d.Status)
					continue
				}
				val2 := int(z.unit(m).Ref().ID)
				if id == idNPCSlowCast {
					// status_change_start sets SC_SLOWCAST's val2 to 20% per
					// level, the cast time it adds.
					val2 = 20
				}
				if e.Val1 != 1 || e.Val2 != val2 {
					t.Errorf("%s: val1 %d, val2 %d, want 1 and %d", d.Name, e.Val1, e.Val2, val2)
				}
				if m.sc.Has(d.Status) {
					t.Errorf("%s: the monster has its own status", d.Name)
				}
				checked++
			}
			t.Logf("%s: %d wide status skills", mode, checked)
		})
	}
}
