package modkit

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// TestClassicCasting runs the Classic Casting sample ruleset mod's tests,
// then checks in Go what wa.testing cannot read, against a world of
// core.ruleset.renewal (the greeter sample mod):
//   - The world gets the mod's rules: renewal with pre-renewal's cast
//     time, element table, cooldowns and Mystical Amplification.
//   - A Mage with 150 DEX kills a Poring with Fire Bolt at once, where
//     renewal's cast time makes it wait.
//   - Fire Bolt does nothing to a Strouf (water 3), and renewal's table
//     lets it burn.
//   - Angelus has pre-renewal's cooldown, none.
func TestClassicCasting(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	e := env(t)
	ctx := context.Background()
	l, err := e.Load(ctx, "../../mods/examples/classic-casting", "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.RunTests(ctx, l, "", func(CaseResult) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}
	want := rules.Base(true)
	want.CastTime, want.ElementTable, want.SkillCooldown = false, false, false
	magicPower := rules.RebalancedSkills["HW_MAGICPOWER"]
	want.Skills = map[int]bool{magicPower: false}
	rl := l.Rules
	if rl == nil || l.Zone.Rules != rl || rl.CastTime || rl.ElementTable || rl.SkillCooldown || rl.Skill(magicPower) ||
		!rl.Renewal || !rl.ASPD || !rl.StatusResist || !rl.LevelPenaltyExp || len(rl.Skills) != 1 {
		t.Fatalf("rules %+v, want %+v", rl, want)
	}
	if got := l.JobTables.Rules(); got.CastTime {
		t.Error("the job tables read renewal's cast bonuses")
	}
	if d := l.Skills.ByName("AL_ANGELUS"); d.Cooldown.At(1) != 0 {
		t.Errorf("Angelus cooldown %d, want pre-renewal's 0", d.Cooldown.At(1))
	}

	core, err := e.Load(ctx, "../../mods/examples/greeter", "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d := core.Skills.ByName("AL_ANGELUS"); d.Cooldown.At(1) != 30000 {
		t.Errorf("renewal's Angelus cooldown %d, want 30000", d.Cooldown.At(1))
	}
	type hp struct{ right, later int64 }
	bolt := func(l *Loaded, target string) hp {
		t.Helper()
		w, err := e.newWorld(l, slog.New(&capture{}))
		if err != nil {
			t.Fatal(err)
		}
		defer w.h.Close()
		vals, err := w.Do("new_player", []script.Value{script.NewTable(script.KV("map", script.String("prontera")),
			script.KV("x", script.Int(150)), script.KV("y", script.Int(150)), script.KV("job", script.Int(2)),
			script.KV("stats", script.NewTable(script.KV("dex", script.Int(150)), script.KV("int", script.Int(99)))),
			script.KV("base_level", script.Int(70)), script.KV("job_level", script.Int(40)),
			script.KV("skills", script.List(script.List(script.Int(1), script.Int(9)), script.List(script.Int(19), script.Int(10)))))})
		if err != nil {
			t.Fatal(err)
		}
		mage, _ := vals[0].AsInt()
		m, err := w.Do("spawn_monster", []script.Value{script.NewTable(script.KV("monster", script.Int(l.Zone.MonsterIDs[target])),
			script.KV("x", script.Int(156)), script.KV("y", script.Int(150)))})
		if err != nil {
			t.Fatal(err)
		}
		mob, _ := m[0].AsInt()
		// Level 1 casts before an aggressive target can walk up and
		// interrupt it.
		if err := w.h.UseSkill(mage, 19, 1, mob); err != nil {
			t.Fatal(err)
		}
		var out hp
		out.right, _ = w.h.MonsterHP(mob)
		w.h.Advance(2000)
		out.later, _ = w.h.MonsterHP(mob)
		return out
	}
	if got := bolt(l, "PORING"); got.right != 0 {
		t.Errorf("classic casting: the Poring has %d HP right after the cast, want 0", got.right)
	}
	if got := bolt(core, "PORING"); got.right == 0 || got.later != 0 {
		t.Errorf("renewal: the Poring has %d HP right after the cast and %d later", got.right, got.later)
	}
	strouf := bolt(l, "STROUF")
	burnt := bolt(core, "STROUF")
	if strouf.later != strouf.right || burnt.later >= burnt.right {
		t.Errorf("Strouf HP: %+v with pre-renewal's table, %+v with renewal's", strouf, burnt)
	}
}
