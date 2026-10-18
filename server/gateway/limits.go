package gateway

import (
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Command rate limits, per connection (strategy section 7.4, review
// 2026-10):
//   - Every message takes a token from the connection's bucket, and most
//     commands also take one from the bucket of their kind. A message
//     with no token left is dropped before it reaches the zone. Chat
//     has its own limit instead (chat.go).
//   - Each dropped message, and each command the zone's queue refused,
//     is a strike. Strikes fall by one every StrikeEvery. A connection
//     with more than MaxStrikes is disconnected with
//     DISCONNECT_REASON_RATE_LIMITED.
//   - DefaultRates has the numbers. Gateway.Rates may set others, such
//     as for tests that send faster than people do.
//   - The limits sit well above what a person or the load test bots
//     send: the web client sends a walk at most every 100 ms while the
//     mouse is held, and a bot thinks every 100 ms.

// Rate is a token bucket: Burst messages at once, then one every Every.
type Rate struct {
	Burst int
	Every time.Duration
}

// Rates are a connection's command rate limits.
type Rates struct {
	// All limits all messages but chat.
	All Rate
	// Ping, Walk, Action, Drop, NPC, Storage, Social and Search limit
	// each kind of command (kindOf).
	Ping, Walk, Action, Drop, NPC, Storage, Social, Search Rate
	// MaxStrikes dropped messages, less one every StrikeEvery, end the
	// connection.
	MaxStrikes  int
	StrikeEvery time.Duration
}

// DefaultRates are the rates a Gateway uses when its Rates are zero.
func DefaultRates() Rates {
	return Rates{
		All:  Rate{Burst: 60, Every: time.Second / 30},
		Ping: Rate{Burst: 10, Every: 200 * time.Millisecond},
		// WalkTo runs a path search.
		Walk: Rate{Burst: 20, Every: 100 * time.Millisecond},
		// Attack, PickUp, UseItem, EquipItem, UnequipItem, InsertCard,
		// RaiseStat, UseSkill, LearnSkill and guild skill ups, as a
		// player pressing hotkeys.
		Action: Rate{Burst: 30, Every: 50 * time.Millisecond},
		// DropItem. The zone limits drops too (zone dropAllowed).
		Drop: Rate{Burst: 10, Every: 200 * time.Millisecond},
		// NpcClick starts a script. NpcReply and the shop commands
		// follow the dialogs.
		NPC: Rate{Burst: 20, Every: 100 * time.Millisecond},
		// Storage moves, as a player moving many stacks.
		Storage: Rate{Burst: 30, Every: 50 * time.Millisecond},
		// Party, friend, guild, trade, mail, store, block, report and
		// channel commands, which reach other players or the database.
		Social: Rate{Burst: 20, Every: 200 * time.Millisecond},
		// A StoreSearch query scans every store of the map.
		Search:      Rate{Burst: 5, Every: time.Second},
		MaxStrikes:  100,
		StrikeEvery: 100 * time.Millisecond,
	}
}

// of is the rate of a kind of command.
func (r *Rates) of(k commandKind) Rate {
	switch k {
	case kindPing:
		return r.Ping
	case kindWalk:
		return r.Walk
	case kindAction:
		return r.Action
	case kindDrop:
		return r.Drop
	case kindNpc:
		return r.NPC
	case kindStorage:
		return r.Storage
	case kindSocial:
		return r.Social
	case kindSearch:
		return r.Search
	}
	return Rate{}
}

// commandKind groups commands that share a rate.
type commandKind int

const (
	// kindOther has only MessageRate.
	kindOther commandKind = iota
	// kindChat has its own limit instead (chat.go), which tells the
	// player a line was too fast.
	kindChat
	kindPing
	kindWalk
	kindAction
	kindDrop
	kindNpc
	kindStorage
	kindSocial
	kindSearch
	kindCount
)

// kindOf is the kind of a client message.
func kindOf(m *wathenav1.ClientMessage) commandKind {
	if m.HasPing() {
		return kindPing
	}
	c := m.GetCommand()
	switch {
	case c == nil:
		return kindOther
	case c.HasChat():
		return kindChat
	case c.HasWalkTo():
		return kindWalk
	case c.HasAttack(), c.HasPickUp(), c.HasUseItem(), c.HasEquipItem(), c.HasUnequipItem(),
		c.HasInsertCard(), c.HasRaiseStat(), c.HasUseSkill(), c.HasLearnSkill():
		return kindAction
	case c.HasGuild() && c.GetGuild().HasSkillUp():
		// A guild master raises guild skills as a player raises its own,
		// a click each, and the change reaches no other player's screen
		// but as a guild update.
		return kindAction
	case c.HasDropItem():
		return kindDrop
	case c.HasNpcClick(), c.HasNpcReply(), c.HasShopBuy(), c.HasShopSell(), c.HasShopClose():
		return kindNpc
	case c.HasStorage():
		return kindStorage
	case c.HasParty(), c.HasFriend(), c.HasPartyBooking(), c.HasGuild(), c.HasTrade(), c.HasMail(),
		c.HasVending(), c.HasBuyingStore(), c.HasBlock(), c.HasReport(), c.HasChannel(), c.HasMemo(),
		c.HasQuestActive():
		return kindSocial
	case c.HasStoreSearch():
		// Only a query scans the stores. Pages, clicks and closes follow
		// the window, as NPC replies do.
		if c.GetStoreSearch().HasQuery() {
			return kindSearch
		}
		return kindNpc
	}
	return kindOther
}

// bucket is a token bucket kept as the time its tokens are all back.
type bucket struct{ full time.Time }

// take takes a token at now, and reports false when none is left.
func (b *bucket) take(now time.Time, r Rate) bool {
	next := b.full
	if next.Before(now) {
		next = now
	}
	next = next.Add(r.Every)
	if next.Sub(now) > time.Duration(r.Burst)*r.Every {
		return false
	}
	b.full = next
	return true
}

// commandLimit is one connection's command rate limits and strikes.
type commandLimit struct {
	rates   Rates
	all     bucket
	kinds   [kindCount]bucket
	strikes float64
	last    time.Time
}

func newCommandLimit(now time.Time, rates Rates) *commandLimit {
	return &commandLimit{rates: rates, last: now}
}

// allow reports whether a message of a kind, sent at now, may go to the
// zone. When it may not, over reports whether the connection has too
// many strikes.
func (l *commandLimit) allow(now time.Time, k commandKind) (ok, over bool) {
	if k == kindChat {
		return true, false
	}
	if k != kindOther && !l.kinds[k].take(now, l.rates.of(k)) {
		return false, l.strike(now)
	}
	if !l.all.take(now, l.rates.All) {
		return false, l.strike(now)
	}
	return true, false
}

// strike counts a dropped message, and reports whether the connection
// has too many strikes.
func (l *commandLimit) strike(now time.Time) bool {
	if d := now.Sub(l.last); d > 0 {
		l.strikes = max(0, l.strikes-float64(d)/float64(l.rates.StrikeEvery))
		l.last = now
	}
	l.strikes++
	return l.strikes > float64(l.rates.MaxStrikes)
}
