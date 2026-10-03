package convert

import (
	"os"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// TestMonsterReportsCurrent checks the monster skill coverage reports in
// docs/generated.
func TestMonsterReportsCurrent(t *testing.T) {
	for _, mode := range reference.Modes {
		path := "../../docs/generated/monster-skills-" + string(mode) + ".md"
		got := loadCore(t, mode).MonsterSkills.MonsterReport()
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != got {
			t.Errorf("%s is not current: run go run ./tools/wactl convert monsters --mode %s --report %s", path, mode, strings.TrimPrefix(path, "../../"))
		}
	}
}

// TestMonsterSkillsConvert checks that every mob_skill_db row converts,
// and a few rows by hand.
func TestMonsterSkillsConvert(t *testing.T) {
	core := loadCore(t, reference.Renewal)
	r := core.MonsterSkills
	if r.Entries != r.Full+r.Partial || r.Entries < 14000 {
		t.Fatalf("rows %d, full %d, partial %d", r.Entries, r.Full, r.Partial)
	}
	yml := r.Files()["data/monsters.yml"]
	for _, want := range []string{
		// Baphomet: a chase skill with a cast time, and a teleport when
		// it cannot reach its target.
		"- name: BAPHOMET\n  skills:\n    - {skill: MO_BODYRELOCATION, state: chase, rate: 2000, cast_time: 200, delay: 1000, cancelable: true}",
		"{skill: AL_TELEPORT, state: idle, cancelable: true, target: self, condition: hit_while_stuck}",
		// Poring's emotion while it loots.
		"{skill: NPC_EMOTION, state: loot, rate: 2000, delay: 5000, cancelable: true, target: self, emotion: ",
	} {
		if !strings.Contains(yml, want) {
			t.Errorf("data/monsters.yml lacks %q", want)
		}
	}
}
