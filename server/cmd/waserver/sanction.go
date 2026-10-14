package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/adminworld"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// devAccount is the staff account of the dev endpoints' audit rows. It
// is no account of the database, so no test character is its own.
const devAccount = 1 << 40

// devStaffHandler gives an online character's account a staff role, as
// if it logged in with a second factor, for the GM command tests
// (decision 0030): POST /debug/wathena/staff?name=<character>&role=<role>.
// An empty role takes it away. It needs --dev.
func devStaffHandler(hub *chat.Hub, reg *staff.Registry) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		q := r.URL.Query()
		role := q.Get("role")
		if role != "" && (role == staff.Owner || !reg.Roles().Exists(role)) {
			http.Error(rw, "role is a role of the config", http.StatusBadRequest)
			return
		}
		err := zone.DevStaffRole(ctx, hub, reg, q.Get("name"), role)
		if errors.Is(err, zone.ErrNotOnline) {
			http.Error(rw, "no such character online", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(rw, err.Error(), http.StatusServiceUnavailable)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte("{}\n"))
	}
}

// devSanctionHandler gives a sanction as the admin API's POST
// /sanctions does, for the end-to-end tests (decision 0030): POST
// /debug/wathena/sanction?kind=<kind>&name=<character>&minutes=<n>
// &reason=<text>. A ban bans the character's account. It runs as a dev
// staff account. It needs --dev and a database.
func devSanctionHandler(aw *adminworld.World, st *store.Store) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		q := r.URL.Query()
		minutes, err := strconv.Atoi(q.Get("minutes"))
		if q.Get("minutes") == "" {
			minutes, err = 0, nil
		}
		if err != nil || minutes < 0 {
			http.Error(rw, "minutes is a number", http.StatusBadRequest)
			return
		}
		c, err := st.Queries().CharacterByName(ctx, q.Get("name"))
		if err != nil {
			http.Error(rw, "no such character", http.StatusNotFound)
			return
		}
		o := admin.SanctionOrder{Actor: admin.Actor{AccountID: devAccount, AccountName: "dev", Role: staff.Owner}, Kind: q.Get("kind"),
			Minutes: minutes, Reason: q.Get("reason")}
		if o.Kind == "ban" {
			o.Account = c.AccountID
		} else {
			o.Character = c.ID
		}
		s, err := aw.Sanction(ctx, o)
		out := map[string]any{}
		var we *admin.WorldError
		switch {
		case errors.As(err, &we):
			out["error"] = we.Code
		case err != nil:
			http.Error(rw, err.Error(), http.StatusServiceUnavailable)
			return
		default:
			out["id"] = s.GetId()
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(out)
	}
}

// devDeletionsHandler runs the account deletion job at once, on the
// server clock, for the end-to-end tests (decision 0032): POST
// /debug/wathena/deletions. It answers with how many accounts it
// deleted. It needs --dev and a database.
func devDeletionsHandler(aw *adminworld.World) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		n, err := aw.DeleteDue(ctx)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusServiceUnavailable)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]int{"deleted": n})
	}
}
