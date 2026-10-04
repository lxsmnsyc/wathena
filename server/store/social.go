package store

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Parties and friend lists (decision 0020). The social registry holds
// them in memory and hands each change to SocialStore, which writes the
// changes on its own goroutine in the order they came. A character's
// friend list loads with the character. Every party loads at start.

// loadFriends reads a character's friend list.
func loadFriends(ctx context.Context, q *db.Queries, c *character.Character) error {
	rows, err := q.CharacterFriends(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		c.Friends = append(c.Friends, character.Friend{CharID: r.ID, Name: r.Name})
	}
	return nil
}

// LoadParties reads every saved party with its members.
func LoadParties(ctx context.Context, pool *pgxpool.Pool) ([]social.PartyRecord, error) {
	q := db.New(pool)
	rows, err := q.Parties(ctx)
	if err != nil {
		return nil, err
	}
	members, err := q.PartyMembers(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64][]db.PartyMembersRow{}
	for _, m := range members {
		byID[m.PartyID] = append(byID[m.PartyID], m)
	}
	out := make([]social.PartyRecord, 0, len(rows))
	for _, r := range rows {
		rec := social.PartyRecord{ID: r.ID, Name: r.Name, Options: social.Options{ExpEven: r.ExpEven, ItemEven: r.ItemEven, PickupShared: r.PickupShared}}
		for _, m := range byID[r.ID] {
			rec.Members = append(rec.Members, social.MemberRecord{Slot: m.Slot, CharID: m.ID, AccountID: m.AccountID, Name: m.Name,
				Level: m.BaseLevel, Job: m.Job, Map: m.Map})
			if m.ID == r.LeaderID {
				rec.Leader = m.Name
			}
		}
		out = append(out, rec)
	}
	return out, nil
}

// SocialStore writes the social registry's changes. It implements
// social.Store: its methods never block. Writes run one at a time in the
// order they came, each in its own transaction. Changes of characters
// that only live in memory, with ID 0, are skipped.
type SocialStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
	log  *slog.Logger

	// Lane, when set, runs guild writes in order with the transfer
	// commits (Store.Lane), so a commit that writes a guild's storage
	// comes after the guild's row and before its deletion.
	Lane func(func(ctx context.Context, q *db.Queries) error)

	mu sync.Mutex
	// seq numbers the exp writes. expSeq and memberSeq hold the last one
	// of each guild and member, which is the only one that writes.
	seq       uint64
	expSeq    map[int64]uint64
	memberSeq map[[2]int64]uint64
	queue     []func(ctx context.Context, q *db.Queries) error
	wake      chan struct{}
	stop      chan struct{}
	stopped   chan struct{}
}

// NewSocialStore starts a SocialStore on a pool. Close stops it.
func NewSocialStore(pool *pgxpool.Pool, log *slog.Logger) *SocialStore {
	if log == nil {
		log = slog.Default()
	}
	s := &SocialStore{pool: pool, q: db.New(pool), log: log, wake: make(chan struct{}, 1), stop: make(chan struct{}), stopped: make(chan struct{})}
	go s.run()
	return s
}

func (s *SocialStore) push(f func(ctx context.Context, q *db.Queries) error) {
	s.mu.Lock()
	s.queue = append(s.queue, f)
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *SocialStore) run() {
	defer close(s.stopped)
	for {
		s.mu.Lock()
		batch := s.queue
		s.queue = nil
		s.mu.Unlock()
		for _, f := range batch {
			ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
			err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return f(ctx, s.q.WithTx(tx)) })
			cancel()
			if err != nil {
				s.log.Error("cannot save a party or friend change", "err", err)
			}
		}
		if len(batch) > 0 {
			continue
		}
		select {
		case <-s.wake:
		case <-s.stop:
			s.mu.Lock()
			empty := len(s.queue) == 0
			s.mu.Unlock()
			if empty {
				return
			}
		}
	}
}

// Close writes what waits, up to ctx, and stops the writer.
func (s *SocialStore) Close(ctx context.Context) error {
	close(s.stop)
	select {
	case <-s.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func leaderID(p social.PartyRecord) int64 {
	for _, m := range p.Members {
		if social.Key(m.Name) == social.Key(p.Leader) {
			return m.CharID
		}
	}
	return 0
}

// CreateParty saves a new party with its members.
func (s *SocialStore) CreateParty(p social.PartyRecord) {
	leader := leaderID(p)
	if leader == 0 {
		return
	}
	s.push(func(ctx context.Context, q *db.Queries) error {
		if err := q.InsertParty(ctx, db.InsertPartyParams{ID: p.ID, Name: p.Name, ExpEven: p.Options.ExpEven, ItemEven: p.Options.ItemEven,
			PickupShared: p.Options.PickupShared, LeaderID: leader}); err != nil {
			return err
		}
		for _, m := range p.Members {
			if err := addMember(ctx, q, p.ID, m); err != nil {
				return err
			}
		}
		return nil
	})
}

// SaveParty saves a party's name, options and leader.
func (s *SocialStore) SaveParty(p social.PartyRecord) {
	leader := leaderID(p)
	if leader == 0 {
		return
	}
	s.push(func(ctx context.Context, q *db.Queries) error {
		return q.UpdateParty(ctx, db.UpdatePartyParams{ID: p.ID, Name: p.Name, ExpEven: p.Options.ExpEven, ItemEven: p.Options.ItemEven,
			PickupShared: p.Options.PickupShared, LeaderID: leader})
	})
}

// DeleteParty deletes a party and its members.
func (s *SocialStore) DeleteParty(id int64) {
	s.push(func(ctx context.Context, q *db.Queries) error { return q.DeleteParty(ctx, id) })
}

func addMember(ctx context.Context, q *db.Queries, party int64, m social.MemberRecord) error {
	if m.CharID == 0 {
		return nil
	}
	if err := q.LeaveParties(ctx, m.CharID); err != nil {
		return err
	}
	return q.InsertPartyMember(ctx, db.InsertPartyMemberParams{PartyID: party, CharacterID: m.CharID, Slot: m.Slot})
}

// AddMember saves a member who joined.
func (s *SocialStore) AddMember(party int64, m social.MemberRecord) {
	if m.CharID == 0 {
		return
	}
	s.push(func(ctx context.Context, q *db.Queries) error { return addMember(ctx, q, party, m) })
}

// RemoveMember saves a member who left.
func (s *SocialStore) RemoveMember(party, charID int64, _ string) {
	if charID == 0 {
		return
	}
	s.push(func(ctx context.Context, q *db.Queries) error {
		return q.DeletePartyMember(ctx, db.DeletePartyMemberParams{PartyID: party, CharacterID: charID})
	})
}

// AddFriend saves one side of a friendship.
func (s *SocialStore) AddFriend(charID int64, _ string, friendID int64, _ string) {
	if charID == 0 || friendID == 0 {
		return
	}
	s.push(func(ctx context.Context, q *db.Queries) error {
		return q.InsertFriend(ctx, db.InsertFriendParams{CharacterID: charID, FriendID: friendID})
	})
}

// RemoveFriend deletes one side of a friendship.
func (s *SocialStore) RemoveFriend(charID int64, _ string, friendID int64, _ string) {
	if charID == 0 || friendID == 0 {
		return
	}
	s.push(func(ctx context.Context, q *db.Queries) error {
		return q.DeleteFriend(ctx, db.DeleteFriendParams{CharacterID: charID, FriendID: friendID})
	})
}

// Flush waits until every change queued before it is written, up to ctx.
func (s *SocialStore) Flush(ctx context.Context) error {
	done := make(chan struct{})
	s.push(func(context.Context, *db.Queries) error {
		close(done)
		return nil
	})
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
