package account

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// passkeyView is a passkey on the settings page.
type passkeyView struct {
	ID       int64
	Name     string
	Created  string
	LastUsed string
}

// settingsNotices are the messages the settings page shows for ?notice=.
var settingsNotices = map[string]string{
	"totp-on":          "Two-factor login is on.",
	"totp-off":         "Two-factor login is off.",
	"totp-already":     "Two-factor login is already on.",
	"passkey-added":    "The passkey is added.",
	"passkey-removed":  "The passkey is removed.",
	"password-changed": "The password is changed. Your other sessions are logged out.",
	"sessions-ended":   "Your other sessions are logged out.",
}

func (s *Service) getSettings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p := s.newPage(w, r, "Account settings")
	if msg, ok := settingsNotices[r.FormValue("notice")]; ok {
		p.Notice = msg
	}
	if err := s.fillSettings(ctx, p, sess); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "settings.html", p)
}

func (s *Service) fillSettings(ctx context.Context, p *page, sess db.UseSessionRow) error {
	p.Account = sess.AccountName
	p.Factors = factorNames(sess.Factors)
	p.TOTPAvailable = s.cfg.Sealer != nil
	p.PasskeysAvailable = s.wa != nil
	if s.wa != nil {
		p.Script, p.Passkey = true, "register"
	}
	totp, err := s.q.TOTPByAccount(ctx, sess.AccountID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return err
	default:
		p.TOTPOn = totp.EnabledAt.Valid
		p.TOTPPending = !totp.EnabledAt.Valid
	}
	if p.TOTPOn {
		if p.RecoveryLeft, err = s.q.RecoveryCodesLeft(ctx, sess.AccountID); err != nil {
			return err
		}
	}
	keys, err := s.q.AccountPasskeys(ctx, sess.AccountID)
	if err != nil {
		return err
	}
	others, err := s.otherSessions(ctx, sess)
	if err != nil {
		return err
	}
	p.Sessions = sessionCount(others)
	for _, k := range keys {
		v := passkeyView{ID: k.ID, Name: k.Name, Created: day(k.CreatedAt.Time), LastUsed: "never"}
		if k.LastUsedAt.Valid {
			v.LastUsed = day(k.LastUsedAt.Time)
		}
		p.Passkeys = append(p.Passkeys, v)
	}
	return nil
}

func day(t time.Time) string { return t.UTC().Format("2006-01-02") }

// settingsAction starts a settings post: the form, its CSRF token, the
// session, and a recent login. It writes the answer and returns false
// when one of them fails.
func (s *Service) settingsAction(ctx context.Context, w http.ResponseWriter, r *http.Request) (db.UseSessionRow, bool) {
	if !s.form(w, r) {
		return db.UseSessionRow{}, false
	}
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok || !s.needRecent(w, r, sess, Prefix+"settings") {
		return sess, false
	}
	return sess, true
}

// startTOTP makes a new TOTP secret that waits for its first code, and
// shows it.
func (s *Service) startTOTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.settingsAction(ctx, w, r)
	if !ok {
		return
	}
	if s.cfg.Sealer == nil {
		s.render(w, http.StatusServiceUnavailable, "message.html", &page{Title: "Not available", Error: "Two-factor codes are not set up on this server."})
		return
	}
	secret := newTOTPSecret()
	n, err := s.q.StartTOTP(ctx, db.StartTOTPParams{AccountID: sess.AccountID, Secret: s.cfg.Sealer.seal(secret, totpAD(sess.AccountID))})
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if n == 0 {
		s.redirect(w, r, withNotice(Prefix+"settings", "totp-already"))
		return
	}
	s.redirect(w, r, Prefix+"settings/totp")
}

// totpSetup fills the setup page: the QR code and the key of the
// secret that waits for its first code. It returns false when there is
// no such secret.
func (s *Service) totpSetup(ctx context.Context, p *page, sess db.UseSessionRow) (sealed []byte, ok bool, err error) {
	row, err := s.q.TOTPByAccount(ctx, sess.AccountID)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && row.EnabledAt.Valid {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	secret, _, err := s.openTOTP(sess.AccountID, row.Secret)
	if err != nil {
		return nil, false, err
	}
	uri, err := totpURI(s.cfg.Issuer, sess.AccountName, secret)
	if err != nil {
		return nil, false, err
	}
	if p.QR, err = qrSVG(uri); err != nil {
		return nil, false, err
	}
	p.Key = formatKey(secret)
	p.Account = sess.AccountName
	return row.Secret, true, nil
}

func (s *Service) getTOTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok || !s.needRecent(w, r, sess, Prefix+"settings") {
		return
	}
	p := s.newPage(w, r, "Turn on two-factor login")
	_, ok, err := s.totpSetup(ctx, p, sess)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if !ok {
		s.redirect(w, r, Prefix+"settings")
		return
	}
	s.render(w, http.StatusOK, "totp.html", p)
}

// confirmTOTP checks the first code from the app, turns TOTP on, and
// shows the recovery codes, once.
func (s *Service) confirmTOTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.settingsAction(ctx, w, r)
	if !ok {
		return
	}
	p := s.newPage(w, r, "Turn on two-factor login")
	sealed, ok, err := s.totpSetup(ctx, p, sess)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if !ok {
		s.redirect(w, r, Prefix+"settings")
		return
	}
	addr, now := s.clientAddr(r), time.Now()
	if ok, wait := s.limit.allow(actLogin, addr, now); !ok {
		p.Error = "Too many attempts from your network. Try again in " + seconds(wait) + "."
		s.render(w, http.StatusTooManyRequests, "totp.html", p)
		return
	}
	wait, _, release := s.limit.reserve(sess.AccountName, addr, now)
	defer release()
	if wait > 0 {
		p.Error = "Too many failed attempts for this account. Try again in " + seconds(wait) + "."
		s.render(w, http.StatusTooManyRequests, "totp.html", p)
		return
	}
	secret, _, err := s.openTOTP(sess.AccountID, sealed)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	step, ok := checkTOTP(secret, normalizeCode(r.PostForm.Get("code")), s.cfg.Clock(), 0)
	if !ok {
		s.limit.fail(sess.AccountName, addr, now)
		p.Error = "That code is wrong. Check the time on your device, and enter the code the app shows now."
		s.render(w, http.StatusBadRequest, "totp.html", p)
		return
	}
	codes, hashes := newRecoveryCodes()
	enabled := false
	err = pgx.BeginFunc(ctx, s.cfg.Pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		n, err := q.EnableTOTP(ctx, db.EnableTOTPParams{AccountID: sess.AccountID, Step: step, Secret: sealed})
		if err != nil || n == 0 {
			return err
		}
		enabled = true
		return replaceRecoveryCodes(ctx, q, sess.AccountID, hashes)
	})
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if !enabled {
		// Another request turned it on, or started over, meanwhile.
		s.redirect(w, r, Prefix+"settings")
		return
	}
	s.limit.succeed(sess.AccountName)
	s.log.Info("TOTP turned on", "account", sess.AccountID)
	s.afterLoginChange(ctx, sess, "totp_on")
	s.showRecoveryCodes(w, r, codes, "Two-factor login is on")
}

func replaceRecoveryCodes(ctx context.Context, q *db.Queries, account int64, hashes [][]byte) error {
	if err := q.DeleteRecoveryCodes(ctx, account); err != nil {
		return err
	}
	return q.InsertRecoveryCodes(ctx, db.InsertRecoveryCodesParams{AccountID: account, Hashes: hashes})
}

// showRecoveryCodes shows new recovery codes. It is the only time they
// are shown, and the page is not cached (Cache-Control: no-store).
func (s *Service) showRecoveryCodes(w http.ResponseWriter, r *http.Request, codes []string, title string) {
	p := s.newPage(w, r, title)
	p.Codes = codes
	s.render(w, http.StatusOK, "recovery.html", p)
}

// newRecovery replaces the recovery codes with new ones.
func (s *Service) newRecovery(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.settingsAction(ctx, w, r)
	if !ok {
		return
	}
	on, err := s.totpOn(ctx, sess.AccountID)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if !on {
		s.redirect(w, r, Prefix+"settings")
		return
	}
	codes, hashes := newRecoveryCodes()
	err = pgx.BeginFunc(ctx, s.cfg.Pool, func(tx pgx.Tx) error {
		return replaceRecoveryCodes(ctx, s.q.WithTx(tx), sess.AccountID, hashes)
	})
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.log.Info("recovery codes replaced", "account", sess.AccountID)
	s.afterLoginChange(ctx, sess, "recovery_codes")
	s.showRecoveryCodes(w, r, codes, "New recovery codes")
}

// disableTOTP turns TOTP off. It needs the password and a current code
// (or a recovery code) in the same form, so it does not need a recent
// login too.
func (s *Service) disableTOTP(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	on, err := s.totpOn(ctx, sess.AccountID)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if !on {
		s.redirect(w, r, Prefix+"settings")
		return
	}
	status, msg, err := s.checkIdentity(ctx, r, sess, true)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if msg != "" {
		p := s.newPage(w, r, "Account settings")
		p.Error = msg
		if err := s.fillSettings(ctx, p, sess); err != nil {
			s.unavailable(w, r, err)
			return
		}
		s.render(w, status, "settings.html", p)
		return
	}
	if err := s.q.DeleteTOTP(ctx, sess.AccountID); err != nil {
		s.unavailable(w, r, err)
		return
	}
	if err := s.q.Reauthenticate(ctx, sess.ID); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.log.Info("TOTP turned off", "account", sess.AccountID)
	s.afterLoginChange(ctx, sess, "totp_off")
	s.redirect(w, r, withNotice(Prefix+"settings", "totp-off"))
}

func (s *Service) removePasskey(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.settingsAction(ctx, w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PostForm.Get("id"), 10, 64)
	if err != nil {
		s.render(w, http.StatusBadRequest, "message.html", &page{Title: "Bad request", Error: "Choose a passkey."})
		return
	}
	n, err := s.q.DeletePasskey(ctx, db.DeletePasskeyParams{ID: id, AccountID: sess.AccountID})
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if n == 1 {
		s.log.Info("passkey removed", "account", sess.AccountID)
		s.afterLoginChange(ctx, sess, "passkey_removed")
	}
	s.redirect(w, r, withNotice(Prefix+"settings", "passkey-removed"))
}
