package zone

import (
	"errors"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// restoreTo is a restore to character 8, Other, with an audit row.
func restoreTo(items ...RestoreItem) *Restore {
	return &Restore{To: mail.Recipient{Key: mail.KeyOf(8, "Other"), Name: "Other"}, Sender: "Staff", Title: "Restored items",
		Items: items, Staff: transfer.Owner{Kind: transfer.KindStaff, ID: 99},
		Audit: staff.AuditRow{AccountID: 99, AccountName: "Admin1", Role: "admin", Action: "items.restore", Source: "admin", Target: "Other",
			Reason: "theft", NeedsReview: true}}
}

func inventoryOfChar(id int64) RestoreHolder {
	return RestoreHolder{Kind: transfer.KindInventory, Character: id}
}

// serialHolders counts the places in the zone and its mails that hold a
// serial.
func serialHolders(z *Zone, s item.Serial) int {
	n := 0
	for _, p := range z.players {
		for _, sl := range p.inventory.Slots {
			if sl.ID != 0 && sl.Serial == s {
				n++
			}
		}
	}
	for _, h := range z.mailRegistry().Audit() {
		for _, it := range h.Items {
			if it.Serial == s {
				n++
			}
		}
	}
	for _, m := range z.mailDeliver {
		for _, it := range m.Items() {
			if it.Serial == s {
				n++
			}
		}
	}
	return n
}

// TestRestoreFromOnlineInventory moves a stack and part of another from
// an online player into one mail, and checks the commit: the player's
// save, the mail rows, the log lines and the audit row.
func TestRestoreFromOnlineInventory(t *testing.T) {
	z, a, b, cm := storageZone(t, stack(0, 909, 10), stack(1, 501, 5))
	whole, part := a.inventory.Slots[0].Serial, a.inventory.Slots[1].Serial
	before := len(cm.got)
	res, err := z.restore(restoreTo(RestoreItem{Serial: whole, Holder: inventoryOfChar(7)},
		RestoreItem{Serial: part, Amount: 2, Holder: inventoryOfChar(7)}))
	if err != nil {
		t.Fatal(err)
	}
	if a.inventory.Count(909) != 0 || a.inventory.Count(501) != 3 {
		t.Errorf("inventory after: %d %d", a.inventory.Count(909), a.inventory.Count(501))
	}
	if len(res.Items) != 2 || res.Items[0].NewSerial != whole || res.Items[1].NewSerial == part || res.Items[1].Amount != 2 ||
		res.Items[0].From != transfer.Inventory(7) || res.Mail == 0 {
		t.Errorf("result %+v", res)
	}
	if len(cm.got) != before+1 {
		t.Fatalf("%d commits, want one", len(cm.got)-before)
	}
	c := cm.got[len(cm.got)-1]
	if len(c.saves) != 1 || c.saves[0].Character.ID != 7 || len(c.writes.Mails) != 1 || c.writes.Mails[0].Row == nil ||
		len(c.writes.Mails[0].Row.Items) != 2 || len(c.writes.Audit) != 1 {
		t.Fatalf("commit %+v", c)
	}
	if au := c.writes.Audit[0]; !au.NeedsReview || au.Arguments["mail_id"] != res.Mail || len(au.Arguments["items"].([]map[string]any)) != 2 {
		t.Errorf("audit %+v", au)
	}
	if len(c.logs.Items) != 2 || c.logs.Items[0].Action != transfer.ActionRestore || c.logs.Items[1].Source != part ||
		c.logs.Items[1].To.Kind != transfer.KindMail {
		t.Errorf("logs %+v", c.logs.Items)
	}
	// The player saw the items leave.
	z.stepAt(z.now + 50)
	rm := eventsOf(a, (*wathenav1.Event).GetInventoryRemove, (*wathenav1.Event).HasInventoryRemove)
	if len(rm) != 2 || rm[0].GetReason() != wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TAKEN {
		t.Errorf("removes %v", rm)
	}
	// The mail reaches the recipient's mailbox once the commit is queued.
	if len(z.mailDeliver) != 0 || serialHolders(z, whole) != 1 {
		t.Errorf("delivery: %d waiting, %d holders", len(z.mailDeliver), serialHolders(z, whole))
	}
	_ = b
	// A serial that is not there, or an amount above the stack, changes
	// nothing.
	if _, err := z.restore(restoreTo(RestoreItem{Serial: whole, Holder: inventoryOfChar(7)})); !errors.Is(err, ErrMoved) {
		t.Errorf("moved serial: %v", err)
	}
	if _, err := z.restore(restoreTo(RestoreItem{Serial: part, Amount: 4, Holder: inventoryOfChar(7)})); !errors.Is(err, ErrAmount) {
		t.Errorf("amount: %v", err)
	}
	if _, err := z.restore(restoreTo(RestoreItem{Serial: part, Holder: inventoryOfChar(8)})); !errors.Is(err, ErrMoved) {
		t.Errorf("wrong holder: %v", err)
	}
	if a.inventory.Count(501) != 3 || len(cm.got) != before+1 {
		t.Errorf("a failed restore changed something: %d, %d commits", a.inventory.Count(501), len(cm.got)-before)
	}
}

// TestRestoreDuringTrade restores an item the holder offered in a
// trade. The trade closes, and its confirm cannot give the item again.
func TestRestoreDuringTrade(t *testing.T) {
	z, a, b, _ := storageZone(t, stack(0, 909, 10))
	serial := a.inventory.Slots[0].Serial
	openTrade(t, z, a, b)
	doTrade(z, a, TradeAction{AddSlot: 0, AddAmount: 10})
	doTrade(z, a, TradeAction{Lock: true})
	doTrade(z, b, TradeAction{Lock: true})
	doTrade(z, a, TradeAction{Confirm: true})
	if _, err := z.restore(restoreTo(RestoreItem{Serial: serial, Holder: inventoryOfChar(7)})); err != nil {
		t.Fatal(err)
	}
	if a.trade != nil || b.trade != nil {
		t.Error("the trade is still open")
	}
	doTrade(z, b, TradeAction{Confirm: true})
	if b.inventory.Count(909) != 0 || a.inventory.Count(909) != 0 || serialHolders(z, serial) != 1 {
		t.Errorf("after the confirm: a %d, b %d, holders %d", a.inventory.Count(909), b.inventory.Count(909), serialHolders(z, serial))
	}
}

// TestRestoreEquipped takes a worn sword off before it moves, so the
// player's stats lose its bonuses.
func TestRestoreEquipped(t *testing.T) {
	z, a, _ := equipZone(t)
	a.record.ID = 30
	sword := give(t, z, a, 1101, 1)
	equip(z, a, sword)
	if a.inventory.Slots[sword].Equip == 0 {
		t.Fatal("the sword is not worn")
	}
	serial := a.inventory.Slots[sword].Serial
	if _, err := z.restore(restoreTo(RestoreItem{Serial: serial, Holder: inventoryOfChar(30)})); err != nil {
		t.Fatal(err)
	}
	if len(a.worn) != 0 || a.inventory.Count(1101) != 0 {
		t.Errorf("worn %v, count %d", a.worn, a.inventory.Count(1101))
	}
}

// TestRestoreCopyAndZeny makes a copy of a gone serial and zeny from the
// staff end.
func TestRestoreCopyAndZeny(t *testing.T) {
	z, _, _, cm := storageZone(t)
	gone := item.NewSerial()
	r := restoreTo(RestoreItem{Serial: gone, Amount: 3, Copy: &item.Slot{ID: 909, Refine: 0}})
	r.Zeny = 500
	res, err := z.restore(r)
	if err != nil {
		t.Fatal(err)
	}
	c := cm.got[len(cm.got)-1]
	if len(c.logs.Items) != 1 || c.logs.Items[0].From.Kind != transfer.KindStaff || c.logs.Items[0].From.ID != 99 ||
		c.logs.Items[0].Source != gone || c.logs.Items[0].Serial == gone || !res.Items[0].Copy {
		t.Errorf("copy logs %+v", c.logs.Items)
	}
	if len(c.logs.Zeny) != 1 || c.logs.Zeny[0].Amount != 500 || c.writes.Mails[0].Row.Zeny != 500 {
		t.Errorf("zeny %+v", c.logs.Zeny)
	}
}

// TestRestoreOffline moves an item from an offline character's
// inventory, which the caller loaded, and from a storage the caller
// acquired.
func TestRestoreOffline(t *testing.T) {
	z, _, _, cm := storageZone(t)
	off := savedCharacter()
	off.ID, off.AccountID, off.Name = 20, 9, "Away"
	sv := &saves{}
	serial := off.Items[0].Serial
	reg := storage.NewRegistry(nil)
	key := storage.Key{Holder: storage.HolderAccount, ID: 9}
	sts, err := reg.Acquire(t.Context(), []storage.Key{key})
	if err != nil {
		t.Fatal(err)
	}
	st := sts[0]
	st.Lock()
	b := st.Prepare(z.Items, 100, 0)
	if _, r := b.Put(&item.Slot{ID: 909}, 6); r != item.AddSuccess {
		t.Fatal(r)
	}
	b.Slots[0].Serial = item.NewSerial()
	stored := b.Slots[0].Serial
	st.Unlock()
	r := restoreTo(RestoreItem{Serial: serial, Holder: inventoryOfChar(20)},
		RestoreItem{Serial: stored, Amount: 2, Holder: RestoreHolder{Kind: transfer.KindStorage, Storage: key}})
	r.Offline = map[int64]*OfflineCharacter{20: {Character: off, Saver: sv}}
	r.Storages = map[storage.Key]*storage.Storage{key: st}
	if _, err := z.restore(r); err != nil {
		t.Fatal(err)
	}
	c := cm.got[len(cm.got)-1]
	if len(c.saves) != 1 || c.saves[0].Saver != Saver(sv) || len(c.saves[0].Character.Items) != 1 || r.Offline[20].Character.Items[0].Serial == serial {
		t.Errorf("offline save %+v", c.saves)
	}
	if len(c.storages) != 1 || c.storages[0].Key != key || len(c.storages[0].Cur) != 1 || c.storages[0].Cur[0].Amount != 4 {
		t.Errorf("storage commit %+v", c.storages)
	}
	// An open storage is busy.
	st.Open("someone")
	r = restoreTo(RestoreItem{Serial: stored, Amount: 1, Holder: RestoreHolder{Kind: transfer.KindStorage, Storage: key}})
	r.Storages = map[storage.Key]*storage.Storage{key: st}
	if _, err := z.restore(r); !errors.Is(err, ErrBusy) {
		t.Errorf("open storage: %v", err)
	}
}

// TestRestoreFromVendingCart takes a cart stack that a vending shop
// sells: the shop's line goes, and the shop closes with no line left.
func TestRestoreFromVendingCart(t *testing.T) {
	z, a, _, _ := storageZone(t)
	cart := z.cartOf(a)
	if cart == nil {
		t.Skip("no cart in the test zone")
	}
	cart.Lock()
	b := z.prepareCart(cart)
	if _, r := b.Put(&item.Slot{ID: 909}, 5); r != item.AddSuccess {
		t.Fatal(r)
	}
	b.Slots[0].Serial = item.NewSerial()
	serial := b.Slots[0].Serial
	cart.Unlock()
	a.vend = &vendShop{title: "shop", lines: []vendLine{{cartSlot: 0, serial: serial, amount: 5, price: 10}}}
	holder := RestoreHolder{Kind: transfer.KindCart, Character: 7, Storage: cart.Key()}
	if _, err := z.restore(restoreTo(RestoreItem{Serial: serial, Amount: 2, Holder: holder})); err != nil {
		t.Fatal(err)
	}
	if a.vend == nil || a.vend.lines[0].amount != 3 {
		t.Fatalf("shop after part: %+v", a.vend)
	}
	if _, err := z.restore(restoreTo(RestoreItem{Serial: serial, Holder: holder})); err != nil {
		t.Fatal(err)
	}
	if a.vend != nil {
		t.Errorf("shop still open: %+v", a.vend)
	}
}
