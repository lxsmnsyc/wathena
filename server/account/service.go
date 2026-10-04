// Package account is the account service: signup, login and logout,
// two-factor login with TOTP codes, passkeys, login sessions, the
// character list with character creation, and the one-time game tickets
// that open a game connection. See
// docs/decisions/0004-account-service-and-game-tickets.md and
// docs/decisions/0005-two-factor-login-and-passkeys.md.
//
// Its pages are plain HTML forms. Only the pages with passkey buttons
// load a script, from the service's own origin. It runs its queries as
// the wathena_account role, the only role that can read password hashes,
// TOTP secrets and passkeys.
package account

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Prefix is the path the service's pages live under.
const Prefix = "/account/"

// Look is what a player picks for a new character.
type Look struct {
	Male            bool
	Hair, HairColor int
}

// Hair styles and colors the character page offers. They are the ones
// the placeholder and classic asset packs draw for every job.
const (
	MinHair      = 1
	MaxHair      = 20
	MaxHairColor = 8
)

// Defaults for Config.
const (
	DefaultSessionLifetime = 30 * 24 * time.Hour
	DefaultSessionIdle     = 7 * 24 * time.Hour
	DefaultTicketLifetime  = 60 * time.Second
	DefaultSlots           = 9
	DefaultHashers         = 4
	DefaultReauthWindow    = 10 * time.Minute
	DefaultPendingLifetime = 5 * time.Minute
	DefaultCodeAttempts    = 5
	// ceremonyLifetime is how long a passkey prompt may take.
	ceremonyLifetime = 5 * time.Minute
	// MaxPasskeys is how many passkeys an account may have.
	MaxPasskeys  = 10
	cleanupEvery = 10 * time.Minute
	// requestTimeout bounds the database and hashing work of one page.
	requestTimeout = 10 * time.Second
	// maxForm is the largest form body read.
	maxForm = 16 << 10
)

// Config configures a Service.
type Config struct {
	// Pool connects as the wathena_account role.
	Pool *pgxpool.Pool
	// PublicURL is the service's own URL as players reach it, such as
	// https://play.example.com. Its scheme decides whether cookies are
	// Secure, and its origin is the one forms must come from. When
	// empty, the origin comes from each request's Host, and cookies are
	// not Secure.
	PublicURL string
	// ClientOrigins are the web client origins the server owner trusts,
	// such as https://play.example.com. Only they may fetch game
	// tickets, and Play only sends players to them. A client origin
	// must be on the same site as PublicURL, or the browser does not
	// send the SameSite=Strict session cookie with the ticket fetch.
	ClientOrigins []string
	// PlayURL is where Play sends the player when the page was not
	// opened from a client. Its origin must be in ClientOrigins.
	PlayURL string
	// NewCharacter makes a new level 1 character with its start stats,
	// position and items. waserver passes the zone's.
	NewCharacter func(name string, look Look) character.Character
	// CheckName checks a new character's name against the chat filter
	// and the reserved names (strategy section 7.10). It may be nil.
	CheckName func(name string) error
	// Captcha is asked for after repeated failures. NoCaptcha when nil.
	Captcha Captcha
	// Hasher hashes passwords. When nil, DefaultParams with
	// DefaultHashers at once.
	Hasher *Hasher
	// Limits are the rate limits. DefaultLimits when zero.
	Limits Limits
	// TrustedProxies are the proxies whose X-Forwarded-For is believed.
	TrustedProxies []netip.Prefix
	// SessionLifetime is the longest a login session lasts, and
	// SessionIdle the longest it lasts unused. TicketLifetime is how long
	// a game ticket waits to be used.
	SessionLifetime, SessionIdle, TicketLifetime time.Duration
	// Slots is how many characters an account may have, at most 15.
	Slots int
	Log   *slog.Logger

	// Sealer encrypts TOTP secrets in the database. Without it, players
	// cannot turn TOTP on, and accounts that have it on cannot log in
	// until it is given.
	Sealer *Sealer
	// Issuer names the server in authenticator apps. "wAthena" when
	// empty.
	Issuer string
	// Guilds lets guild masters upload emblems. Without it the emblem
	// page lists no guild.
	Guilds Guilds
	// Ended is told which login sessions ended, such as at logout, so it
	// can close the game connections they opened. The gateway is one.
	Ended SessionEnder
	// ReauthWindow is how long after logging in, or proving who they are
	// again, a player may change their login methods without being asked
	// again. DefaultReauthWindow when 0.
	ReauthWindow time.Duration
	// PendingLifetime is how long a login waits for its TOTP code, and
	// MaxCodeAttempts how many codes it may try. DefaultPendingLifetime
	// and DefaultCodeAttempts when 0.
	PendingLifetime time.Duration
	MaxCodeAttempts int
	// Clock gives the time that TOTP codes are checked against. time.Now
	// when nil. Tests set it.
	Clock func() time.Time
}

// SessionEnder closes what a login session opened when the session
// ends. *gateway.Gateway is one. A PostgreSQL LISTEN/NOTIFY bridge can
// be one when the gateway runs in another process.
type SessionEnder interface {
	EndLoginSessions(ids ...int64)
}

// Service serves the account pages. It is an http.Handler for the paths
// under Prefix.
type Service struct {
	cfg     Config
	q       *db.Queries
	hasher  *Hasher
	captcha Captcha
	limit   *limiter
	mux     *http.ServeMux
	log     *slog.Logger

	secure    bool
	origin    string
	clients   map[string]bool
	csp       string
	cspScript string
	csrfKey   []byte
	// wa runs passkey ceremonies. It is nil when PublicURL is not set,
	// since a passkey needs a fixed relying party ID.
	wa   *webauthn.WebAuthn
	rpID string
}

// New makes a service.
func New(cfg Config) (*Service, error) {
	if cfg.Pool == nil || cfg.NewCharacter == nil {
		return nil, errors.New("account: Pool and NewCharacter are required")
	}
	s := &Service{cfg: cfg, q: db.New(cfg.Pool), hasher: cfg.Hasher, captcha: cfg.Captcha, log: cfg.Log, clients: map[string]bool{}}
	if s.hasher == nil {
		s.hasher = NewHasher(DefaultParams, DefaultHashers)
	}
	if s.captcha == nil {
		s.captcha = NoCaptcha{}
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.cfg.Limits == (Limits{}) {
		s.cfg.Limits = DefaultLimits
	}
	s.limit = newLimiter(s.cfg.Limits)
	if s.cfg.SessionLifetime <= 0 {
		s.cfg.SessionLifetime = DefaultSessionLifetime
	}
	if s.cfg.SessionIdle <= 0 {
		s.cfg.SessionIdle = DefaultSessionIdle
	}
	if s.cfg.TicketLifetime <= 0 {
		s.cfg.TicketLifetime = DefaultTicketLifetime
	}
	if s.cfg.Slots <= 0 {
		s.cfg.Slots = DefaultSlots
	}
	s.cfg.Slots = min(s.cfg.Slots, 15)
	if s.cfg.ReauthWindow <= 0 {
		s.cfg.ReauthWindow = DefaultReauthWindow
	}
	if s.cfg.PendingLifetime <= 0 {
		s.cfg.PendingLifetime = DefaultPendingLifetime
	}
	if s.cfg.MaxCodeAttempts <= 0 {
		s.cfg.MaxCodeAttempts = DefaultCodeAttempts
	}
	if s.cfg.Issuer == "" {
		s.cfg.Issuer = "wAthena"
	}
	if s.cfg.Clock == nil {
		s.cfg.Clock = time.Now
	}
	if cfg.PublicURL != "" {
		if s.origin = originOf(cfg.PublicURL); s.origin == "" {
			return nil, errors.New("account: PublicURL is not an http or https URL")
		}
		s.secure = strings.HasPrefix(s.origin, "https://")
		if err := s.setupWebAuthn(); err != nil {
			return nil, err
		}
	}
	var clients []string
	for _, o := range cfg.ClientOrigins {
		if originOf(o) != o {
			return nil, errors.New("account: a client origin is not scheme://host[:port]: " + o)
		}
		s.clients[o] = true
		clients = append(clients, o)
	}
	if cfg.PlayURL != "" && !s.clients[originOf(cfg.PlayURL)] {
		return nil, errors.New("account: PlayURL's origin is not in ClientOrigins")
	}
	s.csp = buildCSP(clients, s.captcha.Sources(), false)
	s.cspScript = buildCSP(clients, s.captcha.Sources(), true)
	s.csrfKey = make([]byte, 32)
	if _, err := rand.Read(s.csrfKey); err != nil {
		return nil, err
	}
	s.routes()
	return s, nil
}

func (s *Service) routes() {
	m := http.NewServeMux()
	m.HandleFunc("GET /account/{$}", s.index)
	m.HandleFunc("GET /account/style.css", s.style)
	m.HandleFunc("GET /account/passkey.js", s.script)
	m.HandleFunc("GET /account/login", s.getLogin)
	m.HandleFunc("POST /account/login", s.postLogin)
	m.HandleFunc("GET /account/login/code", s.getLoginCode)
	m.HandleFunc("POST /account/login/code", s.postLoginCode)
	m.HandleFunc("GET /account/reauth", s.getReauth)
	m.HandleFunc("POST /account/reauth", s.postReauth)
	m.HandleFunc("GET /account/settings", s.getSettings)
	m.HandleFunc("POST /account/settings/totp/start", s.startTOTP)
	m.HandleFunc("GET /account/settings/totp", s.getTOTP)
	m.HandleFunc("POST /account/settings/totp", s.confirmTOTP)
	m.HandleFunc("POST /account/settings/totp/disable", s.disableTOTP)
	m.HandleFunc("POST /account/settings/recovery", s.newRecovery)
	m.HandleFunc("POST /account/settings/passkeys/remove", s.removePasskey)
	m.HandleFunc("POST /account/passkeys/register/begin", s.beginRegister)
	m.HandleFunc("POST /account/passkeys/register/finish", s.finishRegister)
	m.HandleFunc("POST /account/passkeys/login/begin", s.beginPasskeyLogin)
	m.HandleFunc("POST /account/passkeys/login/finish", s.finishPasskeyLogin)
	m.HandleFunc("GET /account/signup", s.getSignup)
	m.HandleFunc("POST /account/signup", s.postSignup)
	m.HandleFunc("POST /account/logout", s.logout)
	m.HandleFunc("GET /account/characters", s.getCharacters)
	m.HandleFunc("POST /account/characters", s.postCharacters)
	m.HandleFunc("POST /account/play", s.play)
	m.HandleFunc("GET /account/guild/emblem", s.getEmblem)
	m.HandleFunc("POST /account/guild/emblem", s.postEmblem)
	m.HandleFunc("POST /account/ticket", s.ticket)
	m.HandleFunc("OPTIONS /account/ticket", s.ticket)
	s.mux = m
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.securityHeaders(w)
	s.mux.ServeHTTP(w, r)
}

// Run deletes expired sessions and tickets, and forgets old rate limit
// counts, every 10 minutes until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(cleanupEvery)
	defer t.Stop()
	for {
		s.Cleanup(ctx)
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

// Cleanup deletes expired and revoked sessions and expired tickets once.
func (s *Service) Cleanup(ctx context.Context) {
	s.limit.sweep(time.Now())
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	n, err := s.q.DeleteExpired(ctx, interval(s.cfg.SessionIdle))
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("cannot delete expired sessions", "err", err)
		}
		return
	}
	if n > 0 {
		s.log.Info("deleted expired sessions", "count", n)
	}
}

func interval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}

//go:embed templates/*.html static/style.css static/passkey.js
var files embed.FS

var pages = template.Must(template.ParseFS(files, "templates/*.html"))

// page is the data every template gets.
type page struct {
	Title   string
	CSRF    string
	Error   string
	Notice  string
	Return  string
	Name    string
	Captcha template.HTML
	Account string
	// Script loads passkey.js, and allows same-origin scripts in the
	// page's Content-Security-Policy. Only pages with passkey buttons
	// set it.
	Script bool
	// Passkey is what the page's passkey button does: login, reauth or
	// register. Empty hides it.
	Passkey string
	// Next is where the reauth page goes afterwards.
	Next string

	// The code and settings pages.
	TOTPOn, TOTPPending, TOTPAvailable bool
	PasskeysAvailable                  bool
	RecoveryLeft                       int64
	Passkeys                           []passkeyView
	Factors                            string
	QR                                 template.HTML
	Key                                string
	Codes                              []string
	AttemptsLeft                       int

	Characters []characterView
	// Guilds are the guilds the account's characters master, on the
	// emblem page.
	Guilds     []guildView
	FreeSlots  []int
	Hairs      []int
	HairColors []int
}

type characterView struct {
	Slot                int
	Name                string
	Job                 string
	BaseLevel, JobLevel int
	Map                 string
}

// newPage starts a page with a CSRF token for the request's cookies.
func (s *Service) newPage(w http.ResponseWriter, r *http.Request, title string) *page {
	csrf := s.ensureCSRF(w, r)
	return &page{Title: title, CSRF: s.csrfToken(csrf, s.cookie(r, sessionCookie)), Return: s.safeReturn(r.FormValue("return"))}
}

func (s *Service) render(w http.ResponseWriter, status int, name string, p *page) {
	if p.Script {
		w.Header().Set("Content-Security-Policy", s.cspScript)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.ExecuteTemplate(w, name, p); err != nil {
		s.log.Error("cannot render a page", "page", name, "err", err)
	}
}

func (s *Service) style(w http.ResponseWriter, _ *http.Request) {
	b, err := files.ReadFile("static/style.css")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "max-age=3600")
	_, _ = w.Write(b)
}

// script serves passkey.js, the one script of the account pages.
func (s *Service) script(w http.ResponseWriter, _ *http.Request) {
	b, err := files.ReadFile("static/passkey.js")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// safeReturn returns a URL to send the player to after Play, when it
// points at a trusted client origin, and "" otherwise.
func (s *Service) safeReturn(raw string) string {
	if raw == "" || len(raw) > 2048 || !s.clients[originOf(raw)] {
		return ""
	}
	return raw
}

// withReturn adds the return URL to a path of the service.
func withReturn(path, ret string) string {
	if ret == "" {
		return path
	}
	return path + "?" + url.Values{"return": {ret}}.Encode()
}

// redirect sends the player on. to is a path of the service, PlayURL, or
// a URL that safeReturn accepted, so never an untrusted site.
func (s *Service) redirect(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther) // #nosec G710 -- see above.
}

// form reads a posted form, at most maxForm bytes, and checks its CSRF
// token. It writes the error page and returns false when either fails.
func (s *Service) form(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxForm)
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusBadRequest, "message.html", &page{Title: "Bad request", Error: "The form could not be read."})
		return false
	}
	if !s.checkCSRF(r) {
		p := s.newPage(w, r, "Try again")
		p.Error = "The form expired or came from another site. Go back, reload the page, and try again. Cookies must be on."
		s.render(w, http.StatusForbidden, "message.html", p)
		return false
	}
	return true
}

// session returns the request's live login session.
func (s *Service) session(ctx context.Context, r *http.Request) (db.UseSessionRow, bool, error) {
	hash, ok := tokenHash(s.cookie(r, sessionCookie))
	if !ok {
		return db.UseSessionRow{}, false, nil
	}
	row, err := s.q.UseSession(ctx, db.UseSessionParams{TokenHash: hash, Idle: interval(s.cfg.SessionIdle)})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.UseSessionRow{}, false, nil
	}
	return row, err == nil, err
}

func (s *Service) index(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	ret := s.safeReturn(r.FormValue("return"))
	if _, ok, err := s.session(ctx, r); err == nil && ok {
		s.redirect(w, r, withReturn(Prefix+"characters", ret))
		return
	}
	s.redirect(w, r, withReturn(Prefix+"login", ret))
}

func (s *Service) getLogin(w http.ResponseWriter, r *http.Request) {
	p := s.loginPage(w, r)
	if s.limit.captchaFor(s.clientAddr(r), time.Now()) {
		p.Captcha = s.captcha.Field()
	}
	if msg, ok := loginNotices[r.FormValue("notice")]; ok {
		p.Notice = msg
	}
	s.render(w, http.StatusOK, "login.html", p)
}

// loginPage starts the login page, with its passkey button when
// passkeys are set up.
func (s *Service) loginPage(w http.ResponseWriter, r *http.Request) *page {
	p := s.newPage(w, r, "Log in")
	if s.wa != nil {
		p.Script, p.Passkey = true, "login"
	}
	return p
}

// loginNotices are the messages the login page shows for ?notice=.
var loginNotices = map[string]string{
	"expired": "The login took too long or had too many wrong codes. Log in again.",
}

// errLogin is the one message for a wrong name or a wrong password, so
// the login form does not tell which accounts exist.
const errLogin = "The account name or password is wrong."

func (s *Service) postLogin(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	addr, now := s.clientAddr(r), time.Now()
	name := strings.TrimSpace(r.PostForm.Get("name"))
	password := r.PostForm.Get("password")
	p := s.loginPage(w, r)
	p.Name = shorten(name, MaxAccountName)
	if ok, wait := s.limit.allow(actLogin, addr, now); !ok {
		p.Error = "Too many login attempts from your network. Try again in " + seconds(wait) + "."
		s.render(w, http.StatusTooManyRequests, "login.html", p)
		return
	}
	wait, needCaptcha := s.limit.check(name, addr, now)
	if needCaptcha {
		p.Captcha = s.captcha.Field()
	}
	if wait > 0 {
		p.Error = "Too many failed logins for this account name. Try again in " + seconds(wait) + "."
		s.render(w, http.StatusTooManyRequests, "login.html", p)
		return
	}
	if needCaptcha {
		ok, err := s.captcha.Verify(ctx, r.PostForm, addr)
		if err != nil {
			s.log.Error("captcha check failed", "err", err)
		}
		if !ok {
			p.Error = "Complete the check below, then log in again."
			s.render(w, http.StatusBadRequest, "login.html", p)
			return
		}
	}
	account, err := s.checkPassword(ctx, name, password)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if account == 0 {
		if !s.limit.fail(name, addr, now) {
			s.log.Warn("login rate limit tables are full")
		}
		if _, c := s.limit.check(name, addr, now); c {
			p.Captcha = s.captcha.Field()
		}
		p.Error = errLogin
		s.render(w, http.StatusBadRequest, "login.html", p)
		return
	}
	// An account with TOTP gets a pending login and the code page. Its
	// failure count for the name is only reset once the code passes.
	on, err := s.totpOn(ctx, account)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if on {
		if err := s.startPending(ctx, w, account, factorPassword); err != nil {
			s.unavailable(w, r, err)
			return
		}
		s.redirect(w, r, withReturn(Prefix+"login/code", p.Return))
		return
	}
	s.limit.succeed(name)
	if err := s.startSession(ctx, w, r, account, []string{factorPassword}); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.redirect(w, r, withReturn(Prefix+"characters", p.Return))
}

// checkPassword returns the account ID for a name and password, or 0
// when either is wrong. A name without an account costs one hash too,
// so the time taken does not tell whether the account exists.
func (s *Service) checkPassword(ctx context.Context, name, password string) (int64, error) {
	if !ValidAccountName(name) || len(password) > MaxPassword || password == "" {
		// No account can have this name or password. The name rules are
		// public, so answering at once reveals nothing.
		return 0, nil
	}
	row, err := s.q.PasswordByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, s.hasher.Burn(ctx, password)
	}
	if err != nil {
		return 0, err
	}
	ok, rehash, err := s.hasher.Verify(ctx, row.Hash, password)
	if errors.Is(err, errBadHash) {
		s.log.Error("an account's password hash cannot be read", "account", row.ID)
		return 0, nil
	}
	if err != nil || !ok {
		return 0, err
	}
	if rehash {
		// The hash was made with older parameters. Store one with the
		// current parameters. A failure only means trying next time.
		if h, err := s.hasher.Hash(ctx, password); err == nil {
			err = s.q.UpdatePasswordHash(ctx, db.UpdatePasswordHashParams{AccountID: row.ID, OldHash: row.Hash, NewHash: h})
			if err != nil {
				s.log.Error("cannot store a new password hash", "account", row.ID, "err", err)
			}
		}
	}
	return row.ID, nil
}

// startSession makes a login session, sets its cookie, and ends the
// session the request came with, if any, so an old cookie cannot be
// fixed on the player. factors are what the player proved, and are kept
// with the session.
func (s *Service) startSession(ctx context.Context, w http.ResponseWriter, r *http.Request, account int64, factors []string) error {
	if old, ok := tokenHash(s.cookie(r, sessionCookie)); ok {
		if err := s.revoke(ctx, old); err != nil {
			return err
		}
	}
	token, hash := newToken()
	var ip *netip.Addr
	if a := s.clientAddr(r); a.IsValid() && !a.IsUnspecified() {
		ip = &a
	}
	_, err := s.q.CreateSession(ctx, db.CreateSessionParams{
		TokenHash: hash, AccountID: account,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.cfg.SessionLifetime), Valid: true},
		UserAgent: userAgent(r), Ip: ip, Factors: factors,
	})
	if err != nil {
		return err
	}
	s.setCookie(w, sessionCookie, token, int(s.cfg.SessionLifetime.Seconds()))
	return nil
}

// revoke ends a session and deletes the game tickets it made and nobody
// used, in one transaction. After the commit, Config.Ended closes the
// game connections the session opened.
func (s *Service) revoke(ctx context.Context, hash []byte) error {
	var ids []int64
	err := pgx.BeginFunc(ctx, s.cfg.Pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.DeleteSessionTickets(ctx, hash); err != nil {
			return err
		}
		var err error
		ids, err = q.RevokeSession(ctx, hash)
		return err
	})
	if err == nil && len(ids) > 0 && s.cfg.Ended != nil {
		s.cfg.Ended.EndLoginSessions(ids...)
	}
	return err
}

func (s *Service) getSignup(w http.ResponseWriter, r *http.Request) {
	p := s.newPage(w, r, "Sign up")
	if s.limit.captchaFor(s.clientAddr(r), time.Now()) {
		p.Captcha = s.captcha.Field()
	}
	s.render(w, http.StatusOK, "signup.html", p)
}

func (s *Service) postSignup(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	addr, now := s.clientAddr(r), time.Now()
	name := strings.TrimSpace(r.PostForm.Get("name"))
	password := r.PostForm.Get("password")
	p := s.newPage(w, r, "Sign up")
	p.Name = shorten(name, MaxAccountName)
	if s.limit.captchaFor(addr, now) {
		p.Captcha = s.captcha.Field()
		if ok, _ := s.captcha.Verify(ctx, r.PostForm, addr); !ok {
			p.Error = "Complete the check below, then sign up again."
			s.render(w, http.StatusBadRequest, "signup.html", p)
			return
		}
	}
	switch {
	case !ValidAccountName(name):
		p.Error = "Account names are 4 to 24 letters, digits or underscores, and start with a letter."
	case password != r.PostForm.Get("confirm"):
		p.Error = "The two passwords are not the same."
	default:
		p.Error = PasswordProblem(name, password)
	}
	if p.Error != "" {
		s.render(w, http.StatusBadRequest, "signup.html", p)
		return
	}
	if ok, wait := s.limit.allow(actSignup, addr, now); !ok {
		p.Error = "Too many new accounts from your network. Try again in " + seconds(wait) + "."
		s.render(w, http.StatusTooManyRequests, "signup.html", p)
		return
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	account, err := s.q.CreateAccountWithPassword(ctx, db.CreateAccountWithPasswordParams{Name: name, Hash: hash})
	if errors.Is(err, pgx.ErrNoRows) {
		// This tells that the name has an account. Signup cannot avoid
		// that: the player needs to know to pick another name.
		p.Error = "That account name is taken."
		s.render(w, http.StatusConflict, "signup.html", p)
		return
	}
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.log.Info("account made", "account", account)
	if err := s.startSession(ctx, w, r, account, []string{factorPassword}); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.redirect(w, r, withReturn(Prefix+"characters", p.Return))
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	if hash, ok := tokenHash(s.cookie(r, sessionCookie)); ok {
		if err := s.revoke(ctx, hash); err != nil {
			s.unavailable(w, r, err)
			return
		}
	}
	s.setCookie(w, sessionCookie, "", -1)
	s.redirect(w, r, withReturn(Prefix+"login", s.safeReturn(r.PostForm.Get("return"))))
}

// loggedIn returns the request's session, or sends the player to the
// login page and returns false.
func (s *Service) loggedIn(ctx context.Context, w http.ResponseWriter, r *http.Request) (db.UseSessionRow, bool) {
	sess, ok, err := s.session(ctx, r)
	if err != nil {
		s.unavailable(w, r, err)
		return sess, false
	}
	if !ok {
		s.redirect(w, r, withReturn(Prefix+"login", s.safeReturn(r.FormValue("return"))))
		return sess, false
	}
	return sess, true
}

func (s *Service) getCharacters(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p := s.newPage(w, r, "Characters")
	if err := s.fillCharacters(ctx, p, sess); err != nil {
		s.unavailable(w, r, err)
		return
	}
	s.render(w, http.StatusOK, "characters.html", p)
}

func (s *Service) fillCharacters(ctx context.Context, p *page, sess db.UseSessionRow) error {
	rows, err := s.q.AccountCharacters(ctx, sess.AccountID)
	if err != nil {
		return err
	}
	p.Account = sess.AccountName
	used := map[int]bool{}
	for _, c := range rows {
		used[c.Slot] = true
		job := "Novice"
		if c.Job != 0 {
			job = "Job " + strconv.Itoa(c.Job)
		}
		p.Characters = append(p.Characters, characterView{
			Slot: c.Slot, Name: c.Name, Job: job, BaseLevel: c.BaseLevel, JobLevel: c.JobLevel,
			Map: strings.TrimPrefix(c.Map, "map/"),
		})
	}
	for i := range s.cfg.Slots {
		if !used[i] {
			p.FreeSlots = append(p.FreeSlots, i)
		}
	}
	for i := MinHair; i <= MaxHair; i++ {
		p.Hairs = append(p.Hairs, i)
	}
	for i := 0; i <= MaxHairColor; i++ {
		p.HairColors = append(p.HairColors, i)
	}
	return nil
}

func (s *Service) postCharacters(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	p := s.newPage(w, r, "Characters")
	if err := s.fillCharacters(ctx, p, sess); err != nil {
		s.unavailable(w, r, err)
		return
	}
	f := r.PostForm
	name := strings.TrimSpace(f.Get("name"))
	p.Name = shorten(name, MaxCharName)
	slot, err1 := strconv.Atoi(f.Get("slot"))
	hair, err2 := strconv.Atoi(f.Get("hair"))
	color, err3 := strconv.Atoi(f.Get("hair_color"))
	sex := f.Get("sex")
	switch {
	case !ValidCharacterName(name):
		p.Error = "Character names are 4 to 24 letters or digits, with single spaces between words."
	case s.cfg.CheckName != nil && s.cfg.CheckName(name) != nil:
		p.Error = "That character name is not allowed. Choose another."
	case err1 != nil || slot < 0 || slot >= s.cfg.Slots:
		p.Error = "Choose a free slot."
	case err2 != nil || hair < MinHair || hair > MaxHair || err3 != nil || color < 0 || color > MaxHairColor:
		p.Error = "Choose a hair style and color from the list."
	case sex != "f" && sex != "m":
		p.Error = "Choose female or male."
	}
	if p.Error != "" {
		s.render(w, http.StatusBadRequest, "characters.html", p)
		return
	}
	c := s.cfg.NewCharacter(name, Look{Male: sex == "m", Hair: hair, HairColor: color})
	c.Slot, c.AccountID = slot, sess.AccountID
	_, err := store.CreateCharacter(ctx, s.cfg.Pool, sess.AccountID, &c)
	switch {
	case errors.Is(err, store.ErrNameTaken):
		p.Error = "That character name is taken."
	case errors.Is(err, store.ErrSlotTaken):
		p.Error = "That slot is taken."
	case err != nil:
		s.unavailable(w, r, err)
		return
	}
	if p.Error != "" {
		s.render(w, http.StatusConflict, "characters.html", p)
		return
	}
	s.redirect(w, r, withReturn(Prefix+"characters", p.Return))
}

// play records the character the player chose, then sends the player to
// the web client, which fetches a game ticket for it.
func (s *Service) play(w http.ResponseWriter, r *http.Request) {
	if !s.form(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok := s.loggedIn(ctx, w, r)
	if !ok {
		return
	}
	slot, err := strconv.Atoi(r.PostForm.Get("slot"))
	if err != nil || slot < 0 || slot >= 15 {
		s.render(w, http.StatusBadRequest, "message.html", &page{Title: "Bad request", Error: "Choose a character."})
		return
	}
	n, err := s.q.SelectCharacter(ctx, db.SelectCharacterParams{SessionID: sess.ID, Slot: slot})
	if err != nil {
		s.unavailable(w, r, err)
		return
	}
	if n == 0 {
		s.render(w, http.StatusBadRequest, "message.html", &page{Title: "Bad request", Error: "That slot has no character."})
		return
	}
	to := s.safeReturn(r.PostForm.Get("return"))
	if to == "" {
		to = s.cfg.PlayURL
	}
	if to == "" {
		p := s.newPage(w, r, "Ready")
		p.Notice = "Your character is chosen. Open the game client to play."
		s.render(w, http.StatusOK, "message.html", p)
		return
	}
	s.redirect(w, r, to)
}

// ticket gives a trusted web client a one-time game ticket for the
// session's chosen character. The client calls it with fetch and
// credentials: "include", and sends the ticket in Hello. Only trusted
// client origins get an answer, through CORS.
func (s *Service) ticket(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" || (!s.clients[origin] && origin != s.ownOrigin(r)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin"})
		return
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	h.Add("Vary", "Origin")
	if r.Method == http.MethodOptions {
		h.Set("Access-Control-Allow-Methods", "POST")
		h.Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if ok, _ := s.limit.allow(actTicket, s.clientAddr(r), time.Now()); !ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()
	sess, ok, err := s.session(ctx, r)
	if err != nil {
		s.log.Error("cannot check a session", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login"})
		return
	}
	if !sess.CharacterID.Valid {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "character"})
		return
	}
	token, hash := newToken()
	err = s.q.CreateTicket(ctx, db.CreateTicketParams{
		TokenHash: hash, SessionID: sess.ID, AccountID: sess.AccountID, CharacterID: sess.CharacterID.Int64,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.cfg.TicketLifetime), Valid: true},
	})
	if err != nil {
		s.log.Error("cannot make a game ticket", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": token, "expires_in": int(s.cfg.TicketLifetime.Seconds())})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// unavailable logs an internal error and shows a generic page. The log
// has the error only, never form values.
func (s *Service) unavailable(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrBusy) || errors.Is(err, context.DeadlineExceeded) {
		s.log.Warn("account request timed out", "path", r.URL.Path)
	} else {
		s.log.Error("account request failed", "path", r.URL.Path, "err", err)
	}
	s.render(w, http.StatusServiceUnavailable, "message.html", &page{Title: "Try again later", Error: "The server cannot do this right now. Try again later."})
}

// seconds writes a wait as whole seconds, rounded up.
func seconds(d time.Duration) string {
	n := int((d + time.Second - 1) / time.Second)
	if n == 1 {
		return "1 second"
	}
	return strconv.Itoa(n) + " seconds"
}

// shorten cuts a string for showing back in a form.
func shorten(s string, n int) string {
	if len(s) > n {
		return ""
	}
	return s
}
