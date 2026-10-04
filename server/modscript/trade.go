package modscript

import (
	"errors"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Trade (proposal 0002, review decision 29):
//   - The trade event runs after a trade completed, with { players,
//     items, zeny }. Its player option narrows it to trades of one
//     player.
//   - The trade hook runs before a trade completes. It returns true, or
//     false and a message to refuse the trade, and needs rules.override.
//
// players is a list of the two players. items lists each offered item
// as { from, item, amount }, where from is the player who gave it. zeny
// is a list of the zeny each player gave, in the order of players.

// TradeItem is one offered item: the player who gave it, the item ID
// and the amount.
type TradeItem struct {
	From   int64
	Item   uint32
	Amount int
}

// TradeEvent is a trade: its two players, its items and the zeny each
// player gave.
type TradeEvent struct {
	Players [2]int64
	Items   []TradeItem
	Zeny    [2]int64
}

func (r *Runtime) tradeFields(t TradeEvent) []script.Field {
	items := make([]script.Value, 0, len(t.Items))
	for _, it := range t.Items {
		items = append(items, script.NewTable(script.KV("from", script.Int(it.From)), script.KV("item", script.Int(int64(it.Item))),
			script.KV("amount", script.Int(int64(it.Amount)))))
	}
	return []script.Field{
		script.KV("players", script.List(script.Int(t.Players[0]), script.Int(t.Players[1]))),
		script.KV("items", script.List(items...)),
		script.KV("zeny", script.List(script.Int(t.Zeny[0]), script.Int(t.Zeny[1]))),
	}
}

// tradeFilter is the options of a trade handler.
type tradeFilter struct {
	player int64
}

func (f *tradeFilter) match(t TradeEvent) bool {
	return f.player == 0 || f.player == t.Players[0] || f.player == t.Players[1]
}

// readTradeFilter reads the options of events.on("trade", ...). prelude
// turns a player handle into its ID.
func readTradeFilter(v script.Value) (*tradeFilter, error) {
	t, ok := v.AsTable()
	if !ok {
		return nil, errors.New(`events.on("trade"): the options must be a table`)
	}
	f := &tradeFilter{}
	for _, field := range t.Fields {
		k, _ := field.Key.AsString()
		switch k {
		case "player":
			id, ok := field.Value.AsInt()
			if !ok || id <= 0 {
				return nil, errors.New(`events.on("trade"): player must be a player handle`)
			}
			f.player = id
		default:
			return nil, fmt.Errorf(`events.on("trade"): %q is not an option; the option is player`, k)
		}
	}
	return f, nil
}

// FireTrade runs the trade event's handlers.
func (r *Runtime) FireTrade(t TradeEvent) {
	list := r.handlers["trade"]
	if len(list) == 0 {
		return
	}
	obj := script.NewTable(r.tradeFields(t)...)
	for _, h := range slices.Clone(list) {
		if h.removed || h.ctx.ended {
			continue
		}
		if h.trade != nil && !h.trade.match(t) {
			continue
		}
		if err := r.run(h.ctx, r.fns.invoke.Value(), h.fn.Value(), script.String("trade"), obj); err != nil {
			r.failed(h.ctx, "event trade", err)
		}
	}
}

// RunTrade runs the trade hooks before a trade completes. It returns
// false and the hook's message when a hook refused the trade. Without
// hooks, or when the chain fails, the trade goes on.
func (r *Runtime) RunTrade(t TradeEvent) (bool, string) {
	vals, ok := r.runHooks("trade", r.tradeFields(t))
	if !ok || len(vals) == 0 {
		return true, ""
	}
	allowed, isBool := vals[0].AsBool()
	if !isBool || allowed {
		return true, ""
	}
	msg := ""
	if len(vals) > 1 {
		msg, _ = vals[1].AsString()
	}
	if len(msg) > MaxMessage {
		msg = msg[:MaxMessage]
	}
	return false, msg
}
