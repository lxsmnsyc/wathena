package store

import (
	"context"
	"errors"
	"slices"

	"github.com/lxsmnsyc/wathena/server/character"
)

// Holds (decision 0029): a restore takes items from characters that are
// offline. While it works, no login may load those characters, or the
// login would read what the restore is about to change.
//   - Hold keeps a list of characters from loading. It fails when one of
//     them has a session that has not ended: the character is online,
//     or its last save is not written yet, and the restore must go
//     through its zone.
//   - Load waits while a Hold keeps its character. Hold.Load loads a
//     held character for the restore itself.
//   - Release lets the characters load again. A restore releases its
//     Hold once its commit is queued: a later load waits for that commit
//     as it waits for any.

// ErrLive is a Hold of a character that has a session.
var ErrLive = errors.New("store: the character has a session")

// Hold keeps characters from loading.
type Hold struct {
	s   *Store
	ids []int64
}

// Hold keeps characters from loading until Release. It returns ErrLive,
// and holds nothing, when one of them has a session that has not ended
// or another Hold keeps it.
func (s *Store) Hold(ids ...int64) (*Hold, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		if s.live[id] > 0 || s.held[id] {
			return nil, ErrLive
		}
	}
	for _, id := range ids {
		s.held[id] = true
	}
	return &Hold{s: s, ids: slices.Clone(ids)}, nil
}

// Live reports whether a character has a session that has not ended.
func (s *Store) Live(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live[id] > 0
}

// Load starts a session for a held character. The session ends as any
// does, with a final save.
func (h *Hold) Load(ctx context.Context, id int64) (*character.Character, *Session, error) {
	if !slices.Contains(h.ids, id) {
		return nil, nil, errors.New("store: the hold does not keep that character")
	}
	return h.s.load(ctx, id, loadHeld)
}

// Release lets the held characters load again.
func (h *Hold) Release() {
	if h == nil {
		return
	}
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range h.ids {
		delete(s.held, id)
	}
	h.ids = nil
	s.broadcast()
}

// Settle waits until the commits queued before the call are written, so
// the database shows what the zones had handed to the store then.
func (s *Store) Settle(ctx context.Context) error {
	s.mu.Lock()
	target := s.seq
	s.mu.Unlock()
	for {
		s.mu.Lock()
		done, ch := s.written >= target || len(s.lane) == 0, s.changed
		s.mu.Unlock()
		if done {
			return nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
