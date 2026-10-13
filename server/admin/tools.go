package admin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
)

// The world, events and mods tools (proposal 0002 review decision 36,
// decision 0031):
//   - GET /world (world.read): the zones with their last minute of
//     ticks, the process, the players online and the planned restart.
//   - POST /announce (world.announce): a line to the world or one map.
//   - POST and DELETE /restart (world.restart, a recent login): plan and
//     cancel a restart.
//   - GET /events, POST /events/{name}/start, /stop and /reset, and
//     PATCH /events/{name} (events.control).
//   - GET /mods (mods.read), and POST /mods/{id}/disable and /enable
//     (mods.disable).
//
// Announcements and restarts write their audit rows here, on the
// account role. Event and mod changes are game data, so World writes
// them with their audit rows.

// MaxAnnouncement is the longest announcement, in characters.
const MaxAnnouncement = 255

var (
	mapPattern = regexp.MustCompile(`^[A-Za-z0-9_@.-]+(/[A-Za-z0-9_@.-]+)?$`)
	modID      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

// validText checks a text of 1 to max characters on one line.
func validText(s string, maxLen int) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= maxLen && !strings.ContainsAny(s, "\x00\r\n")
}

func validMap(s string) bool { return len(s) <= 64 && mapPattern.MatchString(s) }

func (h *Handler) worldView(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	if !h.worldOnly(w) {
		return
	}
	v, err := h.cfg.World.WorldView(ctx)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.write(w, http.StatusOK, v)
}

func (h *Handler) announce(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	req := &adminv1.AnnounceRequest{}
	if !h.read(w, r, req) {
		return
	}
	text, m := strings.TrimSpace(req.GetText()), strings.TrimSpace(req.GetMap())
	switch {
	case !validText(text, MaxAnnouncement):
		h.fail(w, http.StatusBadRequest, "bad_request", "An announcement is 1 to 255 characters on one line.")
		return
	case m != "" && !validMap(m):
		h.fail(w, http.StatusBadRequest, "bad_request", "map is a map's name, such as prontera, or empty for the world.")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	if err := h.cfg.World.Announce(ctx, text, m); err != nil {
		if errors.Is(err, ErrNotFound) {
			h.fail(w, http.StatusNotFound, "not_found", "That map does not run in this world.")
			return
		}
		h.worldFailed(w, r, err)
		return
	}
	where := m
	if where == "" {
		where = "world"
	}
	if err := h.writeAudit(ctx, h.q, c, "world.announce", target{name: where}, map[string]any{"text": text}, "", false); err != nil {
		h.unavailable(w, r, err)
		return
	}
	h.log.Info("announcement", "by", c.sess.AccountID, "map", where)
	h.write(w, http.StatusOK, adminv1.AnnounceResult_builder{Text: text, Map: m}.Build())
}

func (h *Handler) planRestart(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	req := &adminv1.RestartRequest{}
	if !h.read(w, r, req) {
		return
	}
	reason, message := strings.TrimSpace(req.GetReason()), strings.TrimSpace(req.GetMessage())
	switch {
	case !req.HasAt() || !req.GetAt().IsValid():
		h.fail(w, http.StatusBadRequest, "bad_request", "A restart needs a time: at.")
		return
	case reason == "" || !validReason(reason):
		h.fail(w, http.StatusBadRequest, "bad_request", "A restart needs a reason of at most 255 characters. Players see it.")
		return
	case !validReason(message):
		h.fail(w, http.StatusBadRequest, "bad_request", "The message is at most 255 characters on one line.")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	at := req.GetAt().AsTime()
	p, err := h.cfg.World.PlanRestart(ctx, RestartOrder{Actor: c.actor(), At: at, Reason: reason, Message: message})
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	if err := h.writeAudit(ctx, h.q, c, "world.restart", target{name: "world"},
		map[string]any{"at": at.UTC().Format("2006-01-02T15:04:05Z"), "message": message}, reason, false); err != nil {
		h.unavailable(w, r, err)
		return
	}
	h.log.Info("restart planned", "by", c.sess.AccountID, "at", at.UTC())
	h.write(w, http.StatusOK, p)
}

func (h *Handler) cancelRestart(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		h.fail(w, http.StatusBadRequest, "bad_request", "The query could not be read.")
		return
	}
	reason := strings.TrimSpace(q.Get("reason"))
	if !validReason(reason) {
		h.fail(w, http.StatusBadRequest, "bad_request", "The reason is at most 255 characters.")
		return
	}
	if !h.worldOnly(w) {
		return
	}
	p, err := h.cfg.World.CancelRestart(ctx)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	if err := h.writeAudit(ctx, h.q, c, "world.restart_cancel", target{name: "world"},
		map[string]any{"at": p.GetAt().AsTime().UTC().Format("2006-01-02T15:04:05Z")}, reason, false); err != nil {
		h.unavailable(w, r, err)
		return
	}
	h.log.Info("planned restart cancelled", "by", c.sess.AccountID)
	h.write(w, http.StatusOK, p)
}

func (h *Handler) events(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	if !h.worldOnly(w) {
		return
	}
	list, err := h.cfg.World.Events(ctx)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.write(w, http.StatusOK, list)
}

// eventName reads the event of the path: 1 to 160 characters.
func (h *Handler) eventName(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if !validText(name, 160) {
		h.fail(w, http.StatusBadRequest, "bad_request", "That is not an event's name.")
		return "", false
	}
	return name, true
}

// eventAction is POST /events/{name}/start, /stop and /reset.
func (h *Handler) eventAction(action string) func(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
		name, ok := h.eventName(w, r)
		if !ok {
			return
		}
		req := &adminv1.EventAction{}
		if !h.read(w, r, req) {
			return
		}
		reason := strings.TrimSpace(req.GetReason())
		if !validReason(reason) {
			h.fail(w, http.StatusBadRequest, "bad_request", "The reason is at most 255 characters.")
			return
		}
		h.runEvent(ctx, w, r, EventOrder{Actor: c.actor(), Name: name, Action: action, Reason: reason})
	}
}

func (h *Handler) changeEvent(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	name, ok := h.eventName(w, r)
	if !ok {
		return
	}
	req := &adminv1.EventChange{}
	if !h.read(w, r, req) {
		return
	}
	o := EventOrder{Actor: c.actor(), Name: name, Action: EventChange, Reason: strings.TrimSpace(req.GetReason()),
		DurationMinutes: int(req.GetDurationMinutes())}
	if req.HasOn() {
		on := req.GetOn()
		o.On = &on
	}
	if req.HasTrigger() {
		o.Trigger = req.GetTrigger().AsMap()
	}
	switch {
	case o.On == nil && o.Trigger == nil && o.DurationMinutes == 0:
		h.fail(w, http.StatusBadRequest, "bad_request", "A change sets on, trigger or durationMinutes.")
		return
	case o.DurationMinutes < 0 || o.DurationMinutes > 10080:
		h.fail(w, http.StatusBadRequest, "bad_request", "durationMinutes is 1 to 10080.")
		return
	case !validReason(o.Reason):
		h.fail(w, http.StatusBadRequest, "bad_request", "The reason is at most 255 characters.")
		return
	}
	h.runEvent(ctx, w, r, o)
}

func (h *Handler) runEvent(ctx context.Context, w http.ResponseWriter, r *http.Request, o EventOrder) {
	if !h.worldOnly(w) {
		return
	}
	e, err := h.cfg.World.EventAction(ctx, o)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.log.Info("world event changed", "by", o.AccountID, "event", o.Name, "action", o.Action)
	h.write(w, http.StatusOK, e)
}

func (h *Handler) mods(ctx context.Context, w http.ResponseWriter, r *http.Request, _ caller) {
	if !h.worldOnly(w) {
		return
	}
	list, err := h.cfg.World.Mods(ctx)
	if err != nil {
		h.worldFailed(w, r, err)
		return
	}
	h.write(w, http.StatusOK, list)
}

// switchMod is POST /mods/{id}/disable and /enable.
func (h *Handler) switchMod(off bool) func(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, c caller) {
		id := r.PathValue("id")
		if !modID.MatchString(id) {
			h.fail(w, http.StatusBadRequest, "bad_request", "That is not a mod's ID.")
			return
		}
		req := &adminv1.ModSwitchRequest{}
		if !h.read(w, r, req) {
			return
		}
		m, reason := strings.TrimSpace(req.GetMap()), strings.TrimSpace(req.GetReason())
		switch {
		case id == "core":
			h.fail(w, http.StatusBadRequest, "bad_request", "Core cannot be turned off.")
			return
		case m != "" && !validMap(m):
			h.fail(w, http.StatusBadRequest, "bad_request", "map is a map's name, such as prontera, or empty for every zone.")
			return
		case off && reason == "", !validReason(reason):
			h.fail(w, http.StatusBadRequest, "bad_request", "Turning a mod off needs a reason of at most 255 characters.")
			return
		}
		if !h.worldOnly(w) {
			return
		}
		info, err := h.cfg.World.SwitchMod(ctx, ModOrder{Actor: c.actor(), Mod: id, Map: m, Off: off, Reason: reason})
		if err != nil {
			h.worldFailed(w, r, err)
			return
		}
		h.log.Info("mod switched", "by", c.sess.AccountID, "mod", id, "map", m, "off", off)
		h.write(w, http.StatusOK, info)
	}
}
