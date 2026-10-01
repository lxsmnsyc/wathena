// Package variables keeps the server variables of mods (proposal 0002,
// "Variables"): values of the whole world, which every zone reads and
// writes. Character and account variables travel with the character
// (server/character) instead.
package variables

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Key names one server variable of a mod.
type Key struct {
	Mod, Name string
}

// Store keeps server variables, such as PostgreSQL through server/store.
type Store interface {
	// ServerVariables returns every saved value, as JSON.
	ServerVariables(ctx context.Context) (map[Key]string, error)
	// SetServerVariable saves a value as JSON. An empty value deletes it.
	SetServerVariable(ctx context.Context, k Key, value string) error
}

// RetryDelay is the wait before a failed write runs again.
const RetryDelay = 5 * time.Second

// Server holds the server variables in memory. Zones read and write
// them from their own goroutines. With a Store, Run writes each change
// soon after it happens: the last value of a variable wins, and a value
// that changes again before it is written is written once.
type Server struct {
	store Store
	log   *slog.Logger

	mu     sync.Mutex
	values map[Key]string
	dirty  map[Key]bool
	wake   chan struct{}
}

// NewServer makes the server variables. store may be nil, which keeps
// them in memory only.
func NewServer(store Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Server{store: store, log: log, values: map[Key]string{}, dirty: map[Key]bool{}, wake: make(chan struct{}, 1)}
}

// Load reads the saved values. Call it before the zones run.
func (s *Server) Load(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	vals, err := s.store.ServerVariables(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = vals
	return nil
}

// Get returns a variable's value as JSON, and false when it has none.
func (s *Server) Get(k Key) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[k]
	return v, ok
}

// Set changes a variable. An empty value is the default, which has no
// row.
func (s *Server) Set(k Key, value string) {
	s.mu.Lock()
	if value == "" {
		delete(s.values, k)
	} else {
		s.values[k] = value
	}
	if s.store != nil {
		s.dirty[k] = true
	}
	s.mu.Unlock()
	if s.store != nil {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// Update changes a variable in one step: f gets the current value, or
// "" without one, and returns the new value. Other zones wait while f
// runs, so f must be quick.
func (s *Server) Update(k Key, f func(old string) (string, error)) (string, error) {
	s.mu.Lock()
	cur := s.values[k]
	next, err := f(cur)
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	if next == "" {
		delete(s.values, k)
	} else {
		s.values[k] = next
	}
	if s.store != nil {
		s.dirty[k] = true
	}
	s.mu.Unlock()
	if s.store != nil {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return next, nil
}

// Run writes changed values until ctx ends, then writes what is left,
// for up to 10 seconds.
func (s *Server) Run(ctx context.Context) {
	if s.store == nil {
		return
	}
	for {
		select {
		case <-s.wake:
			s.flush(ctx)
		case <-ctx.Done():
			last, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			s.flush(last)
			cancel()
			return
		}
	}
}

// flush writes the dirty values. A value that fails stays dirty, and is
// tried again after RetryDelay.
func (s *Server) flush(ctx context.Context) {
	s.mu.Lock()
	batch := make(map[Key]string, len(s.dirty))
	for k := range s.dirty {
		batch[k] = s.values[k]
	}
	clear(s.dirty)
	s.mu.Unlock()
	failed := false
	for k, v := range batch {
		if err := s.store.SetServerVariable(ctx, k, v); err != nil {
			s.log.Error("server variable not saved; trying again", "mod", k.Mod, "name", k.Name, "err", err)
			s.mu.Lock()
			s.dirty[k] = true
			s.mu.Unlock()
			failed = true
		}
	}
	if failed && ctx.Err() == nil {
		time.AfterFunc(RetryDelay, func() {
			select {
			case s.wake <- struct{}{}:
			default:
			}
		})
	}
}

// Pending reports how many changed values wait to be written.
func (s *Server) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.dirty)
}
