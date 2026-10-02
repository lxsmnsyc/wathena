package store

import (
	"context"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// loadStatuses reads a character's saved statuses and timed bonuses.
func loadStatuses(ctx context.Context, q *db.Queries, c *character.Character) error {
	rows, err := q.Statuses(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		c.Statuses = append(c.Statuses, character.Status{Name: r.Name, Values: [4]int{r.Val1, r.Val2, r.Val3, r.Val4}, Tick: r.Tick, End: r.EndMs})
	}
	timed, err := q.TimedBonuses(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, r := range timed {
		c.TimedBonuses = append(c.TimedBonuses, character.TimedBonus{Key: r.Key, Remaining: r.RemainingMs, Bonuses: string(r.Bonuses),
			EndsOn: r.EndsOn, Icon: r.Icon, Kind: r.Kind})
	}
	return nil
}

// sortedStatuses returns statuses by name, as Statuses reads them.
func sortedStatuses(list []character.Status) []character.Status {
	out := slices.Clone(list)
	slices.SortFunc(out, func(a, b character.Status) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// writeStatuses replaces a character's statuses and timed bonuses when
// they changed from old.
func writeStatuses(ctx context.Context, q *db.Queries, id int64, old, cur *character.Character) error {
	if !slices.Equal(sortedStatuses(old.Statuses), sortedStatuses(cur.Statuses)) {
		if err := q.DeleteStatuses(ctx, id); err != nil {
			return err
		}
		if n := len(cur.Statuses); n > 0 {
			p := db.InsertStatusesParams{CharacterID: id, Names: make([]string, n), Val1s: make([]int, n), Val2s: make([]int, n),
				Val3s: make([]int, n), Val4s: make([]int, n), Ticks: make([]int64, n), Ends: make([]int64, n)}
			for i, s := range cur.Statuses {
				p.Names[i], p.Ticks[i], p.Ends[i] = s.Name, s.Tick, s.End
				p.Val1s[i], p.Val2s[i], p.Val3s[i], p.Val4s[i] = s.Values[0], s.Values[1], s.Values[2], s.Values[3]
			}
			if err := q.InsertStatuses(ctx, p); err != nil {
				return err
			}
		}
	}
	if slices.EqualFunc(old.TimedBonuses, cur.TimedBonuses, func(a, b character.TimedBonus) bool { return a.Equal(&b) }) {
		return nil
	}
	if err := q.DeleteTimedBonuses(ctx, id); err != nil {
		return err
	}
	for i, t := range cur.TimedBonuses {
		ends := t.EndsOn
		if ends == nil {
			ends = []string{}
		}
		err := q.InsertTimedBonus(ctx, db.InsertTimedBonusParams{CharacterID: id, Position: i, Key: t.Key, RemainingMs: t.Remaining,
			Bonuses: []byte(t.Bonuses), EndsOn: ends, Icon: t.Icon, Kind: t.Kind})
		if err != nil {
			return err
		}
	}
	return nil
}
