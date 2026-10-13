package adminworld

import (
	"context"
	"errors"
	"time"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// Restores (decision 0029):
//   - The database says where each serial is, after the commits queued
//     so far are written. A serial in no place is gone, and the restore
//     makes a copy of its last log line, which needs items.create.
//   - A serial in the inventory or the cart of an online character
//     moves in that character's zone. Every other holder is a shared
//     object with a lock: storages and mailboxes come from the world's
//     registries, guild storages from the social registry.
//   - The characters that own an offline holder are held in the store,
//     so no login reads them while the restore changes them. An offline
//     inventory is loaded under the hold, and saved with the commit.
//   - The zone checks every serial again before it moves anything. A
//     serial that moved, or a storage that is open, makes the restore
//     start over, a few times, from the database.
//   - The restore's mail, its holders, its log lines and its audit row
//     go in one commit, so one transaction writes them.

// restoreTries is how often a restore starts over when its items move.
const restoreTries = 8

// mailSlots is the most items a restore's mail holds: mail_items slots
// go up to 20.
const mailSlots = 20

// errRetry is a restore that must start over.
var errRetry = errors.New("adminworld: the items moved")

// Restore gives items and zeny back to a character by mail.
func (w *World) Restore(ctx context.Context, r admin.Restore) (*adminv1.RestoreResult, error) {
	if w.cfg.Store == nil {
		return nil, errNoStore
	}
	rows, err := w.cfg.Store.Queries().CharacterNames(ctx, []int64{r.Character})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, &admin.WorldError{Code: "not_found", Message: "There is no character with that ID."}
	}
	to := rows[0]
	for try := range restoreTries {
		if try > 0 {
			select {
			case <-time.After(time.Duration(try) * 100 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		res, err := w.tryRestore(ctx, r, mail.Recipient{Key: mail.KeyOf(to.ID, to.Name), Name: to.Name}, to.AccountID)
		if errors.Is(err, errRetry) {
			continue
		}
		return res, err
	}
	return nil, &admin.WorldError{Code: "conflict", Message: "The items kept moving, or a storage stayed open. Try again."}
}

// restoreRun is what one try holds and lets go at its end.
type restoreRun struct {
	w         *World
	holds     []*store.Hold
	held      map[int64]bool
	offline   map[int64]*zone.OfflineCharacter
	sessions  map[int64]*store.Session
	storages  map[storage.Key]*storage.Storage
	mailboxes map[mail.Key]*mail.Mailbox
}

// release ends the offline sessions with a final save, lets go of the
// storages and mailboxes, and then of the holds. The restore's commit,
// if any, is queued already, so later loads wait for it.
func (run *restoreRun) release() {
	for id, x := range run.sessions {
		x.Save(run.offline[id].Character, true)
	}
	for _, st := range run.storages {
		run.w.cfg.Storages.Release(st)
	}
	for _, b := range run.mailboxes {
		run.w.cfg.Mail.Release(b)
	}
	for _, h := range run.holds {
		h.Release()
	}
}

// hold keeps characters from loading. It returns false when one of them
// has a session.
func (run *restoreRun) hold(ids []int64) (bool, error) {
	var want []int64
	for _, id := range ids {
		if !run.held[id] {
			want = append(want, id)
		}
	}
	if len(want) == 0 {
		return true, nil
	}
	h, err := run.w.cfg.Store.Hold(want...)
	if errors.Is(err, store.ErrLive) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	run.holds = append(run.holds, h)
	for _, id := range want {
		run.held[id] = true
	}
	return true, nil
}

func (w *World) tryRestore(ctx context.Context, r admin.Restore, to mail.Recipient, toAccount int64) (*adminv1.RestoreResult, error) {
	w.settle(ctx)
	q := w.cfg.Store.Queries()
	run := &restoreRun{w: w, held: map[int64]bool{}, offline: map[int64]*zone.OfflineCharacter{}, sessions: map[int64]*store.Session{},
		storages: map[storage.Key]*storage.Storage{}, mailboxes: map[mail.Key]*mail.Mailbox{}}
	defer run.release()
	plan := &zone.Restore{To: to, Sender: "Staff", Title: "Restored items", Zeny: r.Zeny,
		Body: "Staff restored these items to you.", Staff: transfer.Owner{Kind: transfer.KindStaff, ID: r.AccountID},
		Offline: run.offline, Storages: run.storages, Mailboxes: run.mailboxes,
		Audit: staff.AuditRow{AccountID: r.AccountID, AccountName: r.AccountName, Role: r.Role, Action: "items.restore", Source: "admin",
			Target: to.Name, TargetAccount: toAccount, Arguments: map[string]any{"character_id": to.Key.ID}, Reason: r.Reason, NeedsReview: true}}
	var pinned *zone.Zone
	pin := func(z *zone.Zone) error {
		if pinned != nil && pinned != z {
			return &admin.WorldError{Code: "bad_request",
				Message: "The items are held by players on different maps. Restore them in separate calls."}
		}
		pinned = z
		return nil
	}
	for _, it := range r.Items {
		holders, err := w.cfg.Store.ItemHolders(ctx, it.Serial)
		if err != nil {
			return nil, err
		}
		ri := zone.RestoreItem{Serial: it.Serial, Amount: it.Amount}
		if len(holders) == 0 {
			if !r.MayCreate {
				return nil, &admin.WorldError{Code: "forbidden",
					Message: "The serial " + it.Serial.String() + " is gone. A copy needs items.create."}
			}
			cp, amount, found, err := w.cfg.Store.LastCopy(ctx, it.Serial)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, &admin.WorldError{Code: "not_found", Message: "No log line names the serial " + it.Serial.String() + "."}
			}
			if ri.Amount == 0 {
				ri.Amount = amount
			}
			ri.Copy = &cp
			plan.Items = append(plan.Items, ri)
			continue
		}
		h := holders[0]
		switch h.Holder {
		case "inventory", "cart":
			ri.Holder = zone.RestoreHolder{Kind: transfer.KindInventory, Character: h.ID}
			if h.Holder == "cart" {
				k := storage.Key{Holder: storage.HolderCart, ID: h.ID}
				ri.Holder = zone.RestoreHolder{Kind: transfer.KindCart, Character: h.ID, Storage: k}
			}
			zones, err := w.locate(ctx, []int64{h.ID})
			if err != nil {
				return nil, err
			}
			if z := zones[h.ID]; z != nil {
				if err := pin(z); err != nil {
					return nil, err
				}
				break
			}
			ok, err := run.hold([]int64{h.ID})
			if err != nil {
				return nil, err
			}
			if !ok {
				// Online, but not in a zone yet: it logs in, warps or
				// logs out.
				return nil, errRetry
			}
			if h.Holder == "inventory" {
				if err := run.load(ctx, h.ID); err != nil {
					return nil, err
				}
			} else if err := run.acquire(ctx, ri.Holder.Storage); err != nil {
				return nil, err
			}
		case "account", "character":
			k := w.storageKeyOf(holderRef(h))
			ri.Holder = zone.RestoreHolder{Kind: transfer.KindStorage, Storage: k}
			owners := []int64{h.ID}
			if k.Holder == storage.HolderAccount {
				if owners, err = q.CharacterIDsOfAccount(ctx, h.ID); err != nil {
					return nil, err
				}
			} else {
				ri.Holder.Character = h.ID
			}
			// An offline account is held, so no login moves the items
			// while the restore does. An online one moves them under the
			// storage's lock, and only when the storage is closed.
			if _, err := run.hold(owners); err != nil {
				return nil, err
			}
			if err := run.acquire(ctx, k); err != nil {
				return nil, err
			}
		case "mail":
			rcpt, err := q.MailRecipient(ctx, h.ID)
			if err != nil {
				return nil, errRetry
			}
			key := mail.Key{ID: rcpt}
			ri.Holder = zone.RestoreHolder{Kind: transfer.KindMail, Mail: h.ID, MailTo: key}
			if _, err := run.hold([]int64{rcpt}); err != nil {
				return nil, err
			}
			if run.mailboxes[key] == nil {
				names, err := q.CharacterNames(ctx, []int64{rcpt})
				if err != nil || len(names) == 0 {
					return nil, errRetry
				}
				b, err := w.cfg.Mail.Acquire(ctx, key, names[0].Name, mailSlots)
				if err != nil {
					return nil, err
				}
				run.mailboxes[key] = b
			}
		case "guild_storage":
			ri.Holder = zone.RestoreHolder{Kind: transfer.KindGuildStorage, Guild: h.ID}
		default:
			return nil, errRetry
		}
		plan.Items = append(plan.Items, ri)
	}
	z := pinned
	if z == nil {
		z = w.cfg.World.Start()
	}
	res, err := z.Restore(ctx, plan)
	switch {
	case errors.Is(err, zone.ErrMoved), errors.Is(err, zone.ErrBusy):
		return nil, errRetry
	case errors.Is(err, zone.ErrAmount):
		return nil, &admin.WorldError{Code: "bad_request", Message: "A stack holds less than the amount asked for."}
	case errors.Is(err, zone.ErrFull):
		return nil, &admin.WorldError{Code: "bad_request", Message: "The items do not fit one mail."}
	case err != nil:
		return nil, err
	}
	return w.restoreResult(ctx, res)
}

// load loads a held offline character for the restore.
func (run *restoreRun) load(ctx context.Context, id int64) error {
	if run.offline[id] != nil {
		return nil
	}
	for _, h := range run.holds {
		c, x, err := h.Load(ctx, id)
		if err != nil {
			continue
		}
		run.offline[id] = &zone.OfflineCharacter{Character: c, Saver: x}
		run.sessions[id] = x
		return nil
	}
	return errRetry
}

// acquire takes a storage or a cart from the registry.
func (run *restoreRun) acquire(ctx context.Context, k storage.Key) error {
	if run.storages[k] != nil {
		return nil
	}
	sts, err := run.w.cfg.Storages.Acquire(ctx, []storage.Key{k})
	if err != nil {
		return err
	}
	run.storages[k] = sts[0]
	return nil
}

// restoreResult is the answer of a restore.
func (w *World) restoreResult(ctx context.Context, res zone.RestoreResult) (*adminv1.RestoreResult, error) {
	var refs []ownerRef
	for _, it := range res.Items {
		refs = append(refs, ownerRef{it.From.Kind, it.From.ID, it.From.Storage})
	}
	names, err := w.owners(ctx, refs)
	if err != nil {
		return nil, err
	}
	out := adminv1.RestoreResult_builder{MailId: res.Mail, Zeny: res.Zeny}
	for _, it := range res.Items {
		out.Items = append(out.Items, adminv1.RestoredItem_builder{Serial: it.Serial.String(), NewSerial: it.NewSerial.String(),
			Copy: it.Copy, ItemId: it.ItemID, ItemName: w.itemName(it.ItemID), Amount: int32(it.Amount), // #nosec G115 -- a stack.
			From: names[ownerRef{it.From.Kind, it.From.ID, it.From.Storage}]}.Build())
	}
	return out.Build(), nil
}
