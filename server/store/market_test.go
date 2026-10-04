package store_test

import (
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/market"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// TestMarketCommits writes, in commits with the characters: Alice's
// stack into a mail to Bob with her own and her account's currencies,
// a shop's stock, and Alice's vending store. It reads each back, finds
// Bob as a recipient, and finds the mail once it expired.
func TestMarketCommits(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{RetryDelay: time.Second})
	ids := make([]int64, 2)
	chars := make([]*character.Character, 2)
	sessions := make([]*store.Session, 2)
	for i, name := range []string{"Alice", "Bob"} {
		id, err := s.DevCharacter(ctx, newCharacter(name))
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		if chars[i], sessions[i], err = s.Load(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	floor, err := s.MailIDFloor(ctx)
	if err != nil || floor != 0 {
		t.Fatalf("floor %d, %v", floor, err)
	}
	stack := chars[0].Items[0]
	alice := copyOf(chars[0])
	alice.Items = alice.Items[1:]
	alice.Currencies = map[uint32]int64{10000: 7}
	alice.MailDay, alice.MailSent = 20261004, 1
	sent := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	row := &mail.Row{ID: 1, From: mail.Key{ID: ids[0]}, FromName: "Alice", To: mail.Key{ID: ids[1]}, ToName: "Bob", Title: "Hi",
		Body: "Text", Zeny: 100, Sent: sent,
		Items: []character.Item{{Slot: 0, ID: stack.ID, Amount: stack.Amount, Serial: stack.Serial}}}
	acc := storage.Key{Holder: storage.HolderAccount, ID: chars[0].AccountID}
	line := transfer.ItemLog{Action: transfer.ActionMail, From: transfer.Inventory(ids[0]), To: transfer.Owner{Kind: transfer.KindMail, ID: 1},
		Serial: stack.Serial, ItemID: stack.ID, Amount: stack.Amount}
	cash := transfer.ZenyLog{Action: transfer.ActionBuy, From: acc.Owner(), To: transfer.End(transfer.KindShop), Amount: 5, Currency: 1}
	st := &market.Store{Owner: ids[0], Title: "Cheap", Map: "map/prontera", X: 150, Y: 180,
		Autotrade: true, Lines: []market.Line{{Slot: item.Slot{ID: 909, Amount: 3, Serial: item.NewSerial()}, Price: 10}}}
	s.CommitAll([]store.Entry{{Session: sessions[0], Char: alice}},
		[]store.StorageWrite{{Key: acc, HasCurrencies: true, Currencies: map[uint32]int64{1: 20}}},
		store.Writes{Mails: []mail.Write{{ID: 1, Row: row, To: row.To}},
			Stock:  []shop.StockRow{{Shop: "core:Market", Item: 714, Stock: 3, Filled: sent}},
			Stores: []market.Write{{Key: st.KeyOf(), Store: st}}},
		transfer.Logs{Items: []transfer.ItemLog{line}, Zeny: []transfer.ZenyLog{cash}})
	rows, err := s.LoadMailbox(ctx, ids[1])
	if err != nil || len(rows) != 1 || rows[0].Zeny != 100 || len(rows[0].Items) != 1 || rows[0].Items[0].Serial != stack.Serial ||
		rows[0].From != (mail.Key{ID: ids[0]}) {
		t.Fatalf("mailbox %+v, %v", rows, err)
	}
	again, _, err := s.Load(ctx, ids[0])
	if err != nil || again.Currencies[10000] != 7 || again.MailSent != 1 || len(again.Items) != len(chars[0].Items)-1 {
		t.Fatalf("alice %+v, %v", again, err)
	}
	cur, err := s.LoadAccountCurrencies(ctx, chars[0].AccountID)
	if err != nil || cur[1] != 20 {
		t.Errorf("account currencies %v, %v", cur, err)
	}
	stock, err := s.ShopStock(ctx)
	if err != nil || len(stock) != 1 || stock[0].Stock != 3 || stock[0].Item != 714 {
		t.Errorf("stock %+v, %v", stock, err)
	}
	if rc, ok, err := s.FindRecipient(ctx, "bob", 0); err != nil || !ok || rc.Key.ID != ids[1] || rc.Mails != 1 || rc.Name != "Bob" {
		t.Errorf("recipient %+v %v %v", rc, ok, err)
	}
	if _, ok, err := s.FindRecipient(ctx, "nobody", 0); err != nil || ok {
		t.Errorf("nobody found: %v", err)
	}
	if due, err := s.ExpiredMails(ctx, sent.Add(time.Hour), time.Time{}, 100); err != nil || len(due) != 1 {
		t.Errorf("expired %+v, %v", due, err)
	}
	if due, err := s.ExpiredMails(ctx, sent.Add(-time.Hour), sent.Add(time.Hour), 100); err != nil || len(due) != 0 {
		t.Errorf("not yet expired %+v, %v", due, err)
	}
	stores, err := s.Autotraders(ctx)
	if err != nil || len(stores) != 1 || stores[0].Lines[0].Slot.Serial != st.Lines[0].Slot.Serial || stores[0].Lines[0].Price != 10 {
		t.Fatalf("stores %+v, %v", stores, err)
	}
	// Taking the mail's items and deleting it, and closing the store.
	bob := copyOf(chars[1])
	bob.Items = append(bob.Items, character.Item{Slot: 9, ID: stack.ID, Amount: stack.Amount, Serial: stack.Serial})
	s.CommitAll([]store.Entry{{Session: sessions[1], Char: bob}}, nil,
		store.Writes{Mails: []mail.Write{{ID: 1, To: row.To}}, Stores: []market.Write{{Key: st.KeyOf()}},
			Stock: []shop.StockRow{{Shop: "core:Market", Item: 714, Delete: true}}}, transfer.Logs{})
	if rows, err := s.LoadMailbox(ctx, ids[1]); err != nil || len(rows) != 0 {
		t.Errorf("after delete %+v, %v", rows, err)
	}
	if stores, err := s.Autotraders(ctx); err != nil || len(stores) != 0 {
		t.Errorf("stores after close %+v, %v", stores, err)
	}
	if stock, err := s.ShopStock(ctx); err != nil || len(stock) != 0 {
		t.Errorf("stock after delete %+v, %v", stock, err)
	}
}
