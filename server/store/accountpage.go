package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The game side of the account page (decision 0032): deletion requests,
// the deletion job's call, and block list changes of offline
// characters. The account service writes the requests on its own role.

// Deletion is an account's deletion request.
type Deletion struct {
	Requested, After time.Time
}

// DeletionOf returns the deletion request of a character's account,
// which refuses the character's game logins.
func (s *Store) DeletionOf(ctx context.Context, character int64) (Deletion, bool, error) {
	r, err := s.q.DeletionOfCharacter(ctx, character)
	if errors.Is(err, pgx.ErrNoRows) {
		return Deletion{}, false, nil
	}
	if err != nil {
		return Deletion{}, false, err
	}
	return Deletion{Requested: r.RequestedAt.Time, After: r.DeleteAfter.Time}, true, nil
}

// DueDeletions returns up to limit accounts whose deletion is due at at,
// oldest request first.
func (s *Store) DueDeletions(ctx context.Context, at time.Time, limit int) ([]int64, error) {
	return s.q.DueDeletions(ctx, db.DueDeletionsParams{At: timestamp(at), MaxRows: int32(min(max(limit, 1), 1000))}) // #nosec G115 -- 1 to 1000.
}

// DeleteAccount deletes an account whose deletion is due at at, with its
// characters, in one transaction. accountStorages are the record IDs of
// the extra storages accounts own, whose log lines name the account. It
// returns false when the account has no due request. The caller holds
// the account's characters, so no login loads them meanwhile.
func (s *Store) DeleteAccount(ctx context.Context, account int64, at time.Time, accountStorages []int) (bool, error) {
	if accountStorages == nil {
		accountStorages = []int{}
	}
	return s.q.DeleteAccount(ctx, db.DeleteAccountParams{AccountID: account, At: timestamp(at), AccountStorages: accountStorages})
}

// AccountCharacter is a character of an account, for the account page.
type AccountCharacter struct {
	ID   int64
	Slot int
	Name string
}

// CharactersOf lists an account's characters in slot order.
func (s *Store) CharactersOf(ctx context.Context, account int64) ([]AccountCharacter, error) {
	rows, err := s.q.CharactersOfAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	out := make([]AccountCharacter, 0, len(rows))
	for _, r := range rows {
		out = append(out, AccountCharacter{ID: r.ID, Slot: r.Slot, Name: r.Name})
	}
	return out, nil
}

// BlocksOf reads the saved block lists of some characters.
func (s *Store) BlocksOf(ctx context.Context, ids []int64) (map[int64][]string, error) {
	rows, err := s.q.BlocksOfCharacters(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := map[int64][]string{}
	for _, r := range rows {
		out[r.CharacterID] = append(out[r.CharacterID], r.Name)
	}
	return out, nil
}

// UnblockOffline takes a name off the saved block list of a character
// that has no session. It returns false when the list does not hold the
// name, or the character has a session: its zone holds the list then.
func (s *Store) UnblockOffline(ctx context.Context, character int64, name string) (removed, live bool, err error) {
	h, err := s.Hold(character)
	if errors.Is(err, ErrLive) {
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	defer h.Release()
	n, err := s.q.DeleteCharacterBlock(ctx, db.DeleteCharacterBlockParams{CharacterID: character, Name: name})
	return n > 0, false, err
}
