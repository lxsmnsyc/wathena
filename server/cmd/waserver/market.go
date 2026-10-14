package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/market"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/transfer"
	"github.com/lxsmnsyc/wathena/server/world"
)

// expiryInterval is how often mail expiry runs, as mail_return_timer.
const expiryInterval = time.Minute

// expireMail returns and deletes the mails that expired, once a minute,
// until ctx ends. Without a database the mails in memory expire, and
// their log lines go to the log.
func expireMail(ctx context.Context, mails *mail.Registry, st *store.Store, rl *rules.Rules, clock func() time.Time, log *slog.Logger) {
	m := rules.MarketOrDefault(rl)
	e := mail.Expiry{ReturnDays: m.MailReturnDays, DeleteDays: m.MailDeleteDays, ReturnEmpty: m.MailReturnEmpty,
		Slots: rules.LimitsOrDefault(rl).MailItems}
	commit := func(w []mail.Write, logs transfer.Logs) {
		if st != nil {
			st.CommitMail(w, logs)
			return
		}
		for _, l := range logs.Items {
			log.Debug("item transfer", "action", l.Action, "from", l.From.Kind.String(), "from_id", l.From.ID,
				"to", l.To.Kind.String(), "to_id", l.To.ID, "item", l.ItemID, "amount", l.Amount)
		}
	}
	t := time.NewTicker(expiryInterval)
	defer t.Stop()
	for {
		now := time.Now()
		if clock != nil {
			now = clock()
		}
		if err := mails.Expire(ctx, now, e, commit); err != nil && ctx.Err() == nil {
			log.Warn("mail expiry failed", "err", err)
		}
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

// autotradesToRestore are the saved autotrade stores to open again: all
// of them, or none when autotrade_restore is off.
func autotradesToRestore(m rules.Market, saved []*market.Store) []*market.Store {
	if !m.AutotradeRestore {
		return nil
	}
	return saved
}

// restoreAutotraders opens the saved autotrade stores again after a
// restart, as do_init_vending_autotrade: each owner logs in without a
// connection and opens its store.
func restoreAutotraders(ctx context.Context, st *store.Store, chars gateway.Characters, w *world.World, m rules.Market, log *slog.Logger) {
	saved, err := st.Autotraders(ctx)
	if err != nil {
		log.Error("cannot load the autotrade stores", "err", err)
		return
	}
	list := autotradesToRestore(m, saved)
	if len(list) < len(saved) {
		st.CommitAll(nil, nil, storeDeletes(saved), transfer.Logs{})
	}
	opened := 0
	for _, s := range list {
		// An account set to be deleted does not play, autotrade either
		// (decision 0032). Its store opens again if the player cancels.
		if _, deleting, err := st.DeletionOf(ctx, s.Owner); err != nil || deleting {
			if err != nil {
				log.Warn("an autotrade store did not open", "character", s.Owner, "err", err)
			}
			continue
		}
		c, saver, err := chars.Load(ctx, s.Owner)
		if err != nil {
			log.Warn("an autotrade store did not open", "character", s.Owner, "err", err)
			continue
		}
		sess, err := w.JoinCharacter(ctx, c, saver)
		if err != nil {
			saver.Save(c, true)
			log.Warn("an autotrade store did not open", "character", s.Owner, "err", err)
			continue
		}
		// Nobody reads the messages of an autotrader.
		go func() {
			for range sess.Out {
			}
		}()
		done := make(chan bool, 1)
		sess.RestoreStore(s, func(ok bool) { done <- ok })
		if <-done {
			opened++
		} else {
			log.Warn("an autotrade store did not open: its items are gone", "character", s.Owner)
		}
	}
	if len(saved) > 0 {
		log.Info("autotrade stores opened", "stores", opened, "saved", len(saved))
	}
}

// storeDeletes are writes that delete stores.
func storeDeletes(list []*market.Store) store.Writes {
	var w store.Writes
	for _, s := range list {
		w.Stores = append(w.Stores, market.Write{Key: s.KeyOf()})
	}
	return w
}
