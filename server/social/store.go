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
	// CreateGuild saves a new guild with its master.
	CreateGuild(g GuildRecord)
	// SaveGuild saves a guild's name, positions, notice, skills,
	// relations, expulsions and master change time.
	SaveGuild(g GuildRecord)
	// SaveGuildExp saves a guild's level, exp and skill points. A Store
	// may write only the last of many.
	SaveGuildExp(guild int64, level int, exp int64, skillPoints int)
	// SaveGuildMemberExp saves what a member paid through the exp tax. A
	// Store may write only the last of many.
	SaveGuildMemberExp(guild, charID, exp int64)
	// SaveGuildEmblem saves a guild's emblem.
	SaveGuildEmblem(guild int64, e Emblem)
	// DeleteGuild deletes a guild, its members and its storage.
	DeleteGuild(id int64)
	// AddGuildMember, SaveGuildMember and RemoveGuildMember save a
	// member who joined, changed, or left.
	AddGuildMember(guild int64, m GuildMemberRecord)
	SaveGuildMember(guild int64, m GuildMemberRecord)
	RemoveGuildMember(guild int64, charID int64, name string)
}
