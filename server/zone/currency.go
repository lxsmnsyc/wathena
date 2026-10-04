package zone

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/currency"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Currencies (proposal 0002, review decision 30): the records of
// data/currencies.yml, such as rAthena's cash and Kafra points.
//   - A character's own currencies are in its record, and save with it.
//   - An account's currencies are with the account's storage
//     (server/storage), which the account's characters share, and save
//     with its commits. A move holds the storage's lock.
//   - Every change goes through server/transfer, so it is logged with
//     the currency's ID, and joins the next commit.
//   - The player gets CurrencyUpdate at login and after each change.

// currencies is the currency table: the mods', or core's.
func (z *Zone) currencies() *currency.Table {
	if z.Mods != nil && z.Mods.Currencies != nil {
		return z.Mods.Currencies
	}
	if z.coreCurrencies == nil {
		z.coreCurrencies = currency.Core()
	}
	return z.coreCurrencies
}

// accountStorage is the player's account storage, which holds the
// account's currencies, or nil.
func (z *Zone) accountStorage(p *Player) *storage.Storage {
	for _, st := range z.storagesOf(p) {
		if st.Key().HoldsCurrencies() {
			return st
		}
	}
	return nil
}

// currencyWallet is a player's balance of a currency as a transfer
// wallet, with the lock to hold while a transfer runs. unlock is a no-op
// for a character's own currency. It is nil when the account's storage
// is missing.
func (z *Zone) currencyWallet(p *Player, r *currency.Record) (w *transfer.CurrencyWallet, lock, unlock func()) {
	noop := func() {}
	if !r.Account {
		if p.currencies == nil {
			p.currencies = map[uint32]int64{}
		}
		return &transfer.CurrencyWallet{Who: p.owner(), ID: r.ID, Max: r.Max, Balances: p.currencies}, noop, noop
	}
	st := z.accountStorage(p)
	if st == nil {
		return nil, noop, noop
	}
	return &transfer.CurrencyWallet{Who: st.Key().Owner(), ID: r.ID, Max: r.Max, Balances: st.CurrenciesUnlocked()}, st.Lock, st.Unlock
}

// balance is a player's amount of a currency.
func (z *Zone) balance(p *Player, r *currency.Record) int64 {
	w, lock, unlock := z.currencyWallet(p, r)
	if w == nil {
		return 0
	}
	lock()
	defer unlock()
	return w.Zeny()
}

// currencyChanged records that a transfer changed a currency of the
// player: the account's storage joins the next commit, and the player
// gets its balances.
func (z *Zone) currencyChanged(p *Player, w *transfer.CurrencyWallet) {
	if r := z.currencies().ByID(w.ID); r != nil && r.Account {
		if st := z.accountStorage(p); st != nil {
			z.personalChanged(st)
		}
	}
	z.sendCurrencies(p)
}

// sendCurrenciesHeld sends the player its currencies when it holds one.
func (z *Zone) sendCurrenciesHeld(p *Player) {
	for _, r := range z.currencies().All() {
		if z.balance(p, r) > 0 {
			z.sendCurrencies(p)
			return
		}
	}
}

// sendCurrencies sends the player its currencies above 0.
func (z *Zone) sendCurrencies(p *Player) {
	var list []*wathenav1.CurrencyAmount
	for _, r := range z.currencies().All() {
		if n := z.balance(p, r); n > 0 {
			list = append(list, wathenav1.CurrencyAmount_builder{Name: r.Name, DisplayName: r.DisplayName,
				Amount: uint64(n)}.Build()) // #nosec G115 -- at most MaxAmount.
		}
	}
	p.pending = append(p.pending, wathenav1.Event_builder{Currencies: wathenav1.CurrencyUpdate_builder{Currencies: list}.Build()}.Build())
}

// applyWithCurrencies runs a transfer of one player's currencies and zeny and
// inventory, holding the account storage's lock when an account currency
// moves. The player sees each change.
func (z *Zone) applyWithCurrencies(p *Player, tx *transfer.Tx, wallets []*transfer.CurrencyWallet,
	reason wathenav1.InventoryRemoveReason) error {
	var locked []*storage.Storage
	for _, w := range wallets {
		if r := z.currencies().ByID(w.ID); r != nil && r.Account {
			if st := z.accountStorage(p); st != nil && !slices.Contains(locked, st) {
				locked = append(locked, st)
			}
		}
	}
	for _, st := range locked {
		st.Lock()
	}
	err := z.apply(p, tx, reason)
	for _, st := range locked {
		st.Unlock()
	}
	if err != nil {
		return err
	}
	for _, w := range wallets {
		z.currencyChanged(p, w)
	}
	return nil
}

// errNoCurrency is a currency name that names no record.
var errNoCurrency = errors.New("there is no such currency in data/currencies.yml")

// currencyRecord finds a currency by the name a mod gave.
func (z *Zone) currencyRecord(name, mod string) (*currency.Record, error) {
	r := z.currencies().Resolve(name, mod)
	if r == nil {
		return nil, fmt.Errorf("currency %q: %w", name, errNoCurrency)
	}
	return r, nil
}

// changeCurrency gives (amount above 0) or takes (below 0) a currency,
// for a script. It returns false, and changes nothing, when the player
// has too little, or the account's storage is missing. A give over the
// record's max fills it to the max, as pc_getcash.
func (z *Zone) changeCurrency(p *Player, r *currency.Record, amount int64) bool {
	w, _, _ := z.currencyWallet(p, r)
	if w == nil {
		return false
	}
	tx := transfer.New(transfer.ActionGive)
	if amount > 0 {
		tx.Earn(transfer.End(transfer.KindScript), amount, w, true)
	} else {
		tx = transfer.New(transfer.ActionTake).Pay(w, -amount, transfer.End(transfer.KindScript))
	}
	return z.applyWithCurrencies(p, tx, []*transfer.CurrencyWallet{w}, 0) == nil
}

// savedCurrencies are the character's own currencies, for a save.
func savedCurrencies(p *Player) map[uint32]int64 {
	if len(p.currencies) == 0 {
		return nil
	}
	return maps.Clone(p.currencies)
}
