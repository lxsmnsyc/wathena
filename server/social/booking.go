package social

import (
	"slices"
	"time"
)

// Party booking, as rAthena's party_booking_*. Ads live in memory, one
// per character, and end at logout, as map_quit's party_booking_delete.

// Limits of party booking.
const (
	// MaxBookingJobs is MAX_PARTY_BOOKING_JOBS.
	MaxBookingJobs = 6
	// MaxBookingResults is MAX_PARTY_BOOKING_RESULTS.
	MaxBookingResults = 10
	// MaxBookingNote is the longest note, in characters. rAthena's ads
	// have no note.
	MaxBookingNote = 80
	// bookingLevelRange is how far below the searched level an ad may
	// be, as party_booking_search.
	bookingLevelRange = 15
)

// Ad is a party booking ad.
type Ad struct {
	Level int
	Map   string
	Jobs  []int
	Note  string
}

// BookingEntry is a placed ad.
type BookingEntry struct {
	Name string
	Ad   Ad
	At   time.Time
	// index orders ads, as party_booking_ad_info.index.
	index int64
	key   string
}

// BookingView answers a booking command.
type BookingView struct {
	Entries []BookingEntry
	More    bool
	// Own is the player's own ad, or nil.
	Own     *BookingEntry
	Refused bool
}

type booking struct {
	ads  []*BookingEntry
	next int64
}

func (b *booking) find(key string) *BookingEntry {
	for _, a := range b.ads {
		if a.key == key {
			return a
		}
	}
	return nil
}

func (b *booking) remove(key string) bool {
	n := len(b.ads)
	b.ads = slices.DeleteFunc(b.ads, func(a *BookingEntry) bool { return a.key == key })
	return len(b.ads) != n
}

func (r *Registry) bookingNotice(p *Player, v BookingView) {
	if own := r.booking.find(p.key); own != nil {
		c := *own
		v.Own = &c
	}
	deliver(p, Notice{Kind: NoticeBooking, Booking: v})
}

// BookingRegister places a player's ad, as party_booking_register. A
// player with an ad is refused.
func (r *Registry) BookingRegister(p *Player, ad Ad) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.booking.find(p.key) != nil {
		r.bookingNotice(p, BookingView{Refused: true})
		return false
	}
	r.booking.next++
	ad.Jobs = slices.Clone(ad.Jobs)
	r.booking.ads = append(r.booking.ads, &BookingEntry{Name: p.name, Ad: ad, At: r.now(), index: r.booking.next, key: p.key})
	r.bookingNotice(p, BookingView{})
	return true
}

// BookingDelete removes a player's ad, as party_booking_delete.
func (r *Registry) BookingDelete(p *Player) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.booking.remove(p.key)
	r.bookingNotice(p, BookingView{})
}

// BookingSearch finds ads, as party_booking_search: a level finds ads
// from 15 below it to it, and a search is by map or by job. As in
// rAthena, a search with both a map and a job finds nothing. It finds at
// most MaxBookingResults, oldest first.
func (r *Registry) BookingSearch(p *Player, level int, mapName string, job int) BookingView {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := BookingView{}
	for _, a := range r.booking.ads {
		if level > 0 && (a.Ad.Level < level-bookingLevelRange || a.Ad.Level > level) {
			continue
		}
		match := false
		switch {
		case mapName == "" && job == 0:
			match = true
		case mapName == "":
			match = slices.Contains(a.Ad.Jobs, job)
		case job == 0:
			match = a.Ad.Map == mapName
		}
		if !match {
			continue
		}
		if len(v.Entries) >= MaxBookingResults {
			v.More = true
			break
		}
		v.Entries = append(v.Entries, *a)
	}
	r.bookingNotice(p, v)
	return v
}
