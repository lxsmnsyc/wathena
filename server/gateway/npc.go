package gateway

import (
	"unicode/utf8"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// MaxShopLines is the most lines a ShopBuy or ShopSell may have.
const MaxShopLines = 100

// MaxReplyText is the longest NpcReply.input_text, in characters.
const MaxReplyText = 255

// validReply checks an NpcReply before the zone sees it: it has an
// answer, and text input is clean, as chat is. The zone checks the
// answer against the step it waits for.
func validReply(r *wathenav1.NpcReply) bool {
	switch {
	case r.HasNext(), r.HasMenu(), r.HasInputNumber(), r.HasClose():
		return true
	case r.HasInputText():
		t := r.GetInputText()
		if !utf8.ValidString(t) || utf8.RuneCountInString(t) > MaxReplyText {
			return false
		}
		clean, ok := CleanChat(t)
		if !ok && t != "" {
			return false
		}
		// The zone reads the cleaned text.
		r.SetInputText(clean)
		return true
	}
	return false
}

// buyLines reads a ShopBuy's lines: 1 to MaxShopLines, each with an
// amount from 1 to 30000.
func buyLines(b *wathenav1.ShopBuy) ([]zone.BuyLine, bool) {
	items := b.GetItems()
	if len(items) == 0 || len(items) > MaxShopLines {
		return nil, false
	}
	out := make([]zone.BuyLine, 0, len(items))
	for _, it := range items {
		if it.GetItemId() == 0 || it.GetAmount() < 1 || it.GetAmount() > item.MaxAmount {
			return nil, false
		}
		out = append(out, zone.BuyLine{Item: it.GetItemId(), Amount: int(it.GetAmount())})
	}
	return out, true
}

// sellLines reads a ShopSell's lines: 1 to MaxShopLines, each with a
// slot below 200 and an amount from 1 to 30000.
func sellLines(b *wathenav1.ShopSell) ([]zone.SellLine, bool) {
	items := b.GetItems()
	if len(items) == 0 || len(items) > MaxShopLines {
		return nil, false
	}
	out := make([]zone.SellLine, 0, len(items))
	for _, it := range items {
		if it.GetSlot() >= item.MaxInventory || it.GetAmount() < 1 || it.GetAmount() > item.MaxAmount {
			return nil, false
		}
		out = append(out, zone.SellLine{Slot: int(it.GetSlot()), Amount: int(it.GetAmount())})
	}
	return out, true
}
