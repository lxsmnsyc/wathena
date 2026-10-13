package admin_test

import (
	"context"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/item"
)

func (w *world) JobName(job int) string {
	if job == 0 {
		return "Novice"
	}
	return ""
}

func (w *world) Characters(_ context.Context, ids []int64) (map[int64]admin.CharacterState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[int64]admin.CharacterState{}
	for _, id := range ids {
		if m, ok := w.online[id]; ok {
			out[id] = admin.CharacterState{Online: true, Position: adminv1.Position_builder{Map: m, X: 1, Y: 2}.Build()}
		}
	}
	return out, nil
}

func (w *world) Character(_ context.Context, id int64) (*adminv1.CharacterDetail, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, online := w.online[id]
	return adminv1.CharacterDetail_builder{Id: id, Name: "from the world", Online: online}.Build(), nil
}

func (w *world) FindSerial(_ context.Context, _ item.Serial) (*adminv1.Owner, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.holder, nil
}

func (w *world) ItemHistory(_ context.Context, s item.Serial) (*adminv1.ItemHistory, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.history == nil {
		return nil, admin.ErrNotFound
	}
	h := w.history
	h.SetSerial(s.String())
	return h, nil
}

func (w *world) CharacterLogs(_ context.Context, id int64, q admin.LogQuery) (*adminv1.CharacterLogs, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logs = append(w.logs, q)
	if id == 404 {
		return nil, admin.ErrNotFound
	}
	return adminv1.CharacterLogs_builder{NextCursor: "next"}.Build(), nil
}

func (w *world) Restore(_ context.Context, r admin.Restore) (*adminv1.RestoreResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.restores = append(w.restores, r)
	if w.restoreErr != nil {
		return nil, w.restoreErr
	}
	return adminv1.RestoreResult_builder{MailId: 7, Zeny: r.Zeny}.Build(), nil
}

// The moderation calls of the fake world (decision 0030). They record
// what the API asked, and answer with what a test set.

func (w *world) Reports(_ context.Context, q admin.ReportQuery) (*adminv1.ReportPage, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reportQueries = append(w.reportQueries, q)
	return adminv1.ReportPage_builder{Reports: []*adminv1.ReportSummary{
		adminv1.ReportSummary_builder{Id: 3, Target: "Bob", Kind: "line", Status: "open"}.Build()}}.Build(), nil
}

func (w *world) Report(_ context.Context, id int64) (*adminv1.ReportDetail, error) {
	if id == 404 {
		return nil, admin.ErrNotFound
	}
	return adminv1.ReportDetail_builder{Report: adminv1.ReportSummary_builder{Id: id, Target: "Bob"}.Build(),
		Lines: []*adminv1.ChatLine{adminv1.ChatLine_builder{Name: "Bob", Text: "rude words", Reported: true}.Build()}}.Build(), nil
}

func (w *world) CloseReport(_ context.Context, c admin.CloseReport) (*adminv1.ReportSummary, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closes = append(w.closes, c)
	if c.ID == 409 {
		return nil, &admin.WorldError{Code: "conflict", Message: "That report is closed already."}
	}
	return adminv1.ReportSummary_builder{Id: c.ID, Status: "closed", Outcome: c.Outcome, SanctionId: c.SanctionID}.Build(), nil
}

func (w *world) Sanction(_ context.Context, o admin.SanctionOrder) (*adminv1.Sanction, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.orders = append(w.orders, o)
	if o.Kind == "kick" && o.Character == 404 {
		return nil, &admin.WorldError{Code: "not_found", Message: "That character is not online."}
	}
	return adminv1.Sanction_builder{Id: 5, Kind: o.Kind, Reason: o.Reason, Permanent: o.Kind == "ban" && o.Minutes == 0}.Build(), nil
}

func (w *world) SanctionByID(_ context.Context, id int64) (*adminv1.Sanction, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	kind, ok := w.sanctionKinds[id]
	if !ok {
		return nil, admin.ErrNotFound
	}
	return adminv1.Sanction_builder{Id: id, Kind: kind, Active: true}.Build(), nil
}

func (w *world) LiftSanction(_ context.Context, l admin.Lift) (*adminv1.Sanction, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lifts = append(w.lifts, l)
	return adminv1.Sanction_builder{Id: l.ID, Kind: w.sanctionKinds[l.ID], LiftReason: l.Reason}.Build(), nil
}

func (w *world) Sanctions(_ context.Context, q admin.SanctionQuery) (*adminv1.SanctionList, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sanctionQueries = append(w.sanctionQueries, q)
	return adminv1.SanctionList_builder{Sanctions: []*adminv1.Sanction{adminv1.Sanction_builder{Id: 1, Kind: "ban"}.Build()}}.Build(), nil
}

func (w *world) Bans(_ context.Context, accounts []int64) (map[int64]*adminv1.Sanction, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[int64]*adminv1.Sanction{}
	for _, a := range accounts {
		if w.banned[a] {
			out[a] = adminv1.Sanction_builder{Kind: "ban", AccountId: a, Active: true}.Build()
		}
	}
	return out, nil
}
