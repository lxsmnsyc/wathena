package account

import (
	"context"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Other sessions (review decision 51):
//   - The settings page shows how many other sessions are logged in,
//     with a button that ends them all. It needs a recent login, so a
//     stolen cookie cannot log the player out of their own devices.
//   - Ending them also ends the logins that wait for a second factor,
//     deletes the game tickets nobody used, and closes the game
//     connections the sessions opened, through Config.Ended.
//   - A change of the password or of the login methods does the same:
//     turning TOTP on or off, new recovery codes, and adding or removing
//     a passkey. The session that made the change stays.

// endOtherSessions ends every session of the account but sess, in one
// transaction, then closes the game connections they opened. It returns
// how many sessions ended.
func (s *Service) endOtherSessions(ctx context.Context, sess db.UseSessionRow) (int, error) {
	var ids []int64
	err := pgx.BeginFunc(ctx, s.cfg.Pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteAccountPendingLogins(ctx, sess.AccountID); err != nil {
			return err
		}
		if err := q.DeleteOtherSessionTickets(ctx, db.DeleteOtherSessionTicketsParams{AccountID: sess.AccountID, KeepID: sess.ID}); err != nil {
			return err
		}
		var err error
		ids, err = q.RevokeOtherSessions(ctx, db.RevokeOtherSessionsParams{AccountID: sess.AccountID, KeepID: sess.ID})
		return err
	})
	if err != nil {
		return 0, err
	}
	if len(ids) > 0 && s.cfg.Ended != nil {
		s.cfg.Ended.EndLoginSessions(ids...)
	}
	return len(ids), nil
}

// afterLoginChange ends the other sessions after a change of the
// password or the login methods. The change is made already, so a
// failure is logged and the page goes on.
func (s *Service) afterLoginChange(ctx context.Context, sess db.UseSessionRow, change string) {
	n, err := s.endOtherSessions(context.WithoutCancel(ctx), sess)
	if err != nil {
		s.log.Error("cannot end the other sessions after a login change", "account", sess.AccountID, "change", change, "err", err)
		return
	}
	if n > 0 {
		s.log.Info("a login change ended the other sessions", "account", sess.AccountID, "change", change, "sessions", n)
	}
}

// otherSessions counts the account's other live sessions, for the
// settings page.
func (s *Service) otherSessions(ctx context.Context, sess db.UseSessionRow) (int64, error) {
	return s.q.CountOtherSessions(ctx, db.CountOtherSessionsParams{AccountID: sess.AccountID, KeepID: sess.ID,
		Idle: interval(s.cfg.SessionIdle)})
}

// endSessions is the settings page's "Log out everywhere else" button.
func (s *Service) endSessions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.settingsAction(ctx, w, r)
	if !ok {
		return
	}
	n, err := s.endOtherSessions(ctx, sess)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.log.Info("other sessions ended", "account", sess.AccountID, "sessions", n)
	s.redirect(w, r, withNotice(Prefix+"settings", "sessions-ended"))
}

// changePassword sets a new password. It needs the current password in
// the same form, so it does not need a recent login too. It ends the
// other sessions.
func (s *Service) changePassword(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	refuse := func(status int, msg string) {
		p := s.newPage(w, r, "Account settings")
		p.Error = msg
		if err := s.fillSettings(ctx, p, sess); err != nil {
			s.unavailable(w, r, err)
			return
		}
		s.render(w, status, "settings.html", p)
	}
	password := r.PostForm.Get("new_password")
	if password != r.PostForm.Get("confirm") {
		refuse(http.StatusBadRequest, "The two new passwords are not the same.")
		return
	}
	if msg := PasswordProblem(sess.AccountName, password); msg != "" {
		refuse(http.StatusBadRequest, msg)
		return
	}
	status, msg, err := s.checkIdentity(ctx, r, sess, false)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if msg != "" {
		refuse(status, msg)
		return
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	n, err := s.q.SetPassword(ctx, db.SetPasswordParams{AccountID: sess.AccountID, Hash: hash})
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if n == 0 {
		refuse(http.StatusConflict, "This account has no password to change.")
		return
	}
	if err := s.q.Reauthenticate(ctx, sess.ID); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.log.Info("password changed", "account", sess.AccountID)
	s.afterLoginChange(ctx, sess, "password")
	s.redirect(w, r, withNotice(Prefix+"settings", "password-changed"))
}

// sessionCount is the settings page's text for the other sessions.
func sessionCount(n int64) string {
	switch n {
	case 0:
		return "No other session is logged in."
	case 1:
		return "1 other session is logged in."
	}
	return strconv.FormatInt(n, 10) + " other sessions are logged in."
}
