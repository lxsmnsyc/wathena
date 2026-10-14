package account

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Account deletion (decision 0032). A player asks for it with a login
// from the last 10 minutes and the account name typed again. After
// DeletionWait, waserver's deletion job deletes the account's personal
// data and characters. Meanwhile the characters cannot play, and the
// account pages show the date with a button that cancels it.

// DeletionWait is how long a deletion request waits before the job
// deletes the account.
const DeletionWait = 14 * 24 * time.Hour

// DeletionText is what a player of an account set to be deleted sees
// when a game login is refused.
func DeletionText(after time.Time) string {
	return "This account is set to be deleted on " + minute(after) +
		". Its characters cannot play until then. Cancel the deletion on the account page to play again."
}

// deletionView is a request that waits, for the pages.
type deletionView struct {
	Requested, After string
}

// deletion returns the account's deletion request, or nil.
func (s *Service) deletion(ctx context.Context, account int64) (*db.AccountDeletionRow, error) {
	d, err := s.q.AccountDeletion(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// fillDeletion puts the account's deletion request on a page.
func (s *Service) fillDeletion(ctx context.Context, p *page, account int64) error {
	d, err := s.deletion(ctx, account)
	if err != nil || d == nil {
		return err
	}
	p.Deletion = &deletionView{Requested: minute(d.RequestedAt.Time), After: minute(d.DeleteAfter.Time)}
	return nil
}

func (s *Service) getData(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p := s.newPage(w, r, "Your data")
	if msg, ok := dataNotices[r.FormValue("notice")]; ok {
		p.Notice = msg
	}
	if err := s.fillData(ctx, p, sess); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "data.html", p)
}

func (s *Service) fillData(ctx context.Context, p *page, sess db.UseSessionRow) error {
	p.Account = sess.AccountName
	p.DeletionDays = int(DeletionWait.Hours() / 24)
	p.Recent = recent(sess.AuthenticatedAt.Time, time.Now(), s.cfg.ReauthWindow)
	last, err := s.q.LastExport(ctx, sess.AccountID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return err
	default:
		p.LastExport = minute(last.Time)
		if next := last.Time.Add(24 * time.Hour); time.Now().Before(next) {
			p.NextExport = minute(next)
		}
	}
	return s.fillDeletion(ctx, p, sess.AccountID)
}

// requestDeletion asks for the account's deletion.
func (s *Service) requestDeletion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.accountAction(ctx, w, r)
	if !ok || !s.needRecent(w, r, sess, Prefix+"data") {
		return
	}
	refuse := func(status int, msg string) {
		p := s.newPage(w, r, "Your data")
		p.Error = msg
		if err := s.fillData(ctx, p, sess); err != nil {
			s.unavailable(w, r, err)
			return
		}
		s.render(w, status, "data.html", p)
	}
	if !strings.EqualFold(strings.TrimSpace(r.PostForm.Get("name")), sess.AccountName) {
		refuse(http.StatusBadRequest, "Type your account name, "+sess.AccountName+", to confirm.")
		return
	}
	for _, o := range s.cfg.Owners {
		if strings.EqualFold(o, sess.AccountName) {
			refuse(http.StatusConflict, "This account is an owner of the server in its config. Take it out of staff.owners first.")
			return
		}
	}
	if s.cfg.Game != nil {
		char, guild, err := s.cfg.Game.LeadsGuild(ctx, sess.AccountID)
		if err != nil {
			s.unavailable(w, r, err)
			return
		}
		if guild != "" {
			refuse(http.StatusConflict, char+" leads the guild "+guild+", which has other members. Make another member the guild's master first.")
			return
		}
	}
	n, err := s.q.RequestDeletion(ctx, db.RequestDeletionParams{AccountID: sess.AccountID, Wait: interval(DeletionWait)})
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	d, err := s.deletion(ctx, sess.AccountID)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if n == 1 {
		s.log.Info("account deletion requested", "account", sess.AccountID)
	}
	// The characters stop playing at once.
	if s.cfg.Game != nil && d != nil {
		s.cfg.Game.Deleting(ctx, sess.AccountID, DeletionText(d.DeleteAfter.Time))
	}
	s.redirect(w, r, withNotice(Prefix+"data", "deletion-requested"))
}

// cancelDeletion cancels the account's deletion. It needs no recent
// login: the button shows right after a login, and a cancel only keeps
// the account.
func (s *Service) cancelDeletion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.accountAction(ctx, w, r)
	if !ok {
		return
	}
	n, err := s.q.CancelDeletion(ctx, sess.AccountID)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if n == 1 {
		s.log.Info("account deletion cancelled", "account", sess.AccountID)
	}
	s.redirect(w, r, withNotice(Prefix+"data", "deletion-cancelled"))
}
