package admin

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
)

// The report queue and the sanctions (proposal 0002 review decision 35,
// decision 0030):
//   - Reports and sanctions are game data, so World reads and writes
//     them, with the audit row of each change in its transaction.
//   - POST /sanctions needs the permission of its kind: moderation.mute,
//     moderation.jail, moderation.kick or moderation.ban. A ban also
//     needs a recent login. A permanent ban, and a ban longer than
//     LongBanMinutes, also need moderation.ban_permanent (review
//     decision 51).
//   - DELETE /sanctions/{id} needs the permission of the sanction's kind,
//     and a recent login for a ban. Lifting a permanent ban, or a ban
//     longer than LongBanMinutes, also needs moderation.ban_permanent
//     (review decisions 40 and 51).
//   - The world refuses a sanction, or a lift, of an owner by a non-owner,
//     and of a staff account whose role the actor's role does not cover
//     (review decision 51).
//   - GET /sanctions needs any permission of the moderation group.

// Limits of the moderation calls.
const (
	DefaultReportPage = 50
	MaxReportPage     = 100
	DefaultSanctions  = 50
	MaxSanctions      = 100
	// MaxSanctionMinutes is the longest mute or jail: a year.
	MaxSanctionMinutes = 525_600
	// MaxBanMinutes is the longest ban with an end: 10 years.
	MaxBanMinutes = 5_256_000
	// LongBanMinutes is the longest ban that moderation.ban gives or
	// lifts alone: a year, the longest mute or jail. A longer ban acts
	// as a permanent one, so it needs moderation.ban_permanent too
	// (review decision 51).
	LongBanMinutes = MaxSanctionMinutes
)

// longBan reports whether a ban needs moderation.ban_permanent: it has
// no end, or lasts longer than LongBanMinutes.
func longBan(s *adminv1.Sanction) bool {
	if s.GetPermanent() {
		return true
	}
	return s.GetEndsAt().AsTime().Sub(s.GetStartedAt().AsTime()) > LongBanMinutes*time.Minute
}

// kindPermission is the permission of a sanction's kind.
var kindPermission = map[string]string{
	"mute": "moderation.mute",
	"jail": "moderation.jail",
	"kick": "moderation.kick",
	"ban":  "moderation.ban",
}

// actor is the caller as a World call's actor.
func (c caller) actor() Actor {
	return Actor{AccountID: c.sess.AccountID, AccountName: c.sess.AccountName, Role: c.role}
}

// pathID reads a positive ID of the path.
func pathID(r *http.Request, key string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(key), 10, 64)
	return id, err == nil && id > 0
}

// queryID reads an optional positive ID of the query.
func queryID(q url.Values, key string) (int64, bool) {
	v := q.Get(key)
	if v == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(v, 10, 64)
	return id, err == nil && id > 0
}

func (h *Handler) searchReports(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The query could not be read.")
		return
	}
	bad := func(msg string) { h.fail(w, http.StatusBadRequest, "bad_request", msg) }
	rq := ReportQuery{Status: "open"}
	switch v := q.Get("status"); v {
	case "", "open":
	case "closed":
		rq.Status = v
	case "all":
		rq.Status = ""
	default:
		bad("status is open, closed or all.")
		return
	}
	switch v := q.Get("kind"); v {
	case "", "player", "line", "guild_name", "guild_emblem":
		rq.Kind = v
	default:
		bad("kind is player, line, guild_name or guild_emblem.")
		return
	}
	rq.Target = strings.TrimSpace(q.Get("target"))
	if utf8.RuneCountInString(rq.Target) > 24 || !utf8.ValidString(rq.Target) {
		bad("target is a name of at most 24 characters.")
		return
	}
	var ok bool
	if rq.BeforeID, ok = queryID(q, "before_id"); !ok {
		bad("before_id is a report ID.")
		return
	}
	if rq.Limit, ok = pageLimit(q, DefaultReportPage, MaxReportPage); !ok {
		bad("limit is 1 to " + strconv.Itoa(MaxReportPage) + ".")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	page, err := h.cfg.World.Reports(ctx, rq)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.write(w, http.StatusOK, page)
}

func (h *Handler) report(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	id, ok := pathID(r, "id")
	if !ok {
		h.fail(w, http.StatusBadRequest, "bad_request", "That is not a report ID.")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	d, err := h.cfg.World.Report(ctx, id)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	// A report shows the target's chat, whispers too, so the audit log
	// keeps each reading (review decision 51).
	rep := d.GetReport()
	if err := h.writeAudit(ctx, h.q, c, "reports.view", target{id: rep.GetTargetAccountId(), name: rep.GetTarget()},
		map[string]any{"report_id": id, "lines": len(d.GetLines())}, "", false); err != nil {
		h.unavailable(w, r, err)
		return
	}
	h.write(w, http.StatusOK, d)
}

func (h *Handler) closeReport(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	id, ok := pathID(r, "id")
	if !ok {
		h.fail(w, http.StatusBadRequest, "bad_request", "That is not a report ID.")
		return
	}
	req := &adminv1.CloseReportRequest{}
	if !h.read(w, r, req) {
		return
	}
	bad := func(msg string) { h.fail(w, http.StatusBadRequest, "bad_request", msg) }
	note := strings.TrimSpace(req.GetNote())
	switch {
	case req.GetOutcome() != "no_action" && req.GetOutcome() != "warned" && req.GetOutcome() != "sanctioned":
		bad("outcome is no_action, warned or sanctioned.")
		return
	case !validReason(note):
		bad("The note is at most 255 characters.")
		return
	case req.GetOutcome() == "sanctioned" && req.GetSanctionId() < 1:
		bad("A sanctioned report links its sanction: sanctionId.")
		return
	case req.GetOutcome() != "sanctioned" && req.GetSanctionId() != 0:
		bad("Only a sanctioned report links a sanction.")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	s, err := h.cfg.World.CloseReport(ctx, CloseReport{Actor: c.actor(), ID: id, Outcome: req.GetOutcome(), Note: note,
		SanctionID: req.GetSanctionId()})
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.log.Info("report closed", "by", c.sess.AccountID, "report", id, "outcome", req.GetOutcome())
	h.write(w, http.StatusOK, s)
}

func (h *Handler) sanction(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	req := &adminv1.SanctionRequest{}
	if !h.read(w, r, req) {
		return
	}
	bad := func(msg string) { h.fail(w, http.StatusBadRequest, "bad_request", msg) }
	kind, reason, minutes := req.GetKind(), strings.TrimSpace(req.GetReason()), int(req.GetMinutes())
	perm, known := kindPermission[kind]
	if !known {
		bad("kind is mute, jail, kick or ban.")
		return
	}
	switch {
	case reason == "" || !validReason(reason):
		bad("A sanction needs a reason of at most 255 characters. The player sees it.")
		return
	case kind == "ban" && (req.GetAccountId() < 1 || req.GetCharacterId() != 0):
		bad("A ban names an account: accountId.")
		return
	case kind != "ban" && (req.GetCharacterId() < 1 || req.GetAccountId() != 0):
		bad("A " + kind + " names a character: characterId.")
		return
	case kind == "kick" && minutes != 0:
		bad("A kick takes no minutes.")
		return
	case (kind == "mute" || kind == "jail") && (minutes < 1 || minutes > MaxSanctionMinutes):
		bad("minutes is 1 to " + strconv.Itoa(MaxSanctionMinutes) + ".")
		return
	case kind == "ban" && (minutes < 0 || minutes > MaxBanMinutes):
		bad("minutes is 1 to " + strconv.Itoa(MaxBanMinutes) + ", or 0 for a permanent ban.")
		return
	}
	if !h.need(w, c, perm, kind == "ban") {
		return
	}
	if kind == "ban" && (minutes == 0 || minutes > LongBanMinutes) && !h.need(w, c, "moderation.ban_permanent", true) {
		return
	}
	if !h.worldOnly(w) {
		return
	}
	s, err := h.cfg.World.Sanction(ctx, SanctionOrder{Actor: c.actor(), Kind: kind, Character: req.GetCharacterId(),
		Account: req.GetAccountId(), Minutes: minutes, Reason: reason})
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.log.Info("sanction given", "by", c.sess.AccountID, "kind", kind, "sanction", s.GetId())
	h.write(w, http.StatusOK, s)
}

func (h *Handler) liftSanction(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	id, ok := pathID(r, "id")
	if !ok {
		h.fail(w, http.StatusBadRequest, "bad_request", "That is not a sanction ID.")
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The query could not be read.")
		return
	}
	reason := strings.TrimSpace(q.Get("reason"))
	if reason == "" || !validReason(reason) {
		h.fail(w, http.StatusBadRequest, "bad_request", "Lifting a sanction needs a reason of at most 255 characters.")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	s, err := h.cfg.World.SanctionByID(ctx, id)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	perm, known := kindPermission[s.GetKind()]
	if !known || s.GetKind() == "kick" {
		h.fail(w, http.StatusBadRequest, "bad_request", "A kick cannot be lifted.")
		return
	}
	if !h.need(w, c, perm, s.GetKind() == "ban") {
		return
	}
	if s.GetKind() == "ban" && longBan(s) && !h.need(w, c, "moderation.ban_permanent", true) {
		return
	}
	s, err = h.cfg.World.LiftSanction(ctx, Lift{Actor: c.actor(), ID: id, Reason: reason})
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.log.Info("sanction lifted", "by", c.sess.AccountID, "sanction", id)
	h.write(w, http.StatusOK, s)
}

func (h *Handler) searchSanctions(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The query could not be read.")
		return
	}
	bad := func(msg string) { h.fail(w, http.StatusBadRequest, "bad_request", msg) }
	var sq SanctionQuery
	var ok bool
	if sq.Character, ok = queryID(q, "character"); !ok {
		bad("character is a character ID.")
		return
	}
	if sq.Account, ok = queryID(q, "account"); !ok {
		bad("account is an account ID.")
		return
	}
	if v := q.Get("kind"); v != "" {
		if _, known := kindPermission[v]; !known {
			bad("kind is mute, jail, kick or ban.")
			return
		}
		sq.Kind = v
	}
	switch q.Get("active") {
	case "", "false":
	case "true":
		sq.Active = true
	default:
		bad("active is true or false.")
		return
	}
	if sq.BeforeID, ok = queryID(q, "before_id"); !ok {
		bad("before_id is a sanction ID.")
		return
	}
	if sq.Limit, ok = pageLimit(q, DefaultSanctions, MaxSanctions); !ok {
		bad("limit is 1 to " + strconv.Itoa(MaxSanctions) + ".")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	list, err := h.cfg.World.Sanctions(ctx, sq)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.write(w, http.StatusOK, list)
}
