package zone

import (
	"context"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Transfers (decision 0017): every change to a player's items and zeny
// goes through server/transfer, so it is checked as one step and logged
// with serials.
//   - addCopy, deleteItem and the zeny calls each run one Tx. Shops run
//     a purchase or a sale as one Tx.
//   - The player sees each change in the order of the moves, with the
//     messages it got before transfers existed.
//   - The log lines wait in the zone. After each event the zone handles,
//     commitTransfers hands the characters the transfers changed and
//     the lines to the Committer, which writes them in one transaction.
//     A player's later saves come after its commits.
//   - Without a Committer, as without a database, the lines go to the
//     log at debug level, and saves happen as before.

// Committer saves the characters that transfers changed, with the log
// lines of the transfers, in one transaction (decision 0017). Commit
// must not block. Commits are written in the order they come, and a
// character's later saves are written after its commits.
type Committer interface {
	Commit(saves []CommitSave, logs transfer.Logs)
}

// CommitSave is a character a commit saves, with the saver that saves
// it otherwise.
type CommitSave struct {
	Saver     Saver
	Character *character.Character
}

// origin is where items given to a player come from, for the logs.
type origin struct {
	action transfer.Action
	from   transfer.Owner
	// source is the serial the items came from, such as the stack a
	// ground item was dropped from, or zero.
	source item.Serial
}

// scriptOrigin is a script's give.
var scriptOrigin = origin{action: transfer.ActionGive, from: transfer.End(transfer.KindScript)}

// skillOrigin is an item a skill makes.
func skillOrigin(id int) origin {
	return origin{action: transfer.ActionSkill, from: transfer.Owner{Kind: transfer.KindSkill, ID: int64(id)}}
}

// monsterOwner is a monster as the end its items come from.
func monsterOwner(e *entity) transfer.Owner {
	if e == nil || e.monster == nil || e.monster.spawn == nil {
		return transfer.End(transfer.KindMonster)
	}
	return transfer.Owner{Kind: transfer.KindMonster, ID: int64(e.monster.spawn.Mob)}
}

// owner is the player's inventory as an owner: its character's database
// ID, or 0 for a character that only lives in memory.
func (p *Player) owner() transfer.Owner { return transfer.Inventory(p.record.ID) }

// box is the player's inventory as a transfer box.
func (z *Zone) box(p *Player) *transfer.InventoryBox {
	return &transfer.InventoryBox{Inv: p.inventory, Items: z.Items, MaxWeight: p.maxWeight, Who: p.owner()}
}

// wallet is the player's zeny as a transfer wallet.
func (z *Zone) wallet(p *Player) *transfer.ZenyWallet {
	return &transfer.ZenyWallet{Who: p.owner(), Z: &p.record.Zeny}
}

// removeEnd is the action and end of a removal, from its reason.
func removeEnd(r wathenav1.InventoryRemoveReason) (transfer.Action, transfer.Owner) {
	switch r {
	case wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_DROPPED:
		return transfer.ActionDrop, transfer.End(transfer.KindGround)
	case wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TAKEN:
		return transfer.ActionTake, transfer.End(transfer.KindScript)
	case wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_SOLD:
		return transfer.ActionSell, transfer.End(transfer.KindShop)
	}
	return transfer.ActionUse, transfer.End(transfer.KindUsed)
}

// apply runs a Tx on a player's inventory and zeny. The player sees each
// change as it is made: an item that enters a slot, an item that leaves
// it for the reason given, and the new weight and zeny. The log lines
// wait for the next commit.
func (z *Zone) apply(p *Player, tx *transfer.Tx, reason wathenav1.InventoryRemoveReason) error {
	if z.Clock != nil {
		tx.Now = z.Clock()
	}
	logs, err := tx.Apply(func(c transfer.Change) {
		switch {
		case c.Wallet != nil:
			if s := z.battleStats(p); s != nil {
				p.pending = append(p.pending, s)
			}
		case c.Amount > 0:
			z.sendAdd(p, c.Slot, c.Amount)
		default:
			p.pending = append(p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
				Slot:   uint32(c.Slot),    // #nosec G115 -- below MaxInventory.
				Amount: uint32(-c.Amount), // #nosec G115 -- at most MaxAmount.
				Reason: reason,
			}.Build()}.Build())
			z.updateWeight(p)
		}
	})
	if err != nil {
		return err
	}
	z.recordTransfer(p, logs)
	return nil
}

// sendAdd tells a player an amount entered a slot, as clif_additem, and
// sends the new weight.
func (z *Zone) sendAdd(p *Player, slot, amount int) {
	s := &p.inventory.Slots[slot]
	d := z.Items.Get(s.ID)
	rolls, cardRolls, opts := z.rollMessages(d, s)
	p.pending = append(p.pending, wathenav1.Event_builder{InventoryAdd: wathenav1.InventoryAdd_builder{
		ItemId:    s.ID,
		Amount:    uint32(amount),           // #nosec G115 -- at most MaxAmount.
		Slot:      uint32(slot),             // #nosec G115 -- below MaxInventory.
		Refine:    uint32(max(s.Refine, 0)), // #nosec G115 -- at most the refine limit.
		Cards:     s.Cards,
		Location:  wornLocation(d),
		Card:      d.IsCard(),
		Rolls:     rolls,
		CardRolls: cardRolls,
		Options:   opts,
	}.Build()}.Build())
	z.updateWeight(p)
}

// recordTransfer keeps a transfer's log lines and the player it changed
// for the next commit.
func (z *Zone) recordTransfer(p *Player, logs transfer.Logs) {
	if logs.Empty() {
		return
	}
	z.txLogs.Add(logs)
	if !p.transferred {
		p.transferred = true
		z.txPlayers = append(z.txPlayers, p)
	}
}

// commitTransfers hands the transfers since the last commit to the
// Committer: a copy of each character they changed and their log lines.
// Without a Committer, the lines go to the log.
func (z *Zone) commitTransfers() {
	if len(z.txPlayers) == 0 {
		return
	}
	logs, players := z.txLogs, z.txPlayers
	z.txLogs, z.txPlayers = transfer.Logs{}, nil
	var saves []CommitSave
	for _, p := range players {
		p.transferred = false
		if s := p.sess; s != nil && s.saver != nil && z.Committer != nil {
			c := z.snapshot(p)
			p.saved = c
			saves = append(saves, CommitSave{Saver: s.saver, Character: c})
		}
	}
	if z.Committer != nil {
		z.Committer.Commit(saves, logs)
		return
	}
	z.logTransfers(logs)
}

// logTransfers writes log lines to the mods' log at debug level, for a
// zone without a Committer.
func (z *Zone) logTransfers(logs transfer.Logs) {
	if z.Mods == nil || z.Mods.Log == nil {
		return
	}
	for _, l := range logs.Items {
		z.Mods.Log.Debug("item transfer", "map", z.Map, "action", l.Action, "from", l.From.Kind.String(), "from_id", l.From.ID,
			"to", l.To.Kind.String(), "to_id", l.To.ID, "item", l.ItemID, "amount", l.Amount, "serial", l.Serial.String(),
			"source", l.Source.String(), "rolls", l.Rolls.Key())
	}
	for _, l := range logs.Zeny {
		z.Mods.Log.Debug("zeny transfer", "map", z.Map, "action", l.Action, "from", l.From.Kind.String(), "from_id", l.From.ID,
			"to", l.To.Kind.String(), "to_id", l.To.ID, "amount", l.Amount)
	}
}

// payZeny takes zeny from a player to an end. It returns false, and
// takes nothing, when the player has too little.
func (z *Zone) payZeny(p *Player, amount int64, a transfer.Action, to transfer.Owner) bool {
	if amount <= 0 {
		return amount == 0
	}
	return z.apply(p, transfer.New(a).Pay(z.wallet(p), amount, to), 0) == nil
}

// earnZeny gives a player zeny from an end. Zeny over MaxZeny is lost,
// as pc_getzeny.
func (z *Zone) earnZeny(p *Player, amount int64, a transfer.Action, from transfer.Owner) {
	if amount <= 0 {
		return
	}
	_ = z.apply(p, transfer.New(a).Earn(from, amount, z.wallet(p), true), 0)
}

// Holding is what one player holds, for Audit.
type Holding struct {
	Name  string
	Owner transfer.Owner
	Zeny  int64
	Items []character.Item
}

// GroundHolding is one ground item, for Audit.
type GroundHolding struct {
	ItemID uint32
	Amount int
	// Origin is the serial of the stack the item was dropped from, or
	// zero for a monster's drop.
	Origin item.Serial
}

// Audit is every item and all zeny in a zone at one moment, for the
// duplication tests (decision 0017). Items on their way to the ground,
// such as a dead monster's drops, are not in it.
type Audit struct {
	Map     string
	Players []Holding
	Ground  []GroundHolding
}

// Audit lists what the zone's players and ground hold. It runs on the
// zone's goroutine.
func (z *Zone) Audit(ctx context.Context) (Audit, error) {
	done := make(chan Audit, 1)
	f := func() {
		a := Audit{Map: z.Map}
		for _, p := range z.players {
			if p.inventory == nil {
				continue
			}
			a.Players = append(a.Players, Holding{Name: p.Name, Owner: p.owner(), Zeny: p.record.Zeny, Items: savedItems(p.inventory)})
		}
		for e := range z.entities.all() {
			if g := e.item; g != nil {
				a.Ground = append(a.Ground, GroundHolding{ItemID: g.id, Amount: g.amount, Origin: g.origin})
			}
		}
		done <- a
	}
	select {
	case z.control <- f:
	case <-ctx.Done():
		return Audit{}, ctx.Err()
	case <-z.stopped:
		return Audit{}, ErrStopped
	}
	select {
	case a := <-done:
		return a, nil
	case <-ctx.Done():
		return Audit{}, ctx.Err()
	}
}
