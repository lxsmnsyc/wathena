package mod

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// IDRange is where the IDs of one kind of record that mods add start
// (strategy section 6.5). Core keeps rAthena's IDs, which are all below.
type IDRange struct {
	// Kind is the record kind, as stored with each ID, such as "item".
	Kind  string
	Start uint32
}

// The mod ID ranges of strategy section 6.5.
var (
	ItemIDs        = IDRange{Kind: "item", Start: 2_000_000}
	MonsterIDs     = IDRange{Kind: "monster", Start: 100_000}
	SkillIDs       = IDRange{Kind: "skill", Start: 20_000}
	JobIDs         = IDRange{Kind: "job", Start: 10_000}
	CompanionIDs   = IDRange{Kind: "companion", Start: 10_000}
	MountIDs       = IDRange{Kind: "mount", Start: 10_000}
	StorageIDs     = IDRange{Kind: "storage", Start: 10_000}
	CurrencyIDs    = IDRange{Kind: "currency", Start: 10_000}
	QuestIDs       = IDRange{Kind: "quest", Start: 1_000_000}
	AchievementIDs = IDRange{Kind: "achievement", Start: 1_000_000}
)

// IDRanges lists every kind of mod ID. The mod_ids table's check takes
// each kind (migrations 00019 and 00020), and a store test reserves an
// ID of each.
var IDRanges = []IDRange{ItemIDs, MonsterIDs, SkillIDs, JobIDs, CompanionIDs, MountIDs, StorageIDs, CurrencyIDs, QuestIDs, AchievementIDs}

// AssignedID is one ID that the server gave to a record a mod added.
type AssignedID struct {
	Kind string
	// Name is the record's full name, such as example.halloween:Pumpkin_Hat.
	Name string
	ID   uint32
	// Mod is the mod that added the record when the ID was assigned.
	Mod string
}

// IDStore keeps the IDs given to mod records. An ID, once given, keeps
// its name forever, and is never given to another name, even after the
// mod is removed. server/store keeps them in PostgreSQL. MemoryIDs keeps
// them for a server without a database.
type IDStore interface {
	// ModIDs returns every ID given so far, of every kind.
	ModIDs(ctx context.Context) ([]AssignedID, error)
	// AssignModID gives the next free ID of a kind to a name, from start
	// up, and returns it. A name that has an ID keeps it.
	AssignModID(ctx context.Context, kind, name, mod string, start uint32) (uint32, error)
}

// IDs finds and assigns the IDs of mod records.
type IDs struct {
	store IDStore
	known map[string]uint32 // kind + "\x00" + name
	// Assigned are the IDs this server start gave for the first time.
	Assigned []AssignedID
}

// LoadIDs reads the IDs the store has given.
func LoadIDs(ctx context.Context, store IDStore) (*IDs, error) {
	list, err := store.ModIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("mod IDs: %w", err)
	}
	ids := &IDs{store: store, known: make(map[string]uint32, len(list))}
	for _, a := range list {
		ids.known[a.Kind+"\x00"+a.Name] = a.ID
	}
	return ids, nil
}

// ID returns the ID of a record a mod added, and gives it one when it
// has none yet.
func (ids *IDs) ID(ctx context.Context, r IDRange, name, mod string) (uint32, error) {
	key := r.Kind + "\x00" + name
	if id, ok := ids.known[key]; ok {
		return id, nil
	}
	id, err := ids.store.AssignModID(ctx, r.Kind, name, mod, r.Start)
	if err != nil {
		return 0, fmt.Errorf("mod IDs: %s %s: %w", r.Kind, name, err)
	}
	if id < r.Start {
		return 0, fmt.Errorf("mod IDs: %s %s got %d, below the mod range at %d", r.Kind, name, id, r.Start)
	}
	ids.known[key] = id
	ids.Assigned = append(ids.Assigned, AssignedID{Kind: r.Kind, Name: name, ID: id, Mod: mod})
	return id, nil
}

// MemoryIDs is an IDStore that lives in memory, for a server without a
// database, where nothing else is saved either, and for tests.
type MemoryIDs struct {
	mu   sync.Mutex
	list []AssignedID
}

// ModIDs returns every ID given so far.
func (m *MemoryIDs) ModIDs(context.Context) ([]AssignedID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]AssignedID(nil), m.list...), nil
}

// AssignModID gives the next free ID of a kind.
func (m *MemoryIDs) AssignModID(_ context.Context, kind, name, mod string, start uint32) (uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := start
	for _, a := range m.list {
		if a.Kind != kind {
			continue
		}
		if a.Name == name {
			return a.ID, nil
		}
		if a.ID >= next {
			next = a.ID + 1
		}
	}
	m.list = append(m.list, AssignedID{Kind: kind, Name: name, ID: next, Mod: mod})
	return next, nil
}

// splitName splits a record name such as example.halloween:Pumpkin_Hat
// into the mod ID and the local name. A core record has no prefix.
func splitName(name string) (modID, local string, ok bool) {
	i := strings.LastIndex(name, ":")
	if i < 0 {
		return "", name, false
	}
	return name[:i], name[i+1:], true
}
