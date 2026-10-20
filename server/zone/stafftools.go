package zone

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// The staff tools' work in a zone (proposal 0002 review decision 34,
// decision 0029):
//   - StaffView and StaffPositions read online players on the zone's
//     goroutine, for the admin API's player details and search.
//   - Restore moves serials back from their holders, and makes copies
//     and zeny, into one new mail for a character, in one transfer. Its
//     commit holds the holders, the mails, the log lines and the audit
//     row, so one transaction writes them.
//   - A holder online in this zone is its player's inventory or cart.
//     Storages, guild storages and mails are shared objects with locks.
//     Offline characters come loaded under a store hold, with their
//     storages and mailboxes acquired, from the caller.
//   - A storage or a guild storage that someone has open is busy: the
//     player's own moves may wait for the next commit. The caller tries
//     again later.

// StaffStorage is a storage or a cart, for the staff tools.
type StaffStorage struct {
	Key   storage.Key
	Items []character.Item
}

// StaffView is what the staff tools see of an online character.
type StaffView struct {
	Character    *character.Character
	Map          string
	X, Y         int
	Party, Guild string
	Storages     []StaffStorage
	// Mails are the mailbox's mails, oldest first.
	Mails []mail.Row
	// AccountCurrencies are the account's currencies, held by its
	// storage.
	AccountCurrencies map[uint32]int64
}

// StaffPosition is where an online character is.
type StaffPosition struct {
	Map  string
	X, Y int
}

// playerByID finds a player of the zone by character ID.
func (z *Zone) playerByID(id int64) *Player {
	if id <= 0 {
		return nil
	}
	for _, p := range z.players {
		if p.record.ID == id && p.entity != nil && p.inventory != nil {
			return p
		}
	}
	return nil
}

// StaffView reads an online character of this zone. found is false when
// the character is not here.
func (z *Zone) StaffView(ctx context.Context, id int64) (v StaffView, found bool, err error) {
	err = z.do(ctx, func() error {
		p := z.playerByID(id)
		if p == nil {
			return nil
		}
		found = true
		v.Character = z.snapshot(p)
		v.Map, v.X, v.Y = z.Map, p.entity.x, p.entity.y
		if pv := p.partyState.view; pv != nil {
			v.Party = pv.Name
		}
		if gv := p.guild.view; gv != nil {
			v.Guild = gv.Name
		}
		for _, st := range z.storagesOf(p) {
			st.Lock()
			v.Storages = append(v.Storages, StaffStorage{Key: st.Key(), Items: st.Slots()})
			if st.Key().HoldsCurrencies() {
				v.AccountCurrencies = maps.Clone(st.CurrenciesUnlocked())
			}
			st.Unlock()
		}
		if b := z.mailboxOf(p); b != nil {
			for _, m := range b.Mails() {
				m.Lock()
				if !m.Deleted() {
					v.Mails = append(v.Mails, mail.Row{ID: m.ID, From: m.From, To: m.To, FromName: m.FromName, ToName: m.ToName,
						Title: m.Title, Body: m.Body, Zeny: m.Zeny, Sent: m.Sent, Read: m.Read, Returned: m.Returned, Items: m.Items()})
				}
				m.Unlock()
			}
		}
		return nil
	})
	return v, found, err
}

// StaffPositions reads where characters of this zone are, by ID. The
// ones that are not here are left out.
func (z *Zone) StaffPositions(ctx context.Context, ids []int64) (map[int64]StaffPosition, error) {
	out := map[int64]StaffPosition{}
	err := z.do(ctx, func() error {
		for _, id := range ids {
			if p := z.playerByID(id); p != nil {
				out[id] = StaffPosition{Map: z.Map, X: p.entity.x, Y: p.entity.y}
			}
		}
		return nil
	})
	return out, err
}

// HasCharacter reports whether a character is a player of this zone.
func (z *Zone) HasCharacter(ctx context.Context, id int64) (bool, error) {
	var here bool
	err := z.do(ctx, func() error {
		here = z.playerByID(id) != nil
		return nil
	})
	return here, err
}

// Errors of Restore.
var (
	// ErrMoved is a serial that is not where the restore looked, such
	// as one that moved since the database was read. Nothing changed.
	ErrMoved = errors.New("zone: the item is not where the restore looked")
	// ErrBusy is a storage that a player has open. Nothing changed.
	ErrBusy = errors.New("zone: the storage is open")
	// ErrAmount is an amount above what the stack holds. Nothing
	// changed.
	ErrAmount = errors.New("zone: the stack holds less than the amount")
	// ErrFull is a restore whose items do not fit one mail.
	ErrFull = errors.New("zone: the items do not fit the mail")
)

// RestoreHolder is where a restore takes a serial from.
type RestoreHolder struct {
	// Kind is inventory, cart, storage, guild_storage or mail.
	Kind transfer.Kind
	// Character is the character of an inventory or a cart.
	Character int64
	// Storage is the key of a storage or a cart.
	Storage storage.Key
	// Guild is the guild of a guild storage.
	Guild int64
	// Mail is a mail's ID, and MailTo its recipient's mailbox.
	Mail   int64
	MailTo mail.Key
}

// RestoreItem is one serial of a restore.
type RestoreItem struct {
	Serial item.Serial
	// Amount is how many. 0 is the whole stack, or the copy's amount.
	Amount int
	Holder RestoreHolder
	// Copy, when set, is a serial that is gone: the restore makes a copy
	// of it, from the staff end, with Serial as its source.
	Copy *item.Slot
}

// OfflineCharacter is a character the caller loaded under a store hold.
type OfflineCharacter struct {
	Character *character.Character
	Saver     Saver
	inv       *item.Inventory
}

// Restore is one restore (decision 0029).
type Restore struct {
	// To is the character that gets the mail.
	To mail.Recipient
	// Sender, Title and Body are the mail's.
	Sender, Title, Body string
	Items               []RestoreItem
	// Zeny is made for the mail.
	Zeny int64
	// Staff is the staff end the copies and zeny come from.
	Staff transfer.Owner
	// Audit is the restore's audit row. Restore adds the mail and the
	// items to its arguments.
	Audit staff.AuditRow
	// Offline are the characters the caller loaded, by ID. Storages are
	// the storages and carts the caller acquired, and Mailboxes the
	// mailboxes.
	Offline   map[int64]*OfflineCharacter
	Storages  map[storage.Key]*storage.Storage
	Mailboxes map[mail.Key]*mail.Mailbox
}

// RestoredItem is what a restore did with one serial.
type RestoredItem struct {
	Serial, NewSerial item.Serial
	Copy              bool
	ItemID            uint32
	Amount            int
	From              transfer.Owner
}

// RestoreResult is what a restore did.
type RestoreResult struct {
	Mail  int64
	Items []RestoredItem
	Zeny  int64
}

// Restore runs a restore on the zone's goroutine. Its commit is queued
// when it returns. On an error nothing changed.
func (z *Zone) Restore(ctx context.Context, r *Restore) (RestoreResult, error) {
	var res RestoreResult
	err := z.do(ctx, func() error {
		var err error
		res, err = z.restore(r)
		return err
	})
	return res, err
}

// restoreSource is a serial's box, found and locked.
type restoreSource struct {
	it     *RestoreItem
	box    transfer.Box
	slot   int
	amount int
	p      *Player
	off    *OfflineCharacter
	st     *storage.Storage
	gst    *social.GuildStorage
	m      *mail.Mail
}

// lockOrder sorts the shared holders as the zone locks them: guild
// storages, then storages, then carts, then mails.
func lockOrder(h RestoreHolder) (int, string) {
	switch h.Kind {
	case transfer.KindGuildStorage:
		return 0, ""
	case transfer.KindStorage:
		return 1, h.Storage.String()
	case transfer.KindCart:
		return 2, h.Storage.String()
	case transfer.KindMail:
		return 3, ""
	}
	return 4, ""
}

func (z *Zone) restore(r *Restore) (RestoreResult, error) {
	var res RestoreResult
	var unlocks []func()
	defer func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}()
	order := make([]int, len(r.Items))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		ka, sa := lockOrder(r.Items[a].Holder)
		kb, sb := lockOrder(r.Items[b].Holder)
		if c := cmp.Compare(ka, kb); c != 0 {
			return c
		}
		if c := cmp.Compare(sa, sb); c != 0 {
			return c
		}
		ha, hb := r.Items[a].Holder, r.Items[b].Holder
		return cmp.Or(cmp.Compare(ha.Guild, hb.Guild), cmp.Compare(ha.Mail, hb.Mail))
	})
	locked := map[any]bool{}
	// boxes keeps one inventory box per character, so two serials of
	// one inventory are checked together.
	boxes := map[int64]*transfer.InventoryBox{}
	lock := func(l interface{ Lock() }, u func()) {
		if !locked[l] {
			locked[l] = true
			l.Lock()
			unlocks = append(unlocks, u)
		}
	}
	srcs := make([]*restoreSource, len(r.Items))
	for _, i := range order {
		it := &r.Items[i]
		if it.Copy != nil {
			srcs[i] = &restoreSource{it: it, amount: it.Amount}
			continue
		}
		s, err := z.findSource(r, it, lock, boxes)
		if err != nil {
			return res, err
		}
		srcs[i] = s
	}
	reg := z.mailRegistry()
	m := reg.NewMail(max(z.limits().MailItems, len(r.Items)))
	m.Lock()
	unlocks = append(unlocks, m.Unlock)
	m.FromName, m.To, m.ToName = r.Sender, r.To.Key, r.To.Name
	m.Title, m.Body, m.Sent = r.Title, r.Body, z.wallClock()
	tx := transfer.New(transfer.ActionRestore)
	tx.Now = z.wallClock()
	to := slotBox{&m.Box}
	for _, s := range srcs {
		if s.it.Copy != nil {
			tx.Create(r.Staff, *s.it.Copy, s.amount, to, s.it.Serial)
			continue
		}
		tx.Move(s.box, s.slot, s.it.Serial, s.amount, to)
	}
	if r.Zeny > 0 {
		tx.Earn(r.Staff, r.Zeny, &transfer.ZenyWallet{Who: m.Owner(), Z: &m.Zeny}, false)
	}
	// An equipped item that leaves comes off first, as pc_delitem. The
	// sources were checked, so only a full mail can fail the transfer
	// after this.
	for _, s := range srcs {
		if s.p != nil {
			if sl := &s.p.inventory.Slots[s.slot]; sl.Equip != 0 {
				z.unequipItem(s.p, s.slot)
			}
		}
		if s.off != nil {
			s.off.inv.Slots[s.slot].Equip = 0
		}
	}
	logs, err := tx.Apply(func(c transfer.Change) {
		for _, s := range srcs {
			switch {
			case s.p == nil || c.Box != s.box || c.Amount >= 0:
				continue
			case s.st != nil:
				// The owner's cart, which the player sees as a cart move.
				weight := 0
				if b, ok := s.box.(*transfer.ListBox); ok {
					weight = b.Weight
				}
				s.p.pending = append(s.p.pending, wathenav1.Event_builder{StorageRemove: wathenav1.StorageRemove_builder{
					Kind: wathenav1.StorageKind_STORAGE_KIND_CART, Slot: uint32(c.Slot), Amount: uint32(-c.Amount), // #nosec G115 -- a slot and a stack.
					Weight: uint32(max(weight, 0)), // #nosec G115 -- a weight.
				}.Build()}.Build())
			default:
				s.p.pending = append(s.p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
					Slot: uint32(c.Slot), Amount: uint32(-c.Amount), // #nosec G115 -- a slot and a stack.
					Reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TAKEN,
				}.Build()}.Build())
				z.updateWeight(s.p)
			}
			return
		}
	})
	if err != nil {
		var te *transfer.Error
		if errors.As(err, &te) && te.Reason == transfer.ReasonNotOwner {
			return res, ErrMoved
		}
		if errors.As(err, &te) && te.Reason == transfer.ReasonAmount {
			return res, ErrAmount
		}
		return res, ErrFull
	}
	m.Touch()
	// The new serials, in the order of the items.
	res.Mail, res.Zeny = m.ID, r.Zeny
	line := 0
	for _, s := range srcs {
		for line < len(logs.Items) && logs.Items[line].To != m.Owner() {
			line++
		}
		ri := RestoredItem{Serial: s.it.Serial, Copy: s.it.Copy != nil, Amount: s.amount, From: r.Staff}
		if s.box != nil {
			ri.From = s.box.Owner()
		}
		if line < len(logs.Items) {
			ri.NewSerial, ri.ItemID = logs.Items[line].Serial, logs.Items[line].ItemID
			line++
		}
		res.Items = append(res.Items, ri)
	}
	// What the commit saves, read under the locks.
	z.txLogs.Add(logs)
	var cartPlayers, vendPlayers []*Player
	for _, s := range srcs {
		switch {
		case s.p != nil && s.st == nil:
			if !s.p.transferred {
				s.p.transferred = true
				z.txPlayers = append(z.txPlayers, s.p)
			}
			if s.p.trade != nil {
				z.tradeCancel(s.p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
			}
		case s.off != nil && s.st == nil:
			// Commits below save the character with the new inventory.
		}
		if s.p != nil && s.st != nil {
			cartPlayers = append(cartPlayers, s.p)
			if s.p.vend != nil {
				vendPlayers = append(vendPlayers, s.p)
			}
		}
	}
	for _, o := range r.Offline {
		if o.inv == nil {
			continue
		}
		c := *o.Character
		c.Items = savedItems(o.inv)
		o.Character = &c
		z.txOffline = append(z.txOffline, CommitSave{Saver: o.Saver, Character: &c})
	}
	seen := map[any]bool{}
	for _, s := range srcs {
		switch {
		case s.st != nil && !seen[s.st]:
			seen[s.st] = true
			base, cur := s.st.Commit()
			c := StorageCommit{Key: s.st.Key(), Base: base, Cur: cur}
			if s.st.Key().HoldsCurrencies() {
				c.CurrencyBase, c.Currencies = s.st.CommitCurrencies()
				c.HasCurrencies = true
			}
			if s.st.Key().Expandable() {
				c.SizeBase, c.Size = s.st.CommitSize()
			}
			z.txStorages = append(z.txStorages, c)
		case s.gst != nil && !seen[s.gst]:
			seen[s.gst] = true
			base, cur := s.gst.Commit()
			z.txStorages = append(z.txStorages, StorageCommit{Guild: s.gst.Guild(), Base: base, Cur: cur})
		case s.m != nil && !seen[s.m]:
			seen[s.m] = true
			s.m.Touch()
			if w, ok := s.m.Snapshot(); ok {
				z.txMailRows = append(z.txMailRows, w)
			}
		}
	}
	if w, ok := m.Snapshot(); ok {
		z.txMailRows = append(z.txMailRows, w)
	}
	z.mailDeliver = append(z.mailDeliver, m)
	audit := r.Audit
	audit.Arguments = maps.Clone(audit.Arguments)
	if audit.Arguments == nil {
		audit.Arguments = map[string]any{}
	}
	audit.Arguments["mail_id"] = m.ID
	var items []map[string]any
	for _, ri := range res.Items {
		items = append(items, map[string]any{"serial": ri.Serial.String(), "new_serial": ri.NewSerial.String(), "item_id": ri.ItemID,
			"amount": ri.Amount, "copy": ri.Copy, "from": ri.From.Kind.String(), "from_id": ri.From.ID})
	}
	audit.Arguments["items"] = items
	if r.Zeny > 0 {
		audit.Arguments["zeny"] = r.Zeny
	}
	z.txAudit = append(z.txAudit, audit)
	// The locks go before the commit reads the boxes again.
	for i := len(unlocks) - 1; i >= 0; i-- {
		unlocks[i]()
	}
	unlocks = nil
	z.commitTransfers()
	for _, p := range cartPlayers {
		z.syncCartWeight(p)
	}
	for _, p := range vendPlayers {
		z.dropVendLines(p, res.Items)
	}
	return res, nil
}

// slotBox is a mail's box that puts each amount in a slot of its own, so
// a restore's stacks keep their serials and do not merge.
type slotBox struct{ *transfer.ListBox }

// Put puts an amount in the first empty slot.
func (b slotBox) Put(s *item.Slot, amount int) (int, item.AddResult) {
	d := b.Items.Get(s.ID)
	switch {
	case d.ID == 0 || amount <= 0:
		return -1, item.AddInvalid
	case amount > item.MaxAmount:
		return -1, item.AddOverAmount
	}
	i := slices.IndexFunc(b.Slots, func(o item.Slot) bool { return o.ID == 0 })
	if i < 0 {
		if len(b.Slots) >= b.Size {
			return -1, item.AddOverItem
		}
		b.Slots = append(b.Slots, item.Slot{})
		i = len(b.Slots) - 1
	}
	b.Slots[i] = item.Slot{ID: s.ID, Amount: amount, Refine: s.Refine, Cards: slices.Clone(s.Cards), Extra: s.Clone()}
	b.Weight += d.Weight * amount
	return i, item.AddSuccess
}

// Clone copies the box.
func (b slotBox) Clone() transfer.Box { return slotBox{b.ListBox.Clone().(*transfer.ListBox)} }

// findSource finds and locks the box of a serial.
func (z *Zone) findSource(r *Restore, it *RestoreItem, lock func(interface{ Lock() }, func()),
	boxes map[int64]*transfer.InventoryBox) (*restoreSource, error) {
	h := it.Holder
	s := &restoreSource{it: it}
	switch h.Kind {
	case transfer.KindInventory:
		if p := z.playerByID(h.Character); p != nil {
			if boxes[h.Character] == nil {
				boxes[h.Character] = z.box(p)
			}
			s.p, s.box = p, boxes[h.Character]
			break
		}
		o := r.Offline[h.Character]
		if o == nil {
			return nil, ErrMoved
		}
		if o.inv == nil {
			o.inv = restoreInventory(z.Items, o.Character, z.limits())
		}
		s.off = o
		if boxes[h.Character] == nil {
			boxes[h.Character] = &transfer.InventoryBox{Inv: o.inv, Items: z.Items, Who: transfer.Inventory(h.Character)}
		}
		s.box = boxes[h.Character]
	case transfer.KindCart, transfer.KindStorage:
		var st *storage.Storage
		if h.Kind == transfer.KindCart {
			if p := z.playerByID(h.Character); p != nil {
				st, s.p = z.cartOf(p), p
			}
		}
		if st == nil {
			st = r.Storages[h.Storage]
		}
		if st == nil || st.Key() != h.Storage {
			return nil, ErrMoved
		}
		lock(st, st.Unlock)
		if h.Kind == transfer.KindStorage && st.IsOpen() {
			return nil, ErrBusy
		}
		s.st = st
		b := st.Box()
		if h.Kind == transfer.KindCart {
			b = z.prepareCart(st)
		} else {
			b = st.Prepare(z.Items, max(b.Size, len(b.Slots)), b.MaxWeight)
		}
		s.box = b
	case transfer.KindGuildStorage:
		var gst *social.GuildStorage
		if z.Social != nil {
			gst, _ = z.Social.StorageOf(h.Guild)
		}
		if gst == nil {
			return nil, ErrMoved
		}
		lock(gst, gst.Unlock)
		if gst.IsOpen() {
			return nil, ErrBusy
		}
		b := gst.Box()
		b.Items = z.Items
		s.gst, s.box = gst, b
	case transfer.KindMail:
		b := r.Mailboxes[h.MailTo]
		if b == nil {
			b = z.mailRegistry().Loaded(h.MailTo)
		}
		if b == nil {
			return nil, ErrMoved
		}
		m := b.Find(h.Mail)
		if m == nil {
			return nil, ErrMoved
		}
		lock(m, m.Unlock)
		if m.Deleted() {
			return nil, ErrMoved
		}
		m.Box.Items = z.Items
		s.m, s.box = m, &m.Box
	default:
		return nil, ErrMoved
	}
	s.slot = -1
	for i := 0; ; i++ {
		sl := s.box.Slot(i)
		if sl == nil {
			break
		}
		if sl.ID != 0 && sl.Serial == it.Serial {
			s.slot = i
			break
		}
	}
	if s.slot < 0 {
		return nil, ErrMoved
	}
	have := s.box.Slot(s.slot).Amount
	s.amount = it.Amount
	if s.amount == 0 {
		s.amount = have
	}
	if s.amount > have {
		return nil, ErrAmount
	}
	return s, nil
}

// dropVendLines takes the restored stacks out of a player's vending
// shop, and closes the shop when no line is left.
func (z *Zone) dropVendLines(p *Player, restored []RestoredItem) {
	v := p.vend
	if v == nil {
		return
	}
	st := z.cartOf(p)
	changed := false
	v.lines = slices.DeleteFunc(v.lines, func(l vendLine) bool {
		if !slices.ContainsFunc(restored, func(ri RestoredItem) bool { return ri.Serial == l.serial }) {
			return false
		}
		changed = true
		left := 0
		if st != nil {
			st.Lock()
			if s := st.Box().Slot(l.cartSlot); s != nil && s.Serial == l.serial {
				left = s.Amount
			}
			st.Unlock()
		}
		return left == 0
	})
	if !changed {
		return
	}
	if st != nil {
		st.Lock()
		for i := range v.lines {
			if s := st.Box().Slot(v.lines[i].cartSlot); s != nil && s.Serial == v.lines[i].serial {
				v.lines[i].amount = min(v.lines[i].amount, s.Amount)
			}
		}
		st.Unlock()
	}
	if len(v.lines) == 0 {
		z.closeVending(p)
		return
	}
	z.publishStore(p, false)
}
