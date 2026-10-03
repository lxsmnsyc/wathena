package social

import "slices"

// Friend lists, as clif.cpp's friend packets:
//   - A request needs the other player online, and room on the
//     requester's list. The other player answers it.
//   - An accepted request adds the other player to the requester's list,
//     and with friend_auto_add the requester to the other's list.
//   - A removal takes each name off the other's list, also when the
//     other player is offline.
//   - A player who blocks the requester refuses at once, without seeing
//     the request. rAthena does not check its ignore list here
//     (decision 0020).

func (r *Registry) friendViews(p *Player) []FriendView {
	out := make([]FriendView, 0, len(p.friends))
	for _, f := range p.friends {
		out = append(out, FriendView{Name: f.Name, Online: r.online(f.Name) != nil})
	}
	return out
}

func hasFriend(p *Player, key string) bool {
	return slices.ContainsFunc(p.friends, func(f Friend) bool { return Key(f.Name) == key })
}

// friendToggle tells the online players who have p on their list that p
// logged in or out, as clif_friendslist_toggle_sub. r.mu must be held.
func (r *Registry) friendToggle(p *Player, online bool) {
	for _, o := range r.players {
		if o != p && o.online && hasFriend(o, p.key) {
			deliver(o, Notice{Kind: NoticeFriendStatus, Name: p.name, Online: online})
		}
	}
}

func friendResult(p *Player, o FriendOutcome, name string) {
	deliver(p, Notice{Kind: NoticeFriendResult, FriendOutcome: o, Name: name})
}

// FriendAdd asks an online player to be friends, as
// clif_parse_FriendsListAdd.
func (r *Registry) FriendAdd(p *Player, name string) FriendOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	to := r.online(name)
	switch {
	case to == nil:
		friendResult(p, FriendNotOnline, name)
		return FriendNotOnline
	case to == p:
		return 0
	case len(p.friends) >= r.limits.Friends:
		friendResult(p, FriendListFull, to.name)
		return FriendListFull
	case hasFriend(p, to.key):
		friendResult(p, FriendAlready, to.name)
		return FriendAlready
	case r.blocks(to, p):
		friendResult(p, FriendRefused, to.name)
		return FriendRefused
	}
	p.friendReq, to.friendReq = to.key, p.key
	deliver(to, Notice{Kind: NoticeFriendRequest, Name: p.name})
	friendResult(p, FriendRequestSent, to.name)
	return FriendRequestSent
}

// addFriend puts f on p's list and saves it. r.mu must be held.
func (r *Registry) addFriend(p, f *Player) {
	p.friends = append(slices.Clone(p.friends), Friend{Name: f.name, CharID: f.charID})
	r.save(func(s Store) { s.AddFriend(p.charID, p.name, f.charID, f.name) })
	friendResult(p, FriendAdded, f.name)
	deliver(p, Notice{Kind: NoticeFriends, Friends: r.friendViews(p)})
}

// FriendReply answers a friend request, as clif_parse_FriendsListReply.
func (r *Registry) FriendReply(p *Player, name string, accept bool) FriendOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	from := r.online(name)
	if from == nil || from == p {
		return 0
	}
	asked := p.friendReq == from.key && from.friendReq == p.key
	p.friendReq, from.friendReq = "", ""
	if !accept || !asked {
		friendResult(from, FriendRefused, p.name)
		return FriendRefused
	}
	if len(from.friends) >= r.limits.Friends {
		friendResult(from, FriendListFull, p.name)
		return FriendListFull
	}
	if !hasFriend(from, p.key) {
		r.addFriend(from, p)
	}
	if !r.rules.FriendAutoAdd || hasFriend(p, from.key) {
		return FriendAdded
	}
	if len(p.friends) >= r.limits.Friends {
		friendResult(p, FriendListFull, from.name)
		return FriendAdded
	}
	r.addFriend(p, from)
	return FriendAdded
}

// FriendRemove takes a name off a player's list and the player off the
// other's list, as clif_parse_FriendsListRemove.
func (r *Registry) FriendRemove(p *Player, name string) FriendOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := Key(name)
	i := slices.IndexFunc(p.friends, func(f Friend) bool { return Key(f.Name) == k })
	if i < 0 {
		friendResult(p, FriendNotFound, name)
		return FriendNotFound
	}
	f := p.friends[i]
	p.friends = slices.Delete(slices.Clone(p.friends), i, i+1)
	r.save(func(s Store) { s.RemoveFriend(p.charID, p.name, f.CharID, f.Name) })
	if o := r.players[k]; o != nil {
		if j := slices.IndexFunc(o.friends, func(g Friend) bool { return Key(g.Name) == p.key }); j >= 0 {
			o.friends = slices.Delete(slices.Clone(o.friends), j, j+1)
			deliver(o, Notice{Kind: NoticeFriends, Friends: r.friendViews(o)})
		}
	}
	// The other side's row goes too, also when that character is not in
	// memory.
	r.save(func(s Store) { s.RemoveFriend(f.CharID, f.Name, p.charID, p.name) })
	friendResult(p, FriendRemoved, f.Name)
	deliver(p, Notice{Kind: NoticeFriends, Friends: r.friendViews(p)})
	return FriendRemoved
}

// IsFriend reports whether a player has a name on its friend list.
func (r *Registry) IsFriend(p *Player, name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return hasFriend(p, Key(name))
}

// FriendNames returns the names on a player's friend list, in order.
func (r *Registry) FriendNames(p *Player) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(p.friends))
	for _, f := range p.friends {
		out = append(out, f.Name)
	}
	return out
}
