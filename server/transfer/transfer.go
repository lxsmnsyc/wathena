// Package transfer moves items and zeny between owners in one step, with
// checks and logs (strategy section 3.5, decision 0017).
//
// A Tx lists moves:
//   - Move takes an amount from a slot of one box and puts it in another.
//   - Create puts an amount in a box from an end: the ground, a monster,
//     a shop, a script or a skill.
//   - Destroy takes an amount from a box to an end.
//   - Pay, Earn and MoveZeny move zeny between wallets and ends.
//
// Apply checks every move on copies of the boxes and wallets first:
// ownership by serial, amounts, weight and slot limits, and the stack
// rules of strategy section 5.4. When one fails, nothing changes. Then
// it makes the moves on the real boxes and returns a log line for each.
//
// Serials follow strategy section 5.4, "Stacking":
//   - A whole stack that moves into a new slot keeps its serial.
//   - Part of a stack that moves into a new slot gets a new serial, and
//     its log line names the stack it came from.
//   - An amount that joins a stack takes that stack's serial, and its
//     log line names where it came from.
//   - A new slot from an end gets a new serial. Its log line names the
//     serial it came from, such as the stack a ground item was dropped
//     from, when there is one.
//
// The package has no database and no locks. The zone runs transfers on
// its own goroutine. The store writes the log lines in the same
// transaction as the characters the transfers changed (server/store).
package transfer

import (
	"time"

	"github.com/lxsmnsyc/wathena/server/item"
)

// Kind is what holds or gives an item or zeny.
type Kind uint8

// Kinds. Holders keep items that are saved. Ends are where items come
// from or go to outside any holder. Their IDs:
//   - Inventory, Cart and Vending: the character's database ID.
//   - Storage: the account's database ID, or for an extra storage that
//     a character owns, the character's. Owner.Storage names the extra
//     storage.
//   - GuildStorage: the guild's ID.
//   - Mail: the mail's ID.
//   - Monster: the monster's ID, such as 1002 for a Poring.
//   - Skill: the skill's ID.
//   - Ground, Shop, Script, Used and Fee: 0.
//   - Staff: the staff account's ID.
//
// A currency's wallet (CurrencyWallet) is the inventory's owner for a
// character's currency, and the account's storage for an account's.
const (
	KindNone Kind = iota
	KindInventory
	KindStorage
	KindCart
	KindGuildStorage
	KindMail
	KindVending
	KindGround
	KindMonster
	KindShop
	KindScript
	KindSkill
	KindUsed
	// KindFee is where fees and taxes go: the mail fee, the mail
	// attachment price and the vending tax.
	KindFee
	// KindEvent is where a world event's rewards come from (review
	// decision 31).
	KindEvent
	// KindStaff is where a restore's copies and zeny come from, when the
	// staff tools make them (decision 0029), and the items of @item
	// (decision 0030). Its ID is the staff account's.
	KindStaff
)

var kindNames = [...]string{
	KindNone:         "none",
	KindInventory:    "inventory",
	KindStorage:      "storage",
	KindCart:         "cart",
	KindGuildStorage: "guild_storage",
	KindMail:         "mail",
	KindVending:      "vending",
	KindGround:       "ground",
	KindMonster:      "monster",
	KindShop:         "shop",
	KindScript:       "script",
	KindSkill:        "skill",
	KindUsed:         "used",
	KindFee:          "fee",
	KindEvent:        "event",
	KindStaff:        "staff",
}

// String is the kind's name in logs and in the database, such as
// inventory.
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "none"
}

// ParseKind reads a kind's name.
func ParseKind(s string) (Kind, bool) {
	for k, n := range kindNames {
		if n == s && k != int(KindNone) {
			return Kind(k), true // #nosec G115 -- below len(kindNames).
		}
	}
	return KindNone, false
}

// Holder reports whether the kind keeps saved items.
func (k Kind) Holder() bool { return k >= KindInventory && k <= KindVending }

// Owner is a holder or an end.
type Owner struct {
	Kind Kind
	ID   int64
	// Storage is the record ID of an extra storage (server/storage), or
	// 0 for an account's storage and every other kind. ID is then the
	// account's or the character's, as the record says.
	Storage uint32
}

// Inventory is a character's inventory.
func Inventory(characterID int64) Owner { return Owner{Kind: KindInventory, ID: characterID} }

// End is an end of a kind with no ID.
func End(k Kind) Owner { return Owner{Kind: k} }

// Action is why a transfer happened. Logs keep it.
type Action string

// Actions of the paths that use transfers.
const (
	// ActionDrop is a player dropping an item on the ground.
	ActionDrop Action = "drop"
	// ActionPickUp is a player taking a ground item.
	ActionPickUp Action = "pick_up"
	// ActionLoot is an item a monster gives straight to a player: a pet
	// egg, an MVP prize, or what a skill steals.
	ActionLoot Action = "loot"
	// ActionBuy and ActionSell are NPC shop trades.
	ActionBuy  Action = "buy"
	ActionSell Action = "sell"
	// ActionGive and ActionTake are a script's give_item and take_item,
	// and its zeny calls.
	ActionGive Action = "give"
	ActionTake Action = "take"
	// ActionUse is an item a player used, or that a skill spent.
	ActionUse Action = "use"
	// ActionSkill is an item or zeny a skill made or cost.
	ActionSkill Action = "skill"
	// ActionMove is a move between two holders, such as a trade or the
	// guild storage.
	ActionMove Action = "move"
	// ActionGuild is an item a guild took: the Emperium that makes a
	// guild, and the guild storage of a guild that broke up.
	ActionGuild Action = "guild"
	// ActionStorage is a move between an inventory or a cart and a
	// storage.
	ActionStorage Action = "storage"
	// ActionCart is a move between an inventory and a cart.
	ActionCart Action = "cart"
	// ActionTrade is a move of a trade between two players.
	ActionTrade Action = "trade"
	// ActionBound is a bound copy that went away with its bond, such as
	// a party-bound item of a player who left the party.
	ActionBound Action = "bound"
	// ActionMail is a mail's attachments and fees: sending, taking, and
	// a mail that went back to its sender or was deleted when it
	// expired.
	ActionMail Action = "mail"
	// ActionVend is a sale of a vending shop, with its tax.
	ActionVend Action = "vend"
	// ActionBuyingStore is a sale to a buying store.
	ActionBuyingStore Action = "buying_store"
	// ActionEvent is a world event's reward, and its mail when it did
	// not fit.
	ActionEvent Action = "event"
	// ActionRestore is a restore of the staff tools: items that move
	// back from their holder, and the copies and zeny it makes, into the
	// mail that brings them (decision 0029).
	ActionRestore Action = "restore"
	// ActionCommand is an item a GM command made, such as @item, from
	// the staff end (decision 0030).
	ActionCommand Action = "command"
	// ActionQuest is an item a quest's drop gave to the player who
	// killed a monster, as rAthena's LOG_TYPE_QUEST (decision 0043).
	ActionQuest Action = "quest"
)

// ItemLog is one item move.
type ItemLog struct {
	At     time.Time
	Action Action
	From   Owner
	To     Owner
	// Serial is the stack or instance the amount ended in. For a move to
	// an end, it is the stack the amount left.
	Serial item.Serial
	// Source is the serial the amount came from when it is not Serial:
	// the stack that a split or a merge came from, or the stack a ground
	// item was dropped from. It is zero otherwise.
	Source item.Serial
	ItemID uint32
	Amount int
	Refine int
	Cards  []uint32
	// Extra holds the rolls, the cards' rolls and the random options.
	item.Extra
}

// ZenyLog is one zeny move, or one currency move.
type ZenyLog struct {
	At     time.Time
	Action Action
	From   Owner
	To     Owner
	// Amount is what reached To. Zeny over MaxZeny that Earn drops is
	// not in it.
	Amount int64
	// Currency is the currency's ID of data/currencies.yml, or 0 for
	// zeny.
	Currency uint32
}

// Logs are the log lines of one or more transfers.
type Logs struct {
	Items []ItemLog
	Zeny  []ZenyLog
}

// Add appends other logs.
func (l *Logs) Add(o Logs) {
	l.Items = append(l.Items, o.Items...)
	l.Zeny = append(l.Zeny, o.Zeny...)
}

// Empty reports whether there are no lines.
func (l *Logs) Empty() bool { return len(l.Items) == 0 && len(l.Zeny) == 0 }
