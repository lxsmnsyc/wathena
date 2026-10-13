package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/adminworld"
)

// devAnnounceHandler shows a line as the admin API's POST /announce
// does, for the end-to-end tests (decision 0031): POST
// /debug/wathena/announce?text=<text>&map=<map>. Without map it goes to
// the world. It needs --dev.
func devAnnounceHandler(aw *adminworld.World) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		text := q.Get("text")
		if text == "" || len(text) > 255 {
			http.Error(rw, "text is 1 to 255 bytes", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := aw.Announce(ctx, text, q.Get("map")); err != nil {
			http.Error(rw, "no such map", http.StatusNotFound)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte("{}\n"))
	}
}

// devRestartHandler plans a restart as the admin API's POST /restart
// does, for the end-to-end tests (decision 0031): POST
// /debug/wathena/restart?in=<duration>&reason=<text>&message=<text>
// plans one that long from the server clock, and DELETE cancels it. It
// answers with the plan, or {"error": code}. It needs --dev.
func devRestartHandler(aw *adminworld.World) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		var m proto.Message
		var err error
		if r.Method == http.MethodDelete {
			m, err = aw.CancelRestart(ctx)
		} else {
			q := r.URL.Query()
			in, perr := time.ParseDuration(q.Get("in"))
			if perr != nil {
				http.Error(rw, "in is a duration, such as 2m", http.StatusBadRequest)
				return
			}
			m, err = aw.PlanRestart(ctx, admin.RestartOrder{Actor: admin.Actor{AccountID: devAccount, AccountName: "dev", Role: "owner"},
				At: aw.Now().Add(in), Reason: q.Get("reason"), Message: q.Get("message")})
		}
		rw.Header().Set("Content-Type", "application/json")
		var we *admin.WorldError
		if errors.As(err, &we) {
			_ = json.NewEncoder(rw).Encode(map[string]string{"error": we.Code, "message": we.Message})
			return
		}
		if err != nil {
			http.Error(rw, err.Error(), http.StatusServiceUnavailable)
			return
		}
		b, _ := protojson.Marshal(m)
		_, _ = rw.Write(b)
	}
}
