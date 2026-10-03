package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/store"
)

// devClock is the server clock of --dev: the system clock plus an
// offset that POST /debug/wathena/clock moves forward, so tests can end
// a mute or reach a schedule without waiting (decision 0018).
type devClock struct{ offset atomic.Int64 }

// Now is the system clock plus the offset.
func (c *devClock) Now() time.Time { return time.Now().Add(time.Duration(c.offset.Load())) }

// clockHandler moves the dev clock forward: POST
// /debug/wathena/clock?advance=90s. It answers with the new time.
func clockHandler(c *devClock) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		d, err := time.ParseDuration(r.URL.Query().Get("advance"))
		if err != nil || d < 0 || d > 366*24*time.Hour {
			http.Error(rw, "advance must be a duration from 0 to 8784h, such as 90s", http.StatusBadRequest)
			return
		}
		c.offset.Add(int64(d))
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]string{"now": c.Now().UTC().Format(time.RFC3339Nano)})
	}
}

// reportLine is a chat line of a report, as JSON.
type reportLine struct {
	ID   string    `json:"id"`
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
	Name string    `json:"name"`
	To   string    `json:"to,omitempty"`
	Map  string    `json:"map"`
	Text string    `json:"text"`
}

// reportJSON is a report, as GET /debug/wathena/reports shows it.
type reportJSON struct {
	ID       int64        `json:"id,omitempty"`
	At       time.Time    `json:"at"`
	Reporter string       `json:"reporter"`
	Target   string       `json:"target"`
	Line     string       `json:"line,omitempty"`
	Reason   string       `json:"reason"`
	Map      string       `json:"map"`
	Status   string       `json:"status"`
	Lines    []reportLine `json:"lines"`
}

func toReportJSON(id int64, status string, r chat.Report) reportJSON {
	out := reportJSON{ID: id, At: r.At, Reporter: r.Reporter, Target: r.Target, Reason: r.Reason, Map: r.Map, Status: status, Lines: []reportLine{}}
	if !r.Line.IsZero() {
		out.Line = r.Line.String()
	}
	for _, l := range r.Context {
		out.Lines = append(out.Lines, reportLine{ID: l.ID.String(), At: l.At, Kind: l.Kind, Name: l.Name, To: l.To, Map: l.Map, Text: l.Text})
	}
	return out
}

// reportsHandler lists the newest reports with their chat, for
// development and the end-to-end tests: GET /debug/wathena/reports. It
// shows chat, so it is for --dev only.
func reportsHandler(memory *chat.MemoryLog, db *store.ChatLog) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		out := []reportJSON{}
		switch {
		case db != nil:
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			list, err := db.Reports(ctx, 100)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusServiceUnavailable)
				return
			}
			for _, s := range list {
				out = append(out, toReportJSON(s.ID, s.Status, s.Report))
			}
		case memory != nil:
			list := memory.Reports()
			for i := len(list) - 1; i >= 0; i-- {
				out = append(out, toReportJSON(0, "open", list[i]))
			}
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(out)
	}
}
