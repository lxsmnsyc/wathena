// Package variables keeps the server variables of mods (proposal 0002,
// "Variables"): values of the whole world, which every zone reads and
// writes. Character and account variables travel with the character
// (server/character) instead.
package variables

import (
	"context"
	"fmt"
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

// Core is core's mod ID, which the count limits leave out.
const Core = "core"

// RetryDelay is the wait before a failed write runs again.
const RetryDelay = 5 * time.Second

// Limits of one mod's server variables (review decision 52).
const (
	// MaxModValues is the most server variables one mod may hold. Core
	// has no such limit (review decision 58): its converted NPCs share
	// one mod.
	MaxModValues = 256
	// MaxModBytes is the most bytes of JSON one mod's server variables
	// may hold together.
	MaxModBytes = 1 << 20
)

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
	// count and bytes are each mod's number of values and their bytes.
	count map[string]int
	bytes map[string]int
}

// NewServer makes the server variables. store may be nil, which keeps
// them in memory only.
func NewServer(store Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Server{store: store, log: log, values: map[Key]string{}, dirty: map[Key]bool{}, wake: make(chan struct{}, 1),
		count: map[string]int{}, bytes: map[string]int{}}
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
	if vals == nil {
		vals = map[Key]string{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = vals
	clear(s.count)
	clear(s.bytes)
	for k, v := range vals {
		s.count[k.Mod]++
		s.bytes[k.Mod] += len(v)
	}
	return nil
}

// put changes a value in memory, within the mod's limits. The lock must
// be held. Values loaded over the limits stay, and can shrink.
func (s *Server) put(k Key, value string) error {
	old, had := s.values[k]
	count, bytes := s.count[k.Mod], s.bytes[k.Mod]-len(old)
	if had {
		count--
	}
	if value != "" {
		count++
		bytes += len(value)
		if !had && count > MaxModValues && k.Mod != Core {
			return fmt.Errorf("%s holds %d server variables, the most a mod may", k.Mod, MaxModValues)
		}
		if len(value) > len(old) && bytes > MaxModBytes {
			return fmt.Errorf("%s's server variables would take %d bytes, more than the %d a mod may", k.Mod, bytes, MaxModBytes)
		}
		s.values[k] = value
	} else {
		delete(s.values, k)
	}
	s.count[k.Mod], s.bytes[k.Mod] = count, bytes
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
// row. It fails when the value goes over the mod's limits.
func (s *Server) Set(k Key, value string) error {
	s.mu.Lock()
	if err := s.put(k, value); err != nil {
		s.mu.Unlock()
		return err
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
	return nil
}

// Update changes a variable in one step: f gets the current value, or
// "" without one, and returns the new value. Other zones wait while f
// runs, so f must be quick.
func (s *Server) Update(k Key, f func(old string) (string, error)) (string, error) {
	s.mu.Lock()
	cur := s.values[k]
	next, err := f(cur)
	if err == nil {
		err = s.put(k, next)
	}
	if err != nil {
		s.mu.Unlock()
		return "", err
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
