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
