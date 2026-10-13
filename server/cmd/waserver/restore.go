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
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/store"
)

// devRestoreHandler restores a serial to a character by mail, as the
// admin API's POST /restore does, for the duplication tests (decision
// 0029): POST /debug/wathena/restore?serial=<serial>&to=<character>
// &amount=<n>. It runs as a dev staff account without items.create, so
// it never makes copies. It needs --dev and a database.
func devRestoreHandler(aw *adminworld.World, st *store.Store) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		q := r.URL.Query()
		serial, ok := item.ParseSerial(q.Get("serial"))
		if !ok {
			http.Error(rw, "serial is an item serial", http.StatusBadRequest)
			return
		}
		amount := 0
		if v := q.Get("amount"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				http.Error(rw, "amount is a number", http.StatusBadRequest)
				return
			}
			amount = n
		}
		to, err := st.Queries().CharacterByName(ctx, q.Get("to"))
		if err != nil {
			http.Error(rw, "no such character", http.StatusNotFound)
			return
		}
		res, err := aw.Restore(ctx, admin.Restore{AccountID: 1, AccountName: "dev", Role: "owner", Character: to.ID,
			Items: []admin.RestoreItem{{Serial: serial, Amount: amount}}, Reason: "duplication test"})
		out := map[string]any{}
		var we *admin.WorldError
		switch {
		case errors.As(err, &we):
			out["error"] = we.Code
		case err != nil:
			http.Error(rw, err.Error(), http.StatusServiceUnavailable)
			return
		default:
			out["mail"] = res.GetMailId()
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(out)
	}
}
