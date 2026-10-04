package store

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Storages and carts (decision 0023): server/storage keeps them in memory
// while a character that uses them is online. The store loads one when
// it enters memory, and writes its slots as a diff in the commits of the
// transfers that changed it.
//   - LoadStorage waits until no commit that holds the storage waits or
//     runs, so a storage that left memory and comes back reads what its
//     last commit wrote.
//   - A commit takes an advisory lock on each account whose storages it
//     writes, in ID order, after the characters' rows. The game role
//     cannot lock account rows.

// LoadStorage reads a storage's slots. It waits for the storage's
// commits that are not written yet.
func (s *Store) LoadStorage(ctx context.Context, k storage.Key) ([]character.Item, error) {
	if err := s.storageIdle(ctx, k); err != nil {
		return nil, err
	}
	var out []character.Item
	add := func(slot, itemID, amount int, serial pgtype.UUID, refine int, cards []int, rolls, cardRolls, opts []byte, identified bool,
		bound int) error {
		row := db.InventoryRow{Slot: slot, ItemID: itemID, Amount: amount, Serial: serial, Refine: refine, Cards: cards, Rolls: rolls,
			CardRolls: cardRolls, RandomOptions: opts, Identified: identified, Bound: bound}
		it, err := inventoryItem(&row)
		if err != nil {
			return err
		}
		out = append(out, it)
		return nil
	}
	if k.Holder == storage.HolderAccount {
		rows, err := s.q.AccountStorage(ctx, db.AccountStorageParams{AccountID: pgtype.Int8{Int64: k.ID, Valid: true}, Storage: int(k.Storage)})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if err := add(r.Slot, r.ItemID, r.Amount, r.Serial, r.Refine, r.Cards, r.Rolls, r.CardRolls, r.RandomOptions, r.Identified, r.Bound); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	rows, err := s.q.CharacterStorage(ctx, db.CharacterStorageParams{Holder: k.Holder.String(), CharacterID: pgtype.Int8{Int64: k.ID, Valid: true},
		Storage: int(k.Storage)})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if err := add(r.Slot, r.ItemID, r.Amount, r.Serial, r.Refine, r.Cards, r.Rolls, r.CardRolls, r.RandomOptions, r.Identified, r.Bound); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// storageIdle waits until no commit of a storage waits or runs.
func (s *Store) storageIdle(ctx context.Context, k storage.Key) error {
	for {
		s.mu.Lock()
		n, ch := s.storageBusy[k], s.changed
		s.mu.Unlock()
		if n == 0 {
			return nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// writePersonalStorages writes the storages and carts of a commit.
func writePersonalStorages(ctx context.Context, q *db.Queries, ws []StorageWrite) error {
	var accounts []int64
	for _, w := range ws {
		if w.Key.Holder == storage.HolderAccount && w.Key.Saved() && !slices.Contains(accounts, w.Key.ID) {
			accounts = append(accounts, w.Key.ID)
		}
	}
	if len(accounts) > 0 {
		slices.Sort(accounts)
		if err := q.LockAccountStorages(ctx, accounts); err != nil {
			return err
		}
	}
	for _, w := range ws {
		k := w.Key
		if k.Holder == 0 || !k.Saved() {
			continue
		}
		del, ins := diffItems(w.Base, w.Cur)
		id := pgtype.Int8{Int64: k.ID, Valid: true}
		if len(del) > 0 {
			var err error
			if k.Holder == storage.HolderAccount {
				err = q.DeleteAccountStorageSlots(ctx, db.DeleteAccountStorageSlotsParams{AccountID: id, Storage: int(k.Storage), Slots: del})
			} else {
				err = q.DeleteCharacterStorageSlots(ctx, db.DeleteCharacterStorageSlotsParams{Holder: k.Holder.String(), CharacterID: id,
					Storage: int(k.Storage), Slots: del})
			}
			if err != nil {
				return err
			}
		}
		if len(ins) > 0 {
			inv := insertParams(0, ins)
			p := db.InsertStorageItemsParams{Holder: k.Holder.String(), Storage: int(k.Storage), Slots: inv.Slots, ItemIds: inv.ItemIds,
				Amounts: inv.Amounts, Serials: inv.Serials, Refines: inv.Refines, Cards: inv.Cards, Rolls: inv.Rolls, CardRolls: inv.CardRolls,
				RandomOptions: inv.RandomOptions, Identified: inv.Identified, Bounds: inv.Bounds}
			if k.Holder == storage.HolderAccount {
				p.AccountID = id
			} else {
				p.CharacterID = id
			}
			if err := q.InsertStorageItems(ctx, p); err != nil {
				return err
			}
		}
	}
	return nil
}
