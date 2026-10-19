package adminworld

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// Moderation (proposal 0002 review decision 35, decision 0030):
//   - Sanctions from the admin API and from GM commands take the same
//     way. A mute or a jail of an online character starts in its zone,
//     then its row and audit row are written. An offline character is
//     held in the store so it cannot log in while its row is written.
//   - A kick closes the character's connection through the Kicker. A
//     ban is a row of the account, then every character of the account
//     is kicked. The gateway refuses the account's game logins, and the
//     account service its logins and tickets, while the ban lasts.
//   - Lifting writes the row first, then takes the sanction off an
//     online character in its zone.

// Kicker closes game connections. In the all-in-one build it is the
// gateway, which also ends the characters' autotrade.
type Kicker interface {
	Kick(ctx context.Context, ids []int64, reason wathenav1.DisconnectReason, text string) []int64
}

var _ zone.Moderator = (*World)(nil)

// Retries of a sanction whose character moves between zones, or logs in
// or out, while the sanction looks for it.
const (
	sanctionTries = 8
	sanctionWait  = 50 * time.Millisecond
)

func (w *World) now() time.Time {
	if w.cfg.Clock != nil {
		return w.cfg.Clock()
	}
	return time.Now()
}

func werr(code, msg string) error { return &admin.WorldError{Code: code, Message: msg} }

// order is a sanction of either source.
type order struct {
	actor     admin.Actor
	source    string
	kind      string
	character int64
	account   int64
	minutes   int
	reason    string
}

// audit is the audit row of an order.
func (o order) audit(action, target string, targetAccount int64, args map[string]any) store.AuditRow {
	return store.AuditRow{AccountID: o.actor.AccountID, AccountName: o.actor.AccountName, Role: o.actor.Role, Action: action,
		Source: o.source, Target: target, TargetAccount: targetAccount, Arguments: args, Reason: o.reason}
}

// Sanction gives a sanction of the admin API.
func (w *World) Sanction(ctx context.Context, o admin.SanctionOrder) (*adminv1.Sanction, error) {
	row, err := w.sanction(ctx, order{actor: o.Actor, source: "admin", kind: o.Kind, character: o.Character, account: o.Account,
		minutes: o.Minutes, reason: o.Reason})
	if err != nil {
		return nil, err
	}
	return w.sanctionMessage(row), nil
}

// Command gives the sanction of @mute, @jail or @kick, for an online
// player found by name.
func (w *World) Command(ctx context.Context, o zone.StaffOrder) (string, error) {
	if w.cfg.Chat == nil {
		return "", &zone.CommandError{Text: "Character not found."}
	}
	f, ok := w.cfg.Chat.Find(o.Target)
	if !ok {
		return "", &zone.CommandError{Text: "Character not found."}
	}
	if f.CharacterID <= 0 {
		// A character of a world without a database has no ID.
		return "", &zone.CommandError{Text: "Sanctions need the game's database."}
	}
	row, err := w.sanction(ctx, order{actor: admin.Actor{AccountID: o.Actor.AccountID, AccountName: o.Actor.AccountName, Role: o.Actor.Role},
		source: "game", kind: o.Kind, character: f.CharacterID, minutes: o.Minutes, reason: o.Reason})
	var we *admin.WorldError
	if errors.As(err, &we) {
		return "", &zone.CommandError{Text: we.Message}
	}
	if err != nil {
		w.log.Error("a GM command's sanction failed", "kind", o.Kind, "err", err)
		return "", err
	}
	switch o.Kind {
	case store.SanctionKick:
		return "Kicked " + row.Character + ".", nil
	case store.SanctionMute:
		return fmt.Sprintf("Muted %s for %d minutes.", row.Character, o.Minutes), nil
	}
	return fmt.Sprintf("Jailed %s for %d minutes.", row.Character, o.Minutes), nil
}

// sanction gives a sanction and returns its row.
func (w *World) sanction(ctx context.Context, o order) (store.SanctionRow, error) {
	switch o.kind {
	case store.SanctionMute, store.SanctionJail:
		return w.characterSanction(ctx, o)
	case store.SanctionKick:
		return w.kick(ctx, o)
	case store.SanctionBan:
		return w.ban(ctx, o)
	}
	return store.SanctionRow{}, werr("bad_request", "kind is mute, jail, kick or ban.")
}

// selfCheck refuses a sanction of the staff member's own account.
func selfCheck(o order, account int64) error {
	if account == o.actor.AccountID {
		return werr("conflict", "You cannot sanction your own account.")
	}
	return nil
}

// targetCheck refuses a sanction of the staff member's own account, and
// of an account that outranks the staff member (review decision 51).
func (w *World) targetCheck(ctx context.Context, o order, account int64, name string) error {
	if err := selfCheck(o, account); err != nil {
		return err
	}
	return w.rankCheck(ctx, o.actor, account, name)
}

// rankCheck refuses a sanction, or the lift of one, of an account that
// the actor's role does not cover, as role changes are refused
// (server/admin's findTarget):
//   - Only an owner acts on an owner.
//   - Any other staff account needs a role of the actor that has every
//     permission of its role.
func (w *World) rankCheck(ctx context.Context, actor admin.Actor, account int64, name string) error {
	if actor.Role == staff.Owner {
		return nil
	}
	var roles *staff.Roles
	if reg := w.cfg.Staff; reg != nil {
		if name == "" && w.cfg.Store != nil {
			n, err := w.cfg.Store.AccountName(ctx, account)
			if err != nil && !errors.Is(err, store.ErrNoRow) {
				return err
			}
			name = n
		}
		if reg.IsOwner(account, name) {
			return werr("forbidden", "That account is an owner. Only an owner can sanction an owner.")
		}
		roles = reg.Roles()
	}
	if w.cfg.Store == nil {
		return nil
	}
	role, err := w.cfg.Store.StaffRoleOf(ctx, account)
	if err != nil {
		return err
	}
	// A role the config no longer has gives no permissions, so it does
	// not protect its account either.
	if role == "" || !roles.Exists(role) {
		return nil
	}
	if !roles.Covers(actor.Role, role) {
		return werr("forbidden", "That account's staff role has permissions your role does not have.")
	}
	return nil
}

func (w *World) characterSanction(ctx context.Context, o order) (store.SanctionRow, error) {
	for range sanctionTries {
		zones, err := w.locate(ctx, []int64{o.character})
		if err != nil {
			return store.SanctionRow{}, err
		}
		if z := zones[o.character]; z != nil {
			row, done, err := w.onlineSanction(ctx, z, o)
			if done || err != nil {
				return row, err
			}
		} else {
			if w.cfg.Store == nil {
				return store.SanctionRow{}, werr("not_found", "That character is not online.")
			}
			row, done, err := w.offlineSanction(ctx, o)
			if done || err != nil {
				return row, err
			}
		}
		select {
		case <-ctx.Done():
			return store.SanctionRow{}, ctx.Err()
		case <-time.After(sanctionWait):
		}
	}
	return store.SanctionRow{}, werr("conflict", "The character keeps moving. Try again.")
}

// onlineSanction starts a mute or a jail in the character's zone, then
// writes its row. done is false when the character left the zone.
func (w *World) onlineSanction(ctx context.Context, z *zone.Zone, o order) (store.SanctionRow, bool, error) {
	var account int64
	if w.cfg.Store != nil {
		p, err := w.cfg.Store.Place(ctx, o.character)
		if err != nil {
			return store.SanctionRow{}, true, err
		}
		account = p.AccountID
		if err := w.targetCheck(ctx, o, account, p.AccountName); err != nil {
			return store.SanctionRow{}, true, err
		}
	}
	s, name, found, err := z.StaffSanction(ctx, o.character, o.kind, o.minutes, o.reason, o.actor.AccountName)
	if err != nil || !found {
		return store.SanctionRow{}, false, err
	}
	row := store.SanctionRow{CharacterID: o.character, Character: name, AccountID: account, Kind: o.kind, Reason: o.reason,
		Start: s.Start, End: s.End, By: o.actor.AccountName}
	if w.cfg.Store == nil {
		return row, true, nil
	}
	row.ID, err = w.cfg.Store.AddCharacterSanction(ctx, store.CharacterSanction{Character: o.character, Kind: o.kind, Reason: o.reason,
		Start: s.Start, End: s.End, By: o.actor.AccountName, ReturnMap: s.ReturnMap, ReturnX: s.ReturnX, ReturnY: s.ReturnY},
		o.audit("moderation."+o.kind, name, account, map[string]any{"character_id": o.character, "minutes": o.minutes}))
	return row, true, err
}

// offlineSanction writes the row of a mute or a jail of an offline
// character, which loads it at its next login. done is false when the
// character has a session.
func (w *World) offlineSanction(ctx context.Context, o order) (store.SanctionRow, bool, error) {
	st := w.cfg.Store
	hold, err := st.Hold(o.character)
	if errors.Is(err, store.ErrLive) {
		return store.SanctionRow{}, false, nil
	}
	if err != nil {
		return store.SanctionRow{}, true, err
	}
	defer hold.Release()
	p, err := st.Place(ctx, o.character)
	if errors.Is(err, store.ErrNotFound) {
		return store.SanctionRow{}, true, werr("not_found", "There is no such character.")
	}
	if err != nil {
		return store.SanctionRow{}, true, err
	}
	if err := w.targetCheck(ctx, o, p.AccountID, p.AccountName); err != nil {
		return store.SanctionRow{}, true, err
	}
	now := w.now()
	c := store.CharacterSanction{Character: o.character, Kind: o.kind, Reason: o.reason, Start: now,
		End: now.Add(time.Duration(o.minutes) * time.Minute), By: o.actor.AccountName}
	if o.kind == character.SanctionJail {
		// The jail sends the character from where it was saved, and back
		// there when it ends.
		c.ReturnMap, c.ReturnX, c.ReturnY = p.Map, p.X, p.Y
	}
	id, err := st.AddCharacterSanction(ctx, c, o.audit("moderation."+o.kind, p.Name, p.AccountID,
		map[string]any{"character_id": o.character, "minutes": o.minutes}))
	return store.SanctionRow{ID: id, CharacterID: o.character, Character: p.Name, AccountID: p.AccountID, Account: p.AccountName,
		Kind: o.kind, Reason: o.reason, Start: c.Start, End: c.End, By: c.By}, true, err
}

// kick closes an online character's connection, and records the kick.
func (w *World) kick(ctx context.Context, o order) (store.SanctionRow, error) {
	row := store.SanctionRow{CharacterID: o.character, Kind: store.SanctionKick, Reason: o.reason, By: o.actor.AccountName}
	if st := w.cfg.Store; st != nil {
		p, err := st.Place(ctx, o.character)
		if errors.Is(err, store.ErrNotFound) {
			return row, werr("not_found", "There is no such character.")
		}
		if err != nil {
			return row, err
		}
		row.Character, row.AccountID, row.Account = p.Name, p.AccountID, p.AccountName
		if err := w.targetCheck(ctx, o, p.AccountID, p.AccountName); err != nil {
			return row, err
		}
	}
	if w.cfg.Kicker == nil || len(w.cfg.Kicker.Kick(ctx, []int64{o.character}, wathenav1.DisconnectReason_DISCONNECT_REASON_KICKED,
		staff.KickText(o.reason))) == 0 {
		return row, werr("not_found", "That character is not online.")
	}
	row.Start = w.now()
	row.End = row.Start
	if w.cfg.Store == nil {
		return row, nil
	}
	var err error
	row.ID, err = w.cfg.Store.AddCharacterSanction(ctx, store.CharacterSanction{Character: o.character, Kind: store.SanctionKick,
		Reason: o.reason, Start: row.Start, End: row.End, By: o.actor.AccountName},
		o.audit("moderation.kick", row.Character, row.AccountID, map[string]any{"character_id": o.character}))
	return row, err
}

// ban bans an account, then kicks its characters.
func (w *World) ban(ctx context.Context, o order) (store.SanctionRow, error) {
	st := w.cfg.Store
	if st == nil {
		return store.SanctionRow{}, errNoStore
	}
	name, err := st.AccountName(ctx, o.account)
	if errors.Is(err, store.ErrNoRow) {
		return store.SanctionRow{}, werr("not_found", "There is no such account.")
	}
	if err != nil {
		return store.SanctionRow{}, err
	}
	if err := w.targetCheck(ctx, o, o.account, name); err != nil {
		return store.SanctionRow{}, err
	}
	now := time.Now()
	row := store.SanctionRow{AccountID: o.account, Account: name, Kind: store.SanctionBan, Reason: o.reason, Start: now,
		By: o.actor.AccountName, Permanent: o.minutes == 0}
	if !row.Permanent {
		row.End = now.Add(time.Duration(o.minutes) * time.Minute)
	}
	row.ID, err = st.AddBan(ctx, o.account, o.reason, row.Start, row.End, o.actor.AccountName,
		o.audit("moderation.ban", name, o.account, map[string]any{"minutes": o.minutes, "permanent": row.Permanent}))
	if err != nil {
		return row, err
	}
	ids, err := st.AccountCharacters(ctx, o.account)
	if err != nil {
		return row, err
	}
	if w.cfg.Kicker != nil && len(ids) > 0 {
		w.cfg.Kicker.Kick(ctx, ids, wathenav1.DisconnectReason_DISCONNECT_REASON_BANNED, staff.BanText(o.reason, row.End, row.Permanent))
	}
	// The ban logs the account out of the account pages too, and ends
	// its logins that wait for a second factor (decision 0036). Logins
	// and tickets already refuse a banned account, so a failure here
	// leaves the ban in effect.
	if w.sessions != nil {
		if n, err := w.sessions.EndAccountSessions(ctx, o.account); err != nil {
			w.log.Error("the ban did not end the account's sessions", "account", o.account, "err", err)
		} else if n > 0 {
			w.log.Info("the ban ended the account's sessions", "account", o.account, "sessions", n)
		}
	}
	return row, nil
}

// SanctionByID reads a sanction.
func (w *World) SanctionByID(ctx context.Context, id int64) (*adminv1.Sanction, error) {
	if w.cfg.Store == nil {
		return nil, errNoStore
	}
	row, err := w.cfg.Store.Sanction(ctx, id)
	if errors.Is(err, store.ErrNoRow) {
		return nil, admin.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return w.sanctionMessage(row), nil
}

// LiftSanction ends a mute, a jail or a ban early.
func (w *World) LiftSanction(ctx context.Context, l admin.Lift) (*adminv1.Sanction, error) {
	st := w.cfg.Store
	if st == nil {
		return nil, errNoStore
	}
	row, err := st.Sanction(ctx, l.ID)
	if errors.Is(err, store.ErrNoRow) {
		return nil, admin.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.Kind == store.SanctionKick {
		return nil, werr("bad_request", "A kick cannot be lifted.")
	}
	if row.AccountID == l.AccountID {
		return nil, werr("conflict", "No one lifts a sanction of their own account.")
	}
	// Lifting needs the same rank as giving. A game master cannot lift
	// an owner's ban of an admin, for one.
	if err := w.rankCheck(ctx, l.Actor, row.AccountID, row.Account); err != nil {
		return nil, err
	}
	now := w.now()
	if row.Kind == store.SanctionBan {
		now = time.Now()
	}
	if !row.Active(now) {
		return nil, werr("conflict", "That sanction is not in effect.")
	}
	target := row.Character
	if row.Kind == store.SanctionBan {
		target = row.Account
	}
	o := order{actor: l.Actor, source: "admin", reason: l.Reason}
	ok, err := st.LiftSanction(ctx, l.ID, now, l.AccountName, l.Reason, o.audit("moderation.lift", target, row.AccountID,
		map[string]any{"sanction_id": l.ID, "kind": row.Kind}))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, werr("conflict", "That sanction is not in effect.")
	}
	if row.Kind != store.SanctionBan {
		w.liftInZone(ctx, row)
	}
	row.LiftedAt, row.LiftedBy, row.LiftReason = now, l.AccountName, l.Reason
	return w.sanctionMessage(row), nil
}

// liftInZone takes a lifted mute or jail off an online character. A
// character that is not online loads without it.
func (w *World) liftInZone(ctx context.Context, row store.SanctionRow) {
	for range sanctionTries {
		zones, err := w.locate(ctx, []int64{row.CharacterID})
		if err != nil {
			w.log.Error("cannot find a character to lift a sanction", "character", row.CharacterID, "err", err)
			return
		}
		z := zones[row.CharacterID]
		if z == nil && !w.cfg.Store.Live(row.CharacterID) {
			return
		}
		if z != nil {
			found, err := z.StaffLift(ctx, row.CharacterID, row.Kind, row.Start)
			if err != nil || found {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(sanctionWait):
		}
	}
}

// Sanctions lists sanctions newest first.
func (w *World) Sanctions(ctx context.Context, q admin.SanctionQuery) (*adminv1.SanctionList, error) {
	if w.cfg.Store == nil {
		return nil, errNoStore
	}
	rows, err := w.cfg.Store.SearchSanctions(ctx, store.SanctionQuery{Character: q.Character, Account: q.Account, Kind: q.Kind,
		Active: q.Active, BeforeID: q.BeforeID, Limit: q.Limit + 1})
	if err != nil {
		return nil, err
	}
	out := &adminv1.SanctionList{}
	if len(rows) > q.Limit {
		rows = rows[:q.Limit]
		out.SetNextBeforeId(rows[len(rows)-1].ID)
	}
	list := make([]*adminv1.Sanction, 0, len(rows))
	for _, r := range rows {
		list = append(list, w.sanctionMessage(r))
	}
	out.SetSanctions(list)
	return out, nil
}

// Bans returns the bans in effect of some accounts.
func (w *World) Bans(ctx context.Context, accounts []int64) (map[int64]*adminv1.Sanction, error) {
	out := map[int64]*adminv1.Sanction{}
	if w.cfg.Store == nil || len(accounts) == 0 {
		return out, nil
	}
	rows, err := w.cfg.Store.ActiveBans(ctx, accounts)
	if err != nil {
		return nil, err
	}
	for a, r := range rows {
		out[a] = w.sanctionMessage(r)
	}
	return out, nil
}

// sanctionMessage is a sanction row as the API shows it.
func (w *World) sanctionMessage(r store.SanctionRow) *adminv1.Sanction {
	now := w.now()
	if r.Kind == store.SanctionBan {
		now = time.Now()
	}
	m := adminv1.Sanction_builder{Id: r.ID, Kind: r.Kind, Reason: r.Reason, StartedAt: timestamppb.New(r.Start), IssuedBy: r.By,
		CharacterId: r.CharacterID, Character: r.Character, AccountId: r.AccountID, Account: r.Account, Permanent: r.Permanent,
		LiftedBy: r.LiftedBy, LiftReason: r.LiftReason, Active: r.Kind != store.SanctionKick && r.Active(now)}
	if !r.Permanent {
		m.EndsAt = timestamppb.New(r.End)
	}
	if !r.LiftedAt.IsZero() {
		m.LiftedAt = timestamppb.New(r.LiftedAt)
	}
	return m.Build()
}

// Reports lists reports newest first.
func (w *World) Reports(ctx context.Context, q admin.ReportQuery) (*adminv1.ReportPage, error) {
	if w.cfg.Store == nil {
		return nil, errNoStore
	}
	if err := w.cfg.Store.Settle(ctx); err != nil {
		return nil, err
	}
	rows, err := w.cfg.Store.Reports(ctx, store.ReportQuery{Status: q.Status, Target: q.Target, Kind: q.Kind,
		BeforeID: q.BeforeID, Limit: q.Limit + 1})
	if err != nil {
		return nil, err
	}
	out := &adminv1.ReportPage{}
	if len(rows) > q.Limit {
		rows = rows[:q.Limit]
		out.SetNextBeforeId(rows[len(rows)-1].ID)
	}
	list := make([]*adminv1.ReportSummary, 0, len(rows))
	for _, r := range rows {
		list = append(list, reportSummary(r))
	}
	out.SetReports(list)
	return out, nil
}

// earlierReports is how many earlier reports and sanctions of the target
// a report's detail shows.
const earlierReports = 20

// Report reads a report with its chat, the target's earlier reports and
// its sanctions.
func (w *World) Report(ctx context.Context, id int64) (*adminv1.ReportDetail, error) {
	st := w.cfg.Store
	if st == nil {
		return nil, errNoStore
	}
	rep, lines, err := st.Report(ctx, id)
	if errors.Is(err, store.ErrNoRow) {
		return nil, admin.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d := adminv1.ReportDetail_builder{Report: reportSummary(rep), EmblemPng: rep.EmblemPNG}
	for _, ln := range lines {
		d.Lines = append(d.Lines, adminv1.ChatLine_builder{Id: ln.ID.String(), At: timestamppb.New(ln.At), Kind: ln.Kind,
			CharacterId: ln.CharacterID, Name: ln.Name, Target: ln.To, Map: strings.TrimPrefix(ln.Map, "map/"), Text: ln.Text,
			Reported: !rep.Line.IsZero() && ln.ID == rep.Line, Seen: ln.Seen}.Build())
	}
	earlier, err := st.Reports(ctx, store.ReportQuery{Target: rep.Target, Limit: earlierReports + 1})
	if err != nil {
		return nil, err
	}
	for _, r := range earlier {
		if r.ID != rep.ID && len(d.Earlier) < earlierReports {
			d.Earlier = append(d.Earlier, reportSummary(r))
		}
	}
	if rep.TargetID > 0 {
		rows, err := st.SearchSanctions(ctx, store.SanctionQuery{Character: rep.TargetID, Limit: earlierReports})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			d.Sanctions = append(d.Sanctions, w.sanctionMessage(r))
		}
	}
	return d.Build(), nil
}

// CloseReport closes an open report with an outcome.
func (w *World) CloseReport(ctx context.Context, c admin.CloseReport) (*adminv1.ReportSummary, error) {
	st := w.cfg.Store
	if st == nil {
		return nil, errNoStore
	}
	rep, _, err := st.Report(ctx, c.ID)
	if errors.Is(err, store.ErrNoRow) {
		return nil, admin.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if rep.Status != "open" {
		return nil, werr("conflict", "That report is closed already.")
	}
	if c.SanctionID > 0 {
		s, err := st.Sanction(ctx, c.SanctionID)
		if errors.Is(err, store.ErrNoRow) {
			return nil, werr("bad_request", "There is no sanction with that ID.")
		}
		if err != nil {
			return nil, err
		}
		if rep.TargetID > 0 && s.CharacterID != rep.TargetID && s.AccountID != rep.TargetAccount {
			return nil, werr("bad_request", "That sanction is not of the reported player.")
		}
	}
	args := map[string]any{"report_id": c.ID, "outcome": c.Outcome}
	if c.SanctionID > 0 {
		args["sanction_id"] = c.SanctionID
	}
	audit := store.AuditRow{AccountID: c.AccountID, AccountName: c.AccountName, Role: c.Role, Action: "moderation.close_report",
		Source: "admin", Target: rep.Target, TargetAccount: rep.TargetAccount, Arguments: args, Reason: c.Note}
	ok, err := st.CloseReport(ctx, c.ID, c.Outcome, c.Note, c.AccountName, c.SanctionID, audit)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, werr("conflict", "That report is closed already.")
	}
	rep, _, err = st.Report(ctx, c.ID)
	if err != nil {
		return nil, err
	}
	return reportSummary(rep), nil
}

func reportSummary(r store.ReportRow) *adminv1.ReportSummary {
	m := adminv1.ReportSummary_builder{Id: r.ID, CreatedAt: timestamppb.New(r.At), Reporter: r.Reporter, ReporterId: r.ReporterID,
		Target: r.Target, TargetId: r.TargetID, Kind: r.Kind(), Reason: r.Reason, Map: strings.TrimPrefix(r.Map, "map/"),
		Status: r.Status, Outcome: r.Outcome, Note: r.Note, ClosedBy: r.By, SanctionId: r.SanctionID, GuildId: r.Guild,
		TargetAccountId: r.TargetAccount}
	if !r.Line.IsZero() {
		m.LineId = r.Line.String()
	}
	if !r.ClosedAt.IsZero() {
		m.ClosedAt = timestamppb.New(r.ClosedAt)
	}
	return m.Build()
}

// banOf is the account's ban in effect to a character's details.
func (w *World) banOf(ctx context.Context, account int64) (*adminv1.Sanction, error) {
	bans, err := w.Bans(ctx, []int64{account})
	if err != nil {
		return nil, err
	}
	return bans[account], nil
}
