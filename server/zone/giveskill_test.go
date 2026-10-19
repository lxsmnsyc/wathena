package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/social"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// hasSkillList reports whether the player's pending events hold a skill
// list, and empties them.
func hasSkillList(p *Player) bool {
	found := false
	for _, e := range p.pending {
		if e.GetSkillList() != nil {
			found = true
		}
	}
	p.pending = nil
	return found
}

// TestGiveSkillPermanent checks player:give_skill's permanent kind, as
// pc_skill's ADDSKILL_PERMANENT: the level is learned outside the tree
// and the skill points, saved, and sent in the skill list. Level 0
// removes the skill.
func TestGiveSkillPermanent(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 1, 20, 10)
	p.char.Skills = map[int]int{skill.IDBasic: 9, skill.IDTrickDead: 1}
	p.pending = nil
	if !z.giveSkill(p, skill.IDMovingRecovery, 1, modscript.SkillPermanent) {
		t.Fatal("Moving HP Recovery refused")
	}
	if p.char.Skills[skill.IDMovingRecovery] != 1 || p.skillLevel(skill.IDMovingRecovery) != 1 {
		t.Errorf("Moving HP Recovery at %d", p.char.Skills[skill.IDMovingRecovery])
	}
	if p.progress.SkillPoints != 0 {
		t.Errorf("skill points %d, want 0", p.progress.SkillPoints)
	}
	if !hasSkillList(p) {
		t.Error("no skill list after a permanent skill")
	}
	found := false
	for _, s := range z.skillList(p).GetSkillList().GetSkills() {
		if s.GetSkillId() == skill.IDMovingRecovery && s.GetLevel() == 1 {
			found = true
		}
	}
	if !found {
		t.Error("the skill list does not have Moving HP Recovery 1")
	}
	if got := learnedSkills(p)[skill.IDMovingRecovery]; got != 1 {
		t.Errorf("saved level %d, want 1", got)
	}
	// A skill outside the job's tree is learned too, as in rAthena.
	if !z.giveSkill(p, skill.IDFireBolt, 2, modscript.SkillPermanent) || p.skillLevel(skill.IDFireBolt) != 2 {
		t.Error("Fire Bolt outside the tree was not learned")
	}
	// Level 0 removes it.
	if !z.giveSkill(p, skill.IDTrickDead, 0, modscript.SkillPermanent) {
		t.Fatal("removing Trick Dead refused")
	}
	if _, ok := p.char.Skills[skill.IDTrickDead]; ok || !hasSkillList(p) {
		t.Errorf("Trick Dead still learned: %v", p.char.Skills)
	}
	// A recalculation keeps permanent levels.
	z.statsChanged(p)
	if p.skillLevel(skill.IDMovingRecovery) != 1 || p.skillLevel(skill.IDFireBolt) != 2 {
		t.Error("a recalculation removed a permanent skill")
	}
	// Unknown and guild skills are refused.
	if z.giveSkill(p, 99999, 1, modscript.SkillPermanent) || z.giveSkill(p, social.SkillApproval, 1, modscript.SkillPermanent) {
		t.Error("an unknown or guild skill was given")
	}
}

// TestGiveSkillTemporary checks the temporary kinds, as ADDSKILL_TEMP and
// ADDSKILL_TEMP_ADDLEVEL: the level holds until the skills are
// recalculated, the client gets nothing, and nothing is saved.
func TestGiveSkillTemporary(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 1, 20, 10)
	p.char.Skills = map[int]int{skill.IDBasic: 9, skill.IDBash: 5}
	p.pending = nil
	// temporary: the higher of the level and the given one.
	z.giveSkill(p, skill.IDBash, 3, modscript.SkillTemporary)
	if p.skillLevel(skill.IDBash) != 5 {
		t.Errorf("Bash at %d after a lower temporary level, want 5", p.skillLevel(skill.IDBash))
	}
	z.giveSkill(p, skill.IDBash, 8, modscript.SkillTemporary)
	z.giveSkill(p, skill.IDFireBolt, 3, modscript.SkillTemporary)
	if p.skillLevel(skill.IDBash) != 8 || p.skillLevel(skill.IDFireBolt) != 3 {
		t.Errorf("Bash %d, Fire Bolt %d, want 8 and 3", p.skillLevel(skill.IDBash), p.skillLevel(skill.IDFireBolt))
	}
	if p.char.Skills[skill.IDBash] != 5 || p.char.Skills[skill.IDFireBolt] != 0 || learnedSkills(p)[skill.IDFireBolt] != 0 {
		t.Errorf("a temporary level was learned: %v", p.char.Skills)
	}
	if hasSkillList(p) {
		t.Error("a temporary skill sent the skill list")
	}
	// temporary_add: on top of the level, temporary ones too, up to 13.
	z.giveSkill(p, skill.IDBash, 2, modscript.SkillTemporaryAdd)
	if p.skillLevel(skill.IDBash) != 10 {
		t.Errorf("Bash at %d after adding 2 to 8, want 10", p.skillLevel(skill.IDBash))
	}
	z.giveSkill(p, skill.IDBash, 10, modscript.SkillTemporaryAdd)
	if p.skillLevel(skill.IDBash) != modscript.MaxGivenSkillLevel {
		t.Errorf("Bash at %d, want the cap %d", p.skillLevel(skill.IDBash), modscript.MaxGivenSkillLevel)
	}
	// A status's own recalculation keeps them: status_calc_bl without
	// SCB_BASE does not recalculate skills.
	if d := z.Statuses.Get(sc.SCBlessing); d == nil || d.Calcs("base") {
		t.Fatal("Blessing recalculates the base")
	}
	z.scStart(p.entity, p.entity, sc.SCBlessing, 100, 10, 60000)
	if p.skillLevel(skill.IDBash) != modscript.MaxGivenSkillLevel || p.skillLevel(skill.IDFireBolt) != 3 {
		t.Errorf("a status's recalculation removed temporary levels: Bash %d, Fire Bolt %d", p.skillLevel(skill.IDBash), p.skillLevel(skill.IDFireBolt))
	}
	// A full recalculation, as status_calc_pc, removes them.
	z.statsChanged(p)
	if p.skillLevel(skill.IDBash) != 5 || p.skillLevel(skill.IDFireBolt) != 0 {
		t.Errorf("after a recalculation: Bash %d, Fire Bolt %d, want 5 and 0", p.skillLevel(skill.IDBash), p.skillLevel(skill.IDFireBolt))
	}
	// A permanent level replaces a temporary one.
	z.giveSkill(p, skill.IDFireBolt, 5, modscript.SkillTemporary)
	z.giveSkill(p, skill.IDFireBolt, 1, modscript.SkillPermanent)
	if p.skillLevel(skill.IDFireBolt) != 1 {
		t.Errorf("Fire Bolt at %d, want the permanent 1", p.skillLevel(skill.IDFireBolt))
	}
}

// TestGiveSkillScript runs player:give_skill from a mod through the zone.
func TestGiveSkillScript(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 1, 20, 10)
	if ok, err := (scriptHost{z}).GiveSkill(int64(p.charID), skill.IDAutoBerserk, 1, modscript.SkillPermanent); err != nil || !ok {
		t.Fatalf("GiveSkill: %v, %v", ok, err)
	}
	if p.char.Skills[skill.IDAutoBerserk] != 1 {
		t.Errorf("Auto Berserk at %d", p.char.Skills[skill.IDAutoBerserk])
	}
	if _, err := (scriptHost{z}).GiveSkill(999999, skill.IDAutoBerserk, 1, modscript.SkillPermanent); err == nil {
		t.Error("a missing player gave no error")
	}
}
