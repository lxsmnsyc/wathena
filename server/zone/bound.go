package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Bound items that lose their bond (decision 0023), as rAthena with
// BOUND_ITEMS:
//   - A player who leaves a party loses its party-bound items, as the
//     party withdraw handler's pc_delitem with LOG_TYPE_BOUND_REMOVAL.
//   - A player who leaves a guild puts its guild-bound items into the
//     guild storage, as guild_retrieveitembound. An item the storage
//     cannot take stays. A guild that broke up takes them with it.
//   - Either ends the player's trade, as party_trade_bound_cancel and
//     guild_trade_bound_cancel.
// Only the inventory is checked, as pc_bound_chk. A player who is
// offline when it leaves keeps the items: that case needs the login
// check of intif_parse_itembound_ack.

// boundSlots are the inventory slots that hold copies bound as b.
func boundSlots(p *Player, b item.Bound) []int {
	var out []int
	for i := range p.inventory.Slots {
		if s := &p.inventory.Slots[i]; s.ID != 0 && s.Bound == b {
			out = append(out, i)
		}
	}
	return out
}

// partyBoundLeft deletes a player's party-bound items when it left its
// party.
func (z *Zone) partyBoundLeft(p *Player) {
	slots := boundSlots(p, item.BoundParty)
	if len(slots) == 0 {
		return
	}
	z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
	z.removeBound(p, slots)
}

// removeBound deletes whole slots of bound copies in one transfer.
func (z *Zone) removeBound(p *Player, slots []int) {
	tx := transfer.New(transfer.ActionBound)
	box := z.box(p)
	for _, i := range slots {
		s := &p.inventory.Slots[i]
		if s.Equip != 0 {
			z.unequipItem(p, i)
		}
		tx.Destroy(box, i, s.Serial, s.Amount, transfer.End(transfer.KindUsed))
	}
	_ = z.apply(p, tx, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_BOUND)
}

// guildBoundLeft moves a player's guild-bound items into the guild
// storage when it left its guild, or deletes them when the guild broke
// up.
func (z *Zone) guildBoundLeft(p *Player, guild int64, reason string) {
	slots := boundSlots(p, item.BoundGuild)
	if len(slots) == 0 {
		return
	}
	z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
	st, size := z.socialHub().StorageOf(guild)
	if reason == social.ReasonDisbanded || st == nil {
		z.removeBound(p, slots)
		return
	}
	st.Lock()
	b := st.Box()
	b.Items, b.Size = z.Items, size
	for _, i := range slots {
		s := &p.inventory.Slots[i]
		if s.Equip != 0 {
			z.unequipItem(p, i)
		}
		tx := transfer.New(transfer.ActionBound).Move(z.box(p), i, s.Serial, s.Amount, b)
		if z.apply(p, tx, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_BOUND) == nil {
			z.storageChanged(st)
		}
	}
	st.Unlock()
}
