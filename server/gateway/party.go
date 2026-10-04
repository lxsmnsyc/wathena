package gateway

import (
	"math"
	"strings"
	"unicode/utf8"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// The party, friend and party booking commands (phase 5 step 4). The
// gateway checks their form; the zone and the social registry check the
// rest. A command whose form is wrong is acknowledged and does nothing.

// limitsMessage is the protocol's Limits of a world's limits.
func limitsMessage(l rules.Limits) *wathenav1.Limits {
	if l == (rules.Limits{}) {
		l = rules.DefaultLimits()
	}
	return wathenav1.Limits_builder{
		PartyMembers: uint32(l.PartyMembers), Friends: uint32(l.Friends), BlockList: uint32(l.BlockList), // #nosec G115 -- small bounded limits.
		GuildMembers: uint32(l.GuildMembers), GuildPositions: uint32(l.GuildPositions), GuildAllies: uint32(l.GuildAllies), // #nosec G115 -- as above.
		GuildEnemies: uint32(l.GuildEnemies), GuildStorage: uint32(l.GuildStorage), // #nosec G115 -- as above.
	}.Build()
}

func partyCommand(p *zone.Session, seq uint32, c *wathenav1.PartyCommand) bool {
	switch {
	case c.HasCreate():
		cr := c.GetCreate()
		name := strings.TrimSpace(cr.GetName())
		if !social.ValidPartyName(name) || strings.ContainsFunc(name, func(r rune) bool { return r < ' ' }) {
			return p.Ack(seq)
		}
		o := cr.GetOptions()
		return p.PartyCreate(seq, name, o.GetItemEven(), o.GetPickupShared())
	case c.HasInvite():
		if !ValidName(c.GetInvite()) {
			return p.Ack(seq)
		}
		return p.PartyInvite(seq, c.GetInvite())
	case c.HasReply():
		r := c.GetReply()
		if r.GetPartyId() == 0 || r.GetPartyId() > math.MaxInt64 {
			return p.Ack(seq)
		}
		return p.PartyReply(seq, int64(r.GetPartyId()), r.GetAccept()) // #nosec G115 -- checked above.
	case c.HasLeave():
		return p.PartyLeave(seq)
	case c.HasExpel():
		if !ValidName(c.GetExpel()) {
			return p.Ack(seq)
		}
		return p.PartyExpel(seq, c.GetExpel())
	case c.HasSetLeader():
		if !ValidName(c.GetSetLeader()) {
			return p.Ack(seq)
		}
		return p.PartySetLeader(seq, c.GetSetLeader())
	case c.HasOptions():
		o := c.GetOptions()
		return p.PartyOptions(seq, o.GetExpEven(), o.GetItemEven(), o.GetPickupShared())
	}
	return p.Ack(seq)
}

func friendCommand(p *zone.Session, seq uint32, c *wathenav1.FriendCommand) bool {
	switch {
	case c.HasAdd():
		if !ValidName(c.GetAdd()) {
			return p.Ack(seq)
		}
		return p.FriendAdd(seq, c.GetAdd())
	case c.HasReply():
		r := c.GetReply()
		if !ValidName(r.GetName()) {
			return p.Ack(seq)
		}
		return p.FriendReply(seq, r.GetName(), r.GetAccept())
	case c.HasRemove():
		if !ValidName(c.GetRemove()) {
			return p.Ack(seq)
		}
		return p.FriendRemove(seq, c.GetRemove())
	}
	return p.Ack(seq)
}

// Bounds of a party booking ad.
const (
	maxBookingLevel = 999
	maxBookingMap   = 64
	maxBookingJob   = 1 << 16
)

func bookingCommand(p *zone.Session, seq uint32, c *wathenav1.PartyBookingCommand) bool {
	switch {
	case c.HasRegister():
		a := c.GetRegister()
		if a.GetLevel() < 1 || a.GetLevel() > maxBookingLevel || len(a.GetMap()) > maxBookingMap || len(a.GetJobs()) > social.MaxBookingJobs {
			return p.Ack(seq)
		}
		note := ""
		if a.GetNote() != "" {
			var ok bool
			if note, ok = CleanChat(a.GetNote()); !ok || utf8.RuneCountInString(note) > social.MaxBookingNote {
				return p.Ack(seq)
			}
		}
		ad := social.Ad{Level: int(a.GetLevel()), Map: a.GetMap(), Note: note}
		for _, j := range a.GetJobs() {
			if j >= maxBookingJob {
				return p.Ack(seq)
			}
			ad.Jobs = append(ad.Jobs, int(j))
		}
		return p.BookingRegister(seq, ad)
	case c.HasSearch():
		s := c.GetSearch()
		if s.GetLevel() > maxBookingLevel || len(s.GetMap()) > maxBookingMap || s.GetJob() >= maxBookingJob {
			return p.Ack(seq)
		}
		return p.BookingSearch(seq, int(s.GetLevel()), s.GetMap(), int(s.GetJob()))
	case c.HasDelete():
		return p.BookingDelete(seq)
	}
	return p.Ack(seq)
}
