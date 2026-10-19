package account

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"time"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// What the admin API needs from the account service (decision 0028).
// The admin API runs on the account service's origin, with its session
// cookie, CSRF tokens and rate limits.

// Session is a live login session.
type Session struct {
	ID, AccountID int64
	AccountName   string
	// Factors are what the player proved at login, such as password and
	// totp.
	Factors []string
	// AuthenticatedAt is when the player last proved who they are: at
	// login, or on the reauth page.
	AuthenticatedAt time.Time
}

// SecondFactor reports whether the session logged in with a second
// factor: a TOTP code, a recovery code or a passkey that verified its
// user. A passkey without user verification proves possession only, so
// it does not count (review decision 54).
func (s Session) SecondFactor() bool { return secondFactor(s.Factors) }

func secondFactor(factors []string) bool {
	return slices.ContainsFunc(factors, func(f string) bool {
		return f == factorTOTP || f == factorRecovery || f == factorPasskeyVerified
	})
}

// Session returns the request's live login session, or false when it
// has none.
func (s *Service) Session(ctx context.Context, r *http.Request) (Session, bool, error) {
	row, ok, err := s.session(ctx, r)
	if !ok || err != nil {
		return Session{}, false, err
	}
	return sessionOf(row), true, nil
}

func sessionOf(row db.UseSessionRow) Session {
	return Session{ID: row.ID, AccountID: row.AccountID, AccountName: row.AccountName, Factors: row.Factors,
		AuthenticatedAt: row.AuthenticatedAt.Time}
}

// Recent reports whether a session's player proved who they are within
// the reauth window, 10 minutes by default.
func (s *Service) Recent(sess Session) bool {
	return recent(sess.AuthenticatedAt, time.Now(), s.cfg.ReauthWindow)
}

// TwoFactorOn reports whether an account has TOTP or a passkey on.
func (s *Service) TwoFactorOn(ctx context.Context, account int64) (bool, error) {
	return s.q.TwoFactorOn(ctx, account)
}

// CSRFToken returns the CSRF token of the request's cookies, setting the
// CSRF cookie when it has none. A script sends it back in the
// X-CSRF-Token header.
func (s *Service) CSRFToken(w http.ResponseWriter, r *http.Request) string {
	return s.csrfToken(s.ensureCSRF(w, r), s.cookie(r, sessionCookie))
}

// CheckCSRF checks a change: the browser's Sec-Fetch-Site or Origin, and
// the token, as the account pages' forms are checked.
func (s *Service) CheckCSRF(r *http.Request, token string) bool { return s.checkCSRFValue(r, token) }

// ClientAddr is the client's address, through trusted proxies.
func (s *Service) ClientAddr(r *http.Request) netip.Addr { return s.clientAddr(r) }

// AllowAdmin takes one token from the address's bucket of admin API
// calls. It returns how long to wait when the bucket is empty.
func (s *Service) AllowAdmin(r *http.Request) (bool, time.Duration) {
	return s.limit.allow(actAdmin, s.clientAddr(r), time.Now())
}

// SecurityHeaders sets the account service's security headers.
func (s *Service) SecurityHeaders(w http.ResponseWriter) { s.securityHeaders(w) }

// ReauthURL is the reauth page, which comes back to next afterwards.
// next must be a path of the account pages or the admin screens.
func ReauthURL(next string) string {
	return Prefix + "reauth?" + url.Values{"next": {safeNext(next)}}.Encode()
}

// LoginURL is the login page.
func LoginURL() string { return Prefix + "login" }
