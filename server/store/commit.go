package store

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Commits (decision 0017): a transfer changes characters in memory, and
// the zone hands copies of them with the transfer's log lines to Commit.
//   - One worker writes commits one at a time, in the order they came,
//     each in one transaction. The transaction locks the characters' rows
//     in ID order, saves each character with its save version, and
//     writes the log lines.
//   - The order matters for items that pass between players through the
//     ground: the drop's commit is written before the pick-up's, so the
//     database never holds the item in both inventories.
//   - A commit that has not started takes in the commits that come after
//     it, so commits batch when the database is slow.
//   - A character whose save version moved, because a newer session
//     loaded it, is left out of the transaction, and its session ends as
//     with a stale save. The rest of the commit is written.
//   - A failed transaction is tried again after RetryDelay. Commits after
//     it wait.

// Entry is one character that a commit saves.
type Entry struct {
	Session *Session
	Char    *character.Character
}

type commit struct {
	entries []Entry
	logs    transfer.Logs
	started bool
	// retryAt is when a commit that failed may run again.
	retryAt time.Time
}

// Commit queues the saves and log lines of transfers. It never blocks.
// The store owns the characters afterwards. Each entry's copy must be
// newer than the session's saves that came before.
func (s *Store) Commit(entries []Entry, logs transfer.Logs) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c *commit
	if n := len(s.lane); n > 0 && !s.lane[n-1].started {
		c = s.lane[n-1]
	}
	if c == nil {
		c = &commit{}
	}
	added := false
	for _, e := range entries {
		x := e.Session
		if x == nil || x.ended {
			s.log.Warn("transfer saved without a character whose session ended", "character", e.Char.ID)
			continue
		}
		// The commit's copy is newer than a save that waits.
		if !x.final {
			x.pending = nil
		}
		if i := slices.IndexFunc(c.entries, func(o Entry) bool { return o.Session == x }); i >= 0 {
			c.entries[i].Char = e.Char
			continue
		}
		c.entries = append(c.entries, e)
		x.lane++
		s.busy[x.id]++
		added = true
	}
	c.logs.Add(logs)
	if len(c.entries) == 0 && c.logs.Empty() {
		return
	}
	if len(s.lane) == 0 || s.lane[len(s.lane)-1] != c {
		s.lane = append(s.lane, c)
		added = true
	}
	if added || !logs.Empty() {
		select {
		case s.laneWake <- struct{}{}:
		default:
		}
	}
}

// laneHead is the commit to write now, or nil: the lane is empty, its
// first commit waits to try again, or a save of one of its sessions is
// being written. s.mu must be held.
func (s *Store) laneHead() *commit {
	if len(s.lane) == 0 {
		return nil
	}
	c := s.lane[0]
	if time.Now().Before(c.retryAt) {
		return nil
	}
	for _, e := range c.entries {
		if e.Session.running {
			return nil
		}
	}
	return c
}

// commitWork writes commits in order until the store stops.
func (s *Store) commitWork() {
	for {
		s.mu.Lock()
		c := s.laneHead()
		for c == nil {
			ch := s.changed
			s.mu.Unlock()
			select {
			case <-s.laneWake:
			case <-ch:
			case <-s.stop:
				return
			}
			s.mu.Lock()
			c = s.laneHead()
		}
		c.started = true
		versions := make([]int64, len(c.entries))
		bases := make([]*character.Character, len(c.entries))
		skip := make([]bool, len(c.entries))
		for i, e := range c.entries {
			x := e.Session
			versions[i], bases[i], skip[i] = x.version, x.saved, x.ended
		}
		s.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		stale, err := s.writeCommit(ctx, c, versions, bases, skip)
		cancel()
		s.mu.Lock()
		s.finishCommit(c, stale, skip, err)
		s.mu.Unlock()
	}
}

// writeCommit writes a commit in one transaction. It returns which
// entries were stale.
func (s *Store) writeCommit(ctx context.Context, c *commit, versions []int64, bases []*character.Character, skip []bool) ([]bool, error) {
	stale := make([]bool, len(c.entries))
	order := make([]int, 0, len(c.entries))
	var ids []int64
	for i, e := range c.entries {
		if !skip[i] {
			order = append(order, i)
			ids = append(ids, e.Session.id)
		}
	}
	slices.SortFunc(order, func(a, b int) int { return cmp.Compare(c.entries[a].Session.id, c.entries[b].Session.id) })
	slices.Sort(ids)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		clear(stale)
		q := s.q.WithTx(tx)
		if len(ids) > 0 {
			if _, err := q.LockCharacters(ctx, ids); err != nil {
				return err
			}
		}
		for _, i := range order {
			e := c.entries[i]
			err := writeCharacter(ctx, q, e.Session.id, versions[i], bases[i], e.Char)
			if errors.Is(err, ErrStale) {
				stale[i] = true
				continue
			}
			if err != nil {
				return err
			}
		}
		return writeLogs(ctx, q, c.logs)
	})
	return stale, err
}

// finishCommit records a commit's result. s.mu must be held.
func (s *Store) finishCommit(c *commit, stale, skip []bool, err error) {
	if err != nil {
		s.log.Error("transfer commit failed; trying again", "characters", len(c.entries), "items", len(c.logs.Items), "err", err)
		c.started = false
		c.retryAt = time.Now().Add(s.retry)
		time.AfterFunc(s.retry, func() {
			select {
			case s.laneWake <- struct{}{}:
			default:
			}
		})
		return
	}
	for i, e := range c.entries {
		x := e.Session
		switch {
		case skip[i] || stale[i]:
			// The transfer is saved without this character. A newer
			// session holds what the database had, so the log lines are
			// what shows a copy that may be duplicated.
			s.log.Error("transfer saved without a character that a newer session loaded", "character", x.id,
				"items", len(c.logs.Items))
			if stale[i] {
				s.goStale(x)
			}
		default:
			x.saved = e.Char
			x.version++
			x.err = nil
		}
		x.lane--
		if x.lane == 0 && x.parked {
			x.parked = false
			s.push(x)
		}
		if s.busy[x.id]--; s.busy[x.id] <= 0 {
			delete(s.busy, x.id)
		}
	}
	s.lane[0] = nil
	s.lane = s.lane[1:]
	s.broadcast()
}

// writeLogs writes the log lines of a commit.
func writeLogs(ctx context.Context, q *db.Queries, logs transfer.Logs) error {
	if len(logs.Items) > 0 {
		p := db.InsertItemLogsParams{}
		for _, l := range logs.Items {
			p.Ats = append(p.Ats, pgtype.Timestamptz{Time: l.At, Valid: true})
			p.Actions = append(p.Actions, string(l.Action))
			p.FromKinds = append(p.FromKinds, l.From.Kind.String())
			p.FromIds = append(p.FromIds, l.From.ID)
			p.ToKinds = append(p.ToKinds, l.To.Kind.String())
			p.ToIds = append(p.ToIds, l.To.ID)
			p.Serials = append(p.Serials, pgtype.UUID{Bytes: l.Serial, Valid: true})
			p.Sources = append(p.Sources, pgtype.UUID{Bytes: l.Source, Valid: !l.Source.IsZero()})
			p.ItemIds = append(p.ItemIds, int(l.ItemID))
			p.Amounts = append(p.Amounts, l.Amount)
			p.Refines = append(p.Refines, max(l.Refine, 0))
			p.Cards = append(p.Cards, cardsLiteral(l.Cards))
			p.Rolls = append(p.Rolls, rollsJSON(l.Rolls))
			cards := make([]item.Rolls, len(l.CardRolls))
			copy(cards, l.CardRolls)
			p.CardRolls = append(p.CardRolls, mustJSON(cards))
			opts := l.Options
			if opts == nil {
				opts = []item.Option{}
			}
			p.RandomOptions = append(p.RandomOptions, mustJSON(opts))
			p.Identified = append(p.Identified, !l.Unidentified)
		}
		if err := q.InsertItemLogs(ctx, p); err != nil {
			return err
		}
	}
	if len(logs.Zeny) > 0 {
		p := db.InsertZenyLogsParams{}
		for _, l := range logs.Zeny {
			p.Ats = append(p.Ats, pgtype.Timestamptz{Time: l.At, Valid: true})
			p.Actions = append(p.Actions, string(l.Action))
			p.FromKinds = append(p.FromKinds, l.From.Kind.String())
			p.FromIds = append(p.FromIds, l.From.ID)
			p.ToKinds = append(p.ToKinds, l.To.Kind.String())
			p.ToIds = append(p.ToIds, l.To.ID)
			p.Amounts = append(p.Amounts, l.Amount)
		}
		return q.InsertZenyLogs(ctx, p)
	}
	return nil
}

// partitionInterval is how often the store makes the log partitions of
// the coming month.
const partitionInterval = 12 * time.Hour

// partitionWork makes the log partitions of this month and the next at
// start, then every partitionInterval, until the store stops.
func (s *Store) partitionWork() {
	t := time.NewTicker(partitionInterval)
	defer t.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		if err := s.q.EnsureLogPartitions(ctx, pgtype.Timestamptz{Time: time.Now(), Valid: true}); err != nil {
			s.log.Warn("cannot make the log partitions", "err", err)
		}
		cancel()
		select {
		case <-t.C:
		case <-s.stop:
			return
		}
	}
}
