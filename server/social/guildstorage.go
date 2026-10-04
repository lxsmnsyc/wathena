package social

import (
	"slices"
	"sync"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Guild storage, as storage.cpp's storage_guild_* functions:
//   - One member at a time has the storage open, as gstor->status. The
//     member's position must allow it (GUILD_PERM_STORAGE).
//   - With Guild Storage in the world's skill tree (renewal), the guild
//     must have learned it, and the storage holds 100 slots and 100 more
//     per level, as OFFICIAL_GUILD_STORAGE, up to the guild_storage
//     limit. Without it in the tree (pre-renewal), the storage holds the
//     limit.
//   - The zone of the member who has it open moves items through
//     server/transfer, with the storage's lock held, and commits the
//     storage with the inventory in one transaction.
//   - A member who leaves, is expelled, logs out or whose guild breaks up
//     has the storage closed.

// StorageSlot is one slot of a guild storage, as the Store saves it.
type StorageSlot = character.Item

// MaxStorageLog is how many moves the guild storage log keeps in memory.
const MaxStorageLog = 100

// StorageLogEntry is one move of the guild storage log, as
// storage_guild_log.
type StorageLogEntry struct {
	At     time.Time
	Name   string
	ItemID uint32
	// Amount is positive for an item put in, negative for one taken out.
	Amount int
	Refine int
	Cards  []uint32
}

// GuildStorage is a guild's storage. Its zone reads and changes it with
// its lock held.
type GuildStorage struct {
	mu    sync.Mutex
	guild int64
	box   transfer.ListBox
	// opener is the member who has it open, or nil.
	opener *Player
	// saved is what the last commit wrote, for the next commit's diff.
	saved []StorageSlot
	log   []StorageLogEntry
}

func newGuildStorage(guild int64, slots []StorageSlot) *GuildStorage {
	st := &GuildStorage{guild: guild, saved: slices.Clone(slots)}
	st.box.Who = transfer.Owner{Kind: transfer.KindGuildStorage, ID: guild}
	for _, s := range slots {
		for len(st.box.Slots) <= s.Slot {
			st.box.Slots = append(st.box.Slots, item.Slot{})
		}
		st.box.Slots[s.Slot] = item.Slot{ID: s.ID, Amount: s.Amount, Serial: s.Serial, Refine: s.Refine, Cards: slices.Clone(s.Cards),
			Extra: s.Clone()}
	}
	return st
}

// Guild is the storage's guild ID.
func (st *GuildStorage) Guild() int64 { return st.guild }

// Lock and Unlock hold the storage while a zone moves items.
func (st *GuildStorage) Lock()   { st.mu.Lock() }
func (st *GuildStorage) Unlock() { st.mu.Unlock() }

// Box is the storage as a transfer box. The lock must be held.
func (st *GuildStorage) Box() *transfer.ListBox { return &st.box }

// Slots are the slots that hold items. The lock must be held.
func (st *GuildStorage) Slots() []StorageSlot { return slotsOf(st.box.Slots) }

func slotsOf(slots []item.Slot) []StorageSlot {
	var out []StorageSlot
	for i, s := range slots {
		if s.ID != 0 && s.Amount > 0 {
			out = append(out, StorageSlot{Slot: i, ID: s.ID, Amount: s.Amount, Serial: s.Serial, Refine: s.Refine,
				Cards: slices.Clone(s.Cards), Extra: s.Clone()})
		}
	}
	return out
}

// Commit returns what the last commit wrote and what the storage holds
// now, and keeps the second for the next commit. The lock must be held.
func (st *GuildStorage) Commit() (base, cur []StorageSlot) {
	cur = st.Slots()
	base, st.saved = st.saved, cur
	return base, cur
}

// takeAll empties the storage and returns what it held, for a guild that
// breaks up. r.mu must be held.
func (st *GuildStorage) takeAll() []StorageSlot {
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	out := st.Slots()
	st.box.Slots, st.box.Weight = nil, 0
	return out
}

// StorageSize is a guild storage's size, and false when the guild cannot
// use it: renewal's Guild Storage skill is not learned. r.mu must be
// held.
func (r *Registry) storageSize(g *Guild) (int, bool) {
	limit := r.limits.GuildStorage
	if r.tables().Skill(SkillGuildStorage) == nil {
		return limit, true
	}
	lv := g.skill(SkillGuildStorage)
	if lv <= 0 {
		return 0, false
	}
	return min(100+100*lv, limit), true
}

// StorageOutcome is why a guild storage action failed, as the
// protocol's StorageOutcome.
type StorageOutcome int

// The storage outcomes.
const (
	StorageOpened StorageOutcome = iota
	StorageInUse
	StorageNoGuild
	StorageNoStorage
	StorageNoPermission
	StorageAlreadyOpen
)

// OpenGuildStorage opens a player's guild storage, as
// storage_guild_storageopen. It returns the storage, which the player's
// zone then moves items in, and its size.
func (r *Registry) OpenGuildStorage(p *Player) (*GuildStorage, int, StorageOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := p.guild
	if g == nil {
		return nil, 0, StorageNoGuild
	}
	size, ok := r.storageSize(g)
	switch {
	case !ok:
		return nil, 0, StorageNoStorage
	case g.storage.opener == p:
		return nil, 0, StorageAlreadyOpen
	case !g.position(g.member(p)).Storage:
		return nil, 0, StorageNoPermission
	case g.storage.opener != nil:
		return nil, 0, StorageInUse
	}
	st := g.storage
	st.mu.Lock()
	st.opener = p
	st.box.Size = size
	st.mu.Unlock()
	return st, size, StorageOpened
}

// CloseGuildStorage closes the storage a player has open, as
// storage_guild_storageclose. It returns false when the player has none
// open.
func (r *Registry) CloseGuildStorage(p *Player, st *GuildStorage) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st == nil {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.opener != p {
		return false
	}
	st.opener = nil
	return true
}

// closeStorageOf closes the guild storage when p has it open, and tells
// p's zone. r.mu must be held.
func (r *Registry) closeStorageOf(g *Guild, p *Player) {
	st := g.storage
	if st == nil {
		return
	}
	st.mu.Lock()
	open := st.opener == p
	if open {
		st.opener = nil
	}
	st.mu.Unlock()
	if open {
		deliver(p, Notice{Kind: NoticeStorageClose})
	}
}

// StorageOpenFor reports whether p still has st open, for a zone about
// to move items. The storage's lock must be held.
func (st *GuildStorage) OpenFor(p *Player) bool { return st.opener == p && p != nil }

// LogStorage adds moves to a guild's storage log, newest last.
func (r *Registry) LogStorage(guild int64, entries []StorageLogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.guilds[guild]
	if g == nil || g.storage == nil || len(entries) == 0 {
		return
	}
	st := g.storage
	st.mu.Lock()
	defer st.mu.Unlock()
	st.log = append(st.log, entries...)
	if n := len(st.log) - MaxStorageLog; n > 0 {
		st.log = slices.Delete(st.log, 0, n)
	}
}

// StorageLog returns a guild's storage log, newest first, for a member
// whose position allows storage, as storage_guild_log_read.
func (r *Registry) StorageLog(p *Player) ([]StorageLogEntry, StorageOutcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := p.guild
	if g == nil {
		return nil, StorageNoGuild
	}
	if _, ok := r.storageSize(g); !ok {
		return nil, StorageNoStorage
	}
	if !g.position(g.member(p)).Storage {
		return nil, StorageNoPermission
	}
	st := g.storage
	st.mu.Lock()
	defer st.mu.Unlock()
	out := slices.Clone(st.log)
	slices.Reverse(out)
	return out, StorageOpened
}

// SetStorageLog sets a guild's storage log, oldest first, at start.
func (r *Registry) SetStorageLog(guild int64, entries []StorageLogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g := r.guilds[guild]; g != nil && g.storage != nil {
		g.storage.log = slices.Clone(entries)
	}
}

// StorageHolding is what one guild storage holds, for the duplication
// tests' audit.
type StorageHolding struct {
	Guild int64
	Items []StorageSlot
}

// AuditStorages lists what every guild storage holds.
func (r *Registry) AuditStorages() []StorageHolding {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []StorageHolding
	for _, g := range r.guilds {
		st := g.storage
		st.mu.Lock()
		out = append(out, StorageHolding{Guild: g.ID, Items: st.Slots()})
		st.mu.Unlock()
	}
	slices.SortFunc(out, func(a, b StorageHolding) int { return int(a.Guild - b.Guild) })
	return out
}
