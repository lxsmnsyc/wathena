package zone

import (
	"context"
	"errors"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Storage and carts (phase 5 step 6, decision 0023), as rAthena's
// storage.cpp and pc.cpp's cart functions:
//   - A player's storages come from the world's storage registry
//     (server/storage): the account's storage, the cart, and the extra
//     storages of data/storages.yml. The session holds them while the
//     player is online, and lets them go after its last commit.
//   - One storage is open at a time, personal or guild, as storage_flag.
//     A script opens it. The cart is open while the player has one.
//   - Every move goes through server/transfer and joins the next commit
//     with the inventory it changed, so the database never holds an item
//     in two places.
//   - Restrictions and bound copies follow itemdb_isrestricted and
//     rAthena's bound checks (item.BoundAllows).

// openStorage is the personal storage a player has open.
type openStorage struct {
	st *storage.Storage
	// rec is the extra storage's record, or nil for the account's.
	rec *storage.Record
}

// storageSize is the slots of a personal storage: an extra storage's
// record's, or the account's storage_slots with its expansion (review
// decision 32). The lock must be held.
func (z *Zone) storageSize(st *storage.Storage, rec *storage.Record) int {
	if rec != nil {
		return rec.Slots
	}
	return z.limitRange("storage_slots").Clamp(st.Size())
}

// cartSize is the slots of a cart: cart_slots with its expansion. The
// lock must be held.
func (z *Zone) cartSize(st *storage.Storage) int {
	return z.limitRange("cart_slots").Clamp(st.Size())
}

// storageRegistry is the world's storage registry, made on first use for
// a zone without one, as in tests.
func (z *Zone) storageRegistry() *storage.Registry {
	if z.Storages == nil {
		z.Storages = storage.NewRegistry(nil)
	}
	return z.Storages
}

// storageRecords are the extra storage records.
func (z *Zone) storageRecords() []*storage.Record {
	if z.Mods == nil {
		return nil
	}
	return z.Mods.Storages
}

// acquireStorages takes a session's storages from the registry. It runs
// on the goroutine that loads the character, before the player joins.
func (z *Zone) acquireStorages(ctx context.Context, s *Session) error {
	if z.Storages == nil || s.player == nil || s.player.load == nil {
		return nil
	}
	sts, err := z.Storages.Acquire(ctx, storage.Keys(&s.player.load.Info, z.storageRecords()))
	if err != nil {
		return err
	}
	s.storages = sts
	return nil
}

// storagesOf are a player's storages: its session's, or for a player that
// a test added, ones taken on first use from a registry without a
// database.
func (z *Zone) storagesOf(p *Player) []*storage.Storage {
	if p.sess != nil && p.sess.storages != nil {
		return p.sess.storages
	}
	if p.ownStorages == nil {
		r := z.storageRegistry()
		if r.Loader != nil {
			return nil
		}
		sts, err := r.Acquire(context.Background(), storage.Keys(&p.record, z.storageRecords()))
		if err != nil {
			return nil
		}
		p.ownStorages = sts
	}
	return p.ownStorages
}

// findStorage is a player's storage: the account's storage for a nil
// record, or the extra storage of rec.
func (z *Zone) findStorage(p *Player, rec *storage.Record) *storage.Storage {
	for _, st := range z.storagesOf(p) {
		k := st.Key()
		switch {
		case rec == nil && k.Holder == storage.HolderAccount && k.Storage == 0:
			return st
		case rec != nil && k.Holder != storage.HolderCart && k.Storage == rec.ID:
			return st
		}
	}
	return nil
}

// cartOf is the player's cart storage.
func (z *Zone) cartOf(p *Player) *storage.Storage {
	for _, st := range z.storagesOf(p) {
		if st.Key().Holder == storage.HolderCart {
			return st
		}
	}
	return nil
}

// storageRecord finds an extra storage record by name. A name without a
// mod prefix is first the record of the calling mod, then core's. A mod
// reaches only its own records and core's (review decision 52). mod is
// "" for the server's own calls, which reach every record.
func (z *Zone) storageRecord(name, mod string) *storage.Record {
	if mod != "" {
		if owner, _, ok := strings.Cut(name, ":"); ok && owner != mod {
			return nil
		}
	}
	recs := z.storageRecords()
	if mod != "" && !strings.Contains(name, ":") {
		for _, r := range recs {
			if r.Name == mod+":"+name {
				return r
			}
		}
	}
	for _, r := range recs {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// cartMaxWeight is cart_weight_max: max_cart_weight.
func (z *Zone) cartMaxWeight() int {
	return rules.ItemsOrDefault(z.worldRules()).MaxCartWeight
}

// prepareCart sets up the cart box for a move or a list. The lock must
// be held.
func (z *Zone) prepareCart(st *storage.Storage) *transfer.ListBox {
	return st.Prepare(z.Items, z.cartSize(st), max(z.cartMaxWeight(), 1))
}

// storageItems lists a box's filled slots as the client shows them.
func (z *Zone) storageItems(b *transfer.ListBox) []*wathenav1.InventoryItem {
	var out []*wathenav1.InventoryItem
	for i := range b.Slots {
		if s := &b.Slots[i]; s.ID != 0 && s.Amount > 0 {
			out = append(out, z.itemMessage(i, s, s.Amount))
		}
	}
	return out
}

// sendStorageList sends a storage's open event and its pages, as
// clif_storagelist and clif_cartlist. The pages are what it holds now:
// moves after this come in later frames.
func (z *Zone) sendStorageList(p *Player, open *wathenav1.StorageOpen, items []*wathenav1.InventoryItem) {
	kind := open.GetKind()
	pages := pageItemsOf(items)
	open.SetItems(pages[0])
	open.SetPages(uint32(len(pages) - 1)) // #nosec G115 -- at most 100 pages.
	events := []*wathenav1.Event{wathenav1.Event_builder{StorageOpen: open}.Build()}
	for i, page := range pages[1:] {
		events = append(events, wathenav1.Event_builder{StorageItems: wathenav1.StorageItems_builder{
			Kind: kind, Items: page, Page: uint32(i + 1), // #nosec G115 -- at most 100 pages.
		}.Build()}.Build())
	}
	p.sendPages(events)
}

// openPersonalStorage is storage_storageopen and
// storage_premiumStorage_load: it opens the account's storage, or an
// extra storage of rec. It returns false when the player has a storage
// open, or another character of the account has this one open.
func (z *Zone) openPersonalStorage(p *Player, rec *storage.Record) bool {
	if p.storage != nil || p.guild.storage != nil {
		// storage_flag is set: rAthena returns without a message.
		return false
	}
	name, display := "", "Storage"
	st := z.findStorage(p, rec)
	if rec != nil {
		name, display = rec.Name, rec.DisplayName
	}
	if st == nil {
		return false
	}
	if !st.Open(p) {
		p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome_STORAGE_OUTCOME_IN_USE))
		return false
	}
	p.storage = &openStorage{st: st, rec: rec}
	st.Lock()
	size := z.storageSize(st, rec)
	box := st.Prepare(z.Items, size, 0)
	items := z.storageItems(box)
	st.Unlock()
	z.sendStorageList(p, wathenav1.StorageOpen_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_PERSONAL,
		Size: uint32(size), Name: name, DisplayName: display}.Build(), items) // #nosec G115 -- at most the storage ceiling.
	return true
}

// closeStorage is storage_storageclose.
func (z *Zone) closeStorage(p *Player) {
	o := p.storage
	if o == nil {
		return
	}
	p.storage = nil
	o.st.Close(p)
	p.pending = append(p.pending, wathenav1.Event_builder{StorageClose: wathenav1.StorageClose_builder{
		Kind: wathenav1.StorageKind_STORAGE_KIND_PERSONAL}.Build()}.Build())
}

// sendCart sends the cart's list, as clif_cartlist, when the player has a
// cart.
func (z *Zone) sendCart(p *Player) {
	st := z.cartOf(p)
	if st == nil || !z.hasCart(p) {
		return
	}
	st.Lock()
	box := z.prepareCart(st)
	items, weight, size := z.storageItems(box), box.Weight, box.Size
	st.Unlock()
	z.sendStorageList(p, wathenav1.StorageOpen_builder{Kind: wathenav1.StorageKind_STORAGE_KIND_CART,
		Size: uint32(size), DisplayName: "Cart", // #nosec G115 -- at most the cart ceiling.
		Weight: uint32(max(weight, 0)), MaxWeight: uint32(max(z.cartMaxWeight(), 0))}.Build(), items) // #nosec G115 -- weights fit.
}

// syncCartWeight sets the battle status's cart weight and limit, which
// Cart Revolution and Cart Termination read, as sd->cart_weight and
// sd->cart_weight_max. They are 0 without a cart.
func (z *Zone) syncCartWeight(p *Player) {
	e := p.entity
	if e == nil || e.status == nil || e.status.Player == nil {
		return
	}
	w := 0
	if st := z.cartOf(p); st != nil && z.hasCart(p) {
		st.Lock()
		w = z.prepareCart(st).Weight
		st.Unlock()
	}
	z.setCartWeight(p, w)
}

// setCartWeight sets the battle status's cart weight, with the limit
// when the player has a cart.
func (z *Zone) setCartWeight(p *Player, w int) {
	e := p.entity
	if e == nil || e.status == nil || e.status.Player == nil {
		return
	}
	m := 0
	if z.hasCart(p) {
		m = z.cartMaxWeight()
	} else {
		w = 0
	}
	if e.status.Player.CartWeight == w && e.status.Player.CartWeightMax == m {
		return
	}
	pl := *e.status.Player
	pl.CartWeight, pl.CartWeightMax = w, m
	e.status.Player = &pl
}

// cartChanged tells the player its cart came or went, as pc_setcart: the
// list when it gets one, a close when it loses it. The items stay in the
// cart.
func (z *Zone) cartChanged(p *Player, had bool) {
	switch has := z.hasCart(p); {
	case has && !had:
		z.sendCart(p)
		z.syncCartWeight(p)
	case !has && had:
		p.pending = append(p.pending, wathenav1.Event_builder{StorageClose: wathenav1.StorageClose_builder{
			Kind: wathenav1.StorageKind_STORAGE_KIND_CART}.Build()}.Build())
	}
}

// end is one side of a storage move: the inventory, a storage or the
// cart.
type end struct {
	// ok is set for an end that exists: the inventory, the open storage,
	// or a cart the player has.
	ok   bool
	kind wathenav1.StorageKind
	// box is the holder. st is the storage behind a personal storage or
	// a cart, and gst a guild storage. Both are nil for the inventory.
	box   transfer.Box
	st    *storage.Storage
	gst   *social.GuildStorage
	place int
	limit item.Restriction
}

func (e *end) inventory() bool { return e.st == nil && e.gst == nil }

// storageMove runs a storage command's move, as storage_storageadd,
// storage_storageget, pc_putitemtocart, pc_getitemfromcart,
// storage_storageaddfromcart and storage_storagegettocart.
func (z *Zone) storageMove(p *Player, m *storageMove) {
	if len(p.pages) > 0 {
		// A list is still on its way: its pages come before any change.
		return
	}
	if p.trade != nil && m.kind != MoveTake && m.kind != MoveCartTake {
		// pc_istrading: clif_parse_MoveToKafra and clif_parse_PutItemToCart
		// do nothing.
		p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome_STORAGE_OUTCOME_BUSY))
		return
	}
	if p.entity.dead && m.kind != MoveCartPut && m.kind != MoveCartTake {
		// A dead player keeps its storage window but moves nothing in or
		// out of it. rAthena has no such check. The window stays open
		// until the player closes it, walks or warps.
		p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome_STORAGE_OUTCOME_BUSY))
		return
	}
	inv := end{ok: true, box: z.box(p)}
	var open, cart end
	switch {
	case p.storage != nil:
		open = end{ok: true, kind: wathenav1.StorageKind_STORAGE_KIND_PERSONAL, st: p.storage.st, place: item.PlaceStorage, limit: item.NoStorage}
	case p.guild.storage != nil:
		open = end{ok: true, kind: wathenav1.StorageKind_STORAGE_KIND_GUILD, gst: p.guild.storage, place: item.PlaceGuildStorage,
			limit: item.NoGuildStorage}
	}
	if st := z.cartOf(p); st != nil && z.hasCart(p) {
		cart = end{ok: true, kind: wathenav1.StorageKind_STORAGE_KIND_CART, st: st, place: item.PlaceCart, limit: item.NoCart}
	}
	var from, to end
	action := transfer.ActionStorage
	usesCart := false
	switch m.kind {
	case MovePut:
		from, to = inv, open
	case MoveTake:
		from, to = open, inv
	case MoveCartPut:
		from, to, action, usesCart = inv, cart, transfer.ActionCart, true
	case MoveCartTake:
		from, to, action, usesCart = cart, inv, transfer.ActionCart, true
	case MoveCartToStorage:
		from, to, usesCart = cart, open, true
	case MoveStorageToCart:
		from, to, usesCart = open, cart, true
	}
	if usesCart && !cart.ok {
		p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome_STORAGE_OUTCOME_NO_CART))
		return
	}
	if !from.ok || !to.ok {
		return
	}
	if from.kind == wathenav1.StorageKind_STORAGE_KIND_GUILD || to.kind == wathenav1.StorageKind_STORAGE_KIND_GUILD {
		action = transfer.ActionMove
	}
	z.moveBetween(p, from, to, m.slot, m.amount, action)
	if usesCart {
		// The cart's lock is let go: syncCartWeight takes it.
		z.syncCartWeight(p)
	}
}

// lockEnds locks the storages of a move, the shared one first, and
// prepares their boxes. It returns false, with nothing locked, when the
// player no longer has an open storage the move names.
func (z *Zone) lockEnds(p *Player, ends ...*end) (unlock func(), ok bool) {
	var held []func()
	unlock = func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i]()
		}
	}
	for _, e := range ends {
		switch {
		case e.gst != nil:
			e.gst.Lock()
			held = append(held, e.gst.Unlock)
			if !e.gst.OpenFor(p.soc) {
				unlock()
				z.closeGuildStorage(p, false)
				return nil, false
			}
			b := e.gst.Box()
			b.Items = z.Items
			e.box = b
		case e.st != nil && e.kind == wathenav1.StorageKind_STORAGE_KIND_CART:
			e.st.Lock()
			held = append(held, e.st.Unlock)
			e.box = z.prepareCart(e.st)
		case e.st != nil:
			e.st.Lock()
			held = append(held, e.st.Unlock)
			if !e.st.OpenFor(p) || p.storage == nil {
				unlock()
				return nil, false
			}
			e.box = e.st.Prepare(z.Items, z.storageSize(e.st, p.storage.rec), 0)
		}
	}
	return unlock, true
}

// moveBetween moves an amount of a slot from one end to another in one
// transfer.
func (z *Zone) moveBetween(p *Player, from, to end, slot, amount int, action transfer.Action) {
	// The cart is private to its character, so the shared storage locks
	// first.
	ends := []*end{&from, &to}
	if from.kind == wathenav1.StorageKind_STORAGE_KIND_CART {
		ends = []*end{&to, &from}
	}
	unlock, ok := z.lockEnds(p, ends...)
	if !ok {
		return
	}
	defer unlock()
	s := from.box.Slot(slot)
	if s == nil || s.ID == 0 || amount < 1 || amount > s.Amount {
		return
	}
	if from.inventory() && s.Equip != 0 {
		// An equipped item stays on: the client does not offer it.
		p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome_STORAGE_OUTCOME_NOT_STORABLE))
		return
	}
	if !to.inventory() {
		if z.Items.Restricted(s, to.limit) {
			// itemdb_canstore, itemdb_canguildstore and
			// itemdb_cancartstore: msg 264.
			p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome_STORAGE_OUTCOME_RESTRICTED))
			return
		}
		if !item.BoundAllows(s.Bound, to.place, false) {
			// msg 294, and msg 264 for the cart.
			p.pending = append(p.pending, storageResult(wathenav1.StorageOutcome_STORAGE_OUTCOME_BOUND))
			return
		}
	}
	tx := transfer.New(action).Move(from.box, slot, s.Serial, amount, to.box)
	if z.Clock != nil {
		tx.Now = z.Clock()
	}
	logs, err := tx.Apply(func(c transfer.Change) { z.storageChange(p, c, &from, &to) })
	if err != nil {
		o := wathenav1.StorageOutcome_STORAGE_OUTCOME_FULL
		var te *transfer.Error
		switch {
		case to.inventory():
			o = wathenav1.StorageOutcome_STORAGE_OUTCOME_CANNOT_CARRY
		case errors.As(err, &te) && te.Reason == transfer.ReasonOverweight:
			o = wathenav1.StorageOutcome_STORAGE_OUTCOME_CART_OVERWEIGHT
		}
		p.pending = append(p.pending, storageResult(o))
		return
	}
	z.recordTransfer(p, logs)
	for _, e := range []*end{&from, &to} {
		switch {
		case e.gst != nil:
			z.storageChanged(e.gst)
			z.logGuildStorage(p, e.gst, logs)
		case e.st != nil:
			z.personalChanged(e.st)
		}
	}
}

// storageChange tells the player of one change of a storage move.
func (z *Zone) storageChange(p *Player, c transfer.Change, from, to *end) {
	e := to
	if c.Box == from.box {
		e = from
	}
	switch {
	case e.inventory() && c.Amount > 0:
		z.sendAdd(p, c.Slot, c.Amount)
	case e.inventory():
		p.pending = append(p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
			Slot: uint32(c.Slot), Amount: uint32(-c.Amount), // #nosec G115 -- below MaxInventory and a stack.
			Reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_STORED,
		}.Build()}.Build())
		z.updateWeight(p)
	default:
		weight := 0
		if e.kind == wathenav1.StorageKind_STORAGE_KIND_CART {
			if b, ok := e.box.(*transfer.ListBox); ok {
				weight = b.Weight
			}
		}
		if c.Amount > 0 {
			p.pending = append(p.pending, wathenav1.Event_builder{StorageAdd: wathenav1.StorageAdd_builder{
				Kind: e.kind, Item: z.itemMessage(c.Slot, e.box.Slot(c.Slot), e.box.Slot(c.Slot).Amount),
				Amount: uint32(c.Amount), Weight: uint32(max(weight, 0)), // #nosec G115 -- a stack and a weight.
			}.Build()}.Build())
		} else {
			p.pending = append(p.pending, wathenav1.Event_builder{StorageRemove: wathenav1.StorageRemove_builder{
				Kind: e.kind, Slot: uint32(c.Slot), Amount: uint32(-c.Amount), // #nosec G115 -- a slot and a stack.
				Weight: uint32(max(weight, 0)), // #nosec G115 -- a weight.
			}.Build()}.Build())
		}
	}
}

// logGuildStorage adds a move's lines to the guild storage log.
func (z *Zone) logGuildStorage(p *Player, st *social.GuildStorage, logs transfer.Logs) {
	var entries []social.StorageLogEntry
	for _, l := range logs.Items {
		e := social.StorageLogEntry{At: l.At, Name: p.Name, ItemID: l.ItemID, Amount: l.Amount, Refine: l.Refine, Cards: l.Cards}
		if l.From.Kind == transfer.KindGuildStorage {
			e.Amount = -l.Amount
		}
		entries = append(entries, e)
	}
	// The storage's lock is held, so the log waits for the caller: the
	// registry takes its own lock before a storage's.
	z.storageLog = append(z.storageLog, storageLogBatch{st.Guild(), entries})
}

// personalChanged puts a storage or a cart in the next commit.
func (z *Zone) personalChanged(st *storage.Storage) {
	for _, o := range z.txPersonal {
		if o == st {
			return
		}
	}
	z.txPersonal = append(z.txPersonal, st)
}

// personalCommits are the storages and carts the transfers since the
// last commit changed.
func (z *Zone) personalCommits() []StorageCommit {
	var out []StorageCommit
	for _, st := range z.txPersonal {
		st.Lock()
		base, cur := st.Commit()
		c := StorageCommit{Key: st.Key(), Base: base, Cur: cur}
		if st.Key().HoldsCurrencies() {
			c.CurrencyBase, c.Currencies = st.CommitCurrencies()
			c.HasCurrencies = true
		}
		if st.Key().Expandable() {
			c.SizeBase, c.Size = st.CommitSize()
		}
		st.Unlock()
		out = append(out, c)
	}
	z.txPersonal = nil
	return out
}

// releaseStorages lets go of the storages of players who logged out, once
// their last commit went to the store.
func (z *Zone) releaseStorages() {
	for _, st := range z.storageRelease {
		z.storageRegistry().Release(st)
	}
	z.storageRelease = nil
}

// storagesLeave closes a leaving player's storage, and at logout queues
// its storages to be let go.
func (z *Zone) storagesLeave(p *Player, logout bool) {
	if p.storage != nil {
		p.storage.st.Close(p)
		p.storage = nil
	}
	if !logout {
		return
	}
	if p.sess != nil && p.sess.storages != nil {
		z.storageRelease = append(z.storageRelease, p.sess.storages...)
		p.sess.storages = nil
	}
	z.storageRelease = append(z.storageRelease, p.ownStorages...)
	p.ownStorages = nil
}

// hadCart reports whether options hold a cart.
func hadCart(o int) bool { return o&companion.OptionCart != 0 }

// countStorage is storagecountitem and cartcountitem: the amount of an
// item in a storage, or -1 when the storage cannot be read: no cart, or
// a storage open, as rAthena's storage_flag check.
func (z *Zone) countStorage(p *Player, st *storage.Storage, cart bool, id uint32) int {
	if st == nil || cart && !z.hasCart(p) {
		return -1
	}
	if !cart && (p.storage != nil || p.guild.storage != nil) {
		return -1
	}
	st.Lock()
	defer st.Unlock()
	n := 0
	for _, s := range st.Box().Slots {
		if s.ID == id {
			n += s.Amount
		}
	}
	return n
}

// forPlayer runs f for a player of this zone by name, on the zone's
// goroutine, for waserver --dev's debug endpoints. found is false when
// no such player is here.
func (z *Zone) forPlayer(ctx context.Context, name string, f func(p *Player) int) (n int, found bool, err error) {
	type result struct {
		n     int
		found bool
	}
	done := make(chan result, 1)
	run := func() {
		for _, p := range z.players {
			if chat.Key(p.Name) == chat.Key(name) && !p.gone() {
				done <- result{f(p), true}
				return
			}
		}
		done <- result{}
	}
	select {
	case z.control <- run:
	case <-ctx.Done():
		return 0, false, ctx.Err()
	case <-z.stopped:
		return 0, false, ErrStopped
	}
	select {
	case r := <-done:
		return r.n, r.found, nil
	case <-ctx.Done():
		return 0, false, ctx.Err()
	}
}

// OpenStorageFor opens the account's storage of a player of this zone by
// name, or the extra storage of record, as an NPC would, for waserver
// --dev's debug endpoint. opened is false when it did not open.
func (z *Zone) OpenStorageFor(ctx context.Context, name, record string) (opened, found bool, err error) {
	n, found, err := z.forPlayer(ctx, name, func(p *Player) int {
		var rec *storage.Record
		if record != "" {
			if rec = z.storageRecord(record, ""); rec == nil {
				return 0
			}
		}
		if z.openPersonalStorage(p, rec) {
			return 1
		}
		return 0
	})
	return n == 1, found, err
}

// SetCartFor gives a player of this zone by name a cart of a look, or
// takes it away with 0, as setcart, for waserver --dev's debug endpoint.
// The player needs Pushcart. ok is false when the cart did not change.
func (z *Zone) SetCartFor(ctx context.Context, name string, look int) (ok, found bool, err error) {
	n, found, err := z.forPlayer(ctx, name, func(p *Player) int {
		if z.setCart(p, look) {
			return 1
		}
		return 0
	})
	return n == 1, found, err
}
