package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/adminworld"
	"github.com/lxsmnsyc/wathena/server/world"
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

// maxStall is the longest stall devStallHandler makes.
const maxStall = 5 * time.Second

// devStallHandler holds a zone's goroutine for a while, as a machine
// that stalls does, for the duplication tests (review 2026-10): POST
// /debug/wathena/stall?map=<map>&ms=<n>. Without map it stalls the start
// zone. It answers once the stall ends. With next=1, it answers at once,
// and the zone stalls after it takes its next call from another
// goroutine, before it runs it. It needs --dev.
func devStallHandler(w *world.World) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		ms, err := strconv.Atoi(q.Get("ms"))
		if err != nil || ms < 0 || time.Duration(ms)*time.Millisecond > maxStall {
			http.Error(rw, "ms is 0 to 5000", http.StatusBadRequest)
			return
		}
		z := w.Start()
		if m := q.Get("map"); m != "" {
			if z = w.Zone(m); z == nil {
				http.Error(rw, "no such map", http.StatusNotFound)
				return
			}
		}
		if q.Get("next") == "1" {
			z.StallNextCall(time.Duration(ms) * time.Millisecond)
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte("{}\n"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), maxStall+10*time.Second)
		defer cancel()
		if err := z.Stall(ctx, time.Duration(ms)*time.Millisecond); err != nil {
			http.Error(rw, err.Error(), http.StatusServiceUnavailable)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		_, _ = rw.Write([]byte("{}\n"))
	}
}
