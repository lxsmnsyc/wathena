package account

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Passkeys (WebAuthn):
//   - The relying party ID is the host of PublicURL, so a passkey only
//     works on the account service's own domain. A client fork on
//     another domain cannot ask for it.
//   - Passkeys are discoverable, so login needs no account name.
//   - User verification is preferred. A passkey login that verified the
//     user (a PIN, a fingerprint) counts as both factors, and its session
//     records passkey_verified. One that did not asks for the TOTP code
//     too, when TOTP is on. Without TOTP it logs in with the factor
//     passkey alone, which does not count for a staff role (review
//     decision 54).
//   - The signature counter must go up when it is used. A passkey whose
//     counter does not is marked with a clone warning and refused.
//   - A ceremony's challenge is kept in the database, under the hash of
//     a random token in a cookie, and can be answered once.
//
// The browser side is static/passkey.js, the only script of the account
// pages.

const maxPasskeyBody = 64 << 10

// setupWebAuthn configures passkeys for PublicURL's host. An IP address
// cannot be a relying party ID, so passkeys stay off for one.
func (s *Service) setupWebAuthn() error {
	u, err := url.Parse(s.cfg.PublicURL)
	if err != nil {
		return err
	}
	host := u.Hostname()
	if net.ParseIP(host) != nil {
		s.log.Warn("passkeys are off: the account service's public URL has an IP address, not a domain name")
		return nil
	}
	timeout := webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyLifetime, TimeoutUVD: ceremonyLifetime}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          host,
		RPDisplayName: s.cfg.Issuer,
		RPOrigins:     []string{s.origin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationPreferred,
		},
		AttestationPreference: protocol.PreferNoAttestation,
		Timeouts:              webauthn.TimeoutsConfig{Login: timeout, Registration: timeout},
	})
	if err != nil {
		return err
	}
	s.wa, s.rpID = wa, host
	return nil
}

// waUser is an account as go-webauthn sees it.
type waUser struct {
	account int64
	handle  []byte
	name    string
	creds   []webauthn.Credential
	// rows are the passkeys' row IDs, in the order of creds.
	rows []int64
}

func (u *waUser) WebAuthnID() []byte                         { return u.handle }
func (u *waUser) WebAuthnName() string                       { return u.name }
func (u *waUser) WebAuthnDisplayName() string                { return u.name }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// row returns the row ID of a credential.
func (u *waUser) row(id []byte) int64 {
	for i, c := range u.creds {
		if hmac.Equal(c.ID, id) {
			return u.rows[i]
		}
	}
	return 0
}

// loadUser reads an account's passkeys for this relying party ID. It
// also returns how many passkeys the account has in all.
func (s *Service) loadUser(ctx context.Context, account int64, name string, handle []byte) (*waUser, int, error) {
	rows, err := s.q.AccountPasskeys(ctx, account)
	if err != nil {
		return nil, 0, err
	}
	u := &waUser{account: account, handle: handle, name: name}
	for _, k := range rows {
		if k.RpID != s.rpID {
			continue
		}
		var transports []protocol.AuthenticatorTransport
		for _, t := range k.Transports {
			transports = append(transports, protocol.AuthenticatorTransport(t))
		}
		u.creds = append(u.creds, webauthn.Credential{
			ID: k.CredentialID, PublicKey: k.PublicKey,
			AttestationType: k.AttestationType, AttestationFormat: k.AttestationFormat,
			Transport: transports,
			Flags:     webauthn.NewCredentialFlags(protocol.AuthenticatorFlags(k.Flags)), // #nosec G115 -- a CHECK keeps flags within a byte.
			Authenticator: webauthn.Authenticator{
				AAGUID: k.Aaguid, SignCount: uint32(k.SignCount), // #nosec G115 -- a CHECK keeps sign_count within 32 bits.
				CloneWarning: k.CloneWarning, Attachment: protocol.AuthenticatorAttachment(k.Attachment),
			},
		})
		u.rows = append(u.rows, k.ID)
	}
	return u, len(rows), nil
}

// jsonError answers a script's request with an error code, and a
// message for the player when there is one.
func jsonError(w http.ResponseWriter, status int, code, message string) {
	body := map[string]string{"error": code}
	if message != "" {
		body["message"] = message
	}
	writeJSON(w, status, body)
}

// passkeyPost starts a request from passkey.js: passkeys must be on, and
// the X-CSRF-Token header must match the cookies.
func (s *Service) passkeyPost(w http.ResponseWriter, r *http.Request) bool {
	if s.wa == nil {
		jsonError(w, http.StatusNotFound, "off", "Passkeys are not set up on this server.")
		return false
	}
	if !s.checkCSRFValue(r, r.Header.Get("X-CSRF-Token")) {
		jsonError(w, http.StatusForbidden, "csrf", "The page expired. Reload it and try again.")
		return false
	}
	return true
}

// loggedInJSON returns the request's session, or answers 401.
func (s *Service) loggedInJSON(ctx context.Context, w http.ResponseWriter, r *http.Request) (db.UseSessionRow, bool) {
	sess, ok, err := s.session(ctx, r)
	if err != nil {
		s.log.Error("cannot check a session", "err", err)
		jsonError(w, http.StatusServiceUnavailable, "unavailable", "The server cannot do this right now. Try again later.")
		return sess, false
	}
	if !ok {
		jsonError(w, http.StatusUnauthorized, "login", "Your login ended. Log in again.")
		return sess, false
	}
	return sess, true
}

// recentJSON answers 403 with the reauth page's address when the
// session did not prove who it is recently.
func (s *Service) recentJSON(w http.ResponseWriter, sess db.UseSessionRow) bool {
	if recent(sess.AuthenticatedAt.Time, time.Now(), s.cfg.ReauthWindow) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "reauth", "next": Prefix + "reauth?" + url.Values{"next": {Prefix + "settings"}}.Encode(),
	})
	return false
}

// readJSON reads a small JSON body.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(v); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid", "The request could not be read.")
		return false
	}
	return true
}

// passkeyName cleans the name a player gives a passkey. It returns ""
// for a name that cannot be used.
func passkeyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Passkey"
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 64 {
		return ""
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return ""
		}
	}
	return name
}

// saveCeremony keeps a ceremony's state until its second step, under a
// new token in the ceremony cookie.
func (s *Service) saveCeremony(ctx context.Context, w http.ResponseWriter, kind string, session int64, name string, sd *webauthn.SessionData) error {
	data, err := json.Marshal(sd)
	if err != nil {
		return err
	}
	token, hash := newToken()
	err = s.q.CreateCeremony(ctx, db.CreateCeremonyParams{
		TokenHash: hash, Kind: kind, SessionID: pgtype.Int8{Int64: session, Valid: session != 0},
		Name: name, Data: data,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(ceremonyLifetime), Valid: true},
	})
	if err != nil {
		return err
	}
	s.setCookie(w, ceremonyCookie, token, int(ceremonyLifetime.Seconds()))
	return nil
}

// ceremony is a ceremony's state, as saveCeremony kept it.
type ceremony struct {
	kind    string
	session int64
	name    string
	data    webauthn.SessionData
}

// takeCeremony uses up the ceremony that the request's cookie names. It
// answers the request and returns false when there is none.
func (s *Service) takeCeremony(ctx context.Context, w http.ResponseWriter, r *http.Request) (ceremony, bool) {
	s.setCookie(w, ceremonyCookie, "", -1)
	hash, ok := tokenHash(s.cookie(r, ceremonyCookie))
	if !ok {
		jsonError(w, http.StatusBadRequest, "expired", "The passkey prompt expired. Try again.")
		return ceremony{}, false
	}
	row, err := s.q.TakeCeremony(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !row.Live {
		jsonError(w, http.StatusBadRequest, "expired", "The passkey prompt expired. Try again.")
		return ceremony{}, false
	}
	if err != nil {
		s.log.Error("cannot read a passkey ceremony", "err", err)
		jsonError(w, http.StatusServiceUnavailable, "unavailable", "The server cannot do this right now. Try again later.")
		return ceremony{}, false
	}
	c := ceremony{kind: row.Kind, session: row.SessionID.Int64, name: row.Name}
	if err := json.Unmarshal(row.Data, &c.data); err != nil {
		s.log.Error("cannot read a passkey ceremony", "err", err)
		jsonError(w, http.StatusServiceUnavailable, "unavailable", "The server cannot do this right now. Try again later.")
		return ceremony{}, false
	}
	return c, true
}

func (s *Service) passkeyUnavailable(w http.ResponseWriter, err error) {
	s.log.Error("passkey request failed", "err", err)
	jsonError(w, http.StatusServiceUnavailable, "unavailable", "The server cannot do this right now. Try again later.")
}

// beginRegister starts adding a passkey to the session's account. It
// needs a recent login.
func (s *Service) beginRegister(w http.ResponseWriter, r *http.Request) {
	if !s.passkeyPost(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedInJSON(ctx, w, r)
	if !ok || !s.recentJSON(w, sess) {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	name := passkeyName(body.Name)
	if name == "" {
		jsonError(w, http.StatusBadRequest, "name", "Use at most 64 letters, digits and spaces for the name.")
		return
	}
	fresh := make([]byte, 32)
	_, _ = rand.Read(fresh)
	handle, err := s.q.WebAuthnHandle(ctx, db.WebAuthnHandleParams{AccountID: sess.AccountID, NewHandle: fresh})
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	user, all, err := s.loadUser(ctx, sess.AccountID, sess.AccountName, handle)
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	if all >= MaxPasskeys {
		jsonError(w, http.StatusConflict, "full", "An account can have 10 passkeys. Remove one first.")
		return
	}
	creation, sd, err := s.wa.BeginRegistration(user, webauthn.WithExclusions(webauthn.Credentials(user.creds).CredentialDescriptors()))
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	if err := s.saveCeremony(ctx, w, "register", sess.ID, name, sd); err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	writeJSON(w, http.StatusOK, creation)
}

// finishRegister checks the new passkey and stores it.
func (s *Service) finishRegister(w http.ResponseWriter, r *http.Request) {
	if !s.passkeyPost(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedInJSON(ctx, w, r)
	if !ok || !s.recentJSON(w, sess) {
		return
	}
	c, ok := s.takeCeremony(ctx, w, r)
	if !ok {
		return
	}
	if c.kind != "register" || c.session != sess.ID {
		jsonError(w, http.StatusBadRequest, "expired", "The passkey prompt expired. Try again.")
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(http.MaxBytesReader(w, r.Body, maxPasskeyBody))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid", "The browser sent a passkey the server cannot read.")
		return
	}
	handle, err := s.q.WebAuthnHandleOf(ctx, sess.AccountID)
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	user, _, err := s.loadUser(ctx, sess.AccountID, sess.AccountName, handle)
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	cred, err := s.wa.CreateCredential(user, c.data, parsed)
	if err != nil {
		s.log.Info("a passkey was refused", "account", sess.AccountID, "err", err)
		jsonError(w, http.StatusBadRequest, "invalid", "The passkey could not be checked. Try again.")
		return
	}
	var transports []string
	for _, t := range cred.Transport {
		if len(transports) < 8 && len(t) <= 32 {
			transports = append(transports, string(t))
		}
	}
	var aaguid []byte
	if len(cred.Authenticator.AAGUID) == 16 {
		aaguid = cred.Authenticator.AAGUID
	}
	n, err := s.q.CreatePasskey(ctx, db.CreatePasskeyParams{
		AccountID: sess.AccountID, RpID: s.rpID, CredentialID: cred.ID, PublicKey: cred.PublicKey,
		AttestationType: shorten(cred.AttestationType, 32), AttestationFormat: shorten(cred.AttestationFormat, 32),
		Aaguid: aaguid, Transports: transports, Attachment: shorten(string(cred.Authenticator.Attachment), 32),
		Flags: int(cred.Flags.ProtocolValue()), SignCount: int64(cred.Authenticator.SignCount),
		Name: c.name, MaxPasskeys: MaxPasskeys,
	})
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	if n == 0 {
		jsonError(w, http.StatusConflict, "full", "An account can have 10 passkeys, and each passkey can be added once.")
		return
	}
	s.log.Info("passkey added", "account", sess.AccountID)
	s.afterLoginChange(ctx, sess, "passkey_added")
	writeJSON(w, http.StatusOK, map[string]string{"next": withNotice(Prefix+"settings", "passkey-added")})
}

// beginPasskeyLogin starts a passkey login, or, with purpose "reauth",
// a passkey check for the session's own account.
func (s *Service) beginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.passkeyPost(w, r) {
		return
	}
	if ok, _ := s.limit.allow(actLogin, s.clientAddr(r), time.Now()); !ok {
		jsonError(w, http.StatusTooManyRequests, "rate", "Too many login attempts from your network. Wait a moment and try again.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	var body struct {
		Purpose string `json:"purpose"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	uv := webauthn.WithUserVerification(protocol.VerificationPreferred)
	if body.Purpose != "reauth" {
		assertion, sd, err := s.wa.BeginDiscoverableLogin(uv)
		if err == nil {
			err = s.saveCeremony(ctx, w, "login", 0, "", sd)
		}
		if err != nil {
			s.passkeyUnavailable(w, err)
			return
		}
		writeJSON(w, http.StatusOK, assertion)
		return
	}
	sess, ok := s.loggedInJSON(ctx, w, r)
	if !ok {
		return
	}
	user, err := s.sessionUser(ctx, sess)
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	if user == nil || len(user.creds) == 0 {
		jsonError(w, http.StatusConflict, "none", "This account has no passkey yet. Use your password.")
		return
	}
	assertion, sd, err := s.wa.BeginLogin(user, uv)
	if err == nil {
		err = s.saveCeremony(ctx, w, "reauth", sess.ID, "", sd)
	}
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	writeJSON(w, http.StatusOK, assertion)
}

// sessionUser returns the session's account with its passkeys, or nil
// when it never had one.
func (s *Service) sessionUser(ctx context.Context, sess db.UseSessionRow) (*waUser, error) {
	handle, err := s.q.WebAuthnHandleOf(ctx, sess.AccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	user, _, err := s.loadUser(ctx, sess.AccountID, sess.AccountName, handle)
	return user, err
}

// finishPasskeyLogin checks a passkey's answer. For a login it starts a
// session, or a pending login when the passkey did not verify the user
// and TOTP is on. For a reauth it marks the session as recently
// authenticated.
func (s *Service) finishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if !s.passkeyPost(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	addr := s.clientAddr(r)
	c, ok := s.takeCeremony(ctx, w, r)
	if !ok {
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(http.MaxBytesReader(w, r.Body, maxPasskeyBody))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid", "The browser sent an answer the server cannot read.")
		return
	}
	refused := func(err error) {
		s.log.Info("a passkey login was refused", "err", err)
		s.limit.failAddr(addr, time.Now())
		jsonError(w, http.StatusBadRequest, "invalid", "That passkey did not work. It may have been removed from this account.")
	}
	var user *waUser
	var cred *webauthn.Credential
	var sess db.UseSessionRow
	switch c.kind {
	case "login":
		handler := func(rawID, userHandle []byte) (webauthn.User, error) {
			owner, err := s.q.PasskeyOwner(ctx, db.PasskeyOwnerParams{CredentialID: rawID, RpID: s.rpID})
			if err != nil {
				return nil, err
			}
			if !hmac.Equal(owner.UserHandle, userHandle) {
				return nil, errors.New("the user handle is not the passkey's")
			}
			user, _, err = s.loadUser(ctx, owner.AccountID, owner.AccountName, owner.UserHandle)
			return user, err
		}
		_, cred, err = s.wa.ValidatePasskeyLogin(handler, c.data, parsed)
	case "reauth":
		if sess, ok = s.loggedInJSON(ctx, w, r); !ok {
			return
		}
		if c.session != sess.ID {
			jsonError(w, http.StatusBadRequest, "expired", "The passkey prompt expired. Try again.")
			return
		}
		if user, err = s.sessionUser(ctx, sess); err == nil && user == nil {
			err = errors.New("the account has no passkey")
		}
		if err == nil {
			cred, err = s.wa.ValidateLogin(user, c.data, parsed)
		}
	default:
		jsonError(w, http.StatusBadRequest, "expired", "The passkey prompt expired. Try again.")
		return
	}
	if err != nil {
		refused(err)
		return
	}
	if err := s.q.UsePasskey(ctx, db.UsePasskeyParams{
		ID: user.row(cred.ID), SignCount: int64(cred.Authenticator.SignCount),
		Flags: int(cred.Flags.ProtocolValue()), CloneWarning: cred.Authenticator.CloneWarning,
	}); err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	if cred.Authenticator.CloneWarning {
		s.log.Warn("a passkey's signature counter did not go up; it may be copied", "account", user.account)
		jsonError(w, http.StatusForbidden, "clone", "This passkey may have been copied, so it is blocked. Log in with your password, then remove it and add it again.")
		return
	}
	verified := parsed.Response.AuthenticatorData.Flags.HasUserVerified()
	on, err := s.totpOn(ctx, user.account)
	if err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	if c.kind == "reauth" {
		if !verified && on {
			jsonError(w, http.StatusForbidden, "verify", "This passkey did not check a PIN or fingerprint. Use your password and code instead.")
			return
		}
		if err := s.q.Reauthenticate(ctx, sess.ID); err != nil {
			s.passkeyUnavailable(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"next": safeNext(queryValue(r, "next"))})
		return
	}
	ret := s.safeReturn(queryValue(r, "return"))
	if text, banned, err := s.ban(ctx, user.account); err != nil || banned {
		if err != nil {
			s.passkeyUnavailable(w, err)
			return
		}
		jsonError(w, http.StatusForbidden, "banned", text)
		return
	}
	if !verified && on {
		// The passkey proved possession only. The TOTP code is still
		// needed.
		if err := s.startPending(ctx, w, user.account, factorPasskey); err != nil {
			s.passkeyUnavailable(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"next": withReturn(Prefix+"login/code", ret)})
		return
	}
	s.limit.succeed(user.name)
	factors := []string{factorPasskey}
	if verified {
		factors = append(factors, factorPasskeyVerified)
	}
	if err := s.startSession(ctx, w, r, user.account, factors); err != nil {
		s.passkeyUnavailable(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"next": withReturn(Prefix+"characters", ret)})
}

// queryValue returns a value of the request's query string. Posts from
// passkey.js carry return and next there, since their body is the
// browser's answer.
func queryValue(r *http.Request, key string) string {
	v, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return ""
	}
	return v.Get(key)
}
