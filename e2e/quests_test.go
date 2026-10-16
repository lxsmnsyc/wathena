package e2e

import (
	"context"
	"slices"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// questOf finds a quest in a QuestList.
func questOf(l *wathenav1.QuestList, id uint32) *wathenav1.QuestInfo {
	for _, q := range l.GetQuests() {
		if q.GetId() == id {
			return q
		}
	}
	return nil
}

// since waits for an event from an earlier mark on, such as one that
// came before the conversation step a step waited for. It does not move
// the mark.
func (p *Player) since(mark int, what string, match func(*wathenav1.Event) bool) *wathenav1.Event {
	p.t.Helper()
	ctx, cancel := p.ctx()
	defer cancel()
	e, _, err := p.Wait(ctx, mark, match)
	if err != nil {
		p.t.Fatalf("%s: waiting for %s: %v", p.Name, what, err)
	}
	return e
}

// questUpdate waits for a QuestUpdate that passes ok, from a mark on.
func (p *Player) questUpdate(mark int, what string, ok func(*wathenav1.QuestInfo) bool) *wathenav1.QuestInfo {
	p.t.Helper()
	return p.since(mark, what, func(e *wathenav1.Event) bool {
		return e.HasQuestUpdate() && ok(e.GetQuestUpdate().GetQuest())
	}).GetQuestUpdate().GetQuest()
}

// questRemoved waits for the QuestRemove of a quest, from a mark on.
func (p *Player) questRemoved(mark int, id uint32) {
	p.t.Helper()
	p.since(mark, "a quest to leave the log", func(e *wathenav1.Event) bool {
		return e.HasQuestRemove() && e.GetQuestRemove().GetId() == id
	})
}

// TestQuestFromCoreNPC: a player takes a quest from a converted core NPC,
// hunts it across a relog, and turns it in (decision 0043). Core's
// Dancer Kim, of rAthena's Wanderer job change, stands on the start map,
// and her quest 2221 counts 2 Dummies, both by the fixture's overlays.
// The character starts at her step: job_wan 6 and quest 2220. Needs
// PostgreSQL.
func TestQuestFromCoreNPC(t *testing.T) {
	srv := Start(t, Options{Database: true})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p := srv.Join(t, "Wanderer")
	p.Expect("the quest list", func(e *wathenav1.Event) bool { return e.HasQuestList() })
	_ = p.Close()
	var id int64
	// The character is saved when its session ends.
	for range 100 {
		if err := srv.DB.QueryRow(ctx, `SELECT id FROM characters WHERE name = 'Wanderer'`).Scan(&id); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if id == 0 {
		t.Fatal("the character was not saved")
	}
	// rAthena's Wanderer quest line up to Dancer Kim.
	if _, err := srv.DB.Exec(ctx, `INSERT INTO character_quests (character_id, quest_id, ord, state) VALUES ($1, 2220, 0, 1)`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DB.Exec(ctx, `INSERT INTO mod_character_variables (character_id, mod_id, name, value) VALUES ($1, 'core', 'job_wan', '6')`, id); err != nil {
		t.Fatal(err)
	}

	p = srv.Join(t, "Wanderer")
	list := p.Expect("the quest list", func(e *wathenav1.Event) bool { return e.HasQuestList() }).GetQuestList()
	if q := questOf(list, 2220); q == nil || q.GetTitle() != "Wanderer Job Change Quest" || !q.GetActive() || q.GetLimitMs() != -1 {
		t.Fatalf("the quest list at login: %v", list)
	}
	talk := p.Talk("Dancer Kim")
	talk.Says("Exercising together")
	talk.Next()
	talk.Next()
	talk.Says("You need to defeat 50 Yellow Novus!")
	talk.Next()
	mark := p.Mark()
	talk.Choose("Yes.")
	talk.Says("Go!")
	p.questRemoved(mark, 2220)
	q := p.questUpdate(mark, "quest 2221", func(q *wathenav1.QuestInfo) bool { return q.GetId() == 2221 })
	if o := q.GetObjectives(); len(o) != 1 || o[0].GetLabel() != "Dummy" || o[0].GetGoal() != 2 || o[0].GetCount() != 0 ||
		q.GetLimitMs() <= 0 || q.GetLimitMs() > 600_000 {
		t.Fatalf("quest 2221 %v", q)
	}
	talk.Close()

	// The first Dummy, then a relog.
	mark = p.Mark()
	p.Kill(p.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy"))
	p.questUpdate(mark, "a kill counted", func(q *wathenav1.QuestInfo) bool {
		return q.GetId() == 2221 && q.GetObjectives()[0].GetCount() == 1
	})
	_ = p.Close()
	var count, state int
	var ends *time.Time
	for range 100 {
		err := srv.DB.QueryRow(ctx, `SELECT count1, state, ends_at FROM character_quests WHERE character_id = $1 AND quest_id = 2221`, id).Scan(&count, &state, &ends)
		if err == nil && count == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if count != 1 || state != 1 || ends == nil || time.Until(*ends) > 10*time.Minute {
		t.Fatalf("saved quest 2221: count %d, state %d, ends %v", count, state, ends)
	}

	p = srv.Join(t, "Wanderer")
	list = p.Expect("the quest list", func(e *wathenav1.Event) bool { return e.HasQuestList() }).GetQuestList()
	if q := questOf(list, 2221); q == nil || q.GetObjectives()[0].GetCount() != 1 || questOf(list, 2220) != nil {
		t.Fatalf("the quest list after the relog: %v", list)
	}
	mark = p.Mark()
	p.Kill(p.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy"))
	p.questUpdate(mark, "the second kill", func(q *wathenav1.QuestInfo) bool {
		return q.GetId() == 2221 && q.GetObjectives()[0].GetCount() == 2
	})
	talk = p.Talk("Dancer Kim")
	talk.Says("Looks like I've lost!")
	for talk.Step.HasNext() {
		talk.Next()
	}
	talk.Choose("How about dancing in Lutie?")
	for talk.Step.HasNext() {
		talk.Next()
	}
	mark = p.Mark()
	talk.Choose("No problem!")
	talk.Says("You're a good person")
	p.questRemoved(mark, 2221)
	p.questUpdate(mark, "quest 2222", func(q *wathenav1.QuestInfo) bool { return q.GetId() == 2222 })
	talk.Close()
	_ = p.Close()

	// After the last relog, the log holds 2222 alone, and job_wan is 8.
	var quests []int64
	var jobWan string
	for range 100 {
		quests = quests[:0]
		rows, err := srv.DB.Query(ctx, `SELECT quest_id FROM character_quests WHERE character_id = $1 ORDER BY ord`, id)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var q int64
			if err := rows.Scan(&q); err != nil {
				t.Fatal(err)
			}
			quests = append(quests, q)
		}
		rows.Close()
		_ = srv.DB.QueryRow(ctx, `SELECT value::text FROM mod_character_variables WHERE character_id = $1 AND mod_id = 'core' AND name = 'job_wan'`, id).Scan(&jobWan)
		if slices.Equal(quests, []int64{2222}) && jobWan == "8" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !slices.Equal(quests, []int64{2222}) || jobWan != "8" {
		t.Errorf("saved quests %v, job_wan %q", quests, jobWan)
	}
	p = srv.Join(t, "Wanderer")
	list = p.Expect("the quest list", func(e *wathenav1.Event) bool { return e.HasQuestList() }).GetQuestList()
	if len(list.GetQuests()) != 1 || questOf(list, 2222) == nil {
		t.Errorf("the quest list at the end: %v", list)
	}
	srv.CheckLogs()
}
