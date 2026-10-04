package rules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Limits and rules of a ruleset (strategy section 6.8, proposal 0002,
// review decision 27):
//   - A limit is a number rAthena compiles in, such as MAX_PARTY. A
//     ruleset mod sets it in its limits block. Its default is rAthena's
//     value, and it has a hard maximum so the database and the protocol
//     can hold every value.
//   - A rule is a value of rAthena's conf files, such as
//     party_share_level. A ruleset mod sets it in its rules block.
//   - A value outside its bounds fails the ruleset's load.
//   - Lowering a limit deletes nothing. A list over the limit keeps its
//     entries and cannot grow.

// Limits are a world's limits.
type Limits struct {
	// PartyMembers is MAX_PARTY: the most members a party holds.
	PartyMembers int
	// Friends is MAX_FRIENDS: the most names a friend list holds.
	Friends int
	// BlockList is MAX_IGNORE_LIST: the most names a block list holds.
	BlockList int
	// GuildMembers is the most members a guild holds before Guild
	// Extension: the 16 of int_guild.cpp's guild_calcinfo. Each level of
	// GD_EXTENSION adds 6.
	GuildMembers int
	// GuildPositions is MAX_GUILDPOSITION: the positions of a guild.
	GuildPositions int
	// GuildAllies and GuildEnemies are max_guild_alliance: the most
	// allied and enemy guilds a guild has. rAthena counts each against
	// the one setting.
	GuildAllies, GuildEnemies int
	// GuildStorage is the most slots a guild storage holds. Renewal's
	// Guild Storage skill gives 100 + 100 per level, up to it.
	GuildStorage int
}

// Guild holds the guild values of rAthena's conf/battle/guild.conf and
// conf/battle/skill.conf.
type Guild struct {
	// EmperiumCheck is guild_emperium_check: making a guild from the
	// client takes an Emperium.
	EmperiumCheck bool
	// ExpLimit is guild_exp_limit: the highest exp tax of a position.
	ExpLimit int
	// EmergencyCall is emergency_call: the bits that say where Emergency
	// Call works (1 outside sieges, 2 in sieges, 4 outside GvG maps, 8
	// on GvG maps, 16 not on nowarpto maps).
	EmergencyCall int
	// SkillRelogDelay is guild_skill_relog_delay in milliseconds: how
	// long a master's guild skills wait after a login or a master change
	// in pre-renewal.
	SkillRelogDelay int
	// LeaderChangeDelay is guild_leaderchange_delay. rAthena compares it
	// with seconds, though its comment says minutes.
	LeaderChangeDelay int
	// AllianceOnlyMaster is guild_alliance_onlygm: only a guild master
	// may accept an alliance.
	AllianceOnlyMaster bool
	// set is set by GuildOf, so a zero Guild reads as rAthena's rules.
	set bool
}

// Party holds the party and friend rules of a world.
type Party struct {
	// ShareLevel is party_share_level of inter_athena.conf: even exp
	// share needs the online members' base levels within this range.
	ShareLevel int
	// EvenShareBonus is party_even_share_bonus: the exp bonus in percent
	// for each member after the first that even exp share counts.
	EvenShareBonus int
	// ItemShareType is party_item_share_type: bit 1 gives even item share
	// only to items with a first looter, and bit 2 picks the member in
	// turn instead of at random.
	ItemShareType int
	// LeaderSameMap is change_party_leader_samemap: a new leader must be
	// on the leader's map.
	LeaderSameMap bool
	// BlockAccount is block_account_in_same_party: two characters of one
	// account cannot be in one party.
	BlockAccount bool
	// FriendAutoAdd is friend_auto_add: a friend request that is
	// accepted adds each player to the other's list, not only the
	// requester's.
	FriendAutoAdd bool
	// set is set by PartyOf, so a zero Party reads as rAthena's rules.
	set bool
}

// Bound is one limit or rule: its name in a ruleset mod, rAthena's
// default and the values it may take.
type Bound struct {
	Name     string
	Default  int
	Min, Max int
	// Bool is set for a rule that is yes or no. It is stored as 1 or 0.
	Bool bool
	// About says what it is, for errors and the docs.
	About string
}

// LimitBounds are the limits a ruleset may set.
var LimitBounds = []Bound{
	{Name: "party_members", Default: 12, Min: 2, Max: 64, About: "the most members a party holds (MAX_PARTY)"},
	{Name: "friends", Default: 40, Min: 1, Max: 200, About: "the most names a friend list holds (MAX_FRIENDS)"},
	{Name: "block_list", Default: 20, Min: 1, Max: 100, About: "the most names a block list holds (MAX_IGNORE_LIST)"},
	{Name: "guild_members", Default: 16, Min: 1, Max: 200, About: "the most members a guild holds before Guild Extension"},
	{Name: "guild_positions", Default: 20, Min: 2, Max: 40, About: "the positions of a guild (MAX_GUILDPOSITION)"},
	{Name: "guild_allies", Default: 3, Min: 0, Max: 10, About: "the most allied guilds of a guild (max_guild_alliance)"},
	{Name: "guild_enemies", Default: 3, Min: 0, Max: 10, About: "the most enemy guilds of a guild (max_guild_alliance)"},
	{Name: "guild_storage", Default: 500, Min: 1, Max: 2000, About: "the most slots a guild storage holds"},
}

// RuleBounds are the rules a ruleset may set.
var RuleBounds = []Bound{
	{Name: "party_share_level", Default: 15, Min: 0, Max: 1000, About: "the base level range of even exp share (party_share_level)"},
	{Name: "party_even_share_bonus", Default: 0, Min: 0, Max: 1000, About: "the even share exp bonus per member in percent (party_even_share_bonus)"},
	{Name: "party_item_share_type", Default: 0, Min: 0, Max: 3, About: "how even item share picks a member (party_item_share_type)"},
	{Name: "change_party_leader_samemap", Default: 1, Min: 0, Max: 1, Bool: true, About: "a new leader must be on the leader's map"},
	{Name: "block_account_in_same_party", Default: 1, Min: 0, Max: 1, Bool: true, About: "two characters of one account cannot be in one party"},
	{Name: "friend_auto_add", Default: 1, Min: 0, Max: 1, Bool: true, About: "an accepted friend request adds both players"},
	{Name: "guild_emperium_check", Default: 1, Min: 0, Max: 1, Bool: true, About: "making a guild takes an Emperium"},
	{Name: "guild_exp_limit", Default: 50, Min: 0, Max: 99, About: "the highest exp tax of a guild position (guild_exp_limit)"},
	{Name: "emergency_call", Default: 11, Min: 0, Max: 31, About: "where Emergency Call works (emergency_call)"},
	{Name: "guild_skill_relog_delay", Default: 300000, Min: 0, Max: 86400000, About: "milliseconds a new master's guild skills wait in pre-renewal"},
	{Name: "guild_leaderchange_delay", Default: 1440, Min: 0, Max: 1000000000, About: "the wait between guild master changes (guild_leaderchange_delay)"},
	{Name: "guild_alliance_onlygm", Default: 0, Min: 0, Max: 1, Bool: true, About: "only a guild master may accept an alliance"},
}

// DefaultLimits are rAthena's limits.
func DefaultLimits() Limits {
	l, _ := LimitsOf(nil)
	return l
}

// DefaultGuild are rAthena's guild rules.
func DefaultGuild() Guild {
	g, _ := GuildOf(nil)
	return g
}

// DefaultParty are rAthena's party rules.
func DefaultParty() Party {
	p, _ := PartyOf(nil)
	return p
}

// FindBound finds a limit or a rule by name.
func FindBound(list []Bound, name string) (Bound, bool) {
	i := slices.IndexFunc(list, func(b Bound) bool { return b.Name == name })
	if i < 0 {
		return Bound{}, false
	}
	return list[i], true
}

// ParseValue reads a value of a bound from a ruleset mod: a whole number,
// or yes, no, on, off, true or false for a rule that is yes or no. It
// checks the bounds.
func (b Bound) ParseValue(s string) (int, error) {
	var n int
	if b.Bool {
		switch strings.ToLower(s) {
		case "yes", "on", "true", "1":
			n = 1
		case "no", "off", "false", "0":
			n = 0
		default:
			return 0, fmt.Errorf("%s is yes or no, not %q", b.Name, s)
		}
		return n, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s is a whole number, not %q", b.Name, s)
	}
	if n < b.Min || n > b.Max {
		return 0, fmt.Errorf("%s is %d to %d, not %d", b.Name, b.Min, b.Max, n)
	}
	return n, nil
}

// valuesOf fills a map with every bound's value: set's, or the default.
func valuesOf(list []Bound, set map[string]int) (map[string]int, error) {
	out := map[string]int{}
	for _, b := range list {
		out[b.Name] = b.Default
	}
	for name, v := range set {
		b, ok := FindBound(list, name)
		if !ok {
			return nil, fmt.Errorf("there is no %s", name)
		}
		if v < b.Min || v > b.Max {
			return nil, fmt.Errorf("%s is %d to %d, not %d", name, b.Min, b.Max, v)
		}
		out[name] = v
	}
	return out, nil
}

// LimitsOf turns the limits a ruleset set into Limits. A limit it does
// not set has rAthena's value.
func LimitsOf(set map[string]int) (Limits, error) {
	v, err := valuesOf(LimitBounds, set)
	if err != nil {
		return Limits{}, err
	}
	return Limits{PartyMembers: v["party_members"], Friends: v["friends"], BlockList: v["block_list"],
		GuildMembers: v["guild_members"], GuildPositions: v["guild_positions"], GuildAllies: v["guild_allies"],
		GuildEnemies: v["guild_enemies"], GuildStorage: v["guild_storage"]}, nil
}

// GuildOf turns the rules a ruleset set into Guild. A rule it does not
// set has rAthena's value.
func GuildOf(set map[string]int) (Guild, error) {
	v, err := valuesOf(RuleBounds, set)
	if err != nil {
		return Guild{}, err
	}
	return Guild{
		EmperiumCheck: v["guild_emperium_check"] == 1, ExpLimit: v["guild_exp_limit"], EmergencyCall: v["emergency_call"],
		SkillRelogDelay: v["guild_skill_relog_delay"], LeaderChangeDelay: v["guild_leaderchange_delay"],
		AllianceOnlyMaster: v["guild_alliance_onlygm"] == 1, set: true,
	}, nil
}

// PartyOf turns the rules a ruleset set into Party. A rule it does not
// set has rAthena's value.
func PartyOf(set map[string]int) (Party, error) {
	v, err := valuesOf(RuleBounds, set)
	if err != nil {
		return Party{}, err
	}
	return Party{
		ShareLevel: v["party_share_level"], EvenShareBonus: v["party_even_share_bonus"], ItemShareType: v["party_item_share_type"],
		LeaderSameMap: v["change_party_leader_samemap"] == 1, BlockAccount: v["block_account_in_same_party"] == 1,
		FriendAutoAdd: v["friend_auto_add"] == 1, set: true,
	}, nil
}

// Named returns the limits by name, in LimitBounds order, for Lua's
// rules.limits and the client.
func (l Limits) Named() []NamedValue {
	return []NamedValue{{"party_members", l.PartyMembers}, {"friends", l.Friends}, {"block_list", l.BlockList},
		{"guild_members", l.GuildMembers}, {"guild_positions", l.GuildPositions}, {"guild_allies", l.GuildAllies},
		{"guild_enemies", l.GuildEnemies}, {"guild_storage", l.GuildStorage}}
}

// NamedValue is a limit's name and value.
type NamedValue struct {
	Name  string
	Value int
}

// LimitsOrDefault returns r's limits, or rAthena's for nil rules or
// rules made before limits existed.
func LimitsOrDefault(r *Rules) Limits {
	if r == nil || r.Limits == (Limits{}) {
		return DefaultLimits()
	}
	l := r.Limits
	if l.GuildPositions == 0 {
		// Rules made before the guild limits existed.
		d := DefaultLimits()
		l.GuildMembers, l.GuildPositions, l.GuildAllies, l.GuildEnemies, l.GuildStorage = d.GuildMembers, d.GuildPositions,
			d.GuildAllies, d.GuildEnemies, d.GuildStorage
	}
	return l
}

// GuildOrDefault returns r's guild rules, or rAthena's for nil rules or
// rules made without them.
func GuildOrDefault(r *Rules) Guild {
	if r == nil || !r.Guild.set {
		return DefaultGuild()
	}
	return r.Guild
}

// PartyOrDefault returns r's party rules, or rAthena's for nil rules or
// rules made without them.
func PartyOrDefault(r *Rules) Party {
	if r == nil || !r.Party.set {
		return DefaultParty()
	}
	return r.Party
}
