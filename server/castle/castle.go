// Package castle holds guild castles (proposal 0004, sections 1 and 2):
// the records of data/castles.yml and the registry of their saved state.
//
// The world keeps one Registry. Every zone reads and writes it, and each
// write sets the value in memory and queues a save, as rAthena's
// setcastledata saves at once (guild_castledatasave). Owner changes are
// sent to every zone's Feed, so scripts hear castle_change.
package castle

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
)

// File is the data file of castles.
const File = "data/castles.yml"

// Guardians is how many guardian slots a castle has, rAthena's
// MAX_GUARDIANS.
const Guardians = 8

// Editions are rAthena's WoE types, in order: fe is First Edition
// (agitstart), se Second Edition (agitstart2), te Training Edition
// (agitstart3, renewal only).
var Editions = []string{"fe", "se", "te"}

// Teleport is where a member of the owning guild arrives through the
// guild window, and what it costs outside and during the castle's siege.
type Teleport struct {
	X, Y      int
	Cost      int64
	SiegeCost int64
}

// Record is a castle of data/castles.yml.
type Record struct {
	// ID is rAthena's castle ID; mods' castles get IDs from 10,000.
	ID uint32
	// Name is the record's full name: Kriemhild for core's, mod:name
	// for a mod's.
	Name string
	// DisplayName is what getcastlename returns, such as Kriemhild.
	DisplayName string
	Map         string
	// Edition is fe, se or te.
	Edition string
	// NPC is the record name of the NPC that hears OnGuildBreak.
	NPC string
	// ClientID is the client's castle number, 0 for none.
	ClientID uint32
	// Teleport is nil when the castle has none.
	Teleport *Teleport
}

// State is what is saved of a castle (proposal 0004, section 2). The
// fields are rAthena's CD_ indexes 1 to 17.
type State struct {
	// Owner is the owning guild's ID, 0 for none.
	Owner           int64
	Economy         int
	Defense         int
	InvestedEconomy int
	InvestedDefense int
	NextTime        int
	PayTime         int
	CreateTime      int
	Kafra           bool
	// Guardians holds the installed bits of guardians 0 to 7.
	Guardians uint8
}

// Installed reports whether guardian slot i is installed.
func (s State) Installed(i int) bool { return i >= 0 && i < Guardians && s.Guardians&(1<<i) != 0 }

// Values are the fields a Set changes. A nil field is left as it is.
type Values struct {
	Economy, Defense, InvestedEconomy, InvestedDefense *int
	NextTime, PayTime, CreateTime                      *int
	Kafra                                              *bool
}

// Store saves castles. The registry calls it with its lock held, in the
// order the changes happen, so a Store must not block.
type Store interface {
	// SaveCastle saves every field of a castle, by record name.
	SaveCastle(name string, s State)
}

// Source says who changed an owner, for the log.
type Source string

// The sources of owner changes (proposal 0004, section 11).
const (
	SourceScript     Source = "script"
	SourceStaff      Source = "staff"
	SourceGuildBreak Source = "guild_break"
)

// Notice is what a Feed carries: an owner change, or a guild whose badge
// changed (a new emblem or name, or the guild broke up).
type Notice struct {
	// Change is set for an owner change.
	Change *Change
	// Badge is a guild whose badge changed, 0 for none.
	Badge int64
}

// Change is an owner change, the castle_change event's object.
type Change struct {
	Castle   *Record
	Previous int64
	Owner    int64
}

// Feed holds a zone's notices until the zone takes them. Any goroutine
// may push; the zone drains it at each step.
type Feed struct {
	mu      sync.Mutex
	notices []Notice
}

func (f *Feed) push(n Notice) {
	f.mu.Lock()
	f.notices = append(f.notices, n)
	f.mu.Unlock()
}

// Take returns the notices since the last Take, oldest first.
func (f *Feed) Take() []Notice {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.notices
	f.notices = nil
	return out
}

type castle struct {
	rec   *Record
	state State
}

// Registry is the world's castles and their state.
type Registry struct {
	// Store saves each write. Without one nothing is saved.
	Store Store
	// GuildBreak runs a castle NPC's OnGuildBreak label, with no player,
	// when its owner's guild breaks up. Without one nothing runs.
	GuildBreak func(npc, label string)
	// Log gets owner changes at info level.
	Log *slog.Logger

	mu     sync.Mutex
	byName map[string]*castle
	byMap  map[string]*castle
	list   []*castle
	feeds  []*Feed
}

// New makes a registry of records, sorted by ID, with their saved
// states by record name. A record without a saved state starts at
// zeros. Saved states of records that are not loaded stay in the
// database, so a mod put back restores its castles.
func New(recs []*Record, saved map[string]State) *Registry {
	r := &Registry{byName: map[string]*castle{}, byMap: map[string]*castle{}, Log: slog.New(slog.DiscardHandler)}
	for _, rec := range recs {
		c := &castle{rec: rec, state: saved[rec.Name]}
		r.byName[rec.Name] = c
		r.byMap[rec.Map] = c
		r.list = append(r.list, c)
	}
	slices.SortFunc(r.list, func(a, b *castle) int { return int(a.rec.ID) - int(b.rec.ID) })
	return r
}

// Subscribe returns a new feed that gets every later notice.
func (r *Registry) Subscribe() *Feed {
	f := &Feed{}
	r.mu.Lock()
	r.feeds = append(r.feeds, f)
	r.mu.Unlock()
	return f
}

// Unsubscribe stops a feed.
func (r *Registry) Unsubscribe(f *Feed) {
	r.mu.Lock()
	r.feeds = slices.DeleteFunc(r.feeds, func(g *Feed) bool { return g == f })
	r.mu.Unlock()
}

func (r *Registry) notify(n Notice) {
	for _, f := range r.feeds {
		f.push(n)
	}
}

// Find returns a castle's record by full name, by plain name (the
// calling mod's, then core's), or by map, since rAthena's scripts name castles
// by map. local is the calling mod's ID, or "".
func (r *Registry) Find(name, local string) *Record {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.find(name, local); c != nil {
		return c.rec
	}
	return nil
}

func (r *Registry) find(name, local string) *castle {
	if local != "" && !strings.Contains(name, ":") {
		if c := r.byName[local+":"+name]; c != nil {
			return c
		}
	}
	if c := r.byName[name]; c != nil {
		return c
	}
	return r.byMap[name]
}

// Records returns every castle's record, by ID.
func (r *Registry) Records() []*Record {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Record, len(r.list))
	for i, c := range r.list {
		out[i] = c.rec
	}
	return out
}

// State returns a castle's state by record name.
func (r *Registry) State(name string) (State, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.byName[name]
	if c == nil {
		return State{}, false
	}
	return c.state, true
}

// Of returns the castles a guild owns, by ID.
func (r *Registry) Of(guild int64) []*Record {
	if r == nil || guild == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*Record
	for _, c := range r.list {
		if c.state.Owner == guild {
			out = append(out, c.rec)
		}
	}
	return out
}

func (r *Registry) save(c *castle) {
	if r.Store != nil {
		r.Store.SaveCastle(c.rec.Name, c.state)
	}
}

// SetOwner sets a castle's owner, 0 for none. It returns false when the
// owner is already that guild or there is no such castle. Every zone's
// feed gets the change.
func (r *Registry) SetOwner(name string, guild int64, src Source) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.byName[name]
	if c == nil || c.state.Owner == guild {
		return false
	}
	r.setOwner(c, guild, src)
	return true
}

func (r *Registry) setOwner(c *castle, guild int64, src Source) {
	prev := c.state.Owner
	c.state.Owner = guild
	r.save(c)
	r.Log.Info("castle owner changed", "castle", c.rec.Name, "previous", prev, "owner", guild, "source", string(src))
	r.notify(Notice{Change: &Change{Castle: c.rec, Previous: prev, Owner: guild}})
}

// Set changes the named fields in one write. It returns an error for a
// negative value or an unknown castle, and changes nothing then.
func (r *Registry) Set(name string, v Values) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.byName[name]
	if c == nil {
		return fmt.Errorf("no castle %s", name)
	}
	s := c.state
	for _, f := range []struct {
		name string
		v    *int
		to   *int
	}{
		{"economy", v.Economy, &s.Economy}, {"defense", v.Defense, &s.Defense},
		{"invested_economy", v.InvestedEconomy, &s.InvestedEconomy}, {"invested_defense", v.InvestedDefense, &s.InvestedDefense},
		{"next_time", v.NextTime, &s.NextTime}, {"pay_time", v.PayTime, &s.PayTime}, {"create_time", v.CreateTime, &s.CreateTime},
	} {
		if f.v == nil {
			continue
		}
		if *f.v < 0 {
			return fmt.Errorf("%s is %d; it cannot be negative", f.name, *f.v)
		}
		*f.to = *f.v
	}
	if v.Kafra != nil {
		s.Kafra = *v.Kafra
	}
	if s == c.state {
		return nil
	}
	c.state = s
	r.save(c)
	return nil
}

// SetGuardian sets or clears guardian slot index's installed bit.
func (r *Registry) SetGuardian(name string, index int, installed bool) error {
	if index < 0 || index >= Guardians {
		return fmt.Errorf("guardian index %d is not 0 to %d", index, Guardians-1)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.byName[name]
	if c == nil {
		return fmt.Errorf("no castle %s", name)
	}
	bits := c.state.Guardians &^ (1 << index)
	if installed {
		bits |= 1 << index
	}
	if bits != c.state.Guardians {
		c.state.Guardians = bits
		r.save(c)
	}
	return nil
}

// GuildBroken runs, for each castle the guild owns, the OnGuildBreak
// label of the castle's NPC, then clears its owner, as rAthena's
// castle_guild_broken_sub. Every zone's feed also gets the guild, so
// NPCs that show its badge drop it. The social registry calls it with
// its own lock held; the registry never calls back into it.
func (r *Registry) GuildBroken(guild int64) {
	if r == nil || guild == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.list {
		if c.state.Owner != guild {
			continue
		}
		if r.GuildBreak != nil {
			r.GuildBreak(c.rec.NPC, "OnGuildBreak")
		}
		r.setOwner(c, 0, SourceGuildBreak)
	}
	r.notify(Notice{Badge: guild})
}

// BadgeChanged tells every zone that a guild's badge changed: a new
// emblem or name.
func (r *Registry) BadgeChanged(guild int64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notify(Notice{Badge: guild})
}
