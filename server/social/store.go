package social

// Store saves parties and friend lists. The registry calls it with its
// lock held, in the order the changes happen, so a Store must not block:
// it queues the writes and runs them in that order.
type Store interface {
	// CreateParty saves a new party with its leader.
	CreateParty(p PartyRecord)
	// SaveParty saves a party's name, options and leader.
	SaveParty(p PartyRecord)
	// DeleteParty deletes a party and its members.
	DeleteParty(id int64)
	// AddMember and RemoveMember save a member who joined or left.
	AddMember(party int64, m MemberRecord)
	RemoveMember(party int64, charID int64, name string)
	// AddFriend and RemoveFriend save one side of a friendship: name on
	// the list of the character charID.
	AddFriend(charID int64, name string, friendID int64, friend string)
	RemoveFriend(charID int64, name string, friendID int64, friend string)
}
