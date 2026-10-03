package modkit

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// TestDesperado runs the Desperado sample mod's tests in both modes,
// then checks in Go what wa.testing cannot read: the new job's skill
// list, with the new skill once Single Action is at level 3, and a cast
// of the new skill that shoots twice.
func TestDesperado(t *testing.T) {
	e := env(t)
	l, err := e.Load(context.Background(), "../../mods/examples/desperado", "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.RunTests(context.Background(), l, "", func(CaseResult) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}

	desperado, ok := l.Jobs.ID("example.desperado:Desperado")
	if !ok {
		t.Fatal("no Desperado")
	}
	quickDraw := l.Skills.ByName("example.desperado:QUICK_DRAW").ID
	singleAction := l.Skills.ByName("GS_SINGLEACTION").ID
	logs := &capture{}
	w, err := e.newWorld(l, slog.New(logs))
	if err != nil {
		t.Fatal(err)
	}
	defer w.h.Close()
	vals, err := w.Do("new_player", []script.Value{script.NewTable(script.KV("map", script.String("prontera")),
		script.KV("x", script.Int(150)), script.KV("y", script.Int(150)), script.KV("job", script.Int(int64(desperado))),
		script.KV("base_level", script.Int(70)), script.KV("job_level", script.Int(20)),
		script.KV("skills", script.List(script.List(script.Int(1), script.Int(9)), script.List(script.Int(int64(singleAction)), script.Int(2)))))})
	if err != nil {
		t.Fatal(err)
	}
	player, _ := vals[0].AsInt()
	list, err := w.h.SkillList(player)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := list[quickDraw]; ok {
		t.Error("Quick Draw shows before Single Action 3")
	}
	if !list[singleAction] || !list[l.Skills.ByName("GS_GLITTERING").ID] {
		t.Errorf("the Desperado lacks the Gunslinger's tree: %v", list)
	}
	vals, err = w.Do("new_player", []script.Value{script.NewTable(script.KV("map", script.String("prontera")),
		script.KV("x", script.Int(150)), script.KV("y", script.Int(152)), script.KV("job", script.Int(int64(desperado))),
		script.KV("stats", script.NewTable(script.KV("dex", script.Int(99)))),
		script.KV("base_level", script.Int(70)), script.KV("job_level", script.Int(20)),
		script.KV("skills", script.List(script.List(script.Int(1), script.Int(9)), script.List(script.Int(int64(singleAction)), script.Int(3)),
			script.List(script.Int(int64(quickDraw)), script.Int(5)))))})
	if err != nil {
		t.Fatal(err)
	}
	shooter, _ := vals[0].AsInt()
	if list, _ = w.h.SkillList(shooter); !hasKey(list, quickDraw) {
		t.Errorf("Quick Draw is not in the list: %v", list)
	}
	if _, err := w.h.Equip(shooter, l.Items.IDs["Six_Shooter"], modscript.GiveRoll{Mode: "default"}); err != nil {
		t.Fatal(err)
	}
	m, err := w.Do("spawn_monster", []script.Value{script.NewTable(script.KV("monster", script.Int(l.Zone.MonsterIDs["GOLEM"])),
		script.KV("x", script.Int(155)), script.KV("y", script.Int(152)))})
	if err != nil {
		t.Fatal(err)
	}
	golem, _ := m[0].AsInt()
	full, err := w.h.MonsterHP(golem)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.h.UseSkill(shooter, quickDraw, 5, golem); err != nil {
		t.Fatal(err)
	}
	// Each shot lowers the Golem's HP once.
	hp := []int64{full}
	for range 40 {
		w.h.Advance(50)
		if now, _ := w.h.MonsterHP(golem); now != hp[len(hp)-1] {
			hp = append(hp, now)
		}
	}
	if len(hp) != 3 {
		t.Errorf("golem HP %v: want two shots", hp)
	}
	if lines := logs.about("desperado"); len(lines) > 0 {
		t.Errorf("logs: %v", lines)
	}
}

func hasKey(m map[int]bool, k int) bool {
	_, ok := m[k]
	return ok
}
