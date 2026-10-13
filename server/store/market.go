package store

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/market"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Currencies, shop stock, mail and stores (decision 0024):
//   - A character's own currencies save with the character, and an
//     account's with its storage's commits.
//   - Shop stock, mails and store rows are Writes of a commit, saved in
//     the same transaction as the characters and log lines of the
//     transfers that changed them.
//   - LoadMailbox waits until no commit that holds a mail of the
//     recipient waits or runs, as LoadStorage does for storages.

// Writes are the rows of a commit beside characters and storages.
type Writes struct {
	Mails  []mail.Write
	Stock  []shop.StockRow
	Stores []market.Write
	// Events are the units of world events' progress whose rewards the
	// commit saves.
	Events []EventUnit
	// Audit are rows of the GM audit log of the changes in the commit,
	// such as a restore's (decision 0029).
	Audit []AuditRow
}

func (w *Writes) empty() bool {
	return len(w.Mails) == 0 && len(w.Stock) == 0 && len(w.Stores) == 0 && len(w.Events) == 0 && len(w.Audit) == 0
}

// CommitAll queues a commit with characters, storages, other rows and
// log lines. They all join one commit, so one transaction writes them.
func (s *Store) CommitAll(entries []Entry, storages []StorageWrite, w Writes, logs transfer.Logs) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.openCommit()
	c.writes.Mails = append(c.writes.Mails, w.Mails...)
	c.writes.Stock = append(c.writes.Stock, w.Stock...)
	c.writes.Stores = append(c.writes.Stores, w.Stores...)
	c.writes.Events = append(c.writes.Events, w.Events...)
	c.writes.Audit = append(c.writes.Audit, w.Audit...)
	for _, m := range w.Mails {
		if m.To.Saved() {
			s.mailBusy[m.To.ID]++
		}
	}
	s.addCommit(c, entries, storages, logs, !w.empty())
}

// CommitMail queues the writes and log lines of mail that expired.
func (s *Store) CommitMail(writes []mail.Write, logs transfer.Logs) {
	s.CommitAll(nil, nil, Writes{Mails: writes}, logs)
}

// finishWrites drops a written commit's mail counts. s.mu must be held.
func (s *Store) finishWrites(w *Writes) {
	for _, m := range w.Mails {
		if m.To.Saved() {
			if s.mailBusy[m.To.ID]--; s.mailBusy[m.To.ID] <= 0 {
				delete(s.mailBusy, m.To.ID)
			}
		}
	}
}

// writeAll writes a commit's other rows.
func writeAll(ctx context.Context, q *db.Queries, w *Writes) error {
	if err := writeStock(ctx, q, w.Stock); err != nil {
		return err
	}
	if err := writeMails(ctx, q, w.Mails); err != nil {
		return err
	}
	if err := writeEventUnits(ctx, q, w.Events); err != nil {
		return err
	}
	for _, a := range w.Audit {
		if err := writeAudit(ctx, q, a); err != nil {
			return err
		}
	}
	return writeStores(ctx, q, w.Stores)
}

// loadCurrencies reads a character's own currencies.
func loadCurrencies(ctx context.Context, q *db.Queries, c *character.Character) error {
	rows, err := q.CharacterCurrencies(ctx, c.ID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if c.Currencies == nil {
			c.Currencies = map[uint32]int64{}
		}
		c.Currencies[uint32(r.Currency)] = r.Amount // #nosec G115 -- the table holds IDs above 0.
	}
	return nil
}

// LoadAccountCurrencies reads an account's currencies, for its storage.
func (s *Store) LoadAccountCurrencies(ctx context.Context, account int64) (map[uint32]int64, error) {
	rows, err := s.q.AccountCurrencies(ctx, account)
	if err != nil {
		return nil, err
	}
	out := map[uint32]int64{}
	for _, r := range rows {
		out[uint32(r.Currency)] = r.Amount // #nosec G115 -- the table holds IDs above 0.
	}
	return out, nil
}

// currencyDiff is what changed between two sets of balances: the
// currencies to delete and the ones to write.
func currencyDiff(base, cur map[uint32]int64) (del []int, ids []int, amounts []int64) {
	for _, id := range slices.Sorted(maps.Keys(base)) {
		if cur[id] == 0 {
			del = append(del, int(id))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(cur)) {
		if n := cur[id]; n > 0 && base[id] != n {
			ids, amounts = append(ids, int(id)), append(amounts, n)
		}
	}
	return del, ids, amounts
}

// writeCharacterCurrencies writes the currencies that changed.
func writeCharacterCurrencies(ctx context.Context, q *db.Queries, id int64, base, cur map[uint32]int64) error {
	del, ids, amounts := currencyDiff(base, cur)
	if len(del) > 0 {
		if err := q.DeleteCharacterCurrencies(ctx, db.DeleteCharacterCurrenciesParams{CharacterID: id, Currencies: del}); err != nil {
			return err
		}
	}
	if len(ids) > 0 {
		return q.UpsertCharacterCurrencies(ctx, db.UpsertCharacterCurrenciesParams{CharacterID: id, Currencies: ids, Amounts: amounts})
	}
	return nil
}

// writeAccountCurrencies writes an account's currencies that changed.
func writeAccountCurrencies(ctx context.Context, q *db.Queries, w *StorageWrite) error {
	if !w.HasCurrencies || !w.Key.Saved() || !w.Key.HoldsCurrencies() {
		return nil
	}
	del, ids, amounts := currencyDiff(w.CurrencyBase, w.Currencies)
	if len(del) > 0 {
		if err := q.DeleteAccountCurrencies(ctx, db.DeleteAccountCurrenciesParams{AccountID: w.Key.ID, Currencies: del}); err != nil {
			return err
		}
	}
	if len(ids) > 0 {
		return q.UpsertAccountCurrencies(ctx, db.UpsertAccountCurrenciesParams{AccountID: w.Key.ID, Currencies: ids, Amounts: amounts})
	}
	return nil
}

// LoadShopStock reads every saved shop stock.
func LoadShopStock(ctx context.Context, q *db.Queries) ([]shop.StockRow, error) {
	rows, err := q.ShopStock(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]shop.StockRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, shop.StockRow{Shop: r.Shop, Item: uint32(r.ItemID), Stock: r.Stock, Filled: r.FilledAt.Time}) // #nosec G115 -- item IDs above 0.
	}
	return out, nil
}

// ShopStock reads every saved shop stock.
func (s *Store) ShopStock(ctx context.Context) ([]shop.StockRow, error) {
	return LoadShopStock(ctx, s.q)
}

// writeStock writes shop stock rows. A later row of an item replaces an
// earlier one.
func writeStock(ctx context.Context, q *db.Queries, rows []shop.StockRow) error {
	if len(rows) == 0 {
		return nil
	}
	type key struct {
		shop string
		item uint32
	}
	last := map[key]shop.StockRow{}
	var order []key
	for _, r := range rows {
		k := key{r.Shop, r.Item}
		if _, ok := last[k]; !ok {
			order = append(order, k)
		}
		last[k] = r
	}
	var up db.UpsertShopStockParams
	var del db.DeleteShopStockParams
	for _, k := range order {
		r := last[k]
		if r.Delete {
			del.Shops, del.ItemIds = append(del.Shops, r.Shop), append(del.ItemIds, int(r.Item))
			continue
		}
		up.Shops, up.ItemIds = append(up.Shops, r.Shop), append(up.ItemIds, int(r.Item))
		up.Stocks = append(up.Stocks, max(r.Stock, 0))
		up.Filled = append(up.Filled, pgtype.Timestamptz{Time: r.Filled, Valid: true})
	}
	if len(del.Shops) > 0 {
		if err := q.DeleteShopStock(ctx, del); err != nil {
			return err
		}
	}
	if len(up.Shops) > 0 {
		return q.UpsertShopStock(ctx, up)
	}
	return nil
}

// MailIDFloor is the highest saved mail ID, so new mails take IDs above.
func (s *Store) MailIDFloor(ctx context.Context) (int64, error) { return s.q.MailIDFloor(ctx) }

// mailRows reads mails with their items.
func mailRows(ctx context.Context, q *db.Queries, mails []db.Mail) ([]mail.Row, error) {
	if len(mails) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(mails))
	for i, m := range mails {
		ids[i] = m.ID
	}
	items, err := q.MailItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	byMail := map[int64][]character.Item{}
	for _, r := range items {
		row := db.InventoryRow{Slot: r.Slot, ItemID: r.ItemID, Amount: r.Amount, Serial: r.Serial, Refine: r.Refine, Cards: r.Cards,
			Rolls: r.Rolls, CardRolls: r.CardRolls, RandomOptions: r.RandomOptions, Identified: r.Identified, Bound: r.Bound}
		it, err := inventoryItem(&row)
		if err != nil {
			return nil, err
		}
		byMail[r.MailID] = append(byMail[r.MailID], it)
	}
	out := make([]mail.Row, 0, len(mails))
	for _, m := range mails {
		r := mail.Row{ID: m.ID, FromName: m.SenderName, To: mail.Key{ID: m.RecipientID}, ToName: m.RecipientName, Title: m.Title,
			Body: m.Body, Zeny: m.Zeny, Sent: m.SentAt.Time, Read: m.Read, Returned: m.Returned, Items: byMail[m.ID]}
		if m.SenderID.Valid {
			r.From = mail.Key{ID: m.SenderID.Int64}
		}
		out = append(out, r)
	}
	return out, nil
}

// LoadMailbox reads a character's mails. It waits for the commits that
// hold the character's mails.
func (s *Store) LoadMailbox(ctx context.Context, character int64) ([]mail.Row, error) {
	for {
		s.mu.Lock()
		n, ch := s.mailBusy[character], s.changed
		s.mu.Unlock()
		if n == 0 {
			break
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	mails, err := s.q.Mailbox(ctx, character)
	if err != nil {
		return nil, err
	}
	return mailRows(ctx, s.q, mails)
}

// FindRecipient finds a character by name, or by ID when id is above 0.
func (s *Store) FindRecipient(ctx context.Context, name string, id int64) (mail.Recipient, bool, error) {
	var rid int64
	var rname string
	var mails int
	var err error
	if id > 0 {
		var r db.RecipientByIDRow
		r, err = s.q.RecipientByID(ctx, id)
		rid, rname, mails = r.ID, r.Name, r.Mails
	} else {
		var r db.RecipientByNameRow
		r, err = s.q.RecipientByName(ctx, name)
		rid, rname, mails = r.ID, r.Name, r.Mails
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return mail.Recipient{}, false, nil
	}
	if err != nil {
		return mail.Recipient{}, false, err
	}
	return mail.Recipient{Key: mail.Key{ID: rid}, Name: rname, Mails: mails}, true, nil
}

// ExpiredMails finds the mails that are due to return or to be deleted.
func (s *Store) ExpiredMails(ctx context.Context, returnBefore, deleteBefore time.Time, limit int) ([]mail.Row, error) {
	p := db.ExpiredMailsParams{Lim: int32(min(max(limit, 1), 10000))} // #nosec G115 -- bounded.
	if !returnBefore.IsZero() {
		p.ReturnBefore = pgtype.Timestamptz{Time: returnBefore, Valid: true}
	}
	if !deleteBefore.IsZero() {
		p.DeleteBefore = pgtype.Timestamptz{Time: deleteBefore, Valid: true}
	}
	mails, err := s.q.ExpiredMails(ctx, p)
	if err != nil {
		return nil, err
	}
	return mailRows(ctx, s.q, mails)
}

// writeMails writes mail rows. Each write holds the mail whole, so its
// items are written again.
func writeMails(ctx context.Context, q *db.Queries, ws []mail.Write) error {
	var del []int64
	for _, w := range ws {
		if w.Row == nil {
			del = append(del, w.ID)
			continue
		}
		r := w.Row
		if !r.To.Saved() {
			continue
		}
		p := db.UpsertMailParams{ID: r.ID, SenderName: r.FromName, RecipientID: r.To.ID, RecipientName: r.ToName, Title: r.Title,
			Body: r.Body, Zeny: r.Zeny, SentAt: pgtype.Timestamptz{Time: r.Sent, Valid: true}, Read: r.Read, Returned: r.Returned}
		if r.From.Saved() {
			p.SenderID = pgtype.Int8{Int64: r.From.ID, Valid: true}
		}
		if err := q.UpsertMail(ctx, p); err != nil {
			return err
		}
		if err := q.DeleteMailItems(ctx, r.ID); err != nil {
			return err
		}
		if len(r.Items) == 0 {
			continue
		}
		inv := insertParams(0, r.Items)
		if err := q.InsertMailItems(ctx, db.InsertMailItemsParams{MailID: r.ID, Slots: inv.Slots, ItemIds: inv.ItemIds,
			Amounts: inv.Amounts, Serials: inv.Serials, Refines: inv.Refines, Cards: inv.Cards, Rolls: inv.Rolls,
			CardRolls: inv.CardRolls, RandomOptions: inv.RandomOptions, Identified: inv.Identified, Bounds: inv.Bounds}); err != nil {
			return err
		}
	}
	if len(del) > 0 {
		return q.DeleteMails(ctx, del)
	}
	return nil
}

// writeStores writes the rows of vending shops and buying stores.
func writeStores(ctx context.Context, q *db.Queries, ws []market.Write) error {
	for _, w := range ws {
		if w.Key.Owner <= 0 {
			continue
		}
		if w.Store == nil {
			if err := q.DeleteStore(ctx, db.DeleteStoreParams{CharacterID: w.Key.Owner, Buying: w.Key.Buying}); err != nil {
				return err
			}
			continue
		}
		st := w.Store
		if err := q.UpsertStore(ctx, db.UpsertStoreParams{CharacterID: st.Owner, Buying: st.Buying, Title: st.Title,
			Map: st.Map, X: st.X, Y: st.Y, Dir: st.Dir, ZenyLimit: st.ZenyLimit, Autotrade: st.Autotrade}); err != nil {
			return err
		}
		if err := q.DeleteStoreItems(ctx, db.DeleteStoreItemsParams{CharacterID: st.Owner, Buying: st.Buying}); err != nil {
			return err
		}
		p := db.InsertStoreItemsParams{CharacterID: st.Owner, Buying: st.Buying}
		for i, l := range st.Lines {
			if l.Slot.Amount <= 0 || i > 11 {
				continue
			}
			p.Idxs = append(p.Idxs, i)
			p.ItemIds = append(p.ItemIds, int(l.Slot.ID))
			p.CartSerials = append(p.CartSerials, pgtype.UUID{Bytes: l.Slot.Serial, Valid: !st.Buying && !l.Slot.Serial.IsZero()})
			p.Amounts = append(p.Amounts, l.Slot.Amount)
			p.Prices = append(p.Prices, l.Price)
		}
		if len(p.Idxs) > 0 {
			if err := q.InsertStoreItems(ctx, p); err != nil {
				return err
			}
		}
	}
	return nil
}

// Autotraders reads the stores saved on autotrade, with their lines,
// and deletes the other stores, as do_init_vending_autotrade.
func (s *Store) Autotraders(ctx context.Context) ([]*market.Store, error) {
	if err := s.q.DeleteIdleStores(ctx); err != nil {
		return nil, err
	}
	rows, err := s.q.AutotradeStores(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.q.StoreItems(ctx)
	if err != nil {
		return nil, err
	}
	var out []*market.Store
	for _, r := range rows {
		st := &market.Store{Owner: r.CharacterID, Buying: r.Buying, Title: r.Title, Map: r.Map, X: r.X, Y: r.Y, Dir: r.Dir,
			ZenyLimit: r.ZenyLimit, Autotrade: true}
		for _, it := range items {
			if it.CharacterID != r.CharacterID || it.Buying != r.Buying {
				continue
			}
			l := market.Line{Index: it.Idx, Slot: item.Slot{ID: uint32(it.ItemID), Amount: it.Amount}, Price: it.Price} // #nosec G115 -- item IDs above 0.
			if it.CartSerial.Valid {
				l.Slot.Serial = it.CartSerial.Bytes
			}
			st.Lines = append(st.Lines, l)
		}
		if len(st.Lines) > 0 {
			out = append(out, st)
		}
	}
	return out, nil
}

var _ storage.CurrencyLoader = (*Store)(nil)
var _ mail.Loader = (*Store)(nil)
var _ mail.ExpiryLoader = (*Store)(nil)
