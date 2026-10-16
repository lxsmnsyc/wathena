package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The quest log (decision 0043) is saved with the character, in
// character_quests (migration 00033): a row per quest, written when it
// is new, changed or moved, and deleted when the quest left the log.

// loadQuests reads a character's quest log.
func loadQuests(ctx context.Context, q *db.Queries, c *character.Character) error {
	rows, err := q.CharacterQuests(ctx, c.ID)
	if err != nil {
		return err
	}
	c.Quests = nil
	for _, r := range rows {
		e := character.Quest{ID: uint32(r.QuestID), State: r.State, Counts: [3]int{r.Count1, r.Count2, r.Count3}} // #nosec G115 -- the column's check keeps it to uint32.
		if r.EndsAt.Valid {
			e.End = r.EndsAt.Time
		}
		c.Quests = append(c.Quests, e)
	}
	return nil
}

// writeQuests writes the quests of c's log that changed from base's,
// and deletes those that left it.
func writeQuests(ctx context.Context, q *db.Queries, id int64, base, c *character.Character) error {
	before := make(map[uint32]int, len(base.Quests))
	for i, e := range base.Quests {
		before[e.ID] = i
	}
	now := make(map[uint32]bool, len(c.Quests))
	p := db.UpsertCharacterQuestsParams{CharacterID: id}
	for i, e := range c.Quests {
		now[e.ID] = true
		if j, ok := before[e.ID]; ok && j == i && base.Quests[j].Equal(&e) {
			continue
		}
		p.QuestIds = append(p.QuestIds, int64(e.ID))
		p.Ords = append(p.Ords, i)
		p.States = append(p.States, e.State)
		p.Ends = append(p.Ends, questEnd(e.End))
		p.Count1s, p.Count2s, p.Count3s = append(p.Count1s, e.Counts[0]), append(p.Count2s, e.Counts[1]), append(p.Count3s, e.Counts[2])
	}
	var gone []int64
	for _, e := range base.Quests {
		if !now[e.ID] {
			gone = append(gone, int64(e.ID))
		}
	}
	if len(gone) > 0 {
		if err := q.DeleteCharacterQuests(ctx, db.DeleteCharacterQuestsParams{CharacterID: id, QuestIds: gone}); err != nil {
			return err
		}
	}
	if len(p.QuestIds) == 0 {
		return nil
	}
	return q.UpsertCharacterQuests(ctx, p)
}

// questEnd is a quest's end for the database: null without a limit.
func questEnd(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}
