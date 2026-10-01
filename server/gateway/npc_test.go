package gateway

import (
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

func TestValidReply(t *testing.T) {
	for _, c := range []struct {
		name  string
		r     *wathenav1.NpcReply
		ok    bool
		clean string
	}{
		{"next", wathenav1.NpcReply_builder{Dialog: 1, Next: new(true)}.Build(), true, ""},
		{"menu", wathenav1.NpcReply_builder{Dialog: 1, Menu: new(uint32(2))}.Build(), true, ""},
		{"no answer", wathenav1.NpcReply_builder{Dialog: 1}.Build(), false, ""},
		{"text", wathenav1.NpcReply_builder{InputText: new(" a\tb\u202e ")}.Build(), true, "a b"},
		{"empty text", wathenav1.NpcReply_builder{InputText: new("")}.Build(), true, ""},
		{"long text", wathenav1.NpcReply_builder{InputText: new(strings.Repeat("x", 256))}.Build(), false, ""},
		{"bad UTF-8", wathenav1.NpcReply_builder{InputText: new("\xff")}.Build(), false, ""},
	} {
		if got := validReply(c.r); got != c.ok {
			t.Errorf("%s: %v, want %v", c.name, got, c.ok)
		}
		if c.ok && c.r.HasInputText() && c.r.GetInputText() != c.clean {
			t.Errorf("%s: cleaned to %q, want %q", c.name, c.r.GetInputText(), c.clean)
		}
	}
}

func TestShopLines(t *testing.T) {
	line := func(id, n uint32) *wathenav1.ShopBuyLine {
		return wathenav1.ShopBuyLine_builder{ItemId: id, Amount: n}.Build()
	}
	if l, ok := buyLines(wathenav1.ShopBuy_builder{Items: []*wathenav1.ShopBuyLine{line(501, 3)}}.Build()); !ok || len(l) != 1 || l[0].Amount != 3 {
		t.Errorf("a good purchase: %v %v", l, ok)
	}
	for name, items := range map[string][]*wathenav1.ShopBuyLine{
		"no lines":     nil,
		"amount 0":     {line(501, 0)},
		"amount 30001": {line(501, 30001)},
		"item 0":       {line(0, 1)},
		"101 lines":    make([]*wathenav1.ShopBuyLine, 101),
	} {
		if name == "101 lines" {
			for i := range items {
				items[i] = line(501, 1)
			}
		}
		if _, ok := buyLines(wathenav1.ShopBuy_builder{Items: items}.Build()); ok {
			t.Errorf("%s was accepted", name)
		}
	}
	sl := func(slot, n uint32) *wathenav1.ShopSellLine {
		return wathenav1.ShopSellLine_builder{Slot: slot, Amount: n}.Build()
	}
	if _, ok := sellLines(wathenav1.ShopSell_builder{Items: []*wathenav1.ShopSellLine{sl(200, 1)}}.Build()); ok {
		t.Error("slot 200 was accepted")
	}
	if l, ok := sellLines(wathenav1.ShopSell_builder{Items: []*wathenav1.ShopSellLine{sl(199, 2)}}.Build()); !ok || l[0].Slot != 199 {
		t.Errorf("a good sale: %v %v", l, ok)
	}
}
