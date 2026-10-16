package store_test

import (
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/store"
)

// TestCharacterQuests: the quest log is saved and loaded with the
// character, with its order, states, ends and counts, and a save writes
// what changed and deletes what left the log.
func TestCharacterQuests(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	id, err := s.DevCharacter(ctx, newCharacter("Hunter"))
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Quests) != 0 {
		t.Fatalf("a new character has quests %v", c.Quests)
	}
	end := time.Date(2026, 10, 17, 4, 0, 0, 0, time.UTC)
	next := c.Clone()
	next.Quests = []character.Quest{
		{ID: 1100, State: character.QuestActive, End: end, Counts: [3]int{5, 0, 0}},
		{ID: 1000, State: character.QuestComplete},
		{ID: 1_000_000, State: character.QuestInactive, Counts: [3]int{1, 2, 65535}},
	}
	sess.Save(next, true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(next) {
		t.Fatalf("loaded %+v, want %+v", got.Quests, next.Quests)
	}
	next = got.Clone()
	next.Quests = []character.Quest{
		{ID: 1_000_000, State: character.QuestActive, Counts: [3]int{2, 2, 65535}},
		{ID: 1100, State: character.QuestComplete, End: end, Counts: [3]int{20, 0, 0}},
		{ID: 7104, State: character.QuestActive, End: end.Add(time.Hour)},
	}
	sess.Save(next, true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err = s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(next) {
		t.Errorf("after the second save %+v, want %+v", got.Quests, next.Quests)
	}
	view, err := s.ReadCharacter(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Character.Quests) != 3 || view.Character.Quests[2].ID != 7104 {
		t.Errorf("the staff view's quests: %+v", view.Character.Quests)
	}
}
