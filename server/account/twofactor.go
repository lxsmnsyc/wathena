package account

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Factors a login session records. account_sessions.factors allows
// these values only.
const (
	factorPassword = "password"
	factorTOTP     = "totp"
	factorRecovery = "recovery_code"
	factorPasskey  = "passkey"
)

// errNoSealer is a TOTP secret that cannot be opened because the server
// has no secret key.
var errNoSealer = errors.New("account: an account has TOTP on, and WATHENA_ACCOUNT_SECRET_KEY is not set")

// totpOn reports whether an account has TOTP turned on.
func (s *Service) totpOn(ctx context.Context, account int64) (bool, error) {
	row, err := s.q.TOTPByAccount(ctx, account)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && row.EnabledAt.Valid, err
}

// startPending starts a login that waits for its TOTP code: a random
// token in a cookie, and its hash in the database. It is not a session:
// it only opens the code form, for PendingLifetime and MaxCodeAttempts.
func (s *Service) startPending(ctx context.Context, w http.ResponseWriter, account int64, factor string) error {
	token, hash := newToken()
	err := s.q.CreatePendingLogin(ctx, db.CreatePendingLoginParams{
		TokenHash: hash, AccountID: account, Factor: factor,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.cfg.PendingLifetime), Valid: true},
	})
	if err != nil {
		return err
	}
	s.setCookie(w, pendingCookie, token, int(s.cfg.PendingLifetime.Seconds()))
	return nil
}

// checkSecondFactor checks a TOTP code or a recovery code for an account
// with TOTP on. It returns the factor that passed, or "" when the code
// is wrong or was used before. A TOTP code is accepted once: its time
// step is stored, in the same statement that checks it is new.
func (s *Service) checkSecondFactor(ctx context.Context, account int64, input string) (string, error) {
	code := normalizeCode(input)
	if isTOTPCode(code) {
		row, err := s.q.TOTPByAccount(ctx, account)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !row.EnabledAt.Valid {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		secret, current, err := s.openTOTP(account, row.Secret)
		if err != nil {
			return "", err
		}
		step, ok := checkTOTP(secret, code, s.cfg.Clock(), row.LastStep)
		if !ok {
			return "", nil
		}
		n, err := s.q.UseTOTPStep(ctx, db.UseTOTPStepParams{AccountID: account, Step: step})
		if err != nil || n == 0 {
			return "", err
		}
		if !current {
			// Sealed under an old key: seal it under the current one. A
			// failure only means trying at the next login.
			err := s.q.ResealTOTP(ctx, db.ResealTOTPParams{
				AccountID: account, OldSecret: row.Secret, NewSecret: s.cfg.Sealer.seal(secret, totpAD(account)),
			})
			if err != nil {
				s.log.Error("cannot seal a TOTP secret under the current key", "account", account, "err", err)
			}
		}
		return factorTOTP, nil
	}
	if hash, ok := recoveryHash(code); ok {
		n, err := s.q.UseRecoveryCode(ctx, db.UseRecoveryCodeParams{AccountID: account, CodeHash: hash})
		if err != nil || n == 0 {
			return "", err
		}
		s.log.Info("recovery code used", "account", account)
		return factorRecovery, nil
	}
	return "", nil
}

// openTOTP opens a sealed TOTP secret. current reports whether the
// current key sealed it.
func (s *Service) openTOTP(account int64, sealed []byte) (secret []byte, current bool, err error) {
	if s.cfg.Sealer == nil {
		return nil, false, errNoSealer
	}
	secret, current, err = s.cfg.Sealer.open(sealed, totpAD(account))
	if err != nil {
		s.log.Error("an account's TOTP secret cannot be opened; is its key in WATHENA_ACCOUNT_OLD_SECRET_KEYS?", "account", account)
	}
	return secret, current, err
}

func (s *Service) getLoginCode(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	hash, ok := tokenHash(s.cookie(r, pendingCookie))
	var row db.PendingLoginRow
	var err error
	if ok {
		row, err = s.q.PendingLogin(ctx, db.PendingLoginParams{TokenHash: hash, MaxAttempts: s.cfg.MaxCodeAttempts})
	}
	if !ok || errors.Is(err, pgx.ErrNoRows) {
		s.redirect(w, r, withNotice(withReturn(Prefix+"login", s.safeReturn(r.FormValue("return"))), "expired"))
		return
	}
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	p := s.newPage(w, r, "Enter your code")
	p.AttemptsLeft = s.cfg.MaxCodeAttempts - row.Attempts
	s.render(w, http.StatusOK, "code.html", p)
}

func (s *Service) postLoginCode(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	addr, now := s.clientAddr(r), time.Now()
	p := s.newPage(w, r, "Enter your code")
	expired := func() {
		s.setCookie(w, pendingCookie, "", -1)
		s.redirect(w, r, withNotice(withReturn(Prefix+"login", p.Return), "expired"))
	}
	if ok, wait := s.limit.allow(actLogin, addr, now); !ok {
		p.Error = "Too many login attempts from your network. Try again in " + seconds(wait) + "."
		s.render(w, http.StatusTooManyRequests, "code.html", p)
		return
	}
	hash, ok := tokenHash(s.cookie(r, pendingCookie))
	if !ok {
		expired()
		return
	}
	// Each post uses one of the pending login's attempts, right or wrong.
	pending, err := s.q.AttemptPendingLogin(ctx, db.AttemptPendingLoginParams{TokenHash: hash, MaxAttempts: s.cfg.MaxCodeAttempts})
	if errors.Is(err, pgx.ErrNoRows) {
		expired()
		return
	}
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	wait, _, release := s.limit.reserve(pending.AccountName, addr, now)
	defer release()
	if wait > 0 {
		p.Error = "Too many failed logins for this account. Try again in " + seconds(wait) + "."
		s.render(w, http.StatusTooManyRequests, "code.html", p)
		return
	}
	factor, err := s.checkSecondFactor(ctx, pending.AccountID, r.PostForm.Get("code"))
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if factor == "" {
		s.limit.fail(pending.AccountName, addr, now)
		row, err := s.q.PendingLogin(ctx, db.PendingLoginParams{TokenHash: hash, MaxAttempts: s.cfg.MaxCodeAttempts})
		if errors.Is(err, pgx.ErrNoRows) {
			expired()
			return
		}
		if err != nil {
			s.unavailable(w, r, err)
			return
		}
		p.AttemptsLeft = s.cfg.MaxCodeAttempts - row.Attempts
		p.Error = "That code is wrong, or was already used."
		s.render(w, http.StatusBadRequest, "code.html", p)
		return
	}
	if err := s.q.DeletePendingLogin(ctx, hash); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.setCookie(w, pendingCookie, "", -1)
	s.limit.succeed(pending.AccountName)
	if text, banned, err := s.ban(ctx, pending.AccountID); err != nil || banned {
		if err != nil {
			s.unavailable(w, r, err)
			return
		}
		lp := s.loginPage(w, r)
		lp.Error = text
		s.render(w, http.StatusForbidden, "login.html", lp)
		return
	}
	if err := s.startSession(ctx, w, r, pending.AccountID, []string{pending.Factor, factor}); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.redirect(w, r, withReturn(Prefix+"characters", p.Return))
}

// withNotice adds ?notice= to a path of the service.
func withNotice(path, notice string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "notice=" + url.QueryEscape(notice)
}

// recent reports whether the player proved who they are within window
// before now: at login, or on the reauth page.
func recent(authenticated, now time.Time, window time.Duration) bool {
	return !authenticated.IsZero() && !authenticated.After(now.Add(time.Minute)) && now.Sub(authenticated) <= window
}

// needRecent sends the player to the reauth page, and returns false,
// when their session did not prove who they are recently. next is the
// page to come back to.
func (s *Service) needRecent(w http.ResponseWriter, r *http.Request, sess db.UseSessionRow, next string) bool {
	if recent(sess.AuthenticatedAt.Time, time.Now(), s.cfg.ReauthWindow) {
		return true
	}
	s.redirect(w, r, Prefix+"reauth?"+url.Values{"next": {next}}.Encode())
	return false
}

// AdminPath is the admin screens' path. The reauth page may go back
// there.
const AdminPath = "/admin"

// safeNext returns a path under Prefix, or the admin screens, to go to
// after the reauth page, or the settings page for anything else.
func safeNext(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || raw == "" || len(raw) > 512 || u.Scheme != "" || u.Host != "" || u.User != nil ||
		!strings.HasPrefix(u.Path, Prefix) && u.Path != AdminPath && !strings.HasPrefix(u.Path, AdminPath+"/") ||
		strings.Contains(raw, "\\") || strings.HasPrefix(raw, "//") || strings.Contains(u.Path, "..") {
		return Prefix + "settings"
	}
	return u.RequestURI()
}

func (s *Service) reauthPage(ctx context.Context, w http.ResponseWriter, r *http.Request, sess db.UseSessionRow) (*page, error) {
	p := s.newPage(w, r, "Confirm it is you")
	p.Account = sess.AccountName
	p.Next = safeNext(r.FormValue("next"))
	on, err := s.totpOn(ctx, sess.AccountID)
	if err != nil {
		return nil, err
	}
	p.TOTPOn = on
	if s.wa != nil {
		p.Script, p.Passkey = true, "reauth"
	}
	return p, nil
}

func (s *Service) getReauth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p, err := s.reauthPage(ctx, w, r, sess)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "reauth.html", p)
}

// postReauth checks the password, and the TOTP code when TOTP is on,
// then marks the session as recently authenticated.
func (s *Service) postReauth(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p, err := s.reauthPage(ctx, w, r, sess)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	status, msg, err := s.checkIdentity(ctx, r, sess, p.TOTPOn)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if msg != "" {
		p.Error = msg
		s.render(w, status, "reauth.html", p)
		return
	}
	if err := s.q.Reauthenticate(ctx, sess.ID); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.redirect(w, r, p.Next)
}

// checkIdentity checks the password in a form, and its code when
// withCode, for the player of a session, with the login rate limits. It
// returns an error message and status for the player, or "" when both
// pass.
func (s *Service) checkIdentity(ctx context.Context, r *http.Request, sess db.UseSessionRow, withCode bool) (int, string, error) {
	addr, now := s.clientAddr(r), time.Now()
	name := sess.AccountName
	if ok, wait := s.limit.allow(actLogin, addr, now); !ok {
		return http.StatusTooManyRequests, "Too many attempts from your network. Try again in " + seconds(wait) + ".", nil
	}
	wait, _, release := s.limit.reserve(name, addr, now)
	defer release()
	if wait > 0 {
		return http.StatusTooManyRequests, "Too many failed attempts for this account. Try again in " + seconds(wait) + ".", nil
	}
	password := r.PostForm.Get("password")
	ok := false
	if password != "" && len(password) <= MaxPassword {
		hash, err := s.q.PasswordByAccount(ctx, sess.AccountID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, "", err
		}
		if err == nil {
			good, _, err := s.hasher.Verify(ctx, hash, password)
			if err != nil && !errors.Is(err, errBadHash) {
				return 0, "", err
			}
			ok = good
		}
	}
	if ok && withCode {
		factor, err := s.checkSecondFactor(ctx, sess.AccountID, r.PostForm.Get("code"))
		if err != nil {
			return 0, "", err
		}
		ok = factor != ""
	}
	if !ok {
		s.limit.fail(name, addr, now)
		if withCode {
			return http.StatusBadRequest, "The password or the code is wrong.", nil
		}
		return http.StatusBadRequest, "The password is wrong.", nil
	}
	s.limit.succeed(name)
	return 0, "", nil
}

// factorNames writes a session's factors for the settings page.
func factorNames(factors []string) string {
	names := map[string]string{
		factorPassword: "password", factorTOTP: "authenticator code",
		factorRecovery: "recovery code", factorPasskey: "passkey",
	}
	var out []string
	for _, f := range factors {
		if n, ok := names[f]; ok {
			out = append(out, n)
		}
	}
	return strings.Join(out, " and ")
}
