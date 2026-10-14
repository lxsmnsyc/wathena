package social

import "slices"

// Account deletion (decision 0032). Before the deletion job deletes an
// account's characters from the database, the registry lets them go, so
// it never writes a row that names them again.

// GuildLedWithOthers returns the guild a character leads, when the guild
// has other members. A request to delete the character's account is
// refused then.
func (r *Registry) GuildLedWithOthers(charID int64) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.guilds {
		if m := g.master(); m != nil && m.p.charID == charID && charID != 0 && g.size() > 1 {
			return g.Name, true
		}
	}
	return "", false
}

// ForgetCharacters takes characters out of the registry, for the
// deletion of their account:
//   - Each leaves its party. A leader that leaves breaks the party up,
//     as the char server does.
//   - Each leaves its guild. A master hands the guild to the member in
//     the lowest slot, as guildchangegm does. A master without other
//     members breaks the guild up, and the guild storage's items go
//     with it: they are returned.
//   - Each goes off the friend lists in memory, and its open requests
//     and ads end. The database deletes the friend rows with the
//     character.
//
// The characters must be offline: their account's logins are refused.
func (r *Registry) ForgetCharacters(ids []int64) (lost []StorageSlot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	gone := map[int64]bool{}
	for _, id := range ids {
		if id != 0 {
			gone[id] = true
		}
	}
	var players []*Player
	for _, p := range r.players {
		if gone[p.charID] {
			players = append(players, p)
		}
	}
	slices.SortFunc(players, func(a, b *Player) int { return int(a.charID - b.charID) })
	keys := map[string]bool{}
	for _, p := range players {
		keys[p.key] = true
		if pt := p.party; pt != nil {
			r.remove(pt, p, PartyLeft, PartyLeft, ReasonLeft)
		}
		if g := p.guild; g != nil {
			lost = append(lost, r.leaveGuild(g, p)...)
		}
		r.booking.remove(p.key)
	}
	for _, o := range r.players {
		if keys[o.key] {
			continue
		}
		if keys[o.friendReq] {
			o.friendReq = ""
		}
		if o.invite != nil && keys[Key(o.invite.from)] {
			o.invite = nil
		}
		n := len(o.friends)
		o.friends = slices.DeleteFunc(slices.Clone(o.friends), func(f Friend) bool {
			return gone[f.CharID] || keys[Key(f.Name)]
		})
		if len(o.friends) != n {
			deliver(o, Notice{Kind: NoticeFriends, Friends: r.friendViews(o)})
		}
	}
	for k := range keys {
		delete(r.players, k)
	}
	return lost
}

// leaveGuild takes a character out of its guild for its deletion. r.mu
// must be held.
func (r *Registry) leaveGuild(g *Guild, p *Player) []StorageSlot {
	m := g.member(p)
	if m == nil {
		return nil
	}
	if g.members[0] == m {
		var next *guildMember
		for _, o := range g.members[1:] {
			if o != nil {
				next = o
				break
			}
		}
		if next == nil {
			left := g.storage.takeAll()
			r.guildBreak(g)
			return left
		}
		r.setMaster(g, next)
	}
	r.guildRemove(g, m, GuildLeft, ReasonLeft, false, "")
	return nil
}
