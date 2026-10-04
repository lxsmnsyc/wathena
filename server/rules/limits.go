package rules

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Limits and rules of a ruleset (strategy section 6.8, proposal 0002,
// review decisions 27 and 29):
//   - A limit is a number rAthena compiles in, such as MAX_PARTY. A
//     ruleset mod sets it in its limits block. Each limit has a range:
//     base, what a player or guild starts with, and max, where
//     expansions such as items, Guild Extension or scripts stop.
//   - The engine has a ceiling per limit (Bound.Max). The database and
//     the protocol hold every value up to it, and a ruleset whose max is
//     above it fails to load.
//   - A rule is a value of rAthena's conf files, such as
//     party_share_level. A ruleset mod sets it in its rules block.
//   - A value outside its bounds fails the ruleset's load.
//   - Lowering a limit deletes nothing. A list over the limit keeps its
//     entries and cannot grow.

// Limits are a world's limits. Each int field is the limit's base. Max
// holds each limit's max, in LimitBounds order: read it with MaxOf.
type Limits struct {
	// PartyMembers is MAX_PARTY: the most members a party holds.
	PartyMembers int
	// Friends is MAX_FRIENDS: the most names a friend list holds.
	Friends int
	// BlockList is MAX_IGNORE_LIST: the most names a block list holds.
	BlockList int
	// GuildMembers is the most members a guild holds before Guild
	// Extension: the 16 of int_guild.cpp's guild_calcinfo. Each level of
	// GD_EXTENSION adds 6, up to the limit's max, MAX_GUILD.
	GuildMembers int
	// GuildPositions is MAX_GUILDPOSITION: the positions of a guild.
	GuildPositions int
	// GuildAllies and GuildEnemies are max_guild_alliance: the most
	// allied and enemy guilds a guild has. rAthena counts each against
	// the one setting.
	GuildAllies, GuildEnemies int
	// GuildStorage is the most slots a guild storage holds without
	// renewal's Guild Storage skill, MAX_GUILD_STORAGE. The skill gives
	// 100 + 100 per level, up to the limit's max.
	GuildStorage int
	// InventorySlots is INVENTORY_BASE_SIZE: the inventory slots a new
	// character has. Its max is MAX_INVENTORY, where inventory
	// expansions stop.
	InventorySlots int
	// StorageSlots is MAX_STORAGE: the slots of an account's storage.
	StorageSlots int
	// CartSlots is MAX_CART: the slots of a cart.
	CartSlots int
	// MailItems is MAIL_MAX_ITEM: the most item stacks one mail holds.
	MailItems int
	// Mailbox is the most mails a mailbox holds.
	Mailbox int
	// Max are the limits' max values, in LimitBounds order.
	Max [NumLimits]int
}

// NumLimits is the number of limits in LimitBounds.
const NumLimits = 13

// Range is a limit's base and max.
type Range struct {
	Base, Max int
}

// RangeSet is what a ruleset mod set of a limit: the base, the max, or
// both. A plain number sets the base.
type RangeSet struct {
	Base, Max       int
	HasBase, HasMax bool
}

// Merge returns s with the parts o sets replaced, for a ruleset that
// extends another.
func (s RangeSet) Merge(o RangeSet) RangeSet {
	if o.HasBase {
		s.Base, s.HasBase = o.Base, true
	}
	if o.HasMax {
		s.Max, s.HasMax = o.Max, true
	}
	return s
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
	Name    string
	Default int
	// Min and Max bound the value. For a limit, Default is the base and
	// Max is the engine's ceiling: a ruleset's base and max must be at
	// most Max.
	Min, Max int
	// DefaultMax is a limit's max when a ruleset sets none.
	DefaultMax int
	// Bool is set for a rule that is yes or no. It is stored as 1 or 0.
	Bool bool
	// About says what it is, for errors and the docs.
	About string
}

// LimitBounds are the limits a ruleset may set. The order is Limits.Max's.
// Max is the engine's ceiling, set from the benchmark of decision 0023.
var LimitBounds = []Bound{
	{Name: "party_members", Default: 12, DefaultMax: 12, Min: 2, Max: 256, About: "the most members a party holds (MAX_PARTY)"},
	{Name: "friends", Default: 40, DefaultMax: 40, Min: 1, Max: 1000, About: "the most names a friend list holds (MAX_FRIENDS)"},
	{Name: "block_list", Default: 20, DefaultMax: 20, Min: 1, Max: 1000, About: "the most names a block list holds (MAX_IGNORE_LIST)"},
	{Name: "guild_members", Default: 16, DefaultMax: 76, Min: 1, Max: 512, About: "the most members a guild holds before Guild Extension, and with it (MAX_GUILD)"},
	{Name: "guild_positions", Default: 20, DefaultMax: 20, Min: 2, Max: 40, About: "the positions of a guild (MAX_GUILDPOSITION)"},
	{Name: "guild_allies", Default: 3, DefaultMax: 3, Min: 0, Max: 10, About: "the most allied guilds of a guild (max_guild_alliance)"},
	{Name: "guild_enemies", Default: 3, DefaultMax: 3, Min: 0, Max: 10, About: "the most enemy guilds of a guild (max_guild_alliance)"},
	{Name: "guild_storage", Default: 600, DefaultMax: 600, Min: 1, Max: 10000, About: "the most slots a guild storage holds (MAX_GUILD_STORAGE)"},
	{Name: "inventory_slots", Default: 100, DefaultMax: 200, Min: 1, Max: 1000, About: "the inventory slots of a new character, and with expansions (MAX_INVENTORY)"},
	{Name: "storage_slots", Default: 600, DefaultMax: 600, Min: 1, Max: 10000, About: "the slots of an account's storage (MAX_STORAGE)"},
	{Name: "cart_slots", Default: 100, DefaultMax: 100, Min: 1, Max: 1000, About: "the slots of a cart (MAX_CART)"},
	// mail_items and mailbox are review decision 30's.
	{Name: "mail_items", Default: 5, DefaultMax: 5, Min: 1, Max: 20, About: "the most item stacks one mail holds (MAIL_MAX_ITEM)"},
	{Name: "mailbox", Default: 100, DefaultMax: 100, Min: 1, Max: 1000, About: "the most mails a mailbox holds"},
}

// Ceiling is a limit's engine ceiling, or 0 for a name that is not a
// limit.
func Ceiling(name string) int {
	b, _ := FindBound(LimitBounds, name)
	return b.Max
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
	{Name: "max_cart_weight", Default: 8000, Min: 0, Max: 1000000000, About: "the weight a cart holds, in tenths (max_cart_weight)"},
	{Name: "basic_skill_check", Default: 1, Min: 0, Max: 1, Bool: true, About: "trading needs Basic Skill level 1, and storage level 6 (basic_skill_check)"},
	// The vending, buying store, store search and mail rules of review
	// decision 30, with the values of rAthena's conf/battle/items.conf,
	// misc.conf, feature.conf, player.conf and char_athena.conf.
	{Name: "vending_tax", Default: 500, Min: 0, Max: 10000, About: "the vending tax in hundredths of a percent (vending_tax)"},
	{Name: "vending_tax_min", Default: 100000000, Min: 0, Max: 2147483647, About: "the smallest purchase the vending tax applies to (vending_tax_min)"},
	{Name: "vending_max_value", Default: 1000000000, Min: 1, Max: 2147483647, About: "the highest price of a vended item (vending_max_value)"},
	{Name: "vending_over_max", Default: 1, Min: 0, Max: 1, Bool: true, About: "a shop may open when its sales would pass the zeny cap (vending_over_max)"},
	{Name: "min_npc_vendchat_distance", Default: 3, Min: 0, Max: 30, About: "how many cells a shop keeps from NPCs (min_npc_vendchat_distance)"},
	{Name: "autotrade_restore", Default: 1, Min: 0, Max: 1, Bool: true, About: "autotrade stores open again after a restart (feature.autotrade)"},
	{Name: "searchstore_querydelay", Default: 10, Min: 0, Max: 3600, About: "the seconds between store searches (searchstore_querydelay)"},
	{Name: "searchstore_maxresults", Default: 30, Min: 1, Max: 1000, About: "the most results of one store search (searchstore_maxresults)"},
	{Name: "mail_zeny_fee", Default: 2, Min: 0, Max: 100, About: "the fee of mailed zeny in percent (mail_zeny_fee)"},
	{Name: "mail_attachment_price", Default: 2500, Min: 0, Max: 2147483647, About: "the zeny for each attached item (mail_attachment_price)"},
	{Name: "mail_attachment_weight", Default: 2000, Min: 0, Max: 1000000000, About: "the most weight of a mail's items, in whole weight units, or 0 for none (mail_attachment_weight)"},
	{Name: "mail_daily_count", Default: 100, Min: 0, Max: 1000000, About: "the most mails a character sends a day, or 0 for no limit (mail_daily_count)"},
	{Name: "mail_delay", Default: 1000, Min: 1000, Max: 3600000, About: "the milliseconds between two mails of a character (mail_delay)"},
	{Name: "mail_return_days", Default: 15, Min: 0, Max: 3650, About: "the days before a mail goes back to its sender, or 0 for never (mail_return_days)"},
	{Name: "mail_delete_days", Default: 15, Min: 0, Max: 3650, About: "the days before a mail that came back is deleted, or 0 for never (mail_delete_days)"},
	{Name: "mail_return_empty", Default: 1, Min: 0, Max: 1, Bool: true, About: "a mail without attachments goes back to its sender too (mail_return_empty)"},
}

// Market holds the vending, buying store, store search and mail rules of
// a world (review decision 30).
type Market struct {
	// VendingTax is vending_tax in hundredths of a percent, and
	// VendingTaxMin vending_tax_min: the tax applies to a purchase of at
	// least that much zeny.
	VendingTax, VendingTaxMin int64
	// VendingMaxValue is vending_max_value: the highest price of a line.
	VendingMaxValue int64
	// VendingOverMax is vending_over_max: a shop may open when its sales
	// would pass MAX_ZENY.
	VendingOverMax bool
	// NPCDistance is min_npc_vendchat_distance.
	NPCDistance int
	// AutotradeRestore is feature.autotrade: autotrade stores open again
	// after a restart.
	AutotradeRestore bool
	// SearchDelay is searchstore_querydelay in seconds, and SearchMax
	// searchstore_maxresults.
	SearchDelay, SearchMax int
	// MailZenyFee is mail_zeny_fee in percent, MailAttachmentPrice
	// mail_attachment_price, and MailAttachmentWeight
	// mail_attachment_weight in whole weight units, or 0 for no limit.
	MailZenyFee, MailAttachmentPrice int64
	MailAttachmentWeight             int
	// MailDailyCount is mail_daily_count, 0 for no limit, and MailDelay
	// mail_delay in milliseconds.
	MailDailyCount, MailDelay int
	// MailReturnDays and MailDeleteDays are mail_return_days and
	// mail_delete_days, 0 for never. MailReturnEmpty is mail_return_empty.
	MailReturnDays, MailDeleteDays int
	MailReturnEmpty                bool
	set                            bool
}

// MarketOf turns the rules a ruleset set into Market. A rule it does not
// set has rAthena's value.
func MarketOf(set map[string]int) (Market, error) {
	v, err := valuesOf(RuleBounds, set)
	if err != nil {
		return Market{}, err
	}
	return Market{
		VendingTax: int64(v["vending_tax"]), VendingTaxMin: int64(v["vending_tax_min"]), VendingMaxValue: int64(v["vending_max_value"]),
		VendingOverMax: v["vending_over_max"] == 1, NPCDistance: v["min_npc_vendchat_distance"],
		AutotradeRestore: v["autotrade_restore"] == 1, SearchDelay: v["searchstore_querydelay"], SearchMax: v["searchstore_maxresults"],
		MailZenyFee: int64(v["mail_zeny_fee"]), MailAttachmentPrice: int64(v["mail_attachment_price"]),
		MailAttachmentWeight: v["mail_attachment_weight"], MailDailyCount: v["mail_daily_count"], MailDelay: v["mail_delay"],
		MailReturnDays: v["mail_return_days"], MailDeleteDays: v["mail_delete_days"], MailReturnEmpty: v["mail_return_empty"] == 1,
		set: true,
	}, nil
}

// DefaultMarket are rAthena's vending, store search and mail rules.
func DefaultMarket() Market {
	m, _ := MarketOf(nil)
	return m
}

// MarketOrDefault returns r's market rules, or rAthena's for nil rules
// or rules made without them.
func MarketOrDefault(r *Rules) Market {
	if r == nil || !r.Market.set {
		return DefaultMarket()
	}
	return r.Market
}

// VendingTaxOf is vending_calc_tax: what the seller keeps of a purchase
// of zeny. The tax applies to the whole purchase.
func (m Market) VendingTaxOf(zeny int64) int64 {
	if m.VendingTax > 0 && zeny >= m.VendingTaxMin {
		return zeny - zeny*m.VendingTax/10000
	}
	return zeny
}

// Items holds the storage, cart and trade values of rAthena's
// conf/battle/player.conf.
type Items struct {
	// MaxCartWeight is max_cart_weight in tenths, as item weights.
	MaxCartWeight int
	// BasicSkillCheck is basic_skill_check: trading needs Basic Skill
	// level 1, and the Kafras' storage level 6.
	BasicSkillCheck bool
	set             bool
}

// ItemsOf turns the rules a ruleset set into Items. A rule it does not
// set has rAthena's value.
func ItemsOf(set map[string]int) (Items, error) {
	v, err := valuesOf(RuleBounds, set)
	if err != nil {
		return Items{}, err
	}
	return Items{MaxCartWeight: v["max_cart_weight"], BasicSkillCheck: v["basic_skill_check"] == 1, set: true}, nil
}

// DefaultItems are rAthena's storage, cart and trade rules.
func DefaultItems() Items {
	i, _ := ItemsOf(nil)
	return i
}

// ItemsOrDefault returns r's item rules, or rAthena's for nil rules or
// rules made without them.
func ItemsOrDefault(r *Rules) Items {
	if r == nil || !r.Items.set {
		return DefaultItems()
	}
	return r.Items
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
// not set has rAthena's range. A max it does not set is the default max,
// or the base when the base is higher. The base must be at most the max,
// and the max at most the engine's ceiling.
func LimitsOf(set map[string]RangeSet) (Limits, error) {
	var out Limits
	base := map[string]int{}
	for name := range set {
		if _, ok := FindBound(LimitBounds, name); !ok {
			return Limits{}, fmt.Errorf("there is no %s", name)
		}
	}
	for i, b := range LimitBounds {
		s := set[b.Name]
		r := Range{Base: b.Default, Max: b.DefaultMax}
		if s.HasBase {
			r.Base = s.Base
		}
		switch {
		case s.HasMax:
			r.Max = s.Max
		case r.Base > r.Max:
			r.Max = r.Base
		}
		if err := b.CheckRange(r); err != nil {
			return Limits{}, err
		}
		base[b.Name] = r.Base
		out.Max[i] = r.Max
	}
	out.PartyMembers, out.Friends, out.BlockList = base["party_members"], base["friends"], base["block_list"]
	out.GuildMembers, out.GuildPositions, out.GuildAllies = base["guild_members"], base["guild_positions"], base["guild_allies"]
	out.GuildEnemies, out.GuildStorage = base["guild_enemies"], base["guild_storage"]
	out.InventorySlots, out.StorageSlots, out.CartSlots = base["inventory_slots"], base["storage_slots"], base["cart_slots"]
	out.MailItems, out.Mailbox = base["mail_items"], base["mailbox"]
	return out, nil
}

// CheckRange checks a limit's range: the base from Min, at most the max,
// and the max at most the engine's ceiling.
func (b Bound) CheckRange(r Range) error {
	switch {
	case r.Max > b.Max:
		return fmt.Errorf("%s: the max is at most %d, the engine's ceiling, not %d", b.Name, b.Max, r.Max)
	case r.Base < b.Min || r.Base > b.Max:
		return fmt.Errorf("%s is %d to %d, not %d", b.Name, b.Min, b.Max, r.Base)
	case r.Base > r.Max:
		return fmt.Errorf("%s: the base, %d, is above the max, %d", b.Name, r.Base, r.Max)
	}
	return nil
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
// rules.limits and the client. Value is the base.
func (l Limits) Named() []NamedValue {
	v := l.bases()
	out := make([]NamedValue, len(LimitBounds))
	for i, b := range LimitBounds {
		out[i] = NamedValue{Name: b.Name, Value: v[i], Max: l.Max[i]}
	}
	return out
}

// bases are the limits' bases, in LimitBounds order.
func (l Limits) bases() []int {
	return []int{l.PartyMembers, l.Friends, l.BlockList, l.GuildMembers, l.GuildPositions, l.GuildAllies, l.GuildEnemies,
		l.GuildStorage, l.InventorySlots, l.StorageSlots, l.CartSlots, l.MailItems, l.Mailbox}
}

// MaxOf is a limit's max by name, or 0 for a name that is not a limit.
func (l Limits) MaxOf(name string) int {
	for i, b := range LimitBounds {
		if b.Name == name {
			return l.Max[i]
		}
	}
	return 0
}

// NamedValue is a limit's name and value.
type NamedValue struct {
	Name  string
	Value int
	// Max is the limit's max.
	Max int
}

// LimitsOrDefault returns r's limits, or rAthena's for nil rules or
// rules made before limits existed. Limits made before a limit existed
// get its default range.
func LimitsOrDefault(r *Rules) Limits {
	if r == nil || r.Limits == (Limits{}) {
		return DefaultLimits()
	}
	l := r.Limits
	d := DefaultLimits()
	if l.GuildPositions == 0 {
		// Rules made before the guild limits existed.
		l.GuildMembers, l.GuildPositions, l.GuildAllies, l.GuildEnemies, l.GuildStorage = d.GuildMembers, d.GuildPositions,
			d.GuildAllies, d.GuildEnemies, d.GuildStorage
	}
	if l.InventorySlots == 0 {
		// Rules made before the storage limits existed.
		l.InventorySlots, l.StorageSlots, l.CartSlots = d.InventorySlots, d.StorageSlots, d.CartSlots
	}
	if l.MailItems == 0 {
		// Rules made before the mail limits existed.
		l.MailItems, l.Mailbox = d.MailItems, d.Mailbox
	}
	for i := range l.Max {
		if l.Max[i] == 0 {
			l.Max[i] = max(d.Max[i], l.bases()[i])
		}
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
