package adminworld

import (
	"context"
	"errors"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/store"
)

// The account deletion job (decision 0032). Once an hour it deletes the
// accounts whose deletion is due on the server clock. For each one:
//   - It holds the account's characters, so no login loads them. A
//     character that still has a session, such as an autotrader, is
//     disconnected, and the account waits for the next run.
//   - The social registry lets the characters go: they leave their
//     parties and guilds, and the friend lists in memory. A guild master
//     hands the guild to the next member.
//   - The mails in memory from the characters lose their sender, as
//     their rows do.
//   - Once the store wrote what was queued, delete_account deletes the
//     account and its characters in one transaction.

// DeletionEvery is how often the deletion job runs.
const DeletionEvery = time.Hour

// maxDeletionsPerRun bounds the accounts one run deletes. The next run
// takes the rest.
const maxDeletionsPerRun = 100

// RunDeletions runs the deletion job at once, then every DeletionEvery,
// until ctx ends.
func (w *World) RunDeletions(ctx context.Context) {
	t := time.NewTicker(DeletionEvery)
	defer t.Stop()
	for {
		if _, err := w.DeleteDue(ctx); err != nil && ctx.Err() == nil {
			w.log.Error("account deletion job failed", "err", err)
		}
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

// DeleteDue deletes the accounts whose deletion is due now, and returns
// how many it deleted.
func (w *World) DeleteDue(ctx context.Context) (int, error) {
	st := w.cfg.Store
	if st == nil {
		return 0, nil
	}
	now := w.now()
	due, err := st.DueDeletions(ctx, now, maxDeletionsPerRun)
	if err != nil {
		return 0, err
	}
	n := 0
	var errs []error
	for _, acc := range due {
		deleted, err := w.deleteAccount(ctx, acc, now)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if deleted {
			n++
		}
	}
	return n, errors.Join(errs...)
}

// deleteAccount deletes one account whose deletion is due at now. It
// returns false when a character still has a session.
func (w *World) deleteAccount(ctx context.Context, acc int64, now time.Time) (bool, error) {
	st := w.cfg.Store
	_, ids, err := w.accountCharacters(ctx, acc)
	if err != nil {
		return false, err
	}
	hold, err := st.Hold(ids...)
	if errors.Is(err, store.ErrLive) {
		if w.cfg.Kicker != nil {
			w.cfg.Kicker.Kick(ctx, ids, wathenav1.DisconnectReason_DISCONNECT_REASON_ACCOUNT_DELETION, account.DeletionText(now))
		}
		w.log.Info("account deletion waits for a character to leave", "account", acc)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer hold.Release()
	lost := 0
	if w.cfg.Social != nil {
		for _, s := range w.cfg.Social.ForgetCharacters(ids) {
			lost += s.Amount
		}
	}
	if w.cfg.Mail != nil {
		w.cfg.Mail.ForgetSenders(ids)
	}
	if err := st.Settle(ctx); err != nil {
		return false, err
	}
	var accountStorages []int
	for _, r := range w.cfg.StorageRecords {
		if r.Account {
			accountStorages = append(accountStorages, int(r.ID))
		}
	}
	deleted, err := st.DeleteAccount(ctx, acc, now, accountStorages)
	if err != nil {
		return false, err
	}
	if deleted {
		w.log.Info("account deleted", "account", acc, "characters", len(ids), "guild_storage_items_lost", lost)
	}
	return deleted, nil
}
