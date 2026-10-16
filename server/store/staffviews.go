package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The staff tools' reads on the game role (decision 0029). They start
// no session: an offline character's view reads what the database
// holds, and a restore that changes it loads the character under a Hold.

// StorageView is the stacks of one storage or cart.
type StorageView struct {
	Key   storage.Key
	Items []character.Item
}

// OfflineView is a character as the database holds it.
type OfflineView struct {
	Character    *character.Character
	Party, Guild string
	// Storages are the account's storages and the character's cart and
	// extra storages that hold items.
	Storages []StorageView
	Mails    []mail.Row
	// AccountCurrencies are the account's currencies.
	AccountCurrencies map[uint32]int64
}

// ReadCharacter reads a character for the staff tools without starting
// a session. It returns ErrNotFound for a character that does not
// exist.
func (s *Store) ReadCharacter(ctx context.Context, id int64) (*OfflineView, error) {
	row, err := s.q.ReadCharacter(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	items, err := s.q.Inventory(ctx, id)
	if err != nil {
		return nil, err
	}
	c, err := fromRow(&row, items)
	if err != nil {
		return nil, err
	}
	if err := loadCurrencies(ctx, s.q, c); err != nil {
		return nil, err
	}
	if err := loadQuests(ctx, s.q, c); err != nil {
		return nil, err
	}
	if c.Sanctions, err = s.activeSanctions(ctx, id); err != nil {
		return nil, err
	}
	v := &OfflineView{Character: c}
	groups, err := s.q.CharacterGroups(ctx, id)
	if err != nil {
		return nil, err
	}
	v.Party, v.Guild = groups.Party, groups.Guild
	rows, err := s.q.StorageItemsOf(ctx, db.StorageItemsOfParams{AccountID: pgtype.Int8{Int64: c.AccountID, Valid: true},
		CharacterID: pgtype.Int8{Int64: id, Valid: true}})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		holder := storage.HolderAccount
		owner := c.AccountID
		switch r.Holder {
		case "cart":
			holder, owner = storage.HolderCart, id
		case "character":
			holder, owner = storage.HolderCharacter, id
		}
		k := storage.Key{Holder: holder, ID: owner, Storage: uint32(r.Storage)} // #nosec G115 -- record IDs are small.
		it, err := inventoryItem(&db.InventoryRow{Slot: r.Slot, ItemID: r.ItemID, Amount: r.Amount, Serial: r.Serial, Refine: r.Refine,
			Cards: r.Cards, Rolls: r.Rolls, CardRolls: r.CardRolls, RandomOptions: r.RandomOptions, Identified: r.Identified, Bound: r.Bound})
		if err != nil {
			return nil, err
		}
		if n := len(v.Storages); n == 0 || v.Storages[n-1].Key != k {
			v.Storages = append(v.Storages, StorageView{Key: k})
		}
		last := &v.Storages[len(v.Storages)-1]
		last.Items = append(last.Items, it)
	}
	mails, err := s.q.Mailbox(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.Mails, err = mailRows(ctx, s.q, mails); err != nil {
		return nil, err
	}
	if v.AccountCurrencies, err = s.LoadAccountCurrencies(ctx, c.AccountID); err != nil {
		return nil, err
	}
	return v, nil
}

// activeSanctions reads a character's sanctions in effect.
func (s *Store) activeSanctions(ctx context.Context, id int64) ([]character.Sanction, error) {
	rows, err := s.q.SanctionsOf(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	out := make([]character.Sanction, 0, len(rows))
	for _, r := range rows {
		out = append(out, character.Sanction{Kind: r.Kind, Reason: r.Reason, Start: r.StartedAt.Time, End: r.EndsAt.Time, By: r.IssuedBy})
	}
	return out, nil
}

// LastCopy reads the newest log line of a serial as a copy, for a
// restore that makes a gone serial again: the item, its refine, cards
// and extra, and the amount of the line. found is false for a serial
// with no line.
func (s *Store) LastCopy(ctx context.Context, serial item.Serial) (copy item.Slot, amount int, found bool, err error) {
	r, err := s.q.LastItemLine(ctx, pgtype.UUID{Bytes: serial, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return item.Slot{}, 0, false, nil
	}
	if err != nil {
		return item.Slot{}, 0, false, err
	}
	it, err := inventoryItem(&db.InventoryRow{ItemID: r.ItemID, Amount: r.Amount, Serial: pgtype.UUID{Bytes: serial, Valid: true},
		Refine: r.Refine, Cards: r.Cards, Rolls: r.Rolls, CardRolls: r.CardRolls, RandomOptions: r.RandomOptions,
		Identified: r.Identified, Bound: r.Bound})
	if err != nil {
		return item.Slot{}, 0, false, err
	}
	return item.Slot{ID: it.ID, Refine: it.Refine, Cards: it.Cards, Extra: it.Extra}, it.Amount, true, nil
}

// Holder is a saved place that holds a serial.
type Holder struct {
	// Holder is inventory, account, character, cart, mail or
	// guild_storage, as the tables name it.
	Holder string
	// ID is the character's, the account's, the mail's or the guild's.
	ID      int64
	Storage uint32
	Slot    int
	ItemID  uint32
	Amount  int
}

// ItemHolders lists the saved places that hold a serial.
func (s *Store) ItemHolders(ctx context.Context, serial item.Serial) ([]Holder, error) {
	rows, err := s.q.ItemHolders(ctx, pgtype.UUID{Bytes: serial, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]Holder, 0, len(rows))
	for _, r := range rows {
		out = append(out, Holder{Holder: r.Holder, ID: r.ID, Storage: uint32(r.Storage), Slot: r.Slot, // #nosec G115 -- record IDs are small.
			ItemID: uint32(r.ItemID), Amount: r.Amount}) // #nosec G115 -- item IDs are above 0.
	}
	return out, nil
}

// Queries are the store's queries, for the staff tools' reads of the
// logs and names.
func (s *Store) Queries() *db.Queries { return s.q }
