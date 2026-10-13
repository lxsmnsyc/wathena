package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Player search, details, logs, item history, restores and reviews
// (proposal 0002 review decision 34, decision 0029):
//   - Accounts, characters and login records come from the account
//     role's pool. Game data comes from World: online characters from
//     their zones, the rest from the game's database.
//   - Detail views and IP searches write audit rows. Name searches do
//     not. Restores write theirs in the world, in the restore's commit.
//     Reviews write one in the transaction of the review.

// Page sizes of the calls of step 2.
const (
	DefaultPlayerPage = 20
	MaxPlayerPage     = 50
	DefaultLogPage    = 50
	MaxLogPage        = 100
	// MaxRestoreItems is the most serials of one restore: the mail's
	// slots go up to 20.
	MaxRestoreItems = 20
	// LoginDays is how far back an IP search looks (strategy section
	// 7.10).
	LoginDays = 90
)

// worldOnly answers unavailable when the API has no world.
func (h *Handler) worldOnly(w http.ResponseWriter) bool {
	if h.cfg.World == nil {
		h.fail(w, http.StatusServiceUnavailable, "unavailable", "The game world is not connected.")
		return false
	}
	return true
}

// worldFailed answers a world call's error.
func (h *Handler) worldFailed(w http.ResponseWriter, r *http.Request, err error) {
	var we *WorldError
	switch {
	case errors.Is(err, ErrNotFound):
		h.fail(w, http.StatusNotFound, "not_found", "There is no such character or item.")
	case errors.As(err, &we):
		status := map[string]int{"bad_request": http.StatusBadRequest, "forbidden": http.StatusForbidden,
			"not_found": http.StatusNotFound, "conflict": http.StatusConflict}[we.Code]
		if status == 0 {
			status = http.StatusServiceUnavailable
		}
		h.fail(w, status, we.Code, we.Message)
	default:
		h.unavailable(w, r, err)
	}
}

// pageLimit reads limit, from 1 to most, def by default.
func pageLimit(q url.Values, def, most int) (int, bool) {
	v := q.Get("limit")
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > most {
		return 0, false
	}
	return n, true
}

// likePrefix is a LIKE pattern for a lower-case prefix, with LIKE's
// wildcards escaped.
func likePrefix(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(strings.ToLower(s)) + "%"
}

// parseRange reads an IP range in CIDR form, or one address.
func parseRange(s string) (netip.Prefix, bool) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p.Masked(), true
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return netip.PrefixFrom(a, a.BitLen()), true
	}
	return netip.Prefix{}, false
}

type accountRow struct {
	id      int64
	name    string
	created pgtype.Timestamptz
}

func (h *Handler) searchPlayers(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The query could not be read.")
		return
	}
	bad := func(msg string) { h.fail(w, http.StatusBadRequest, "bad_request", msg) }
	var kinds []string
	for _, k := range []string{"name", "id", "serial", "ip"} {
		if strings.TrimSpace(q.Get(k)) != "" {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) != 1 {
		bad("Search by one of name, id, serial or ip.")
		return
	}
	limit, ok := pageLimit(q, DefaultPlayerPage, MaxPlayerPage)
	if !ok {
		bad("limit is 1 to " + strconv.Itoa(MaxPlayerPage) + ".")
		return
	}
	var after int64
	if v := q.Get("cursor"); v != "" {
		if after, err = strconv.ParseInt(v, 10, 64); err != nil || after < 1 {
			bad("cursor is the next_cursor of the last page.")
			return
		}
	}
	value := strings.TrimSpace(q.Get(kinds[0]))
	max := int32(limit + 1) // #nosec G115 -- at most MaxPlayerPage + 1.
	out := &adminv1.PlayerSearch{}
	var accounts []accountRow
	var cidr netip.Prefix
	switch kinds[0] {
	case "name":
		if utf8.RuneCountInString(value) > 24 || !utf8.ValidString(value) {
			bad("name is at most 24 characters.")
			return
		}
		rows, err := h.q.SearchAccountsByName(ctx, db.SearchAccountsByNameParams{Prefix: likePrefix(value), AfterID: after, MaxRows: max})
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		for _, a := range rows {
			accounts = append(accounts, accountRow{a.ID, a.Name, a.CreatedAt})
		}
	case "id":
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id < 1 {
			bad("id is an account or a character ID.")
			return
		}
		rows, err := h.q.SearchAccountsByID(ctx, db.SearchAccountsByIDParams{ID: id, AfterID: after, MaxRows: max})
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		for _, a := range rows {
			accounts = append(accounts, accountRow{a.ID, a.Name, a.CreatedAt})
		}
	case "serial":
		serial, ok := item.ParseSerial(value)
		if !ok {
			bad("serial is an item serial, such as 0192f0a8-1234-7abc-8def-0123456789ab.")
			return
		}
		if !h.worldOnly(w) {
			return
		}
		owner, err := h.cfg.World.FindSerial(ctx, serial)
		if err != nil {
			h.worldFailed(w, r, err)
			return
		}
		out.SetHolder(owner)
		if owner != nil && owner.GetAccountId() > after {
			rows, err := h.q.AccountsByIDs(ctx, []int64{owner.GetAccountId()})
			if err != nil {
				h.unavailable(w, r, err)
				return
			}
			for _, a := range rows {
				accounts = append(accounts, accountRow{a.ID, a.Name, a.CreatedAt})
			}
		}
	case "ip":
		if !h.cfg.Roles.Has(c.role, "players.read_ip") {
			h.fail(w, http.StatusForbidden, "forbidden", "Your role does not allow IP searches.")
			return
		}
		var ok bool
		if cidr, ok = parseRange(value); !ok {
			bad("ip is an address or a range in CIDR form, such as 203.0.113.0/24.")
			return
		}
		since := pgtype.Timestamptz{Time: time.Now().Add(-LoginDays * 24 * time.Hour), Valid: true}
		ids, err := h.q.SearchAccountsByIP(ctx, db.SearchAccountsByIPParams{Cidr: cidr, Since: since, AfterID: after, MaxRows: max})
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		// An IP search is a lookup the audit log keeps.
		err = h.writeAudit(ctx, h.q, c, "players.search_ip", target{name: cidr.String()},
			map[string]any{"accounts": len(ids)}, "", false)
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		rows, err := h.q.AccountsByIDs(ctx, ids)
		if err != nil {
			h.unavailable(w, r, err)
			return
		}
		for _, a := range rows {
			accounts = append(accounts, accountRow{a.ID, a.Name, a.CreatedAt})
		}
	}
	if len(accounts) > limit {
		accounts = accounts[:limit]
		out.SetNextCursor(strconv.FormatInt(accounts[limit-1].id, 10))
	}
	list, err := h.summaries(ctx, accounts, cidr)
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	out.SetAccounts(list)
	h.write(w, http.StatusOK, out)
}

// summaries fills the search results of accounts: their characters,
// last logins, the game's state of each character, and for an IP search
// the logins that matched.
func (h *Handler) summaries(ctx context.Context, accounts []accountRow, cidr netip.Prefix) ([]*adminv1.AccountSummary, error) {
	if len(accounts) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(accounts))
	for i, a := range accounts {
		ids[i] = a.id
	}
	chars, err := h.q.CharactersOfAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	last, err := h.q.LastLogins(ctx, ids)
	if err != nil {
		return nil, err
	}
	var logins []db.LoginsByIPRow
	if cidr.IsValid() {
		since := pgtype.Timestamptz{Time: time.Now().Add(-LoginDays * 24 * time.Hour), Valid: true}
		if logins, err = h.q.LoginsByIP(ctx, db.LoginsByIPParams{Ids: ids, Cidr: cidr, Since: since}); err != nil {
			return nil, err
		}
	}
	charIDs := make([]int64, len(chars))
	for i, ch := range chars {
		charIDs[i] = ch.ID
	}
	states := map[int64]CharacterState{}
	if h.cfg.World != nil && len(charIDs) > 0 {
		if states, err = h.cfg.World.Characters(ctx, charIDs); err != nil {
			return nil, err
		}
	}
	out := make([]*adminv1.AccountSummary, 0, len(accounts))
	for _, a := range accounts {
		s := adminv1.AccountSummary_builder{AccountId: a.id, Account: a.name, CreatedAt: timestamppb.New(a.created.Time)}
		for _, l := range last {
			if l.AccountID == a.id && l.At.Valid {
				s.LastLogin = timestamppb.New(l.At.Time)
			}
		}
		for _, ch := range chars {
			if ch.AccountID != a.id {
				continue
			}
			st := states[ch.ID]
			pos := st.Position
			if pos == nil {
				pos = adminv1.Position_builder{Map: mapName(ch.Map), X: int32(ch.X), Y: int32(ch.Y)}.Build() // #nosec G115 -- cells below 1024.
			}
			cs := adminv1.CharacterSummary_builder{
				Id: ch.ID, Name: ch.Name, Job: int32(ch.Job), // #nosec G115 -- job IDs are small.
				BaseLevel: int32(ch.BaseLevel), JobLevel: int32(ch.JobLevel), // #nosec G115 -- levels up to 275.
				Online: st.Online, Position: pos, Sanctions: st.Sanctions,
			}
			if h.cfg.World != nil {
				cs.JobName = h.cfg.World.JobName(ch.Job)
			}
			s.Characters = append(s.Characters, cs.Build())
		}
		for _, l := range logins {
			if l.AccountID != a.id {
				continue
			}
			rec := adminv1.LoginRecord_builder{At: timestamppb.New(l.At.Time), UserAgent: l.UserAgent}
			if l.Ip != nil {
				rec.Ip = l.Ip.String()
			}
			s.Logins = append(s.Logins, rec.Build())
		}
		out = append(out, s.Build())
	}
	return out, nil
}

// mapName is a map's name without its asset folder, such as prontera.
func mapName(m string) string { return strings.TrimPrefix(m, "map/") }

// characterID reads the {id} of a path.
func (h *Handler) characterID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		h.fail(w, http.StatusBadRequest, "bad_request", "That is not a character ID.")
		return 0, false
	}
	return id, true
}

func (h *Handler) character(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	id, ok := h.characterID(w, r)
	if !ok || !h.worldOnly(w) {
		return
	}
	ch, err := h.q.CharacterWithAccount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "not_found", "There is no character with that ID.")
		return
	}
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	d, err := h.cfg.World.Character(ctx, id)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	d.SetAccount(ch.AccountName)
	// A detail view is a lookup the audit log keeps.
	err = h.writeAudit(ctx, h.q, c, "players.view", target{id: ch.AccountID, name: ch.Name},
		map[string]any{"character_id": id, "online": d.GetOnline()}, "", false)
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	h.write(w, http.StatusOK, d)
}

func (h *Handler) characterLogs(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	id, ok := h.characterID(w, r)
	if !ok || !h.worldOnly(w) {
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The query could not be read.")
		return
	}
	lq := LogQuery{Cursor: q.Get("cursor")}
	if lq.Limit, ok = pageLimit(q, DefaultLogPage, MaxLogPage); !ok {
		h.fail(w, http.StatusBadRequest, "bad_request", "limit is 1 to "+strconv.Itoa(MaxLogPage)+".")
		return
	}
	for _, f := range []struct {
		key string
		to  *time.Time
	}{{"since", &lq.Since}, {"until", &lq.Until}} {
		v := q.Get(f.key)
		if v == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			t, err = time.Parse(time.DateOnly, v)
		}
		if err != nil {
			h.fail(w, http.StatusBadRequest, "bad_request", f.key+" is a date, such as 2026-10-12, or an RFC 3339 time.")
			return
		}
		*f.to = t
	}
	logs, err := h.cfg.World.CharacterLogs(ctx, id, lq)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.write(w, http.StatusOK, logs)
}

func (h *Handler) itemHistory(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	serial, ok := item.ParseSerial(r.PathValue("serial"))
	if !ok {
		h.fail(w, http.StatusBadRequest, "bad_request", "That is not an item serial.")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	hist, err := h.cfg.World.ItemHistory(ctx, serial)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.write(w, http.StatusOK, hist)
}

func (h *Handler) restore(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	req := &adminv1.RestoreRequest{}
	if !h.read(w, r, req) {
		return
	}
	bad := func(msg string) { h.fail(w, http.StatusBadRequest, "bad_request", msg) }
	reason := strings.TrimSpace(req.GetReason())
	switch {
	case reason == "" || !validReason(reason):
		bad("A restore needs a reason of at most 255 characters.")
		return
	case req.GetCharacterId() < 1:
		bad("character_id is the character that gets the items.")
		return
	case len(req.GetItems()) == 0 && req.GetZeny() == 0:
		bad("A restore gives back items, zeny or both.")
		return
	case len(req.GetItems()) > MaxRestoreItems:
		bad("A restore gives back at most " + strconv.Itoa(MaxRestoreItems) + " items.")
		return
	case req.GetZeny() < 0 || req.GetZeny() > 2147483647:
		bad("zeny is 0 to 2147483647.")
		return
	}
	mayCreate := h.cfg.Roles.Has(c.role, "items.create")
	if req.GetZeny() > 0 && !mayCreate {
		h.fail(w, http.StatusForbidden, "forbidden", "Zeny is made for a restore, which needs items.create.")
		return
	}
	rr := Restore{AccountID: c.sess.AccountID, AccountName: c.sess.AccountName, Role: c.role, Character: req.GetCharacterId(),
		Zeny: req.GetZeny(), Reason: reason, MayCreate: mayCreate}
	for _, it := range req.GetItems() {
		s, ok := item.ParseSerial(it.GetSerial())
		if !ok {
			bad("Each item names a serial, such as 0192f0a8-1234-7abc-8def-0123456789ab.")
			return
		}
		if it.GetAmount() < 0 || it.GetAmount() > item.MaxAmount {
			bad("amount is 0, for the whole stack, to " + strconv.Itoa(item.MaxAmount) + ".")
			return
		}
		if slices.ContainsFunc(rr.Items, func(o RestoreItem) bool { return o.Serial == s }) {
			bad("Each serial goes in a restore once.")
			return
		}
		rr.Items = append(rr.Items, RestoreItem{Serial: s, Amount: int(it.GetAmount())})
	}
	if !h.worldOnly(w) {
		return
	}
	res, err := h.cfg.World.Restore(ctx, rr)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.log.Info("items restored", "by", c.sess.AccountID, "character", rr.Character, "mail", res.GetMailId())
	h.write(w, http.StatusOK, res)
}

func (h *Handler) review(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		h.fail(w, http.StatusBadRequest, "bad_request", "That is not an audit row ID.")
		return
	}
	req := &adminv1.ReviewRequest{}
	if !h.read(w, r, req) {
		return
	}
	note := strings.TrimSpace(req.GetNote())
	switch {
	case req.GetVerdict() != "ok" && req.GetVerdict() != "flag":
		h.fail(w, http.StatusBadRequest, "bad_request", "verdict is ok or flag.")
		return
	case !validReason(note):
		h.fail(w, http.StatusBadRequest, "bad_request", "The note is at most 255 characters.")
		return
	case req.GetVerdict() == "flag" && note == "":
		h.fail(w, http.StatusBadRequest, "bad_request", "A flag needs a note that says what is wrong.")
		return
	}
	row, err := h.q.AuditRowForReview(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		h.fail(w, http.StatusNotFound, "not_found", "There is no audit row with that ID.")
		return
	case err != nil:
		h.unavailable(w, r, err)
		return
	case row.AccountID == c.sess.AccountID:
		h.fail(w, http.StatusConflict, "conflict", "No one reviews their own action.")
		return
	case !row.NeedsReview:
		h.fail(w, http.StatusConflict, "conflict", "That row is not marked for review.")
		return
	case row.Reviewed:
		h.fail(w, http.StatusConflict, "conflict", "That row has a review already.")
		return
	}
	err = pgx.BeginFunc(ctx, h.cfg.Pool, func(tx pgx.Tx) error {
		q := h.q.WithTx(tx)
		n, err := q.AddStaffAuditReview(ctx, db.AddStaffAuditReviewParams{AuditID: id, ReviewerID: c.sess.AccountID,
			ReviewerName: c.sess.AccountName, Verdict: req.GetVerdict(), Note: note})
		if err != nil {
			return err
		}
		if n == 0 {
			return errReviewed
		}
		return h.writeAudit(ctx, q, c, "audit.review", target{name: strconv.FormatInt(id, 10)},
			map[string]any{"audit_id": id, "verdict": req.GetVerdict()}, note, false)
	})
	if errors.Is(err, errReviewed) {
		h.fail(w, http.StatusConflict, "conflict", "That row has a review already.")
		return
	}
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	entry, err := h.q.StaffAuditEntry(ctx, id)
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	e, err := auditEntry(entry)
	if err != nil {
		h.unavailable(w, r, err)
		return
	}
	h.write(w, http.StatusOK, e)
}

// errReviewed is a review that another review beat.
var errReviewed = errors.New("admin: the row has a review")

// writeAudit writes an audit row of the API with q.
func (h *Handler) writeAudit(ctx context.Context, q *db.Queries, c caller, action string, t target, args map[string]any,
	reason string, needsReview bool) error {
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	_, err = q.AddStaffAudit(ctx, db.AddStaffAuditParams{
		AccountID: c.sess.AccountID, AccountName: c.sess.AccountName, Role: c.role, Action: action, Source: "admin",
		Target: t.name, TargetAccountID: pgtype.Int8{Int64: t.id, Valid: t.id > 0}, Arguments: b, Reason: reason,
		NeedsReview: needsReview,
	})
	return err
}
