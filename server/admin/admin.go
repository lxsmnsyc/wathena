// Package admin is the admin API (strategy section 10.6, proposal 0002
// review decision 33, decision 0028). It serves JSON at /admin/api/v1/
// on the account service's origin:
//   - Each call needs the account session cookie. Each change also needs
//     the X-CSRF-Token header, with the token that GET /me returns.
//   - Each call but /me checks a permission of the caller's staff role.
//     A role counts only when the session logged in with a second
//     factor and the account still has TOTP or a passkey on.
//   - Role changes also need a login, or a reauth, within 10 minutes.
//   - Each change writes a row of the GM audit log, in the transaction
//     of the change.
//   - Calls are rate limited per address, as the account pages are.
//
// The messages are protobuf, wathena.admin.v1, in their JSON form. Step
// 1 has GET /me, GET, POST and DELETE /staff, and GET /audit. Step 2
// (decision 0029) has GET /players, GET /characters/{id} and its /logs,
// GET /items/{serial}/history, POST /restore and POST
// /audit/{id}/review. Step 3 (decision 0030) has the report queue and
// the sanctions (moderation.go). Step 4 (decision 0031) has the world,
// events and mods tools (tools.go). Phase 8 step 3 (decision 0049) has
// GET /retention (retention.go).
//
// The API reaches the world through World, so a distributed build can
// put the world service behind the network.
package admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Prefix is the path the API lives under.
const Prefix = "/admin/api/v1/"

// CSRFHeader is the header that carries the CSRF token of a change.
const CSRFHeader = "X-CSRF-Token"

const (
	// requestTimeout bounds the database work of one call.
	requestTimeout = 10 * time.Second
	// maxBody is the largest request body read.
	maxBody = 16 << 10
	// DefaultAuditPage and MaxAuditPage are GET /audit's page sizes.
	DefaultAuditPage = 50
	MaxAuditPage     = 100
	// MaxReason is the longest reason, in characters.
	MaxReason = 255
)

// Auth is what the API needs from the account service. *account.Service
// is one.
type Auth interface {
	Session(ctx context.Context, r *http.Request) (account.Session, bool, error)
	Recent(sess account.Session) bool
	TwoFactorOn(ctx context.Context, account int64) (bool, error)
	CSRFToken(w http.ResponseWriter, r *http.Request) string
	CheckCSRF(r *http.Request, token string) bool
	AllowAdmin(r *http.Request) (bool, time.Duration)
	SecurityHeaders(w http.ResponseWriter)
}

// Config configures a Handler.
type Config struct {
	// Pool connects as the wathena_account role.
	Pool *pgxpool.Pool
	Auth Auth
	// World is told of role changes. It may be nil.
	World World
	// Roles are the configured roles, and Owners the config's owners.
	Roles  *staff.Roles
	Owners []string
	// Retention is the retention job, for GET /retention. It may be nil.
	Retention Retention
	Log       *slog.Logger
}

// Handler serves the admin API.
type Handler struct {
	cfg    Config
	q      *db.Queries
	owners map[string]bool
	mux    *http.ServeMux
	log    *slog.Logger
}

// New makes a handler.
func New(cfg Config) (*Handler, error) {
	if cfg.Pool == nil || cfg.Auth == nil || cfg.Roles == nil {
		return nil, errors.New("admin: Pool, Auth and Roles are required")
	}
	h := &Handler{cfg: cfg, q: db.New(cfg.Pool), owners: map[string]bool{}, log: cfg.Log}
	if h.log == nil {
		h.log = slog.Default()
	}
	for _, o := range cfg.Owners {
		h.owners[strings.ToLower(o)] = true
	}
	m := http.NewServeMux()
	m.HandleFunc("GET "+Prefix+"me", h.call("", read, h.me))
	m.HandleFunc("GET "+Prefix+"staff", h.call("staff.assign", read, h.listStaff))
	m.HandleFunc("POST "+Prefix+"staff", h.call("staff.assign", changeRecent, h.giveRole))
	m.HandleFunc("DELETE "+Prefix+"staff/{account}", h.call("staff.assign", changeRecent, h.removeRole))
	m.HandleFunc("GET "+Prefix+"audit", h.call("audit.read", read, h.searchAudit))
	m.HandleFunc("POST "+Prefix+"audit/{id}/review", h.call("audit.review", change, h.review))
	m.HandleFunc("GET "+Prefix+"players", h.call("players.read", read, h.searchPlayers))
	m.HandleFunc("GET "+Prefix+"characters/{id}", h.call("players.read", read, h.character))
	m.HandleFunc("GET "+Prefix+"characters/{id}/logs", h.call("items.history", read, h.characterLogs))
	m.HandleFunc("GET "+Prefix+"items/{serial}/history", h.call("items.history", read, h.itemHistory))
	m.HandleFunc("POST "+Prefix+"restore", h.call("items.restore", changeRecent, h.restore))
	m.HandleFunc("GET "+Prefix+"reports", h.call("moderation.reports", read, h.searchReports))
	m.HandleFunc("GET "+Prefix+"reports/{id}", h.call("moderation.reports", read, h.report))
	m.HandleFunc("POST "+Prefix+"reports/{id}/close", h.call("moderation.reports", change, h.closeReport))
	m.HandleFunc("POST "+Prefix+"sanctions", h.call(anyModeration, change, h.sanction))
	m.HandleFunc("DELETE "+Prefix+"sanctions/{id}", h.call(anyModeration, change, h.liftSanction))
	m.HandleFunc("GET "+Prefix+"sanctions", h.call(anyModeration, read, h.searchSanctions))
	m.HandleFunc("GET "+Prefix+"world", h.call("world.read", read, h.worldView))
	m.HandleFunc("GET "+Prefix+"retention", h.call("world.read", read, h.retention))
	m.HandleFunc("POST "+Prefix+"announce", h.call("world.announce", change, h.announce))
	m.HandleFunc("POST "+Prefix+"restart", h.call("world.restart", changeRecent, h.planRestart))
	m.HandleFunc("DELETE "+Prefix+"restart", h.call("world.restart", changeRecent, h.cancelRestart))
	m.HandleFunc("GET "+Prefix+"events", h.call("events.control", read, h.events))
	m.HandleFunc("PATCH "+Prefix+"events/{name}", h.call("events.control", change, h.changeEvent))
	m.HandleFunc("POST "+Prefix+"events/{name}/start", h.call("events.control", change, h.eventAction(EventStart)))
	m.HandleFunc("POST "+Prefix+"events/{name}/stop", h.call("events.control", change, h.eventAction(EventStop)))
	m.HandleFunc("POST "+Prefix+"events/{name}/reset", h.call("events.control", change, h.eventAction(EventReset)))
	m.HandleFunc("POST "+Prefix+"rotations/{dungeon}/rotate", h.call("events.control", change, h.forceChange(false)))
	m.HandleFunc("POST "+Prefix+"regions/{region}/change", h.call("events.control", change, h.forceChange(true)))
	m.HandleFunc("GET "+Prefix+"mods", h.call("mods.read", read, h.mods))
	m.HandleFunc("POST "+Prefix+"mods/{id}/disable", h.call("mods.disable", change, h.switchMod(true)))
	m.HandleFunc("POST "+Prefix+"mods/{id}/enable", h.call("mods.disable", change, h.switchMod(false)))
	m.HandleFunc(Prefix, func(w http.ResponseWriter, _ *http.Request) {
		h.fail(w, http.StatusNotFound, "not_found", "There is no such call.")
	})
	h.mux = m
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.cfg.Auth.SecurityHeaders(w)
	h.mux.ServeHTTP(w, r)
}

// caller is the account behind a call, with the role that counts.
type caller struct {
	sess account.Session
	// role is the role that counts, or "". assigned is the account's
	// role whether or not it counts.
	role, assigned string
	twoFactor      bool
}

// callKind says what a call needs beyond its permission.
type callKind int

const (
	// read needs nothing more.
	read callKind = iota
	// change needs the CSRF token.
	change
	// changeRecent needs the CSRF token and a login within 10 minutes:
	// role changes, restores and planned restarts (review decisions 33,
	// 34 and 36).
	changeRecent
)

// call wraps a call with the checks every call has: the rate limit, the
// CSRF token of a change, the session, the role and the permission.
// Role changes and restores also need a recent login. permission ""
// needs only a session.
func (h *Handler) call(permission string, kind callKind, f func(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller)) http.HandlerFunc {
	change := kind != read
	return func(w http.ResponseWriter, r *http.Request) {
		if ok, wait := h.cfg.Auth.AllowAdmin(r); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			h.fail(w, http.StatusTooManyRequests, "rate", "Too many calls from your network. Wait a moment.")
			return
		}
		if change && !h.cfg.Auth.CheckCSRF(r, r.Header.Get(CSRFHeader)) {
			h.fail(w, http.StatusForbidden, "csrf", "The request came without a valid CSRF token. Reload the page.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		sess, ok, err := h.cfg.Auth.Session(ctx, r)
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		if !ok {
			h.fail(w, http.StatusUnauthorized, "login", "Log in first.")
			return
		}
		c, err := h.caller(ctx, sess)
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		if permission != "" {
			switch {
			case c.role == "" && c.assigned != "" && !c.twoFactor:
				h.fail(w, http.StatusForbidden, "two_factor", "Staff roles need two-factor login. Turn on TOTP or a passkey, then log in with it.")
				return
			case !h.has(c, permission):
				h.fail(w, http.StatusForbidden, "forbidden", "Your role does not allow this.")
				return
			}
			if kind == changeRecent && !h.cfg.Auth.Recent(sess) {
				h.write(w, http.StatusForbidden, adminv1.Error_builder{Code: "reauth",
					Message:   "Confirm it is you: log in again on the account page, then try again.",
					ReauthUrl: account.ReauthURL(account.AdminPath)}.Build())
				return
			}
		}
		f(ctx, w, r, c)
	}
}

// anyModeration is the permission of the calls that need any permission
// of the moderation group, such as GET /sanctions. Each sanction call
// then checks the permission of its kind.
const anyModeration = "moderation.*"

// has reports whether the caller's role has a permission, or for
// anyModeration any permission of the moderation group.
func (h *Handler) has(c caller, permission string) bool {
	if permission == anyModeration {
		for _, p := range staff.Match(anyModeration) {
			if h.cfg.Roles.Has(c.role, p) {
				return true
			}
		}
		return false
	}
	return h.cfg.Roles.Has(c.role, permission)
}

// need checks a permission inside a call, and a recent login when
// recent is set. It writes the error and returns false when not.
func (h *Handler) need(w http.ResponseWriter, c caller, permission string, recent bool) bool {
	if !h.cfg.Roles.Has(c.role, permission) {
		h.fail(w, http.StatusForbidden, "forbidden", "Your role does not allow this: it needs "+permission+".")
		return false
	}
	if recent && !h.cfg.Auth.Recent(c.sess) {
		h.write(w, http.StatusForbidden, adminv1.Error_builder{Code: "reauth",
			Message:   "Confirm it is you: log in again on the account page, then try again.",
			ReauthUrl: account.ReauthURL(account.AdminPath)}.Build())
		return false
	}
	return true
}

// caller finds a session's role: owner for the config's owners, else
// the role the database keeps while the config has it. It counts only
// with a second factor.
func (h *Handler) caller(ctx context.Context, sess account.Session) (caller, error) {
	c := caller{sess: sess}
	if h.owners[strings.ToLower(sess.AccountName)] {
		c.assigned = staff.Owner
	} else {
		role, err := h.q.StaffRoleOf(ctx, sess.AccountID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return c, err
		case h.cfg.Roles.Exists(role):
			c.assigned = role
		}
	}
	if c.assigned == "" || !sess.SecondFactor() {
		return c, nil
	}
	on, err := h.cfg.Auth.TwoFactorOn(ctx, sess.AccountID)
	if err != nil {
		return c, err
	}
	if on {
		c.twoFactor, c.role = true, c.assigned
	}
	return c, nil
}

func (h *Handler) me(_ context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	h.write(w, http.StatusOK, adminv1.Me_builder{
		Account: c.sess.AccountName, Role: c.role, Permissions: h.cfg.Roles.Permissions(c.role),
		TwoFactor: c.twoFactor, AssignedRole: c.assigned, RecentLogin: h.cfg.Auth.Recent(c.sess),
		CsrfToken: h.cfg.Auth.CSRFToken(w, r), ReauthUrl: account.ReauthURL(account.AdminPath),
	}.Build())
}

func (h *Handler) listStaff(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	rows, err := h.q.StaffMembers(ctx)
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	out := &adminv1.StaffList{}
	var members []*adminv1.StaffMember
	listed := map[int64]bool{}
	// The owners first, in the config's order. An owner name without an
	// account yet is listed with account ID 0.
	if len(h.cfg.Owners) > 0 {
		names := make([]string, len(h.cfg.Owners))
		for i, o := range h.cfg.Owners {
			names[i] = strings.ToLower(o)
		}
		accounts, err := h.q.StaffAccountsByNames(ctx, names)
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		ids := make([]int64, 0, len(accounts))
		for _, a := range accounts {
			ids = append(ids, a.ID)
		}
		twoFactor := map[int64]bool{}
		tf, err := h.q.AccountsTwoFactor(ctx, ids)
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		for _, t := range tf {
			twoFactor[t.ID] = t.TwoFactor
		}
		for _, o := range h.cfg.Owners {
			m := adminv1.StaffMember_builder{Account: o, Role: staff.Owner, FromConfig: true}
			for _, a := range accounts {
				if strings.EqualFold(a.Name, o) {
					m.AccountId, m.Account, m.TwoFactor = a.ID, a.Name, twoFactor[a.ID]
					listed[a.ID] = true
				}
			}
			members = append(members, m.Build())
		}
	}
	for _, row := range rows {
		if listed[row.AccountID] {
			continue
		}
		members = append(members, adminv1.StaffMember_builder{
			AccountId: row.AccountID, Account: row.Name, Role: row.Role, GivenBy: row.GivenBy,
			GivenAt: timestamppb.New(row.GivenAt.Time), TwoFactor: row.TwoFactor,
		}.Build())
	}
	out.SetMembers(members)
	var roles []*adminv1.Role
	for _, n := range h.cfg.Roles.Names() {
		roles = append(roles, adminv1.Role_builder{Name: n, Permissions: h.cfg.Roles.Permissions(n)}.Build())
	}
	out.SetRoles(roles)
	h.write(w, http.StatusOK, out)
}

// target is the account a role change acts on.
type target struct {
	id   int64
	name string
	// role is its current role in the database, or "".
	role string
}

// findTarget finds the account of a role change and checks that the
// caller may change its role. It writes the error and returns false
// when not.
func (h *Handler) findTarget(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller, name string) (target, bool) {
	if !account.ValidAccountName(name) {
		h.fail(w, http.StatusBadRequest, "bad_request", "That is not an account name.")
		return target{}, false
	}
	a, err := h.q.StaffAccountByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "not_found", "There is no account with that name.")
		return target{}, false
	}
	if err != nil {
		h.unavailable(w, r, err)
		return target{}, false
	}
	t := target{id: a.ID, name: a.Name}
	switch {
	case a.ID == c.sess.AccountID:
		h.fail(w, http.StatusConflict, "conflict", "You cannot change your own role.")
		return t, false
	case h.owners[strings.ToLower(a.Name)]:
		h.fail(w, http.StatusConflict, "conflict", "That account is an owner. Owners come from the config's staff.owners.")
		return t, false
	}
	role, err := h.q.StaffRoleOf(ctx, a.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		h.unavailable(w, r, err)
		return t, false
	default:
		t.role = role
	}
	// A caller may give and take only roles whose permissions it has
	// all of, so no one gives more than they hold.
	if t.role != "" && !h.cfg.Roles.Covers(c.role, t.role) {
		h.fail(w, http.StatusForbidden, "forbidden", "That account's role has permissions your role does not have.")
		return t, false
	}
	return t, true
}

func (h *Handler) giveRole(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	req := &adminv1.GiveRoleRequest{}
	if !h.read(w, r, req) {
		return
	}
	role, reason := req.GetRole(), strings.TrimSpace(req.GetReason())
	switch {
	case role == staff.Owner:
		h.fail(w, http.StatusBadRequest, "bad_request", "The owner role comes only from the config's staff.owners.")
		return
	case !h.cfg.Roles.Exists(role):
		h.fail(w, http.StatusBadRequest, "bad_request", "There is no such role. The roles are "+strings.Join(h.cfg.Roles.Names(), ", ")+".")
		return
	case !validReason(reason):
		h.fail(w, http.StatusBadRequest, "bad_request", "The reason is at most 255 characters.")
		return
	case !h.cfg.Roles.Covers(c.role, role):
		h.fail(w, http.StatusForbidden, "forbidden", "That role has permissions your role does not have.")
		return
	}
	t, ok := h.findTarget(ctx, w, r, c, strings.TrimSpace(req.GetAccount()))
	if !ok {
		return
	}
	if t.role != role {
		args := map[string]any{"role": role}
		if t.role != "" {
			args["previous"] = t.role
		}
		err := pgx.BeginFunc(ctx, h.cfg.Pool, func(tx pgx.Tx) error {
			q := h.q.WithTx(tx)
			if err := q.GiveStaffRole(ctx, db.GiveStaffRoleParams{AccountID: t.id, Role: role,
				GivenBy: pgtype.Int8{Int64: c.sess.AccountID, Valid: true}}); err != nil {
				return err
			}
			return h.audit(ctx, q, c, "staff.give", t, args, reason)
		})
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		h.log.Info("staff role given", "by", c.sess.AccountID, "account", t.id, "role", role)
		h.tellWorld(ctx, t.id, role)
	}
	h.write(w, http.StatusOK, adminv1.StaffMember_builder{AccountId: t.id, Account: t.name, Role: role,
		GivenBy: c.sess.AccountName, GivenAt: timestamppb.Now()}.Build())
}

func (h *Handler) removeRole(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The query could not be read.")
		return
	}
	reason := strings.TrimSpace(query.Get("reason"))
	if !validReason(reason) {
		h.fail(w, http.StatusBadRequest, "bad_request", "The reason is at most 255 characters.")
		return
	}
	t, ok := h.findTarget(ctx, w, r, c, r.PathValue("account"))
	if !ok {
		return
	}
	if t.role == "" {
		h.fail(w, http.StatusNotFound, "not_found", "That account has no role.")
		return
	}
	err = pgx.BeginFunc(ctx, h.cfg.Pool, func(tx pgx.Tx) error {
		q := h.q.WithTx(tx)
		if _, err := q.RemoveStaffRole(ctx, t.id); err != nil {
			return err
		}
		return h.audit(ctx, q, c, "staff.remove", t, map[string]any{"role": t.role}, reason)
	})
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	h.log.Info("staff role removed", "by", c.sess.AccountID, "account", t.id, "role", t.role)
	h.tellWorld(ctx, t.id, "")
	w.WriteHeader(http.StatusNoContent)
}

// audit writes the audit row of a change made through the API.
func (h *Handler) audit(ctx context.Context, q *db.Queries, c caller, action string, t target, args map[string]any, reason string) error {
	return h.writeAudit(ctx, q, c, action, t, args, reason, false)
}

// tellWorld passes a role change to the world. The database has the
// change already, so a failure only delays it until the world loads
// the roles again, at its next start.
func (h *Handler) tellWorld(ctx context.Context, account int64, role string) {
	if h.cfg.World == nil {
		return
	}
	if err := h.cfg.World.StaffChanged(ctx, account, role); err != nil {
		h.log.Error("the world did not take a staff role change", "account", account, "err", err)
	}
}

func (h *Handler) searchAudit(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The query could not be read.")
		return
	}
	p := db.SearchStaffAuditParams{MaxRows: DefaultAuditPage + 1}
	bad := func(msg string) { h.fail(w, http.StatusBadRequest, "bad_request", msg) }
	text := func(key string, max int) (pgtype.Text, bool) {
		v := strings.TrimSpace(q.Get(key))
		if v == "" {
			return pgtype.Text{}, true
		}
		if utf8.RuneCountInString(v) > max || !utf8.ValidString(v) {
			bad(key + " is at most " + strconv.Itoa(max) + " characters.")
			return pgtype.Text{}, false
		}
		return pgtype.Text{String: v, Valid: true}, true
	}
	when := func(key string) (pgtype.Timestamptz, bool) {
		v := q.Get(key)
		if v == "" {
			return pgtype.Timestamptz{}, true
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t, err = time.Parse(time.DateOnly, v)
		}
		if err != nil {
			bad(key + " is a date, such as 2026-10-12, or an RFC 3339 time.")
			return pgtype.Timestamptz{}, false
		}
		return pgtype.Timestamptz{Time: t, Valid: true}, true
	}
	var ok bool
	if p.Actor, ok = text("account", 24); !ok {
		return
	}
	if p.Action, ok = text("action", 64); !ok {
		return
	}
	if p.Target, ok = text("target", 64); !ok {
		return
	}
	if p.Since, ok = when("since"); !ok {
		return
	}
	if p.Until, ok = when("until"); !ok {
		return
	}
	switch v := q.Get("needs_review"); v {
	case "":
	case "true", "false":
		p.NeedsReview = pgtype.Bool{Bool: v == "true", Valid: true}
	default:
		bad("needs_review is true or false.")
		return
	}
	switch v := q.Get("review"); v {
	case "":
	case "pending", "reviewed":
		p.Review = pgtype.Text{String: v, Valid: true}
	default:
		bad("review is pending or reviewed.")
		return
	}
	if v := q.Get("before_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 1 {
			bad("before_id is a row ID.")
			return
		}
		p.BeforeID = pgtype.Int8{Int64: id, Valid: true}
	}
	limit := DefaultAuditPage
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxAuditPage {
			bad("limit is 1 to " + strconv.Itoa(MaxAuditPage) + ".")
			return
		}
		limit = n
	}
	p.MaxRows = int32(limit + 1) // #nosec G115 -- at most MaxAuditPage + 1.
	rows, err := h.q.SearchStaffAudit(ctx, p)
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	page := &adminv1.AuditPage{}
	if len(rows) > limit {
		rows = rows[:limit]
		page.SetNextBeforeId(rows[limit-1].ID)
	}
	entries := make([]*adminv1.AuditEntry, 0, len(rows))
	for _, row := range rows {
		e, err := auditEntry(db.StaffAuditEntryRow(row))
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		entries = append(entries, e)
	}
	page.SetEntries(entries)
	h.write(w, http.StatusOK, page)
}

// auditEntry is an audit row as the API shows it.
func auditEntry(row db.StaffAuditEntryRow) (*adminv1.AuditEntry, error) {
	args := &structpb.Struct{}
	if err := protojson.Unmarshal(row.Arguments, args); err != nil {
		return nil, err
	}
	e := adminv1.AuditEntry_builder{
		Id: row.ID, At: timestamppb.New(row.At.Time), Account: row.AccountName, Role: row.Role,
		Action: row.Action, Source: row.Source, Target: row.Target, Arguments: args, Reason: row.Reason,
		NeedsReview: row.NeedsReview, ReviewedBy: row.ReviewerName, Verdict: row.Verdict, ReviewNote: row.ReviewNote,
		AccountId: row.AccountID,
	}
	if row.ReviewedAt.Valid {
		e.ReviewedAt = timestamppb.New(row.ReviewedAt.Time)
	}
	return e.Build(), nil
}

func validReason(s string) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= MaxReason && !strings.ContainsAny(s, "\x00\r\n")
}

// read decodes a JSON request body of at most maxBody bytes. Unknown
// fields are errors.
func (h *Handler) read(w http.ResponseWriter, r *http.Request, m proto.Message) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		h.fail(w, http.StatusUnsupportedMediaType, "bad_request", "Send JSON, with Content-Type: application/json.")
		return false
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err == nil {
		err = protojson.Unmarshal(b, m)
	}
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The request body could not be read.")
		return false
	}
	return true
}

func (h *Handler) write(w http.ResponseWriter, status int, m proto.Message) {
	b, err := protojson.Marshal(m)
	if err != nil {
		h.log.Error("cannot encode an admin answer", "err", err)
		status, b = http.StatusInternalServerError, []byte(`{"code":"unavailable"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b) // #nosec G705 -- protobuf JSON as application/json, with nosniff and the account service's CSP, which allows no scripts.
}

func (h *Handler) fail(w http.ResponseWriter, status int, code, msg string) {
	h.write(w, status, adminv1.Error_builder{Code: code, Message: msg}.Build())
}

// unavailable logs an internal error, never request values, and answers
// with a generic error.
func (h *Handler) unavailable(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		h.log.Warn("admin call timed out", "path", r.URL.Path)
	} else {
		h.log.Error("admin call failed", "path", r.URL.Path, "err", err)
	}
	h.fail(w, http.StatusServiceUnavailable, "unavailable", "The server cannot do this right now. Try again later.")
}
